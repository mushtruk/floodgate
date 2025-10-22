# Memory Configuration Guide

## Default Configuration (Recommended)

**200 percentile samples** = ~3.2 KB per method

This is the **sweet spot** for most services:
- ✅ Accurate P95/P99 percentiles (±0.5% error)
- ✅ Low memory footprint
- ✅ Fast percentile calculation
- ✅ Suitable for services with 100+ methods

**Typical usage**:
- 50 methods: ~160 KB
- 100 methods: ~320 KB
- 512 methods (max cache): ~1.6 MB

---

## When to Adjust Sample Size

### Use **100 samples** (~1.6 KB) if:

- Running in memory-constrained environments (containers with <512 MB RAM)
- Service has 500+ unique methods
- Percentile accuracy isn't critical (±1% error is acceptable)

```go
tracker := floodgate.NewTracker(
    floodgate.WithAlpha(0.1),
    floodgate.WithWindowSize(50),
    floodgate.WithPercentiles(100), // Minimal memory
)
```

**Trade-offs**:
- ✅ 50% less memory
- ⚠️ Slightly less accurate percentiles (still excellent)
- ✅ Still benefits from lazy caching

---

### Use **500 samples** (~8 KB) if:

- Need high percentile accuracy for SLO monitoring
- Service has <50 unique methods
- Memory is not a constraint

```go
tracker := floodgate.NewTracker(
    floodgate.WithAlpha(0.1),
    floodgate.WithWindowSize(50),
    floodgate.WithPercentiles(500), // Higher precision
)
```

**Trade-offs**:
- ⚠️ 2.5x more memory
- ✅ Very accurate percentiles (±0.2% error)
- ✅ Still benefits from lazy caching

---

### Use **1000 samples** (~16 KB) **ONLY if**:

- Critical production service with strict SLOs
- Very few methods (<10)
- Need extremely accurate P99.9 tracking

```go
tracker := floodgate.NewTracker(
    floodgate.WithAlpha(0.1),
    floodgate.WithWindowSize(50),
    floodgate.WithPercentiles(1000), // Maximum precision
)
```

**Trade-offs**:
- ⚠️ 5x more memory than default
- ✅ Highest precision percentiles
- ⚠️ Not recommended for most use cases

---

## Memory Calculation Formula

**Memory per tracker** = `(samples × 2 × 8 bytes) + (windowSize × 8 bytes) + 200 bytes`

Where:
- `samples × 2`: samples array + sortBuffer
- `windowSize`: EMA trend analysis window (default 50)
- 200 bytes: struct overhead

### Examples

| Samples | Window | Memory per Tracker | 512 Methods Total |
|---------|--------|-------------------|-------------------|
| 100 | 50 | ~1.6 KB | ~800 KB |
| **200** | **50** | **~3.2 KB** | **~1.6 MB** ⭐ Recommended |
| 500 | 50 | ~8 KB | ~4 MB |
| 1000 | 50 | ~16 KB | ~8 MB |

---

## Percentile Accuracy vs Sample Size

| Sample Size | P95 Error | P99 Error | Suitable For |
|-------------|-----------|-----------|--------------|
| 100 | ±1.0% | ±2.0% | Non-critical services |
| **200** | **±0.5%** | **±1.0%** | **Most services** ⭐ |
| 500 | ±0.2% | ±0.5% | SLO monitoring |
| 1000 | ±0.1% | ±0.2% | Critical services only |

**Note**: With lazy caching, larger sample sizes don't significantly impact performance (all are ~35ns).

---

## gRPC Interceptor Configuration

### Default (Balanced)

```go
cfg := grpc.DefaultConfig()
// Uses 200 samples per method = ~3.2 KB each
// 512 method cache = ~1.6 MB total
```

### Memory-Constrained

```go
cfg := grpc.DefaultConfig()
cfg.CacheSize = 128 // Reduce cache size

// In interceptor, create trackers with:
floodgate.WithPercentiles(100) // Minimal: ~1.6 KB per method
// 128 methods × 1.6 KB = ~200 KB total
```

### High-Precision

```go
cfg := grpc.DefaultConfig()
cfg.CacheSize = 64 // Fewer methods, more precision

// In interceptor, create trackers with:
floodgate.WithPercentiles(500) // ~8 KB per method
// 64 methods × 8 KB = ~512 KB total
```

---

## Without Percentiles

If you don't need P95/P99 tracking, disable percentiles entirely:

```go
tracker := floodgate.NewTracker(
    floodgate.WithAlpha(0.1),
    floodgate.WithWindowSize(50),
    // No WithPercentiles() call
)
```

**Memory**: ~600 bytes per tracker (97% reduction!)

**Trade-offs**:
- ✅ Minimal memory (~300 KB for 512 methods)
- ⚠️ No P95/P99 percentiles
- ✅ Still have EMA, slope, drift for basic backpressure
- ✅ Falls back to slope-based level detection

---

## Recommendations by Service Size

### Small Service (<20 methods)

```go
floodgate.WithPercentiles(500) // ~8 KB per method
// Total: ~160 KB (excellent precision, negligible memory)
```

### Medium Service (20-100 methods)

```go
floodgate.WithPercentiles(200) // ~3.2 KB per method ⭐ Default
// Total: ~320 KB to 1.6 MB (balanced)
```

### Large Service (100+ methods)

```go
floodgate.WithPercentiles(100) // ~1.6 KB per method
// Or reduce CacheSize to 256 with 200 samples
// Total: ~800 KB (optimized for many methods)
```

### Massive Service (500+ methods)

```go
// Option 1: No percentiles (minimal memory)
floodgate.NewTracker(
    floodgate.WithAlpha(0.1),
    floodgate.WithWindowSize(50),
)

// Option 2: Small cache + minimal samples
cfg.CacheSize = 128
floodgate.WithPercentiles(100)
```

---

## Monitoring Memory Usage

To monitor actual memory usage:

```go
import _ "net/http/pprof"

go func() {
    http.ListenAndServe("localhost:6060", nil)
}()

// Then: go tool pprof http://localhost:6060/debug/pprof/heap
```

Look for `emaTracker` allocations to see actual memory usage.

---

## Summary

**For 95% of services**: Use the **default 200 samples**
- ~3.2 KB per method
- ~1.6 MB for 512 methods
- Excellent accuracy
- Negligible performance impact

**Only optimize if**:
- You have 500+ methods → reduce to 100 samples or smaller cache
- Memory is extremely constrained → disable percentiles
- Need extreme precision → increase to 500 samples (not 1000!)

The default configuration is carefully tuned for the best balance of accuracy, memory, and performance.
