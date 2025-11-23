package codel

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mushtruk/floodgate"
)

// resultChannelPool reuses result channels to reduce allocations.
var resultChannelPool = sync.Pool{
	New: func() interface{} {
		return make(chan QueueResult, 1)
	},
}

// QueuedRequest represents a request waiting in the queue with its entry timestamp.
// The sojourn time is calculated as (dequeue_time - EnqueueTime).
type QueuedRequest struct {
	EnqueueTime time.Time
	Ctx         context.Context
	Handler     func(context.Context) error
	Result      chan<- QueueResult
}

// QueueResult carries the handler execution result along with measured sojourn time.
type QueueResult struct {
	Error       error
	SojournTime time.Duration
	Rejected    bool
}

// QueueAlgorithm implements CoDel with true queue-based sojourn time measurement.
// Unlike the standard Algorithm which uses P95 latency as a proxy, this implementation
// maintains an explicit queue and measures actual time requests spend waiting.
//
// This is the "correct" CoDel implementation as described in the original paper:
// "Controlling Queue Delay" by Nichols & Jacobson (2012).
//
// Example usage:
//
//	queueAlgo := codel.NewQueueAlgorithm(
//	    codel.WithTargetDelay(5*time.Millisecond),
//	    codel.WithWorkers(100),
//	    codel.WithQueueSize(1000),
//	)
//	queueAlgo.Start()
//	defer queueAlgo.Stop()
type QueueAlgorithm struct {
	queue    chan QueuedRequest
	codel    *Algorithm
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Stats for observability
	enqueued  atomic.Uint64
	rejected  atomic.Uint64
	queueFull atomic.Uint64
	started   atomic.Bool
	workers   int
}

// QueueOption configures the QueueAlgorithm.
type QueueOption func(*QueueAlgorithm)

// WithWorkers sets the number of worker goroutines processing the queue.
// Default is 100. More workers = higher concurrency but more goroutines.
func WithWorkers(n int) QueueOption {
	return func(q *QueueAlgorithm) {
		q.workers = n
	}
}

// WithQueueSize sets the queue buffer size.
// Default is 1000. When full, new requests are immediately rejected.
func WithQueueSize(n int) QueueOption {
	return func(q *QueueAlgorithm) {
		q.queue = make(chan QueuedRequest, n)
	}
}

// NewQueueAlgorithm creates a queue-based CoDel algorithm.
// Accepts the same CoDel options as NewAlgorithm (WithTargetDelay, WithInterval)
// plus queue-specific options (WithWorkers, WithQueueSize).
//
// You must call Start() to begin processing requests and Stop() when done.
func NewQueueAlgorithm(opts ...interface{}) *QueueAlgorithm {
	q := &QueueAlgorithm{
		queue:    make(chan QueuedRequest, 1000),
		workers:  100,
		stopChan: make(chan struct{}),
	}

	// Separate CoDel options from queue options
	var codelOpts []Option
	for _, opt := range opts {
		switch o := opt.(type) {
		case Option:
			codelOpts = append(codelOpts, o)
		case QueueOption:
			o(q)
		}
	}

	// Create underlying CoDel algorithm
	q.codel = NewAlgorithm(codelOpts...)

	return q
}

// Start begins processing queued requests with the configured worker pool.
// This must be called before enqueuing any requests.
func (q *QueueAlgorithm) Start() {
	if !q.started.CompareAndSwap(false, true) {
		return // Already started
	}

	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker()
	}
}

// Stop gracefully shuts down the worker pool.
// It waits for all in-flight requests to complete.
func (q *QueueAlgorithm) Stop() {
	if !q.started.Load() {
		return // Not started
	}

	close(q.stopChan)
	q.wg.Wait()
	q.started.Store(false)
}

// Queue returns the request queue channel for enqueuing new requests.
// Use select with default to detect queue-full condition:
//
//	select {
//	case q.Queue() <- req:
//	    // Enqueued successfully
//	default:
//	    // Queue full
//	}
func (q *QueueAlgorithm) Queue() chan<- QueuedRequest {
	return q.queue
}

// Stats returns current queue statistics for observability.
func (q *QueueAlgorithm) Stats() (enqueued, rejected, queueFull uint64) {
	return q.enqueued.Load(), q.rejected.Load(), q.queueFull.Load()
}

// worker is the main processing loop for each worker goroutine.
// It dequeues requests, measures sojourn time, applies CoDel, and executes handlers.
func (q *QueueAlgorithm) worker() {
	defer q.wg.Done()

	for {
		select {
		case <-q.stopChan:
			return

		case req := <-q.queue:
			q.processRequest(req)
		}
	}
}

// processRequest handles a single queued request:
// 1. Measure sojourn time (now - enqueue_time)
// 2. Apply CoDel decision based on sojourn time
// 3. If rejected: send error immediately
// 4. If accepted: execute handler and send result
func (q *QueueAlgorithm) processRequest(req QueuedRequest) {
	// Measure true sojourn time
	sojournTime := time.Since(req.EnqueueTime)

	// Use CoDel to make drop decision based on sojourn time
	// We use P95 field as a carrier for sojourn time since CoDel logic is the same
	stats := floodgate.Stats{
		P95: sojournTime,
		EMA: sojournTime, // Fallback if P95 logic changes
	}
	decision := q.codel.Decide(stats)

	if decision.Reject {
		// CoDel decided to drop this request
		q.rejected.Add(1)
		req.Result <- QueueResult{
			Error:       floodgate.ErrBackpressure,
			SojournTime: sojournTime,
			Rejected:    true,
		}
		return
	}

	// Execute handler
	err := req.Handler(req.Ctx)

	req.Result <- QueueResult{
		Error:       err,
		SojournTime: sojournTime,
		Rejected:    false,
	}
}

// Enqueue is a helper method that enqueues a request and waits for the result.
// Returns an error if the queue is full or if the handler fails.
//
// This is a convenience wrapper around Queue() channel for simpler integration.
func (q *QueueAlgorithm) Enqueue(ctx context.Context, handler func(context.Context) error) error {
	if !q.started.Load() {
		return floodgate.ErrBackpressure // Not started
	}

	// Check context before doing any work
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Get channel from pool to reduce allocations
	resultChanRaw := resultChannelPool.Get()
	resultChan, ok := resultChanRaw.(chan QueueResult)
	if !ok {
		panic("resultChannelPool returned unexpected type")
	}

	req := QueuedRequest{
		EnqueueTime: time.Now(),
		Ctx:         ctx,
		Handler:     handler,
		Result:      resultChan,
	}

	// Try to enqueue with context cancellation
	select {
	case q.queue <- req:
		q.enqueued.Add(1)
		// Enqueued successfully, wait for result
		result := <-resultChan
		// Return channel to pool for reuse
		resultChannelPool.Put(resultChan)
		return result.Error

	case <-ctx.Done():
		// Return unused channel to pool
		resultChannelPool.Put(resultChan)
		return ctx.Err()

	default:
		// Queue full - immediate rejection
		// Return unused channel to pool
		resultChannelPool.Put(resultChan)
		q.queueFull.Add(1)
		return floodgate.ErrBackpressure
	}
}
