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
	"github.com/matsest/lazyazure/pkg/gui/panels"
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

	// Filtered lists for search
	subList *panels.FilteredList[*domain.Subscription]
	rgList  *panels.FilteredList[*domain.ResourceGroup]
	resList *panels.FilteredList[*domain.Resource]

	// Search state
	searchBar       *panels.SearchBar
	isSearching     bool
	searchTarget    string // "list" or "main" - tracks which search mode is active
	mainPanelSearch *panels.MainPanelSearch

	mu sync.RWMutex
}

// NewGui creates a new GUI instance
func NewGui(azureClient AzureClient, clientFactory AzureClientFactory) (*Gui, error) {
	return &Gui{
		azureClient:     azureClient,
		clientFactory:   clientFactory,
		taskManager:     tasks.NewTaskManager(),
		tabIndex:        0,
		activePanel:     "subscriptions",
		subList:         panels.NewFilteredList[*domain.Subscription](),
		rgList:          panels.NewFilteredList[*domain.ResourceGroup](),
		resList:         panels.NewFilteredList[*domain.Resource](),
		mainPanelSearch: panels.NewMainPanelSearch(),
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

	// Enable mouse support
	gui.g.Mouse = true
	utils.Log("Gui.Run: Mouse support enabled")

	// Set up color scheme (green border for active/focused elements)
	gui.g.SelFgColor = gocui.ColorGreen
	gui.g.SelBgColor = gocui.ColorDefault
	gui.g.SelFrameColor = gocui.ColorGreen
	// Don't set g.FgColor - let inactive titles use default (white)
	// Don't set g.BgColor - let it use the terminal default

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
		v.Frame = true
		v.FrameColor = gocui.ColorWhite
		gui.resourcesView = v
	}

	// 5. Main panel (right side)
	if v, err := gui.g.SetView("main", rightX0, 0, maxX-1, statusY, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
		// Use gocui's native tab support instead of title
		v.Tabs = []string{"Summary", "JSON"}
		v.TabIndex = 0
		v.Wrap = true
		// Enable scrolling
		v.Autoscroll = false
		// Editable and focusable for scrolling, but no highlight (not a list)
		v.Editable = false
		v.Highlight = false
		v.Frame = true
		v.FrameColor = gocui.ColorWhite
		// Tab colors: selected tab uses SelFgColor (green), inactive uses FgColor (white)
		// SelBgColor and BgColor set to default to avoid background highlight
		// Include AttrBold in SelFgColor so that when gocui subtracts it for non-current views,
		// we end up with just the color without bold
		v.SelFgColor = gocui.ColorGreen | gocui.AttrBold
		v.SelBgColor = gocui.ColorDefault
		v.FgColor = gocui.ColorWhite
		v.BgColor = gocui.ColorDefault
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

	// Global quit - Ctrl+C works everywhere including search
	quitKeys := []string{"", "subscriptions", "resourcegroups", "resources", "main", "search"}
	for _, view := range quitKeys {
		if err := gui.g.SetKeybinding(view, gocui.KeyCtrlC, gocui.ModNone, gui.quit); err != nil {
			return err
		}
	}
	// 'q' to quit - not bound to search view since 'q' is used for search input
	quitKeysNoSearch := []string{"", "subscriptions", "resourcegroups", "resources", "main"}
	for _, view := range quitKeysNoSearch {
		if err := gui.g.SetKeybinding(view, 'q', gocui.ModNone, gui.quit); err != nil {
			return err
		}
	}
	utils.Log("setupKeybindings: Quit keybindings set")

	// Mouse click to focus panels (list panels)
	panels := []string{"subscriptions", "resourcegroups", "resources"}
	for _, panel := range panels {
		if err := gui.g.SetKeybinding(panel, gocui.MouseLeft, gocui.ModNone, gui.onPanelClick); err != nil {
			return err
		}
	}
	utils.Log("setupKeybindings: Mouse click keybindings set")

	// Mouse click for main panel (with coordinate support for tabs)
	if err := gui.g.SetViewClickBinding(&gocui.ViewMouseBinding{
		ViewName: "main",
		Key:      gocui.MouseLeft,
		Handler:  gui.onMainViewClick,
	}); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Main panel click binding set")

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
	// Mouse wheel scrolling for subscriptions panel
	if err := gui.g.SetKeybinding("subscriptions", gocui.MouseWheelUp, gocui.ModNone, gui.prevSub); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("subscriptions", gocui.MouseWheelDown, gocui.ModNone, gui.nextSub); err != nil {
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
	// Mouse wheel scrolling for resource groups panel
	if err := gui.g.SetKeybinding("resourcegroups", gocui.MouseWheelUp, gocui.ModNone, gui.prevRG); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resourcegroups", gocui.MouseWheelDown, gocui.ModNone, gui.nextRG); err != nil {
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
	// Mouse wheel scrolling for resources panel
	if err := gui.g.SetKeybinding("resources", gocui.MouseWheelUp, gocui.ModNone, gui.prevRes); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resources", gocui.MouseWheelDown, gocui.ModNone, gui.nextRes); err != nil {
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

	// Open portal link in browser (global)
	if err := gui.g.SetKeybinding("", 'o', gocui.ModNone, gui.openPortalUrl); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Open portal link key set")

	// Panel switching with Tab key
	if err := gui.g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, gui.switchPanel); err != nil {
		return err
	}
	// Panel switching with Shift+Tab (reverse direction) - uses KeyBacktab
	if err := gui.g.SetKeybinding("", gocui.KeyBacktab, gocui.ModNone, gui.switchPanelReverse); err != nil {
		return err
	}

	// Search (/) - activates for current panel
	if err := gui.g.SetKeybinding("", '/', gocui.ModNone, gui.startSearch); err != nil {
		return err
	}

	// Escape to clear filter (for list panels)
	if err := gui.g.SetKeybinding("subscriptions", gocui.KeyEsc, gocui.ModNone, gui.clearFilter); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resourcegroups", gocui.KeyEsc, gocui.ModNone, gui.clearFilter); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resources", gocui.KeyEsc, gocui.ModNone, gui.clearFilter); err != nil {
		return err
	}

	// Search input handling (only when search is active)
	// These will be bound dynamically when search is activated

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
	// Mouse wheel scrolling for main panel
	if err := gui.g.SetKeybinding("main", gocui.MouseWheelUp, gocui.ModNone, gui.scrollUp); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("main", gocui.MouseWheelDown, gocui.ModNone, gui.scrollDown); err != nil {
		return err
	}
	// Escape to clear main panel search
	if err := gui.g.SetKeybinding("main", gocui.KeyEsc, gocui.ModNone, gui.onMainPanelClearSearch); err != nil {
		return err
	}
	// n/N to navigate matches in main panel
	if err := gui.g.SetKeybinding("main", 'n', gocui.ModNone, gui.onMainPanelSearchNext); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("main", 'N', gocui.ModNone, gui.onMainPanelSearchPrev); err != nil {
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

// startSearch activates search mode for the current panel
func (gui *Gui) startSearch(g *gocui.Gui, v *gocui.View) error {
	utils.Log("startSearch: Activating search mode")

	gui.mu.RLock()
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	if activePanel == "main" {
		// Start main panel search (highlight mode)
		return gui.startMainPanelSearch(g, v)
	}

	// Start list panel search (filter mode)
	// Always recreate search bar with list panel callbacks (in case we switched from main panel)
	gui.searchBar = panels.NewSearchBar(gui.g, gui.onSearchChanged, gui.onSearchCancel, gui.onSearchConfirm)

	if err := gui.searchBar.Show(); err != nil {
		utils.Log("startSearch: ERROR showing search bar: %v", err)
		return err
	}

	gui.isSearching = true
	gui.searchTarget = "list"
	utils.Log("startSearch: Setting up search keybindings...")
	gui.setupSearchKeybindings()
	utils.Log("startSearch: Search keybindings setup complete")
	gui.updateStatus()
	utils.Log("startSearch: Status updated, search mode ready")

	return nil
}

// startMainPanelSearch activates search mode for the main/details panel
func (gui *Gui) startMainPanelSearch(g *gocui.Gui, v *gocui.View) error {
	utils.Log("startMainPanelSearch: Activating main panel search")

	// Create search bar for main panel
	if gui.searchBar == nil {
		gui.searchBar = panels.NewSearchBar(gui.g, gui.onMainPanelSearchChanged, gui.onMainPanelSearchCancel, gui.onMainPanelSearchConfirm)
	} else {
		// Update callbacks for main panel search
		gui.searchBar = panels.NewSearchBar(gui.g, gui.onMainPanelSearchChanged, gui.onMainPanelSearchCancel, gui.onMainPanelSearchConfirm)
	}

	if err := gui.searchBar.Show(); err != nil {
		utils.Log("startMainPanelSearch: ERROR showing search bar: %v", err)
		return err
	}

	gui.isSearching = true
	gui.searchTarget = "main"
	gui.setupMainPanelSearchKeybindings()
	gui.updateStatus()
	utils.Log("startMainPanelSearch: Main panel search mode ready")

	return nil
}

// setupMainPanelSearchKeybindings binds keys for main panel search
func (gui *Gui) setupMainPanelSearchKeybindings() {
	utils.Log("setupMainPanelSearchKeybindings: Setting up main panel search keybindings")

	// Character input - a-z
	for ch := 'a'; ch <= 'z'; ch++ {
		gui.g.SetKeybinding("search", ch, gocui.ModNone, gui.makeMainPanelSearchRuneHandler(ch))
	}
	// Character input - A-Z
	for ch := 'A'; ch <= 'Z'; ch++ {
		gui.g.SetKeybinding("search", ch, gocui.ModNone, gui.makeMainPanelSearchRuneHandler(ch))
	}
	// Numbers
	for ch := '0'; ch <= '9'; ch++ {
		gui.g.SetKeybinding("search", ch, gocui.ModNone, gui.makeMainPanelSearchRuneHandler(ch))
	}
	// Special characters
	specialChars := []rune{'-', '_', '.', '@', '/', '(', ')'}
	for _, ch := range specialChars {
		gui.g.SetKeybinding("search", ch, gocui.ModNone, gui.makeMainPanelSearchRuneHandler(ch))
	}

	// Space
	gui.g.SetKeybinding("search", gocui.KeySpace, gocui.ModNone, gui.onMainPanelSearchSpace)
	// Backspace
	gui.g.SetKeybinding("search", gocui.KeyBackspace, gocui.ModNone, gui.onMainPanelSearchBackspace)
	gui.g.SetKeybinding("search", gocui.KeyBackspace2, gocui.ModNone, gui.onMainPanelSearchBackspace)
	// Ctrl+U - clear all
	gui.g.SetKeybinding("search", gocui.KeyCtrlU, gocui.ModNone, gui.onMainPanelSearchClear)
	// Ctrl+W - delete word
	gui.g.SetKeybinding("search", gocui.KeyCtrlW, gocui.ModNone, gui.onMainPanelSearchDeleteWord)
	// Enter - confirm
	gui.g.SetKeybinding("search", gocui.KeyEnter, gocui.ModNone, gui.onMainPanelSearchEnter)
	// Escape - cancel (handled by unified handler)
	gui.g.SetKeybinding("search", gocui.KeyEsc, gocui.ModNone, gui.onSearchEscapeUnified)
	// n - next match
	gui.g.SetKeybinding("search", 'n', gocui.ModNone, gui.onMainPanelSearchNext)
	// N - previous match
	gui.g.SetKeybinding("search", 'N', gocui.ModNone, gui.onMainPanelSearchPrev)

	utils.Log("setupMainPanelSearchKeybindings: Main panel search keybindings set")
}

// setupSearchKeybindings binds keys for search input
func (gui *Gui) setupSearchKeybindings() {
	utils.Log("setupSearchKeybindings: Setting up search keybindings")

	// Character input - a-z
	for ch := 'a'; ch <= 'z'; ch++ {
		if err := gui.g.SetKeybinding("search", ch, gocui.ModNone, gui.makeSearchRuneHandler(ch)); err != nil {
			utils.Log("setupSearchKeybindings: ERROR binding '%c': %v", ch, err)
		}
	}
	utils.Log("setupSearchKeybindings: a-z bound")

	// Character input - A-Z
	for ch := 'A'; ch <= 'Z'; ch++ {
		if err := gui.g.SetKeybinding("search", ch, gocui.ModNone, gui.makeSearchRuneHandler(ch)); err != nil {
			utils.Log("setupSearchKeybindings: ERROR binding '%c': %v", ch, err)
		}
	}
	utils.Log("setupSearchKeybindings: A-Z bound")

	// Numbers
	for ch := '0'; ch <= '9'; ch++ {
		if err := gui.g.SetKeybinding("search", ch, gocui.ModNone, gui.makeSearchRuneHandler(ch)); err != nil {
			utils.Log("setupSearchKeybindings: ERROR binding '%c': %v", ch, err)
		}
	}
	utils.Log("setupSearchKeybindings: 0-9 bound")

	// Special characters common in Azure names
	specialChars := []rune{'-', '_', '.', '@', '/', '(', ')'}
	for _, ch := range specialChars {
		if err := gui.g.SetKeybinding("search", ch, gocui.ModNone, gui.makeSearchRuneHandler(ch)); err != nil {
			utils.Log("setupSearchKeybindings: ERROR binding '%c': %v", ch, err)
		}
	}
	utils.Log("setupSearchKeybindings: Special chars bound")

	// Space
	if err := gui.g.SetKeybinding("search", gocui.KeySpace, gocui.ModNone, gui.onSearchSpace); err != nil {
		utils.Log("setupSearchKeybindings: ERROR binding space: %v", err)
	}

	// Backspace
	if err := gui.g.SetKeybinding("search", gocui.KeyBackspace, gocui.ModNone, gui.onSearchBackspace); err != nil {
		utils.Log("setupSearchKeybindings: ERROR binding backspace: %v", err)
	}
	if err := gui.g.SetKeybinding("search", gocui.KeyBackspace2, gocui.ModNone, gui.onSearchBackspace); err != nil {
		utils.Log("setupSearchKeybindings: ERROR binding backspace2: %v", err)
	}

	// Ctrl+U - clear all
	if err := gui.g.SetKeybinding("search", gocui.KeyCtrlU, gocui.ModNone, gui.onSearchClear); err != nil {
		utils.Log("setupSearchKeybindings: ERROR binding Ctrl+U: %v", err)
	}

	// Ctrl+W - delete word
	if err := gui.g.SetKeybinding("search", gocui.KeyCtrlW, gocui.ModNone, gui.onSearchDeleteWord); err != nil {
		utils.Log("setupSearchKeybindings: ERROR binding Ctrl+W: %v", err)
	}

	// Enter - confirm search and exit
	if err := gui.g.SetKeybinding("search", gocui.KeyEnter, gocui.ModNone, gui.onSearchEnter); err != nil {
		utils.Log("setupSearchKeybindings: ERROR binding Enter: %v", err)
	}

	// Escape - cancel search (handled by unified handler)
	if err := gui.g.SetKeybinding("search", gocui.KeyEsc, gocui.ModNone, gui.onSearchEscapeUnified); err != nil {
		utils.Log("setupSearchKeybindings: ERROR binding Escape: %v", err)
	}

	utils.Log("setupSearchKeybindings: All search keybindings set")
}

// onSearchEnter confirms the search
func (gui *Gui) onSearchEnter(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.Confirm()
		gui.endSearch()
	}
	return nil
}

// onSearchEscape cancels the search
func (gui *Gui) onSearchEscape(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.Cancel()
		gui.endSearch()
	}
	return nil
}

