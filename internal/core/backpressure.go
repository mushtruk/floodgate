// Package core provides protocol-agnostic backpressure logic.
// This eliminates duplication between HTTP and gRPC implementations.
package core

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/mushtruk/floodgate"
)

// Config holds the configuration for BackpressureCore.
type Config struct {
	Algorithm                      floodgate.Algorithm
	Logger                         floodgate.Logger
	Metrics                        floodgate.MetricsCollector
	CircuitBreakerMaxFailures      int
	CircuitBreakerTimeout          time.Duration
	MetricsInterval                time.Duration
	CacheSize                      int
	DispatcherBufferSize           int
	CacheTTL                       time.Duration
	CircuitBreakerSuccessThreshold int
	TrackerWindowSize              int
	TrackerSampleSize              int
	TrackerAlpha                   float32
	EnableMetrics                  bool
}

// BackpressureCore contains the protocol-agnostic backpressure logic.
// This eliminates ~250 lines of duplication between HTTP and gRPC.
type BackpressureCore struct {
	registry       *expirable.LRU[string, floodgate.Tracker[time.Duration, floodgate.Stats]]
	dispatcher     *floodgate.Dispatcher[time.Duration]
	circuitBreaker *floodgate.CircuitBreaker
	algorithm      floodgate.Algorithm
	metrics        floodgate.MetricsCollector
	logger         floodgate.Logger
	config         Config
	started        atomic.Bool
}

// DecisionResult contains the backpressure decision and associated tracker.
type DecisionResult struct {
	Tracker  floodgate.Tracker[time.Duration, floodgate.Stats]
	Decision floodgate.Decision
	Stats    floodgate.Stats
}

// NewBackpressureCore creates a new protocol-agnostic backpressure core.
func NewBackpressureCore(ctx context.Context, cfg Config) *BackpressureCore {
	// Apply defaults
	if cfg.Logger == nil {
		cfg.Logger = floodgate.NewDefaultLogger()
	}
	if cfg.Metrics == nil {
		cfg.Metrics = &floodgate.NoOpMetrics{}
	}
	if cfg.Algorithm == nil {
		cfg.Algorithm = floodgate.NewThresholdAlgorithm(floodgate.DefaultThresholds())
	}

	core := &BackpressureCore{
		registry: expirable.NewLRU[string, floodgate.Tracker[time.Duration, floodgate.Stats]](
			cfg.CacheSize,
			nil,
			cfg.CacheTTL,
		),
		dispatcher: floodgate.NewDispatcher[time.Duration](ctx, cfg.DispatcherBufferSize),
		circuitBreaker: floodgate.NewCircuitBreaker(
			cfg.CircuitBreakerMaxFailures,
			cfg.CircuitBreakerTimeout,
			cfg.CircuitBreakerSuccessThreshold,
		),
		algorithm: cfg.Algorithm,
		metrics:   cfg.Metrics,
		logger:    cfg.Logger,
		config:    cfg,
	}

	// Start periodic metrics if enabled
	if cfg.EnableMetrics {
		core.startPeriodicMetrics(ctx)
	}

	core.started.Store(true)
	return core
}

