package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	floodgatehttp "github.com/mushtruk/floodgate/http"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Configure backpressure middleware
	cfg := floodgatehttp.DefaultConfig()
	cfg.Thresholds.EMAWarning = 50 * time.Millisecond
	cfg.Thresholds.P95Moderate = 100 * time.Millisecond
	cfg.Thresholds.EMACritical = 150 * time.Millisecond
	cfg.Thresholds.P95Critical = 200 * time.Millisecond
	cfg.Thresholds.P99Emergency = 300 * time.Millisecond

	// Create router
	mux := http.NewServeMux()

	// Register handlers
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	mux.HandleFunc("/api/fast", func(w http.ResponseWriter, r *http.Request) {
		// Fast endpoint
		time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Fast response")
	})

	mux.HandleFunc("/api/slow", func(w http.ResponseWriter, r *http.Request) {
		// Slow endpoint that might trigger backpressure
		time.Sleep(time.Duration(50+rand.Intn(200)) * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Slow response")
	})

	mux.HandleFunc("/api/variable", func(w http.ResponseWriter, r *http.Request) {
		// Variable latency endpoint
		latency := rand.Intn(300)
		time.Sleep(time.Duration(latency) * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Variable response (latency: %dms)", latency)
	})

	// Wrap with backpressure middleware
	handler := floodgatehttp.Middleware(ctx, cfg)(mux)

	// Create server
	server := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	// Start server
	go func() {
		log.Printf("Starting HTTP server on :8080")
		log.Printf("Endpoints:")
		log.Printf("  GET /health        - Health check (skipped from tracking)")
		log.Printf("  GET /api/fast      - Fast endpoint (~10ms)")
		log.Printf("  GET /api/slow      - Slow endpoint (50-250ms)")
		log.Printf("  GET /api/variable  - Variable latency (0-300ms)")
		log.Printf("")
		log.Printf("Try: curl http://localhost:8080/api/slow")
		log.Printf("Load test: while true; do curl http://localhost:8080/api/slow & done")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down server...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server stopped")
}
