package api

import (
	"context"
	"net/http"

	"github.com/cloudfly/go/binder"
)

type API struct {
	mux             *http.ServeMux
	notFoundHandler http.Handler
	middlewares     []Middleware
	pathPrefix      string
}

func New(opts ...Option) *API {
	srv := &API{
		mux: http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(srv)
	}
	return srv
}

func (api *API) ANY(path string, h http.Handler) {
	api.mux.Handle(path, wrapMiddleware(h, api.middlewares))
}
func (api *API) GET(path string, h http.Handler) {
	api.mux.Handle("GET "+path, wrapMiddleware(h, api.middlewares))
}
func (api *API) POST(path string, h http.Handler) {
	api.mux.Handle("POST "+path, wrapMiddleware(h, api.middlewares))
}
func (api *API) PUT(path string, h http.Handler) {
	api.mux.Handle("PUT "+path, wrapMiddleware(h, api.middlewares))
}
func (api *API) PATCH(path string, h http.Handler) {
	api.mux.Handle("PATCH "+path, wrapMiddleware(h, api.middlewares))
}
func (api *API) DELETE(path string, h http.Handler) {
	api.mux.Handle("DELETE "+path, wrapMiddleware(h, api.middlewares))
}
func (api *API) TRACE(path string, h http.Handler) {
	api.mux.Handle("TRACE "+path, wrapMiddleware(h, api.middlewares))
}
func (api *API) HEAD(path string, h http.Handler) {
	api.mux.Handle("HEAD "+path, wrapMiddleware(h, api.middlewares))
}
func (api *API) OPTION(path string, h http.Handler) {
	api.mux.Handle("OPTION "+path, wrapMiddleware(h, api.middlewares))
}
func (api *API) CONNECT(path string, h http.Handler) {
	api.mux.Handle("CONNECT "+path, wrapMiddleware(h, api.middlewares))
}

// GROUP create a api group with custom url prefix and middlewares, the middlewares only works on handlers registerd on this group
func (api *API) GROUP(path string, middlewares ...Middleware) *API {
	return &API{
		pathPrefix:      api.pathPrefix + path,
		notFoundHandler: api.notFoundHandler,
		mux:             api.mux,
		middlewares:     append(append([]Middleware{}, api.middlewares...), middlewares...),
	}
}

// ServeHTTP implements the http.Handler interface
func (api *API) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	api.mux.ServeHTTP(w, req)
}

func (api *API) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, api)
}

type Option func(*API)

// WithNotFoundHandler specifics a http handler for 404 case.
func WithNotFoundHandler(h http.Handler) Option {
	return func(srv *API) {
		srv.notFoundHandler = h
	}
}

// WithMiddleware specifics middlewares for all the service handlers.
func WithMiddleware(middlewares ...Middleware) Option {
	return func(srv *API) {
		srv.middlewares = middlewares
	}
}

// Middleware wrap the http.HandlerFunc, so that it can handle the http.Request in advance and intercept the request if required(eg. authorization, logging)
type Middleware func(http.Handler) http.Handler

func wrapMiddleware(handler http.Handler, middlewares []Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

type TypedHandlerFunc[REQ, RESP any] func(context.Context, REQ) (RESP, error)

func HandlerFunc[REQ, RESP any](handle TypedHandlerFunc[REQ, RESP], opts ...ReturnOption) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req REQ
		err := binder.BindHttp(r, &req)
		if err != nil {
			Fail(w, err, opts...)
			return
		}
		resp, err := handle(r.Context(), req)
		if err != nil {
			Fail(w, err, opts...)
			return
		}
		ReturnJSON(w, resp, opts...)
	}
}
