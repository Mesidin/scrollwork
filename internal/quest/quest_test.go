package quest_test

import (
	"testing"

	"sudengine/internal/quest"
	"sudengine/internal/world"
)

func TestQuestLifecycle(t *testing.T) {
	w := world.New()
	player := &world.Entity{
		ID:        "player",
		Kind:      world.KindPlayer,
		Resources: map[string]world.Resource{"coin": {Current: 0, Max: 100}},
	}
	w.Add(player)

	q1 := quest.Quest{
		ID:    "q1",
		Title: "First Steps",
		Objectives: []quest.Objective{
			{ID: "kill_gob", Kind: quest.KindKill, Target: "goblin", Count: 1},
			{ID: "visit_cave", Kind: quest.KindVisit, Target: "cave"},
		},
		Rewards: quest.Rewards{
			XP:       50,
			Currency: "coin",
			Amount:   20,
			Recipes:  []string{"craft_torch"},
		},
	}

	q2 := quest.Quest{
		ID:            "q2",
		Title:         "Next Level",
		RequiresQuest: []string{"q1"},
	}

	all := []quest.Quest{q1, q2}

	if !quest.IsAvailable(player, q1, all) {
		t.Fatalf("q1 should be available")
	}
	if quest.IsAvailable(player, q2, all) {
		t.Fatalf("q2 should not be available before q1 complete")
	}

	if !quest.Start(player, q1) {
		t.Fatalf("failed to start q1")
	}
	if !quest.IsActive(player, q1) {
		t.Fatalf("q1 should be active")
	}

	// Not ready yet
	if quest.IsReady(player, w, q1) {
		t.Fatalf("q1 should not be ready")
	}

	// Update visit
	visitNotices := quest.UpdateVisit(player, "cave", all, w)
	if len(visitNotices) != 1 || !visitNotices[0].ObjectiveDone {
		t.Fatalf("expected visit notice, got %+v", visitNotices)
	}

	// Still not ready
	if quest.IsReady(player, w, q1) {
		t.Fatalf("q1 still needs kill")
	}

	// Mob kill
	gob := &world.Entity{ID: "goblin-1", Kind: world.KindMobile, Keywords: []string{"goblin"}}
	w.Add(gob)
	killNotices := quest.UpdateKill(player, gob, all, w)
	if len(killNotices) != 1 || !killNotices[0].QuestReady {
		t.Fatalf("expected quest ready notice, got %+v", killNotices)
	}

	if !quest.IsReady(player, w, q1) {
		t.Fatalf("q1 should now be ready")
	}

	// Complete q1
	if err := quest.Complete(player, w, q1, 100); err != nil {
		t.Fatalf("complete error: %v", err)
	}

	if !quest.IsCompleted(player, q1) {
		t.Fatalf("q1 should be completed")
	}
	if player.XP != 0 {
		t.Fatalf("complete should leave XP to the command, got %d", player.XP)
	}
	if player.Res("coin").Current != 20 {
		t.Fatalf("expected 20 coin, got %d", player.Res("coin").Current)
	}
	if !player.KnowsRecipe("craft_torch") {
		t.Fatalf("player should know craft_torch recipe")
	}

	// Now q2 should be available
	if !quest.IsAvailable(player, q2, all) {
		t.Fatalf("q2 should now be available")
	}
}

func TestMatchQuests(t *testing.T) {
	all := []quest.Quest{
		{ID: "clear_wolves", Title: "Clear the Wolves"},
		{ID: "lost_silver", Title: "The Lost Silver"},
	}
	if q := quest.FindQuest(all, "the"); q != nil {
		t.Fatalf("exact lookup matched %s", q.ID)
	}
	if q := quest.FindQuest(all, "clear_wolves"); q == nil || q.ID != "clear_wolves" {
		t.Fatal("exact id should match")
	}
	one := quest.MatchQuests(all, "wolves")
	if len(one) != 1 || one[0].ID != "clear_wolves" {
		t.Fatalf("wolves: %+v", one)
	}
	many := quest.MatchQuests(all, "the")
	if len(many) != 2 {
		t.Fatalf("the should be ambiguous, got %d", len(many))
	}
}

