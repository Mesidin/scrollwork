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
		Emit: func(_ *world.Entity, _ string, text string, _ Hear, _ ...*world.Entity) {
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

type heard struct {
	to      world.ID
	channel string
	text    string
	hear    Hear
	except  []world.ID
}

func recordingEngine() (*Engine, *[]heard) {
	got := []heard{}
	e := &Engine{
		Primary: "hp",
		Emit: func(to *world.Entity, channel, text string, hear Hear, also ...*world.Entity) {
			h := heard{channel: channel, text: text, hear: hear}
			if to != nil {
				h.to = to.ID
			}
			for _, ex := range also {
				if ex != nil {
					h.except = append(h.except, ex.ID)
				}
			}
			got = append(got, h)
		},
	}
	return e, &got
}

func TestSwingLinesSplitTheRoom(t *testing.T) {
	ada := &world.Entity{ID: "ada", Kind: world.KindPlayer, Short: "Ada"}
	bob := &world.Entity{ID: "bob", Kind: world.KindPlayer, Short: "Bob"}
	wolf := &world.Entity{ID: "wolf", Kind: world.KindMobile, Short: "a grey wolf"}
	rat := &world.Entity{ID: "rat", Kind: world.KindMobile, Short: "a rat"}
	slash := world.Attack{
		Name: "slash", Damage: "1d6",
		Hit: "You slash {name}.", Miss: "You miss {name}.",
		HitRoom: "{Name} slashes {name}.", MissRoom: "{Name} misses {name}.",
	}
	bite := world.Attack{
		Name: "bite", Damage: "1d4",
		Hit: "The {Name} tore into you.", Miss: "The {Name} snapped and missed.",
		HitRoom: "The {Name} tore into {name}.", MissRoom: "The {Name} snapped at {name}.",
	}

	e, got := recordingEngine()
	e.report(nil, ada, wolf, slash, 4, nil, false)
	want := []heard{
		{to: "ada", channel: "narrative", text: "You slash a grey wolf.", hear: HearPrivate},
		{to: "ada", channel: "narrative", text: "Ada slashes a grey wolf.", hear: HearOthers},
	}
	if !sameHeard(*got, want) {
		t.Fatalf("player swing:\n got %#v\nwant %#v", *got, want)
	}

	*got = nil
	e.report(nil, ada, wolf, slash, 0, nil, true)
	want = []heard{
		{to: "ada", channel: "narrative", text: "You miss a grey wolf.", hear: HearPrivate},
		{to: "ada", channel: "narrative", text: "Ada misses a grey wolf.", hear: HearOthers},
	}
	if !sameHeard(*got, want) {
		t.Fatalf("player miss:\n got %#v\nwant %#v", *got, want)
	}

	*got = nil
	e.report(nil, wolf, ada, bite, 3, nil, false)
	want = []heard{
		{to: "ada", channel: "combat", text: "The A grey wolf tore into you.", hear: HearPrivate},
		{to: "ada", channel: "combat", text: "The A grey wolf tore into Ada.", hear: HearOthers},
	}
	if !sameHeard(*got, want) {
		t.Fatalf("wolf hit:\n got %#v\nwant %#v", *got, want)
	}

	*got = nil
	e.report(nil, wolf, ada, bite, 0, nil, true)
	want = []heard{
		{to: "ada", channel: "narrative", text: "The A grey wolf snapped and missed.", hear: HearPrivate},
		{to: "ada", channel: "narrative", text: "The A grey wolf snapped at Ada.", hear: HearOthers},
	}
	if !sameHeard(*got, want) {
		t.Fatalf("wolf miss:\n got %#v\nwant %#v", *got, want)
	}

	*got = nil
	e.report(nil, ada, bob, slash, 4, nil, false)
	want = []heard{
		{to: "ada", channel: "narrative", text: "You slash Bob.", hear: HearPrivate},
		{to: "bob", channel: "combat", text: "Ada slashes you for 4.", hear: HearPrivate},
		{to: "ada", channel: "narrative", text: "Ada slashes Bob.", hear: HearOthers, except: []world.ID{"bob"}},
	}
	if !sameHeard(*got, want) {
		t.Fatalf("player versus player:\n got %#v\nwant %#v", *got, want)
	}

	*got = nil
	e.report(nil, wolf, rat, world.Attack{Name: "bite", Hit: "The {Name} tore into you."}, 2, nil, false)
	if len(*got) != 1 || (*got)[0].hear != HearRoom || strings.Contains((*got)[0].text, "you") {
		t.Fatalf("mob versus mob leaked a you-line: %#v", *got)
	}
	if (*got)[0].text != "A grey wolf bites a rat for 2." {
		t.Fatalf("mob versus mob room line: %q", (*got)[0].text)
	}

	*got = nil
	e.report(nil, wolf, rat, bite, 2, nil, false)
	if len(*got) != 1 || (*got)[0].text != "The A grey wolf tore into a rat." || (*got)[0].hear != HearRoom {
		t.Fatalf("mob versus mob template: %#v", *got)
	}

	*got = nil
	e.Rules = &rpg.Schema{Feedback: rpg.Feedback{Numbers: true}}
	e.report(nil, ada, wolf, world.Attack{Name: "hit", Damage: "1d4"}, 4, nil, false)
	if len(*got) != 2 {
		t.Fatalf("unarmed lines: %#v", *got)
	}
	if (*got)[0].text != "You hit a grey wolf for 4." || strings.Contains((*got)[0].text, "[-") {
		t.Fatalf("unarmed private line changed: %q", (*got)[0].text)
	}
	if (*got)[1].hear != HearOthers || (*got)[1].text != "Ada hits a grey wolf for 4." {
		t.Fatalf("unarmed room line: %#v", (*got)[1])
	}

	*got = nil
	e.report(nil, ada, wolf, slash, 4, nil, false)
	if !strings.Contains((*got)[0].text, "[-4 hp]") || !strings.Contains((*got)[1].text, "[-4 hp]") {
		t.Fatalf("templated lines should both show the bracket: %#v", *got)
	}

	*got = nil
	e.Rules = nil
	e.Damage(nil, ada, wolf, 5, "strike")
	want = []heard{
		{to: "ada", channel: "narrative", text: "You strike a grey wolf for 5.", hear: HearPrivate},
		{to: "ada", channel: "narrative", text: "Ada strikes a grey wolf for 5.", hear: HearOthers},
	}
	if !sameHeard(*got, want) {
		t.Fatalf("script damage:\n got %#v\nwant %#v", *got, want)
	}

	*got = nil
	e.Damage(nil, ada, bob, 5, "strike")
	want = []heard{
		{to: "ada", channel: "narrative", text: "You strike Bob for 5.", hear: HearPrivate},
		{to: "bob", channel: "combat", text: "Ada strikes you for 5.", hear: HearPrivate},
		{to: "ada", channel: "narrative", text: "Ada strikes Bob for 5.", hear: HearOthers, except: []world.ID{"bob"}},
	}
	if !sameHeard(*got, want) {
		t.Fatalf("script damage on a player:\n got %#v\nwant %#v", *got, want)
	}
}

func sameHeard(got, want []heard) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i].to != want[i].to || got[i].channel != want[i].channel || got[i].text != want[i].text || got[i].hear != want[i].hear {
			return false
		}
		if len(got[i].except) != len(want[i].except) {
			return false
		}
		for j := range want[i].except {
			if got[i].except[j] != want[i].except[j] {
				return false
			}
		}
	}
	return true
}
