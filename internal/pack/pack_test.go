package pack

import (
	"path/filepath"
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
}
