package codel

import "errors"

// Configuration errors returned by NewAlgorithm and NewQueueAlgorithm.
var (
	// ErrInvalidTargetDelay is returned when targetDelay is zero or negative.
	ErrInvalidTargetDelay = errors.New("codel: targetDelay must be positive")

	// ErrInvalidInterval is returned when interval is zero or negative.
	ErrInvalidInterval = errors.New("codel: interval must be positive")
)
