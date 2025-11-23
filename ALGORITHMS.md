# Backpressure Algorithms

Floodgate supports pluggable backpressure algorithms that determine when and how to shed load. This document explains the available algorithms, their performance characteristics, and when to use each one.

## Overview

All algorithms implement the `Algorithm` interface:

```go
type Algorithm interface {
    Decide(stats Stats) Decision
}

type Decision struct {
    Level  Level  // Backpressure severity (Normal to Emergency)
    Reject bool   // Whether to reject this request
}
```

## Available Algorithms

### 1. Threshold Algorithm (Default)

**Location**: Core package (`floodgate.ThresholdAlgorithm`)

**How it works**: Uses fixed latency thresholds to classify backpressure levels. Rejects requests when latency exceeds Emergency thresholds.

**Configuration**:
```go
cfg.Algorithm = floodgate.NewThresholdAlgorithm(floodgate.Thresholds{
    P99Emergency: 10 * time.Second,
    P95Critical:  2 * time.Second,
    EMACritical:  500 * time.Millisecond,
    P95Moderate:  1 * time.Second,
    EMAWarning:   300 * time.Millisecond,
    SlopeWarning: 10 * time.Millisecond,
})
```

**Performance**:
```
BenchmarkThresholdAlgorithm_Decide-10    235015132    5.108 ns/op    0 B/op    0 allocs/op
```

**When to use**:
- **Predictable workloads** with known latency SLAs
- **Fixed performance requirements** (e.g., "P95 must be < 100ms")
- **Simple deployments** where you want explicit control over thresholds
- **Production systems** where you understand your latency baselines

**Pros**:
- Extremely fast (~5 ns/op)
- Zero allocations
- Predictable behavior
- Easy to tune based on SLAs

**Cons**:
- Requires manual tuning for different workloads
- Fixed thresholds don't adapt to changing conditions
- May be too aggressive or too lenient depending on load patterns

---

### 2. CoDel Algorithm (Adaptive)

**Location**: `github.com/mushtruk/floodgate/algorithms/codel`

**How it works**: Based on the CoDel (Controlled Delay) active queue management algorithm. Monitors sojourn time (queueing delay) and adaptively drops requests when delay persistently exceeds a target. Uses a control law to increase drop frequency with `sqrt(count)`.

Floodgate provides two CoDel variants:
- **Decision-based CoDel** (`codel.NewAlgorithm`): Returns decision to accept/reject
- **Queue-based CoDel** (`codel.NewQueueAlgorithm`): Manages actual request queue with true sojourn time

**Configuration (Decision-based)**:
```go
import "github.com/mushtruk/floodgate/algorithms/codel"

cfg.Algorithm = codel.NewAlgorithm(
    codel.WithTargetDelay(5 * time.Millisecond),  // Target queueing delay
    codel.WithInterval(100 * time.Millisecond),   // Persistence interval
)
```

**Configuration (Queue-based)**:
```go
import "github.com/mushtruk/floodgate/algorithms/codel"

// Queue-based CoDel with actual request queuing
queue := codel.NewQueueAlgorithm(
    100,                        // Queue size
    5*time.Millisecond,         // Target delay
    100*time.Millisecond,       // Interval
)

// Enqueue request with handler
err := queue.Enqueue(ctx, func(ctx context.Context) error {
    // Your request handler
    return processRequest(ctx)
})

if errors.Is(err, floodgate.ErrBackpressure) {
    // Request rejected or queue full
}
```

**Performance**:
```
BenchmarkCoDel_Decide-10                 24100654    49.75 ns/op    0 B/op    0 allocs/op
BenchmarkQueueAlgorithm_Enqueue-10        2313836   517.8 ns/op    0 B/op    0 allocs/op
BenchmarkQueueAlgorithm_Throughput-10     1485504   800.4 ns/op    0 B/op    0 allocs/op
```

