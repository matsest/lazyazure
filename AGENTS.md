# LazyAzure - Agent Guidelines

This document provides guidelines and lessons learned for agents working on the LazyAzure codebase.

## Project Overview

LazyAzure is a TUI (Terminal User Interface) application for Azure resource management, inspired by lazydocker. It uses:
- **gocui** for the terminal interface
- **Azure SDK for Go** for Azure API interactions
- **DefaultAzureCredential** for authentication (supports multiple auth methods)

## Critical Guidelines

### 1. TUI Development with gocui

#### Event Loop and Threading
- **CRITICAL**: Never hold locks (mutex) when calling gocui operations
- **CRITICAL**: Never call `gui.g.Update()` or `gui.g.UpdateAsync()` while holding a lock
- Use `go func()` for async operations, then use `gui.g.UpdateAsync()` for UI updates
- `UpdateAsync()` is safer than `Update()` as it doesn't spawn a goroutine

#### Cursor Handling
- Use `v.SetCursor(x, y)` directly instead of `v.TextArea.MoveCursorDown/Up`
- Get cursor position with `v.Cursor()` which returns (x, y)
- Don't mix cursor tracking with origin/offset calculations unless necessary

#### View Management
- Use `gocui.IsUnknownView(err)` to check if a view already exists
- Views are identified by string names (e.g., "subscriptions", "resourcegroups")
- Set current view with `gui.g.SetCurrentView("viewname")`

### 2. Mutex Best Practices

#### Deadlock Prevention
```go
// ❌ WRONG: Holding lock while calling another function that needs lock
func (gui *Gui) nextLine() {
    gui.mu.Lock()
    defer gui.mu.Unlock()
    gui.selectItem()  // selectItem() also needs Lock = DEADLOCK!
}

// ✅ CORRECT: Release lock before calling other methods
func (gui *Gui) nextLine() {
    gui.mu.RLock()
    count := len(gui.items)
    gui.mu.RUnlock()
    
    // ... do work ...
    
    gui.selectItem()  // Now safe to acquire Lock
}
```

#### Lock Hierarchy
1. `RLock()` for reading data length/slices
2. Do work without locks
3. `Lock()` only for updating state
4. Never hold locks during I/O or UI operations

### 3. Debug Logging

The application supports opt-in debug logging:

```bash
# Enable debug logging
LAZYAZURE_DEBUG=1 ./lazyazure

# View logs
cat ~/.lazyazure/debug.log
```

**When to add logging:**
- Entry/exit of complex functions
- Before/after async operations
- When acquiring/releasing locks
- Error conditions
- User actions (key presses, selections)

**Implementation:**
```go
import "github.com/matsest/lazyazure/pkg/utils"

// Logging is a no-op if LAZYAZURE_DEBUG is not set
utils.Log("message: %s", value)
```

### 4. Layout and Panels

#### Stacked Panel Layout
The UI uses a 4-panel stacked layout on the left side:
```
┌─────────────────────┬──────────────────────────────────┐
│ Auth (3-5 lines)    │  Details Panel                   │
├─────────────────────┤                                  │
│ Subscriptions       │  Shows selected item details     │
│ (~20% of sidebar)   │  with Summary/JSON tabs          │
├─────────────────────┤                                  │
│ Resource Groups     │                                  │
│ (~30% of sidebar)   │                                  │
├─────────────────────┤                                  │
│ Resources           │                                  │
│ (remaining space)   │                                  │
└─────────────────────┴──────────────────────────────────┘
[Status Bar: context-aware help text                     ]
```

#### Panel Focus
- Use `activePanel` field to track which panel has focus
- Visual indicator: Frame color (green = active, white = inactive)
- Switch panels with `Tab` key
- Each panel has independent navigation keybindings

#### Panel Alignment
- Calculate Y coordinates carefully to align panel bottoms
- The resources panel should extend to `statusY` to align with the main/details panel
- Account for frame borders when calculating heights: view ends at coordinate Y, border is drawn at Y

