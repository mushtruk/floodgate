// Package grpc provides gRPC interceptors with adaptive backpressure.
package grpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mushtruk/floodgate"
	"github.com/mushtruk/floodgate/internal/core"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	md "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// UnaryServerInterceptor creates a gRPC unary server interceptor with adaptive backpressure.
//
// This implementation uses BackpressureCore to eliminate code duplication with HTTP middleware.
// Both HTTP and gRPC now share the same core backpressure logic, reducing maintenance burden.
//
// Usage:
//
//	interceptor := grpc.UnaryServerInterceptor(ctx, cfg)
//	server := grpc.NewServer(grpc.UnaryInterceptor(interceptor))
//
//nolint:gocognit,gocyclo // Interceptor requires higher complexity for request lifecycle management
func UnaryServerInterceptor(ctx context.Context, cfg Config) grpc.UnaryServerInterceptor {
	// Convert gRPC Config to core.Config
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

	// Pre-allocate metadata to avoid allocation on hot path
	retryAfterCircuit := md.Pairs("retry-after", fmt.Sprintf("%d", cfg.RetryAfterCircuit))
	retryAfterEmergency := md.Pairs("retry-after", fmt.Sprintf("%d", cfg.RetryAfterEmergency))
	retryAfterCritical := md.Pairs("retry-after", fmt.Sprintf("%d", cfg.RetryAfterCritical))

	skipMethods := cfg.SkipMethods

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		method := info.FullMethod

		// Fast prefix check (optimized for small n=2-3 prefixes)
		for _, skipPrefix := range skipMethods {
			if strings.HasPrefix(method, skipPrefix) {
				return handler(ctx, req)
			}
		}

		// Check backpressure using core
		result, err := bpCore.CheckBackpressure(ctx, method)
		if err != nil {
			// Determine retry-after based on decision level
			var retryAfter md.MD
			switch result.Decision.Level {
			case floodgate.Emergency:
				retryAfter = retryAfterEmergency
			case floodgate.Critical:
				retryAfter = retryAfterCritical
			default:
				retryAfter = retryAfterCircuit
			}

			_ = grpc.SetTrailer(ctx, retryAfter) //nolint:errcheck // Trailer is best-effort

			// Return appropriate error
			if result.Decision.Level == floodgate.Emergency {
				return nil, status.Errorf(codes.ResourceExhausted, "service overloaded - %s backpressure", result.Decision.Level.String())
			}
			return nil, status.Errorf(codes.Unavailable, "service circuit breaker open")
		}

		// Execute handler and record latency
		start := time.Now()
		resp, handlerErr := handler(ctx, req)
		latency := time.Since(start)

		// Record latency via core
		bpCore.RecordLatency(ctx, result, latency, handlerErr)

		return resp, handlerErr
	}
}
