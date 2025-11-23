package floodgate

// Algorithm determines whether to shed load based on current statistics.
// This allows different backpressure strategies to be plugged in at runtime,
// similar to the pluggable Logger and MetricsCollector interfaces.
//
// Example with default threshold-based:
//
//	cfg.Algorithm = floodgate.NewThresholdAlgorithm(
//	    floodgate.DefaultThresholds(),
//	)
//
// Example with adaptive CoDel:
//
//	cfg.Algorithm = codel.NewAlgorithm(
//	    codel.WithTargetDelay(5*time.Millisecond),
//	)
//
// To implement custom algorithms:
//
//	type MyAlgorithm struct {}
//	func (a *MyAlgorithm) Decide(stats Stats) Decision {
//	    // Your logic here
//	    return Decision{Level: level, Reject: shouldReject}
//	}
//	cfg.Algorithm = &MyAlgorithm{}
type Algorithm interface {
	// Decide determines whether to shed load and at what backpressure level.
	// This is called for every request after latency statistics are computed.
	Decide(stats Stats) Decision
}

// Decision represents an algorithm's backpressure decision for a single request.
type Decision struct {
	// Level indicates the severity of backpressure (Normal to Emergency).
	Level Level

	// Reject indicates whether the request should be rejected.
	// When true, the middleware will return an error to the client.
	Reject bool
}

// NoOpAlgorithm never sheds load, always allows requests through.
// This is useful for testing, debugging, or temporarily disabling backpressure.
type NoOpAlgorithm struct{}

// Decide implements Algorithm.
func (NoOpAlgorithm) Decide(_ Stats) Decision {
	return Decision{Level: Normal, Reject: false}
}

// ThresholdAlgorithm uses fixed latency thresholds to determine backpressure levels.
// This is the default algorithm and provides predictable, easy-to-understand behavior.
//
// It works by comparing current latency metrics (EMA, percentiles, slope) against
// configured thresholds. When metrics exceed thresholds, it escalates the backpressure
// level and eventually starts rejecting requests at Emergency level.
type ThresholdAlgorithm struct {
	Thresholds Thresholds
}

// NewThresholdAlgorithm creates a threshold-based algorithm with the given thresholds.
func NewThresholdAlgorithm(thresholds Thresholds) *ThresholdAlgorithm {
	return &ThresholdAlgorithm{Thresholds: thresholds}
}

// Decide implements Algorithm using threshold-based logic.
func (a *ThresholdAlgorithm) Decide(stats Stats) Decision {
	level := stats.LevelWithThresholds(a.Thresholds)

	return Decision{
		Level:  level,
		Reject: level >= Emergency,
	}
}
