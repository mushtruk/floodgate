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

### 2. NoOp Algorithm (Testing)

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

**Key insights**:
- All algorithms have **zero heap allocations** per decision
- At 1M req/s, Threshold costs 0.0043s CPU time (0.4%)

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

## Algorithm Selection Guide

**Choose Threshold Algorithm if**:
- You have well-defined latency SLAs
- Your workload is relatively stable and predictable
- You want maximum performance (~5ns per decision)
- You prefer explicit, predictable behavior
- You're comfortable tuning thresholds

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
algo := floodgate.NewThresholdAlgorithm(floodgate.DefaultThresholds())
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
algo := floodgate.NewThresholdAlgorithm(floodgate.DefaultThresholds())
cachedAlgo := floodgate.NewCachedAlgorithm(algo, 100*time.Millisecond)

// First call: computes decision
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
base := floodgate.NewThresholdAlgorithm(floodgate.DefaultThresholds())

// Add layers of functionality
var algo floodgate.Algorithm = floodgate.WithTracing(base, tracer) // Distributed tracing
algo = floodgate.NewCachedAlgorithm(algo, 50*time.Millisecond)     // Caching
algo = floodgate.WithFallback(algo, fallbackAlgo, logger)          // Panic recovery

// Or use the convenience function
algo = floodgate.NewInstrumentedAlgorithm(
    base,
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
- Tracing: +100ns
- Caching (hit): +5ns
- Fallback: +2ns

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
go test -bench=Algorithm -benchmem

# Test with your traffic patterns
# - Run load tests with each candidate algorithm or threshold set
# - Compare P95, P99 latencies
# - Compare rejection rates
# - Measure CPU overhead
```

## References

- **Cloudflare Blog**: [Adaptive Backpressure](https://blog.cloudflare.com/)
