package floodgate

import (
	"sync"
	"time"
)

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
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

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.lastStateTime = time.Now()
}
