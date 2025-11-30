// Package http provides HTTP middleware with adaptive backpressure.
package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mushtruk/floodgate"
	"github.com/mushtruk/floodgate/internal/core"
)

// errHTTPStatus is a sentinel error used to indicate an HTTP error status code.
var errHTTPStatus = errors.New("http error status")

// matchPath checks if a request path matches a skip pattern.
// Patterns ending with "*" match as prefixes (e.g., "/api/*" matches "/api/users").
// All other patterns require an exact match (e.g., "/health" matches only "/health").
func matchPath(path, pattern string) bool {
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		// Prefix match: "/api/*" matches "/api/anything"
		prefix := pattern[:len(pattern)-1]
		return len(path) >= len(prefix) && path[:len(prefix)] == prefix
	}
	// Exact match
	return path == pattern
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// WriteHeader captures the status code before writing it.
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures 200 OK if WriteHeader wasn't called explicitly.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.statusCode = http.StatusOK
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter for middleware that need it.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Middleware creates an HTTP middleware with adaptive backpressure.
//
// This implementation uses BackpressureCore to eliminate code duplication with gRPC interceptor.
// Both HTTP and gRPC now share the same core backpressure logic, reducing maintenance burden.
//
// Usage:
//
//	middleware := http.Middleware(ctx, cfg)
//	handler := middleware(mux)
//	server := &http.Server{Handler: handler}
//
//nolint:gocognit,gocyclo // Middleware requires higher complexity for request lifecycle management
func Middleware(ctx context.Context, cfg Config) func(http.Handler) http.Handler {
	// Convert HTTP Config to core.Config
	coreConfig := core.Config{
		Algorithm:                      cfg.Algorithm,
		Logger:                         cfg.Logger,
		Metrics:                        cfg.Metrics,
		CacheSize:                      cfg.CacheSize,
		CacheTTL:                       cfg.CacheTTL,
		DispatcherBufferSize:           cfg.DispatcherBufferSize,
		CircuitBreakerMaxFailures:      cfg.CircuitBreakerMaxFailures,
		CircuitBreakerTimeout:          cfg.CircuitBreakerTimeout,
		CircuitBreakerSuccessThreshold: cfg.CircuitBreakerSuccessThreshold,
		TrackerAlpha:                   cfg.TrackerAlpha,
		TrackerWindowSize:              cfg.TrackerWindowSize,
		TrackerSampleSize:              cfg.TrackerSampleSize,
		MetricsInterval:                cfg.MetricsInterval,
		EnableMetrics:                  cfg.EnableMetrics,
	}

	// If algorithm not provided, use thresholds
	if coreConfig.Algorithm == nil {
		coreConfig.Algorithm = floodgate.NewThresholdAlgorithm(cfg.Thresholds)
	}

	// Create core backpressure logic
	bpCore := core.NewBackpressureCore(ctx, coreConfig)

	skipPaths := cfg.SkipPaths

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Check if path should be skipped
			// Supports exact match (e.g., "/health") or prefix match (e.g., "/api/*")
			for _, skipPath := range skipPaths {
				if matchPath(path, skipPath) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Route key: METHOD + path for more granular tracking
			routeKey := r.Method + " " + path

			// Check backpressure using core
			result, err := bpCore.CheckBackpressure(r.Context(), routeKey)
			if err != nil {
				// Determine retry-after based on decision level
				var retryAfter int
				switch result.Decision.Level {
				case floodgate.Emergency:
					retryAfter = cfg.RetryAfterEmergency
				case floodgate.Critical:
					retryAfter = cfg.RetryAfterCritical
				default:
					retryAfter = cfg.RetryAfterCircuit
				}

				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))

				// Return appropriate error
				if result.Decision.Level == floodgate.Emergency || result.Decision.Level == floodgate.Critical {
					http.Error(w, fmt.Sprintf("Service Unavailable - %s backpressure", result.Decision.Level.String()), http.StatusServiceUnavailable)
				} else {
					http.Error(w, "Service Unavailable - circuit breaker open", http.StatusServiceUnavailable)
				}
				return
			}

			// Wrap ResponseWriter to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Execute handler and record latency
			start := time.Now()
			next.ServeHTTP(wrapped, r)
			latency := time.Since(start)

			// Determine if response was an error (5xx status codes)
			var handlerErr error
			if wrapped.statusCode >= 500 {
				handlerErr = errHTTPStatus
			}

			// Record latency via core
			bpCore.RecordLatency(r.Context(), result, latency, handlerErr)
		})
	}
}
