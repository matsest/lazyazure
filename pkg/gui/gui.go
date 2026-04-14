package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	"github.com/matsest/lazyazure/pkg/resources"
	"github.com/matsest/lazyazure/pkg/tasks"
	"github.com/matsest/lazyazure/pkg/utils"
)

// VersionInfo holds version information
type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

// ANSI color code for gray text (256-color palette)
const grayColor = "\x1b[38;5;245m"
const resetColor = "\x1b[0m"
const loadingSymbol = "↻"

// Timing constants
const (
	APITimeout             = 30 * time.Second
	ShortAPITimeout        = 10 * time.Second
	VersionCheckTimeout    = 10 * time.Second
	HTTPClientTimeout      = 10 * time.Second
	VersionDisplayDuration = 5 * time.Second
	StatusMessageDuration  = 2 * time.Second
	SemaphoreWaitThreshold = 100 * time.Millisecond
)

// Layout constants (in lines/characters)
const (
	AuthViewHeight = 5 // Height of the auth panel
)

// formatWithGraySuffix formats a name with a gray suffix in parentheses
func formatWithGraySuffix(name, suffix string) string {
	if suffix == "" {
		return name
	}
	return name + " " + grayColor + "(" + suffix + ")" + resetColor
}

// sortSubscriptions sorts subscriptions by name (case-insensitive) with pre-computed lowercase
func sortSubscriptions(subs []*domain.Subscription) {
	type item struct {
		sub       *domain.Subscription
		lowerName string
	}
	items := make([]item, len(subs))
	for i, s := range subs {
		items[i] = item{s, strings.ToLower(s.Name)}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].lowerName < items[j].lowerName
	})
	for i, it := range items {
		subs[i] = it.sub
	}
}

// sortResourceGroups sorts resource groups by name (case-insensitive) with pre-computed lowercase
func sortResourceGroups(rgs []*domain.ResourceGroup) {
	type item struct {
		rg        *domain.ResourceGroup
		lowerName string
	}
	items := make([]item, len(rgs))
	for i, rg := range rgs {
		items[i] = item{rg, strings.ToLower(rg.Name)}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].lowerName < items[j].lowerName
	})
	for i, it := range items {
		rgs[i] = it.rg
	}
}

// sortResources sorts resources by name (case-insensitive) with pre-computed lowercase
func sortResources(resources []*domain.Resource) {
	type item struct {
		res       *domain.Resource
		lowerName string
	}
	items := make([]item, len(resources))
	for i, r := range resources {
		items[i] = item{r, strings.ToLower(r.Name)}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].lowerName < items[j].lowerName
	})
	for i, it := range items {
		resources[i] = it.res
	}
}

// getLoadingText returns the current loading message if any panel is loading
func (gui *Gui) getLoadingText() string {
	gui.mu.RLock()
	defer gui.mu.RUnlock()

	// During refresh, show which panel triggered it
	if gui.refreshTriggeredBy != "" {
		switch gui.refreshTriggeredBy {
		case "subscriptions":
			return fmt.Sprintf("%s Refreshing subscriptions...", loadingSymbol)
		case "resourcegroups":
			return fmt.Sprintf("%s Refreshing resource groups...", loadingSymbol)
		case "resources":
			return fmt.Sprintf("%s Refreshing resources...", loadingSymbol)
		case "main":
			return fmt.Sprintf("%s Refreshing...", loadingSymbol)
		}
	}

	if gui.loadingSubs {
		return fmt.Sprintf("%s Loading subscriptions...", loadingSymbol)
	}
	if gui.loadingRGs {
		return fmt.Sprintf("%s Loading resource groups...", loadingSymbol)
	}
	if gui.loadingRes {
		return fmt.Sprintf("%s Loading resources...", loadingSymbol)
	}
	if gui.loadingUser {
		return fmt.Sprintf("%s Authenticating...", loadingSymbol)
	}
	return ""
}

// Gui is the main GUI controller
type Gui struct {
	g                   *gocui.Gui
	azureClient         AzureClient
	clientFactory       AzureClientFactory
	subClient           SubscriptionsClient
	rgClient            ResourceGroupsClient
	resClient           ResourcesClient
	resourceGraphClient ResourceGraphClient
	taskManager         *tasks.TaskManager

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

	// Type filter
	typePicker       *panels.TypePicker
	activeTypeFilter string // Current type filter (full Azure type string, or "" for none)

	// Global search state (cross-subscription Resource Graph search)
	globalSearchMode   bool               // True when showing global search results
	globalSearchQuery  string             // Current global search query
	globalSearchCancel context.CancelFunc // Cancel function for in-progress search

	// Subscription-level resource view (all resources in a subscription, no RG selected)
	subscriptionViewMode bool // True when showing all resources in a subscription

	// Version info
	versionInfo         VersionInfo
	latestVersion       string
	versionCheckPending bool
	showingVersion      bool
	versionTimer        *time.Timer

	// Loading state
	loadingSubs bool // Loading subscriptions
	loadingRGs  bool // Loading resource groups
	loadingRes  bool // Loading resources
	loadingUser bool // Loading user info

	// Track which panel triggered refresh for better loading messages
	refreshTriggeredBy string

	// Background preloading cache
	preloadCache *PreloadCache

	mu sync.RWMutex
}

