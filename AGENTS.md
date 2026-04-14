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
- Resource IDs (full Azure resource IDs, subscription IDs, resource group IDs)
- Search query content
- Full JWT tokens or credentials

**Safe alternatives for debugging:**
- Use presence indicators: `hasSub := savedSubID != ""` then log `hasSub=true/false`
- Log indices/positions instead of IDs: `found at index 5` instead of `found ID xyz`
- Log counts and lengths: `loaded 20 subscriptions` instead of listing them
- Use anonymized identifiers: `resource-1`, `rg-A` if item identification is needed

The goal is to provide enough context to diagnose issues without exposing 
personally identifiable information or organizational data.

### 3.1 Performance Metrics

The application includes comprehensive performance metrics for validating optimizations:

**Metrics Tracked:**
- **Cache hits/misses**: Track cache efficiency with hit rate percentage
- **API call timing**: Average duration of Azure API calls
- **Cache size**: Current and maximum sizes for all cache tiers
- **Evictions**: Count of cache evictions due to size limits

**Implementation:**
```go
import "github.com/matsest/lazyazure/pkg/utils"

// Record cache hit/miss (automatic in PreloadCache)
utils.GetMetrics().RecordCacheHit()
utils.GetMetrics().RecordCacheMiss()

// Record API call timing
record := utils.StartAPITimer("ListSubscriptions")
defer record(err)  // Records duration and error status

// Get current stats
stats := utils.GetMetrics().GetStats()
fmt.Printf("Hit rate: %.1f%%\n", stats.CacheHitRate)

// Log metrics to debug output
utils.LogMetrics()
```

**Viewing Metrics:**
When `LAZYAZURE_DEBUG=1` is enabled, metrics are automatically logged:
```
[2026-04-02 10:15:30.123] [METRICS] Cache: 150 hits, 50 misses (75.0% hit rate), 10 evictions | API: 25 calls, 0 errors, avg 145ms | Size: RG=10/20, Res=50/100, FullRes=25/50
```

The metrics are stored in a global singleton (`utils.GetMetrics()`) and updated automatically by:
- `PreloadCache` for cache operations (hit/miss, size, evictions)
- Azure client methods for API call timing (via `StartAPITimer`)

### 3.2 Timing and Layout Constants

Application-wide constants are centralized in `pkg/gui/gui.go` for maintainability:

**Timing Constants:**
```go
const (
    APITimeout             = 30 * time.Second  // Azure API calls
    ShortAPITimeout        = 10 * time.Second  // Quick operations
    VersionCheckTimeout    = 10 * time.Second  // GitHub API version check
    HTTPClientTimeout      = 10 * time.Second  // General HTTP requests
    VersionDisplayDuration = 5 * time.Second   // How long version info shows
    StatusMessageDuration  = 2 * time.Second   // Status bar message duration
)
```

**Layout Constants:**
```go
const (
    AuthViewHeight = 5  // Lines reserved for auth panel
)
```

**Concurrent Operation Limits:**
```go
const (
    MaxConcurrentPreloads   = 50            // Background preload goroutines
    SemaphoreWaitThreshold  = 100 * time.Millisecond  // Log warning threshold
)
```

**Benefits:**
- Single location to adjust timeouts and limits
- Self-documenting code (e.g., `APITimeout` vs `30 * time.Second`)
- Easier to write tests with different values
- Prevents magic number proliferation

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

#### Cursor Position Preservation
When pressing `r` to refresh data, cursor positions are preserved:
- Before refresh: Save the IDs of currently selected items (`selectedSub.ID`, `selectedRG.ID`, `selectedRes.ID`)
- After data loads: Use `FindIndex()` to locate items with matching IDs in filtered lists
- Restore cursor position using `SetCursor(0, targetIndex)`
- Update selection pointers to reference items from new data

Implementation: See `loadSubscriptionsWithSelection()`, `loadResourceGroupsWithSelection()`, and `loadResourcesWithSelection()` functions

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

