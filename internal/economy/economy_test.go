package economy

import (
	"math/rand"
	"testing"

	"sudengine/internal/rpg"
	"sudengine/internal/world"
)

func TestDurabilityAndRepair(t *testing.T) {
	item := &world.Entity{
		ID:            "sword",
		Name:          "iron sword",
		Value:         20,
		Durability:    50,
		MaxDurability: 50,
	}

	if Condition(item.Durability, item.MaxDurability, false) != "pristine" {
		t.Fatalf("expected pristine condition")
	}

	// Degrade by 10 points
	broke := Degrade(item, 10)
	if broke {
		t.Fatalf("should not break after 10 points")
	}
	if item.Durability != 40 {
		t.Fatalf("expected durability 40, got %d", item.Durability)
	}
	if Condition(item.Durability, item.MaxDurability, false) != "good" {
		t.Fatalf("expected good condition")
	}

	// Degrade another 15 points (now 25/50 = 50%, worn)
	broke = Degrade(item, 15)
	if broke || Condition(item.Durability, item.MaxDurability, false) != "worn" {
		t.Fatalf("expected worn condition")
	}

	// Cost to repair: missing 25 of 50, value 20 -> (25 * 20) / (50 * 2) = 500 / 100 = 5 coins
	cost := RepairCost(item)
	if cost != 5 {
		t.Fatalf("expected repair cost 5, got %d", cost)
	}

	// Degrade until broken
	broke = Degrade(item, 35)
	if !broke || !item.HasFlag("broken") || item.Durability != 0 {
		t.Fatalf("expected item to be broken at 0 durability")
	}
	if Condition(item.Durability, item.MaxDurability, true) != "broken" {
		t.Fatalf("expected broken condition")
	}

	// Repair
	ok := Repair(item)
	if !ok || item.Durability != 50 || item.HasFlag("broken") {
		t.Fatalf("expected item to be repaired to 50 without broken flag")
	}
}

func TestDynamicMerchantPurse(t *testing.T) {
	vendor := &world.Entity{
		ID:   "blacksmith",
		Name: "Goran",
		Shop: &world.Shop{
			Currency: "copper",
			Purse:    50,
			Dynamic:  true,
		},
	}

	// Vendor can afford 30
	if !CanVendorAfford(vendor, 30) {
		t.Fatalf("vendor with 50 should afford 30")
	}

	// Vendor pays 30
	VendorPay(vendor, 30)
	if vendor.Shop.Purse != 20 {
		t.Fatalf("expected vendor purse 20, got %d", vendor.Shop.Purse)
	}

	// Vendor cannot afford 25
	if CanVendorAfford(vendor, 25) {
		t.Fatalf("vendor with 20 should not afford 25")
	}

	// Vendor receives 40
	VendorReceive(vendor, 40)
	if vendor.Shop.Purse != 60 {
		t.Fatalf("expected vendor purse 60, got %d", vendor.Shop.Purse)
	}
}

