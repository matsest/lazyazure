package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/matsest/lazyazure/pkg/gui"
)

func TestModelView(t *testing.T) {
	// Create a model
	versionInfo := gui.VersionInfo{
		Version: "test",
		Commit:  "abc123",
		Date:    "2026-01-01",
	}

	m := NewModel(nil, nil, versionInfo, true)
	m.width = 120
	m.height = 40
	m.calculateLayout()

	// Get the view output
	output := m.View()

	// Check that all expected elements are present
	tests := []struct {
		name     string
		expected string
	}{
		{"Auth panel", "Auth"},
		{"Subscriptions panel", "Subscriptions"},
		{"Resource Groups panel", "Resource Groups"},
		{"Resources panel", "Resources"},
		{"Status bar", "Welcome to LazyAzure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(output, tt.expected) {
				t.Errorf("View() missing expected content %q\nOutput:\n%s", tt.expected, output)
			}
		})
	}
}

func TestLayoutFitsWithinTerminal(t *testing.T) {
	versionInfo := gui.VersionInfo{
		Version: "test",
		Commit:  "abc123",
		Date:    "2026-01-01",
	}

	// Test with various terminal sizes
	sizes := []struct {
		width  int
		height int
	}{
		{120, 40}, // Large terminal
		{100, 30}, // Medium terminal
		{80, 24},  // Small terminal
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m := NewModel(nil, nil, versionInfo, true)
			m.width = size.width
			m.height = size.height
			m.calculateLayout()

			output := m.View()

			// Check that output width doesn't exceed terminal width
			lines := strings.Split(output, "\n")
			for i, line := range lines {
				lineWidth := lipgloss.Width(line)
				if lineWidth > size.width {
					t.Errorf("Line %d exceeds terminal width: got %d, want <= %d\nLine: %s",
						i, lineWidth, size.width, line)
				}
			}

			// Check that output height doesn't exceed terminal height by too much
			// Remove trailing empty line if present (from trailing newline)
			if len(lines) > 0 && lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			// Allow up to 5 lines of overflow for lipgloss border rendering
			maxAllowedHeight := size.height + 5
			if len(lines) > maxAllowedHeight {
				t.Errorf("Output exceeds terminal height by too much: got %d lines, want <= %d (last line: %q)",
					len(lines), maxAllowedHeight, lines[len(lines)-1])
			}
		})
	}
}
