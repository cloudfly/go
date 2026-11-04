package orm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

type timeValuer struct{ t time.Time }

func (v timeValuer) Time() time.Time { return v.t }

func TestAny2Time(t *testing.T) {
	want := time.Unix(1700000000, 0)

	tests := []struct {
		name   string
		input  any
		wantOK bool
	}{
		{"nil", nil, false},
		{"Time() interface", timeValuer{want}, true},
		{"int64 seconds", int64(1700000000), true},
		{"int seconds", int(1700000000), true},
		{"int32", int32(1700000000), true},
		{"uint", uint(1700000000), true},
		{"uint32", uint32(1700000000), true},
		{"uint64", uint64(1700000000), true},
		{"float64", float64(1700000000.9), true},
		{"float32", float32(1700000000), true},
		{"string", "1700000000", true},
		{"bytes", []byte("1700000000"), true},
		{"json.Number", json.Number("1700000000"), true},
		{"milliseconds", int64(1700000000000), true},
		{"microseconds", int64(1700000000000000), true},
		{"nanoseconds", int64(1700000000000000000), true},
		{"zero", int64(0), false},
		{"zero string", "0", false},
		{"non numeric string", "2024-01-01 00:00:00", false},
		{"unsupported", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Any2Time(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("Any2Time() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.Unix() != want.Unix() {
				t.Fatalf("Any2Time() = %v, want %v", got, want)
			}
		})
	}
}

func TestNormalizeUnixSeconds(t *testing.T) {
	tests := map[int64]int64{
		1700000000:          1700000000,
		1700000000000:       1700000000,
		1700000000000000:    1700000000,
		1700000000000000000: 1700000000,
	}
	for input, want := range tests {
		if got := normalizeUnixSeconds(input); got != want {
			t.Errorf("normalizeUnixSeconds(%d) = %d, want %d", input, got, want)
		}
	}
}

func TestAny2Slice(t *testing.T) {
	if got := Any2Slice(nil); len(got) != 0 {
		t.Fatalf("Any2Slice(nil) = %#v", got)
	}
	var nilSlice []int
	if got := Any2Slice(nilSlice); len(got) != 0 {
		t.Fatalf("Any2Slice(nil slice) = %#v", got)
	}
	if got := Any2Slice(1); !reflect.DeepEqual(got, []any{1}) {
		t.Fatalf("Any2Slice(1) = %#v", got)
	}
	if got := Any2Slice([]int{1, 2}); !reflect.DeepEqual(got, []any{1, 2}) {
		t.Fatalf("Any2Slice([]int) = %#v", got)
	}
	s := []int{1, 2}
	if got := Any2Slice(&s); !reflect.DeepEqual(got, []any{1, 2}) {
		t.Fatalf("Any2Slice(&slice) = %#v", got)
	}
}

func TestKVsFromMap(t *testing.T) {
	tests := []struct {
		name   string
		filter map[string]any
		want   KVs
	}{
		{"nil value", map[string]any{"a": nil}, KVs{{Key: "a", Value: nil, Extra: "e"}}},
		{"scalar", map[string]any{"a": 0}, KVs{{Key: "a", Value: 0, Extra: "e"}}},
		{"slice", map[string]any{"a": []int{1, 2}}, KVs{{Key: "a", Value: []int{1, 2}, Extra: "in"}}},
		{"bytes", map[string]any{"a": []byte("x")}, KVs{{Key: "a", Value: "x", Extra: "e"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KVsFromMap(nil, tt.filter)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("KVsFromMap() = %#v, want %#v", got, tt.want)
			}
		})
	}

	dst := KVs{{Key: "keep", Value: 1}}
	got := KVsFromMap(dst, map[string]any{"a": 2})
	if len(got) != 2 || got[0].Key != "keep" {
		t.Fatalf("KVsFromMap should append to dst, got %#v", got)
	}
}

func TestErrorHelpers(t *testing.T) {
	if IsNotFound(nil) {
		t.Fatalf("IsNotFound(nil) = true")
	}
	wrapped := fmt.Errorf("wrapped: %w", sql.ErrNoRows)
	if !IsNotFound(wrapped) {
		t.Fatalf("IsNotFound(wrapped) = false")
	}

	if IsDuplicate(nil) {
		t.Fatalf("IsDuplicate(nil) = true")
	}
	if !IsDuplicate(errors.New("Error 1062: Duplicate entry '1' for key 'PRIMARY'")) {
		t.Fatalf("IsDuplicate() = false")
	}
	if IsDuplicate(errors.New("Error 1062: something else")) {
		t.Fatalf("IsDuplicate() should require Duplicate entry")
	}
}

func TestPtr(t *testing.T) {
	p := Ptr(3)
	if *p != 3 {
		t.Fatalf("*Ptr(3) = %d", *p)
	}
}

func TestDereferencedValue(t *testing.T) {
	n := 3
	if got := dereferencedValue(reflect.ValueOf(&n)); got.Kind() != reflect.Int {
		t.Fatalf("dereferencedValue(ptr).Kind() = %v", got.Kind())
	}
	pp := &n
	if got := dereferencedValue(reflect.ValueOf(&pp)); got.Kind() != reflect.Int {
		t.Fatalf("dereferencedValue(ptr ptr).Kind() = %v", got.Kind())
	}
	var v any = nil
	if got := dereferencedValue(reflect.ValueOf(v)); got.IsValid() {
		t.Fatalf("dereferencedValue(nil) should be invalid")
	}
}

func TestDereferencedTypeNil(t *testing.T) {
	if got := dereferencedType(nil); got != nil {
		t.Fatalf("dereferencedType(nil) = %v", got)
	}
	if got := dereferencedElemType(nil); got != nil {
		t.Fatalf("dereferencedElemType(nil) = %v", got)
	}
}

func TestKVsAsFilterHelpers(t *testing.T) {
	ctx := context.Background()
	c := &condition{}
	whereFrom(ctx, c, KVsFromMap(nil, map[string]any{"status": 0}))
	if !reflect.DeepEqual(c.exprs, []string{"`status` = ?"}) {
		t.Fatalf("exprs = %v", c.exprs)
	}
}
