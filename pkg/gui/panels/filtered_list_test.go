package panels

import (
	"strings"
	"testing"
)

func TestNewFilteredList(t *testing.T) {
	fl := NewFilteredList[string]()

	if fl.Len() != 0 {
		t.Errorf("Expected empty list, got Len() = %d", fl.Len())
	}

	if fl.IsFiltering() {
		t.Error("New list should not be filtering")
	}

	showing, total := fl.GetFilterStats()
	if showing != 0 || total != 0 {
		t.Errorf("Expected (0, 0), got (%d, %d)", showing, total)
	}
}

func TestSetItems(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "cherry"}

	fl.SetItems(items, func(s string) string { return s })

	if fl.Len() != 3 {
		t.Errorf("Expected Len() = 3, got %d", fl.Len())
	}

	showing, total := fl.GetFilterStats()
	if showing != 3 || total != 3 {
		t.Errorf("Expected (3, 3), got (%d, %d)", showing, total)
	}
}

func TestSetFilter(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "application", "cherry", "pineapple"}
	fl.SetItems(items, func(s string) string { return s })

	// Test basic filter
	fl.SetFilter("app")

	if !fl.IsFiltering() {
		t.Error("Should be filtering after SetFilter")
	}

	if fl.GetFilterText() != "app" {
		t.Errorf("Expected filter text 'app', got '%s'", fl.GetFilterText())
	}

	// Should match: apple, application, pineapple
	if fl.Len() != 3 {
		t.Errorf("Expected 3 matches, got %d", fl.Len())
	}

	showing, total := fl.GetFilterStats()
	if showing != 3 || total != 5 {
		t.Errorf("Expected (3, 5), got (%d, %d)", showing, total)
	}

	// Test case insensitivity
	fl.SetFilter("APP")
	if fl.Len() != 3 {
		t.Errorf("Expected 3 case-insensitive matches, got %d", fl.Len())
	}

	// Test no matches
	fl.SetFilter("xyz")
	if fl.Len() != 0 {
		t.Errorf("Expected 0 matches, got %d", fl.Len())
	}
}

func TestClearFilter(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "cherry"}
	fl.SetItems(items, func(s string) string { return s })

	fl.SetFilter("app")
	if fl.Len() != 1 {
		t.Errorf("Expected 1 match before clear, got %d", fl.Len())
	}

	fl.ClearFilter()

	if fl.IsFiltering() {
		t.Error("Should not be filtering after ClearFilter")
	}

	if fl.Len() != 3 {
		t.Errorf("Expected all 3 items after clear, got %d", fl.Len())
	}

	if fl.GetFilterText() != "" {
		t.Errorf("Expected empty filter text, got '%s'", fl.GetFilterText())
	}
}

func TestGet(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "cherry"}
	fl.SetItems(items, func(s string) string { return s })

	// Test getting items
	if item, ok := fl.Get(0); !ok || item != "apple" {
		t.Errorf("Expected 'apple' at index 0, got '%s'", item)
	}

	if item, ok := fl.Get(2); !ok || item != "cherry" {
		t.Errorf("Expected 'cherry' at index 2, got '%s'", item)
	}

	// Test out of bounds
	if _, ok := fl.Get(-1); ok {
		t.Error("Expected false for negative index")
	}

	if _, ok := fl.Get(10); ok {
		t.Error("Expected false for index beyond length")
	}

	// Test with filter
	fl.SetFilter("banana")
	if item, ok := fl.Get(0); !ok || item != "banana" {
		t.Errorf("Expected 'banana' at filtered index 0, got '%s'", item)
	}

	if _, ok := fl.Get(1); ok {
		t.Error("Expected false for index beyond filtered length")
	}
}

func TestGetDisplayString(t *testing.T) {
	fl := NewFilteredList[int]()
	items := []int{1, 2, 3}
	fl.SetItems(items, func(i int) string { return string('A' + rune(i-1)) })

	if display, ok := fl.GetDisplayString(0); !ok || display != "A" {
		t.Errorf("Expected 'A' at index 0, got '%s'", display)
	}

	if display, ok := fl.GetDisplayString(2); !ok || display != "C" {
		t.Errorf("Expected 'C' at index 2, got '%s'", display)
	}

	if _, ok := fl.GetDisplayString(10); ok {
		t.Error("Expected false for out of bounds index")
	}
}

func TestGetFilteredDisplayStrings(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "cherry"}
	fl.SetItems(items, func(s string) string { return "Item: " + s })

	displays := fl.GetFilteredDisplayStrings()
	if len(displays) != 3 {
		t.Errorf("Expected 3 display strings, got %d", len(displays))
	}

	if displays[0] != "Item: apple" {
		t.Errorf("Expected 'Item: apple', got '%s'", displays[0])
	}

	// Test with filter
	fl.SetFilter("app")
	displays = fl.GetFilteredDisplayStrings()
	if len(displays) != 1 {
		t.Errorf("Expected 1 display string after filter, got %d", len(displays))
	}

	if displays[0] != "Item: apple" {
		t.Errorf("Expected 'Item: apple', got '%s'", displays[0])
	}
}

