package floodgate

import "errors"

// ErrBackpressure is returned when a request is rejected due to backpressure.
// This can happen when the system is overloaded and needs to shed load to maintain
// acceptable latency for the requests it does process.
var ErrBackpressure = errors.New("request rejected due to backpressure")
