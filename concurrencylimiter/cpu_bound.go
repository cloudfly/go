package concurrencylimiter

import (
	"sync"

	"github.com/cloudfly/go/cgroup"
)

// ScheduleUnmarshalWork schedules uw to run in the worker pool.
//
// It is expected that StartUnmarshalWorkers is already called.
func ScheduleWork(uw Worker) {
	workCh <- uw
}

// Worker is a unit of unmarshal work.
type Worker interface {
	// Do must implement CPU-bound work.
	Do()
}

// StartWorkers starts unmarshal workers.
func StartWorkers() {
	if workCh != nil {
		panic("BUG: it looks like startUnmarshalWorkers() has been alread called without stopUnmarshalWorkers()")
	}
	gomaxprocs := cgroup.AvailableCPUs()
	workCh = make(chan Worker, gomaxprocs)
	workersWG.Add(gomaxprocs)
	for i := 0; i < gomaxprocs; i++ {
		go func() {
			defer workersWG.Done()
			for uw := range workCh {
				uw.Do()
			}
		}()
	}
}

// StopWorkers stops unmarshal workers.
//
// No more calles to ScheduleWork are allowed after calling stopWorkers
func StopWorkers() {
	close(workCh)
	workersWG.Wait()
	workCh = nil
}

var (
	workCh    chan Worker
	workersWG sync.WaitGroup
)
