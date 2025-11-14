// Package main demonstrates distributed tracing integration with Jaeger.
//
// This example shows:
// - OpenTelemetry tracing setup with Jaeger exporter
// - Backpressure events captured as trace spans
// - Correlation between metrics and traces
// - Visualizing cascading failures in Jaeger UI
//
// Run Jaeger first:
//
//	docker run -d --name jaeger \
//	  -p 16686:16686 \
//	  -p 4318:4318 \
//	  jaegertracing/all-in-one:latest
//
// Then run the example:
//
//	go run main.go
//
// Access:
// - http://localhost:8080/api/fast - Fast endpoint
// - http://localhost:8080/api/slow - Slow endpoint (triggers backpressure)
// - http://localhost:16686 - Jaeger UI
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
	"github.com/mushtruk/floodgate/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

func initTracer() (*sdktrace.TracerProvider, error) {
	// Create OTLP HTTP exporter for Jaeger
	exporter, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint("localhost:4318"),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("floodgate-demo"),
			semconv.ServiceVersion("1.3.0"),
			semconv.DeploymentEnvironment("dev"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator for distributed tracing
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

func main() {
	ctx := context.Background()

	// Initialize tracing
	tp, err := initTracer()
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	// Create tracer for application spans
	tracer := otel.Tracer("floodgate-demo")
	_ = tracing.NewTracer(tracer) // Available for future middleware integration

	// Configure backpressure (HTTP spans created automatically by otelhttp)
	cfg := bphttp.DefaultConfig()
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

	// Fast endpoint
	mux.HandleFunc("/api/fast", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		_, span := tracer.Start(ctx, "handle_fast_request")
		defer span.End()

		time.Sleep(time.Duration(1+rand.Intn(4)) * time.Millisecond)
		fmt.Fprintf(w, "Fast response")
	})

	// Slow endpoint (triggers backpressure)
	mux.HandleFunc("/api/slow", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		_, span := tracer.Start(ctx, "handle_slow_request")
		defer span.End()

		time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)
		fmt.Fprintf(w, "Slow response")
	})

	// Simulated cascade - calls downstream service
	mux.HandleFunc("/api/cascade", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		_, span := tracer.Start(ctx, "handle_cascade_request")
		defer span.End()

		// Simulate calling downstream service (which might also have backpressure)
		_, downstreamSpan := tracer.Start(ctx, "call_downstream_service",
			trace.WithSpanKind(trace.SpanKindClient),
		)
		time.Sleep(150 * time.Millisecond)
		downstreamSpan.End()

		fmt.Fprintf(w, "Cascade response")
	})

	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})

	// Info page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Floodgate Tracing Example</title></head>
<body>
<h1>Floodgate Distributed Tracing Example</h1>
<p>Server running on :8080</p>
<p>Jaeger UI: <a href="http://localhost:16686" target="_blank">http://localhost:16686</a></p>

<h2>Endpoints</h2>
<ul>
  <li><a href="/api/fast">/api/fast</a> - Fast endpoint (1-5ms)</li>
  <li><a href="/api/slow">/api/slow</a> - Slow endpoint (100-300ms, triggers backpressure)</li>
  <li><a href="/api/cascade">/api/cascade</a> - Cascading calls example</li>
  <li><a href="/health">/health</a> - Health check (no backpressure)</li>
</ul>

<h2>What to Look For in Jaeger</h2>
<ol>
  <li><strong>Backpressure Spans</strong>: Look for "floodgate.backpressure" spans</li>
  <li><strong>Rejection Events</strong>: Spans marked as errors when requests are rejected</li>
  <li><strong>Latency Correlation</strong>: See P95/P99 latency attributes on backpressure spans</li>
  <li><strong>Circuit Breaker</strong>: Track when circuit opens/closes across requests</li>
  <li><strong>Cascading Failures</strong>: Follow trace propagation through /api/cascade</li>
</ol>

<h2>Generate Load</h2>
<pre>
# Normal load
for i in {1..100}; do curl http://localhost:8080/api/fast & done

# Trigger backpressure
for i in {1..50}; do curl http://localhost:8080/api/slow & done

# Cascade scenario
for i in {1..30}; do curl http://localhost:8080/api/cascade & done
</pre>

<h2>Jaeger Queries</h2>
<ul>
  <li>Service: <code>floodgate-demo</code></li>
  <li>Operation: <code>floodgate.backpressure</code></li>
  <li>Tags: <code>backpressure.rejected=true</code></li>
  <li>Tags: <code>backpressure.level=Critical</code></li>
</ul>

<h3>Example Trace Attributes</h3>
<ul>
  <li><code>backpressure.level</code>: Normal, Warning, Moderate, Critical, Emergency</li>
  <li><code>backpressure.ema</code>: Exponential moving average latency</li>
  <li><code>backpressure.p95</code>: 95th percentile latency</li>
  <li><code>backpressure.p99</code>: 99th percentile latency</li>
  <li><code>backpressure.rejected</code>: true/false</li>
  <li><code>circuit_breaker.state</code>: closed, open, half_open</li>
</ul>
</body>
</html>`)
	})

	// Wrap with backpressure middleware that includes tracing
	handler := bphttp.Middleware(ctx, cfg)(mux)

	// Wrap with OpenTelemetry HTTP instrumentation for automatic span creation
	otelHandler := otelhttp.NewHandler(handler, "floodgate-demo")

	addr := ":8080"
	log.Printf("Starting HTTP server on %s", addr)
	log.Printf("Jaeger UI: http://localhost:16686")
	log.Printf("Service: floodgate-demo")
	log.Printf("")
	log.Printf("Endpoints:")
	log.Printf("  - http://localhost%s/ (info page)", addr)
	log.Printf("  - http://localhost%s/api/fast", addr)
	log.Printf("  - http://localhost%s/api/slow (triggers backpressure)", addr)
	log.Printf("  - http://localhost%s/api/cascade", addr)

	if err := http.ListenAndServe(addr, otelHandler); err != nil {
		log.Fatal(err)
	}
}
