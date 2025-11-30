package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/mushtruk/floodgate"
	"github.com/mushtruk/floodgate/algorithms/codel"
	fhttp "github.com/mushtruk/floodgate/http"
)

// Middleware represents a composable HTTP middleware function.
type Middleware func(http.Handler) http.Handler

// Chain composes multiple middleware functions into a single middleware.
func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// LoggingMiddleware logs all incoming requests.
func LoggingMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			log.Printf("[%s] %s %s - Started", r.Method, r.URL.Path, r.RemoteAddr)

			next.ServeHTTP(w, r)

			log.Printf("[%s] %s - Completed in %v", r.Method, r.URL.Path, time.Since(start))
		})
	}
}

// RecoveryMiddleware recovers from panics and returns 500.
func RecoveryMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Printf("PANIC: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware adds CORS headers.
func CORSMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TimeoutMiddleware enforces request timeouts.
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)

			done := make(chan struct{})
			go func() {
				next.ServeHTTP(w, r)
				close(done)
			}()

			select {
			case <-done:
				// Request completed
			case <-ctx.Done():
				http.Error(w, "Request Timeout", http.StatusRequestTimeout)
			}
		})
	}
}

// BackpressureMiddleware demonstrates combining backpressure with other middleware.
func BackpressureMiddleware(ctx context.Context) Middleware {
	// Create CoDel algorithm for adaptive backpressure
	algo, err := codel.NewAlgorithm(
		codel.WithTargetDelay(5*time.Millisecond),
		codel.WithInterval(100*time.Millisecond),
	)
	if err != nil {
		log.Fatalf("Failed to create CoDel algorithm: %v", err)
	}

	cfg := fhttp.DefaultConfig()
	cfg.Algorithm = algo
	cfg.EnableMetrics = true
	cfg.Logger = floodgate.NewDefaultLogger()

	return fhttp.Middleware(ctx, cfg)
}

func main() {
	ctx := context.Background()

	// Build middleware chain
	// Order matters: outer middleware executes first (top-down for requests)
	chain := Chain(
		RecoveryMiddleware(),              // 1. Catch panics (outermost)
		LoggingMiddleware(),               // 2. Log requests
		CORSMiddleware(),                  // 3. Add CORS headers
		TimeoutMiddleware(30*time.Second), // 4. Enforce timeouts
		BackpressureMiddleware(ctx),       // 5. Apply backpressure (innermost before handler)
	)

	// Application routes
	mux := http.NewServeMux()

	// Fast endpoint
	mux.HandleFunc("/fast", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "fast", "latency": "~1ms"}`))
	})

	// Slow endpoint (will trigger backpressure under load)
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "slow", "latency": "~50ms"}`))
	})

	// Endpoint that panics (recovery middleware will catch)
	mux.HandleFunc("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("intentional panic for testing recovery")
	})

	// Health check (bypasses backpressure in config)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "healthy"}`))
	})

	// Apply middleware chain to all routes
	handler := chain(mux)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Println("=== Middleware Chain Example ===")
	log.Println("Middleware order (outer to inner):")
	log.Println("  1. Recovery (panic handling)")
	log.Println("  2. Logging (request/response logging)")
	log.Println("  3. CORS (cross-origin headers)")
	log.Println("  4. Timeout (30s request timeout)")
	log.Println("  5. Backpressure (CoDel adaptive)")
	log.Println()
	log.Println("Available endpoints:")
	log.Println("  GET  /fast   - Fast endpoint (~1ms)")
	log.Println("  GET  /slow   - Slow endpoint (~50ms, triggers backpressure)")
	log.Println("  GET  /panic  - Panics (recovery catches it)")
	log.Println("  GET  /health - Health check")
	log.Println()
	log.Printf("Server listening on %s", server.Addr)
	log.Println()
	log.Println("Test commands:")
	log.Println("  curl http://localhost:8080/fast")
	log.Println("  curl http://localhost:8080/slow")
	log.Println("  curl http://localhost:8080/panic")
	log.Println("  # Load test to trigger backpressure:")
	log.Println("  hey -z 10s -c 50 http://localhost:8080/slow")

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
