package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mushtruk/floodgate"
	floodgatehttp "github.com/mushtruk/floodgate/http"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create slog logger with JSON output and custom level
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Set minimum log level (Debug, Info, Warn, Error)
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Add custom attributes or modify existing ones
			if a.Key == slog.TimeKey {
				return slog.Attr{Key: "timestamp", Value: a.Value}
			}
			return a
		},
	})
	slogLogger := slog.New(handler)

	// Wrap slog with floodgate adapter
	logger := floodgate.NewSlogAdapter(slogLogger)

	// Configure backpressure middleware with slog logger
	cfg := floodgatehttp.DefaultConfig()
	cfg.Thresholds.EMAWarning = 50 * time.Millisecond
	cfg.Thresholds.P95Moderate = 100 * time.Millisecond
	cfg.Thresholds.EMACritical = 150 * time.Millisecond
	cfg.Thresholds.P95Critical = 200 * time.Millisecond
	cfg.Thresholds.P99Emergency = 300 * time.Millisecond
	cfg.Logger = logger // Inject slog logger

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
	httpHandler := floodgatehttp.Middleware(ctx, cfg)(mux)

	// Create server
	server := &http.Server{
		Addr:    ":8080",
		Handler: httpHandler,
	}

	// Start server
	go func() {
		log.Printf("Starting HTTP server on :8080 with slog (Go 1.21+)")
		log.Printf("Logs will be in JSON format with structured fields")
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
