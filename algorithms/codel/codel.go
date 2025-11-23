// Package codel implements the CoDel (Controlled Delay) active queue management algorithm.
//
// CoDel is a modern algorithm designed to combat bufferbloat by keeping latency low
// while maintaining high throughput. Unlike traditional algorithms that rely on queue
// length, CoDel uses sojourn time (time in queue) to make dropping decisions.
//
// The algorithm adapts automatically to varying conditions without manual tuning,
// making it ideal for dynamic workloads where fixed thresholds may not work well.
//
// References:
//   - "Controlling Queue Delay" (Nichols & Jacobson, 2012)
//   - Linux kernel implementation
//   - Cloudflare's adaptive backpressure
package codel

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mushtruk/floodgate"
)

// Pre-computed square roots for common drop counts to avoid math.Sqrt() calls.
// Covers drop counts 1-100, which handles most real-world scenarios.
var sqrtLookup = [101]float64{
	0, 1.0000, 1.4142, 1.7321, 2.0000, 2.2361, 2.4495, 2.6458, 2.8284, 3.0000,
	3.1623, 3.3166, 3.4641, 3.6056, 3.7417, 3.8730, 4.0000, 4.1231, 4.2426, 4.3589,
	4.4721, 4.5826, 4.6904, 4.7958, 4.8990, 5.0000, 5.0990, 5.1962, 5.2915, 5.3852,
	5.4772, 5.5678, 5.6569, 5.7446, 5.8310, 5.9161, 6.0000, 6.0828, 6.1644, 6.2450,
	6.3246, 6.4031, 6.4807, 6.5574, 6.6332, 6.7082, 6.7823, 6.8557, 6.9282, 7.0000,
	7.0711, 7.1414, 7.2111, 7.2801, 7.3485, 7.4162, 7.4833, 7.5498, 7.6158, 7.6811,
	7.7460, 7.8102, 7.8740, 7.9373, 8.0000, 8.0623, 8.1240, 8.1854, 8.2462, 8.3066,
	8.3666, 8.4261, 8.4853, 8.5440, 8.6023, 8.6603, 8.7178, 8.7750, 8.8318, 8.8882,
	8.9443, 9.0000, 9.0554, 9.1104, 9.1652, 9.2195, 9.2736, 9.3274, 9.3808, 9.4340,
	9.4868, 9.5394, 9.5917, 9.6437, 9.6954, 9.7468, 9.7980, 9.8489, 9.8995, 9.9499, 10.0000,
}

// Algorithm implements the CoDel (Controlled Delay) algorithm for adaptive backpressure.
//
// CoDel works by:
//  1. Measuring sojourn time (how long requests are delayed)
//  2. Comparing to a target delay (default 5ms)
//  3. Entering "dropping mode" if delay exceeds target for an interval
//  4. Drop interval decreases with sqrt(count), increasing drop frequency using a control law
//
// The algorithm is self-tuning and adapts to network conditions automatically.
type Algorithm struct {
	mu sync.Mutex

	// Configuration
	targetDelay   time.Duration // Target sojourn time (default: 5ms)
	interval      time.Duration // Measurement interval (default: 100ms)
	intervalNs    int64         // Cached interval in nanoseconds for controlLaw
	targetDelayNs int64         // Cached target delay in nanoseconds for mapToLevel

	// State (protected by mu)
	dropping    bool      // Currently in dropping state
	dropNext    time.Time // Time for next drop
	count       int       // Drop count in current dropping episode
	lastDropped time.Time // Time of last drop
	firstAbove  time.Time // When delay first exceeded target

	// Atomic flag for fast-path optimization (0 = not dropping, 1 = dropping)
	droppingFlag uint32
}

// Option configures the CoDel algorithm.
type Option func(*Algorithm)

// WithTargetDelay sets the target sojourn time.
// This is the acceptable queueing delay. Requests experiencing higher
// delays will trigger backpressure. Default is 5ms.
//
// Lower values = more aggressive backpressure, better tail latency.
// Higher values = more lenient, higher throughput under load.
func WithTargetDelay(d time.Duration) Option {
	return func(a *Algorithm) {
		a.targetDelay = d
	}
}

// WithInterval sets the measurement interval.
// CoDel only enters dropping mode if delay exceeds target for this duration.
// This prevents reacting to transient spikes. Default is 100ms.
func WithInterval(d time.Duration) Option {
	return func(a *Algorithm) {
		a.interval = d
	}
}

// NewAlgorithm creates a new CoDel algorithm with sensible defaults.
//
// Default configuration:
//   - Target delay: 5ms (good for most applications)
//   - Interval: 100ms (prevents overreaction to spikes)
//
// These values work well for typical microservices. Adjust based on your SLA:
//   - Low-latency APIs: 2-5ms target
//   - Standard APIs: 5-10ms target
//   - Batch processing: 20-50ms target
func NewAlgorithm(opts ...Option) *Algorithm {
	a := &Algorithm{
		targetDelay: 5 * time.Millisecond,
		interval:    100 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(a)
	}

	// Validate configuration to prevent division by zero
	if a.targetDelay <= 0 {
		panic("codel: targetDelay must be positive")
	}
	if a.interval <= 0 {
		panic("codel: interval must be positive")
	}

	// Cache nanoseconds for performance
	a.intervalNs = a.interval.Nanoseconds()
	a.targetDelayNs = a.targetDelay.Nanoseconds()

	return a
}

