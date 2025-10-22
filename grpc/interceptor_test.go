package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/mushtruk/floodgate"
	"google.golang.org/grpc"
)

// Mock handler for testing
func mockHandler(ctx context.Context, req any) (any, error) {
	// Simulate some work
	time.Sleep(1 * time.Millisecond)
	return "response", nil
}

func mockSlowHandler(ctx context.Context, req any) (any, error) {
	// Simulate slow work
	time.Sleep(100 * time.Millisecond)
	return "response", nil
}

func mockInfo(method string) *grpc.UnaryServerInfo {
	return &grpc.UnaryServerInfo{
		FullMethod: method,
	}
}

// BenchmarkInterceptor_NormalPath benchmarks the happy path with normal latency
func BenchmarkInterceptor_NormalPath(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false // Disable metrics for cleaner benchmark

	interceptor := UnaryServerInterceptor(ctx, cfg)
	info := mockInfo("/test.Service/Method")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = interceptor(ctx, nil, info, mockHandler)
	}
}

// BenchmarkInterceptor_SkippedMethod benchmarks skipped methods (health checks)
func BenchmarkInterceptor_SkippedMethod(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	interceptor := UnaryServerInterceptor(ctx, cfg)
	info := mockInfo("/grpc.health.v1/Check")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = interceptor(ctx, nil, info, mockHandler)
	}
}

// BenchmarkInterceptor_MultipleMethodsConcurrent benchmarks multiple methods concurrently
func BenchmarkInterceptor_MultipleMethodsConcurrent(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	interceptor := UnaryServerInterceptor(ctx, cfg)

	methods := []string{
		"/test.Service/Method1",
		"/test.Service/Method2",
		"/test.Service/Method3",
		"/test.Service/Method4",
		"/test.Service/Method5",
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			method := methods[i%len(methods)]
			info := mockInfo(method)
			_, _ = interceptor(ctx, nil, info, mockHandler)
			i++
		}
	})
}

// BenchmarkInterceptor_EmergencyRejection benchmarks rejection path during emergency
func BenchmarkInterceptor_EmergencyRejection(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false
	// Set aggressive thresholds to trigger emergency
	cfg.Thresholds = floodgate.Thresholds{
		P99Emergency: 50 * time.Millisecond,
		P95Critical:  20 * time.Millisecond,
		EMACritical:  10 * time.Millisecond,
		P95Moderate:  10 * time.Millisecond,
		EMAWarning:   5 * time.Millisecond,
		SlopeWarning: 1 * time.Millisecond,
	}

	interceptor := UnaryServerInterceptor(ctx, cfg)
	info := mockInfo("/test.Service/SlowMethod")

	// Prime the tracker with slow requests to trigger emergency
	for i := 0; i < 100; i++ {
		_, _ = interceptor(ctx, nil, info, mockSlowHandler)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = interceptor(ctx, nil, info, mockHandler)
	}
}

// BenchmarkInterceptor_NewMethodCreation benchmarks tracker creation for new methods
func BenchmarkInterceptor_NewMethodCreation(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create new interceptor for each iteration to measure cold start
		interceptor := UnaryServerInterceptor(ctx, cfg)
		info := mockInfo("/test.Service/NewMethod")
		_, _ = interceptor(ctx, nil, info, mockHandler)
	}
}

// BenchmarkInterceptor_StatsEvaluation benchmarks just the stats evaluation and level check
func BenchmarkInterceptor_StatsEvaluation(b *testing.B) {
	tracker := floodgate.NewTracker(
		floodgate.WithAlpha(0.1),
		floodgate.WithWindowSize(50),
		floodgate.WithPercentiles(1000),
	)

	// Prime with data
	for i := 0; i < 1000; i++ {
		tracker.Process(100 * time.Millisecond)
	}

	thresholds := floodgate.DefaultThresholds()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stats := tracker.Value()
		_ = stats.LevelWithThresholds(thresholds)
	}
}

// BenchmarkConfig_Default benchmarks default config creation
func BenchmarkConfig_Default(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = DefaultConfig()
	}
}

// Test to ensure interceptor works correctly
func TestInterceptor_BasicFlow(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	interceptor := UnaryServerInterceptor(ctx, cfg)
	info := mockInfo("/test.Service/Method")

	resp, err := interceptor(ctx, nil, info, mockHandler)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp != "response" {
		t.Fatalf("Expected 'response', got %v", resp)
	}
}

// Test to ensure skipped methods bypass tracking
func TestInterceptor_SkipMethods(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	interceptor := UnaryServerInterceptor(ctx, cfg)

	// Test health check skip
	info := mockInfo("/grpc.health.v1/Check")
	resp, err := interceptor(ctx, nil, info, mockHandler)
	if err != nil {
		t.Fatalf("Expected no error for health check, got %v", err)
	}
	if resp != "response" {
		t.Fatalf("Expected 'response', got %v", resp)
	}

	// Test reflection skip
	info = mockInfo("/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo")
	resp, err = interceptor(ctx, nil, info, mockHandler)
	if err != nil {
		t.Fatalf("Expected no error for reflection, got %v", err)
	}
	if resp != "response" {
		t.Fatalf("Expected 'response', got %v", resp)
	}
}

// Test circuit breaker integration
func TestInterceptor_CircuitBreaker(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false
	cfg.Thresholds = floodgate.Thresholds{
		P99Emergency: 50 * time.Millisecond,
		P95Critical:  20 * time.Millisecond,
		EMACritical:  10 * time.Millisecond,
		P95Moderate:  10 * time.Millisecond,
		EMAWarning:   5 * time.Millisecond,
		SlopeWarning: 1 * time.Millisecond,
	}

	interceptor := UnaryServerInterceptor(ctx, cfg)
	info := mockInfo("/test.Service/SlowMethod")

	// Trigger emergency state multiple times to trip circuit breaker
	for i := 0; i < 10; i++ {
		_, _ = interceptor(ctx, nil, info, mockSlowHandler)
	}

	// Circuit should eventually open, but we can't deterministically test this
	// without exposing circuit breaker state, so just verify no panic
	_, _ = interceptor(ctx, nil, info, mockHandler)
}
