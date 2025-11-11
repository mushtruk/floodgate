# Prometheus Metrics Example

This example demonstrates how to integrate Prometheus metrics with floodgate HTTP middleware.

## Features

- **Prometheus Integration**: Exposes floodgate metrics in Prometheus format
- **Multiple Endpoints**: Fast and slow endpoints to demonstrate backpressure behavior
- **Real-time Metrics**: Live metrics available at `/metrics` endpoint
- **Grafana Dashboard**: Pre-configured dashboard for visualization (see below)

## Running the Example

```bash
cd examples/prometheus-metrics
go run main.go
```

The server starts on `http://localhost:8080` with the following endpoints:

- `GET /api/fast` - Fast endpoint (1-5ms latency)
- `GET /api/slow` - Slow endpoint (100-300ms latency, triggers backpressure)
- `GET /health` - Health check (skipped by backpressure middleware)
- `GET /metrics` - Prometheus metrics endpoint

## Testing Backpressure

### Generate normal load:
```bash
for i in {1..100}; do curl http://localhost:8080/api/fast & done
```

### Trigger backpressure:
```bash
for i in {1..50}; do curl http://localhost:8080/api/slow & done
```

### View metrics:
```bash
curl http://localhost:8080/metrics | grep floodgate
```

## Available Metrics

### Request Metrics
- `floodgate_requests_total{method, level, result}` - Total requests by method, backpressure level, and result
- `floodgate_requests_rejected_total{method, level}` - Total rejected requests by method and level
- `floodgate_request_duration_seconds{method}` - Request latency histogram by method

### Circuit Breaker
- `floodgate_circuit_breaker_state{method}` - Circuit breaker state (0=closed, 1=open, 2=half-open)

### Cache & Dispatcher
- `floodgate_cache_size` - Number of active method/route trackers in cache
- `floodgate_dispatcher_drops_total` - Total events dropped by async dispatcher
- `floodgate_dispatcher_events_total` - Total events emitted to async dispatcher

## Prometheus Configuration

Add this job to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'floodgate-example'
    scrape_interval: 5s
    static_configs:
      - targets: ['localhost:8080']
```

## Grafana Dashboard

A pre-configured Grafana dashboard is available at `grafana-dashboard.json`.

### Import Steps:
1. Open Grafana UI
2. Navigate to Dashboards → Import
3. Upload `grafana-dashboard.json`
4. Select your Prometheus data source
5. Click Import

### Dashboard Panels:
- **Request Rate**: Requests per second by endpoint
- **Rejection Rate**: Backpressure rejections per second
- **Latency Percentiles**: P50, P95, P99 latency by endpoint
- **Circuit Breaker Status**: Circuit breaker state over time
- **Cache Utilization**: Active trackers in LRU cache
- **Dispatcher Performance**: Event processing and drop rates

## Example Queries

### Request rate by backpressure level:
```promql
rate(floodgate_requests_total[1m])
```

### Rejection rate:
```promql
rate(floodgate_requests_rejected_total[1m])
```

### P95 latency:
```promql
histogram_quantile(0.95, rate(floodgate_request_duration_seconds_bucket[5m]))
```

### Circuit breaker open count:
```promql
sum(floodgate_circuit_breaker_state == 1)
```

### Dispatcher drop rate:
```promql
rate(floodgate_dispatcher_drops_total[1m]) / rate(floodgate_dispatcher_events_total[1m])
```

## Docker Compose Stack

A complete monitoring stack with Prometheus and Grafana is available:

```bash
docker-compose up -d
```

Access:
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)

The dashboard will be automatically provisioned.
