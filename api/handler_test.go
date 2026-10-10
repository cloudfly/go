package api

import (
	"context"
	"net/http"
	"testing"
)

type handlerPathRequest struct{}
type handlerPathResponse struct{}

func namedHandlerForPathTest(context.Context, *handlerPathRequest) (*handlerPathResponse, error) {
	return nil, nil
}

func TestHandlerFuncAutomaticPathFromNamedFunction(t *testing.T) {
	handler := HandlerFunc(namedHandlerForPathTest)

	if got, want := handler.(*patternedHandler).Path(), "/namedHandlerForPathTest"; got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestHandlerFuncAutomaticPathFromClosure(t *testing.T) {
	handler := HandlerFunc(func(context.Context, *handlerPathRequest) (*handlerPathResponse, error) {
		return nil, nil
	})

	if got, want := handler.(*patternedHandler).Path(), "/TestHandlerFuncAutomaticPathFromClosure"; got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestHandlerFuncPathOptionOverridesAutomaticPath(t *testing.T) {
	handler := HandlerFunc(namedHandlerForPathTest, WithPath("GET /custom"))
	got := handler.(*patternedHandler)

	if got.Path() != "/custom" {
		t.Fatalf("Path() = %q, want %q", got.Path(), "/custom")
	}
	if got.Method() != http.MethodGet {
		t.Fatalf("Method() = %q, want %q", got.Method(), http.MethodGet)
	}
}