// NewGui creates a new GUI instance
func NewGui(azureClient AzureClient, clientFactory AzureClientFactory, versionInfo VersionInfo) (*Gui, error) {
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
		versionInfo:     versionInfo,
		preloadCache:    NewPreloadCache(),
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

	// Initialize Resource Graph client (for cross-subscription queries)
	rgClient, err := gui.clientFactory.NewResourceGraphClient()
	if err != nil {
		utils.Log("Gui.Run: WARNING - could not initialize Resource Graph client: %v", err)
		// Non-fatal - type filtering will still work via client-side filtering
	} else {
		gui.resourceGraphClient = rgClient
		utils.Log("Gui.Run: Resource Graph client initialized")
	}
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
	authHeight := AuthViewHeight
	remainingHeight := maxY - authHeight - 2 // -2 for status bar
	// Divide remaining space: 20% for subscriptions, 30% for RGs, rest for resources
	subHeight := remainingHeight / 5       // 20%
	rgHeight := (remainingHeight * 3) / 10 // 30%

	// Status bar at bottom
	statusY := maxY - 2

	// 1. Auth panel (top, small)
	if v, err := gui.g.SetView("auth", 0, 0, sidebarWidth-1, authHeight, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
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
		if !errors.Is(err, gocui.ErrUnknownView) {
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
		if !errors.Is(err, gocui.ErrUnknownView) {
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
		if !errors.Is(err, gocui.ErrUnknownView) {
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
		if !errors.Is(err, gocui.ErrUnknownView) {
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
		if !errors.Is(err, gocui.ErrUnknownView) {
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

	// '?' to show version info - works in all views except search
	versionKeys := []string{"", "subscriptions", "resourcegroups", "resources", "main"}
	for _, view := range versionKeys {
		if err := gui.g.SetKeybinding(view, '?', gocui.ModNone, gui.showVersionInfo); err != nil {
			return err
		}
	}
	utils.Log("setupKeybindings: Version keybinding set")

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
	// Page up/down for subscriptions panel
	if err := gui.g.SetKeybinding("subscriptions", gocui.KeyPgup, gocui.ModNone, gui.pageUpSub); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("subscriptions", gocui.KeyPgdn, gocui.ModNone, gui.pageDownSub); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Subscriptions navigation set")

	// Global search (G) - cross-subscription search using Resource Graph
	if err := gui.g.SetKeybinding("subscriptions", 'G', gocui.ModNone, gui.startGlobalSearch); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Global search keybinding set")

	// All resources in subscription (A) - view all resources without selecting RG
	if err := gui.g.SetKeybinding("subscriptions", 'A', gocui.ModNone, gui.viewAllSubscriptionResources); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Subscription resources keybinding set")

	// Type filter (T) in subscriptions panel - set filter before global search
	if err := gui.g.SetKeybinding("subscriptions", 'T', gocui.ModNone, gui.showTypePicker); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Type filter in subscriptions keybinding set")

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
	// Page up/down for resource groups panel
	if err := gui.g.SetKeybinding("resourcegroups", gocui.KeyPgup, gocui.ModNone, gui.pageUpRG); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resourcegroups", gocui.KeyPgdn, gocui.ModNone, gui.pageDownRG); err != nil {
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
	// Page up/down for resources panel
	if err := gui.g.SetKeybinding("resources", gocui.KeyPgup, gocui.ModNone, gui.pageUpRes); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("resources", gocui.KeyPgdn, gocui.ModNone, gui.pageDownRes); err != nil {
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
	if err := gui.g.SetKeybinding("", gocui.MouseRight, gocui.ModNone, gui.copyPortalUrl); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Copy portal link key set")

	// Open portal link in browser (global)
	if err := gui.g.SetKeybinding("", 'o', gocui.ModNone, gui.openPortalUrl); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.MouseMiddle, gocui.ModNone, gui.openPortalUrl); err != nil {
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

	// Type filter (T) - opens type picker modal for resources panel
	if err := gui.g.SetKeybinding("resources", 'T', gocui.ModNone, gui.showTypePicker); err != nil {
		return err
	}
	utils.Log("setupKeybindings: Type filter keybinding set")

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
	utils.LogMetrics()
	utils.Log("quit: Metrics logged, shutting down")
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

	gui.mu.RLock()
	showingVersion := gui.showingVersion
	gui.mu.RUnlock()

	// First check if we're showing version info - if so, clear it
	if showingVersion {
		gui.clearVersionDisplay()
		return nil
	}

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
	gui.mu.RLock()
	showingVersion := gui.showingVersion
	gui.mu.RUnlock()

	// First check if we're showing version info - if so, clear it
	if showingVersion {
		gui.clearVersionDisplay()
		return nil
	}

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
	utils.Log("onMainPanelSearchChanged: Search active, length=%d", len(searchText))

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

	gui.mu.RLock()
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	utils.Log("onSearchChanged: Search active, length=%d, panel=%s", len(searchText), activePanel)

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
	showingVersion := gui.showingVersion
	globalSearchMode := gui.globalSearchMode
	subscriptionViewMode := gui.subscriptionViewMode
	gui.mu.RUnlock()

	// First check if we're showing version info - if so, clear it
	if showingVersion {
		gui.clearVersionDisplay()
		return nil
	}

	// Check if we're in global search mode - exit it
	if globalSearchMode && activePanel == "resources" {
		gui.exitGlobalSearchMode()
		return nil
	}

	// Check if we're in subscription view mode - exit it
	if subscriptionViewMode && activePanel == "resources" {
		gui.exitSubscriptionViewMode()
		return nil
	}

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
	// Set loading state
	gui.mu.Lock()
	gui.loadingUser = true
	gui.mu.Unlock()

	// Show loading in status bar immediately (blocking)
	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), ShortAPITimeout)
		defer cancel()

		user, err := gui.azureClient.GetUserInfo(ctx)
		if err != nil {
			utils.Log("loadUserInfo: Error: %v", err)
		} else {
			gui.mu.Lock()
			gui.currentUser = user
			gui.mu.Unlock()

			gui.g.UpdateAsync(func(g *gocui.Gui) error {
				gui.refreshAuthPanel()
				return nil
			})
		}

		// Clear loading state
		gui.mu.Lock()
		gui.loadingUser = false
		gui.mu.Unlock()

		// Refresh status bar
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.updateStatus()
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

	// Reset cursor and origin to top after refresh (e.g., after filtering)
	gui.subscriptionsView.SetOrigin(0, 0)
	gui.subscriptionsView.SetCursor(0, 0)
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

	// Reset cursor and origin to top after refresh (e.g., after filtering)
	gui.resourceGroupsView.SetOrigin(0, 0)
	gui.resourceGroupsView.SetCursor(0, 0)
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

	// Reset cursor and origin to top after refresh (e.g., after filtering)
	gui.resourcesView.SetOrigin(0, 0)
	gui.resourcesView.SetCursor(0, 0)
}

// Placeholder implementations for the rest
func (gui *Gui) nextSub(g *gocui.Gui, v *gocui.View) error {
	subCount := gui.subList.Len()

	if subCount == 0 {
		return nil
	}

	cx, cy := v.Cursor()
	_, height := v.Size()
	ox, oy := v.Origin()

	// Calculate absolute position and check if we can move
	if oy+cy < subCount-1 {
		if cy < height-1 {
			// Move cursor within visible area
			v.SetCursor(cx, cy+1)
		} else {
			// At bottom of view, scroll down
			v.SetOrigin(ox, oy+1)
		}
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
	ox, oy := v.Origin()

	// Calculate absolute position and check if we can move
	if oy+cy > 0 {
		if cy > 0 {
			// Move cursor within visible area
			v.SetCursor(cx, cy-1)
		} else {
			// At top of view, scroll up
			v.SetOrigin(ox, oy-1)
		}
		gui.updateSubscriptionSelection(v)
		gui.refreshMainPanel()
	}
	return nil
}

func (gui *Gui) updateSubscriptionSelection(v *gocui.View) {
	_, cy := v.Cursor()
	_, oy := v.Origin()
	// Calculate absolute index based on origin + cursor position
	if sub, ok := gui.subList.Get(oy + cy); ok {
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
	_, height := v.Size()
	ox, oy := v.Origin()

	// Calculate absolute position and check if we can move
	if oy+cy < rgCount-1 {
		if cy < height-1 {
			// Move cursor within visible area
			v.SetCursor(cx, cy+1)
		} else {
			// At bottom of view, scroll down
			v.SetOrigin(ox, oy+1)
		}
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
	ox, oy := v.Origin()

	// Calculate absolute position and check if we can move
	if oy+cy > 0 {
		if cy > 0 {
			// Move cursor within visible area
			v.SetCursor(cx, cy-1)
		} else {
			// At top of view, scroll up
			v.SetOrigin(ox, oy-1)
		}
		gui.updateRGSelection(v)
		gui.refreshMainPanel()
	}
	return nil
}

func (gui *Gui) updateRGSelection(v *gocui.View) {
	_, cy := v.Cursor()
	_, oy := v.Origin()
	// Calculate absolute index based on origin + cursor position
	if rg, ok := gui.rgList.Get(oy + cy); ok {
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
	_, height := v.Size()
	ox, oy := v.Origin()

	// Calculate absolute position and check if we can move
	if oy+cy < resCount-1 {
		if cy < height-1 {
			// Move cursor within visible area
			v.SetCursor(cx, cy+1)
		} else {
			// At bottom of view, scroll down
			v.SetOrigin(ox, oy+1)
		}
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
	ox, oy := v.Origin()

	// Calculate absolute position and check if we can move
	if oy+cy > 0 {
		if cy > 0 {
			// Move cursor within visible area
			v.SetCursor(cx, cy-1)
		} else {
			// At top of view, scroll up
			v.SetOrigin(ox, oy-1)
		}
		gui.updateResSelection(v)
		gui.refreshMainPanel()
	}
	return nil
}

func (gui *Gui) updateResSelection(v *gocui.View) {
	_, cy := v.Cursor()
	_, oy := v.Origin()
	// Calculate absolute index based on origin + cursor position
	if res, ok := gui.resList.Get(oy + cy); ok {
		gui.mu.Lock()
		gui.selectedRes = res
		gui.mu.Unlock()
		// Clear main panel search when switching resources
		if gui.mainPanelSearch != nil && gui.mainPanelSearch.IsActive() {
			gui.clearMainPanelSearch()
		}
	}
}

// Page up/down functions for subscriptions
func (gui *Gui) pageDownSub(g *gocui.Gui, v *gocui.View) error {
	subCount := gui.subList.Len()
	if subCount == 0 {
		return nil
	}

	_, height := v.Size()
	ox, oy := v.Origin()
	maxOy := subCount - height
	if maxOy < 0 {
		maxOy = 0
	}

	// Scroll down by page, keeping cursor position
	newOy := oy + height - 1
	if newOy > maxOy {
		newOy = maxOy
	}
	v.SetOrigin(ox, newOy)
	gui.updateSubscriptionSelection(v)
	gui.refreshMainPanel()
	return nil
}

func (gui *Gui) pageUpSub(g *gocui.Gui, v *gocui.View) error {
	subCount := gui.subList.Len()
	if subCount == 0 {
		return nil
	}

	_, height := v.Size()
	ox, oy := v.Origin()

	// Scroll up by page, keeping cursor position
	if oy > height-1 {
		v.SetOrigin(ox, oy-(height-1))
	} else if oy > 0 {
		v.SetOrigin(ox, 0)
	}
	gui.updateSubscriptionSelection(v)
	gui.refreshMainPanel()
	return nil
}

// Page up/down functions for resource groups
func (gui *Gui) pageDownRG(g *gocui.Gui, v *gocui.View) error {
	rgCount := gui.rgList.Len()
	if rgCount == 0 {
		return nil
	}

	_, height := v.Size()
	ox, oy := v.Origin()
	maxOy := rgCount - height
	if maxOy < 0 {
		maxOy = 0
	}

	// Scroll down by page, keeping cursor position
	newOy := oy + height - 1
	if newOy > maxOy {
		newOy = maxOy
	}
	v.SetOrigin(ox, newOy)
	gui.updateRGSelection(v)
	gui.refreshMainPanel()
	return nil
}

func (gui *Gui) pageUpRG(g *gocui.Gui, v *gocui.View) error {
	rgCount := gui.rgList.Len()
	if rgCount == 0 {
		return nil
	}

	_, height := v.Size()
	ox, oy := v.Origin()

	// Scroll up by page, keeping cursor position
	if oy > height-1 {
		v.SetOrigin(ox, oy-(height-1))
	} else if oy > 0 {
		v.SetOrigin(ox, 0)
	}
	gui.updateRGSelection(v)
	gui.refreshMainPanel()
	return nil
}

// Page up/down functions for resources
func (gui *Gui) pageDownRes(g *gocui.Gui, v *gocui.View) error {
	resCount := gui.resList.Len()
	if resCount == 0 {
		return nil
	}

	_, height := v.Size()
	ox, oy := v.Origin()
	maxOy := resCount - height
	if maxOy < 0 {
		maxOy = 0
	}

	// Scroll down by page, keeping cursor position
	newOy := oy + height - 1
	if newOy > maxOy {
		newOy = maxOy
	}
	v.SetOrigin(ox, newOy)
	gui.updateResSelection(v)
	gui.refreshMainPanel()
	return nil
}

func (gui *Gui) pageUpRes(g *gocui.Gui, v *gocui.View) error {
	resCount := gui.resList.Len()
	if resCount == 0 {
		return nil
	}

	_, height := v.Size()
	ox, oy := v.Origin()

	// Scroll up by page, keeping cursor position
	if oy > height-1 {
		v.SetOrigin(ox, oy-(height-1))
	} else if oy > 0 {
		v.SetOrigin(ox, 0)
	}
	gui.updateResSelection(v)
	gui.refreshMainPanel()
	return nil
}

func (gui *Gui) onSubEnter(g *gocui.Gui, v *gocui.View) error {
	if gui.subList.Len() == 0 {
		return nil
	}

	_, cy := v.Cursor()
	_, oy := v.Origin()
	// Calculate absolute index based on origin + cursor position
	if sub, ok := gui.subList.Get(oy + cy); ok {
		gui.mu.Lock()
		gui.selectedSub = sub
		subID := sub.ID
		gui.mu.Unlock()

		// Check if resource groups are already cached
		if rgs, ok := gui.preloadCache.GetRGs(subID); ok {
			utils.Log("onSubEnter: Using cached resource groups (%d RGs)", len(rgs))
			gui.useCachedResourceGroups(subID, rgs)
		} else {
			// Load resource groups normally
			gui.loadResourceGroups(subID)
		}

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
	_, oy := v.Origin()
	// Calculate absolute index based on origin + cursor position
	if rg, ok := gui.rgList.Get(oy + cy); ok {
		gui.mu.Lock()
		gui.selectedRG = rg
		rgName := rg.Name
		subID := gui.selectedSub.ID
		// Clear type filter when switching resource groups
		gui.activeTypeFilter = ""
		gui.mu.Unlock()

		// Check if resources are already cached for this resource group
		if resources, ok := gui.preloadCache.GetRes(subID, rgName); ok {
			utils.Log("onRGEnter: Using cached resources (%d resources)", len(resources))
			gui.useCachedResources(subID, rgName, resources)
		} else {
			// Load resources normally
			gui.loadResources(subID, rgName)
		}

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
	_, oy := v.Origin()
	// Calculate absolute index based on origin + cursor position
	if selectedRes, ok := gui.resList.Get(oy + cy); ok {
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
	activeTypeFilter := gui.activeTypeFilter
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
		// Update title based on mode
		gui.mu.RLock()
		globalSearchMode := gui.globalSearchMode
		subscriptionViewMode := gui.subscriptionViewMode
		gui.mu.RUnlock()

		if globalSearchMode {
			gui.resourcesView.Title = " Global Search Results "
		} else if subscriptionViewMode {
			gui.resourcesView.Title = " All Subscription Resources "
		} else if activeTypeFilter != "" {
			displayName := resources.GetResourceTypeDisplayName(activeTypeFilter)
			gui.resourcesView.Title = fmt.Sprintf(" Resources [%s] ", displayName)
		} else {
			gui.resourcesView.Title = " Resources "
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
	// Save current selection IDs to restore cursor positions after refresh
	gui.mu.RLock()
	savedSubID := ""
	savedRGID := ""
	savedResID := ""
	if gui.selectedSub != nil {
		savedSubID = gui.selectedSub.ID
	}
	if gui.selectedRG != nil {
		savedRGID = gui.selectedRG.ID
	}
	if gui.selectedRes != nil {
		savedResID = gui.selectedRes.ID
	}
	gui.mu.RUnlock()

	// Determine what we're refreshing based on active panel
	gui.mu.RLock()
	activePanel := gui.activePanel
	gui.mu.RUnlock()

	// Panel-specific refresh: only reload data for the current panel
	refreshSubs := activePanel == "subscriptions"
	refreshRGs := activePanel == "resourcegroups" && savedSubID != ""
	refreshRes := activePanel == "resources" && savedRGID != ""

	// Set loading states based on what will be refreshed
	gui.mu.Lock()
	gui.loadingSubs = refreshSubs
	gui.loadingRGs = refreshRGs
	gui.loadingRes = refreshRes
	// Store which panel triggered the refresh for better UX
	gui.refreshTriggeredBy = activePanel
	gui.mu.Unlock()

	// Show loading indicator immediately
	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	// Invalidate cache based on what panel is being refreshed
	if refreshSubs {
		gui.preloadCache.InvalidateSubs()
		utils.Log("refresh: Invalidated all caches (subscriptions refresh)")
	} else if refreshRGs {
		gui.preloadCache.InvalidateRGs(savedSubID)
		utils.Log("refresh: Invalidated RG cache for sub (RGs refresh)")
	} else if refreshRes {
		// Find RG name from ID for cache invalidation
		gui.mu.RLock()
		var rgName string
		if gui.selectedRG != nil {
			rgName = gui.selectedRG.Name
		}
		gui.mu.RUnlock()
		if rgName != "" {
			gui.preloadCache.InvalidateRes(savedSubID, rgName)
			utils.Log("refresh: Invalidated resource cache for RG (resources refresh)")
		}
	}

	// Reload data in a single goroutine to avoid race conditions
	go func() {
		startTime := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Log refresh start with ID presence indicators (not actual IDs for privacy)
		hasSub := savedSubID != ""
		hasRG := savedRGID != ""
		hasRes := savedResID != ""
		utils.Log("refresh: Starting panel-specific refresh (panel=%s, hasSub=%v, hasRG=%v, hasRes=%v)", activePanel, hasSub, hasRG, hasRes)

		// Clear filter for the panel being refreshed
		if refreshSubs {
			gui.subList.ClearFilter()
		}
		if refreshRGs {
			gui.rgList.ClearFilter()
		}
		if refreshRes {
			gui.resList.ClearFilter()
		}

		var subTargetIndex int = -1
		var rgTargetIndex int = -1
		var resTargetIndex int = -1

		// Load subscriptions only if on subscriptions panel
		if refreshSubs {
			gui.mu.RLock()
			subClient := gui.subClient
			gui.mu.RUnlock()

			if subClient != nil {
				subs, err := subClient.ListSubscriptions(ctx)
				if err != nil {
					utils.Log("refresh: Error loading subscriptions: %v", err)
				} else {
					// Sort subscriptions alphabetically
					sortSubscriptions(subs)

					gui.mu.Lock()
					gui.subscriptions = subs
					gui.mu.Unlock()

					gui.subList.SetItems(subs, func(sub *domain.Subscription) string {
						return formatWithGraySuffix(sub.DisplayString(), sub.GetDisplaySuffix())
					})

					// Find saved subscription index
					if savedSubID != "" {
						if idx, ok := gui.subList.FindIndex(func(sub *domain.Subscription) bool {
							return sub.ID == savedSubID
						}); ok {
							subTargetIndex = idx
							utils.Log("refresh: Found subscription at index %d", idx)
						} else {
							utils.Log("refresh: Saved subscription not found in new data")
						}
					}
				}
			}
		}

		// Load resource groups only if on resourcegroups panel
		if refreshRGs {
			gui.mu.RLock()
			rgClient := gui.rgClient
			gui.mu.RUnlock()

			if rgClient != nil && savedSubID != "" {
				rgs, err := rgClient.ListResourceGroups(ctx)
				if err != nil {
					utils.Log("refresh: Error loading resource groups: %v", err)
				} else {
					// Sort RGs alphabetically
					sortResourceGroups(rgs)

					gui.mu.Lock()
					gui.resourceGroups = rgs
					gui.mu.Unlock()

					gui.rgList.SetItems(rgs, func(rg *domain.ResourceGroup) string {
						return formatWithGraySuffix(rg.DisplayString(), rg.GetDisplaySuffix())
					})

					// Find saved RG index
					if savedRGID != "" {
						if idx, ok := gui.rgList.FindIndex(func(rg *domain.ResourceGroup) bool {
							return rg.ID == savedRGID
						}); ok {
							rgTargetIndex = idx
							utils.Log("refresh: Found RG at index %d", idx)
						} else {
							utils.Log("refresh: Saved resource group not found in new data")
						}
					}
				}
			}
		}

		// Load resources only if on resources panel
		if refreshRes {
			gui.mu.RLock()
			resClient := gui.resClient
			gui.mu.RUnlock()

			if resClient != nil && savedRGID != "" {
				resources, err := resClient.ListResourcesByResourceGroup(ctx, gui.selectedRG.Name)
				if err != nil {
					utils.Log("refresh: Error loading resources: %v", err)
				} else {
					// Sort resources alphabetically
					sortResources(resources)

					gui.mu.Lock()
					gui.resources = resources
					gui.mu.Unlock()

					gui.resList.SetItems(resources, func(res *domain.Resource) string {
						return formatWithGraySuffix(res.DisplayString(), res.GetDisplaySuffix())
					})

					// Find saved resource index
					if savedResID != "" {
						if idx, ok := gui.resList.FindIndex(func(res *domain.Resource) bool {
							return res.ID == savedResID
						}); ok {
							resTargetIndex = idx
							utils.Log("refresh: Found resource at index %d", idx)
						} else {
							utils.Log("refresh: Saved resource not found in new data")
						}
					}
				}
			}
		}

		utils.Log("refresh: Updating UI with subIndex=%d, rgIndex=%d, resIndex=%d",
			subTargetIndex, rgTargetIndex, resTargetIndex)

		// Helper function to set cursor and adjust origin to keep cursor visible
		setCursorWithOrigin := func(v *gocui.View, targetIndex int) {
			_, height := v.Size()
			// Calculate appropriate origin to keep cursor visible
			// If target is within view height, origin stays at 0
			// Otherwise, scroll so target is at bottom of view
			var originY int
			if targetIndex < height {
				originY = 0
			} else {
				originY = targetIndex - height + 1
			}
			v.SetOrigin(0, originY)
			v.SetCursor(0, targetIndex-originY)
		}

		// Clear loading states and refresh trigger
		gui.mu.Lock()
		gui.loadingSubs = false
		gui.loadingRGs = false
		gui.loadingRes = false
		gui.refreshTriggeredBy = ""
		gui.mu.Unlock()

		// Update panels in a single UI update
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			// Refresh only the panel(s) that were refreshed
			if refreshSubs {
				gui.refreshSubscriptionsPanel()
				if subTargetIndex >= 0 && gui.subscriptionsView != nil {
					setCursorWithOrigin(gui.subscriptionsView, subTargetIndex)
					gui.updateSubscriptionSelection(gui.subscriptionsView)
					utils.Log("refresh: Set subscription cursor to %d", subTargetIndex)
				}
			}

			if refreshRGs {
				gui.refreshResourceGroupsPanel()
				if rgTargetIndex >= 0 && gui.resourceGroupsView != nil {
					setCursorWithOrigin(gui.resourceGroupsView, rgTargetIndex)
					gui.updateRGSelection(gui.resourceGroupsView)
					utils.Log("refresh: Set RG cursor to %d", rgTargetIndex)
				}
			}

			if refreshRes {
				gui.refreshResourcesPanel()
				if resTargetIndex >= 0 && gui.resourcesView != nil {
					setCursorWithOrigin(gui.resourcesView, resTargetIndex)
					gui.updateResSelection(gui.resourcesView)
					utils.Log("refresh: Set resource cursor to %d", resTargetIndex)
				}
			}

			gui.refreshMainPanel()
			gui.updateStatus()
			utils.Log("refresh: Completed panel-specific refresh in %v", time.Since(startTime))
			return nil
		})
	}()

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
		utils.Log("copyPortalUrl: failed to copy %s URL to clipboard", itemType)
		return nil
	}

	utils.Log("copyPortalUrl: copied %s URL to clipboard", itemType)
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
		utils.Log("openPortalUrl: failed to open %s URL in browser", itemType)
		return nil
	}

	utils.Log("openPortalUrl: opened %s URL in browser", itemType)
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
		time.Sleep(StatusMessageDuration)
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
	lines = append(lines, fmt.Sprintf("%sSubscription:%s %s", colorBoldKey, colorReset, gui.getSubscriptionNameByID(res.SubscriptionID)))
	lines = append(lines, fmt.Sprintf("%sResource Group:%s %s", colorBoldKey, colorReset, res.ResourceGroup))
	lines = append(lines, fmt.Sprintf("%sID:%s %s", colorBoldKey, colorReset, res.ID))
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

// getSubscriptionNameByID looks up a subscription name by its ID
func (gui *Gui) getSubscriptionNameByID(subID string) string {
	gui.mu.RLock()
	defer gui.mu.RUnlock()

	for _, sub := range gui.subList.GetItems() {
		if sub.ID == subID {
			return sub.Name
		}
	}
	return subID // Fallback to ID if not found
}

// buildRGSummaryLines builds the summary view lines for a resource group
func (gui *Gui) buildRGSummaryLines(rg *domain.ResourceGroup) []string {
	var lines []string
	lines = append(lines, fmt.Sprintf("%sName:%s %s", colorBoldKey, colorReset, rg.Name))
	lines = append(lines, fmt.Sprintf("%sLocation:%s %s", colorBoldKey, colorReset, rg.Location))
	lines = append(lines, fmt.Sprintf("%sSubscription:%s %s", colorBoldKey, colorReset, gui.getSubscriptionNameByID(rg.SubscriptionID)))
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

	// Check if loading - takes precedence over other status messages
	loadingText := gui.getLoadingText()
	if loadingText != "" {
		fmt.Fprint(gui.statusView, loadingText)
		return
	}

	gui.mu.RLock()
	activePanel := gui.activePanel
	showingVersion := gui.showingVersion
	gui.mu.RUnlock()

	// If showing version info, display that instead
	if showingVersion {
		gui.renderVersionStatus()
		return
	}

	// Get filtered counts
	subShowing, subTotal := gui.subList.GetFilterStats()
	rgShowing, rgTotal := gui.rgList.GetFilterStats()
	resShowing, resTotal := gui.resList.GetFilterStats()

	var status string
	switch activePanel {
	case "subscriptions":
		// Check if in global search input mode
		gui.mu.RLock()
		globalSearchMode := gui.globalSearchMode
		globalSearchQuery := gui.globalSearchQuery
		typeFilter := gui.activeTypeFilter
		gui.mu.RUnlock()

		typeFilterName := ""
		if typeFilter != "" {
			typeFilterName = resources.GetResourceTypeDisplayName(typeFilter)
		}

		if globalSearchMode && gui.isSearching {
			if typeFilterName != "" {
				status = fmt.Sprintf("Global Search [%s]: %s_ | Enter: Search | Esc: Cancel", typeFilterName, globalSearchQuery)
			} else {
				status = fmt.Sprintf("Global Search: %s_ | Enter: Search | Esc: Cancel", globalSearchQuery)
			}
		} else if gui.subList.IsFiltering() {
			if typeFilterName != "" {
				status = fmt.Sprintf("Filter: \"%s\" (%d/%d) | Type: %s | Esc: Clear | T: Types | A: All | G: Global | c: Copy | q: Quit",
					gui.subList.GetFilterText(), subShowing, subTotal, typeFilterName)
			} else {
				status = fmt.Sprintf("Filter: \"%s\" (%d/%d) | Esc: Clear | Enter: Load RGs | T: Types | A: All | G: Global | c: Copy | q: Quit",
					gui.subList.GetFilterText(), subShowing, subTotal)
			}
		} else {
			if typeFilterName != "" {
				status = fmt.Sprintf("↑↓: Navigate | Type: %s | T: Types | /: Search | A: All | G: Global | c: Copy | o: Open | r: Refresh | q: Quit | Subs: %d",
					typeFilterName, subTotal)
			} else {
				status = fmt.Sprintf("↑↓: Navigate | /: Search | T: Types | Enter: Load RGs | A: All | G: Global | c: Copy | o: Open | r: Refresh | q: Quit | Subs: %d", subTotal)
			}
		}
	case "resourcegroups":
		if gui.rgList.IsFiltering() {
			status = fmt.Sprintf("Filter: \"%s\" (%d/%d) | Esc: Clear | Enter: Load Resources | c: Copy | o: Open | Tab: Switch | r: Refresh | q: Quit",
				gui.rgList.GetFilterText(), rgShowing, rgTotal)
		} else {
			status = fmt.Sprintf("↑↓: Navigate | /: Search | Enter: Load Resources | c: Copy | o: Open | Tab: Switch | []: Tabs | r: Refresh | q: Quit | RGs: %d", rgTotal)
		}
	case "resources":
		// Check if in global search mode or subscription view mode
		gui.mu.RLock()
		globalSearchMode := gui.globalSearchMode
		globalSearchQuery := gui.globalSearchQuery
		subscriptionViewMode := gui.subscriptionViewMode
		activeTypeFilter := gui.activeTypeFilter
		gui.mu.RUnlock()

		if globalSearchMode {
			// Global search results mode
			typeFilterName := ""
			if activeTypeFilter != "" {
				typeFilterName = resources.GetResourceTypeDisplayName(activeTypeFilter)
			}
			if gui.isSearching {
				if typeFilterName != "" {
					status = fmt.Sprintf("Global Search: %s_ [%s] | Enter: Search | Esc: Cancel", globalSearchQuery, typeFilterName)
				} else {
					status = fmt.Sprintf("Global Search: %s_ | Enter: Search | Esc: Cancel", globalSearchQuery)
				}
			} else {
				if typeFilterName != "" {
					status = fmt.Sprintf("Global: \"%s\" [%s] | ↑↓: Navigate | Enter: View | T: Types | Esc: Exit | c: Copy | o: Open | Results: %d",
						globalSearchQuery, typeFilterName, resTotal)
				} else {
					status = fmt.Sprintf("Global: \"%s\" | ↑↓: Navigate | Enter: View | T: Types | Esc: Exit | c: Copy | o: Open | Results: %d",
						globalSearchQuery, resTotal)
				}
			}
		} else if subscriptionViewMode {
			// Subscription-level resource view mode
			typeFilterName := gui.getTypeFilterDisplayName()
			if typeFilterName != "" {
				status = fmt.Sprintf("All Resources [%s] | ↑↓: Navigate | Enter: View Details | T: Types | Esc: Exit | c: Copy | o: Open | Results: %d",
					typeFilterName, resTotal)
			} else {
				status = fmt.Sprintf("All Subscription Resources | ↑↓: Navigate | /: Search | T: Filter Type | Enter: View Details | Esc: Exit | c: Copy | o: Open | Results: %d", resTotal)
			}
		} else {
			// Normal resources panel
			typeFilterName := gui.getTypeFilterDisplayName()
			if gui.resList.IsFiltering() {
				if typeFilterName != "" {
					status = fmt.Sprintf("Filter: \"%s\" | Type: %s (%d/%d) | Esc: Clear | T: Types | c: Copy | o: Open | Tab: Switch | q: Quit",
						gui.resList.GetFilterText(), typeFilterName, resShowing, resTotal)
				} else {
					status = fmt.Sprintf("Filter: \"%s\" (%d/%d) | Esc: Clear | Enter: View Details | T: Types | c: Copy | o: Open | Tab: Switch | q: Quit",
						gui.resList.GetFilterText(), resShowing, resTotal)
				}
			} else if typeFilterName != "" {
				status = fmt.Sprintf("Type: %s (%d/%d) | T: Clear/Change | /: Search | Enter: View Details | c: Copy | o: Open | Tab: Switch | q: Quit",
					typeFilterName, resShowing, resTotal)
			} else {
				status = fmt.Sprintf("↑↓: Navigate | /: Search | T: Filter by Type | Enter: View Details | c: Copy | o: Open | Tab: Switch | []: Tabs | r: Refresh | q: Quit | Resources: %d", resTotal)
			}
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

	// Set loading state
	gui.mu.Lock()
	gui.loadingSubs = true
	gui.mu.Unlock()

	// Show loading in status bar immediately (blocking)
	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	go func() {
		startTime := time.Now()
		utils.Log("loadSubscriptions: Starting load")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		subs, err := subClient.ListSubscriptions(ctx)
		if err != nil {
			utils.Log("loadSubscriptions: Error after %v: %v", time.Since(startTime), err)
		} else {
			// Sort subscriptions alphabetically by name (case-insensitive)
			sortSubscriptions(subs)

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
				return nil
			})

			utils.Log("loadSubscriptions: Loaded %d subscriptions", len(subs))

			// Trigger background preload of resource groups if we have a selected subscription
			gui.mu.RLock()
			selectedSub := gui.selectedSub
			gui.mu.RUnlock()
			if selectedSub != nil {
				gui.preloadResourceGroups(selectedSub.ID)
			}
		}

		// Clear loading state
		gui.mu.Lock()
		gui.loadingSubs = false
		gui.mu.Unlock()

		// Refresh status bar
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.updateStatus()
			return nil
		})
	}()
}

func (gui *Gui) loadResourceGroups(subscriptionID string) {
	// Set loading state
	gui.mu.Lock()
	gui.loadingRGs = true
	gui.mu.Unlock()

	// Show loading in status bar immediately (blocking)
	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	go func() {
		startTime := time.Now()
		utils.Log("loadResourceGroups: Starting load")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		rgClient, err := gui.clientFactory.NewResourceGroupsClient(subscriptionID)
		if err != nil {
			utils.Log("loadResourceGroups: Error creating client after %v: %v", time.Since(startTime), err)
		} else {
			rgs, err := rgClient.ListResourceGroups(ctx)
			if err != nil {
				utils.Log("loadResourceGroups: Error listing RGs after %v: %v", time.Since(startTime), err)
			} else {
				// Sort resource groups alphabetically by name (case-insensitive)
				sortResourceGroups(rgs)

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
					return nil
				})

				utils.Log("loadResourceGroups: Loaded %d resource groups", len(rgs))

				// Trigger background preload of resources for top 5 RGs
				gui.preloadTopResources(subscriptionID, rgs)
			}
		}

		// Clear loading state
		gui.mu.Lock()
		gui.loadingRGs = false
		gui.mu.Unlock()

		// Refresh status bar
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.updateStatus()
			return nil
		})
	}()
}

func (gui *Gui) loadResources(subscriptionID string, resourceGroupName string) {
	// Set loading state
	gui.mu.Lock()
	gui.loadingRes = true
	gui.mu.Unlock()

	// Show loading in status bar immediately (blocking)
	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	go func() {
		startTime := time.Now()
		utils.Log("loadResources: Starting load")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resClient, err := gui.clientFactory.NewResourcesClient(subscriptionID)
		if err != nil {
			utils.Log("loadResources: Error creating client after %v: %v", time.Since(startTime), err)
		} else {
			resources, err := resClient.ListResourcesByResourceGroup(ctx, resourceGroupName)
			if err != nil {
				utils.Log("loadResources: Error listing resources after %v: %v", time.Since(startTime), err)
			} else {
				// Sort resources alphabetically by name (case-insensitive)
				sortResources(resources)

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
					return nil
				})

				utils.Log("loadResources: Loaded %d resources", len(resources))
			}
		}

		// Clear loading state
		gui.mu.Lock()
		gui.loadingRes = false
		gui.mu.Unlock()

		// Refresh status bar
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.updateStatus()
			return nil
		})
	}()
}

// loadSubscriptionsWithSelection loads subscriptions and restores cursor position
func (gui *Gui) loadSubscriptionsWithSelection(savedSubID string) {
	gui.mu.RLock()
	subClient := gui.subClient
	gui.mu.RUnlock()

	if subClient == nil {
		return
	}

	// Set loading state
	gui.mu.Lock()
	gui.loadingSubs = true
	gui.mu.Unlock()

	// Show loading in status bar immediately (blocking)
	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		subs, err := subClient.ListSubscriptions(ctx)
		if err != nil {
			utils.Log("loadSubscriptionsWithSelection: Error: %v", err)
		} else {
			// Sort subscriptions alphabetically by name (case-insensitive)
			sortSubscriptions(subs)

			gui.mu.Lock()
			gui.subscriptions = subs
			// Only set default selection if no saved selection exists
			if len(subs) > 0 && gui.selectedSub == nil && savedSubID == "" {
				gui.selectedSub = subs[0]
			}
			gui.mu.Unlock()

			// Update filtered list
			gui.subList.SetItems(subs, func(sub *domain.Subscription) string {
				return formatWithGraySuffix(sub.DisplayString(), sub.GetDisplaySuffix())
			})

			// Find the index of the saved subscription in the filtered list
			var targetIndex int = -1
			if savedSubID != "" {
				if idx, ok := gui.subList.FindIndex(func(sub *domain.Subscription) bool {
					return sub.ID == savedSubID
				}); ok {
					targetIndex = idx
				}
			}

			gui.g.UpdateAsync(func(g *gocui.Gui) error {
				gui.refreshSubscriptionsPanel()
				// Restore cursor position if we found the saved subscription
				if targetIndex >= 0 && gui.subscriptionsView != nil {
					gui.subscriptionsView.SetCursor(0, targetIndex)
					// Update selection to match cursor
					gui.updateSubscriptionSelection(gui.subscriptionsView)
				}
				gui.refreshMainPanel()
				return nil
			})
		}

		// Clear loading state
		gui.mu.Lock()
		gui.loadingSubs = false
		gui.mu.Unlock()

		// Refresh status bar
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.updateStatus()
			return nil
		})
	}()
}