// onSearchEscapeUnified handles Escape key for both search modes
func (gui *Gui) onSearchEscapeUnified(g *gocui.Gui, v *gocui.View) error {
	utils.Log("onSearchEscapeUnified: Escape pressed, searchTarget=%s", gui.searchTarget)

	if gui.searchTarget == "main" {
		// Main panel search mode
		if gui.searchBar != nil {
			gui.searchBar.Cancel()
			gui.clearMainPanelSearch()
			gui.endMainPanelSearch()
		}
	} else {
		// List panel search mode
		if gui.searchBar != nil {
			gui.searchBar.Cancel()
			gui.endSearch()
		}
	}
	return nil
}

// makeSearchRuneHandler creates a handler for a specific character
func (gui *Gui) makeSearchRuneHandler(ch rune) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		utils.Log("makeSearchRuneHandler: Character '%c' pressed", ch)
		if gui.searchBar != nil {
			gui.searchBar.HandleRune(ch)
		}
		return nil
	}
}

// onSearchSpace handles space input in search
func (gui *Gui) onSearchSpace(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.HandleRune(' ')
	}
	return nil
}

// onSearchBackspace handles backspace in search
func (gui *Gui) onSearchBackspace(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.Backspace()
	}
	return nil
}

// onSearchClear clears the search text
func (gui *Gui) onSearchClear(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.Clear()
	}
	return nil
}

