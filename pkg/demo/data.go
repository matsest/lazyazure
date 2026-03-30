package demo

import (
	"fmt"
	"time"

	"github.com/matsest/lazyazure/pkg/domain"
)

// DemoData holds all mock data for the demo mode
type DemoData struct {
	User           *domain.User
	Subscriptions  []*domain.Subscription
	ResourceGroups map[string][]*domain.ResourceGroup // key: subscription ID
	Resources      map[string][]*domain.Resource      // key: resource group ID
}

// NewDemoData creates a complete set of demo data
func NewDemoData() *DemoData {
	data := &DemoData{
		User: &domain.User{
			DisplayName:       "Demo User",
			UserPrincipalName: "demo.user@example.com",
			Type:              "user",
			TenantID:          "00000000-0000-0000-0000-000000000000",
		},
		Subscriptions:  createDemoSubscriptions(),
		ResourceGroups: make(map[string][]*domain.ResourceGroup),
		Resources:      make(map[string][]*domain.Resource),
	}

	// Create resource groups for each subscription
	for _, sub := range data.Subscriptions {
		data.ResourceGroups[sub.ID] = createDemoResourceGroups(sub.ID)
	}

	// Create resources for each resource group
	for _, rgs := range data.ResourceGroups {
		for _, rg := range rgs {
			data.Resources[rg.Name] = createDemoResources(subIDFromRGID(rg.ID), rg.Name)
		}
	}

	return data
}

func createDemoSubscriptions() []*domain.Subscription {
	return []*domain.Subscription{
		{
			ID:       "00000000-0000-0000-0000-000000000001",
			Name:     "Demo Production",
			State:    "Enabled",
			TenantID: "00000000-0000-0000-0000-000000000000",
		},
		{
			ID:       "00000000-0000-0000-0000-000000000002",
			Name:     "Demo Development",
			State:    "Enabled",
			TenantID: "00000000-0000-0000-0000-000000000000",
		},
	}
}

func createDemoResourceGroups(subscriptionID string) []*domain.ResourceGroup {
	var prefix string
	if subscriptionID == "00000000-0000-0000-0000-000000000001" {
		prefix = "prod"
	} else {
		prefix = "dev"
	}

	return []*domain.ResourceGroup{
		{
			Name:              "rg-" + prefix + "-web",
			Location:          "westus2",
			ID:                "/subscriptions/" + subscriptionID + "/resourceGroups/rg-" + prefix + "-web",
			ProvisioningState: "Succeeded",
			Tags: map[string]string{
				"Environment": prefix,
				"Team":        "platform",
				"CostCenter":  "IT-001",
			},
			SubscriptionID: subscriptionID,
		},
		{
			Name:              "rg-" + prefix + "-analytics",
			Location:          "eastus",
			ID:                "/subscriptions/" + subscriptionID + "/resourceGroups/rg-" + prefix + "-analytics",
			ProvisioningState: "Succeeded",
			Tags: map[string]string{
				"Environment": prefix,
				"Team":        "data",
				"CostCenter":  "IT-002",
			},
			SubscriptionID: subscriptionID,
		},
		{
			Name:              "rg-" + prefix + "-storage",
			Location:          "westeurope",
			ID:                "/subscriptions/" + subscriptionID + "/resourceGroups/rg-" + prefix + "-storage",
			ProvisioningState: "Succeeded",
			Tags: map[string]string{
				"Environment": prefix,
				"Team":        "storage",
				"CostCenter":  "IT-003",
			},
			SubscriptionID: subscriptionID,
		},
		{
			Name:              "rg-" + prefix + "-networking",
			Location:          "southeastasia",
			ID:                "/subscriptions/" + subscriptionID + "/resourceGroups/rg-" + prefix + "-networking",
			ProvisioningState: "Succeeded",
			Tags: map[string]string{
				"Environment": prefix,
				"Team":        "network",
				"CostCenter":  "IT-004",
			},
			SubscriptionID: subscriptionID,
		},
	}
}

