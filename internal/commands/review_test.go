package commands_test

import (
	"strings"
	"testing"

	"sudengine/internal/commands"
	"sudengine/internal/economy"
	"sudengine/internal/quest"
	"sudengine/internal/world"
)

func TestBuyPetRefundsWhenFollowWouldLoop(t *testing.T) {
	c, w, player, out := newTestContext()
	player.SetRes("coin", world.Resource{Current: 50, Max: 50})

	vendor := &world.Entity{
		ID: "vendor", Kind: world.KindMobile, Name: "trainer", Keywords: []string{"vendor"},
		Shop: &world.Shop{Stock: []string{"puppy"}, Currency: "coin"},
	}
	puppy := &world.Entity{
		ID: "puppy", Kind: world.KindMobile, Name: "playful puppy", Keywords: []string{"puppy"},
		Value: 20, MaxLoyalty: 40,
	}
	w.Add(vendor)
	w.Add(puppy)
	_ = w.Move(vendor.ID, "room1")
	_ = w.Move(puppy.ID, vendor.ID)
	if err := w.StartFollowing(player, puppy); err != nil {
		t.Fatal(err)
	}

	*out = nil
	commands.Lookup("buy")(c, parse("buy puppy from vendor"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "already following") {
		t.Fatalf("expected the follow error, got: %s", all)
	}
	if strings.Contains(all, "following you") {
		t.Fatalf("buy announced success: %s", all)
	}
	if player.Res("coin").Current != 50 {
		t.Fatalf("coins changed to %d", player.Res("coin").Current)
	}
	if puppy.Owner != "" || puppy.Companion || puppy.Leader != "" || puppy.Loyalty != 0 || puppy.Parent != vendor.ID {
		t.Fatalf("puppy changed: owner %s companion %v leader %s loyalty %d parent %s", puppy.Owner, puppy.Companion, puppy.Leader, puppy.Loyalty, puppy.Parent)
	}
	if player.Leader != puppy.ID {
		t.Fatal("player should still be following the puppy")
	}
}

func TestBuyPetLoyaltyUsesMax(t *testing.T) {
	c, w, player, _ := newTestContext()
	player.SetRes("coin", world.Resource{Current: 50, Max: 50})
	vendor := &world.Entity{
		ID: "vendor", Kind: world.KindMobile, Name: "trainer", Keywords: []string{"vendor"},
		Shop: &world.Shop{Stock: []string{"puppy"}},
	}
	puppy := &world.Entity{
		ID: "puppy", Kind: world.KindMobile, Name: "playful puppy", Keywords: []string{"puppy"},
		Value: 10, Loyalty: 80, MaxLoyalty: 40,
	}
	w.Add(vendor)
	w.Add(puppy)
	_ = w.Move(vendor.ID, "room1")
	_ = w.Move(puppy.ID, vendor.ID)

	commands.Lookup("buy")(c, parse("buy puppy from vendor"))
	if puppy.Owner != player.ID || puppy.Leader != player.ID {
		t.Fatalf("puppy owner %s leader %s", puppy.Owner, puppy.Leader)
	}
	if puppy.Loyalty != 40 || puppy.MaxLoyalty != 40 {
		t.Fatalf("loyalty %d/%d", puppy.Loyalty, puppy.MaxLoyalty)
	}
	if player.Res("coin").Current != 40 {
		t.Fatalf("coins %d", player.Res("coin").Current)
	}
}

func TestInviteRefusesReverseFollow(t *testing.T) {
	c, w, player, out := newTestContext()
	ranger := &world.Entity{
		ID: "ranger", Kind: world.KindMobile, Name: "friendly ranger", Keywords: []string{"ranger"},
		Flags: map[string]bool{"party_member": true},
	}
	w.Add(ranger)
	_ = w.Move(ranger.ID, "room1")
	if err := w.StartFollowing(player, ranger); err != nil {
		t.Fatal(err)
	}

	*out = nil
	commands.Lookup("invite")(c, parse("invite ranger"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "already following") {
		t.Fatalf("expected the follow error, got: %s", all)
	}
	if strings.Contains(all, "joins your party") {
		t.Fatalf("invite announced success: %s", all)
	}
	if ranger.PartyLeader != "" || ranger.Leader != "" {
		t.Fatalf("ranger party %s leader %s", ranger.PartyLeader, ranger.Leader)
	}
	if player.PartyLeader != "" || player.Leader != ranger.ID {
		t.Fatalf("player party %s leader %s", player.PartyLeader, player.Leader)
	}
}

func TestDismissReleasesPlayerFollowingPet(t *testing.T) {
	c, w, player, out := newTestContext()
	hound := &world.Entity{
		ID: "hound", Kind: world.KindMobile, Name: "hound", Keywords: []string{"hound"},
		Companion: true, Owner: player.ID, Stance: "assist", Loyalty: 40, MaxLoyalty: 100,
	}
	w.Add(hound)
	_ = w.Move(hound.ID, "room1")
	hound.PartyLeader = hound.ID
	player.PartyLeader = hound.ID
	if err := w.StartFollowing(player, hound); err != nil {
		t.Fatal(err)
	}

	*out = nil
	commands.Lookup("order")(c, parse("order hound dismiss"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "dismiss") {
		t.Fatalf("expected dismiss, got: %s", all)
	}
	if player.Leader != "" || player.PartyLeader != "" {
		t.Fatalf("player still tied: leader %s party %s", player.Leader, player.PartyLeader)
	}
	if hound.Leader != "" || hound.PartyLeader != "" || hound.Owner != "" || hound.Companion {
		t.Fatalf("hound still tied: leader %s party %s owner %s companion %v", hound.Leader, hound.PartyLeader, hound.Owner, hound.Companion)
	}
}

func TestAskCompletesTalkOnlyOnOneReply(t *testing.T) {
	c, w, player, out := newTestContext()
	bram := &world.Entity{
		ID: "bram", Kind: world.KindMobile, Name: "Bram", Keywords: []string{"bram"},
		Topics: map[string]string{
			"crafting": "Bring me a cudgel.",
			"roads":    "The lane runs east.",
		},
		TeachRecipes: map[string]string{"crafting": "craft_cudgel"},
	}
	w.Add(bram)
	_ = w.Move(bram.ID, "room1")
	player.Quests = map[string]*world.QuestState{
		"word": {ID: "word", Status: "active"},
	}
	c.Pack.Quests = []quest.Quest{{
		ID:    "word",
		Title: "A Word",
		Objectives: []quest.Objective{{
			ID: "talk", Kind: quest.KindTalk, Target: "bram", Desc: "Speak with Bram",
		}},
	}}
	c.Pack.Recipes = []economy.Recipe{{ID: "craft_cudgel", Name: "cudgel", Discovery: "grant"}}

	*out = nil
	commands.Lookup("ask")(c, parse("ask bram about asdf"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "doesn't know about that") {
		t.Fatalf("expected a refusal, got: %s", all)
	}
	if strings.Contains(all, "Quest") || player.KnowsRecipe("craft_cudgel") {
		t.Fatalf("unanswered ask advanced the quest or recipe: %s", all)
	}

	*out = nil
	commands.Lookup("ask")(c, parse("ask bram about r"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "isn't sure") || !strings.Contains(all, "crafting") || !strings.Contains(all, "roads") {
		t.Fatalf("expected an ambiguous topic, got: %s", all)
	}
	if strings.Contains(all, "learned") || player.KnowsRecipe("craft_cudgel") {
		t.Fatalf("ambiguous ask taught a recipe: %s", all)
	}
	if player.Quests["word"].Progress != nil {
		t.Fatalf("ambiguous ask completed the objective: %+v", player.Quests["word"].Progress)
	}

	*out = nil
	commands.Lookup("ask")(c, parse("ask bram about craft"))
	all = strings.Join(*out, "\n")
	if !strings.Contains(all, "Bring me a cudgel") || !strings.Contains(all, "learned how to craft cudgel") {
		t.Fatalf("expected the crafting reply and the lesson, got: %s", all)
	}
	if player.RecipeMethod("craft_cudgel") != "grant" {
		t.Fatalf("method %s", player.RecipeMethod("craft_cudgel"))
	}
	if !strings.Contains(all, "Quest Ready") {
		t.Fatalf("expected the talk objective to finish, got: %s", all)
	}
}

func TestBuyingGoodsDoesNotTeachTheRecipe(t *testing.T) {
	c, w, player, out := newTestContext()
	player.SetRes("coin", world.Resource{Current: 20, Max: 20})
	c.Pack.Recipes = []economy.Recipe{{
		ID: "potion", Name: "healing potion", Discovery: "buy",
	}}
	manual := &world.Entity{
		ID: "manual", Kind: world.KindItem, Name: "potion manual", Keywords: []string{"manual"},
		Use: &world.UseEffect{LearnRecipe: "potion"},
	}
	potion := &world.Entity{
		ID: "potion1", Kind: world.KindItem, Name: "healing potion", PrototypeID: "potion",
		Keywords: []string{"potion"}, Value: 5,
	}
	vendor := &world.Entity{
		ID: "vendor", Kind: world.KindMobile, Name: "apothecary", Keywords: []string{"vendor"},
		Shop: &world.Shop{Stock: []string{"potion"}, Currency: "coin"},
	}
	w.Add(manual)
	w.Add(potion)
	w.Add(vendor)
	_ = w.Move(vendor.ID, "room1")
	_ = w.Move(potion.ID, vendor.ID)
	_ = w.Move(manual.ID, player.ID)

	*out = nil
	commands.Lookup("buy")(c, parse("buy potion from vendor"))
	if player.KnowsRecipe("potion") {
		t.Fatal("buying the finished good taught the recipe")
	}
	if potion.Parent != player.ID {
		t.Fatalf("potion parent %s, output %s", potion.Parent, strings.Join(*out, "\n"))
	}

	*out = nil
	commands.Lookup("use")(c, parse("use manual"))
	all := strings.Join(*out, "\n")
	if !strings.Contains(all, "learn how to craft healing potion") {
		t.Fatalf("expected the manual to teach, got: %s", all)
	}
	if player.RecipeMethod("potion") != "buy" {
		t.Fatalf("method %q", player.RecipeMethod("potion"))
	}
}
