package combat

import (
	"math/rand"
	"strings"
	"testing"

	"sudengine/internal/rpg"
	"sudengine/internal/world"
)

func TestHitModCanTurnAHitIntoAMiss(t *testing.T) {
	w := world.New()
	att := &world.Entity{ID: "p", Kind: world.KindPlayer, Name: "Ada", Resources: map[string]world.Resource{"hp": {Current: 10, Max: 10}}}
	def := &world.Entity{ID: "wolf", Kind: world.KindMobile, Name: "wolf", Resources: map[string]world.Resource{"hp": {Current: 10, Max: 10}}}
	w.Add(att)
	w.Add(def)
	var lines []string
	e := &Engine{
		RNG:     rand.New(rand.NewSource(1)),
		Primary: "hp",
		Rules: &rpg.Schema{Checks: map[string]rpg.Check{
			"hit": {Roll: "6", Difficulty: 6},
		}},
		Emit: func(_ *world.Entity, _ string, text string) {
			lines = append(lines, text)
		},
	}
	e.swing(w, att, def)
	if len(lines) == 0 || strings.Contains(lines[0], "miss") {
		t.Fatalf("unpenalized 6 vs 6 should hit, got %v", lines)
	}
	lines = nil
	e.HitMod = func(*world.Entity) int { return 1 }
	e.swing(w, att, def)
	if len(lines) == 0 || !strings.Contains(lines[0], "miss") {
		t.Fatalf("penalty should miss, got %v", lines)
	}
}
