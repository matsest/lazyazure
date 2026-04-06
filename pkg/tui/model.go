// Package tui provides the Bubbletea-based terminal user interface
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/gui"
	"github.com/matsest/lazyazure/pkg/gui/panels"
	"github.com/matsest/lazyazure/pkg/tui/components"
)

// ActivePanel represents which panel currently has focus
type ActivePanel int

const (
	SubscriptionsPanel ActivePanel = iota
	ResourceGroupsPanel
	ResourcesPanel
	MainPanel
)

// Panel names for display
var panelNames = []string{
	"subscriptions",
	"resourcegroups",
	"resources",
	"main",
}

// Model is the main Bubbletea model for the LazyAzure TUI
type Model struct {
	// Azure clients
	azureClient   gui.AzureClient
	clientFactory gui.AzureClientFactory
	versionInfo   gui.VersionInfo

	// Terminal dimensions
	width  int
	height int

	// Panel dimensions (calculated on resize)
	sidebarWidth int
	mainWidth    int

	// UI Components
	subListPanel *components.ListPanel[*domain.Subscription]
	rgListPanel  *components.ListPanel[*domain.ResourceGroup]
	resListPanel *components.ListPanel[*domain.Resource]
	mainPanel    *components.MainPanel
	statusBar    *components.StatusBar
	searchInput  textinput.Model

	// UI State
	activePanel     ActivePanel
	showingVersion  bool
	versionTimer    *time.Timer
	searchMode      bool
	searchingMain   bool
	mainPanelSearch *panels.MainPanelSearch

	// Demo mode
	isDemo bool
}

// KeyMap defines keybindings for the application
type KeyMap struct {
	Quit       key.Binding
	Tab        key.Binding
	ShiftTab   key.Binding
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Refresh    key.Binding
	Search     key.Binding
	CopyURL    key.Binding
	OpenPortal key.Binding
	Help       key.Binding
	NextTab    key.Binding
	PrevTab    key.Binding
}

// DefaultKeyMap returns the default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q/ctrl+c", "quit"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next panel"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev panel"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		CopyURL: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "copy URL"),
		),
		OpenPortal: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open portal"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "prev tab"),
		),
	}
}

// NewModel creates a new TUI model with the given dependencies
func NewModel(azureClient gui.AzureClient, clientFactory gui.AzureClientFactory, versionInfo gui.VersionInfo, isDemo bool) *Model {
	m := &Model{
		azureClient:    azureClient,
		clientFactory:  clientFactory,
		versionInfo:    versionInfo,
		width:          120,
		height:         40,
		activePanel:    SubscriptionsPanel,
		showingVersion: false,
		searchMode:     false,
		isDemo:         isDemo,
	}

	// Initialize components
	m.subListPanel = components.NewListPanel[*domain.Subscription](
		"Subscriptions",
		func(s *domain.Subscription) string { return s.DisplayString() },
		func(s *domain.Subscription) string { return s.GetDisplaySuffix() },
	)
	m.rgListPanel = components.NewListPanel[*domain.ResourceGroup](
		"Resource Groups",
		func(rg *domain.ResourceGroup) string { return rg.DisplayString() },
		func(rg *domain.ResourceGroup) string { return rg.GetDisplaySuffix() },
	)
	m.resListPanel = components.NewListPanel[*domain.Resource](
		"Resources",
		func(r *domain.Resource) string { return r.DisplayString() },
		func(r *domain.Resource) string { return r.GetDisplaySuffix() },
	)
	m.mainPanel = components.NewMainPanel()
	m.statusBar = components.NewStatusBar()

	// Initialize search input
	m.searchInput = textinput.New()
	m.searchInput.Placeholder = "Search..."
	m.searchInput.CharLimit = 100

	// Initialize main panel search
	m.mainPanelSearch = panels.NewMainPanelSearch()

	// Calculate initial layout
	m.calculateLayout()

	// Set initial panel states (subscriptions panel active)
	m.updatePanelStates()

	// Set initial status bar text
	m.updateStatusBar()

	return m
}

