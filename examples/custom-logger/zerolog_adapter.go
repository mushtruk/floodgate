package main

import (
	"context"

	"github.com/mushtruk/floodgate"
	"github.com/rs/zerolog"
)

// ZeroLogAdapter adapts zerolog.Logger to floodgate.Logger interface.
type ZeroLogAdapter struct {
	logger zerolog.Logger
}

// NewZeroLogAdapter creates a new zerolog adapter.
func NewZeroLogAdapter(logger zerolog.Logger) *ZeroLogAdapter {
	return &ZeroLogAdapter{logger: logger}
}

// DebugContext implements floodgate.Logger.
func (z *ZeroLogAdapter) DebugContext(ctx context.Context, msg string, keysAndValues ...interface{}) {
	event := z.logger.Debug().Ctx(ctx)
	z.addFields(event, keysAndValues)
	event.Msg(msg)
}

// InfoContext implements floodgate.Logger.
func (z *ZeroLogAdapter) InfoContext(ctx context.Context, msg string, keysAndValues ...interface{}) {
	event := z.logger.Info().Ctx(ctx)
	z.addFields(event, keysAndValues)
	event.Msg(msg)
}

// WarnContext implements floodgate.Logger.
func (z *ZeroLogAdapter) WarnContext(ctx context.Context, msg string, keysAndValues ...interface{}) {
	event := z.logger.Warn().Ctx(ctx)
	z.addFields(event, keysAndValues)
	event.Msg(msg)
}

// ErrorContext implements floodgate.Logger.
func (z *ZeroLogAdapter) ErrorContext(ctx context.Context, msg string, keysAndValues ...interface{}) {
	event := z.logger.Error().Ctx(ctx)
	z.addFields(event, keysAndValues)
	event.Msg(msg)
}

func (z *ZeroLogAdapter) addFields(event *zerolog.Event, keysAndValues []interface{}) {
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := keysAndValues[i].(string)
			value := keysAndValues[i+1]
			event.Interface(key, value)
		}
	}
}

// Verify interface compliance at compile time.
var _ floodgate.Logger = (*ZeroLogAdapter)(nil)