### 5. Azure SDK Patterns

#### Authentication

LazyAzure uses `DefaultAzureCredential` which automatically tries multiple authentication methods in order:

1. Environment variables (`AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_TENANT_ID`)
2. Managed Identity (for apps running in Azure)
3. Azure CLI (`az login`)
4. Azure PowerShell
5. Visual Studio Code credentials
6. Azure Developer CLI (`azd`)

```go
// Uses DefaultAzureCredential - tries multiple auth methods automatically
az login  # Optional - only one of many auth methods
```

**Implementation note:** User info is extracted by parsing the access token JWT claims (tid, oid, upn, name, appid, azp) rather than relying on Azure CLI commands. This allows the auth panel to display user information regardless of which authentication method is used.

#### API Calls
- Always use context with timeout: `context.WithTimeout(ctx, 30*time.Second)`
- Run API calls in goroutines to keep UI responsive
- Handle errors gracefully with user-friendly messages
- Cache API versions for resource providers to avoid repeated lookups

### 6. Testing

**CRITICAL: Always add or update tests when making changes.**

When implementing features or fixes:
- **New domain models?** Add JSON serialization tests in `pkg/domain/`
- **New Azure client methods?** Add client tests in `pkg/azure/`
- **New GUI features?** Add tests in `pkg/gui/` if applicable and run the automated TUI testing with tmux
- **Bug fixes?** Add a test that would have caught the bug

Run tests frequently:
```bash
go test ./pkg/...
```

Key test files:
- `pkg/tasks/tasks_test.go` - Task manager tests
- `pkg/azure/client_test.go` - Azure client tests
- `pkg/gui/gui_test.go` - GUI tests
- `pkg/domain/domain_test.go` - Domain model tests (JSON tags, helpers)

### 6.1 Automated TUI Testing with tmux

For programmatic TUI testing using tmux, see [docs/TUI_TESTING.md](docs/TUI_TESTING.md).

**Capabilities:**
- **Text-based testing**: Capture pane content for assertions (works in any environment including CI/CD)
- **Screenshot testing**: Visual verification using `grim` (Wayland) or `import` (X11) - requires display
- **Subagent delegation**: Automated testing via tmux scripting

**Prerequisites:**
- Required: `tmux`
- For screenshots: `grim` (Wayland), `import` (ImageMagick, X11), or `scrot`
- Note: Screenshot tools require visible terminal window and active display server

### 7. Common Issues and Fixes

#### Issue: App hangs when navigating
**Cause**: Holding mutex while calling UI methods  
**Fix**: Release locks before calling any gocui methods

#### Issue: Arrow keys don't work in a panel
**Cause**: Wrong cursor calculation (mixing origin + cursor position)  
**Fix**: Use `v.SetCursor(x, y)` with simple Y coordinate

#### Issue: Active panel frame color doesn't change
**Cause**: Forgot to call `gui.updatePanelTitles()` after switching panels  
**Fix**: Always update panel frame colors when changing `activePanel`

#### Issue: Ctrl+C doesn't work
**Cause**: Keybinding not registered for current view  
**Fix**: Bind quit keys to ALL views ("", "subscriptions", "resourcegroups", "main")

### 8. Build and Run

```bash
# Build
go build .

# Run
./lazyazure

# Run with debug logging
LAZYAZURE_DEBUG=1 ./lazyazure

# Run in demo mode (mock data, no Azure credentials needed)
LAZYAZURE_DEMO=1 ./lazyazure

# Test
go test ./pkg/...
```

### 9. File Organization

