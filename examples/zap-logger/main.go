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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create zap logger with production config
	zapConfig := zap.NewProductionConfig()
	zapConfig.EncoderConfig.TimeKey = "timestamp"
	zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)

	zapLogger, err := zapConfig.Build()
	if err != nil {
		log.Fatalf("Failed to create zap logger: %v", err)
	}
	defer zapLogger.Sync()

	// Wrap zap with floodgate adapter
	logger := NewZapAdapter(zapLogger)

	// Configure backpressure middleware with zap logger
	cfg := floodgatehttp.DefaultConfig()
	cfg.Thresholds.EMAWarning = 50 * time.Millisecond
	cfg.Thresholds.P95Moderate = 100 * time.Millisecond
	cfg.Thresholds.EMACritical = 150 * time.Millisecond
	cfg.Thresholds.P95Critical = 200 * time.Millisecond
	cfg.Thresholds.P99Emergency = 300 * time.Millisecond
	cfg.Logger = logger // Inject zap logger

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
		log.Printf("Starting HTTP server on :8080 with zap")
		log.Printf("Logs will be in JSON format with high-performance structured fields")
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