// onSearchDeleteWord deletes the last word
func (gui *Gui) onSearchDeleteWord(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.DeleteWord()
	}
	return nil
}

// Main Panel Search Handlers

// makeMainPanelSearchRuneHandler creates a handler for a specific character in main panel search
func (gui *Gui) makeMainPanelSearchRuneHandler(ch rune) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		if gui.searchBar != nil {
			gui.searchBar.HandleRune(ch)
		}
		return nil
	}
}

// onMainPanelSearchSpace handles space input in main panel search
func (gui *Gui) onMainPanelSearchSpace(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.HandleRune(' ')
	}
	return nil
}

// onMainPanelSearchBackspace handles backspace in main panel search
func (gui *Gui) onMainPanelSearchBackspace(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.Backspace()
	}
	return nil
}

// onMainPanelSearchClear clears the main panel search text
func (gui *Gui) onMainPanelSearchClear(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.Clear()
	}
	return nil
}

// onMainPanelSearchDeleteWord deletes the last word in main panel search
func (gui *Gui) onMainPanelSearchDeleteWord(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.DeleteWord()
	}
	return nil
}

// onMainPanelSearchEnter confirms the main panel search
func (gui *Gui) onMainPanelSearchEnter(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.Confirm()
		gui.endMainPanelSearch()
	}
	return nil
}

