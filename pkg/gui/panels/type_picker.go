package panels

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/jesseduffield/gocui"
	"github.com/matsest/lazyazure/pkg/resources"
	"github.com/matsest/lazyazure/pkg/utils"
)

const (
	typePickerViewName = "typepicker"
	typePickerWidth    = 70
	typePickerHeight   = 15
)

// TypePicker provides a modal dialog for selecting resource types with fuzzy search
type TypePicker struct {
	g             *gocui.Gui
	allTypes      []resources.ResourceTypeEntry
	filteredTypes []resources.ResourceTypeEntry
	filterText    string
	selectedIdx   int
	isVisible     bool
	onSelect      func(resourceType string) // Called with full type or "" to clear
	onCancel      func()
	mu            sync.Mutex
}

// NewTypePicker creates a new type picker component
func NewTypePicker(g *gocui.Gui, onSelect func(string), onCancel func()) *TypePicker {
	tp := &TypePicker{
		g:        g,
		allTypes: resources.GetAllResourceTypes(),
		onSelect: onSelect,
		onCancel: onCancel,
	}
	tp.filteredTypes = tp.allTypes
	return tp
}

// Show displays the type picker modal
func (tp *TypePicker) Show() error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if tp.isVisible {
		return nil
	}

	tp.isVisible = true
	tp.filterText = ""
	tp.filteredTypes = tp.allTypes
	// Default to first actual type (index 1), not "[Clear filter]" (index 0)
	if len(tp.filteredTypes) > 0 {
		tp.selectedIdx = 1
	} else {
		tp.selectedIdx = 0
	}

	utils.Log("TypePicker: showing picker with %d types", len(tp.allTypes))

	return tp.createView()
}

// Hide closes the type picker modal
func (tp *TypePicker) Hide() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if !tp.isVisible {
		return
	}

	tp.isVisible = false
	utils.Log("TypePicker: hiding picker")

	tp.g.DeleteView(typePickerViewName)
	tp.g.DeleteViewKeybindings(typePickerViewName)
}

// IsVisible returns whether the type picker is currently shown
func (tp *TypePicker) IsVisible() bool {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.isVisible
}

// HandleKey processes a key event, returns true if handled
func (tp *TypePicker) HandleKey(key gocui.Key, ch rune, mod gocui.Modifier) bool {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if !tp.isVisible {
		return false
	}

	switch {
	case key == gocui.KeyArrowUp:
		tp.moveSelection(-1)
		return true
	case key == gocui.KeyArrowDown:
		tp.moveSelection(1)
		return true
	case key == gocui.KeyEnter:
		tp.confirmSelection()
		return true
	case key == gocui.KeyEsc:
		tp.cancel()
		return true
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		if len(tp.filterText) > 0 {
			tp.filterText = tp.filterText[:len(tp.filterText)-1]
			tp.applyFilter()
		}
		return true
	case key == gocui.KeyCtrlU:
		tp.filterText = ""
		tp.applyFilter()
		return true
	case key == gocui.KeySpace:
		// Handle space key explicitly
		tp.filterText += " "
		tp.applyFilter()
		return true
	case ch != 0 && ch >= 32 && ch < 127:
		tp.filterText += string(ch)
		tp.applyFilter()
		return true
	}

	return false
}

// createView creates the type picker view
func (tp *TypePicker) createView() error {
	maxX, maxY := tp.g.Size()

	// Center the picker
	x0 := (maxX - typePickerWidth) / 2
	y0 := (maxY - typePickerHeight) / 2
	x1 := x0 + typePickerWidth
	y1 := y0 + typePickerHeight

	v, err := tp.g.SetView(typePickerViewName, x0, y0, x1, y1, 0)
	if err != nil && !errors.Is(err, gocui.ErrUnknownView) {
		return err
	}

	v.Title = " Filter by Resource Type "
	v.Wrap = false
	v.Frame = true

	// Set keybindings for the type picker view
	tp.g.SetKeybinding(typePickerViewName, gocui.KeyArrowUp, gocui.ModNone, tp.keyUp)
	tp.g.SetKeybinding(typePickerViewName, gocui.KeyArrowDown, gocui.ModNone, tp.keyDown)
	tp.g.SetKeybinding(typePickerViewName, gocui.KeyEnter, gocui.ModNone, tp.keyEnter)
	tp.g.SetKeybinding(typePickerViewName, gocui.KeyEsc, gocui.ModNone, tp.keyEsc)

	// Set as current view
	tp.g.SetCurrentView(typePickerViewName)

	// Set editor for text input
	v.Editor = gocui.EditorFunc(tp.editorFunc)
	v.Editable = true

	tp.renderContent()
	return nil
}

