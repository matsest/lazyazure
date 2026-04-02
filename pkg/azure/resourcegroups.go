package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/utils"
)

// ResourceGroupsClient wraps the Azure resource groups client
type ResourceGroupsClient struct {
	client         *armresources.ResourceGroupsClient
	subscriptionID string
}

// NewResourceGroupsClient creates a new resource groups client for a subscription
func NewResourceGroupsClient(client *Client, subscriptionID string) (*ResourceGroupsClient, error) {
	rgClient, err := armresources.NewResourceGroupsClient(subscriptionID, client.Credential(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource groups client: %w", err)
	}

	return &ResourceGroupsClient{
		client:         rgClient,
		subscriptionID: subscriptionID,
	}, nil
}

// ListResourceGroups retrieves all resource groups in the subscription
func (c *ResourceGroupsClient) ListResourceGroups(ctx context.Context) ([]*domain.ResourceGroup, error) {
	record := utils.StartAPITimer("ListResourceGroups")

	pager := c.client.NewListPager(nil)

	var resourceGroups []*domain.ResourceGroup

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			record(err)
			return nil, fmt.Errorf("failed to list resource groups: %w", err)
		}

		for _, rg := range page.Value {
			if rg != nil {
				resourceGroups = append(resourceGroups, &domain.ResourceGroup{
					Name:              deref(rg.Name),
					Location:          deref(rg.Location),
					ID:                deref(rg.ID),
					ProvisioningState: deref(rg.Properties.ProvisioningState),
					Tags:              convertTags(rg.Tags),
					SubscriptionID:    c.subscriptionID,
				})
			}
		}
	}

	record(nil)
	if utils.IsDebugEnabled() {
		utils.Log("[API] ListResourceGroups loaded %d resource groups", len(resourceGroups))
	}
	return resourceGroups, nil
}

// convertTags converts Azure SDK tags map to a regular map
func convertTags(tags map[string]*string) map[string]string {
	result := make(map[string]string)
	for k, v := range tags {
		result[k] = deref(v)
	}
	return result
}
