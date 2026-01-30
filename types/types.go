package types

import (
	"reflect"
	"time"
)

func Same(a, b reflect.Type) bool {
	return a.PkgPath() == b.PkgPath() && a.Name() == b.Name()
}

func TypeOf(data interface{}) reflect.Type {
	dataType := reflect.TypeOf(data)
	if dataType.Kind() == reflect.Ptr {
		return dataType.Elem()
	}
	return dataType
}

func Empty(data interface{}) bool {
	if data == nil {
		return true
	}
	if t, ok := data.(time.Time); ok {
		return t.IsZero()
	}
	dataType, dataValue := reflect.TypeOf(data), reflect.ValueOf(data)

	switch dataType.Kind() {
	case reflect.Array, reflect.String, reflect.Map, reflect.Slice:
		return dataValue.Len() == 0
	}

	return false
}

func IsNil(object interface{}) bool {
	if object == nil {
		return true
	}

	value := reflect.ValueOf(object)
	switch value.Kind() {
	case
		reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Map,
		reflect.Ptr, reflect.Slice, reflect.UnsafePointer:

		return value.IsNil()
	}

	return false
}

func Or[T any](first T, data ...T) T {
	v := reflect.ValueOf(first)
	if !v.IsZero() {
		return first
	}
	for _, item := range data {
		v := reflect.ValueOf(item)
		if !v.IsZero() {
			return item
		}
	}
	return first
}

func Ptr[T any](v T) *T {
	return &v
}

func Value[T any](v *T) T {
	if v == nil {
		var empty T
		return empty
	}
	return *v
}

func If[T any](cond bool, then, elseT T) T {
	if cond {
		return then
	}
	return elseT
}