// Decide implements floodgate.Algorithm using the CoDel control law.
func (a *Algorithm) Decide(stats floodgate.Stats) floodgate.Decision {
	// Use P95 as sojourn time estimate (or fall back to EMA)
	sojournTime := stats.P95
	if sojournTime == 0 {
		sojournTime = stats.EMA
	}

	// Fast path: if below target and not dropping, return immediately (lock-free)
	// This optimizes the common case (no backpressure)
	aboveTarget := sojournTime > a.targetDelay
	if !aboveTarget && atomic.LoadUint32(&a.droppingFlag) == 0 {
		// Common case: not above target, not dropping
		// However, we still need to reset firstAbove if it was set during a transient spike
		// Check if firstAbove is set (requires lock unfortunately)
		a.mu.Lock()
		if !a.firstAbove.IsZero() {
			a.firstAbove = time.Time{}
		}
		a.mu.Unlock()

		return floodgate.Decision{
			Level:  a.mapToLevel(sojournTime),
			Reject: false,
		}
	}

	// Need to acquire lock for state changes
	a.mu.Lock()

	if !aboveTarget {
		// Below target - exit dropping state if necessary
		if a.dropping {
			a.dropping = false
			atomic.StoreUint32(&a.droppingFlag, 0)
		}
		// CRITICAL: Always reset firstAbove when below target to prevent
		// accumulated time from incorrectly triggering dropping mode later
		a.firstAbove = time.Time{}
		a.mu.Unlock()

		return floodgate.Decision{
			Level:  a.mapToLevel(sojournTime),
			Reject: false,
		}
	}

	// Above target - check for persistent delay
	now := time.Now()

	// Track when we first went above target
	if a.firstAbove.IsZero() {
		a.firstAbove = now
	}

	// Has delay been above target for the full interval?
	persistentlyAbove := now.Sub(a.firstAbove) >= a.interval

	if persistentlyAbove {
		if a.dropping {
			// Already dropping - check if it's time for next drop
			if !now.Before(a.dropNext) {
				// Drop this request
				a.count++
				a.lastDropped = now
				a.dropNext = a.controlLaw(now)
				level := a.mapToLevel(sojournTime)
				a.mu.Unlock()

				return floodgate.Decision{
					Level:  level,
					Reject: true,
				}
			}
		} else {
			// Enter dropping state
			a.dropping = true
			atomic.StoreUint32(&a.droppingFlag, 1)
			a.count = 1
			a.lastDropped = now
			a.dropNext = a.controlLaw(now)
			a.mu.Unlock()

			return floodgate.Decision{
				Level:  floodgate.Moderate,
				Reject: true,
			}
		}
	}

	a.mu.Unlock()

	// Not dropping this request (calculate level outside lock)
	return floodgate.Decision{
		Level:  a.mapToLevel(sojournTime),
		Reject: false,
	}
}

// controlLaw calculates the next drop time using CoDel's control law.
// Drop frequency increases with sqrt(count) to quickly drain the queue
// while avoiding over-dropping.
//
// Formula: next_drop = current_time + interval / sqrt(count).
func (a *Algorithm) controlLaw(t time.Time) time.Time {
	// Use lookup table for common counts, fall back to math.Sqrt for larger values
	var sqrtCount float64
	if a.count < len(sqrtLookup) {
		sqrtCount = sqrtLookup[a.count]
	} else {
		sqrtCount = math.Sqrt(float64(a.count))
	}

	// Use pre-cached intervalNs to avoid Nanoseconds() call
	nextInterval := time.Duration(float64(a.intervalNs) / sqrtCount)

	return t.Add(nextInterval)
}

// mapToLevel maps sojourn time to a backpressure level.
// This provides observability even when not dropping.
func (a *Algorithm) mapToLevel(sojournTime time.Duration) floodgate.Level {
	// Use integer arithmetic to avoid float division
	// ratio = sojournTime / targetDelay, but we use fixed-point math
	// Multiply by 10 to get ratio*10 for comparison (avoids float division)
	sojournNs := int64(sojournTime)

	// Fast path: if sojourn < target, it's Normal
	if sojournNs <= a.targetDelayNs {
		return floodgate.Normal
	}

	// ratio*10 = (sojournNs * 10) / targetDelayNs
	ratio10 := (sojournNs * 10) / a.targetDelayNs

	switch {
	case ratio10 >= 100: // ratio >= 10
		return floodgate.Emergency
	case ratio10 >= 50: // ratio >= 5
		return floodgate.Critical
	case ratio10 >= 30: // ratio >= 3
		return floodgate.Moderate
	case ratio10 >= 15: // ratio >= 1.5
		return floodgate.Warning
	default:
		return floodgate.Normal
	}
}
