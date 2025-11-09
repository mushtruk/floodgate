package main

import (
	"context"

	"github.com/mushtruk/floodgate"
	"go.uber.org/zap"
)

// ZapAdapter adapts zap.Logger to floodgate.Logger interface.
type ZapAdapter struct {
	logger *zap.Logger
}

// NewZapAdapter creates a new zap adapter.
func NewZapAdapter(logger *zap.Logger) *ZapAdapter {
	return &ZapAdapter{logger: logger}
}

// DebugContext implements floodgate.Logger.
func (z *ZapAdapter) DebugContext(ctx context.Context, msg string, keysAndValues ...any) {
	z.logger.Debug(msg, z.toZapFields(keysAndValues)...)
}

// InfoContext implements floodgate.Logger.
func (z *ZapAdapter) InfoContext(ctx context.Context, msg string, keysAndValues ...any) {
	z.logger.Info(msg, z.toZapFields(keysAndValues)...)
}

// WarnContext implements floodgate.Logger.
func (z *ZapAdapter) WarnContext(ctx context.Context, msg string, keysAndValues ...any) {
	z.logger.Warn(msg, z.toZapFields(keysAndValues)...)
}

// ErrorContext implements floodgate.Logger.
func (z *ZapAdapter) ErrorContext(ctx context.Context, msg string, keysAndValues ...any) {
	z.logger.Error(msg, z.toZapFields(keysAndValues)...)
}

func (z *ZapAdapter) toZapFields(keysAndValues []any) []zap.Field {
	if len(keysAndValues) == 0 {
		return nil
	}

	fields := make([]zap.Field, 0, len(keysAndValues)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := keysAndValues[i].(string)
			value := keysAndValues[i+1]
			fields = append(fields, zap.Any(key, value))
		}
	}
	return fields
}

// Verify interface compliance at compile time.
var _ floodgate.Logger = (*ZapAdapter)(nil)
