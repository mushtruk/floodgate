package main

import (
	"context"
	"log"
	"net"
	"time"

	bpgrpc "github.com/mushtruk/floodgate/grpc"
	"google.golang.org/grpc"
)

// This is a basic example demonstrating the floodgate interceptor.
// In production:
// - Use structured logging (e.g., zerolog, zap)
// - Implement graceful shutdown with signal handling
// - Add proper error recovery and monitoring
// - Configure TLS and authentication

func main() {
	ctx := context.Background()

	// Configure backpressure
	cfg := bpgrpc.DefaultConfig()
	cfg.Thresholds.P95Critical = 1 * time.Second
	cfg.Thresholds.EMAWarning = 200 * time.Millisecond

	// Create gRPC server with backpressure interceptor
	server := grpc.NewServer(
		grpc.UnaryInterceptor(bpgrpc.UnaryServerInterceptor(ctx, cfg)),
	)

	// Register your services here
	// pb.RegisterYourServiceServer(server, &yourService{})

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err) // In production, handle gracefully
	}

	log.Println("Server starting with adaptive backpressure on :50051")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err) // In production, handle gracefully
	}
}