**Performance comparison**:
- **Decision-based CoDel**: 49.75ns/op (~11.6x slower than Threshold 4.27ns)
- **Queue-based CoDel**: 517.8ns/op (enqueue) + 800.4ns/op (throughput)
- **Zero allocations**: Both variants achieve zero allocations via channel pooling

**Optimizations**:

*Decision-based CoDel*:
- Lock-free fast path for normal operation (atomic flag check)
- Pre-computed sqrt lookup table for drop counts 1-100 (avoids math.Sqrt)
- Integer arithmetic in mapToLevel() (avoids float division)
- Cached nanoseconds to avoid repeated time.Duration conversions
- Minimal lock contention - calculations done outside critical sections
- Zero heap allocations per request

*Queue-based CoDel*:
- Channel pooling via `sync.Pool` eliminates per-request allocations
- True sojourn time measurement (tracks actual time in queue)
- Buffered queue prevents blocking on enqueue
- Graceful degradation when queue is full (immediate rejection)

**When to use Decision-based CoDel**:
- Variable workloads with unpredictable traffic patterns
- Multi-tenant systems where different users have different latency needs
- Services without fixed SLAs where you want automatic adaptation
- Microservices that experience bursty traffic
- Systems prioritizing tail latency over throughput
- Integration with existing middleware (gRPC/HTTP interceptors)

**When to use Queue-based CoDel**:
- Services where you want to measure true sojourn time (time in queue)
- Rate limiting with adaptive queue management
- Protecting downstream services from overload
- Workloads where queueing delay is the primary concern
- Standalone request processors (not middleware-based)

**Pros**:
- Self-tuning - no manual threshold configuration needed
- Adapts to changing workload conditions
- Modern algorithm designed for bufferbloat control
- Works well with bursty traffic patterns
- Better tail latency under dynamic load
- Queue-based variant provides true sojourn time measurement

**Cons**:
- 11.6x slower than Threshold for decision-based (still only 50ns/op)
- Queue-based is 121x slower (518ns/op, but includes actual queuing)
- Less predictable behavior (adapts to conditions)
- Requires understanding of target delay and interval parameters
- Queue-based requires different integration pattern (not drop-in middleware)
- May be overly aggressive for workloads with natural latency variance

---

### 3. NoOp Algorithm (Testing)

**Location**: Core package (`floodgate.NoOpAlgorithm`)

**How it works**: Never rejects requests. Always returns `Normal` level.

**Configuration**:
```go
cfg.Algorithm = &floodgate.NoOpAlgorithm{}
```

**Performance**:
```
BenchmarkNoOpAlgorithm_Decide-10    1000000000    0.3768 ns/op    0 B/op    0 allocs/op
```

**When to use**:
- **Testing** and debugging
- **Temporarily disabling** backpressure without removing middleware
- **Load testing** to establish baselines
- **Gradual rollout** where you want observability without rejections

---

## Performance Summary

| Algorithm | ns/op | Relative Speed | Allocations | Use Case |
|-----------|-------|----------------|-------------|----------|
| NoOp | 0.32 | 1x (baseline) | 0 | Testing only |
| Threshold | 4.27 | 13.3x | 0 | Production default |
| CoDel (decision) | 49.75 | 155x | 0 | Adaptive workloads |
| CoDel (queue enqueue) | 517.8 | 1,618x | 0 | True sojourn tracking |
| CoDel (queue throughput) | 800.4 | 2,501x | 0 | End-to-end queueing |

**Key insights**:
- All algorithms have **zero heap allocations** per decision
- Decision-based CoDel is very fast at 49.75ns/op (11.6x slower than Threshold)
- Queue-based CoDel includes actual request queuing (517.8ns/op)
- At 1M req/s, algorithm overhead is:
  - Threshold: 0.0043s CPU time (0.4%)
  - CoDel (decision): 0.050s CPU time (5%)
  - CoDel (queue): 0.518s CPU time (51.8%)
- **Performance difference is negligible** for decision-based algorithms
- Queue-based CoDel overhead justified by true sojourn time measurement
- v1.5.0 optimizations: Threshold 4.27ns (was 5.55ns), CoDel 49.75ns (was 59.26ns)

