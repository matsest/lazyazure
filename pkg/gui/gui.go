package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/jesseduffield/gocui"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/tasks"
	"github.com/matsest/lazyazure/pkg/utils"
)

// ANSI color code for gray text (256-color palette)
const grayColor = "\x1b[38;5;245m"
const resetColor = "\x1b[0m"

// formatWithGraySuffix formats a name with a gray suffix in parentheses
func formatWithGraySuffix(name, suffix string) string {
	if suffix == "" {
		return name
	}
	return name + " " + grayColor + "(" + suffix + ")" + resetColor
}

// Gui is the main GUI controller
type Gui struct {
	g             *gocui.Gui
	azureClient   AzureClient
	clientFactory AzureClientFactory
	subClient     SubscriptionsClient
	rgClient      ResourceGroupsClient
	resClient     ResourcesClient
	taskManager   *tasks.TaskManager

	// Views - Left sidebar (stacked panels)
	authView           *gocui.View
	subscriptionsView  *gocui.View
	resourceGroupsView *gocui.View
	resourcesView      *gocui.View

	// Views - Right panel and status
	mainView   *gocui.View
	statusView *gocui.View

	// Selection state
	selectedSub *domain.Subscription
	selectedRG  *domain.ResourceGroup
	selectedRes *domain.Resource

	// Data
	subscriptions  []*domain.Subscription
	resourceGroups []*domain.ResourceGroup
	resources      []*domain.Resource
	currentUser    *domain.User

	// UI state
	tabIndex    int    // 0 = summary, 1 = json
	activePanel string // "subscriptions", "resourcegroups", or "resources"

	mu sync.RWMutex
}

// NewGui creates a new GUI instance
func NewGui(azureClient AzureClient, clientFactory AzureClientFactory) (*Gui, error) {
	return &Gui{
		azureClient:   azureClient,
		clientFactory: clientFactory,
		taskManager:   tasks.NewTaskManager(),
		tabIndex:      0,
		activePanel:   "subscriptions",
	}, nil
}

// Run starts the GUI event loop
func (gui *Gui) Run() error {
	utils.Log("Gui.Run: Creating gocui...")
	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode:       gocui.OutputTrue,
		RuneReplacements: map[rune]string{},
	})
	if err != nil {
		utils.Log("Gui.Run: ERROR creating gocui: %v", err)
		return err
	}
	defer g.Close()

	gui.g = g
	utils.Log("Gui.Run: gocui created successfully")

	// Set up color scheme (green border for active/focused elements)
	gui.g.SelFgColor = gocui.ColorGreen
	gui.g.SelFrameColor = gocui.ColorGreen

	// Set up initial layout
	utils.Log("Gui.Run: Setting up views...")
	if err := gui.setupViews(); err != nil {
		utils.Log("Gui.Run: ERROR setting up views: %v", err)
		return err
	}
	utils.Log("Gui.Run: Views set up successfully")

	// Set up keybindings
	utils.Log("Gui.Run: Setting up keybindings...")
	if err := gui.setupKeybindings(); err != nil {
		utils.Log("Gui.Run: ERROR setting up keybindings: %v", err)
		return err
	}
	utils.Log("Gui.Run: Keybindings set up successfully")

	// Initialize Azure clients
	utils.Log("Gui.Run: Initializing Azure clients...")
	subClient, err := gui.clientFactory.NewSubscriptionsClient()
	if err != nil {
		utils.Log("Gui.Run: ERROR initializing subscription client: %v", err)
		return fmt.Errorf("failed to initialize subscription client: %w", err)
	}
	gui.subClient = subClient
	utils.Log("Gui.Run: Azure clients initialized")

	// Load initial data
	utils.Log("Gui.Run: Loading initial data...")
	gui.loadUserInfo()
	gui.loadSubscriptions()

	// Start the main loop
	utils.Log("Gui.Run: Starting MainLoop...")
	return g.MainLoop()
}

