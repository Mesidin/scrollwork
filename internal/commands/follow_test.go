package commands_test

import (
	"strings"
	"testing"

	"sudengine/internal/combat"
	"sudengine/internal/commands"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/world"
)

func newTestContext() (*commands.Context, *world.World, *world.Entity, *[]string) {
	w := world.New()
	var output []string

	r1 := &world.Entity{
		ID:    "room1",
		Kind:  world.KindRoom,
		Name:  "Town Square",
		Exits: map[string]world.Exit{"north": {To: "room2", Dir: "north"}},
	}
	r2 := &world.Entity{
		ID:    "room2",
		Kind:  world.KindRoom,
		Name:  "North Road",
		Exits: map[string]world.Exit{"south": {To: "room1", Dir: "south"}},
	}
	w.Add(r1)
	w.Add(r2)

	player := &world.Entity{
		ID:        "player",
		Kind:      world.KindPlayer,
		Name:      "Hero",
		Resources: map[string]world.Resource{"vigor": {Current: 10, Max: 10}},
	}
	w.Add(player)
	_ = w.Move(player.ID, r1.ID)

	p := &pack.Pack{
		Lexicon: pack.Lexicon{},
	}

	c := &commands.Context{
		World: w,
		Actor: player,
		Pack:  p,
		Tell: func(channel protocol.Channel, text string) {
			output = append(output, text)
		},
	}
	return c, w, player, &output
}

func parse(input string) commands.Parsed {
	return commands.Parse(input, nil)
}

