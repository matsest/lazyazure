package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/gui"
)

// Client implements the same interface as azure.Client but returns demo data
type Client struct {
	data      *DemoData
	token     string
	startTime time.Time
	mode      string
}

// NewClient creates a new demo client with mock data (small dataset)
func NewClient() *Client {
	return &Client{
		data:      NewDemoData(),
		token:     "demo-token-for-demo-mode",
		startTime: time.Now(),
		mode:      "1",
	}
}

// NewClientWithMode creates a new demo client with specified mode
// Mode "1": Small dataset (2 subs, 4 RGs each, 2-4 resources each)
// Mode "2": Large dataset (10 subs, 15 RGs each, 15 resources each)
func NewClientWithMode(mode string) *Client {
	var data *DemoData
	if mode == "2" {
		data = NewLargeDemoData()
	} else {
		data = NewDemoData()
	}

	return &Client{
		data:      data,
		token:     "demo-token-for-demo-mode",
		startTime: time.Now(),
		mode:      mode,
	}
}

// GetUserInfo returns the demo user information
func (c *Client) GetUserInfo(ctx context.Context) (*domain.User, error) {
	// Simulate API delay
	simulateDelay(100, 200)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return c.data.GetUser(), nil
	}
}

// VerifyAuthentication always succeeds in demo mode
func (c *Client) VerifyAuthentication(ctx context.Context) error {
	// Simulate authentication check
	simulateDelay(50, 100)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// Credential returns a mock credential for demo mode
func (c *Client) Credential() azcore.TokenCredential {
	return &demoCredential{token: c.token}
}

// demoCredential implements azcore.TokenCredential for demo mode
type demoCredential struct {
	token string
}

// GetToken returns a mock access token
func (d *demoCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{
		Token:     d.token,
		ExpiresOn: time.Now().Add(time.Hour),
	}, nil
}

// Factory methods to implement gui.AzureClientFactory

// NewSubscriptionsClient creates a subscriptions client
func (c *Client) NewSubscriptionsClient() (gui.SubscriptionsClient, error) {
	return NewDemoSubscriptionsClient(c.data), nil
}

// NewResourceGroupsClient creates a resource groups client for a subscription
func (c *Client) NewResourceGroupsClient(subscriptionID string) (gui.ResourceGroupsClient, error) {
	return NewDemoResourceGroupsClient(c, subscriptionID), nil
}

// NewResourcesClient creates a resources client for a subscription
func (c *Client) NewResourcesClient(subscriptionID string) (gui.ResourcesClient, error) {
	return NewDemoResourcesClient(c, subscriptionID), nil
}

// NewResourceGraphClient creates a resource graph client for cross-subscription queries
func (c *Client) NewResourceGraphClient() (gui.ResourceGraphClient, error) {
	return NewDemoResourceGraphClient(c.data), nil
}

// DemoSubscriptionsClient wraps demo data for subscriptions
type DemoSubscriptionsClient struct {
	data *DemoData
}

// NewDemoSubscriptionsClient creates a new demo subscriptions client
func NewDemoSubscriptionsClient(data *DemoData) *DemoSubscriptionsClient {
	return &DemoSubscriptionsClient{data: data}
}

// ListSubscriptions returns all demo subscriptions
func (c *DemoSubscriptionsClient) ListSubscriptions(ctx context.Context) ([]*domain.Subscription, error) {
	simulateDelay(150, 300)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return c.data.GetSubscriptions(), nil
	}
}

// GetSubscription returns a specific demo subscription
func (c *DemoSubscriptionsClient) GetSubscription(ctx context.Context, subscriptionID string) (*domain.Subscription, error) {
	simulateDelay(100, 200)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		sub := c.data.GetSubscription(subscriptionID)
		if sub == nil {
			return nil, fmt.Errorf("subscription %s not found", subscriptionID)
		}
		return sub, nil
	}
}

// DemoResourceGroupsClient wraps demo data for resource groups
type DemoResourceGroupsClient struct {
	data           *DemoData
	subscriptionID string
}

// NewDemoResourceGroupsClient creates a new demo resource groups client
func NewDemoResourceGroupsClient(client *Client, subscriptionID string) *DemoResourceGroupsClient {
	return &DemoResourceGroupsClient{
		data:           client.data,
		subscriptionID: subscriptionID,
	}
}

// ListResourceGroups returns all demo resource groups for a subscription
func (c *DemoResourceGroupsClient) ListResourceGroups(ctx context.Context) ([]*domain.ResourceGroup, error) {
	simulateDelay(200, 400)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		rgs := c.data.GetResourceGroups(c.subscriptionID)
		if rgs == nil {
			return []*domain.ResourceGroup{}, nil
		}
		return rgs, nil
	}
}

// DemoResourcesClient wraps demo data for resources
type DemoResourcesClient struct {
	data           *DemoData
	subscriptionID string
}

// NewDemoResourcesClient creates a new demo resources client
func NewDemoResourcesClient(client *Client, subscriptionID string) *DemoResourcesClient {
	return &DemoResourcesClient{
		data:           client.data,
		subscriptionID: subscriptionID,
	}
}

// ListResourcesByResourceGroup returns all demo resources for a resource group
// Returns resources with basic info only (no properties) to simulate lazy loading
func (c *DemoResourcesClient) ListResourcesByResourceGroup(ctx context.Context, resourceGroupName string) ([]*domain.Resource, error) {
	simulateDelay(200, 500)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		resources := c.data.GetResources(resourceGroupName)
		if resources == nil {
			return []*domain.Resource{}, nil
		}

		// Create copies without properties to simulate list view behavior
		// Properties will be loaded when GetResource is called (on Enter press)
		var listResources []*domain.Resource
		for _, res := range resources {
			listRes := &domain.Resource{
				ID:             res.ID,
				Name:           res.Name,
				Type:           res.Type,
				Location:       res.Location,
				ResourceGroup:  res.ResourceGroup,
				SubscriptionID: res.SubscriptionID,
				Tags:           res.Tags,
				// Properties is nil - will be loaded on Enter
				CreatedTime: res.CreatedTime,
				ChangedTime: res.ChangedTime,
			}
			listResources = append(listResources, listRes)
		}

		return listResources, nil
	}
}

