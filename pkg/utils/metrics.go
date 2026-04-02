package utils

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics provides performance tracking for cache operations and API calls
type Metrics struct {
	mu sync.RWMutex

	// Cache metrics
	cacheHits   int64
	cacheMisses int64
	evictions   int64

	// API call metrics
	apiCalls    int64
	apiErrors   int64
	apiDuration int64 // total duration in nanoseconds

	// Cache size tracking
	currentRGSize      int
	currentResSize     int
	currentFullResSize int
	maxRGSize          int
	maxResSize         int
	maxFullResSize     int
}

// Global metrics instance
var globalMetrics = &Metrics{}

// GetMetrics returns the global metrics instance
func GetMetrics() *Metrics {
	return globalMetrics
}

// Reset clears all metrics (useful for testing)
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	atomic.StoreInt64(&m.cacheHits, 0)
	atomic.StoreInt64(&m.cacheMisses, 0)
	atomic.StoreInt64(&m.evictions, 0)
	atomic.StoreInt64(&m.apiCalls, 0)
	atomic.StoreInt64(&m.apiErrors, 0)
	atomic.StoreInt64(&m.apiDuration, 0)

	m.currentRGSize = 0
	m.currentResSize = 0
	m.currentFullResSize = 0
	m.maxRGSize = 0
	m.maxResSize = 0
	m.maxFullResSize = 0
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit() {
	atomic.AddInt64(&m.cacheHits, 1)
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss() {
	atomic.AddInt64(&m.cacheMisses, 1)
}

// RecordEviction records a cache eviction
func (m *Metrics) RecordEviction(count int) {
	atomic.AddInt64(&m.evictions, int64(count))
}

// RecordAPICall records an API call with its duration
func (m *Metrics) RecordAPICall(duration time.Duration, err error) {
	atomic.AddInt64(&m.apiCalls, 1)
	atomic.AddInt64(&m.apiDuration, duration.Nanoseconds())
	if err != nil {
		atomic.AddInt64(&m.apiErrors, 1)
	}
}

// UpdateCacheSize updates the current cache size tracking
func (m *Metrics) UpdateCacheSize(rgCount, resCount, fullResCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentRGSize = rgCount
	m.currentResSize = resCount
	m.currentFullResSize = fullResCount

	if rgCount > m.maxRGSize {
		m.maxRGSize = rgCount
	}
	if resCount > m.maxResSize {
		m.maxResSize = resCount
	}
	if fullResCount > m.maxFullResSize {
		m.maxFullResSize = fullResCount
	}
}

// GetCacheHitRate returns the cache hit rate as a percentage
func (m *Metrics) GetCacheHitRate() float64 {
	hits := atomic.LoadInt64(&m.cacheHits)
	misses := atomic.LoadInt64(&m.cacheMisses)
	total := hits + misses

	if total == 0 {
		return 0.0
	}
	return float64(hits) / float64(total) * 100.0
}

// GetStats returns a snapshot of current metrics
func (m *Metrics) GetStats() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hits := atomic.LoadInt64(&m.cacheHits)
	misses := atomic.LoadInt64(&m.cacheMisses)
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100.0
	}

	apiCalls := atomic.LoadInt64(&m.apiCalls)
	var avgAPIDuration time.Duration
	if apiCalls > 0 {
		avgNs := atomic.LoadInt64(&m.apiDuration) / apiCalls
		avgAPIDuration = time.Duration(avgNs)
	}

	return MetricsSnapshot{
		CacheHits:          hits,
		CacheMisses:        misses,
		CacheHitRate:       hitRate,
		Evictions:          atomic.LoadInt64(&m.evictions),
		APICalls:           apiCalls,
		APIErrors:          atomic.LoadInt64(&m.apiErrors),
		AvgAPIDuration:     avgAPIDuration,
		CurrentRGSize:      m.currentRGSize,
		CurrentResSize:     m.currentResSize,
		CurrentFullResSize: m.currentFullResSize,
		MaxRGSize:          m.maxRGSize,
		MaxResSize:         m.maxResSize,
		MaxFullResSize:     m.maxFullResSize,
	}
}

// MetricsSnapshot provides a point-in-time view of metrics
type MetricsSnapshot struct {
	CacheHits          int64
	CacheMisses        int64
	CacheHitRate       float64
	Evictions          int64
	APICalls           int64
	APIErrors          int64
	AvgAPIDuration     time.Duration
	CurrentRGSize      int
	CurrentResSize     int
	CurrentFullResSize int
	MaxRGSize          int
	MaxResSize         int
	MaxFullResSize     int
}

// String returns a formatted string representation of metrics
func (s MetricsSnapshot) String() string {
	return fmt.Sprintf(
		"Cache: %d hits, %d misses (%.1f%% hit rate), %d evictions | "+
			"API: %d calls, %d errors, avg %v | "+
			"Size: RG=%d/%d, Res=%d/%d, FullRes=%d/%d",
		s.CacheHits, s.CacheMisses, s.CacheHitRate, s.Evictions,
		s.APICalls, s.APIErrors, s.AvgAPIDuration,
		s.CurrentRGSize, s.MaxRGSize,
		s.CurrentResSize, s.MaxResSize,
		s.CurrentFullResSize, s.MaxFullResSize,
	)
}

// LogMetrics logs current metrics to the debug log
func LogMetrics() {
	if !IsDebugEnabled() {
		return
	}

	stats := globalMetrics.GetStats()
	Log("[METRICS] %s", stats.String())
}

// StartAPITimer starts a timer for an API call and returns a function to record the result
// Usage: defer StartAPITimer("operation")()
func StartAPITimer(operation string) func(error) {
	start := time.Now()
	return func(err error) {
		duration := time.Since(start)
		globalMetrics.RecordAPICall(duration, err)
		if IsDebugEnabled() {
			status := "success"
			if err != nil {
				status = "error"
			}
			Log("[API] %s completed in %v (%s)", operation, duration, status)
		}
	}
}
