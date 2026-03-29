package panels

import (
	"strings"
	"sync"

	"github.com/jesseduffield/gocui"
	"github.com/matsest/lazyazure/pkg/utils"
)

// SearchBar manages the search input UI at the bottom of the screen
type SearchBar struct {
	g          *gocui.Gui
	view       *gocui.View
	isActive   bool
	searchText string
	onSearch   func(text string) // Callback when search changes
	onCancel   func()            // Callback when search is cancelled
	onConfirm  func()            // Callback when search is confirmed
	mu         sync.RWMutex
}

// NewSearchBar creates a new search bar manager
func NewSearchBar(g *gocui.Gui, onSearch, onCancel, onConfirm func()) *SearchBar {
	return &SearchBar{
		g:          g,
		isActive:   false,
		searchText: "",
		onSearch: func(text string) {
			if onSearch != nil {
				onSearch()
			}
		},
		onCancel:  onCancel,
		onConfirm: onConfirm,
	}
}

// Show activates the search bar
func (sb *SearchBar) Show() error {
	sb.mu.Lock()

	if sb.isActive {
		sb.mu.Unlock()
		return nil
	}

	maxX, maxY := sb.g.Size()

	// Create search view at bottom of screen
	if v, err := sb.g.SetView("search", 0, maxY-2, maxX-1, maxY, 0); err != nil {
		if !gocui.IsUnknownView(err) {
			sb.mu.Unlock()
			return err
		}
		v.Frame = false
		v.BgColor = gocui.ColorDefault
		v.FgColor = gocui.ColorWhite
		v.Editable = false // Not editable, we handle input via keybindings
		v.Wrap = false
		sb.view = v
		utils.Log("SearchBar: View created successfully")
	}

	sb.isActive = true
	sb.searchText = ""
	sb.updateView()
	sb.mu.Unlock()

	// Set focus to search view (outside of lock)
	utils.Log("SearchBar: Setting current view to search...")
	if _, err := sb.g.SetCurrentView("search"); err != nil {
		utils.Log("SearchBar: ERROR setting current view: %v", err)
		return err
	}

	utils.Log("SearchBar: Search mode activated, view focused")
	return nil
}

// Hide deactivates the search bar
func (sb *SearchBar) Hide() error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if !sb.isActive {
		return nil
	}

	// Delete the search view
	if err := sb.g.DeleteView("search"); err != nil {
		if !gocui.IsUnknownView(err) {
			return err
		}
	}

	sb.isActive = false
	sb.view = nil
	utils.Log("SearchBar: Search mode deactivated")
	return nil
}

// IsActive returns true if search bar is currently shown
func (sb *SearchBar) IsActive() bool {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.isActive
}

// GetText returns the current search text
func (sb *SearchBar) GetText() string {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.searchText
}

// SetText sets the search text programmatically
func (sb *SearchBar) SetText(text string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	sb.searchText = text
	sb.updateView()
	sb.onSearch(text)
}

// HandleRune processes a character input
func (sb *SearchBar) HandleRune(ch rune) {
	sb.mu.Lock()
	// Only accept printable ASCII characters
	if ch < 32 || ch >= 127 {
		sb.mu.Unlock()
		return
	}
	sb.searchText += string(ch)
	sb.updateView()
	searchText := sb.searchText
	sb.mu.Unlock()

	// Call callback outside of lock to avoid deadlock
	sb.onSearch(searchText)
	utils.Log("SearchBar: Char added, length=%d", len(searchText))
}

// Backspace removes the last character
func (sb *SearchBar) Backspace() {
	sb.mu.Lock()
	if len(sb.searchText) == 0 {
		sb.mu.Unlock()
		return
	}
	sb.searchText = sb.searchText[:len(sb.searchText)-1]
	sb.updateView()
	searchText := sb.searchText
	sb.mu.Unlock()

	// Call callback outside of lock to avoid deadlock
	sb.onSearch(searchText)
	utils.Log("SearchBar: Backspace, length=%d", len(searchText))
}

// Clear removes all text
func (sb *SearchBar) Clear() {
	sb.mu.Lock()
	sb.searchText = ""
	sb.updateView()
	sb.mu.Unlock()

	// Call callback outside of lock to avoid deadlock
	sb.onSearch("")
	utils.Log("SearchBar: Cleared")
}

// DeleteWord removes the last word
func (sb *SearchBar) DeleteWord() {
	sb.mu.Lock()
	sb.searchText = strings.TrimRightFunc(sb.searchText, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '_' || r == '-'
	})

	// Find last space or start of string
	lastSpace := strings.LastIndexAny(sb.searchText, " \t_-")
	if lastSpace >= 0 {
		sb.searchText = sb.searchText[:lastSpace+1]
	} else {
		sb.searchText = ""
	}

	sb.updateView()
	searchText := sb.searchText
	sb.mu.Unlock()

	// Call callback outside of lock to avoid deadlock
	sb.onSearch(searchText)
	utils.Log("SearchBar: Word deleted, length=%d", len(searchText))
}

// Cancel cancels the search and restores previous state
func (sb *SearchBar) Cancel() {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	sb.searchText = ""
	sb.updateView()

	if sb.onCancel != nil {
		sb.onCancel()
	}
	utils.Log("SearchBar: Cancelled")
}

// Confirm confirms the current search
func (sb *SearchBar) Confirm() {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.onConfirm != nil {
		sb.onConfirm()
	}
	utils.Log("SearchBar: Confirmed")
}

// updateView refreshes the display
// Must be called with lock held
func (sb *SearchBar) updateView() {
	if sb.view == nil {
		return
	}

	sb.view.Clear()

	// Show the search prompt with current text
	prompt := "/" + sb.searchText + "_"
	sb.view.WriteString(prompt)
}

// Resize updates the search bar position on terminal resize
func (sb *SearchBar) Resize() error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if !sb.isActive || sb.view == nil {
		return nil
	}

	maxX, maxY := sb.g.Size()
	_, err := sb.g.SetView("search", 0, maxY-2, maxX-1, maxY, 0)
	return err
}
