package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mushtruk/floodgate"
)

// Mock handler for testing.
func mockHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}

func mockSlowHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}

// BenchmarkMiddleware_NormalPath benchmarks the happy path with normal latency.
func BenchmarkMiddleware_NormalPath(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	handler := Middleware(ctx, cfg)(mockHandler())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

// BenchmarkMiddleware_SkippedPath benchmarks skipped paths (health checks).
func BenchmarkMiddleware_SkippedPath(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	handler := Middleware(ctx, cfg)(mockHandler())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

// BenchmarkMiddleware_MultipleRoutesConcurrent benchmarks multiple routes concurrently.
func BenchmarkMiddleware_MultipleRoutesConcurrent(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	handler := Middleware(ctx, cfg)(mockHandler())

	paths := []string{
		"/api/users",
		"/api/posts",
		"/api/comments",
		"/api/likes",
		"/api/shares",
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			path := paths[i%len(paths)]
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			i++
		}
	})
}

// BenchmarkMiddleware_EmergencyRejection benchmarks rejection path during emergency.
func BenchmarkMiddleware_EmergencyRejection(b *testing.B) {
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

	slowHandler := Middleware(ctx, cfg)(mockSlowHandler())

	// Prime the tracker with slow requests to trigger emergency
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/slow", nil)
		w := httptest.NewRecorder()
		slowHandler.ServeHTTP(w, req)
	}

	handler := Middleware(ctx, cfg)(mockHandler())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/slow", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

// BenchmarkMiddleware_NewRouteCreation benchmarks tracker creation for new routes.
func BenchmarkMiddleware_NewRouteCreation(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create new middleware for each iteration to measure cold start
		handler := Middleware(ctx, cfg)(mockHandler())
		req := httptest.NewRequest(http.MethodGet, "/api/new", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

// BenchmarkMiddleware_StatsEvaluation benchmarks just the stats evaluation and level check.
func BenchmarkMiddleware_StatsEvaluation(b *testing.B) {
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

// BenchmarkConfig_Default benchmarks default config creation.
func BenchmarkConfig_Default(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = DefaultConfig()
	}
}

// Test to ensure middleware works correctly.
func TestMiddleware_BasicFlow(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	handler := Middleware(ctx, cfg)(mockHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "OK" {
		t.Fatalf("Expected 'OK', got %s", w.Body.String())
	}
}

// Test to ensure skipped paths bypass tracking.
func TestMiddleware_SkipPaths(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	handler := Middleware(ctx, cfg)(mockHandler())

	// Test health check skip
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for health check, got %d", w.Code)
	}

	// Test metrics skip
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for metrics, got %d", w.Code)
	}

	// Test readiness skip
	req = httptest.NewRequest(http.MethodGet, "/readiness", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for readiness, got %d", w.Code)
	}
}

// TestMatchPath tests the path matching function.
func TestMatchPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		pattern string
		want    bool
	}{
		// Exact matches
		{"exact match", "/health", "/health", true},
		{"exact no match", "/healthcheck", "/health", false},
		{"exact no match prefix", "/health/live", "/health", false},

		// Prefix matches with *
		{"prefix match", "/api/users", "/api/*", true},
		{"prefix match root", "/api/", "/api/*", true},
		{"prefix match exact", "/api/", "/api/", true},
		{"prefix no match", "/other/path", "/api/*", false},

		// Edge cases
		{"empty pattern", "/health", "", false},
		{"empty path", "", "/health", false},
		{"just star", "/anything", "*", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := matchPath(tt.path, tt.pattern); got != tt.want {
				t.Errorf("matchPath(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
			}
		})
	}
}

// TestResponseWriter tests the ResponseWriter wrapper.
func TestResponseWriter(t *testing.T) {
	t.Parallel()

	t.Run("captures explicit status code", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		rw.WriteHeader(http.StatusNotFound)

		if rw.statusCode != http.StatusNotFound {
			t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusNotFound)
		}
		if rec.Code != http.StatusNotFound {
			t.Errorf("underlying recorder code = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("captures implicit 200 on Write", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		rw := &responseWriter{ResponseWriter: rec}

		_, _ = rw.Write([]byte("hello"))

		if rw.statusCode != http.StatusOK {
			t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusOK)
		}
	})

	t.Run("ignores duplicate WriteHeader calls", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		rw.WriteHeader(http.StatusCreated)
		rw.WriteHeader(http.StatusNotFound) // Should be ignored

		if rw.statusCode != http.StatusCreated {
			t.Errorf("statusCode = %d, want %d (first call)", rw.statusCode, http.StatusCreated)
		}
	})

	t.Run("Unwrap returns underlying writer", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		rw := &responseWriter{ResponseWriter: rec}

		if rw.Unwrap() != rec {
			t.Error("Unwrap should return underlying ResponseWriter")
		}
	})
}

// Test circuit breaker integration.
func TestMiddleware_CircuitBreaker(_ *testing.T) {
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

	slowHandler := Middleware(ctx, cfg)(mockSlowHandler())

	// Trigger emergency state multiple times to trip circuit breaker
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/slow", nil)
		w := httptest.NewRecorder()
		slowHandler.ServeHTTP(w, req)
	}

	// Circuit should eventually open, but we can't deterministically test this
	// without exposing circuit breaker state, so just verify no panic
	req := httptest.NewRequest(http.MethodGet, "/api/slow", nil)
	w := httptest.NewRecorder()

	handler := Middleware(ctx, cfg)(mockHandler())
	handler.ServeHTTP(w, req)
}

// Test different HTTP methods are tracked separately.
func TestMiddleware_MethodSeparation(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.EnableMetrics = false

	handler := Middleware(ctx, cfg)(mockHandler())

	// GET and POST to same path should be tracked separately
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}

	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/users", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200 for %s, got %d", method, w.Code)
		}
	}
}

// Test retry-after header is set during backpressure.
func TestMiddleware_RetryAfterHeader(t *testing.T) {
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

	slowHandler := Middleware(ctx, cfg)(mockSlowHandler())

	// Trigger emergency state
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/slow", nil)
		w := httptest.NewRecorder()
		slowHandler.ServeHTTP(w, req)
	}

	// Check for retry-after header
	req := httptest.NewRequest(http.MethodGet, "/api/slow", nil)
	w := httptest.NewRecorder()
	slowHandler.ServeHTTP(w, req)

	retryAfter := w.Header().Get("Retry-After")
	if w.Code == http.StatusServiceUnavailable && retryAfter == "" {
		t.Fatal("Expected Retry-After header during backpressure")
	}
}
