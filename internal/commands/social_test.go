package commands_test

import (
	"math/rand"
	"strings"
	"testing"

	"sudengine/internal/commands"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/rpg"
	"sudengine/internal/world"
)

func TestWhisperRefusesExactLinkdeadBeforeSubstring(t *testing.T) {
	w := world.New()
	room := &world.Entity{ID: "room", Kind: world.KindRoom, Name: "Porch"}
	hero := &world.Entity{ID: "hero", Kind: world.KindPlayer, Name: "Hero"}
	bob := &world.Entity{ID: "player.bob", Kind: world.KindPlayer, Name: "Bob"}
	bobby := &world.Entity{ID: "player.bobby", Kind: world.KindPlayer, Name: "Bobby"}
	bob.SetFlag("linkdead", true)
	w.Add(room)
	w.Add(hero)
	w.Add(bob)
	w.Add(bobby)
	_ = w.Move(hero.ID, room.ID)
	_ = w.Move(bob.ID, room.ID)
	_ = w.Move(bobby.ID, room.ID)

	var lines []string
	c := &commands.Context{
		World: w,
		Actor: hero,
		Pack:  &pack.Pack{},
		Tell: func(_ protocol.Channel, text string) {
			lines = append(lines, text)
		},
		TellPlayer: func(player *world.Entity, _ protocol.Channel, text string) {
			lines = append(lines, player.Name+":"+text)
		},
		ConnectedPlayers: func() []*world.Entity {
			return []*world.Entity{hero, bobby}
		},
	}
	commands.Dispatch(c, commands.Parse("whisper bob hello", nil))
	got := strings.Join(lines, "\n")
	if strings.Contains(got, "Bobby") {
		t.Fatalf("link-dead Bob was skipped and Bobby took the whisper:\n%s", got)
	}
	if !strings.Contains(got, `You don't see "bob" here.`) {
		t.Fatalf("expected an exact link-dead refusal, got:\n%s", got)
	}

	lines = nil
	w.Detach(bob.ID)
	commands.Dispatch(c, commands.Parse("whisper bob hello", nil))
	got = strings.Join(lines, "\n")
	if !strings.Contains(got, "You whisper to Bobby") {
		t.Fatalf("expected a connected substring match when nobody exact is here, got:\n%s", got)
	}
}

func TestShoutSkipsClosedAndLockedExits(t *testing.T) {
	w := world.New()
	here := &world.Entity{
		ID:   "here",
		Kind: world.KindRoom,
		Name: "Porch",
		Exits: map[string]world.Exit{
			"east":  {To: "yard", Dir: "east", Closed: true},
			"west":  {To: "lane", Dir: "west"},
			"south": {To: "gate", Dir: "south", Locked: true},
			"north": {To: "hall", Dir: "north"},
		},
	}
	yard := &world.Entity{ID: "yard", Kind: world.KindRoom, Name: "Yard"}
	lane := &world.Entity{ID: "lane", Kind: world.KindRoom, Name: "Lane"}
	gate := &world.Entity{ID: "gate", Kind: world.KindRoom, Name: "Gate"}
	hall := &world.Entity{ID: "hall", Kind: world.KindRoom, Name: "Hall"}
	yard.SetFlag("outdoor", true)
	lane.SetFlag("outdoor", true)
	gate.SetFlag("outdoor", true)
	hero := &world.Entity{ID: "hero", Kind: world.KindPlayer, Name: "Hero"}
	for _, e := range []*world.Entity{here, yard, lane, gate, hall, hero} {
		w.Add(e)
	}
	_ = w.Move(hero.ID, here.ID)

	var heard []string
	c := &commands.Context{
		World: w,
		Actor: hero,
		Pack:  &pack.Pack{},
		Tell:  func(_ protocol.Channel, text string) {},
		TellRoom: func(room *world.Entity, _ protocol.Channel, text string, _ ...*world.Entity) {
			heard = append(heard, string(room.ID)+":"+text)
		},
	}
	commands.Dispatch(c, commands.Parse("shout hello", nil))
	got := strings.Join(heard, "\n")
	if !strings.Contains(got, "lane:") {
		t.Fatalf("expected the open outdoor room to hear the shout, got:\n%s", got)
	}
	if strings.Contains(got, "yard:") || strings.Contains(got, "gate:") || strings.Contains(got, "hall:") {
		t.Fatalf("closed, locked, or indoor rooms heard the shout:\n%s", got)
	}
}

func TestAbilityDamageFailsBadDiceAndClamps(t *testing.T) {
	w := world.New()
	room := &world.Entity{ID: "room", Kind: world.KindRoom, Name: "Yard"}
	room.SetFlag("pvp", true)
	hero := &world.Entity{
		ID:   "hero",
		Kind: world.KindPlayer,
		Name: "Hero",
		Resources: map[string]world.Resource{
			"calm": {Current: 3, Max: 3},
			"hp":   {Current: 10, Max: 10},
		},
	}
	rat := &world.Entity{
		ID:        "rat",
		Kind:      world.KindMobile,
		Name:      "rat",
		Resources: map[string]world.Resource{"hp": {Current: 10, Max: 10}},
	}
	w.Add(room)
	w.Add(hero)
	w.Add(rat)
	_ = w.Move(hero.ID, room.ID)
	_ = w.Move(rat.ID, room.ID)

	var lines []string
	var harmed []int
	c := &commands.Context{
		World: w,
		Actor: hero,
		Pack: &pack.Pack{RPG: rpg.Schema{Abilities: []rpg.Ability{{
			ID:     "zap",
			Name:   "Zap",
			Verb:   "zap",
			Target: "any",
			Cost:   map[string]int{"calm": 1},
			Effects: []rpg.Effect{{
				Damage: &rpg.ResDice{Dice: "nope"},
			}},
		}}}},
		RNG: rand.New(rand.NewSource(1)),
		Tell: func(_ protocol.Channel, text string) {
			lines = append(lines, text)
		},
		Harm: func(target *world.Entity, amount int, _ string) {
			harmed = append(harmed, amount)
			target.AdjustRes("hp", -amount)
		},
	}
	commands.Dispatch(c, commands.Parse("zap rat", nil))
	if len(harmed) != 0 || hero.Res("calm").Current != 3 {
		t.Fatalf("bad dice still paid or struck: calm %d harmed %v lines %v", hero.Res("calm").Current, harmed, lines)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "cannot be rolled") {
		t.Fatalf("expected a dice failure, got %v", lines)
	}

	c.Pack.RPG.Abilities[0].Effects[0].Damage.Dice = "1-10"
	lines = nil
	commands.Dispatch(c, commands.Parse("zap rat", nil))
	if hero.Res("calm").Current != 2 {
		t.Fatalf("clamped hit did not pay its cost, calm %d", hero.Res("calm").Current)
	}
	if len(harmed) != 1 || harmed[0] != 0 || rat.Res("hp").Current != 10 {
		t.Fatalf("negative roll healed or was not clamped: harmed %v hp %d", harmed, rat.Res("hp").Current)
	}
}
