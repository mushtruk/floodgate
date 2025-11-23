package codel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mushtruk/floodgate"
)

func TestNewQueueAlgorithm_Defaults(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm()

	if q.workers != 100 {
		t.Errorf("NewQueueAlgorithm() workers = %d, want 100", q.workers)
	}

	if cap(q.queue) != 1000 {
		t.Errorf("NewQueueAlgorithm() queue capacity = %d, want 1000", cap(q.queue))
	}

	if q.codel == nil {
		t.Error("NewQueueAlgorithm() codel is nil")
	}
}

func TestNewQueueAlgorithm_CustomOptions(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm(
		WithWorkers(50),
		WithQueueSize(500),
		WithTargetDelay(10*time.Millisecond),
		WithInterval(200*time.Millisecond),
	)

	if q.workers != 50 {
		t.Errorf("NewQueueAlgorithm() workers = %d, want 50", q.workers)
	}

	if cap(q.queue) != 500 {
		t.Errorf("NewQueueAlgorithm() queue capacity = %d, want 500", cap(q.queue))
	}

	if q.codel.targetDelay != 10*time.Millisecond {
		t.Errorf("NewQueueAlgorithm() targetDelay = %v, want 10ms", q.codel.targetDelay)
	}
}

func TestQueueAlgorithm_StartStop(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm(WithWorkers(5))

	// Should not be started initially
	if q.started.Load() {
		t.Error("QueueAlgorithm should not be started initially")
	}

	q.Start()

	if !q.started.Load() {
		t.Error("QueueAlgorithm should be started after Start()")
	}

	// Double start should be safe
	q.Start()

	if !q.started.Load() {
		t.Error("QueueAlgorithm should still be started after double Start()")
	}

	q.Stop()

	if q.started.Load() {
		t.Error("QueueAlgorithm should not be started after Stop()")
	}

	// Double stop should be safe
	q.Stop()
}

func TestQueueAlgorithm_Enqueue_Success(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm(
		WithWorkers(10),
		WithTargetDelay(100*time.Millisecond), // High threshold
	)
	q.Start()
	defer q.Stop()

	ctx := context.Background()
	executed := atomic.Bool{}

	handler := func(ctx context.Context) error {
		executed.Store(true)
		return nil
	}

	err := q.Enqueue(ctx, handler)

	if err != nil {
		t.Errorf("Enqueue() error = %v, want nil", err)
	}

	// Give worker time to execute
	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("Handler was not executed")
	}

	enqueued, rejected, queueFull := q.Stats()
	if enqueued != 1 {
		t.Errorf("Stats() enqueued = %d, want 1", enqueued)
	}
	if rejected != 0 {
		t.Errorf("Stats() rejected = %d, want 0", rejected)
	}
	if queueFull != 0 {
		t.Errorf("Stats() queueFull = %d, want 0", queueFull)
	}
}

func TestQueueAlgorithm_Enqueue_HandlerError(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm(WithWorkers(10))
	q.Start()
	defer q.Stop()

	ctx := context.Background()
	testErr := errors.New("handler error")

	handler := func(ctx context.Context) error {
		return testErr
	}

	err := q.Enqueue(ctx, handler)

	if !errors.Is(err, testErr) {
		t.Errorf("Enqueue() error = %v, want %v", err, testErr)
	}
}

func TestQueueAlgorithm_Enqueue_QueueFull(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm(
		WithWorkers(1),
		WithQueueSize(2),
	)
	q.Start()
	defer q.Stop()

	ctx := context.Background()
	blockChan := make(chan struct{})

	// Fill the queue
	blocker := func(ctx context.Context) error {
		select {
		case <-blockChan:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Enqueue blocking requests to fill queue
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = q.Enqueue(ctx, blocker)
		}()
	}

	// Give time for requests to enter queue
	time.Sleep(50 * time.Millisecond)

	// Try to enqueue when queue is full
	err := q.Enqueue(ctx, func(ctx context.Context) error {
		return nil
	})

	if !errors.Is(err, floodgate.ErrBackpressure) {
		t.Errorf("Enqueue() with full queue error = %v, want ErrBackpressure", err)
	}

	// Unblock and cleanup
	close(blockChan)
	wg.Wait()

	_, _, queueFull := q.Stats()
	if queueFull == 0 {
		t.Error("Stats() queueFull should be > 0 after queue full rejection")
	}
}

func TestQueueAlgorithm_Enqueue_NotStarted(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm()
	// Don't start

	ctx := context.Background()
	err := q.Enqueue(ctx, func(ctx context.Context) error {
		return nil
	})

	if !errors.Is(err, floodgate.ErrBackpressure) {
		t.Errorf("Enqueue() when not started error = %v, want ErrBackpressure", err)
	}
}

