package concurrencylimiter

import (
	"fmt"
	"time"

	"github.com/cloudfly/go/timerpool"
)

var (
	defaultQueueDuration = time.Minute

	ErrConcurrencyLimitReached = fmt.Errorf("concurrency limit reached")
)

var defaultLimiter *Limiter

// Init initializes concurrencylimiter.
//
// Init must be called after flag.Parse call.
func Init(size int, d ...time.Duration) {
	defaultLimiter = New(size, d...)
}

// Do calls f with the limited concurrency.
func Do(f func() error) error {
	return defaultLimiter.Do(f)
}

func New(limit int, d ...time.Duration) *Limiter {
	limiter := &Limiter{
		limit:    limit,
		duration: defaultQueueDuration,
		ch:       make(chan struct{}, limit),
	}
	if len(d) > 0 {
		limiter.duration = d[0]
	}
	return limiter
}

type Limiter struct {
	limit    int
	duration time.Duration
	ch       chan struct{}
}

// Do calls f with the limited concurrency.
func (limiter *Limiter) Do(f func() error) error {
	// Limit the number of conurrent f calls in order to prevent from excess
	// memory usage and CPU trashing.
	select {
	case limiter.ch <- struct{}{}:
		err := f()
		<-limiter.ch
		return err
	default:
	}

	if limiter.duration == 0 {
		return ErrConcurrencyLimitReached
	}

	// All the workers are busy.
	// Sleep for up to *maxQueueDuration.
	t := timerpool.Get(limiter.duration)
	select {
	case limiter.ch <- struct{}{}:
		timerpool.Put(t)
		err := f()
		<-limiter.ch
		return err
	case <-t.C:
		timerpool.Put(t)
		return fmt.Errorf("cannot handle more than %d concurrent inserts during %s", limiter.limit, limiter.duration)
	}
}
