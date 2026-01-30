package set

import (
	"testing"
)

func TestSet(t *testing.T) {
	set := New(4, 2, 4, 6, 73, 2)
	s := set.String()
	t.Log(s)

	var set2 Set[int]
	err := (&set2).Scan(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(set2.Slice())
	t.Log(set2.String())
	t.Log(set2.Value())

	set3 := Set[string]{}
	t.Log(set3.String())
	t.Log(set3.Slice())
}
