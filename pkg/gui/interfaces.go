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

// AzureClientFactory creates resource-specific clients
type AzureClientFactory interface {
	NewSubscriptionsClient() (SubscriptionsClient, error)
	NewResourceGroupsClient(subscriptionID string) (ResourceGroupsClient, error)
	NewResourcesClient(subscriptionID string) (ResourcesClient, error)
}