// loadResourceGroupsWithSelection loads resource groups and restores cursor position
func (gui *Gui) loadResourceGroupsWithSelection(subscriptionID, savedRGID string) {
	// Set loading state
	gui.mu.Lock()
	gui.loadingRGs = true
	gui.mu.Unlock()

	// Show loading in status bar immediately (blocking)
	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		rgClient, err := gui.clientFactory.NewResourceGroupsClient(subscriptionID)
		if err != nil {
			utils.Log("loadResourceGroupsWithSelection: Error creating client: %v", err)
		} else {
			rgs, err := rgClient.ListResourceGroups(ctx)
			if err != nil {
				utils.Log("loadResourceGroupsWithSelection: Error listing RGs: %v", err)
			} else {
				// Sort resource groups alphabetically by name (case-insensitive)
				sortResourceGroups(rgs)

				gui.mu.Lock()
				gui.resourceGroups = rgs
				gui.rgClient = rgClient
				// Only set default selection if no saved selection exists
				if len(rgs) > 0 && savedRGID == "" {
					gui.selectedRG = rgs[0]
				}
				gui.mu.Unlock()

				// Update filtered list
				gui.rgList.SetItems(rgs, func(rg *domain.ResourceGroup) string {
					return formatWithGraySuffix(rg.DisplayString(), rg.GetDisplaySuffix())
				})

				// Find the index of the saved resource group in the filtered list
				var targetIndex int = -1
				if savedRGID != "" {
					if idx, ok := gui.rgList.FindIndex(func(rg *domain.ResourceGroup) bool {
						return rg.ID == savedRGID
					}); ok {
						targetIndex = idx
					}
				}

				gui.g.UpdateAsync(func(g *gocui.Gui) error {
					gui.refreshResourceGroupsPanel()
					// Restore cursor position if we found the saved resource group
					if targetIndex >= 0 && gui.resourceGroupsView != nil {
						gui.resourceGroupsView.SetCursor(0, targetIndex)
						// Update selection to match cursor
						gui.updateRGSelection(gui.resourceGroupsView)
					}
					gui.refreshResourcesPanel()
					gui.refreshMainPanel()
					return nil
				})
			}
		}

		// Clear loading state
		gui.mu.Lock()
		gui.loadingRGs = false
		gui.mu.Unlock()

		// Refresh status bar
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.updateStatus()
			return nil
		})
	}()
}

