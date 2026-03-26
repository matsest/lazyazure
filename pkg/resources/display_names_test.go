package resources

import (
	"testing"
)

func TestGetResourceTypeDisplayName_CoreMapping(t *testing.T) {
	tests := []struct {
		resourceType string
		want         string
	}{
		// Compute
		{"Microsoft.Compute/virtualMachines", "Virtual Machine"},
		{"Microsoft.Compute/virtualMachineScaleSets", "VM Scale Set"},
		{"Microsoft.Compute/disks", "Managed Disk"},

		// Storage
		{"Microsoft.Storage/storageAccounts", "Storage Account"},

		// Web
		{"Microsoft.Web/sites", "Web App"},
		{"Microsoft.Web/serverFarms", "App Service Plan"},

		// Database
		{"Microsoft.Sql/servers", "SQL Server"},
		{"Microsoft.Sql/servers/databases", "SQL Database"},
		{"Microsoft.DocumentDB/databaseAccounts", "Cosmos DB"},

		// Networking
		{"Microsoft.Network/virtualNetworks", "Virtual Network"},
		{"Microsoft.Network/networkInterfaces", "Network Interface"},
		{"Microsoft.Network/publicIPAddresses", "Public IP"},
		{"Microsoft.Network/loadBalancers", "Load Balancer"},
		{"Microsoft.Network/networkSecurityGroups", "NSG"},

		// Identity
		{"Microsoft.KeyVault/vaults", "Key Vault"},

		// Container
		{"Microsoft.ContainerService/managedClusters", "AKS Cluster"},

		// Monitoring
		{"Microsoft.Insights/workbooks", "Workbook"},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			got := GetResourceTypeDisplayName(tt.resourceType)
			if got != tt.want {
				t.Errorf("GetResourceTypeDisplayName(%q) = %q, want %q", tt.resourceType, got, tt.want)
			}
		})
	}
}

func TestGetResourceTypeDisplayName_FallbackAlgorithm(t *testing.T) {
	tests := []struct {
		resourceType string
		want         string
	}{
		// Multi-word resource names
		{"Microsoft.Compute/availabilitySets", "Availability Set"},
		{"Microsoft.Network/routeTables", "Route Table"},
		{"Microsoft.Network/privateEndpoints", "Private Endpoint"},

		// Single word resource names - use provider
		{"Microsoft.Sql/servers", "SQL Server"},                   // Should be in core mapping
		{"Microsoft.Insights/components", "Application Insights"}, // In core mapping
		{"Microsoft.DataFactory/factories", "Data Factory"},       // Should be in core

		// Acronyms
		{"Microsoft.Network/publicIPAddresses", "Public IP"},
		{"Microsoft.Network/networkSecurityGroups", "NSG"},

		// Plural to singular
		{"Microsoft.Compute/snapshots", "Snapshot"},
		{"Microsoft.Storage/queues", "Storage Queue"}, // Single word + provider
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			got := GetResourceTypeDisplayName(tt.resourceType)
			if got != tt.want {
				t.Errorf("GetResourceTypeDisplayName(%q) = %q, want %q", tt.resourceType, got, tt.want)
			}
		})
	}
}

func TestGetResourceTypeDisplayName_UnknownTypes(t *testing.T) {
	tests := []struct {
		resourceType string
		want         string
	}{
		{"Microsoft.Custom/resourcePools", "Resource Pool"}, // 2 words, uses resource name
		{"Microsoft.Custom/single", "Custom Single"},        // 1 word, uses provider
		{"justType", "Just Type"},                           // No provider, just camelCase
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			got := GetResourceTypeDisplayName(tt.resourceType)
			if got != tt.want {
				t.Errorf("GetResourceTypeDisplayName(%q) = %q, want %q", tt.resourceType, got, tt.want)
			}
		})
	}
}

func TestGenerateDisplayName(t *testing.T) {
	tests := []struct {
		resourceType string
		want         string
	}{
		// Multi-word resource names (fallback algorithm)
		{"Microsoft.Test/virtualMachines", "Virtual Machine"},
		{"Microsoft.Test/storageAccounts", "Storage Account"},
		{"Microsoft.Test/networkInterfaces", "Network Interface"},

		// Single word resource names - use provider (fallback algorithm)
		{"Microsoft.Sql/servers", "SQL Server"},
		{"Microsoft.KeyVault/vaults", "Key Vault"},

		// Acronyms (fallback algorithm)
		{"Microsoft.Test/publicIPAddresses", "Public IPAddress"}, // Not in core, uses fallback (camelCase split limitation)
		{"Microsoft.Network/nsgRules", "NSG Rule"},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			got := generateDisplayName(tt.resourceType)
			if got != tt.want {
				t.Errorf("generateDisplayName(%q) = %q, want %q", tt.resourceType, got, tt.want)
			}
		})
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"machines", "machine"},
		{"accounts", "account"},
		{"services", "service"},
		{"parties", "party"},
		{"buses", "bus"},
		{"tables", "table"},
		{"queues", "queue"},
		{"snapshots", "snapshot"},
		{"status", "status"},     // Should not change
		{"address", "address"},   // Should not change
		{"class", "class"},       // Should not change
		{"insights", "insights"}, // Should not change - brand name
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := singularize(tt.input)
			if got != tt.want {
				t.Errorf("singularize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
