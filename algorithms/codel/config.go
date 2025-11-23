package codel

import (
	"time"

	"github.com/mushtruk/floodgate"
)

// AlgorithmType specifies which CoDel variant to use.
type AlgorithmType string

const (
	// AlgorithmTypeStandard uses P95 latency as sojourn time proxy.
	// Best for: CPU-bound services, low overhead (59 ns/op).
	// Trade-off: Not true sojourn time, may false-positive on I/O delays.
	AlgorithmTypeStandard AlgorithmType = "standard"

	// AlgorithmTypeQueue uses explicit queue with true sojourn time measurement.
	// Best for: I/O-bound services, accurate queue management.
	// Trade-off: Higher overhead (676 ns/op), requires worker pool.
	AlgorithmTypeQueue AlgorithmType = "queue"

	// AlgorithmTypeThreshold uses fixed latency thresholds (not CoDel).
	// Best for: Multi-signal backpressure, proven and battle-tested.
	// Trade-off: Requires threshold tuning per service.
	AlgorithmTypeThreshold AlgorithmType = "threshold"
)

// Config provides a unified way to configure any algorithm variant.
// This makes it easy to switch algorithms based on deployment needs
// without changing application code.
type Config struct {
	// Type selects which algorithm to use.
	Type AlgorithmType

	// CoDel settings (used by both Standard and Queue types)
	TargetDelay time.Duration // Default: 5ms
	Interval    time.Duration // Default: 100ms

	// Queue-specific settings (only used when Type = AlgorithmTypeQueue)
	Workers   int // Default: 100
	QueueSize int // Default: 1000

	// Threshold settings (only used when Type = AlgorithmTypeThreshold)
	Thresholds floodgate.Thresholds
}

// NewAlgorithmFromConfig creates the appropriate algorithm based on configuration.
// This allows algorithm selection via config files or environment variables.
//
// Example usage:
//
//	cfg := codel.Config{
//	    Type: codel.AlgorithmTypeQueue,  // or from env: os.Getenv("CODEL_TYPE")
//	    TargetDelay: 5 * time.Millisecond,
//	    Workers: 100,
//	    QueueSize: 1000,
//	}
//	algo := codel.NewAlgorithmFromConfig(cfg)
func NewAlgorithmFromConfig(cfg Config) interface{} {
	// Set defaults
	if cfg.TargetDelay == 0 {
		cfg.TargetDelay = 5 * time.Millisecond
	}
	if cfg.Interval == 0 {
		cfg.Interval = 100 * time.Millisecond
	}
	if cfg.Workers == 0 {
		cfg.Workers = 100
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 1000
	}

	switch cfg.Type {
	case AlgorithmTypeStandard:
		return NewAlgorithm(
			WithTargetDelay(cfg.TargetDelay),
			WithInterval(cfg.Interval),
		)

	case AlgorithmTypeQueue:
		return NewQueueAlgorithm(
			WithTargetDelay(cfg.TargetDelay),
			WithInterval(cfg.Interval),
			WithWorkers(cfg.Workers),
			WithQueueSize(cfg.QueueSize),
		)

	case AlgorithmTypeThreshold:
		thresholds := cfg.Thresholds
		if thresholds.EMAWarning == 0 {
			thresholds = floodgate.DefaultThresholds()
		}
		return floodgate.NewThresholdAlgorithm(thresholds)

	default:
		// Default to standard CoDel
		return NewAlgorithm(
			WithTargetDelay(cfg.TargetDelay),
			WithInterval(cfg.Interval),
		)
	}
}

// RecommendAlgorithmType suggests the best algorithm based on service characteristics.
func RecommendAlgorithmType(cpuBound bool, avgRequestTime time.Duration) AlgorithmType {
	if cpuBound {
		// CPU-bound: Standard CoDel works well, low overhead
		return AlgorithmTypeStandard
	}

	if avgRequestTime > 100*time.Millisecond {
		// I/O-bound with high latency: Queue CoDel for true sojourn time
		return AlgorithmTypeQueue
	}

	// Default: Threshold algorithm (proven, multi-signal)
	return AlgorithmTypeThreshold
}
