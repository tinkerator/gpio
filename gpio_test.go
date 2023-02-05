package gpio

import "testing"

func TestFlag(t *testing.T) {
	var f *Flag
	if err := f.valid(300); err == nil {
		t.Fatalf("nil Flag gives no error for index 300")
	}

	f = NewFlag()
	if err := f.Set(1, true); err != nil {
		t.Fatalf("unable to set flag[1]: %v", err)
	}
	if v, err := f.Get(1); err != nil {
		t.Fatalf("failed to read flag[1]: %v", err)
	} else if !v {
		t.Fatalf("reading flag[1], got=%v want=true", v)
	}
}

func TestVector(t *testing.T) {
	var v *Vector
	if err := v.valid(3); err == nil {
		t.Fatalf("nil Vector gives no error for index 3")
	}
	v = NewVector(5)
	if err := v.Set(1, 42); err != nil {
		t.Fatalf("unable to set vec[1]: %v", err)
	}
	if n, err := v.Get(1); err != nil {
		t.Fatalf("failed to read vec[1]: %v", err)
	} else if n != 42 {
		t.Fatalf("reading vec[1], got=%v want=52", n)
	}
}
