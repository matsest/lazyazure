package gui

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/utils"
)

func TestNewPreloadCache(t *testing.T) {
	cache := NewPreloadCache()

	if cache == nil {
		t.Fatal("NewPreloadCache returned nil")
	}

	// Default should be medium
	if cache.rgLimit != mediumRGCache {
		t.Errorf("Expected rgLimit to be %d, got %d", mediumRGCache, cache.rgLimit)
	}

	if cache.resLimit != mediumResCache {
		t.Errorf("Expected resLimit to be %d, got %d", mediumResCache, cache.resLimit)
	}

	if cache.rgs == nil {
		t.Error("RGs map not initialized")
	}

	if cache.res == nil {
		t.Error("Resources map not initialized")
	}
}

func TestPreloadCache_SetAndGetRGs(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-subscription-123"

	// Test setting RGs
	rgs := []*domain.ResourceGroup{
		{Name: "rg1", Location: "eastus"},
		{Name: "rg2", Location: "westus"},
	}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache.SetRGs(subID, rgs, cancel)

	// Test getting RGs
	retrieved, ok := cache.GetRGs(subID)
	if !ok {
		t.Error("Expected to find cached RGs")
	}

	if len(retrieved) != len(rgs) {
		t.Errorf("Expected %d RGs, got %d", len(rgs), len(retrieved))
	}

	// Test non-existent subscription
	_, ok = cache.GetRGs("non-existent-sub")
	if ok {
		t.Error("Should not find RGs for non-existent subscription")
	}
}

func TestPreloadCache_RGsExpiration(t *testing.T) {
	// Create cache with very short TTL for testing
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-subscription"

	rgs := []*domain.ResourceGroup{{Name: "rg1"}}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache.SetRGs(subID, rgs, cancel)

	// Should exist immediately
	_, ok := cache.GetRGs(subID)
	if !ok {
		t.Error("RGs should exist immediately after setting")
	}

	// Manually expire the entry by setting timestamp to past
	cache.mu.Lock()
	cached := cache.rgs[subID]
	cached.timestamp = time.Now().Add(-16 * time.Minute) // Expired (15min TTL)
	cache.mu.Unlock()

	// Should not exist after expiration
	_, ok = cache.GetRGs(subID)
	if ok {
		t.Error("RGs should be expired after TTL")
	}
}

func TestPreloadCache_SetAndGetRes(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-subscription"
	rgName := "test-rg"

	// Test setting resources
	resources := []*domain.Resource{
		{Name: "res1", Type: "Microsoft.Compute/virtualMachines"},
		{Name: "res2", Type: "Microsoft.Storage/storageAccounts"},
	}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache.SetRes(subID, rgName, resources, cancel)

	// Test getting resources
	retrieved, ok := cache.GetRes(subID, rgName)
	if !ok {
		t.Error("Expected to find cached resources")
	}

	if len(retrieved) != len(resources) {
		t.Errorf("Expected %d resources, got %d", len(resources), len(retrieved))
	}

	// Test non-existent resource group
	_, ok = cache.GetRes(subID, "non-existent-rg")
	if ok {
		t.Error("Should not find resources for non-existent RG")
	}
}

func TestPreloadCache_ResExpiration(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-subscription"
	rgName := "test-rg"

	resources := []*domain.Resource{{Name: "res1"}}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache.SetRes(subID, rgName, resources, cancel)

	// Should exist immediately
	_, ok := cache.GetRes(subID, rgName)
	if !ok {
		t.Error("Resources should exist immediately after setting")
	}

	// Manually expire the entry
	cache.mu.Lock()
	key := subID + "/" + rgName
	cached := cache.res[key]
	cached.timestamp = time.Now().Add(-11 * time.Minute) // Expired (10min TTL)
	cache.mu.Unlock()

	// Should not exist after expiration
	_, ok = cache.GetRes(subID, rgName)
	if ok {
		t.Error("Resources should be expired after TTL")
	}
}

