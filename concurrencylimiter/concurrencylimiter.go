package concurrencylimiter

import (
	"fmt"
	"time"

	"github.com/cloudfly/go/timerpool"
)

var (
	Size             int
	maxQueueDuration = time.Minute
)

// ch is the channel for limiting concurrent calls to Do.
var ch chan struct{}

// Init initializes concurrencylimiter.
//
// Init must be called after flag.Parse call.
func Init(size int, d ...time.Duration) {
	Size = size
	ch = make(chan struct{}, size)
	if len(d) > 0 {
		maxQueueDuration = d[0]
	}
}

// Do calls f with the limited concurrency.
func Do(f func() error) error {
	// Limit the number of conurrent f calls in order to prevent from excess
	// memory usage and CPU trashing.
	select {
	case ch <- struct{}{}:
		err := f()
		<-ch
		return err
	default:
	}

	// All the workers are busy.
	// Sleep for up to *maxQueueDuration.
	t := timerpool.Get(maxQueueDuration)
	select {
	case ch <- struct{}{}:
		timerpool.Put(t)
		err := f()
		<-ch
		return err
	case <-t.C:
		timerpool.Put(t)
		return fmt.Errorf("cannot handle more than %d concurrent inserts during %s", Size, maxQueueDuration)
	}
}
