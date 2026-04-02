package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/utils"
)

// ResourcesClient wraps the Azure resources client
type ResourcesClient struct {
	client          *armresources.Client
	subscriptionID  string
	apiVersionCache *APIVersionCache
}

// NewResourcesClient creates a new resources client for a subscription
func NewResourcesClient(client *Client, subscriptionID string) (*ResourcesClient, error) {
	resClient, err := armresources.NewClient(subscriptionID, client.Credential(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resources client: %w", err)
	}

	// Create API version cache
	apiCache, err := NewAPIVersionCache(client, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create API version cache: %w", err)
	}

	return &ResourcesClient{
		client:          resClient,
		subscriptionID:  subscriptionID,
		apiVersionCache: apiCache,
	}, nil
}

// ListResourcesByResourceGroup retrieves all resources in a specific resource group
func (c *ResourcesClient) ListResourcesByResourceGroup(ctx context.Context, resourceGroupName string) ([]*domain.Resource, error) {
	record := utils.StartAPITimer("ListResourcesByResourceGroup")

	// Create filter to get resources only in the specified resource group
	filter := fmt.Sprintf("resourceGroup eq '%s'", resourceGroupName)
	// Expand to get additional properties like createdTime, changedTime, provisioningState
	expand := "createdTime,changedTime,provisioningState"
	options := &armresources.ClientListOptions{
		Filter: &filter,
		Expand: &expand,
	}

	pager := c.client.NewListPager(options)

	var resources []*domain.Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			record(err)
			return nil, fmt.Errorf("failed to list resources in resource group %s: %w", resourceGroupName, err)
		}

		for _, res := range page.Value {
			if res != nil {
				resource := &domain.Resource{
					ID:             deref(res.ID),
					Name:           deref(res.Name),
					Type:           deref(res.Type),
					Location:       deref(res.Location),
					ResourceGroup:  resourceGroupName,
					SubscriptionID: c.subscriptionID,
					Tags:           convertTags(res.Tags),
					Properties:     extractProperties(res),
				}
				// Set created/changed times if available
				if res.CreatedTime != nil {
					resource.CreatedTime = res.CreatedTime.Format(time.RFC3339)
				}
				if res.ChangedTime != nil {
					resource.ChangedTime = res.ChangedTime.Format(time.RFC3339)
				}
				resources = append(resources, resource)
			}
		}
	}

	record(nil)
	if utils.IsDebugEnabled() {
		utils.Log("[API] ListResourcesByResourceGroup loaded %d resources", len(resources))
	}
	return resources, nil
}

// GetResource retrieves a specific resource by ID with full properties
func (c *ResourcesClient) GetResource(ctx context.Context, resourceID string, resourceType string) (*domain.Resource, error) {
	defer utils.StartAPITimer("GetResource")(nil)

	// Get the latest API version for this resource type from Azure
	apiVersion, err := c.apiVersionCache.GetLatestAPIVersion(ctx, resourceType)
	if err != nil {
		utils.Log("GetResource: Failed to get API version, using default: %v", err)
		apiVersion = "2024-03-01" // Fallback to recent stable version
	}

	utils.Log("GetResource: Using API version %s", apiVersion)

	resp, err := c.client.GetByID(ctx, resourceID, apiVersion, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	// Parse resource ID to extract resource group
	resourceGroup := ""
	parts := parseResourceID(resourceID)
	if rg, ok := parts["resourceGroup"]; ok {
		resourceGroup = rg
	}

	resource := &domain.Resource{
		ID:             deref(resp.ID),
		Name:           deref(resp.Name),
		Type:           deref(resp.Type),
		Location:       deref(resp.Location),
		SubscriptionID: c.subscriptionID,
		ResourceGroup:  resourceGroup,
		Tags:           convertTags(resp.Tags),
		Properties:     extractPropertiesGeneric(resp.Properties),
	}

	return resource, nil
}

// parseResourceID parses a resource ID into its components
func parseResourceID(resourceID string) map[string]string {
	result := make(map[string]string)
	// Resource ID format: /subscriptions/{sub}/resourceGroups/{rg}/providers/{provider}/{type}/{name}
	// Split by /
	parts := make([]string, 0)
	current := ""
	for _, char := range resourceID {
		if char == '/' {
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

	// Parse parts
	for i := 0; i < len(parts)-1; i++ {
		switch parts[i] {
		case "subscriptions":
			if i+1 < len(parts) {
				result["subscription"] = parts[i+1]
			}
		case "resourceGroups":
			if i+1 < len(parts) {
				result["resourceGroup"] = parts[i+1]
			}
		case "providers":
			if i+1 < len(parts) {
				result["provider"] = parts[i+1]
			}
		}
	}

	return result
}

// extractProperties extracts additional properties from the resource response
func extractProperties(res *armresources.GenericResourceExpanded) map[string]interface{} {
	props := make(map[string]interface{})

	if res == nil {
		return props
	}

	// Note: createdTime and changedTime are extracted as top-level fields
	// in the Resource struct, not in Properties

	// Extract standard expanded fields if available (except createdTime/changedTime)
	if res.ManagedBy != nil {
		props["managedBy"] = *res.ManagedBy
	}
	if res.Kind != nil {
		props["kind"] = *res.Kind
	}

	// Extract the main Properties field which contains resource-specific data
	if res.Properties != nil {
		// Try to marshal and unmarshal to get all properties
		data, err := json.Marshal(res.Properties)
		if err == nil {
			var properties map[string]interface{}
			if err := json.Unmarshal(data, &properties); err == nil {
				// Merge into props
				for k, v := range properties {
					props[k] = v
				}
			}
		}
	}

	return props
}

// extractPropertiesGeneric extracts properties from the resource response by ID
func extractPropertiesGeneric(properties interface{}) map[string]interface{} {
	props := make(map[string]interface{})

	if properties != nil {
		data, err := json.Marshal(properties)
		if err == nil {
			var result map[string]interface{}
			if err := json.Unmarshal(data, &result); err == nil {
				return result
			}
		}
	}

	return props
}