func TestFollowAndMovementPropagation(t *testing.T) {
	c, w, player, out := newTestContext()

	hound := &world.Entity{
		ID:        "hound",
		Kind:      world.KindMobile,
		Name:      "faithful hound",
		Keywords:  []string{"hound", "dog"},
		Companion: true,
	}
	w.Add(hound)
	_ = w.Move(hound.ID, "room1")

	// 1. Hound follows player
	if err := w.StartFollowing(hound, player); err != nil {
		t.Fatalf("StartFollowing: %v", err)
	}

	// 2. Check follow status
	*out = nil
	commands.Lookup("follow")(c, parse("follow"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "faithful hound") {
		t.Fatalf("expected follow status to list hound, got: %s", all)
	}

	// 3. Move north -> hound should propagate to room2
	*out = nil
	commands.Lookup("north")(c, parse("north"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "faithful hound follows you north") {
		t.Fatalf("expected follow echo, got: %s", all)
	}

	if player.Parent != "room2" {
		t.Fatalf("expected player in room2, got %s", player.Parent)
	}
	if hound.Parent != "room2" {
		t.Fatalf("expected hound in room2, got %s", hound.Parent)
	}

	// 4. Stance 'stay' prevents following
	hound.Stance = "stay"
	*out = nil
	commands.Lookup("south")(c, parse("south"))
	if player.Parent != "room1" {
		t.Fatalf("expected player back in room1")
	}
	if hound.Parent != "room2" {
		t.Fatalf("hound in stay stance should have stayed in room2, got %s", hound.Parent)
	}
}

func TestFollowCommandsAndLoopCheck(t *testing.T) {
	c, w, player, out := newTestContext()

	guard := &world.Entity{
		ID:       "guard",
		Kind:     world.KindMobile,
		Name:     "town guard",
		Keywords: []string{"guard"},
	}
	w.Add(guard)
	_ = w.Move(guard.ID, "room1")

	// Follow guard
	*out = nil
	commands.Lookup("follow")(c, parse("follow guard"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "You begin following town guard.") {
		t.Fatalf("expected follow confirmation, got: %s", all)
	}
	if player.Leader != "guard" {
		t.Fatalf("expected player leader guard, got %s", player.Leader)
	}

	// Loop prevention
	if err := w.StartFollowing(guard, player); err == nil {
		t.Fatalf("expected loop detection error")
	}

	// Unfollow
	*out = nil
	commands.Lookup("unfollow")(c, parse("unfollow"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "You stop following town guard.") {
		t.Fatalf("expected unfollow confirmation, got: %s", all)
	}
	if player.Leader != "" {
		t.Fatalf("expected leader cleared")
	}
}

func TestPartyAndInvite(t *testing.T) {
	c, w, player, out := newTestContext()

	scout := &world.Entity{
		ID:        "scout",
		Kind:      world.KindMobile,
		Name:      "elven scout",
		Keywords:  []string{"scout"},
		Companion: true,
		Level:     2,
		Resources: map[string]world.Resource{"vigor": {Current: 14, Max: 14}},
	}
	w.Add(scout)
	_ = w.Move(scout.ID, "room1")

	// Invite companion scout
	*out = nil
	commands.Lookup("invite")(c, parse("invite scout"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "elven scout joins your party!") {
		t.Fatalf("expected companion to join party, got: %s", all)
	}

	if scout.PartyLeader != player.ID {
		t.Fatalf("expected scout party leader player, got %s", scout.PartyLeader)
	}
	if scout.Leader != player.ID {
		t.Fatalf("expected scout following player, got %s", scout.Leader)
	}

	// Group display uses the pack's primary resource, not a fixed key list.
	c.Pack.RPG.PrimaryResource = "vigor"
	*out = nil
	commands.Lookup("group")(c, parse("group"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "Party of Hero:") || !strings.Contains(all, "elven scout (Lvl 2)") || !strings.Contains(all, "vigor: 14/14") {
		t.Fatalf("expected group roster, got: %s", all)
	}

	// Shake off follower
	*out = nil
	commands.Lookup("lose")(c, parse("lose scout"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "You shake off elven scout.") {
		t.Fatalf("expected lose echo, got: %s", all)
	}
	if scout.Leader != "" {
		t.Fatalf("expected scout not following player")
	}
}

func TestPetOrderAndFeed(t *testing.T) {
	c, w, player, out := newTestContext()

	hound := &world.Entity{
		ID:         "hound",
		Kind:       world.KindMobile,
		Name:       "faithful hound",
		Keywords:   []string{"hound", "dog"},
		Companion:  true,
		Owner:      player.ID,
		Loyalty:    50,
		MaxLoyalty: 100,
		Stance:     "assist",
	}
	w.Add(hound)
	_ = w.Move(hound.ID, "room1")
	_ = w.StartFollowing(hound, player)

	// 1. Order stay
	*out = nil
	commands.Lookup("order")(c, parse("order hound stay"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "stays put") {
		t.Fatalf("expected stay output, got: %s", all)
	}
	if hound.Leader != "" || hound.Stance != "stay" {
		t.Fatalf("hound should not be following and stance should be stay")
	}

	// 2. Order follow
	*out = nil
	commands.Lookup("order")(c, parse("order hound follow"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "begins following you") {
		t.Fatalf("expected follow output, got: %s", all)
	}
	if hound.Leader != player.ID {
		t.Fatalf("hound should be following player")
	}
	if hound.Stance != "assist" {
		t.Fatalf("follow should restore the pre-stay stance, got %s", hound.Stance)
	}

	// 3. Order stance defend
	*out = nil
	commands.Lookup("order")(c, parse("order hound stance defend"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "stance is now defend") {
		t.Fatalf("expected stance output, got: %s", all)
	}
	if hound.Stance != "defend" {
		t.Fatalf("expected hound stance defend, got %s", hound.Stance)
	}

	// 4. Order attack target
	goblin := &world.Entity{
		ID:       "goblin",
		Kind:     world.KindMobile,
		Name:     "snarl goblin",
		Keywords: []string{"goblin"},
	}
	w.Add(goblin)
	_ = w.Move(goblin.ID, "room1")

	*out = nil
	commands.Lookup("order")(c, parse("order hound attack goblin"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "leaps into combat") {
		t.Fatalf("expected attack order output, got: %s", all)
	}
	if hound.Combat == nil || hound.Combat.Target != goblin.ID {
		t.Fatalf("hound should be targeting goblin")
	}

	// 5. Feeding non-food
	sword := &world.Entity{
		ID:       "sword1",
		Kind:     world.KindItem,
		Name:     "iron sword",
		Keywords: []string{"sword"},
	}
	w.Add(sword)
	_ = w.Move(sword.ID, player.ID)

	*out = nil
	commands.Lookup("feed")(c, parse("feed hound with sword"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "not food") {
		t.Fatalf("expected refusal to eat sword, got: %s", all)
	}
	if hound.Loyalty != 50 {
		t.Fatalf("loyalty should not change")
	}

	// 6. Feeding food
	bread := &world.Entity{
		ID:       "bread1",
		Kind:     world.KindItem,
		Name:     "crusty bread",
		Keywords: []string{"bread"},
		Flags:    map[string]bool{"food": true},
	}
	w.Add(bread)
	_ = w.Move(bread.ID, player.ID)

	*out = nil
	commands.Lookup("feed")(c, parse("feed hound with bread"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "happily devours it") || !strings.Contains(all, "Loyalty: 65/100") {
		t.Fatalf("expected successful feed, got: %s", all)
	}
	if hound.Loyalty != 65 {
		t.Fatalf("expected loyalty 65, got %d", hound.Loyalty)
	}
	if w.Get("bread1") != nil {
		t.Fatalf("bread should be consumed")
	}

	// 7. Order dismiss
	*out = nil
	commands.Lookup("order")(c, parse("order hound dismiss"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "dismiss") {
		t.Fatalf("expected dismiss output, got: %s", all)
	}
	if hound.Owner != "" || hound.Companion {
		t.Fatalf("hound should be dismissed")
	}
}

func TestBuyPetInShop(t *testing.T) {
	c, w, player, out := newTestContext()
	player.SetRes("coin", world.Resource{Current: 50, Max: 50})

	vendor := &world.Entity{
		ID:       "vendor",
		Kind:     world.KindMobile,
		Name:     "animal trainer",
		Keywords: []string{"vendor", "trainer"},
		Shop: &world.Shop{
			Stock: []string{"puppy"},
		},
	}
	w.Add(vendor)
	_ = w.Move(vendor.ID, "room1")

	puppy := &world.Entity{
		ID:        "puppy",
		Kind:      world.KindMobile,
		Name:      "playful puppy",
		Keywords:  []string{"puppy", "dog"},
		Value:     20,
		Companion: true,
	}
	w.Add(puppy)
	_ = w.Move(puppy.ID, vendor.ID)

	*out = nil
	commands.Lookup("buy")(c, parse("buy puppy from vendor"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "You buy playful puppy for 20 coin") || !strings.Contains(all, "following you") {
		t.Fatalf("expected buy pet output, got: %s", all)
	}

	if player.Res("coin").Current != 30 {
		t.Fatalf("expected 30 coins left, got %d", player.Res("coin").Current)
	}
	if puppy.Owner != player.ID {
		t.Fatalf("expected puppy owner player, got %s", puppy.Owner)
	}
	if puppy.Parent != "room1" {
		t.Fatalf("expected puppy in room1, got %s", puppy.Parent)
	}
	if puppy.Leader != player.ID {
		t.Fatalf("expected puppy following player")
	}
	if puppy.Loyalty != 100 || puppy.Stance != "assist" {
		t.Fatalf("expected loyalty 100 and assist stance, got %d, %s", puppy.Loyalty, puppy.Stance)
	}

	loose := &world.Entity{
		ID:          "stray",
		Kind:        world.KindMobile,
		Name:        "stray puppy",
		Keywords:    []string{"puppy", "stray"},
		PrototypeID: "puppy",
	}
	w.Add(loose)
	_ = w.Move(loose.ID, "room1")
	*out = nil
	commands.Lookup("buy")(c, parse("buy puppy from vendor"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "doesn't have") {
		t.Fatalf("second buy should not match the owned pet or a floor copy, got: %s", all)
	}
	if player.Res("coin").Current != 30 {
		t.Fatalf("second buy charged the player, coins %d", player.Res("coin").Current)
	}
	if loose.Parent != "room1" || loose.Owner != "" {
		t.Fatal("floor copy should stay unsold")
	}
}

func TestNPCPartyJoiningAndOffers(t *testing.T) {
	c, w, player, out := newTestContext()

	// 1. Regular NPC declines invite
	guard := &world.Entity{
		ID:       "guard",
		Kind:     world.KindMobile,
		Name:     "castle guard",
		Keywords: []string{"guard"},
	}
	w.Add(guard)
	_ = w.Move(guard.ID, "room1")

	*out = nil
	commands.Lookup("invite")(c, parse("invite guard"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "declines to join your party") {
		t.Fatalf("expected decline, got: %s", all)
	}

	// 2. NPC with party_member flag accepts invite
	ranger := &world.Entity{
		ID:       "ranger",
		Kind:     world.KindMobile,
		Name:     "friendly ranger",
		Keywords: []string{"ranger"},
		Flags:    map[string]bool{"party_member": true},
	}
	w.Add(ranger)
	_ = w.Move(ranger.ID, "room1")

	*out = nil
	commands.Lookup("invite")(c, parse("invite ranger"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "accepts your invitation and joins your party") {
		t.Fatalf("expected accept, got: %s", all)
	}
	if ranger.PartyLeader != player.ID || ranger.Leader != player.ID {
		t.Fatalf("ranger should be in party and following player")
	}

	// 3. Player joins open party of NPC
	paladin := &world.Entity{
		ID:       "paladin",
		Kind:     world.KindMobile,
		Name:     "crusader paladin",
		Keywords: []string{"paladin"},
		Flags:    map[string]bool{"open_party": true},
	}
	w.Add(paladin)
	_ = w.Move(paladin.ID, "room1")

	*out = nil
	commands.Lookup("join")(c, parse("join paladin"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "You have joined crusader paladin's party") {
		t.Fatalf("expected player joining paladin's party, got: %s", all)
	}
	if player.PartyLeader != paladin.ID || player.Leader != paladin.ID {
		t.Fatalf("player should have party leader paladin and follow paladin")
	}
	if ranger.PartyLeader != "" {
		t.Fatalf("joining another party should release the old member, still %s", ranger.PartyLeader)
	}
}

func TestOrderDoesNotControlPartyNPCs(t *testing.T) {
	c, w, _, out := newTestContext()
	ranger := &world.Entity{
		ID:       "ranger",
		Kind:     world.KindMobile,
		Name:     "friendly ranger",
		Keywords: []string{"ranger"},
		Flags:    map[string]bool{"party_member": true},
	}
	w.Add(ranger)
	_ = w.Move(ranger.ID, "room1")
	commands.Lookup("invite")(c, parse("invite ranger"))
	*out = nil
	commands.Lookup("order")(c, parse("order ranger attack ranger"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "does not take orders") {
		t.Fatalf("party member should refuse orders, got: %s", all)
	}
	if ranger.Companion || ranger.Owner != "" {
		t.Fatal("refused order should not strip the ranger")
	}
}

func TestFollowRefusalUsesTheMobsFlags(t *testing.T) {
	c, w, player, out := newTestContext()
	mob := &world.Entity{
		ID:       "sleeper",
		Kind:     world.KindMobile,
		Name:     "sleeping bear",
		Keywords: []string{"bear"},
		AI:       &world.AI{Profile: "aggressive", Aggro: "look", AggroFlag: "hostile"},
		Flags:    map[string]bool{"hostile": true},
	}
	w.Add(mob)
	_ = w.Move(mob.ID, "room1")
	player.SetFlag("marked", true)
	*out = nil
	commands.Lookup("follow")(c, parse("follow bear"))
	if player.Leader == mob.ID {
		t.Fatal("an awake look-aggro mob should refuse a follower")
	}
	if !strings.Contains(strings.Join(*out, "\n"), "won't let you follow") {
		t.Fatalf("expected refusal, got: %s", strings.Join(*out, "\n"))
	}

	calm := &world.Entity{
		ID:       "calm",
		Kind:     world.KindMobile,
		Name:     "calm stag",
		Keywords: []string{"stag"},
		AI:       &world.AI{Profile: "aggressive", Aggro: "flag", AggroFlag: "hostile"},
	}
	w.Add(calm)
	_ = w.Move(calm.ID, "room1")
	player.SetFlag("hostile", true)
	*out = nil
	commands.Lookup("follow")(c, parse("follow stag"))
	if player.Leader != calm.ID {
		t.Fatalf("flag aggro should check the mob, got: %s", strings.Join(*out, "\n"))
	}
}

func TestKillOpensAssistBeforeRetaliation(t *testing.T) {
	c, w, player, _ := newTestContext()
	dog := &world.Entity{ID: "dog", Kind: world.KindMobile, Name: "dog", Short: "dog", Stance: "assist", Owner: player.ID, Leader: player.ID}
	bear := &world.Entity{ID: "bear", Kind: world.KindMobile, Name: "bear", Short: "bear", Stance: "defend", Owner: player.ID, Leader: player.ID}
	foe := &world.Entity{ID: "foe", Kind: world.KindMobile, Name: "foe", Short: "forest predator", Keywords: []string{"predator"}}
	for _, e := range []*world.Entity{dog, bear, foe} {
		w.Add(e)
		_ = w.Move(e.ID, "room1")
	}
	player.Followers = []world.ID{dog.ID, bear.ID}
	var seenTarget string
	eng := &combat.Engine{Every: 4}
	c.Assist = func(target *world.Entity, leaderAttacking bool) {
		if !leaderAttacking {
			t.Fatal("kill should report that the leader opened the fight")
		}
		if target.Combat != nil && target.Combat.Target != "" {
			t.Fatal("assist ran after the mob was already pointed at the player")
		}
		seenTarget = string(target.ID)
		eng.CheckAssists(w, player, target, 1, leaderAttacking)
	}
	c.Tick = 1
	commands.Lookup("kill")(c, parse("kill predator"))
	if seenTarget != "foe" {
		t.Fatalf("assist target %s", seenTarget)
	}
	if dog.Combat == nil || dog.Combat.Target != foe.ID {
		t.Fatal("assist pet should join the player's attack")
	}
	if bear.Combat != nil && bear.Combat.Target != "" {
		t.Fatal("defend pet should wait until the player is attacked")
	}
	if foe.Combat == nil || foe.Combat.Target != player.ID {
		t.Fatal("the mob should still fight back")
	}
}

func TestFeedRejectsUntameableAndNameSubstrings(t *testing.T) {
	c, w, player, out := newTestContext()
	brigand := &world.Entity{ID: "brigand", Kind: world.KindMobile, Name: "brigand", Keywords: []string{"brigand"}}
	hook := &world.Entity{ID: "hook", Kind: world.KindItem, Name: "fishhook", Keywords: []string{"fishhook"}}
	hound := &world.Entity{
		ID: "hound", Kind: world.KindMobile, Name: "hound", Keywords: []string{"hound"},
		Companion: true, Owner: player.ID, MaxLoyalty: 100, Loyalty: 10,
		Flags: map[string]bool{"tameable": true},
	}
	bone := &world.Entity{ID: "bone", Kind: world.KindItem, Name: "a bone", Keywords: []string{"bone"}}
	for _, e := range []*world.Entity{brigand, hound} {
		w.Add(e)
		_ = w.Move(e.ID, "room1")
	}
	w.Add(hook)
	w.Add(bone)
	_ = w.Move(hook.ID, player.ID)
	_ = w.Move(bone.ID, player.ID)

	*out = nil
	commands.Lookup("feed")(c, parse("feed brigand with bone"))
	if w.Get("bone") == nil {
		t.Fatal("feeding a brigand should keep the food")
	}
	if !strings.Contains(strings.Join(*out, "\n"), "will not take food") {
		t.Fatalf("expected refusal, got: %s", strings.Join(*out, "\n"))
	}
	*out = nil
	commands.Lookup("feed")(c, parse("feed hound with fishhook"))
	if w.Get("hook") == nil {
		t.Fatal("fishhook matched as food")
	}
	if hound.Loyalty != 10 {
		t.Fatalf("loyalty changed to %d", hound.Loyalty)
	}
}

func TestStayThenFollowRestoresDefend(t *testing.T) {
	c, w, player, _ := newTestContext()
	hound := &world.Entity{
		ID: "hound", Kind: world.KindMobile, Name: "hound", Keywords: []string{"hound"},
		Companion: true, Owner: player.ID, Stance: "defend", Loyalty: 40, MaxLoyalty: 100,
	}
	w.Add(hound)
	_ = w.Move(hound.ID, "room1")
	_ = w.StartFollowing(hound, player)
	commands.Lookup("order")(c, parse("order hound stay"))
	if hound.Stance != "stay" || hound.RestStance != "defend" {
		t.Fatalf("stay should remember defend, stance %s rest %s", hound.Stance, hound.RestStance)
	}
	commands.Lookup("order")(c, parse("order hound follow"))
	if hound.Leader != player.ID || hound.Stance != "defend" || hound.RestStance != "" {
		t.Fatalf("follow should restore defend, leader %s stance %s rest %s", hound.Leader, hound.Stance, hound.RestStance)
	}
}
