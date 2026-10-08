package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sudengine/internal/commands"
	"sudengine/internal/protocol"
	"sudengine/internal/world"
)

func TestFollowAndPartyServerIntegration(t *testing.T) {
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

	player := srv.World.Player()
	if player == nil {
		t.Fatalf("player not found")
	}

	// Spawn a friendly scout companion in hollow.square
	scout, err := srv.World.Spawn("hollow.wolf", player.Parent)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	scout.Name = "tamed wolf"
	scout.Short = "tamed wolf"
	scout.Keywords = []string{"tamed", "wolf"}
	scout.Companion = true
	scout.Owner = player.ID
	scout.AI = nil // not wild

	// 1. Invite tamed wolf to party
	sess.In <- "invite wolf"
	time.Sleep(40 * time.Millisecond)
	lines := drainLines(sess)
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "tamed wolf joins your party!") {
		t.Fatalf("expected wolf to join party, got:\n%s", all)
	}

	if scout.Leader != player.ID {
		t.Fatalf("expected wolf following player, got %s", scout.Leader)
	}
	if scout.PartyLeader != player.ID {
		t.Fatalf("expected wolf in party, got %s", scout.PartyLeader)
	}

	// 2. View group
	sess.In <- "group"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "Party of Hero:") || !strings.Contains(all, "tamed wolf") {
		t.Fatalf("expected group roster to show wolf, got:\n%s", all)
	}

	// 3. Move east into lane
	sess.In <- "east"
	time.Sleep(40 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "tamed wolf follows you east") {
		t.Fatalf("expected wolf follow echo, got:\n%s", all)
	}

	if player.Parent != "hollow.lane" {
		t.Fatalf("expected player in lane, got %s", player.Parent)
	}
	if scout.Parent != "hollow.lane" {
		t.Fatalf("expected wolf in lane, got %s", scout.Parent)
	}

	// 4. Shake off follower
	sess.In <- "lose wolf"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	all = strings.Join(lines, "\n")
	if !strings.Contains(all, "You shake off tamed wolf.") {
		t.Fatalf("expected lose echo, got:\n%s", all)
	}

	// 5. Move west back to square -> wolf should remain in lane
	sess.In <- "west"
	time.Sleep(40 * time.Millisecond)
	drainLines(sess)

	if player.Parent != "hollow.square" {
		t.Fatalf("expected player in square, got %s", player.Parent)
	}
	if scout.Parent != "hollow.lane" {
		t.Fatalf("expected wolf to remain in lane, got %s", scout.Parent)
	}
}

func TestCompanionCombatAndQuestKillAttribution(t *testing.T) {
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

	player := srv.World.Player()
	if player == nil {
		t.Fatalf("player not found")
	}

	// 1. Companion dog with assist stance
	dog, err := srv.World.Spawn("hollow.wolf", player.Parent)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	dog.Short = "war hound"
	dog.Companion = true
	dog.Owner = player.ID
	dog.Stance = "assist"
	dog.Loyalty = 100
	dog.MaxLoyalty = 100
	dog.AI = nil
	_ = srv.World.StartFollowing(dog, player)

	// Companion bear with defend stance
	bear, err := srv.World.Spawn("hollow.wolf", player.Parent)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	bear.Short = "guard bear"
	bear.Companion = true
	bear.Owner = player.ID
	bear.Stance = "defend"
	bear.Loyalty = 100
	bear.MaxLoyalty = 100
	bear.AI = nil
	_ = srv.World.StartFollowing(bear, player)

	// Spawn hostile mob
	target, err := srv.World.Spawn("hollow.wolf", player.Parent)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	target.Short = "forest predator"
	target.AI = nil // manual trigger

	// Player attacks target -> assist stance joins, defend stance holds.
	commands.Dispatch(srv.ctx(), commands.Parse("kill predator", nil))
	if dog.Combat == nil || dog.Combat.Target != target.ID {
		t.Fatalf("dog with assist stance should join combat against target")
	}
	if bear.Combat != nil && bear.Combat.Target != "" {
		t.Fatalf("bear with defend stance should NOT join unprovoked player attack")
	}
	if target.Combat == nil || target.Combat.Target != player.ID {
		t.Fatalf("kill should still point the mob at the player")
	}

	// Now target mob attacks player -> defend stance engages!
	target.Combat.Target = player.ID
	srv.Combat.CheckAssists(srv.World, player, target, srv.World.Tick, false)
	if bear.Combat == nil || bear.Combat.Target != target.ID {
		t.Fatalf("bear with defend stance should engage when target attacks player")
	}

	// 2. Quest kill attribution: companion kills target
	primary := srv.Pack.RPG.Primary()
	target.SetRes(primary, world.Resource{Current: 0, Max: 20})
	target.SlainBy = dog.ID
	target.XP = 8
	initialXP := player.XP
	initialLevel := player.Level
	srv.reapDead()

	// Player should receive XP or level even though dog dealt the killing blow
	if player.XP != initialXP+8 && player.Level == initialLevel {
		t.Fatalf("expected player to gain XP or level from companion kill, got xp %d level %d", player.XP, player.Level)
	}
}