func (gui *Gui) setupViews() error {
	maxX, maxY := gui.g.Size()

	// Left sidebar width (33% of screen)
	sidebarWidth := maxX / 3
	if sidebarWidth < 30 {
		sidebarWidth = 30
	}

	// Right panel starts after sidebar
	rightX0 := sidebarWidth

	// Calculate heights for stacked panels
	// Auth: 5 lines, then distribute remaining: 20% subscriptions, 30% RGs, ~50% resources
	authHeight := 5
	remainingHeight := maxY - authHeight - 2 // -2 for status bar
	// Divide remaining space: 20% for subscriptions, 30% for RGs, rest for resources
	subHeight := remainingHeight / 5       // 20%
	rgHeight := (remainingHeight * 3) / 10 // 30%

	// Status bar at bottom
	statusY := maxY - 2

	// 1. Auth panel (top, small)
	if v, err := gui.g.SetView("auth", 0, 0, sidebarWidth-1, authHeight, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Auth "
		v.Wrap = true
		v.Frame = true
		v.FrameColor = gocui.ColorWhite
		gui.authView = v
	}

	// 2. Subscriptions panel
	subY0 := authHeight + 1
	subY1 := subY0 + subHeight
	if v, err := gui.g.SetView("subscriptions", 0, subY0, sidebarWidth-1, subY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Subscriptions "
		v.Highlight = true
		v.SelBgColor = gocui.ColorBlue
		v.SelFgColor = gocui.ColorWhite | gocui.AttrBold
		v.Frame = true
		v.FrameColor = gocui.ColorWhite
		gui.subscriptionsView = v
		// Set as current view initially
		gui.g.SetCurrentView("subscriptions")
	}

	// 3. Resource Groups panel
	rgY0 := subY1 + 1
	rgY1 := rgY0 + rgHeight
	if v, err := gui.g.SetView("resourcegroups", 0, rgY0, sidebarWidth-1, rgY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Resource Groups "
		v.Highlight = true
		v.SelBgColor = gocui.ColorBlue
		v.SelFgColor = gocui.ColorWhite | gocui.AttrBold
		v.Frame = true
		v.FrameColor = gocui.ColorWhite
		gui.resourceGroupsView = v
	}

	// 4. Resources panel (new!)
	resY0 := rgY1 + 1
	// Resources should align with main panel which ends at statusY
	// Both panels' bottom borders should be at the same Y coordinate
	resY1 := statusY
	if v, err := gui.g.SetView("resources", 0, resY0, sidebarWidth-1, resY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Resources "
		v.Highlight = true
		v.SelBgColor = gocui.ColorBlue
		v.SelFgColor = gocui.ColorWhite | gocui.AttrBold
		v.Frame = true
		v.FrameColor = gocui.ColorWhite
		gui.resourcesView = v
	}

	// 5. Main panel (right side)
	if v, err := gui.g.SetView("main", rightX0, 0, maxX-1, statusY, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Details "
		v.Wrap = true
		// Enable scrolling
		v.Autoscroll = false
		// Editable and focusable for scrolling, but no highlight (not a list)
		v.Editable = false
		v.Highlight = false
		v.Frame = true
		v.FrameColor = gocui.ColorWhite
		gui.mainView = v
	}

	// 6. Status bar (bottom)
	if v, err := gui.g.SetView("status", 0, statusY, maxX-1, maxY, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.BgColor = gocui.ColorDefault
		v.FgColor = gocui.ColorWhite
		v.Frame = false
		gui.statusView = v
	}

	gui.updatePanelTitles()
	gui.updateStatus()
	gui.refreshAuthPanel()
	gui.refreshSubscriptionsPanel()
	gui.refreshResourceGroupsPanel()
	gui.refreshResourcesPanel()
	gui.refreshMainPanel()

	return nil
}

func (gui *Gui) setupKeybindings() error {
	utils.Log("setupKeybindings: Setting up keybindings...")

	// Global quit
	quitKeys := []string{"", "subscriptions", "resourcegroups", "resources", "main"}
	for _, view := range quitKeys {
		if err := gui.g.SetKeybinding(view, gocui.KeyCtrlC, gocui.ModNone, gui.quit); err != nil {
			return err
		}
		if err := gui.g.SetKeybinding(view, 'q', gocui.ModNone, gui.quit); err != nil {
			return err
		}
	}
	utils.Log("setupKeybindings: Quit keybindings set")

	// Subscriptions panel navigation
	if err := gui.g.SetKeybinding("subscriptions", gocui.KeyArrowDown, gocui.ModNone, gui.nextSub); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("subscriptions", gocui.KeyArrowUp, gocui.ModNone, gui.prevSub); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("subscriptions", 'j', gocui.ModNone, gui.nextSub); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("subscriptions", 'k', gocui.ModNone, gui.prevSub); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("subscriptions", gocui.KeyEnter, gocui.ModNone, gui.onSubEnter); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Subscriptions navigation set")

	// Resource Groups panel navigation
	if err := gui.g.SetKeybinding("resourcegroups", gocui.KeyArrowDown, gocui.ModNone, gui.nextRG); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resourcegroups", gocui.KeyArrowUp, gocui.ModNone, gui.prevRG); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resourcegroups", 'j', gocui.ModNone, gui.nextRG); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resourcegroups", 'k', gocui.ModNone, gui.prevRG); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resourcegroups", gocui.KeyEnter, gocui.ModNone, gui.onRGEnter); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Resource groups navigation set")

	// Resources panel navigation
	if err := gui.g.SetKeybinding("resources", gocui.KeyArrowDown, gocui.ModNone, gui.nextRes); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resources", gocui.KeyArrowUp, gocui.ModNone, gui.prevRes); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resources", 'j', gocui.ModNone, gui.nextRes); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resources", 'k', gocui.ModNone, gui.prevRes); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resources", gocui.KeyEnter, gocui.ModNone, gui.onResEnter); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Resources navigation set")

	// Tab switching (global)
	if err := gui.g.SetKeybinding("", '[', gocui.ModNone, gui.prevTab); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", ']', gocui.ModNone, gui.nextTab); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Tab keys set")

	// Refresh (global)
	if err := gui.g.SetKeybinding("", 'r', gocui.ModNone, gui.refresh); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Refresh key set")

	// Copy portal link (global)
	if err := gui.g.SetKeybinding("", 'c', gocui.ModNone, gui.copyPortalUrl); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Copy portal link key set")

	// Panel switching with Tab key
	if err := gui.g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, gui.switchPanel); err != nil {
		return err
	}
	// Panel switching with Shift+Tab (reverse direction) - uses KeyBacktab
	if err := gui.g.SetKeybinding("", gocui.KeyBacktab, gocui.ModNone, gui.switchPanelReverse); err != nil {
		return err
	}

	// Main panel scrolling (when viewing resource details)
	if err := gui.g.SetKeybinding("main", gocui.KeyArrowDown, gocui.ModNone, gui.scrollDown); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("main", gocui.KeyArrowUp, gocui.ModNone, gui.scrollUp); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("main", 'j', gocui.ModNone, gui.scrollDown); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("main", 'k', gocui.ModNone, gui.scrollUp); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("main", gocui.KeyPgdn, gocui.ModNone, gui.scrollPageDown); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("main", gocui.KeyPgup, gocui.ModNone, gui.scrollPageUp); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Main panel scrolling set")

	utils.Log("setupKeybindings: All keybindings set successfully")
	return nil
}

