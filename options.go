package floodgate

// Option configures a latency tracker.
type Option func(*emaTracker)

// WithAlpha sets the EMA smoothing factor (0 < alpha < 1).
// Lower values produce smoother curves, higher values are more responsive.
// Values outside the valid range are clamped to [0.01, 0.99].
func WithAlpha(alpha float32) Option {
	// Clamp to valid range instead of panicking (production-safe)
	if alpha <= 0 {
		alpha = 0.01
	}
	if alpha >= 1 {
		alpha = 0.99
	}

	return func(t *emaTracker) {
		t.alpha = int64(alpha * scale)
		t.alphaComp = scale - t.alpha
	}
}

// WithWindowSize sets the number of EMA samples to retain for trend analysis.
// Values less than 4 are clamped to 4 (minimum for trend calculation).
func WithWindowSize(size int) Option {
	if size < 4 {
		size = 4
	}
	return func(t *emaTracker) {
		t.windowSize = size
		t.emaSlice = make([]int64, 0, size)
	}
}

// WithPercentiles enables percentile tracking with the specified sample buffer size.
// Values less than 10 are clamped to 10 (minimum for meaningful percentiles).
func WithPercentiles(sampleSize int) Option {
	if sampleSize < 10 {
		sampleSize = 10
	}
	return func(t *emaTracker) {
		t.percentileEnabled = true
		t.sampleSize = sampleSize
		t.samples = make([]int64, 0, sampleSize)
		t.sortBuffer = make([]int64, sampleSize)
	}
}
