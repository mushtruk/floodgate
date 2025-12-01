package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushtruk/floodgate"
)

func TestNewBackpressureCore_Defaults(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := Config{
		CacheSize:            100,
		CacheTTL:             1 * time.Minute,
		DispatcherBufferSize: 100,
	}

	core := NewBackpressureCore(ctx, cfg)

	if core == nil {
		t.Fatal("NewBackpressureCore returned nil")
	}

	if !core.started.Load() {
		t.Error("Core should be started")
	}

	// Should have default logger
	if core.logger == nil {
		t.Error("Core should have default logger")
	}

	// Should have default metrics
	if core.metrics == nil {
		t.Error("Core should have default metrics")
	}

	// Should have default algorithm
	if core.algorithm == nil {
		t.Error("Core should have default algorithm")
	}
}

func TestBackpressureCore_CheckBackpressure_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := Config{
		CacheSize:            100,
		CacheTTL:             1 * time.Minute,
		DispatcherBufferSize: 100,
		Algorithm:            &floodgate.NoOpAlgorithm{}, // Always allows
	}

	core := NewBackpressureCore(ctx, cfg)

	result, err := core.CheckBackpressure(ctx, "test.method")

	if err != nil {
		t.Errorf("CheckBackpressure returned error: %v", err)
	}

	if result == nil {
		t.Fatal("Result should not be nil")
	}

	if result.Decision.Reject {
		t.Error("NoOpAlgorithm should not reject")
	}

	if result.Decision.Level != floodgate.Normal {
		t.Errorf("Level = %v, want Normal", result.Decision.Level)
	}

	if result.Tracker == nil {
		t.Error("Tracker should not be nil")
	}
}

func TestBackpressureCore_CheckBackpressure_AlgorithmRejects(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Mock algorithm that always rejects
	mockAlgo := &MockAlgorithm{
		DecideFunc: func(stats floodgate.Stats) floodgate.Decision {
			return floodgate.Decision{
				Level:  floodgate.Emergency,
				Reject: true,
			}
		},
	}

	cfg := Config{
		CacheSize:            100,
		CacheTTL:             1 * time.Minute,
		DispatcherBufferSize: 100,
		Algorithm:            mockAlgo,
	}

	core := NewBackpressureCore(ctx, cfg)

	result, err := core.CheckBackpressure(ctx, "test.method")

	if !errors.Is(err, floodgate.ErrBackpressure) {
		t.Errorf("Expected ErrBackpressure, got %v", err)
	}

	if result == nil {
		t.Fatal("Result should not be nil")
	}

	if !result.Decision.Reject {
		t.Error("Decision should reject")
	}

	if result.Decision.Level != floodgate.Emergency {
		t.Errorf("Level = %v, want Emergency", result.Decision.Level)
	}
}

func TestBackpressureCore_CheckBackpressure_CircuitBreakerOpen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cfg := Config{
		CacheSize:                      100,
		CacheTTL:                       1 * time.Minute,
		DispatcherBufferSize:           100,
		CircuitBreakerMaxFailures:      1,
		CircuitBreakerTimeout:          1 * time.Second,
		CircuitBreakerSuccessThreshold: 1,
	}

	core := NewBackpressureCore(ctx, cfg)

	// Force circuit breaker to open - need to wait minTimeBetweenOps (1 second)
	core.circuitBreaker.RecordFailure()
	time.Sleep(1100 * time.Millisecond) // Wait for minTimeBetweenOps
	core.circuitBreaker.RecordFailure()

	if core.circuitBreaker.State() != floodgate.StateOpen {
		t.Errorf("Circuit should be open, got %v", core.circuitBreaker.State())
	}

	result, err := core.CheckBackpressure(ctx, "test.method")

	if !errors.Is(err, floodgate.ErrBackpressure) {
		t.Errorf("Expected ErrBackpressure, got %v", err)
	}

	if result == nil {
		t.Fatal("Result should not be nil")
	}

	if !result.Decision.Reject {
		t.Error("Decision should reject when circuit is open")
	}
}

func TestBackpressureCore_RecordLatency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := Config{
		CacheSize:            100,
		CacheTTL:             1 * time.Minute,
		DispatcherBufferSize: 100,
	}

	core := NewBackpressureCore(ctx, cfg)

	// First get a tracker
	result, err := core.CheckBackpressure(ctx, "test.method")
	if err != nil {
		t.Fatalf("CheckBackpressure failed: %v", err)
	}

	// Verify RouteKey is populated in DecisionResult
	if result.RouteKey != "test.method" {
		t.Errorf("RouteKey = %q, want %q", result.RouteKey, "test.method")
	}

	// Record latency
	core.RecordLatency(ctx, result, 100*time.Millisecond, nil)

	// Dispatcher is async, give it time to process
	time.Sleep(50 * time.Millisecond)

	// Verify tracker was updated
	stats := result.Tracker.Value()
	if stats.EMA == 0 {
		t.Error("EMA should be updated after RecordLatency")
	}
}

func TestBackpressureCore_TrackerCaching(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := Config{
		CacheSize:            100,
		CacheTTL:             1 * time.Minute,
		DispatcherBufferSize: 100,
	}

	core := NewBackpressureCore(ctx, cfg)

	// Request same route twice
	result1, _ := core.CheckBackpressure(ctx, "test.method")
	result2, _ := core.CheckBackpressure(ctx, "test.method")

	// Should return same tracker instance
	if result1.Tracker != result2.Tracker {
		t.Error("Should return same tracker for same route")
	}

	// Different route should have different tracker
	result3, _ := core.CheckBackpressure(ctx, "other.method")
	if result1.Tracker == result3.Tracker {
		t.Error("Different routes should have different trackers")
	}
}

func TestBackpressureCore_Stats(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := Config{
		CacheSize:            100,
		CacheTTL:             1 * time.Minute,
		DispatcherBufferSize: 100,
	}

	core := NewBackpressureCore(ctx, cfg)

	// Add some routes
	core.CheckBackpressure(ctx, "route1")
	core.CheckBackpressure(ctx, "route2")

	cacheSize, dropRate, circuitState := core.Stats()

	if cacheSize != 2 {
		t.Errorf("CacheSize = %d, want 2", cacheSize)
	}

	if dropRate != 0 {
		t.Errorf("DropRate = %f, want 0", dropRate)
	}

	if circuitState != floodgate.StateClosed {
		t.Errorf("CircuitState = %v, want Closed", circuitState)
	}
}

func TestBackpressureCore_Reset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := Config{
		CacheSize:                 100,
		CacheTTL:                  1 * time.Minute,
		DispatcherBufferSize:      100,
		CircuitBreakerMaxFailures: 1,
	}

	core := NewBackpressureCore(ctx, cfg)

	// Open circuit - need to wait minTimeBetweenOps (1 second)
	core.circuitBreaker.RecordFailure()
	time.Sleep(1100 * time.Millisecond)
	core.circuitBreaker.RecordFailure()

	if core.circuitBreaker.State() != floodgate.StateOpen {
		t.Fatal("Circuit should be open")
	}

	// Reset
	core.Reset()

	if core.circuitBreaker.State() != floodgate.StateClosed {
		t.Errorf("Circuit should be closed after reset, got %v", core.circuitBreaker.State())
	}
}

// MockAlgorithm for testing
type MockAlgorithm struct {
	DecideFunc func(floodgate.Stats) floodgate.Decision
}

func (m *MockAlgorithm) Decide(stats floodgate.Stats) floodgate.Decision {
	if m.DecideFunc != nil {
		return m.DecideFunc(stats)
	}
	return floodgate.Decision{Level: floodgate.Normal, Reject: false}
}
