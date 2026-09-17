package pack

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOldHouse(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Meta.ID != "old-house" {
		t.Fatalf("id %s", p.Meta.ID)
	}
	w, err := p.Instantiate()
	if err != nil {
		t.Fatal(err)
	}
	if w.Get("house.foyer") == nil {
		t.Fatal("missing foyer")
	}
	SpawnPlayer(w, p, "you", "", "")
	if w.Player() == nil {
		t.Fatal("no player")
	}
	if w.RoomOf(w.Player()).ID != "house.porch" {
		t.Fatalf("start %s", w.RoomOf(w.Player()).ID)
	}
	if p.IntroArt == "" {
		t.Fatal("expected intro art")
	}
	if !p.RPG.ChargenEnabled() {
		t.Fatal("chargen should be on")
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCatchesBadExit(t *testing.T) {
	p := Blank("t", t.TempDir())
	p.Rooms[0].Exits = map[string]ExitYAML{"north": {To: "nowhere"}}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nowhere") {
		t.Fatal(err)
	}
}

func TestInstallDir(t *testing.T) {
	src := filepath.Join("..", "..", "games", "old-house")
	destRoot := t.TempDir()
	info, err := Install(src, destRoot)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "old-house" {
		t.Fatal(info.ID)
	}
	if _, err := Load(info.Dir); err != nil {
		t.Fatal(err)
	}
}
