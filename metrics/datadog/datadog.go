// Package datadog provides a Datadog implementation of the floodgate.MetricsCollector interface.
//
// This implementation sends backpressure metrics to Datadog using the official DogStatsD client,
// allowing you to:
// - Monitor backpressure in Datadog dashboards
// - Create alerts on rejection rates and circuit breaker states
// - Correlate with APM traces and infrastructure metrics
// - Use Datadog's powerful querying and visualization
//
// Example usage:
//
//	import (
//	    "github.com/DataDog/datadog-go/v5/statsd"
//	    ddmetrics "github.com/mushtruk/floodgate/metrics/datadog"
//	)
//
//	// Create DogStatsD client
//	client, _ := statsd.New("localhost:8125")
//	defer client.Close()
//
//	// Create metrics collector
//	metrics := ddmetrics.NewMetrics(client, ddmetrics.WithNamespace("myapp"))
//	cfg.Metrics = metrics
package datadog

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/mushtruk/floodgate"
)

// Metrics implements floodgate.MetricsCollector using Datadog DogStatsD.
type Metrics struct {
	client    statsd.ClientInterface
	namespace string
	tags      []string

	// Track previous values for delta calculation
	lastDropped uint64
	lastTotal   uint64
}

// Option configures Datadog metrics.
type Option func(*Metrics)

// WithNamespace sets a namespace prefix for all metrics.
// Example: WithNamespace("myapp") produces "myapp.floodgate.requests.total"
func WithNamespace(ns string) Option {
	return func(m *Metrics) {
		m.namespace = ns
	}
}

// WithTags adds global tags to all metrics.
// Example: WithTags("env:prod", "service:api")
func WithTags(tags ...string) Option {
	return func(m *Metrics) {
		m.tags = append(m.tags, tags...)
	}
}

// NewMetrics creates a new Datadog metrics collector.
// The provided client is used to send metrics via DogStatsD.
//
// Example:
//
//	client, _ := statsd.New("localhost:8125",
//	    statsd.WithNamespace("myapp"),
//	    statsd.WithTags([]string{"env:prod"}),
//	)
//	metrics := ddmetrics.NewMetrics(client)
func NewMetrics(client statsd.ClientInterface, opts ...Option) *Metrics {
	m := &Metrics{
		client: client,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// metricName builds the full metric name with optional namespace.
func (m *Metrics) metricName(name string) string {
	if m.namespace != "" {
		return m.namespace + ".floodgate." + name
	}
	return "floodgate." + name
}

// mergeTags combines global tags with metric-specific tags.
func (m *Metrics) mergeTags(tags []string) []string {
	if len(m.tags) == 0 {
		return tags
	}
	merged := make([]string, 0, len(m.tags)+len(tags))
	merged = append(merged, m.tags...)
	merged = append(merged, tags...)
	return merged
}

// RecordRequest implements floodgate.MetricsCollector.
func (m *Metrics) RecordRequest(ctx context.Context, labels floodgate.RequestLabels, latency time.Duration, rejected bool) {
	tags := []string{
		fmt.Sprintf("method:%s", labels.Method),
		fmt.Sprintf("level:%s", labels.Level),
		fmt.Sprintf("result:%s", labels.Result),
	}
	tags = m.mergeTags(tags)

	// Increment total requests
	_ = m.client.Incr(m.metricName("requests.total"), tags, 1.0)

	// Track rejections separately for easier alerting
	if rejected {
		rejectTags := []string{
			fmt.Sprintf("method:%s", labels.Method),
			fmt.Sprintf("level:%s", labels.Level),
		}
		rejectTags = m.mergeTags(rejectTags)
		_ = m.client.Incr(m.metricName("requests.rejected"), rejectTags, 1.0)
	}

	// Record latency distribution
	latencyTags := []string{
		fmt.Sprintf("method:%s", labels.Method),
	}
	latencyTags = m.mergeTags(latencyTags)
	_ = m.client.Timing(m.metricName("request.duration"), latency, latencyTags, 1.0)
}

// RecordCircuitBreakerState implements floodgate.MetricsCollector.
func (m *Metrics) RecordCircuitBreakerState(method string, state floodgate.CircuitState) {
	var stateValue int64
	var stateName string

	switch state {
	case floodgate.StateClosed:
		stateValue = 0
		stateName = "closed"
	case floodgate.StateOpen:
		stateValue = 1
		stateName = "open"
	case floodgate.StateHalfOpen:
		stateValue = 2
		stateName = "half_open"
	}

	tags := []string{
		fmt.Sprintf("method:%s", method),
		fmt.Sprintf("state:%s", stateName),
	}
	tags = m.mergeTags(tags)

	_ = m.client.Gauge(m.metricName("circuit_breaker.state"), float64(stateValue), tags, 1.0)
}

// RecordCacheSize implements floodgate.MetricsCollector.
func (m *Metrics) RecordCacheSize(size int) {
	tags := m.mergeTags(nil)
	_ = m.client.Gauge(m.metricName("cache.size"), float64(size), tags, 1.0)
}

// RecordDispatcherStats implements floodgate.MetricsCollector.
func (m *Metrics) RecordDispatcherStats(dropped, total uint64) {
	tags := m.mergeTags(nil)

	// Calculate deltas since last call (counters should track increments)
	dropsDelta := int64(dropped - m.lastDropped)
	totalDelta := int64(total - m.lastTotal)

	if dropsDelta > 0 {
		_ = m.client.Count(m.metricName("dispatcher.drops"), dropsDelta, tags, 1.0)
	}
	if totalDelta > 0 {
		_ = m.client.Count(m.metricName("dispatcher.events"), totalDelta, tags, 1.0)
	}

	// Also send gauges for current absolute values
	_ = m.client.Gauge(m.metricName("dispatcher.drops.total"), float64(dropped), tags, 1.0)
	_ = m.client.Gauge(m.metricName("dispatcher.events.total"), float64(total), tags, 1.0)

	// Update last known values
	m.lastDropped = dropped
	m.lastTotal = total
}