func TestOwnedAndStayMobsSkipWildAI(t *testing.T) {
	packDir := filepath.Join("..", "..", "games", "green-hollow")
	srv, _, err := Launch(Options{PackDir: packDir, Mode: protocol.ModePlay, PlayerName: "Hero", Seed: 1})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	player := srv.World.Player()
	lane := srv.World.Get("hollow.lane")
	if player == nil || lane == nil {
		t.Fatal("missing player or lane")
	}
	_ = srv.World.Move(player.ID, lane.ID)

	pet, err := srv.World.Spawn("hollow.wolf", lane.ID)
	if err != nil {
		t.Fatal(err)
	}
	pet.Owner = player.ID
	pet.Companion = true
	pet.Stance = "assist"
	pet.AI.Rooms = nil
	pet.AI.WanderEvery = 1
	_ = srv.World.StartFollowing(pet, player)

	stay, err := srv.World.Spawn("hollow.wolf", lane.ID)
	if err != nil {
		t.Fatal(err)
	}
	stay.Stance = "stay"
	stay.AI.Rooms = nil
	stay.AI.WanderEvery = 1

	srv.ai()
	if pet.InCombat() {
		t.Fatal("owned pet attacked its owner")
	}
	if stay.InCombat() {
		t.Fatal("stay mob entered combat on its own")
	}
	for i := 0; i < 40; i++ {
		srv.World.Tick++
		srv.ai()
	}
	if pet.Parent != lane.ID {
		t.Fatalf("owned pet wandered to %s", pet.Parent)
	}
	if stay.Parent != lane.ID {
		t.Fatalf("stay mob wandered to %s", stay.Parent)
	}

	wild, err := srv.World.Spawn("hollow.wolf", lane.ID)
	if err != nil {
		t.Fatal(err)
	}
	wild.AI.Rooms = nil
	wild.Short = "wild wolf"
	srv.ai()
	if wild.Combat == nil || wild.Combat.Target != player.ID {
		t.Fatal("wild wolf should still attack on sight")
	}
	if pet.Combat != nil && pet.Combat.Target == player.ID {
		t.Fatal("owned pet attacked its owner after a wild wolf arrived")
	}
}