// onMainPanelSearchEscape cancels the main panel search
func (gui *Gui) onMainPanelSearchEscape(g *gocui.Gui, v *gocui.View) error {
	if gui.searchBar != nil {
		gui.searchBar.Cancel()
		gui.clearMainPanelSearch()
		gui.endMainPanelSearch()
	}
	return nil
}

// onMainPanelSearchNext jumps to next match
func (gui *Gui) onMainPanelSearchNext(g *gocui.Gui, v *gocui.View) error {
	if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
		lineNum := gui.mainPanelSearch.NextMatch()
		if lineNum >= 0 {
			gui.scrollToLine(lineNum)
			gui.refreshMainPanelWithSearch()
		}
		gui.updateStatus()
	}
	return nil
}

// onMainPanelClearSearch clears the main panel search when pressed in main view
func (gui *Gui) onMainPanelClearSearch(g *gocui.Gui, v *gocui.View) error {
	if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
		utils.Log("onMainPanelClearSearch: Clearing main panel search from main view")
		gui.clearMainPanelSearch()
		gui.updateStatus()
	}
	return nil
}

// onMainPanelSearchPrev jumps to previous match
func (gui *Gui) onMainPanelSearchPrev(g *gocui.Gui, v *gocui.View) error {
	if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
		lineNum := gui.mainPanelSearch.PrevMatch()
		if lineNum >= 0 {
			gui.scrollToLine(lineNum)
			gui.refreshMainPanelWithSearch()
		}
		gui.updateStatus()
	}
	return nil
}

// endMainPanelSearch exits main panel search mode
func (gui *Gui) endMainPanelSearch() error {
	if gui.searchBar != nil {
		gui.searchBar.Hide()
	}

	gui.isSearching = false
	gui.updateStatus()

	// Return focus to the main panel
	gui.g.SetCurrentView("main")

	return nil
}

// clearMainPanelSearch clears the search and removes highlights
func (gui *Gui) clearMainPanelSearch() {
	if gui.mainPanelSearch != nil {
		gui.mainPanelSearch.ClearSearch()
		gui.refreshMainPanelWithSearch()
	}
}

// scrollToLine scrolls the main view to show a specific line
func (gui *Gui) scrollToLine(lineNum int) {
	if gui.mainView == nil {
		return
	}

	// Set origin to show the match line
	// We try to center the match in the view
	_, height := gui.mainView.Size()
	var targetY int
	if lineNum > height/2 {
		targetY = lineNum - height/2
	} else {
		targetY = 0
	}

	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		ox, _ := gui.mainView.Origin()
		gui.mainView.SetOrigin(ox, targetY)
		return nil
	})
}

// onMainPanelSearchChanged is called when main panel search text changes
func (gui *Gui) onMainPanelSearchChanged() {
	if gui.searchBar == nil || gui.mainPanelSearch == nil {
		return
	}

	searchText := gui.searchBar.GetText()
	utils.Log("onMainPanelSearchChanged: Search text changed to: %s", searchText)

	gui.mainPanelSearch.SetSearch(searchText)
	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.refreshMainPanelWithSearch()
		gui.updateStatus()
		return nil
	})
}

// onMainPanelSearchCancel is called when main panel search is cancelled
func (gui *Gui) onMainPanelSearchCancel() {
	utils.Log("onMainPanelSearchCancel: Cancelling main panel search")
	gui.clearMainPanelSearch()
}

// onMainPanelSearchConfirm is called when main panel search is confirmed
func (gui *Gui) onMainPanelSearchConfirm() {
	utils.Log("onMainPanelSearchConfirm: Main panel search confirmed")
	// Keep the current search highlights
}

// refreshMainPanelWithSearch refreshes the main panel with search highlights applied
func (gui *Gui) refreshMainPanelWithSearch() {
	if gui.mainView == nil {
		return
	}

	gui.mainView.Clear()

	// Get the highlighted content
	var lines []string
	if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
		lines = gui.mainPanelSearch.GetHighlightedContent()
	} else {
		// If search not active, get the content from the last render
		// We need to re-render the content without highlights
		gui.renderMainPanelContent()
		return
	}

	// Write the highlighted content
	for _, line := range lines {
		fmt.Fprintln(gui.mainView, line)
	}
}

// renderMainPanelContent re-renders the main panel content without highlights
// This is used when search is cleared or not active
func (gui *Gui) renderMainPanelContent() {
	// Delegate to the existing refreshMainPanel logic
	// We'll need to modify refreshMainPanel to store its content
	gui.refreshMainPanel()
}

// endSearch exits search mode and returns focus to the active panel
func (gui *Gui) endSearch() error {
	if gui.searchBar != nil {
		gui.searchBar.Hide()
	}

	gui.isSearching = false
	gui.updateStatus()

	// Return focus to the active panel
	gui.mu.RLock()
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	gui.g.SetCurrentView(activePanel)

	return nil
}

