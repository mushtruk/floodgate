package floodgate

import (
	"context"
	"hash/fnv"
	"sync"
	"time"
)

// EventFilter transforms or rejects events before they're processed.
// This implements the Chain of Responsibility pattern.
type EventFilter[T any] interface {
	// Filter processes an event and returns whether it should continue.
	// Returns (modifiedEvent, shouldContinue).
	Filter(ev Event[T]) (Event[T], bool)
}

// FilterChain applies multiple filters in sequence.
type FilterChain[T any] []EventFilter[T]

// Apply runs all filters in the chain.
// Returns the final event and whether it passed all filters.
func (chain FilterChain[T]) Apply(ev Event[T]) (Event[T], bool) {
	for _, filter := range chain {
		var ok bool
		ev, ok = filter.Filter(ev)
		if !ok {
			return ev, false
		}
	}
	return ev, true
}

// SamplingFilter samples events at a specified rate (0.0 to 1.0).
type SamplingFilter[T any] struct {
	rate    float64
	counter uint64
}

// NewSamplingFilter creates a filter that samples at the given rate.
// Rate=0.1 means 10% of events pass through.
func NewSamplingFilter[T any](rate float64) *SamplingFilter[T] {
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	return &SamplingFilter[T]{rate: rate}
}

func (f *SamplingFilter[T]) Filter(ev Event[T]) (Event[T], bool) {
	if f.rate >= 1.0 {
		return ev, true // Pass all
	}
	if f.rate <= 0.0 {
		return ev, false // Block all
	}

	// Deterministic sampling based on counter
	f.counter++
	// Use modulo for deterministic sampling
	threshold := uint64(1.0 / f.rate)
	return ev, (f.counter % threshold) == 0
}

// DeduplicationFilter drops duplicate events within a time window.
type DeduplicationFilter[T comparable] struct {
	seen   map[T]time.Time
	window time.Duration
	mu     sync.Mutex
}

// NewDeduplicationFilter creates a filter that drops duplicate values.
func NewDeduplicationFilter[T comparable](window time.Duration) *DeduplicationFilter[T] {
	return &DeduplicationFilter[T]{
		seen:   make(map[T]time.Time),
		window: window,
	}
}

func (f *DeduplicationFilter[T]) Filter(ev Event[T]) (Event[T], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()

	// Clean expired entries periodically
	if len(f.seen) > 1000 {
		for value, seenAt := range f.seen {
			if now.Sub(seenAt) > f.window {
				delete(f.seen, value)
			}
		}
	}

	// Check if we've seen this value recently
	if seenAt, exists := f.seen[ev.Value]; exists {
		if now.Sub(seenAt) < f.window {
			return ev, false // Duplicate - drop it
		}
	}

	// Record this value
	f.seen[ev.Value] = now
	return ev, true
}

// ThresholdFilter only passes events where the value exceeds a threshold.
type ThresholdFilter struct {
	threshold time.Duration
}

// NewThresholdFilter creates a filter for time.Duration values.
func NewThresholdFilter(threshold time.Duration) *ThresholdFilter {
	return &ThresholdFilter{threshold: threshold}
}

func (f *ThresholdFilter) Filter(ev Event[time.Duration]) (Event[time.Duration], bool) {
	return ev, ev.Value >= f.threshold
}

// RateLimitFilter limits the rate of events per second.
type RateLimitFilter[T any] struct {
	lastRefill   time.Time
	mu           sync.Mutex
	maxPerSecond int
	bucket       int
}

// NewRateLimitFilter creates a token bucket rate limiter.
func NewRateLimitFilter[T any](maxPerSecond int) *RateLimitFilter[T] {
	return &RateLimitFilter[T]{
		maxPerSecond: maxPerSecond,
		bucket:       maxPerSecond,
		lastRefill:   time.Now(),
	}
}

func (f *RateLimitFilter[T]) Filter(ev Event[T]) (Event[T], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(f.lastRefill)

	// Refill bucket based on time elapsed
	tokensToAdd := int(elapsed.Seconds() * float64(f.maxPerSecond))
	if tokensToAdd > 0 {
		f.bucket += tokensToAdd
		if f.bucket > f.maxPerSecond {
			f.bucket = f.maxPerSecond
		}
		f.lastRefill = now
	}

	// Try to consume a token
	if f.bucket > 0 {
		f.bucket--
		return ev, true
	}

	return ev, false
}

// PartitionFilter routes events to specific observers based on a hash function.
// This enables load distribution across multiple observers.
type PartitionFilter[T any] struct {
	partitions int
	partition  int
}

// NewPartitionFilter creates a filter for distributed processing.
// Only passes events that hash to the specified partition.
func NewPartitionFilter[T any](partitions, partition int) *PartitionFilter[T] {
	return &PartitionFilter[T]{
		partitions: partitions,
		partition:  partition,
	}
}

func (f *PartitionFilter[T]) Filter(ev Event[T]) (Event[T], bool) {
	// Hash the observer to determine partition
	h := fnv.New32a()
	// Use observer address as hash input
	targetPartition := int(h.Sum32()) % f.partitions
	return ev, targetPartition == f.partition
}

// FilteredDispatcher wraps a Dispatcher with event filtering.
type FilteredDispatcher[T any] struct {
	dispatcher *Dispatcher[T]
	filters    FilterChain[T]
	filtered   uint64 // Count of filtered events
}

// NewFilteredDispatcher creates a dispatcher with event filters.
func NewFilteredDispatcher[T any](
	ctx context.Context,
	bufSize int,
	filters ...EventFilter[T],
) *FilteredDispatcher[T] {
	return &FilteredDispatcher[T]{
		dispatcher: NewDispatcher[T](ctx, bufSize),
		filters:    FilterChain[T](filters),
	}
}

// Emit applies filters before dispatching the event.
func (fd *FilteredDispatcher[T]) Emit(target Observer[T], value T) {
	ev := Event[T]{Target: target, Value: value}

	// Apply filter chain
	ev, ok := fd.filters.Apply(ev)
	if !ok {
		fd.filtered++
		return
	}

	// Pass to underlying dispatcher
	fd.dispatcher.Emit(ev.Target, ev.Value)
}

// DroppedCount returns events dropped by the underlying dispatcher.
func (fd *FilteredDispatcher[T]) DroppedCount() uint64 {
	return fd.dispatcher.DroppedCount()
}

// FilteredCount returns events blocked by filters.
func (fd *FilteredDispatcher[T]) FilteredCount() uint64 {
	return fd.filtered
}

// TotalCount returns all events submitted.
func (fd *FilteredDispatcher[T]) TotalCount() uint64 {
	return fd.dispatcher.TotalCount() + fd.filtered
}

// DropRate returns the percentage of events dropped or filtered.
func (fd *FilteredDispatcher[T]) DropRate() float64 {
	total := fd.TotalCount()
	if total == 0 {
		return 0
	}
	dropped := fd.DroppedCount() + fd.filtered
	return float64(dropped) / float64(total) * 100
}
