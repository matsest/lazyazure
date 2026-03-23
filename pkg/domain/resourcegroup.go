package domain

// ResourceGroup represents an Azure resource group
type ResourceGroup struct {
	Name              string            `json:"name"`
	Location          string            `json:"location"`
	ID                string            `json:"id"`
	ProvisioningState string            `json:"provisioningState"`
	Tags              map[string]string `json:"tags"`
	SubscriptionID    string            `json:"subscriptionId"`
}

// DisplayString returns a string representation for the UI
func (rg *ResourceGroup) DisplayString() string {
	return rg.Name
}

// GetID returns the resource group ID
func (rg *ResourceGroup) GetID() string {
	return rg.ID
}
