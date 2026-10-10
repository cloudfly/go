package api

import (
	"net/http"
	"strings"
)

type API struct {
	mux         *http.ServeMux
	middlewares []Middleware
	pathPrefix  string
}

func New() *API {
	srv := &API{
		mux: http.NewServeMux(),
	}
	return srv
}

func (api *API) GET(path string, h http.Handler) {
	api.Handle("", patternedHandler{method: "GET", path: path, Handler: h})
}

func (api *API) POST(path string, h http.Handler) {
	api.Handle("", patternedHandler{method: "POST", path: path, Handler: h})
}

func (api *API) PUT(path string, h http.Handler) {
	api.Handle("", patternedHandler{method: "PUT", path: path, Handler: h})
}

func (api *API) PATCH(path string, h http.Handler) {
	api.Handle("", patternedHandler{method: "PATCH", path: path, Handler: h})
}

func (api *API) DELETE(path string, h http.Handler) {
	api.Handle("", patternedHandler{method: "DELETE", path: path, Handler: h})
}

func (api *API) HEAD(path string, h http.Handler) {
	api.Handle("", patternedHandler{method: "HEAD", path: path, Handler: h})
}

func (api *API) OPTION(path string, h http.Handler) {
	api.Handle("", patternedHandler{method: "OPTION", path: path, Handler: h})
}

func (api *API) CONNECT(path string, h http.Handler) {
	api.Handle("", patternedHandler{method: "CONNECT", path: path, Handler: h})
}

// HandlePath bind a handler to the given url path, the path can be in the format of "method path" or "path"
//
// The path would be '/' as default if left empty.
func (api *API) Handle(path string, h http.Handler) {
	var (
		method  = ""
		uriPath = "/"
	)
	fields := strings.Fields(path)
	if len(fields) == 0 {
		if v, ok := h.(interface{ Path() string }); ok {
			uriPath = v.Path()
		}
		if v, ok := h.(interface{ Method() string }); ok {
			method = v.Method()
		}
	} else if len(fields) == 1 {
		uriPath = fields[0]
		if v, ok := h.(interface{ Method() string }); ok {
			method = v.Method()
		}
	} else if len(fields) == 2 {
		method = fields[0]
		uriPath = fields[1]
	}

	api.mux.Handle(strings.TrimSpace(method+" "+api.pathPrefix+uriPath), wrapMiddleware(h, api.middlewares))
}

func (api *API) Use(middleware Middleware, others ...Middleware) *API {
	api.middlewares = append(api.middlewares, middleware)
	if len(others) > 0 {
		api.middlewares = append(api.middlewares, others...)
	}
	return api
}

// GROUP create a api group with custom url prefix and middlewares, the middlewares only works on handlers registerd on this group
func (api *API) GROUP(path string) *API {
	return &API{
		pathPrefix:  api.pathPrefix + path,
		mux:         api.mux,
		middlewares: append([]Middleware{}, api.middlewares...),
	}
}

// ServeHTTP implements the http.Handler interface
func (api *API) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	api.mux.ServeHTTP(w, req)
}

func (api *API) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, api)
}
