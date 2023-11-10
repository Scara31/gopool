package gopool

import (
	"context"
	"sync"
)

type GoPool[T1, T2 any] struct {
	// Allows to send values (consumer).
	// Must be closed using CloseInChan() after
	InChan chan T1
	// Allows to send produced values (worker) and receive them (consumer).
	// Blocks until all the workers are finished
	OutChan chan T2

	ctx       context.Context    // Global pool context
	ctxCancel context.CancelFunc // Called on CancelCtx()
	waitGroup *sync.WaitGroup    // Wait group for workers
	dieChan   chan struct{}      // Used for shrinkage

	workFunction func(gp *GoPool[T1, T2], v T1)
}

// Returns new worker pool of a given size.
// Channels may be buffered or unbuffered
func NewGoPool[T1, T2 any](
	size int,
	ctx context.Context,
	inChan chan T1,
	outChan chan T2,
	workFunction func(gp *GoPool[T1, T2], v T1),
) *GoPool[T1, T2] {
	ctx, ctxCancel := context.WithCancel(ctx)

	gp := &GoPool[T1, T2]{
		InChan:  inChan,
		OutChan: outChan,

		ctx:       ctx,
		ctxCancel: ctxCancel,
		waitGroup: &sync.WaitGroup{},
		dieChan:   make(chan struct{}),

		workFunction: workFunction,
	}

	gp.waitGroup.Add(size)
	go func() {
		gp.waitGroup.Wait()
		close(gp.OutChan)
	}()

	for i := 0; i < size; i++ {
		go gp.runNewWorker()
	}

	return gp
}

// Runs new worker
func (gp *GoPool[T1, T2]) runNewWorker() {
	defer gp.waitGroup.Done()

	c := context.Background()
	c.Done()

	for {
		select {
		// The context was cancelled
		case <-gp.ctx.Done():
			return
		// Shrink() was called
		case <-gp.dieChan:
			return
		case v, more := <-gp.InChan:
			if !more {
				return
			}
			gp.workFunction(gp, v)
		}
	}
}

// Adds new workers to the pool
func (gp *GoPool[T1, T2]) Grow(increase int) {
	gp.waitGroup.Add(increase)
	for i := 0; i < increase; i++ {
		go gp.runNewWorker()
	}
}

// Removes workers from the pool gracefully
func (gp *GoPool[T1, T2]) Shrink(decrease int) {
	for i := 0; i < decrease; i++ {
		gp.dieChan <- struct{}{}
	}
}

// Closes the incoming channel - workers will be released after the InChan is empty.
// In other words, workers will finish all the scheduled tasks
func (gp *GoPool[T1, T2]) CloseInChan() {
	close(gp.InChan)
}

// Cancels pool context - all workers will quit after their current task,
// disregarding tasks left in InChan.
// In other words, workers won't finish all the scheduled tasks
func (gp *GoPool[T1, T2]) CancelCtx() {
	gp.ctxCancel()
}

// All workers will quit as soon as possible.
// Don't call it if CloseInChan() or CancelCtx() was called already
func (gp *GoPool[T1, T2]) FinishImmediately() {
	gp.CloseInChan()
	gp.CancelCtx()
}
