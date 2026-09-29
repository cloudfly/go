package api

import "net/http"

// Middleware wrap the http.HandlerFunc, so that it can handle the http.Request in advance and intercept the request if required(eg. authorization, logging)
type Middleware func(http.Handler) http.Handler

func wrapMiddleware(handler http.Handler, middlewares []Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// RecoverPanic keeps one misbehaving handler from taking down the process.
func RecoverPanic(errorHandler func(any)) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					errorHandler(rec)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
