// Package main demonstrates how to integrate OpenTelemetry metrics with floodgate gRPC interceptor.
//
// This example shows:
// - Setting up OpenTelemetry with Prometheus exporter
// - Configuring floodgate with OpenTelemetry metrics
// - Exposing /metrics endpoint for scraping
// - Simulating various backpressure scenarios via gRPC
//
// Run the example:
//
//	go run main.go
//
// Then test with grpcurl:
//
//	# Install grpcurl if needed
//	go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
//
//	# Test fast endpoint (normal operations)
//	grpcurl -plaintext -d '{"name": "test"}' localhost:50051 example.EchoService/FastEcho
//
//	# Test slow endpoint (triggers backpressure)
//	grpcurl -plaintext -d '{"name": "test"}' localhost:50051 example.EchoService/SlowEcho
//
//	# View metrics
//	curl http://localhost:8080/metrics
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/mushtruk/floodgate"
	bpgrpc "github.com/mushtruk/floodgate/grpc"
	otelmetrics "github.com/mushtruk/floodgate/metrics/opentelemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "github.com/mushtruk/floodgate/examples/otel-metrics/proto"
	promhttp "github.com/prometheus/client_golang/prometheus/promhttp"
)

type echoServer struct {
	pb.UnimplementedEchoServiceServer
}

func (s *echoServer) FastEcho(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	// Simulate 1-5ms processing
	time.Sleep(time.Duration(1+rand.Intn(4)) * time.Millisecond)
	return &pb.EchoResponse{Message: fmt.Sprintf("Fast echo: %s", req.Name)}, nil
}

func (s *echoServer) SlowEcho(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	// Simulate 100-300ms processing (triggers backpressure)
	time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)
	return &pb.EchoResponse{Message: fmt.Sprintf("Slow echo: %s", req.Name)}, nil
}

func (s *echoServer) HealthCheck(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Status: "OK"}, nil
}

func main() {
	ctx := context.Background()

	// Create OpenTelemetry Prometheus exporter
	exporter, err := prometheus.New()
	if err != nil {
		log.Fatalf("Failed to create Prometheus exporter: %v", err)
	}

	// Create meter provider with Prometheus exporter
	provider := metric.NewMeterProvider(
		metric.WithReader(exporter),
	)
	otel.SetMeterProvider(provider)

	// Create meter
	meter := otel.Meter("floodgate")

	// Create floodgate OpenTelemetry metrics collector
	metrics, err := otelmetrics.NewMetrics(meter)
	if err != nil {
		log.Fatalf("Failed to create metrics: %v", err)
	}

	// Configure backpressure with OpenTelemetry metrics
	cfg := bpgrpc.DefaultConfig()
	cfg.Metrics = metrics
	cfg.Thresholds = floodgate.Thresholds{
		P99Emergency: 500 * time.Millisecond,  // Emergency at 500ms P99
		P95Critical:  200 * time.Millisecond,  // Critical at 200ms P95
		EMACritical:  100 * time.Millisecond,  // And 100ms EMA
		P95Moderate:  150 * time.Millisecond,  // Moderate at 150ms P95
		EMAWarning:   50 * time.Millisecond,   // Warning at 50ms EMA
		SlopeWarning: 10 * time.Millisecond,   // Warning on 10ms slope
	}
	cfg.SkipMethods = []string{
		"/grpc.health.",
		"/grpc.reflection.",
		"/example.EchoService/HealthCheck",
	}

	// Create gRPC server with backpressure interceptor
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(bpgrpc.UnaryServerInterceptor(ctx, cfg)),
	)

	// Register echo service
	pb.RegisterEchoServiceServer(grpcServer, &echoServer{})

	// Register reflection service for grpcurl
	reflection.Register(grpcServer)

	// Start gRPC server
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	go func() {
		log.Printf("Starting gRPC server on :50051")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Start HTTP server for metrics
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Floodgate OpenTelemetry Example</title></head>
<body>
<h1>Floodgate OpenTelemetry Metrics Example</h1>
<p>gRPC server running on :50051</p>
<p><a href="/metrics">View Metrics</a></p>

<h2>Test Commands</h2>
<pre>
# Install grpcurl
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# Fast endpoint (normal load)
grpcurl -plaintext -d '{"name": "test"}' localhost:50051 example.EchoService/FastEcho

# Slow endpoint (triggers backpressure)
grpcurl -plaintext -d '{"name": "test"}' localhost:50051 example.EchoService/SlowEcho

# Health check (skipped by backpressure)
grpcurl -plaintext -d '{}' localhost:50051 example.EchoService/HealthCheck

# List services
grpcurl -plaintext localhost:50051 list

# Generate load (fast)
for i in {1..100}; do
  grpcurl -plaintext -d '{"name": "test"}' localhost:50051 example.EchoService/FastEcho &
done

# Generate load (slow - triggers backpressure)
for i in {1..50}; do
  grpcurl -plaintext -d '{"name": "test"}' localhost:50051 example.EchoService/SlowEcho &
done
</pre>
</body>
</html>`)
	})

	metricsAddr := ":8080"
	log.Printf("Starting HTTP metrics server on %s", metricsAddr)
	log.Printf("Endpoints:")
	log.Printf("  - http://localhost%s (info page)", metricsAddr)
	log.Printf("  - http://localhost%s/metrics (OpenTelemetry metrics)", metricsAddr)
	log.Printf("")
	log.Printf("gRPC Methods:")
	log.Printf("  - example.EchoService/FastEcho (fast endpoint)")
	log.Printf("  - example.EchoService/SlowEcho (slow endpoint - triggers backpressure)")
	log.Printf("  - example.EchoService/HealthCheck (health check - no backpressure)")

	if err := http.ListenAndServe(metricsAddr, nil); err != nil {
		log.Fatal(err)
	}
}