// loadResourcesWithSelection loads resources and restores cursor position
func (gui *Gui) loadResourcesWithSelection(subscriptionID, resourceGroupName, savedResID string) {
	// Set loading state
	gui.mu.Lock()
	gui.loadingRes = true
	gui.mu.Unlock()

	// Show loading in status bar immediately (blocking)
	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resClient, err := gui.clientFactory.NewResourcesClient(subscriptionID)
		if err != nil {
			utils.Log("loadResourcesWithSelection: Error creating client: %v", err)
		} else {
			resources, err := resClient.ListResourcesByResourceGroup(ctx, resourceGroupName)
			if err != nil {
				utils.Log("loadResourcesWithSelection: Error listing resources: %v", err)
			} else {
				// Sort resources alphabetically by name (case-insensitive)
				sortResources(resources)

				gui.mu.Lock()
				gui.resources = resources
				gui.resClient = resClient
				// Only set default selection if no saved selection exists
				if len(resources) > 0 && savedResID == "" {
					gui.selectedRes = resources[0]
				}
				gui.mu.Unlock()

				// Update filtered list
				gui.resList.SetItems(resources, func(res *domain.Resource) string {
					return formatWithGraySuffix(res.DisplayString(), res.GetDisplaySuffix())
				})

				// Find the index of the saved resource in the filtered list
				var targetIndex int = -1
				if savedResID != "" {
					if idx, ok := gui.resList.FindIndex(func(res *domain.Resource) bool {
						return res.ID == savedResID
					}); ok {
						targetIndex = idx
					}
				}

				gui.g.UpdateAsync(func(g *gocui.Gui) error {
					gui.refreshResourcesPanel()
					// Restore cursor position if we found the saved resource
					if targetIndex >= 0 && gui.resourcesView != nil {
						gui.resourcesView.SetCursor(0, targetIndex)
						// Update selection to match cursor
						gui.updateResSelection(gui.resourcesView)
					}
					gui.refreshMainPanel()
					return nil
				})
			}
		}

		// Clear loading state
		gui.mu.Lock()
		gui.loadingRes = false
		gui.mu.Unlock()

		// Refresh status bar
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.updateStatus()
			return nil
		})
	}()
}

