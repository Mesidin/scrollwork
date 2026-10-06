package server

import (
	"context"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sudengine/internal/combat"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/rpg"
	"sudengine/internal/script"
	"sudengine/internal/world"
)

func newTestServerWithHunting() (*Server, *Session) {
	w := world.New()
	p := &pack.Pack{
		Meta: pack.Meta{
			ID:          "hunt-pack",
			Title:       "Hunting Test",
			TickMS:      50,
			StartRoom:   "room.clearing",
			CombatEvery: 1,
		},
		Lexicon: pack.Lexicon{Labels: map[string]string{"copper": "copper"}},
		RPG: rpg.Schema{
			PrimaryResource: "hp",
			Resources: []rpg.ResDef{
				{Key: "hp", Label: "Health", Max: 100},
			},
			Attributes: []rpg.AttrDef{
				{Key: "crafting", Label: "Crafting"},
			},
			Checks: map[string]rpg.Check{
				"crafting": {Roll: "10", Difficulty: 5},
			},
		},
	}

	clearing := &world.Entity{
		ID:        "room.clearing",
		Kind:      world.KindRoom,
		Name:      "Forest Clearing",
		Long:      "A quiet sunlit clearing.",
		Contents:  []world.ID{"mob.wolf"},
		HasCoords: true,
	}

	player := &world.Entity{
		ID:     "player",
		Kind:   world.KindPlayer,
		Name:   "Hunter",
		Parent: clearing.ID,
		Attrs:  map[string]int{"crafting": 10},
		Resources: map[string]world.Resource{
			"hp": {Current: 30, Max: 30},
		},
		Equipment: map[string]world.ID{},
	}

	wolfProto := &world.Entity{
		ID:           "mob.wolf",
		Kind:         world.KindMobile,
		Keywords:     []string{"wolf"},
		Name:         "a grey wolf",
		Short:        "a menacing grey wolf",
		HomeRoom:     clearing.ID,
		RespawnEvery: 3,
		Resources: map[string]world.Resource{
			"hp": {Current: 10, Max: 10},
		},
		Loot: []world.LootDrop{
			{Item: "item.meat", Chance: 100, Count: 1, SkinOnly: false},
			{Item: "item.pelt", Chance: 100, Count: 1, SkinOnly: true},
		},
	}

	meatProto := &world.Entity{
		ID:       "item.meat",
		Kind:     world.KindItem,
		Keywords: []string{"meat", "chunk"},
		Name:     "a chunk of raw meat",
		Takeable: true,
	}

	peltProto := &world.Entity{
		ID:       "item.pelt",
		Kind:     world.KindItem,
		Keywords: []string{"pelt", "skin"},
		Name:     "a wolf pelt",
		Takeable: true,
		Salvage: []world.SalvageYield{
			{Item: "item.strips", Count: 2},
		},
	}

	stripsProto := &world.Entity{
		ID:       "item.strips",
		Kind:     world.KindItem,
		Keywords: []string{"strips", "leather"},
		Name:     "leather strips",
		Takeable: true,
	}

	whetstoneProto := &world.Entity{
		ID:       "item.whetstone",
		Kind:     world.KindItem,
		Keywords: []string{"whetstone", "stone"},
		Name:     "a whetstone",
		Takeable: true,
		RepairKit: &world.RepairKit{
			Uses:    2,
			MaxUses: 2,
			Amount:  15,
			Skill:   "crafting",
			DC:      5,
		},
	}

	sword := &world.Entity{
		ID:            "item.sword",
		Kind:          world.KindItem,
		Keywords:      []string{"sword"},
		Name:          "a hunting sword",
		Takeable:      true,
		Wearable:      true,
		Slot:          "wield",
		Durability:    10,
		MaxDurability: 30,
		Repairable:    true,
		Parent:        player.ID,
	}

	whetstone := whetstoneProto.Clone()
	whetstone.ID = "item.whetstone#1"
	whetstone.Parent = player.ID

	wolf := wolfProto.Clone()
	wolf.ID = "mob.wolf#1"
	wolf.PrototypeID = "mob.wolf"
	wolf.Parent = clearing.ID

	clearing.Contents = []world.ID{player.ID, wolf.ID}
	player.Contents = []world.ID{sword.ID, whetstone.ID}
	player.Equipment["wield"] = sword.ID

	w.Entities[clearing.ID] = clearing
	w.Entities[player.ID] = player
	w.Entities[wolf.ID] = wolf
	w.Entities[sword.ID] = sword
	w.Entities[whetstone.ID] = whetstone

	w.Protos[wolfProto.ID] = wolfProto
	w.Protos[meatProto.ID] = meatProto
	w.Protos[peltProto.ID] = peltProto
	w.Protos[stripsProto.ID] = stripsProto
	w.Protos[whetstoneProto.ID] = whetstoneProto

	w.PlayerID = player.ID
	w.StartRoom = clearing.ID

	sess := &Session{
		ID:       "hunt-session",
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
		rng:     rand.New(rand.NewSource(1)),
	}

	return srv, sess
}