func (gui *Gui) quit(g *gocui.Gui, v *gocui.View) error {
	utils.Log("quit: Ctrl+C or q pressed - quitting application")
	gui.taskManager.StopAll()
	utils.Log("quit: Task manager stopped")
	return gocui.ErrQuit
}

func (gui *Gui) loadUserInfo() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		user, err := gui.azureClient.GetUserInfo(ctx)
		if err != nil {
			utils.Log("loadUserInfo: Error: %v", err)
			return
		}

		gui.mu.Lock()
		gui.currentUser = user
		gui.mu.Unlock()

		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.refreshAuthPanel()
			return nil
		})
	}()
}

func (gui *Gui) refreshAuthPanel() {
	if gui.authView == nil {
		return
	}

	gui.authView.Clear()
	gui.mu.RLock()
	user := gui.currentUser
	gui.mu.RUnlock()

	if user != nil {
		// Display user information
		fmt.Fprintf(gui.authView, "Name:  %s\n", user.DisplayName)
		// Only show UPN for service principals, not for regular users
		// (since users authenticated via some methods don't have UPN in the token)
		if user.Type == "serviceprincipal" {
			fmt.Fprintf(gui.authView, "AppID: %s\n", user.UserPrincipalName)
		}
		fmt.Fprintf(gui.authView, "Type:  %s", user.Type)
	} else {
		fmt.Fprint(gui.authView, "Authenticating...")
	}
}

func (gui *Gui) refreshSubscriptionsPanel() {
	if gui.subscriptionsView == nil {
		return
	}

	gui.subscriptionsView.Clear()
	gui.mu.RLock()
	subs := gui.subscriptions
	gui.mu.RUnlock()

	for _, sub := range subs {
		fmt.Fprintln(gui.subscriptionsView, formatWithGraySuffix(sub.DisplayString(), sub.GetDisplaySuffix()))
	}
}

func (gui *Gui) refreshResourceGroupsPanel() {
	if gui.resourceGroupsView == nil {
		return
	}

	gui.resourceGroupsView.Clear()
	gui.mu.RLock()
	rgs := gui.resourceGroups
	gui.mu.RUnlock()

	for _, rg := range rgs {
		fmt.Fprintln(gui.resourceGroupsView, formatWithGraySuffix(rg.DisplayString(), rg.GetDisplaySuffix()))
	}
}

func (gui *Gui) refreshResourcesPanel() {
	if gui.resourcesView == nil {
		return
	}

	gui.resourcesView.Clear()
	gui.mu.RLock()
	resources := gui.resources
	gui.mu.RUnlock()

	for _, res := range resources {
		fmt.Fprintln(gui.resourcesView, formatWithGraySuffix(res.DisplayString(), res.GetDisplaySuffix()))
	}
}

// Placeholder implementations for the rest
func (gui *Gui) nextSub(g *gocui.Gui, v *gocui.View) error {
	gui.mu.RLock()
	subCount := len(gui.subscriptions)
	gui.mu.RUnlock()

	if subCount == 0 {
		return nil
	}

	cx, cy := v.Cursor()
	if cy < subCount-1 {
		v.SetCursor(cx, cy+1)
		gui.updateSubscriptionSelection(v)
		gui.refreshMainPanel()
	}
	return nil
}

func (gui *Gui) prevSub(g *gocui.Gui, v *gocui.View) error {
	gui.mu.RLock()
	subCount := len(gui.subscriptions)
	gui.mu.RUnlock()

	if subCount == 0 {
		return nil
	}

	cx, cy := v.Cursor()
	if cy > 0 {
		v.SetCursor(cx, cy-1)
		gui.updateSubscriptionSelection(v)
		gui.refreshMainPanel()
	}
	return nil
}

func (gui *Gui) updateSubscriptionSelection(v *gocui.View) {
	_, cy := v.Cursor()
	idx := cy

	gui.mu.Lock()
	if idx >= 0 && idx < len(gui.subscriptions) {
		gui.selectedSub = gui.subscriptions[idx]
	}
	gui.mu.Unlock()
}

func (gui *Gui) nextRG(g *gocui.Gui, v *gocui.View) error {
	gui.mu.RLock()
	rgCount := len(gui.resourceGroups)
	gui.mu.RUnlock()

	if rgCount == 0 {
		return nil
	}

	cx, cy := v.Cursor()
	if cy < rgCount-1 {
		v.SetCursor(cx, cy+1)
		gui.updateRGSelection(v)
		gui.refreshMainPanel()
	}
	return nil
}

