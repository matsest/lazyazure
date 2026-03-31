package azure

import (
	"testing"
)

func TestLoadCuratedAPIVersions(t *testing.T) {
	// The init() function should have already loaded curated versions
	// Let's verify they exist in the global cache

	// Check some expected curated resource types
	testCases := []struct {
		resourceType string
		shouldExist  bool
	}{
		{"Microsoft.Storage/storageAccounts", true},
		{"Microsoft.Compute/virtualMachines", true},
		{"Microsoft.KeyVault/vaults", true},
		{"Microsoft.Network/virtualNetworks", true},
		{"Microsoft.Sql/servers", true},
		{"Microsoft.Unknown/nonexistent", false},
	}

	for _, tc := range testCases {
		t.Run(tc.resourceType, func(t *testing.T) {
			globalAPICacheMux.RLock()
			versions, exists := globalAPICache[tc.resourceType]
			globalAPICacheMux.RUnlock()

			if tc.shouldExist {
				if !exists {
					t.Errorf("Expected %s to be in curated cache, but it wasn't found", tc.resourceType)
				} else if len(versions) == 0 {
					t.Errorf("Expected %s to have versions in cache, but it was empty", tc.resourceType)
				} else {
					t.Logf("✓ %s: %s", tc.resourceType, versions[0])
				}
			} else {
				if exists {
					t.Errorf("Expected %s to NOT be in cache, but it was found", tc.resourceType)
				}
			}
		})
	}

	// Check that we loaded at least some curated versions
	globalAPICacheMux.RLock()
	cacheSize := len(globalAPICache)
	globalAPICacheMux.RUnlock()

	if cacheSize == 0 {
		t.Error("Expected global cache to contain curated versions, but it was empty")
	} else {
		t.Logf("Global cache contains %d curated API versions", cacheSize)
	}
}
