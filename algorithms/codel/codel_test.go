package codel

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mushtruk/floodgate"
)

func TestNewAlgorithm_Defaults(t *testing.T) {
	t.Parallel()

	algo := NewAlgorithm()

	if algo.targetDelay != 5*time.Millisecond {
		t.Errorf("NewAlgorithm() targetDelay = %v, want %v", algo.targetDelay, 5*time.Millisecond)
	}

	if algo.interval != 100*time.Millisecond {
		t.Errorf("NewAlgorithm() interval = %v, want %v", algo.interval, 100*time.Millisecond)
	}

	if algo.dropping {
		t.Error("NewAlgorithm() dropping = true, want false")
	}

	if algo.count != 0 {
		t.Errorf("NewAlgorithm() count = %d, want 0", algo.count)
	}
}

func TestNewAlgorithm_CustomOptions(t *testing.T) {
	t.Parallel()

	targetDelay := 10 * time.Millisecond
	interval := 200 * time.Millisecond

	algo := NewAlgorithm(
		WithTargetDelay(targetDelay),
		WithInterval(interval),
	)

	if algo.targetDelay != targetDelay {
		t.Errorf("NewAlgorithm() targetDelay = %v, want %v", algo.targetDelay, targetDelay)
	}

	if algo.interval != interval {
		t.Errorf("NewAlgorithm() interval = %v, want %v", algo.interval, interval)
	}
}

func TestAlgorithm_InitialState_NoRejection(t *testing.T) {
	t.Parallel()

	algo := NewAlgorithm()

	// Low latency should not trigger rejection
	stats := floodgate.Stats{
		EMA: 1 * time.Millisecond,
		P95: 2 * time.Millisecond,
		P99: 3 * time.Millisecond,
	}

	decision := algo.Decide(stats)

	if decision.Reject {
		t.Error("Algorithm.Decide() with low latency Reject = true, want false")
	}

	if decision.Level != floodgate.Normal {
		t.Errorf("Algorithm.Decide() with low latency Level = %v, want %v", decision.Level, floodgate.Normal)
	}
}

func TestAlgorithm_EntersDropping_PersistentDelay(t *testing.T) {
	t.Parallel()

	algo := NewAlgorithm(
		WithTargetDelay(5*time.Millisecond),
		WithInterval(100*time.Millisecond),
	)

	// High latency exceeding target
	stats := floodgate.Stats{
		EMA: 50 * time.Millisecond,
		P95: 60 * time.Millisecond,
		P99: 70 * time.Millisecond,
	}

	// First call - above target but not persistent yet
	// Manually set firstAbove to simulate time passing
	algo.mu.Lock()
	algo.firstAbove = time.Now().Add(-150 * time.Millisecond) // Past the interval
	algo.mu.Unlock()

	decision := algo.Decide(stats)

	// Should enter dropping mode and reject
	if !decision.Reject {
		t.Error("Algorithm.Decide() with persistent high latency Reject = false, want true")
	}

	if decision.Level != floodgate.Moderate {
		t.Errorf("Algorithm.Decide() entering dropping Level = %v, want %v", decision.Level, floodgate.Moderate)
	}

	algo.mu.Lock()
	dropping := algo.dropping
	count := algo.count
	algo.mu.Unlock()

	if !dropping {
		t.Error("Algorithm should be in dropping state after persistent delay")
	}

	if count != 1 {
		t.Errorf("Algorithm drop count = %d, want 1", count)
	}
}

func TestAlgorithm_ExitsDropping_DelayImproves(t *testing.T) {
	t.Parallel()

	algo := NewAlgorithm(
		WithTargetDelay(5*time.Millisecond),
		WithInterval(100*time.Millisecond),
	)

	// Set up dropping state
	algo.mu.Lock()
	algo.dropping = true
	atomic.StoreUint32(&algo.droppingFlag, 1)
	algo.count = 3
	algo.firstAbove = time.Now().Add(-200 * time.Millisecond)
	algo.mu.Unlock()

	// Now latency improves below target
	stats := floodgate.Stats{
		EMA: 2 * time.Millisecond,
		P95: 3 * time.Millisecond,
		P99: 4 * time.Millisecond,
	}

	decision := algo.Decide(stats)

	if decision.Reject {
		t.Error("Algorithm.Decide() after improvement Reject = true, want false")
	}

	algo.mu.Lock()
	dropping := algo.dropping
	firstAbove := algo.firstAbove
	algo.mu.Unlock()

	if dropping {
		t.Error("Algorithm should exit dropping state when delay improves")
	}

	if !firstAbove.IsZero() {
		t.Error("Algorithm should reset firstAbove when delay improves")
	}
}