// calculateLayout computes panel dimensions based on terminal size
func (m *Model) calculateLayout() {
	// Sidebar is ~33% width, min 30 chars
	m.sidebarWidth = m.width / 3
	if m.sidebarWidth < 30 {
		m.sidebarWidth = 30
	}
	// Main panel takes remaining width
	m.mainWidth = m.width - m.sidebarWidth

	// Calculate available height for panels (excluding status bar and border overhead)
	// The -9 accounts for: 1 status bar + 8 border lines (2 per panel × 4 panels in sidebar)
	availableHeight := m.height - 9
	if availableHeight < 15 {
		availableHeight = 15
	}

	// Auth panel: fixed height
	authHeight := 3
	listHeight := availableHeight - authHeight

	// Divide list area proportionally: subs 20%, RGs 30%, resources 50%
	subHeight := listHeight / 5
	rgHeight := (listHeight * 3) / 10
	resHeight := listHeight - subHeight - rgHeight

	// Ensure minimum panel heights
	if subHeight < 3 {
		subHeight = 3
	}
	if rgHeight < 3 {
		rgHeight = 3
	}
	if resHeight < 3 {
		resHeight = 3
	}

	// Update all panel sizes
	m.subListPanel.SetSize(m.sidebarWidth-2, subHeight)
	m.rgListPanel.SetSize(m.sidebarWidth-2, rgHeight)
	m.resListPanel.SetSize(m.sidebarWidth-2, resHeight)

	// Main panel height should match sidebar exactly
	// sidebar = auth + sub + rg + res (each panel renders with its own borders)
	// Add +6 for lipgloss border alignment when rendering separately
	sidebarHeight := authHeight + subHeight + rgHeight + resHeight
	m.mainPanel.SetSize(m.mainWidth-2, sidebarHeight+6)
	m.statusBar.SetSize(m.width)
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	// TODO: Load initial data (subscriptions)
	return nil
}

// Update implements tea.Model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle global keys first
		if handled, cmd := m.handleGlobalKeys(msg); handled {
			return m, cmd
		}

		// Handle search mode
		if m.searchMode {
			return m.handleSearchMode(msg)
		}

		// Handle main panel search mode
		if m.searchingMain {
			return m.handleMainPanelSearchMode(msg)
		}

		// Handle panel-specific keys
		return m.handlePanelKeys(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.calculateLayout()

	case ClearVersionMsg:
		m.showingVersion = false
		m.updateStatusBar()

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	// Update search input if in search mode
	if m.searchMode {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// ClearVersionMsg is sent when version display should be cleared
type ClearVersionMsg struct{}

// handleGlobalKeys handles keys that work from any panel
func (m *Model) handleGlobalKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return true, tea.Quit

	case "tab":
		m.nextPanel()
		return true, nil

	case "shift+tab":
		m.prevPanel()
		return true, nil

	case "?":
		cmd := m.showVersion()
		return true, cmd

	case "esc":
		if m.showingVersion {
			m.clearVersionDisplay()
			return true, nil
		}

	case "[":
		m.mainPanel.PrevTab()
		m.clearMainPanelSearch()
		return true, nil

	case "]":
		m.mainPanel.NextTab()
		m.clearMainPanelSearch()
		return true, nil
	}

	return false, nil
}

// handlePanelKeys handles keys that depend on the active panel
func (m *Model) handlePanelKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.activePanel {
	case SubscriptionsPanel:
		return m.handleSubscriptionKeys(msg)
	case ResourceGroupsPanel:
		return m.handleResourceGroupKeys(msg)
	case ResourcesPanel:
		return m.handleResourceKeys(msg)
	case MainPanel:
		return m.handleMainPanelKeys(msg)
	}
	return m, nil
}

// handleSubscriptionKeys handles keys for the subscriptions panel
func (m *Model) handleSubscriptionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.subListPanel.Prev()
	case "down", "j":
		m.subListPanel.Next()
	case "pgup":
		m.subListPanel.PageUp()
	case "pgdown":
		m.subListPanel.PageDown()
	case "enter":
		return m.handleSubEnter()
	case "/":
		m.startSearchMode()
	case "r":
		// TODO: Refresh subscriptions
		m.updateStatusBarWithText("Refreshing subscriptions...")
	case "esc":
		if m.subListPanel.IsFiltering() {
			m.subListPanel.ClearFilter()
		}
	}
	m.updateStatusBar()
	return m, nil
}

