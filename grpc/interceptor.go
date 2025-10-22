// Package grpc provides gRPC interceptors with adaptive backpressure.
package grpc

import (
	"context"
	"fmt"
	"log"
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
			log.Printf("Circuit breaker open for %s", method)
			return nil, status.Errorf(codes.Unavailable, "service circuit breaker open")
		}

		stats := tracker.Value()
		level := stats.LevelWithThresholds(cfg.Thresholds)

		switch level {
		case floodgate.Emergency:
			circuitBreaker.RecordFailure()
			_ = grpc.SetTrailer(ctx, retryAfterEmergency)
			log.Printf("Backpressure emergency for %s - EMA: %v, P95: %v, P99: %v",
				method, stats.EMA, stats.P95, stats.P99)
			return nil, status.Errorf(codes.ResourceExhausted, "service overloaded - emergency backpressure")

		case floodgate.Critical:
			circuitBreaker.RecordFailure()
			_ = grpc.SetTrailer(ctx, retryAfterCritical)
			log.Printf("Backpressure critical for %s - EMA: %v, P95: %v, P99: %v",
				method, stats.EMA, stats.P95, stats.P99)
			return nil, status.Errorf(codes.ResourceExhausted, "service overloaded - critical backpressure")

		case floodgate.Warning, floodgate.Moderate:
			log.Printf("Backpressure %s for %s - EMA: %v, P95: %v, P99: %v",
				level, method, stats.EMA, stats.P95, stats.P99)

		case floodgate.Normal:
			circuitBreaker.RecordSuccess()
		}

		start := time.Now()
		resp, err := handler(ctx, req)
		latency := time.Since(start)

		dispatcher.Emit(tracker, latency)

		return resp, err
	}
}