// loadResourceDetails fetches full resource details with provider-specific API version
func (gui *Gui) loadResourceDetails(originalRes *domain.Resource) {
	// Set loading state (using loadingRes since this is loading resource details)
	gui.mu.Lock()
	gui.loadingRes = true
	gui.mu.Unlock()

	// Show loading in status bar immediately (blocking)
	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	go func() {
		// Check if full resource details are already cached
		if cachedRes, ok := gui.preloadCache.GetFullRes(originalRes.ID); ok {
			utils.Log("loadResourceDetails: Using cached full details")

			gui.mu.Lock()
			gui.selectedRes = cachedRes
			gui.loadingRes = false
			gui.mu.Unlock()

			gui.g.UpdateAsync(func(g *gocui.Gui) error {
				gui.refreshMainPanel()
				gui.updatePanelTitles()
				gui.updateStatus()
				return nil
			})
			return
		}

		// Check if already loading
		if gui.preloadCache.IsFullResLoading(originalRes.ID) {
			utils.Log("loadResourceDetails: Already loading, skipping duplicate request")
			return
		}

		// Mark as loading
		gui.preloadCache.SetFullResLoading(originalRes.ID, true)
		utils.Log("loadResourceDetails: Starting to load full details")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		defer gui.preloadCache.SetFullResLoading(originalRes.ID, false)

		gui.mu.RLock()
		resClient := gui.resClient
		subscriptionID := ""
		// Prefer the resource's subscription ID (for global search results)
		// Fall back to selected subscription (for normal browsing)
		if originalRes.SubscriptionID != "" {
			subscriptionID = originalRes.SubscriptionID
		} else if gui.selectedSub != nil {
			subscriptionID = gui.selectedSub.ID
		}
		gui.mu.RUnlock()

		// Create client if not available or if subscription doesn't match
		// (e.g., when using cached resources or global search results from different subscription)
		needNewClient := resClient == nil
		if !needNewClient && subscriptionID != "" {
			// Check if current client is for a different subscription
			gui.mu.RLock()
			currentSubID := ""
			if gui.selectedSub != nil {
				currentSubID = gui.selectedSub.ID
			}
			gui.mu.RUnlock()
			needNewClient = currentSubID != subscriptionID
		}

		if needNewClient {
			if subscriptionID == "" {
				utils.Log("loadResourceDetails: No subscription available")
				gui.mu.Lock()
				gui.loadingRes = false
				gui.mu.Unlock()
				gui.g.UpdateAsync(func(g *gocui.Gui) error {
					gui.updateStatus()
					return nil
				})
				return
			}

			var err error
			resClient, err = gui.clientFactory.NewResourcesClient(subscriptionID)
			if err != nil {
				utils.Log("loadResourceDetails: Error creating resources client: %v", err)
				gui.mu.Lock()
				gui.loadingRes = false
				gui.mu.Unlock()
				gui.g.UpdateAsync(func(g *gocui.Gui) error {
					gui.updateStatus()
					return nil
				})
				return
			}
			utils.Log("loadResourceDetails: Created new client for subscription")
		}

		// Fetch full resource details with provider-specific properties
		resource, err := resClient.GetResource(ctx, originalRes.ID, originalRes.Type)
		if err != nil {
			utils.Log("loadResourceDetails: Error loading resource, type=%s, error=%v", originalRes.Type, err)
		} else {
			// Preserve createdTime and changedTime from the original list data
			// (these aren't returned by GetByID but were in the list view)
			resource.CreatedTime = originalRes.CreatedTime
			resource.ChangedTime = originalRes.ChangedTime

			// Cache the full resource details
			gui.preloadCache.SetFullRes(originalRes.ID, resource, cancel)
			utils.Log("loadResourceDetails: Cached full details for future use")

			gui.mu.Lock()
			gui.selectedRes = resource
			gui.mu.Unlock()

			gui.g.UpdateAsync(func(g *gocui.Gui) error {
				gui.refreshMainPanel()
				gui.updatePanelTitles() // Restore focus indicator
				return nil
			})
		}

		// Clear loading state
		gui.mu.Lock()
		gui.loadingRes = false
		gui.mu.Unlock()

		// Refresh status bar
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.updateStatus()
			return nil
		})

		utils.Log("loadResourceDetails: Completed")
	}()
}

// GitHubRelease represents a GitHub release API response
type GitHubRelease struct {
	TagName string `json:"tag_name"`
}

// showVersionInfo displays version information and checks for updates
func (gui *Gui) showVersionInfo(g *gocui.Gui, v *gocui.View) error {
	gui.mu.Lock()
	defer gui.mu.Unlock()

	// Cancel any existing timer
	if gui.versionTimer != nil {
		gui.versionTimer.Stop()
	}

	gui.showingVersion = true

	// Check if we already have the latest version cached
	if gui.latestVersion == "" && !gui.versionCheckPending {
		gui.versionCheckPending = true
		go gui.checkLatestVersion()
	}

	// Update status immediately
	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	// Start auto-revert timer (5 seconds)
	gui.versionTimer = time.AfterFunc(VersionDisplayDuration, func() {
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.clearVersionDisplay()
			return nil
		})
	})

	return nil
}

// clearVersionDisplay reverts the status bar to normal
func (gui *Gui) clearVersionDisplay() {
	gui.mu.Lock()
	if gui.versionTimer != nil {
		gui.versionTimer.Stop()
		gui.versionTimer = nil
	}
	gui.showingVersion = false
	gui.mu.Unlock()

	gui.updateStatus()
}

