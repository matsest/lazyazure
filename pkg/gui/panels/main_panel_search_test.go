package panels

import (
	"strings"
	"testing"
)

func TestNewMainPanelSearch(t *testing.T) {
	mps := NewMainPanelSearch()

	if mps.IsActive() {
		t.Error("New MainPanelSearch should not be active")
	}

	if mps.GetSearchText() != "" {
		t.Errorf("Expected empty search text, got '%s'", mps.GetSearchText())
	}

	if mps.GetMatchCount() != 0 {
		t.Errorf("Expected 0 matches, got %d", mps.GetMatchCount())
	}

	current, total := mps.GetCurrentMatch()
	if current != 0 || total != 0 {
		t.Errorf("Expected (0, 0), got (%d, %d)", current, total)
	}
}

func TestSetContent(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Name: my-resource",
		"Type: Storage Account",
		"Location: East US",
	}

	mps.SetContent(lines)

	// Without search, should return original content
	content := mps.GetHighlightedContent()
	if len(content) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(content))
	}

	for i, line := range content {
		if line != lines[i] {
			t.Errorf("Line %d: expected '%s', got '%s'", i, lines[i], line)
		}
	}
}

func TestSetSearch(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Name: my-resource",
		"Type: Storage Account",
		"Location: East US",
		"Tags: environment=production",
	}
	mps.SetContent(lines)

	mps.SetSearch("storage")

	if !mps.IsActive() {
		t.Error("Search should be active after SetSearch")
	}

	if mps.GetSearchText() != "storage" {
		t.Errorf("Expected search text 'storage', got '%s'", mps.GetSearchText())
	}

	// Should find 1 match ("Storage Account")
	if mps.GetMatchCount() != 1 {
		t.Errorf("Expected 1 match, got %d", mps.GetMatchCount())
	}

	// Current match should be set to first match
	current, total := mps.GetCurrentMatch()
	if current != 1 || total != 1 {
		t.Errorf("Expected current match (1, 1), got (%d, %d)", current, total)
	}
}

func TestCaseInsensitiveSearch(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Name: My-Resource",
		"Type: Storage Account",
		"LOCATION: EAST US",
	}
	mps.SetContent(lines)

	mps.SetSearch("storage")
	if mps.GetMatchCount() != 1 {
		t.Errorf("Case-insensitive search should find 'Storage', got %d matches", mps.GetMatchCount())
	}

	mps.SetSearch("EAST")
	if mps.GetMatchCount() != 1 {
		t.Errorf("Case-insensitive search should find 'EAST', got %d matches", mps.GetMatchCount())
	}

	mps.SetSearch("resource")
	if mps.GetMatchCount() != 1 {
		t.Errorf("Case-insensitive search should find 'Resource', got %d matches", mps.GetMatchCount())
	}
}

func TestMultipleMatches(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Name: prod-database",
		"Type: SQL Database",
		"Server: prod-sql-server",
		"Database: production",
	}
	mps.SetContent(lines)

	mps.SetSearch("prod")

	// Should find 3 matches
	if mps.GetMatchCount() != 3 {
		t.Errorf("Expected 3 matches for 'prod', got %d", mps.GetMatchCount())
	}

	// Current match should be first
	current, total := mps.GetCurrentMatch()
	if current != 1 || total != 3 {
		t.Errorf("Expected (1, 3), got (%d, %d)", current, total)
	}
}

func TestClearSearch(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Name: my-resource",
		"Type: Storage Account",
	}
	mps.SetContent(lines)
	mps.SetSearch("storage")

	if !mps.IsActive() {
		t.Fatal("Search should be active")
	}

	mps.ClearSearch()

	if mps.IsActive() {
		t.Error("Search should not be active after ClearSearch")
	}

	if mps.GetSearchText() != "" {
		t.Errorf("Search text should be empty, got '%s'", mps.GetSearchText())
	}

	if mps.GetMatchCount() != 0 {
		t.Errorf("Match count should be 0, got %d", mps.GetMatchCount())
	}

	// Content should be returned without highlights
	content := mps.GetHighlightedContent()
	if content[0] != lines[0] {
		t.Error("Content should be unchanged after clearing search")
	}
}

func TestNextPrevMatch(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Line 1: test",
		"Line 2: no match",
		"Line 3: test here",
		"Line 4: no match",
		"Line 5: test again",
	}
	mps.SetContent(lines)
	mps.SetSearch("test")

	// Should have 3 matches on lines 0, 2, 4
	if mps.GetMatchCount() != 3 {
		t.Fatalf("Expected 3 matches, got %d", mps.GetMatchCount())
	}

	// Initial match should be line 0
	if line := mps.GetCurrentMatchLine(); line != 0 {
		t.Errorf("Initial match should be line 0, got %d", line)
	}

	// Next match should be line 2
	if line := mps.NextMatch(); line != 2 {
		t.Errorf("Next match should be line 2, got %d", line)
	}

	current, _ := mps.GetCurrentMatch()
	if current != 2 {
		t.Errorf("Current match should be 2, got %d", current)
	}

	// Next match should be line 4
	if line := mps.NextMatch(); line != 4 {
		t.Errorf("Next match should be line 4, got %d", line)
	}

	// Next should wrap to line 0
	if line := mps.NextMatch(); line != 0 {
		t.Errorf("Should wrap to line 0, got %d", line)
	}

	// Previous should go back to line 4
	if line := mps.PrevMatch(); line != 4 {
		t.Errorf("Prev match should be line 4, got %d", line)
	}

	// Previous should be line 2
	if line := mps.PrevMatch(); line != 2 {
		t.Errorf("Prev match should be line 2, got %d", line)
	}
}