func TestCraftingPipeline(t *testing.T) {
	w := world.New()

	room := &world.Entity{
		ID:       "smithy",
		Kind:     world.KindRoom,
		Name:     "Blacksmith Shop",
		Contents: []world.ID{"anvil"},
	}
	anvil := &world.Entity{
		ID:       "anvil",
		Kind:     world.KindScenery,
		Keywords: []string{"anvil"},
		Name:     "a sturdy iron anvil",
		Parent:   room.ID,
	}

	player := &world.Entity{
		ID:       "player",
		Kind:     world.KindPlayer,
		Name:     "Crafter",
		Parent:   room.ID,
		Contents: []world.ID{"hammer_1", "ingot_1", "ingot_2"},
	}

	hammer := &world.Entity{
		ID:       "hammer_1",
		Kind:     world.KindItem,
		Keywords: []string{"hammer"},
		Name:     "a blacksmith hammer",
		Parent:   player.ID,
	}
	ingot1 := &world.Entity{
		ID:       "ingot_1",
		Kind:     world.KindItem,
		Keywords: []string{"iron", "ingot"},
		Name:     "an iron ingot",
		Parent:   player.ID,
	}
	ingot2 := &world.Entity{
		ID:       "ingot_2",
		Kind:     world.KindItem,
		Keywords: []string{"iron", "ingot"},
		Name:     "an iron ingot",
		Parent:   player.ID,
	}

	swordProto := &world.Entity{
		ID:            "hollow.sword",
		Kind:          world.KindItem,
		Keywords:      []string{"sword", "broadsword"},
		Name:          "an iron broadsword",
		Durability:    50,
		MaxDurability: 50,
	}

	w.Entities[room.ID] = room
	w.Entities[anvil.ID] = anvil
	w.Entities[player.ID] = player
	w.Entities[hammer.ID] = hammer
	w.Entities[ingot1.ID] = ingot1
	w.Entities[ingot2.ID] = ingot2
	w.Protos[swordProto.ID] = swordProto

	recipe := Recipe{
		ID:       "forge_sword",
		Name:     "iron broadsword",
		Station:  "anvil",
		Tools:    []string{"hammer"},
		Requires: []Ingredient{{Item: "ingot", Count: 2}},
		Produces: "hollow.sword",
	}

	rng := rand.New(rand.NewSource(1))
	crafted, err := Craft(w, player, recipe, rng, nil)
	if err != nil {
		t.Fatalf("crafting failed: %v", err)
	}
	if crafted == nil || crafted.Name != "an iron broadsword" {
		t.Fatalf("expected crafted sword, got %+v", crafted)
	}

	// Verify ingredients were consumed
	if w.Get("ingot_1") != nil || w.Get("ingot_2") != nil {
		t.Fatalf("expected iron ingots to be consumed")
	}

	// Verify tool remained
	if w.Get("hammer_1") == nil {
		t.Fatalf("hammer should not be consumed")
	}
}

func TestSellPriceOfBrokenItem(t *testing.T) {
	item := &world.Entity{
		Value: 20, Durability: 0, MaxDurability: 20,
		Flags: map[string]bool{"broken": true},
	}
	if SellPrice(item) != 0 {
		t.Fatalf("shattered item sold for %d", SellPrice(item))
	}
	item.Durability = 8
	delete(item.Flags, "broken")
	if SellPrice(item) != 8 {
		t.Fatalf("scaled price %d", SellPrice(item))
	}
	item.Flags["broken"] = true
	if SellPrice(item) != 2 {
		t.Fatalf("broken quarter %d", SellPrice(item))
	}
}

func TestPurseStaysFiniteAtZero(t *testing.T) {
	vendor := &world.Entity{
		Shop: &world.Shop{Currency: "coin", Purse: 5, Finite: true},
	}
	VendorPay(vendor, 5)
	if vendor.Shop.Purse != 0 || !HasFinitePurse(vendor) {
		t.Fatalf("purse %d finite %v", vendor.Shop.Purse, HasFinitePurse(vendor))
	}
	if CanVendorAfford(vendor, 1) {
		t.Fatal("empty till afforded a purchase")
	}
	VendorReceive(vendor, 4)
	if vendor.Shop.Purse != 4 {
		t.Fatalf("till did not refill, got %d", vendor.Shop.Purse)
	}
}