// checkLatestVersion fetches the latest release from GitHub
func (gui *Gui) checkLatestVersion() {
	ctx, cancel := context.WithTimeout(context.Background(), ShortAPITimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/matsest/lazyazure/releases/latest", nil)
	if err != nil {
		utils.Log("checkLatestVersion: Failed to create request: %v", err)
		gui.versionCheckComplete("")
		return
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "lazyazure")

	client := &http.Client{Timeout: HTTPClientTimeout}
	resp, err := client.Do(req)
	if err != nil {
		utils.Log("checkLatestVersion: Failed to fetch release: %v", err)
		gui.versionCheckComplete("")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		utils.Log("checkLatestVersion: GitHub API returned status %d", resp.StatusCode)
		gui.versionCheckComplete("")
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Log("checkLatestVersion: Failed to read response: %v", err)
		gui.versionCheckComplete("")
		return
	}

	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		utils.Log("checkLatestVersion: Failed to parse response: %v", err)
		gui.versionCheckComplete("")
		return
	}

	gui.versionCheckComplete(release.TagName)
}

// versionCheckComplete updates the GUI after version check completes
func (gui *Gui) versionCheckComplete(latestVersion string) {
	gui.mu.Lock()
	gui.latestVersion = latestVersion
	gui.versionCheckPending = false
	gui.mu.Unlock()

	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.mu.RLock()
		showingVersion := gui.showingVersion
		gui.mu.RUnlock()

		if showingVersion {
			gui.updateStatus()
		}
		return nil
	})
}

// versionNeedsUpdate checks if current version is older than latest
func (gui *Gui) versionNeedsUpdate() bool {
	// Simple string comparison - assumes versions are in format vX.Y.Z
	// This is a basic check; for production, use semver comparison
	if gui.latestVersion == "" || gui.versionInfo.Version == "dev" {
		return false
	}
	// Normalize by stripping "v" prefix from both versions
	latest := strings.TrimPrefix(gui.latestVersion, "v")
	current := strings.TrimPrefix(gui.versionInfo.Version, "v")
	return latest != current
}

// isDevelopmentBuild checks if this is a development/non-release build
func (gui *Gui) isDevelopmentBuild() bool {
	version := gui.versionInfo.Version
	// "dev" is the default for plain go build
	if version == "dev" {
		return true
	}
	// Check for git describe indicators of dev builds:
	// - "dirty" = uncommitted changes
	// - "-g" or "-g[hex]" = commit hash (git describe format)
	// - distance from tag (e.g., "-2-gc15ffdf")
	if strings.Contains(version, "dirty") {
		return true
	}
	if strings.Contains(version, "-") {
		// Check if it contains a commit hash pattern (-g[hex])
		// This handles cases like v0.2.1-2-gc15ffdf
		parts := strings.Split(version, "-")
		for i, part := range parts {
			// Skip the first part (the tag version like "v0.2.1")
			if i == 0 {
				continue
			}
			// Check for git hash pattern: starts with 'g' followed by hex chars
			if len(part) > 1 && part[0] == 'g' {
				return true
			}
		}
	}
	return false
}

// renderVersionStatus renders version information in the status bar
func (gui *Gui) renderVersionStatus() {
	gui.mu.RLock()
	version := gui.versionInfo.Version
	commit := gui.versionInfo.Commit
	latestVersion := gui.latestVersion
	checkPending := gui.versionCheckPending
	gui.mu.RUnlock()

	// Shorten commit for display
	displayCommit := commit
	if len(displayCommit) > 7 {
		displayCommit = displayCommit[:7]
	}

	// Handle development builds specially
	if gui.isDevelopmentBuild() {
		if checkPending {
			fmt.Fprintf(gui.statusView, "lazyazure %s (%s) - Checking for updates... | Esc: Dismiss", version, displayCommit)
		} else {
			fmt.Fprintf(gui.statusView, "lazyazure %s (%s) - Development build | Esc: Dismiss", version, displayCommit)
		}
		return
	}

	var status string
	if checkPending {
		status = fmt.Sprintf("lazyazure %s (%s) - Checking for updates... | Esc: Dismiss", version, displayCommit)
	} else if latestVersion == "" {
		status = fmt.Sprintf("lazyazure %s (%s) - Update check failed | Esc: Dismiss", version, displayCommit)
	} else if gui.versionNeedsUpdate() {
		status = fmt.Sprintf("lazyazure %s (%s) - Update available: %s | Esc: Dismiss", version, displayCommit, latestVersion)
	} else {
		status = fmt.Sprintf("lazyazure %s (%s) - Up to date | Esc: Dismiss", version, displayCommit)
	}

	fmt.Fprint(gui.statusView, status)
}

// useCachedResourceGroups uses cached resource groups data to update the UI
// This is called when cache hit occurs in onSubEnter
func (gui *Gui) useCachedResourceGroups(subscriptionID string, rgs []*domain.ResourceGroup) {
	// Set loading state briefly for consistency
	gui.mu.Lock()
	gui.loadingRGs = true
	gui.mu.Unlock()

	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	// Update data immediately (no async needed since we have the data)
	gui.mu.Lock()
	gui.resourceGroups = rgs
	gui.rgClient = nil // Will be created on demand if needed
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
		return nil
	})

	// Clear loading state
	gui.mu.Lock()
	gui.loadingRGs = false
	gui.mu.Unlock()

	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	// Trigger background preload of resources for top 5 RGs
	gui.preloadTopResources(subscriptionID, rgs)
}

// useCachedResources uses cached resources data to update the UI
// This is called when cache hit occurs in onRGEnter
func (gui *Gui) useCachedResources(subscriptionID string, rgName string, resources []*domain.Resource) {
	// Set loading state briefly for consistency
	gui.mu.Lock()
	gui.loadingRes = true
	gui.mu.Unlock()

	gui.g.Update(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	// Update data immediately (no async needed since we have the data)
	gui.mu.Lock()
	gui.resources = resources
	gui.resClient = nil // Will be created on demand if needed
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
		return nil
	})

	// Clear loading state
	gui.mu.Lock()
	gui.loadingRes = false
	gui.mu.Unlock()

	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})
}

// preloadResourceGroups loads resource groups for a subscription in the background
// and stores them in cache. This is called silently without UI updates.
func (gui *Gui) preloadResourceGroups(subscriptionID string) {
	// Check if already cached and not expired
	if _, ok := gui.preloadCache.GetRGs(subscriptionID); ok {
		utils.Log("preloadResourceGroups: Already cached, skipping")
		return
	}

	// Try to start loading - returns false if another goroutine is already loading
	if !gui.preloadCache.TryStartRGLoading(subscriptionID) {
		utils.Log("preloadResourceGroups: Already in progress, skipping")
		return
	}

	utils.Log("preloadResourceGroups: Starting preload")

	ctx, cancel := context.WithCancel(context.Background())

	// Acquire semaphore slot before starting goroutine
	semaphore := gui.preloadCache.GetSemaphore()
	acquireStart := time.Now()
	if err := semaphore.Acquire(ctx); err != nil {
		// Context cancelled (user switched away), abort
		utils.Log("preloadResourceGroups: Context cancelled, aborting")
		gui.preloadCache.SetRGLoading(subscriptionID, false)
		cancel()
		return
	}
	acquireDuration := time.Since(acquireStart)
	if acquireDuration > SemaphoreWaitThreshold {
		inUse, capacity := semaphore.GetUtilization()
		utils.Log("preloadResourceGroups: Waited %v for semaphore slot (%d/%d in use)", acquireDuration, inUse, capacity)
	}

	go func() {
		defer cancel()
		defer semaphore.Release()
		defer gui.preloadCache.SetRGLoading(subscriptionID, false)

		rgClient, err := gui.clientFactory.NewResourceGroupsClient(subscriptionID)
		if err != nil {
			utils.Log("preloadResourceGroups: Error creating client: %v", err)
			return
		}

		rgs, err := rgClient.ListResourceGroups(ctx)
		if err != nil {
			utils.Log("preloadResourceGroups: Error listing RGs: %v", err)
			return
		}

		// Sort resource groups alphabetically
		sortResourceGroups(rgs)

		// Store in cache
		gui.preloadCache.SetRGs(subscriptionID, rgs, cancel)
		utils.Log("preloadResourceGroups: Cached %d RGs", len(rgs))

		// Now preload resources for top 10 RGs
		gui.preloadTopResources(subscriptionID, rgs)
	}()
}

// preloadTopResources loads resources for the first 10 resource groups in the background
func (gui *Gui) preloadTopResources(subscriptionID string, rgs []*domain.ResourceGroup) {
	if len(rgs) == 0 {
		return
	}

	// Limit to top 10 RGs for better coverage
	count := 10
	if len(rgs) < count {
		count = len(rgs)
	}

	utils.Log("preloadTopResources: Preloading resources for top %d RGs", count)

	semaphore := gui.preloadCache.GetSemaphore()

	for i := 0; i < count; i++ {
		rgName := rgs[i].Name

		// Check if already cached
		if _, ok := gui.preloadCache.GetRes(subscriptionID, rgName); ok {
			utils.Log("preloadTopResources: Already cached for RG #%d, skipping", i)
			continue
		}

		// Try to start loading - returns false if another goroutine is already loading
		if !gui.preloadCache.TryStartResLoading(subscriptionID, rgName) {
			utils.Log("preloadTopResources: Already loading for RG #%d, skipping", i)
			continue
		}

		// Create context for this preload operation
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		// Acquire semaphore slot before starting goroutine
		acquireStart := time.Now()
		if err := semaphore.Acquire(ctx); err != nil {
			// Context cancelled or timeout, abort this preload
			utils.Log("preloadTopResources: Could not acquire semaphore for RG #%d, aborting", i)
			gui.preloadCache.SetResLoading(subscriptionID, rgName, false)
			cancel()
			continue
		}
		acquireDuration := time.Since(acquireStart)
		if acquireDuration > SemaphoreWaitThreshold {
			inUse, capacity := semaphore.GetUtilization()
			utils.Log("preloadTopResources: Waited %v for semaphore slot (RG #%d, %d/%d in use)", acquireDuration, i, inUse, capacity)
		}

		// Preload this RG's resources in background
		go func(rgName string, index int) {
			defer semaphore.Release()
			defer gui.preloadCache.SetResLoading(subscriptionID, rgName, false)
			defer cancel()

			resClient, err := gui.clientFactory.NewResourcesClient(subscriptionID)
			if err != nil {
				utils.Log("preloadTopResources: Error creating client for RG #%d: %v", index, err)
				return
			}

			resources, err := resClient.ListResourcesByResourceGroup(ctx, rgName)
			if err != nil {
				utils.Log("preloadTopResources: Error listing resources for RG #%d: %v", index, err)
				return
			}

			// Sort resources alphabetically
			sortResources(resources)

			// Store in cache
			gui.preloadCache.SetRes(subscriptionID, rgName, resources, cancel)
			utils.Log("preloadTopResources: Cached %d resources for RG #%d", len(resources), index)
		}(rgName, i)
	}
}

