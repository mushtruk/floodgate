package floodgate

// Level represents the severity of backpressure.
type Level int

const (
	Normal Level = iota
	Warning
	Moderate
	Critical
	Emergency
)

func (b Level) String() string {
	switch b {
	case Normal:
		return "normal"
	case Warning:
		return "warning"
	case Moderate:
		return "moderate"
	case Critical:
		return "critical"
	case Emergency:
		return "emergency"
	default:
		return "unknown"
	}
}

// Level calculates backpressure level using default thresholds.
func (stats Stats) Level() Level {
	return stats.LevelWithThresholds(DefaultThresholds())
}

// LevelWithThresholds calculates backpressure level using custom thresholds.
func (stats Stats) LevelWithThresholds(thresholds Thresholds) Level {
	ema := stats.EMA
	slope := stats.Slope

	// Use percentiles if available
	if stats.P95 > 0 && stats.P99 > 0 {
		switch {
		case stats.P99 > thresholds.P99Emergency:
			return Emergency
		case stats.P95 > thresholds.P95Critical && ema > thresholds.EMACritical:
			return Critical
		case stats.P95 > thresholds.P95Moderate:
			return Moderate
		case ema > thresholds.EMAWarning || slope > thresholds.SlopeWarning:
			return Warning
		}
	} else {
		// Fallback to slope-based detection
		switch {
		case slope > 5*thresholds.SlopeWarning/10:
			return Critical
		case slope > 3*thresholds.SlopeWarning/10:
			return Moderate
		case slope > thresholds.SlopeWarning/10:
			return Warning
		}
	}

	return Normal
}
