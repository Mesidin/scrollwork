package commands

import (
	"strings"
	"testing"

	"sudengine/internal/economy"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/world"
)

func TestMatchRecipesUsesWords(t *testing.T) {
	recipes := []economy.Recipe{
		{ID: "craft_cudgel", Name: "carved cudgel"},
		{ID: "craft_torch", Name: "pitch torch"},
	}
	if got := matchRecipes(recipes, "a"); len(got) != 0 {
		t.Fatalf("craft a matched %+v", got)
	}
	if got := matchRecipes(recipes, "c"); len(got) != 0 {
		t.Fatalf("craft c matched %+v", got)
	}
	got := matchRecipes(recipes, "torch")
	if len(got) != 1 || got[0].ID != "craft_torch" {
		t.Fatalf("torch -> %+v", got)
	}
	got = matchRecipes(recipes, "cudgel")
	if len(got) != 1 || got[0].ID != "craft_cudgel" {
		t.Fatalf("cudgel -> %+v", got)
	}
}

func TestRepairAllSkipsWhatYouCannotAfford(t *testing.T) {
	w := world.New()
	room := &world.Entity{ID: "room", Kind: world.KindRoom, Contents: []world.ID{"smith", "player"}}
	smith := &world.Entity{ID: "smith", Kind: world.KindMobile, Name: "Smith", Repairer: true, Parent: room.ID}
	sword := &world.Entity{
		ID: "sword", Kind: world.KindItem, Name: "sword", Short: "a sword", Keywords: []string{"sword"},
		Durability: 0, MaxDurability: 10, Value: 20, Repairable: true, Parent: "player",
	}
	cap := &world.Entity{
		ID: "cap", Kind: world.KindItem, Name: "cap", Short: "a cap", Keywords: []string{"cap"},
		Durability: 8, MaxDurability: 10, Value: 2, Repairable: true, Parent: "player",
	}
	player := &world.Entity{
		ID: "player", Kind: world.KindPlayer, Name: "Ada", Parent: room.ID,
		Contents:  []world.ID{"sword", "cap"},
		Resources: map[string]world.Resource{"coin": {Current: 5, Max: 5}},
	}
	for _, e := range []*world.Entity{room, smith, sword, cap, player} {
		w.Entities[e.ID] = e
	}
	lines := tellLines()
	c := &Context{World: w, Actor: player, Pack: &pack.Pack{}, Tell: lines.tell}
	repairCmd(c, Parsed{Rest: "all"})
	if cap.Durability != 10 {
		t.Fatalf("cap durability %d", cap.Durability)
	}
	if sword.Durability != 0 {
		t.Fatalf("sword was repaired")
	}
	if !strings.Contains(lines.text(), "Still needs repair") || !strings.Contains(lines.text(), "a sword") {
		t.Fatalf("%s", lines.text())
	}

	smith.Repairer = false
	smith.Shop = &world.Shop{Currency: "coin", Purse: 50, Finite: true}
	lines.reset()
	repairCmd(c, Parsed{Rest: "cap"})
	if !strings.Contains(lines.text(), "No one here can repair") {
		t.Fatalf("grocer repaired: %s", lines.text())
	}
}

func TestSellGrowsACoinPile(t *testing.T) {
	w := world.New()
	room := &world.Entity{ID: "room", Kind: world.KindRoom, Contents: []world.ID{"bram", "player"}}
	bram := &world.Entity{
		ID: "bram", Kind: world.KindMobile, Name: "Bram", Parent: room.ID,
		Shop: &world.Shop{Currency: "coin", Purse: 30, Finite: true},
	}
	sword := &world.Entity{
		ID: "sword", Kind: world.KindItem, Name: "short sword", Short: "a short sword",
		Keywords: []string{"sword"}, Value: 8, Takeable: true, Parent: "player",
	}
	player := &world.Entity{
		ID: "player", Kind: world.KindPlayer, Name: "Ada", Parent: room.ID,
		Contents:  []world.ID{"sword"},
		Resources: map[string]world.Resource{"coin": {Current: 12, Max: 12, Pile: true}},
	}
	for _, e := range []*world.Entity{room, bram, sword, player} {
		w.Entities[e.ID] = e
	}
	lines := tellLines()
	c := &Context{World: w, Actor: player, Pack: &pack.Pack{}, Tell: lines.tell}
	sell(c, Parsed{Rest: "sword"})
	if player.Res("coin").Current != 20 {
		t.Fatalf("coin %d, lines %s", player.Res("coin").Current, lines.text())
	}
	if bram.Shop.Purse != 22 {
		t.Fatalf("till %d", bram.Shop.Purse)
	}
	if !strings.Contains(lines.text(), "for 8") {
		t.Fatalf("%s", lines.text())
	}
}

type lineBuf struct {
	lines []string
}

func tellLines() *lineBuf { return &lineBuf{} }

func (b *lineBuf) tell(_ protocol.Channel, text string) { b.lines = append(b.lines, text) }

func (b *lineBuf) text() string { return strings.Join(b.lines, "\n") }

func (b *lineBuf) reset() { b.lines = nil }
