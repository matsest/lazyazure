# LazyAzure

A TUI application for Azure resource management, inspired by [lazydocker](https://github.com/jesseduffield/lazydocker). Browse Azure subscriptions, resource groups, and resources with an interactive terminal interface.

> **About This Project**: This project is vibe-coded with [OpenCode](https://opencode.ai). It is provided as-is without warranties ([MIT License](./LICENSE)). See [AGENTS.md](AGENTS.md) for development guidelines.

![demo](./demo.png)

## Features

- **Full Resource Hierarchy**: Browse Subscriptions → Resource Groups → Resources
- **Multiple Authentication Methods**: Supports Azure CLI, Managed Identity, Environment Variables, and more
- **Rich Detail Views**: 
  - Summary view with color-coded keys and formatted nested properties
  - JSON view with syntax highlighting
  - Scrollable content for long resource details
- **Intuitive Navigation**: 
  - Tab/Shift+Tab to cycle between panels
  - Enter to drill down hierarchy
  - Visual focus indicators (green border on active panel)
- **Smart Resource Loading**: Fetches full resource details with provider-specific API versions
- **Real-time Updates**: Refresh data without restarting the application

See [PLAN.md](./PLAN.md) for implementation details and roadmap.

## Installation

### Prerequisites

- Azure account with appropriate permissions
- Go 1.26.1+ installed

Optional:
- Azure CLI installed (for `az login` convenience method)

### Install from Source

```bash
go install github.com/matsest/lazyazure@latest
```

Or clone and build:

```bash
git clone https://github.com/matsest/lazyazure.git
cd lazyazure
go build .
```

## Usage

### Quick Start

1. **Authenticate** (choose one method):
   ```bash
   # Option A: Azure CLI (easiest for local development)
   az login
   
   # Option B: Environment variables
   export AZURE_CLIENT_ID="your-client-id"
   export AZURE_CLIENT_SECRET="your-client-secret"
   export AZURE_TENANT_ID="your-tenant-id"
   ```

2. **Run lazyazure**:
   ```bash
   ./lazyazure
   ```

### Controls

**Navigation:**
- **↑ / ↓** or **j / k**: Navigate items in current panel
- **Tab**: Switch focus forward between panels (Subscriptions → Resource Groups → Resources → Details)
- **Shift+Tab**: Switch focus backward between panels (Details → Resources → Resource Groups → Subscriptions)
- **Enter** (on subscription): Load resource groups for that subscription
- **Enter** (on resource group): Load resources in that resource group
- **Enter** (on resource): View resource details and focus right panel

**View Controls:**
- **[ / ]**: Switch between Summary and JSON tabs
  - Summary: Color-coded keys (green) with formatted values
  - JSON: Syntax highlighted with proper formatting
- **↑ / ↓** or **j / k** (in details panel): Scroll content up/down
- **PgUp / PgDn**: Scroll content by page
- **r**: Refresh current data

**Application:**
- **q** or **Ctrl+C**: Quit

## Authentication

LazyAzure uses Azure's `DefaultAzureCredential` which automatically tries multiple authentication methods in order:

1. **Environment Variables** - Set `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, and `AZURE_TENANT_ID`
2. **Managed Identity** - Automatic authentication when running in Azure (VMs, containers, etc.)
3. **Azure CLI** - Run `az login` (convenient for local development)
4. **Azure PowerShell** - Uses PowerShell credentials if available
5. **Visual Studio Code** - Uses VS Code Azure extension credentials
6. **Azure Developer CLI** - Uses `azd` credentials

For most users, simply run `az login` before starting lazyazure.

## Demo Mode

LazyAzure includes a demo mode that runs with mock Azure data - no Azure credentials required! This is perfect for:
- Creating GIFs or screenshots for documentation
- Testing the UI without a real Azure subscription
- Demonstrating features to users

To run in demo mode:

```bash
LAZYAZURE_DEMO=1 ./lazyazure
```

Demo mode provides:
- 2 mock subscriptions (Demo Production & Demo Development)
- 4 resource groups per subscription with various locations
- Multiple resource types (Storage Accounts, Key Vaults, VMs, SQL Databases, Load Balancers)
- Realistic nested properties and tags
- Simulated API response times for authentic feel

All data in demo mode is completely fake and safe to display publicly.

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
│   │   ├── factory.go           # Client factory for dependency injection
│   │   ├── subscriptions.go     # Subscription operations
│   │   ├── resourcegroups.go    # Resource group operations
│   │   └── resources.go         # Generic resource operations
│   ├── demo/
│   │   ├── client.go            # Demo client (mock Azure data)
│   │   └── data.go              # Demo data structures
│   ├── domain/
│   │   ├── user.go              # User domain model
│   │   ├── subscription.go      # Subscription domain model
│   │   ├── resourcegroup.go     # ResourceGroup domain model
│   │   └── resource.go          # Generic Resource domain model
│   ├── gui/
│   │   ├── gui.go               # Main TUI controller
│   │   ├── interfaces.go        # Client interfaces
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
- **Phase 3**: ✅ Complete - Resources browser with 3-level hierarchy
- **Phase 4**: 📝 Planned - Polish & advanced features

See [PLAN.md](./PLAN.md) for the full implementation roadmap.

## Dependencies

- [gocui](https://github.com/jesseduffield/gocui) - TUI framework
- [Chroma](https://github.com/alecthomas/chroma) - Syntax highlighting for JSON
- Azure SDK for Go:
  - `azidentity` - Authentication
  - `azcore` - Core types
  - `armsubscriptions` - Subscription management
  - `armresources` - Resource management

## License

[MIT](LICENSE)
