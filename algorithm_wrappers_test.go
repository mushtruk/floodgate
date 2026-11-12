package floodgate

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingAlgorithm struct {
	decision Decision
	calls    atomic.Uint64
}

func (a *countingAlgorithm) Decide(Stats) Decision {
	a.calls.Add(1)
	return a.decision
}

func TestCachedAlgorithm_DecideReachesInnerAlgorithmEveryTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		decision Decision
	}{
		{name: "admit", decision: Decision{Level: Normal}},
		{name: "reject", decision: Decision{Level: Critical, Reject: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inner := &countingAlgorithm{decision: tt.decision}
			algo := NewCachedAlgorithm(inner, time.Minute)
			stats := Stats{EMA: 10 * time.Millisecond, P95: 20 * time.Millisecond}

			const calls = 5
			for i := 0; i < calls; i++ {
				if got := algo.Decide(stats); got != tt.decision {
					t.Fatalf("Decide() = %+v, want %+v", got, tt.decision)
				}
			}

			if got := inner.calls.Load(); got != calls {
				t.Errorf("inner calls = %d, want %d", got, calls)
			}
			hits, misses := algo.CacheStats()
			if hits != 0 || misses != calls {
				t.Errorf("CacheStats() = (%d, %d), want (0, %d)", hits, misses, calls)
			}
		})
	}
}

func TestCachedAlgorithm_ConcurrentDecide(t *testing.T) {
	t.Parallel()

	const (
		goroutines = 8
		calls      = 1000
		total      = goroutines * calls
	)

	inner := &countingAlgorithm{decision: Decision{Level: Warning}}
	algo := NewCachedAlgorithm(inner, time.Minute)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < calls; i++ {
				algo.Decide(Stats{EMA: time.Duration(g*calls + i)})
			}
		}()
	}
	wg.Wait()

	if got := inner.calls.Load(); got != total {
		t.Errorf("inner calls = %d, want %d", got, total)
	}
	hits, misses := algo.CacheStats()
	if hits != 0 || misses != total {
		t.Errorf("CacheStats() = (%d, %d), want (0, %d)", hits, misses, total)
	}
}