#### API Version Caching

LazyAzure uses a two-tier caching strategy for Azure API versions to minimize API calls:

**Tier 1: Pre-loaded Curated Cache** (`pkg/azure/api_versions_curated.json`)
- 126 curated resource types covering all major Azure services
- Embedded in binary via Go embed
- Loaded at startup via `init()` - zero Azure API calls for these types
- Covers the most commonly used resource types

**Tier 2: Runtime Cache** (global singleton)
- For unknown resource types, fetches from Azure once
- Stored in thread-safe `globalAPICache` with RWMutex
- Persists across all `APIVersionCache` instances
- Subsequent lookups use cached value

**Updating Curated API Versions:**
```bash
# Update from bicep-types-az GitHub repo (downloads ~5MB index.json)
make update-api-versions

# Review changes and commit
# The tool extracts from https://github.com/Azure/bicep-types-az
```

**How it works:**
1. App starts → loads curated versions into cache (<1ms overhead)
2. User views resource → cache hit for known types (zero Azure calls)
3. Unknown resource type → fetches from Azure API → caches for future

**Performance impact:**
- First view of common resource: **100-200ms faster** (no API version lookup)
- Unknown resource types: One-time penalty, then cached
- Memory: ~15KB for curated cache, grows with runtime cache

### 5.1 Background Preloading Cache

LazyAzure implements a three-tier cache system to reduce perceived loading time:

**Cache Architecture:**
```
Subscription selected
    ↓
[Background] Load resource groups → Cache (5 min TTL)
    ↓
[Background] Load resources for top 10 RGs → Cache (3 min TTL)
    ↓
User selects RG → Instant display (cached)
    ↓
User presses Enter on resource → Fetch full details → Cache (3 min TTL)
    ↓
Next time: Instant full details (cached)
```

**Three Cache Tiers:**

1. **Resource Group Lists**
   - Max 100 entries, 5-minute TTL
   - Stores list of RGs per subscription
   - Preloaded when subscription selected

2. **Resource Lists**
   - Max 500 entries, 3-minute TTL
   - Stores list of resources per RG
   - Preloaded for top 10 RGs

3. **Full Resource Details**
   - Max 500 entries, 3-minute TTL
   - Stores complete resource with properties
   - Cached on-demand when user views resource

**Eviction:** When any cache tier is full, remove oldest 50%
**Memory:** ~20-40 MB max

**Cache Invalidation on Refresh ('r' key):**
- Subscriptions panel refresh → Clear all caches
- Resource Groups panel refresh → Clear RG + resource caches for current subscription
- Resources panel refresh → Clear only current RG's resource cache

**Key Methods:**
- `PreloadCache.GetRGs(subID)` - Check RG cache before loading
- `PreloadCache.GetRes(subID, rgName)` - Check resource list cache
- `PreloadCache.GetFullRes(resourceID)` - Check full details cache
- `PreloadCache.InvalidateSubs/RGs/Res()` - Scope-based invalidation
- `preloadResourceGroups()` - Background loader for RGs
- `preloadTopResources()` - Background loader for top 10 RG resources

**Design Principles:**
- Preload is **completely silent** - no UI updates, no loading indicators
- User experience unchanged if preload fails
- Context cancellation prevents wasted work when user navigates away
- Duplicate prevention via loading flags
- Privacy-safe logging (no IDs or names in debug logs)
- Cache lookups in `onSubEnter` and `onRGEnter` for instant display

### 5.2 Concurrent Preloading Limit

To prevent excessive goroutine spawning and Azure API rate limiting, background preloading uses a semaphore to limit concurrent operations.

**Limit:** 50 concurrent background operations (configurable via `MaxConcurrentPreloads` constant)

**Semaphore Implementation:**
The semaphore uses a channel as the single source of truth for simplicity and correctness:

