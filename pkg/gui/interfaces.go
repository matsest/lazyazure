package gui

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/matsest/lazyazure/pkg/domain"
)

// SubscriptionsClient provides subscription operations
type SubscriptionsClient interface {
	ListSubscriptions(ctx context.Context) ([]*domain.Subscription, error)
}

// ResourceGroupsClient provides resource group operations
type ResourceGroupsClient interface {
	ListResourceGroups(ctx context.Context) ([]*domain.ResourceGroup, error)
}

// ResourcesClient provides resource operations
type ResourcesClient interface {
	ListResourcesByResourceGroup(ctx context.Context, resourceGroupName string) ([]*domain.Resource, error)
	GetResource(ctx context.Context, resourceID string, resourceType string) (*domain.Resource, error)
}

// AzureClient combines all client capabilities
type AzureClient interface {
	GetUserInfo(ctx context.Context) (*domain.User, error)
	VerifyAuthentication(ctx context.Context) error
	Credential() azcore.TokenCredential
}

// ResourceGraphClient provides cross-subscription resource queries
type ResourceGraphClient interface {
	// SearchResources searches for resources across subscriptions
	// nameFilter: substring match on resource name (empty = no filter)
	// typeFilter: exact match on resource type e.g., "Microsoft.Compute/virtualMachines" (empty = no filter)
	// subscriptionIDs: limit to these subscriptions (empty = query all accessible)
	SearchResources(ctx context.Context, nameFilter, typeFilter string, subscriptionIDs []string) ([]*domain.Resource, error)

	// ListResourcesBySubscription lists all resources in a subscription
	ListResourcesBySubscription(ctx context.Context, subscriptionID string) ([]*domain.Resource, error)

	// ListResourcesByType lists resources of a specific type
	ListResourcesByType(ctx context.Context, resourceType string, subscriptionIDs []string) ([]*domain.Resource, error)

	// GetDistinctResourceTypes returns all distinct resource types in the user's subscriptions
	GetDistinctResourceTypes(ctx context.Context, subscriptionIDs []string) ([]string, error)
}

// AzureClientFactory creates resource-specific clients
type AzureClientFactory interface {
	NewSubscriptionsClient() (SubscriptionsClient, error)
	NewResourceGroupsClient(subscriptionID string) (ResourceGroupsClient, error)
	NewResourcesClient(subscriptionID string) (ResourcesClient, error)
	NewResourceGraphClient() (ResourceGraphClient, error)
}
