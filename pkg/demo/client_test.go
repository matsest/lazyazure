package demo

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

func TestNewDemoData(t *testing.T) {
	data := NewDemoData()

	if data == nil {
		t.Fatal("NewDemoData returned nil")
	}

	// Check user
	if data.User == nil {
		t.Fatal("Demo user is nil")
	}
	if data.User.DisplayName != "Demo User" {
		t.Errorf("Expected DisplayName='Demo User', got %q", data.User.DisplayName)
	}
	if data.User.Type != "user" {
		t.Errorf("Expected Type='user', got %q", data.User.Type)
	}

	// Check subscriptions
	if len(data.Subscriptions) != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", len(data.Subscriptions))
	}

	// Check resource groups exist for each subscription
	for _, sub := range data.Subscriptions {
		rgs := data.GetResourceGroups(sub.ID)
		if len(rgs) != 4 {
			t.Errorf("Expected 4 resource groups for subscription %s, got %d", sub.Name, len(rgs))
		}
	}
}

func TestDemoClientGetUserInfo(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	user, err := client.GetUserInfo(ctx)
	if err != nil {
		t.Fatalf("GetUserInfo failed: %v", err)
	}

	if user == nil {
		t.Fatal("GetUserInfo returned nil user")
	}

	if user.DisplayName != "Demo User" {
		t.Errorf("Expected DisplayName='Demo User', got %q", user.DisplayName)
	}

	// Should complete within reasonable time (simulated delay)
	start := time.Now()
	_, _ = client.GetUserInfo(ctx)
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Logf("GetUserInfo took %v (may be slow but OK)", elapsed)
	}
}

func TestDemoClientVerifyAuthentication(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	err := client.VerifyAuthentication(ctx)
	if err != nil {
		t.Fatalf("VerifyAuthentication failed: %v", err)
	}
}

func TestDemoClientCredential(t *testing.T) {
	client := NewClient()

	cred := client.Credential()
	if cred == nil {
		t.Fatal("Credential() returned nil")
	}

	// Test GetToken
	ctx := context.Background()
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{})
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	if token.Token != "demo-token-for-demo-mode" {
		t.Errorf("Expected token='demo-token-for-demo-mode', got %q", token.Token)
	}
}

func TestDemoSubscriptionsClient(t *testing.T) {
	client := NewClient()
	subClient, err := client.NewSubscriptionsClient()
	if err != nil {
		t.Fatalf("NewSubscriptionsClient failed: %v", err)
	}

	ctx := context.Background()
	subs, err := subClient.ListSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListSubscriptions failed: %v", err)
	}

	if len(subs) != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", len(subs))
	}

	// Check subscription names
	expectedNames := map[string]bool{
		"Demo Production":  false,
		"Demo Development": false,
	}
	for _, sub := range subs {
		expectedNames[sub.Name] = true
	}
	for name, found := range expectedNames {
		if !found {
			t.Errorf("Expected subscription %q not found", name)
		}
	}
}

func TestDemoResourceGroupsClient(t *testing.T) {
	client := NewClient()

	// Test production subscription
	rgClient, err := client.NewResourceGroupsClient("00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("NewResourceGroupsClient failed: %v", err)
	}

	ctx := context.Background()
	rgs, err := rgClient.ListResourceGroups(ctx)
	if err != nil {
		t.Fatalf("ListResourceGroups failed: %v", err)
	}

	if len(rgs) != 4 {
		t.Errorf("Expected 4 resource groups, got %d", len(rgs))
	}

	// Check that resource groups have prod prefix
	for _, rg := range rgs {
		if rg.SubscriptionID != "00000000-0000-0000-0000-000000000001" {
			t.Errorf("Resource group %s has wrong subscription ID: %s", rg.Name, rg.SubscriptionID)
		}
	}
}

func TestDemoResourcesClient(t *testing.T) {
	client := NewClient()

	resClient, err := client.NewResourcesClient("00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("NewResourcesClient failed: %v", err)
	}

	ctx := context.Background()

	// Test listing resources
	resources, err := resClient.ListResourcesByResourceGroup(ctx, "rg-prod-web")
	if err != nil {
		t.Fatalf("ListResourcesByResourceGroup failed: %v", err)
	}

	if len(resources) == 0 {
		t.Error("Expected resources, got none")
	}

	// In list view, properties should be nil (lazy loading)
	for _, res := range resources {
		if res.Properties != nil {
			t.Errorf("Resource %s has properties in list view (should be nil for lazy loading)", res.Name)
		}
	}
}

func TestDemoResourcesClientGetResource(t *testing.T) {
	client := NewClient()

	resClient, err := client.NewResourcesClient("00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("NewResourcesClient failed: %v", err)
	}

	ctx := context.Background()

	// Get a resource from the list first
	resources, err := resClient.ListResourcesByResourceGroup(ctx, "rg-prod-web")
	if err != nil || len(resources) == 0 {
		t.Fatal("Failed to get resources for testing")
	}

	firstRes := resources[0]

	// Now fetch full resource details
	fullRes, err := resClient.GetResource(ctx, firstRes.ID, firstRes.Type)
	if err != nil {
		t.Fatalf("GetResource failed: %v", err)
	}

	if fullRes == nil {
		t.Fatal("GetResource returned nil")
	}

	// Full resource should have properties
	if fullRes.Properties == nil {
		t.Error("GetResource returned resource without properties")
	}

	if len(fullRes.Properties) == 0 {
		t.Error("GetResource returned resource with empty properties")
	}
}

