// Package tracing provides OpenTelemetry distributed tracing integration for floodgate.
//
// This package adds trace spans to backpressure operations, allowing you to:
// - Track backpressure events in distributed traces
// - Correlate rejections with upstream/downstream services
// - Visualize latency bottlenecks across service boundaries
// - Debug cascading failures with full request context
//
// Example usage:
//
//	import (
//	    "go.opentelemetry.io/otel"
//	    "github.com/mushtruk/floodgate/tracing"
//	)
//
//	// Enable tracing in your middleware config
//	cfg.Tracer = tracing.NewTracer(otel.Tracer("myservice"))
package tracing

import (
	"context"

	"github.com/mushtruk/floodgate"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Tracer wraps an OpenTelemetry tracer for backpressure instrumentation.
type Tracer struct {
	tracer trace.Tracer
}

// NewTracer creates a new tracer wrapper.
//
// Example:
//
//	tracer := otel.Tracer("myservice")
//	cfg.Tracer = tracing.NewTracer(tracer)
func NewTracer(tracer trace.Tracer) *Tracer {
	return &Tracer{tracer: tracer}
}

// StartBackpressureSpan creates a span for backpressure evaluation.
// This span tracks the entire backpressure check and decision process.
func (t *Tracer) StartBackpressureSpan(ctx context.Context, method string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "floodgate.backpressure",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("method", method),
			attribute.String("component", "floodgate"),
		),
	)
}

// RecordBackpressureDecision records the backpressure level and decision in the span.
func (t *Tracer) RecordBackpressureDecision(span trace.Span, stats floodgate.Stats, level floodgate.Level, rejected bool) {
	span.SetAttributes(
		attribute.String("backpressure.level", level.String()),
		attribute.Float64("backpressure.ema", stats.EMA.Seconds()),
		attribute.Float64("backpressure.p50", stats.P50.Seconds()),
		attribute.Float64("backpressure.p95", stats.P95.Seconds()),
		attribute.Float64("backpressure.p99", stats.P99.Seconds()),
		attribute.Float64("backpressure.slope", stats.Slope.Seconds()),
		attribute.Bool("backpressure.rejected", rejected),
	)

	if rejected {
		span.SetStatus(codes.Error, "request rejected due to backpressure")
		span.RecordError(ErrBackpressureRejected{Level: level})
	}
}

// RecordCircuitBreakerState records circuit breaker state changes in the span.
func (t *Tracer) RecordCircuitBreakerState(span trace.Span, state floodgate.CircuitState, rejected bool) {
	var stateName string
	switch state {
	case floodgate.StateClosed:
		stateName = "closed"
	case floodgate.StateOpen:
		stateName = "open"
	case floodgate.StateHalfOpen:
		stateName = "half_open"
	}

	span.SetAttributes(
		attribute.String("circuit_breaker.state", stateName),
		attribute.Bool("circuit_breaker.rejected", rejected),
	)

	if rejected {
		span.SetStatus(codes.Error, "request rejected by circuit breaker")
		span.RecordError(ErrCircuitBreakerOpen{})
	}
}

// ErrBackpressureRejected is recorded when a request is rejected due to backpressure.
type ErrBackpressureRejected struct {
	Level floodgate.Level
}

func (e ErrBackpressureRejected) Error() string {
	return "backpressure rejected at level: " + e.Level.String()
}

// ErrCircuitBreakerOpen is recorded when a request is rejected by the circuit breaker.
type ErrCircuitBreakerOpen struct{}

func (e ErrCircuitBreakerOpen) Error() string {
	return "circuit breaker open"
}

// NoOpTracer is a tracer that does nothing (zero overhead).
type NoOpTracer struct{}

// StartBackpressureSpan implements the Tracer interface with no-op behavior.
func (NoOpTracer) StartBackpressureSpan(ctx context.Context, method string) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}

// RecordBackpressureDecision implements the Tracer interface with no-op behavior.
func (NoOpTracer) RecordBackpressureDecision(span trace.Span, stats floodgate.Stats, level floodgate.Level, rejected bool) {
}

// RecordCircuitBreakerState implements the Tracer interface with no-op behavior.
func (NoOpTracer) RecordCircuitBreakerState(span trace.Span, state floodgate.CircuitState, rejected bool) {
}
