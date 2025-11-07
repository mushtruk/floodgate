// Package http provides HTTP middleware with adaptive backpressure.
package http

import (
	"context"
	"fmt"
	"log"
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
	}
}

// Middleware creates an HTTP middleware with adaptive backpressure.
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

					if cacheLen > 0 || dropRate > 0 {
						log.Printf("Backpressure metrics - cache: %d/%d (%.1f%%), dispatcher drops: %d/%d (%.2f%%), circuit: %s",
							cacheLen, cfg.CacheSize, float64(cacheLen)/float64(cfg.CacheSize)*100,
							dispatcher.DroppedCount(), dispatcher.TotalCount(), dropRate,
							circuitBreaker.State())
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
				log.Printf("Circuit breaker open for %s", routeKey)
				http.Error(w, "Service Unavailable - circuit breaker open", http.StatusServiceUnavailable)
				return
			}

			stats := tracker.Value()
			level := stats.LevelWithThresholds(cfg.Thresholds)

			switch level {
			case floodgate.Emergency:
				circuitBreaker.RecordFailure()
				w.Header().Set("Retry-After", fmt.Sprintf("%d", cfg.RetryAfterEmergency))
				log.Printf("Backpressure emergency for %s - EMA: %v, P95: %v, P99: %v",
					routeKey, stats.EMA, stats.P95, stats.P99)
				http.Error(w, "Service Unavailable - emergency backpressure", http.StatusServiceUnavailable)
				return

			case floodgate.Critical:
				circuitBreaker.RecordFailure()
				w.Header().Set("Retry-After", fmt.Sprintf("%d", cfg.RetryAfterCritical))
				log.Printf("Backpressure critical for %s - EMA: %v, P95: %v, P99: %v",
					routeKey, stats.EMA, stats.P95, stats.P99)
				http.Error(w, "Service Unavailable - critical backpressure", http.StatusServiceUnavailable)
				return

			case floodgate.Warning, floodgate.Moderate:
				log.Printf("Backpressure %s for %s - EMA: %v, P95: %v, P99: %v",
					level, routeKey, stats.EMA, stats.P95, stats.P99)

			case floodgate.Normal:
				circuitBreaker.RecordSuccess()
			}

			start := time.Now()
			next.ServeHTTP(w, r)
			latency := time.Since(start)

			dispatcher.Emit(tracker, latency)
		})
	}
}