func TestCraftRejectsMissingPatternAndSkill(t *testing.T) {
	w, player := craftBench()
	recipe := Recipe{
		ID: "forge_sword", Name: "iron broadsword", Station: "anvil",
		Requires: []Ingredient{{Item: "ingot", Count: 2}},
		Produces: "missing.sword",
	}
	if _, err := Craft(w, player, recipe, rand.New(rand.NewSource(1)), nil); err == nil {
		t.Fatal("missing pattern should fail")
	}
	if w.Get("ingot_1") == nil || w.Get("ingot_2") == nil {
		t.Fatal("ingredients consumed for a missing pattern")
	}

	recipe.Produces = "hollow.sword"
	recipe.Skill = "smithing"
	if _, err := Craft(w, player, recipe, rand.New(rand.NewSource(1)), &rpg.Schema{}); err == nil {
		t.Fatal("unknown check should fail")
	}
	if w.Get("ingot_1") == nil || w.Get("ingot_2") == nil {
		t.Fatal("ingredients consumed when the check is missing")
	}

	rules := &rpg.Schema{Checks: map[string]rpg.Check{
		"smithing": {Roll: "1d6", Difficulty: 99},
	}}
	if _, err := Craft(w, player, recipe, rand.New(rand.NewSource(1)), rules); err == nil {
		t.Fatal("impossible check should fail")
	}
	left := 0
	if w.Get("ingot_1") != nil {
		left++
	}
	if w.Get("ingot_2") != nil {
		left++
	}
	if left != 1 {
		t.Fatalf("failed craft should ruin one ingot, %d remain", left)
	}
}

func TestMinSkillUsesTrainedRank(t *testing.T) {
	w, player := craftBench()
	player.Attrs = map[string]int{"body": 5}
	recipe := Recipe{
		ID: "forge_sword", Name: "iron broadsword", Station: "anvil",
		Skill: "smithing", MinSkill: 1,
		Requires: []Ingredient{{Item: "ingot", Count: 1}},
		Produces: "hollow.sword",
	}
	rules := &rpg.Schema{Checks: map[string]rpg.Check{
		"smithing": {Roll: "10", Difficulty: 10},
	}}
	if _, err := Craft(w, player, recipe, rand.New(rand.NewSource(1)), rules); err == nil {
		t.Fatal("untrained rank should fail min_skill")
	}
	if w.Get("ingot_1") == nil || w.Get("ingot_2") == nil {
		t.Fatal("min_skill failure consumed an ingredient")
	}
	player.Skills = map[string]int{"smithing": 1}
	crafted, err := Craft(w, player, recipe, rand.New(rand.NewSource(1)), rules)
	if err != nil {
		t.Fatal(err)
	}
	if crafted.HasFlag("superior") {
		t.Fatal("a roll that only meets the target is not superior")
	}
}

func TestCraftSkipsWornIngredients(t *testing.T) {
	w, player := craftBench()
	player.Equipment = map[string]world.ID{"wield": "ingot_1"}
	recipe := Recipe{
		ID: "forge_sword", Name: "iron broadsword", Station: "anvil",
		Requires: []Ingredient{{Item: "ingot", Count: 1}},
		Produces: "hollow.sword",
	}
	if _, err := Craft(w, player, recipe, rand.New(rand.NewSource(1)), nil); err != nil {
		t.Fatal(err)
	}
	if w.Get("ingot_1") == nil {
		t.Fatal("worn ingot was consumed")
	}
	if w.Get("ingot_2") != nil {
		t.Fatal("loose ingot should have been consumed")
	}
	if player.Equipment["wield"] != "ingot_1" {
		t.Fatal("wield slot cleared")
	}
}

func craftBench() (*world.World, *world.Entity) {
	w := world.New()
	room := &world.Entity{ID: "smithy", Kind: world.KindRoom, Contents: []world.ID{"anvil", "player"}}
	anvil := &world.Entity{ID: "anvil", Kind: world.KindScenery, Keywords: []string{"anvil"}, Parent: room.ID}
	player := &world.Entity{
		ID: "player", Kind: world.KindPlayer, Parent: room.ID,
		Contents: []world.ID{"ingot_1", "ingot_2"},
	}
	ingot1 := &world.Entity{ID: "ingot_1", Kind: world.KindItem, Keywords: []string{"ingot"}, Parent: player.ID}
	ingot2 := &world.Entity{ID: "ingot_2", Kind: world.KindItem, Keywords: []string{"ingot"}, Parent: player.ID}
	sword := &world.Entity{ID: "hollow.sword", Kind: world.KindItem, Name: "an iron broadsword"}
	for _, e := range []*world.Entity{room, anvil, player, ingot1, ingot2} {
		w.Entities[e.ID] = e
	}
	w.Protos[sword.ID] = sword
	return w, player
}
