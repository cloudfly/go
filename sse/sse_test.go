package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNewWriterReturnedWriterWritesReadableSSEEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	writer := NewWriter(ctx, pw)

	type ssePayload struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Index   int    `json:"index"`
	}

	readResult := make(chan []ssePayload, 1)
	readErr := make(chan error, 1)
	go func() {
		events, err := readSSEPayloads[ssePayload](pr, 2)
		if err != nil {
			readErr <- err
			return
		}
		readResult <- events
	}()

	want := []ssePayload{
		{Type: "message", Message: "hello", Index: 1},
		{Type: "done", Message: "world", Index: 2},
	}
	if err := writer.Write(want[0], nil, want[1]); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-readResult:
		if !slices.Equal(want, got) {
			t.Fatalf("want %v; got %v", want, got)
		}
	case err := <-readErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading SSE events")
	}

	writer.Flush()
	writer.Close()
}

func readSSEPayloads[T any](r io.Reader, n int) ([]T, error) {
	reader := bufio.NewReader(r)
	events := make([]T, 0, n)
	for len(events) < n {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSuffix(line, "\n")
		if !strings.HasPrefix(line, "data: ") {
			return nil, io.ErrUnexpectedEOF
		}

		var event T
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			return nil, err
		}
		events = append(events, event)

		line, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if strings.TrimSuffix(line, "\n") != "" {
			return nil, io.ErrUnexpectedEOF
		}
	}
	return events, nil
}
