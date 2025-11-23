package floodgate

import (
	"testing"
	"time"
)

func TestNoOpAlgorithm_NeverRejects(t *testing.T) {
	t.Parallel()

	algo := &NoOpAlgorithm{}

	tests := []struct {
		name  string
		stats Stats
	}{
		{
			name: "zero latency",
			stats: Stats{
				EMA:   0,
				P95:   0,
				P99:   0,
				Slope: 0,
			},
		},
		{
			name: "high latency",
			stats: Stats{
				EMA:   500 * time.Millisecond,
				P95:   1 * time.Second,
				P99:   2 * time.Second,
				Slope: 100,
			},
		},
		{
			name: "extreme latency",
			stats: Stats{
				EMA:   10 * time.Second,
				P95:   30 * time.Second,
				P99:   60 * time.Second,
				Slope: 1000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			decision := algo.Decide(tt.stats)

			if decision.Reject {
				t.Errorf("NoOpAlgorithm.Decide() Reject = true, want false")
			}

			if decision.Level != Normal {
				t.Errorf("NoOpAlgorithm.Decide() Level = %v, want %v", decision.Level, Normal)
			}
		})
	}
}

func TestThresholdAlgorithm_RejectsAtEmergency(t *testing.T) {
	t.Parallel()

	thresholds := DefaultThresholds()
	algo := NewThresholdAlgorithm(thresholds)

	tests := []struct {
		name       string
		stats      Stats
		wantLevel  Level
		wantReject bool
	}{
		{
			name: "normal latency",
			stats: Stats{
				EMA:   10 * time.Millisecond,
				P95:   15 * time.Millisecond,
				P99:   20 * time.Millisecond,
				Slope: 0,
			},
			wantLevel:  Normal,
			wantReject: false,
		},
		{
			name: "warning latency - EMA threshold",
			stats: Stats{
				EMA:   310 * time.Millisecond, // > EMAWarning (300ms)
				P95:   350 * time.Millisecond,
				P99:   400 * time.Millisecond,
				Slope: 5 * time.Millisecond,
			},
			wantLevel:  Warning,
			wantReject: false,
		},
		{
			name: "warning latency - Slope threshold",
			stats: Stats{
				EMA:   100 * time.Millisecond,
				P95:   150 * time.Millisecond,
				P99:   200 * time.Millisecond,
				Slope: 11 * time.Millisecond, // > SlopeWarning (10ms)
			},
			wantLevel:  Warning,
			wantReject: false,
		},
		{
			name: "moderate latency - P95 threshold",
			stats: Stats{
				EMA:   200 * time.Millisecond,
				P95:   1100 * time.Millisecond, // > P95Moderate (1s)
				P99:   1500 * time.Millisecond,
				Slope: 5 * time.Millisecond,
			},
			wantLevel:  Moderate,
			wantReject: false,
		},
		{
			name: "critical latency - P95 and EMA both exceed",
			stats: Stats{
				EMA:   510 * time.Millisecond,  // > EMACritical (500ms)
				P95:   2100 * time.Millisecond, // > P95Critical (2s)
				P99:   3000 * time.Millisecond,
				Slope: 15 * time.Millisecond,
			},
			wantLevel:  Critical,
			wantReject: false,
		},
		{
			name: "emergency latency - P99 threshold",
			stats: Stats{
				EMA:   1 * time.Second,
				P95:   5 * time.Second,
				P99:   11 * time.Second, // > P99Emergency (10s)
				Slope: 30 * time.Millisecond,
			},
			wantLevel:  Emergency,
			wantReject: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			decision := algo.Decide(tt.stats)

			if decision.Level != tt.wantLevel {
				t.Errorf("ThresholdAlgorithm.Decide() Level = %v, want %v", decision.Level, tt.wantLevel)
			}

			if decision.Reject != tt.wantReject {
				t.Errorf("ThresholdAlgorithm.Decide() Reject = %v, want %v", decision.Reject, tt.wantReject)
			}
		})
	}
}

