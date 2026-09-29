package api

import "net/http"

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

func WithBasePath(path string) Option {
	return func(srv *API) {
		srv.pathPrefix = path
	}
}

type handleOption struct {
	contentType string
	statusCode  int
	errCode     string
	header      map[string]string
	marshaler   ResponseMarshaler

	method string
	path   string
}

type HandleOption func(*handleOption)

func WithMethod(method string) HandleOption {
	return func(o *handleOption) {
		o.method = method
	}
}

func WithPath(path string) HandleOption {
	return func(o *handleOption) {
		o.path = path
	}
}

func WithContentType(s string) HandleOption {
	return func(o *handleOption) {
		if s != "" {
			o.contentType = s
		}
	}
}

func WithStatusCode(code int) HandleOption {
	return func(o *handleOption) {
		if code > 0 {
			o.statusCode = code
		}
	}
}

func WithHeader(name, value string) HandleOption {
	return func(o *handleOption) {
		if o.header == nil {
			o.header = make(map[string]string)
		}
		o.header[name] = value
	}
}

func WithMarshaler(marshaler ResponseMarshaler) HandleOption {
	return func(o *handleOption) {
		if marshaler != nil {
			o.marshaler = marshaler
		}
	}
}

func WithErrorCode(code string) HandleOption {
	return func(o *handleOption) {
		o.errCode = code
	}
}