func createDemoResources(subscriptionID, resourceGroupName string) []*domain.Resource {
	createdTime := time.Now().AddDate(0, -3, 0).Format(time.RFC3339)
	changedTime := time.Now().AddDate(0, -1, 0).Format(time.RFC3339)

	baseID := "/subscriptions/" + subscriptionID + "/resourceGroups/" + resourceGroupName + "/providers"

	resources := []*domain.Resource{
		{
			ID:             baseID + "/Microsoft.Storage/storageAccounts/demo" + resourceGroupName + "storage",
			Name:           "demo" + resourceGroupName + "storage",
			Type:           "Microsoft.Storage/storageAccounts",
			Location:       "westus2",
			ResourceGroup:  resourceGroupName,
			SubscriptionID: subscriptionID,
			Tags: map[string]string{
				"Purpose": "data-storage",
			},
			Properties: map[string]interface{}{
				"accessTier":               "Hot",
				"allowBlobPublicAccess":    false,
				"minimumTlsVersion":        "TLS1_2",
				"supportsHttpsTrafficOnly": true,
			},
			CreatedTime: createdTime,
			ChangedTime: changedTime,
		},
		{
			ID:             baseID + "/Microsoft.KeyVault/vaults/demo-" + resourceGroupName + "-kv",
			Name:           "demo-" + resourceGroupName + "-kv",
			Type:           "Microsoft.KeyVault/vaults",
			Location:       "eastus",
			ResourceGroup:  resourceGroupName,
			SubscriptionID: subscriptionID,
			Tags: map[string]string{
				"Purpose": "secrets",
			},
			Properties: map[string]interface{}{
				"sku": map[string]interface{}{
					"family": "A",
					"name":   "standard",
				},
				"tenantId":                  subscriptionID,
				"enableRbacAuthorization":   true,
				"enableSoftDelete":          true,
				"softDeleteRetentionInDays": 90,
			},
			CreatedTime: createdTime,
			ChangedTime: changedTime,
		},
	}

	// Add VM for web resource groups
	if contains(resourceGroupName, "web") {
		resources = append(resources, &domain.Resource{
			ID:             baseID + "/Microsoft.Compute/virtualMachines/demo-web-vm-01",
			Name:           "demo-web-vm-01",
			Type:           "Microsoft.Compute/virtualMachines",
			Location:       "westus2",
			ResourceGroup:  resourceGroupName,
			SubscriptionID: subscriptionID,
			Tags: map[string]string{
				"Purpose": "web-server",
				"OS":      "Linux",
			},
			Properties: map[string]interface{}{
				"hardwareProfile": map[string]interface{}{
					"vmSize": "Standard_B2s",
				},
				"storageProfile": map[string]interface{}{
					"osDisk": map[string]interface{}{
						"osType":       "Linux",
						"createOption": "FromImage",
						"caching":      "ReadWrite",
						"managedDisk":  map[string]interface{}{"storageAccountType": "Premium_LRS"},
						"diskSizeGB":   30,
					},
				},
				"osProfile": map[string]interface{}{
					"computerName":  "demo-web-01",
					"adminUsername": "azureuser",
				},
				"networkProfile": map[string]interface{}{
					"networkInterfaces": []map[string]interface{}{
						{"id": baseID + "/Microsoft.Network/networkInterfaces/demo-web-vm-01-nic"},
					},
				},
			},
			CreatedTime: createdTime,
			ChangedTime: changedTime,
		})
	}

	// Add SQL Database for analytics resource groups
	if contains(resourceGroupName, "analytics") {
		resources = append(resources, &domain.Resource{
			ID:             baseID + "/Microsoft.Sql/servers/demo-analytics-sql/databases/warehouse",
			Name:           "warehouse",
			Type:           "Microsoft.Sql/servers/databases",
			Location:       "eastus",
			ResourceGroup:  resourceGroupName,
			SubscriptionID: subscriptionID,
			Tags: map[string]string{
				"Purpose": "analytics",
			},
			Properties: map[string]interface{}{
				"collation":                "SQL_Latin1_General_CP1_CI_AS",
				"maxSizeBytes":             10737418240,
				"status":                   "Online",
				"databaseId":               "demo-db-id",
				"creationDate":             createdTime,
				"defaultSecondaryLocation": "westus",
				"readScale":                "Disabled",
				"zoneRedundant":            false,
			},
			CreatedTime: createdTime,
			ChangedTime: changedTime,
		})
	}

	// Add Load Balancer for networking resource groups
	if contains(resourceGroupName, "networking") {
		resources = append(resources, &domain.Resource{
			ID:             baseID + "/Microsoft.Network/loadBalancers/demo-public-lb",
			Name:           "demo-public-lb",
			Type:           "Microsoft.Network/loadBalancers",
			Location:       "southeastasia",
			ResourceGroup:  resourceGroupName,
			SubscriptionID: subscriptionID,
			Tags: map[string]string{
				"Purpose": "public-access",
			},
			Properties: map[string]interface{}{
				"sku": map[string]interface{}{
					"name": "Standard",
					"tier": "Regional",
				},
				"frontendIPConfigurations": []map[string]interface{}{
					{
						"name": "LoadBalancerFrontEnd",
						"properties": map[string]interface{}{
							"privateIPAllocationMethod": "Dynamic",
							"publicIPAddress": map[string]interface{}{
								"id": baseID + "/Microsoft.Network/publicIPAddresses/demo-public-ip",
							},
						},
					},
				},
				"backendAddressPools": []map[string]interface{}{
					{"name": "BackendPool1"},
				},
				"loadBalancingRules": []map[string]interface{}{
					{
						"name": "HTTP",
						"properties": map[string]interface{}{
							"frontendPort":         80,
							"backendPort":          80,
							"enableFloatingIP":     false,
							"idleTimeoutInMinutes": 4,
							"protocol":             "Tcp",
							"loadDistribution":     "Default",
						},
					},
				},
			},
			CreatedTime: createdTime,
			ChangedTime: changedTime,
		})
	}

	return resources
}