```go
type Semaphore struct {
    ch chan struct{}  // Buffered channel acts as slots
}

func (s *Semaphore) Acquire(ctx context.Context) error {
    select {
    case s.ch <- struct{}{}:  // Blocks when at capacity
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (s *Semaphore) Release() {
    <-s.ch  // Free up a slot
}
```

**How it works:**
1. Channel is buffered with capacity equal to `MaxConcurrentPreloads`
2. Each `Acquire()` sends to the channel - blocks when full (natural backpressure)
3. Channel length gives accurate in-use count (`len(s.ch)`)
4. `Release()` receives from channel to free a slot
5. No separate mutex or counter needed - channel provides thread-safety

**Why this design:**
- **Simple**: Single source of truth (the channel)
- **Race-free**: Channel operations are atomic
- **Context-aware**: Respects cancellation while waiting
- **No deadlocks**: No lock hierarchy to violate

**User Impact:**
- First 5 subscriptions clicked (50 RGs total): All preload in parallel, no waiting
- 6th+ subscription: Resource preloads queue briefly until slots free up
- RG list loading: Never blocked (foreground operation)
- Memory usage: Bounded at ~8-15MB regardless of subscription count

**Monitoring:**
When `LAZYAZURE_DEBUG=1`, logs show:
```
preloadResourceGroups: Waited 250ms for semaphore slot (47/50 in use)
preloadTopResources: Waited 180ms for semaphore slot (RG #5, 49/50 in use)
```

**Key Methods:**
- `Semaphore.Acquire(ctx)` - Acquire slot or wait (context-aware)
- `Semaphore.Release()` - Release slot
- `Semaphore.GetUtilization()` - Get current usage stats (inUse, capacity)

### 5.3 Cache Sizing

Cache limits are configurable via environment variable. Defaults to medium which is suitable for most users.

**Environment Variable:** `LAZYAZURE_CACHE_SIZE`

| Value  | RG Cache | Resource Cache | Full Resource Cache | Approx Memory | Best For |
|--------|----------|----------------|---------------------|---------------|----------|
| small  | 100      | 500            | 500                 | ~20-40 MB     | Low memory environments (<4GB RAM) |
| medium | 300      | 1,500          | 1,500               | ~60-120 MB    | **Default** - Most users |
| large  | 600      | 3,000          | 3,000               | ~120-240 MB   | 100+ subscriptions |

**Examples:**
```bash
# Default (medium)
./lazyazure

# Force specific size
LAZYAZURE_CACHE_SIZE=small ./lazyazure    # 100/500 cache
LAZYAZURE_CACHE_SIZE=medium ./lazyazure   # 300/1500 cache  
LAZYAZURE_CACHE_SIZE=large ./lazyazure    # 600/3000 cache
```

**Implementation:**
- `GetCacheConfig()` - Parses environment and returns `CacheConfig`
- `NewPreloadCache()` - Uses environment-based config automatically (defaults to medium)
- `NewPreloadCacheWithConfig()` - Allows explicit configuration for testing

### 5.x gocui Local Patch Workaround

LazyAzure uses a local patched version of gocui to fix upstream bugs (e.g., MouseRight/MouseMiddle not working in v0.3.1+).

**Structure:**
```
vendor_gocui/                  # Local copy of gocui with patches applied
scripts/gocui-mouse-right.patch  # Standalone patch file for the fix
```

**go.mod uses replace directive:**
```
replace github.com/jesseduffield/gocui => ./vendor_gocui
```

**To update when upstream releases a fixed version:**
```bash
# 1. Copy new upstream gocui version
cp -r /path/to/new/gocui/* vendor_gocui/

# 2. Reapply patch
cd vendor_gocui && patch -p1 < ../scripts/gocui-mouse-right.patch

# 3. Test, then update go.mod to new version and remove replace directive
```

**Current patches applied:**
- MouseRight/MouseMiddle click events now work correctly (were silently discarded)

### 6. Testing

