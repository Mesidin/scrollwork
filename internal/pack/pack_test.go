package pack

import (
	"os"
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

func TestRecipesLoadAndRepairableDefaultsOff(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Recipes) != 3 {
		t.Fatalf("recipes %d", len(p.Recipes))
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	e := itemFromYAML(ItemYAML{ID: "x", Name: "x", Durability: 3, MaxDurability: 3})
	if e.Repairable {
		t.Fatal("repairable should default off")
	}

	wrapped := t.TempDir()
	if err := os.WriteFile(filepath.Join(wrapped, "pack.yaml"), []byte("id: wrap\ntitle: Wrap\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "recipes:\n  - id: cup\n    name: clay cup\n    produces:\n      item: item.cup\n      count: 2\n    skill: pottery\n"
	if err := os.WriteFile(filepath.Join(wrapped, "recipes.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	wp, err := Load(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if len(wp.Recipes) != 1 || wp.Recipes[0].Produces != "item.cup" || wp.Recipes[0].Count != 2 {
		t.Fatalf("%+v", wp.Recipes)
	}
	verr := wp.Validate()
	if verr == nil || !strings.Contains(verr.Error(), "item.cup") || !strings.Contains(verr.Error(), "pottery") {
		t.Fatal(verr)
	}

	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "pack.yaml"), []byte("id: bad\ntitle: Bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "recipes.yaml"), []byte("recipes: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Fatal("broken recipes.yaml should fail load")
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

func TestEmptyRecipeFileIsCleared(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "recipes.yaml"), []byte("- id: old\n  name: old\n  produces: item.old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &Pack{Dir: dir, Meta: Meta{ID: "t", Title: "T", StartRoom: "room"}}
	if err := p.WriteWorld(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "recipes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("deleted recipes should clear the file, got %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "quests.yaml")); !os.IsNotExist(err) {
		t.Fatal("quests.yaml should not be created for a pack that never had one")
	}
}
