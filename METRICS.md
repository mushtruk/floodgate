# Metrics Integration Guide

Floodgate provides a pluggable metrics interface that allows you to integrate with any metrics backend (Prometheus, OpenTelemetry, StatsD, Datadog, etc.). This guide covers how to configure, use, and customize metrics collection.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Available Metrics](#available-metrics)
- [Prometheus Integration](#prometheus-integration)
- [OpenTelemetry Integration](#opentelemetry-integration)
- [Custom Metrics Collector](#custom-metrics-collector)
- [Grafana Dashboards](#grafana-dashboards)
- [Best Practices](#best-practices)

## Overview

Floodgate follows a **pluggable metrics pattern** similar to its logger interface. You can:

- **Choose your metrics backend**: Prometheus, OpenTelemetry, StatsD, Datadog, or custom
- **Zero overhead when disabled**: NoOpMetrics implementation has no runtime cost
- **Vendor-neutral**: Interface-based design works with any metrics system
- **Production-ready**: Built-in implementations for popular backends

### Architecture

```
┌─────────────────────┐
│  gRPC/HTTP Middleware │
└──────────┬──────────┘
           │
           ▼
┌──────────────────────┐
│ MetricsCollector     │ ◄── Interface
│ Interface            │
└──────────┬───────────┘
           │
    ┌──────┴──────┬──────────┬────────────┐
    │             │          │            │
    ▼             ▼          ▼            ▼
┌─────────┐ ┌──────────┐ ┌────────┐ ┌─────────┐
│NoOpMetrics│ │Prometheus│ │OpenTel │ │ Custom  │
└─────────┘ └──────────┘ └────────┘ └─────────┘
```

## Quick Start

### Default (Metrics Disabled)

By default, metrics are disabled using `NoOpMetrics` for zero overhead:

```go
cfg := bphttp.DefaultConfig()
// cfg.Metrics = &floodgate.NoOpMetrics{} // This is the default
```

### Enable Prometheus Metrics

```go
import (
    "github.com/mushtruk/floodgate"
    bphttp "github.com/mushtruk/floodgate/http"
    prommetrics "github.com/mushtruk/floodgate/metrics/prometheus"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// Create Prometheus registry
reg := prometheus.NewRegistry()

// Create floodgate metrics collector
metrics := prommetrics.NewMetrics(reg)

// Configure backpressure with metrics
cfg := bphttp.DefaultConfig()
cfg.Metrics = metrics

// Create middleware
handler := bphttp.Middleware(ctx, cfg)(mux)

// Expose metrics endpoint
http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
```

### Enable OpenTelemetry Metrics

```go
import (
    "github.com/mushtruk/floodgate"
    bpgrpc "github.com/mushtruk/floodgate/grpc"
    otelmetrics "github.com/mushtruk/floodgate/metrics/opentelemetry"
    "go.opentelemetry.io/otel/metric"
)

// Create OpenTelemetry meter
meter := otel.Meter("floodgate")

// Create floodgate metrics collector
metrics := otelmetrics.NewMetrics(meter)

// Configure backpressure with metrics
cfg := bpgrpc.DefaultConfig()
cfg.Metrics = metrics

// Create interceptor
interceptor := bpgrpc.UnaryServerInterceptor(ctx, cfg)
```

## Available Metrics

Floodgate exposes the following metrics:

### Request Metrics

#### `floodgate_requests_total`
- **Type**: Counter
- **Labels**: `method`, `level`, `result`
- **Description**: Total number of requests processed
- **Values**:
  - `level`: Normal, Warning, Moderate, Critical, Emergency
  - `result`: success, rejected, error

#### `floodgate_requests_rejected_total`
- **Type**: Counter
- **Labels**: `method`, `level`
- **Description**: Total number of requests rejected due to backpressure
- **Use**: Calculate rejection rate, alert on sustained rejections

#### `floodgate_request_duration_seconds`
- **Type**: Histogram
- **Labels**: `method`
- **Description**: Request latency distribution in seconds
- **Buckets**: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s, 30s
- **Use**: Calculate P50/P95/P99 percentiles, detect latency spikes

### Circuit Breaker Metrics

#### `floodgate_circuit_breaker_state`
- **Type**: Gauge
- **Labels**: `method`
- **Description**: Current circuit breaker state
- **Values**:
  - `0` = Closed (normal operation)
  - `1` = Open (rejecting all requests)
  - `2` = Half-Open (testing recovery)
- **Use**: Alert when circuit opens, track recovery times

### Cache Metrics

#### `floodgate_cache_size`
- **Type**: Gauge
- **Labels**: None
- **Description**: Number of active method/route trackers in LRU cache
- **Use**: Monitor memory usage, detect tracker thrashing

### Dispatcher Metrics

#### `floodgate_dispatcher_drops_total`
- **Type**: Counter
- **Labels**: None
- **Description**: Total events dropped by async dispatcher due to buffer overflow
- **Use**: Detect backlog buildup, tune buffer size

#### `floodgate_dispatcher_events_total`
- **Type**: Counter
- **Labels**: None
- **Description**: Total events emitted to async dispatcher
- **Use**: Calculate drop rate: `drops / events`

## Prometheus Integration

### Installation

```bash
go get github.com/mushtruk/floodgate/metrics/prometheus
```

### Full Example

```go
package main

import (
    "context"
    "net/http"
    "time"

    "github.com/mushtruk/floodgate"
    bphttp "github.com/mushtruk/floodgate/http"
    prommetrics "github.com/mushtruk/floodgate/metrics/prometheus"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    ctx := context.Background()

    // Create isolated Prometheus registry
    reg := prometheus.NewRegistry()

    // Create floodgate metrics collector
    metrics := prommetrics.NewMetrics(reg)

    // Configure backpressure
    cfg := bphttp.DefaultConfig()
    cfg.Metrics = metrics
    cfg.Thresholds = floodgate.Thresholds{
        P99Emergency: 500 * time.Millisecond,
        P95Critical:  200 * time.Millisecond,
        EMACritical:  100 * time.Millisecond,
    }

    // Create HTTP server
    mux := http.NewServeMux()
    mux.HandleFunc("/api/users", handleUsers)
    mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

    // Wrap with backpressure middleware
    handler := bphttp.Middleware(ctx, cfg)(mux)

    http.ListenAndServe(":8080", handler)
}
```

### Prometheus Queries

**Request rate by backpressure level:**
```promql
rate(floodgate_requests_total[1m])
```

**Rejection rate:**
```promql
rate(floodgate_requests_rejected_total[1m])
```

**P95 latency:**
```promql
histogram_quantile(0.95, rate(floodgate_request_duration_seconds_bucket[5m]))
```

**Circuit breaker open endpoints:**
```promql
floodgate_circuit_breaker_state == 1
```

**Dispatcher drop rate:**
```promql
rate(floodgate_dispatcher_drops_total[1m]) / rate(floodgate_dispatcher_events_total[1m])
```

### Alerting Rules

```yaml
groups:
  - name: floodgate
    interval: 30s
    rules:
      # Alert on sustained high rejection rate
      - alert: FloodgateHighRejectionRate
        expr: |
          rate(floodgate_requests_rejected_total[5m]) > 10
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High backpressure rejection rate"
          description: "{{ $labels.method }} is rejecting {{ $value }} req/s"

      # Alert when circuit breaker opens
      - alert: FloodgateCircuitBreakerOpen
        expr: |
          floodgate_circuit_breaker_state == 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Circuit breaker open"
          description: "{{ $labels.method }} circuit breaker is open"

      # Alert on high dispatcher drop rate
      - alert: FloodgateHighDropRate
        expr: |
          rate(floodgate_dispatcher_drops_total[5m]) /
          rate(floodgate_dispatcher_events_total[5m]) > 0.05
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High dispatcher drop rate"
          description: "Dropping {{ $value | humanizePercentage }} of events"
```

## OpenTelemetry Integration

### Installation

```bash
go get github.com/mushtruk/floodgate/metrics/opentelemetry
```

### Full Example

```go
package main

import (
    "context"

    "github.com/mushtruk/floodgate"
    bpgrpc "github.com/mushtruk/floodgate/grpc"
    otelmetrics "github.com/mushtruk/floodgate/metrics/opentelemetry"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/prometheus"
    "go.opentelemetry.io/otel/sdk/metric"
    "google.golang.org/grpc"
)

func main() {
    ctx := context.Background()

    // Create OpenTelemetry Prometheus exporter
    exporter, _ := prometheus.New()
    provider := metric.NewMeterProvider(metric.WithReader(exporter))
    otel.SetMeterProvider(provider)

    // Create meter
    meter := otel.Meter("floodgate")

    // Create floodgate metrics collector
    metrics := otelmetrics.NewMetrics(meter)

    // Configure backpressure
    cfg := bpgrpc.DefaultConfig()
    cfg.Metrics = metrics

    // Create gRPC server with backpressure
    server := grpc.NewServer(
        grpc.UnaryInterceptor(bpgrpc.UnaryServerInterceptor(ctx, cfg)),
    )

    // ... register services and serve
}
```

## Datadog Integration

### Installation

```bash
go get github.com/mushtruk/floodgate/metrics/datadog
```

### Full Example

```go
package main

import (
    "context"
    "net/http"
    "time"

    "github.com/DataDog/datadog-go/v5/statsd"
    "github.com/mushtruk/floodgate"
    bphttp "github.com/mushtruk/floodgate/http"
    ddmetrics "github.com/mushtruk/floodgate/metrics/datadog"
)

func main() {
    ctx := context.Background()

    // Create Datadog DogStatsD client
    client, _ := statsd.New("localhost:8125",
        statsd.WithNamespace("myapp"),
        statsd.WithTags([]string{"env:prod", "service:api"}),
    )
    defer client.Close()

    // Create floodgate metrics collector
    metrics := ddmetrics.NewMetrics(client)

    // Configure backpressure
    cfg := bphttp.DefaultConfig()
    cfg.Metrics = metrics
    cfg.Thresholds = floodgate.Thresholds{
        P99Emergency: 500 * time.Millisecond,
        P95Critical:  200 * time.Millisecond,
        EMACritical:  100 * time.Millisecond,
    }

    // Create HTTP server
    mux := http.NewServeMux()
    mux.HandleFunc("/api/users", handleUsers)

    // Wrap with backpressure middleware
    handler := bphttp.Middleware(ctx, cfg)(mux)

    http.ListenAndServe(":8080", handler)
}
```

### Datadog Queries

**Request rate by backpressure level:**
```
sum:myapp.floodgate.requests.total{*} by {level}.as_rate()
```

**Rejection rate:**
```
sum:myapp.floodgate.requests.rejected{*}.as_rate()
```

**P95 latency:**
```
p95:myapp.floodgate.request.duration{*} by {method}
```

**Circuit breaker open endpoints:**
```
sum:myapp.floodgate.circuit_breaker.state{state:open}
```

**Dispatcher drop rate:**
```
sum:myapp.floodgate.dispatcher.drops{*}.as_rate() / sum:myapp.floodgate.dispatcher.events{*}.as_rate()
```

### Alert Configuration

```
Alert name: High Backpressure Rejection Rate
Metric: sum:myapp.floodgate.requests.rejected{*}.as_rate()
Warning: > 5 requests/sec
Critical: > 10 requests/sec
Message: Backpressure rejecting {{value}} req/s on {{service.name}}
```

## Custom Metrics Collector

You can implement the `MetricsCollector` interface for any metrics backend:

```go
type MetricsCollector interface {
    RecordRequest(ctx context.Context, labels RequestLabels, latency time.Duration, rejected bool)
    RecordCircuitBreakerState(method string, state CircuitState)
    RecordCacheSize(size int)
    RecordDispatcherStats(dropped, total uint64)
}
```

### Example: StatsD Implementation

```go
package statsd

import (
    "context"
    "fmt"
    "time"

    "github.com/mushtruk/floodgate"
    "github.com/cactus/go-statsd-client/v5/statsd"
)

type Metrics struct {
    client statsd.Statter
}

func NewMetrics(client statsd.Statter) *Metrics {
    return &Metrics{client: client}
}

func (m *Metrics) RecordRequest(ctx context.Context, labels floodgate.RequestLabels, latency time.Duration, rejected bool) {
    // Increment request counter
    tags := []string{
        fmt.Sprintf("method:%s", labels.Method),
        fmt.Sprintf("level:%s", labels.Level),
        fmt.Sprintf("result:%s", labels.Result),
    }
    m.client.Inc("floodgate.requests", 1, 1.0, tags...)

    // Record latency
    m.client.TimingDuration("floodgate.latency", latency, 1.0, tags...)

    // Track rejections
    if rejected {
        m.client.Inc("floodgate.rejections", 1, 1.0, tags...)
    }
}

func (m *Metrics) RecordCircuitBreakerState(method string, state floodgate.CircuitState) {
    tags := []string{fmt.Sprintf("method:%s", method)}
    value := int64(state) // 0=closed, 1=open, 2=half-open
    m.client.Gauge("floodgate.circuit_breaker", value, 1.0, tags...)
}

func (m *Metrics) RecordCacheSize(size int) {
    m.client.Gauge("floodgate.cache_size", int64(size), 1.0)
}

func (m *Metrics) RecordDispatcherStats(dropped, total uint64) {
    m.client.Gauge("floodgate.dispatcher.dropped", int64(dropped), 1.0)
    m.client.Gauge("floodgate.dispatcher.total", int64(total), 1.0)
}
```

Usage:

```go
client, _ := statsd.NewClient("localhost:8125", "myapp")
defer client.Close()

cfg.Metrics = statsd.NewMetrics(client)
```

## Grafana Dashboards

Pre-built Grafana dashboards are available in the [examples/prometheus-metrics](examples/prometheus-metrics) directory.

### Dashboard Panels

1. **Request Rate** - Requests per second by endpoint and backpressure level
2. **Rejection Rate** - Backpressure rejections over time
3. **Latency Percentiles** - P50, P95, P99 latency by endpoint
4. **Circuit Breaker Status** - Circuit breaker state timeline
5. **Cache Utilization** - Active trackers vs cache capacity
6. **Dispatcher Performance** - Event processing and drop rates

### Import Dashboard

1. Download `examples/prometheus-metrics/grafana-dashboard.json`
2. Open Grafana → Dashboards → Import
3. Upload JSON file
4. Select Prometheus data source
5. Click Import

## Best Practices

### 1. Use Isolated Registry (Prometheus)

Create a dedicated registry to avoid conflicts with application metrics:

```go
reg := prometheus.NewRegistry() // Isolated
// vs
reg := prometheus.DefaultRegisterer // Global (avoid)
```

### 2. Configure Appropriate Thresholds

Tune thresholds based on your SLOs:

```go
cfg.Thresholds = floodgate.Thresholds{
    P99Emergency: 2 * time.Second,   // 99th percentile SLO
    P95Critical:  1 * time.Second,   // 95th percentile SLO
    EMACritical:  500 * time.Millisecond,
}
```

### 3. Set Up Alerting

Alert on key metrics:
- **Rejection rate** - Sustained rejections indicate capacity issues
- **Circuit breaker state** - Open circuit means service degradation
- **Dispatcher drop rate** - High drops mean buffer overflow

### 4. Monitor Cache Size

Track cache utilization to detect:
- **High cardinality** - Too many unique routes/methods
- **Thrashing** - Frequent evictions due to small cache

```go
cfg.CacheSize = 1024 // Increase if needed
```

### 5. Correlate with Application Metrics

Combine floodgate metrics with your application metrics:

```promql
# Backpressure impact on database load
rate(floodgate_requests_rejected_total[5m]) vs rate(db_queries_total[5m])
```

### 6. Use Labels Wisely

Be mindful of label cardinality:

```go
// Good: Fixed set of routes
"/api/users/:id" → "GET /api/users/:id"

// Bad: High cardinality
"/api/users/12345" → "GET /api/users/12345"
```

### 7. Disable Metrics in Development

Use NoOpMetrics for zero overhead during development:

```go
if env == "development" {
    cfg.Metrics = &floodgate.NoOpMetrics{}
}
```

## Performance

The metrics implementation is designed for minimal overhead:

- **NoOpMetrics**: Zero runtime cost (methods are inlined)
- **Prometheus**: ~200ns per RecordRequest call
- **OpenTelemetry**: ~300ns per RecordRequest call

Total overhead: <1% for typical request processing times (>1ms).

## Troubleshooting

### Metrics not appearing

**Issue**: Metrics endpoint returns empty or doesn't include floodgate metrics

**Solutions**:
1. Verify registry is properly configured:
   ```go
   reg := prometheus.NewRegistry()
   metrics := prommetrics.NewMetrics(reg) // Must use same registry
   ```

2. Check middleware is applied:
   ```go
   handler := bphttp.Middleware(ctx, cfg)(mux) // Must wrap handler
   ```

3. Ensure metrics endpoint uses correct registry:
   ```go
   http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
   ```

### High cardinality warnings

**Issue**: Too many unique metric labels

**Solutions**:
1. Normalize routes with parameters:
   ```go
   "/api/users/:id" instead of "/api/users/12345"
   ```

2. Increase cache size to reduce churn:
   ```go
   cfg.CacheSize = 2048
   ```

### Dispatcher drops

**Issue**: High `dispatcher_drops_total` metric

**Solutions**:
1. Increase buffer size:
   ```go
   cfg.DispatcherBufferSize = 4096
   ```

2. Reduce metrics interval:
   ```go
   cfg.MetricsInterval = 30 * time.Second
   ```

## Examples

Complete working examples are available in the repository:

- **[Prometheus HTTP](examples/prometheus-metrics)** - HTTP server with Prometheus metrics
- **[OpenTelemetry gRPC](examples/otel-metrics)** - gRPC server with OpenTelemetry metrics

## Further Reading

- [Prometheus Best Practices](https://prometheus.io/docs/practices/naming/)
- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/instrumentation/go/)
- [Grafana Dashboard Best Practices](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/best-practices/)
