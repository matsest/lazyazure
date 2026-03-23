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
The UI uses a fixed layout:
```
┌─────────────────────┬──────────────────────────────────┐
│ Auth (3 lines)      │  Details Panel                   │
├─────────────────────┤                                  │
│ Subscriptions (40%)  │  Shows selected item details      │
│                     │                                  │
├─────────────────────┤                                  │
│ Resource Groups      │                                  │
│ (remaining space)   │                                  │
└─────────────────────┴──────────────────────────────────┘
[Status Bar: context-aware help text                     ]
```

#### Panel Focus
- Use `activePanel` field to track which panel has focus
- Visual indicator: `▶` arrow in panel title
- Switch panels with `Tab` key
- Each panel has independent navigation keybindings

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
- **New GUI features?** Add tests in `pkg/gui/` (or at minimum, test manually)
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

### 7. Common Issues and Fixes

#### Issue: App hangs when navigating
**Cause**: Holding mutex while calling UI methods  
**Fix**: Release locks before calling any gocui methods

#### Issue: Arrow keys don't work in a panel
**Cause**: Wrong cursor calculation (mixing origin + cursor position)  
**Fix**: Use `v.SetCursor(x, y)` with simple Y coordinate

#### Issue: Visual indicator doesn't move
**Cause**: Forgot to call `gui.updatePanelTitles()` after switching panels  
**Fix**: Always update titles when changing `activePanel`

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

# Test
go test ./pkg/...
```

### 9. File Organization

```
pkg/
├── azure/          # Azure SDK clients
│   ├── client.go
│   ├── subscriptions.go
│   └── resourcegroups.go
├── domain/         # Domain models (structs)
│   ├── user.go
│   ├── subscription.go
│   └── resourcegroup.go
├── gui/            # TUI implementation
│   ├── gui.go      # Main GUI controller
│   └── panels/
│       └── filtered_list.go
├── tasks/          # Async task management
│   └── tasks.go
└── utils/          # Utilities
    └── logger.go   # Debug logging
```

### 10. Code Style

- **Formatting**: Code must pass `gofmt -l .` (no output means properly formatted)
- **Logging**: Use `utils.Log()` liberally during development (disabled by default)
- **Error handling**: Return errors up the call stack, handle at boundaries
- **Naming**: Use camelCase for unexported, CamelCase for exported
- **Comments**: Document complex mutex patterns and why they're needed

## Session Checklist

Before finishing a session:

- [ ] Code builds without errors: `go build .`
- [ ] Tests pass: `go test ./pkg/...`
- [ ] Code is properly formatted: `gofmt -l .` returns empty
- [ ] Debug logging is properly guarded with `LAZYAZURE_DEBUG` check
- [ ] No mutex deadlocks introduced (verify lock patterns)
- [ ] Documentation updated if needed (README.md, this file)

## Key Lessons from Development

1. **Terminal UI is hard**: Threading + UI event loops require careful coordination
2. **Mutexes are tricky**: Deadlocks happen easily when mixing UI and data operations
3. **Test in real terminal**: IDE consoles don't work properly for TUI apps
4. **Ghostty**: Preferred terminal for testing
5. **Debug logs are essential**: But must be opt-in for production

### 11. UI Styling and Formatting

#### ANSI Colors in gocui
- gocui supports ANSI escape codes for colored text in views
- Use 256-color palette for precise color matching: `\x1b[38;5;114m` (color 114)
- Combine with bold: `\x1b[1;38;5;114m` for bold + color
- Always reset after color: `\x1b[0m`

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

## Questions to Ask User

When uncertain about:
- UI behavior or appearance preferences
- Feature prioritization
- Architecture decisions
- Breaking changes

Always default to asking rather than assuming!