// handleResourceGroupKeys handles keys for the resource groups panel
func (m *Model) handleResourceGroupKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.rgListPanel.Prev()
	case "down", "j":
		m.rgListPanel.Next()
	case "pgup":
		m.rgListPanel.PageUp()
	case "pgdown":
		m.rgListPanel.PageDown()
	case "enter":
		return m.handleRGEnter()
	case "/":
		m.startSearchMode()
	case "r":
		// TODO: Refresh resource groups
		m.updateStatusBarWithText("Refreshing resource groups...")
	case "esc":
		if m.rgListPanel.IsFiltering() {
			m.rgListPanel.ClearFilter()
		}
	}
	m.updateStatusBar()
	return m, nil
}

// handleResourceKeys handles keys for the resources panel
func (m *Model) handleResourceKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.resListPanel.Prev()
	case "down", "j":
		m.resListPanel.Next()
	case "pgup":
		m.resListPanel.PageUp()
	case "pgdown":
		m.resListPanel.PageDown()
	case "enter":
		return m.handleResourceEnter()
	case "/":
		m.startSearchMode()
	case "r":
		// TODO: Refresh resources
		m.updateStatusBarWithText("Refreshing resources...")
	case "esc":
		if m.resListPanel.IsFiltering() {
			m.resListPanel.ClearFilter()
		}
	}
	m.updateStatusBar()
	return m, nil
}

// handleMainPanelKeys handles keys for the main panel
func (m *Model) handleMainPanelKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.mainPanel.ScrollUp()
	case "down", "j":
		m.mainPanel.ScrollDown()
	case "pgup":
		m.mainPanel.ScrollPageUp()
	case "pgdown":
		m.mainPanel.ScrollPageDown()
	case "home", "g":
		m.mainPanel.ScrollTop()
	case "end", "G":
		m.mainPanel.ScrollBottom()
	case "/":
		if !m.searchingMain {
			m.startMainPanelSearch()
		}
	case "c":
		// TODO: Copy URL
		m.updateStatusBarWithText("Copy URL: TODO")
	case "o":
		// TODO: Open portal
		m.updateStatusBarWithText("Open portal: TODO")
	}
	m.updateStatusBar()
	return m, nil
}

// clearMainPanelSearch clears the main panel search
func (m *Model) clearMainPanelSearch() {
	if m.mainPanelSearch.IsActive() {
		m.mainPanelSearch.ClearSearch()
		m.updateMainPanelContent()
	}
}

// handleSubEnter handles Enter in subscriptions panel
func (m *Model) handleSubEnter() (tea.Model, tea.Cmd) {
	if sub, ok := m.subListPanel.Selected(); ok {
		// Move to RG panel
		m.activePanel = ResourceGroupsPanel
		// TODO: Load RGs for this subscription
		_ = sub
		m.updatePanelStates()
	}
	m.updateStatusBar()
	return m, nil
}

// handleRGEnter handles Enter in resource groups panel
func (m *Model) handleRGEnter() (tea.Model, tea.Cmd) {
	if rg, ok := m.rgListPanel.Selected(); ok {
		// Move to Resources panel
		m.activePanel = ResourcesPanel
		// TODO: Load resources for this RG
		_ = rg
		m.updatePanelStates()
	}
	m.updateStatusBar()
	return m, nil
}

// handleResourceEnter handles Enter in resources panel
func (m *Model) handleResourceEnter() (tea.Model, tea.Cmd) {
	if res, ok := m.resListPanel.Selected(); ok {
		// Move to main panel and show details
		m.activePanel = MainPanel
		// TODO: Load resource details
		_ = res
		m.updatePanelStates()
	}
	m.updateStatusBar()
	return m, nil
}

