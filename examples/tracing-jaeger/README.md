# Distributed Tracing with Jaeger Example

This example demonstrates OpenTelemetry distributed tracing integration with Jaeger, showing how backpressure events are captured and visualized in traces.

## Features

- **OpenTelemetry Integration**: Industry-standard distributed tracing
- **Jaeger Visualization**: Beautiful trace visualization UI
- **Backpressure Spans**: Dedicated spans for backpressure evaluation
- **Error Tracking**: Rejections marked as errors in traces
- **Latency Correlation**: P95/P99 metrics visible in trace attributes
- **Cascade Debugging**: Track failures across service boundaries

## Prerequisites

### Run Jaeger

Using Docker:
```bash
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4318:4318 \
  jaegertracing/all-in-one:latest
```

Using Docker Compose (see `docker-compose.yml` in this directory):
```bash
docker-compose up -d
```

Access Jaeger UI: http://localhost:16686

## Running the Example

```bash
cd examples/tracing-jaeger
go run main.go
```

The server starts on `http://localhost:8080` with endpoints:
- `GET /` - Info page with instructions
- `GET /api/fast` - Fast endpoint (1-5ms)
- `GET /api/slow` - Slow endpoint (100-300ms, triggers backpressure)
- `GET /api/cascade` - Demonstrates cascading service calls

## Testing Backpressure Tracing

### Generate Normal Load
```bash
for i in {1..100}; do curl http://localhost:8080/api/fast & done
```

### Trigger Backpressure
```bash
for i in {1..50}; do curl http://localhost:8080/api/slow & done
```

### Cascade Scenario
```bash
for i in {1..30}; do curl http://localhost:8080/api/cascade & done
```

## What to Look For in Jaeger

### 1. Service Overview
- Navigate to Jaeger UI: http://localhost:16686
- Select service: **floodgate-demo**
- Click "Find Traces"

### 2. Backpressure Spans
Look for spans named `floodgate.backpressure`:
- **Green spans**: Normal operation (request allowed)
- **Red spans**: Backpressure rejection (request denied)

### 3. Trace Attributes
Each backpressure span includes:
```
backpressure.level: Normal | Warning | Moderate | Critical | Emergency
backpressure.ema: 0.045 (45ms exponential moving average)
backpressure.p50: 0.023 (23ms median latency)
backpressure.p95: 0.187 (187ms 95th percentile)
backpressure.p99: 0.289 (289ms 99th percentile)
backpressure.slope: 0.012 (12ms slope)
backpressure.rejected: true | false
```

### 4. Circuit Breaker State
```
circuit_breaker.state: closed | open | half_open
circuit_breaker.rejected: true | false
```

### 5. Error Tracking
Rejected requests are marked as errors with:
- Error status: `backpressure rejected at level: Critical`
- Error event recorded in span timeline

## Jaeger Query Examples

### Find All Rejections
```
Service: floodgate-demo
Tags: backpressure.rejected=true
```

### Find Critical Backpressure
```
Service: floodgate-demo
Tags: backpressure.level=Critical
```

### Find High Latency Requests
```
Service: floodgate-demo
Min Duration: 200ms
```

### Circuit Breaker Opens
```
Service: floodgate-demo
Tags: circuit_breaker.state=open
```

## Trace Topology

A typical trace with backpressure:

```
HTTP Request (200ms total)
├─ floodgate.backpressure (1ms)
│  └─ Attributes: level=Warning, p95=150ms, rejected=false
├─ handle_slow_request (180ms)
│  └─ database_query (150ms)
└─ response_serialization (19ms)
```

A rejected request trace:

```
HTTP Request (1ms total)
└─ floodgate.backpressure (1ms) [ERROR]
   └─ Attributes: level=Critical, p95=280ms, rejected=true
   └─ Error: "backpressure rejected at level: Critical"
```

## Cascading Failures

The `/api/cascade` endpoint demonstrates how backpressure propagates:

```
Frontend Service (300ms)
├─ floodgate.backpressure (1ms) - level=Normal
├─ call_api_service (250ms)
│  ├─ API Service: floodgate.backpressure (1ms) - level=Warning
│  └─ call_downstream_service (200ms)
│     ├─ Downstream Service: floodgate.backpressure (1ms) [ERROR]
│     │  └─ level=Critical, rejected=true
│     └─ Error: cascading failure
└─ Error handling (49ms)
```

This visualizes how a slow downstream service triggers backpressure up the chain.

## Advanced Queries

### Latency Distribution by Endpoint
Group by: `http.route`
Metric: P95 duration

### Rejection Rate Over Time
Count traces with: `backpressure.rejected=true`
Time series graph

### Circuit Breaker Events
Timeline view of `circuit_breaker.state` changes

## Integration with APM

### Datadog APM
Jaeger traces can be exported to Datadog:
```go
// Add Datadog exporter
import "go.opentelemetry.io/otel/exporters/datadog"

exporter, _ := datadog.NewExporter(
    datadog.WithAgentAddr("localhost:8126"),
)
```

### Correlating with Metrics
- Traces show **individual requests**
- Metrics show **aggregate behavior**
- Use trace IDs in logs to correlate all three

Example: High P99 latency in metrics → Find traces with >200ms → Identify slow queries

## Production Considerations

### Sampling
For high-throughput services, use sampling:
```go
tp := sdktrace.NewTracerProvider(
    sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)), // 10% sampling
)
```

### Performance Impact
- Trace creation: ~10-50µs per span
- Backpressure span: +1-2µs additional overhead
- Negligible compared to request processing time

### Storage
- Jaeger stores traces in-memory (all-in-one)
- For production, use Elasticsearch, Cassandra, or Kafka backend
- Set retention policies (default: 72 hours)

## Troubleshooting

### No Traces Appearing

1. **Check Jaeger is running**:
   ```bash
   curl http://localhost:16686/api/services
   ```

2. **Verify OTLP endpoint**:
   ```bash
   curl http://localhost:4318/v1/traces -X POST
   ```

3. **Check application logs**:
   Look for "Failed to create exporter" errors

### Spans Missing Attributes

Ensure the tracer is properly initialized before middleware:
```go
tp, _ := initTracer()  // Must be called first
defer tp.Shutdown(ctx)

otel.SetTracerProvider(tp)  // Set global provider
```

## Further Reading

- [OpenTelemetry Tracing Specification](https://opentelemetry.io/docs/specs/otel/trace/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
- [Distributed Tracing Best Practices](https://opentelemetry.io/docs/instrumentation/go/manual/)

## Example Docker Compose

```yaml
version: '3'
services:
  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "16686:16686"  # Jaeger UI
      - "4318:4318"    # OTLP HTTP
    environment:
      - COLLECTOR_OTLP_ENABLED=true
```
