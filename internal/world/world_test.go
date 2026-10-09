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

func TestFollowingAndParty(t *testing.T) {
	w := New()
	alice := &Entity{ID: "alice", Kind: KindPlayer, Name: "Alice"}
	bob := &Entity{ID: "bob", Kind: KindPlayer, Name: "Bob"}
	charlie := &Entity{ID: "charlie", Kind: KindMobile, Name: "Charlie"}
	w.Add(alice)
	w.Add(bob)
	w.Add(charlie)

	// Alice follows Bob
	if err := w.StartFollowing(alice, bob); err != nil {
		t.Fatalf("StartFollowing: %v", err)
	}
	if alice.Leader != "bob" {
		t.Fatalf("expected alice leader bob, got %s", alice.Leader)
	}
	if len(bob.Followers) != 1 || bob.Followers[0] != "alice" {
		t.Fatalf("expected bob followers [alice], got %+v", bob.Followers)
	}

	// Loop detection: Bob cannot follow Alice
	if err := w.StartFollowing(bob, alice); err == nil {
		t.Fatalf("expected loop error when Bob follows Alice, got nil")
	}

	// Charlie follows Alice (Charlie -> Alice -> Bob)
	if err := w.StartFollowing(charlie, alice); err != nil {
		t.Fatalf("charlie follows alice: %v", err)
	}
	// Bob cannot follow Charlie either
	if err := w.StartFollowing(bob, charlie); err == nil {
		t.Fatalf("expected indirect loop error when Bob follows Charlie")
	}

	// Stop following
	w.StopFollowing(alice)
	if alice.Leader != "" {
		t.Fatalf("expected alice leader cleared, got %s", alice.Leader)
	}
	if len(bob.Followers) != 0 {
		t.Fatalf("expected bob followers empty, got %+v", bob.Followers)
	}

	// Party grouping
	bob.PartyLeader = "bob"
	alice.PartyLeader = "bob"
	members := w.PartyMembers("bob")
	if len(members) != 2 {
		t.Fatalf("expected 2 party members, got %d", len(members))
	}
	if !bob.IsPartyLeader() || alice.IsPartyLeader() {
		t.Fatalf("party leader checks failed")
	}

	bob.PartyLeader = "carol"
	members = w.PartyMembers("bob")
	if len(members) != 1 || members[0].ID != "alice" {
		t.Fatalf("a leader who left should not stay on the roster: %+v", members)
	}
}

func TestPullFollowersLeavesStayAndDistantPets(t *testing.T) {
	w := New()
	a := &Entity{ID: "a", Kind: KindRoom}
	b := &Entity{ID: "b", Kind: KindRoom}
	leader := &Entity{ID: "leader", Kind: KindMobile, Name: "leader"}
	near := &Entity{ID: "near", Kind: KindMobile, Name: "near"}
	stay := &Entity{ID: "stay", Kind: KindMobile, Name: "stay", Stance: "stay"}
	far := &Entity{ID: "far", Kind: KindMobile, Name: "far"}
	for _, e := range []*Entity{a, b, leader, near, stay, far} {
		w.Add(e)
	}
	_ = w.Move(leader.ID, a.ID)
	_ = w.Move(near.ID, a.ID)
	_ = w.Move(stay.ID, a.ID)
	_ = w.Move(far.ID, b.ID)
	_ = w.StartFollowing(near, leader)
	_ = w.StartFollowing(stay, leader)
	_ = w.StartFollowing(far, leader)
	moved := w.PullFollowers(leader, a, b)
	if len(moved) != 1 || moved[0].Follower.ID != "near" {
		t.Fatalf("expected only the nearby free follower, got %+v", moved)
	}
	if near.Parent != b.ID || stay.Parent != a.ID || far.Parent != b.ID {
		t.Fatalf("parents near %s stay %s far %s", near.Parent, stay.Parent, far.Parent)
	}
}

func TestResolveNonMobileIsUnique(t *testing.T) {
	w := New()
	room := &Entity{ID: "room", Kind: KindRoom, Name: "Ada's cottage"}
	gem := &Entity{ID: "gem", Kind: KindItem, Name: "adamant shard"}
	lamp := &Entity{ID: "lamp", Kind: KindItem, Name: "brass lantern", Keywords: []string{"lantern"}}
	for _, e := range []*Entity{room, gem, lamp} {
		w.Add(e)
	}
	_ = w.Move(gem.ID, room.ID)
	_ = w.Move(lamp.ID, room.ID)

	if _, err := w.ResolveNonMobile("ada", room.ID); err != ErrAmbiguous {
		t.Fatalf("ada: %v", err)
	}
	got, err := w.ResolveNonMobile("lantern", room.ID)
	if err != nil || got == nil || got.ID != "lamp" {
		t.Fatalf("lantern: %v %+v", err, got)
	}
}
