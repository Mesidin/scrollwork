package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sudengine/internal/combat"
	"sudengine/internal/economy"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/script"
	"sudengine/internal/world"
)

func newTestServerWithEconomy() (*Server, *Session) {
	w := world.New()
	p := &pack.Pack{
		Meta: pack.Meta{
			ID:          "eco-pack",
			Title:       "Economy Test",
			TickMS:      50,
			StartRoom:   "room.smithy",
			CombatEvery: 1,
		},
		Lexicon: pack.Lexicon{Labels: map[string]string{"copper": "copper"}},
		Recipes: []economy.Recipe{
			{
				ID:       "forge_dagger",
				Name:     "iron dagger",
				Station:  "anvil",
				Tools:    []string{"hammer"},
				Requires: []economy.Ingredient{{Item: "scrap", Count: 1}},
				Produces: "item.dagger",
			},
		},
	}

	smithy := &world.Entity{
		ID:        "room.smithy",
		Kind:      world.KindRoom,
		Name:      "Blacksmith Forge",
		Long:      "A busy forge with an anvil in the center.",
		Contents:  []world.ID{"station.anvil", "npc.blacksmith", "item.scrap"},
		HasCoords: true,
	}

	anvil := &world.Entity{
		ID:       "station.anvil",
		Kind:     world.KindScenery,
		Keywords: []string{"anvil"},
		Name:     "a heavy anvil",
		Parent:   smithy.ID,
	}

	blacksmith := &world.Entity{
		ID:       "npc.blacksmith",
		Kind:     world.KindMobile,
		Keywords: []string{"blacksmith", "goran"},
		Name:     "Goran the Smith",
		Parent:   smithy.ID,
		Repairer: true,
		Shop: &world.Shop{
			Currency: "copper",
			Purse:    25,
			Dynamic:  true,
		},
	}

	scrap := &world.Entity{
		ID:       "item.scrap",
		Kind:     world.KindItem,
		Keywords: []string{"scrap", "metal"},
		Name:     "a scrap of iron",
		Takeable: true,
		Parent:   smithy.ID,
	}

	player := &world.Entity{
		ID:     "player",
		Kind:   world.KindPlayer,
		Name:   "Tester",
		Parent: smithy.ID,
		Attrs:  map[string]int{"body": 10},
		Resources: map[string]world.Resource{
			"hp":     {Current: 20, Max: 20},
			"copper": {Current: 100, Max: 1000},
		},
		Equipment: map[string]world.ID{},
	}

	hammer := &world.Entity{
		ID:       "item.hammer",
		Kind:     world.KindItem,
		Keywords: []string{"hammer"},
		Name:     "a smithing hammer",
		Takeable: true,
		Parent:   player.ID,
	}

	sword := &world.Entity{
		ID:            "item.sword",
		Kind:          world.KindItem,
		Keywords:      []string{"sword"},
		Name:          "an iron sword",
		Takeable:      true,
		Wearable:      true,
		Slot:          "wield",
		Weapon:        &world.Weapon{Damage: "1d6", Verb: "slash"},
		Durability:    30,
		MaxDurability: 50,
		Repairable:    true,
		Value:         20,
		Parent:        player.ID,
	}

	daggerProto := &world.Entity{
		ID:            "item.dagger",
		Kind:          world.KindItem,
		Keywords:      []string{"dagger"},
		Name:          "an iron dagger",
		Takeable:      true,
		Wearable:      true,
		Slot:          "wield",
		Weapon:        &world.Weapon{Damage: "1d4", Verb: "stab"},
		Durability:    25,
		MaxDurability: 25,
		Value:         10,
	}

	player.Contents = []world.ID{hammer.ID, sword.ID}
	player.Equipment["wield"] = sword.ID

	w.Entities[smithy.ID] = smithy
	w.Entities[anvil.ID] = anvil
	w.Entities[blacksmith.ID] = blacksmith
	w.Entities[scrap.ID] = scrap
	w.Entities[player.ID] = player
	w.Entities[hammer.ID] = hammer
	w.Entities[sword.ID] = sword
	w.Protos[daggerProto.ID] = daggerProto

	w.PlayerID = player.ID
	w.StartRoom = smithy.ID

	sess := &Session{
		ID:       "eco-session",
		Mode:     protocol.ModePlay,
		PlayerID: player.ID,
		Out:      make(chan protocol.Event, 256),
		In:       make(chan string, 32),
	}

	h := script.New(map[string]string{})
	h.World = w
	srv := &Server{
		Pack:    p,
		World:   w,
		Scripts: h,
		Combat:  &combat.Engine{Primary: "hp", Every: 1},
		Session: sess,
	}

	return srv, sess
}