## Configuration Examples

### HTTP Middleware with Threshold Algorithm

```go
import (
    "github.com/mushtruk/floodgate"
    httpMiddleware "github.com/mushtruk/floodgate/http"
)

cfg := httpMiddleware.DefaultConfig()

// Option 1: Use default thresholds (nil algorithm)
cfg.Algorithm = nil  // Uses ThresholdAlgorithm(DefaultThresholds())

// Option 2: Custom thresholds
cfg.Algorithm = floodgate.NewThresholdAlgorithm(floodgate.Thresholds{
    P99Emergency: 5 * time.Second,    // More aggressive
    P95Critical:  1 * time.Second,
    EMACritical:  200 * time.Millisecond,
    P95Moderate:  500 * time.Millisecond,
    EMAWarning:   100 * time.Millisecond,
    SlopeWarning: 5 * time.Millisecond,
})
```

### HTTP Middleware with CoDel Algorithm

```go
import (
    "github.com/mushtruk/floodgate/algorithms/codel"
    httpMiddleware "github.com/mushtruk/floodgate/http"
)

cfg := httpMiddleware.DefaultConfig()

// Low-latency API (aggressive)
cfg.Algorithm = codel.NewAlgorithm(
    codel.WithTargetDelay(2 * time.Millisecond),
    codel.WithInterval(50 * time.Millisecond),
)

// Standard API (balanced)
cfg.Algorithm = codel.NewAlgorithm(
    codel.WithTargetDelay(5 * time.Millisecond),
    codel.WithInterval(100 * time.Millisecond),
)

// Batch processing (lenient)
cfg.Algorithm = codel.NewAlgorithm(
    codel.WithTargetDelay(20 * time.Millisecond),
    codel.WithInterval(200 * time.Millisecond),
)
```

### gRPC Interceptor with CoDel Algorithm

```go
import (
    "github.com/mushtruk/floodgate/algorithms/codel"
    grpcInterceptor "github.com/mushtruk/floodgate/grpc"
)

cfg := grpcInterceptor.DefaultConfig()
cfg.Algorithm = codel.NewAlgorithm()

interceptor := grpcInterceptor.UnaryServerInterceptor(ctx, cfg)
```

## Algorithm Selection Guide

**Choose Threshold Algorithm if**:
- You have well-defined latency SLAs
- Your workload is relatively stable and predictable
- You want maximum performance (5ns vs 70ns)
- You prefer explicit, predictable behavior
- You're comfortable tuning thresholds

**Choose CoDel Algorithm if**:
- Your workload is highly variable or bursty
- You don't want to manually tune thresholds
- You're willing to trade 65ns for adaptability
- Tail latency is more important than raw throughput
- You operate a multi-tenant or shared infrastructure

**Choose NoOp Algorithm if**:
- You're testing or debugging
- You want metrics without rejections
- You're establishing performance baselines

## Algorithm Decorators (v1.5.0)

Enhance algorithms with composable wrappers for observability and resilience.

### Tracing Wrapper

Add distributed tracing to algorithm decisions:

```go
import "github.com/mushtruk/floodgate"

// Implement Tracer interface (OpenTelemetry, Jaeger, etc.)
type MyTracer struct {
    tracer trace.Tracer
}

func (t *MyTracer) StartSpan(ctx context.Context, name string) (context.Context, floodgate.Span) {
    ctx, span := t.tracer.Start(ctx, name)
    return ctx, &MySpan{span: span}
}

// Wrap algorithm with tracing
algo := codel.NewAlgorithm()
tracedAlgo := floodgate.WithTracing(algo, tracer)

// Decisions are now traced
decision := tracedAlgo.Decide(stats)
```

**Performance**: +100ns per decision (span creation overhead)

### Caching Wrapper

Cache algorithm decisions to reduce computation:

