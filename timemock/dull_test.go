package timemock

import (
	"testing"
	"time"
)

func TestDullTicker(t *testing.T) {
	mock := NewMockClock()
	mock.Set(time.Now())
	tick := NewDullTicker(WithDullClock(mock), WithDullResetDuration(time.Second*3), WithDullMinInterval(time.Second*9))
	defer tick.Stop()

	// sleep for a while, waiting ticker activate
	time.Sleep(time.Second)

	tickCount := 0
	go func() {
		for range tick.C {
			tickCount++
		}
	}()

	t.Run("dull step", func(t *testing.T) {
		tickCount = 0
		for i := 0; i < 70; i++ {
			tick.Touch()
			mock.Add(time.Second)
		}

		// wait for a while
		time.Sleep(time.Second)

		if tickCount != 3 {
			t.Errorf("tickCount should be %d, but got %d", 3, tickCount)
		}
	})

	t.Run("reset", func(t *testing.T) {
		tickCount = 0
		tick.Reset()
		for i := 0; i < 40; i++ {
			tick.Touch()
			mock.Add(time.Second)
		}

		for i := 0; i < 10; i++ {
			// move time 5 seconds, dullticker should be reset
			mock.Add(time.Second * 5)
			tick.Touch()
		}

		// wait for a while
		time.Sleep(time.Second)

		if tickCount != 12 {
			t.Errorf("tickCount should be %d, but got %d", 12, tickCount)
		}
	})
}

func TestDullTickerReal(t *testing.T) {
	t.Log("This test will run for a long time, please waiting for it")

	dt := NewDullTicker()

	go func() {
		for i := 0; i < 200; i++ {
			dt.Touch()
			time.Sleep(time.Second)
		}
		time.Sleep(time.Minute * 5)
		for i := 0; i < 200; i++ {
			dt.Touch()
			time.Sleep(time.Second)
		}
	}()

	for now := range dt.C {
		t.Log("tick:", now)
	}
}
