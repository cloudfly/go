package deepcopy

import (
	"testing"
)

type Data struct {
	Slice []byte
	Any   any
}

func TestSlice(t *testing.T) {
	from := []byte("hello")
	var to []byte
	err := Copy(&to, from)
	if err != nil {
		t.Fatal(err.Error())
	}
	from[0] = 'W'
	if string(to) != "hello" {
		t.Log(string(to))
		t.FailNow()
	}
}

func TestSliceInStruct(t *testing.T) {
	from := Data{
		Slice: []byte("hello"),
	}
	var to Data
	err := Copy(&to, from)
	if err != nil {
		t.Fatal(err.Error())
	}
	from.Slice[0] = 'W'
	if string(to.Slice) != "hello" {
		t.Log(string(to.Slice))
		t.FailNow()
	}
}

func TestAnyInStruct(t *testing.T) {
	from := Data{
		Any: []byte("hello"),
	}
	var to Data
	err := Copy(&to, from)
	if err != nil {
		t.Fatal(err.Error())
	}
	from.Any.([]byte)[0] = 'W'
	if string(to.Any.([]byte)) != "hello" {
		t.Log(string(to.Slice))
		t.FailNow()
	}
}
