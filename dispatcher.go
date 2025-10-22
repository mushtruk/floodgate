package floodgate

import (
	"context"
	"log"
	"sync/atomic"
)

type Observer[T any] interface {
	Process(T)
}

type Event[T any] struct {
	Target Observer[T]
	Value  T
}

// Dispatcher asynchronously delivers values to observers.
type Dispatcher[T any] struct {
	inputCh      chan Event[T]
	droppedCount atomic.Uint64
	totalCount   atomic.Uint64
}

func NewDispatcher[T any](ctx context.Context, bufSize int) *Dispatcher[T] {
	d := &Dispatcher[T]{
		inputCh: make(chan Event[T], bufSize),
	}
	go d.run(ctx)
	return d
}

// Emit submits a value to be processed. Drops if buffer is full.
func (d *Dispatcher[T]) Emit(target Observer[T], value T) {
	d.totalCount.Add(1)
	select {
	case d.inputCh <- Event[T]{Target: target, Value: value}:
	default:
		dropped := d.droppedCount.Add(1)
		total := d.totalCount.Load()

		if dropped%100 == 0 {
			dropRate := float64(dropped) / float64(total) * 100
			log.Printf("Dispatcher buffer full - dropped: %d, total: %d, drop rate: %.2f%%", dropped, total, dropRate)
		}
	}
}

func (d *Dispatcher[T]) DroppedCount() uint64 {
	return d.droppedCount.Load()
}

func (d *Dispatcher[T]) TotalCount() uint64 {
	return d.totalCount.Load()
}

func (d *Dispatcher[T]) DropRate() float64 {
	total := d.totalCount.Load()
	if total == 0 {
		return 0
	}
	return float64(d.droppedCount.Load()) / float64(total) * 100
}

func (d *Dispatcher[T]) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-d.inputCh:
			ev.Target.Process(ev.Value)
		}
	}
}