// showTypePicker opens the type picker modal for filtering resources by type
func (gui *Gui) showTypePicker(g *gocui.Gui, v *gocui.View) error {
	utils.Log("showTypePicker: Opening type picker")

	// Create type picker if not exists
	if gui.typePicker == nil {
		gui.typePicker = panels.NewTypePicker(gui.g, gui.onTypeSelected, gui.onTypePickerCancel)
	}

	return gui.typePicker.Show()
}

// onTypeSelected is called when a type is selected in the type picker
func (gui *Gui) onTypeSelected(resourceType string) {
	utils.Log("onTypeSelected: Type selected (filter active: %v)", resourceType != "")

	gui.mu.Lock()
	gui.activeTypeFilter = resourceType
	prevPanel := gui.activePanel
	globalSearchMode := gui.globalSearchMode
	globalSearchQuery := gui.globalSearchQuery
	subscriptionViewMode := gui.subscriptionViewMode
	gui.mu.Unlock()

	// Restore focus to appropriate panel
	if globalSearchMode || subscriptionViewMode {
		gui.g.SetCurrentView("resources")
		gui.mu.Lock()
		gui.activePanel = "resources"
		gui.mu.Unlock()
	} else if prevPanel == "subscriptions" {
		// Stay in subscriptions panel if that's where we were
		gui.g.SetCurrentView("subscriptions")
		gui.mu.Lock()
		gui.activePanel = "subscriptions"
		gui.mu.Unlock()
	} else {
		gui.g.SetCurrentView("resources")
		gui.mu.Lock()
		gui.activePanel = "resources"
		gui.mu.Unlock()
	}

	// Handle special modes - re-execute the search with new filter
	if globalSearchMode && globalSearchQuery != "" {
		// Re-run global search with the new type filter
		gui.rerunGlobalSearch()
	} else if subscriptionViewMode {
		// Re-run subscription view with new type filter
		gui.rerunSubscriptionView()
	} else if resourceType != "" {
		// Apply client-side type filter to current resource list
		gui.applyTypeFilter()
	} else {
		// Clear filter - reload resources without filter
		gui.clearTypeFilter()
	}

	gui.updatePanelTitles()
	gui.updateStatus()

	utils.Log("onTypeSelected: Filter applied, restored focus from %s", prevPanel)
}

// onTypePickerCancel is called when the type picker is cancelled
func (gui *Gui) onTypePickerCancel() {
	utils.Log("onTypePickerCancel: Type picker cancelled")

	// Restore focus to appropriate panel
	gui.mu.RLock()
	globalSearchMode := gui.globalSearchMode
	subscriptionViewMode := gui.subscriptionViewMode
	gui.mu.RUnlock()

	// If we're in global search or subscription view mode, return to resources
	// Otherwise, return to the panel that was active before type picker
	if globalSearchMode || subscriptionViewMode {
		gui.g.SetCurrentView("resources")
		gui.mu.Lock()
		gui.activePanel = "resources"
		gui.mu.Unlock()
	} else {
		// Default to resources panel, but check if we have resources loaded
		gui.mu.RLock()
		hasResources := len(gui.resources) > 0
		gui.mu.RUnlock()

		if hasResources {
			gui.g.SetCurrentView("resources")
			gui.mu.Lock()
			gui.activePanel = "resources"
			gui.mu.Unlock()
		} else {
			// No resources loaded, return to subscriptions
			gui.g.SetCurrentView("subscriptions")
			gui.mu.Lock()
			gui.activePanel = "subscriptions"
			gui.mu.Unlock()
		}
	}

	gui.updatePanelTitles()
	gui.updateStatus()
}

// applyTypeFilter filters the current resource list by the active type filter
func (gui *Gui) applyTypeFilter() {
	gui.mu.RLock()
	activeFilter := gui.activeTypeFilter
	allResources := gui.resources
	gui.mu.RUnlock()

	if activeFilter == "" || len(allResources) == 0 {
		return
	}

	// Filter resources by type (case-insensitive comparison)
	filterLower := strings.ToLower(activeFilter)
	var filtered []*domain.Resource
	for _, res := range allResources {
		if strings.ToLower(res.Type) == filterLower {
			filtered = append(filtered, res)
		}
	}

	// Update the filtered list with type-filtered results
	gui.resList.SetItems(filtered, func(res *domain.Resource) string {
		return formatWithGraySuffix(res.DisplayString(), res.GetDisplaySuffix())
	})

	// Update selection
	gui.mu.Lock()
	if len(filtered) > 0 {
		gui.selectedRes = filtered[0]
	} else {
		gui.selectedRes = nil
	}
	gui.mu.Unlock()

	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.refreshResourcesPanel()
		gui.refreshMainPanel()
		return nil
	})

	utils.Log("applyTypeFilter: Filtered to %d/%d resources", len(filtered), len(allResources))
}

// clearTypeFilter removes the type filter and shows all resources
func (gui *Gui) clearTypeFilter() {
	gui.mu.RLock()
	allResources := gui.resources
	gui.mu.RUnlock()

	// Restore full resource list
	gui.resList.SetItems(allResources, func(res *domain.Resource) string {
		return formatWithGraySuffix(res.DisplayString(), res.GetDisplaySuffix())
	})

	// Update selection
	gui.mu.Lock()
	if len(allResources) > 0 {
		gui.selectedRes = allResources[0]
	} else {
		gui.selectedRes = nil
	}
	gui.mu.Unlock()

	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.refreshResourcesPanel()
		gui.refreshMainPanel()
		return nil
	})

	utils.Log("clearTypeFilter: Showing all %d resources", len(allResources))
}

// getTypeFilterDisplayName returns the display name for the active type filter
func (gui *Gui) getTypeFilterDisplayName() string {
	gui.mu.RLock()
	activeFilter := gui.activeTypeFilter
	gui.mu.RUnlock()

	if activeFilter == "" {
		return ""
	}

	// Get display name from resources package
	return resources.GetResourceTypeDisplayName(activeFilter)
}

// ============================================================================
// Global Search (Cross-Subscription Resource Graph Search)
// ============================================================================

// startGlobalSearch initiates a cross-subscription search using Resource Graph
func (gui *Gui) startGlobalSearch(g *gocui.Gui, v *gocui.View) error {
	utils.Log("startGlobalSearch: Starting global search mode")

	// Check if Resource Graph client is available
	gui.mu.RLock()
	rgClient := gui.resourceGraphClient
	gui.mu.RUnlock()

	if rgClient == nil {
		utils.Log("startGlobalSearch: Resource Graph client not available")
		// Show error in status bar
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			if gui.statusView != nil {
				gui.statusView.Clear()
				fmt.Fprint(gui.statusView, "Global search unavailable - Resource Graph client not initialized")
			}
			return nil
		})
		return nil
	}

	// Enter global search mode
	gui.mu.Lock()
	gui.globalSearchMode = true
	gui.globalSearchQuery = ""
	gui.mu.Unlock()

	// Create search bar with global search callbacks
	gui.searchBar = panels.NewSearchBar(gui.g, gui.onGlobalSearchChanged, gui.onGlobalSearchCancel, gui.onGlobalSearchConfirm)

	if err := gui.searchBar.Show(); err != nil {
		utils.Log("startGlobalSearch: ERROR showing search bar: %v", err)
		gui.mu.Lock()
		gui.globalSearchMode = false
		gui.mu.Unlock()
		return err
	}

	gui.isSearching = true
	gui.searchTarget = "global"
	gui.setupSearchKeybindings()

	// Clear resources panel to show placeholder
	gui.resList.SetItems([]*domain.Resource{}, func(res *domain.Resource) string { return "" })
	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.refreshResourcesPanel()
		gui.updatePanelTitles()
		gui.updateStatus()
		return nil
	})

	utils.Log("startGlobalSearch: Global search mode activated")
	return nil
}

// rerunGlobalSearch re-executes the global search with the current type filter
func (gui *Gui) rerunGlobalSearch() {
	gui.mu.RLock()
	query := gui.globalSearchQuery
	gui.mu.RUnlock()

	if query == "" {
		return
	}

	utils.Log("rerunGlobalSearch: Re-running search with query=%s", query)
	gui.executeGlobalSearch(query)
}

// rerunSubscriptionView re-executes the subscription view with the current type filter
func (gui *Gui) rerunSubscriptionView() {
	gui.mu.RLock()
	selectedSub := gui.selectedSub
	typeFilter := gui.activeTypeFilter
	rgClient := gui.resourceGraphClient
	gui.mu.RUnlock()

	if selectedSub == nil || rgClient == nil {
		return
	}

	utils.Log("rerunSubscriptionView: Re-running subscription view with typeFilter=%s", typeFilter)

	// Set loading state
	gui.mu.Lock()
	gui.loadingRes = true
	gui.mu.Unlock()

	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), APITimeout)
		defer cancel()

		var results []*domain.Resource
		var err error

		if typeFilter != "" {
			results, err = rgClient.ListResourcesByType(ctx, typeFilter, []string{selectedSub.ID})
		} else {
			results, err = rgClient.ListResourcesBySubscription(ctx, selectedSub.ID)
		}

		if err != nil {
			utils.Log("rerunSubscriptionView: Error: %v", err)
			gui.mu.Lock()
			gui.loadingRes = false
			gui.mu.Unlock()
			gui.g.UpdateAsync(func(g *gocui.Gui) error {
				gui.updateStatus()
				return nil
			})
			return
		}

		// Sort results
		sortResources(results)

		// Update resources with subscription view display format
		gui.resList.SetItems(results, func(res *domain.Resource) string {
			return gui.formatSubscriptionViewResult(res)
		})

		gui.mu.Lock()
		gui.resources = results
		if len(results) > 0 {
			gui.selectedRes = results[0]
		} else {
			gui.selectedRes = nil
		}
		gui.loadingRes = false
		gui.mu.Unlock()

		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.refreshResourcesPanel()
			gui.updateStatus()
			return nil
		})

		utils.Log("rerunSubscriptionView: Found %d results", len(results))
	}()
}

// onGlobalSearchChanged is called when the global search text changes
// This only updates the UI to show what the user is typing - no API calls
func (gui *Gui) onGlobalSearchChanged() {
	query := gui.searchBar.GetText()

	gui.mu.Lock()
	gui.globalSearchQuery = query
	gui.mu.Unlock()

	// Just update the status bar to show the query being typed
	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})
}

// onGlobalSearchConfirm is called when the user presses Enter in global search
func (gui *Gui) onGlobalSearchConfirm() {
	gui.mu.RLock()
	query := gui.globalSearchQuery
	gui.mu.RUnlock()

	utils.Log("onGlobalSearchConfirm: Confirming global search with query: %s", query)

	// Hide search bar but stay in global search mode
	gui.searchBar.Hide()
	gui.isSearching = false

	// Don't search for empty queries
	if query == "" {
		utils.Log("onGlobalSearchConfirm: Empty query, exiting global search")
		gui.exitGlobalSearchMode()
		return
	}

	// Switch focus to resources panel to navigate results
	gui.g.SetCurrentView("resources")
	gui.mu.Lock()
	gui.activePanel = "resources"
	gui.mu.Unlock()

	gui.updatePanelTitles()
	gui.updateStatus()

	// Execute the search
	gui.executeGlobalSearch(query)
}