// onSearchChanged is called when search text changes
func (gui *Gui) onSearchChanged() {
	if gui.searchBar == nil {
		return
	}

	searchText := gui.searchBar.GetText()
	utils.Log("onSearchChanged: Search text changed to: %s", searchText)

	gui.mu.RLock()
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	utils.Log("onSearchChanged: Active panel is %s, calling UpdateAsync...", activePanel)

	// Apply filter to the active panel's list
	// Use UpdateAsync to avoid blocking the keybinding handler
	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		utils.Log("onSearchChanged: UpdateAsync callback executing")
		switch activePanel {
		case "subscriptions":
			gui.subList.SetFilter(searchText)
			gui.refreshSubscriptionsPanel()
			gui.updateSubscriptionSelectionFromFiltered()
		case "resourcegroups":
			gui.rgList.SetFilter(searchText)
			gui.refreshResourceGroupsPanel()
			gui.updateRGSelectionFromFiltered()
		case "resources":
			gui.resList.SetFilter(searchText)
			gui.refreshResourcesPanel()
			gui.updateResSelectionFromFiltered()
		}

		gui.refreshMainPanel()
		gui.updateStatus()
		utils.Log("onSearchChanged: UpdateAsync callback completed")
		return nil
	})

	utils.Log("onSearchChanged: UpdateAsync queued")
}

// onSearchCancel is called when search is cancelled
func (gui *Gui) onSearchCancel() {
	gui.mu.RLock()
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	// Clear the filter for the active panel
	switch activePanel {
	case "subscriptions":
		gui.subList.ClearFilter()
		gui.refreshSubscriptionsPanel()
	case "resourcegroups":
		gui.rgList.ClearFilter()
		gui.refreshResourceGroupsPanel()
	case "resources":
		gui.resList.ClearFilter()
		gui.refreshResourcesPanel()
	}

	gui.refreshMainPanel()
	gui.updateStatus()
}

// onSearchConfirm is called when search is confirmed
func (gui *Gui) onSearchConfirm() {
	// Keep the current filter, just exit search mode
	utils.Log("onSearchConfirm: Search confirmed")
}

// clearFilter clears the current panel's filter
func (gui *Gui) clearFilter(g *gocui.Gui, v *gocui.View) error {
	gui.mu.RLock()
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	switch activePanel {
	case "subscriptions":
		if gui.subList.IsFiltering() {
			gui.subList.ClearFilter()
			gui.refreshSubscriptionsPanel()
		}
	case "resourcegroups":
		if gui.rgList.IsFiltering() {
			gui.rgList.ClearFilter()
			gui.refreshResourceGroupsPanel()
		}
	case "resources":
		if gui.resList.IsFiltering() {
			gui.resList.ClearFilter()
			gui.refreshResourcesPanel()
		}
	}

	gui.refreshMainPanel()
	gui.updateStatus()
	return nil
}

// updateSubscriptionSelectionFromFiltered updates selection based on filtered results
func (gui *Gui) updateSubscriptionSelectionFromFiltered() {
	if gui.subscriptionsView == nil {
		return
	}

	_, cy := gui.subscriptionsView.Cursor()
	if sub, ok := gui.subList.Get(cy); ok {
		gui.mu.Lock()
		gui.selectedSub = sub
		gui.mu.Unlock()
		// Clear main panel search when switching subscriptions
		if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
			gui.clearMainPanelSearch()
		}
	}
}

// updateRGSelectionFromFiltered updates selection based on filtered results
func (gui *Gui) updateRGSelectionFromFiltered() {
	if gui.resourceGroupsView == nil {
		return
	}

	_, cy := gui.resourceGroupsView.Cursor()
	if rg, ok := gui.rgList.Get(cy); ok {
		gui.mu.Lock()
		gui.selectedRG = rg
		gui.mu.Unlock()
		// Clear main panel search when switching resource groups
		if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
			gui.clearMainPanelSearch()
		}
	}
}

// updateResSelectionFromFiltered updates selection based on filtered results
func (gui *Gui) updateResSelectionFromFiltered() {
	if gui.resourcesView == nil {
		return
	}

	_, cy := gui.resourcesView.Cursor()
	if res, ok := gui.resList.Get(cy); ok {
		gui.mu.Lock()
		gui.selectedRes = res
		gui.mu.Unlock()
	}
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
	displayStrings := gui.subList.GetFilteredDisplayStrings()

	for _, display := range displayStrings {
		fmt.Fprintln(gui.subscriptionsView, display)
	}
}

func (gui *Gui) refreshResourceGroupsPanel() {
	if gui.resourceGroupsView == nil {
		return
	}

	gui.resourceGroupsView.Clear()
	displayStrings := gui.rgList.GetFilteredDisplayStrings()

	for _, display := range displayStrings {
		fmt.Fprintln(gui.resourceGroupsView, display)
	}
}

func (gui *Gui) refreshResourcesPanel() {
	if gui.resourcesView == nil {
		return
	}

	gui.resourcesView.Clear()
	displayStrings := gui.resList.GetFilteredDisplayStrings()

	for _, display := range displayStrings {
		fmt.Fprintln(gui.resourcesView, display)
	}
}

// Placeholder implementations for the rest
func (gui *Gui) nextSub(g *gocui.Gui, v *gocui.View) error {
	subCount := gui.subList.Len()

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
	subCount := gui.subList.Len()

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
	if sub, ok := gui.subList.Get(cy); ok {
		gui.mu.Lock()
		gui.selectedSub = sub
		gui.mu.Unlock()
	}
}

func (gui *Gui) nextRG(g *gocui.Gui, v *gocui.View) error {
	rgCount := gui.rgList.Len()

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
	rgCount := gui.rgList.Len()

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
	if rg, ok := gui.rgList.Get(cy); ok {
		gui.mu.Lock()
		gui.selectedRG = rg
		gui.mu.Unlock()
		// Clear main panel search when switching resource groups
		if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
			gui.clearMainPanelSearch()
		}
	}
}

func (gui *Gui) nextRes(g *gocui.Gui, v *gocui.View) error {
	resCount := gui.resList.Len()

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
	resCount := gui.resList.Len()

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
	if res, ok := gui.resList.Get(cy); ok {
		gui.mu.Lock()
		gui.selectedRes = res
		gui.mu.Unlock()
		// Clear main panel search when switching resources
		if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
			gui.clearMainPanelSearch()
		}
	}
}

