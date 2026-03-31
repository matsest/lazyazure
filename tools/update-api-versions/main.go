package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Curated list of resource type patterns to include (covers ~95% of common usage)
// Organized by Azure service category for maintainability
var curatedPatterns = []string{
	// Compute (7 types)
	"Microsoft.Compute/virtualMachines",
	"Microsoft.Compute/virtualMachineScaleSets",
	"Microsoft.Compute/disks",
	"Microsoft.Compute/images",
	"Microsoft.Compute/snapshots",
	"Microsoft.Compute/availabilitySets",
	"Microsoft.Compute/proximityPlacementGroups",

	// Storage (4 types)
	"Microsoft.Storage/storageAccounts",
	"Microsoft.Storage/storageAccounts/blobServices",
	"Microsoft.Storage/storageAccounts/fileServices",
	"Microsoft.Storage/storageAccounts/queueServices",

	// Network Core (15 types)
	"Microsoft.Network/virtualNetworks",
	"Microsoft.Network/networkSecurityGroups",
	"Microsoft.Network/publicIPAddresses",
	"Microsoft.Network/publicIPPrefixes",
	"Microsoft.Network/loadBalancers",
	"Microsoft.Network/applicationGateways",
	"Microsoft.Network/azureFirewalls",
	"Microsoft.Network/firewallPolicies",
	"Microsoft.Network/natGateways",
	"Microsoft.Network/privateEndpoints",
	"Microsoft.Network/privateLinkServices",
	"Microsoft.Network/networkInterfaces",
	"Microsoft.Network/routeTables",
	"Microsoft.Network/bastionHosts",
	"Microsoft.Network/virtualNetworkGateways",

	// Network Advanced (10 types)
	"Microsoft.Network/vpnGateways",
	"Microsoft.Network/vpnSites",
	"Microsoft.Network/expressRouteCircuits",
	"Microsoft.Network/expressRouteGateways",
	"Microsoft.Network/virtualWans",
	"Microsoft.Network/virtualHubs",
	"Microsoft.Network/applicationSecurityGroups",
	"Microsoft.Network/ddosProtectionPlans",
	"Microsoft.Network/ipGroups",
	"Microsoft.Network/networkWatchers",
	"Microsoft.Network/frontDoors",
	"Microsoft.Network/trafficmanagerprofiles",
	"Microsoft.Network/dnsZones",
	"Microsoft.Network/privateDnsZones",
	"Microsoft.Network/dnsForwardingRulesets",
	"Microsoft.Network/frontdoorwebapplicationfirewallpolicies",

	// Identity & Security (6 types)
	"Microsoft.ManagedIdentity/userAssignedIdentities",
	"Microsoft.KeyVault/vaults",
	"Microsoft.KeyVault/managedHSMs",
	"Microsoft.Authorization/roleAssignments",
	"Microsoft.Authorization/roleDefinitions",
	"Microsoft.Security/pricings",

	// Sentinel (Security Insights) (4 types)
	"Microsoft.SecurityInsights/incidents",
	"Microsoft.SecurityInsights/alertRules",
	"Microsoft.SecurityInsights/dataConnectors",
	"Microsoft.SecurityInsights/watchlists",

	// Web & App Services (7 types)
	"Microsoft.Web/sites",
	"Microsoft.Web/serverFarms",
	"Microsoft.Web/staticSites",
	"Microsoft.Web/certificates",
	"Microsoft.DomainRegistration/domains",
	"Microsoft.Cdn/profiles",
	"Microsoft.Cdn/profiles/endpoints",
	"Microsoft.Cdn/CdnWebApplicationFirewallPolicies",

	// Relational Databases (10 types)
	"Microsoft.Sql/servers",
	"Microsoft.Sql/servers/databases",
	"Microsoft.Sql/servers/elasticPools",
	"Microsoft.Sql/managedInstances",
	"Microsoft.DBforPostgreSQL/servers",
	"Microsoft.DBforPostgreSQL/flexibleServers",
	"Microsoft.DBforMySQL/servers",
	"Microsoft.DBforMySQL/flexibleServers",
	"Microsoft.DBforMariaDB/servers",
	"Microsoft.DBforMySQL/flexibleServers",

	// NoSQL & Big Data (8 types)
	"Microsoft.DocumentDB/databaseAccounts",
	"Microsoft.Cache/redis",
	"Microsoft.Cache/redisEnterprise",
	"Microsoft.EventHub/namespaces",
	"Microsoft.EventHub/clusters",
	"Microsoft.StreamAnalytics/streamingJobs",
	"Microsoft.DataLakeStore/accounts",
	"Microsoft.DataLakeAnalytics/accounts",
	"Microsoft.Databricks/workspaces",

	// Containers & Kubernetes (8 types)
	"Microsoft.ContainerService/managedClusters",
	"Microsoft.ContainerRegistry/registries",
	"Microsoft.ContainerRegistry/registries/tasks",
	"Microsoft.ContainerInstance/containerGroups",
	"Microsoft.ServiceFabric/clusters",
	"Microsoft.ServiceFabric/managedClusters",
	"Microsoft.App/containerApps",
	"Microsoft.App/managedEnvironments",

	// AI & Machine Learning (6 types)
	"Microsoft.CognitiveServices/accounts",
	"Microsoft.MachineLearningServices/workspaces",
	"Microsoft.MachineLearning/registries",
	"Microsoft.MachineLearning/workspaces",
	"Microsoft.Search/searchServices",
	"Microsoft.BotService/botServices",

	// Monitoring & Management (8 types)
	"Microsoft.OperationalInsights/workspaces",
	"Microsoft.Insights/components",
	"Microsoft.Insights/actionGroups",
	"Microsoft.AlertsManagement/smartDetectorAlertRules",
	"Microsoft.Insights/scheduledQueryRules",
	"Microsoft.Insights/metricAlerts",
	"Microsoft.Insights/activityLogAlerts",
	"Microsoft.Maintenance/configurationAssignments",

	// Integration & Messaging (8 types)
	"Microsoft.ServiceBus/namespaces",
	"Microsoft.ServiceBus/namespaces/topics",
	"Microsoft.ServiceBus/namespaces/queues",
	"Microsoft.EventGrid/topics",
	"Microsoft.EventGrid/domains",
	"Microsoft.EventGrid/systemTopics",
	"Microsoft.EventGrid/eventSubscriptions",
	"Microsoft.Logic/workflows",

	// API Management & Integration (4 types)
	"Microsoft.ApiManagement/service",
	"Microsoft.ApiManagement/service/apis",
	"Microsoft.DataFactory/factories",
	"Microsoft.Synapse/workspaces",

	// IoT (5 types)
	"Microsoft.Devices/IotHubs",
	"Microsoft.Devices/provisioningServices",
	"Microsoft.TimeSeriesInsights/environments",
	"Microsoft.DigitalTwins/digitalTwinsInstances",
	"Microsoft.Maps/accounts",

	// Migration & Backup (4 types)
	"Microsoft.RecoveryServices/vaults",
	"Microsoft.RecoveryServices/vaults/backupPolicies",
	"Microsoft.DataProtection/backupVaults",
	"Microsoft.Migrate/migrateProjects",
	"Microsoft.Migrate/assessmentProjects",

	// DevOps & Developer Tools (6 types)
	"Microsoft.DevTestLab/labs",
	"Microsoft.HybridCompute/machines",
	"Microsoft.Automation/automationAccounts",
	"Microsoft.DesktopVirtualization/hostPools",
	"Microsoft.DesktopVirtualization/applicationGroups",
	"Microsoft.DesktopVirtualization/workspaces",

	// Resources & Deployment (3 types)
	"Microsoft.Resources/resourceGroups",
	"Microsoft.Resources/deployments",
	"Microsoft.Resources/templateSpecs",
}

