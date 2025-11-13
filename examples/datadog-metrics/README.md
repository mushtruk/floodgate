# Datadog Metrics Example

This example demonstrates how to integrate Datadog metrics with floodgate HTTP middleware.

## Features

- **Datadog Integration**: Sends floodgate metrics via DogStatsD protocol
- **Custom Namespaces**: Prefix metrics with your application name
- **Global Tags**: Add environment, service, and version tags
- **Real-time Monitoring**: Live metrics in Datadog dashboards
- **APM Integration**: Correlate with traces and infrastructure metrics

## Prerequisites

### Local Development
Install and run the Datadog agent locally:

```bash
# macOS
brew install datadog/agent/datadog-agent

# Start the agent
datadog-agent run

# Verify DogStatsD is listening on port 8125
datadog-agent status
```

### Docker
Run the Datadog agent in Docker:

```bash
docker run -d --name datadog-agent \
  -e DD_API_KEY=<your_api_key> \
  -e DD_SITE=datadoghq.com \
  -e DD_DOGSTATSD_NON_LOCAL_TRAFFIC=true \
  -p 8125:8125/udp \
  -p 8126:8126/tcp \
  gcr.io/datadoghq/agent:latest
```

### Kubernetes
Deploy via Datadog Operator or Helm chart - DogStatsD is enabled by default.

## Running the Example

```bash
cd examples/datadog-metrics
go run main.go
```

The server starts on `http://localhost:8080` with the following endpoints:

- `GET /` - Info page with usage instructions
- `GET /api/fast` - Fast endpoint (1-5ms latency)
- `GET /api/slow` - Slow endpoint (100-300ms latency, triggers backpressure)
- `GET /health` - Health check (skipped by backpressure middleware)

## Testing Backpressure

### Generate normal load:
```bash
for i in {1..100}; do curl http://localhost:8080/api/fast & done
```

### Trigger backpressure:
```bash
for i in {1..50}; do curl http://localhost:8080/api/slow & done
```

## Available Metrics

All metrics are prefixed with the namespace: `myapp.floodgate.*`

### Request Metrics
- `myapp.floodgate.requests.total` - Total requests (tags: method, level, result)
- `myapp.floodgate.requests.rejected` - Rejected requests (tags: method, level)
- `myapp.floodgate.request.duration` - Request latency timing (tags: method)

### Circuit Breaker
- `myapp.floodgate.circuit_breaker.state` - Circuit breaker state (tags: method, state)
  - Values: 0=closed, 1=open, 2=half_open

### Cache & Dispatcher
- `myapp.floodgate.cache.size` - Active tracker count
- `myapp.floodgate.dispatcher.drops` - Dropped events (delta)
- `myapp.floodgate.dispatcher.events` - Processed events (delta)
- `myapp.floodgate.dispatcher.drops.total` - Total dropped (absolute)
- `myapp.floodgate.dispatcher.events.total` - Total events (absolute)

## Global Tags

All metrics include these tags:
- `env:dev`
- `service:api`
- `version:1.3.0`

## Datadog Dashboard Queries

### Request Rate by Backpressure Level
```
sum:myapp.floodgate.requests.total{*} by {level}.as_rate()
```

### Rejection Rate
```
sum:myapp.floodgate.requests.rejected{*}.as_rate()
```

### P95 Latency by Method
```
p95:myapp.floodgate.request.duration{*} by {method}
```

### Circuit Breaker Open Count
```
sum:myapp.floodgate.circuit_breaker.state{state:open}
```

### Dispatcher Drop Rate
```
sum:myapp.floodgate.dispatcher.drops{*}.as_rate() / sum:myapp.floodgate.dispatcher.events{*}.as_rate()
```

## Creating Alerts

### High Rejection Rate Alert
```
Alert when: avg(last_5m):sum:myapp.floodgate.requests.rejected{*}.as_rate() > 10
Warning threshold: 5 requests/sec
Critical threshold: 10 requests/sec
```

### Circuit Breaker Open Alert
```
Alert when: sum:myapp.floodgate.circuit_breaker.state{state:open} > 0
Critical threshold: Any circuit open
```

### High Latency Alert
```
Alert when: p95:myapp.floodgate.request.duration{*} > 500ms
Warning threshold: 300ms
Critical threshold: 500ms
```

## Dashboard Creation

1. Navigate to Datadog → Dashboards → New Dashboard
2. Add widgets for:
   - **Timeseries**: Request rate by level
   - **Query Value**: Current rejection rate
   - **Heatmap**: Latency distribution
   - **Toplist**: Endpoints by request count
   - **Change**: Circuit breaker state changes
3. Save and share with your team

## APM Integration

To correlate metrics with APM traces:

```go
import (
    "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// Start tracer
tracer.Start(
    tracer.WithService("api"),
    tracer.WithEnv("dev"),
)
defer tracer.Stop()

// Traces will automatically include env:dev and service:api tags
// matching your metrics for easy correlation
```

## Environment Variables

- `DD_AGENT_HOST` - Datadog agent address (default: `localhost:8125`)
- `DD_API_KEY` - API key for Datadog agent (only needed if agent requires it)
- `DD_SITE` - Datadog site (e.g., `datadoghq.com`, `datadoghq.eu`)

## Production Considerations

### Agent Configuration
Ensure DogStatsD is enabled in your agent config:

```yaml
# datadog.yaml
dogstatsd_enabled: true
dogstatsd_port: 8125
dogstatsd_non_local_traffic: false  # Set true for Docker/K8s
```

### Network Performance
- DogStatsD uses UDP (fire-and-forget)
- Minimal overhead (~50-100µs per metric)
- No blocking on application side
- Agent buffers metrics before sending to Datadog

### Tag Cardinality
Be mindful of tag cardinality:
- ✅ Good: `method:/api/users/:id` (normalized)
- ❌ Bad: `method:/api/users/12345` (high cardinality)

### Sampling
For high-throughput services, use sampling:

```go
// Sample 10% of requests
client, _ := statsd.New("localhost:8125",
    statsd.WithDevMode(false),
    statsd.WithMaxMessagesPerPayload(512),
)
```

## Troubleshooting

### Metrics not appearing in Datadog

1. **Check agent status**:
   ```bash
   datadog-agent status
   ```

2. **Verify DogStatsD is listening**:
   ```bash
   netstat -an | grep 8125
   ```

3. **Check agent logs**:
   ```bash
   tail -f /var/log/datadog/agent.log
   ```

4. **Test connectivity**:
   ```bash
   echo "custom_metric:1|c" | nc -u -w1 localhost 8125
   ```

### High metric volume

If you're sending too many metrics:
- Increase agent buffer size in `datadog.yaml`
- Reduce metrics interval in floodgate config
- Use metric aggregation in the agent

## Further Reading

- [Datadog DogStatsD Documentation](https://docs.datadoghq.com/developers/dogstatsd/)
- [Datadog Metrics Best Practices](https://docs.datadoghq.com/metrics/)
- [Datadog APM Integration](https://docs.datadoghq.com/tracing/)
