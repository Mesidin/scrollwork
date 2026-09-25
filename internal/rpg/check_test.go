package rpg

import (
	"math/rand"
	"testing"

	"sudengine/internal/world"
)

func TestResolveOverAndUnder(t *testing.T) {
	w := world.New()
	actor := &world.Entity{
		ID: "p", Kind: world.KindPlayer,
		Attrs:  map[string]int{"will": 1},
		Skills: map[string]int{},
	}
	coat := &world.Entity{ID: "coat", Kind: world.KindItem, Mods: map[string]int{"escape": 1}}
	w.Add(actor)
	w.Add(coat)
	actor.Equipment = map[string]world.ID{"body": "coat"}

	s := Schema{
		Attributes: []AttrDef{{Key: "will", Base: 1}},
		Checks: map[string]Check{
			"escape": {Roll: "6", Mode: "over", Actor: []string{"attr.will"}, Gear: []string{"escape"}, Difficulty: 8},
			"fear":   {Roll: "50", Mode: "under", Actor: []string{"attr.will"}, Difficulty: 40},
		},
		Feedback: Feedback{Rolls: true, Numbers: true},
	}
	// 6 + will 1 + coat 1 = 8 vs 8, success.
	got, ok := s.Resolve(rand.New(rand.NewSource(1)), w, actor, "escape", 0, false)
	if !ok || !got.Success || got.Total != 8 || got.Mod != 2 {
		t.Fatalf("escape %+v ok %v", got, ok)
	}
	// Opponent 11 replaces the default. 8 vs 11 fails.
	got, ok = s.Resolve(rand.New(rand.NewSource(1)), w, actor, "escape", 11, true)
	if !ok || got.Success || got.Target != 11 {
		t.Fatalf("oppose %+v", got)
	}
	// Roll under: natural 50, ceiling 40+1 = 41, 50 <= 41 is false.
	got, ok = s.Resolve(rand.New(rand.NewSource(1)), w, actor, "fear", 0, false)
	if !ok || got.Success || got.Target != 41 || got.Natural != 50 {
		t.Fatalf("under %+v", got)
	}
	line := s.Decorate("The wolf bit you.", 4, false, "Vigor", &got)
	if line == "The wolf bit you." {
		t.Fatal("expected roll and number decoration")
	}
}

func TestSkillParentAndDamage(t *testing.T) {
	s := Schema{
		Attributes: []AttrDef{{Key: "body"}},
		Skills:     []SkillDef{{Key: "melee", Parent: "body", DamageEvery: 2, Max: 10}},
	}
	e := &world.Entity{Attrs: map[string]int{"body": 2}, Skills: map[string]int{"melee": 2}}
	if got := s.Effective(e, "skill.melee"); got != 4 {
		t.Fatalf("effective %d", got)
	}
	if got := s.DamageBonus(e, "melee"); got != 2 {
		t.Fatalf("damage bonus %d", got)
	}
}

func TestGrantLevels(t *testing.T) {
	s := Schema{Advancement: Advancement{
		Enabled: true, XP: 20, XPStep: 10, SkillPoints: 1, AttributeEvery: 10,
		Resources: map[string]int{"hp": 2},
	}}
	e := &world.Entity{Level: 1, Resources: map[string]world.Resource{"hp": {Current: 10, Max: 10}}}
	msgs := s.Grant(e, 20)
	if e.Level != 2 || e.SkillPoints != 1 || e.Res("hp").Max != 12 || len(msgs) != 1 {
		t.Fatalf("level %+v msgs %v", e, msgs)
	}
	e.Level = 9
	e.XP = 0
	msgs = s.Grant(e, s.Advancement.Cost(9))
	if e.Level != 10 || e.AttrPoints != 1 {
		t.Fatalf("attr point level %d points %d", e.Level, e.AttrPoints)
	}
	if s.Grant(e, 0) != nil {
		t.Fatal("zero xp")
	}
}

func TestShowDefaults(t *testing.T) {
	s := Schema{
		PrimaryResource: "vigor",
		Resources: []ResDef{
			{Key: "vigor"},
			{Key: "focus"},
			{Key: "coin", Pile: true},
			{Key: "luck", Show: "sheet"},
		},
	}
	if s.Show("vigor") != "bar" || s.Show("focus") != "side" || s.Show("coin") != "side" || s.Show("luck") != "sheet" {
		t.Fatalf("vigor %s focus %s coin %s luck %s", s.Show("vigor"), s.Show("focus"), s.Show("coin"), s.Show("luck"))
	}
}

func TestOmittedCheck(t *testing.T) {
	s := Schema{}
	_, ok := s.Resolve(nil, nil, nil, "escape", 0, false)
	if ok {
		t.Fatal("missing check should not roll")
	}
}
