package gui

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/matsest/lazyazure/pkg/domain"
	"github.com/matsest/lazyazure/pkg/utils"
)

// Cache sizing constants - base values
const (
	rgCacheTTL   = 15 * time.Minute
	resCacheTTL  = 10 * time.Minute
	baseRGCache  = 100
	baseResCache = 500
)

// MaxConcurrentPreloads limits concurrent background loading operations
// to prevent excessive goroutine spawning and API rate limiting
const MaxConcurrentPreloads = 50

// Cache size tiers
const (
	smallRGCache  = 100
	smallResCache = 500

	mediumRGCache  = 300
	mediumResCache = 1500

	largeRGCache  = 600
	largeResCache = 3000
)

// CacheSizeLevel represents the cache size configuration level
type CacheSizeLevel int

const (
	CacheSizeSmall  CacheSizeLevel = iota // Small cache (0.5x base)
	CacheSizeMedium                       // Medium cache (1x base) - default
	CacheSizeLarge                        // Large cache (3x base)
)

// CacheConfig holds cache size configuration
type CacheConfig struct {
	Level            CacheSizeLevel
	RGCacheSize      int
	ResCacheSize     int
	FullResCacheSize int
}

// parseCacheSizeLevel parses a string into CacheSizeLevel
// Empty string returns CacheSizeMedium (default)
func parseCacheSizeLevel(s string) CacheSizeLevel {
	switch strings.ToLower(s) {
	case "small":
		return CacheSizeSmall
	case "large":
		return CacheSizeLarge
	case "medium":
		return CacheSizeMedium
	default:
		return CacheSizeMedium // Default to medium for empty or invalid values
	}
}

// GetCacheConfig returns the cache configuration based on environment
// Defaults to medium if LAZYAZURE_CACHE_SIZE is not set or invalid
func GetCacheConfig() CacheConfig {
	levelStr := os.Getenv("LAZYAZURE_CACHE_SIZE")
	level := parseCacheSizeLevel(levelStr)

	config := CacheConfig{
		Level: level,
	}

	switch level {
	case CacheSizeSmall:
		config.RGCacheSize = smallRGCache
		config.ResCacheSize = smallResCache
		config.FullResCacheSize = smallResCache
	case CacheSizeLarge:
		config.RGCacheSize = largeRGCache
		config.ResCacheSize = largeResCache
		config.FullResCacheSize = largeResCache
	default: // CacheSizeMedium
		config.RGCacheSize = mediumRGCache
		config.ResCacheSize = mediumResCache
		config.FullResCacheSize = mediumResCache
	}

	return config
}

// cachedRGs holds cached resource groups for a subscription
type cachedRGs struct {
	groups    []*domain.ResourceGroup
	timestamp time.Time
	cancel    context.CancelFunc
}

// cachedResources holds cached resources for a resource group
type cachedResources struct {
	resources []*domain.Resource
	timestamp time.Time
	cancel    context.CancelFunc
}

// cachedFullResource holds cached full resource details
type cachedFullResource struct {
	resource  *domain.Resource
	timestamp time.Time
	cancel    context.CancelFunc
}

// Semaphore provides a weighted semaphore for limiting concurrent operations
type Semaphore struct {
	ch    chan struct{}
	cap   int
	mu    sync.RWMutex
	inUse int
}

// NewSemaphore creates a new semaphore with the given capacity
func NewSemaphore(n int) *Semaphore {
	return &Semaphore{
		ch:  make(chan struct{}, n),
		cap: n,
	}
}

