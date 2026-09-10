package sse

import (
	"context"
	"fmt"
	"iter"
)

type Queue interface {
	Push(context.Context, ...any) error
	Pop(context.Context) iter.Seq2[any, error]
}

func NewChanQueue(bufferSize int) Queue {
	return &chanQueue{
		q: make(chan any, bufferSize),
	}
}

type chanQueue struct {
	q chan any
}

func (q *chanQueue) Push(ctx context.Context, event ...any) error {
	for _, e := range event {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled")
		case q.q <- e:
		}
	}
	return nil
}

func (q *chanQueue) Pop(ctx context.Context) iter.Seq2[any, error] {
	return func(yield func(any, error) bool) {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-q.q:
				if !yield(event, nil) {
					return
				}
			}
		}
	}
}

type redisQueue struct {
}
