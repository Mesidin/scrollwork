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
