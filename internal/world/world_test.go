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
