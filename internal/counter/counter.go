// Package counter converts cumulative counter samples into the increments that metric backends record.
//
// The v1 metrics submodules import this package across module boundaries and
// build against any later v1 root release, so its API is frozen within v1.
package counter

import "math"

// Delta returns how far a cumulative counter advanced from prev to current.
// A counter that went backwards was reset and reports no advance, and an
// advance beyond the int64 range saturates, so the result never wraps.
func Delta(current, prev uint64) int64 {
	if current <= prev {
		return 0
	}
	if d := current - prev; d <= math.MaxInt64 {
		return int64(d)
	}
	return math.MaxInt64
}