// showVersion displays version info in the status bar for 5 seconds
func (m *Model) showVersion() tea.Cmd {
	m.showingVersion = true
	if m.versionTimer != nil {
		m.versionTimer.Stop()
	}
	m.updateStatusBar()
	// Return a command that will clear the version after 5 seconds
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return ClearVersionMsg{}
	})
}

// updateStatusBar updates the status bar with context-aware help
func (m *Model) updateStatusBar() {
	var text string
	if m.showingVersion {
		text = m.formatVersionText()
	} else if m.searchMode {
		text = m.formatSearchInputStatus()
	} else if m.searchingMain {
		text = m.formatMainPanelSearchStatus()
	} else {
		text = m.formatContextHelp()
	}
	m.statusBar.SetText(text)
}

// formatSearchInputStatus returns status text for list panel search input mode
func (m *Model) formatSearchInputStatus() string {
	return fmt.Sprintf("/%s_ | Esc: Cancel | Enter: Confirm", m.searchInput.Value())
}

// formatMainPanelSearchStatus returns status text for main panel search
func (m *Model) formatMainPanelSearchStatus() string {
	searchText := m.searchInput.Value()
	if m.mainPanelSearch.IsActive() && m.mainPanelSearch.GetMatchCount() > 0 {
		current, total := m.mainPanelSearch.GetCurrentMatch()
		return fmt.Sprintf("/%s_ | Match %d/%d | n: Next | N: Prev | Esc: Cancel", searchText, current, total)
	}
	return fmt.Sprintf("/%s_ | Esc: Cancel | Enter: Confirm", searchText)
}

// updateStatusBarWithText sets a specific status bar message
func (m *Model) updateStatusBarWithText(text string) {
	m.statusBar.SetText(text)
}

// formatVersionText returns the version display text
func (m *Model) formatVersionText() string {
	version := m.versionInfo.Version
	commit := m.versionInfo.Commit

	// Shorten commit for display
	displayCommit := commit
	if len(displayCommit) > 7 {
		displayCommit = displayCommit[:7]
	}

	return "lazyazure " + version + " (" + displayCommit + ") | Esc: Dismiss"
}

// clearVersionDisplay clears the version display and reverts status bar
func (m *Model) clearVersionDisplay() {
	if m.versionTimer != nil {
		m.versionTimer.Stop()
		m.versionTimer = nil
	}
	m.showingVersion = false
	m.updateStatusBar()
}

// formatContextHelp returns context-aware help text (matching gocui implementation)
func (m *Model) formatContextHelp() string {
	switch m.activePanel {
	case SubscriptionsPanel:
		subShowing, subTotal := m.subListPanel.GetFilterStats()
		if m.subListPanel.IsFiltering() {
			return fmt.Sprintf("Filter: \"%s\" (%d/%d) | Esc: Clear | Enter: Load RGs | c: Copy | o: Open | Tab: Switch | r: Refresh | q: Quit",
				m.subListPanel.GetFilterText(), subShowing, subTotal)
		}
		return fmt.Sprintf("↑↓: Navigate | /: Search | Enter: Load RGs | c: Copy | o: Open | Tab: Switch | r: Refresh | q: Quit | Subs: %d", subTotal)

	case ResourceGroupsPanel:
		rgShowing, rgTotal := m.rgListPanel.GetFilterStats()
		if m.rgListPanel.IsFiltering() {
			return fmt.Sprintf("Filter: \"%s\" (%d/%d) | Esc: Clear | Enter: Load Resources | c: Copy | o: Open | Tab: Switch | r: Refresh | q: Quit",
				m.rgListPanel.GetFilterText(), rgShowing, rgTotal)
		}
		return fmt.Sprintf("↑↓: Navigate | /: Search | Enter: Load Resources | c: Copy | o: Open | Tab: Switch | []: Tabs | r: Refresh | q: Quit | RGs: %d", rgTotal)

	case ResourcesPanel:
		resShowing, resTotal := m.resListPanel.GetFilterStats()
		if m.resListPanel.IsFiltering() {
			return fmt.Sprintf("Filter: \"%s\" (%d/%d) | Esc: Clear | Enter: View Details | c: Copy | o: Open | Tab: Switch | r: Refresh | q: Quit",
				m.resListPanel.GetFilterText(), resShowing, resTotal)
		}
		return fmt.Sprintf("↑↓: Navigate | /: Search | Enter: View Details | c: Copy | o: Open | Tab: Switch | []: Tabs | r: Refresh | q: Quit | Resources: %d", resTotal)

	case MainPanel:
		return "↑/↓ or j/k: Scroll | PgUp/PgDn: Page | /: Search | c: Copy | o: Open | Tab: Back to List | []: Tabs | r: Refresh | q: Quit"

	default:
		return "↑↓: Navigate | Tab: Switch | r: Refresh | q: Quit"
	}
}

