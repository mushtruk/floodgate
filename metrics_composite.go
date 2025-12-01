package floodgate

import (
	"context"
	"sync"
	"time"
)

// CompositeMetrics forwards metrics to multiple collectors.
// This enables sending metrics to multiple backends (Prometheus, Datadog, etc.) simultaneously.
//
// CompositeMetrics is safe for concurrent use. Metrics recording uses a read lock,
// while Add/Remove operations use a write lock.
//
// Example usage:
//
//	composite := floodgate.NewCompositeMetrics(
//	    prometheus.NewCollector(),
//	    datadog.NewCollector(),
//	    customLogger,
//	)
//
//nolint:govet // fieldalignment: struct layout prioritizes logical grouping over size
type CompositeMetrics struct {
	mu         sync.RWMutex
	collectors []MetricsCollector
}

// NewCompositeMetrics creates a new composite metrics collector.
func NewCompositeMetrics(collectors ...MetricsCollector) *CompositeMetrics {
	return &CompositeMetrics{
		collectors: collectors,
	}
}

// RecordRequest forwards to all collectors.
func (c *CompositeMetrics) RecordRequest(ctx context.Context, labels RequestLabels, latency time.Duration, rejected bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, collector := range c.collectors {
		collector.RecordRequest(ctx, labels, latency, rejected)
	}
}

// RecordCacheSize forwards to all collectors.
func (c *CompositeMetrics) RecordCacheSize(size int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, collector := range c.collectors {
		collector.RecordCacheSize(size)
	}
}

// RecordDispatcherStats forwards to all collectors.
func (c *CompositeMetrics) RecordDispatcherStats(dropped, total uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, collector := range c.collectors {
		collector.RecordDispatcherStats(dropped, total)
	}
}

// RecordCircuitBreakerState forwards to all collectors.
func (c *CompositeMetrics) RecordCircuitBreakerState(method string, state CircuitState) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, collector := range c.collectors {
		collector.RecordCircuitBreakerState(method, state)
	}
}

// Add adds a new collector to the composite.
func (c *CompositeMetrics) Add(collector MetricsCollector) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.collectors = append(c.collectors, collector)
}

// Remove removes a collector from the composite.
func (c *CompositeMetrics) Remove(collector MetricsCollector) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, col := range c.collectors {
		if col == collector {
			c.collectors = append(c.collectors[:i], c.collectors[i+1:]...)
			return
		}
	}
}

// Count returns the number of collectors in the composite.
func (c *CompositeMetrics) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.collectors)
}
