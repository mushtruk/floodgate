package floodgate

import (
	"context"
	"time"
)

// CircuitBreakerWrapper defines the interface for circuit breaker wrappers.
// This allows composing multiple behaviors (logging, metrics, alerting).
type CircuitBreakerWrapper interface {
	Allow() bool
	RecordSuccess()
	RecordFailure()
	State() CircuitState
	Reset()
}

// WithLogging wraps a circuit breaker to log state transitions.
func WithLogging(cb *CircuitBreaker, logger Logger) CircuitBreakerWrapper {
	return &loggingCircuitBreaker{
		cb:     cb,
		logger: logger,
	}
}

type loggingCircuitBreaker struct {
	cb     *CircuitBreaker
	logger Logger
}

func (l *loggingCircuitBreaker) Allow() bool {
	return l.cb.Allow()
}

func (l *loggingCircuitBreaker) RecordSuccess() {
	oldState := l.cb.State()
	l.cb.RecordSuccess()
	newState := l.cb.State()

	if oldState != newState {
		l.logger.InfoContext(context.Background(), "circuit breaker state transition",
			"from", oldState.String(),
			"to", newState.String(),
			"event", "success")
	}
}

func (l *loggingCircuitBreaker) RecordFailure() {
	oldState := l.cb.State()
	l.cb.RecordFailure()
	newState := l.cb.State()

	if oldState != newState {
		l.logger.WarnContext(context.Background(), "circuit breaker state transition",
			"from", oldState.String(),
			"to", newState.String(),
			"event", "failure")
	}
}

func (l *loggingCircuitBreaker) State() CircuitState {
	return l.cb.State()
}

func (l *loggingCircuitBreaker) Reset() {
	oldState := l.cb.State()
	l.cb.Reset()
	l.logger.InfoContext(context.Background(), "circuit breaker reset",
		"from", oldState.String(),
		"to", "closed")
}

// WithMetrics wraps a circuit breaker to collect metrics.
func WithMetrics(cb *CircuitBreaker, metrics MetricsCollector) CircuitBreakerWrapper {
	return &metricsCircuitBreaker{
		cb:      cb,
		metrics: metrics,
	}
}

type metricsCircuitBreaker struct {
	cb      *CircuitBreaker
	metrics MetricsCollector
}

func (m *metricsCircuitBreaker) Allow() bool {
	return m.cb.Allow()
}

func (m *metricsCircuitBreaker) RecordSuccess() {
	m.cb.RecordSuccess()
	m.metrics.RecordCircuitBreakerState("circuit_breaker", m.cb.State())
}

func (m *metricsCircuitBreaker) RecordFailure() {
	m.cb.RecordFailure()
	m.metrics.RecordCircuitBreakerState("circuit_breaker", m.cb.State())
}

func (m *metricsCircuitBreaker) State() CircuitState {
	return m.cb.State()
}

func (m *metricsCircuitBreaker) Reset() {
	m.cb.Reset()
	m.metrics.RecordCircuitBreakerState("circuit_breaker", m.cb.State())
}

// Alerter sends alerts for critical events.
type Alerter interface {
	Alert(level string, message string, fields ...interface{})
}

// WithAlerting wraps a circuit breaker to send alerts on state changes.
func WithAlerting(cb *CircuitBreaker, alerter Alerter) CircuitBreakerWrapper {
	return &alertingCircuitBreaker{
		cb:      cb,
		alerter: alerter,
	}
}

type alertingCircuitBreaker struct {
	cb      *CircuitBreaker
	alerter Alerter
}

func (a *alertingCircuitBreaker) Allow() bool {
	return a.cb.Allow()
}

func (a *alertingCircuitBreaker) RecordSuccess() {
	oldState := a.cb.State()
	a.cb.RecordSuccess()
	newState := a.cb.State()

	if oldState != newState && newState == StateClosed {
		a.alerter.Alert("info", "Circuit breaker recovered",
			"from", oldState.String(),
			"to", newState.String())
	}
}

func (a *alertingCircuitBreaker) RecordFailure() {
	oldState := a.cb.State()
	a.cb.RecordFailure()
	newState := a.cb.State()

	if oldState != newState && newState == StateOpen {
		a.alerter.Alert("critical", "Circuit breaker opened",
			"from", oldState.String(),
			"to", newState.String(),
			"message", "Service experiencing failures")
	}
}

func (a *alertingCircuitBreaker) State() CircuitState {
	return a.cb.State()
}

func (a *alertingCircuitBreaker) Reset() {
	a.cb.Reset()
	a.alerter.Alert("info", "Circuit breaker manually reset")
}

// InstrumentedCircuitBreaker combines all observability wrappers.
type InstrumentedCircuitBreaker struct {
	cb      *CircuitBreaker
	logger  Logger
	metrics MetricsCollector
	alerter Alerter
}

// NewInstrumentedCircuitBreaker creates a circuit breaker with full observability.
func NewInstrumentedCircuitBreaker(
	maxFailures int,
	timeout time.Duration,
	successThreshold int,
	logger Logger,
	metrics MetricsCollector,
	alerter Alerter,
) *InstrumentedCircuitBreaker {
	return &InstrumentedCircuitBreaker{
		cb:      NewCircuitBreaker(maxFailures, timeout, successThreshold),
		logger:  logger,
		metrics: metrics,
		alerter: alerter,
	}
}

func (i *InstrumentedCircuitBreaker) Allow() bool {
	return i.cb.Allow()
}

func (i *InstrumentedCircuitBreaker) RecordSuccess() {
	oldState := i.cb.State()
	i.cb.RecordSuccess()
	newState := i.cb.State()

	if oldState != newState {
		if i.logger != nil {
			i.logger.InfoContext(context.Background(), "circuit breaker state transition",
				"from", oldState.String(),
				"to", newState.String(),
				"event", "success")
		}

		if i.alerter != nil && newState == StateClosed {
			i.alerter.Alert("info", "Circuit breaker recovered",
				"from", oldState.String(),
				"to", newState.String())
		}
	}

	if i.metrics != nil {
		i.metrics.RecordCircuitBreakerState("circuit_breaker", newState)
	}
}

func (i *InstrumentedCircuitBreaker) RecordFailure() {
	oldState := i.cb.State()
	i.cb.RecordFailure()
	newState := i.cb.State()

	if oldState != newState {
		if i.logger != nil {
			i.logger.WarnContext(context.Background(), "circuit breaker state transition",
				"from", oldState.String(),
				"to", newState.String(),
				"event", "failure")
		}

		if i.alerter != nil && newState == StateOpen {
			i.alerter.Alert("critical", "Circuit breaker opened",
				"from", oldState.String(),
				"to", newState.String(),
				"message", "Service experiencing failures")
		}
	}

	if i.metrics != nil {
		i.metrics.RecordCircuitBreakerState("circuit_breaker", newState)
	}
}

func (i *InstrumentedCircuitBreaker) State() CircuitState {
	return i.cb.State()
}

func (i *InstrumentedCircuitBreaker) Reset() {
	oldState := i.cb.State()
	i.cb.Reset()

	if i.logger != nil {
		i.logger.InfoContext(context.Background(), "circuit breaker reset",
			"from", oldState.String(),
			"to", "closed")
	}

	if i.alerter != nil {
		i.alerter.Alert("info", "Circuit breaker manually reset")
	}

	if i.metrics != nil {
		i.metrics.RecordCircuitBreakerState("circuit_breaker", StateClosed)
	}
}