func TestPreloadCache_InvalidateSubs(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})

	// Set up some data
	sub1ID := "sub-1"
	sub2ID := "sub-2"

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache.SetRGs(sub1ID, []*domain.ResourceGroup{{Name: "rg1"}}, cancel)
	cache.SetRGs(sub2ID, []*domain.ResourceGroup{{Name: "rg2"}}, cancel)
	cache.SetRes(sub1ID, "rg1", []*domain.Resource{{Name: "res1"}}, cancel)
	cache.SetRes(sub2ID, "rg2", []*domain.Resource{{Name: "res2"}}, cancel)

	// Invalidate all
	cache.InvalidateSubs()

	// All should be gone
	if _, ok := cache.GetRGs(sub1ID); ok {
		t.Error("sub1 RGs should be invalidated")
	}
	if _, ok := cache.GetRGs(sub2ID); ok {
		t.Error("sub2 RGs should be invalidated")
	}
	if _, ok := cache.GetRes(sub1ID, "rg1"); ok {
		t.Error("sub1 resources should be invalidated")
	}
	if _, ok := cache.GetRes(sub2ID, "rg2"); ok {
		t.Error("sub2 resources should be invalidated")
	}
}

func TestPreloadCache_InvalidateRGs(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	sub1ID := "sub-1"
	sub2ID := "sub-2"

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up data for both subscriptions
	cache.SetRGs(sub1ID, []*domain.ResourceGroup{{Name: "rg1"}}, cancel)
	cache.SetRGs(sub2ID, []*domain.ResourceGroup{{Name: "rg2"}}, cancel)
	cache.SetRes(sub1ID, "rg1", []*domain.Resource{{Name: "res1"}}, cancel)
	cache.SetRes(sub2ID, "rg2", []*domain.Resource{{Name: "res2"}}, cancel)

	// Invalidate only sub1's RGs
	cache.InvalidateRGs(sub1ID)

	// sub1 should be gone
	if _, ok := cache.GetRGs(sub1ID); ok {
		t.Error("sub1 RGs should be invalidated")
	}
	if _, ok := cache.GetRes(sub1ID, "rg1"); ok {
		t.Error("sub1 resources should be invalidated")
	}

	// sub2 should still exist
	if _, ok := cache.GetRGs(sub2ID); !ok {
		t.Error("sub2 RGs should still exist")
	}
	if _, ok := cache.GetRes(sub2ID, "rg2"); !ok {
		t.Error("sub2 resources should still exist")
	}
}

func TestPreloadCache_InvalidateRes(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-sub"

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up data
	cache.SetRGs(subID, []*domain.ResourceGroup{{Name: "rg1"}, {Name: "rg2"}}, cancel)
	cache.SetRes(subID, "rg1", []*domain.Resource{{Name: "res1"}}, cancel)
	cache.SetRes(subID, "rg2", []*domain.Resource{{Name: "res2"}}, cancel)

	// Invalidate only rg1's resources
	cache.InvalidateRes(subID, "rg1")

	// RG1 resources should be gone
	if _, ok := cache.GetRes(subID, "rg1"); ok {
		t.Error("rg1 resources should be invalidated")
	}

	// RG2 resources should still exist
	if _, ok := cache.GetRes(subID, "rg2"); !ok {
		t.Error("rg2 resources should still exist")
	}

	// RGs should still exist
	if _, ok := cache.GetRGs(subID); !ok {
		t.Error("RGs should still exist")
	}
}

