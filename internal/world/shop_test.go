package world

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPurseZeroStaysFinite(t *testing.T) {
	var s Shop
	if err := yaml.Unmarshal([]byte("currency: coin\npurse: 0\n"), &s); err != nil {
		t.Fatal(err)
	}
	if !s.Finite {
		t.Fatal("purse: 0 should mark the till finite")
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var again Shop
	if err := json.Unmarshal(b, &again); err != nil {
		t.Fatal(err)
	}
	if !again.Finite {
		t.Fatalf("saved till lost finite: %s", b)
	}
}

func TestPileGrowsPastMax(t *testing.T) {
	e := &Entity{Resources: map[string]Resource{
		"coin":  {Current: 12, Max: 12, Pile: true},
		"vigor": {Current: 10, Max: 10},
	}}
	if e.AdjustRes("coin", 8) != 20 {
		t.Fatalf("coin %d", e.Res("coin").Current)
	}
	if e.AdjustRes("vigor", 5) != 10 {
		t.Fatalf("vigor %d", e.Res("vigor").Current)
	}
}