```go
import (
    "time"
    "github.com/mushtruk/floodgate"
)

// Cache decisions for 100ms
algo := codel.NewAlgorithm()
cachedAlgo := floodgate.NewCachedAlgorithm(algo, 100*time.Millisecond)

// First call: computes decision (50ns)
decision1 := cachedAlgo.Decide(stats)

// Subsequent calls within TTL: cached (~5ns)
decision2 := cachedAlgo.Decide(stats)

// Check cache stats
fmt.Printf("Hit rate: %.2f%%\n", cachedAlgo.HitRate())
```

**Performance**:
- Cache hit: +5ns overhead
- Cache miss: +10ns overhead + algorithm time
- Typical hit rate: 80-90% for stable traffic

**When to use**:
- High-frequency decision making (>10K/sec per method)
- Expensive custom algorithms
- Stats don't change rapidly

### Fallback Wrapper

Add panic recovery with fallback algorithm:

```go
import "github.com/mushtruk/floodgate"

// Primary algorithm (might panic on edge cases)
primary := &MyExperimentalAlgorithm{}

// Fallback to simple threshold on panic
fallback := floodgate.NewThresholdAlgorithm(floodgate.DefaultThresholds())

// Wrap with fallback protection
safeAlgo := floodgate.WithFallback(primary, fallback, logger)

// If primary panics, automatically uses fallback
decision := safeAlgo.Decide(stats)
```

**Performance**: +2ns defer overhead (only pays cost on panic)

**When to use**:
- Testing experimental algorithms in production
- Gradual rollout of new algorithms
- High-reliability systems requiring graceful degradation

### Composing Multiple Decorators

Stack decorators for comprehensive observability:

```go
import "github.com/mushtruk/floodgate"

// Start with base algorithm
algo := codel.NewAlgorithm()

// Add layers of functionality
algo = floodgate.WithTracing(algo, tracer)           // Distributed tracing
algo = floodgate.NewCachedAlgorithm(algo, 50*time.Millisecond) // Caching
algo = floodgate.WithFallback(algo, fallbackAlgo, logger)      // Panic recovery

// Or use the convenience function
algo = floodgate.NewInstrumentedAlgorithm(
    codel.NewAlgorithm(),
    tracer,
    logger,
    metrics,
)
```

**Order matters**:
1. **Fallback** (outermost) - Catches panics from everything
2. **Caching** (middle) - Caches traced decisions
3. **Tracing** (innermost) - Traces actual algorithm execution

**Performance**: Decorators add overhead only when used:
- Unwrapped algorithm: 50ns
- + Tracing: 150ns
- + Caching (hit): 155ns
- + Fallback: 157ns

## Custom Algorithms

You can implement custom algorithms by satisfying the `Algorithm` interface:

```go
type MyAlgorithm struct {
    // Your state here
}

func (a *MyAlgorithm) Decide(stats floodgate.Stats) floodgate.Decision {
    // Your decision logic here
    // stats contains: EMA, Slope, Drift, P50, P95, P99

    if stats.P99 > yourThreshold {
        return floodgate.Decision{
            Level:  floodgate.Emergency,
            Reject: true,
        }
    }

    return floodgate.Decision{
        Level:  floodgate.Normal,
        Reject: false,
    }
}

// Use it (with optional decorators)
algo := &MyAlgorithm{}
algo = floodgate.WithTracing(algo, tracer)
cfg.Algorithm = algo
```

## Benchmarking Your Workload

To determine which algorithm works best for your application:

```bash
# Run benchmarks
go test -bench=BenchmarkCoDel_vs_Threshold -benchmem

# Test with your traffic patterns
# - Run load tests with both algorithms
# - Compare P95, P99 latencies
# - Compare rejection rates
# - Measure CPU overhead
```

## References

- **CoDel Algorithm**: "Controlling Queue Delay" (Nichols & Jacobson, 2012)
- **Linux Kernel CoDel**: [Documentation](https://www.kernel.org/doc/html/latest/networking/codel.html)
- **Cloudflare Blog**: [Adaptive Backpressure](https://blog.cloudflare.com/)