**CRITICAL: Always add or update tests when making changes.**

When implementing features or fixes:
- **New or changed domain models?** Add JSON serialization tests in `pkg/domain/`
- **New or changed Azure client methods?** Add client tests in `pkg/azure/`
- **New or changed GUI features?** You MUST use automated TUI testing with tmux - see section 6.1 below
- **Bug fixes?** Add a test that would have caught the bug

Run tests frequently:
```bash
make test
```

Key test files:
- `pkg/tasks/tasks_test.go` - Task manager tests
- `pkg/azure/client_test.go` - Azure client tests
- `pkg/gui/gui_test.go` - GUI tests
- `pkg/domain/domain_test.go` - Domain model tests (JSON tags, helpers)

### 6.1 Automated TUI Testing with tmux (REQUIRED for GUI changes)

**For any GUI changes (navigation, scrolling, panels, keybindings), you MUST use automated TUI testing with tmux.**

Do NOT attempt to test TUI applications by running them directly - they require a TTY which is not available in this environment. Instead, use tmux as documented in [docs/TUI_TESTING.md](docs/TUI_TESTING.md).

**Required for GUI changes:**
1. Create or update a test script in `scripts/test-*.sh`
2. Use `tmux` to programmatically control the application
3. Send keystrokes and capture pane content to verify behavior
4. Run the test to verify the fix works

**Example workflow for GUI bug fixes:**
```bash
# 1. Create or update a test script in scripts/test-*.sh
# 2. Make the script executable
chmod +x scripts/test-<feature>.sh

# 3. Run the test
./scripts/test-<feature>.sh
```

**Available test scripts:**
- `scripts/test-scrolling.sh` - Tests scrolling functionality in list panels
- `scripts/test-search.sh` - Tests search/filter functionality
- `scripts/test-panel-switch.sh` - Tests Tab/Shift+Tab panel navigation

**Capabilities:**
- **Text-based testing**: Capture pane content for assertions (works in any environment including CI/CD)
- **Screenshot testing**: Visual verification using `grim` (Wayland) or `import` (X11) - requires display
- **Subagent delegation**: Automated testing via tmux scripting

**Prerequisites:**
- Required: `tmux`
- For screenshots: `grim` (Wayland), `import` (ImageMagick, X11), or `scrot`
- Note: Screenshot tools require visible terminal window and active display server

**Demo mode for testing:**
Always use `LAZYAZURE_DEMO=1` (small dataset) or `LAZYAZURE_DEMO=2` (large dataset) for testing to avoid Azure credential requirements:
```bash
# Small dataset: 2 subs, 4 RGs each, 2-4 resources
LAZYAZURE_DEMO=1 ./lazyazure

# Large dataset: 15 subs, 20 RGs each, 15 resources (4,500 total) - good for testing scrolling
LAZYAZURE_DEMO=2 ./lazyazure
```

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

# Run
./lazyazure

# Run with debug logging
LAZYAZURE_DEBUG=1 ./lazyazure

# Run in demo mode (mock data, no Azure credentials needed)
LAZYAZURE_DEMO=1 ./lazyazure  # Small dataset: 2 subs, 4 RGs each, 2-4 resources
LAZYAZURE_DEMO=2 ./lazyazure  # Large dataset: 15 subs, 20 RGs each, 15 resources (4,500 total)

# Show help
./lazyazure --help
# or
./lazyazure -h

# Check version
./lazyazure --version
# or
./lazyazure -v

# Check for updates
./lazyazure --check-update

# Test
make test

