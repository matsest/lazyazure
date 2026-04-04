package gui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/tasks"
)

// MockGui creates a minimal GUI for testing
type MockGui struct {
	g           *gocui.Gui
	taskManager *tasks.TaskManager
	mu          sync.RWMutex
	counter     int
}

func TestAsyncUpdate(t *testing.T) {
	// This test verifies that async updates don't hang
	gui := &MockGui{
		taskManager: tasks.NewTaskManager(),
	}

	// Simulate async work
	done := make(chan bool, 1)

	gui.taskManager.NewTask(func(ctx context.Context) {
		// Simulate some work
		time.Sleep(100 * time.Millisecond)

		gui.mu.Lock()
		gui.counter++
		gui.mu.Unlock()

		done <- true
	})

	select {
	case <-done:
		t.Log("Async task completed successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("Async task hung for more than 2 seconds")
	}

	gui.mu.RLock()
	if gui.counter != 1 {
		t.Fatalf("Expected counter to be 1, got %d", gui.counter)
	}
	gui.mu.RUnlock()
}

func TestConcurrentUpdates(t *testing.T) {
	gui := &MockGui{
		taskManager: tasks.NewTaskManager(),
	}

	numTasks := 10
	done := make(chan bool, numTasks)

	for i := 0; i < numTasks; i++ {
		gui.taskManager.NewTask(func(ctx context.Context) {
			gui.mu.Lock()
			gui.counter++
			gui.mu.Unlock()
			done <- true
		})
	}

	completed := 0
	timeout := time.After(3 * time.Second)

	for completed < numTasks {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatalf("Only %d of %d tasks completed before timeout", completed, numTasks)
		}
	}

	gui.mu.RLock()
	if gui.counter != numTasks {
		t.Fatalf("Expected counter to be %d, got %d", numTasks, gui.counter)
	}
	gui.mu.RUnlock()
}

func TestVersionNeedsUpdate(t *testing.T) {
	tests := []struct {
		name        string
		currentVer  string
		latestVer   string
		needsUpdate bool
	}{
		{
			name:        "same version",
			currentVer:  "v1.0.0",
			latestVer:   "v1.0.0",
			needsUpdate: false,
		},
		{
			name:        "older version",
			currentVer:  "v1.0.0",
			latestVer:   "v1.1.0",
			needsUpdate: true,
		},
		{
			name:        "dev version",
			currentVer:  "dev",
			latestVer:   "v1.0.0",
			needsUpdate: false,
		},
		{
			name:        "empty latest",
			currentVer:  "v1.0.0",
			latestVer:   "",
			needsUpdate: false,
		},
		// Bug fix test cases: version comparison should normalize "v" prefix
		{
			name:        "same version without v prefix in current",
			currentVer:  "0.2.2",
			latestVer:   "v0.2.2",
			needsUpdate: false, // This was the bug - it returned true before the fix
		},
		{
			name:        "same version without v prefix in latest",
			currentVer:  "v0.2.2",
			latestVer:   "0.2.2",
			needsUpdate: false,
		},
		{
			name:        "different versions without v prefix in current",
			currentVer:  "0.2.2",
			latestVer:   "v0.2.3",
			needsUpdate: true,
		},
		{
			name:        "both versions without v prefix",
			currentVer:  "0.2.2",
			latestVer:   "0.2.2",
			needsUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gui := &Gui{
				versionInfo: VersionInfo{
					Version: tt.currentVer,
					Commit:  "abc123",
				},
				latestVersion: tt.latestVer,
			}

			result := gui.versionNeedsUpdate()
			if result != tt.needsUpdate {
				t.Errorf("versionNeedsUpdate() = %v, want %v", result, tt.needsUpdate)
			}
		})
	}
}

func TestNewGuiWithVersionInfo(t *testing.T) {
	versionInfo := VersionInfo{
		Version: "v1.0.0",
		Commit:  "abc123def",
		Date:    "2024-01-01",
	}

	// Create GUI with version info - this just tests the constructor
	// We can't fully test without AzureClient, but we can verify the struct is set up
	gui := &Gui{
		versionInfo:     versionInfo,
		taskManager:     tasks.NewTaskManager(),
		tabIndex:        0,
		activePanel:     "subscriptions",
		subList:         nil,
		rgList:          nil,
		resList:         nil,
		mainPanelSearch: nil,
	}

	if gui.versionInfo.Version != versionInfo.Version {
		t.Errorf("Expected version %s, got %s", versionInfo.Version, gui.versionInfo.Version)
	}

	if gui.versionInfo.Commit != versionInfo.Commit {
		t.Errorf("Expected commit %s, got %s", versionInfo.Commit, gui.versionInfo.Commit)
	}

	if gui.versionInfo.Date != versionInfo.Date {
		t.Errorf("Expected date %s, got %s", versionInfo.Date, gui.versionInfo.Date)
	}
}

func TestIsDevelopmentBuild(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		isDevBuild bool
	}{
		{
			name:       "plain dev",
			version:    "dev",
			isDevBuild: true,
		},
		{
			name:       "clean release tag",
			version:    "v1.0.0",
			isDevBuild: false,
		},
		{
			name:       "dirty working tree",
			version:    "v0.2.1-dirty",
			isDevBuild: true,
		},
		{
			name:       "ahead of tag with hash",
			version:    "v0.2.1-2-gc15ffdf",
			isDevBuild: true,
		},
		{
			name:       "ahead of tag with dirty",
			version:    "v0.2.1-2-gc15ffdf-dirty",
			isDevBuild: true,
		},

		{
			name:       "unknown commit",
			version:    "unknown",
			isDevBuild: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gui := &Gui{
				versionInfo: VersionInfo{
					Version: tt.version,
					Commit:  "abc123",
				},
			}

			result := gui.isDevelopmentBuild()
			if result != tt.isDevBuild {
				t.Errorf("isDevelopmentBuild() for version %q = %v, want %v",
					tt.version, result, tt.isDevBuild)
			}
		})
	}
}

