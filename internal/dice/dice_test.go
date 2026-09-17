package dice

import (
	"math/rand"
	"testing"
)

func TestRollFlat(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	n, err := Roll(rng, "6")
	if err != nil || n != 6 {
		t.Fatalf("got %d %v", n, err)
	}
	n, err = Roll(rng, "10-2")
	if err != nil || n != 8 {
		t.Fatalf("got %d %v", n, err)
	}
}

func TestRollDiceRange(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 50; i++ {
		n, err := Roll(rng, "2d4+1")
		if err != nil {
			t.Fatal(err)
		}
		if n < 3 || n > 9 {
			t.Fatalf("2d4+1 out of range: %d", n)
		}
	}
}

func TestRollBad(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if _, err := Roll(rng, "fireball"); err == nil {
		t.Fatal("expected error")
	}
}
