package azure

import (
	"github.com/matsest/lazyazure/pkg/gui"
)

// ClientFactory implements gui.AzureClientFactory for real Azure clients
type ClientFactory struct {
	client *Client
}

// NewClientFactory creates a new client factory for real Azure clients
func NewClientFactory(client *Client) *ClientFactory {
	return &ClientFactory{client: client}
}

// NewSubscriptionsClient creates a subscriptions client
func (f *ClientFactory) NewSubscriptionsClient() (gui.SubscriptionsClient, error) {
	return NewSubscriptionsClient(f.client)
}

// NewResourceGroupsClient creates a resource groups client for a subscription
func (f *ClientFactory) NewResourceGroupsClient(subscriptionID string) (gui.ResourceGroupsClient, error) {
	return NewResourceGroupsClient(f.client, subscriptionID)
}

// NewResourcesClient creates a resources client for a subscription
func (f *ClientFactory) NewResourcesClient(subscriptionID string) (gui.ResourcesClient, error) {
	return NewResourcesClient(f.client, subscriptionID)
}