func (gui *Gui) prevRG(g *gocui.Gui, v *gocui.View) error {
	gui.mu.RLock()
	rgCount := len(gui.resourceGroups)
	gui.mu.RUnlock()

	if rgCount == 0 {
		return nil
	}

	cx, cy := v.Cursor()
	if cy > 0 {
		v.SetCursor(cx, cy-1)
		gui.updateRGSelection(v)
		gui.refreshMainPanel()
	}
	return nil
}

func (gui *Gui) updateRGSelection(v *gocui.View) {
	_, cy := v.Cursor()
	idx := cy

	gui.mu.Lock()
	if idx >= 0 && idx < len(gui.resourceGroups) {
		gui.selectedRG = gui.resourceGroups[idx]
	}
	gui.mu.Unlock()
}

func (gui *Gui) nextRes(g *gocui.Gui, v *gocui.View) error {
	gui.mu.RLock()
	resCount := len(gui.resources)
	gui.mu.RUnlock()

	if resCount == 0 {
		return nil
	}

	cx, cy := v.Cursor()
	if cy < resCount-1 {
		v.SetCursor(cx, cy+1)
		gui.updateResSelection(v)
		gui.refreshMainPanel()
	}
	return nil
}

func (gui *Gui) prevRes(g *gocui.Gui, v *gocui.View) error {
	gui.mu.RLock()
	resCount := len(gui.resources)
	gui.mu.RUnlock()

	if resCount == 0 {
		return nil
	}

	cx, cy := v.Cursor()
	if cy > 0 {
		v.SetCursor(cx, cy-1)
		gui.updateResSelection(v)
		gui.refreshMainPanel()
	}
	return nil
}

func (gui *Gui) updateResSelection(v *gocui.View) {
	_, cy := v.Cursor()
	idx := cy

	gui.mu.Lock()
	if idx >= 0 && idx < len(gui.resources) {
		gui.selectedRes = gui.resources[idx]
	}
	gui.mu.Unlock()
}

func (gui *Gui) onSubEnter(g *gocui.Gui, v *gocui.View) error {
	gui.mu.Lock()
	if len(gui.subscriptions) == 0 {
		gui.mu.Unlock()
		return nil
	}

	_, cy := v.Cursor()
	idx := cy

	if idx >= 0 && idx < len(gui.subscriptions) {
		gui.selectedSub = gui.subscriptions[idx]
		subID := gui.selectedSub.ID
		gui.mu.Unlock()

		// Load resource groups for this subscription
		gui.loadResourceGroups(subID)

		// Switch focus to resource groups panel
		gui.g.SetCurrentView("resourcegroups")
		gui.mu.Lock()
		gui.activePanel = "resourcegroups"
		gui.mu.Unlock()
		gui.updatePanelTitles()
		gui.updateStatus()
	} else {
		gui.mu.Unlock()
	}
	return nil
}

func (gui *Gui) onRGEnter(g *gocui.Gui, v *gocui.View) error {
	gui.mu.Lock()
	if len(gui.resourceGroups) == 0 {
		gui.mu.Unlock()
		return nil
	}

	_, cy := v.Cursor()
	idx := cy

	if idx >= 0 && idx < len(gui.resourceGroups) {
		gui.selectedRG = gui.resourceGroups[idx]
		rgName := gui.selectedRG.Name
		subID := gui.selectedSub.ID
		gui.mu.Unlock()

		// Load resources for this resource group
		gui.loadResources(subID, rgName)

		// Switch focus to resources panel
		gui.g.SetCurrentView("resources")
		gui.mu.Lock()
		gui.activePanel = "resources"
		gui.mu.Unlock()
		gui.updatePanelTitles()
		gui.updateStatus()
	} else {
		gui.mu.Unlock()
	}
	return nil
}

func (gui *Gui) onResEnter(g *gocui.Gui, v *gocui.View) error {
	gui.mu.Lock()
	if len(gui.resources) == 0 {
		gui.mu.Unlock()
		return nil
	}

	_, cy := v.Cursor()
	idx := cy

	if idx >= 0 && idx < len(gui.resources) {
		selectedRes := gui.resources[idx]
		// Set selectedRes immediately so basic info shows while loading
		gui.selectedRes = selectedRes
		gui.mu.Unlock()

		// Refresh to show basic info immediately
		gui.refreshMainPanel()

		// Load full resource details asynchronously
		gui.loadResourceDetails(selectedRes)

		// Move focus to main panel to view the details
		gui.g.SetCurrentView("main")
		gui.mu.Lock()
		gui.activePanel = "main"
		gui.mu.Unlock()
		gui.updatePanelTitles()
		gui.updateStatus()
	} else {
		gui.mu.Unlock()
	}

	return nil
}

