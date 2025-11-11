// Package grpc provides gRPC interceptors with adaptive backpressure.
package grpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/mushtruk/floodgate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	md "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Config holds configuration for the backpressure interceptor.
type Config struct {
	CacheSize            int
	CacheTTL             time.Duration
	DispatcherBufferSize int
	Thresholds           floodgate.Thresholds
	SkipMethods          []string
	EnableMetrics        bool
	MetricsInterval      time.Duration

	// Circuit breaker configuration
	CircuitBreakerMaxFailures      int
	CircuitBreakerTimeout          time.Duration
	CircuitBreakerSuccessThreshold int

	// Tracker configuration per method
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
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() Config {
	return Config{
		CacheSize:            512,
		CacheTTL:             2 * time.Minute,
		DispatcherBufferSize: 1024,
		Thresholds:           floodgate.DefaultThresholds(),
		SkipMethods: []string{
			"/grpc.health.",
			"/grpc.reflection.",
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

		Logger:  floodgate.NewDefaultLogger(),
		Metrics: &floodgate.NoOpMetrics{}, // Disabled by default
	}
}

// UnaryServerInterceptor creates a gRPC unary server interceptor with adaptive backpressure.
func UnaryServerInterceptor(ctx context.Context, cfg Config) grpc.UnaryServerInterceptor {
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
	skipMethods := cfg.SkipMethods

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

	// Pre-allocate metadata to avoid allocation on hot path
	retryAfterCircuit := md.Pairs("retry-after", fmt.Sprintf("%d", cfg.RetryAfterCircuit))
	retryAfterEmergency := md.Pairs("retry-after", fmt.Sprintf("%d", cfg.RetryAfterEmergency))
	retryAfterCritical := md.Pairs("retry-after", fmt.Sprintf("%d", cfg.RetryAfterCritical))

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

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		method := info.FullMethod

		// Fast prefix check (optimized for small n=2-3 prefixes)
		for _, skipPrefix := range skipMethods {
			if strings.HasPrefix(method, skipPrefix) {
				return handler(ctx, req)
			}
		}

		tracker, ok := registry.Get(method)
		if !ok {
			tracker = floodgate.NewTracker(
				floodgate.WithAlpha(cfg.TrackerAlpha),
				floodgate.WithWindowSize(cfg.TrackerWindowSize),
				floodgate.WithPercentiles(cfg.TrackerSampleSize),
			)
			registry.Add(method, tracker)
		}

		if !circuitBreaker.Allow() {
			_ = grpc.SetTrailer(ctx, retryAfterCircuit)
			logger.WarnContext(ctx, "circuit breaker open", "method", method)
			metrics.RecordCircuitBreakerState(method, circuitBreaker.State())

			// Record rejected request
			metrics.RecordRequest(ctx, floodgate.RequestLabels{
				Method: method,
				Level:  floodgate.Emergency,
				Result: "rejected",
			}, 0, true)

			return nil, status.Errorf(codes.Unavailable, "service circuit breaker open")
		}

		stats := tracker.Value()
		level := stats.LevelWithThresholds(cfg.Thresholds)

		var rejected bool

		switch level {
		case floodgate.Emergency:
			circuitBreaker.RecordFailure()
			_ = grpc.SetTrailer(ctx, retryAfterEmergency)
			logger.ErrorContext(ctx, "backpressure emergency",
				"method", method,
				"ema", stats.EMA,
				"p95", stats.P95,
				"p99", stats.P99)
			rejected = true
			metrics.RecordCircuitBreakerState(method, circuitBreaker.State())
			metrics.RecordRequest(ctx, floodgate.RequestLabels{
				Method: method,
				Level:  level,
				Result: "rejected",
			}, 0, true)
			return nil, status.Errorf(codes.ResourceExhausted, "service overloaded - emergency backpressure")

		case floodgate.Critical:
			circuitBreaker.RecordFailure()
			_ = grpc.SetTrailer(ctx, retryAfterCritical)
			logger.ErrorContext(ctx, "backpressure critical",
				"method", method,
				"ema", stats.EMA,
				"p95", stats.P95,
				"p99", stats.P99)
			rejected = true
			metrics.RecordCircuitBreakerState(method, circuitBreaker.State())
			metrics.RecordRequest(ctx, floodgate.RequestLabels{
				Method: method,
				Level:  level,
				Result: "rejected",
			}, 0, true)
			return nil, status.Errorf(codes.ResourceExhausted, "service overloaded - critical backpressure")

		case floodgate.Warning, floodgate.Moderate:
			logger.WarnContext(ctx, "backpressure detected",
				"level", level,
				"method", method,
				"ema", stats.EMA,
				"p95", stats.P95,
				"p99", stats.P99)

		case floodgate.Normal:
			circuitBreaker.RecordSuccess()
			metrics.RecordCircuitBreakerState(method, circuitBreaker.State())
		}

		start := time.Now()
		resp, err := handler(ctx, req)
		latency := time.Since(start)

		dispatcher.Emit(tracker, latency)

		// Record successful request completion
		result := "success"
		if err != nil {
			result = "error"
		}
		metrics.RecordRequest(ctx, floodgate.RequestLabels{
			Method: method,
			Level:  level,
			Result: result,
		}, latency, rejected)

		return resp, err
	}
}
