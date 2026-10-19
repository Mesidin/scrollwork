package server

import (
	"path/filepath"
	"strings"
	"testing"

	"sudengine/internal/protocol"
	"sudengine/internal/world"
)

func TestStage1QuitDoesNotStopTheOtherSession(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, ada, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	bob, _ := srv.SpawnPlayerSession("Bob", "", "")
	bob.quit = true
	if srv.reapQuits() {
		t.Fatal("Bob quitting stopped the local session")
	}
	if !bob.Closed() {
		t.Fatal("Bob was not disconnected")
	}
	if _, ok := srv.Sessions[bob.ID]; ok {
		t.Fatal("Bob stayed in the session map")
	}
	if ada.Closed() || srv.localQuit {
		t.Fatal("Ada was disconnected with Bob")
	}
	evs := takeEvents(bob)
	if !hasDisconnect(evs, "quit") {
		t.Fatalf("Bob missing quit disconnect: %#v", evs)
	}

	ada.quit = true
	if !srv.reapQuits() {
		t.Fatal("the local session quitting should end Run")
	}
	if !ada.Closed() {
		t.Fatal("Ada was not disconnected")
	}
	if !hasDisconnect(takeEvents(ada), "quit") {
		t.Fatal("Ada missing quit disconnect")
	}
}

func TestStage1StallDisconnectsOnlyThatSession(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, ada, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	bob, _ := srv.SpawnPlayerSession("Bob", "", "")
	for i := 0; i < cap(bob.Out); i++ {
		bob.Send(protocol.TextEvent{Channel: protocol.ChanNarrative, Text: "held"})
	}
	if len(bob.Out) != cap(bob.Out) {
		t.Fatalf("expected a full queue, len %d cap %d", len(bob.Out), cap(bob.Out))
	}
	srv.tick()
	if bob.Closed() {
		t.Fatal("one full tick should not disconnect")
	}
	srv.tick()
	if !bob.Closed() {
		t.Fatal("two full ticks should disconnect Bob")
	}
	if ada.Closed() || srv.localQuit {
		t.Fatal("Ada's world stopped because Bob's queue was full")
	}
	if _, ok := srv.Sessions[bob.ID]; ok {
		t.Fatal("stalled Bob stayed in the session map")
	}
	var all []protocol.Event
	for i := 0; i < 6; i++ {
		batch := takeEvents(bob)
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
	}
	if !hasDisconnect(all, "stalled") {
		t.Fatal("stalled session did not receive a disconnect event")
	}
	texts := 0
	for _, ev := range all {
		if te, ok := ev.(protocol.TextEvent); ok && te.Text == "held" {
			texts++
		}
	}
	if texts != cap(bob.Out) {
		t.Fatalf("stall dropped held lines: got %d want %d", texts, cap(bob.Out))
	}
}

func TestOwnedPetDoesNotGoWildAwayFromOwner(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, _, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	ada := srv.World.Player()
	square := srv.World.Get("hollow.square")
	lane := srv.World.Get("hollow.lane")
	if ada == nil || square == nil || lane == nil {
		t.Fatal("missing rooms")
	}
	_, bob := srv.SpawnPlayerSession("Bob", "", "")
	_ = srv.World.Move(bob.ID, square.ID)
	_ = srv.World.Move(ada.ID, lane.ID)

	pet, err := srv.World.Spawn("hollow.wolf", square.ID)
	if err != nil {
		t.Fatal(err)
	}
	pet.Owner = ada.ID
	pet.Companion = true
	pet.Short = "loyal hound"
	pet.AI.Profile = "aggressive"
	pet.AI.Rooms = nil
	pet.AI.WanderEvery = 1
	_ = srv.World.StartFollowing(pet, ada)
	pet.Combat = &world.Combat{Target: "hollow.wolf"}

	for i := 0; i < 40; i++ {
		srv.World.Tick++
		srv.ai()
	}
	if pet.Parent != square.ID {
		t.Fatalf("pet left behind wandered to %s", pet.Parent)
	}
	if pet.Combat == nil || pet.Combat.Target != "hollow.wolf" {
		t.Fatalf("pet retargeted while away from its owner: %+v", pet.Combat)
	}
	if bob.InCombat() {
		t.Fatal("pet attacked the other player")
	}
}