// executeGlobalSearch performs the actual global search query
func (gui *Gui) executeGlobalSearch(query string) {
	utils.Log("executeGlobalSearch: START with query='%s'", query)

	gui.mu.RLock()
	rgClient := gui.resourceGraphClient
	typeFilter := gui.activeTypeFilter
	subs := gui.subscriptions
	gui.mu.RUnlock()

	utils.Log("executeGlobalSearch: rgClient=%v, typeFilter='%s', subs=%d", rgClient != nil, typeFilter, len(subs))

	if rgClient == nil || len(subs) == 0 {
		utils.Log("executeGlobalSearch: No client or subscriptions available")
		return
	}

	// Set loading state
	gui.mu.Lock()
	gui.loadingRes = true
	gui.mu.Unlock()

	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.updateStatus()
		return nil
	})

	// Get subscription IDs
	subIDs := make([]string, len(subs))
	for i, sub := range subs {
		subIDs[i] = sub.ID
	}

	// Build subscription name map for formatting
	subNameMap := make(map[string]string)
	for _, sub := range subs {
		subNameMap[sub.ID] = sub.Name
	}

	utils.Log("executeGlobalSearch: Searching for '%s' across %d subscriptions", query, len(subIDs))

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), APITimeout)
		defer cancel()

		// Store cancel function for potential cancellation
		gui.mu.Lock()
		gui.globalSearchCancel = cancel
		gui.mu.Unlock()

		results, err := rgClient.SearchResources(ctx, query, typeFilter, subIDs)
		if err != nil {
			if ctx.Err() == context.Canceled {
				utils.Log("executeGlobalSearch: Search cancelled")
				return
			}
			utils.Log("executeGlobalSearch: Search error: %v", err)
			gui.mu.Lock()
			gui.loadingRes = false
			gui.mu.Unlock()
			gui.g.UpdateAsync(func(g *gocui.Gui) error {
				gui.updateStatus()
				return nil
			})
			return
		}

		utils.Log("executeGlobalSearch: Got %d results", len(results))

		// Sort results alphabetically
		sortResources(results)

		// Update resources with subscription context display
		gui.resList.SetItems(results, func(res *domain.Resource) string {
			return formatGlobalSearchResultWithMap(res, subNameMap)
		})

		gui.mu.Lock()
		gui.resources = results
		if len(results) > 0 {
			gui.selectedRes = results[0]
		} else {
			gui.selectedRes = nil
		}
		gui.loadingRes = false
		gui.mu.Unlock()

		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.refreshResourcesPanel()
			gui.updateStatus()
			return nil
		})

		utils.Log("executeGlobalSearch: Done, found %d results", len(results))
	}()
}

// onGlobalSearchCancel is called when the user presses Escape in global search
func (gui *Gui) onGlobalSearchCancel() {
	utils.Log("onGlobalSearchCancel: Cancelling global search")
	gui.exitGlobalSearchMode()
}

// exitGlobalSearchMode cleans up global search state and returns to normal mode
func (gui *Gui) exitGlobalSearchMode() {
	// Cancel any in-progress search
	gui.mu.Lock()
	if gui.globalSearchCancel != nil {
		gui.globalSearchCancel()
		gui.globalSearchCancel = nil
	}
	gui.globalSearchMode = false
	gui.globalSearchQuery = ""
	gui.mu.Unlock()

	// Hide search bar
	if gui.searchBar != nil {
		gui.searchBar.Hide()
	}
	gui.isSearching = false

	// Clear resources panel
	gui.resList.SetItems([]*domain.Resource{}, func(res *domain.Resource) string { return "" })
	gui.mu.Lock()
	gui.resources = nil
	gui.selectedRes = nil
	gui.mu.Unlock()

	// Return focus to subscriptions panel
	gui.g.SetCurrentView("subscriptions")
	gui.mu.Lock()
	gui.activePanel = "subscriptions"
	gui.mu.Unlock()

	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.refreshResourcesPanel()
		gui.updatePanelTitles()
		gui.updateStatus()
		return nil
	})
}

// formatGlobalSearchResult formats a resource for display in global search results
// Format: "name (rg-name / Type)" or "name (sub-name / rg-name / Type)" depending on context
// Note: This method acquires gui.mu - do NOT call from inside SetItems callback
func (gui *Gui) formatGlobalSearchResult(res *domain.Resource) string {
	// Build subscription name map
	gui.mu.RLock()
	subNameMap := make(map[string]string)
	for _, sub := range gui.subscriptions {
		subNameMap[sub.ID] = sub.Name
	}
	gui.mu.RUnlock()

	return formatGlobalSearchResultWithMap(res, subNameMap)
}

// formatGlobalSearchResultWithMap formats a resource using a pre-built subscription name map
// This is safe to call from inside SetItems callback since it doesn't acquire locks
func formatGlobalSearchResultWithMap(res *domain.Resource, subNameMap map[string]string) string {
	typeName := resources.GetResourceTypeDisplayName(res.Type)

	subName := subNameMap[res.SubscriptionID]

	// Format with subscription and RG context
	if subName != "" {
		// Truncate long names
		if len(subName) > 15 {
			subName = subName[:12] + "..."
		}
		rgName := res.ResourceGroup
		if len(rgName) > 15 {
			rgName = rgName[:12] + "..."
		}
		return fmt.Sprintf("%s %s(%s / %s / %s)%s", res.Name, grayColor, subName, rgName, typeName, resetColor)
	}

	// Fallback: just RG and type
	return fmt.Sprintf("%s %s(%s / %s)%s", res.Name, grayColor, res.ResourceGroup, typeName, resetColor)
}

// ============================================================================
// Subscription-Level Resource View (All Resources in Subscription)
// ============================================================================

// viewAllSubscriptionResources loads all resources in the selected subscription using Resource Graph
func (gui *Gui) viewAllSubscriptionResources(g *gocui.Gui, v *gocui.View) error {
	if gui.subList.Len() == 0 {
		return nil
	}

	_, cy := v.Cursor()
	_, oy := v.Origin()

	sub, ok := gui.subList.Get(oy + cy)
	if !ok {
		return nil
	}

	utils.Log("viewAllSubscriptionResources: Loading all resources for subscription")

	// Check if Resource Graph client is available
	gui.mu.RLock()
	rgClient := gui.resourceGraphClient
	gui.mu.RUnlock()

	if rgClient == nil {
		utils.Log("viewAllSubscriptionResources: Resource Graph client not available")
		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			if gui.statusView != nil {
				gui.statusView.Clear()
				fmt.Fprint(gui.statusView, "Subscription view unavailable - Resource Graph client not initialized")
			}
			return nil
		})
		return nil
	}

	// Set subscription and enter subscription view mode
	gui.mu.Lock()
	gui.selectedSub = sub
	gui.selectedRG = nil // No RG selected in subscription view
	gui.subscriptionViewMode = true
	gui.activeTypeFilter = "" // Clear type filter
	gui.loadingRes = true
	gui.mu.Unlock()

	// Clear RGs and show loading state
	gui.rgList.SetItems([]*domain.ResourceGroup{}, func(rg *domain.ResourceGroup) string { return "" })
	gui.resList.SetItems([]*domain.Resource{}, func(res *domain.Resource) string { return "" })

	gui.g.Update(func(g *gocui.Gui) error {
		gui.refreshResourceGroupsPanel()
		gui.refreshResourcesPanel()
		gui.updatePanelTitles()
		gui.updateStatus()
		return nil
	})

	// Load resources in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), APITimeout)
		defer cancel()

		gui.mu.RLock()
		subID := gui.selectedSub.ID
		typeFilter := gui.activeTypeFilter
		gui.mu.RUnlock()

		// Use Resource Graph to get all resources in subscription
		results, err := rgClient.SearchResources(ctx, "", typeFilter, []string{subID})
		if err != nil {
			utils.Log("viewAllSubscriptionResources: Error loading resources: %v", err)
			gui.mu.Lock()
			gui.loadingRes = false
			gui.mu.Unlock()
			gui.g.UpdateAsync(func(g *gocui.Gui) error {
				gui.updateStatus()
				return nil
			})
			return
		}

		// Sort results
		sortResources(results)

		// Update resources with RG context display
		gui.resList.SetItems(results, func(res *domain.Resource) string {
			return gui.formatSubscriptionViewResult(res)
		})

		gui.mu.Lock()
		gui.resources = results
		if len(results) > 0 {
			gui.selectedRes = results[0]
		} else {
			gui.selectedRes = nil
		}
		gui.loadingRes = false
		gui.mu.Unlock()

		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.refreshResourcesPanel()
			gui.updatePanelTitles()
			gui.updateStatus()
			return nil
		})

		// Switch to resources panel
		gui.g.SetCurrentView("resources")
		gui.mu.Lock()
		gui.activePanel = "resources"
		gui.mu.Unlock()

		gui.g.UpdateAsync(func(g *gocui.Gui) error {
			gui.updatePanelTitles()
			gui.updateStatus()
			return nil
		})

		utils.Log("viewAllSubscriptionResources: Loaded %d resources", len(results))
	}()

	return nil
}

// formatSubscriptionViewResult formats a resource for subscription-level view
// Format: "name (rg-name / Type)"
func (gui *Gui) formatSubscriptionViewResult(res *domain.Resource) string {
	typeName := resources.GetResourceTypeDisplayName(res.Type)
	rgName := res.ResourceGroup
	if len(rgName) > 20 {
		rgName = rgName[:17] + "..."
	}
	return fmt.Sprintf("%s %s(%s / %s)%s", res.Name, grayColor, rgName, typeName, resetColor)
}

// exitSubscriptionViewMode exits subscription-level view and returns to normal navigation
func (gui *Gui) exitSubscriptionViewMode() {
	gui.mu.Lock()
	gui.subscriptionViewMode = false
	gui.mu.Unlock()

	// Clear resources
	gui.resList.SetItems([]*domain.Resource{}, func(res *domain.Resource) string { return "" })
	gui.mu.Lock()
	gui.resources = nil
	gui.selectedRes = nil
	gui.mu.Unlock()

	// Return focus to subscriptions panel
	gui.g.SetCurrentView("subscriptions")
	gui.mu.Lock()
	gui.activePanel = "subscriptions"
	gui.mu.Unlock()

	gui.g.UpdateAsync(func(g *gocui.Gui) error {
		gui.refreshResourcesPanel()
		gui.updatePanelTitles()
		gui.updateStatus()
		return nil
	})
}