// GetResource returns a specific demo resource by ID
func (c *DemoResourcesClient) GetResource(ctx context.Context, resourceID string, resourceType string) (*domain.Resource, error) {
	simulateDelay(150, 350)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		resource := c.data.GetResource(resourceID)
		if resource == nil {
			return nil, fmt.Errorf("resource %s not found", resourceID)
		}
		return resource, nil
	}
}

// DemoResourceGraphClient provides cross-subscription queries for demo data
type DemoResourceGraphClient struct {
	data *DemoData
}

// NewDemoResourceGraphClient creates a new demo resource graph client
func NewDemoResourceGraphClient(data *DemoData) *DemoResourceGraphClient {
	return &DemoResourceGraphClient{data: data}
}

// SearchResources searches for resources across all demo subscriptions
func (c *DemoResourceGraphClient) SearchResources(ctx context.Context, nameFilter, typeFilter string, subscriptionIDs []string) ([]*domain.Resource, error) {
	simulateDelay(100, 300)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var results []*domain.Resource

	// Get all resources from all subscriptions
	for rgName, resources := range c.data.Resources {
		for _, res := range resources {
			// Check subscription filter
			if len(subscriptionIDs) > 0 && !containsString(subscriptionIDs, res.SubscriptionID) {
				continue
			}

			// Check name filter (case-insensitive contains)
			if nameFilter != "" && !containsIgnoreCase(res.Name, nameFilter) {
				continue
			}

			// Check type filter (case-insensitive exact match)
			if typeFilter != "" && !equalsIgnoreCase(res.Type, typeFilter) {
				continue
			}

			// Create a copy with resource group info
			resCopy := &domain.Resource{
				ID:             res.ID,
				Name:           res.Name,
				Type:           res.Type,
				Location:       res.Location,
				ResourceGroup:  rgName,
				SubscriptionID: res.SubscriptionID,
				Tags:           res.Tags,
				CreatedTime:    res.CreatedTime,
				ChangedTime:    res.ChangedTime,
			}
			results = append(results, resCopy)
		}
	}

	return results, nil
}

// ListResourcesBySubscription lists all resources in a subscription
func (c *DemoResourceGraphClient) ListResourcesBySubscription(ctx context.Context, subscriptionID string) ([]*domain.Resource, error) {
	return c.SearchResources(ctx, "", "", []string{subscriptionID})
}

// ListResourcesByType lists resources of a specific type
func (c *DemoResourceGraphClient) ListResourcesByType(ctx context.Context, resourceType string, subscriptionIDs []string) ([]*domain.Resource, error) {
	return c.SearchResources(ctx, "", resourceType, subscriptionIDs)
}

// GetDistinctResourceTypes returns all distinct resource types in demo data
func (c *DemoResourceGraphClient) GetDistinctResourceTypes(ctx context.Context, subscriptionIDs []string) ([]string, error) {
	simulateDelay(50, 100)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	typeSet := make(map[string]bool)

	for _, resources := range c.data.Resources {
		for _, res := range resources {
			// Check subscription filter
			if len(subscriptionIDs) > 0 && !containsString(subscriptionIDs, res.SubscriptionID) {
				continue
			}
			typeSet[res.Type] = true
		}
	}

	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}

	return types, nil
}

// Helper functions for string matching
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func containsIgnoreCase(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func equalsIgnoreCase(a, b string) bool {
	return toLower(a) == toLower(b)
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func findSubstring(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// simulateDelay sleeps for a random duration between min and max milliseconds
// to simulate realistic API response times
func simulateDelay(minMs, maxMs int) {
	duration := time.Duration(minMs+(maxMs-minMs)/2) * time.Millisecond
	time.Sleep(duration)
}

// Ensure interface compatibility with azure.Client
// These type assertions will fail at compile time if interfaces don't match
var (
	// Note: These variables are used for compile-time interface verification
	// They won't be instantiated, just checked by the compiler
	_ interface {
		GetUserInfo(ctx context.Context) (*domain.User, error)
		VerifyAuthentication(ctx context.Context) error
		Credential() azcore.TokenCredential
	} = (*Client)(nil)

	_ interface {
		ListSubscriptions(ctx context.Context) ([]*domain.Subscription, error)
	} = (*DemoSubscriptionsClient)(nil)

	_ interface {
		ListResourceGroups(ctx context.Context) ([]*domain.ResourceGroup, error)
	} = (*DemoResourceGroupsClient)(nil)

	_ interface {
		ListResourcesByResourceGroup(ctx context.Context, resourceGroupName string) ([]*domain.Resource, error)
		GetResource(ctx context.Context, resourceID string, resourceType string) (*domain.Resource, error)
	} = (*DemoResourcesClient)(nil)

	_ interface {
		SearchResources(ctx context.Context, nameFilter, typeFilter string, subscriptionIDs []string) ([]*domain.Resource, error)
		ListResourcesBySubscription(ctx context.Context, subscriptionID string) ([]*domain.Resource, error)
		ListResourcesByType(ctx context.Context, resourceType string, subscriptionIDs []string) ([]*domain.Resource, error)
		GetDistinctResourceTypes(ctx context.Context, subscriptionIDs []string) ([]string, error)
	} = (*DemoResourceGraphClient)(nil)
)
