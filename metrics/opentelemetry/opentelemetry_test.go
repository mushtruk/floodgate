package opentelemetry_test

import (
	"context"
	"math"
	"testing"

	"github.com/mushtruk/floodgate/metrics/opentelemetry"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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

			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() {
				if err := provider.Shutdown(context.Background()); err != nil {
					t.Errorf("shut down meter provider: %v", err)
				}
			})
			m, err := opentelemetry.NewMetrics(provider.Meter("test"))
			if err != nil {
				t.Fatalf("NewMetrics: %v", err)
			}

			m.RecordDispatcherStats(tt.first.dropped, tt.first.total)
			dropsBefore, totalBefore := dispatcherCounters(t, reader)
			m.RecordDispatcherStats(tt.second.dropped, tt.second.total)
			dropsAfter, totalAfter := dispatcherCounters(t, reader)

			if got := dropsAfter - dropsBefore; got != tt.wantDrops {
				t.Errorf("drops increment = %d, want %d", got, tt.wantDrops)
			}
			if got := totalAfter - totalBefore; got != tt.wantTotal {
				t.Errorf("events increment = %d, want %d", got, tt.wantTotal)
			}
		})
	}
}

func dispatcherCounters(t *testing.T, r sdkmetric.Reader) (drops, total int64) {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := r.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, md := range sm.Metrics {
			sum, ok := md.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			var v int64
			for _, dp := range sum.DataPoints {
				v += dp.Value
			}
			switch md.Name {
			case "floodgate.dispatcher.drops":
				drops = v
			case "floodgate.dispatcher.events":
				total = v
			}
		}
	}
	return drops, total
}
