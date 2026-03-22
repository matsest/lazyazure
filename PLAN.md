# LazyAzure Implementation Plan

## Overview
A TUI application for Azure resource management, inspired by lazydocker. It provides an interactive terminal interface for browsing Azure subscriptions, resource groups, and resources with detailed viewers.

## Architecture

### Inspiration from lazydocker
- TUI Library: `gocui` (same as lazydocker)
- Generic panel system with Go generics
- Async task management
- Tab-based right panel viewers
- Filterable/sortable lists
- Box layout system

### Azure SDK Stack
- Authentication: `github.com/Azure/azure-sdk-for-go/sdk/azidentity` (DefaultAzureCredential)
- Subscriptions: `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions`
- Resource Groups: `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources`
- Generic Resources: `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources`

---

## Phase 1: MVP - Authentication & Subscription List ✅ COMPLETE

**Goal:** Working TUI showing Azure auth status and subscription picker

### Implementation Steps:

1. **Project Setup** ✅
   - Initialize Go module: `go mod init github.com/matsest/lazyazure`
   - Add dependencies:
     - `github.com/jesseduffield/gocui` (TUI)
     - `github.com/Azure/azure-sdk-for-go/sdk/azidentity`
     - `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions`

2. **Azure Client Layer** (`pkg/azure/`) ✅
   - `client.go`: Wrapper around Azure SDK clients
   - `subscriptions.go`: List subscriptions with name and ID

3. **Domain Models** (`pkg/domain/`) ✅
   - `subscription.go`: Subscription struct with Name, ID, State
   - `user.go`: User info (name)

4. **TUI Foundation** (`pkg/gui/`) ✅
   - `gui.go`: Main GUI struct, event loop, view arrangement

5. **Panels** (`pkg/gui/panels/`) ✅
   - `filtered_list.go`: Generic filtered list component

6. **Right Panel Viewer** ✅
   - JSON and Summary tabs implemented directly in gui.go

7. **Main Entry** (`main.go`) ✅
   - Initialize Azure client with DefaultAzureCredential
   - Start GUI event loop

---

## Phase 2: Resource Groups & Navigation ✅ COMPLETE

**Goal:** Interactive hierarchy: Subscriptions → Resource Groups

### Implementation Summary:

1. **Azure Client Updates** ✅
   - Added `resourcegroups.go`: List resource groups with name, location, state
   - Uses `armresources` SDK

2. **Domain Models** ✅
   - `resourcegroup.go`: ResourceGroup struct with Name, Location, ID, ProvisioningState, Tags

3. **Layout Redesign** ✅
   - Stacked panel layout (inspired by lazydocker)
   - Auth panel (3 lines) - shows current user
   - Subscriptions panel (40% of sidebar)
   - Resource Groups panel (remaining space)
   - All panels visible simultaneously

4. **Navigation System** ✅
   - Tab key switches between subscriptions and resource groups panels
   - Enter key on subscription loads resource groups
   - Arrow keys navigate within active panel
   - Visual indicator (▶) shows active panel

5. **Right Panel** ✅
   - Shows subscription details (name, ID, state, tenant)
   - Shows resource group details (name, location, ID, provisioning state, tags)
   - Summary and JSON tabs

6. **Status Bar** ✅
   - Context-sensitive help text
   - Shows current panel and available actions

---

## Phase 3: Resources & Deep Viewing

**Goal:** Full hierarchy: Subscriptions → Resource Groups → Resources

### Implementation Steps:

1. **Azure Client**
   - Add `resources.go`: List resources with type, name, location
   - Generic resource client using `armresources.NewClient()`

2. **Domain Models**
   - `resource.go`: Generic Azure resource (ID, Name, Type, Location, Tags)

3. **Updated Panels**
   - `resources_panel.go`: List resources in selected resource group
   - Context switching between all three levels

4. **Enhanced Viewer**
   - Full JSON representation of any selected resource
   - Summary view with formatted key fields
   - Tag display

---

## Phase 4: Polish & Advanced Features

**Goal:** Production-ready with UX improvements

### Implementation Steps:

1. **Search & Filtering**
   - Real-time search/filter in all panels
   - Fuzzy matching
   - Case-insensitive search

2. **Keyboard Shortcuts**
   - `/` for search
   - `q` or `Ctrl+C` to quit
   - Arrow keys for navigation
   - `Tab` for switching right panel tabs
   - `Enter` to drill down, `Esc` or `h` to go back

3. **Caching**
   - In-memory cache for API responses
   - Refresh with `r` key
   - Expire cache after time interval

4. **Configuration**
   - Config file support (`~/.config/lazyazure/config.yml`)
   - Theme customization
   - Default subscription preference

5. **Error Handling**
   - Graceful handling of auth failures
   - Retry logic for API calls
   - Status bar messages

6. **Performance**
   - Lazy loading (fetch on demand)
   - Pagination for large resource lists
   - Background refresh

---

## Project Structure

```
lazyazure/
├── main.go                       # Entry point
├── go.mod
├── go.sum
├── LICENSE                       # MIT License
├── README.md                     # User documentation
├── AGENTS.md                     # Development guidelines for AI agents
├── PLAN.md                       # This file - implementation roadmap
├── pkg/
│   ├── azure/
│   │   ├── client.go            # Azure SDK wrapper with DefaultAzureCredential
│   │   ├── client_test.go       # Azure client tests
│   │   ├── subscriptions.go     # Subscription operations
│   │   ├── resourcegroups.go    # Resource group operations
│   │   └── resourcegroups_test.go # RG tests
│   ├── domain/
│   │   ├── user.go              # User domain model
│   │   ├── subscription.go      # Subscription domain model
│   │   └── resourcegroup.go     # ResourceGroup domain model
│   ├── gui/
│   │   ├── gui.go               # Main GUI controller with all TUI logic
│   │   ├── gui_test.go          # GUI tests
│   │   └── panels/
│   │       └── filtered_list.go # Generic filtered list component
│   ├── tasks/
│   │   ├── tasks.go             # Async task management
│   │   └── tasks_test.go        # Task manager tests
│   └── utils/
│       └── logger.go            # Debug logging (opt-in via LAZYAZURE_DEBUG)
```

---

## Key Technical Decisions

1. **TUI Library**: `gocui` - Same as lazydocker, proven, battle-tested
2. **Azure Auth**: `DefaultAzureCredential` - Seamlessly uses Azure CLI auth
3. **Generic Panels**: Go generics for type-safe, reusable UI components
4. **Async Tasks**: Background loading to keep UI responsive
5. **Layout**: Box-based responsive layout system from lazycore

---

## MVP Success Criteria

- [x] User can launch and see current Azure CLI identity
- [x] Left panel shows list of subscriptions (name, ID)
- [x] Can navigate subscriptions with arrow keys
- [x] Right panel shows subscription details in JSON and summary tabs
- [x] Can switch tabs with `[` and `]`
- [x] App gracefully handles Azure CLI not being logged in
- [x] Clean exit with `q` or `Ctrl+C`
- [x] Navigate to resource groups within subscriptions
- [x] View resource group details (name, location, provisioning state, tags)