func (gui *Gui) switchPanel(g *gocui.Gui, v *gocui.View) error {
	gui.mu.Lock()
	currentPanel := gui.activePanel
	gui.mu.Unlock()

	var nextView string
	switch currentPanel {
	case "subscriptions":
		nextView = "resourcegroups"
	case "resourcegroups":
		nextView = "resources"
	case "resources":
		nextView = "main"
	case "main":
		nextView = "subscriptions"
	default:
		nextView = "subscriptions"
	}

	utils.Log("switchPanel: switching from %s to %s", currentPanel, nextView)

	if _, err := gui.g.SetCurrentView(nextView); err != nil {
		utils.Log("switchPanel: ERROR setting current view: %v", err)
		return err
	}

	gui.mu.Lock()
	gui.activePanel = nextView
	gui.mu.Unlock()

	// Update visual indicators
	gui.updatePanelTitles()
	gui.updateStatus()

	utils.Log("switchPanel: switched successfully to %s", nextView)
	return nil
}

func (gui *Gui) switchPanelReverse(g *gocui.Gui, v *gocui.View) error {
	gui.mu.Lock()
	currentPanel := gui.activePanel
	gui.mu.Unlock()

	var nextView string
	switch currentPanel {
	case "subscriptions":
		nextView = "main"
	case "main":
		nextView = "resources"
	case "resources":
		nextView = "resourcegroups"
	case "resourcegroups":
		nextView = "subscriptions"
	default:
		nextView = "subscriptions"
	}

	utils.Log("switchPanelReverse: switching from %s to %s", currentPanel, nextView)

	if _, err := gui.g.SetCurrentView(nextView); err != nil {
		utils.Log("switchPanelReverse: ERROR setting current view: %v", err)
		return err
	}

	gui.mu.Lock()
	gui.activePanel = nextView
	gui.mu.Unlock()

	// Update visual indicators
	gui.updatePanelTitles()
	gui.updateStatus()

	utils.Log("switchPanelReverse: switched successfully to %s", nextView)
	return nil
}

// scrollDown scrolls the main panel down by one line
func (gui *Gui) scrollDown(g *gocui.Gui, v *gocui.View) error {
	if gui.mainView != nil {
		ox, oy := gui.mainView.Origin()
		gui.mainView.SetOrigin(ox, oy+1)
	}
	return nil
}

// scrollUp scrolls the main panel up by one line
func (gui *Gui) scrollUp(g *gocui.Gui, v *gocui.View) error {
	if gui.mainView != nil {
		ox, oy := gui.mainView.Origin()
		if oy > 0 {
			gui.mainView.SetOrigin(ox, oy-1)
		}
	}
	return nil
}

// scrollPageDown scrolls the main panel down by one page
func (gui *Gui) scrollPageDown(g *gocui.Gui, v *gocui.View) error {
	if gui.mainView != nil {
		_, height := gui.mainView.Size()
		ox, oy := gui.mainView.Origin()
		gui.mainView.SetOrigin(ox, oy+height-1)
	}
	return nil
}

// scrollPageUp scrolls the main panel up by one page
func (gui *Gui) scrollPageUp(g *gocui.Gui, v *gocui.View) error {
	if gui.mainView != nil {
		_, height := gui.mainView.Size()
		ox, oy := gui.mainView.Origin()
		if oy > height-1 {
			gui.mainView.SetOrigin(ox, oy-(height-1))
		} else {
			gui.mainView.SetOrigin(ox, 0)
		}
	}
	return nil
}

func (gui *Gui) updatePanelTitles() {
	gui.mu.RLock()
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	// Update frame colors to show which panel is active (green = active, white = inactive)
	if gui.subscriptionsView != nil {
		if activePanel == "subscriptions" {
			gui.subscriptionsView.FrameColor = gocui.ColorGreen
		} else {
			gui.subscriptionsView.FrameColor = gocui.ColorWhite
		}
	}

	if gui.resourceGroupsView != nil {
		if activePanel == "resourcegroups" {
			gui.resourceGroupsView.FrameColor = gocui.ColorGreen
		} else {
			gui.resourceGroupsView.FrameColor = gocui.ColorWhite
		}
	}

	if gui.resourcesView != nil {
		if activePanel == "resources" {
			gui.resourcesView.FrameColor = gocui.ColorGreen
		} else {
			gui.resourcesView.FrameColor = gocui.ColorWhite
		}
	}

	// Update main panel frame color
	if gui.mainView != nil {
		if activePanel == "main" {
			gui.mainView.FrameColor = gocui.ColorGreen
		} else {
			gui.mainView.FrameColor = gocui.ColorWhite
		}
	}
}

func (gui *Gui) refresh(g *gocui.Gui, v *gocui.View) error {
	// Reload all data
	gui.loadUserInfo()
	gui.loadSubscriptions()

	gui.mu.RLock()
	selectedSub := gui.selectedSub
	selectedRG := gui.selectedRG
	gui.mu.RUnlock()

	if selectedSub != nil {
		gui.loadResourceGroups(selectedSub.ID)
	}

	if selectedRG != nil && selectedSub != nil {
		gui.loadResources(selectedSub.ID, selectedRG.Name)
	}
	return nil
}

func (gui *Gui) prevTab(g *gocui.Gui, v *gocui.View) error {
	gui.mu.Lock()
	if gui.tabIndex > 0 {
		gui.tabIndex--
	}
	gui.mu.Unlock()
	gui.refreshMainPanel()
	return nil
}

