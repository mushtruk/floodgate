// Package opentelemetry provides an OpenTelemetry implementation of the floodgate.MetricsCollector interface.
//
// This implementation exposes backpressure metrics using OpenTelemetry, allowing you to:
// - Export metrics to any OpenTelemetry-compatible backend (Prometheus, Jaeger, etc.)
// - Integrate with distributed tracing for correlation
// - Use vendor-neutral observability standards
// - Benefit from OpenTelemetry's ecosystem and tooling
//
// Example usage:
//
//	import (
//	    "go.opentelemetry.io/otel"
//	    otelmetrics "github.com/mushtruk/floodgate/metrics/opentelemetry"
//	)
//
//	// Create OpenTelemetry meter
//	meter := otel.Meter("floodgate")
//
//	// Create metrics collector
//	metrics := otelmetrics.NewMetrics(meter)
//	cfg.Metrics = metrics
package opentelemetry

import (
	"context"
	"time"

	"github.com/mushtruk/floodgate"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics implements floodgate.MetricsCollector using OpenTelemetry.
type Metrics struct {
	requestsTotal    metric.Int64Counter
	requestsRejected metric.Int64Counter
	latencyHistogram metric.Float64Histogram
	circuitBreaker   metric.Int64Gauge
	cacheSize        metric.Int64Gauge
	dispatcherDrops  metric.Int64Counter
	dispatcherTotal  metric.Int64Counter

	// Track previous values for delta calculation
	lastDropped uint64
	lastTotal   uint64
}

// NewMetrics creates a new OpenTelemetry metrics collector.
// The provided meter is used to create all metric instruments.
//
// If meter is nil, this function will panic.
// Use otel.Meter("floodgate") to create a meter.
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	requestsTotal, err := meter.Int64Counter(
		"floodgate.requests.total",
		metric.WithDescription("Total number of requests processed by method, level, and result"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	requestsRejected, err := meter.Int64Counter(
		"floodgate.requests.rejected",
		metric.WithDescription("Total number of requests rejected due to backpressure by method and level"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	latencyHistogram, err := meter.Float64Histogram(
		"floodgate.request.duration",
		metric.WithDescription("Request latency distribution in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0),
	)
	if err != nil {
		return nil, err
	}

	circuitBreaker, err := meter.Int64Gauge(
		"floodgate.circuit_breaker.state",
		metric.WithDescription("Circuit breaker state by method (0=closed, 1=open, 2=half-open)"),
		metric.WithUnit("{state}"),
	)
	if err != nil {
		return nil, err
	}

	cacheSize, err := meter.Int64Gauge(
		"floodgate.cache.size",
		metric.WithDescription("Number of active method/route trackers in cache"),
		metric.WithUnit("{tracker}"),
	)
	if err != nil {
		return nil, err
	}

	dispatcherDrops, err := meter.Int64Counter(
		"floodgate.dispatcher.drops",
		metric.WithDescription("Total number of events dropped by async dispatcher due to buffer overflow"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	dispatcherTotal, err := meter.Int64Counter(
		"floodgate.dispatcher.events",
		metric.WithDescription("Total number of events emitted to async dispatcher"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		requestsTotal:    requestsTotal,
		requestsRejected: requestsRejected,
		latencyHistogram: latencyHistogram,
		circuitBreaker:   circuitBreaker,
		cacheSize:        cacheSize,
		dispatcherDrops:  dispatcherDrops,
		dispatcherTotal:  dispatcherTotal,
	}, nil
}

// RecordRequest implements floodgate.MetricsCollector.
func (m *Metrics) RecordRequest(ctx context.Context, labels floodgate.RequestLabels, latency time.Duration, rejected bool) {
	attrs := []attribute.KeyValue{
		attribute.String("method", labels.Method),
		attribute.String("level", labels.Level.String()),
		attribute.String("result", labels.Result),
	}

	// Increment total requests
	m.requestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))

	// Track rejections separately for easier alerting
	if rejected {
		rejectAttrs := []attribute.KeyValue{
			attribute.String("method", labels.Method),
			attribute.String("level", labels.Level.String()),
		}
		m.requestsRejected.Add(ctx, 1, metric.WithAttributes(rejectAttrs...))
	}

	// Record latency distribution
	latencyAttrs := []attribute.KeyValue{
		attribute.String("method", labels.Method),
	}
	m.latencyHistogram.Record(ctx, latency.Seconds(), metric.WithAttributes(latencyAttrs...))
}

// RecordCircuitBreakerState implements floodgate.MetricsCollector.
func (m *Metrics) RecordCircuitBreakerState(method string, state floodgate.CircuitState) {
	var stateValue int64
	switch state {
	case floodgate.StateClosed:
		stateValue = 0
	case floodgate.StateOpen:
		stateValue = 1
	case floodgate.StateHalfOpen:
		stateValue = 2
	}

	attrs := []attribute.KeyValue{
		attribute.String("method", method),
	}
	m.circuitBreaker.Record(context.Background(), stateValue, metric.WithAttributes(attrs...))
}

// RecordCacheSize implements floodgate.MetricsCollector.
func (m *Metrics) RecordCacheSize(size int) {
	m.cacheSize.Record(context.Background(), int64(size))
}

// RecordDispatcherStats implements floodgate.MetricsCollector.
func (m *Metrics) RecordDispatcherStats(dropped, total uint64) {
	ctx := context.Background()

	// Calculate deltas since last call (counters must always increase)
	dropsDelta := int64(dropped - m.lastDropped)
	totalDelta := int64(total - m.lastTotal)

	if dropsDelta > 0 {
		m.dispatcherDrops.Add(ctx, dropsDelta)
	}
	if totalDelta > 0 {
		m.dispatcherTotal.Add(ctx, totalDelta)
	}

	// Update last known values
	m.lastDropped = dropped
	m.lastTotal = total
}
