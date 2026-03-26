package domain

import (
	"encoding/json"
	"testing"
)

func TestUserJSONTags(t *testing.T) {
	user := &User{
		DisplayName:       "Test User",
		UserPrincipalName: "test@example.com",
		Type:              "user",
		TenantID:          "12345",
	}

	data, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("Failed to marshal user: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Check camelCase keys
	if _, ok := result["displayName"]; !ok {
		t.Error("Expected 'displayName' key in JSON")
	}
	if _, ok := result["userPrincipalName"]; !ok {
		t.Error("Expected 'userPrincipalName' key in JSON")
	}
	if _, ok := result["type"]; !ok {
		t.Error("Expected 'type' key in JSON")
	}
	if _, ok := result["tenantId"]; !ok {
		t.Error("Expected 'tenantId' key in JSON")
	}

	// Verify values
	if result["displayName"] != "Test User" {
		t.Errorf("Expected displayName='Test User', got %v", result["displayName"])
	}
}

func TestSubscriptionJSONTags(t *testing.T) {
	sub := &Subscription{
		ID:       "sub-123",
		Name:     "Test Subscription",
		State:    "Enabled",
		TenantID: "tenant-456",
	}

	data, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("Failed to marshal subscription: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if _, ok := result["id"]; !ok {
		t.Error("Expected 'id' key in JSON")
	}
	if _, ok := result["tenantId"]; !ok {
		t.Error("Expected 'tenantId' key in JSON")
	}
}

func TestSubscriptionDisplayString(t *testing.T) {
	sub := &Subscription{
		ID:   "sub-123",
		Name: "Test Subscription",
	}

	got := sub.DisplayString()
	want := "Test Subscription"
	if got != want {
		t.Errorf("DisplayString() = %q, want %q", got, want)
	}
}

func TestSubscriptionGetDisplaySuffix(t *testing.T) {
	sub := &Subscription{
		ID:   "sub-123",
		Name: "Test Subscription",
	}

	got := sub.GetDisplaySuffix()
	want := "sub-123"
	if got != want {
		t.Errorf("GetDisplaySuffix() = %q, want %q", got, want)
	}
}

func TestResourceGroupJSONTags(t *testing.T) {
	rg := &ResourceGroup{
		Name:              "test-rg",
		Location:          "westeurope",
		ID:                "/subscriptions/123/resourceGroups/test-rg",
		ProvisioningState: "Succeeded",
		Tags:              map[string]string{"env": "test"},
		SubscriptionID:    "sub-123",
	}

	data, err := json.Marshal(rg)
	if err != nil {
		t.Fatalf("Failed to marshal resource group: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if _, ok := result["provisioningState"]; !ok {
		t.Error("Expected 'provisioningState' key in JSON")
	}
	if _, ok := result["subscriptionId"]; !ok {
		t.Error("Expected 'subscriptionId' key in JSON")
	}
}

func TestResourceGroupDisplayString(t *testing.T) {
	rg := &ResourceGroup{
		Name:     "test-rg",
		Location: "westeurope",
	}

	got := rg.DisplayString()
	want := "test-rg"
	if got != want {
		t.Errorf("DisplayString() = %q, want %q", got, want)
	}
}

func TestResourceGroupGetDisplaySuffix(t *testing.T) {
	rg := &ResourceGroup{
		Name:     "test-rg",
		Location: "westeurope",
	}

	got := rg.GetDisplaySuffix()
	want := "westeurope"
	if got != want {
		t.Errorf("GetDisplaySuffix() = %q, want %q", got, want)
	}
}

func TestResourceJSONTags(t *testing.T) {
	res := &Resource{
		ID:             "/subscriptions/123/resourceGroups/rg/providers/Microsoft.Compute/vm",
		Name:           "test-vm",
		Type:           "Microsoft.Compute/virtualMachines",
		Location:       "westeurope",
		ResourceGroup:  "rg",
		SubscriptionID: "sub-123",
		Tags:           map[string]string{"env": "prod"},
		Properties:     map[string]interface{}{"size": "Standard_DS1"},
		CreatedTime:    "2024-01-01T00:00:00Z",
		ChangedTime:    "2024-01-02T00:00:00Z",
	}

	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("Failed to marshal resource: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Check camelCase keys
	if _, ok := result["resourceGroup"]; !ok {
		t.Error("Expected 'resourceGroup' key in JSON")
	}
	if _, ok := result["subscriptionId"]; !ok {
		t.Error("Expected 'subscriptionId' key in JSON")
	}
	if _, ok := result["createdTime"]; !ok {
		t.Error("Expected 'createdTime' key in JSON")
	}
	if _, ok := result["changedTime"]; !ok {
		t.Error("Expected 'changedTime' key in JSON")
	}
	if _, ok := result["properties"]; !ok {
		t.Error("Expected 'properties' key in JSON")
	}
}

func TestResourceGetShortType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Microsoft.Compute/virtualMachines", "virtualMachines"},
		{"Microsoft.Storage/storageAccounts", "storageAccounts"},
		{"Microsoft.Network/virtualNetworks", "virtualNetworks"},
		{"simpleType", "simpleType"},
	}

	for _, tt := range tests {
		res := &Resource{Type: tt.input}
		got := res.GetShortType()
		if got != tt.expected {
			t.Errorf("GetShortType(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestResourceDisplayString(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		want         string
	}{
		{"my-vm", "Microsoft.Compute/virtualMachines", "my-vm"},
		{"prod-storage", "Microsoft.Storage/storageAccounts", "prod-storage"},
		{"web-api", "Microsoft.Web/sites", "web-api"},
		{"my-keyvault", "Microsoft.KeyVault/vaults", "my-keyvault"},
		{"aks-cluster", "Microsoft.ContainerService/managedClusters", "aks-cluster"},
	}

	for _, tt := range tests {
		res := &Resource{
			Name: tt.name,
			Type: tt.resourceType,
		}
		got := res.DisplayString()
		if got != tt.want {
			t.Errorf("DisplayString() = %q, want %q", got, tt.want)
		}
	}
}

func TestResourceGetDisplaySuffix(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		want         string
	}{
		{"my-vm", "Microsoft.Compute/virtualMachines", "Virtual Machine"},
		{"prod-storage", "Microsoft.Storage/storageAccounts", "Storage Account"},
		{"web-api", "Microsoft.Web/sites", "Web App"},
		{"my-keyvault", "Microsoft.KeyVault/vaults", "Key Vault"},
		{"aks-cluster", "Microsoft.ContainerService/managedClusters", "AKS Cluster"},
	}

	for _, tt := range tests {
		res := &Resource{
			Name: tt.name,
			Type: tt.resourceType,
		}
		got := res.GetDisplaySuffix()
		if got != tt.want {
			t.Errorf("GetDisplaySuffix() = %q, want %q", got, tt.want)
		}
	}
}