func TestRepairAndDurability(t *testing.T) {
	srv, sess := newTestServerWithEconomy()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	// Examine sword condition (30/50 durability)
	sess.In <- "examine sword"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "30/50") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 30/50 durability in examine output, got: %v", lines)
	}

	// Check repair list
	sess.In <- "repair"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "Goran the Smith can repair") || !strings.Contains(all, "iron sword") {
		t.Fatalf("expected repair offer from Goran, got: %v", lines)
	}

	// Repair the sword
	sess.In <- "repair sword"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	found = false
	for _, l := range lines {
		if strings.Contains(l, "repairs an iron sword back to pristine condition") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected successful repair output, got: %v", lines)
	}

	// Verify sword is 50/50
	sword := srv.World.Get("item.sword")
	if sword.Durability != 50 {
		t.Fatalf("expected sword durability 50, got %d", sword.Durability)
	}
}

func TestFiniteMerchantPurse(t *testing.T) {
	srv, sess := newTestServerWithEconomy()
	// Set sword to pristine condition so it sells for full base value (20)
	srv.World.Get("item.sword").Durability = 50

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	// Check shop list - should show till
	sess.In <- "list"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "25 copper in the till") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected till amount in shop list, got: %v", lines)
	}

	// Sell sword (value 20) -> vendor purse should drop to 5
	sess.In <- "sell sword"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	found = false
	for _, l := range lines {
		if strings.Contains(l, "You sell an iron sword for 20 copper") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected successful sell output, got: %v", lines)
	}

	smith := srv.World.Get("npc.blacksmith")
	if smith.Shop.Purse != 5 {
		t.Fatalf("expected smith purse 5, got %d", smith.Shop.Purse)
	}

	// Buy it back for 20 -> vendor purse should increase back to 25
	sess.In <- "buy sword"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	found = false
	for _, l := range lines {
		if strings.Contains(l, "You buy an iron sword") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected successful buy output, got: %v", lines)
	}
	if smith.Shop.Purse != 25 {
		t.Fatalf("expected smith purse 25, got %d", smith.Shop.Purse)
	}
}

func TestCraftingCommand(t *testing.T) {
	srv, sess := newTestServerWithEconomy()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	// Pick up scrap from room
	sess.In <- "get scrap"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	// List recipes
	sess.In <- "recipes"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "iron dagger") && strings.Contains(l, "anvil") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected iron dagger in recipes, got: %v", lines)
	}

	// Craft the dagger
	sess.In <- "craft dagger"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	found = false
	for _, l := range lines {
		if strings.Contains(l, "You craft an iron dagger") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected successful craft output, got: %v", lines)
	}

	// Scrap should be gone from player
	player := srv.World.Get("player")
	for _, id := range player.Contents {
		it := srv.World.Get(id)
		if it != nil && it.HasKeyword("scrap") {
			t.Fatalf("scrap should have been consumed")
		}
	}
}

func TestGreenHollowEconomy(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, sess, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada", RoleID: "warden"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	// 1. Talk to Bram in square, check topics
	sess.In <- "talk bram"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "repair") {
		t.Fatalf("expected repair topic on Bram, got:\n%s", all)
	}

	// 2. Move south into cottage where workbench, strongbox, and manual are
	sess.In <- "s"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "workbench") || !strings.Contains(all, "strongbox") {
		t.Fatalf("expected workbench and strongbox in cottage, got:\n%s", all)
	}

	// 3. Test Banking at strongbox
	sess.In <- "deposit 5"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "You deposit 5 Coin into your vault") {
		t.Fatalf("expected successful deposit, got:\n%s", all)
	}

	sess.In <- "balance"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "5 Coin") {
		t.Fatalf("expected 5 Coin in balance, got:\n%s", all)
	}

	sess.In <- "withdraw 2"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "You withdraw 2 Coin from your vault") {
		t.Fatalf("expected successful withdraw, got:\n%s", all)
	}

	// 4. Check recipes at workbench - cudgel should be locked/undiscovered initially
	sess.In <- "recipes"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "pitch torch") {
		t.Fatalf("expected pitch torch in recipes, got:\n%s", all)
	}
	if strings.Contains(all, "carved cudgel") {
		t.Fatalf("carved cudgel should be undiscovered initially, got:\n%s", all)
	}

	// 5. Try to craft cudgel before learning it
	sess.In <- "craft cudgel"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "You don't know how to craft") {
		t.Fatalf("expected craft rejection for unknown recipe, got:\n%s", all)
	}

	// 6. Learn cudgel recipe from carpenter's notes
	sess.In <- "take manual"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)
	sess.In <- "use manual"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "master the craft of whittling a cudgel") && !strings.Contains(all, "learn how to craft") {
		t.Fatalf("expected recipe learned output, got:\n%s", all)
	}

	// 7. Verify cudgel is now in recipes
	sess.In <- "recipes"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "carved cudgel") {
		t.Fatalf("expected carved cudgel to be known now, got:\n%s", all)
	}

	// 8. Take linen strips from cottage
	sess.In <- "take strips"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	// 9. Go north, east to lane, take sword
	sess.In <- "n"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)
	sess.In <- "e"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)
	sess.In <- "take sword"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	// 10. Gather branches from brambles in lane using perception
	sess.In <- "gather brambles"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "You gather") && !strings.Contains(all, "find nothing usable") {
		t.Fatalf("expected gather attempt output, got:\n%s", all)
	}

	// Ensure player has a branch (take branch if needed or already gathered)
	player := srv.World.Get(srv.World.PlayerID)
	hasBranch := false
	for _, id := range player.Contents {
		it := srv.World.Get(id)
		if it != nil && it.HasKeyword("branch") {
			hasBranch = true
			break
		}
	}
	if !hasBranch {
		_, _ = srv.World.Spawn("hollow.branch", player.ID)
	}

	// 11. Go west back to square and train crafting with Bram
	sess.In <- "w"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	// Withdraw enough to pay trainer if needed
	sess.In <- "s"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)
	sess.In <- "withdraw all"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)
	sess.In <- "n"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	sess.In <- "train crafting"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "drills you in Crafting") {
		t.Fatalf("expected training confirmation from Bram, got:\n%s", all)
	}

	// 12. Go south to cottage and craft cudgel
	sess.In <- "s"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	sess.In <- "craft cudgel"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "craft") && !strings.Contains(all, "craftsmanship") {
		t.Fatalf("expected successful cudgel craft, got:\n%s", all)
	}

	// 13. Go north to square, damage sword slightly to test repair with Bram
	sess.In <- "n"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	// Find sword in player's inventory
	var sword *world.Entity
	for _, id := range player.Contents {
		it := srv.World.Get(id)
		if it != nil && it.HasKeyword("sword") {
			sword = it
			break
		}
	}
	if sword == nil {
		t.Fatalf("player does not have sword")
	}
	sword.Durability = 30 // Damage sword

	sess.In <- "repair sword"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "repairs a short sword back to pristine condition") {
		t.Fatalf("expected repair output from Bram, got:\n%s", all)
	}
	if sword.Durability != sword.MaxDurability {
		t.Fatalf("expected sword to be repaired to %d, got %d", sword.MaxDurability, sword.Durability)
	}
}

