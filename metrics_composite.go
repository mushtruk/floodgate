package floodgate

import (
	"context"
	"time"
)

// CompositeMetrics forwards metrics to multiple collectors.
// This enables sending metrics to multiple backends (Prometheus, Datadog, etc.) simultaneously.
//
// Example usage:
//
//	composite := floodgate.NewCompositeMetrics(
//	    prometheus.NewCollector(),
//	    datadog.NewCollector(),
//	    customLogger,
//	)
type CompositeMetrics struct {
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
	for _, collector := range c.collectors {
		collector.RecordRequest(ctx, labels, latency, rejected)
	}
}

// RecordCacheSize forwards to all collectors.
func (c *CompositeMetrics) RecordCacheSize(size int) {
	for _, collector := range c.collectors {
		collector.RecordCacheSize(size)
	}
}

// RecordDispatcherStats forwards to all collectors.
func (c *CompositeMetrics) RecordDispatcherStats(dropped, total uint64) {
	for _, collector := range c.collectors {
		collector.RecordDispatcherStats(dropped, total)
	}
}

// RecordCircuitBreakerState forwards to all collectors.
func (c *CompositeMetrics) RecordCircuitBreakerState(method string, state CircuitState) {
	for _, collector := range c.collectors {
		collector.RecordCircuitBreakerState(method, state)
	}
}

// Add adds a new collector to the composite.
func (c *CompositeMetrics) Add(collector MetricsCollector) {
	c.collectors = append(c.collectors, collector)
}

// Remove removes a collector from the composite.
func (c *CompositeMetrics) Remove(collector MetricsCollector) {
	for i, col := range c.collectors {
		if col == collector {
			c.collectors = append(c.collectors[:i], c.collectors[i+1:]...)
			return
		}
	}
}

// Count returns the number of collectors in the composite.
func (c *CompositeMetrics) Count() int {
	return len(c.collectors)
}
