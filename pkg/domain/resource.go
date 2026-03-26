package domain

import "github.com/matsest/lazyazure/pkg/resources"

// Resource represents a generic Azure resource
type Resource struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Type           string                 `json:"type"`
	Location       string                 `json:"location"`
	ResourceGroup  string                 `json:"resourceGroup"`
	SubscriptionID string                 `json:"subscriptionId"`
	Tags           map[string]string      `json:"tags"`
	Properties     map[string]interface{} `json:"properties"`  // Additional properties for details view
	CreatedTime    string                 `json:"createdTime"` // Resource creation time
	ChangedTime    string                 `json:"changedTime"` // Last modified time
}

// DisplayString returns a string representation for the UI
func (r *Resource) DisplayString() string {
	return r.Name
}

// GetDisplaySuffix returns the suffix to display (resource type)
func (r *Resource) GetDisplaySuffix() string {
	return resources.GetResourceTypeDisplayName(r.Type)
}

// GetID returns the resource ID
func (r *Resource) GetID() string {
	return r.ID
}

// GetType returns the resource type (e.g., Microsoft.Compute/virtualMachines)
func (r *Resource) GetType() string {
	return r.Type
}

// GetShortType returns a shortened type name for display (e.g., virtualMachines)
func (r *Resource) GetShortType() string {
	parts := splitType(r.Type)
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return r.Type
}

// splitType splits the resource type string (helper function)
func splitType(typeStr string) []string {
	result := make([]string, 0)
	current := ""
	for _, char := range typeStr {
		if char == '/' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
