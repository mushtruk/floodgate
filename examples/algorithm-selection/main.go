package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mushtruk/floodgate"
	"github.com/mushtruk/floodgate/algorithms/codel"
	fhttp "github.com/mushtruk/floodgate/http"
)

func main() {
	ctx := context.Background()

	// Type-safe configuration
	cfg := codel.Config{
		TargetDelay: 5 * time.Millisecond,
		Interval:    100 * time.Millisecond,
		Workers:     100,
		QueueSize:   1000,
		Thresholds:  floodgate.DefaultThresholds(),
	}

	// Type-safe algorithm selection
	switch os.Getenv("SERVICE_PROFILE") {
	case "cpu-bound":
		cfg.Type = codel.AlgorithmTypeStandard
		log.Printf("Using Standard CoDel (CPU-bound, 49 ns/op)")

	case "io-bound":
		cfg.Type = codel.AlgorithmTypeQueue
		log.Printf("Using Queue CoDel (I/O-bound, true sojourn time)")

	case "", "safe-default":
		cfg.Type = codel.AlgorithmTypeThreshold
		log.Printf("Using Threshold (multi-signal, 5 ns/op)")

	default:
		// Auto-recommend based on service characteristics
		cpuBound := os.Getenv("CPU_BOUND") == "true"
		avgLatency := 50 * time.Millisecond
		cfg.Type = codel.RecommendAlgorithmType(cpuBound, avgLatency)
		log.Printf("Auto-selected: %s (cpuBound=%v)", cfg.Type, cpuBound)
	}

	// Create algorithm from type-safe config
	algo, err := codel.NewAlgorithmFromConfig(cfg)
	if err != nil {
		log.Fatalf("Failed to create algorithm: %v", err)
	}

	// Log the algorithm type
	switch algo.(type) {
	case *codel.Algorithm:
		log.Printf("Created: Standard CoDel")
	case *floodgate.ThresholdAlgorithm:
		log.Printf("Created: Threshold")
	default:
		log.Printf("Created: Algorithm of type %T", algo)
	}

	// Start server with the selected algorithm
	startStandardServer(ctx, algo)
}

func startStandardServer(ctx context.Context, algorithm floodgate.Algorithm) {
	cfg := fhttp.DefaultConfig()
	cfg.Algorithm = algorithm
	cfg.EnableMetrics = true

	middleware := fhttp.Middleware(ctx, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/fast", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Fast endpoint\n")
	})

	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprintf(w, "Slow endpoint\n")
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK\n")
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: middleware(mux),
	}

	log.Printf("Server starting on :8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

