package test

import (
	"bufio"
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

type Helper interface {
	Helper()
}

type T interface {
	Errorf(format string, args ...interface{})
	FailNow()
}

type labeledContent struct {
	label   string
	content string
}

func messageFromMsgAndArgs(msgAndArgs ...interface{}) string {
	if len(msgAndArgs) == 0 || msgAndArgs == nil {
		return ""
	}
	if len(msgAndArgs) == 1 {
		msg := msgAndArgs[0]
		if msgAsStr, ok := msg.(string); ok {
			return msgAsStr
		}
		return fmt.Sprintf("%+v", msg)
	}
	if len(msgAndArgs) > 1 {
		return fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
	}
	return ""
}

func labeledOutput(content ...labeledContent) string {
	longestLabel := 0
	for _, v := range content {
		if len(v.label) > longestLabel {
			longestLabel = len(v.label)
		}
	}
	var output string
	for _, v := range content {
		output += "\t" + v.label + ":" + strings.Repeat(" ", longestLabel-len(v.label)) + "\t" + indentMessageLines(v.content, longestLabel) + "\n"
	}
	return output
}

func indentMessageLines(message string, longestLabelLen int) string {
	outBuf := new(bytes.Buffer)

	for i, scanner := 0, bufio.NewScanner(strings.NewReader(message)); scanner.Scan(); i++ {
		// no need to align first line because it starts at the correct location (after the label)
		if i != 0 {
			// append alignLen+1 spaces to align with "{{longestLabel}}:" before adding tab
			outBuf.WriteString("\n\t" + strings.Repeat(" ", longestLabelLen+1) + "\t")
		}
		outBuf.WriteString(scanner.Text())
	}

	return outBuf.String()
}

func IsEmpty(object interface{}) bool {

	// get nil case out of the way
	if object == nil {
		return true
	}

	objValue := reflect.ValueOf(object)

	switch objValue.Kind() {
	// collection types are empty when they have no element
	case reflect.Chan, reflect.Map, reflect.Slice:
		return objValue.Len() == 0
	// pointers are empty if nil or if the value they point to is empty
	case reflect.Ptr:
		if objValue.IsNil() {
			return true
		}
		deref := objValue.Elem().Interface()
		return IsEmpty(deref)
	// for all other types, compare against the zero value
	// array types are empty when they match their zero-initialized state
	default:
		zero := reflect.Zero(objValue.Type())
		return reflect.DeepEqual(object, zero.Interface())
	}
}

func IsNil(object interface{}) bool {
	if object == nil {
		return true
	}

	value := reflect.ValueOf(object)
	kind := value.Kind()
	isNilableKind := slices.Contains(
		[]reflect.Kind{
			reflect.Chan, reflect.Func,
			reflect.Interface, reflect.Map,
			reflect.Ptr, reflect.Slice, reflect.UnsafePointer},
		kind)

	if isNilableKind && value.IsNil() {
		return true
	}

	return false
}

func Assert(t T) *Tester {
	return &Tester{
		T: t,
	}
}

func Require(t T) *Tester {
	return &Tester{
		T: t,
		ActionOnFail: func(t T) {
			t.FailNow()
		},
	}
}

type Tester struct {
	T
	ActionOnFail func(T)
}

// True asserts that the specified value is true.
//
//	assert.True(t, myBool)
func (t *Tester) True(value bool, msgAndArgs ...interface{}) bool {
	if !value {
		return t.Fail("Should be true", msgAndArgs...)
	}
	return true
}

// False asserts that the specified value is false.
//
//	assert.False(t, myBool)
func (t *Tester) False(value bool, msgAndArgs ...interface{}) bool {
	if value {
		return t.Fail("Should be false", msgAndArgs...)
	}
	return true
}

// NoError asserts that a function returned no error (i.e. `nil`).
//
//	  actualObj, err := SomeFunction()
//	  if assert.NoError(t, err) {
//		   assert.Equal(t, expectedObj, actualObj)
//	  }
func (t *Tester) NoError(err error, msgAndArgs ...interface{}) bool {
	if err != nil {
		return t.Fail(fmt.Sprintf("Received unexpected error:\n%+v", err), msgAndArgs...)
	}

	return true
}

// Error asserts that a function returned an error (i.e. not `nil`).
//
//	  actualObj, err := SomeFunction()
//	  if assert.Error(t, err) {
//		   assert.Equal(t, expectedError, err)
//	  }
func (t *Tester) Error(err error, msgAndArgs ...interface{}) bool {
	if err == nil {
		return t.Fail("An error is expected but got nil.", msgAndArgs...)
	}

	return true
}

func (t *Tester) Equal(expected, actual any, msgAndArgs ...interface{}) bool {
	if reflect.DeepEqual(expected, actual) {
		return true
	}

	return t.Fail(fmt.Sprintf("Not equal: \n"+
		"expected: %v\n"+
		"actual  : %v", expected, actual), msgAndArgs...)
}

func (t *Tester) NotEqual(expected, actual any, msgAndArgs ...interface{}) bool {
	if !reflect.DeepEqual(expected, actual) {
		return true
	}

	return t.Fail(fmt.Sprintf("Should not be: %v", actual), msgAndArgs...)
}

func (t *Tester) NotEmpty(object interface{}, msgAndArgs ...interface{}) bool {
	pass := !IsEmpty(object)
	if !pass {
		t.Fail(fmt.Sprintf("Should NOT be empty, but was %v", object), msgAndArgs...)
	}
	return pass
}

func (t *Tester) Empty(object interface{}, msgAndArgs ...interface{}) bool {
	pass := IsEmpty(object)
	if !pass {
		t.Fail(fmt.Sprintf("Should be empty, but was %v", object), msgAndArgs...)
	}
	return pass
}

func (t *Tester) Nil(object interface{}, msgAndArgs ...interface{}) bool {
	if IsNil(object) {
		return true
	}
	return t.Fail(fmt.Sprintf("Expected nil, but got: %#v", object), msgAndArgs...)
}

func (t *Tester) NotNil(object interface{}, msgAndArgs ...interface{}) bool {
	if !IsNil(object) {
		return true
	}
	return t.Fail("Expected value not to be nil.", msgAndArgs...)
}

// Fail reports a failure through
func (t *Tester) Fail(failureMessage string, msgAndArgs ...interface{}) bool {
	content := []labeledContent{
		{"Error", failureMessage},
	}

	message := messageFromMsgAndArgs(msgAndArgs...)
	if len(message) > 0 {
		content = append(content, labeledContent{"Messages", message})
	}

	t.Errorf("\n%s", ""+labeledOutput(content...))

	if t.ActionOnFail != nil {
		t.ActionOnFail(t)
	}

	return false
}