// Acquire acquires a slot in the semaphore, blocking until one is available
// or the context is cancelled. Returns context error if cancelled.
func (s *Semaphore) Acquire(ctx context.Context) error {
	select {
	case s.ch <- struct{}{}:
		s.mu.Lock()
		s.inUse++
		s.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release releases a slot in the semaphore
func (s *Semaphore) Release() {
	select {
	case <-s.ch:
		s.mu.Lock()
		s.inUse--
		s.mu.Unlock()
	default:
		// Should never happen - releasing without acquiring
	}
}

// GetUtilization returns current semaphore usage stats (inUse, capacity)
func (s *Semaphore) GetUtilization() (inUse, capacity int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inUse, s.cap
}

// PreloadCache provides in-memory caching for resource groups and resources
// with TTL-based expiration and size limits
type PreloadCache struct {
	mu             sync.RWMutex
	rgs            map[string]*cachedRGs          // key: subscriptionID
	res            map[string]*cachedResources    // key: "subID/rgName"
	fullRes        map[string]*cachedFullResource // key: resourceID
	rgLimit        int
	resLimit       int
	fullResLimit   int
	rgLoading      map[string]bool // Track in-progress RG preloads
	resLoading     map[string]bool // Track in-progress resource preloads
	fullResLoading map[string]bool // Track in-progress full resource detail loads
	semaphore      *Semaphore      // Limits concurrent background operations
}

// NewPreloadCache creates a new preload cache with environment-based limits
// Uses LAZYAZURE_CACHE_SIZE environment variable: small, medium (default), large
func NewPreloadCache() *PreloadCache {
	config := GetCacheConfig()
	return NewPreloadCacheWithConfig(config)
}

// NewPreloadCacheWithConfig creates a new preload cache with specific configuration
func NewPreloadCacheWithConfig(config CacheConfig) *PreloadCache {
	return &PreloadCache{
		rgs:            make(map[string]*cachedRGs),
		res:            make(map[string]*cachedResources),
		fullRes:        make(map[string]*cachedFullResource),
		rgLimit:        config.RGCacheSize,
		resLimit:       config.ResCacheSize,
		fullResLimit:   config.FullResCacheSize,
		rgLoading:      make(map[string]bool),
		resLoading:     make(map[string]bool),
		fullResLoading: make(map[string]bool),
		semaphore:      NewSemaphore(MaxConcurrentPreloads),
	}
}

// GetRGs retrieves cached resource groups for a subscription
// Returns nil, false if not found or expired
func (c *PreloadCache) GetRGs(subID string) ([]*domain.ResourceGroup, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, ok := c.rgs[subID]
	if !ok {
		utils.GetMetrics().RecordCacheMiss()
		return nil, false
	}

	if c.isExpired(cached.timestamp, rgCacheTTL) {
		utils.GetMetrics().RecordCacheMiss()
		return nil, false
	}

	utils.GetMetrics().RecordCacheHit()
	return cached.groups, true
}

// SetRGs stores resource groups for a subscription in cache
// Cancels any existing preload operation for this subscription
func (c *PreloadCache) SetRGs(subID string, groups []*domain.ResourceGroup, cancel context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel existing preload if any
	if existing, ok := c.rgs[subID]; ok && existing.cancel != nil {
		existing.cancel()
	}

	// Check if we need to evict
	if len(c.rgs) >= c.rgLimit {
		evicted := c.rgLimit / 2
		c.evictOldestRGs(evicted)
		utils.GetMetrics().RecordEviction(evicted)
	}

	c.rgs[subID] = &cachedRGs{
		groups:    groups,
		timestamp: time.Now(),
		cancel:    cancel,
	}
	// Clear loading flag
	delete(c.rgLoading, subID)

	// Update metrics
	utils.GetMetrics().UpdateCacheSize(len(c.rgs), len(c.res), len(c.fullRes))
}

// GetRes retrieves cached resources for a resource group
// Returns nil, false if not found or expired
func (c *PreloadCache) GetRes(subID, rgName string) ([]*domain.Resource, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := subID + "/" + rgName
	cached, ok := c.res[key]
	if !ok {
		utils.GetMetrics().RecordCacheMiss()
		return nil, false
	}

	if c.isExpired(cached.timestamp, resCacheTTL) {
		utils.GetMetrics().RecordCacheMiss()
		return nil, false
	}

	utils.GetMetrics().RecordCacheHit()
	return cached.resources, true
}

// SetRes stores resources for a resource group in cache
// Cancels any existing preload operation for this resource group
func (c *PreloadCache) SetRes(subID, rgName string, resources []*domain.Resource, cancel context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := subID + "/" + rgName

	// Cancel existing preload if any
	if existing, ok := c.res[key]; ok && existing.cancel != nil {
		existing.cancel()
	}

	// Check if we need to evict
	if len(c.res) >= c.resLimit {
		evicted := c.resLimit / 2
		c.evictOldestRes(evicted)
		utils.GetMetrics().RecordEviction(evicted)
	}

	c.res[key] = &cachedResources{
		resources: resources,
		timestamp: time.Now(),
		cancel:    cancel,
	}
	// Clear loading flag
	delete(c.resLoading, key)

	// Update metrics
	utils.GetMetrics().UpdateCacheSize(len(c.rgs), len(c.res), len(c.fullRes))
}

// IsRGLoading checks if resource groups are currently being loaded for a subscription
func (c *PreloadCache) IsRGLoading(subID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rgLoading[subID]
}

// SetRGLoading sets the loading state for resource groups
func (c *PreloadCache) SetRGLoading(subID string, loading bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if loading {
		c.rgLoading[subID] = true
	} else {
		delete(c.rgLoading, subID)
	}
}

// TryStartRGLoading atomically checks if resource groups are loading and marks them as loading if not.
// Returns true if it successfully marked them as loading (caller should proceed with loading),
// or false if they are already being loaded by another goroutine (caller should skip).
func (c *PreloadCache) TryStartRGLoading(subID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rgLoading[subID] {
		return false
	}
	c.rgLoading[subID] = true
	return true
}

// IsResLoading checks if resources are currently being loaded for a resource group
func (c *PreloadCache) IsResLoading(subID, rgName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := subID + "/" + rgName
	return c.resLoading[key]
}

// SetResLoading sets the loading state for resources
func (c *PreloadCache) SetResLoading(subID, rgName string, loading bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := subID + "/" + rgName
	if loading {
		c.resLoading[key] = true
	} else {
		delete(c.resLoading, key)
	}
}

// TryStartResLoading atomically checks if resources are loading and marks them as loading if not.
// Returns true if it successfully marked them as loading (caller should proceed with loading),
// or false if they are already being loaded by another goroutine (caller should skip).
func (c *PreloadCache) TryStartResLoading(subID, rgName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := subID + "/" + rgName
	if c.resLoading[key] {
		return false
	}
	c.resLoading[key] = true
	return true
}

// GetFullRes retrieves cached full resource details
// Returns nil, false if not found or expired
func (c *PreloadCache) GetFullRes(resourceID string) (*domain.Resource, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, ok := c.fullRes[resourceID]
	if !ok {
		utils.GetMetrics().RecordCacheMiss()
		return nil, false
	}

	if c.isExpired(cached.timestamp, resCacheTTL) {
		utils.GetMetrics().RecordCacheMiss()
		return nil, false
	}

	utils.GetMetrics().RecordCacheHit()
	return cached.resource, true
}

// SetFullRes stores full resource details in cache
func (c *PreloadCache) SetFullRes(resourceID string, resource *domain.Resource, cancel context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel existing preload if any
	if existing, ok := c.fullRes[resourceID]; ok && existing.cancel != nil {
		existing.cancel()
	}

	// Check if we need to evict
	if len(c.fullRes) >= c.fullResLimit {
		evicted := c.fullResLimit / 2
		c.evictOldestFullRes(evicted)
		utils.GetMetrics().RecordEviction(evicted)
	}

	c.fullRes[resourceID] = &cachedFullResource{
		resource:  resource,
		timestamp: time.Now(),
		cancel:    cancel,
	}
	// Clear loading flag
	delete(c.fullResLoading, resourceID)

	// Update metrics
	utils.GetMetrics().UpdateCacheSize(len(c.rgs), len(c.res), len(c.fullRes))
}

// IsFullResLoading checks if full resource details are currently being loaded
func (c *PreloadCache) IsFullResLoading(resourceID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fullResLoading[resourceID]
}

// SetFullResLoading sets the loading state for full resource details
func (c *PreloadCache) SetFullResLoading(resourceID string, loading bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if loading {
		c.fullResLoading[resourceID] = true
	} else {
		delete(c.fullResLoading, resourceID)
	}
}

// TryStartFullResLoading atomically checks if full resource details are loading and marks them as loading if not.
// Returns true if it successfully marked them as loading (caller should proceed with loading),
// or false if they are already being loaded by another goroutine (caller should skip).
func (c *PreloadCache) TryStartFullResLoading(resourceID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fullResLoading[resourceID] {
		return false
	}
	c.fullResLoading[resourceID] = true
	return true
}

// InvalidateSubs clears all cached subscriptions, resource groups, and resources
func (c *PreloadCache) InvalidateSubs() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel all active preloads
	for _, cached := range c.rgs {
		if cached.cancel != nil {
			cached.cancel()
		}
	}
	for _, cached := range c.res {
		if cached.cancel != nil {
			cached.cancel()
		}
	}
	for _, cached := range c.fullRes {
		if cached.cancel != nil {
			cached.cancel()
		}
	}

	c.rgs = make(map[string]*cachedRGs)
	c.res = make(map[string]*cachedResources)
	c.fullRes = make(map[string]*cachedFullResource)
	c.rgLoading = make(map[string]bool)
	c.resLoading = make(map[string]bool)
	c.fullResLoading = make(map[string]bool)

	// Update metrics
	utils.GetMetrics().UpdateCacheSize(0, 0, 0)
}

// InvalidateRGs clears cached resource groups for a subscription and their resources
func (c *PreloadCache) InvalidateRGs(subID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel and remove RG cache
	if cached, ok := c.rgs[subID]; ok {
		if cached.cancel != nil {
			cached.cancel()
		}
		delete(c.rgs, subID)
	}

	// Cancel and remove all resource caches for this subscription
	prefix := subID + "/"
	for key, cached := range c.res {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			if cached.cancel != nil {
				cached.cancel()
			}
			delete(c.res, key)
		}
	}

	// Clear all resource loading flags for this subscription
	for key := range c.resLoading {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(c.resLoading, key)
		}
	}

	// Clear loading flag for this subscription
	delete(c.rgLoading, subID)

	// Update metrics
	utils.GetMetrics().UpdateCacheSize(len(c.rgs), len(c.res), len(c.fullRes))
}

