# Algorithm Selection Example

This example demonstrates how to select and configure different backpressure algorithms based on service characteristics and runtime configuration.

## Algorithms

### 1. Standard CoDel (`standard`)
- **Performance**: 49 ns/op
- **Best for**: CPU-bound services
- **Trade-off**: Uses P95 as sojourn time proxy (not true queue time)

### 2. Queue CoDel (`queue`)
- **Performance**: 600 ns/op
- **Best for**: I/O-bound services (databases, external APIs)
- **Trade-off**: Higher overhead, but true sojourn time measurement

### 3. Threshold (`threshold`)
- **Performance**: 5 ns/op
- **Best for**: Predictable behavior, multi-signal monitoring
- **Trade-off**: Requires manual threshold tuning

## Usage

### Environment-driven selection:

```bash
# Use Standard CoDel (CPU-bound)
BACKPRESSURE_ALGORITHM=standard go run main.go

# Use Queue CoDel (I/O-bound)
BACKPRESSURE_ALGORITHM=queue go run main.go

# Use Threshold (default, proven)
BACKPRESSURE_ALGORITHM=threshold go run main.go
```

### Auto-recommendation:

```bash
# Let the system recommend based on service profile
AUTO_SELECT=true CPU_BOUND=true go run main.go
AUTO_SELECT=true CPU_BOUND=false go run main.go
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

| Service Type | Avg Latency | Algorithm | Why |
|-------------|-------------|-----------|-----|
| API Gateway | < 10ms | `standard` | Low overhead |
| Database Proxy | 50-200ms | `queue` | True sojourn time matters |
| Compute Service | 10-50ms | `threshold` | Multi-signal safety |
| External API | > 200ms | `queue` | High variance |

## Configuration Examples

### Production (environment variables)
```bash
export BACKPRESSURE_ALGORITHM=queue
export BACKPRESSURE_TARGET_DELAY=5ms
export BACKPRESSURE_WORKERS=200
export BACKPRESSURE_QUEUE_SIZE=2000
```

### Staging (faster feedback)
```bash
export BACKPRESSURE_ALGORITHM=standard
export BACKPRESSURE_TARGET_DELAY=2ms
```

### Development (safe defaults)
```bash
export BACKPRESSURE_ALGORITHM=threshold
```