func TestFollowerDoesNotHideTheMove(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, adaSess, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	ada := srv.World.Player()
	square := srv.World.Get("hollow.square")
	lane := srv.World.Get("hollow.lane")
	bobSess, bob := srv.SpawnPlayerSession("Bob", "", "")
	caraSess, cara := srv.SpawnPlayerSession("Cara", "", "")
	_ = srv.World.Move(ada.ID, lane.ID)
	_ = srv.World.Move(bob.ID, square.ID)
	_ = srv.World.Move(cara.ID, square.ID)

	wolf, err := srv.World.Spawn("hollow.wolf", square.ID)
	if err != nil {
		t.Fatal(err)
	}
	wolf.AI.Rooms = nil
	wolf.AI.Pursue = true
	wolf.AI.Profile = "aggressive"
	wolf.Combat = &world.Combat{Target: ada.ID}
	_ = srv.World.StartFollowing(cara, wolf)

	_ = takeEvents(adaSess)
	_ = takeEvents(bobSess)
	_ = takeEvents(caraSess)
	srv.pursue(wolf, square)

	bobText := texts(takeEvents(bobSess))
	adaText := texts(takeEvents(adaSess))
	caraText := texts(takeEvents(caraSess))
	if !strings.Contains(bobText, "A grey wolf leaves east.") {
		t.Fatalf("bystander missed the leave:\n%s", bobText)
	}
	if !strings.Contains(adaText, "A grey wolf follows you.") {
		t.Fatalf("target missed the pursue:\n%s", adaText)
	}
	if !strings.Contains(caraText, "You follow a grey wolf east.") {
		t.Fatalf("follower missed follow line:\n%s", caraText)
	}
	if strings.Contains(caraText, "A grey wolf arrives.") {
		t.Fatalf("follower also got the bystander arrive:\n%s", caraText)
	}
}

func TestFollowingPlayerEntersOnce(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, adaSess, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	ada := srv.World.Player()
	_, bob := srv.SpawnPlayerSession("Bob", "", "")
	_ = srv.World.StartFollowing(bob, ada)
	lane := srv.World.Get("hollow.lane")
	lane.Scripts = "enter.lua"
	srv.Scripts.Reload(map[string]string{
		"enter.lua": "function on_enter(actor, self)\n  echo(\"entered\")\nend\n",
	})
	hits := 0
	prev := srv.Scripts.Echo
	srv.Scripts.Echo = func(actor *world.Entity, text string) {
		if text == "entered" {
			hits++
		}
		if prev != nil {
			prev(actor, text)
		}
	}
	_ = srv.HandleSessionLine(adaSess, "east")
	if hits != 2 {
		t.Fatalf("on_enter ran %d times, want 2 (walker and follower)", hits)
	}
	if bob.Parent != lane.ID {
		t.Fatalf("Bob stayed in %s", bob.Parent)
	}
}

