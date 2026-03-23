package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/matsest/lazyazure/pkg/azure"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/tasks"
	"github.com/matsest/lazyazure/pkg/utils"
)

// Gui is the main GUI controller
type Gui struct {
	g           *gocui.Gui
	azureClient *azure.Client
	subClient   *azure.SubscriptionsClient
	rgClient    *azure.ResourceGroupsClient
	resClient   *azure.ResourcesClient
	taskManager *tasks.TaskManager

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
func NewGui(azureClient *azure.Client) (*Gui, error) {
	return &Gui{
		azureClient: azureClient,
		taskManager: tasks.NewTaskManager(),
		tabIndex:    0,
		activePanel: "subscriptions",
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
	subClient, err := gui.azureClient.InitSubscriptionsClient()
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
	// Auth: 5 lines, then distribute remaining among subscriptions, RGs, and resources
	authHeight := 5
	remainingHeight := maxY - authHeight - 2 // -2 for status bar
	// Divide remaining space among 3 panels (subscriptions, RGs, resources)
	panelHeight := remainingHeight / 3

	// Status bar at bottom
	statusY := maxY - 2

	// 1. Auth panel (top, small)
	if v, err := gui.g.SetView("auth", 0, 0, sidebarWidth-1, authHeight, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Auth "
		v.Wrap = true
		gui.authView = v
	}

	// 2. Subscriptions panel
	subY0 := authHeight + 1
	subY1 := subY0 + panelHeight
	if v, err := gui.g.SetView("subscriptions", 0, subY0, sidebarWidth-1, subY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Subscriptions "
		v.Highlight = true
		v.SelBgColor = gocui.ColorBlue
		v.SelFgColor = gocui.ColorWhite
		gui.subscriptionsView = v
		// Set as current view initially
		gui.g.SetCurrentView("subscriptions")
	}

	// 3. Resource Groups panel
	rgY0 := subY1 + 1
	rgY1 := rgY0 + panelHeight
	if v, err := gui.g.SetView("resourcegroups", 0, rgY0, sidebarWidth-1, rgY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Resource Groups "
		v.Highlight = true
		v.SelBgColor = gocui.ColorBlue
		v.SelFgColor = gocui.ColorWhite
		gui.resourceGroupsView = v
	}

	// 4. Resources panel (new!)
	resY0 := rgY1 + 1
	resY1 := resY0 + panelHeight
	// If there's extra space due to integer division, give it to resources
	if resY1 < statusY-1 {
		resY1 = statusY - 1
	}
	if v, err := gui.g.SetView("resources", 0, resY0, sidebarWidth-1, resY1, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Resources "
		v.Highlight = true
		v.SelBgColor = gocui.ColorBlue
		v.SelFgColor = gocui.ColorWhite
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
		fmt.Fprintln(gui.subscriptionsView, sub.DisplayString())
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
		fmt.Fprintln(gui.resourceGroupsView, rg.DisplayString())
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
		fmt.Fprintln(gui.resourcesView, res.DisplayString())
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

	// Update titles to show which panel is active
	if gui.subscriptionsView != nil {
		if activePanel == "subscriptions" {
			gui.subscriptionsView.Title = " ▶ Subscriptions "
		} else {
			gui.subscriptionsView.Title = "   Subscriptions "
		}
	}

	if gui.resourceGroupsView != nil {
		if activePanel == "resourcegroups" {
			gui.resourceGroupsView.Title = " ▶ Resource Groups "
		} else {
			gui.resourceGroupsView.Title = "   Resource Groups "
		}
	}

	if gui.resourcesView != nil {
		if activePanel == "resources" {
			gui.resourcesView.Title = " ▶ Resources "
		} else {
			gui.resourcesView.Title = "   Resources "
		}
	}

	// Update main panel title to show when it's active
	if gui.mainView != nil {
		if activePanel == "main" {
			gui.mainView.Title = " ▶ Details "
		} else {
			gui.mainView.Title = "   Details "
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
			fmt.Fprintf(gui.mainView, "Name: %s\n", selectedRes.Name)
			fmt.Fprintf(gui.mainView, "Type: %s\n", selectedRes.Type)
			fmt.Fprintf(gui.mainView, "Location: %s\n", selectedRes.Location)
			fmt.Fprintf(gui.mainView, "ID: %s\n", selectedRes.ID)
			fmt.Fprintf(gui.mainView, "Resource Group: %s\n", selectedRes.ResourceGroup)
			if selectedRes.CreatedTime != "" {
				fmt.Fprintf(gui.mainView, "Created: %s\n", selectedRes.CreatedTime)
			}
			if selectedRes.ChangedTime != "" {
				fmt.Fprintf(gui.mainView, "Modified: %s\n", selectedRes.ChangedTime)
			}
			if len(selectedRes.Tags) > 0 {
				fmt.Fprintln(gui.mainView, "\nTags:")
				for k, v := range selectedRes.Tags {
					fmt.Fprintf(gui.mainView, "  %s: %s\n", k, v)
				}
			}
			// Show resource properties if available
			if len(selectedRes.Properties) > 0 {
				fmt.Fprintln(gui.mainView, "\nProperties:")
				for k, v := range selectedRes.Properties {
					fmt.Fprintf(gui.mainView, "  %s: %v\n", k, v)
				}
			}
		} else {
			// JSON tab
			gui.mainView.Title = " Details [JSON] "
			jsonData, err := json.MarshalIndent(selectedRes, "", "  ")
			if err != nil {
				fmt.Fprintf(gui.mainView, "Error marshaling JSON: %v\n", err)
			} else {
				fmt.Fprint(gui.mainView, string(jsonData))
			}
		}
	} else if selectedRG != nil && (activePanel == "resourcegroups" || activePanel == "resources") {
		// Show resource group details
		if tabIndex == 0 {
			// Summary tab
			gui.mainView.Title = " Details [Summary] "
			fmt.Fprintf(gui.mainView, "Name: %s\n", selectedRG.Name)
			fmt.Fprintf(gui.mainView, "Location: %s\n", selectedRG.Location)
			fmt.Fprintf(gui.mainView, "Subscription ID: %s\n", selectedRG.SubscriptionID)
			fmt.Fprintf(gui.mainView, "ID: %s\n", selectedRG.ID)
			fmt.Fprintf(gui.mainView, "Provisioning State: %s\n", selectedRG.ProvisioningState)
			if len(selectedRG.Tags) > 0 {
				fmt.Fprintln(gui.mainView, "\nTags:")
				for k, v := range selectedRG.Tags {
					fmt.Fprintf(gui.mainView, "  %s: %s\n", k, v)
				}
			}
		} else {
			// JSON tab
			gui.mainView.Title = " Details [JSON] "
			jsonData, err := json.MarshalIndent(selectedRG, "", "  ")
			if err != nil {
				fmt.Fprintf(gui.mainView, "Error marshaling JSON: %v\n", err)
			} else {
				fmt.Fprint(gui.mainView, string(jsonData))
			}
		}
	} else if selectedSub != nil {
		// Show subscription details
		if tabIndex == 0 {
			// Summary tab
			gui.mainView.Title = " Details [Summary] "
			fmt.Fprintf(gui.mainView, "Name: %s\n", selectedSub.Name)
			fmt.Fprintf(gui.mainView, "ID: %s\n", selectedSub.ID)
			fmt.Fprintf(gui.mainView, "State: %s\n", selectedSub.State)
			fmt.Fprintf(gui.mainView, "Tenant ID: %s\n", selectedSub.TenantID)
		} else {
			// JSON tab
			gui.mainView.Title = " Details [JSON] "
			jsonData, err := json.MarshalIndent(selectedSub, "", "  ")
			if err != nil {
				fmt.Fprintf(gui.mainView, "Error marshaling JSON: %v\n", err)
			} else {
				fmt.Fprint(gui.mainView, string(jsonData))
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
		status = fmt.Sprintf("↑↓: Navigate | Enter: Load RGs | Tab: Switch | r: Refresh | q: Quit | Subs: %d", subCount)
	case "resourcegroups":
		status = fmt.Sprintf("↑↓: Navigate | Enter: Load Resources | Tab: Switch | []: Tabs | r: Refresh | q: Quit | RGs: %d", rgCount)
	case "resources":
		status = fmt.Sprintf("↑↓: Navigate | Enter: View Details | Tab: Switch | []: Tabs | r: Refresh | q: Quit | Resources: %d", resCount)
	case "main":
		status = fmt.Sprintf("↑/↓ or j/k: Scroll | PgUp/PgDn: Page | Tab: Back to List | []: Tabs | r: Refresh | q: Quit")
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

		rgClient, err := azure.NewResourceGroupsClient(gui.azureClient, subscriptionID)
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

		resClient, err := azure.NewResourcesClient(gui.azureClient, subscriptionID)
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