func TestPreloadCache_EvictOldestRGs(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      4,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Add 4 entries
	for i := 0; i < 4; i++ {
		subID := string(rune('a' + i))
		cache.SetRGs(subID, []*domain.ResourceGroup{{Name: "rg"}}, cancel)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Add one more - should trigger eviction
	cache.SetRGs("e", []*domain.ResourceGroup{{Name: "rg"}}, cancel)

	// Should have evicted oldest 2 (50% of 4)
	if _, ok := cache.GetRGs("a"); ok {
		t.Error("Oldest entry 'a' should have been evicted")
	}
	if _, ok := cache.GetRGs("b"); ok {
		t.Error("Second oldest entry 'b' should have been evicted")
	}

	// c, d, e should still exist
	if _, ok := cache.GetRGs("c"); !ok {
		t.Error("Entry 'c' should still exist")
	}
	if _, ok := cache.GetRGs("d"); !ok {
		t.Error("Entry 'd' should still exist")
	}
	if _, ok := cache.GetRGs("e"); !ok {
		t.Error("Entry 'e' should still exist")
	}
}

func TestPreloadCache_EvictOldestRes(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     4,
		FullResCacheSize: 500,
	})

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Add 4 entries
	for i := 0; i < 4; i++ {
		rgName := string(rune('a' + i))
		cache.SetRes("sub", rgName, []*domain.Resource{{Name: "res"}}, cancel)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Add one more - should trigger eviction
	cache.SetRes("sub", "e", []*domain.Resource{{Name: "res"}}, cancel)

	// Should have evicted oldest 2 (50% of 4)
	if _, ok := cache.GetRes("sub", "a"); ok {
		t.Error("Oldest entry 'a' should have been evicted")
	}
	if _, ok := cache.GetRes("sub", "b"); ok {
		t.Error("Second oldest entry 'b' should have been evicted")
	}

	// c, d, e should still exist
	if _, ok := cache.GetRes("sub", "c"); !ok {
		t.Error("Entry 'c' should still exist")
	}
	if _, ok := cache.GetRes("sub", "d"); !ok {
		t.Error("Entry 'd' should still exist")
	}
	if _, ok := cache.GetRes("sub", "e"); !ok {
		t.Error("Entry 'e' should still exist")
	}
}

func TestPreloadCache_CancelOnSet(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-sub"

	// First cancel func
	ctx1, cancel1 := context.WithCancel(context.Background())
	cache.SetRGs(subID, []*domain.ResourceGroup{{Name: "rg1"}}, cancel1)

	// Second cancel func should cancel the first
	ctx2, cancel2 := context.WithCancel(context.Background())
	cache.SetRGs(subID, []*domain.ResourceGroup{{Name: "rg2"}}, cancel2)

	// First context should be cancelled
	select {
	case <-ctx1.Done():
		// Good, it was cancelled
	default:
		t.Error("First context should have been cancelled")
	}

	// Second context should not be cancelled
	select {
	case <-ctx2.Done():
		t.Error("Second context should not be cancelled yet")
	default:
		// Good, not cancelled
	}

	cancel2() // Clean up
}

func TestPreloadCache_GetCacheStats(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initially empty
	rgCount, resCount, fullResCount := cache.GetCacheStats()
	if rgCount != 0 || resCount != 0 || fullResCount != 0 {
		t.Errorf("Expected empty cache, got %d RGs, %d resources, %d full resources", rgCount, resCount, fullResCount)
	}

	// Add some data
	cache.SetRGs("sub1", []*domain.ResourceGroup{{Name: "rg1"}}, cancel)
	cache.SetRGs("sub2", []*domain.ResourceGroup{{Name: "rg2"}}, cancel)
	cache.SetRes("sub1", "rg1", []*domain.Resource{{Name: "res1"}}, cancel)

	rgCount, resCount, fullResCount = cache.GetCacheStats()
	if rgCount != 2 {
		t.Errorf("Expected 2 RGs, got %d", rgCount)
	}
	if resCount != 1 {
		t.Errorf("Expected 1 resource, got %d", resCount)
	}
	if fullResCount != 0 {
		t.Errorf("Expected 0 full resources, got %d", fullResCount)
	}
}

func TestPreloadCache_ConcurrentAccess(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			subID := string(rune('a' + (id % 26)))
			cache.SetRGs(subID, []*domain.ResourceGroup{{Name: "rg"}}, cancel)
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(id int) {
			subID := string(rune('a' + (id % 26)))
			cache.GetRGs(subID)
			done <- true
		}(i)
	}

	// Wait for all
	for i := 0; i < 20; i++ {
		<-done
	}

	// Should not have panicked
	rgCount, _, _ := cache.GetCacheStats()
	if rgCount > 26 {
		t.Errorf("Expected at most 26 RGs, got %d", rgCount)
	}
}

func TestPreloadCache_IsRGLoading(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-sub"

	// Initially not loading
	if cache.IsRGLoading(subID) {
		t.Error("Should not be loading initially")
	}

	// Set loading
	cache.SetRGLoading(subID, true)
	if !cache.IsRGLoading(subID) {
		t.Error("Should be loading after SetRGLoading(true)")
	}

	// Clear loading
	cache.SetRGLoading(subID, false)
	if cache.IsRGLoading(subID) {
		t.Error("Should not be loading after SetRGLoading(false)")
	}
}

