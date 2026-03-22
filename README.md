# LazyAzure

A TUI application for Azure resource management, inspired by lazydocker. Browse Azure subscriptions, resource groups, and resources with an interactive terminal interface.

## Features

### Phase 1 & 2 Complete ✓

- **Azure CLI Authentication** - Seamless integration via DefaultAzureCredential
- **Subscription Browser** - View all accessible subscriptions with name and ID
- **Resource Group Explorer** - Navigate resource groups within subscriptions
- **Stacked Panel Layout** - All panels visible simultaneously (inspired by lazydocker)
  - Auth panel (shows current user)
  - Subscriptions panel (40% of sidebar)
  - Resource Groups panel (remaining space)
- **Interactive Details** - View details in Summary or JSON format
- **Visual Focus Indicator** - ▶ arrow shows which panel is active

### Layout

```
┌─────────────────────┬──────────────────────────────────┐
│ Auth                │                                  │
│ User: Authenticated │  Details [Summary] [JSON]        │
├─────────────────────┤                                  │
│ ▶ Subscriptions     │  Name: Production Sub             │
│ • Sub A             │  ID: abc-123-456                 │
│ • Sub B  [selected] │  State: Enabled                  │
│ • Sub C             │  Tenant ID: xyz-789              │
├─────────────────────┤                                  │
│   Resource Groups   │                                  │
│ • RG-1              │                                  │
│ • RG-2  [selected] │                                  │
│ • RG-3              │                                  │
└─────────────────────┴──────────────────────────────────┘
 ↑↓ Navigate | Enter: Load/View | Tab: Switch | [] Tabs | q: Quit
```

## Usage

### Prerequisites

- Azure CLI installed and authenticated (`az login`)
- Go 1.26.1+ installed

### Building

```bash
go build .
```

### Running

```bash
./lazyazure
```

### Controls

**Navigation:**
- **↑ / ↓** or **j / k**: Navigate items in current panel
- **Tab**: Switch focus between Subscriptions and Resource Groups panels
- **Enter** (on subscription): Load resource groups for that subscription
- **Enter** (on resource group): View details in right panel

**View Controls:**
- **[ / ]**: Switch between Summary and JSON tabs
- **r**: Refresh current data

**Application:**
- **q** or **Ctrl+C**: Quit

## Authentication

LazyAzure uses Azure's `DefaultAzureCredential` which automatically:
1. Checks environment variables
2. Checks for managed identity
3. Falls back to Azure CLI credentials (primary method for this app)

To authenticate:
```bash
az login
```

## Debug Logging

To enable debug logging for troubleshooting, set the `LAZYAZURE_DEBUG` environment variable:

```bash
LAZYAZURE_DEBUG=1 ./lazyazure
```

Debug logs are written to `~/.lazyazure/debug.log`.

To view logs:
```bash
cat ~/.lazyazure/debug.log
```

## Architecture

```
lazyazure/
├── main.go                       # Entry point
├── PLAN.md                       # Full implementation plan
├── pkg/
│   ├── azure/
│   │   ├── client.go            # Azure SDK wrapper
│   │   ├── subscriptions.go     # Subscription operations
│   │   └── resourcegroups.go    # Resource group operations
│   ├── domain/
│   │   ├── user.go              # User domain model
│   │   ├── subscription.go      # Subscription domain model
│   │   └── resourcegroup.go     # ResourceGroup domain model
│   ├── gui/
│   │   ├── gui.go               # Main TUI controller
│   │   └── panels/
│   │       └── filtered_list.go # Generic filtered list
│   ├── tasks/
│   │   └── tasks.go             # Async task management
│   └── utils/
│       └── logger.go            # Debug logging utility
```

## Project Status

- **Phase 1 (MVP)**: ✅ Complete - Auth & subscriptions working
- **Phase 2**: ✅ Complete - Resource groups with stacked layout
- **Phase 3**: 📝 Planned - Resources browser
- **Phase 4**: 📝 Planned - Polish & advanced features

See `PLAN.md` for the full implementation roadmap.

## Dependencies

- [gocui](https://github.com/jesseduffield/gocui) - TUI framework
- Azure SDK for Go:
  - `azidentity` - Authentication
  - `azcore` - Core types
  - `armsubscriptions` - Subscription management
  - `armresources` - Resource group management

## License

[MIT](LICENSE)