// startSearchMode activates search mode for the current list panel
func (m *Model) startSearchMode() {
	m.searchMode = true
	m.searchInput.Focus()
	m.updateStatusBar()
}

// handleSearchMode handles key input when in search mode
func (m *Model) handleSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		// Clear filter on current panel
		switch m.activePanel {
		case SubscriptionsPanel:
			m.subListPanel.ClearFilter()
		case ResourceGroupsPanel:
			m.rgListPanel.ClearFilter()
		case ResourcesPanel:
			m.resListPanel.ClearFilter()
		}
		m.updateStatusBar()
		return m, nil

	case "enter":
		m.searchMode = false
		m.searchInput.Blur()
		m.updateStatusBar()
		return m, nil

	case "ctrl+u":
		m.searchInput.SetValue("")
		m.updateSearchFilter("")
		m.updateStatusBar()
		return m, nil

	case "ctrl+w":
		// Delete last word
		val := m.searchInput.Value()
		if len(val) > 0 {
			// Find last space
			lastSpace := -1
			for i := len(val) - 1; i >= 0; i-- {
				if val[i] == ' ' {
					lastSpace = i
					break
				}
			}
			if lastSpace >= 0 {
				m.searchInput.SetValue(val[:lastSpace])
			} else {
				m.searchInput.SetValue("")
			}
			m.updateSearchFilter(m.searchInput.Value())
			m.updateStatusBar()
		}
		return m, nil

	default:
		// Let textinput handle the key
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		// Update filter on the fly
		m.updateSearchFilter(m.searchInput.Value())
		m.updateStatusBar()
		return m, cmd
	}
}

// updateSearchFilter updates the filter on the current panel
func (m *Model) updateSearchFilter(text string) {
	switch m.activePanel {
	case SubscriptionsPanel:
		m.subListPanel.SetFilter(text)
	case ResourceGroupsPanel:
		m.rgListPanel.SetFilter(text)
	case ResourcesPanel:
		m.resListPanel.SetFilter(text)
	}
}

// startMainPanelSearch starts search mode for the main panel
func (m *Model) startMainPanelSearch() {
	m.searchingMain = true
	m.searchInput.SetValue("")
	m.searchInput.Focus()

	// Get current content from main panel and set it in search
	lines := m.mainPanel.GetContentLines()
	m.mainPanelSearch.SetContent(lines)
	m.mainPanelSearch.SetSearch("")

	m.updateStatusBar()
}

// handleMainPanelSearchMode handles keys when searching in main panel
func (m *Model) handleMainPanelSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchingMain = false
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		m.mainPanelSearch.ClearSearch()
		// Restore original content
		m.updateMainPanelContent()
		m.updateStatusBar()
		return m, nil

	case "enter":
		m.searchingMain = false
		m.searchInput.Blur()
		m.updateStatusBar()
		return m, nil

	case "n":
		// Next match
		if m.mainPanelSearch.IsActive() && m.mainPanelSearch.GetMatchCount() > 0 {
			lineNum := m.mainPanelSearch.NextMatch()
			m.mainPanel.GotoLine(lineNum)
			m.updateMainPanelHighlightedContent()
		}
		m.updateStatusBar()
		return m, nil

	case "N":
		// Previous match
		if m.mainPanelSearch.IsActive() && m.mainPanelSearch.GetMatchCount() > 0 {
			lineNum := m.mainPanelSearch.PrevMatch()
			m.mainPanel.GotoLine(lineNum)
			m.updateMainPanelHighlightedContent()
		}
		m.updateStatusBar()
		return m, nil

	case "ctrl+u":
		m.searchInput.SetValue("")
		m.mainPanelSearch.SetSearch("")
		m.updateMainPanelContent()
		m.updateStatusBar()
		return m, nil

	default:
		// Let textinput handle the key
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		// Update search and refresh highlights
		m.mainPanelSearch.SetSearch(m.searchInput.Value())
		m.updateMainPanelHighlightedContent()
		m.updateStatusBar()
		return m, cmd
	}
}

