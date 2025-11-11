package floodgate

import (
	"context"
	"time"
)

// MetricsCollector is the interface for collecting backpressure metrics.
// Implementations can integrate with Prometheus, OpenTelemetry, StatsD, Datadog, etc.
//
// The interface follows the pluggable design pattern used by Logger, allowing complete
// decoupling from specific metrics backends. Applications choose their metrics system
// at runtime by injecting a concrete implementation.
//
// Example with Prometheus:
//
//	metrics := prometheus.NewMetrics(registry)
//	cfg.Metrics = metrics
//
// Example with OpenTelemetry:
//
//	metrics := opentelemetry.NewMetrics(meter)
//	cfg.Metrics = metrics
//
// To disable metrics collection entirely:
//
//	cfg.Metrics = &floodgate.NoOpMetrics{}
//
// The metrics collector records four key categories of backpressure observability:
// - Request outcomes (accepted/rejected, latency, backpressure level)
// - Circuit breaker state transitions
// - Cache utilization (active trackers)
// - Dispatcher performance (async processing drops)
type MetricsCollector interface {
	// RecordRequest records a completed request with its outcome.
	// This is called for every request processed by the middleware.
	//
	// Parameters:
	//   ctx: request context for trace correlation
	//   labels: structured labels (method/route, level, result)
	//   latency: request duration
	//   rejected: true if request was rejected due to backpressure
	//
	// Implementations should:
	// - Increment request counters by method and result
	// - Record latency histograms for percentile calculation
	// - Track rejection rates by backpressure level
	RecordRequest(ctx context.Context, labels RequestLabels, latency time.Duration, rejected bool)

	// RecordCircuitBreakerState records circuit breaker state changes.
	// Called when the circuit breaker transitions between states.
	//
	// Parameters:
	//   method: gRPC method or HTTP route
	//   state: current circuit breaker state
	//
	// Implementations should:
	// - Update gauge metrics to show current state
	// - Use numeric values: Closed=0, Open=1, HalfOpen=2
	RecordCircuitBreakerState(method string, state CircuitState)

	// RecordCacheSize records the current number of active trackers.
	// Called periodically (if metrics are enabled) to monitor memory usage.
	//
	// Parameters:
	//   size: number of active method/route trackers in cache
	//
	// Implementations should:
	// - Update gauge metric showing cache utilization
	// - Alert when approaching cache limit
	RecordCacheSize(size int)

	// RecordDispatcherStats records async dispatcher performance metrics.
	// Called periodically to monitor event processing.
	//
	// Parameters:
	//   dropped: total events dropped due to buffer overflow
	//   total: total events emitted since start
	//
	// Implementations should:
	// - Track drop rate as percentage: dropped/total
	// - Monitor buffer pressure
	// - Alert on sustained drop rates
	RecordDispatcherStats(dropped, total uint64)
}

// RequestLabels contains structured labels for request metrics.
// Using a struct provides type safety and makes it easy to add new labels.
type RequestLabels struct {
	// Method is the gRPC method name (e.g., "/api.UserService/GetUser")
	// or HTTP route (e.g., "GET /api/users").
	Method string

	// Level is the backpressure level at the time of the request.
	// Values: Normal, Warning, Moderate, Critical, Emergency
	Level Level

	// Result indicates the request outcome.
	// Values: "success" (request accepted), "rejected" (backpressure rejection)
	Result string
}

// NoOpMetrics is a metrics collector that discards all metrics.
// Use this to completely disable metrics collection in production if desired.
//
// This implementation has zero overhead - all methods are empty and will be
// inlined by the compiler, resulting in no runtime cost.
type NoOpMetrics struct{}

// RecordRequest implements MetricsCollector.
func (NoOpMetrics) RecordRequest(ctx context.Context, labels RequestLabels, latency time.Duration, rejected bool) {
}

// RecordCircuitBreakerState implements MetricsCollector.
func (NoOpMetrics) RecordCircuitBreakerState(method string, state CircuitState) {}

// RecordCacheSize implements MetricsCollector.
func (NoOpMetrics) RecordCacheSize(size int) {}

// RecordDispatcherStats implements MetricsCollector.
func (NoOpMetrics) RecordDispatcherStats(dropped, total uint64) {}
