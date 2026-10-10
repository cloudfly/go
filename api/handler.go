package api

import (
	"context"
	"net/http"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"github.com/cloudfly/go/binder"
)

type TypedHandlerFunc[REQ, RESP any] func(context.Context, *REQ) (*RESP, error)

func HandlerFunc[REQ, RESP any](handle TypedHandlerFunc[REQ, RESP], opts ...HandleOption) http.Handler {
	var option = &handleOption{
		method: "POST",
		path:   "/",
	}

	functionName := runtime.FuncForPC(reflect.ValueOf(handle).Pointer()).Name()
	items := strings.Split(functionName, ".")
	slices.Reverse(items)
	for _, item := range items {
		if !strings.HasPrefix(item, "func") { // this is a anonymous closure
			option.path += item
			break
		}
	}

	for _, opt := range opts {
		opt(option)
	}

	f := func(w http.ResponseWriter, r *http.Request) {
		var req REQ
		err := binder.BindHttp(r, &req)
		if err != nil {
			Fail(w, err, opts...)
			return
		}
		ctx := withWriter(r.Context(), w)
		resp, err := handle(ctx, &req)
		if err != nil {
			Fail(w, err, opts...)
			return
		}
		if resp != nil {
			ReturnJSON(w, resp, opts...)
		}
	}

	return &patternedHandler{
		Handler: http.HandlerFunc(f),
		method:  option.method,
		path:    option.path,
	}
}

type patternedHandler struct {
	http.Handler
	method string
	path   string
}

func (h *patternedHandler) Method() string {
	return h.method
}
func (h *patternedHandler) Path() string {
	return h.path
}
