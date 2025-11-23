// Package main demonstrates using the CoDel algorithm with HTTP middleware.
//
// This example shows how to use the adaptive CoDel algorithm instead of
// the default threshold-based algorithm for backpressure control.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mushtruk/floodgate/algorithms/codel"
	httpMiddleware "github.com/mushtruk/floodgate/http"
)

func main() {
	// math/rand/v2 doesn't require seeding - automatically seeded

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Configure HTTP middleware with CoDel algorithm
	cfg := httpMiddleware.DefaultConfig()

	// Use CoDel algorithm with custom target delay
	// Target delay: 5ms - requests experiencing higher queueing delay will trigger backpressure
	// Interval: 100ms - delay must persist for this duration before entering dropping mode
	cfg.Algorithm = codel.NewAlgorithm(
		codel.WithTargetDelay(5*time.Millisecond),
		codel.WithInterval(100*time.Millisecond),
	)

	// Enable metrics logging
	cfg.EnableMetrics = true
	cfg.MetricsInterval = 10 * time.Second

	// Skip health check endpoints
	cfg.SkipPaths = []string{"/health", "/metrics"}

	// Create middleware
	middleware := httpMiddleware.Middleware(ctx, cfg)

	// Create HTTP mux
	mux := http.NewServeMux()

	// Health check endpoint (skipped by middleware)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Fast endpoint - low latency
	mux.HandleFunc("/fast", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Fast response"))
	})

	// Variable endpoint - simulates bursty workload
	mux.HandleFunc("/variable", func(w http.ResponseWriter, _ *http.Request) {
		// Random latency: 1-50ms (using math/rand for demo simplicity)
		latency := time.Duration(1+rand.IntN(50)) * time.Millisecond //nolint:gosec // Demo code, not security-sensitive
		time.Sleep(latency)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "Variable response (latency: %v)", latency)
	})

	// Slow endpoint - high latency
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		// This will trigger CoDel backpressure after persistent delay
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Slow response"))
	})

	// Apply middleware
	handler := middleware(mux)

	// Create server
	server := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on :8080")
		log.Printf("Using CoDel algorithm (target: 5ms, interval: 100ms)")
		log.Println()
		log.Println("Endpoints:")
		log.Println("  GET /health    - Health check (no backpressure)")
		log.Println("  GET /fast      - Fast endpoint (~1ms)")
		log.Println("  GET /variable  - Variable latency (1-50ms)")
		log.Println("  GET /slow      - Slow endpoint (~20ms, triggers backpressure)")
		log.Println()
		log.Println("Try these commands:")
		log.Println("  # Fast endpoint - should never trigger backpressure")
		log.Println("  hey -n 10000 -c 50 http://localhost:8080/fast")
		log.Println()
		log.Println("  # Variable endpoint - may trigger backpressure under high load")
		log.Println("  hey -n 10000 -c 100 http://localhost:8080/variable")
		log.Println()
		log.Println("  # Slow endpoint - will trigger CoDel backpressure")
		log.Println("  hey -n 10000 -c 100 http://localhost:8080/slow")
		log.Println()

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("\nShutting down server...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
		shutdownCancel()
		os.Exit(1) //nolint:gocritic // Acceptable for example code to exit on shutdown error
	}

	shutdownCancel()
	log.Println("Server stopped")
}
