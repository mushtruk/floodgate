package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mushtruk/floodgate"
	fhttp "github.com/mushtruk/floodgate/http"
)

func main() {
	ctx := context.Background()

	algo, err := selectAlgorithm(os.Getenv("SERVICE_PROFILE"))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Created: algorithm of type %T", algo)

	startStandardServer(ctx, algo)
}

// selectAlgorithm maps a service profile to the backpressure algorithm that suits it.
func selectAlgorithm(profile string) (floodgate.Algorithm, error) {
	switch profile {
	case "", "default":
		log.Printf("Using Threshold with default thresholds")
		return floodgate.NewThresholdAlgorithm(floodgate.DefaultThresholds()), nil

	case "latency-sensitive":
		log.Printf("Using Threshold with tight thresholds")
		return floodgate.NewThresholdAlgorithm(floodgate.Thresholds{
			P99Emergency: 500 * time.Millisecond,
			P95Critical:  200 * time.Millisecond,
			EMACritical:  100 * time.Millisecond,
			P95Moderate:  100 * time.Millisecond,
			EMAWarning:   50 * time.Millisecond,
			SlopeWarning: 5 * time.Millisecond,
		}), nil

	case "high-throughput":
		// Reusing a decision for a short TTL trades reaction time for fewer evaluations per request.
		log.Printf("Using Threshold with cached decisions")
		return floodgate.NewCachedAlgorithm(
			floodgate.NewThresholdAlgorithm(floodgate.DefaultThresholds()),
			100*time.Millisecond,
		), nil

	case "disabled":
		log.Printf("Using NoOp (backpressure disabled)")
		return floodgate.NoOpAlgorithm{}, nil

	default:
		return nil, fmt.Errorf("unknown SERVICE_PROFILE %q: want default, latency-sensitive, high-throughput or disabled", profile)
	}
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
