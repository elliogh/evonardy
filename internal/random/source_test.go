package random_test

import (
	"evonardy/internal/random"
	"testing"
)

func TestNamedStreamsAndStateRestore(t *testing.T) {
	a, b := random.New(42, "dice", 7), random.New(42, "dice", 7)
	for i := 0; i < 20; i++ {
		if a.IntN(6) != b.IntN(6) {
			t.Fatal("not reproducible")
		}
	}
	state, err := a.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	c := random.New(0, "other", 0)
	if err := c.UnmarshalBinary(state); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if a.IntN(1000) != c.IntN(1000) {
			t.Fatal("restore changed stream")
		}
	}
	d := random.New(42, "exploration", 7)
	same := true
	for i := 0; i < 20; i++ {
		if b.IntN(1000) != d.IntN(1000) {
			same = false
		}
	}
	if same {
		t.Fatal("stream labels not separated")
	}
	if err := c.UnmarshalBinary([]byte("bad")); err == nil {
		t.Fatal("invalid state accepted")
	}
}
