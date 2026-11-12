package counter_test

import (
	"math"
	"testing"

	"github.com/mushtruk/floodgate/internal/counter"
)

func TestDelta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		current, prev uint64
		want          int64
	}{
		{name: "advance", current: 15, prev: 10, want: 5},
		{name: "unchanged", current: 10, prev: 10, want: 0},
		{name: "first sample", current: 7, prev: 0, want: 7},
		{name: "reset", current: 3, prev: 10, want: 0},
		{name: "largest representable advance", current: math.MaxInt64, prev: 0, want: math.MaxInt64},
		{name: "advance beyond int64 saturates", current: math.MaxUint64, prev: 0, want: math.MaxInt64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := counter.Delta(tt.current, tt.prev); got != tt.want {
				t.Errorf("Delta(%d, %d) = %d, want %d", tt.current, tt.prev, got, tt.want)
			}
		})
	}
}