func TestCollectObjectiveConsumes(t *testing.T) {
	w := world.New()
	player := &world.Entity{ID: "player", Kind: world.KindPlayer}
	w.Add(player)

	ring1 := &world.Entity{ID: "ring-1", PrototypeID: "silver.ring", Kind: world.KindItem}
	ring2 := &world.Entity{ID: "ring-2", PrototypeID: "silver.ring", Kind: world.KindItem}
	w.Add(ring1)
	w.Add(ring2)
	_ = w.Move(ring1.ID, player.ID)
	_ = w.Move(ring2.ID, player.ID)

	q := quest.Quest{
		ID:    "ring_quest",
		Title: "The Ring",
		Objectives: []quest.Objective{
			{ID: "obj1", Kind: quest.KindCollect, Target: "silver.ring", Count: 1, Consume: true},
		},
	}

	quest.Start(player, q)
	if !quest.IsReady(player, w, q) {
		t.Fatalf("quest should be ready since player holds 2 rings")
	}

	if err := quest.Complete(player, w, q, 1); err != nil {
		t.Fatalf("failed to complete: %v", err)
	}

	// Exactly 1 ring should have been consumed, leaving 1 ring in player inventory
	kids := w.Children(player.ID)
	if len(kids) != 1 {
		t.Fatalf("expected 1 remaining ring, got %d", len(kids))
	}
}

func TestQuestRelocation(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom}
	player := &world.Entity{ID: "player", Kind: world.KindPlayer}
	guard := &world.Entity{
		ID:       "mob.guard",
		Kind:     world.KindMobile,
		Keywords: []string{"guard"},
		HomeRoom: roomA.ID,
		AI: &world.AI{
			Profile: "wander",
			Rooms:   []world.ID{roomA.ID},
		},
	}
	w.Add(roomA)
	w.Add(roomB)
	w.Add(player)
	w.Add(guard)
	_ = w.Move(player.ID, roomA.ID)
	_ = w.Move(guard.ID, roomA.ID)

	q := quest.Quest{
		ID:    "escort_guard",
		Title: "Escort the Guard",
		Rewards: quest.Rewards{
			Relocate: []quest.Relocation{
				{
					Mob:        "mob.guard",
					To:         "roomB",
					UpdateHome: true,
					Message:    "The guard reports to the new post.",
				},
			},
		},
	}

	quest.Start(player, q)
	if err := quest.Complete(player, w, q, 1); err != nil {
		t.Fatalf("failed to complete quest: %v", err)
	}
	if guard.Parent != roomA.ID {
		t.Fatalf("complete should leave the guard in %s until relocate runs, got %s", roomA.ID, guard.Parent)
	}
	moved := quest.ApplyRelocations(w, roomA.ID, q.Rewards.Relocate)
	if len(moved) != 1 || !moved[0].OK {
		t.Fatalf("relocate: %+v", moved)
	}

	if guard.Parent != roomB.ID {
		t.Fatalf("expected guard parent %s, got %s", roomB.ID, guard.Parent)
	}
	if guard.HomeRoom != roomB.ID {
		t.Fatalf("expected guard home room %s, got %s", roomB.ID, guard.HomeRoom)
	}
	if !guard.AI.Allows(roomB.ID) {
		t.Fatalf("expected guard AI to allow roomB, rooms: %+v", guard.AI.Rooms)
	}
}

func TestRelocatePrefersTheMobileInTheRoom(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom}
	roomC := &world.Entity{ID: "roomC", Kind: world.KindRoom}
	here := &world.Entity{ID: "mob.guard#1", PrototypeID: "mob.guard", Kind: world.KindMobile, Name: "here"}
	away := &world.Entity{ID: "mob.guard#2", PrototypeID: "mob.guard", Kind: world.KindMobile, Name: "away"}
	for _, e := range []*world.Entity{roomA, roomB, roomC, here, away} {
		w.Add(e)
	}
	_ = w.Move(here.ID, roomA.ID)
	_ = w.Move(away.ID, roomC.ID)

	rel := []quest.Relocation{{Mob: "mob.guard", To: "roomB", UpdateHome: true, Message: "The guard reports to the new post."}}
	moved := quest.ApplyRelocations(w, roomA.ID, rel)
	if len(moved) != 1 || !moved[0].OK {
		t.Fatalf("relocate: %+v", moved)
	}
	if here.Parent != roomB.ID {
		t.Fatalf("expected the in-room guard to move, parent %s", here.Parent)
	}
	if away.Parent != roomC.ID {
		t.Fatalf("expected the other guard to stay, parent %s", away.Parent)
	}
}

