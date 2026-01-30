package resource

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

const (
	TypeString  = "string"
	TypeNumber  = "number"
	TypeObject  = "object"
	TypeArray   = "array"
	TypeBoolean = "boolean"
	TypeNull    = "null"
	TypeUnknown = "unknown"
)

type Validator interface {
	Validate(string, interface{}) error
}

func NewValidator(name string, value string) (Validator, error) {
	switch name {
	case "ip":
		return RuleIp(true), nil
	case "ipv4":
		return RuleIpv4(true), nil
	case "ipv6":
		return RuleIpv6(true), nil
	case "url":
		return RuleUrl(true), nil
	case "maxLength":
		max, err := strconv.Atoi(value)
		return RuleMaxLength(max), err
	case "minLength":
		min, err := strconv.Atoi(value)
		return RuleMinLength(min), err
	case "startsWith":
		return RuleStartsWith(value), nil
	case "startsNotWith":
		return RuleStartsNotWith(value), nil
	case "endsWith":
		return RuleEndsWith(value), nil
	case "endsNotWith":
		return RuleEndsNotWith(value), nil
	case "contains":
		return RuleContains(value), nil
	case "notContains":
		return RuleNotContains(value), nil
	case "match":
		return RuleMatch(value), nil
	case "max":
		i, err := strconv.ParseInt(value, 10, 64)
		return RuleMax(i), err
	case "min":
		i, err := strconv.Atoi(value)
		return RuleMin(i), err
	}
	return nil, fmt.Errorf("unknown validator '%s'", name)
}

type RuleMaxLength int

func (rule RuleMaxLength) Validate(t string, data interface{}) error {
	// MaxLength 只对 string 和 array 生效
	switch t {
	case TypeArray, TypeString:
		if reflect.ValueOf(data).Len() > int(rule) {
			return fmt.Errorf("maximum length(%d) limit exceeded", rule)
		}
	}
	return nil
}

type RuleMinLength int

func (rule RuleMinLength) Validate(t string, data interface{}) error {
	// MaxLength 只对 string 和 array 生效
	switch t {
	case TypeArray, TypeString:
		if reflect.ValueOf(data).Len() < int(rule) {
			return fmt.Errorf("minimum length(%d) limit exceeded", rule)
		}
	}
	return nil
}

type RuleStartsWith string

func (rule RuleStartsWith) Validate(t string, data interface{}) error {
	if t == TypeString && !strings.HasPrefix(reflect.ValueOf(data).String(), string(rule)) {
		return fmt.Errorf("value must starts with '%s'", rule)
	}
	return nil
}

type RuleStartsNotWith string

func (rule RuleStartsNotWith) Validate(t string, data interface{}) error {
	if t == TypeString && strings.HasPrefix(reflect.ValueOf(data).String(), string(rule)) {
		return fmt.Errorf("value can not starts with '%s'", rule)
	}
	return nil
}

type RuleEndsWith string

func (rule RuleEndsWith) Validate(t string, data interface{}) error {
	if t == TypeString && !strings.HasSuffix(reflect.ValueOf(data).String(), string(rule)) {
		return fmt.Errorf("value must ends with '%s'", rule)
	}
	return nil
}

type RuleEndsNotWith string

func (rule RuleEndsNotWith) Validate(t string, data interface{}) error {
	if t == TypeString && strings.HasSuffix(reflect.ValueOf(data).String(), string(rule)) {
		return fmt.Errorf("value can not ends with '%s'", rule)
	}
	return nil
}

type RuleContains string

func (rule RuleContains) Validate(t string, data interface{}) error {
	if t == TypeString && strings.Contains(reflect.ValueOf(data).String(), string(rule)) {
		return fmt.Errorf("value must contain '%s'", rule)
	}
	return nil
}

type RuleNotContains string

func (rule RuleNotContains) Validate(t string, data interface{}) error {
	if t == TypeString && strings.Contains(reflect.ValueOf(data).String(), string(rule)) {
		return fmt.Errorf("value can not contain '%s'", rule)
	}
	return nil
}

type RuleMax int64

func (rule RuleMax) Validate(t string, data interface{}) error {
	if t == TypeNumber {
		if reflect.ValueOf(data).Float() > float64(rule) {
			return fmt.Errorf("value can not greator than %v", rule)
		}
	}
	return nil
}

type RuleMin int64

func (rule RuleMin) Validate(t string, data interface{}) error {
	if t == TypeNumber {
		if reflect.ValueOf(data).Float() < float64(rule) {
			return fmt.Errorf("value can not smaller than %v", rule)
		}
	}
	return nil
}

type RuleIpv4 bool

var ipv4Regexp = regexp.MustCompile(`^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)

func (rule RuleIpv4) Validate(t string, data interface{}) error {
	if t == TypeString && !ipv4Regexp.MatchString(reflect.ValueOf(data).String()) {
		return fmt.Errorf("ipv4 format error")
	}
	return nil
}

type RuleIpv6 bool

var ipv6Regexp = regexp.MustCompile(`(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))`)

func (rule RuleIpv6) Validate(t string, data interface{}) error {
	if t == TypeString && !ipv6Regexp.MatchString(reflect.ValueOf(data).String()) {
		return fmt.Errorf("ipv6 format error")
	}
	return nil
}

type RuleIp bool

func (rule RuleIp) Validate(t string, data interface{}) error {
	if t == TypeString && !ipv6Regexp.MatchString(reflect.ValueOf(data).String()) && !ipv4Regexp.MatchString(reflect.ValueOf(data).String()) {
		return fmt.Errorf("ip format error")
	}
	return nil
}

type RuleUrl bool

var urlRegexp = regexp.MustCompile(`https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b([-a-zA-Z0-9()@:%_\+.~#?&//=]*)`)

func (rule RuleUrl) Validate(t string, data interface{}) error {
	if t == TypeString && !urlRegexp.MatchString(reflect.ValueOf(data).String()) {
		return fmt.Errorf("url format error")
	}
	return nil
}

type RuleEnums []string

func (rule RuleEnums) Validate(t string, data interface{}) error { return nil }

type RuleMatch string

func (rule RuleMatch) Validate(t string, data interface{}) error {
	if t != TypeString {
		return nil
	}
	exp, err := regexp.Compile(string(rule))
	if err != nil {
		return err
	}
	if !exp.MatchString(reflect.ValueOf(data).String()) {
		return fmt.Errorf("value not match %v", rule)
	}
	return nil
}
