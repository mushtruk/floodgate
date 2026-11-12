package prometheus_test

import (
	"math"
	"testing"

	fgprometheus "github.com/mushtruk/floodgate/metrics/prometheus"
	"github.com/prometheus/client_golang/prometheus"
)

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
		wantDrops float64
		wantTotal float64
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

			reg := prometheus.NewRegistry()
			m := fgprometheus.NewMetrics(reg)

			m.RecordDispatcherStats(tt.first.dropped, tt.first.total)
			dropsBefore, totalBefore := dispatcherCounters(t, reg)
			m.RecordDispatcherStats(tt.second.dropped, tt.second.total)
			dropsAfter, totalAfter := dispatcherCounters(t, reg)

			if got := dropsAfter - dropsBefore; got != tt.wantDrops {
				t.Errorf("drops increment = %g, want %g", got, tt.wantDrops)
			}
			if got := totalAfter - totalBefore; got != tt.wantTotal {
				t.Errorf("events increment = %g, want %g", got, tt.wantTotal)
			}
		})
	}
}

func dispatcherCounters(t *testing.T, g prometheus.Gatherer) (drops, total float64) {
	t.Helper()

	families, err := g.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, f := range families {
		switch f.GetName() {
		case "floodgate_dispatcher_drops_total":
			drops = f.GetMetric()[0].GetCounter().GetValue()
		case "floodgate_dispatcher_events_total":
			total = f.GetMetric()[0].GetCounter().GetValue()
		}
	}
	return drops, total
}
