package timemock

import (
	"context"
	"time"
)

const (
	DefaultDullResetDuration = time.Minute * 3
	DefaultDullMaxInterval   = time.Hour
	DefaultDullMinInterval   = time.Second * 15
)

// DullTicker is a dynamic ticker mostly used in watching scene. the Ticker would become more and more dull if it keeping be touched.
// And if stop Touch() in some period, the ticker would be reset.
// User can also custom the dull behavior, by default the tick interval will be extended exponentially until it reaches the MaxInterval.
type DullTikcker struct {
	clock Clock
	c     chan time.Time
	C     <-chan time.Time
	op    chan string
	stop  func()

	next func(time.Duration) time.Duration

	// maxInterval represent the maximum interval for posting message to webhook, if there is continuous messages ingest.
	maxInterval time.Duration
	minInterval time.Duration
	// resetDuration represent the maximum delay time for posting latest message to webhook, if no any message ingest.
	resetDuration time.Duration

	// interval represent current step interval, it will increase after sending tick from minStep until reach maxStep
	interval time.Duration
	// tickTime represent the latest time that send tick
	tickTime time.Time
	// touchTime represent the latest Touch() time
	touchTime time.Time
}

func NewDullTicker(opts ...DullOption) *DullTikcker {
	var (
		ctx  context.Context
		dull = &DullTikcker{
			clock:         NewClock(),
			op:            make(chan string),
			c:             make(chan time.Time, 1),
			next:          DefaultDullFunc,
			maxInterval:   DefaultDullMaxInterval,
			minInterval:   DefaultDullMinInterval,
			resetDuration: DefaultDullResetDuration,
		}
	)

	for _, opt := range opts {
		opt(dull)
	}
	dull.C = dull.c
	dull.interval = dull.minInterval

	ctx, dull.stop = context.WithCancel(context.Background())

	go dull.activate(ctx)

	return dull
}

func (dull *DullTikcker) Touch() {
	dull.op <- "touch"
}

func (dull *DullTikcker) Reset() {
	dull.op <- "reset"
}

func (dull *DullTikcker) Stop() {
	if dull.stop != nil {
		dull.stop()
	}
}

func (dull *DullTikcker) activate(ctx context.Context) {
	var ticker *Ticker
	if dull.minInterval >= time.Second*2 {
		ticker = dull.clock.Ticker(time.Second)
	} else {
		ticker = dull.clock.Ticker(time.Millisecond * 100)
	}
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case op := <-dull.op:
			switch op {
			case "reset":
				dull.interval = dull.minInterval
				dull.tickTime = time.Unix(0, 0)
				dull.touchTime = time.Time{}
			case "touch":
				dull.touchTime = dull.clock.Now()
			}
		case now := <-ticker.C:
			/*
				fmt.Println("--------")
				fmt.Println("    now:", now)
				fmt.Println("    touch:", dull.touchTime, dull.resetDuration)
				fmt.Println("    tick:", dull.tickTime, dull.interval)
			*/
			if dull.touchTime.IsZero() {
				break
			}

			reachReset := now.After(dull.touchTime.Add(dull.resetDuration))
			reachStep := now.After(dull.tickTime.Add(dull.interval))
			if !reachReset && !reachStep {
				// the send time not reached
				break
			}

			select {
			case dull.c <- dull.touchTime:
			default:
			}

			if reachReset {
				// triggered by max wait duration, reset the interval
				dull.interval = dull.minInterval
			} else {
				// triggered by interval
				dull.interval = dull.next(dull.interval)
			}
			if dull.maxInterval > 0 && dull.interval > dull.maxInterval {
				dull.interval = dull.maxInterval
			}
			if dull.minInterval > 0 && dull.interval < dull.minInterval {
				dull.interval = dull.minInterval
			}

			dull.tickTime = now
			dull.touchTime = time.Time{}
		}
	}
}

// DullOption custom options for DullTicker
type DullOption func(*DullTikcker)

// WithDullMaxInterval set the maximum interval, use DefaulDullMaxStep by default.
func WithDullMaxInterval(interval time.Duration) DullOption {
	return func(dull *DullTikcker) {
		dull.maxInterval = interval
	}
}

// WithDullMaxInterval set the minimum interval, use DefaulDullMinStep by default.
func WithDullMinInterval(interval time.Duration) DullOption {
	return func(dull *DullTikcker) {
		dull.minInterval = interval
	}
}

// WithDullResetDuration set duration that reset the DullTicker if no Touch() in this peroid.
func WithDullResetDuration(reset time.Duration) DullOption {
	return func(dull *DullTikcker) {
		dull.resetDuration = reset
	}
}

// WithDullClock set the Clock object, mostly used to set MockClock in test.
func WithDullClock(clock Clock) DullOption {
	return func(dull *DullTikcker) {
		dull.clock = clock
	}
}

// WithDullFunc set the dull funciton. The function is used to caculate next tick interval
// THe value return by function  should be between MinInterval and MaxInterval, otherwize MinInterval and MaxInterval will be used(if they specified).
func WithDullFunc(f func(time.Duration) time.Duration) DullOption {
	return func(dull *DullTikcker) {
		dull.next = f
	}
}

// DefaultDullFunc double the given duration
func DefaultDullFunc(d time.Duration) time.Duration {
	// d * 2
	return d * 2
}