// updateMainPanelContent restores the original content (without highlights)
func (m *Model) updateMainPanelContent() {
	lines := m.mainPanel.GetContentLines()
	m.mainPanel.SetHighlightedContent(lines)
}

// updateMainPanelHighlightedContent updates the main panel with highlighted search results
func (m *Model) updateMainPanelHighlightedContent() {
	if m.mainPanelSearch.IsActive() && m.mainPanelSearch.GetMatchCount() > 0 {
		highlightedLines := m.mainPanelSearch.GetHighlightedContent()
		m.mainPanel.SetHighlightedContent(highlightedLines)
	} else {
		m.updateMainPanelContent()
	}
}

// handleMouse handles mouse events
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Get the zone manager
	z := zone.New()

	// Define zone IDs for panels
	subZoneID := "subscriptions"
	rgZoneID := "resourcegroups"
	resZoneID := "resources"
	mainZoneID := "main"

	switch msg.Type {
	case tea.MouseLeft:
		// Check which zone was clicked
		if z.Get(subZoneID).InBounds(msg) {
			m.activePanel = SubscriptionsPanel
			// TODO: Calculate which item was clicked and select it
		} else if z.Get(rgZoneID).InBounds(msg) {
			m.activePanel = ResourceGroupsPanel
		} else if z.Get(resZoneID).InBounds(msg) {
			m.activePanel = ResourcesPanel
		} else if z.Get(mainZoneID).InBounds(msg) {
			m.activePanel = MainPanel
		}
		m.updatePanelStates()

	case tea.MouseRight:
		// Copy URL - TODO: implement
		m.updateStatusBarWithText("Right-click: Copy URL (TODO)")

	case tea.MouseMiddle:
		// Open portal - TODO: implement
		m.updateStatusBarWithText("Middle-click: Open portal (TODO)")

	case tea.MouseWheelUp:
		// Scroll up based on active panel
		switch m.activePanel {
		case SubscriptionsPanel:
			m.subListPanel.Prev()
		case ResourceGroupsPanel:
			m.rgListPanel.Prev()
		case ResourcesPanel:
			m.resListPanel.Prev()
		case MainPanel:
			m.mainPanel.ScrollUp()
		}

	case tea.MouseWheelDown:
		// Scroll down based on active panel
		switch m.activePanel {
		case SubscriptionsPanel:
			m.subListPanel.Next()
		case ResourceGroupsPanel:
			m.rgListPanel.Next()
		case ResourcesPanel:
			m.resListPanel.Next()
		case MainPanel:
			m.mainPanel.ScrollDown()
		}
	}

	m.updateStatusBar()
	return m, nil
}

// handleNavigation handles navigation keys for the active panel
func (m *Model) handleNavigation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.activePanel {
	case SubscriptionsPanel:
		m.subListPanel.Update(msg)
	case ResourceGroupsPanel:
		m.rgListPanel.Update(msg)
	case ResourcesPanel:
		m.resListPanel.Update(msg)
	case MainPanel:
		m.mainPanel.Update(msg)
	}
	return m, nil
}

// handleEnter handles the Enter key for the active panel
func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.activePanel {
	case SubscriptionsPanel:
		// Load resource groups for selected subscription
		// TODO: Implement
	case ResourceGroupsPanel:
		// Load resources for selected resource group
		// TODO: Implement
	case ResourcesPanel:
		// Show resource details in main panel
		// TODO: Implement
		m.activePanel = MainPanel
	}
	m.updatePanelStates()
	return m, nil
}