// editorFunc handles key input for the type picker
func (tp *TypePicker) editorFunc(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	return tp.HandleKey(key, ch, mod)
}

// Keybinding handlers (wrap HandleKey to match gocui signature)
func (tp *TypePicker) keyUp(g *gocui.Gui, v *gocui.View) error {
	tp.HandleKey(gocui.KeyArrowUp, 0, gocui.ModNone)
	return nil
}

func (tp *TypePicker) keyDown(g *gocui.Gui, v *gocui.View) error {
	tp.HandleKey(gocui.KeyArrowDown, 0, gocui.ModNone)
	return nil
}

func (tp *TypePicker) keyEnter(g *gocui.Gui, v *gocui.View) error {
	tp.HandleKey(gocui.KeyEnter, 0, gocui.ModNone)
	return nil
}

func (tp *TypePicker) keyEsc(g *gocui.Gui, v *gocui.View) error {
	tp.HandleKey(gocui.KeyEsc, 0, gocui.ModNone)
	return nil
}

// moveSelection moves the selection by delta
func (tp *TypePicker) moveSelection(delta int) {
	// Account for "[Clear filter]" option at index 0
	maxIdx := len(tp.filteredTypes) // 0 is clear, 1..N are types
	if maxIdx == 0 {
		tp.selectedIdx = 0
		return
	}

	tp.selectedIdx += delta
	if tp.selectedIdx < 0 {
		tp.selectedIdx = maxIdx
	} else if tp.selectedIdx > maxIdx {
		tp.selectedIdx = 0
	}

	tp.renderContent()
}

// confirmSelection confirms the current selection
func (tp *TypePicker) confirmSelection() {
	var selectedType string

	if tp.selectedIdx == 0 {
		// "[Clear filter]" selected
		selectedType = ""
		utils.Log("TypePicker: clearing type filter")
	} else if tp.selectedIdx <= len(tp.filteredTypes) {
		selectedType = tp.filteredTypes[tp.selectedIdx-1].FullType
		utils.Log("TypePicker: selected type filter (index %d)", tp.selectedIdx-1)
	}

	tp.isVisible = false

	tp.g.DeleteView(typePickerViewName)
	tp.g.DeleteViewKeybindings(typePickerViewName)

	if tp.onSelect != nil {
		tp.onSelect(selectedType)
	}
}

// cancel closes the picker without selection
func (tp *TypePicker) cancel() {
	tp.isVisible = false

	tp.g.DeleteView(typePickerViewName)
	tp.g.DeleteViewKeybindings(typePickerViewName)

	if tp.onCancel != nil {
		tp.onCancel()
	}
}

// applyFilter applies the current filter text using fuzzy matching
func (tp *TypePicker) applyFilter() {
	if tp.filterText == "" {
		tp.filteredTypes = tp.allTypes
	} else {
		tp.filteredTypes = fuzzyFilterTypes(tp.allTypes, tp.filterText)
	}

	// Reset selection if out of bounds
	maxIdx := len(tp.filteredTypes)
	if tp.selectedIdx > maxIdx {
		tp.selectedIdx = 0
	}

	tp.renderContent()
}

