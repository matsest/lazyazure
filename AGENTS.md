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

**Debug Logging Privacy:**

When `LAZYAZURE_DEBUG=1` is enabled, logs are written to `~/.lazyazure/debug.log`. 
The logs are designed to be safe to share for debugging while protecting sensitive information:

**What IS logged:**
- Application flow and lifecycle events (initialization, view setup, etc.)
- UI interactions (panel switches, keybindings, clicks)
- Performance metrics (operation timing, result counts)
- Error messages and types
- Authentication success/failure (without identity details)
- Search activity (character counts, not content)

**What is NOT logged:**
- User Principal Names (UPN/email addresses)
- Display names
- Tenant IDs or Object IDs
- Resource IDs
- Search query content
- Full JWT tokens or credentials

The goal is to provide enough context to diagnose issues without exposing 
personally identifiable information or organizational data.

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
- Switch panels with `Tab` key or **mouse click**
- Click a list item to select it and trigger the Enter action (loads next panel)
- Click Summary/JSON tabs in the main panel to switch views
- Each panel has independent navigation keybindings

#### Mouse Support
- Mouse support is enabled via `gui.g.Mouse = true`
- List panels use standard keybindings with `gocui.MouseLeft`
- Main panel uses `SetViewClickBinding` to detect tab clicks via `GetClickedTabIndex`
- Clicking sets focus and triggers appropriate actions:
  - **Subscriptions/Resource Groups/Resources**: Selects item + Enter action
  - **Main panel tabs**: Switches between Summary/JSON views

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
- **New or changed domain models?** Add JSON serialization tests in `pkg/domain/`
- **New or changed Azure client methods?** Add client tests in `pkg/azure/`
- **New or changed GUI features?** Add tests in `pkg/gui/` if applicable and run the automated TUI testing with tmux
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
**Fix**: Bind quit keys to ALL views ("", "subscriptions", "resourcegroups", "main", "search")

#### Issue: Search hangs when typing
**Cause**: UI updates from keybinding handler without using UpdateAsync  
**Fix**: Wrap all UI updates in `gui.g.UpdateAsync()` when triggered from callbacks

#### Issue: Search causes deadlock
**Cause**: Callback holding lock while triggering UI updates that need the same lock  
**Fix**: Release lock before calling callbacks that may trigger UI updates:
```go
// ❌ WRONG: Holding lock during callback
mu.Lock()
defer mu.Unlock()
updateView()
onSearch(text)  // May trigger UI updates = DEADLOCK!

// ✅ CORRECT: Release lock first
mu.Lock()
updateView()
text := searchText  // Copy value
mu.Unlock()
onSearch(text)  // Safe - no lock held
```

#### Issue: Timer callback causes deadlock in version display
**Cause**: Timer callback holding lock while calling updateStatus() which also needs lock  
**Fix**: Release lock before calling updateStatus():
```go
// ❌ WRONG: Holding lock while calling function that needs lock
func (gui *Gui) clearVersionDisplay() {
    gui.mu.Lock()
    defer gui.mu.Unlock()
    gui.showingVersion = false
    gui.updateStatus()  // DEADLOCK - tries to acquire same lock!
}

// ✅ CORRECT: Release lock before calling updateStatus()
func (gui *Gui) clearVersionDisplay() {
    gui.mu.Lock()
    gui.showingVersion = false
    gui.mu.Unlock()
    gui.updateStatus()  // Safe - lock released
}
```

### 8. Build and Run

```bash
# Build
# with version information (default)
make build

# without version information
go build .

# Run
./lazyazure

# Run with debug logging
LAZYAZURE_DEBUG=1 ./lazyazure

# Run in demo mode (mock data, no Azure credentials needed)
LAZYAZURE_DEMO=1 ./lazyazure

# Check version
./lazyazure --version
# or
./lazyazure -v

# Check for updates
./lazyazure --check-update

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
│       ├── filtered_list.go      # Generic filtered list component
│       ├── filtered_list_test.go # Filtered list tests
│       ├── search_bar.go         # Search bar UI component
│       ├── search_bar_test.go    # Search bar tests
│       ├── main_panel_search.go  # Main panel search (highlighting)
│       └── main_panel_search_test.go # Main panel search tests
├── tasks/          # Async task management
│   ├── tasks.go
│   └── tasks_test.go        # Task manager tests
└── utils/          # Utilities
    ├── logger.go            # Debug logging (opt-in via LAZYAZURE_DEBUG)
    ├── clipboard.go         # Clipboard operations (cross-platform)
    ├── browser.go           # Browser opening operations (cross-platform)
    ├── browser_test.go      # Browser utility tests
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

### Search Implementation

The search feature uses a two-component architecture:

#### 1. FilteredList (`pkg/gui/panels/filtered_list.go`)
- Generic list with filtering capability using Go generics
- Stores both items AND their display strings (what the user sees)
- Case-insensitive substring matching on display strings
- Thread-safe with RWMutex for concurrent access
- Key methods:
  - `SetItems(items, getDisplay)` - Initialize with display function
  - `SetFilter(text)` - Apply filter
  - `ClearFilter()` - Remove filter
  - `GetFilteredDisplayStrings()` - Get filtered results for UI

#### 2. SearchBar (`pkg/gui/panels/search_bar.go`)
- UI component at bottom of screen for text input
- Handles character input, backspace, Ctrl+U (clear), Ctrl+W (delete word)
- Uses gocui Editor interface for key handling
- Thread-safe with mutex protection
- Triggers callback on every text change for real-time filtering

#### Integration Pattern
```go
// In GUI setup
subList := panels.NewFilteredList[*domain.Subscription]()
searchBar := panels.NewSearchBar(g, onSearchChanged, onSearchCancel, onSearchConfirm)

