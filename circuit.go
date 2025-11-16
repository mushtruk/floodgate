package floodgate

import (
	"sync"
	"time"
)

// CircuitState represents the current state of a circuit breaker.
type CircuitState int

const (
	// StateClosed indicates the circuit breaker is closed and requests flow through.
	StateClosed CircuitState = iota
	// StateOpen indicates the circuit breaker is open and requests are rejected.
	StateOpen
	// StateHalfOpen indicates the circuit breaker is testing if the service has recovered.
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker prevents cascading failures.
type CircuitBreaker struct {
	mu sync.RWMutex

	state         CircuitState
	failureCount  int
	successCount  int
	lastStateTime time.Time

	maxFailures       int
	timeout           time.Duration
	successThreshold  int
	minTimeBetweenOps time.Duration
}

// NewCircuitBreaker creates a new circuit breaker with the specified configuration.
func NewCircuitBreaker(maxFailures int, timeout time.Duration, successThreshold int) *CircuitBreaker {
	return &CircuitBreaker{
		state:             StateClosed,
		lastStateTime:     time.Now(),
		maxFailures:       maxFailures,
		timeout:           timeout,
		successThreshold:  successThreshold,
		minTimeBetweenOps: 1 * time.Second,
	}
}

// Allow checks if a request should be allowed through the circuit breaker.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		return true

	case StateOpen:
		if now.Sub(cb.lastStateTime) >= cb.timeout {
			cb.state = StateHalfOpen
			cb.successCount = 0
			cb.failureCount = 0
			cb.lastStateTime = now
			return true
		}
		return false

	case StateHalfOpen:
		return true

	default:
		return false
	}
}

// RecordSuccess records a successful request, which may close the circuit breaker.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			if now.Sub(cb.lastStateTime) >= cb.minTimeBetweenOps {
				cb.state = StateClosed
				cb.failureCount = 0
				cb.successCount = 0
				cb.lastStateTime = now
			}
		}

	case StateClosed:
		cb.failureCount = 0
	}
}

// RecordFailure records a failed request, which may open the circuit breaker.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.maxFailures {
			if now.Sub(cb.lastStateTime) >= cb.minTimeBetweenOps {
				cb.state = StateOpen
				cb.lastStateTime = now
			}
		}

	case StateHalfOpen:
		if now.Sub(cb.lastStateTime) >= cb.minTimeBetweenOps {
			cb.state = StateOpen
			cb.lastStateTime = now
		}
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to its initial closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.lastStateTime = time.Now()
}
