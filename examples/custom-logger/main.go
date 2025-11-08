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
	"github.com/rs/zerolog"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create zerolog logger with JSON output
	zerologLogger := zerolog.New(os.Stdout).
		With().
		Timestamp().
		Str("service", "floodgate-example").
		Logger()

	// Wrap zerolog with floodgate adapter
	logger := NewZeroLogAdapter(zerologLogger)

	// Configure backpressure middleware with custom logger
	cfg := floodgatehttp.DefaultConfig()
	cfg.Thresholds.EMAWarning = 50 * time.Millisecond
	cfg.Thresholds.P95Moderate = 100 * time.Millisecond
	cfg.Thresholds.EMACritical = 150 * time.Millisecond
	cfg.Thresholds.P95Critical = 200 * time.Millisecond
	cfg.Thresholds.P99Emergency = 300 * time.Millisecond
	cfg.Logger = logger // Inject custom logger

	// You can also disable logging entirely with NoOpLogger:
	// cfg.Logger = &floodgate.NoOpLogger{}

	// Create router
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	mux.HandleFunc("/api/fast", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Fast response")
	})

	mux.HandleFunc("/api/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(50+rand.Intn(200)) * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Slow response")
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
		log.Printf("Starting HTTP server on :8080 with zerolog")
		log.Printf("Logs will be in JSON format")
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
