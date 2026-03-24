package utils

import "fmt"

// BuildSubscriptionPortalURL builds the Azure Portal URL for a subscription
func BuildSubscriptionPortalURL(tenantID, subscriptionID string) string {
	return fmt.Sprintf("https://portal.azure.com/#@%s/resource/subscriptions/%s/overview", tenantID, subscriptionID)
}

// BuildResourceGroupPortalURL builds the Azure Portal URL for a resource group
func BuildResourceGroupPortalURL(tenantID, subscriptionID, resourceGroupName string) string {
	return fmt.Sprintf("https://portal.azure.com/#@%s/resource/subscriptions/%s/resourceGroups/%s/overview", tenantID, subscriptionID, resourceGroupName)
}

// BuildResourcePortalURL builds the Azure Portal URL for a resource
// Note: resourceID should start with / (e.g., /subscriptions/...)
func BuildResourcePortalURL(tenantID, resourceID string) string {
	// resourceID already starts with /, so we don't need an extra one
	return fmt.Sprintf("https://portal.azure.com/#@%s/resource%s/overview", tenantID, resourceID)
}
