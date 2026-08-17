package api

import "encoding/json"

var (
	defaultResponseMarshaler = DefaultResponseMarshaler
)

// ResponseMarshaler is a function type for marshaling the response data into []byte content.
type ResponseMarshaler func(any) ([]byte, error)

func DefaultResponseMarshaler(data any) ([]byte, error) {
	if data == nil {
		return json.Marshal(responseBody{Code: ""})
	}
	if v, ok := data.(responseBody); ok {
		return json.Marshal(v)
	}
	return json.Marshal(responseBody{Code: "", Data: data})
}

func SetDefaultResponseMarshaler(marshaler ResponseMarshaler) {
	defaultResponseMarshaler = marshaler
}

// responseBody represents data type in response body
type responseBody struct {
	Code    string `json:"code"`
	LogId   string `json:"log_id,omitempty"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}