func (gui *Gui) nextTab(g *gocui.Gui, v *gocui.View) error {
	gui.mu.Lock()
	if gui.tabIndex < 1 {
		gui.tabIndex++
	}
	gui.mu.Unlock()
	gui.refreshMainPanel()
	return nil
}

// copyPortalUrl copies the Azure Portal URL for the currently selected item to clipboard
func (gui *Gui) copyPortalUrl(g *gocui.Gui, v *gocui.View) error {
	gui.mu.RLock()
	selectedSub := gui.selectedSub
	selectedRG := gui.selectedRG
	selectedRes := gui.selectedRes
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	var url string
	var itemType string

	// Build URL based on what's selected and active panel
	switch activePanel {
	case "subscriptions":
		if selectedSub == nil {
			gui.showTemporaryStatus("No subscription selected")
			return nil
		}
		url = utils.BuildSubscriptionPortalURL(selectedSub.TenantID, selectedSub.ID)
		itemType = "subscription"
	case "resourcegroups":
		if selectedRG == nil || selectedSub == nil {
			gui.showTemporaryStatus("No resource group selected")
			return nil
		}
		url = utils.BuildResourceGroupPortalURL(selectedSub.TenantID, selectedRG.SubscriptionID, selectedRG.Name)
		itemType = "resource group"
	case "resources", "main":
		if selectedRes == nil || selectedSub == nil {
			gui.showTemporaryStatus("No resource selected")
			return nil
		}
		url = utils.BuildResourcePortalURL(selectedSub.TenantID, selectedRes.ID)
		itemType = "resource"
	default:
		gui.showTemporaryStatus("No item selected")
		return nil
	}

	// Copy to clipboard
	if err := utils.CopyToClipboard(url); err != nil {
		gui.showTemporaryStatus(fmt.Sprintf("Failed to copy: %v", err))
		return nil
	}

	gui.showTemporaryStatus(fmt.Sprintf("Copied %s portal link to clipboard", itemType))
	return nil
}

// showTemporaryStatus shows a temporary status message that reverts after a delay
func (gui *Gui) showTemporaryStatus(message string) {
	if gui.statusView == nil {
		return
	}

	gui.statusView.Clear()
	fmt.Fprint(gui.statusView, message)

	// Restore normal status after 2 seconds
	go func() {
		time.Sleep(2 * time.Second)
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.updateStatus()
			return nil
		})
	}()
}

// ANSI color codes
const (
	// Color 114 from 256-color palette (github-dark key color) + bold
	colorBoldKey = "\x1b[1;38;5;114m" // Bold + green (color 114)
	colorWhite   = "\x1b[37m"         // White for values
	colorReset   = "\x1b[0m"          // Reset
)

// printKeyValue prints a key-value pair with bold green key and white value
func printKeyValue(view *gocui.View, key string, value string) {
	fmt.Fprintf(view, "%s%s:%s %s\n", colorBoldKey, key, colorReset, value)
}

// formatPropertyValue formats a property value, handling nested maps properly
func formatPropertyValue(view *gocui.View, key string, value interface{}, indent string) {
	switch v := value.(type) {
	case map[string]interface{}:
		// For maps, print the key and then recurse into the nested values
		fmt.Fprintf(view, "%s%s%s:%s\n", colorBoldKey, indent+key, colorReset, "")
		// Sort keys for consistent display
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, nestedKey := range keys {
			formatPropertyValue(view, nestedKey, v[nestedKey], indent+"  ")
		}
	case []interface{}:
		// For arrays, print the key and then each item
		fmt.Fprintf(view, "%s%s%s:%s\n", colorBoldKey, indent+key, colorReset, "")
		for i, item := range v {
			formatPropertyValue(view, fmt.Sprintf("[%d]", i), item, indent+"  ")
		}
	case nil:
		// Handle nil values explicitly
		fmt.Fprintf(view, "%s%s%s:%s null\n", colorBoldKey, indent+key, colorReset, "")
	default:
		// For simple values, print key-value pair
		fmt.Fprintf(view, "%s%s%s:%s %v\n", colorBoldKey, indent+key, colorReset, "", v)
	}
}

// highlightJSON uses Chroma to syntax highlight JSON output with bold keys
func highlightJSON(jsonData string) string {
	// Use the JSON lexer
	lexer := lexers.Get("json")
	if lexer == nil {
		lexer = lexers.Fallback
	}

	// Use github-dark theme
	style := styles.Get("github-dark")
	if style == nil {
		style = styles.Fallback
	}

	// Use terminal256 formatter for full color support
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	// Tokenize and format
	iterator, err := lexer.Tokenise(nil, jsonData)
	if err != nil {
		return jsonData // Return unformatted on error
	}

	var buf bytes.Buffer
	err = formatter.Format(&buf, style, iterator)
	if err != nil {
		return jsonData // Return unformatted on error
	}

	// Post-process to add bold to green keys (color 114)
	// Replace [38;5;114m with [1;38;5;114m to add bold
	result := buf.String()
	result = strings.ReplaceAll(result, "\x1b[38;5;114m", "\x1b[1;38;5;114m")

	return result
}

