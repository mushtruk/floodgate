# CoDel HTTP Middleware Example

This example demonstrates using the CoDel (Controlled Delay) algorithm with Floodgate's HTTP middleware.

## What is CoDel?

CoDel is an adaptive queue management algorithm that:
- Monitors **sojourn time** (queueing delay) instead of queue length
- Automatically adapts to varying network conditions
- Uses a control law to increase drop frequency when delay persists
- Designed to combat bufferbloat while maintaining high throughput

Unlike the default threshold-based algorithm, CoDel doesn't require manual tuning of latency thresholds.

## Running the Example

```bash
cd examples/codel-http
go run main.go
```

The server starts on `:8080` with four endpoints:

- `GET /health` - Health check (skipped by middleware)
- `GET /fast` - Fast endpoint (~1ms latency)
- `GET /variable` - Variable latency (1-50ms, simulates bursty traffic)
- `GET /slow` - Slow endpoint (~20ms, triggers backpressure)

## Testing with Load

Install `hey` for load testing:
```bash
go install github.com/rakyll/hey@latest
```

### Test 1: Fast Endpoint (No Backpressure)

```bash
hey -n 10000 -c 50 http://localhost:8080/fast
```

**Expected**: No rejections. Latency stays low (~1ms), below CoDel's 5ms target.

### Test 2: Variable Endpoint (Adaptive Behavior)

```bash
hey -n 10000 -c 100 http://localhost:8080/variable
```

**Expected**: CoDel may trigger backpressure under high concurrency when queueing delay exceeds 5ms persistently. Watch server logs for "backpressure detected" messages.

### Test 3: Slow Endpoint (Backpressure Triggered)

```bash
hey -n 10000 -c 100 http://localhost:8080/slow
```

**Expected**:
- CoDel detects persistent delay (20ms >> 5ms target)
- Enters dropping mode after 100ms interval
- Drops requests at increasing frequency using control law
- Responses include `503 Service Unavailable` with `Retry-After` headers

## Configuration

```go
algo, err := codel.NewAlgorithm(
    codel.WithTargetDelay(5*time.Millisecond),  // Target queueing delay
    codel.WithInterval(100*time.Millisecond),   // Persistence check interval
)
if err != nil {
    log.Fatal(err)
}
cfg.Algorithm = algo
```

**Parameters**:

- **TargetDelay** (default: 5ms): Acceptable queueing delay. Lower = more aggressive backpressure.
  - Low-latency APIs: 2-5ms
  - Standard APIs: 5-10ms
  - Batch processing: 20-50ms

- **Interval** (default: 100ms): How long delay must persist before dropping starts. Prevents reaction to transient spikes.

## Observing CoDel Behavior

Watch server logs for:

```
[floodgate] WARN: backpressure detected level=moderate method=GET /slow ema=15ms p95=18ms p99=20ms
[floodgate] ERROR: backpressure rejection route=GET /slow level=emergency ema=22ms p95=25ms p99=28ms
```

CoDel will:
1. Detect when P95 latency exceeds target (5ms)
2. Wait for persistence interval (100ms) to confirm it's not transient
3. Enter dropping mode and start rejecting requests
4. Increase drop frequency using `interval / sqrt(count)` control law
5. Exit dropping mode when latency returns below target

## Comparing with Threshold Algorithm

To compare with the default threshold-based algorithm, comment out the CoDel configuration:

```go
// cfg.Algorithm = codel.NewAlgorithm(...)  // Disabled
cfg.Algorithm = nil  // Use default ThresholdAlgorithm
```

**Key Differences**:

| Aspect | Threshold Algorithm | CoDel Algorithm |
|--------|---------------------|-----------------|
| Configuration | Requires manual threshold tuning | Self-tuning, minimal config |
| Adaptability | Fixed thresholds | Adapts to conditions |
| Performance | ~5.5 ns/op | ~59 ns/op |
| Best for | Predictable workloads | Variable/bursty workloads |
| Tail latency | Good with proper tuning | Excellent, automatic |

## Performance Impact

CoDel algorithm overhead:
- **59 ns/op** per request decision
- **Zero heap allocations**
- At 1M req/s: ~59ms total CPU time

The performance difference vs threshold algorithm (5.5ns vs 59ns) is negligible for most applications.

## References

- [CoDel Paper](https://queue.acm.org/detail.cfm?id=2209336) - "Controlling Queue Delay" by Nichols & Jacobson
- [Linux Kernel CoDel](https://www.kernel.org/doc/html/latest/networking/codel.html)
- [Algorithm Documentation](../../ALGORITHMS.md)
