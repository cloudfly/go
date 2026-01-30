package floats

import (
	"math"
	"testing"
)

func TestU2B(t *testing.T) {
	var n uint64 = 325
	b := U2B(n)
	if n != B2U(b) {
		t.Fatalf("%d should equal to %s", n, b)
	}
}

func TestU2BFloat64(t *testing.T) {
	var f float64 = 325.2342
	u := U2B(math.Float64bits(f))

	if f != math.Float64frombits(B2U(u)) {
		t.Fatalf("%f should equal to %s", f, u)
	}
}

func TestStoreFloat64(t *testing.T) {
	var f float64 = 1

	StoreFloat64(&f, 3.4)

	if float64(3.4) != f {
		t.Fatalf("%f should equal to %f", f, 3.4)
	}

	if float64(3.4) != LoadFloat64(&f) {
		t.Fatalf("%f should equal to %f", 3.4, LoadFloat64(&f))
	}
}

func TestFloat64(t *testing.T) {
	v, err := Float64([]byte{
		49, 48, 48,
	})
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	t.Log(v)

	v, err = Float64("100")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	t.Log(v)
}
