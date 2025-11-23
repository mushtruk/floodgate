# Floodgate 🌊

[![Go Reference](https://pkg.go.dev/badge/github.com/mushtruk/floodgate.svg)](https://pkg.go.dev/github.com/mushtruk/floodgate)
[![Go Report Card](https://goreportcard.com/badge/github.com/mushtruk/floodgate)](https://goreportcard.com/report/github.com/mushtruk/floodgate)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A sophisticated, production-ready Go library for adaptive backpressure and load shedding based on latency tracking. Designed to prevent cascading failures in distributed systems by intelligently rejecting requests when services are overloaded.

## Features

- ⚡ **Adaptive Backpressure**: Automatically adjusts to system load using EMA (Exponential Moving Average) latency tracking
- 📊 **Percentile Tracking**: Monitors P50, P95, P99 latencies for tail latency detection
- 🔌 **Circuit Breaker**: Prevents rapid on/off toggling during emergency states
- 🎯 **gRPC & HTTP Middleware**: Drop-in middleware for gRPC and HTTP servers
- 📈 **Multi-Signal Detection**: Combines EMA, slope, drift, and percentiles for accurate backpressure levels
- 🔧 **Fully Configurable**: Environment-based thresholds for different deployment scenarios
- ⚡ **High Performance**: Sub-microsecond stats evaluation, zero allocations, <3μs total overhead per request
- 📊 **Pluggable Metrics**: Prometheus, OpenTelemetry, Datadog, or custom metrics backends
- 🔍 **Distributed Tracing**: OpenTelemetry tracing for visualizing backpressure in Jaeger, Zipkin, or APM tools
- 🔌 **Pluggable Logging**: Context-aware logging interface compatible with any Go logging framework
- 🎨 **Decorator Patterns**: Composable wrappers for observability (logging, metrics, tracing, caching, filtering)
- 🔀 **Pluggable Algorithms**: CoDel, Threshold, or custom backpressure algorithms

## Installation

```bash
go get github.com/mushtruk/floodgate
```

## Quick Start

### Basic Latency Tracking

```go
package main

import (
    "fmt"
    "time"

    "github.com/mushtruk/floodgate"
)

func main() {
    // Create a tracker
    tracker := floodgate.NewTracker(
        floodgate.WithAlpha(0.25),
        floodgate.WithWindowSize(30),
        floodgate.WithPercentiles(200), // Default: ~3.2KB per tracker
    )

    // Record latencies
    tracker.Process(150 * time.Millisecond)

    // Get statistics
    stats := tracker.Value()
    fmt.Printf("EMA: %v, P95: %v, Level: %s\n",
        stats.EMA, stats.P95, stats.Level())
}
```

### gRPC Server with Backpressure

```go
package main

import (
    "context"
    "time"

    bpgrpc "github.com/mushtruk/floodgate/grpc"
    "google.golang.org/grpc"
)

func main() {
    ctx := context.Background()

    // Configure backpressure
    cfg := bpgrpc.DefaultConfig()
    cfg.Thresholds.P95Critical = 1 * time.Second

    // Create server with backpressure
    server := grpc.NewServer(
        grpc.UnaryInterceptor(bpgrpc.UnaryServerInterceptor(ctx, cfg)),
    )

    // ... register services and serve
}
```

### HTTP Server with Backpressure

```go
package main

import (
    "context"
    "net/http"
    "time"

    bphttp "github.com/mushtruk/floodgate/http"
)

func main() {
    ctx := context.Background()

    // Configure backpressure
    cfg := bphttp.DefaultConfig()
    cfg.Thresholds.P95Critical = 1 * time.Second

    // Create your HTTP handler
    mux := http.NewServeMux()
    mux.HandleFunc("/api/users", handleUsers)

    // Wrap with backpressure middleware
    handler := bphttp.Middleware(ctx, cfg)(mux)

    // Start server
    http.ListenAndServe(":8080", handler)
}
```

## Architecture

### Backpressure Levels

The system recognizes five backpressure levels:

| Level | Description | Action |
|-------|-------------|--------|
| **Normal** | System operating normally | Allow all requests |
| **Warning** | Latency increasing | Log warnings, allow requests |
| **Moderate** | Sustained high latency | Log warnings, allow requests |
| **Critical** | P95 high + EMA elevated | Reject requests (503), retry-after: 5s |
| **Emergency** | P99 extreme outliers | Reject requests (503), retry-after: 10s |

### Detection Algorithms

#### With Percentiles Enabled (Recommended)
```
Emergency:  P99 > 10s
Critical:   P95 > 2s AND EMA > 500ms
Moderate:   P95 > 1s
Warning:    EMA > 300ms OR Slope > 10ms
```

#### Without Percentiles (Fallback)
```
Critical:   Slope > 5ms
Moderate:   Slope > 3ms
Warning:    Slope > 1ms
```

## Configuration

### Latency Tracker Options

```go
tracker := floodgate.NewTracker(
    floodgate.WithAlpha(0.25),       // EMA smoothing (0 < α < 1)
    floodgate.WithWindowSize(30),    // Trend analysis window
    floodgate.WithPercentiles(200), // Enable percentiles (default: ~3.2KB)
)
```

**WithAlpha(α float32)**
- Lower values (0.1): Smoother, less responsive to spikes
- Higher values (0.5): More responsive, tracks changes quickly
- Default: 0.25

**WithWindowSize(n int)**
- Number of EMA samples for trend calculation
- Larger = smoother trends, slower detection
- Default: 20

**WithPercentiles(bufferSize int)**
- Enables P50/P95/P99 tracking
- Buffer uses ring buffer (constant memory)
- Recommended: 1000-10000 samples

### Custom Thresholds

```go
thresholds := floodgate.Thresholds{
    P99Emergency: 10 * time.Second,
    P95Critical:  2 * time.Second,
    EMACritical:  500 * time.Millisecond,
    P95Moderate:  1 * time.Second,
    EMAWarning:   300 * time.Millisecond,
    SlopeWarning: 10 * time.Millisecond,
}

level := stats.LevelWithThresholds(thresholds)
```

### gRPC Interceptor Config

```go
cfg := bpgrpc.Config{
    CacheSize:            512,                          // Method tracker cache
    CacheTTL:             2 * time.Minute,             // Cache entry TTL
    DispatcherBufferSize: 1024,                        // Async event buffer
    Thresholds:           floodgate.DefaultThresholds(),
    SkipMethods:          []string{"/grpc.health."},   // Skip endpoints
    EnableMetrics:        true,
    MetricsInterval:      1 * time.Minute,
}
```

## Advanced Features

### Circuit Breaker

Prevents rapid on/off toggling during emergency conditions:

```go
cb := floodgate.NewCircuitBreaker(
    3,              // Open after 3 failures
    30*time.Second, // Wait 30s before trying half-open
    5,              // Close after 5 successes
)

if cb.Allow() {
    // Execute operation
    if success {
        cb.RecordSuccess()
    } else {
        cb.RecordFailure()
    }
}

fmt.Println(cb.State()) // "closed", "open", or "half-open"
```

### Async Dispatcher

Non-blocking latency recording:

```go
dispatcher := floodgate.NewDispatcher[time.Duration](ctx, 1024)

// Emit events (non-blocking)
dispatcher.Emit(tracker, latency)

// Monitor metrics
fmt.Printf("Drop rate: %.2f%%\n", dispatcher.DropRate())
```

## Performance

- **Total overhead**: <3μs per request (0.3% overhead for 1ms requests, 0.03% for 10ms)
- **Stats evaluation**: 29ns via intelligent caching (v1.5.0: 17% faster)
- **Process latency**: 32ns to record a measurement (v1.5.0: 16.7% faster)
- **Algorithm decisions**: 4.3ns (Threshold) to 50ns (CoDel), zero allocations
- **Memory**: ~3KB per tracked method (200 samples, configurable: 100-1000)
- **Zero allocations**: All hot paths are allocation-free
- **Concurrency**: Thread-safe with minimal lock contention
- **Scalability**: Linear scaling with concurrent requests
- **Decorator overhead**: Pay-per-use (zero when not instantiated)

**Benefit**: Negligible performance impact even under extreme load (100K+ req/s).

**Typical memory usage**: ~1.6 MB for 512 methods (vs 8 MB with 1K samples)

See [BENCHMARKS.md](BENCHMARKS.md) for detailed performance analysis including v1.5.0 decorator pattern overhead.

## Decorator Patterns (v1.5.0)

Floodgate provides composable decorator wrappers for adding observability without modifying core logic.

### Circuit Breaker Wrappers

Add logging, metrics, and alerting to circuit breakers:

```go
import "github.com/mushtruk/floodgate"

// Individual decorators
cb := floodgate.NewCircuitBreaker(10, 5*time.Second, 3)
cb = floodgate.WithLogging(cb, logger)
cb = floodgate.WithMetrics(cb, metrics)
cb = floodgate.WithAlerting(cb, alerter)

// Or use the fully instrumented version
cb = floodgate.NewInstrumentedCircuitBreaker(
    10,                // maxFailures
    5*time.Second,     // timeout
    3,                 // successThreshold
    logger,
    metrics,
    alerter,
)

// Usage remains the same
if cb.Allow() {
    if success {
        cb.RecordSuccess()
    } else {
        cb.RecordFailure()
    }
}
```

**Performance**: Logging/metrics overhead only on state transitions (~15ns), not on every call.

### Algorithm Wrappers

Add tracing, caching, and fallback behavior to algorithms:

```go
import (
    "github.com/mushtruk/floodgate"
    "github.com/mushtruk/floodgate/algorithms/codel"
)

// Base algorithm
algo := codel.NewAlgorithm()

// Add distributed tracing
algo = floodgate.WithTracing(algo, tracer)

// Add decision caching (100ms TTL)
algo = floodgate.NewCachedAlgorithm(algo, 100*time.Millisecond)

// Add fallback on panic
fallback := floodgate.NewThresholdAlgorithm(floodgate.DefaultThresholds())
algo = floodgate.WithFallback(algo, fallback, logger)

// Or use fully instrumented algorithm
algo = floodgate.NewInstrumentedAlgorithm(
    codel.NewAlgorithm(),
    tracer,
    logger,
    metrics,
)
```

**Performance**:
- Tracing: +100ns (acceptable for distributed tracing value)
- Caching: +5ns (hit), +10ns (miss)
- Fallback: +2ns (defer overhead only)

### Dispatcher Filters

Filter events before they're processed:

```go
import "github.com/mushtruk/floodgate"

// Create dispatcher with filters
dispatcher := floodgate.NewFilteredDispatcher[time.Duration](
    ctx,
    1024,
    floodgate.NewSamplingFilter[time.Duration](0.1),      // Sample 10%
    floodgate.NewRateLimitFilter[time.Duration](1000),    // Max 1000/sec
    floodgate.NewDeduplicationFilter[time.Duration](),    // Remove duplicates
)

// Filter chain applies in order
dispatcher.Emit(tracker, latency)
```

**Available Filters**:
- `SamplingFilter` - Sample events at specified rate (0.0 to 1.0)
- `RateLimitFilter` - Limit events per second
- `DeduplicationFilter` - Remove duplicate events
- `ThresholdFilter` - Filter based on value thresholds
- `PartitionFilter` - Route to different observers based on key

**Performance**: 3-25ns overhead per filter (early rejection avoids downstream processing).

See [ALGORITHMS.md](ALGORITHMS.md) for algorithm decorator examples.

## Observability

### Pluggable Metrics

Floodgate provides vendor-neutral metrics integration with Prometheus, OpenTelemetry, Datadog, or custom backends:

```go
import (
    prommetrics "github.com/mushtruk/floodgate/metrics/prometheus"
    "github.com/prometheus/client_golang/prometheus"
)

// Create Prometheus registry
reg := prometheus.NewRegistry()

// Configure metrics
cfg.Metrics = prommetrics.NewMetrics(reg)

// Expose /metrics endpoint
http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
```

**Available Metrics**:
- `floodgate_requests_total` - Total requests by method, level, result
- `floodgate_requests_rejected_total` - Rejected requests by method, level
- `floodgate_request_duration_seconds` - Latency histogram by method
- `floodgate_circuit_breaker_state` - Circuit breaker state (0=closed, 1=open, 2=half-open)
- `floodgate_cache_size` - Active trackers in cache
- `floodgate_dispatcher_drops_total` - Async dispatcher drops
- `floodgate_dispatcher_events_total` - Total dispatcher events

See [METRICS.md](METRICS.md) for complete metrics documentation with Prometheus, OpenTelemetry, Datadog, and custom implementations.

### Pluggable Logging

Floodgate supports any Go logging framework through a simple interface. Use the standard library slog, or integrate with zap, zerolog, or any other logger:

```go
// Using slog (Go 1.21+, recommended)
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})
cfg.Logger = floodgate.NewSlogAdapter(slog.New(handler))

// Using zap
zapLogger, _ := zap.NewProduction()
cfg.Logger = NewZapAdapter(zapLogger)

// Using zerolog
zerologLogger := zerolog.New(os.Stdout).With().Timestamp().Logger()
cfg.Logger = NewZeroLogAdapter(zerologLogger)

// Disable logging entirely
cfg.Logger = &floodgate.NoOpLogger{}
```

See [LOGGER.md](LOGGER.md) for complete logging documentation with examples for slog, zap, and zerolog.

## Examples

See the [examples](examples/) directory for complete working examples:
- [Basic Usage](examples/basic/main.go) - Core latency tracking and backpressure
- [gRPC Server](examples/grpc-server/main.go) - gRPC interceptor integration
- [HTTP Server](examples/http-server/main.go) - HTTP middleware integration
- [Prometheus Metrics](examples/prometheus-metrics/main.go) - HTTP server with Prometheus metrics and Grafana dashboard
- [OpenTelemetry Metrics](examples/otel-metrics/main.go) - gRPC server with OpenTelemetry metrics
- [Datadog Metrics](examples/datadog-metrics/main.go) - HTTP server with Datadog DogStatsD integration
- [Distributed Tracing](examples/tracing-jaeger/main.go) - Jaeger integration for visualizing backpressure in traces
- [Custom Logging](LOGGER.md#examples) - Examples for slog, zap, and zerolog integration

## Testing

```bash
go test ./...
go test -race ./...
go test -bench=. ./...
```

## Use Cases

- **API Gateways**: Protect downstream services from overload
- **Microservices**: Prevent cascading failures across service mesh
- **Queue Processors**: Adaptive rate limiting based on processing time
- **Database Proxies**: Load shedding when query latency spikes

## Comparison with Alternatives

| Feature | floodgate | netflix/concurrency-limits | uber/ratelimit |
|---------|----------------|---------------------------|----------------|
| Latency-based | ✅ | ✅ | ❌ |
| Percentile tracking | ✅ | ❌ | ❌ |
| Circuit breaker | ✅ | ✅ | ❌ |
| gRPC integration | ✅ | ❌ | ✅ |
| Configurable thresholds | ✅ | Limited | ✅ |

## Contributing

Contributions are welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure `go test ./...` passes
5. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Credits

Inspired by:
- Netflix's [concurrency-limits](https://github.com/Netflix/concurrency-limits)
- Google SRE practices for adaptive throttling
- TCP congestion control algorithms

## Support

- 📖 [Documentation](https://pkg.go.dev/github.com/mushtruk/floodgate)
- 🐛 [Issue Tracker](https://github.com/mushtruk/floodgate/issues)
- 💬 [Discussions](https://github.com/mushtruk/floodgate/discussions)

---

Made with ❤️ for building resilient distributed systems