func TestDeathCreditsTheKillerNotTheFirstPlayer(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, adaSess, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	ada := srv.World.Player()
	bobSess, bob := srv.SpawnPlayerSession("Bob", "", "")
	bob.PartyLeader = bob.ID
	primary := srv.Pack.RPG.Primary()
	srv.Scripts.Reload(map[string]string{
		"death.lua": `function on_death(actor, self)
  if actor ~= nil then
    echo(self.name .. " by " .. actor.name)
  end
end`,
	})

	ownerDog, err := srv.World.Spawn("hollow.wolf", ada.Parent)
	if err != nil {
		t.Fatal(err)
	}
	ownerDog.AI = nil
	ownerDog.Owner = bob.ID
	chainHound, err := srv.World.Spawn("hollow.wolf", ada.Parent)
	if err != nil {
		t.Fatal(err)
	}
	chainHound.AI = nil
	chainHound.Owner = bob.ID
	chainDog, err := srv.World.Spawn("hollow.wolf", ada.Parent)
	if err != nil {
		t.Fatal(err)
	}
	chainDog.AI = nil
	chainDog.Owner = chainHound.ID
	ally, err := srv.World.Spawn("hollow.wolf", ada.Parent)
	if err != nil {
		t.Fatal(err)
	}
	ally.AI = nil
	ally.PartyLeader = bob.ID

	spawnVictim := func(name string, slain world.ID) {
		t.Helper()
		v, err := srv.World.Spawn("hollow.wolf", ada.Parent)
		if err != nil {
			t.Fatal(err)
		}
		v.AI = nil
		v.Name = name
		v.Short = name
		v.XP = 8
		v.Scripts = "death.lua"
		v.SlainBy = slain
		v.SetRes(primary, world.Resource{Current: 0, Max: 10})
	}
	spawnVictim("owner", ownerDog.ID)
	spawnVictim("chain", chainDog.ID)
	spawnVictim("party", ally.ID)
	spawnVictim("loose", "")

	adaXP, adaLevel := ada.XP, ada.Level
	bobXP, bobLevel := bob.XP, bob.Level
	_ = takeEvents(adaSess)
	_ = takeEvents(bobSess)
	srv.reapDead()

	if ada.XP != adaXP || ada.Level != adaLevel {
		t.Fatalf("Ada was credited: xp %d->%d level %d->%d", adaXP, ada.XP, adaLevel, ada.Level)
	}
	if bob.XP == bobXP && bob.Level == bobLevel {
		t.Fatal("Bob gained nothing from his allies' kills")
	}
	adaText := texts(takeEvents(adaSess))
	bobText := texts(takeEvents(bobSess))
	for _, name := range []string{"owner by Bob", "chain by Bob", "party by Bob"} {
		if !strings.Contains(bobText, name) {
			t.Fatalf("Bob missing %q:\n%s", name, bobText)
		}
		if strings.Contains(adaText, name) {
			t.Fatalf("Ada heard %q:\n%s", name, adaText)
		}
	}
	if strings.Contains(adaText, "loose") || strings.Contains(bobText, "loose") {
		t.Fatalf("unattributed death was credited:\nada:\n%s\nbob:\n%s", adaText, bobText)
	}
}

func TestPlayerDeathRefreshesBothRooms(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, adaSess, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	ada := srv.World.Player()
	bobSess, bob := srv.SpawnPlayerSession("Bob", "", "")
	caraSess, _ := srv.SpawnPlayerSession("Cara", "", "")
	lane := srv.World.Get("hollow.lane")
	start := srv.World.Get(srv.World.StartRoom)
	if lane == nil || start == nil || start.ID == lane.ID {
		t.Fatal("need a start room and a different lane")
	}
	lane.SetFlag("lit", true)
	_ = srv.World.Move(ada.ID, lane.ID)
	_ = srv.World.Move(bob.ID, lane.ID)
	keep := false
	srv.Pack.RPG.Death.Player.KeepItems = &keep
	coin := &world.Entity{ID: "ada.coin", Kind: world.KindItem, Name: "coin", Short: "a coin", Takeable: true}
	srv.World.Add(coin)
	_ = srv.World.Move(coin.ID, ada.ID)
	primary := srv.Pack.RPG.Primary()
	max := ada.Res(primary).Max
	if max < 1 {
		max = 10
	}
	ada.SetRes(primary, world.Resource{Current: 0, Max: max})
	_ = takeEvents(adaSess)
	_ = takeEvents(bobSess)
	_ = takeEvents(caraSess)
	srv.reapDead()

	if ada.Parent != start.ID {
		t.Fatalf("Ada parent %s", ada.Parent)
	}
	if coin.Parent != lane.ID {
		t.Fatalf("coin parent %s", coin.Parent)
	}
	bobEvs := takeEvents(bobSess)
	caraEvs := takeEvents(caraSess)
	adaEvs := takeEvents(adaSess)
	bobText := texts(bobEvs)
	caraText := texts(caraEvs)
	adaText := texts(adaEvs)
	if !strings.Contains(bobText, "Ada collapses.") {
		t.Fatalf("Bob missed the collapse:\n%s", bobText)
	}
	if !strings.Contains(caraText, "Ada arrives.") {
		t.Fatalf("Cara missed the recall:\n%s", caraText)
	}
	if !strings.Contains(adaText, "You die.") {
		t.Fatalf("Ada missed the fade line:\n%s", adaText)
	}
	if strings.Contains(bobText, "Ada arrives.") {
		t.Fatalf("Bob is not in the start room:\n%s", bobText)
	}
	bobRoom := lastRoom(bobEvs)
	if bobRoom == nil {
		t.Fatal("Bob's pane was not refreshed")
	}
	for _, occ := range bobRoom.Occupants {
		if strings.Contains(occ, "Ada") {
			t.Fatalf("Bob still sees Ada: %v", bobRoom.Occupants)
		}
	}
	foundCoin := false
	for _, it := range bobRoom.Items {
		if strings.Contains(it, "coin") {
			foundCoin = true
		}
	}
	if !foundCoin {
		t.Fatalf("dropped coin missing from Bob's pane: %v", bobRoom.Items)
	}
}