func TestPreloadCache_IsResLoading(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-sub"
	rgName := "test-rg"

	// Initially not loading
	if cache.IsResLoading(subID, rgName) {
		t.Error("Should not be loading initially")
	}

	// Set loading
	cache.SetResLoading(subID, rgName, true)
	if !cache.IsResLoading(subID, rgName) {
		t.Error("Should be loading after SetResLoading(true)")
	}

	// Clear loading
	cache.SetResLoading(subID, rgName, false)
	if cache.IsResLoading(subID, rgName) {
		t.Error("Should not be loading after SetResLoading(false)")
	}
}

func TestPreloadCache_ClearLoadingOnSet(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-sub"
	rgName := "test-rg"

	// Set loading flags
	cache.SetRGLoading(subID, true)
	cache.SetResLoading(subID, rgName, true)

	// Verify loading
	if !cache.IsRGLoading(subID) {
		t.Error("RG should be loading")
	}
	if !cache.IsResLoading(subID, rgName) {
		t.Error("Res should be loading")
	}

	// Set data in cache (should clear loading flags)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache.SetRGs(subID, []*domain.ResourceGroup{{Name: "rg"}}, cancel)
	cache.SetRes(subID, rgName, []*domain.Resource{{Name: "res"}}, cancel)

	// Verify loading flags cleared
	if cache.IsRGLoading(subID) {
		t.Error("RG loading flag should be cleared after SetRGs")
	}
	if cache.IsResLoading(subID, rgName) {
		t.Error("Res loading flag should be cleared after SetRes")
	}
}

func TestPreloadCache_ClearLoadingOnInvalidate(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-sub"
	rgName := "test-rg"

	// Set loading flags
	cache.SetRGLoading(subID, true)
	cache.SetResLoading(subID, rgName, true)

	// InvalidateRes should clear res loading
	cache.InvalidateRes(subID, rgName)
	if cache.IsResLoading(subID, rgName) {
		t.Error("Res loading flag should be cleared after InvalidateRes")
	}
	// RG loading should still be set
	if !cache.IsRGLoading(subID) {
		t.Error("RG loading flag should still be set after InvalidateRes")
	}

	// Reset and test InvalidateRGs
	cache.SetResLoading(subID, rgName, true)
	cache.InvalidateRGs(subID)
	if cache.IsRGLoading(subID) {
		t.Error("RG loading flag should be cleared after InvalidateRGs")
	}
	if cache.IsResLoading(subID, rgName) {
		t.Error("Res loading flag should be cleared after InvalidateRGs")
	}

	// Reset and test InvalidateSubs
	cache.SetRGLoading(subID, true)
	cache.SetResLoading(subID, rgName, true)
	cache.InvalidateSubs()
	if cache.IsRGLoading(subID) {
		t.Error("RG loading flag should be cleared after InvalidateSubs")
	}
	if cache.IsResLoading(subID, rgName) {
		t.Error("Res loading flag should be cleared after InvalidateSubs")
	}
}

func TestPreloadCache_FullResourceCache(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	resID := "/subscriptions/sub-123/resourceGroups/rg-456/providers/Microsoft.Compute/virtualMachines/vm-1"

	// Initially not found
	if _, ok := cache.GetFullRes(resID); ok {
		t.Error("Should not find uncached full resource")
	}

	// Set cache
	resource := &domain.Resource{
		ID:   resID,
		Name: "vm-1",
		Type: "Microsoft.Compute/virtualMachines",
	}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.SetFullRes(resID, resource, cancel)

	// Should be found
	cached, ok := cache.GetFullRes(resID)
	if !ok {
		t.Error("Should find cached full resource")
	}
	if cached.Name != "vm-1" {
		t.Errorf("Expected name 'vm-1', got '%s'", cached.Name)
	}
}

