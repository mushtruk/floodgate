// Package prometheus provides a Prometheus implementation of the floodgate.MetricsCollector interface.
//
// This implementation exposes backpressure metrics in Prometheus format, allowing you to:
// - Monitor request acceptance/rejection rates
// - Track latency distributions and percentiles
// - Observe circuit breaker state changes
// - Alert on cache utilization and dispatcher drops
//
// Example usage:
//
//	reg := prometheus.NewRegistry()
//	metrics := prometheus.NewMetrics(reg)
//	cfg.Metrics = metrics
//
//	// Expose metrics endpoint
//	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
package prometheus

import (
	"context"
	"time"

	"github.com/mushtruk/floodgate"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics implements floodgate.MetricsCollector using Prometheus.
type Metrics struct {
	requestsTotal    *prometheus.CounterVec
	requestsRejected *prometheus.CounterVec
	latencyHistogram *prometheus.HistogramVec
	circuitBreaker   *prometheus.GaugeVec
	cacheSize        prometheus.Gauge
	dispatcherDrops  prometheus.Counter
	dispatcherTotal  prometheus.Counter

	// Track previous values for delta calculation
	lastDropped uint64
	lastTotal   uint64
}

// NewMetrics creates a new Prometheus metrics collector.
// The provided registerer is used to register all metrics.
//
// If reg is nil, metrics will not be registered and will panic when recorded.
// Use prometheus.DefaultRegisterer for the global registry, or create a new
// registry with prometheus.NewRegistry() for isolation.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "floodgate",
				Name:      "requests_total",
				Help:      "Total number of requests processed by method, level, and result",
			},
			[]string{"method", "level", "result"},
		),
		requestsRejected: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "floodgate",
				Name:      "requests_rejected_total",
				Help:      "Total number of requests rejected due to backpressure by method and level",
			},
			[]string{"method", "level"},
		),
		latencyHistogram: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "floodgate",
				Name:      "request_duration_seconds",
				Help:      "Request latency distribution in seconds",
				// Buckets optimized for typical API latencies (1ms to 30s)
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0},
			},
			[]string{"method"},
		),
		circuitBreaker: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "floodgate",
				Name:      "circuit_breaker_state",
				Help:      "Circuit breaker state by method (0=closed, 1=open, 2=half-open)",
			},
			[]string{"method"},
		),
		cacheSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "floodgate",
				Name:      "cache_size",
				Help:      "Number of active method/route trackers in cache",
			},
		),
		dispatcherDrops: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "floodgate",
				Name:      "dispatcher_drops_total",
				Help:      "Total number of events dropped by async dispatcher due to buffer overflow",
			},
		),
		dispatcherTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "floodgate",
				Name:      "dispatcher_events_total",
				Help:      "Total number of events emitted to async dispatcher",
			},
		),
	}

	// Register all metrics
	reg.MustRegister(
		m.requestsTotal,
		m.requestsRejected,
		m.latencyHistogram,
		m.circuitBreaker,
		m.cacheSize,
		m.dispatcherDrops,
		m.dispatcherTotal,
	)

	return m
}

// RecordRequest implements floodgate.MetricsCollector.
func (m *Metrics) RecordRequest(ctx context.Context, labels floodgate.RequestLabels, latency time.Duration, rejected bool) {
	// Increment total requests
	m.requestsTotal.WithLabelValues(labels.Method, labels.Level.String(), labels.Result).Inc()

	// Track rejections separately for easier alerting
	if rejected {
		m.requestsRejected.WithLabelValues(labels.Method, labels.Level.String()).Inc()
	}

	// Record latency distribution
	m.latencyHistogram.WithLabelValues(labels.Method).Observe(latency.Seconds())
}

// RecordCircuitBreakerState implements floodgate.MetricsCollector.
func (m *Metrics) RecordCircuitBreakerState(method string, state floodgate.CircuitState) {
	var stateValue float64
	switch state {
	case floodgate.StateClosed:
		stateValue = 0
	case floodgate.StateOpen:
		stateValue = 1
	case floodgate.StateHalfOpen:
		stateValue = 2
	}
	m.circuitBreaker.WithLabelValues(method).Set(stateValue)
}

// RecordCacheSize implements floodgate.MetricsCollector.
func (m *Metrics) RecordCacheSize(size int) {
	m.cacheSize.Set(float64(size))
}

// RecordDispatcherStats implements floodgate.MetricsCollector.
func (m *Metrics) RecordDispatcherStats(dropped, total uint64) {
	// Calculate deltas since last call (Prometheus counters must always increase)
	dropsDelta := dropped - m.lastDropped
	totalDelta := total - m.lastTotal

	if dropsDelta > 0 {
		m.dispatcherDrops.Add(float64(dropsDelta))
	}
	if totalDelta > 0 {
		m.dispatcherTotal.Add(float64(totalDelta))
	}

	// Update last known values
	m.lastDropped = dropped
	m.lastTotal = total
}
