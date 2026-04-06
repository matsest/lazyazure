// Package tui provides the Bubbletea-based terminal user interface
package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/matsest/lazyazure/pkg/gui"
)

// Model is the main Bubbletea model for the LazyAzure TUI
type Model struct {
	// Azure clients
	azureClient   gui.AzureClient
	clientFactory gui.AzureClientFactory
	versionInfo   gui.VersionInfo

	// Terminal dimensions
	width  int
	height int

	// UI state
	quit bool
}

// NewModel creates a new TUI model with the given dependencies
func NewModel(azureClient gui.AzureClient, clientFactory gui.AzureClientFactory, versionInfo gui.VersionInfo) *Model {
	return &Model{
		azureClient:   azureClient,
		clientFactory: clientFactory,
		versionInfo:   versionInfo,
		width:         80,
		height:        24,
	}
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quit = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// View implements tea.Model
func (m *Model) View() tea.View {
	if m.quit {
		return tea.View{}
	}

	return tea.NewView("LazyAzure - Bubbletea Edition\n\nPress q to quit")
}