func TestPreloadCache_FullResourceExpiration(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	resID := "test-resource-id"

	resource := &domain.Resource{Name: "test"}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.SetFullRes(resID, resource, cancel)

	// Should exist immediately
	if _, ok := cache.GetFullRes(resID); !ok {
		t.Error("Full resource should exist immediately after setting")
	}

	// Manually expire the entry
	cache.mu.Lock()
	cached := cache.fullRes[resID]
	cached.timestamp = time.Now().Add(-11 * time.Minute) // Expired (10min TTL)
	cache.mu.Unlock()

	// Should not exist after expiration
	if _, ok := cache.GetFullRes(resID); ok {
		t.Error("Full resource should be expired after TTL")
	}
}

func TestPreloadCache_FullResourceLoading(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	resID := "test-resource-id"

	// Initially not loading
	if cache.IsFullResLoading(resID) {
		t.Error("Should not be loading initially")
	}

	// Set loading
	cache.SetFullResLoading(resID, true)
	if !cache.IsFullResLoading(resID) {
		t.Error("Should be loading after SetFullResLoading(true)")
	}

	// Clear loading
	cache.SetFullResLoading(resID, false)
	if cache.IsFullResLoading(resID) {
		t.Error("Should not be loading after SetFullResLoading(false)")
	}
}

func TestPreloadCache_FullResourceClearOnSet(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	resID := "test-resource-id"

	// Set loading flag
	cache.SetFullResLoading(resID, true)
	if !cache.IsFullResLoading(resID) {
		t.Error("Should be loading")
	}

	// Set data in cache (should clear loading flag)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.SetFullRes(resID, &domain.Resource{Name: "test"}, cancel)

	// Loading flag should be cleared
	if cache.IsFullResLoading(resID) {
		t.Error("Loading flag should be cleared after SetFullRes")
	}
}

func TestPreloadCache_FullResourceInvalidateSubs(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	resID := "test-resource-id"

	// Set cache first (this clears loading flag)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache.SetFullRes(resID, &domain.Resource{Name: "test"}, cancel)

	// Then set loading flag for another resource
	cache.SetFullResLoading("other-res", true)

	// Verify cache exists
	if _, ok := cache.GetFullRes(resID); !ok {
		t.Error("Should be cached")
	}

	// Verify loading flag exists for other resource
	if !cache.IsFullResLoading("other-res") {
		t.Error("Other resource should be loading")
	}

	// Invalidate all
	cache.InvalidateSubs()

	// Both should be cleared
	if cache.IsFullResLoading("other-res") {
		t.Error("Loading flag should be cleared after InvalidateSubs")
	}
	if _, ok := cache.GetFullRes(resID); ok {
		t.Error("Cache should be cleared after InvalidateSubs")
	}
}

func TestPreloadCache_FullResourceCacheStats(t *testing.T) {
	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initially empty
	_, _, fullResCount := cache.GetCacheStats()
	if fullResCount != 0 {
		t.Errorf("Expected 0 full resources, got %d", fullResCount)
	}

	// Add full resource
	cache.SetFullRes("res1", &domain.Resource{Name: "test1"}, cancel)
	cache.SetFullRes("res2", &domain.Resource{Name: "test2"}, cancel)

	_, _, fullResCount = cache.GetCacheStats()
	if fullResCount != 2 {
		t.Errorf("Expected 2 full resources, got %d", fullResCount)
	}
}

// Cache configuration tests

func TestGetCacheConfig_Default(t *testing.T) {
	// Save original env var
	origVal := os.Getenv("LAZYAZURE_CACHE_SIZE")
	defer func() {
		if origVal == "" {
			os.Unsetenv("LAZYAZURE_CACHE_SIZE")
		} else {
			os.Setenv("LAZYAZURE_CACHE_SIZE", origVal)
		}
	}()

	// Test default (medium)
	os.Unsetenv("LAZYAZURE_CACHE_SIZE")
	config := GetCacheConfig()

	if config.Level != CacheSizeMedium {
		t.Errorf("Expected default level CacheSizeMedium, got %v", config.Level)
	}
	if config.RGCacheSize != mediumRGCache {
		t.Errorf("Expected RGCacheSize %d, got %d", mediumRGCache, config.RGCacheSize)
	}
	if config.ResCacheSize != mediumResCache {
		t.Errorf("Expected ResCacheSize %d, got %d", mediumResCache, config.ResCacheSize)
	}
}

