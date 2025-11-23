package floodgate

import (
	"context"
	"testing"
	"time"
)

type mockLogger struct {
	lastMsg    string
	debugCount int
	infoCount  int
	warnCount  int
	errorCount int
}

func (m *mockLogger) DebugContext(_ context.Context, msg string, fields ...interface{}) {
	m.debugCount++
	m.lastMsg = msg
}

func (m *mockLogger) InfoContext(_ context.Context, msg string, fields ...interface{}) {
	m.infoCount++
	m.lastMsg = msg
}

func (m *mockLogger) WarnContext(_ context.Context, msg string, fields ...interface{}) {
	m.warnCount++
	m.lastMsg = msg
}

func (m *mockLogger) ErrorContext(_ context.Context, msg string, fields ...interface{}) {
	m.errorCount++
	m.lastMsg = msg
}

type mockAlerter struct {
	lastLevel  string
	lastMsg    string
	alertCount int
}

func (m *mockAlerter) Alert(level string, message string, fields ...interface{}) {
	m.alertCount++
	m.lastLevel = level
	m.lastMsg = message
}

func newTestCircuitBreaker() *CircuitBreaker {
	cb := NewCircuitBreaker(2, 1*time.Second, 2)
	// Set minTimeBetweenOps to 0 for testing
	cb.minTimeBetweenOps = 0
	return cb
}

func TestWithLogging(t *testing.T) {
	t.Parallel()

	logger := &mockLogger{}
	cb := newTestCircuitBreaker()
	wrapped := WithLogging(cb, logger)

	// Trigger transition to open
	wrapped.RecordFailure()
	wrapped.RecordFailure()

	if logger.warnCount != 1 {
		t.Errorf("Expected 1 warn log, got %d", logger.warnCount)
	}

	if logger.lastMsg != "circuit breaker state transition" {
		t.Errorf("Unexpected log message: %s", logger.lastMsg)
	}
}

func TestWithMetrics(t *testing.T) {
	t.Parallel()

	metrics := &MockMetricsCollector{}
	cb := newTestCircuitBreaker()
	wrapped := WithMetrics(cb, metrics)

	wrapped.RecordFailure()

	if metrics.circuitBreakerCalls != 1 {
		t.Errorf("Expected 1 circuit breaker metric call, got %d", metrics.circuitBreakerCalls)
	}
}

func TestWithAlerting(t *testing.T) {
	t.Parallel()

	alerter := &mockAlerter{}
	cb := newTestCircuitBreaker()
	wrapped := WithAlerting(cb, alerter)

	// Trigger transition to open (should alert)
	wrapped.RecordFailure()
	wrapped.RecordFailure()

	if alerter.alertCount != 1 {
		t.Errorf("Expected 1 alert, got %d", alerter.alertCount)
	}

	if alerter.lastLevel != "critical" {
		t.Errorf("Expected critical alert, got %s", alerter.lastLevel)
	}
}

func TestInstrumentedCircuitBreaker(t *testing.T) {
	t.Parallel()

	logger := &mockLogger{}
	metrics := &MockMetricsCollector{}
	alerter := &mockAlerter{}

	cb := NewInstrumentedCircuitBreaker(2, 1*time.Second, 2, logger, metrics, alerter)
	cb.cb.minTimeBetweenOps = 0

	// Trigger transition to open
	cb.RecordFailure()
	cb.RecordFailure()

	// All three should fire
	if logger.warnCount != 1 {
		t.Errorf("Expected 1 warn log, got %d", logger.warnCount)
	}

	if metrics.circuitBreakerCalls != 2 {
		t.Errorf("Expected 2 circuit breaker metric calls, got %d", metrics.circuitBreakerCalls)
	}

	if alerter.alertCount != 1 {
		t.Errorf("Expected 1 alert, got %d", alerter.alertCount)
	}

	// Transition to half-open manually
	cb.cb.state = StateHalfOpen
	cb.cb.lastStateTime = time.Now()

	// Record successes to close
	cb.RecordSuccess()
	cb.RecordSuccess()

	// Should log recovery
	if logger.infoCount == 0 {
		t.Error("Expected info log for recovery")
	}

	// Should alert recovery
	if alerter.lastLevel != "info" {
		t.Errorf("Expected info alert for recovery, got %s", alerter.lastLevel)
	}
}

func TestInstrumentedCircuitBreaker_NilDependencies(t *testing.T) {
	t.Parallel()

	// Should not panic with nil dependencies
	cb := NewInstrumentedCircuitBreaker(2, 1*time.Second, 2, nil, nil, nil)
	cb.cb.minTimeBetweenOps = 0

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()

	if cb.State() != StateOpen {
		t.Errorf("Expected StateOpen, got %v", cb.State())
	}
}

func TestWithLogging_Reset(t *testing.T) {
	t.Parallel()

	logger := &mockLogger{}
	cb := newTestCircuitBreaker()
	wrapped := WithLogging(cb, logger)

	// Open the circuit
	wrapped.RecordFailure()
	wrapped.RecordFailure()

	// Reset
	wrapped.Reset()

	if logger.infoCount != 1 {
		t.Errorf("Expected 1 info log for reset, got %d", logger.infoCount)
	}

	if wrapped.State() != StateClosed {
		t.Errorf("Expected StateClosed after reset, got %v", wrapped.State())
	}
}