# Update curated API versions from bicep-types-az
make update-api-versions
```

#### CLI Testability

The `main.go` file has been refactored for testability with extracted functions:
- `parseArgs(args []string) (CLIArgs, error)` - Parses command-line arguments
- `printVersion(version, commit string)` - Prints version information
- `printHelp()` - Prints help message with environment variable documentation
- `checkForUpdates(version string, client *http.Client) (bool, string, error)` - Checks GitHub releases

These functions can be tested without running the full TUI application. See `main_test.go` for examples.

### 9. File Organization

```
main.go                      # Application entry point (refactored for testability)
main_test.go                 # CLI tests (argument parsing, version checking)
docs/
└── TUI_TESTING.md           # TUI testing documentation with tmux
pkg/
├── azure/          # Azure SDK clients
│   ├── client.go            # Azure SDK wrapper with DefaultAzureCredential
│   ├── client_test.go       # Azure client tests
│   ├── factory.go           # Client factory for dependency injection
│   ├── subscriptions.go     # Subscription operations
│   ├── subscriptions_test.go # Subscription client tests
│   ├── resourcegroups.go    # Resource group operations
│   ├── resourcegroups_test.go # RG tests
│   ├── resources.go         # Generic resource operations
│   ├── resources_test.go    # Resource client tests
│   ├── resourcegraph.go     # Azure Resource Graph client for cross-subscription queries
│   ├── api_versions.go      # Dynamic API version lookup with caching
│   ├── api_versions_curated.json # Pre-loaded API versions for common types
│   └── api_versions_test.go # API version cache tests
├── demo/           # Demo mode (mock Azure data)
│   ├── data.go              # Mock data structures (LAZYAZURE_DEMO=1 and LAZYAZURE_DEMO=2)
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
│   ├── cache.go             # Background preloading cache with dynamic sizing
│   ├── cache_test.go        # Cache tests
│   └── panels/
│       ├── filtered_list.go      # Generic filtered list component
│       ├── filtered_list_test.go # Filtered list tests
│       ├── search_bar.go         # Search bar UI component
│       ├── search_bar_test.go    # Search bar tests
│       ├── type_picker.go        # Type filter modal with fuzzy search
│       ├── main_panel_search.go  # Main panel search (highlighting)
│       └── main_panel_search_test.go # Main panel search tests
├── tasks/          # Async task management
│   ├── tasks.go
│   └── tasks_test.go        # Task manager tests
└── utils/          # Utilities
    ├── logger.go            # Debug logging (opt-in via LAZYAZURE_DEBUG)
    ├── logger_test.go       # Logger tests with privacy protection
    ├── metrics.go           # Performance metrics tracking (cache hits, API timing)
    ├── metrics_test.go      # Metrics tests
    ├── clipboard.go         # Clipboard operations (cross-platform)
    ├── browser.go           # Browser opening operations (cross-platform)
    ├── browser_test.go      # Browser utility tests
    ├── portal_urls.go       # Azure Portal URL generation
    └── portal_urls_test.go  # Portal URL tests
scripts/            # TUI integration test scripts (tmux-based)
├── test-scrolling.sh      # Scrolling functionality tests
├── test-search.sh         # Search functionality tests
└── test-panel-switch.sh   # Panel switching tests (Tab/Shift+Tab)
tools/
└── update-api-versions/
    └── main.go              # Tool to extract API versions from bicep-types-az
.opencode/
└── skills/
    └── release-summary/
        └── SKILL.md         # Release summary skill for agents
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

### Type Picker (Resource Type Filtering)

The type picker allows filtering resources by type using a modal with fuzzy search.

#### Implementation (`pkg/gui/panels/type_picker.go`)

**Architecture:**
- Modal overlay that appears on top of the resources panel
- Uses `display_names.json` for the list of known resource types
- Fuzzy search filters types as user types
- First option is always `[Clear filter]` to remove the active filter

**Key Features:**
- Case-insensitive fuzzy matching (matches anywhere in type name)
- Shows both display name and Azure type (e.g., "Virtual Machine (Microsoft.Compute/virtualMachines)")
- Scrollable list with cursor navigation
- Server-side filtering via Azure Resource Graph when querying