// CheckBackpressure performs backpressure check for a given route.
// Returns the decision and tracker for latency recording.
func (c *BackpressureCore) CheckBackpressure(ctx context.Context, routeKey string) (*DecisionResult, error) {
	// Check circuit breaker
	if !c.circuitBreaker.Allow() {
		c.logger.WarnContext(ctx, "circuit breaker open", "route", routeKey)
		c.metrics.RecordCircuitBreakerState(routeKey, c.circuitBreaker.State())

		// Record rejected request
		c.metrics.RecordRequest(ctx, floodgate.RequestLabels{
			Method: routeKey,
			Level:  floodgate.Emergency,
			Result: "rejected",
		}, 0, true)

		return &DecisionResult{
			Decision: floodgate.Decision{
				Level:  floodgate.Emergency,
				Reject: true,
			},
		}, floodgate.ErrBackpressure
	}

	// Get or create tracker for this route
	tracker := c.getOrCreateTracker(routeKey)

	// Get current stats and make decision
	stats := tracker.Value()
	decision := c.algorithm.Decide(stats)

	// Check if algorithm decided to reject
	if decision.Reject {
		c.circuitBreaker.RecordFailure()
		c.logger.ErrorContext(ctx, "backpressure rejection",
			"route", routeKey,
			"level", decision.Level.String(),
			"ema", stats.EMA,
			"p95", stats.P95,
			"p99", stats.P99)
		c.metrics.RecordCircuitBreakerState(routeKey, c.circuitBreaker.State())
		c.metrics.RecordRequest(ctx, floodgate.RequestLabels{
			Method: routeKey,
			Level:  decision.Level,
			Result: "rejected",
		}, 0, true)

		return &DecisionResult{
			Decision: decision,
			Tracker:  tracker,
			Stats:    stats,
		}, floodgate.ErrBackpressure
	}

	// Log warnings for elevated backpressure (not rejecting)
	switch decision.Level {
	case floodgate.Warning, floodgate.Moderate:
		c.logger.WarnContext(ctx, "backpressure detected",
			"level", decision.Level.String(),
			"route", routeKey,
			"ema", stats.EMA,
			"p95", stats.P95,
			"p99", stats.P99)

	case floodgate.Normal:
		c.circuitBreaker.RecordSuccess()
		c.metrics.RecordCircuitBreakerState(routeKey, c.circuitBreaker.State())
	}

	return &DecisionResult{
		Decision: decision,
		Tracker:  tracker,
		Stats:    stats,
	}, nil
}

// RecordLatency records request latency for the tracker.
// This should be called after the request completes.
func (c *BackpressureCore) RecordLatency(ctx context.Context, result *DecisionResult, latency time.Duration, err error) {
	if result.Tracker != nil {
		c.dispatcher.Emit(result.Tracker, latency)
	}

	// Record request metrics
	reqResult := "success"
	if err != nil {
		reqResult = "error"
	}

	c.metrics.RecordRequest(ctx, floodgate.RequestLabels{
		Method: "", // Filled by protocol adapter
		Level:  result.Decision.Level,
		Result: reqResult,
	}, latency, result.Decision.Reject)
}

// getOrCreateTracker retrieves or creates a tracker for the given route.
func (c *BackpressureCore) getOrCreateTracker(routeKey string) floodgate.Tracker[time.Duration, floodgate.Stats] {
	tracker, ok := c.registry.Get(routeKey)
	if !ok {
		tracker = floodgate.NewTracker(
			floodgate.WithAlpha(c.config.TrackerAlpha),
			floodgate.WithWindowSize(c.config.TrackerWindowSize),
			floodgate.WithPercentiles(c.config.TrackerSampleSize),
		)
		c.registry.Add(routeKey, tracker)
	}
	return tracker
}

// startPeriodicMetrics emits periodic cache and dispatcher metrics.
func (c *BackpressureCore) startPeriodicMetrics(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(c.config.MetricsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cacheLen := c.registry.Len()
				dropRate := c.dispatcher.DropRate()

				// Record cache and dispatcher metrics
				c.metrics.RecordCacheSize(cacheLen)
				c.metrics.RecordDispatcherStats(c.dispatcher.DroppedCount(), c.dispatcher.TotalCount())

				if cacheLen > 0 || dropRate > 0 {
					c.logger.InfoContext(ctx, "backpressure metrics",
						"cache_used", cacheLen,
						"cache_size", c.config.CacheSize,
						"cache_pct", float64(cacheLen)/float64(c.config.CacheSize)*100,
						"drops", c.dispatcher.DroppedCount(),
						"total", c.dispatcher.TotalCount(),
						"drop_rate", dropRate,
						"circuit", c.circuitBreaker.State())
				}
			}
		}
	}()
}

// Stats returns current core statistics.
func (c *BackpressureCore) Stats() (cacheSize int, dropRate float64, circuitState floodgate.CircuitState) {
	return c.registry.Len(), c.dispatcher.DropRate(), c.circuitBreaker.State()
}

// Reset resets the circuit breaker to closed state.
func (c *BackpressureCore) Reset() {
	c.circuitBreaker.Reset()
}
