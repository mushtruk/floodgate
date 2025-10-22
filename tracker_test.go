package floodgate

import (
	"testing"
	"time"
)

func TestTracker_BasicUsage(t *testing.T) {
	tracker := NewTracker(
		WithAlpha(0.25),
		WithWindowSize(20),
		WithPercentiles(100),
	)

	// Add samples
	for i := 1; i <= 100; i++ {
		tracker.Process(time.Duration(i) * time.Millisecond)
	}

	stats := tracker.Value()

	if stats.EMA <= 0 {
		t.Errorf("Expected positive EMA, got %v", stats.EMA)
	}

	if stats.P95 <= 0 {
		t.Errorf("Expected positive P95, got %v", stats.P95)
	}

	if stats.P99 <= 0 {
		t.Errorf("Expected positive P99, got %v", stats.P99)
	}
}

func TestStats_Level(t *testing.T) {
	tests := []struct {
		name     string
		stats    Stats
		expected Level
	}{
		{
			name: "Normal",
			stats: Stats{
				EMA:   100 * time.Millisecond,
				P95:   200 * time.Millisecond,
				P99:   300 * time.Millisecond,
				Slope: 1 * time.Millisecond,
			},
			expected: Normal,
		},
		{
			name: "Emergency",
			stats: Stats{
				EMA:   1 * time.Second,
				P95:   5 * time.Second,
				P99:   11 * time.Second,
				Slope: 50 * time.Millisecond,
			},
			expected: Emergency,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := tt.stats.Level()
			if level != tt.expected {
				t.Errorf("Expected level %v, got %v", tt.expected, level)
			}
		})
	}
}

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{Normal, "normal"},
		{Warning, "warning"},
		{Moderate, "moderate"},
		{Critical, "critical"},
		{Emergency, "emergency"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("Level.String() = %v, want %v", got, tt.expected)
		}
	}
}

func BenchmarkTracker_Process(b *testing.B) {
	tracker := NewTracker(
		WithAlpha(0.25),
		WithWindowSize(20),
		WithPercentiles(1000),
	)

	latency := 100 * time.Millisecond

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.Process(latency)
	}
}

func BenchmarkTracker_Value(b *testing.B) {
	tracker := NewTracker(
		WithAlpha(0.25),
		WithWindowSize(20),
		WithPercentiles(1000),
	)

	// Prime with data
	for i := 0; i < 100; i++ {
		tracker.Process(100 * time.Millisecond)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = tracker.Value()
	}
}

// BenchmarkTracker_ValueWithLargePercentiles benchmarks Value() with large sample size
// This measures the optimization of using pre-allocated sortBuffer
func BenchmarkTracker_ValueWithLargePercentiles(b *testing.B) {
	tracker := NewTracker(
		WithAlpha(0.1),
		WithWindowSize(50),
		WithPercentiles(10000), // Large sample size
	)

	// Prime with full sample buffer
	for i := 0; i < 10000; i++ {
		tracker.Process(time.Duration(i%1000) * time.Microsecond)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = tracker.Value()
	}
}

// BenchmarkTracker_ValueNoPercentiles benchmarks Value() without percentile tracking
func BenchmarkTracker_ValueNoPercentiles(b *testing.B) {
	tracker := NewTracker(
		WithAlpha(0.25),
		WithWindowSize(20),
	)

	// Prime with data
	for i := 0; i < 100; i++ {
		tracker.Process(100 * time.Millisecond)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = tracker.Value()
	}
}

// BenchmarkTracker_ConcurrentProcessAndValue benchmarks concurrent Process and Value calls
func BenchmarkTracker_ConcurrentProcessAndValue(b *testing.B) {
	tracker := NewTracker(
		WithAlpha(0.1),
		WithWindowSize(50),
		WithPercentiles(1000),
	)

	// Prime with data
	for i := 0; i < 100; i++ {
		tracker.Process(100 * time.Millisecond)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				tracker.Process(100 * time.Millisecond)
			} else {
				_ = tracker.Value()
			}
			i++
		}
	})
}

// BenchmarkTracker_LevelWithThresholds benchmarks level calculation
func BenchmarkTracker_LevelWithThresholds(b *testing.B) {
	stats := Stats{
		EMA:   500 * time.Millisecond,
		P95:   1500 * time.Millisecond,
		P99:   3000 * time.Millisecond,
		Slope: 10 * time.Millisecond,
	}

	thresholds := DefaultThresholds()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = stats.LevelWithThresholds(thresholds)
	}
}
