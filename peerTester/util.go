package peerTester

import (
	"sync"
	"time"
)

func wgWaitTimout(wg *sync.WaitGroup, timeout time.Duration) bool {
	t := make(chan struct{})
	go func() {
		defer close(t)
		wg.Wait()
	}()
	select {
	case <-t:
		return true
	case <-time.After(timeout):
		return false
	}
}
