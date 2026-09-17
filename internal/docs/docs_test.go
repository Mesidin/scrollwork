package docs

import "testing"

func TestEngineTopics(t *testing.T) {
	ts := EngineTopics()
	if len(ts) < 4 {
		t.Fatalf("expected engine manuals, got %d", len(ts))
	}
	if _, ok := EngineLookup("playing"); !ok {
		t.Fatal("missing playing")
	}
	if _, ok := EngineLookup("building"); !ok {
		t.Fatal("missing building")
	}
}

func TestMergePackWins(t *testing.T) {
	merged := Merge(map[string]string{
		"playing": "# Local playing\n\nThis pack overrides.",
		"house":   "# The House\n\nA local topic.",
	})
	var playing, house bool
	for _, tpc := range merged {
		if tpc.Key == "playing" {
			playing = true
			if tpc.Source != SourceGame {
				t.Fatal("pack should win on playing")
			}
		}
		if tpc.Key == "house" {
			house = true
		}
	}
	if !playing || !house {
		t.Fatal(merged)
	}
}

func TestBanner(t *testing.T) {
	if EngineBanner() == "" {
		t.Fatal("empty banner")
	}
}