func (gui *Gui) onSubEnter(g *gocui.Gui, v *gocui.View) error {
	if gui.subList.Len() == 0 {
		return nil
	}

	_, cy := v.Cursor()
	if sub, ok := gui.subList.Get(cy); ok {
		gui.mu.Lock()
		gui.selectedSub = sub
		subID := sub.ID
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
	}
	return nil
}

func (gui *Gui) onRGEnter(g *gocui.Gui, v *gocui.View) error {
	if gui.rgList.Len() == 0 {
		return nil
	}

	_, cy := v.Cursor()
	if rg, ok := gui.rgList.Get(cy); ok {
		gui.mu.Lock()
		gui.selectedRG = rg
		rgName := rg.Name
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
	}
	return nil
}

func (gui *Gui) onResEnter(g *gocui.Gui, v *gocui.View) error {
	if gui.resList.Len() == 0 {
		return nil
	}

	_, cy := v.Cursor()
	if selectedRes, ok := gui.resList.Get(cy); ok {
		// Set selectedRes immediately so basic info shows while loading
		gui.mu.Lock()
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

// onPanelClick handles mouse clicks on panels to focus them
func (gui *Gui) onPanelClick(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}

	viewName := v.Name()
	utils.Log("onPanelClick: Clicked on panel %s", viewName)

	// Map view names to panel names
	panelName := viewName
	switch viewName {
	case "subscriptions", "resourcegroups", "resources":
		// These are the focusable list panels
	default:
		// Not a focusable panel (main is handled separately via onMainViewClick)
		return nil
	}

	// Set the current view and update active panel
	if _, err := gui.g.SetCurrentView(viewName); err != nil {
		utils.Log("onPanelClick: ERROR setting current view: %v", err)
		return err
	}

	gui.mu.Lock()
	gui.activePanel = panelName
	gui.mu.Unlock()

	// Update visual indicators
	gui.updatePanelTitles()
	gui.updateStatus()

	// For list panels, also trigger the enter action (as if user pressed Enter on the item)
	// gocui automatically positions the cursor at the clicked line
	switch viewName {
	case "subscriptions":
		_, cy := v.Cursor()
		if _, ok := gui.subList.Get(cy); ok {
			gui.updateSubscriptionSelection(v)
			gui.refreshMainPanel()
			return gui.onSubEnter(g, v)
		}
	case "resourcegroups":
		_, cy := v.Cursor()
		if _, ok := gui.rgList.Get(cy); ok {
			gui.updateRGSelection(v)
			gui.refreshMainPanel()
			return gui.onRGEnter(g, v)
		}
	case "resources":
		_, cy := v.Cursor()
		if _, ok := gui.resList.Get(cy); ok {
			gui.updateResSelection(v)
			gui.refreshMainPanel()
			return gui.onResEnter(g, v)
		}
	}

	utils.Log("onPanelClick: Focused panel %s", panelName)
	return nil
}

// onMainViewClick handles mouse clicks on the main view, including tab clicks
func (gui *Gui) onMainViewClick(opts gocui.ViewMouseBindingOpts) error {
	v, err := gui.g.View("main")
	if err != nil {
		return err
	}

	utils.Log("onMainViewClick: Clicked on main panel at (%d, %d)", opts.X, opts.Y)

	// Check if click was on a tab
	tabIdx := v.GetClickedTabIndex(opts.X)
	if tabIdx >= 0 {
		utils.Log("onMainViewClick: Clicked on tab %d", tabIdx)
		gui.mu.Lock()
		gui.tabIndex = tabIdx
		gui.mu.Unlock()
		gui.refreshMainPanel()
		// Still focus the panel even when clicking a tab
		if _, err := gui.g.SetCurrentView("main"); err != nil {
			return err
		}
		gui.mu.Lock()
		gui.activePanel = "main"
		gui.mu.Unlock()
		gui.updatePanelTitles()
		gui.updateStatus()
		return nil
	}

	// Not a tab click, just focus the panel
	if _, err := gui.g.SetCurrentView("main"); err != nil {
		return err
	}
	gui.mu.Lock()
	gui.activePanel = "main"
	gui.mu.Unlock()
	gui.updatePanelTitles()
	gui.updateStatus()
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

// openPortalUrl opens the Azure Portal URL for the currently selected item in the browser
func (gui *Gui) openPortalUrl(g *gocui.Gui, v *gocui.View) error {
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

	// Open in browser
	if err := utils.OpenBrowser(url); err != nil {
		gui.showTemporaryStatus(fmt.Sprintf("Failed to open browser: %v", err))
		return nil
	}

	gui.showTemporaryStatus(fmt.Sprintf("Opening %s portal link in browser", itemType))
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

	gui.mu.RLock()
	tabIndex := gui.tabIndex
	selectedSub := gui.selectedSub
	selectedRG := gui.selectedRG
	selectedRes := gui.selectedRes
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	// Build content lines
	var lines []string

	// Determine what to display based on what's selected and active panel
	if selectedRes != nil && (activePanel == "resources" || activePanel == "main") {
		// Show resource details
		if tabIndex == 0 {
			lines = gui.buildResourceSummaryLines(selectedRes, activePanel == "resources")
		} else {
			lines = gui.buildResourceJSONLines(selectedRes, activePanel == "resources")
		}
	} else if selectedRG != nil && (activePanel == "resourcegroups" || activePanel == "resources") {
		// Show resource group details
		if tabIndex == 0 {
			lines = gui.buildRGSummaryLines(selectedRG)
		} else {
			lines = gui.buildRGJSONLines(selectedRG)
		}
	} else if selectedSub != nil {
		// Show subscription details
		if tabIndex == 0 {
			lines = gui.buildSubSummaryLines(selectedSub)
		} else {
			lines = gui.buildSubJSONLines(selectedSub)
		}
	}

	// Update the view - gocui handles tab rendering via Tabs/TabIndex
	gui.mainView.TabIndex = tabIndex
	gui.mainView.Clear()

	// Store content for search highlighting
	if gui.mainPanelSearch != nil {
		gui.mainPanelSearch.SetContent(lines)
	}

	// If search is active, show highlighted content
	if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
		highlightedLines := gui.mainPanelSearch.GetHighlightedContent()
		for _, line := range highlightedLines {
			fmt.Fprintln(gui.mainView, line)
		}
	} else {
		// Show normal content
		for _, line := range lines {
			fmt.Fprintln(gui.mainView, line)
		}
	}
}

// buildResourceSummaryLines builds the summary view lines for a resource
func (gui *Gui) buildResourceSummaryLines(res *domain.Resource, showHint bool) []string {
	var lines []string
	lines = append(lines, fmt.Sprintf("%sName:%s %s", colorBoldKey, colorReset, res.Name))
	lines = append(lines, fmt.Sprintf("%sType:%s %s", colorBoldKey, colorReset, res.Type))
	lines = append(lines, fmt.Sprintf("%sLocation:%s %s", colorBoldKey, colorReset, res.Location))
	lines = append(lines, fmt.Sprintf("%sID:%s %s", colorBoldKey, colorReset, res.ID))
	lines = append(lines, fmt.Sprintf("%sResource Group:%s %s", colorBoldKey, colorReset, res.ResourceGroup))
	if res.CreatedTime != "" {
		lines = append(lines, fmt.Sprintf("%sCreated:%s %s", colorBoldKey, colorReset, res.CreatedTime))
	}
	if res.ChangedTime != "" {
		lines = append(lines, fmt.Sprintf("%sModified:%s %s", colorBoldKey, colorReset, res.ChangedTime))
	}
	if len(res.Tags) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%sTags:%s", colorBoldKey, colorReset))
		tagKeys := make([]string, 0, len(res.Tags))
		for k := range res.Tags {
			tagKeys = append(tagKeys, k)
		}
		sort.Strings(tagKeys)
		for _, k := range tagKeys {
			lines = append(lines, fmt.Sprintf("%s  %s:%s %s", colorBoldKey, k, colorReset, res.Tags[k]))
		}
	}
	if len(res.Properties) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%sProperties:%s", colorBoldKey, colorReset))
		propKeys := make([]string, 0, len(res.Properties))
		for k := range res.Properties {
			propKeys = append(propKeys, k)
		}
		sort.Strings(propKeys)
		for _, k := range propKeys {
			lines = append(lines, gui.formatPropertyLines(k, res.Properties[k], "  ")...)
		}
	}
	if showHint {
		lines = append(lines, "")
		lines = append(lines, "─────────────────────────────────────────")
		lines = append(lines, "[Press Enter to load full resource details]")
	}
	return lines
}