// renderContent updates the picker view content
func (tp *TypePicker) renderContent() {
	v, err := tp.g.View(typePickerViewName)
	if err != nil {
		return
	}

	v.Clear()

	// Search input line
	fmt.Fprintf(v, " Search: %s█\n", tp.filterText)
	fmt.Fprintln(v, " "+strings.Repeat("─", typePickerWidth-4))

	// "[Clear filter]" option - always at index 0
	if tp.selectedIdx == 0 {
		fmt.Fprintln(v, " \x1b[7m► [Clear filter]\x1b[0m")
	} else {
		fmt.Fprintln(v, "   [Clear filter]")
	}

	// Calculate visible range (scrolling)
	visibleLines := typePickerHeight - 6 // Account for header, footer, borders
	startIdx := 0
	if tp.selectedIdx > visibleLines {
		startIdx = tp.selectedIdx - visibleLines
	}

	endIdx := startIdx + visibleLines
	if endIdx > len(tp.filteredTypes) {
		endIdx = len(tp.filteredTypes)
	}

	// Render visible type entries
	for i := startIdx; i < endIdx; i++ {
		entry := tp.filteredTypes[i]
		displayIdx := i + 1 // +1 because [Clear filter] is at 0

		// Truncate display name and full type to fit
		displayName := truncateString(entry.DisplayName, 25)
		fullType := truncateString(entry.FullType, 38)

		if displayIdx == tp.selectedIdx {
			fmt.Fprintf(v, " \x1b[7m► %-25s %s\x1b[0m\n", displayName, fullType)
		} else {
			fmt.Fprintf(v, "   %-25s \x1b[38;5;245m%s\x1b[0m\n", displayName, fullType)
		}
	}

	// Footer
	fmt.Fprintln(v, " "+strings.Repeat("─", typePickerWidth-4))
	fmt.Fprintf(v, " %d/%d types │ ↑↓: navigate │ Enter: select │ Esc: cancel",
		len(tp.filteredTypes), len(tp.allTypes))
}

// fuzzyFilterTypes filters types using fuzzy matching
// Returns types sorted by match quality (best matches first)
func fuzzyFilterTypes(types []resources.ResourceTypeEntry, query string) []resources.ResourceTypeEntry {
	if query == "" {
		return types
	}

	query = strings.ToLower(query)
	tokens := strings.Fields(query)

	type scoredEntry struct {
		entry resources.ResourceTypeEntry
		score int
	}

	var matches []scoredEntry

	for _, entry := range types {
		score := fuzzyMatchScore(entry, tokens)
		if score > 0 {
			matches = append(matches, scoredEntry{entry: entry, score: score})
		}
	}

	// Sort by score (highest first), then by display name
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].entry.DisplayName < matches[j].entry.DisplayName
	})

	result := make([]resources.ResourceTypeEntry, len(matches))
	for i, m := range matches {
		result[i] = m.entry
	}

	return result
}

// fuzzyMatchScore calculates a match score for a type entry against query tokens
// Returns 0 if no match, higher scores for better matches
func fuzzyMatchScore(entry resources.ResourceTypeEntry, tokens []string) int {
	displayLower := strings.ToLower(entry.DisplayName)
	typeLower := strings.ToLower(entry.FullType)

	score := 0

	// All tokens must match somewhere (AND logic)
	for _, token := range tokens {
		tokenMatched := false
		tokenScore := 0

		// Check display name
		if strings.Contains(displayLower, token) {
			tokenMatched = true
			if strings.HasPrefix(displayLower, token) {
				tokenScore = 30 // Prefix match in display name
			} else if strings.Contains(displayLower, " "+token) {
				tokenScore = 25 // Word boundary match
			} else {
				tokenScore = 15 // Contains match
			}

			// Exact match bonus
			if displayLower == token {
				tokenScore = 50
			}
		}

		// Check full type
		if strings.Contains(typeLower, token) {
			tokenMatched = true
			typeScore := 10 // Contains match in type
			if strings.HasPrefix(typeLower, token) {
				typeScore = 20
			}
			if typeScore > tokenScore {
				tokenScore = typeScore
			}
		}

		if !tokenMatched {
			return 0 // All tokens must match
		}
		score += tokenScore
	}

	return score
}

// truncateString truncates a string to maxLen, adding ".." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-2] + ".."
}
