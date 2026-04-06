// Package tui provides the Bubbletea-based terminal user interface
package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/gui"
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

	// UI State
	activePanel    ActivePanel
	showingVersion bool
	searchMode     bool

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

	// Calculate initial layout
	m.calculateLayout()

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
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			m.nextPanel()
		case "shift+tab":
			m.prevPanel()
		case "up", "down", "j", "k", "pgup", "pgdown":
			return m.handleNavigation(msg)
		case "enter":
			return m.handleEnter()
		case "[":
			if m.activePanel == MainPanel {
				m.mainPanel.PrevTab()
			}
		case "]":
			if m.activePanel == MainPanel {
				m.mainPanel.NextTab()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.calculateLayout()
	}

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
	// Build sidebar
	sidebar := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderAuthPanel(),
		m.subListPanel.View(),
		m.rgListPanel.View(),
		m.resListPanel.View(),
	)

	// Main panel
	mainPanel := m.mainPanel.View()

	// Combine sidebar and main panel horizontally
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebar,
		mainPanel,
	)

	// Status bar
	status := m.statusBar.View()

	// Final layout
	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		status,
	)
}

// renderAuthPanel renders the authentication status panel
func (m *Model) renderAuthPanel() string {
	styles := components.NewStyles()

	// Simple auth panel - will be enhanced in Phase 4
	var content string
	if m.azureClient != nil {
		content = "Auth\nAuthenticated"
	} else if m.isDemo {
		content = "Auth\nDemo Mode"
	} else {
		content = "Auth\nAuthenticating..."
	}

	return styles.AuthPanel.
		Width(m.sidebarWidth - 2).
		Height(3).
		Render(content)
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
