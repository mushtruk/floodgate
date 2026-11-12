# Algorithm Selection Example

This example demonstrates how to select and configure a backpressure algorithm from a service profile given at runtime.

## Profiles

`SERVICE_PROFILE` picks the algorithm:

### 1. `default` (or unset)
- **Algorithm**: `floodgate.NewThresholdAlgorithm(floodgate.DefaultThresholds())`
- **Best for**: Most services; predictable, multi-signal behavior
- **Trade-off**: Thresholds suit typical web latencies, not every workload

### 2. `latency-sensitive`
- **Algorithm**: Threshold with tighter thresholds
- **Best for**: Low-latency APIs that should shed load early
- **Trade-off**: Rejects sooner under transient spikes

### 3. `disabled`
- **Algorithm**: `floodgate.NoOpAlgorithm`
- **Best for**: Debugging or temporarily turning backpressure off
- **Trade-off**: No load shedding at all

Any other value stops the program with an error naming the valid profiles.

## Usage

```bash
# Default thresholds
go run main.go

# Tight thresholds
SERVICE_PROFILE=latency-sensitive go run main.go

# Backpressure off
SERVICE_PROFILE=disabled go run main.go
```

## Testing

```bash
# Start server
go run main.go

# In another terminal, test endpoints
curl http://localhost:8080/fast
curl http://localhost:8080/slow
curl http://localhost:8080/health

# Load test to trigger backpressure
hey -z 30s -c 100 http://localhost:8080/slow
```

## Decision Matrix

| Service Type | Avg Latency | Profile | Why |
|-------------|-------------|---------|-----|
| API Gateway | < 10ms | `latency-sensitive` | Shed load before latency compounds |
| Compute Service | 10-50ms | `default` | Multi-signal safety |
| Local Debugging | any | `disabled` | No rejections while investigating |