// buildResourceJSONLines builds the JSON view lines for a resource
func (gui *Gui) buildResourceJSONLines(res *domain.Resource, showHint bool) []string {
	var lines []string
	if showHint {
		lines = append(lines, "// Press Enter to load full resource details with all properties")
		lines = append(lines, "")
	}
	jsonData, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		lines = append(lines, fmt.Sprintf("Error marshaling JSON: %v", err))
	} else {
		// Split the highlighted JSON into lines
		highlighted := highlightJSON(string(jsonData))
		lines = append(lines, strings.Split(highlighted, "\n")...)
	}
	return lines
}

// buildRGSummaryLines builds the summary view lines for a resource group
func (gui *Gui) buildRGSummaryLines(rg *domain.ResourceGroup) []string {
	var lines []string
	lines = append(lines, fmt.Sprintf("%sName:%s %s", colorBoldKey, colorReset, rg.Name))
	lines = append(lines, fmt.Sprintf("%sLocation:%s %s", colorBoldKey, colorReset, rg.Location))
	lines = append(lines, fmt.Sprintf("%sSubscription ID:%s %s", colorBoldKey, colorReset, rg.SubscriptionID))
	lines = append(lines, fmt.Sprintf("%sID:%s %s", colorBoldKey, colorReset, rg.ID))
	lines = append(lines, fmt.Sprintf("%sProvisioning State:%s %s", colorBoldKey, colorReset, rg.ProvisioningState))
	if len(rg.Tags) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%sTags:%s", colorBoldKey, colorReset))
		tagKeys := make([]string, 0, len(rg.Tags))
		for k := range rg.Tags {
			tagKeys = append(tagKeys, k)
		}
		sort.Strings(tagKeys)
		for _, k := range tagKeys {
			lines = append(lines, fmt.Sprintf("%s  %s:%s %s", colorBoldKey, k, colorReset, rg.Tags[k]))
		}
	}
	return lines
}

// buildRGJSONLines builds the JSON view lines for a resource group
func (gui *Gui) buildRGJSONLines(rg *domain.ResourceGroup) []string {
	var lines []string
	jsonData, err := json.MarshalIndent(rg, "", "  ")
	if err != nil {
		lines = append(lines, fmt.Sprintf("Error marshaling JSON: %v", err))
	} else {
		highlighted := highlightJSON(string(jsonData))
		lines = append(lines, strings.Split(highlighted, "\n")...)
	}
	return lines
}

// buildSubSummaryLines builds the summary view lines for a subscription
func (gui *Gui) buildSubSummaryLines(sub *domain.Subscription) []string {
	var lines []string
	lines = append(lines, fmt.Sprintf("%sName:%s %s", colorBoldKey, colorReset, sub.Name))
	lines = append(lines, fmt.Sprintf("%sID:%s %s", colorBoldKey, colorReset, sub.ID))
	lines = append(lines, fmt.Sprintf("%sState:%s %s", colorBoldKey, colorReset, sub.State))
	lines = append(lines, fmt.Sprintf("%sTenant ID:%s %s", colorBoldKey, colorReset, sub.TenantID))
	return lines
}