func TestHuntingCorpseLootAndSkinning(t *testing.T) {
	srv, sess := newTestServerWithHunting()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	// Damage wolf to 0
	wolf := srv.World.Get("mob.wolf#1")
	if wolf == nil {
		t.Fatalf("wolf not found")
	}
	hp := wolf.Resources["hp"]
	hp.Current = 0
	wolf.Resources["hp"] = hp

	// Allow tick to reap dead wolf
	time.Sleep(70 * time.Millisecond)
	drainLines(sess)

	// Check that meat dropped into the room
	room := srv.World.Get("room.clearing")
	var hasMeat, hasCorpse bool
	var corpseID world.ID
	for _, cid := range room.Contents {
		ent := srv.World.Get(cid)
		if ent != nil && ent.HasKeyword("meat") {
			hasMeat = true
		}
		if ent != nil && ent.HasKeyword("corpse") {
			hasCorpse = true
			corpseID = ent.ID
			if ent.HasKeyword("wolf") {
				t.Fatalf("corpse matched the living wolf keyword: %s %v", ent.ID, ent.Keywords)
			}
		}
	}
	if !hasMeat {
		t.Fatalf("expected meat to drop in room on wolf death")
	}
	if !hasCorpse {
		t.Fatalf("expected corpse to appear in room on wolf death")
	}

	// Skin the corpse
	sess.In <- "skin corpse"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "butcher") || !strings.Contains(all, "wolf pelt") {
		t.Fatalf("expected skinning output with wolf pelt, got: %s", all)
	}

	// Verify player now has pelt
	player := srv.World.Player()
	var hasPelt bool
	for _, cid := range player.Contents {
		ent := srv.World.Get(cid)
		if ent != nil && ent.HasKeyword("pelt") {
			hasPelt = true
			break
		}
	}
	if !hasPelt {
		t.Fatalf("expected wolf pelt in player inventory after skinning")
	}
	for _, cid := range player.Contents {
		ent := srv.World.Get(cid)
		if ent != nil && ent.HasKeyword("meat") {
			t.Fatal("skin harvested meat that already dropped on death")
		}
	}

	// Corpse should now be marked butchered
	corpse := srv.World.Get(corpseID)
	if corpse == nil || !corpse.HasFlag("skinned") {
		t.Fatalf("expected corpse to have skinned flag")
	}

	// Skinning again should report already butchered
	sess.In <- "skin corpse"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "already been butchered") {
		t.Fatalf("expected 'already been butchered' warning, got: %s", all)
	}
}

func TestSalvageCommand(t *testing.T) {
	srv, sess := newTestServerWithHunting()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	// Spawn pelt into player inventory
	pelt, err := srv.World.Spawn("item.pelt", srv.World.PlayerID)
	if err != nil || pelt == nil {
		t.Fatalf("failed to spawn pelt: %v", err)
	}

	sess.In <- "salvage pelt"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "break down") || !strings.Contains(all, "leather strips") {
		t.Fatalf("expected salvage output for pelt, got: %s", all)
	}

	// Check that pelt is gone and 2 strips exist
	player := srv.World.Player()
	stripsCount := 0
	for _, cid := range player.Contents {
		ent := srv.World.Get(cid)
		if ent != nil {
			if ent.HasKeyword("pelt") {
				t.Fatalf("pelt should be destroyed after salvage")
			}
			if ent.HasKeyword("strips") {
				stripsCount++
			}
		}
	}
	if stripsCount != 2 {
		t.Fatalf("expected 2 leather strips, got %d", stripsCount)
	}
}

func TestFieldRepairKit(t *testing.T) {
	srv, sess := newTestServerWithHunting()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	sword := srv.World.Get("item.sword")
	if sword == nil || sword.Durability != 10 {
		t.Fatalf("expected sword durability 10")
	}

	// Test repair sword with whetstone
	sess.In <- "repair sword with whetstone"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "repair") || !strings.Contains(all, "25/30") {
		t.Fatalf("expected whetstone repair output to 25/30, got: %s", all)
	}
	if sword.Durability != 25 {
		t.Fatalf("expected sword durability 25, got %d", sword.Durability)
	}

	// Kit should now have 1 use remaining
	whetstone := srv.World.Get("item.whetstone#1")
	if whetstone == nil || whetstone.RepairKit.Uses != 1 {
		t.Fatalf("expected whetstone uses 1")
	}

	// Test use whetstone on sword
	sess.In <- "use whetstone on sword"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "30/30") || !strings.Contains(all, "used up") {
		t.Fatalf("expected whetstone used up output, got: %s", all)
	}
	if sword.Durability != 30 {
		t.Fatalf("expected sword fully repaired to 30, got %d", sword.Durability)
	}

	// Whetstone should be destroyed after exhausting uses
	if srv.World.Get("item.whetstone#1") != nil {
		t.Fatalf("expected whetstone to be destroyed after exhaustion")
	}
}