// nextPanel moves focus to the next panel
func (m *Model) nextPanel() {
	switch m.activePanel {
	case SubscriptionsPanel:
		m.activePanel = ResourceGroupsPanel
	case ResourceGroupsPanel:
		m.activePanel = ResourcesPanel
	case ResourcesPanel:
		m.activePanel = MainPanel
	case MainPanel:
		m.activePanel = SubscriptionsPanel
	}
	m.updatePanelStates()
}

// prevPanel moves focus to the previous panel
func (m *Model) prevPanel() {
	switch m.activePanel {
	case SubscriptionsPanel:
		m.activePanel = MainPanel
	case ResourceGroupsPanel:
		m.activePanel = SubscriptionsPanel
	case ResourcesPanel:
		m.activePanel = ResourceGroupsPanel
	case MainPanel:
		m.activePanel = ResourcesPanel
	}
	m.updatePanelStates()
}

// updatePanelStates updates the active state of all panels
func (m *Model) updatePanelStates() {
	m.subListPanel.SetActive(m.activePanel == SubscriptionsPanel)
	m.rgListPanel.SetActive(m.activePanel == ResourceGroupsPanel)
	m.resListPanel.SetActive(m.activePanel == ResourcesPanel)
	m.mainPanel.SetActive(m.activePanel == MainPanel)
}

// View implements tea.Model
func (m *Model) View() string {
	// Create zone manager
	z := zone.New()

	// Build sidebar with zone markers
	authPanel := m.renderAuthPanel()
	subPanel := z.Mark("subscriptions", m.subListPanel.View())
	rgPanel := z.Mark("resourcegroups", m.rgListPanel.View())
	resPanel := z.Mark("resources", m.resListPanel.View())

	sidebar := lipgloss.JoinVertical(
		lipgloss.Left,
		authPanel,
		subPanel,
		rgPanel,
		resPanel,
	)

	// Main panel with zone marker
	mainPanel := z.Mark("main", m.mainPanel.View())

	// Combine sidebar and main panel horizontally
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebar,
		mainPanel,
	)

	// Status bar (or search input when in search mode)
	var status string
	if m.searchMode || m.searchingMain {
		status = m.renderSearchBar()
	} else {
		status = m.statusBar.View()
	}

	// Final layout
	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		status,
	)
}

// renderSearchBar renders the search input at the bottom
func (m *Model) renderSearchBar() string {
	styles := components.NewStyles()

	// Update search input width
	m.searchInput.Width = m.width - 2

	// Create search bar with prompt
	prompt := "/"
	input := m.searchInput.View()

	// Combine and style
	searchBar := prompt + input
	return styles.StatusBar.Width(m.width).Render(searchBar)
}

// renderAuthPanel renders the authentication status panel
func (m *Model) renderAuthPanel() string {
	styles := components.NewStyles()

	// Simple auth panel - will be enhanced in Phase 4
	var content string
	if m.azureClient != nil {
		content = "Authenticated"
	} else if m.isDemo {
		content = "Demo Mode"
	} else {
		content = "Authenticating..."
	}

	// Render panel and embed title on border
	rendered := styles.AuthPanel.
		Width(m.sidebarWidth - 2).
		Height(3).
		Render(content)

	return components.EmbedBorderTitle(rendered, "Auth")
}

// SetSubscriptionData sets the subscription data
func (m *Model) SetSubscriptionData(subs []*domain.Subscription) {
	m.subListPanel.SetItems(subs)
}

// SetResourceGroupData sets the resource group data
func (m *Model) SetResourceGroupData(rgs []*domain.ResourceGroup) {
	m.rgListPanel.SetItems(rgs)
}

// SetResourceData sets the resource data
func (m *Model) SetResourceData(resources []*domain.Resource) {
	m.resListPanel.SetItems(resources)
}

// SetMainContent sets the main panel content
func (m *Model) SetMainContent(summary, json string) {
	m.mainPanel.SetContent(summary, json)
}
