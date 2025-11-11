// Package main demonstrates how to integrate Prometheus metrics with floodgate HTTP middleware.
//
// This example shows:
// - Setting up Prometheus registry
// - Configuring floodgate with Prometheus metrics
// - Exposing /metrics endpoint for scraping
// - Simulating various backpressure scenarios
//
// Run the example:
//
//	go run main.go
//
// Then access:
// - http://localhost:8080/api/fast - Fast endpoint (normal operations)
// - http://localhost:8080/api/slow - Slow endpoint (triggers backpressure)
// - http://localhost:8080/metrics - Prometheus metrics endpoint
//
// View metrics with curl:
//
//	curl http://localhost:8080/metrics | grep floodgate
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/mushtruk/floodgate"
	bphttp "github.com/mushtruk/floodgate/http"
	prommetrics "github.com/mushtruk/floodgate/metrics/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	ctx := context.Background()

	// Create Prometheus registry
	reg := prometheus.NewRegistry()

	// Create floodgate Prometheus metrics collector
	metrics := prommetrics.NewMetrics(reg)

	// Configure backpressure with Prometheus metrics
	cfg := bphttp.DefaultConfig()
	cfg.Metrics = metrics
	cfg.Thresholds = floodgate.Thresholds{
		P99Emergency: 500 * time.Millisecond,  // Emergency at 500ms P99
		P95Critical:  200 * time.Millisecond,  // Critical at 200ms P95
		EMACritical:  100 * time.Millisecond,  // And 100ms EMA
		P95Moderate:  150 * time.Millisecond,  // Moderate at 150ms P95
		EMAWarning:   50 * time.Millisecond,   // Warning at 50ms EMA
		SlopeWarning: 10 * time.Millisecond,   // Warning on 10ms slope
	}
	cfg.SkipPaths = []string{"/health", "/metrics"}

	// Create HTTP handlers
	mux := http.NewServeMux()

	// Fast endpoint - minimal latency
	mux.HandleFunc("/api/fast", func(w http.ResponseWriter, r *http.Request) {
		// Simulate 1-5ms processing
		time.Sleep(time.Duration(1+rand.Intn(4)) * time.Millisecond)
		fmt.Fprintf(w, "Fast response")
	})

	// Slow endpoint - high latency (triggers backpressure)
	mux.HandleFunc("/api/slow", func(w http.ResponseWriter, r *http.Request) {
		// Simulate 100-300ms processing
		time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)
		fmt.Fprintf(w, "Slow response")
	})

	// Health endpoint (skipped by backpressure)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		Registry: reg,
	}))

	// Wrap with backpressure middleware
	handler := bphttp.Middleware(ctx, cfg)(mux)

	// Start server
	addr := ":8080"
	log.Printf("Starting HTTP server on %s", addr)
	log.Printf("Endpoints:")
	log.Printf("  - http://localhost%s/api/fast (fast endpoint)", addr)
	log.Printf("  - http://localhost%s/api/slow (slow endpoint - triggers backpressure)", addr)
	log.Printf("  - http://localhost%s/health (health check - no backpressure)", addr)
	log.Printf("  - http://localhost%s/metrics (Prometheus metrics)", addr)
	log.Printf("")
	log.Printf("Example commands:")
	log.Printf("  # Generate load on fast endpoint")
	log.Printf("  for i in {1..100}; do curl http://localhost%s/api/fast & done", addr)
	log.Printf("")
	log.Printf("  # Generate load on slow endpoint (triggers backpressure)")
	log.Printf("  for i in {1..50}; do curl http://localhost%s/api/slow & done", addr)
	log.Printf("")
	log.Printf("  # View metrics")
	log.Printf("  curl http://localhost%s/metrics | grep floodgate", addr)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
