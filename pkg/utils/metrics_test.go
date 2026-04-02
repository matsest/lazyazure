package utils

import (
	"errors"
	"testing"
	"time"
)

func TestMetrics_CacheHitRate(t *testing.T) {
	m := &Metrics{}
	m.Reset()

	// Initially should be 0
	if rate := m.GetCacheHitRate(); rate != 0.0 {
		t.Errorf("Expected 0%% hit rate, got %.1f%%", rate)
	}

	// Record some hits and misses
	m.RecordCacheHit()
	m.RecordCacheHit()
	m.RecordCacheHit()
	m.RecordCacheMiss()

	// Should be 75%
	if rate := m.GetCacheHitRate(); rate != 75.0 {
		t.Errorf("Expected 75%% hit rate, got %.1f%%", rate)
	}
}

func TestMetrics_Eviction(t *testing.T) {
	m := &Metrics{}
	m.Reset()

	m.RecordEviction(5)
	m.RecordEviction(3)

	stats := m.GetStats()
	if stats.Evictions != 8 {
		t.Errorf("Expected 8 evictions, got %d", stats.Evictions)
	}
}

func TestMetrics_APICall(t *testing.T) {
	m := &Metrics{}
	m.Reset()

	// Record successful API call
	m.RecordAPICall(100*time.Millisecond, nil)

	// Record failed API call
	m.RecordAPICall(50*time.Millisecond, errors.New("test error"))

	stats := m.GetStats()

	if stats.APICalls != 2 {
		t.Errorf("Expected 2 API calls, got %d", stats.APICalls)
	}

	if stats.APIErrors != 1 {
		t.Errorf("Expected 1 API error, got %d", stats.APIErrors)
	}

	// Average should be 75ms
	expectedAvg := 75 * time.Millisecond
	if stats.AvgAPIDuration < expectedAvg-1*time.Millisecond || stats.AvgAPIDuration > expectedAvg+1*time.Millisecond {
		t.Errorf("Expected avg duration ~%v, got %v", expectedAvg, stats.AvgAPIDuration)
	}
}

func TestMetrics_CacheSize(t *testing.T) {
	m := &Metrics{}
	m.Reset()

	// Update sizes
	m.UpdateCacheSize(10, 20, 30)

	stats := m.GetStats()
	if stats.CurrentRGSize != 10 {
		t.Errorf("Expected RG size 10, got %d", stats.CurrentRGSize)
	}
	if stats.CurrentResSize != 20 {
		t.Errorf("Expected Res size 20, got %d", stats.CurrentResSize)
	}
	if stats.CurrentFullResSize != 30 {
		t.Errorf("Expected FullRes size 30, got %d", stats.CurrentFullResSize)
	}

	// Max should be updated
	if stats.MaxRGSize != 10 {
		t.Errorf("Expected max RG size 10, got %d", stats.MaxRGSize)
	}

	// Update with larger sizes
	m.UpdateCacheSize(50, 40, 35)

	stats = m.GetStats()
	if stats.MaxRGSize != 50 {
		t.Errorf("Expected max RG size 50, got %d", stats.MaxRGSize)
	}
	if stats.MaxResSize != 40 {
		t.Errorf("Expected max Res size 40, got %d", stats.MaxResSize)
	}
	if stats.MaxFullResSize != 35 {
		t.Errorf("Expected max FullRes size 35, got %d", stats.MaxFullResSize)
	}
}

func TestMetrics_GetStats(t *testing.T) {
	m := &Metrics{}
	m.Reset()

	// Populate with data
	m.RecordCacheHit()
	m.RecordCacheHit()
	m.RecordCacheMiss()
	m.RecordEviction(1)
	m.RecordAPICall(100*time.Millisecond, nil)
	m.UpdateCacheSize(5, 10, 15)

	stats := m.GetStats()

	// Verify all fields
	if stats.CacheHits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.CacheHits)
	}
	if stats.CacheMisses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.CacheMisses)
	}
	if stats.Evictions != 1 {
		t.Errorf("Expected 1 eviction, got %d", stats.Evictions)
	}
	if stats.APICalls != 1 {
		t.Errorf("Expected 1 API call, got %d", stats.APICalls)
	}
	if stats.CurrentRGSize != 5 {
		t.Errorf("Expected RG size 5, got %d", stats.CurrentRGSize)
	}

	// Check hit rate (allow small floating point variance)
	expectedRate := 66.66666666666667
	if stats.CacheHitRate < expectedRate-0.001 || stats.CacheHitRate > expectedRate+0.001 {
		t.Errorf("Expected ~%.5f%% hit rate, got %f", expectedRate, stats.CacheHitRate)
	}
}

func TestMetricsSnapshot_String(t *testing.T) {
	s := MetricsSnapshot{
		CacheHits:          100,
		CacheMisses:        50,
		CacheHitRate:       66.7,
		Evictions:          10,
		APICalls:           25,
		APIErrors:          2,
		AvgAPIDuration:     150 * time.Millisecond,
		CurrentRGSize:      10,
		CurrentResSize:     50,
		CurrentFullResSize: 25,
		MaxRGSize:          20,
		MaxResSize:         100,
		MaxFullResSize:     50,
	}

	str := s.String()

	// Verify key parts are present
	expectedParts := []string{
		"100 hits",
		"50 misses",
		"66.7%",
		"10 evictions",
		"25 calls",
		"2 errors",
		"RG=10/20",
		"Res=50/100",
		"FullRes=25/50",
	}

	for _, part := range expectedParts {
		if !contains(str, part) {
			t.Errorf("Expected string to contain '%s', got: %s", part, str)
		}
	}
}

func TestGlobalMetrics(t *testing.T) {
	// Ensure global metrics exists
	if GetMetrics() == nil {
		t.Error("GetMetrics() returned nil")
	}

	// Reset to clean state
	GetMetrics().Reset()

	// Record something
	GetMetrics().RecordCacheHit()

	stats := GetMetrics().GetStats()
	if stats.CacheHits != 1 {
		t.Errorf("Expected 1 hit on global metrics, got %d", stats.CacheHits)
	}
}

func TestStartAPITimer(t *testing.T) {
	m := &Metrics{}
	m.Reset()

	// Simulate API call with timer
	record := StartAPITimer("test-operation")
	time.Sleep(10 * time.Millisecond)
	record(nil)

	stats := GetMetrics().GetStats()
	if stats.APICalls != 1 {
		t.Errorf("Expected 1 API call, got %d", stats.APICalls)
	}
	if stats.AvgAPIDuration < 10*time.Millisecond {
		t.Errorf("Expected duration >= 10ms, got %v", stats.AvgAPIDuration)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
