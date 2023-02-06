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
	if got := f.Label(1); got != "<FLAG[1]>" {
		t.Errorf("bad label: got=%q want=\"<FLAG[1]>\"", got)
	}
	f.SetAlias("F")
	if got := f.Label(2); got != "<F[2]>" {
		t.Errorf("bad label: got=%q want=\"<F[2]>\"", got)
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
	if got := v.Label(1); got != "<VECTOR[1]>" {
		t.Errorf("bad label: got=%q want=\"<R[1]>\"", got)
	}
	v.SetAlias("R")
	if got := v.Label(2); got != "<R[2]>" {
		t.Errorf("bad label: got=%q want=\"<R[2]>\"", got)
	}
}