func TestCombatPrivacyIsNotProse(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, adaSess, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	ada := srv.World.Player()
	bobSess, bob := srv.SpawnPlayerSession("Bob", "", "")
	_ = srv.World.Move(bob.ID, ada.Parent)
	wolf, err := srv.World.Spawn("hollow.wolf", ada.Parent)
	if err != nil {
		t.Fatal(err)
	}
	wolf.AI = nil
	primary := srv.Pack.RPG.Primary()
	wolf.SetRes(primary, world.Resource{Current: 40, Max: 40})
	rules := *srv.Combat.Rules
	rules.Checks = nil
	srv.Combat.Rules = &rules
	blade := &world.Entity{
		ID: "blade", Kind: world.KindItem, Name: "blade", Short: "a blade",
		MaxDurability: 1, Durability: 1,
		Weapon: &world.Weapon{Verb: "hit", Damage: "1d4", Hit: "Your blade bites {name}."},
	}
	srv.World.Add(blade)
	_ = srv.World.Move(blade.ID, ada.ID)
	ada.Equipment = map[string]world.ID{"wield": blade.ID}
	now := srv.World.Tick
	srv.Combat.Start(ada, wolf, now)
	wolf.Combat.NextSwing = now + 50
	ada.Combat.NextSwing = now
	_ = takeEvents(adaSess)
	_ = takeEvents(bobSess)
	srv.Combat.Tick(srv.World, now)

	adaText := texts(takeEvents(adaSess))
	bobText := texts(takeEvents(bobSess))
	if !strings.Contains(adaText, "Your blade bites") || !strings.Contains(bobText, "Your blade bites") {
		t.Fatalf("swing line should reach the room:\nada:\n%s\nbob:\n%s", adaText, bobText)
	}
	if !strings.Contains(adaText, "Your blade breaks!") {
		t.Fatalf("Ada missed the break:\n%s", adaText)
	}
	if strings.Contains(bobText, "breaks") {
		t.Fatalf("gear break was broadcast:\n%s", bobText)
	}

	wolf.SetRes(primary, world.Resource{Current: 40, Max: 40})
	ada.SetRes(primary, world.Resource{Current: 1, Max: 20})
	srv.Combat.Start(wolf, ada, now)
	ada.Combat.NextSwing = now + 50
	wolf.Combat.NextSwing = now
	_ = takeEvents(adaSess)
	_ = takeEvents(bobSess)
	srv.Combat.Tick(srv.World, now)
	adaText = texts(takeEvents(adaSess))
	bobText = texts(takeEvents(bobSess))
	if !strings.Contains(adaText, "You collapse.") {
		t.Fatalf("Ada missed collapse:\n%s", adaText)
	}
	if strings.Contains(bobText, "You collapse.") {
		t.Fatalf("collapse was broadcast:\n%s", bobText)
	}
}

func hasDisconnect(evs []protocol.Event, reason string) bool {
	for _, ev := range evs {
		if d, ok := ev.(protocol.DisconnectEvent); ok && d.Reason == reason {
			return true
		}
	}
	return false
}

func lastRoom(evs []protocol.Event) *protocol.RoomEvent {
	var last *protocol.RoomEvent
	for i := range evs {
		if room, ok := evs[i].(protocol.RoomEvent); ok {
			r := room
			last = &r
		}
	}
	return last
}