func TestAlgorithm_ControlLaw_IncreasingDropRate(t *testing.T) {
	t.Parallel()

	algo := NewAlgorithm(
		WithInterval(100 * time.Millisecond),
	)

	now := time.Now()

	// Test control law for increasing drop counts
	tests := []struct {
		count        int
		wantDuration time.Duration // Approximate
	}{
		{count: 1, wantDuration: 100 * time.Millisecond},  // interval / sqrt(1) = 100ms
		{count: 4, wantDuration: 50 * time.Millisecond},   // interval / sqrt(4) = 50ms
		{count: 16, wantDuration: 25 * time.Millisecond},  // interval / sqrt(16) = 25ms
		{count: 100, wantDuration: 10 * time.Millisecond}, // interval / sqrt(100) = 10ms
	}

	for _, tt := range tests {
		algo.mu.Lock()
		algo.count = tt.count
		nextDrop := algo.controlLaw(now)
		algo.mu.Unlock()

		actualDuration := nextDrop.Sub(now)

		// Allow 1ms tolerance for floating point rounding
		if actualDuration < tt.wantDuration-time.Millisecond ||
			actualDuration > tt.wantDuration+time.Millisecond {
			t.Errorf("controlLaw(count=%d) duration = %v, want ~%v",
				tt.count, actualDuration, tt.wantDuration)
		}
	}
}

func TestAlgorithm_MapToLevel_RatioBased(t *testing.T) {
	t.Parallel()

	algo := NewAlgorithm(
		WithTargetDelay(5 * time.Millisecond),
	)

	tests := []struct {
		name        string
		sojournTime time.Duration
		wantLevel   floodgate.Level
	}{
		{
			name:        "normal - below target",
			sojournTime: 3 * time.Millisecond, // ratio = 0.6
			wantLevel:   floodgate.Normal,
		},
		{
			name:        "normal - at target",
			sojournTime: 5 * time.Millisecond, // ratio = 1.0
			wantLevel:   floodgate.Normal,
		},
		{
			name:        "warning - 1.5x target",
			sojournTime: 8 * time.Millisecond, // ratio = 1.6
			wantLevel:   floodgate.Warning,
		},
		{
			name:        "moderate - 3x target",
			sojournTime: 16 * time.Millisecond, // ratio = 3.2
			wantLevel:   floodgate.Moderate,
		},
		{
			name:        "critical - 5x target",
			sojournTime: 26 * time.Millisecond, // ratio = 5.2
			wantLevel:   floodgate.Critical,
		},
		{
			name:        "emergency - 10x target",
			sojournTime: 51 * time.Millisecond, // ratio = 10.2
			wantLevel:   floodgate.Emergency,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			level := algo.mapToLevel(tt.sojournTime)

			if level != tt.wantLevel {
				t.Errorf("mapToLevel(%v) = %v, want %v",
					tt.sojournTime, level, tt.wantLevel)
			}
		})
	}
}

func TestAlgorithm_UsesP95_FallbackToEMA(t *testing.T) {
	t.Parallel()

	algo := NewAlgorithm(
		WithTargetDelay(5 * time.Millisecond),
	)

	tests := []struct {
		name      string
		stats     floodgate.Stats
		wantLevel floodgate.Level
	}{
		{
			name: "uses P95 when available",
			stats: floodgate.Stats{
				EMA: 100 * time.Millisecond, // Would be Emergency
				P95: 3 * time.Millisecond,   // Normal
				P99: 200 * time.Millisecond,
			},
			wantLevel: floodgate.Normal, // Should use P95
		},
		{
			name: "falls back to EMA when P95 is zero",
			stats: floodgate.Stats{
				EMA: 8 * time.Millisecond, // Warning
				P95: 0,                    // Zero
				P99: 200 * time.Millisecond,
			},
			wantLevel: floodgate.Warning, // Should use EMA
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			decision := algo.Decide(tt.stats)

			if decision.Level != tt.wantLevel {
				t.Errorf("Algorithm.Decide() Level = %v, want %v",
					decision.Level, tt.wantLevel)
			}
		})
	}
}

func TestAlgorithm_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	algo := NewAlgorithm()

	stats := floodgate.Stats{
		EMA: 10 * time.Millisecond,
		P95: 15 * time.Millisecond,
		P99: 20 * time.Millisecond,
	}

	var wg sync.WaitGroup
	concurrency := 100

	// Run concurrent Decide calls
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			_ = algo.Decide(stats)
		}()
	}

	wg.Wait()

	// If we get here without a data race, the test passes
}