func TestRelocateAmbiguousOrMissingDoesNotNarrateSuccess(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom}
	roomC := &world.Entity{ID: "roomC", Kind: world.KindRoom}
	a := &world.Entity{ID: "mob.guard#1", PrototypeID: "mob.guard", Kind: world.KindMobile}
	b := &world.Entity{ID: "mob.guard#2", PrototypeID: "mob.guard", Kind: world.KindMobile}
	for _, e := range []*world.Entity{roomA, roomB, roomC, a, b} {
		w.Add(e)
	}
	_ = w.Move(a.ID, roomA.ID)
	_ = w.Move(b.ID, roomB.ID)

	rel := []quest.Relocation{{Mob: "mob.guard", To: "roomC", Message: "The guard reports to the new post."}}
	moved := quest.ApplyRelocations(w, "", rel)
	if len(moved) != 1 || moved[0].OK || moved[0].Reason == "" {
		t.Fatalf("ambiguous relocate should fail visibly: %+v", moved)
	}
	if a.Parent != roomA.ID || b.Parent != roomB.ID {
		t.Fatal("ambiguous relocate moved a guard")
	}

	missing := quest.ApplyRelocations(w, roomA.ID, []quest.Relocation{{Mob: "mob.missing", To: "roomC", Message: "Gone."}})
	if len(missing) != 1 || missing[0].OK || missing[0].Reason == "" {
		t.Fatalf("missing mobile should fail visibly: %+v", missing)
	}
}

func TestRelocateWithoutUpdateHomeLeavesLeash(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom}
	guard := &world.Entity{
		ID: "mob.guard", Kind: world.KindMobile, HomeRoom: roomA.ID,
		AI: &world.AI{Profile: "wander", Rooms: []world.ID{roomA.ID}},
	}
	w.Add(roomA)
	w.Add(roomB)
	w.Add(guard)
	_ = w.Move(guard.ID, roomA.ID)

	moved := quest.ApplyRelocations(w, roomA.ID, []quest.Relocation{{Mob: "mob.guard", To: "roomB"}})
	if len(moved) != 1 || !moved[0].OK {
		t.Fatalf("relocate: %+v", moved)
	}
	if guard.Parent != roomB.ID {
		t.Fatalf("expected parent %s, got %s", roomB.ID, guard.Parent)
	}
	if guard.HomeRoom != roomA.ID {
		t.Fatalf("one-shot move changed home to %s", guard.HomeRoom)
	}
	if guard.AI.Allows(roomB.ID) {
		t.Fatalf("one-shot move widened the leash: %+v", guard.AI.Rooms)
	}
}

func TestRelocatePullsFollowersInTheSameRoom(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom}
	roomC := &world.Entity{ID: "roomC", Kind: world.KindRoom}
	guard := &world.Entity{ID: "mob.guard", Kind: world.KindMobile, Name: "guard"}
	page := &world.Entity{ID: "page", Kind: world.KindMobile, Name: "page"}
	stay := &world.Entity{ID: "stay", Kind: world.KindMobile, Name: "stay", Stance: "stay"}
	far := &world.Entity{ID: "far", Kind: world.KindMobile, Name: "far"}
	for _, e := range []*world.Entity{roomA, roomB, roomC, guard, page, stay, far} {
		w.Add(e)
	}
	_ = w.Move(guard.ID, roomA.ID)
	_ = w.Move(page.ID, roomA.ID)
	_ = w.Move(stay.ID, roomA.ID)
	_ = w.Move(far.ID, roomC.ID)
	_ = w.StartFollowing(page, guard)
	_ = w.StartFollowing(stay, guard)
	_ = w.StartFollowing(far, guard)

	moved := quest.ApplyRelocations(w, roomA.ID, []quest.Relocation{{Mob: "mob.guard", To: "roomB"}})
	if len(moved) != 1 || !moved[0].OK {
		t.Fatalf("relocate: %+v", moved)
	}
	if page.Parent != roomB.ID {
		t.Fatalf("page should follow the guard, parent %s", page.Parent)
	}
	if stay.Parent != roomA.ID || far.Parent != roomC.ID {
		t.Fatalf("stay %s far %s", stay.Parent, far.Parent)
	}
	if len(moved[0].Pulled) != 1 || moved[0].Pulled[0] != "page" {
		t.Fatalf("pulled %+v", moved[0].Pulled)
	}
}
