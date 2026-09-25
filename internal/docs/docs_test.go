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
	var gamePlaying, enginePlaying, house int
	for _, tpc := range merged {
		if tpc.Key == "playing" && tpc.Source == SourceGame {
			gamePlaying++
		}
		if tpc.Key == "playing" && tpc.Source == SourceEngine {
			enginePlaying++
		}
		if tpc.Key == "house" && tpc.Source == SourceGame {
			house++
		}
	}
	if gamePlaying != 1 || enginePlaying != 1 || house != 1 {
		t.Fatalf("game playing %d, engine playing %d, house %d", gamePlaying, enginePlaying, house)
	}
	if merged[0].Key != "house" && merged[0].Key != "playing" {
		t.Fatalf("pack topics should lead, got %s", merged[0].Key)
	}
}

func TestBanner(t *testing.T) {
	if EngineBanner() == "" {
		t.Fatal("empty banner")
	}
}