// BicepIndex represents the structure of bicep-types-az index.json
type BicepIndex struct {
	Resources map[string]interface{} `json:"resources"`
}

// APIVersionsOutput represents our generated API versions JSON
type APIVersionsOutput struct {
	LastUpdated   string            `json:"lastUpdated"`
	Source        string            `json:"source"`
	ResourceCount int               `json:"resourceCount"`
	APIVersions   map[string]string `json:"apiVersions"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <path/to/index.json> [output.json]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExtracts API versions for curated resource types from bicep-types-az index.json\n")
		os.Exit(1)
	}

	inputPath := os.Args[1]
	outputPath := "pkg/azure/api_versions_curated.json"
	if len(os.Args) >= 3 {
		outputPath = os.Args[2]
	}

	// Read and parse index.json
	data, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", inputPath, err)
		os.Exit(1)
	}

	var index BicepIndex
	if err := json.Unmarshal(data, &index); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	// Extract versions for curated patterns
	apiVersions := extractCuratedVersions(index.Resources)

	// Create output
	output := APIVersionsOutput{
		LastUpdated:   time.Now().UTC().Format(time.RFC3339),
		Source:        "https://github.com/Azure/bicep-types-az",
		ResourceCount: len(apiVersions),
		APIVersions:   apiVersions,
	}

	// Write output
	outputData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling output: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s with %d curated API versions\n", outputPath, len(apiVersions))
	fmt.Printf("Source: %s\n", output.Source)
	fmt.Printf("Last Updated: %s\n", output.LastUpdated)
}

// extractCuratedVersions extracts the best API version for each curated resource type
func extractCuratedVersions(resources map[string]interface{}) map[string]string {
	// Group entries by resource type (without version)
	typeVersions := make(map[string][]string)

	for key := range resources {
		// Key format: "Provider/ResourceType@api-version"
		parts := strings.Split(key, "@")
		if len(parts) != 2 {
			continue
		}
		resourceType := parts[0]
		apiVersion := parts[1]

		// Check if this matches any curated pattern
		for _, pattern := range curatedPatterns {
			if resourceType == pattern {
				typeVersions[resourceType] = append(typeVersions[resourceType], apiVersion)
				break
			}
		}
	}

	// Select best version for each resource type
	result := make(map[string]string)
	for resourceType, versions := range typeVersions {
		bestVersion := selectBestVersion(versions)
		if bestVersion != "" {
			result[resourceType] = bestVersion
		}
	}

	return result
}

// selectBestVersion selects the latest stable version, or latest preview if no stable
func selectBestVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}

	// Separate stable and preview versions
	var stableVersions, previewVersions []string
	for _, v := range versions {
		if strings.Contains(strings.ToLower(v), "preview") {
			previewVersions = append(previewVersions, v)
		} else {
			stableVersions = append(stableVersions, v)
		}
	}

	// Prefer stable versions
	if len(stableVersions) > 0 {
		sortVersions(stableVersions)
		return stableVersions[0]
	}

	// Fall back to preview
	if len(previewVersions) > 0 {
		sortVersions(previewVersions)
		return previewVersions[0]
	}

	return ""
}

// sortVersions sorts versions in descending order (newest first)
// Simple string sort works for ISO date versions
func sortVersions(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] > versions[j]
	})
}
