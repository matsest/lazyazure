package demo

import (
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
	prefix := "demo"
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