func TestHighlightLine(t *testing.T) {
	mps := NewMainPanelSearch()
	mps.searchText = "test"
	mps.isActive = true

	tests := []struct {
		name           string
		line           string
		isCurrentMatch bool
		expected       string
	}{
		{
			name:           "single match",
			line:           "This is a test line",
			isCurrentMatch: false,
			expected:       "This is a " + highlightStart + "test" + highlightEnd + " line",
		},
		{
			name:           "multiple matches",
			line:           "test line with test in it",
			isCurrentMatch: false,
			expected:       highlightStart + "test" + highlightEnd + " line with " + highlightStart + "test" + highlightEnd + " in it",
		},
		{
			name:           "no match",
			line:           "This line has no matches",
			isCurrentMatch: false,
			expected:       "This line has no matches",
		},
		{
			name:           "case insensitive",
			line:           "This is a TEST line",
			isCurrentMatch: false,
			expected:       "This is a " + highlightStart + "TEST" + highlightEnd + " line",
		},
		{
			name:           "current match",
			line:           "This is a test line",
			isCurrentMatch: true,
			expected:       "This is a " + highlightStartCurrent + "test" + highlightEnd + " line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mps.highlightLine(tt.line, tt.isCurrentMatch)
			if result != tt.expected {
				t.Errorf("highlightLine() = '%s', expected '%s'", result, tt.expected)
			}
		})
	}
}

func TestGetHighlightedContent(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Name: test-resource",
		"Type: test Account",
		"Location: East US",
	}
	mps.SetContent(lines)
	mps.SetSearch("test")

	content := mps.GetHighlightedContent()

	// Check that matches are highlighted (line 0 is current match, so uses highlightStartCurrent)
	if !strings.Contains(content[0], highlightStartCurrent) {
		t.Error("First line (current match) should contain current highlight start code")
	}

	if !strings.Contains(content[0], highlightEnd) {
		t.Error("First line should contain highlight end code")
	}

	// Check line 1 is also highlighted ("test Account") - regular highlight since it's not current
	if !strings.Contains(content[1], highlightStart) {
		t.Error("Second line should contain highlight")
	}

	// Check line 2 has no highlight
	if strings.Contains(content[2], highlightStart) || strings.Contains(content[2], highlightStartCurrent) {
		t.Error("Third line should not contain highlight")
	}
}

func TestEmptySearch(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Name: my-resource",
		"Type: Storage Account",
	}
	mps.SetContent(lines)

	// Setting empty search should deactivate
	mps.SetSearch("")

	if mps.IsActive() {
		t.Error("Empty search should not be active")
	}

	// Content should be unchanged
	content := mps.GetHighlightedContent()
	for i, line := range content {
		if line != lines[i] {
			t.Errorf("Line %d should be unchanged", i)
		}
	}
}

func TestNoMatches(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Name: my-resource",
		"Type: Storage Account",
	}
	mps.SetContent(lines)

	mps.SetSearch("xyz")

	if mps.GetMatchCount() != 0 {
		t.Errorf("Expected 0 matches for 'xyz', got %d", mps.GetMatchCount())
	}

	current, total := mps.GetCurrentMatch()
	if current != 0 || total != 0 {
		t.Errorf("Expected (0, 0) for no matches, got (%d, %d)", current, total)
	}

	if mps.GetCurrentMatchLine() != -1 {
		t.Error("GetCurrentMatchLine should return -1 when no matches")
	}
}

func TestMainPanelSearchConcurrentAccess(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Line 1: test",
		"Line 2: test",
		"Line 3: test",
	}
	mps.SetContent(lines)

	done := make(chan bool, 4)

	// Concurrent set search
	go func() {
		for i := 0; i < 50; i++ {
			mps.SetSearch("test")
		}
		done <- true
	}()

	// Concurrent clear
	go func() {
		for i := 0; i < 50; i++ {
			mps.ClearSearch()
		}
		done <- true
	}()

	// Concurrent get content
	go func() {
		for i := 0; i < 50; i++ {
			mps.GetHighlightedContent()
		}
		done <- true
	}()

	// Concurrent navigation
	go func() {
		for i := 0; i < 50; i++ {
			mps.NextMatch()
			mps.PrevMatch()
		}
		done <- true
	}()

	for i := 0; i < 4; i++ {
		<-done
	}
}

func TestReApplySearchOnContentChange(t *testing.T) {
	mps := NewMainPanelSearch()
	lines := []string{
		"Name: my-resource",
		"Type: Storage Account",
	}
	mps.SetContent(lines)
	mps.SetSearch("storage")

	if mps.GetMatchCount() != 1 {
		t.Fatalf("Expected 1 match initially, got %d", mps.GetMatchCount())
	}

	// Change content while search is active
	newLines := []string{
		"Name: storage-blob",
		"Type: Storage Account",
		"Location: West US",
	}
	mps.SetContent(newLines)

	// Should now find 2 matches
	if mps.GetMatchCount() != 2 {
		t.Errorf("Expected 2 matches after content change, got %d", mps.GetMatchCount())
	}
}