func TestThresholdAlgorithm_CustomThresholds(t *testing.T) {
	t.Parallel()

	// Create custom thresholds more aggressive than defaults
	customThresholds := Thresholds{
		P99Emergency: 80 * time.Millisecond,
		P95Critical:  60 * time.Millisecond,
		EMACritical:  40 * time.Millisecond,
		P95Moderate:  30 * time.Millisecond,
		EMAWarning:   20 * time.Millisecond,
		SlopeWarning: 2 * time.Millisecond,
	}

	algo := NewThresholdAlgorithm(customThresholds)

	// Stats that would be Normal with defaults, Emergency with custom
	stats := Stats{
		EMA:   85 * time.Millisecond,
		P95:   100 * time.Millisecond,
		P99:   150 * time.Millisecond,
		Slope: 5 * time.Millisecond,
	}

	decision := algo.Decide(stats)

	if decision.Level != Emergency {
		t.Errorf("ThresholdAlgorithm.Decide() with custom thresholds Level = %v, want %v", decision.Level, Emergency)
	}

	if !decision.Reject {
		t.Errorf("ThresholdAlgorithm.Decide() with custom thresholds Reject = false, want true")
	}
}

func TestDecision_StructFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		decision Decision
	}{
		{
			name: "normal no reject",
			decision: Decision{
				Level:  Normal,
				Reject: false,
			},
		},
		{
			name: "emergency with reject",
			decision: Decision{
				Level:  Emergency,
				Reject: true,
			},
		},
		{
			name: "warning no reject",
			decision: Decision{
				Level:  Warning,
				Reject: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Verify fields are accessible and match expectations
			if tt.decision.Level < Normal || tt.decision.Level > Emergency {
				t.Errorf("Decision.Level = %v is out of valid range", tt.decision.Level)
			}

			// Test zero value
			var zeroDecision Decision
			if zeroDecision.Level != Normal {
				t.Errorf("Zero Decision.Level = %v, want Normal (0)", zeroDecision.Level)
			}
			if zeroDecision.Reject {
				t.Error("Zero Decision.Reject = true, want false")
			}
		})
	}
}

// BenchmarkThresholdAlgorithm_Decide benchmarks the threshold algorithm decision performance.
func BenchmarkThresholdAlgorithm_Decide(b *testing.B) {
	thresholds := DefaultThresholds()
	algo := NewThresholdAlgorithm(thresholds)

	stats := Stats{
		EMA:   100 * time.Millisecond,
		P95:   150 * time.Millisecond,
		P99:   200 * time.Millisecond,
		Slope: 10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = algo.Decide(stats)
	}
}

// BenchmarkNoOpAlgorithm_Decide benchmarks the no-op algorithm decision performance.
func BenchmarkNoOpAlgorithm_Decide(b *testing.B) {
	algo := &NoOpAlgorithm{}

	stats := Stats{
		EMA:   100 * time.Millisecond,
		P95:   150 * time.Millisecond,
		P99:   200 * time.Millisecond,
		Slope: 10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = algo.Decide(stats)
	}
}

// BenchmarkAlgorithmInterface benchmarks using the algorithm through the interface.
func BenchmarkAlgorithmInterface(b *testing.B) {
	var algo Algorithm = NewThresholdAlgorithm(DefaultThresholds())

	stats := Stats{
		EMA:   100 * time.Millisecond,
		P95:   150 * time.Millisecond,
		P99:   200 * time.Millisecond,
		Slope: 10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = algo.Decide(stats)
	}
}

// BenchmarkDecisionAllocation benchmarks memory allocation for Decision.
func BenchmarkDecisionAllocation(b *testing.B) {
	algo := NewThresholdAlgorithm(DefaultThresholds())

	stats := Stats{
		EMA:   100 * time.Millisecond,
		P95:   150 * time.Millisecond,
		P99:   200 * time.Millisecond,
		Slope: 10,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decision := algo.Decide(stats)
		_ = decision
	}
}