// InvalidateRes clears cached resources for a specific resource group
func (c *PreloadCache) InvalidateRes(subID, rgName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := subID + "/" + rgName
	if cached, ok := c.res[key]; ok {
		if cached.cancel != nil {
			cached.cancel()
		}
		delete(c.res, key)
	}
	// Clear loading flag
	delete(c.resLoading, key)

	// Update metrics
	utils.GetMetrics().UpdateCacheSize(len(c.rgs), len(c.res), len(c.fullRes))
}

// isExpired checks if a timestamp has exceeded the TTL
func (c *PreloadCache) isExpired(timestamp time.Time, ttl time.Duration) bool {
	return time.Since(timestamp) > ttl
}

// evictOldestRGs removes the oldest N resource group cache entries
func (c *PreloadCache) evictOldestRGs(count int) {
	if count <= 0 {
		return
	}

	type keyTime struct {
		key       string
		timestamp time.Time
	}
	entries := make([]keyTime, 0, len(c.rgs))
	for key, cached := range c.rgs {
		entries = append(entries, keyTime{key, cached.timestamp})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].timestamp.Before(entries[j].timestamp)
	})

	for i := 0; i < count && i < len(entries); i++ {
		key := entries[i].key
		if cached, ok := c.rgs[key]; ok && cached.cancel != nil {
			cached.cancel()
		}
		delete(c.rgs, key)
	}
}