// When data loads
subList.SetItems(subs, func(sub *domain.Subscription) string {
    return formatWithGraySuffix(sub.DisplayString(), sub.GetDisplaySuffix())
})

// Display in panel
for _, display := range subList.GetFilteredDisplayStrings() {
    fmt.Fprintln(view, display)
}
```

#### Search Keybindings
- `/` - Activate search for current panel
- `a-z`, `0-9`, special chars - Type in search
- `Backspace` - Delete last character
- `Ctrl+U` - Clear entire search
- `Ctrl+W` - Delete last word
- `Enter` - Confirm and exit search mode
- `Escape` - Cancel and clear filter

### Main Panel Search

The main/details panel (right side) has a different search mode that **highlights** matching lines instead of filtering items.

#### Implementation (`pkg/gui/panels/main_panel_search.go`)

**Key differences from list panel search:**
- Highlights matching lines with light grey background (ANSI 250) for other matches
- Current match highlighted with yellow background (ANSI 226) for visibility
- Shows all content, just highlights matches
- Supports navigation between matches with `n`/`N` keys
- Clears search when switching to a different resource

**Usage pattern:**
```go
// When rendering content to main panel
lines := gui.buildResourceSummaryLines(resource)
gui.mainPanelSearch.SetContent(lines)

// Render with or without highlights
if gui.mainPanelSearch.IsActive() {
    highlightedLines := gui.mainPanelSearch.GetHighlightedContent()
    // render highlighted lines
} else {
    // render normal lines
}
```

**Important considerations:**
- Store content lines before applying highlights
- Search is case-insensitive
- JSON content has existing ANSI codes from Chroma - search works on the visible text
- Other matches get wrapped with `\x1b[48;5;250m` (light grey bg)
- Current match gets wrapped with `\x1b[48;5;226m` (yellow bg)
- Both use `\x1b[0m` (reset)
- Use `NextMatch()`/`PrevMatch()` to navigate and scroll view to match position

#### Main Panel Search Keybindings (when in main panel)
- `/` - Start search
- `n` - Jump to next match
- `N` - Jump to previous match
- `Enter` - Confirm and exit search input mode
- `Escape` - Clear search and remove highlights

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

### Version Display and Update Checking

#### TUI Version Display (`?` keybinding)
- Press `?` in any view to show version information in the status bar
- Displays: current version, commit hash, and update status
- Fetches latest version from GitHub releases API (cached for session)
- Auto-dismisses after 5 seconds or press Escape to clear
- Development builds (dev, dirty, ahead-of-tag) show "Development build" message

#### CLI Update Checking (`--check-update` flag)
- `./lazyazure --check-update` checks for updates non-interactively
- Exit codes: 0=up to date/dev, 1=update available, 2=error
- Development builds skip version comparison but still show latest version

#### Implementation Notes
- GitHub API: `https://api.github.com/repos/matsest/lazyazure/releases/latest`
- 10-second timeout on HTTP requests
- Development build detection uses git describe patterns:
  - `dev` = plain go build
  - `-dirty` = uncommitted changes
  - `-g[hex]` = commits ahead of tag (e.g., `v0.2.1-2-gc15ffdf`)

## Session Checklist

**CRITICAL: Complete this checklist BEFORE committing any changes.**

## Third-Party Dependencies and License Compliance

When adding new third-party dependencies to the project:

### Required Actions
1. **Add dependency to go.mod**: `go get <package>`
2. **Verify license compatibility**: Ensure the license is compatible with this project's license
3. **Update THIRD-PARTY-NOTICES.txt**: Add the complete license text for the new dependency
   - **MIT**: Include full permission notice with copyright
   - **BSD 2/3-Clause**: Include full text with conditions and disclaimer
   - **Apache 2.0**: Include full license text
   - **Other licenses**: Include full license text as required by the license terms

### Format in THIRD-PARTY-NOTICES.txt
Follow the existing format:
```
--------------------------------------------------------------------------------
<Package Name> (<repository URL>)
License: <License Name>
Copyright (c) <Year> <Author/Company>
--------------------------------------------------------------------------------

<Full license text here>
```

### Important Notes
- **Full license text required**: Just listing the package name and license type is NOT sufficient for binary distribution compliance
- **Transitive dependencies**: Only need to list URLs; full texts not required unless they are direct dependencies
- **When in doubt**: Include the full license text - it's always safer

Before finishing a session or committing changes:

- [ ] Code builds without errors: `go build .`
- [ ] Tests have been updated or added per the guideline in this file
- [ ] Tests pass: `go test ./pkg/...`
- [ ] Code is properly formatted: `gofmt -l .` returns empty
- [ ] Modules are tidy: `go mod tidy`
- [ ] Debug logging is properly guarded with `LAZYAZURE_DEBUG` check
- [ ] No mutex deadlocks introduced (verify lock patterns)
- [ ] **Third-party dependencies updated**:
  - [ ] THIRD-PARTY-NOTICES.txt updated with full license text for any new dependencies
  - [ ] License compatibility verified for new dependencies
- [ ] **Documentation updated**:
  - [ ] AGENTS.md - File organization section, relevant guidelines, and checklist updated
  - [ ] New features/patterns documented in appropriate sections
- [ ] **File organization documented**:
  - [ ] New packages added to AGENTS.md section 9 (File Organization)
  - [ ] Any new patterns or conventions documented

**Do not commit until all checklist items are verified!**

Always use conventional commit style commit message headers (check latest commits if in doubt).

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