func (gui *Gui) refreshMainPanel() {
	if gui.mainView == nil {
		return
	}

	gui.mainView.Clear()
	gui.mu.RLock()
	tabIndex := gui.tabIndex
	selectedSub := gui.selectedSub
	selectedRG := gui.selectedRG
	selectedRes := gui.selectedRes
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	// Determine what to display based on what's selected and active panel
	if selectedRes != nil && (activePanel == "resources" || activePanel == "main") {
		// Show resource details
		if tabIndex == 0 {
			// Summary tab
			gui.mainView.Title = " Details [Summary] "
			printKeyValue(gui.mainView, "Name", selectedRes.Name)
			printKeyValue(gui.mainView, "Type", selectedRes.Type)
			printKeyValue(gui.mainView, "Location", selectedRes.Location)
			printKeyValue(gui.mainView, "ID", selectedRes.ID)
			printKeyValue(gui.mainView, "Resource Group", selectedRes.ResourceGroup)
			if selectedRes.CreatedTime != "" {
				printKeyValue(gui.mainView, "Created", selectedRes.CreatedTime)
			}
			if selectedRes.ChangedTime != "" {
				printKeyValue(gui.mainView, "Modified", selectedRes.ChangedTime)
			}
			if len(selectedRes.Tags) > 0 {
				fmt.Fprintln(gui.mainView, "")
				printKeyValue(gui.mainView, "Tags", "")
				// Sort tag keys for consistent display
				tagKeys := make([]string, 0, len(selectedRes.Tags))
				for k := range selectedRes.Tags {
					tagKeys = append(tagKeys, k)
				}
				sort.Strings(tagKeys)
				for _, k := range tagKeys {
					printKeyValue(gui.mainView, "  "+k, selectedRes.Tags[k])
				}
			}
			// Show resource properties if available
			if len(selectedRes.Properties) > 0 {
				fmt.Fprintln(gui.mainView, "")
				printKeyValue(gui.mainView, "Properties", "")
				// Sort property keys for consistent display
				propKeys := make([]string, 0, len(selectedRes.Properties))
				for k := range selectedRes.Properties {
					propKeys = append(propKeys, k)
				}
				sort.Strings(propKeys)
				for _, k := range propKeys {
					formatPropertyValue(gui.mainView, k, selectedRes.Properties[k], "  ")
				}
			}
			// Show hint at the bottom when browsing from list view (not in main panel)
			if activePanel == "resources" {
				fmt.Fprintln(gui.mainView, "")
				fmt.Fprintln(gui.mainView, "─────────────────────────────────────────")
				fmt.Fprintln(gui.mainView, "[Press Enter to load full resource details]")
			}
		} else {
			// JSON tab
			gui.mainView.Title = " Details [JSON] "
			// Show hint at the top when browsing from list view
			if activePanel == "resources" {
				fmt.Fprintln(gui.mainView, "// Press Enter to load full resource details with all properties")
				fmt.Fprintln(gui.mainView, "")
			}
			jsonData, err := json.MarshalIndent(selectedRes, "", "  ")
			if err != nil {
				fmt.Fprintf(gui.mainView, "Error marshaling JSON: %v\n", err)
			} else {
				fmt.Fprint(gui.mainView, highlightJSON(string(jsonData)))
			}
		}
	} else if selectedRG != nil && (activePanel == "resourcegroups" || activePanel == "resources") {
		// Show resource group details
		if tabIndex == 0 {
			// Summary tab
			gui.mainView.Title = " Details [Summary] "
			printKeyValue(gui.mainView, "Name", selectedRG.Name)
			printKeyValue(gui.mainView, "Location", selectedRG.Location)
			printKeyValue(gui.mainView, "Subscription ID", selectedRG.SubscriptionID)
			printKeyValue(gui.mainView, "ID", selectedRG.ID)
			printKeyValue(gui.mainView, "Provisioning State", selectedRG.ProvisioningState)
			if len(selectedRG.Tags) > 0 {
				fmt.Fprintln(gui.mainView, "")
				printKeyValue(gui.mainView, "Tags", "")
				// Sort tag keys for consistent display
				tagKeys := make([]string, 0, len(selectedRG.Tags))
				for k := range selectedRG.Tags {
					tagKeys = append(tagKeys, k)
				}
				sort.Strings(tagKeys)
				for _, k := range tagKeys {
					printKeyValue(gui.mainView, "  "+k, selectedRG.Tags[k])
				}
			}
		} else {
			// JSON tab
			gui.mainView.Title = " Details [JSON] "
			jsonData, err := json.MarshalIndent(selectedRG, "", "  ")
			if err != nil {
				fmt.Fprintf(gui.mainView, "Error marshaling JSON: %v\n", err)
			} else {
				fmt.Fprint(gui.mainView, highlightJSON(string(jsonData)))
			}
		}
	} else if selectedSub != nil {
		// Show subscription details
		if tabIndex == 0 {
			// Summary tab
			gui.mainView.Title = " Details [Summary] "
			printKeyValue(gui.mainView, "Name", selectedSub.Name)
			printKeyValue(gui.mainView, "ID", selectedSub.ID)
			printKeyValue(gui.mainView, "State", selectedSub.State)
			printKeyValue(gui.mainView, "Tenant ID", selectedSub.TenantID)
		} else {
			// JSON tab
			gui.mainView.Title = " Details [JSON] "
			jsonData, err := json.MarshalIndent(selectedSub, "", "  ")
			if err != nil {
				fmt.Fprintf(gui.mainView, "Error marshaling JSON: %v\n", err)
			} else {
				fmt.Fprint(gui.mainView, highlightJSON(string(jsonData)))
			}
		}
	}
}

