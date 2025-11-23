package http

import (
	"time"

	"github.com/mushtruk/floodgate"
)

// Config holds configuration for the backpressure middleware.
type Config struct {
	// Logger for backpressure events. If nil, uses DefaultLogger.
	Logger floodgate.Logger
	// Metrics collector for observability. If nil, uses NoOpMetrics (disabled).
	Metrics floodgate.MetricsCollector
	// Algorithm determines backpressure decisions (optional).
	// If nil, uses ThresholdAlgorithm with cfg.Thresholds.
	//
	// Examples:
	//   cfg.Algorithm = nil  // Use default thresholds (backward compatible)
	//   cfg.Algorithm = floodgate.NewThresholdAlgorithm(customThresholds)
	//   cfg.Algorithm = codel.NewAlgorithm()
	Algorithm                      floodgate.Algorithm
	SkipPaths                      []string
	Thresholds                     floodgate.Thresholds
	CacheSize                      int
	CircuitBreakerMaxFailures      int
	MetricsInterval                time.Duration
	CacheTTL                       time.Duration
	DispatcherBufferSize           int
	TrackerWindowSize              int
	TrackerSampleSize              int
	CircuitBreakerTimeout          time.Duration
	CircuitBreakerSuccessThreshold int
	RetryAfterEmergency            int
	RetryAfterCritical             int
	RetryAfterCircuit              int
	TrackerAlpha                   float32
	EnableMetrics                  bool
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() Config {
	return Config{
		CacheSize:            512,
		CacheTTL:             2 * time.Minute,
		DispatcherBufferSize: 1024,
		Thresholds:           floodgate.DefaultThresholds(),
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/readiness",
		},
		EnableMetrics:   true,
		MetricsInterval: 1 * time.Minute,

		CircuitBreakerMaxFailures:      3,
		CircuitBreakerTimeout:          30 * time.Second,
		CircuitBreakerSuccessThreshold: 5,

		TrackerAlpha:      0.1,
		TrackerWindowSize: 50,
		TrackerSampleSize: 200,

		RetryAfterEmergency: 10,
		RetryAfterCritical:  5,
		RetryAfterCircuit:   30,

		Logger:    floodgate.NewDefaultLogger(),
		Metrics:   &floodgate.NoOpMetrics{}, // Disabled by default
		Algorithm: nil,                      // nil = use threshold-based (backward compatible)
	}
}
