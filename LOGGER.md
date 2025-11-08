# Configurable Logging

Floodgate provides a flexible logging interface that integrates with any Go logging framework including slog (Go 1.21+), zerolog, zap, and others.

## Quick Start

### Using slog (Recommended for Go 1.21+)

```go
import (
    "log/slog"
    "os"
    "github.com/mushtruk/floodgate"
    floodgatehttp "github.com/mushtruk/floodgate/http"
)

// Create slog logger with JSON output
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo, // Set minimum log level
})
slogLogger := slog.New(handler)

// Wrap with floodgate adapter
logger := floodgate.NewSlogAdapter(slogLogger)

// Use in configuration
cfg := floodgatehttp.DefaultConfig()
cfg.Logger = logger
```

### Using zap

```go
import (
    "go.uber.org/zap"
    "github.com/mushtruk/floodgate"
)

// Create zap logger with production config
zapConfig := zap.NewProductionConfig()
zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
zapLogger, _ := zapConfig.Build()

// Create adapter (see examples/zap-logger/zap_adapter.go)
logger := NewZapAdapter(zapLogger)

cfg.Logger = logger
```

### Using zerolog

```go
import (
    "github.com/rs/zerolog"
    "github.com/mushtruk/floodgate"
)

// Create your zerolog logger
zerologLogger := zerolog.New(os.Stdout).
    With().
    Timestamp().
    Logger()

// Create adapter (see examples/custom-logger/zerolog_adapter.go)
logger := NewZeroLogAdapter(zerologLogger)

cfg.Logger = logger
```

### Using Default Logger

```go
// Uses Go's standard library log package
cfg := floodgatehttp.DefaultConfig()
// cfg.Logger is already set to DefaultLogger
```

### Disabling Logging

```go
cfg := floodgatehttp.DefaultConfig()
cfg.Logger = &floodgate.NoOpLogger{}
```

## Logger Interface

All logging methods accept a context and variadic key-value pairs for structured logging:

```go
type Logger interface {
    DebugContext(ctx context.Context, msg string, keysAndValues ...any)
    InfoContext(ctx context.Context, msg string, keysAndValues ...any)
    WarnContext(ctx context.Context, msg string, keysAndValues ...any)
    ErrorContext(ctx context.Context, msg string, keysAndValues ...any)
}
```

## What Floodgate Logs

### Info Level
- **Periodic metrics** (when `EnableMetrics: true`)
  ```
  cache_used=10 cache_size=512 cache_pct=1.95 drops=5 total=1000 drop_rate=0.005 circuit=closed
  ```

### Warn Level
- **Circuit breaker open** - Service circuit breaker has opened
  ```
  method=/api/users
  ```
- **Backpressure detected** - Warning or Moderate backpressure levels
  ```
  level=warning method=/api/users ema=75ms p95=90ms p99=120ms
  ```

### Error Level
- **Critical backpressure** - Service rejecting requests
  ```
  method=/api/users ema=180ms p95=210ms p99=250ms
  ```
- **Emergency backpressure** - Service severely overloaded
  ```
  method=/api/users ema=320ms p95=350ms p99=400ms
  ```

## Context Support

All log methods accept a context, allowing you to:
- Extract trace IDs for distributed tracing
- Include request IDs for correlation
- Add custom fields from context values

Example with trace ID:

```go
// In your slog adapter
func (s *SlogAdapter) InfoContext(ctx context.Context, msg string, keysAndValues ...any) {
    // Extract trace ID from context if available
    if traceID := trace.SpanContextFromContext(ctx).TraceID(); traceID.IsValid() {
        s.logger.InfoContext(ctx, msg, append(keysAndValues, "trace_id", traceID.String())...)
        return
    }
    s.logger.InfoContext(ctx, msg, keysAndValues...)
}
```

## Log Levels

Configure minimum log level in your handler:

```go
// Only log Info and above (suppresses Debug)
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})

// Log everything including Debug
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
})
```

## Integration with Observability Tools

### OpenTelemetry

```go
import (
    "log/slog"
    "go.opentelemetry.io/contrib/bridges/otelslog"
)

// Create OpenTelemetry slog handler
handler := otelslog.NewHandler("floodgate")
logger := floodgate.NewSlogAdapter(slog.New(handler))
```

### Datadog

```go
// Use zerolog with Datadog
zerologLogger := zerolog.New(os.Stdout).
    With().
    Str("service", "my-service").
    Str("env", "production").
    Logger()

logger := NewZeroLogAdapter(zerologLogger)
```

## Examples

See complete working examples:
- [examples/slog-logger/](examples/slog-logger/) - Using Go 1.21+ slog (recommended)
- [examples/zap-logger/](examples/zap-logger/) - Using uber-go/zap (high performance)
- [examples/custom-logger/](examples/custom-logger/) - Using rs/zerolog (zero allocation)

## Why Context-Based Logging?

Context-based logging enables:

1. **Distributed Tracing** - Automatically include trace/span IDs from context
2. **Request Correlation** - Track logs across service boundaries using request IDs
3. **Dynamic Configuration** - Change log behavior based on context values
4. **Structured Context** - Pass structured data through the context chain

## Performance

- **SlogAdapter**: Zero-allocation wrapper, delegates directly to slog
- **DefaultLogger**: Uses strings.Builder for efficient string concatenation
- **NoOpLogger**: Zero overhead, all methods are empty

## Migration from v1.1.0

If you're upgrading from v1.1.0 (which had hardcoded logging), the default behavior remains the same. The `DefaultLogger` provides the same output format as before, just more configurable.

```go
// v1.1.0 - Hardcoded logging
cfg := grpc.DefaultConfig()

// v1.2.0 - Same behavior, now configurable
cfg := grpc.DefaultConfig()
cfg.Logger = floodgate.NewDefaultLogger() // Already the default
```