func TestMapFilteredToOriginal(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "cherry", "application"}
	fl.SetItems(items, func(s string) string { return s })

	// Without filter: index 0 should map to original 0
	if orig, ok := fl.MapFilteredToOriginal(0); !ok || orig != 0 {
		t.Errorf("Expected original index 0, got %d", orig)
	}

	// With filter "app" -> apple (0), application (3)
	fl.SetFilter("app")

	if orig, ok := fl.MapFilteredToOriginal(0); !ok || orig != 0 {
		t.Errorf("Expected original index 0 for filtered index 0, got %d", orig)
	}

	if orig, ok := fl.MapFilteredToOriginal(1); !ok || orig != 3 {
		t.Errorf("Expected original index 3 for filtered index 1, got %d", orig)
	}

	if _, ok := fl.MapFilteredToOriginal(10); ok {
		t.Error("Expected false for out of bounds filtered index")
	}
}

func TestFindByOriginalIndex(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "cherry", "application"}
	fl.SetItems(items, func(s string) string { return s })

	fl.SetFilter("app")

	// apple is at filtered index 0, application at filtered index 1
	if filtered, ok := fl.FindByOriginalIndex(0); !ok || filtered != 0 {
		t.Errorf("Expected filtered index 0 for original 0, got %d", filtered)
	}

	if filtered, ok := fl.FindByOriginalIndex(3); !ok || filtered != 1 {
		t.Errorf("Expected filtered index 1 for original 3, got %d", filtered)
	}

	// banana (1) and cherry (2) are not in filtered list
	if _, ok := fl.FindByOriginalIndex(1); ok {
		t.Error("Expected false for original index not in filtered list")
	}
}

func TestFilterPersistenceOnSetItems(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "cherry"}
	fl.SetItems(items, func(s string) string { return s })

	fl.SetFilter("app")
	if fl.Len() != 1 {
		t.Errorf("Expected 1 match, got %d", fl.Len())
	}

	// Update items with filter still active
	newItems := []string{"application", "banana", "pineapple", "cherry"}
	fl.SetItems(newItems, func(s string) string { return s })

	// Filter should be reapplied: application, pineapple
	if fl.Len() != 2 {
		t.Errorf("Expected 2 matches after SetItems with filter, got %d", fl.Len())
	}

	if !fl.IsFiltering() {
		t.Error("Filter should still be active after SetItems")
	}
}

func TestConcurrentAccess(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "cherry"}
	fl.SetItems(items, func(s string) string { return s })

	// Concurrent reads
	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 100; i++ {
			fl.Len()
			fl.GetFilterStats()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			fl.Get(0)
			fl.GetDisplayString(0)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			fl.SetFilter("app")
			fl.ClearFilter()
		}
		done <- true
	}()

	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestSearchWithSuffix(t *testing.T) {
	// Test case similar to actual usage: "name (suffix)" format
	fl := NewFilteredList[string]()
	items := []string{"my-rg-1", "my-rg-2", "prod-rg"}
	fl.SetItems(items, func(name string) string {
		location := "East US"
		if name == "prod-rg" {
			location = "West US"
		}
		return name + " (" + location + ")"
	})

	// Search by location
	fl.SetFilter("east")
	if fl.Len() != 2 {
		t.Errorf("Expected 2 matches for 'east', got %d", fl.Len())
	}

	// Search by name
	fl.SetFilter("prod")
	if fl.Len() != 1 {
		t.Errorf("Expected 1 match for 'prod', got %d", fl.Len())
	}

	item, _ := fl.Get(0)
	if item != "prod-rg" {
		t.Errorf("Expected 'prod-rg', got '%s'", item)
	}
}

func TestEmptyFilterShowsAll(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "cherry"}
	fl.SetItems(items, func(s string) string { return s })

	// Empty filter should show all items
	fl.SetFilter("")
	if fl.Len() != 3 {
		t.Errorf("Expected 3 items with empty filter, got %d", fl.Len())
	}

	if fl.IsFiltering() {
		t.Error("Should not be filtering with empty text")
	}
}

func TestFindIndex(t *testing.T) {
	fl := NewFilteredList[string]()
	items := []string{"apple", "banana", "cherry", "application", "pineapple"}
	fl.SetItems(items, func(s string) string { return s })

	// Find existing item
	if idx, ok := fl.FindIndex(func(s string) bool { return s == "banana" }); !ok || idx != 1 {
		t.Errorf("Expected index 1 for 'banana', got %d, ok=%v", idx, ok)
	}

	// Find first matching item
	if idx, ok := fl.FindIndex(func(s string) bool { return strings.HasPrefix(s, "app") }); !ok || idx != 0 {
		t.Errorf("Expected index 0 for first 'app*' item, got %d, ok=%v", idx, ok)
	}

	// Find non-existing item
	if _, ok := fl.FindIndex(func(s string) bool { return s == "orange" }); ok {
		t.Error("Expected false for non-existing item")
	}

	// Test with filter active
	fl.SetFilter("app")
	// Filtered items: apple (0), application (1), pineapple (2)

	// Find item in filtered list
	if idx, ok := fl.FindIndex(func(s string) bool { return s == "application" }); !ok || idx != 1 {
		t.Errorf("Expected index 1 for 'application' in filtered list, got %d, ok=%v", idx, ok)
	}

	// Find item not in filtered list
	if _, ok := fl.FindIndex(func(s string) bool { return s == "banana" }); ok {
		t.Error("Expected false for item not in filtered list")
	}
}