func TestKillCreditSurvivesCompanionDeath(t *testing.T) {
	packDir := filepath.Join("..", "..", "games", "green-hollow")
	srv, _, err := Launch(Options{PackDir: packDir, Mode: protocol.ModePlay, PlayerName: "Hero", Seed: 1})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	player := srv.World.Player()
	primary := srv.Pack.RPG.Primary()
	dog, err := srv.World.Spawn("hollow.wolf", player.Parent)
	if err != nil {
		t.Fatal(err)
	}
	dog.Owner = player.ID
	dog.Companion = true
	dog.AI = nil
	victim, err := srv.World.Spawn("hollow.wolf", player.Parent)
	if err != nil {
		t.Fatal(err)
	}
	victim.AI = nil
	victim.XP = 8
	victim.SlainBy = dog.ID
	victim.SetRes(primary, world.Resource{Current: 0, Max: 10})
	dog.SlainBy = victim.ID
	dog.SetRes(primary, world.Resource{Current: 0, Max: 10})
	before := player.XP
	level := player.Level
	srv.reapDead()
	if player.XP != before+8 && player.Level == level {
		t.Fatalf("expected credit after the companion died in the same pass, xp %d level %d", player.XP, player.Level)
	}
}

func TestPlayerFollowArrives(t *testing.T) {
	packDir := filepath.Join("..", "..", "games", "green-hollow")
	srv, sess, err := Launch(Options{PackDir: packDir, Mode: protocol.ModePlay, PlayerName: "Hero", Seed: 1})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	player := srv.World.Player()
	guide, err := srv.World.Spawn("hollow.wolf", player.Parent)
	if err != nil {
		t.Fatal(err)
	}
	guide.AI = nil
	guide.Short = "trail guide"
	old := srv.World.RoomOf(guide)
	lane := srv.World.Get("hollow.lane")
	if old == nil || lane == nil || old.ID == lane.ID {
		t.Fatal("need two distinct rooms")
	}
	_ = srv.World.StartFollowing(player, guide)
	_ = srv.World.Move(guide.ID, lane.ID)
	if !srv.moveFollowers(guide, old, lane, "east") {
		t.Fatal("expected the player to follow")
	}
	if player.Parent != lane.ID {
		t.Fatalf("player stayed in %s", player.Parent)
	}
	all := strings.Join(drainLines(sess), "\n")
	if !strings.Contains(all, "You follow trail guide east.") {
		t.Fatalf("missing follow line:\n%s", all)
	}
	if lane.Long != "" && !strings.Contains(all, strings.TrimSpace(strings.Split(lane.Long, "\n")[0])) {
		t.Fatalf("following skipped the room text:\n%s", all)
	}
}

func TestDeathRecallsOwnedCompanions(t *testing.T) {
	packDir := filepath.Join("..", "..", "games", "green-hollow")
	srv, _, err := Launch(Options{PackDir: packDir, Mode: protocol.ModePlay, PlayerName: "Hero", Seed: 1})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	player := srv.World.Player()
	primary := srv.Pack.RPG.Primary()
	lane := srv.World.Get("hollow.lane")
	if lane == nil || srv.World.StartRoom == "" {
		t.Fatal("missing lane or start")
	}
	pet, err := srv.World.Spawn("hollow.wolf", lane.ID)
	if err != nil {
		t.Fatal(err)
	}
	pet.AI = nil
	pet.Owner = player.ID
	pet.Short = "loyal hound"
	_ = srv.World.StartFollowing(pet, player)
	stay, err := srv.World.Spawn("hollow.wolf", lane.ID)
	if err != nil {
		t.Fatal(err)
	}
	stay.AI = nil
	stay.Owner = player.ID
	stay.Stance = "stay"
	stay.Short = "parked mule"
	max := player.Res(primary).Max
	if max < 1 {
		max = 10
	}
	player.SetRes(primary, world.Resource{Current: 0, Max: max})
	srv.reapDead()
	if player.Parent != srv.World.StartRoom {
		t.Fatalf("player parent %s", player.Parent)
	}
	if pet.Parent != srv.World.StartRoom {
		t.Fatalf("owned pet left in %s", pet.Parent)
	}
	if stay.Parent != lane.ID || stay.Leader != "" {
		t.Fatalf("stay pet should remain and stop following, parent %s leader %s", stay.Parent, stay.Leader)
	}
}
