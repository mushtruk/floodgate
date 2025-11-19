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

**Configuration**:
```go
import "github.com/mushtruk/floodgate/algorithms/codel"

cfg.Algorithm = codel.NewAlgorithm(
    codel.WithTargetDelay(5 * time.Millisecond),  // Target queueing delay
    codel.WithInterval(100 * time.Millisecond),   // Persistence interval
)
```

**Performance**:
```
BenchmarkCoDel_Decide-10                 20244579    59.26 ns/op    0 B/op    0 allocs/op
BenchmarkCoDel_vs_Threshold/CoDel-10     20838925    59.25 ns/op    0 B/op    0 allocs/op
BenchmarkCoDel_vs_Threshold/Threshold-10 216883636    5.550 ns/op    0 B/op    0 allocs/op
```

**Performance comparison**: CoDel is **~10.7x slower** than Threshold (59ns vs 5.5ns), but still extremely fast in absolute terms. Zero allocations for both algorithms.

**Optimizations**: The CoDel implementation includes several performance optimizations:
- **Lock-free fast path** for normal operation (uses atomic flag check)
- **Pre-computed sqrt lookup table** for drop counts 1-100 (avoids math.Sqrt)
- **Integer arithmetic** in mapToLevel() (avoids float division)
- **Cached nanoseconds** to avoid repeated time.Duration conversions
- **Minimal lock contention** - calculations done outside critical sections
- **Zero heap allocations** per request

**When to use**:
- **Variable workloads** with unpredictable traffic patterns
- **Multi-tenant systems** where different users have different latency needs
- **Services without fixed SLAs** where you want automatic adaptation
- **Microservices** that experience bursty traffic
- **Systems prioritizing tail latency** over throughput

**Pros**:
- Self-tuning - no manual threshold configuration needed
- Adapts to changing workload conditions
- Modern algorithm designed for bufferbloat control
- Works well with bursty traffic patterns
- Better tail latency under dynamic load

**Cons**:
- 10.7x slower than Threshold (though still only 59ns/op)
- Less predictable behavior (adapts to conditions)
- Requires understanding of target delay and interval parameters
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
| NoOp | 0.38 | 1x (baseline) | 0 | Testing only |
| Threshold | 5.55 | 14.6x | 0 | Production default |
| CoDel | 59.26 | 156x | 0 | Adaptive workloads |

**Key insights**:
- All algorithms have **zero heap allocations** per decision
- Even the "slowest" algorithm (CoDel) is still very fast at 59ns/op
- At 1M req/s, algorithm overhead is:
  - Threshold: 0.0055s CPU time
  - CoDel: 0.059s CPU time
- **Performance difference is negligible** for most applications
- CoDel's optimizations bring it within **10.7x** of Threshold (down from initial 12x)

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

// Use it
cfg.Algorithm = &MyAlgorithm{}
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