```
docs/
└── TUI_TESTING.md           # TUI testing documentation with tmux
pkg/
├── azure/          # Azure SDK clients
│   ├── client.go            # Azure SDK wrapper with DefaultAzureCredential
│   ├── client_test.go       # Azure client tests
│   ├── factory.go           # Client factory for dependency injection
│   ├── subscriptions.go     # Subscription operations
│   ├── resourcegroups.go    # Resource group operations
│   ├── resourcegroups_test.go # RG tests
│   ├── resources.go         # Generic resource operations
│   └── api_versions.go      # Dynamic API version lookup
├── demo/           # Demo mode (mock Azure data)
│   ├── data.go              # Mock data structures
│   ├── client.go            # Demo client implementing AzureClient interface
│   └── client_test.go       # Demo client tests
├── domain/         # Domain models (structs)
│   ├── user.go              # User domain model
│   ├── subscription.go      # Subscription domain model
│   ├── resourcegroup.go     # ResourceGroup domain model
│   ├── resource.go          # Generic Resource domain model
│   └── domain_test.go       # Domain model tests (JSON tags, helpers)
├── resources/      # Resource type display names
│   ├── display_names.go     # Loader and fallback algorithm
│   ├── display_names.json   # Azure resource type to human-readable name mappings
│   └── display_names_test.go # Display name tests
├── gui/            # TUI implementation
│   ├── gui.go               # Main GUI controller with all TUI logic
│   ├── gui_test.go          # GUI tests
│   ├── interfaces.go        # Client interfaces for abstraction
│   └── panels/
│       └── filtered_list.go # Generic filtered list component
├── tasks/          # Async task management
│   ├── tasks.go
│   └── tasks_test.go        # Task manager tests
└── utils/          # Utilities
    ├── logger.go            # Debug logging (opt-in via LAZYAZURE_DEBUG)
    ├── clipboard.go         # Clipboard operations (cross-platform)
    ├── portal_urls.go       # Azure Portal URL generation
    └── portal_urls_test.go  # Portal URL tests
```

### 10. Code Style

- **Formatting**: Code must pass `gofmt -l .` (no output means properly formatted)
- **Logging**: Use `utils.Log()` liberally during development (disabled by default)
- **Error handling**: Return errors up the call stack, handle at boundaries
- **Naming**: Use camelCase for unexported, CamelCase for exported
- **Comments**: Document complex mutex patterns and why they're needed

### 11. UI Styling and Formatting

#### ANSI Colors in gocui
- gocui supports ANSI escape codes for colored text in **view content**
- Use 256-color palette for precise color matching: `\x1b[38;5;114m` (color 114)
- Combine with bold: `\x1b[1;38;5;114m` for bold + color
- Always reset after color: `\x1b[0m`

**Important**: gocui's `Title` field does NOT support ANSI escape codes. They will render as literal text (e.g., `[1m Title [0m`). Only use ANSI codes in view content (text written to the view), not in titles.

#### Chroma for JSON Syntax Highlighting
- Use `terminal256` formatter for full color support (not `terminal`)
- github-dark theme works well for dark terminals with green keys
- Some themes (like github-dark) need 256-color support to render properly
- Post-process output to add bold: replace `[38;5;114m` with `[1;38;5;114m`

#### Consistent Styling Between Views
- Keep colors consistent between Summary and JSON tabs
- Use same color codes in both manual formatting (Summary) and Chroma (JSON)
- Test both views side-by-side to ensure visual consistency

#### Formatting Nested Data
- Don't use `fmt.Sprintf("%v", value)` for maps/arrays (shows ugly Go syntax)
- Implement recursive formatting for nested structures
- Use indentation to show hierarchy
- Example: `formatPropertyValue()` for maps with nested key-value pairs

#### Sorting for Consistent UI
- Map iteration order is random in Go
- Sort keys alphabetically for consistent display: `sort.Strings(keys)`
- Apply to tags, properties, or any map data shown in UI
- Prevents "shuffle" effect when navigating between items

#### Display Pattern for List Items
Domain models that appear in sidebar lists follow a consistent pattern:
- `DisplayString()` returns the primary name (plain text, no ANSI codes)
- `GetDisplaySuffix()` returns additional info to display in gray
- GUI calls `formatWithGraySuffix(name, suffix)` to apply gray formatting

