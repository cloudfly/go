package flagx

import (
	"encoding/base64"
	"flag"
)

// Text is a flag that used to hold complex content, such as json.
// the passed value should be base64-encoded string, it will be decoded before assignment.
type Text struct {
	Value string
}

func (t *Text) MarshalText() ([]byte, error) {
	r := base64.StdEncoding.EncodeToString([]byte(t.Value))
	return []byte(r), nil
}

func (t *Text) UnmarshalText(text []byte) error {
	r, err := base64.StdEncoding.DecodeString(string(text))
	if err != nil {
		return err
	}
	t.Value = string(r)
	return nil
}

// NewText creates a new text flag.
func NewText(name string, value string, usage string, opts ...Option) *Text {
	t := Text{Value: value}
	x := (&option{}).apply(opts)
	usage += "\nThe flag value shoud be a base64-encoded string, it will be decoded before assignment"
	flag.TextVar(&t, name, &t, x.usage(name, usage))
	return &t
}