func TestQueueAlgorithm_Enqueue_ContextCanceled(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm(
		WithWorkers(1),
		WithQueueSize(1),
	)
	q.Start()
	defer q.Stop()

	// Test 1: Context already canceled before Enqueue
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := q.Enqueue(ctx, func(ctx context.Context) error {
		t.Error("Handler should not be called with canceled context")
		return nil
	})

	// Since queue is empty and context is already done, ctx.Done() case should trigger
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Enqueue() with canceled context error = %v, want context.Canceled", err)
	}

	// Test 2: Context canceled while handler executes
	cancelChan := make(chan struct{})
	ctx2, cancel2 := context.WithCancel(context.Background())

	go func() {
		<-cancelChan
		cancel2()
	}()

	executed := false
	err2 := q.Enqueue(ctx2, func(ctx context.Context) error {
		executed = true
		close(cancelChan)
		// Handler completes normally even if context canceled during execution
		return nil
	})

	if err2 != nil {
		t.Errorf("Enqueue() error = %v, want nil (handler completes)", err2)
	}

	if !executed {
		t.Error("Handler should have executed")
	}
}

func TestQueueAlgorithm_CoDelRejects_HighSojournTime(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm(
		WithWorkers(1), // Single worker to create backlog
		WithQueueSize(100),
		WithTargetDelay(1*time.Millisecond), // Very low threshold
		WithInterval(10*time.Millisecond),
	)
	q.Start()
	defer q.Stop()

	// Create artificial backlog by making handler slow
	handler := func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond) // Slow handler
		return nil
	}

	// Enqueue many requests to build up queue
	var rejections atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := q.Enqueue(context.Background(), handler)
			if errors.Is(err, floodgate.ErrBackpressure) {
				rejections.Add(1)
			}
		}()
		time.Sleep(1 * time.Millisecond) // Steady stream
	}

	wg.Wait()

	// With high sojourn time and low threshold, CoDel should reject some
	if rejections.Load() == 0 {
		t.Error("Expected CoDel to reject some requests due to high sojourn time, but got 0 rejections")
	}

	t.Logf("CoDel rejected %d requests out of 50", rejections.Load())

	_, rejected, _ := q.Stats()
	if rejected == 0 {
		t.Error("Stats() rejected should be > 0")
	}
}

func TestQueueAlgorithm_LowLatency_NoRejections(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm(
		WithWorkers(50), // Many workers
		WithTargetDelay(5*time.Millisecond),
	)
	q.Start()
	defer q.Stop()

	// Fast handler
	handler := func(ctx context.Context) error {
		return nil
	}

	// Enqueue many requests
	var rejections atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := q.Enqueue(context.Background(), handler)
			if errors.Is(err, floodgate.ErrBackpressure) {
				rejections.Add(1)
			}
		}()
	}

	wg.Wait()

	// With low sojourn time, CoDel should not reject
	if rejections.Load() > 0 {
		t.Errorf("Expected no rejections with low latency, but got %d rejections", rejections.Load())
	}

	_, rejected, _ := q.Stats()
	if rejected > 0 {
		t.Errorf("Stats() rejected = %d, want 0", rejected)
	}
}

func TestQueueAlgorithm_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	q := NewQueueAlgorithm(
		WithWorkers(20),
		WithQueueSize(500),
	)
	q.Start()
	defer q.Stop()

	handler := func(ctx context.Context) error {
		time.Sleep(1 * time.Millisecond)
		return nil
	}

	var wg sync.WaitGroup
	concurrency := 100

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = q.Enqueue(context.Background(), handler)
		}()
	}

	wg.Wait()

	// If we get here without data races, the test passes
}

// BenchmarkQueueAlgorithm_Enqueue benchmarks the enqueue operation.
func BenchmarkQueueAlgorithm_Enqueue(b *testing.B) {
	q := NewQueueAlgorithm(
		WithWorkers(100),
		WithQueueSize(10000),
	)
	q.Start()
	defer q.Stop()

	handler := func(ctx context.Context) error {
		return nil
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.Enqueue(ctx, handler)
	}
}

// BenchmarkQueueAlgorithm_Throughput benchmarks throughput with concurrent requests.
func BenchmarkQueueAlgorithm_Throughput(b *testing.B) {
	q := NewQueueAlgorithm(
		WithWorkers(100),
		WithQueueSize(10000),
	)
	q.Start()
	defer q.Stop()

	handler := func(ctx context.Context) error {
		return nil
	}

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = q.Enqueue(ctx, handler)
		}
	})
}
