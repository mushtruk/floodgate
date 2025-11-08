package floodgate

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
)

// Logger is the interface for logging in floodgate.
// Implementations can integrate with any logging framework (zap, zerolog, slog, etc.).
//
// All logging methods accept a context and variadic key-value pairs for structured logging.
// Keys must be strings, values can be any type. For example:
//
//	logger.InfoContext(ctx, "request processed",
//	    "method", "GET",
//	    "path", "/api/users",
//	    "duration_ms", 42,
//	    "status", 200)
//
// The logger is used for backpressure events, circuit breaker state changes,
// and periodic metrics reporting.
//
// For Go 1.21+, consider using NewSlogAdapter() to wrap the standard library's
// slog.Logger for zero-dependency structured logging.
type Logger interface {
	// DebugContext logs debug-level messages with optional structured key-value pairs.
	// Used for detailed diagnostic information.
	DebugContext(ctx context.Context, msg string, keysAndValues ...any)

	// InfoContext logs info-level messages with optional structured key-value pairs.
	// Used for general informational messages like periodic metrics.
	InfoContext(ctx context.Context, msg string, keysAndValues ...any)

	// WarnContext logs warning-level messages with optional structured key-value pairs.
	// Used for backpressure warnings and circuit breaker events.
	WarnContext(ctx context.Context, msg string, keysAndValues ...any)

	// ErrorContext logs error-level messages with optional structured key-value pairs.
	// Used for critical/emergency backpressure states that reject requests.
	ErrorContext(ctx context.Context, msg string, keysAndValues ...any)
}

// NoOpLogger is a logger that discards all output.
// Use this to completely disable logging in production if desired.
type NoOpLogger struct{}

// DebugContext implements Logger.
func (NoOpLogger) DebugContext(ctx context.Context, msg string, keysAndValues ...any) {}

// InfoContext implements Logger.
func (NoOpLogger) InfoContext(ctx context.Context, msg string, keysAndValues ...any) {}

// WarnContext implements Logger.
func (NoOpLogger) WarnContext(ctx context.Context, msg string, keysAndValues ...any) {}

// ErrorContext implements Logger.
func (NoOpLogger) ErrorContext(ctx context.Context, msg string, keysAndValues ...any) {}

// DefaultLogger is a simple logger that writes to stderr using the standard library.
// For production use, consider using NewSlogAdapter() instead for better performance
// and integration with modern observability tools.
type DefaultLogger struct {
	logger *log.Logger
}

// NewDefaultLogger creates a new default logger that writes to stderr.
func NewDefaultLogger() *DefaultLogger {
	return &DefaultLogger{
		logger: log.New(os.Stderr, "[floodgate] ", log.LstdFlags),
	}
}

// DebugContext implements Logger.
func (l *DefaultLogger) DebugContext(ctx context.Context, msg string, keysAndValues ...any) {
	l.log("DEBUG", msg, keysAndValues...)
}

// InfoContext implements Logger.
func (l *DefaultLogger) InfoContext(ctx context.Context, msg string, keysAndValues ...any) {
	l.log("INFO", msg, keysAndValues...)
}

// WarnContext implements Logger.
func (l *DefaultLogger) WarnContext(ctx context.Context, msg string, keysAndValues ...any) {
	l.log("WARN", msg, keysAndValues...)
}

// ErrorContext implements Logger.
func (l *DefaultLogger) ErrorContext(ctx context.Context, msg string, keysAndValues ...any) {
	l.log("ERROR", msg, keysAndValues...)
}

func (l *DefaultLogger) log(level, msg string, keysAndValues ...any) {
	if len(keysAndValues) == 0 {
		l.logger.Printf("%s: %s", level, msg)
		return
	}

	// Format key-value pairs efficiently
	var sb strings.Builder
	sb.WriteString(level)
	sb.WriteString(": ")
	sb.WriteString(msg)

	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			// Ensure key is a string
			key, ok := keysAndValues[i].(string)
			if !ok {
				key = fmt.Sprintf("%v", keysAndValues[i])
			}
			sb.WriteString(" ")
			sb.WriteString(key)
			sb.WriteString("=")
			// Format value appropriately
			value := keysAndValues[i+1]
			if s, ok := value.(string); ok {
				sb.WriteString(s)
			} else {
				sb.WriteString(fmt.Sprintf("%v", value))
			}
		} else {
			// Handle odd number of arguments gracefully
			sb.WriteString(" ")
			sb.WriteString(fmt.Sprintf("!MISSING_VALUE=%v", keysAndValues[i]))
		}
	}

	l.logger.Print(sb.String())
}

// SlogAdapter wraps a slog.Logger to implement the floodgate Logger interface.
// This is the recommended approach for Go 1.21+ applications.
//
// Example usage:
//
//	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
//	    Level: slog.LevelInfo, // Set minimum log level
//	})
//	logger := floodgate.NewSlogAdapter(slog.New(handler))
//	cfg.Logger = logger
type SlogAdapter struct {
	logger *slog.Logger
}

// NewSlogAdapter creates a new slog adapter.
// This allows you to use Go's standard library structured logging with floodgate.
func NewSlogAdapter(logger *slog.Logger) *SlogAdapter {
	return &SlogAdapter{logger: logger}
}

// DebugContext implements Logger.
func (s *SlogAdapter) DebugContext(ctx context.Context, msg string, keysAndValues ...any) {
	s.logger.DebugContext(ctx, msg, keysAndValues...)
}

// InfoContext implements Logger.
func (s *SlogAdapter) InfoContext(ctx context.Context, msg string, keysAndValues ...any) {
	s.logger.InfoContext(ctx, msg, keysAndValues...)
}

// WarnContext implements Logger.
func (s *SlogAdapter) WarnContext(ctx context.Context, msg string, keysAndValues ...any) {
	s.logger.WarnContext(ctx, msg, keysAndValues...)
}

// ErrorContext implements Logger.
func (s *SlogAdapter) ErrorContext(ctx context.Context, msg string, keysAndValues ...any) {
	s.logger.ErrorContext(ctx, msg, keysAndValues...)
}