// Helper function to extract subscription ID from resource group ID
func subIDFromRGID(rgID string) string {
	// Resource ID format: /subscriptions/{sub}/resourceGroups/{name}
	// Extract subscription ID between /subscriptions/ and next /
	parts := splitString(rgID, '/')
	for i, part := range parts {
		if part == "subscriptions" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func splitString(s string, sep rune) []string {
	var parts []string
	current := ""
	for _, char := range s {
		if char == sep {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// GetUser returns the demo user
func (d *DemoData) GetUser() *domain.User {
	return d.User
}

// GetSubscriptions returns all demo subscriptions
func (d *DemoData) GetSubscriptions() []*domain.Subscription {
	return d.Subscriptions
}

// GetSubscription returns a specific subscription by ID
func (d *DemoData) GetSubscription(id string) *domain.Subscription {
	for _, sub := range d.Subscriptions {
		if sub.ID == id {
			return sub
		}
	}
	return nil
}

// GetResourceGroups returns resource groups for a subscription
func (d *DemoData) GetResourceGroups(subscriptionID string) []*domain.ResourceGroup {
	return d.ResourceGroups[subscriptionID]
}

// GetResourceGroup returns a specific resource group by name
func (d *DemoData) GetResourceGroup(subscriptionID, name string) *domain.ResourceGroup {
	for _, rg := range d.ResourceGroups[subscriptionID] {
		if rg.Name == name {
			return rg
		}
	}
	return nil
}

// GetResources returns resources for a resource group
func (d *DemoData) GetResources(resourceGroupName string) []*domain.Resource {
	return d.Resources[resourceGroupName]
}

// GetResource returns a specific resource by ID
func (d *DemoData) GetResource(resourceID string) *domain.Resource {
	for _, resources := range d.Resources {
		for _, res := range resources {
			if res.ID == resourceID {
				return res
			}
		}
	}
	return nil
}

// Large demo data constants
const (
	numLargeSubscriptions   = 15
	numResourceGroupsPerSub = 20
	numResourcesPerRG       = 15
)

// Subscription configurations for large demo
var largeSubscriptionConfigs = []struct {
	Name  string
	State string
}{
	{"Prod-East", "Enabled"},
	{"Prod-West", "Enabled"},
	{"Prod-Central", "Enabled"},
	{"Dev-East", "Enabled"},
	{"Dev-West", "Enabled"},
	{"Staging", "Enabled"},
	{"QA-Environment", "Enabled"},
	{"Testing", "Enabled"},
	{"Sandbox", "Enabled"},
	{"Demo-Tenant", "Enabled"},
	{"Production-UK", "Enabled"},
	{"Production-APAC", "Enabled"},
	{"Development-Experimental", "Enabled"},
	{"Training", "Enabled"},
	{"Pre-Production", "Enabled"},
}

// Resource group types/purposes
var resourceGroupTypes = []string{
	"networking",
	"web-apps",
	"databases",
	"analytics",
	"storage",
	"compute",
	"security",
	"monitoring",
	"identity",
	"containers",
	"integration",
	"backup",
	"devtools",
	"shared-services",
	"workloads",
	"frontend",
	"api-gateways",
	"message-queues",
	"caching",
	"ai-ml",
}

// Azure resource types for large demo (realistic mix)
var resourceTypesForDemo = []struct {
	Type       string
	Locations  []string
	Properties map[string]interface{}
}{
	{
		Type:      "Microsoft.Compute/virtualMachines",
		Locations: []string{"eastus", "westus2", "westeurope", "southeastasia"},
		Properties: map[string]interface{}{
			"hardwareProfile": map[string]interface{}{"vmSize": "Standard_D2s_v3"},
			"storageProfile": map[string]interface{}{
				"osDisk": map[string]interface{}{
					"osType": "Linux", "createOption": "FromImage",
					"managedDisk": map[string]interface{}{"storageAccountType": "Premium_LRS"},
				},
			},
		},
	},
	{
		Type:      "Microsoft.Storage/storageAccounts",
		Locations: []string{"eastus", "westus2", "westeurope"},
		Properties: map[string]interface{}{
			"accessTier": "Hot", "minimumTlsVersion": "TLS1_2",
			"supportsHttpsTrafficOnly": true,
		},
	},
	{
		Type:      "Microsoft.KeyVault/vaults",
		Locations: []string{"eastus", "westus2", "westeurope"},
		Properties: map[string]interface{}{
			"sku":                       map[string]interface{}{"family": "A", "name": "standard"},
			"enableRbacAuthorization":   true,
			"enableSoftDelete":          true,
			"softDeleteRetentionInDays": 90,
		},
	},
	{
		Type:      "Microsoft.Sql/servers/databases",
		Locations: []string{"eastus", "westus2"},
		Properties: map[string]interface{}{
			"collation":    "SQL_Latin1_General_CP1_CI_AS",
			"maxSizeBytes": 10737418240,
			"status":       "Online",
		},
	},
	{
		Type:      "Microsoft.Network/virtualNetworks",
		Locations: []string{"eastus", "westus2", "westeurope", "southeastasia"},
		Properties: map[string]interface{}{
			"addressSpace": map[string]interface{}{
				"addressPrefixes": []string{"10.0.0.0/16"},
			},
		},
	},
	{
		Type:      "Microsoft.Network/networkSecurityGroups",
		Locations: []string{"eastus", "westus2", "westeurope"},
		Properties: map[string]interface{}{
			"securityRules": []map[string]interface{}{
				{
					"name": "AllowSSH",
					"properties": map[string]interface{}{
						"protocol": "Tcp", "sourcePortRange": "*",
						"destinationPortRange": "22", "access": "Allow",
						"priority": 100, "direction": "Inbound",
					},
				},
			},
		},
	},
	{
		Type:      "Microsoft.Network/loadBalancers",
		Locations: []string{"eastus", "westus2"},
		Properties: map[string]interface{}{
			"sku": map[string]interface{}{"name": "Standard", "tier": "Regional"},
		},
	},
	{
		Type:      "Microsoft.Network/publicIPAddresses",
		Locations: []string{"eastus", "westus2", "westeurope"},
		Properties: map[string]interface{}{
			"publicIPAllocationMethod": "Static",
			"sku":                      map[string]interface{}{"name": "Standard"},
		},
	},
	{
		Type:      "Microsoft.Web/sites",
		Locations: []string{"eastus", "westus2", "westeurope"},
		Properties: map[string]interface{}{
			"enabled":               true,
			"httpsOnly":             true,
			"clientAffinityEnabled": false,
			"reserved":              false,
		},
	},
	{
		Type:      "Microsoft.Web/serverFarms",
		Locations: []string{"eastus", "westus2"},
		Properties: map[string]interface{}{
			"sku": map[string]interface{}{
				"name": "P1v2", "tier": "Premium", "size": "P1v2",
			},
		},
	},
	{
		Type:      "Microsoft.ContainerService/managedClusters",
		Locations: []string{"eastus", "westus2", "westeurope"},
		Properties: map[string]interface{}{
			"kubernetesVersion": "1.28.0",
			"dnsPrefix":         "aks-cluster",
			"agentPoolProfiles": []map[string]interface{}{
				{
					"name":   "nodepool1",
					"count":  3,
					"vmSize": "Standard_D2s_v3",
					"osType": "Linux",
				},
			},
		},
	},
	{
		Type:      "Microsoft.ContainerRegistry/registries",
		Locations: []string{"eastus", "westus2"},
		Properties: map[string]interface{}{
			"sku":              map[string]interface{}{"name": "Standard"},
			"adminUserEnabled": true,
		},
	},
	{
		Type:      "Microsoft.OperationalInsights/workspaces",
		Locations: []string{"eastus", "westus2"},
		Properties: map[string]interface{}{
			"sku":             map[string]interface{}{"name": "PerGB2018"},
			"retentionInDays": 30,
		},
	},
	{
		Type:      "Microsoft.Insights/components",
		Locations: []string{"eastus", "westus2"},
		Properties: map[string]interface{}{
			"Application_Type": "web",
			"RetentionInDays":  90,
		},
	},
	{
		Type:      "Microsoft.EventHub/namespaces",
		Locations: []string{"eastus", "westus2"},
		Properties: map[string]interface{}{
			"sku": map[string]interface{}{
				"name": "Standard", "tier": "Standard", "capacity": 1,
			},
		},
	},
}

// NewLargeDemoData creates a comprehensive demo dataset for realistic testing
// 10 subscriptions × 15 resource groups × 15 resources = 2,250 total resources
func NewLargeDemoData() *DemoData {
	data := &DemoData{
		User: &domain.User{
			DisplayName:       "Demo Administrator",
			UserPrincipalName: "demo.admin@contoso.com",
			Type:              "user",
			TenantID:          "00000000-0000-0000-0000-000000000000",
		},
		Subscriptions:  createLargeDemoSubscriptions(),
		ResourceGroups: make(map[string][]*domain.ResourceGroup),
		Resources:      make(map[string][]*domain.Resource),
	}

	// Create resource groups for each subscription
	for _, sub := range data.Subscriptions {
		data.ResourceGroups[sub.ID] = createLargeDemoResourceGroups(sub.ID, sub.Name)
	}

	// Create resources for each resource group
	for _, rgs := range data.ResourceGroups {
		for _, rg := range rgs {
			data.Resources[rg.Name] = createLargeDemoResources(subIDFromRGID(rg.ID), rg)
		}
	}

	return data
}

func createLargeDemoSubscriptions() []*domain.Subscription {
	subs := make([]*domain.Subscription, numLargeSubscriptions)
	for i := 0; i < numLargeSubscriptions; i++ {
		config := largeSubscriptionConfigs[i]
		subs[i] = &domain.Subscription{
			ID:       fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1),
			Name:     config.Name,
			State:    config.State,
			TenantID: "00000000-0000-0000-0000-000000000000",
		}
	}
	return subs
}

func createLargeDemoResourceGroups(subscriptionID string, subscriptionName string) []*domain.ResourceGroup {
	rgs := make([]*domain.ResourceGroup, numResourceGroupsPerSub)
	prefix := getEnvironmentPrefix(subscriptionName)

	locations := []string{"eastus", "westus2", "westeurope", "southeastasia", "northeurope"}
	teams := []string{"platform", "data", "security", "devops", "backend", "frontend", "infrastructure", "sre", "compliance"}
	costCenters := []string{"IT-001", "IT-002", "IT-003", "ENG-001", "ENG-002", "OPS-001", "SEC-001"}

	for i := 0; i < numResourceGroupsPerSub; i++ {
		rgType := resourceGroupTypes[i%len(resourceGroupTypes)]
		location := locations[i%len(locations)]
		team := teams[i%len(teams)]
		costCenter := costCenters[i%len(costCenters)]

		rgs[i] = &domain.ResourceGroup{
			Name:              fmt.Sprintf("rg-%s-%s-%02d", prefix, rgType, i+1),
			Location:          location,
			ID:                fmt.Sprintf("/subscriptions/%s/resourceGroups/rg-%s-%s-%02d", subscriptionID, prefix, rgType, i+1),
			ProvisioningState: "Succeeded",
			Tags: map[string]string{
				"Environment": prefix,
				"Team":        team,
				"CostCenter":  costCenter,
				"Project":     fmt.Sprintf("project-%s", rgType),
			},
			SubscriptionID: subscriptionID,
		}
	}
	return rgs
}

func getEnvironmentPrefix(subName string) string {
	switch {
	case contains(subName, "Prod"):
		return "prod"
	case contains(subName, "Dev"):
		return "dev"
	case contains(subName, "Staging"):
		return "stg"
	case contains(subName, "QA"):
		return "qa"
	case contains(subName, "Testing"):
		return "test"
	case contains(subName, "Sandbox"):
		return "sandbox"
	default:
		return "demo"
	}
}

func createLargeDemoResources(subscriptionID string, rg *domain.ResourceGroup) []*domain.Resource {
	createdTime := time.Now().AddDate(0, -6, -int(subscriptionID[35]-'0')*10).Format(time.RFC3339)
	changedTime := time.Now().AddDate(0, -1, -int(subscriptionID[35]-'0')*5).Format(time.RFC3339)

	resources := make([]*domain.Resource, numResourcesPerRG)
	baseID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers", subscriptionID, rg.Name)

	// Generate varied purposes based on resource group type
	purposes := generatePurposesForRGType(rg.Name)

	for i := 0; i < numResourcesPerRG; i++ {
		// Pick a resource type (cycle through available types)
		resTypeConfig := resourceTypesForDemo[i%len(resourceTypesForDemo)]
		location := resTypeConfig.Locations[i%len(resTypeConfig.Locations)]

		// Generate unique name
		resName := fmt.Sprintf("%s-%s-%02d", rg.Name[3:], getShortResourceTypeName(resTypeConfig.Type), i+1)

		// Build resource ID
		resID := fmt.Sprintf("%s/%s/%s", baseID, resTypeConfig.Type, resName)

		resources[i] = &domain.Resource{
			ID:             resID,
			Name:           resName,
			Type:           resTypeConfig.Type,
			Location:       location,
			ResourceGroup:  rg.Name,
			SubscriptionID: subscriptionID,
			Tags: map[string]string{
				"Purpose":   purposes[i%len(purposes)],
				"ManagedBy": "terraform",
			},
			Properties:  copyProperties(resTypeConfig.Properties),
			CreatedTime: createdTime,
			ChangedTime: changedTime,
		}
	}

	return resources
}

func getShortResourceTypeName(fullType string) string {
	parts := splitString(fullType, '/')
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// Shorten common names
		shortNames := map[string]string{
			"virtualMachines":       "vm",
			"storageAccounts":       "sa",
			"vaults":                "kv",
			"databases":             "db",
			"virtualNetworks":       "vnet",
			"networkSecurityGroups": "nsg",
			"loadBalancers":         "lb",
			"publicIPAddresses":     "pip",
			"sites":                 "app",
			"serverFarms":           "plan",
			"managedClusters":       "aks",
			"registries":            "acr",
			"workspaces":            "law",
			"components":            "ai",
			"namespaces":            "evh",
		}
		if short, ok := shortNames[lastPart]; ok {
			return short
		}
		return lastPart
	}
	return "res"
}

func copyProperties(props map[string]interface{}) map[string]interface{} {
	if props == nil {
		return nil
	}
	copy := make(map[string]interface{}, len(props))
	for k, v := range props {
		copy[k] = v
	}
	return copy
}

func generatePurposesForRGType(rgName string) []string {
	if contains(rgName, "networking") {
		return []string{"vpc", "firewall", "load-balancing", "vpn", "dns", "cdn", "expressroute", "ddos"}
	}
	if contains(rgName, "web-apps") {
		return []string{"frontend", "api", "microservice", "static-site", "spa", "mobile-backend"}
	}
	if contains(rgName, "databases") {
		return []string{"oltp", "analytics", "cache", "search", "nosql", "timeseries", "graph"}
	}
	if contains(rgName, "analytics") {
		return []string{"data-warehouse", "lake", "stream", "bi", "ml", "reporting"}
	}
	if contains(rgName, "storage") {
		return []string{"blob", "files", "queues", "tables", "archive", "backup"}
	}
	if contains(rgName, "compute") {
		return []string{"web-server", "app-server", "batch", "hpc", "rendering", "ci-runner"}
	}
	if contains(rgName, "security") {
		return []string{"secrets", "certificates", "waf", "ids", "siem", "key-mgmt"}
	}
	if contains(rgName, "monitoring") {
		return []string{"logs", "metrics", "alerts", "dashboards", "tracing", "apm"}
	}
	if contains(rgName, "identity") {
		return []string{"auth", "mfa", "rbac", "federation", "sso", "directory"}
	}
	if contains(rgName, "containers") {
		return []string{"orchestration", "registry", "serverless", "mesh", "build"}
	}
	if contains(rgName, "integration") {
		return []string{"messaging", "api-mgmt", "workflow", "event-grid", "relay"}
	}
	if contains(rgName, "backup") {
		return []string{"vm-backup", "sql-backup", "file-backup", "dr", "archive"}
	}
	if contains(rgName, "devtools") {
		return []string{"build", "test", "deploy", "repo", "artifact"}
	}
	if contains(rgName, "shared-services") {
		return []string{"domain", "dns", "ntp", "logging", "monitoring"}
	}
	if contains(rgName, "workloads") {
		return []string{"erp", "crm", "hrms", "finance", "inventory"}
	}
	if contains(rgName, "frontend") {
		return []string{"react", "vue", "angular", "static", "cdn"}
	}
	if contains(rgName, "api-gateways") {
		return []string{"rest", "graphql", "grpc", "websocket", "webhook"}
	}
	if contains(rgName, "message-queues") {
		return []string{"orders", "notifications", "events", "tasks", "emails"}
	}
	if contains(rgName, "caching") {
		return []string{"session", "query", "object", "cdn", "rate-limit"}
	}
	if contains(rgName, "ai-ml") {
		return []string{"training", "inference", "nlp", "vision", "forecasting"}
	}
	return []string{"general", "shared", "utility", "common"}
}