func TestAlgorithm_DroppingEpisode_MultipleDrops(t *testing.T) {
	t.Parallel()

	algo := NewAlgorithm(
		WithTargetDelay(5*time.Millisecond),
		WithInterval(100*time.Millisecond),
	)

	// Set up persistent high latency
	stats := floodgate.Stats{
		EMA: 50 * time.Millisecond,
		P95: 60 * time.Millisecond,
		P99: 70 * time.Millisecond,
	}

	// Simulate time has passed beyond interval
	algo.mu.Lock()
	algo.firstAbove = time.Now().Add(-150 * time.Millisecond)
	algo.mu.Unlock()

	// First drop - enter dropping mode
	decision1 := algo.Decide(stats)
	if !decision1.Reject {
		t.Error("First decision should reject")
	}

	// Simulate time passing (wait for next drop time)
	algo.mu.Lock()
	algo.dropNext = time.Now().Add(-1 * time.Millisecond) // Make it past drop time
	algo.mu.Unlock()

	// Second drop - should increment count
	decision2 := algo.Decide(stats)
	if !decision2.Reject {
		t.Error("Second decision should reject")
	}

	algo.mu.Lock()
	count := algo.count
	algo.mu.Unlock()

	if count != 2 {
		t.Errorf("After 2 drops, count = %d, want 2", count)
	}
}

// BenchmarkCoDel_Decide benchmarks the CoDel algorithm decision performance.
func BenchmarkCoDel_Decide(b *testing.B) {
	algo := NewAlgorithm()

	stats := floodgate.Stats{
		EMA: 10 * time.Millisecond,
		P95: 15 * time.Millisecond,
		P99: 20 * time.Millisecond,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = algo.Decide(stats)
	}
}

// BenchmarkCoDel_Decide_HighLatency benchmarks CoDel with high latency (dropping mode).
func BenchmarkCoDel_Decide_HighLatency(b *testing.B) {
	algo := NewAlgorithm()

	// Simulate being in dropping mode
	algo.mu.Lock()
	algo.dropping = true
	atomic.StoreUint32(&algo.droppingFlag, 1)
	algo.count = 5
	algo.firstAbove = time.Now().Add(-200 * time.Millisecond)
	algo.dropNext = time.Now().Add(50 * time.Millisecond)
	algo.mu.Unlock()

	stats := floodgate.Stats{
		EMA: 50 * time.Millisecond,
		P95: 60 * time.Millisecond,
		P99: 70 * time.Millisecond,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = algo.Decide(stats)
	}
}

// BenchmarkCoDel_ControlLaw benchmarks the control law calculation.
func BenchmarkCoDel_ControlLaw(b *testing.B) {
	algo := NewAlgorithm()
	now := time.Now()

	algo.mu.Lock()
	algo.count = 10
	algo.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		algo.mu.Lock()
		_ = algo.controlLaw(now)
		algo.mu.Unlock()
	}
}

// BenchmarkCoDel_MapToLevel benchmarks level mapping.
func BenchmarkCoDel_MapToLevel(b *testing.B) {
	algo := NewAlgorithm()
	sojournTime := 15 * time.Millisecond

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = algo.mapToLevel(sojournTime)
	}
}

// BenchmarkCoDel_vs_Threshold compares CoDel and Threshold algorithm performance.
func BenchmarkCoDel_vs_Threshold(b *testing.B) {
	stats := floodgate.Stats{
		EMA: 100 * time.Millisecond,
		P95: 150 * time.Millisecond,
		P99: 200 * time.Millisecond,
	}

	b.Run("CoDel", func(b *testing.B) {
		algo := NewAlgorithm()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = algo.Decide(stats)
		}
	})

	b.Run("Threshold", func(b *testing.B) {
		algo := floodgate.NewThresholdAlgorithm(floodgate.DefaultThresholds())
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = algo.Decide(stats)
		}
	})
}

// BenchmarkCoDel_Allocation benchmarks memory allocation.
func BenchmarkCoDel_Allocation(b *testing.B) {
	algo := NewAlgorithm()

	stats := floodgate.Stats{
		EMA: 10 * time.Millisecond,
		P95: 15 * time.Millisecond,
		P99: 20 * time.Millisecond,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decision := algo.Decide(stats)
		_ = decision
	}
}
