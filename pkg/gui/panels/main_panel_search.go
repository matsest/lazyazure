package panels

import (
	"strings"
	"sync"
)

// ANSI codes for highlighting
const (
	highlightStart        = "\x1b[48;5;250m" // Light grey background for other matches
	highlightStartCurrent = "\x1b[48;5;226m" // Yellow background for current match
	highlightEnd          = "\x1b[0m"        // Reset
)

// MainPanelSearch manages search functionality for the main/details panel
type MainPanelSearch struct {
	isActive     bool
	searchText   string
	lines        []string // Original content lines
	matchIndices []int    // Line indices that contain matches
	currentMatch int      // Currently focused match (-1 if none)
	mu           sync.RWMutex
}

// NewMainPanelSearch creates a new main panel search manager
func NewMainPanelSearch() *MainPanelSearch {
	return &MainPanelSearch{
		lines:        make([]string, 0),
		matchIndices: make([]int, 0),
		currentMatch: -1,
	}
}

// SetContent sets the content to search within
func (mps *MainPanelSearch) SetContent(lines []string) {
	mps.mu.Lock()
	defer mps.mu.Unlock()

	mps.lines = lines
	// Re-apply search if active
	if mps.isActive && mps.searchText != "" {
		mps.findMatches()
	}
}

// SetSearch sets the search text and finds matches
func (mps *MainPanelSearch) SetSearch(text string) {
	mps.mu.Lock()
	defer mps.mu.Unlock()

	mps.searchText = strings.ToLower(text)
	mps.isActive = text != ""
	mps.currentMatch = -1

	if mps.isActive {
		mps.findMatches()
		if len(mps.matchIndices) > 0 {
			mps.currentMatch = 0
		}
	} else {
		mps.matchIndices = make([]int, 0)
	}
}

// ClearSearch clears the current search
func (mps *MainPanelSearch) ClearSearch() {
	mps.mu.Lock()
	defer mps.mu.Unlock()

	mps.isActive = false
	mps.searchText = ""
	mps.matchIndices = make([]int, 0)
	mps.currentMatch = -1
}

// IsActive returns true if search is active
func (mps *MainPanelSearch) IsActive() bool {
	mps.mu.RLock()
	defer mps.mu.RUnlock()
	return mps.isActive
}

// GetSearchText returns the current search text
func (mps *MainPanelSearch) GetSearchText() string {
	mps.mu.RLock()
	defer mps.mu.RUnlock()
	return mps.searchText
}

// GetMatchCount returns the total number of matches
func (mps *MainPanelSearch) GetMatchCount() int {
	mps.mu.RLock()
	defer mps.mu.RUnlock()
	return len(mps.matchIndices)
}

// GetCurrentMatch returns the current match index (1-based) and total
func (mps *MainPanelSearch) GetCurrentMatch() (current, total int) {
	mps.mu.RLock()
	defer mps.mu.RUnlock()

	total = len(mps.matchIndices)
	if mps.currentMatch >= 0 && total > 0 {
		current = mps.currentMatch + 1 // Convert to 1-based
	}
	return current, total
}

// GetCurrentMatchLine returns the line number of the current match
func (mps *MainPanelSearch) GetCurrentMatchLine() int {
	mps.mu.RLock()
	defer mps.mu.RUnlock()

	if mps.currentMatch >= 0 && mps.currentMatch < len(mps.matchIndices) {
		return mps.matchIndices[mps.currentMatch]
	}
	return -1
}

// NextMatch moves to the next match and returns the line number
func (mps *MainPanelSearch) NextMatch() int {
	mps.mu.Lock()
	defer mps.mu.Unlock()

	if len(mps.matchIndices) == 0 {
		return -1
	}

	mps.currentMatch++
	if mps.currentMatch >= len(mps.matchIndices) {
		mps.currentMatch = 0 // Wrap around
	}

	return mps.matchIndices[mps.currentMatch]
}

// PrevMatch moves to the previous match and returns the line number
func (mps *MainPanelSearch) PrevMatch() int {
	mps.mu.Lock()
	defer mps.mu.Unlock()

	if len(mps.matchIndices) == 0 {
		return -1
	}

	mps.currentMatch--
	if mps.currentMatch < 0 {
		mps.currentMatch = len(mps.matchIndices) - 1 // Wrap around
	}

	return mps.matchIndices[mps.currentMatch]
}

// GetHighlightedContent returns the content with search matches highlighted
func (mps *MainPanelSearch) GetHighlightedContent() []string {
	mps.mu.RLock()
	defer mps.mu.RUnlock()

	if !mps.isActive || mps.searchText == "" {
		return mps.lines
	}

	// Get the current match line index
	currentMatchLine := -1
	if mps.currentMatch >= 0 && mps.currentMatch < len(mps.matchIndices) {
		currentMatchLine = mps.matchIndices[mps.currentMatch]
	}

	result := make([]string, len(mps.lines))
	for i, line := range mps.lines {
		// Use brighter highlight for the current match line
		isCurrentMatch := (i == currentMatchLine)
		result[i] = mps.highlightLine(line, isCurrentMatch)
	}
	return result
}

// findMatches finds all lines containing the search term
// Must be called with lock held
func (mps *MainPanelSearch) findMatches() {
	mps.matchIndices = make([]int, 0)
	searchLower := mps.searchText

	for i, line := range mps.lines {
		if strings.Contains(strings.ToLower(line), searchLower) {
			mps.matchIndices = append(mps.matchIndices, i)
		}
	}
}

// highlightLine highlights all occurrences of search term in a line
// Must be called with lock held
// isCurrentMatch indicates if this is the currently focused match line
func (mps *MainPanelSearch) highlightLine(line string, isCurrentMatch bool) string {
	if mps.searchText == "" {
		return line
	}

	searchLower := mps.searchText
	lineLower := strings.ToLower(line)

	// Choose highlight style based on whether this is the current match line
	highlightCode := highlightStart
	if isCurrentMatch {
		highlightCode = highlightStartCurrent
	}

	// Find all occurrences
	var result strings.Builder
	start := 0

	for {
		idx := strings.Index(lineLower[start:], searchLower)
		if idx == -1 {
			break
		}

		// Adjust index to be relative to original line
		idx += start

		// Write text before match
		result.WriteString(line[start:idx])

		// Write highlighted match with appropriate style
		result.WriteString(highlightCode)
		result.WriteString(line[idx : idx+len(mps.searchText)])
		result.WriteString(highlightEnd)

		// Move past this match
		start = idx + len(mps.searchText)
	}

	// Write remaining text
	result.WriteString(line[start:])

	return result.String()
}