func (gui *Gui) updateStatus() {
	if gui.statusView == nil {
		return
	}

	gui.statusView.Clear()
	gui.mu.RLock()
	activePanel := gui.activePanel
	subCount := len(gui.subscriptions)
	rgCount := len(gui.resourceGroups)
	resCount := len(gui.resources)
	gui.mu.RUnlock()

	var status string
	switch activePanel {
	case "subscriptions":
		status = fmt.Sprintf("↑↓: Navigate | Enter: Load RGs | c: Copy Link | Tab: Switch | r: Refresh | q: Quit | Subs: %d", subCount)
	case "resourcegroups":
		status = fmt.Sprintf("↑↓: Navigate | Enter: Load Resources | c: Copy Link | Tab: Switch | []: Tabs | r: Refresh | q: Quit | RGs: %d", rgCount)
	case "resources":
		status = fmt.Sprintf("↑↓: Navigate | Enter: View Details | c: Copy Link | Tab: Switch | []: Tabs | r: Refresh | q: Quit | Resources: %d", resCount)
	case "main":
		status = fmt.Sprintf("↑/↓ or j/k: Scroll | PgUp/PgDn: Page | c: Copy Link | Tab: Back to List | []: Tabs | r: Refresh | q: Quit")
	default:
		status = fmt.Sprintf("↑↓: Navigate | Tab: Switch | r: Refresh | q: Quit")
	}
	fmt.Fprint(gui.statusView, status)
}

func (gui *Gui) loadSubscriptions() {
	gui.mu.RLock()
	subClient := gui.subClient
	gui.mu.RUnlock()

	if subClient == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		subs, err := subClient.ListSubscriptions(ctx)
		if err != nil {
			utils.Log("loadSubscriptions: Error: %v", err)
			return
		}

		gui.mu.Lock()
		gui.subscriptions = subs
		if len(subs) > 0 && gui.selectedSub == nil {
			gui.selectedSub = subs[0]
		}
		gui.mu.Unlock()

		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.refreshSubscriptionsPanel()
			gui.refreshMainPanel()
			gui.updateStatus()
			return nil
		})
	}()
}

func (gui *Gui) loadResourceGroups(subscriptionID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		rgClient, err := gui.clientFactory.NewResourceGroupsClient(subscriptionID)
		if err != nil {
			utils.Log("loadResourceGroups: Error creating client: %v", err)
			return
		}

		rgs, err := rgClient.ListResourceGroups(ctx)
		if err != nil {
			utils.Log("loadResourceGroups: Error listing RGs: %v", err)
			return
		}

		gui.mu.Lock()
		gui.resourceGroups = rgs
		gui.rgClient = rgClient
		if len(rgs) > 0 {
			gui.selectedRG = rgs[0]
		} else {
			gui.selectedRG = nil
		}
		// Clear resources when switching subscriptions
		gui.resources = nil
		gui.selectedRes = nil
		gui.mu.Unlock()

		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.refreshResourceGroupsPanel()
			gui.refreshResourcesPanel()
			gui.refreshMainPanel()
			gui.updateStatus()
			return nil
		})
	}()
}

func (gui *Gui) loadResources(subscriptionID string, resourceGroupName string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resClient, err := gui.clientFactory.NewResourcesClient(subscriptionID)
		if err != nil {
			utils.Log("loadResources: Error creating client: %v", err)
			return
		}

		resources, err := resClient.ListResourcesByResourceGroup(ctx, resourceGroupName)
		if err != nil {
			utils.Log("loadResources: Error listing resources: %v", err)
			return
		}

		gui.mu.Lock()
		gui.resources = resources
		gui.resClient = resClient
		if len(resources) > 0 {
			gui.selectedRes = resources[0]
		} else {
			gui.selectedRes = nil
		}
		gui.mu.Unlock()

		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.refreshResourcesPanel()
			gui.refreshMainPanel()
			gui.updateStatus()
			return nil
		})
	}()
}

// loadResourceDetails fetches full resource details with provider-specific API version
func (gui *Gui) loadResourceDetails(originalRes *domain.Resource) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		gui.mu.RLock()
		resClient := gui.resClient
		gui.mu.RUnlock()

		if resClient == nil {
			utils.Log("loadResourceDetails: No resources client available")
			return
		}

		// Fetch full resource details with provider-specific properties
		resource, err := resClient.GetResource(ctx, originalRes.ID, originalRes.Type)
		if err != nil {
			utils.Log("loadResourceDetails: Error getting resource %s: %v", originalRes.ID, err)
			return
		}

		// Preserve createdTime and changedTime from the original list data
		// (these aren't returned by GetByID but were in the list view)
		resource.CreatedTime = originalRes.CreatedTime
		resource.ChangedTime = originalRes.ChangedTime

		gui.mu.Lock()
		gui.selectedRes = resource
		gui.mu.Unlock()

		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.refreshMainPanel()
			gui.updatePanelTitles() // Restore focus indicator
			return nil
		})
	}()
}
