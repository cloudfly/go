package ormx

import (
	"reflect"
	"testing"
)

func TestDereferencedElemType(t *testing.T) {

	t.Run("pointer", func(t *testing.T) {
		tp := reflect.TypeOf([]any{})
		if v := dereferencedElemType(tp); v == nil {
			t.Errorf("dereferencedElemType(%v) = %v, want %v", t, tp, reflect.TypeOf([]any{}))
		} else {
			t.Log(v)
		}
	})

	t.Run("common", func(t *testing.T) {
		type Test struct{}
		var i any = &Test{}
		tp := reflect.TypeOf(i)
		if v := dereferencedElemType(tp).String(); v != "ormx.Test" {
			t.Errorf("dereferencedElemType(%v) = %v, want %v", tp.String(), v, "Test")
		}
	})

	t.Run("slice", func(t *testing.T) {
		type Test struct{}
		var i []*Test = []*Test{&Test{}}
		tp := reflect.TypeOf(i)
		if v := dereferencedElemType(tp).String(); v != "ormx.Test" {
			t.Errorf("dereferencedElemType(%v) = %v, want %v", tp.String(), v, "Test")
		}
	})
}

func TestDereferencedType(t *testing.T) {
	var v *int64 = nil
	tp := dereferencedType(reflect.TypeOf(v))
	t.Log(tp)

	c := &Cond{}
	t.Log(c.E("id", v), c.args)
}
