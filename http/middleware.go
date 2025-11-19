// Package http provides HTTP middleware with adaptive backpressure.
package http

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/mushtruk/floodgate"
)

// Config holds configuration for the backpressure middleware.
type Config struct {
	CacheSize            int
	CacheTTL             time.Duration
	DispatcherBufferSize int
	Thresholds           floodgate.Thresholds
	SkipPaths            []string
	EnableMetrics        bool
	MetricsInterval      time.Duration

	// Circuit breaker configuration
	CircuitBreakerMaxFailures      int
	CircuitBreakerTimeout          time.Duration
	CircuitBreakerSuccessThreshold int

	// Tracker configuration per route
	TrackerAlpha      float32
	TrackerWindowSize int
	TrackerSampleSize int

	// Retry-after headers (seconds)
	RetryAfterEmergency int
	RetryAfterCritical  int
	RetryAfterCircuit   int

	// Logger for backpressure events. If nil, uses DefaultLogger.
	Logger floodgate.Logger

	// Metrics collector for observability. If nil, uses NoOpMetrics (disabled).
	Metrics floodgate.MetricsCollector

	// Algorithm determines backpressure decisions (optional).
	// If nil, uses ThresholdAlgorithm with cfg.Thresholds.
	//
	// Examples:
	//   cfg.Algorithm = nil  // Use default thresholds (backward compatible)
	//   cfg.Algorithm = floodgate.NewThresholdAlgorithm(customThresholds)
	//   cfg.Algorithm = codel.NewAlgorithm()
	Algorithm floodgate.Algorithm
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() Config {
	return Config{
		CacheSize:            512,
		CacheTTL:             2 * time.Minute,
		DispatcherBufferSize: 1024,
		Thresholds:           floodgate.DefaultThresholds(),
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/readiness",
		},
		EnableMetrics:   true,
		MetricsInterval: 1 * time.Minute,

		CircuitBreakerMaxFailures:      3,
		CircuitBreakerTimeout:          30 * time.Second,
		CircuitBreakerSuccessThreshold: 5,

		TrackerAlpha:      0.1,
		TrackerWindowSize: 50,
		TrackerSampleSize: 200,

		RetryAfterEmergency: 10,
		RetryAfterCritical:  5,
		RetryAfterCircuit:   30,

		Logger:    floodgate.NewDefaultLogger(),
		Metrics:   &floodgate.NoOpMetrics{}, // Disabled by default
		Algorithm: nil,                      // nil = use threshold-based (backward compatible)
	}
}