func TestGetCacheConfig_Small(t *testing.T) {
	origVal := os.Getenv("LAZYAZURE_CACHE_SIZE")
	defer os.Setenv("LAZYAZURE_CACHE_SIZE", origVal)

	os.Setenv("LAZYAZURE_CACHE_SIZE", "small")
	config := GetCacheConfig()

	if config.Level != CacheSizeSmall {
		t.Errorf("Expected level CacheSizeSmall, got %v", config.Level)
	}
	if config.RGCacheSize != smallRGCache {
		t.Errorf("Expected RGCacheSize %d, got %d", smallRGCache, config.RGCacheSize)
	}
	if config.ResCacheSize != smallResCache {
		t.Errorf("Expected ResCacheSize %d, got %d", smallResCache, config.ResCacheSize)
	}
}

func TestGetCacheConfig_Large(t *testing.T) {
	origVal := os.Getenv("LAZYAZURE_CACHE_SIZE")
	defer os.Setenv("LAZYAZURE_CACHE_SIZE", origVal)

	os.Setenv("LAZYAZURE_CACHE_SIZE", "large")
	config := GetCacheConfig()

	if config.Level != CacheSizeLarge {
		t.Errorf("Expected level CacheSizeLarge, got %v", config.Level)
	}
	if config.RGCacheSize != largeRGCache {
		t.Errorf("Expected RGCacheSize %d, got %d", largeRGCache, config.RGCacheSize)
	}
	if config.ResCacheSize != largeResCache {
		t.Errorf("Expected ResCacheSize %d, got %d", largeResCache, config.ResCacheSize)
	}
}

func TestGetCacheConfig_CaseInsensitive(t *testing.T) {
	origVal := os.Getenv("LAZYAZURE_CACHE_SIZE")
	defer os.Setenv("LAZYAZURE_CACHE_SIZE", origVal)

	os.Setenv("LAZYAZURE_CACHE_SIZE", "LARGE")
	config := GetCacheConfig()
	if config.Level != CacheSizeLarge {
		t.Errorf("Expected level CacheSizeLarge for uppercase, got %v", config.Level)
	}

	os.Setenv("LAZYAZURE_CACHE_SIZE", "Small")
	config = GetCacheConfig()
	if config.Level != CacheSizeSmall {
		t.Errorf("Expected level CacheSizeSmall for mixed case, got %v", config.Level)
	}
}

func TestGetCacheConfig_ExplicitMedium(t *testing.T) {
	origVal := os.Getenv("LAZYAZURE_CACHE_SIZE")
	defer os.Setenv("LAZYAZURE_CACHE_SIZE", origVal)

	// Test explicit medium setting
	os.Setenv("LAZYAZURE_CACHE_SIZE", "medium")
	config := GetCacheConfig()

	if config.Level != CacheSizeMedium {
		t.Errorf("Expected level CacheSizeMedium, got %v", config.Level)
	}
	if config.RGCacheSize != mediumRGCache {
		t.Errorf("Expected RGCacheSize %d, got %d", mediumRGCache, config.RGCacheSize)
	}
	if config.ResCacheSize != mediumResCache {
		t.Errorf("Expected ResCacheSize %d, got %d", mediumResCache, config.ResCacheSize)
	}
}

func TestGetCacheConfig_Invalid(t *testing.T) {
	origVal := os.Getenv("LAZYAZURE_CACHE_SIZE")
	defer func() {
		if origVal == "" {
			os.Unsetenv("LAZYAZURE_CACHE_SIZE")
		} else {
			os.Setenv("LAZYAZURE_CACHE_SIZE", origVal)
		}
	}()

	// Invalid values should default to medium
	os.Setenv("LAZYAZURE_CACHE_SIZE", "invalid")
	config := GetCacheConfig()

	if config.Level != CacheSizeMedium {
		t.Errorf("Expected default level CacheSizeMedium for invalid value, got %v", config.Level)
	}
	if config.RGCacheSize != mediumRGCache {
		t.Errorf("Expected medium RGCacheSize for invalid value, got %d", config.RGCacheSize)
	}
}

