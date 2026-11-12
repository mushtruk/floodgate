# Middleware Chain Pattern Example

This example demonstrates composable HTTP middleware with Floodgate backpressure integration.

## Pattern: Middleware Chain

The Chain of Responsibility pattern allows you to compose multiple middleware layers, each with a single responsibility. Middleware executes in order from outer to inner for requests, and inner to outer for responses.

## Architecture

```
Request Flow (top to bottom):
┌─────────────────────────┐
│  Recovery Middleware    │ ← Catches panics
├─────────────────────────┤
│  Logging Middleware     │ ← Logs requests/responses
├─────────────────────────┤
│  CORS Middleware        │ ← Adds CORS headers
├─────────────────────────┤
│  Timeout Middleware     │ ← Enforces request timeout
├─────────────────────────┤
│ Backpressure Middleware │ ← Threshold-based backpressure
├─────────────────────────┤
│   Application Handler   │ ← Your business logic
└─────────────────────────┘
```

## Benefits

1. **Separation of Concerns**: Each middleware handles one responsibility
2. **Composability**: Easy to add, remove, or reorder middleware
3. **Reusability**: Middleware can be shared across routes
4. **Testability**: Each layer can be tested independently

## Running the Example

```bash
cd examples/middleware-chain
go run main.go
```

## Testing

### Normal Request
```bash
curl http://localhost:8080/fast
```

Expected: Fast response (~1ms), logged, with CORS headers

### Slow Request (triggers backpressure)
```bash
# Single request
curl http://localhost:8080/slow

# Load test to trigger backpressure
hey -z 10s -c 50 http://localhost:8080/slow
```

Expected:
- Initial requests succeed
- Once `/slow` latency exceeds the emergency threshold, the backpressure middleware rejects requests with 503
- Logs show backpressure decisions

### Panic Recovery
```bash
curl http://localhost:8080/panic
```

Expected: 500 Internal Server Error (panic caught by recovery middleware)

### CORS Preflight
```bash
curl -X OPTIONS http://localhost:8080/fast -H "Access-Control-Request-Method: GET"
```

Expected: 200 OK with CORS headers

## Middleware Ordering

The order matters:

```go
Chain(
    RecoveryMiddleware(),        // Outermost - catches all panics
    LoggingMiddleware(),         // Logs everything including errors
    CORSMiddleware(),            // CORS headers on all responses
    TimeoutMiddleware(30*time.Second), // Per-request timeout
    BackpressureMiddleware(ctx), // Reject overload before hitting handler
)
```

## Customization

### Add Authentication
```go
func AuthMiddleware(secret string) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := r.Header.Get("Authorization")
            if !validateToken(token, secret) {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

// Add to chain
chain := Chain(
    RecoveryMiddleware(),
    LoggingMiddleware(),
    AuthMiddleware("my-secret"),    // Add authentication
    BackpressureMiddleware(ctx),
)
```

### Per-Route Middleware
```go
// Different chains for different route groups
publicChain := Chain(
    RecoveryMiddleware(),
    LoggingMiddleware(),
    CORSMiddleware(),
)

protectedChain := Chain(
    RecoveryMiddleware(),
    LoggingMiddleware(),
    AuthMiddleware("secret"),
    BackpressureMiddleware(ctx),
)

mux.Handle("/public/", publicChain(publicHandler))
mux.Handle("/api/", protectedChain(apiHandler))
```

## Integration with Floodgate

The backpressure middleware uses Floodgate's HTTP integration:

```go
func BackpressureMiddleware(ctx context.Context) Middleware {
    cfg := fhttp.DefaultConfig()
    cfg.Algorithm = floodgate.NewThresholdAlgorithm(floodgate.Thresholds{
        P99Emergency: 40 * time.Millisecond,
        P95Critical:  30 * time.Millisecond,
        EMACritical:  25 * time.Millisecond,
        P95Moderate:  20 * time.Millisecond,
        EMAWarning:   10 * time.Millisecond,
        SlopeWarning: 5 * time.Millisecond,
    })
    cfg.EnableMetrics = true

    return fhttp.Middleware(ctx, cfg)
}
```

This shows how Floodgate integrates seamlessly into standard middleware chains.