**Keybindings:**
- `T` (in subscriptions or resources panel) - Open type picker
- `↑/↓` or `j/k` - Navigate list
- Characters - Fuzzy search filter
- `Backspace` - Delete search character
- `Enter` - Select type and apply filter
- `Escape` - Cancel without changing filter

**Note:** Setting a type filter in the subscriptions panel persists to global search (`G`) and subscription view (`A`). The filter is applied server-side via Azure Resource Graph for these cross-subscription queries.

**Integration:**
```go
// In GUI
gui.typePicker = panels.NewTypePicker(g, gui.onTypeSelected, gui.onTypePickerCancel)

// Show picker
gui.typePicker.Show()

// Apply filter in onTypeSelected callback
gui.activeTypeFilter = selectedType
gui.applyTypeFilter()
```

### Subscription-Level Resource Viewing

Allows viewing all resources in a subscription without selecting a specific resource group.

**Activation:**
- Press `A` in the subscriptions panel to view all resources in the selected subscription
- Uses Azure Resource Graph for efficient cross-RG queries

**Display Format:**
- Resources show: `name (rg-name / Type)`
- Panel title shows: "All Subscription Resources"
- Status bar shows context about the view mode

**State Management:**
- `gui.subscriptionViewMode` boolean tracks if in this mode
- `Escape` exits the mode and returns to normal navigation
- Selecting a different subscription or RG clears the mode

**Implementation:**
```go
// Enter subscription view mode
gui.subscriptionViewMode = true
gui.loadSubscriptionResources(subID)

// Exit mode
gui.exitSubscriptionViewMode()
```

### Cross-Subscription Global Search

Enables searching for resources across ALL subscriptions using Azure Resource Graph.

**Activation:**
- Press `G` in the subscriptions panel to start global search
- Opens search bar at bottom for entering query
- Type query and press Enter to execute search
- Uses Azure Resource Graph KQL for efficient queries

**Display Format:**
- Results show: `name (sub-name / rg-name / Type)`
- Panel title shows: "Global Search Results: <query>"
- Supports viewing details of resources from any subscription

**Key Features:**
- Search executes when Enter is pressed (not on every keystroke)
- Searches across resource name, type, and resource group name
- Respects active type filter if set (set with `T` before or during search)
- Background execution with loading indicator

**Keybindings:**
- `G` (in subscriptions panel) - Start global search
- Characters - Type search query
- `Enter` - Execute search
- `Escape` - Cancel and exit global search mode
- `T` (after search) - Change type filter and re-run search

**Implementation:**
```go
// ResourceGraphClient interface
type ResourceGraphClient interface {
    QueryResources(ctx context.Context, subscriptionIDs []string, query string) ([]*domain.Resource, error)
}

// Usage
resources, err := gui.resourceGraphClient.QueryResources(ctx, subIDs, kqlQuery)
```

**KQL Query Pattern:**
```kql
Resources
| where name contains 'searchterm' or type contains 'searchterm' or resourceGroup contains 'searchterm'
| project id, name, type, location, resourceGroup, subscriptionId, tags, properties
| limit 500
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

- [ ] Code builds without errors: `make build`
- [ ] Tests have been updated or added per the guideline in this file
- [ ] Tests pass: `make test`
- [ ] Code is properly formatted: `gofmt -l .` returns empty
- [ ] Modules are tidy: `go mod tidy`
- [ ] Debug logging is properly guarded with `LAZYAZURE_DEBUG` check
- [ ] Debug logging does NOT contain sensitive data (UPN, IDs, resource IDs) - see Debug Logging Privacy section
- [ ] No mutex deadlocks introduced (verify lock patterns)
- [ ] **Third-party dependencies updated**:
  - [ ] THIRD-PARTY-NOTICES.txt updated with full license text for any new dependencies
  - [ ] License compatibility verified for new dependencies
- [ ] **Documentation updated**:
  - [ ] AGENTS.md - File organization section, relevant guidelines, and checklist updated
  - [ ] README.md - User-facing documentation updated for any new features that's relevant for users
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
