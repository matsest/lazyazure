package utils

import (
	"strings"
	"testing"
)

func TestBuildSubscriptionPortalURL(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		subscriptionID string
		want           string
	}{
		{
			name:           "valid subscription URL",
			tenantID:       "12345678-1234-1234-1234-123456789012",
			subscriptionID: "87654321-4321-4321-4321-210987654321",
			want:           "https://portal.azure.com/#@12345678-1234-1234-1234-123456789012/resource/subscriptions/87654321-4321-4321-4321-210987654321/overview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSubscriptionPortalURL(tt.tenantID, tt.subscriptionID)
			if got != tt.want {
				t.Errorf("BuildSubscriptionPortalURL() = %v, want %v", got, tt.want)
			}
			// Verify no double slashes (except in protocol)
			if strings.Contains(got, "://") {
				path := strings.SplitN(got, "://", 2)[1]
				if strings.Contains(path, "//") {
					t.Errorf("URL contains double slashes: %v", got)
				}
			}
		})
	}
}

func TestBuildResourceGroupPortalURL(t *testing.T) {
	tests := []struct {
		name              string
		tenantID          string
		subscriptionID    string
		resourceGroupName string
		want              string
	}{
		{
			name:              "valid resource group URL",
			tenantID:          "12345678-1234-1234-1234-123456789012",
			subscriptionID:    "87654321-4321-4321-4321-210987654321",
			resourceGroupName: "my-resource-group",
			want:              "https://portal.azure.com/#@12345678-1234-1234-1234-123456789012/resource/subscriptions/87654321-4321-4321-4321-210987654321/resourceGroups/my-resource-group/overview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildResourceGroupPortalURL(tt.tenantID, tt.subscriptionID, tt.resourceGroupName)
			if got != tt.want {
				t.Errorf("BuildResourceGroupPortalURL() = %v, want %v", got, tt.want)
			}
			// Verify no double slashes (except in protocol)
			if strings.Contains(got, "://") {
				path := strings.SplitN(got, "://", 2)[1]
				if strings.Contains(path, "//") {
					t.Errorf("URL contains double slashes: %v", got)
				}
			}
		})
	}
}

func TestBuildResourcePortalURL(t *testing.T) {
	tests := []struct {
		name       string
		tenantID   string
		resourceID string
		want       string
	}{
		{
			name:       "valid resource URL with leading slash",
			tenantID:   "12345678-1234-1234-1234-123456789012",
			resourceID: "/subscriptions/87654321-4321-4321-4321-210987654321/resourceGroups/my-rg/providers/Microsoft.Compute/virtualMachines/my-vm",
			want:       "https://portal.azure.com/#@12345678-1234-1234-1234-123456789012/resource/subscriptions/87654321-4321-4321-4321-210987654321/resourceGroups/my-rg/providers/Microsoft.Compute/virtualMachines/my-vm/overview",
		},
		{
			name:       "resource URL should not have double slashes",
			tenantID:   "tenant-123",
			resourceID: "/subscriptions/sub-123/resourceGroups/rg-1/providers/Microsoft.Storage/storageAccounts/account1",
			want:       "https://portal.azure.com/#@tenant-123/resource/subscriptions/sub-123/resourceGroups/rg-1/providers/Microsoft.Storage/storageAccounts/account1/overview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildResourcePortalURL(tt.tenantID, tt.resourceID)
			if got != tt.want {
				t.Errorf("BuildResourcePortalURL() = %v, want %v", got, tt.want)
			}
			// Verify no double slashes (except in protocol)
			if strings.Contains(got, "://") {
				path := strings.SplitN(got, "://", 2)[1]
				if strings.Contains(path, "//") {
					t.Errorf("URL contains double slashes: %v", got)
				}
			}
		})
	}
}
