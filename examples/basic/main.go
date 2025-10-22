package main

import (
	"fmt"
	"time"

	"github.com/mushtruk/floodgate"
)

func main() {
	// Create a latency tracker with custom options
	tracker := floodgate.NewTracker(
		floodgate.WithAlpha(0.25),      // Moderate smoothing
		floodgate.WithWindowSize(30),   // 30-sample window
		floodgate.WithPercentiles(200), // Track percentiles with 200 samples (~3.2KB)
	)

	// Simulate baseline latency
	fmt.Println("Simulating baseline latency (100ms)...")
	for i := 0; i < 20; i++ {
		tracker.Process(100 * time.Millisecond)
	}

	stats := tracker.Value()
	fmt.Printf("Baseline - EMA: %v, P95: %v, Level: %s\n\n",
		stats.EMA, stats.P95, stats.Level())

	// Simulate latency spike
	fmt.Println("Simulating latency spike (500ms)...")
	for i := 0; i < 20; i++ {
		tracker.Process(500 * time.Millisecond)
	}

	stats = tracker.Value()
	fmt.Printf("Spike - EMA: %v, P95: %v, P99: %v, Slope: %v, Level: %s\n\n",
		stats.EMA, stats.P95, stats.P99, stats.Slope, stats.Level())

	// Simulate recovery
	fmt.Println("Simulating recovery (100ms)...")
	for i := 0; i < 20; i++ {
		tracker.Process(100 * time.Millisecond)
	}

	stats = tracker.Value()
	fmt.Printf("Recovery - EMA: %v, P95: %v, Slope: %v, Level: %s\n",
		stats.EMA, stats.P95, stats.Slope, stats.Level())

	// Custom thresholds example
	fmt.Println("\n--- Custom Thresholds ---")
	customThresholds := floodgate.Thresholds{
		P99Emergency: 2 * time.Second,
		P95Critical:  1 * time.Second,
		EMACritical:  300 * time.Millisecond,
		P95Moderate:  500 * time.Millisecond,
		EMAWarning:   200 * time.Millisecond,
		SlopeWarning: 5 * time.Millisecond,
	}

	level := stats.LevelWithThresholds(customThresholds)
	fmt.Printf("Custom thresholds level: %s\n", level)
}