Example:
```go
// In domain model
func (r *Resource) DisplayString() string { return r.Name }
func (r *Resource) GetDisplaySuffix() string { return resources.GetResourceTypeDisplayName(r.Type) }

// In GUI
fmt.Fprintln(view, formatWithGraySuffix(res.DisplayString(), res.GetDisplaySuffix()))
// Output: "my-vm (Virtual Machine)" with type in gray
```

### Resource Type Display Names

Azure resource types (e.g., "Microsoft.Compute/virtualMachines") are mapped to human-readable names (e.g., "Virtual Machine") for the UI.

#### Implementation (`pkg/resources/`)

**Core Mapping** (`display_names.json`):
- JSON file with 75+ resource type mappings
- Manually curated for most common Azure resource types
- Easy to extend via PRs

**Lookup Strategy** (`display_names.go`):
1. Exact match in core mapping
2. Case-insensitive match (Azure returns lowercase from list API)
3. Algorithmic fallback:
   - Multi-word resource names: Convert camelCase to spaces + singularize
   - Single word: Use provider name + resource name
   - Known acronyms: IP, SQL, NSG, AKS, etc.
   - Plural handling: services→service, addresses→address, machines→machine

**Adding New Mappings**:
1. Add entry to `display_names.json` following existing format
2. Add test case in `display_names_test.go`
3. Test both exact and case-insensitive lookups

## Session Checklist

**CRITICAL: Complete this checklist BEFORE committing any changes.**

Before finishing a session or committing changes:

- [ ] Code builds without errors: `go build .`
- [ ] Tests have been updated or added per the guideline in this file
- [ ] Tests pass: `go test ./pkg/...`
- [ ] Code is properly formatted: `gofmt -l .` returns empty
- [ ] Modules are tidy: `go mod tidy`
- [ ] Debug logging is properly guarded with `LAZYAZURE_DEBUG` check
- [ ] No mutex deadlocks introduced (verify lock patterns)
- [ ] **Documentation updated**:
  - [ ] AGENTS.md - File organization section, relevant guidelines, and checklist updated
  - [ ] New features/patterns documented in appropriate sections
- [ ] **File organization documented**:
  - [ ] New packages added to AGENTS.md section 9 (File Organization)
  - [ ] Any new patterns or conventions documented

**Do not commit until all checklist items are verified!**

## Key Lessons from Development

1. **Terminal UI is hard**: Threading + UI event loops require careful coordination
2. **Mutexes are tricky**: Deadlocks happen easily when mixing UI and data operations
3. **Test in real terminal**: IDE consoles don't work properly for TUI apps
4. **Ghostty**: Preferred terminal for testing
5. **Debug logs are essential**: But must be opt-in for production

## Terminal Requirements

### Required Terminal Features

The application requires terminals that support:
- **Unicode box-drawing characters**: `┌─┐│└┘` for panel borders
- **256-color ANSI support**: For green color-coded keys (ANSI 256-color code 114) and JSON syntax highlighting
- **ANSI escape sequences**: For bold text and color resets (`\x1b[0m`)

### Platform-Specific Considerations

| Feature | Linux | macOS | Windows |
|---------|-------|-------|---------|
| Unicode | Full | Full | Windows Terminal only |
| 256-color | Full | Full | Windows Terminal only |
| Clipboard | Needs xclip/xsel/wl-copy | Native | Native |
| Recommended Terminal | Ghostty, Alacritty, Kitty | iTerm2, Ghostty, Terminal.app | Windows Terminal |

### IDE Consoles

**IMPORTANT**: IDE built-in terminals (VS Code, JetBrains, etc.) may not render TUI applications correctly:
- May not process arrow keys properly
- May display box-drawing characters incorrectly
- May not support all ANSI escape sequences

Always test in a standalone terminal application.

## Questions to Ask User

When uncertain about:
- UI behavior or appearance preferences
- Feature prioritization
- Architecture decisions
- Breaking changes

Always default to asking rather than assuming!