func TestMobRespawnHeartbeat(t *testing.T) {
	srv, sess := newTestServerWithHunting()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	// Kill wolf
	wolf := srv.World.Get("mob.wolf#1")
	hp := wolf.Resources["hp"]
	hp.Current = 0
	wolf.Resources["hp"] = hp

	// Wait for reapDead
	time.Sleep(70 * time.Millisecond)
	drainLines(sess)

	if len(srv.World.Respawns) != 1 {
		t.Fatalf("expected 1 respawn task, got %d", len(srv.World.Respawns))
	}

	// RespawnEvery is 3 ticks. Wait 4 ticks (4 * 50ms = 200ms)
	time.Sleep(250 * time.Millisecond)
	lines := drainLines(sess)
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "appears") {
		t.Fatalf("expected respawn announcement 'appears', got: %s", all)
	}

	// Clearing should have a new wolf
	room := srv.World.Get("room.clearing")
	var wolfFound bool
	for _, cid := range room.Contents {
		ent := srv.World.Get(cid)
		if ent != nil && ent.Kind == world.KindMobile && ent.HasKeyword("wolf") {
			wolfFound = true
			break
		}
	}
	if !wolfFound {
		t.Fatalf("expected respawned wolf in clearing")
	}
}

func TestGreenHollowHuntingIntegration(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, sess, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada", RoleID: "warden"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(50 * time.Millisecond)
	drainLines(sess)

	// 1. Move south to cottage and pick up whetstone
	sess.In <- "s"
	time.Sleep(30 * time.Millisecond)
	sess.In <- "get whetstone"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "whetstone") {
		t.Fatalf("expected to take whetstone in cottage, got: %s", all)
	}

	// 2. Return to square (north), go east to lane and pick up cloak to test salvage
	sess.In <- "n"
	time.Sleep(30 * time.Millisecond)
	sess.In <- "e"
	time.Sleep(30 * time.Millisecond)
	sess.In <- "get cloak"
	time.Sleep(30 * time.Millisecond)
	sess.In <- "get sword"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	// Salvage cloak into strips
	sess.In <- "salvage cloak"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "break down") || !strings.Contains(all, "linen strips") {
		t.Fatalf("expected salvage output for cloak, got: %s", all)
	}

	// 3. Move north to thicket where wolves are
	sess.In <- "n"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	// Find wolf in thicket
	thicket := srv.World.Get("hollow.thicket")
	var wolf *world.Entity
	for _, cid := range thicket.Contents {
		ent := srv.World.Get(cid)
		if ent != nil && ent.HasKeyword("wolf") {
			wolf = ent
			break
		}
	}
	if wolf == nil {
		t.Fatalf("wolf not found in hollow.thicket")
	}

	// Kill wolf
	hp := wolf.Resources["vigor"]
	hp.Current = 0
	wolf.Resources["vigor"] = hp

	// Wait for reapDead (tick_ms is 250)
	time.Sleep(300 * time.Millisecond)
	drainLines(sess)

	// Skin wolf corpse
	sess.In <- "skin corpse"
	time.Sleep(40 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "butcher") || !strings.Contains(all, "wolf pelt") {
		t.Fatalf("expected butchering wolf for wolf pelt, got: %s", all)
	}

	// Now head south to lane, west to square, south to cottage (where workbench is)
	sess.In <- "s"
	time.Sleep(30 * time.Millisecond)
	sess.In <- "w"
	time.Sleep(30 * time.Millisecond)
	sess.In <- "s"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	// Craft cloak using the pelt + strips!
	sess.In <- "craft cloak"
	time.Sleep(40 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "craft") && !strings.Contains(all, "wool cloak") {
		t.Fatalf("expected crafting cloak to succeed, got: %s", all)
	}

	// Field repair: damage sword slightly and repair with whetstone
	player := srv.World.Player()
	var sword *world.Entity
	for _, cid := range player.Contents {
		ent := srv.World.Get(cid)
		if ent != nil && ent.HasKeyword("sword") {
			sword = ent
			break
		}
	}
	if sword == nil {
		t.Fatalf("player missing short sword")
	}
	sword.Durability = 15

	sess.In <- "repair sword with whetstone"
	time.Sleep(40 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "repair") {
		t.Fatalf("expected repair with whetstone, got: %s", all)
	}
	if sword.Durability <= 15 {
		t.Fatalf("expected sword durability to increase above 15, got %d", sword.Durability)
	}
}