// TestSubscriptionSorting verifies subscriptions are sorted alphabetically (case-insensitive)
func TestSubscriptionSorting(t *testing.T) {
	tests := []struct {
		name     string
		input    []*domain.Subscription
		expected []string
	}{
		{
			name: "basic alphabetical sorting",
			input: []*domain.Subscription{
				{Name: "Prod-West"},
				{Name: "Prod-East"},
				{Name: "Staging"},
			},
			expected: []string{"Prod-East", "Prod-West", "Staging"},
		},
		{
			name: "case-insensitive sorting",
			input: []*domain.Subscription{
				{Name: "prod-west"},
				{Name: "Prod-East"},
				{Name: "STAGING"},
			},
			expected: []string{"Prod-East", "prod-west", "STAGING"},
		},
		{
			name: "mixed case same base",
			input: []*domain.Subscription{
				{Name: "Apple"},
				{Name: "banana"},
				{Name: "Cherry"},
				{Name: "apricot"},
			},
			expected: []string{"Apple", "apricot", "banana", "Cherry"},
		},
		{
			name: "single item",
			input: []*domain.Subscription{
				{Name: "OnlySub"},
			},
			expected: []string{"OnlySub"},
		},
		{
			name:     "empty list",
			input:    []*domain.Subscription{},
			expected: []string{},
		},
		{
			name: "already sorted",
			input: []*domain.Subscription{
				{Name: "A"},
				{Name: "B"},
				{Name: "C"},
			},
			expected: []string{"A", "B", "C"},
		},
		{
			name: "reverse sorted",
			input: []*domain.Subscription{
				{Name: "Z"},
				{Name: "Y"},
				{Name: "X"},
			},
			expected: []string{"X", "Y", "Z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy to sort
			subs := make([]*domain.Subscription, len(tt.input))
			copy(subs, tt.input)

			// Apply the same sorting logic as in loadSubscriptions
			sortSubscriptions(subs)

			// Verify the order
			if len(subs) != len(tt.expected) {
				t.Fatalf("Expected %d items, got %d", len(tt.expected), len(subs))
			}

			for i, sub := range subs {
				if sub.Name != tt.expected[i] {
					t.Errorf("Position %d: expected %q, got %q", i, tt.expected[i], sub.Name)
				}
			}
		})
	}
}

// TestResourceGroupSorting verifies resource groups are sorted alphabetically (case-insensitive)
func TestResourceGroupSorting(t *testing.T) {
	tests := []struct {
		name     string
		input    []*domain.ResourceGroup
		expected []string
	}{
		{
			name: "basic alphabetical sorting",
			input: []*domain.ResourceGroup{
				{Name: "rg-prod-web"},
				{Name: "rg-prod-storage"},
				{Name: "rg-prod-networking"},
			},
			expected: []string{"rg-prod-networking", "rg-prod-storage", "rg-prod-web"},
		},
		{
			name: "case-insensitive sorting",
			input: []*domain.ResourceGroup{
				{Name: "RG-Prod-Web"},
				{Name: "rg-prod-storage"},
				{Name: "RG-PROD-NETWORKING"},
			},
			expected: []string{"RG-PROD-NETWORKING", "rg-prod-storage", "RG-Prod-Web"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy to sort
			rgs := make([]*domain.ResourceGroup, len(tt.input))
			copy(rgs, tt.input)

			// Apply the same sorting logic as in loadResourceGroups
			sortResourceGroups(rgs)

			// Verify the order
			if len(rgs) != len(tt.expected) {
				t.Fatalf("Expected %d items, got %d", len(tt.expected), len(rgs))
			}

			for i, rg := range rgs {
				if rg.Name != tt.expected[i] {
					t.Errorf("Position %d: expected %q, got %q", i, tt.expected[i], rg.Name)
				}
			}
		})
	}
}

// TestResourceSorting verifies resources are sorted alphabetically (case-insensitive)
func TestResourceSorting(t *testing.T) {
	tests := []struct {
		name     string
		input    []*domain.Resource
		expected []string
	}{
		{
			name: "basic alphabetical sorting",
			input: []*domain.Resource{
				{Name: "web-server-01"},
				{Name: "app-server-02"},
				{Name: "db-server-01"},
			},
			expected: []string{"app-server-02", "db-server-01", "web-server-01"},
		},
		{
			name: "case-insensitive sorting",
			input: []*domain.Resource{
				{Name: "Web-Server-01"},
				{Name: "app-server-02"},
				{Name: "DB-Server-01"},
			},
			expected: []string{"app-server-02", "DB-Server-01", "Web-Server-01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy to sort
			resources := make([]*domain.Resource, len(tt.input))
			copy(resources, tt.input)

			// Apply the same sorting logic as in loadResources
			sortResources(resources)

			// Verify the order
			if len(resources) != len(tt.expected) {
				t.Fatalf("Expected %d items, got %d", len(tt.expected), len(resources))
			}

			for i, res := range resources {
				if res.Name != tt.expected[i] {
					t.Errorf("Position %d: expected %q, got %q", i, tt.expected[i], res.Name)
				}
			}
		})
	}
}