// Middleware creates an HTTP middleware with adaptive backpressure.
//
//nolint:gocognit // Middleware requires higher complexity for request lifecycle management
func Middleware(ctx context.Context, cfg Config) func(http.Handler) http.Handler {
	registry := expirable.NewLRU[string, floodgate.Tracker[time.Duration, floodgate.Stats]](
		cfg.CacheSize,
		nil,
		cfg.CacheTTL,
	)

	dispatcher := floodgate.NewDispatcher[time.Duration](ctx, cfg.DispatcherBufferSize)
	circuitBreaker := floodgate.NewCircuitBreaker(
		cfg.CircuitBreakerMaxFailures,
		cfg.CircuitBreakerTimeout,
		cfg.CircuitBreakerSuccessThreshold,
	)
	skipPaths := cfg.SkipPaths

	// Use provided logger or default
	logger := cfg.Logger
	if logger == nil {
		logger = floodgate.NewDefaultLogger()
	}

	// Use provided metrics or no-op
	metrics := cfg.Metrics
	if metrics == nil {
		metrics = &floodgate.NoOpMetrics{}
	}

	// Use provided algorithm or default to threshold-based
	algo := cfg.Algorithm
	if algo == nil {
		algo = floodgate.NewThresholdAlgorithm(cfg.Thresholds)
	}

	// Periodic metrics
	if cfg.EnableMetrics {
		go func() {
			ticker := time.NewTicker(cfg.MetricsInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					cacheLen := registry.Len()
					dropRate := dispatcher.DropRate()

					// Record cache and dispatcher metrics
					metrics.RecordCacheSize(cacheLen)
					metrics.RecordDispatcherStats(dispatcher.DroppedCount(), dispatcher.TotalCount())

					if cacheLen > 0 || dropRate > 0 {
						logger.InfoContext(ctx, "backpressure metrics",
							"cache_used", cacheLen,
							"cache_size", cfg.CacheSize,
							"cache_pct", float64(cacheLen)/float64(cfg.CacheSize)*100,
							"drops", dispatcher.DroppedCount(),
							"total", dispatcher.TotalCount(),
							"drop_rate", dropRate,
							"circuit", circuitBreaker.State())
					}
				}
			}
		}()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Fast prefix check (optimized for small n=2-3 prefixes)
			for _, skipPrefix := range skipPaths {
				if strings.HasPrefix(path, skipPrefix) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Route key: METHOD + path for more granular tracking
			routeKey := r.Method + " " + path

			tracker, ok := registry.Get(routeKey)
			if !ok {
				tracker = floodgate.NewTracker(
					floodgate.WithAlpha(cfg.TrackerAlpha),
					floodgate.WithWindowSize(cfg.TrackerWindowSize),
					floodgate.WithPercentiles(cfg.TrackerSampleSize),
				)
				registry.Add(routeKey, tracker)
			}

			if !circuitBreaker.Allow() {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", cfg.RetryAfterCircuit))
				logger.WarnContext(r.Context(), "circuit breaker open", "route", routeKey)
				metrics.RecordCircuitBreakerState(routeKey, circuitBreaker.State())

				// Record rejected request
				metrics.RecordRequest(r.Context(), floodgate.RequestLabels{
					Method: routeKey,
					Level:  floodgate.Emergency,
					Result: "rejected",
				}, 0, true)

				http.Error(w, "Service Unavailable - circuit breaker open", http.StatusServiceUnavailable)
				return
			}

			stats := tracker.Value()
			decision := algo.Decide(stats)

			var rejected bool

			// Check if algorithm decided to reject
			if decision.Reject {
				var retryAfter int
				switch decision.Level {
				case floodgate.Emergency:
					retryAfter = cfg.RetryAfterEmergency
				case floodgate.Critical:
					retryAfter = cfg.RetryAfterCritical
				default:
					retryAfter = cfg.RetryAfterEmergency
				}

				circuitBreaker.RecordFailure()
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				logger.ErrorContext(r.Context(), "backpressure rejection",
					"route", routeKey,
					"level", decision.Level,
					"ema", stats.EMA,
					"p95", stats.P95,
					"p99", stats.P99)
				metrics.RecordCircuitBreakerState(routeKey, circuitBreaker.State())
				metrics.RecordRequest(r.Context(), floodgate.RequestLabels{
					Method: routeKey,
					Level:  decision.Level,
					Result: "rejected",
				}, 0, true)
				http.Error(w, fmt.Sprintf("Service Unavailable - %s backpressure", decision.Level), http.StatusServiceUnavailable)
				return
			}

			// Log warnings for elevated backpressure (not rejecting)
			switch decision.Level {
			case floodgate.Warning, floodgate.Moderate:
				logger.WarnContext(r.Context(), "backpressure detected",
					"level", decision.Level,
					"route", routeKey,
					"ema", stats.EMA,
					"p95", stats.P95,
					"p99", stats.P99)

			case floodgate.Normal:
				circuitBreaker.RecordSuccess()
				metrics.RecordCircuitBreakerState(routeKey, circuitBreaker.State())
			}

			start := time.Now()
			next.ServeHTTP(w, r)
			latency := time.Since(start)

			dispatcher.Emit(tracker, latency)

			// Record successful request completion
			metrics.RecordRequest(r.Context(), floodgate.RequestLabels{
				Method: routeKey,
				Level:  decision.Level,
				Result: "success",
			}, latency, rejected)
		})
	}
}
