package timelimiter

import (
	"sync/atomic"
	"time"
)

type TimeLimiter struct {
	max  uint64
	c    uint64
	d    time.Duration
	stop chan struct{}
}

func New(d time.Duration, c uint64) *TimeLimiter {
	tl := &TimeLimiter{
		max:  c,
		d:    d,
		stop: make(chan struct{}),
	}
	go tl.active()
	return tl
}

func (tl *TimeLimiter) active() {
	ticker := time.NewTicker(tl.d)
	defer ticker.Stop()
	for {
		select {
		case <-tl.stop:
			return
		case <-ticker.C:
			atomic.StoreUint64(&tl.c, 0)
		}
	}
}

func (tl *TimeLimiter) Pass() bool {
	return atomic.AddUint64(&tl.c, 1) <= tl.max
}

func (tl *TimeLimiter) Stop() {
	close(tl.stop)
}