// evictOldestRes removes the oldest N resource cache entries
func (c *PreloadCache) evictOldestRes(count int) {
	if count <= 0 {
		return
	}

	type keyTime struct {
		key       string
		timestamp time.Time
	}
	entries := make([]keyTime, 0, len(c.res))
	for key, cached := range c.res {
		entries = append(entries, keyTime{key, cached.timestamp})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].timestamp.Before(entries[j].timestamp)
	})

	for i := 0; i < count && i < len(entries); i++ {
		key := entries[i].key
		if cached, ok := c.res[key]; ok && cached.cancel != nil {
			cached.cancel()
		}
		delete(c.res, key)
	}
}

// evictOldestFullRes removes the oldest N full resource cache entries
func (c *PreloadCache) evictOldestFullRes(count int) {
	if count <= 0 {
		return
	}

	type keyTime struct {
		key       string
		timestamp time.Time
	}
	entries := make([]keyTime, 0, len(c.fullRes))
	for key, cached := range c.fullRes {
		entries = append(entries, keyTime{key, cached.timestamp})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].timestamp.Before(entries[j].timestamp)
	})

	for i := 0; i < count && i < len(entries); i++ {
		key := entries[i].key
		if cached, ok := c.fullRes[key]; ok && cached.cancel != nil {
			cached.cancel()
		}
		delete(c.fullRes, key)
		delete(c.fullResLoading, key)
	}
}

// GetCacheStats returns current cache statistics (for debugging)
func (c *PreloadCache) GetCacheStats() (rgCount, resCount, fullResCount int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.rgs), len(c.res), len(c.fullRes)
}

// GetSemaphore returns the semaphore for limiting concurrent operations
func (c *PreloadCache) GetSemaphore() *Semaphore {
	return c.semaphore
}
