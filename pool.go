package gopool

import "sync"

type GoPool struct {
	// Allows to send values (consumer)
	InChan chan any
	// Allows to send produced values (worker)
	// and receive them (consumer)
	OutChan chan any

	waitGroup *sync.WaitGroup
	doneChan  chan struct{}
	dieChan   chan struct{}

	workFunction func(gp *GoPool, v any)
}

// Returns new worker pool of a given size
func NewGoPool(size int, workFunction func(gp *GoPool, v any)) *GoPool {
	gp := &GoPool{
		InChan:  make(chan any),
		OutChan: make(chan any),

		waitGroup: &sync.WaitGroup{},
		doneChan:  make(chan struct{}),
		dieChan:   make(chan struct{}),

		workFunction: workFunction,
	}

	gp.waitGroup.Add(size)
	go func() {
		gp.waitGroup.Wait()
		gp.doneChan <- struct{}{}
	}()

	for i := 0; i < size; i++ {
		go gp.runNewWorker()
	}

	return gp
}

// Runs new worker
func (gp *GoPool) runNewWorker() {
	defer gp.waitGroup.Done()

	for {
		select {
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
func (gp *GoPool) Grow(increase int) {
	gp.waitGroup.Add(increase)
	for i := 0; i < increase; i++ {
		go gp.runNewWorker()
	}
}

// Removes workers from the pool gracefully
func (gp *GoPool) Shrink(decrease int) {
	for i := 0; i < decrease; i++ {
		gp.dieChan <- struct{}{}
	}
}

// Closes the incoming channel and returns channel
// that blocks until all goroutines are finished
func (gp *GoPool) CollectResults() (doneChan chan struct{}) {
	close(gp.InChan)
	return gp.doneChan
}
