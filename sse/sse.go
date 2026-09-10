package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
)

// HTTPResponseWriter prepares an HTTP response for SSE streaming and returns it
// as an io.Writer.
func HTTPResponseWriter(w http.ResponseWriter) io.Writer {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Type")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	return w
}

type Writer interface {
	// Write writes the event to the stream.
	Write(...any) error
	// Flush flushes the stream.
	Flush() error
	// Close closes the stream.
	Close() error
}

type Encoder interface {
	Encode(any) error
}

type writer struct {
	ctx     context.Context
	cancel  context.CancelFunc
	w       io.Writer
	q       Queue
	flush   func() error
	done    chan struct{}
	err     atomic.Value
	encoder Encoder
}

// NewWriter creates an asynchronous SSE writer. Callers should close the writer
// when the stream is no longer needed to stop its background goroutine.
func NewWriter(ctx context.Context, w io.Writer) Writer {
	_w := &writer{
		w:       w,
		q:       &chanQueue{q: make(chan any)},
		done:    make(chan struct{}),
		encoder: json.NewEncoder(w),
	}
	if fe, ok := w.(interface{ Flush() error }); ok {
		_w.flush = func() error {
			return fe.Flush()
		}
	} else if f, ok := w.(interface{ Flush() }); ok {
		_w.flush = func() error {
			f.Flush()
			return nil
		}
	}
	_w.ctx, _w.cancel = context.WithCancel(ctx)

	go _w.active()

	return _w
}

// NewHTTPWriter prepares the HTTP response headers and creates an SSE writer.
func NewHTTPWriter(ctx context.Context, w http.ResponseWriter) Writer {
	return NewWriter(ctx, HTTPResponseWriter(w))
}

func (w *writer) active() {
	defer close(w.done)
	defer w.cancel()

	for event, err := range w.q.Pop(w.ctx) {
		if err != nil {
			w.setErr(err)
			return
		}
		if err := w.sendEvent(event); err != nil {
			w.setErr(err)
			return
		}
	}
}

func (w *writer) sendEvent(event any) error {
	if event == nil {
		return nil
	}
	_, err := fmt.Fprintf(w.w, "data: ")
	if err != nil {
		return fmt.Errorf("write response error: %w", err)
	}

	err = w.encoder.Encode(event)
	if err != nil {
		return fmt.Errorf("encode response error: %w", err)
	}
	_, err = fmt.Fprintf(w.w, "\n")
	if err != nil {
		return fmt.Errorf("write response error: %w", err)
	}

	// Flush every event so SSE clients can observe streaming updates
	// without waiting for the response writer's internal buffer.
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush response error: %w", err)
	}
	return nil
}

func (w *writer) Write(events ...any) error {
	for _, event := range events {
		if event == nil {
			continue
		}
		if err := w.getErr(); err != nil {
			return err
		}
		if err := w.q.Push(w.ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (w *writer) Flush() error {
	if w.flush == nil {
		return nil
	}
	return w.flush()
}

func (w *writer) Close() error {
	// Do not close w.q here. Write may be concurrently selecting on a send,
	// and closing the channel would make that path panic.
	w.cancel()
	<-w.done
	if err := w.Flush(); err != nil {
		w.setErr(fmt.Errorf("flush response error: %w", err))
	}
	return w.getErr()
}

func (w *writer) setErr(err error) {
	w.err.Store(err)
}

func (w *writer) getErr() error {
	v := w.err.Load()
	if v == nil {
		return nil
	}
	return v.(error)
}

type Option func(*writer)

func WithEncoder(encoder Encoder) Option {
	return func(w *writer) {
		w.encoder = encoder
	}
}

func WithQueue(q Queue) Option {
	return func(w *writer) {
		w.q = q
	}
}
