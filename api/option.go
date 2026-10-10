package api

import (
	"strings"
)

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
		fields := strings.Fields(path)
		if len(fields) == 0 {
			o.path = "/"
		} else if len(fields) == 1 {
			o.path = fields[0]
		} else if len(fields) == 2 {
			o.method = fields[0]
			o.path = fields[1]
		} else {
			panic("invalid handler path, must be in the format of 'method path' or 'path'")
		}
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