// buildSubJSONLines builds the JSON view lines for a subscription
func (gui *Gui) buildSubJSONLines(sub *domain.Subscription) []string {
	var lines []string
	jsonData, err := json.MarshalIndent(sub, "", "  ")
	if err != nil {
		lines = append(lines, fmt.Sprintf("Error marshaling JSON: %v", err))
	} else {
		highlighted := highlightJSON(string(jsonData))
		lines = append(lines, strings.Split(highlighted, "\n")...)
	}
	return lines
}

// formatPropertyLines formats a property value and returns lines (for nested structures)
func (gui *Gui) formatPropertyLines(key string, value interface{}, indent string) []string {
	var lines []string
	switch v := value.(type) {
	case map[string]interface{}:
		lines = append(lines, fmt.Sprintf("%s%s%s:%s", colorBoldKey, indent+key, colorReset, ""))
		nestedKeys := make([]string, 0, len(v))
		for k := range v {
			nestedKeys = append(nestedKeys, k)
		}
		sort.Strings(nestedKeys)
		for _, nestedKey := range nestedKeys {
			lines = append(lines, gui.formatPropertyLines(nestedKey, v[nestedKey], indent+"  ")...)
		}
	case []interface{}:
		lines = append(lines, fmt.Sprintf("%s%s%s:%s", colorBoldKey, indent+key, colorReset, ""))
		for i, item := range v {
			lines = append(lines, gui.formatPropertyLines(fmt.Sprintf("[%d]", i), item, indent+"  ")...)
		}
	case nil:
		lines = append(lines, fmt.Sprintf("%s%s%s:%s null", colorBoldKey, indent+key, colorReset, ""))
	default:
		lines = append(lines, fmt.Sprintf("%s%s%s:%s %v", colorBoldKey, indent+key, colorReset, "", v))
	}
	return lines
}

func (gui *Gui) updateStatus() {
	if gui.statusView == nil {
		return
	}

	gui.statusView.Clear()
	gui.mu.RLock()
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	// Get filtered counts
	subShowing, subTotal := gui.subList.GetFilterStats()
	rgShowing, rgTotal := gui.rgList.GetFilterStats()
	resShowing, resTotal := gui.resList.GetFilterStats()

	var status string
	switch activePanel {
	case "subscriptions":
		if gui.subList.IsFiltering() {
			status = fmt.Sprintf("Filter: \"%s\" (%d/%d) | Esc: Clear | Enter: Load RGs | c: Copy | o: Open | Tab: Switch | r: Refresh | q: Quit",
				gui.subList.GetFilterText(), subShowing, subTotal)
		} else {
			status = fmt.Sprintf("↑↓: Navigate | /: Search | Enter: Load RGs | c: Copy | o: Open | Tab: Switch | r: Refresh | q: Quit | Subs: %d", subTotal)
		}
	case "resourcegroups":
		if gui.rgList.IsFiltering() {
			status = fmt.Sprintf("Filter: \"%s\" (%d/%d) | Esc: Clear | Enter: Load Resources | c: Copy | o: Open | Tab: Switch | r: Refresh | q: Quit",
				gui.rgList.GetFilterText(), rgShowing, rgTotal)
		} else {
			status = fmt.Sprintf("↑↓: Navigate | /: Search | Enter: Load Resources | c: Copy | o: Open | Tab: Switch | []: Tabs | r: Refresh | q: Quit | RGs: %d", rgTotal)
		}
	case "resources":
		if gui.resList.IsFiltering() {
			status = fmt.Sprintf("Filter: \"%s\" (%d/%d) | Esc: Clear | Enter: View Details | c: Copy | o: Open | Tab: Switch | r: Refresh | q: Quit",
				gui.resList.GetFilterText(), resShowing, resTotal)
		} else {
			status = fmt.Sprintf("↑↓: Navigate | /: Search | Enter: View Details | c: Copy | o: Open | Tab: Switch | []: Tabs | r: Refresh | q: Quit | Resources: %d", resTotal)
		}
	case "main":
		if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
			if gui.isSearching {
				// Currently in search input mode
				status = fmt.Sprintf("/%s_ | n: Next | N: Prev | Esc: Cancel",
					gui.mainPanelSearch.GetSearchText())
				if gui.mainPanelSearch.GetMatchCount() > 0 {
					current, total := gui.mainPanelSearch.GetCurrentMatch()
					status = fmt.Sprintf("/%s_ | Match %d/%d | n: Next | N: Prev | Esc: Cancel",
						gui.mainPanelSearch.GetSearchText(), current, total)
				}
			} else {
				// Search active but not in input mode
				current, total := gui.mainPanelSearch.GetCurrentMatch()
				status = fmt.Sprintf("Match %d/%d for \"%s\" | n: Next | N: Prev | /: New Search | Esc: Clear",
					current, total, gui.mainPanelSearch.GetSearchText())
			}
		} else {
			status = fmt.Sprintf("↑/↓ or j/k: Scroll | PgUp/PgDn: Page | /: Search | c: Copy | o: Open | Tab: Back to List | []: Tabs | r: Refresh | q: Quit")
		}
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

		// Update filtered list
		gui.subList.SetItems(subs, func(sub *domain.Subscription) string {
			return formatWithGraySuffix(sub.DisplayString(), sub.GetDisplaySuffix())
		})

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

		// Update filtered list
		gui.rgList.SetItems(rgs, func(rg *domain.ResourceGroup) string {
			return formatWithGraySuffix(rg.DisplayString(), rg.GetDisplaySuffix())
		})
		// Clear resource list when switching subscriptions
		gui.resList.SetItems([]*domain.Resource{}, func(res *domain.Resource) string {
			return ""
		})

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

		// Update filtered list
		gui.resList.SetItems(resources, func(res *domain.Resource) string {
			return formatWithGraySuffix(res.DisplayString(), res.GetDisplaySuffix())
		})

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
