package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/utils"
)

// SubscriptionsClient wraps the Azure subscriptions client
type SubscriptionsClient struct {
	client *armsubscriptions.Client
}

// NewSubscriptionsClient creates a new subscriptions client
func NewSubscriptionsClient(client *Client) (*SubscriptionsClient, error) {
	subClient, err := armsubscriptions.NewClient(client.Credential(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscriptions client: %w", err)
	}

	return &SubscriptionsClient{
		client: subClient,
	}, nil
}

// ListSubscriptions retrieves all subscriptions accessible to the user
func (c *SubscriptionsClient) ListSubscriptions(ctx context.Context) ([]*domain.Subscription, error) {
	record := utils.StartAPITimer("ListSubscriptions")

	pager := c.client.NewListPager(nil)

	var subscriptions []*domain.Subscription

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			record(err)
			return nil, fmt.Errorf("failed to list subscriptions: %w", err)
		}

		for _, sub := range page.Value {
			if sub != nil {
				subscriptions = append(subscriptions, &domain.Subscription{
					ID:       deref(sub.SubscriptionID),
					Name:     deref(sub.DisplayName),
					State:    string(*sub.State),
					TenantID: deref(sub.TenantID),
				})
			}
		}
	}

	record(nil)
	if utils.IsDebugEnabled() {
		utils.Log("[API] ListSubscriptions loaded %d subscriptions", len(subscriptions))
	}
	return subscriptions, nil
}

// GetSubscription retrieves a specific subscription by ID
func (c *SubscriptionsClient) GetSubscription(ctx context.Context, subscriptionID string) (*domain.Subscription, error) {
	defer utils.StartAPITimer("GetSubscription")(nil)

	resp, err := c.client.Get(ctx, subscriptionID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription %s: %w", subscriptionID, err)
	}

	return &domain.Subscription{
		ID:       deref(resp.SubscriptionID),
		Name:     deref(resp.DisplayName),
		State:    string(*resp.State),
		TenantID: deref(resp.TenantID),
	}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
