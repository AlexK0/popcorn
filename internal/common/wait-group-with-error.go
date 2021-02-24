package common

import (
	"sync"
	"sync/atomic"
)

// WaitGroupWithError ...
type WaitGroupWithError struct {
	err   error
	isSet int32
	wg    sync.WaitGroup
}

// Add ...
func (wg *WaitGroupWithError) Add(delta int) {
	wg.wg.Add(delta)
}

// Done ...
func (wg *WaitGroupWithError) Done(err error) {
	if err != nil && atomic.SwapInt32(&wg.isSet, 1) == 0 {
		wg.err = err
	}
	wg.wg.Done()
}

// Wait ...
func (wg *WaitGroupWithError) Wait() error {
	wg.wg.Wait()
	return wg.err
}
