package floodgate

import (
	"context"
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

// WithCaching wraps an algorithm with a decision cache for expensive algorithms.
type CachedAlgorithm struct {
	algo      Algorithm
	cache     map[Stats]cacheEntry
	ttl       time.Duration
	hitCount  uint64
	missCount uint64
}

type cacheEntry struct {
	cachedAt time.Time
	decision Decision
}

// NewCachedAlgorithm creates an algorithm with result caching.
// Useful for expensive algorithm implementations.
func NewCachedAlgorithm(algo Algorithm, ttl time.Duration) *CachedAlgorithm {
	return &CachedAlgorithm{
		algo:  algo,
		cache: make(map[Stats]cacheEntry),
		ttl:   ttl,
	}
}

func (c *CachedAlgorithm) Decide(stats Stats) Decision {
	// Check cache
	if entry, ok := c.cache[stats]; ok {
		if time.Since(entry.cachedAt) < c.ttl {
			c.hitCount++
			return entry.decision
		}
		// Expired, remove from cache
		delete(c.cache, stats)
	}

	// Cache miss - compute decision
	c.missCount++
	decision := c.algo.Decide(stats)

	// Store in cache
	c.cache[stats] = cacheEntry{
		decision: decision,
		cachedAt: time.Now(),
	}

	return decision
}

// CacheStats returns cache hit and miss counts.
func (c *CachedAlgorithm) CacheStats() (hits, misses uint64) {
	return c.hitCount, c.missCount
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