func TestDemoDataGetSubscription(t *testing.T) {
	data := NewDemoData()

	sub := data.GetSubscription("00000000-0000-0000-0000-000000000001")
	if sub == nil {
		t.Fatal("GetSubscription returned nil for valid ID")
	}
	if sub.Name != "Demo Production" {
		t.Errorf("Expected 'Demo Production', got %q", sub.Name)
	}

	// Test invalid ID
	sub = data.GetSubscription("invalid-id")
	if sub != nil {
		t.Error("GetSubscription should return nil for invalid ID")
	}
}

func TestDemoDataGetResourceGroup(t *testing.T) {
	data := NewDemoData()

	rg := data.GetResourceGroup("00000000-0000-0000-0000-000000000001", "rg-prod-web")
	if rg == nil {
		t.Fatal("GetResourceGroup returned nil for valid RG")
	}
	if rg.Location != "westus2" {
		t.Errorf("Expected location 'westus2', got %q", rg.Location)
	}

	// Test invalid RG
	rg = data.GetResourceGroup("00000000-0000-0000-0000-000000000001", "nonexistent")
	if rg != nil {
		t.Error("GetResourceGroup should return nil for invalid name")
	}
}

func TestDemoDataGetResource(t *testing.T) {
	data := NewDemoData()

	// Get a resource that we know exists
	resources := data.GetResources("rg-prod-web")
	if len(resources) == 0 {
		t.Fatal("No resources found for testing")
	}

	res := data.GetResource(resources[0].ID)
	if res == nil {
		t.Fatal("GetResource returned nil for valid ID")
	}

	// Test invalid ID
	res = data.GetResource("/subscriptions/invalid/resourceGroups/invalid/providers/Microsoft.Storage/storageAccounts/invalid")
	if res != nil {
		t.Error("GetResource should return nil for invalid ID")
	}
}

func TestDemoClientContextCancellation(t *testing.T) {
	client := NewClient()

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.GetUserInfo(ctx)
	if err == nil {
		t.Error("GetUserInfo should return error for cancelled context")
	}

	_, err = client.NewSubscriptionsClient()
	if err != nil {
		t.Fatalf("NewSubscriptionsClient failed: %v", err)
	}
}

func TestSubIDFromRGID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"/subscriptions/123/resourceGroups/myRG",
			"123",
		},
		{
			"/subscriptions/abc/providers/Microsoft.Storage/storageAccounts/test",
			"abc",
		},
		{
			"invalid-format",
			"",
		},
	}

	for _, tt := range tests {
		result := subIDFromRGID(tt.input)
		if result != tt.expected {
			t.Errorf("subIDFromRGID(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "world", true},
		{"hello world", "foo", false},
		{"", "test", false},
		{"test", "", true},
		{"rg-prod-web", "prod", true},
	}

	for _, tt := range tests {
		result := contains(tt.s, tt.substr)
		if result != tt.expected {
			t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
		}
	}
}

func TestSplitString(t *testing.T) {
	tests := []struct {
		input    string
		sep      rune
		expected []string
	}{
		{"/subscriptions/123/resourceGroups/myRG", '/', []string{"subscriptions", "123", "resourceGroups", "myRG"}},
		{"hello-world-test", '-', []string{"hello", "world", "test"}},
		{"", '/', []string{}},
		{"nodepths", '/', []string{"nodepths"}},
	}

	for _, tt := range tests {
		result := splitString(tt.input, tt.sep)
		if len(result) != len(tt.expected) {
			t.Errorf("splitString(%q, %q) returned %d parts, want %d", tt.input, string(tt.sep), len(result), len(tt.expected))
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("splitString(%q, %q)[%d] = %q, want %q", tt.input, string(tt.sep), i, result[i], tt.expected[i])
			}
		}
	}
}

// Test that demo data contains expected resource types
func TestDemoDataResourceTypes(t *testing.T) {
	data := NewDemoData()

	expectedTypes := map[string]bool{
		"Microsoft.Storage/storageAccounts": false,
		"Microsoft.KeyVault/vaults":         false,
		"Microsoft.Compute/virtualMachines": false,
		"Microsoft.Sql/servers/databases":   false,
		"Microsoft.Network/loadBalancers":   false,
	}

	// Check all resources
	for _, resources := range data.Resources {
		for _, res := range resources {
			if _, ok := expectedTypes[res.Type]; ok {
				expectedTypes[res.Type] = true
			}
		}
	}

	// Verify all expected types were found
	for resType, found := range expectedTypes {
		if !found {
			t.Errorf("Expected resource type %q not found in demo data", resType)
		}
	}
}

// Test that resource groups have correct structure
func TestDemoResourceGroupsStructure(t *testing.T) {
	data := NewDemoData()

	for _, sub := range data.Subscriptions {
		rgs := data.GetResourceGroups(sub.ID)
		for _, rg := range rgs {
			// Check required fields
			if rg.Name == "" {
				t.Error("Resource group has empty name")
			}
			if rg.Location == "" {
				t.Errorf("Resource group %s has empty location", rg.Name)
			}
			if rg.ID == "" {
				t.Errorf("Resource group %s has empty ID", rg.Name)
			}
			if rg.ProvisioningState != "Succeeded" {
				t.Errorf("Resource group %s has unexpected provisioning state: %s", rg.Name, rg.ProvisioningState)
			}
			if rg.SubscriptionID != sub.ID {
				t.Errorf("Resource group %s has wrong subscription ID", rg.Name)
			}
			// Check tags exist
			if len(rg.Tags) == 0 {
				t.Errorf("Resource group %s has no tags", rg.Name)
			}
		}
	}
}