func TestPriceCurveServerIntegration(t *testing.T) {
	srv, sess := newTestServerWithEconomy()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	// Configure Goran to use PriceCurve and have base stock of 2 daggers
	goran := srv.World.Get("npc.blacksmith")
	goran.Shop.PriceCurve = true
	goran.Shop.Stock = []string{"item.dagger", "item.dagger"}
	goran.Shop.Purse = 50

	d1 := &world.Entity{
		ID:            "dag1",
		PrototypeID:   "item.dagger",
		Kind:          world.KindItem,
		Keywords:      []string{"dagger"},
		Name:          "an iron dagger",
		Value:         10,
		Takeable:      true,
	}
	d2 := &world.Entity{
		ID:            "dag2",
		PrototypeID:   "item.dagger",
		Kind:          world.KindItem,
		Keywords:      []string{"dagger"},
		Name:          "an iron dagger",
		Value:         10,
		Takeable:      true,
	}
	srv.World.Add(d1)
	srv.World.Add(d2)
	_ = srv.World.Move(d1.ID, goran.ID)
	_ = srv.World.Move(d2.ID, goran.ID)

	// 1. Initial list: 2 daggers in stock with base stock 2 -> price should be 10 copper
	sess.In <- "list"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "iron dagger  (10 copper)") {
		t.Fatalf("expected dagger at 10 copper, got:\n%s", all)
	}

	// 2. Buy one dagger -> inventory drops to 1, price should rise to 11 copper
	sess.In <- "buy dagger"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "You buy an iron dagger for 10 copper") {
		t.Fatalf("expected successful buy for 10 copper, got:\n%s", all)
	}

	// List again -> price scaled up to 11 copper due to scarcity
	sess.In <- "list"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "iron dagger  (11 copper)") {
		t.Fatalf("expected dagger price increased to 11 copper, got:\n%s", all)
	}

	// 3. Flooding vendor with items drops sell price down to nominal scrap value (1 copper)
	for i := 0; i < 15; i++ {
		junk := &world.Entity{
			ID:          world.ID(fmt.Sprintf("junk_%d", i)),
			PrototypeID: "item.scrap_blade",
			Kind:        world.KindItem,
			Keywords:    []string{"junkblade"},
			Name:        "a junk blade",
			Value:       10,
			Takeable:    true,
		}
		srv.World.Add(junk)
		_ = srv.World.Move(junk.ID, goran.ID)
	}
	playerJunk := &world.Entity{
		ID:          "player_junk",
		PrototypeID: "item.scrap_blade",
		Kind:        world.KindItem,
		Keywords:    []string{"junkblade"},
		Name:        "a junk blade",
		Value:       10,
		Takeable:    true,
	}
	srv.World.Add(playerJunk)
	_ = srv.World.Move(playerJunk.ID, srv.World.PlayerID)

	sess.In <- "sell junkblade"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "You sell a junk blade for 1 copper") {
		t.Fatalf("expected flooded sell price 1 copper (scrap value), got:\n%s", all)
	}
}
