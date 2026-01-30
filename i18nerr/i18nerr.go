package i18nerr

import (
	"fmt"
	"strings"
)

type Code string

const (
	InvalidParameter  Code = "InvalidParameter"
	OperationDenied   Code = "OperationDenied"
	PermissionDenied  Code = "PermissionDenied"
	AlreadyExist      Code = "AlreadyExist"
	NotFound          Code = "NotFound"
	OperationConflict Code = "OperationConflict"
	LimitExceeded     Code = "LimitExceeded"
	TokenExpired      Code = "TokenExpired"
	LoginRequired     Code = "LoginRequired"
	InternalError     Code = "InternalError"
	Unknown           Code = "Unknown"
)

type Error struct {
	// Code represent the error code in string
	Code Code `json:"Code" query:"Code"`
	// chinese
	Zh string `json:"Zh" query:"Zh"`
	// chinese in taiwan
	Tw string `json:"Tw" query:"Tw"`
	// english
	En string `json:"En" query:"En"`
	// japanese
	Ja string `json:"Ja" query:"Ja"`
	// korean
	Ko string `json:"Ko" query:"Ko"`
	// french
	Fr string `json:"Fr" query:"Fr"`
	// russian
	Ru string `json:"Ru" query:"Ru"`
	// arguments used for format string
	Args []any `json:"Args,omitempty" query:"Args"`
}

func (d Error) Error() string {
	return d.ErrorIn(d.En)
}

func (d Error) ErrorIn(lang string) string {
	format := d.En
	lang = strings.ToLower(lang)
	switch {
	case strings.Contains(lang, "zh"):
		format = d.Zh
	case strings.Contains(lang, "en"):
		format = d.En
	case strings.Contains(lang, "ja"):
		format = d.Ja
	case strings.Contains(lang, "ko"):
		format = d.Ko
	case strings.Contains(lang, "fr"):
		format = d.Fr
	case strings.Contains(lang, "ru"):
		format = d.Ru
	case strings.Contains(lang, "tw"):
		format = d.Tw
	}
	if len(d.Args) == 0 {
		return format
	}
	return fmt.Sprintf(format, d.Args...)
}

func (d Error) New(args ...any) Error {
	result := d
	result.Args = append([]any{}, args...)
	if result.Code == "" {
		result.Code = Unknown
	}
	return result
}
