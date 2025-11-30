package floodgate

import (
	"context"
	"testing"
	"time"
)

// MockMetricsCollector for testing
type MockMetricsCollector struct {
	requests          int
	cacheSize         int
	dispatcherCalls   int
	circuitBreakerCalls int
}

func (m *MockMetricsCollector) RecordRequest(ctx context.Context, labels RequestLabels, latency time.Duration, rejected bool) {
	m.requests++
}

func (m *MockMetricsCollector) RecordCacheSize(size int) {
	m.cacheSize = size
}

func (m *MockMetricsCollector) RecordDispatcherStats(dropped, total uint64) {
	m.dispatcherCalls++
}

func (m *MockMetricsCollector) RecordCircuitBreakerState(method string, state CircuitState) {
	m.circuitBreakerCalls++
}

func TestCompositeMetrics_RecordRequest(t *testing.T) {
	t.Parallel()

	mock1 := &MockMetricsCollector{}
	mock2 := &MockMetricsCollector{}

	composite := NewCompositeMetrics(mock1, mock2)

	ctx := context.Background()
	labels := RequestLabels{Method: "test", Level: Normal, Result: "success"}

	composite.RecordRequest(ctx, labels, 100*time.Millisecond, false)

	if mock1.requests != 1 {
		t.Errorf("mock1.requests = %d, want 1", mock1.requests)
	}

	if mock2.requests != 1 {
		t.Errorf("mock2.requests = %d, want 1", mock2.requests)
	}
}

func TestCompositeMetrics_RecordCacheSize(t *testing.T) {
	t.Parallel()

	mock1 := &MockMetricsCollector{}
	mock2 := &MockMetricsCollector{}

	composite := NewCompositeMetrics(mock1, mock2)

	composite.RecordCacheSize(42)

	if mock1.cacheSize != 42 {
		t.Errorf("mock1.cacheSize = %d, want 42", mock1.cacheSize)
	}

	if mock2.cacheSize != 42 {
		t.Errorf("mock2.cacheSize = %d, want 42", mock2.cacheSize)
	}
}

func TestCompositeMetrics_RecordDispatcherStats(t *testing.T) {
	t.Parallel()

	mock1 := &MockMetricsCollector{}
	mock2 := &MockMetricsCollector{}

	composite := NewCompositeMetrics(mock1, mock2)

	composite.RecordDispatcherStats(10, 100)

	if mock1.dispatcherCalls != 1 {
		t.Errorf("mock1.dispatcherCalls = %d, want 1", mock1.dispatcherCalls)
	}

	if mock2.dispatcherCalls != 1 {
		t.Errorf("mock2.dispatcherCalls = %d, want 1", mock2.dispatcherCalls)
	}
}

func TestCompositeMetrics_RecordCircuitBreakerState(t *testing.T) {
	t.Parallel()

	mock1 := &MockMetricsCollector{}
	mock2 := &MockMetricsCollector{}

	composite := NewCompositeMetrics(mock1, mock2)

	composite.RecordCircuitBreakerState("test.method", StateOpen)

	if mock1.circuitBreakerCalls != 1 {
		t.Errorf("mock1.circuitBreakerCalls = %d, want 1", mock1.circuitBreakerCalls)
	}

	if mock2.circuitBreakerCalls != 1 {
		t.Errorf("mock2.circuitBreakerCalls = %d, want 1", mock2.circuitBreakerCalls)
	}
}

func TestCompositeMetrics_AddRemove(t *testing.T) {
	t.Parallel()

	mock1 := &MockMetricsCollector{}
	mock2 := &MockMetricsCollector{}
	mock3 := &MockMetricsCollector{}

	composite := NewCompositeMetrics(mock1, mock2)

	if composite.Count() != 2 {
		t.Errorf("Count() = %d, want 2", composite.Count())
	}

	// Add a third collector
	composite.Add(mock3)

	if composite.Count() != 3 {
		t.Errorf("Count() after Add = %d, want 3", composite.Count())
	}

	// Test that all three receive metrics
	composite.RecordCacheSize(100)

	if mock1.cacheSize != 100 || mock2.cacheSize != 100 || mock3.cacheSize != 100 {
		t.Error("All collectors should receive metrics after Add")
	}

	// Remove mock2
	composite.Remove(mock2)

	if composite.Count() != 2 {
		t.Errorf("Count() after Remove = %d, want 2", composite.Count())
	}

	// Reset and test again
	mock1.cacheSize = 0
	mock2.cacheSize = 0
	mock3.cacheSize = 0

	composite.RecordCacheSize(200)

	if mock1.cacheSize != 200 {
		t.Error("mock1 should still receive metrics")
	}

	if mock2.cacheSize != 0 {
		t.Error("mock2 should not receive metrics after removal")
	}

	if mock3.cacheSize != 200 {
		t.Error("mock3 should still receive metrics")
	}
}

func TestCompositeMetrics_EmptyComposite(t *testing.T) {
	t.Parallel()

	composite := NewCompositeMetrics()

	if composite.Count() != 0 {
		t.Errorf("Count() = %d, want 0", composite.Count())
	}

	// Should not panic with no collectors
	ctx := context.Background()
	composite.RecordRequest(ctx, RequestLabels{}, 0, false)
	composite.RecordCacheSize(10)
	composite.RecordDispatcherStats(1, 10)
	composite.RecordCircuitBreakerState("test", StateClosed)
}

func TestCompositeMetrics_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	composite := NewCompositeMetrics()

	// Add initial collectors
	for i := 0; i < 5; i++ {
		composite.Add(&MockMetricsCollector{})
	}

	ctx := context.Background()
	labels := RequestLabels{Method: "test", Level: Normal, Result: "success"}

	// Run concurrent operations
	done := make(chan struct{})

	// Writers: Add/Remove collectors
	go func() {
		for i := 0; i < 100; i++ {
			mock := &MockMetricsCollector{}
			composite.Add(mock)
			composite.Remove(mock)
		}
		done <- struct{}{}
	}()

	// Readers: Record metrics
	go func() {
		for i := 0; i < 1000; i++ {
			composite.RecordRequest(ctx, labels, 100*time.Millisecond, false)
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			composite.RecordCacheSize(i)
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			composite.RecordDispatcherStats(uint64(i), uint64(i*10))
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			composite.RecordCircuitBreakerState("method", StateClosed)
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			_ = composite.Count()
		}
		done <- struct{}{}
	}()

	// Wait for all goroutines
	for i := 0; i < 6; i++ {
		<-done
	}

	// If we get here without a data race, the test passes
}
