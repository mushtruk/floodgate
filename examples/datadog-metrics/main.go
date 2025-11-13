// Package main demonstrates how to integrate Datadog metrics with floodgate HTTP middleware.
//
// This example shows:
// - Setting up Datadog DogStatsD client
// - Configuring floodgate with Datadog metrics
// - Sending metrics to Datadog agent
// - Using custom namespaces and tags
//
// Prerequisites:
// - Datadog agent running locally on port 8125
// - Or use DD_AGENT_HOST environment variable
//
// Run the example:
//
//	go run main.go
//
// Then test with:
//
//	# Fast endpoint (normal load)
//	curl http://localhost:8080/api/fast
//
//	# Slow endpoint (triggers backpressure)
//	curl http://localhost:8080/api/slow
//
//	# Generate load
//	for i in {1..100}; do curl http://localhost:8080/api/slow & done
//
// View metrics in Datadog:
// - Navigate to Metrics Explorer
// - Search for "floodgate.requests.total"
// - Filter by service:api, env:dev
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/mushtruk/floodgate"
	bphttp "github.com/mushtruk/floodgate/http"
	ddmetrics "github.com/mushtruk/floodgate/metrics/datadog"
)

func main() {
	ctx := context.Background()

	// Get Datadog agent address from environment or use default
	ddAgentAddr := os.Getenv("DD_AGENT_HOST")
	if ddAgentAddr == "" {
		ddAgentAddr = "localhost:8125"
	}

	// Create Datadog DogStatsD client
	client, err := statsd.New(ddAgentAddr,
		statsd.WithNamespace("myapp"),
		statsd.WithTags([]string{
			"env:dev",
			"service:api",
			"version:1.3.0",
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create DogStatsD client: %v", err)
	}
	defer client.Close()

	// Create floodgate Datadog metrics collector
	metrics := ddmetrics.NewMetrics(client)

	// Configure backpressure with Datadog metrics
	cfg := bphttp.DefaultConfig()
	cfg.Metrics = metrics
	cfg.Thresholds = floodgate.Thresholds{
		P99Emergency: 500 * time.Millisecond,
		P95Critical:  200 * time.Millisecond,
		EMACritical:  100 * time.Millisecond,
		P95Moderate:  150 * time.Millisecond,
		EMAWarning:   50 * time.Millisecond,
		SlopeWarning: 10 * time.Millisecond,
	}
	cfg.SkipPaths = []string{"/health"}

	// Create HTTP handlers
	mux := http.NewServeMux()

	// Fast endpoint - minimal latency
	mux.HandleFunc("/api/fast", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(1+rand.Intn(4)) * time.Millisecond)
		fmt.Fprintf(w, "Fast response")
	})

	// Slow endpoint - high latency (triggers backpressure)
	mux.HandleFunc("/api/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)
		fmt.Fprintf(w, "Slow response")
	})

	// Health endpoint (skipped by backpressure)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})

	// Info page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Floodgate Datadog Example</title></head>
<body>
<h1>Floodgate Datadog Metrics Example</h1>
<p>Server running on :8080</p>
<p>Metrics sent to Datadog agent at %s</p>

<h2>Endpoints</h2>
<ul>
  <li><a href="/api/fast">/api/fast</a> - Fast endpoint (1-5ms)</li>
  <li><a href="/api/slow">/api/slow</a> - Slow endpoint (100-300ms, triggers backpressure)</li>
  <li><a href="/health">/health</a> - Health check (no backpressure)</li>
</ul>

<h2>Datadog Metrics</h2>
<p>View in Datadog Metrics Explorer:</p>
<ul>
  <li><code>myapp.floodgate.requests.total</code></li>
  <li><code>myapp.floodgate.requests.rejected</code></li>
  <li><code>myapp.floodgate.request.duration</code></li>
  <li><code>myapp.floodgate.circuit_breaker.state</code></li>
  <li><code>myapp.floodgate.cache.size</code></li>
  <li><code>myapp.floodgate.dispatcher.drops</code></li>
  <li><code>myapp.floodgate.dispatcher.events</code></li>
</ul>

<h2>Tags</h2>
<ul>
  <li>env:dev</li>
  <li>service:api</li>
  <li>version:1.3.0</li>
  <li>method:&lt;endpoint&gt;</li>
  <li>level:&lt;backpressure_level&gt;</li>
</ul>

<h2>Generate Load</h2>
<pre>
# Normal load
for i in {1..100}; do curl http://localhost:8080/api/fast & done

# Trigger backpressure
for i in {1..50}; do curl http://localhost:8080/api/slow & done
</pre>
</body>
</html>`, ddAgentAddr)
	})

	// Wrap with backpressure middleware
	handler := bphttp.Middleware(ctx, cfg)(mux)

	addr := ":8080"
	log.Printf("Starting HTTP server on %s", addr)
	log.Printf("Datadog agent: %s", ddAgentAddr)
	log.Printf("Endpoints:")
	log.Printf("  - http://localhost%s/ (info page)", addr)
	log.Printf("  - http://localhost%s/api/fast (fast endpoint)", addr)
	log.Printf("  - http://localhost%s/api/slow (slow endpoint - triggers backpressure)", addr)
	log.Printf("  - http://localhost%s/health (health check)", addr)
	log.Printf("")
	log.Printf("Metrics namespace: myapp.floodgate.*")
	log.Printf("Tags: env:dev, service:api, version:1.3.0")

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
