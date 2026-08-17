package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
)

type returnOption struct {
	contentType string
	statusCode  int
	errCode     string
	header      map[string]string
	marshaler   ResponseMarshaler
}

type ReturnOption func(*returnOption)

func WithContentType(s string) ReturnOption {
	return func(o *returnOption) {
		if s != "" {
			o.contentType = s
		}
	}
}

func WithStatusCode(code int) ReturnOption {
	return func(o *returnOption) {
		if code > 0 {
			o.statusCode = code
		}
	}
}

func WithHeader(name, value string) ReturnOption {
	return func(o *returnOption) {
		if o.header == nil {
			o.header = make(map[string]string)
		}
		o.header[name] = value
	}
}

func WithMarshaler(marshaler ResponseMarshaler) ReturnOption {
	return func(o *returnOption) {
		if marshaler != nil {
			o.marshaler = marshaler
		}
	}
}

func WithErrorCode(code string) ReturnOption {
	return func(o *returnOption) {
		o.errCode = code
	}
}

// Return write the result and code into ResponseWriter
func Return(w http.ResponseWriter, data any, opts ...ReturnOption) {
	var (
		content []byte
		err     error
		option  = &returnOption{
			contentType: "application/json",
			statusCode:  200,
			header:      make(map[string]string),
			marshaler:   defaultResponseMarshaler,
		}
	)
	for _, opt := range opts {
		opt(option)
	}

	for k, v := range option.header {
		w.Header().Set(k, v)
	}
	if option.contentType != "" {
		w.Header().Set("ContentType", option.contentType)
	}

	if data != nil {
		content, err = option.marshaler(data)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
	}

	w.WriteHeader(option.statusCode)
	w.Write(content)
}

func ReturnJSON(w http.ResponseWriter, data any, opts ...ReturnOption) {
	opts = append(opts, WithContentType("application/json"))
	Return(w, data, opts...)
}

func ReturnText(w http.ResponseWriter, data any, opts ...ReturnOption) {
	opts = append(opts,
		WithContentType("text/plain"),
		WithMarshaler(
			func(data any) ([]byte, error) {
				switch v := data.(type) {
				case string:
					return []byte(v), nil
				case []byte:
					return v, nil
				}
				return nil, fmt.Errorf("type %s can not be marshared into text, string or []byte requried", reflect.TypeOf(data).Name())
			},
		),
	)
	Return(w, data, opts...)
}

func Fail(w http.ResponseWriter, err error, opts ...ReturnOption) {
	opts = append(opts, WithContentType("application/json"), WithMarshaler(json.Marshal))
	Return(w, responseBody{
		Code:    "InternalServerError",
		Message: err.Error(),
	})
}

type ctxKey string

func Writer(ctx context.Context) http.ResponseWriter {
	return ctx.Value(ctxKey("writer")).(http.ResponseWriter)
}

func withWriter(ctx context.Context, w http.ResponseWriter) context.Context {
	return context.WithValue(ctx, ctxKey("writer"), w)
}
