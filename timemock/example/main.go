package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	timemock "github.com/cloudfly/go/timemock"
)

type Group struct {
	sync.Mutex
	ctx      context.Context
	callback func(map[string]any)
	ch       chan map[string]any
	data     map[string]any
	opts     []timemock.DullOption
	started  bool
}

func NewGroup(ctx context.Context, callback func(map[string]any), opts ...timemock.DullOption) *Group {
	ch := &Group{
		ctx:      ctx,
		ch:       make(chan map[string]any),
		callback: callback,
		opts:     opts,
	}
	return ch
}

func (ch *Group) Emit(item map[string]any) {
	if ch.callback != nil && !ch.started {
		ch.Lock()
		defer ch.Unlock()
		if ch.started {
			return
		}
		go ch.Run()
		ch.started = true
	}
	ch.ch <- item
}

func (ch *Group) Run() {
	opts := ch.opts
	if opts == nil {
		opts = []timemock.DullOption{}
	}
	xticker := timemock.NewDullTicker(opts...)
	defer xticker.Stop()

	for {
		select {
		case <-ch.ctx.Done():
			return
		case data := <-ch.ch:
			ch.data = data
			xticker.Touch()
		case <-xticker.C:
			ch.callback(ch.data)
			ch.data = nil
		}
	}
}

func main() {
	callback := func(data map[string]any) {
		fmt.Println(">>>", time.Now(), data)
	}
	group := NewGroup(context.Background(), callback, timemock.WithDullMinInterval(time.Second))

	go func() {
		for {
			group.Emit(map[string]any{"hello": "world"})
			time.Sleep(time.Second)
		}
	}()
	time.Sleep(time.Hour)
}