func TestNewPreloadCacheWithConfig(t *testing.T) {
	config := CacheConfig{
		Level:            CacheSizeLarge,
		RGCacheSize:      200,
		ResCacheSize:     1000,
		FullResCacheSize: 500,
	}

	cache := NewPreloadCacheWithConfig(config)

	if cache.rgLimit != 200 {
		t.Errorf("Expected rgLimit 200, got %d", cache.rgLimit)
	}
	if cache.resLimit != 1000 {
		t.Errorf("Expected resLimit 1000, got %d", cache.resLimit)
	}
	if cache.fullResLimit != 500 {
		t.Errorf("Expected fullResLimit 500, got %d", cache.fullResLimit)
	}
}

func TestNewPreloadCache_EnvironmentVariable(t *testing.T) {
	origVal := os.Getenv("LAZYAZURE_CACHE_SIZE")
	defer os.Setenv("LAZYAZURE_CACHE_SIZE", origVal)

	os.Setenv("LAZYAZURE_CACHE_SIZE", "large")
	cache := NewPreloadCache()

	if cache.rgLimit != largeRGCache {
		t.Errorf("Expected rgLimit %d for large cache, got %d", largeRGCache, cache.rgLimit)
	}
	if cache.resLimit != largeResCache {
		t.Errorf("Expected resLimit %d for large cache, got %d", largeResCache, cache.resLimit)
	}
}

func TestParseCacheSizeLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected CacheSizeLevel
	}{
		{"small", CacheSizeSmall},
		{"SMALL", CacheSizeSmall},
		{"Small", CacheSizeSmall},
		{"medium", CacheSizeMedium},
		{"MEDIUM", CacheSizeMedium},
		{"large", CacheSizeLarge},
		{"LARGE", CacheSizeLarge},
		{"", CacheSizeMedium},        // empty defaults to medium
		{"invalid", CacheSizeMedium}, // invalid defaults to medium
		{"unknown", CacheSizeMedium},
	}

	for _, tc := range tests {
		result := parseCacheSizeLevel(tc.input)
		if result != tc.expected {
			t.Errorf("parseCacheSizeLevel(%q) = %v, expected %v", tc.input, result, tc.expected)
		}
	}
}

// Metrics integration tests

func TestPreloadCache_MetricsCacheHit(t *testing.T) {
	// Reset metrics before test
	utils.GetMetrics().Reset()

	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})
	subID := "test-subscription"

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set data in cache
	cache.SetRGs(subID, []*domain.ResourceGroup{{Name: "rg1"}}, cancel)

	// Get data - should be a cache hit
	_, ok := cache.GetRGs(subID)
	if !ok {
		t.Error("Expected to find cached RGs")
	}

	// Check metrics
	stats := utils.GetMetrics().GetStats()
	if stats.CacheHits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", stats.CacheHits)
	}
}

func TestPreloadCache_MetricsCacheMiss(t *testing.T) {
	// Reset metrics before test
	utils.GetMetrics().Reset()

	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})

	// Get non-existent data - should be a cache miss
	_, ok := cache.GetRGs("non-existent-sub")
	if ok {
		t.Error("Should not find non-existent RGs")
	}

	// Check metrics
	stats := utils.GetMetrics().GetStats()
	if stats.CacheMisses != 1 {
		t.Errorf("Expected 1 cache miss, got %d", stats.CacheMisses)
	}
}

func TestPreloadCache_MetricsCacheSizeTracking(t *testing.T) {
	// Reset metrics before test
	utils.GetMetrics().Reset()

	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Add items
	cache.SetRGs("sub1", []*domain.ResourceGroup{{Name: "rg1"}}, cancel)
	cache.SetRGs("sub2", []*domain.ResourceGroup{{Name: "rg2"}}, cancel)
	cache.SetRes("sub1", "rg1", []*domain.Resource{{Name: "res1"}}, cancel)

	// Check metrics reflect current size
	stats := utils.GetMetrics().GetStats()
	if stats.CurrentRGSize != 2 {
		t.Errorf("Expected current RG size 2, got %d", stats.CurrentRGSize)
	}
	if stats.CurrentResSize != 1 {
		t.Errorf("Expected current Res size 1, got %d", stats.CurrentResSize)
	}
	if stats.MaxRGSize != 2 {
		t.Errorf("Expected max RG size 2, got %d", stats.MaxRGSize)
	}
}

