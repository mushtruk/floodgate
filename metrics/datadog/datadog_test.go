package datadog_test

import (
	"math"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/mushtruk/floodgate/metrics/datadog"
)

// countRecorder sums the Count increments a collector sends, by metric name.
type countRecorder struct {
	statsd.NoOpClient
	counts map[string]int64
}

func (r *countRecorder) Count(name string, value int64, _ []string, _ float64) error {
	r.counts[name] += value
	return nil
}

type dispatcherSample struct {
	dropped uint64
	total   uint64
}

func TestRecordDispatcherStatsCountsOnlyAdvances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		first     dispatcherSample
		second    dispatcherSample
		wantDrops int64
		wantTotal int64
	}{
		{name: "advance", first: dispatcherSample{10, 20}, second: dispatcherSample{15, 30}, wantDrops: 5, wantTotal: 10},
		{name: "reset", first: dispatcherSample{10, 20}, second: dispatcherSample{3, 5}},
		{
			name:   "reset from beyond the int64 range",
			first:  dispatcherSample{math.MaxInt64 + 10, math.MaxInt64 + 20},
			second: dispatcherSample{3, 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := &countRecorder{counts: map[string]int64{}}
			m := datadog.NewMetrics(rec)

			m.RecordDispatcherStats(tt.first.dropped, tt.first.total)
			clear(rec.counts)
			m.RecordDispatcherStats(tt.second.dropped, tt.second.total)

			if got := rec.counts["floodgate.dispatcher.drops"]; got != tt.wantDrops {
				t.Errorf("drops increment = %d, want %d", got, tt.wantDrops)
			}
			if got := rec.counts["floodgate.dispatcher.events"]; got != tt.wantTotal {
				t.Errorf("events increment = %d, want %d", got, tt.wantTotal)
			}
		})
	}
}
