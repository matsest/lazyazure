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
	taskManager *tasks.TaskManager

	// Views - Left sidebar (stacked panels)
	authView           *gocui.View
	subscriptionsView  *gocui.View
	resourceGroupsView *gocui.View

	// Views - Right panel and status
	mainView   *gocui.View
	statusView *gocui.View

	// Selection state
	selectedSub *domain.Subscription
	selectedRG  *domain.ResourceGroup

	// Data
	subscriptions  []*domain.Subscription
	resourceGroups []*domain.ResourceGroup
	currentUser    *domain.User

	// UI state
	tabIndex    int    // 0 = summary, 1 = json
	activePanel string // "subscriptions" or "resourcegroups"

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
	// Auth: 5 lines (for 3 lines of content + title), Subscriptions: 35%, Resource Groups: remaining
	authHeight := 5
	remainingHeight := maxY - authHeight - 2 // -2 for status bar
	subHeight := remainingHeight * 40 / 100  // 40% of remaining
	rgHeight := remainingHeight - subHeight  // rest goes to RG

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

	// 2. Subscriptions panel (middle)
	subY0 := authHeight + 1
	subY1 := subY0 + subHeight
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

	// 3. Resource Groups panel (bottom)
	rgY0 := subY1 + 1
	rgY1 := rgY0 + rgHeight
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

	// 4. Main panel (right side)
	if v, err := gui.g.SetView("main", rightX0, 0, maxX-1, statusY, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		v.Title = " Details "
		v.Wrap = true
		gui.mainView = v
	}

	// 5. Status bar (bottom)
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
	gui.refreshMainPanel()

	return nil
}

func (gui *Gui) setupKeybindings() error {
	utils.Log("setupKeybindings: Setting up keybindings...")

	// Global quit
	quitKeys := []string{"", "subscriptions", "resourcegroups", "main"}
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
		gui.updatePanelTitles() // Update the visual indicator
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
	}
	gui.mu.Unlock()

	// Update main panel to show RG details
	gui.refreshMainPanel()
	return nil
}

func (gui *Gui) switchPanel(g *gocui.Gui, v *gocui.View) error {
	gui.mu.Lock()
	currentPanel := gui.activePanel
	gui.mu.Unlock()

	var nextView string
	if currentPanel == "subscriptions" {
		nextView = "resourcegroups"
	} else {
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
}

func (gui *Gui) refresh(g *gocui.Gui, v *gocui.View) error {
	// Reload all data
	gui.loadUserInfo()
	gui.loadSubscriptions()

	gui.mu.RLock()
	selectedSub := gui.selectedSub
	gui.mu.RUnlock()

	if selectedSub != nil {
		gui.loadResourceGroups(selectedSub.ID)
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
	gui.mu.RUnlock()

	// Determine what to display based on what's selected
	if selectedRG != nil && gui.activePanel == "resourcegroups" {
		// Show resource group details
		if tabIndex == 0 {
			// Summary tab
			gui.mainView.Title = " Details [Summary] "
			fmt.Fprintf(gui.mainView, "Name: %s\n", selectedRG.Name)
			fmt.Fprintf(gui.mainView, "Location: %s\n", selectedRG.Location)
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
	gui.mu.RUnlock()

	var status string
	if activePanel == "subscriptions" {
		status = fmt.Sprintf("↑↓: Navigate | Enter: Load RGs | Tab: Switch | r: Refresh | q: Quit | Subs: %d", subCount)
	} else {
		status = fmt.Sprintf("↑↓: Navigate | Enter: View Details | Tab: Switch | []: Tabs | r: Refresh | q: Quit | RGs: %d", rgCount)
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
		gui.mu.Unlock()

		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.refreshResourceGroupsPanel()
			gui.refreshMainPanel()
			gui.updateStatus()
			return nil
		})
	}()
}