func TestPreloadCache_MetricsEviction(t *testing.T) {
	// Reset metrics before test
	utils.GetMetrics().Reset()

	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      4,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Add 4 entries
	for i := 0; i < 4; i++ {
		subID := string(rune('a' + i))
		cache.SetRGs(subID, []*domain.ResourceGroup{{Name: "rg"}}, cancel)
		time.Sleep(10 * time.Millisecond)
	}

	// Add one more - should trigger eviction of 2 items
	cache.SetRGs("e", []*domain.ResourceGroup{{Name: "rg"}}, cancel)

	// Check metrics
	stats := utils.GetMetrics().GetStats()
	if stats.Evictions != 2 {
		t.Errorf("Expected 2 evictions, got %d", stats.Evictions)
	}
}

func TestPreloadCache_MetricsOnInvalidate(t *testing.T) {
	// Reset metrics before test
	utils.GetMetrics().Reset()

	cache := NewPreloadCacheWithConfig(CacheConfig{
		RGCacheSize:      100,
		ResCacheSize:     500,
		FullResCacheSize: 500,
	})

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Add data
	cache.SetRGs("sub1", []*domain.ResourceGroup{{Name: "rg1"}}, cancel)
	cache.SetRGs("sub2", []*domain.ResourceGroup{{Name: "rg2"}}, cancel)

	// Verify metrics show items
	stats := utils.GetMetrics().GetStats()
	if stats.CurrentRGSize != 2 {
		t.Errorf("Expected RG size 2 before invalidate, got %d", stats.CurrentRGSize)
	}

	// Invalidate all
	cache.InvalidateSubs()

	// Check metrics are updated
	stats = utils.GetMetrics().GetStats()
	if stats.CurrentRGSize != 0 {
		t.Errorf("Expected RG size 0 after invalidate, got %d", stats.CurrentRGSize)
	}
}

// Semaphore tests

func TestSemaphore_New(t *testing.T) {
	s := NewSemaphore(10)
	if s == nil {
		t.Fatal("NewSemaphore returned nil")
	}
	if s.ch == nil {
		t.Fatal("Semaphore channel not initialized")
	}
}

func TestSemaphore_AcquireAndRelease(t *testing.T) {
	s := NewSemaphore(2)
	ctx := context.Background()

	// Should be able to acquire twice
	if err := s.Acquire(ctx); err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}
	if err := s.Acquire(ctx); err != nil {
		t.Fatalf("Second acquire failed: %v", err)
	}

	// Release one
	s.Release()

	// Should be able to acquire again
	if err := s.Acquire(ctx); err != nil {
		t.Fatalf("Third acquire after release failed: %v", err)
	}

	// Release remaining
	s.Release()
	s.Release()
}

func TestSemaphore_AcquireContextCancellation(t *testing.T) {
	s := NewSemaphore(1)
	ctx1 := context.Background()

	// Acquire the only slot
	if err := s.Acquire(ctx1); err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}

	// Create a context that will be cancelled
	ctx2, cancel := context.WithCancel(context.Background())

	// Start acquiring in a goroutine (will block)
	done := make(chan error, 1)
	go func() {
		done <- s.Acquire(ctx2)
	}()

	// Cancel the context
	cancel()

	// Should receive context error
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Acquire did not return after context cancellation")
	}

	// Release the slot
	s.Release()
}

func TestSemaphore_PreloadCacheIntegration(t *testing.T) {
	cache := NewPreloadCache()
	semaphore := cache.GetSemaphore()

	if semaphore == nil {
		t.Fatal("PreloadCache semaphore is nil")
	}

	// Should be able to acquire from cache's semaphore
	ctx := context.Background()
	if err := semaphore.Acquire(ctx); err != nil {
		t.Fatalf("Acquire from cache semaphore failed: %v", err)
	}

	// Release
	semaphore.Release()
}

func TestSemaphore_MaxConcurrentPreloadsConstant(t *testing.T) {
	// Verify the constant is set to expected value
	if MaxConcurrentPreloads != 50 {
		t.Errorf("Expected MaxConcurrentPreloads to be 50, got %d", MaxConcurrentPreloads)
	}
}
