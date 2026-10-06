package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/quest"
	"sudengine/internal/world"
)

func TestGreenHollowQuests(t *testing.T) {
	hollowDir := filepath.Join("..", "..", "games", "green-hollow")

	p, err := pack.Load(hollowDir)
	if err != nil {
		t.Fatalf("load green-hollow: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate green-hollow: %v", err)
	}
	if len(p.Quests) != 3 {
		t.Fatalf("expected 3 quests in green-hollow, got %d", len(p.Quests))
	}

	srv, sess, err := Launch(Options{
		PackDir: hollowDir,
		Mode:    protocol.ModePlay,
	})
	if err != nil {
		t.Fatalf("launch server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)

	drainLines(sess)

	// 1. Talk to Bram - should see available quests
	sess.In <- "talk bram"
	time.Sleep(40 * time.Millisecond)
	lines := drainLines(sess)
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "Quest Available: Clear the Wolves") || !strings.Contains(all, "Quest Available: Lost Silver") {
		t.Fatalf("expected Bram to offer quests, got:\n%s", all)
	}

	// 2. Check quests journal initially empty
	sess.In <- "quests"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "no active or completed quests") {
		t.Fatalf("expected empty quests, got:\n%s", all)
	}

	// 3. Accept Clear the Wolves
	sess.In <- "quest accept clear_wolves"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "accepted quest: Clear the Wolves") {
		t.Fatalf("expected accept confirmation, got:\n%s", all)
	}

	// 4. View active quests
	sess.In <- "quests"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "Clear the Wolves") || !strings.Contains(all, "(0/2)") {
		t.Fatalf("expected active quest with 0/2 wolves, got:\n%s", all)
	}

	// 5. Defeat 2 wolves
	player := srv.World.Get(srv.World.PlayerID)
	startCoin := player.Res("coin").Current
	startXP := player.XP

	// The player landed both blows. A death with no attacker does not count.
	for _, ent := range srv.World.Entities {
		if ent.Kind == world.KindMobile && (ent.PrototypeID == "hollow.wolf" || ent.PrototypeID == "hollow.wolf-2" || ent.HasKeyword("wolf")) {
			ent.SlainBy = player.ID
			ent.SetRes("vigor", world.Resource{Current: 0, Max: 10})
		}
	}
	// Tick loop will reap dead
	time.Sleep(350 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "Quest Ready for Turn-in: Clear the Wolves") {
		t.Fatalf("expected quest ready notice, got:\n%s", all)
	}

	// 6. Bram should now offer turn-in
	sess.In <- "talk bram"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "Quest Ready for Turn-in: Clear the Wolves") {
		t.Fatalf("expected Bram to show quest ready for turn in, got:\n%s", all)
	}

	// 7. Complete quest
	sess.In <- "quest complete clear_wolves"
	time.Sleep(40 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "Quest Completed: Clear the Wolves!") || !strings.Contains(strings.ToLower(all), "15 coin") {
		t.Fatalf("expected quest completion rewards, got:\n%s", all)
	}

	// Verify rewards granted
	if player.Res("coin").Current != startCoin+15 {
		t.Fatalf("coin %d, started %d", player.Res("coin").Current, startCoin)
	}
	// Two wolves grant 8 XP each (16). Leaving level 1 costs 15, so 1 remains.
	// The 20 XP quest reward is granted once, leaving level 2 with 21 XP.
	if player.Level != 2 || player.XP != 21 {
		t.Fatalf("level %d xp %d (start xp %d)", player.Level, player.XP, startXP)
	}
	if !player.KnowsRecipe("craft_cudgel") {
		t.Fatalf("expected player to learn craft_cudgel recipe from quest reward")
	}

	// 8. Scout the Brigand Camp should now be unlocked and available!
	sess.In <- "talk bram"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "Quest Available: Scout the Brigand Camp") {
		t.Fatalf("expected Scout the Brigand Camp to be available after prerequisite complete, got:\n%s", all)
	}

	// 9. Accept Lost Silver and complete it
	sess.In <- "quest accept lost_silver"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	// Spawn the silver ring into player inventory
	_, _ = srv.World.Spawn("hollow.ring", player.ID)

	sess.In <- "quest complete lost_silver"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "Quest Completed: Lost Silver!") {
		t.Fatalf("expected Lost Silver completion, got:\n%s", all)
	}

	// Verify ring was consumed
	for _, cid := range player.Contents {
		it := srv.World.Get(cid)
		if it != nil && strings.EqualFold(string(it.PrototypeID), "hollow.ring") {
			t.Fatalf("ring should have been consumed on completion")
		}
	}
}

func TestQuestRelocationIntegration(t *testing.T) {
	packDir := filepath.Join("..", "..", "games", "green-hollow")
	srv, sess, err := Launch(Options{
		PackDir:    packDir,
		Mode:       protocol.ModePlay,
		PlayerName: "Hero",
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	defer func() { sess.In <- "quit" }()

	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	var bram *world.Entity
	for _, e := range srv.World.Entities {
		if e.Kind == world.KindMobile && e.PrototypeID == "hollow.bram" {
			bram = e
			break
		}
	}
	if bram == nil {
		t.Fatalf("bram not found")
	}

	srv.Pack.Scripts["bram_reloc.lua"] = `
function on_quest_complete(actor, self, quest_id)
    echo_room("Bram raises his hand in a sharp salute.")
    if is_quest_completed(quest_id) then
        echo("The quest is already written down.")
    end
    return true
end
`
	srv.Scripts.Reload(srv.Pack.Scripts)
	bram.Scripts = "bram_reloc.lua"

	relocQuest := quest.Quest{
		ID:     "reloc_test",
		Title:  "Move Bram",
		TurnIn: "hollow.bram",
		Rewards: quest.Rewards{
			Relocate: []quest.Relocation{
				{
					Mob:        "hollow.bram",
					To:         "hollow.cottage",
					UpdateHome: true,
					Message:    "Bram marches south into the cottage.",
				},
			},
		},
	}
	srv.Pack.Quests = append(srv.Pack.Quests, relocQuest)

	sess.In <- "quest accept reloc_test"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	sess.In <- "quest complete reloc_test"
	time.Sleep(40 * time.Millisecond)
	lines := drainLines(sess)
	all := strings.Join(lines, "\n")

	if !strings.Contains(all, "Quest Completed: Move Bram!") {
		t.Fatalf("expected quest completion, got:\n%s", all)
	}
	if !strings.Contains(all, "Bram marches south into the cottage.") {
		t.Fatalf("expected relocation message, got:\n%s", all)
	}
	salute := strings.Index(all, "Bram raises his hand in a sharp salute.")
	march := strings.Index(all, "Bram marches south into the cottage.")
	if salute < 0 || march < 0 || salute > march {
		t.Fatalf("expected the room echo before Bram moved, got:\n%s", all)
	}
	if !strings.Contains(all, "The quest is already written down.") {
		t.Fatalf("expected the hook to see a completed quest, got:\n%s", all)
	}

	if bram.Parent != "hollow.cottage" {
		t.Fatalf("expected Bram parent hollow.cottage, got %s", bram.Parent)
	}
	if bram.HomeRoom != "hollow.cottage" {
		t.Fatalf("expected Bram home room hollow.cottage, got %s", bram.HomeRoom)
	}
}
