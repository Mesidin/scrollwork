package world

import "testing"

func TestMoveAndMatch(t *testing.T) {
	w := New()
	r := &Entity{ID: "r", Kind: KindRoom, Name: "Room", Exits: map[string]Exit{}}
	lamp := &Entity{ID: "lamp", Kind: KindItem, Name: "lamp", Short: "a lamp", Keywords: []string{"lamp"}, Takeable: true}
	player := &Entity{ID: "p", Kind: KindPlayer, Name: "you", Keywords: []string{"me", "self"}}
	w.Add(r)
	w.Add(lamp)
	w.Add(player)
	if err := w.Move(player.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.Move(lamp.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	got, many := w.Match(player, "lamp", ScopeRoom)
	if got == nil || got.ID != "lamp" || len(many) != 1 {
		t.Fatalf("match failed: %#v %#v", got, many)
	}
	if err := w.Move(lamp.ID, player.ID); err != nil {
		t.Fatal(err)
	}
	if w.RoomOf(lamp) != r {
		t.Fatal("lamp should still be in room via player")
	}
}

func TestExtinguishToggleLights(t *testing.T) {
	w := New()
	room := &Entity{ID: "r", Kind: KindRoom, Name: "Yard", Flags: map[string]bool{"outdoor": true}}
	player := &Entity{ID: "p", Kind: KindPlayer, Name: "you"}
	lamp := &Entity{
		ID: "lamp", Kind: KindItem, Name: "lamp",
		Use:   &UseEffect{ToggleFlag: "light"},
		Flags: map[string]bool{"light": true},
	}
	gem := &Entity{ID: "gem", Kind: KindItem, Name: "gem", Flags: map[string]bool{"light": true}}
	w.Add(room)
	w.Add(player)
	w.Add(lamp)
	w.Add(gem)
	if err := w.Move(player.ID, room.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.Move(lamp.ID, player.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.Move(gem.ID, room.ID); err != nil {
		t.Fatal(err)
	}
	gone := w.ExtinguishToggleLights(room)
	if len(gone) != 1 || gone[0].ID != "lamp" {
		t.Fatalf("expected the lamp only, got %+v", gone)
	}
	if lamp.HasFlag("light") || !gem.HasFlag("light") {
		t.Fatal("toggle lights go out; a permanent light flag stays")
	}
}

func TestSpawn(t *testing.T) {
	w := New()
	room := &Entity{ID: "r", Kind: KindRoom, Name: "R", Exits: map[string]Exit{}}
	w.Add(room)
	w.Protos["rat"] = &Entity{ID: "rat", Kind: KindMobile, Name: "rat", Keywords: []string{"rat"}}
	a, err := w.Spawn("rat", "r")
	if err != nil {
		t.Fatal(err)
	}
	b, err := w.Spawn("rat", "r")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatal("ids should differ")
	}
	if a.PrototypeID != "rat" {
		t.Fatalf("proto %s", a.PrototypeID)
	}
}

func TestSlugAndDirs(t *testing.T) {
	if Slug("The Kitchen!") != "the-kitchen" {
		t.Fatalf("%q", Slug("The Kitchen!"))
	}
	if Opposite("n") != "south" {
		t.Fatal(Opposite("n"))
	}
	dx, dy, dz, ok := DirDelta("north")
	if !ok || dx != 0 || dy != 1 || dz != 0 {
		t.Fatal(dx, dy, dz, ok)
	}
}
