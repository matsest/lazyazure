package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/utils"
)

// ResourceGraphClient provides cross-subscription resource queries using Azure Resource Graph
type ResourceGraphClient struct {
	client *armresourcegraph.Client
}

// NewResourceGraphClient creates a new Resource Graph client
func NewResourceGraphClient(azClient *Client) (*ResourceGraphClient, error) {
	client, err := armresourcegraph.NewClient(azClient.Credential(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource graph client: %w", err)
	}

	return &ResourceGraphClient{
		client: client,
	}, nil
}

// SearchResources searches for resources across subscriptions using Azure Resource Graph
// Parameters:
//   - nameFilter: substring match on resource name (empty = no filter)
//   - typeFilter: exact match on resource type e.g., "Microsoft.Compute/virtualMachines" (empty = no filter)
//   - subscriptionIDs: limit to these subscriptions (empty = query all accessible subscriptions)
//
// Returns resources with basic info (no properties - use GetResource for full details)
func (c *ResourceGraphClient) SearchResources(ctx context.Context, nameFilter, typeFilter string, subscriptionIDs []string) ([]*domain.Resource, error) {
	record := utils.StartAPITimer("ResourceGraph.SearchResources")

	// Build KQL query
	query := c.buildSearchQuery(nameFilter, typeFilter)

	utils.Log("ResourceGraph query: %s", query)

	// Build request
	request := armresourcegraph.QueryRequest{
		Query: to.Ptr(query),
		Options: &armresourcegraph.QueryRequestOptions{
			ResultFormat: to.Ptr(armresourcegraph.ResultFormatObjectArray),
		},
	}

	// Add subscription filter if specified
	if len(subscriptionIDs) > 0 {
		subs := make([]*string, len(subscriptionIDs))
		for i, id := range subscriptionIDs {
			subs[i] = to.Ptr(id)
		}
		request.Subscriptions = subs
	}

	// Execute query
	resp, err := c.client.Resources(ctx, request, nil)
	if err != nil {
		record(err)
		return nil, fmt.Errorf("resource graph query failed: %w", err)
	}

	// Parse results
	resources, err := c.parseQueryResponse(resp.QueryResponse)
	if err != nil {
		record(err)
		return nil, fmt.Errorf("failed to parse resource graph response: %w", err)
	}

	record(nil)
	utils.Log("ResourceGraph returned %d resources", len(resources))

	return resources, nil
}

// ListResourcesBySubscription lists all resources in a specific subscription using Resource Graph
func (c *ResourceGraphClient) ListResourcesBySubscription(ctx context.Context, subscriptionID string) ([]*domain.Resource, error) {
	return c.SearchResources(ctx, "", "", []string{subscriptionID})
}

// ListResourcesByType lists all resources of a specific type across subscriptions
func (c *ResourceGraphClient) ListResourcesByType(ctx context.Context, resourceType string, subscriptionIDs []string) ([]*domain.Resource, error) {
	return c.SearchResources(ctx, "", resourceType, subscriptionIDs)
}

// buildSearchQuery constructs a KQL query for the given filters
func (c *ResourceGraphClient) buildSearchQuery(nameFilter, typeFilter string) string {
	var parts []string

	// Start with Resources table
	parts = append(parts, "Resources")

	// Add type filter if specified (exact match, lowercase for Azure Resource Graph)
	if typeFilter != "" {
		// Azure Resource Graph stores types in lowercase
		// Use == for exact match which is faster than =~ (case-insensitive)
		escapedType := strings.ReplaceAll(strings.ToLower(typeFilter), "'", "''")
		parts = append(parts, fmt.Sprintf("| where type == '%s'", escapedType))
	}

	// Add name filter if specified (contains, case-insensitive)
	if nameFilter != "" {
		// Escape single quotes in name filter
		escapedName := strings.ReplaceAll(nameFilter, "'", "''")
		parts = append(parts, fmt.Sprintf("| where name contains '%s'", escapedName))
	}

	// Project the fields we need
	parts = append(parts, "| project id, name, type, location, resourceGroup, subscriptionId, tags")

	// Order by name for consistent results
	parts = append(parts, "| order by name asc")

	// Limit results to prevent overwhelming the UI
	parts = append(parts, "| limit 1000")

	return strings.Join(parts, " ")
}

// parseQueryResponse converts the Resource Graph response to domain.Resource objects
func (c *ResourceGraphClient) parseQueryResponse(resp armresourcegraph.QueryResponse) ([]*domain.Resource, error) {
	if resp.Data == nil {
		return []*domain.Resource{}, nil
	}

	// Data is an array of objects when ResultFormat is ObjectArray
	dataArray, ok := resp.Data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected data format: expected []interface{}, got %T", resp.Data)
	}

	resources := make([]*domain.Resource, 0, len(dataArray))

	for _, item := range dataArray {
		resourceMap, ok := item.(map[string]interface{})
		if !ok {
			utils.Log("ResourceGraph: skipping invalid item type %T", item)
			continue
		}

		resource := c.mapToResource(resourceMap)
		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// mapToResource converts a map from Resource Graph response to a domain.Resource
func (c *ResourceGraphClient) mapToResource(data map[string]interface{}) *domain.Resource {
	resource := &domain.Resource{}

	if id, ok := data["id"].(string); ok {
		resource.ID = id
	}
	if name, ok := data["name"].(string); ok {
		resource.Name = name
	}
	if resourceType, ok := data["type"].(string); ok {
		resource.Type = resourceType
	}
	if location, ok := data["location"].(string); ok {
		resource.Location = location
	}
	if rg, ok := data["resourceGroup"].(string); ok {
		resource.ResourceGroup = rg
	}
	if subID, ok := data["subscriptionId"].(string); ok {
		resource.SubscriptionID = subID
	}

	// Parse tags
	if tagsData, ok := data["tags"].(map[string]interface{}); ok {
		resource.Tags = make(map[string]string)
		for k, v := range tagsData {
			if strVal, ok := v.(string); ok {
				resource.Tags[k] = strVal
			}
		}
	}

	// Validate we have at least ID and Name
	if resource.ID == "" || resource.Name == "" {
		return nil
	}

	return resource
}

// GetDistinctResourceTypes returns all distinct resource types across subscriptions
// Useful for populating a type filter dropdown
func (c *ResourceGraphClient) GetDistinctResourceTypes(ctx context.Context, subscriptionIDs []string) ([]string, error) {
	record := utils.StartAPITimer("ResourceGraph.GetDistinctResourceTypes")

	query := "Resources | summarize count() by type | order by type asc"

	request := armresourcegraph.QueryRequest{
		Query: to.Ptr(query),
		Options: &armresourcegraph.QueryRequestOptions{
			ResultFormat: to.Ptr(armresourcegraph.ResultFormatObjectArray),
		},
	}

	if len(subscriptionIDs) > 0 {
		subs := make([]*string, len(subscriptionIDs))
		for i, id := range subscriptionIDs {
			subs[i] = to.Ptr(id)
		}
		request.Subscriptions = subs
	}

	resp, err := c.client.Resources(ctx, request, nil)
	if err != nil {
		record(err)
		return nil, fmt.Errorf("resource graph query failed: %w", err)
	}

	record(nil)

	// Parse the response
	dataArray, ok := resp.Data.([]interface{})
	if !ok {
		return []string{}, nil
	}

	types := make([]string, 0, len(dataArray))
	for _, item := range dataArray {
		if itemMap, ok := item.(map[string]interface{}); ok {
			if typeStr, ok := itemMap["type"].(string); ok {
				types = append(types, typeStr)
			}
		}
	}

	utils.Log("ResourceGraph found %d distinct resource types", len(types))
	return types, nil
}
