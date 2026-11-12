package floodgate

import (
	"context"
	"sync/atomic"
	"time"
)

// Tracer provides distributed tracing integration for algorithms.
type Tracer interface {
	// StartSpan begins a new trace span
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

// Span represents an active trace span.
type Span interface {
	// SetAttribute adds an attribute to the span
	SetAttribute(key string, value interface{})
	// End completes the span
	End()
}

// WithTracing wraps an algorithm to add distributed tracing.
func WithTracing(algo Algorithm, tracer Tracer) Algorithm {
	return &tracedAlgorithm{
		algo:   algo,
		tracer: tracer,
	}
}

type tracedAlgorithm struct {
	algo   Algorithm
	tracer Tracer
}

func (t *tracedAlgorithm) Decide(stats Stats) Decision {
	_, span := t.tracer.StartSpan(context.Background(), "algorithm.decide")
	defer span.End()

	// Record input stats
	span.SetAttribute("stats.ema_ms", stats.EMA.Milliseconds())
	span.SetAttribute("stats.p50_ms", stats.P50.Milliseconds())
	span.SetAttribute("stats.p95_ms", stats.P95.Milliseconds())
	span.SetAttribute("stats.p99_ms", stats.P99.Milliseconds())
	span.SetAttribute("stats.slope_ms", stats.Slope.Milliseconds())

	// Execute algorithm
	decision := t.algo.Decide(stats)

	// Record decision
	span.SetAttribute("decision.level", decision.Level.String())
	span.SetAttribute("decision.reject", decision.Reject)

	return decision
}

// CachedAlgorithm forwards every decision to the wrapped Algorithm.
//
// Deprecated: Decide calls the wrapped Algorithm directly and the ttl passed to
// NewCachedAlgorithm is ignored. Stats holds continuous durations, so a
// decision cache almost never hits and costs more than the algorithms it
// wraps. Use the Algorithm itself.
type CachedAlgorithm struct {
	algo   Algorithm
	misses atomic.Uint64
}

// NewCachedAlgorithm wraps algo in a CachedAlgorithm. The ttl is ignored.
//
// Deprecated: Decide calls algo directly. Use algo itself.
func NewCachedAlgorithm(algo Algorithm, _ time.Duration) *CachedAlgorithm {
	return &CachedAlgorithm{algo: algo}
}

func (c *CachedAlgorithm) Decide(stats Stats) Decision {
	c.misses.Add(1)
	return c.algo.Decide(stats)
}

// CacheStats returns hits, which is always 0, and misses, which counts Decide
// calls. It is safe for concurrent use.
func (c *CachedAlgorithm) CacheStats() (hits, misses uint64) {
	return 0, c.misses.Load()
}

// WithFallback wraps an algorithm with a fallback for panic recovery.
func WithFallback(primary, fallback Algorithm, logger Logger) Algorithm {
	return &fallbackAlgorithm{
		primary:  primary,
		fallback: fallback,
		logger:   logger,
	}
}

type fallbackAlgorithm struct {
	primary  Algorithm
	fallback Algorithm
	logger   Logger
}

func (f *fallbackAlgorithm) Decide(stats Stats) Decision {
	defer func() {
		if r := recover(); r != nil {
			if f.logger != nil {
				f.logger.ErrorContext(context.Background(), "algorithm panic, using fallback",
					"panic", r,
					"algorithm", "primary")
			}
		}
	}()

	decision := f.primary.Decide(stats)
	return decision
}

// InstrumentedAlgorithm combines tracing and metrics for full observability.
type InstrumentedAlgorithm struct {
	algo    Algorithm
	tracer  Tracer
	metrics MetricsCollector
	logger  Logger
}

// NewInstrumentedAlgorithm creates an algorithm with full observability.
func NewInstrumentedAlgorithm(
	algo Algorithm,
	tracer Tracer,
	metrics MetricsCollector,
	logger Logger,
) *InstrumentedAlgorithm {
	return &InstrumentedAlgorithm{
		algo:    algo,
		tracer:  tracer,
		metrics: metrics,
		logger:  logger,
	}
}

func (i *InstrumentedAlgorithm) Decide(stats Stats) Decision {
	var span Span
	var ctx context.Context

	if i.tracer != nil {
		ctx, span = i.tracer.StartSpan(context.Background(), "algorithm.decide")
		defer span.End()

		span.SetAttribute("stats.ema_ms", stats.EMA.Milliseconds())
		span.SetAttribute("stats.p95_ms", stats.P95.Milliseconds())
		span.SetAttribute("stats.p99_ms", stats.P99.Milliseconds())
	}

	start := time.Now()
	decision := i.algo.Decide(stats)
	duration := time.Since(start)

	if span != nil {
		span.SetAttribute("decision.level", decision.Level.String())
		span.SetAttribute("decision.reject", decision.Reject)
		span.SetAttribute("duration_us", duration.Microseconds())
	}

	if i.logger != nil && decision.Reject {
		logCtx := context.Background()
		i.logger.WarnContext(logCtx, "algorithm rejected request",
			"level", decision.Level.String(),
			"ema", stats.EMA,
			"p95", stats.P95)
	}

	if i.metrics != nil && ctx != nil {
		i.metrics.RecordRequest(ctx, RequestLabels{
			Method: "algorithm.decide",
			Result: map[bool]string{true: "rejected", false: "accepted"}[decision.Reject],
			Level:  decision.Level,
		}, duration, decision.Reject)
	}

	return decision
}
