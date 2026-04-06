package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matsest/lazyazure/pkg/gui/panels"
)

// ListPanel is a generic list panel component that wraps FilteredList
type ListPanel[T any] struct {
	list             *panels.FilteredList[T]
	cursor           int
	width            int
	height           int
	title            string
	active           bool
	getDisplay       func(T) string
	getDisplaySuffix func(T) string
}

// NewListPanel creates a new list panel
func NewListPanel[T any](title string, getDisplay, getDisplaySuffix func(T) string) *ListPanel[T] {
	return &ListPanel[T]{
		list:             panels.NewFilteredList[T](),
		cursor:           0,
		title:            title,
		getDisplay:       getDisplay,
		getDisplaySuffix: getDisplaySuffix,
	}
}

// SetItems sets the items in the list
func (lp *ListPanel[T]) SetItems(items []T) {
	lp.list.SetItems(items, func(t T) string {
		name := lp.getDisplay(t)
		suffix := lp.getDisplaySuffix(t)
		return name + "|" + suffix // Store separator for rendering
	})
	// Reset cursor if out of bounds
	if lp.cursor >= lp.list.Len() && lp.list.Len() > 0 {
		lp.cursor = lp.list.Len() - 1
	}
}

// SetSize sets the panel dimensions
func (lp *ListPanel[T]) SetSize(width, height int) {
	lp.width = width
	lp.height = height
}

// SetActive sets whether this panel is currently focused
func (lp *ListPanel[T]) SetActive(active bool) {
	lp.active = active
}

// Cursor returns the current cursor position
func (lp *ListPanel[T]) Cursor() int {
	return lp.cursor
}

// SetCursor sets the cursor position (clamped to valid range)
func (lp *ListPanel[T]) SetCursor(pos int) {
	if pos < 0 {
		pos = 0
	}
	if pos >= lp.list.Len() {
		if lp.list.Len() > 0 {
			pos = lp.list.Len() - 1
		} else {
			pos = 0
		}
	}
	lp.cursor = pos
}

// Selected returns the currently selected item
func (lp *ListPanel[T]) Selected() (T, bool) {
	return lp.list.Get(lp.cursor)
}

// Next moves the cursor down
func (lp *ListPanel[T]) Next() bool {
	if lp.cursor < lp.list.Len()-1 {
		lp.cursor++
		return true
	}
	return false
}

// Prev moves the cursor up
func (lp *ListPanel[T]) Prev() bool {
	if lp.cursor > 0 {
		lp.cursor--
		return true
	}
	return false
}

// PageDown moves the cursor down by page size
func (lp *ListPanel[T]) PageDown() bool {
	pageSize := lp.height - 2 // Account for borders
	if pageSize < 1 {
		pageSize = 1
	}
	newPos := lp.cursor + pageSize
	if newPos >= lp.list.Len() {
		newPos = lp.list.Len() - 1
	}
	if newPos > lp.cursor {
		lp.cursor = newPos
		return true
	}
	return false
}

// PageUp moves the cursor up by page size
func (lp *ListPanel[T]) PageUp() bool {
	pageSize := lp.height - 2
	if pageSize < 1 {
		pageSize = 1
	}
	newPos := lp.cursor - pageSize
	if newPos < 0 {
		newPos = 0
	}
	if newPos < lp.cursor {
		lp.cursor = newPos
		return true
	}
	return false
}

// SetFilter applies a filter to the list
func (lp *ListPanel[T]) SetFilter(text string) {
	lp.list.SetFilter(text)
	// Reset cursor when filtering
	lp.cursor = 0
}

// ClearFilter clears the current filter
func (lp *ListPanel[T]) ClearFilter() {
	lp.list.ClearFilter()
}

// IsFiltering returns whether the list is currently filtered
func (lp *ListPanel[T]) IsFiltering() bool {
	return lp.list.IsFiltering()
}

// GetFilterStats returns showing/total counts
func (lp *ListPanel[T]) GetFilterStats() (showing, total int) {
	return lp.list.GetFilterStats()
}

// Len returns the number of filtered items
func (lp *ListPanel[T]) Len() int {
	return lp.list.Len()
}

// Update handles messages for the list panel
func (lp *ListPanel[T]) Update(msg tea.Msg) (*ListPanel[T], tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			lp.Prev()
		case "down", "j":
			lp.Next()
		case "pgup":
			lp.PageUp()
		case "pgdown":
			lp.PageDown()
		}
	}
	return lp, nil
}

// View renders the list panel
func (lp *ListPanel[T]) View() string {
	styles := NewStyles()

	// Determine border style based on active state
	panelStyle := styles.InactivePanel
	if lp.active {
		panelStyle = styles.ActivePanel
	}

	// Apply size
	// Account for borders (2 lines vertically, 2 chars horizontally)
	contentWidth := lp.width - 2
	contentHeight := lp.height - 2
	panelStyle = panelStyle.Width(lp.width).Height(lp.height)

	// Build content
	var content strings.Builder

	// Add title header
	titleStyle := lipgloss.NewStyle().
		Foreground(TitleColor).
		Bold(true)
	if lp.active {
		titleStyle = titleStyle.Foreground(BorderColorActive)
	}
	titleLine := titleStyle.Render(lp.title)
	content.WriteString(titleLine)
	content.WriteString("\n")

	displayStrings := lp.list.GetFilteredDisplayStrings()

	// Calculate visible range (viewport scrolling)
	// Subtract 1 for title line
	listHeight := contentHeight - 1
	if listHeight < 1 {
		listHeight = 1
	}
	visibleStart := 0
	if lp.cursor >= listHeight {
		visibleStart = lp.cursor - listHeight + 1
	}
	visibleEnd := visibleStart + listHeight
	if visibleEnd > len(displayStrings) {
		visibleEnd = len(displayStrings)
	}

	// Render visible items
	for i := visibleStart; i < visibleEnd && i < len(displayStrings); i++ {
		if i > visibleStart {
			content.WriteString("\n")
		}

		// Parse display string (format: "name|suffix")
		parts := strings.SplitN(displayStrings[i], "|", 2)
		name := parts[0]
		suffix := ""
		if len(parts) > 1 {
			suffix = parts[1]
		}

		// Truncate to fit width
		line := FormatWithGraySuffix(name, suffix)
		renderedLen := lipgloss.Width(line)
		if renderedLen > contentWidth {
			// Simple truncation for now
			line = name
			if len(line) > contentWidth-3 {
				line = line[:contentWidth-3] + "..."
			}
		}

		// Apply selection styling
		if i == lp.cursor {
			line = styles.ListItemSelected.Width(contentWidth).Render(line)
		} else {
			line = styles.ListItem.Width(contentWidth).Render(line)
		}

		content.WriteString(line)
	}

	// Fill empty space (account for title line)
	visibleItems := visibleEnd - visibleStart
	for i := visibleItems; i < listHeight; i++ {
		content.WriteString("\n")
		content.WriteString(strings.Repeat(" ", contentWidth))
	}

	return panelStyle.Render(content.String())
}
