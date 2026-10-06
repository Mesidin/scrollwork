package olc

import (
	"strings"
	"testing"

	"sudengine/internal/commands"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/rpg"
	"sudengine/internal/world"
)

func newTestOLCContext() (*commands.Context, *world.World, *pack.Pack, *[]string) {
	w := world.New()
	p := &pack.Pack{
		Meta: pack.Meta{
			ID:        "test-pack",
			Title:     "Test Pack",
			StartRoom: "room.square",
		},
		Lexicon: pack.Lexicon{Labels: map[string]string{"coin": "coin"}},
		RPG: rpg.Schema{
			PrimaryResource: "hp",
			Resources: []rpg.ResDef{
				{Key: "hp", Label: "Health", Max: 20},
				{Key: "coin", Label: "Coin", Pile: true},
			},
		},
	}

	room := &world.Entity{
		ID:        "room.square",
		Kind:      world.KindRoom,
		Name:      "Town Square",
		Short:     "the square",
		Long:      "A bustling square with paved stones.",
		HasCoords: true,
	}

	player := &world.Entity{
		ID:     "player",
		Kind:   world.KindPlayer,
		Name:   "Builder",
		Parent: room.ID,
	}

	bramProto := &world.Entity{
		ID:       "mob.bram",
		Kind:     world.KindMobile,
		Keywords: []string{"bram", "soldier"},
		Name:     "Bram the Guard",
		Short:    "a sturdy soldier",
	}

	wolfProto := &world.Entity{
		ID:       "mob.wolf",
		Kind:     world.KindMobile,
		Keywords: []string{"wolf"},
		Name:     "a grey wolf",
		Short:    "a snarling wolf",
	}

	swordProto := &world.Entity{
		ID:       "item.sword",
		Kind:     world.KindItem,
		Keywords: []string{"sword"},
		Name:     "a steel sword",
		Takeable: true,
	}

	chestProto := &world.Entity{
		ID:       "item.chest",
		Kind:     world.KindItem,
		Keywords: []string{"chest"},
		Name:     "an iron chest",
		Takeable: false,
	}

	bram := bramProto.Clone()
	bram.ID = "mob.bram#1"
	bram.PrototypeID = "mob.bram"
	bram.Parent = room.ID

	sword := swordProto.Clone()
	sword.ID = "item.sword#1"
	sword.PrototypeID = "item.sword"
	sword.Parent = player.ID

	room.Contents = []world.ID{player.ID, bram.ID}
	player.Contents = []world.ID{sword.ID}

	w.Entities[room.ID] = room
	w.Entities[player.ID] = player
	w.Entities[bram.ID] = bram
	w.Entities[sword.ID] = sword

	w.Protos[bramProto.ID] = bramProto
	w.Protos[wolfProto.ID] = wolfProto
	w.Protos[swordProto.ID] = swordProto
	w.Protos[chestProto.ID] = chestProto

	w.PlayerID = player.ID
	w.StartRoom = room.ID

	var logs []string
	ctx := &commands.Context{
		World: w,
		Actor: player,
		Pack:  p,
		Mode:  protocol.ModeBuild,
		Tell: func(ch protocol.Channel, text string) {
			logs = append(logs, text)
		},
	}

	return ctx, w, p, &logs
}

func parse(input string) commands.Parsed {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return commands.Parsed{}
	}
	verb := fields[0]
	rest := strings.TrimSpace(strings.TrimPrefix(input, verb))
	return commands.Parsed{
		Verb: verb,
		Args: fields[1:],
		Rest: rest,
	}
}

func TestMsetBuilder(t *testing.T) {
	c, w, _, _ := newTestOLCContext()

	// 1. Set AI on mob.bram
	Lookup("mset")(c, parse("mset bram ai sentinel"))
	bram := w.Get("mob.bram#1")
	if bram.AI == nil || bram.AI.Profile != "sentinel" {
		t.Fatalf("expected bram AI sentinel, got %v", bram.AI)
	}
	Lookup("mset")(c, parse("mset bram ai aggressive"))
	if bram.AI.Wander || bram.AI.Profile != "aggressive" {
		t.Fatalf("aggressive should stay put, got %+v", bram.AI)
	}
	Lookup("mset")(c, parse("mset bram ai aggressive on"))
	if !bram.AI.Wander {
		t.Fatal("explicit wander should roam")
	}
	Lookup("mset")(c, parse("mset bram ai sentinel"))

	// 2. Set service flags
	Lookup("mset")(c, parse("mset bram trainer on"))
	Lookup("mset")(c, parse("mset bram repairer on"))
	Lookup("mset")(c, parse("mset bram banker on"))
	if !bram.Trainer || !bram.Repairer || !bram.Banker {
		t.Fatalf("expected bram trainer, repairer, banker all true")
	}

	// 3. Set XP and Respawn
	Lookup("mset")(c, parse("mset bram xp 50"))
	Lookup("mset")(c, parse("mset bram respawn 15"))
	if bram.XP != 50 || bram.RespawnEvery != 15 {
		t.Fatalf("expected bram xp 50 and respawn 15, got %d, %d", bram.XP, bram.RespawnEvery)
	}

	// 4. Set Shop properties
	Lookup("mset")(c, parse("mset bram shop currency coin"))
	Lookup("mset")(c, parse("mset bram shop purse 100"))
	Lookup("mset")(c, parse("mset bram shop maxpurse 300"))
	Lookup("mset")(c, parse("mset bram shop dynamic on"))
	if bram.Shop == nil || bram.Shop.Currency != "coin" || bram.Shop.Purse != 100 || bram.Shop.MaxPurse != 300 || !bram.Shop.Dynamic || !bram.Shop.Finite {
		t.Fatalf("expected shop properly configured, got %+v", bram.Shop)
	}
	Lookup("mset")(c, parse("mset bram shop dynamic off"))
	if bram.Shop.Dynamic || !bram.Shop.Finite || bram.Shop.Purse != 100 {
		t.Fatalf("dynamic off should keep the purse finite, got %+v", bram.Shop)
	}

	// 5. Set Topic and Teach Recipe
	Lookup("mset")(c, parse("mset bram topic training Train hard every day."))
	Lookup("mset")(c, parse("mset bram teach training craft_sword"))
	if bram.Topics["training"] != "Train hard every day." {
		t.Fatalf("expected topic training, got %q", bram.Topics["training"])
	}
	if bram.TeachRecipes["training"] != "craft_sword" {
		t.Fatalf("expected teach recipe craft_sword, got %q", bram.TeachRecipes["training"])
	}

	// 6. Set Loot drops
	Lookup("mset")(c, parse("mset bram loot add item.sword 80 1"))
	Lookup("mset")(c, parse("mset bram loot add item.pelt 100 2 skin"))
	if len(bram.Loot) != 2 {
		t.Fatalf("expected 2 loot drops, got %d", len(bram.Loot))
	}
	if bram.Loot[0].Item != "item.sword" || bram.Loot[0].Chance != 80 || bram.Loot[0].SkinOnly {
		t.Fatalf("unexpected loot 0: %+v", bram.Loot[0])
	}
	if bram.Loot[1].Item != "item.pelt" || bram.Loot[1].Count != 2 || !bram.Loot[1].SkinOnly {
		t.Fatalf("unexpected loot 1: %+v", bram.Loot[1])
	}

	// 7. Verify prototype sync
	proto := w.Protos["mob.bram"]
	if proto == nil || !proto.Trainer || !proto.Repairer || !proto.Banker || proto.Shop == nil || len(proto.Loot) != 2 {
		t.Fatalf("prototype mob.bram not properly synchronized: %+v", proto)
	}
}

func TestIsetBuilder(t *testing.T) {
	c, w, _, _ := newTestOLCContext()

	// 1. Set armor and slot
	Lookup("iset")(c, parse("iset sword armor 3"))
	Lookup("iset")(c, parse("iset sword slot wield"))
	sword := w.Get("item.sword#1")
	if sword.Armor == nil || sword.Armor.Soak != 3 || sword.Slot != "wield" || !sword.Wearable {
		t.Fatalf("expected sword armor soak 3 and slot wield, got %+v", sword)
	}

	// 2. Set durability and repairable
	Lookup("iset")(c, parse("iset sword durability 50"))
	if sword.Durability != 50 || sword.MaxDurability != 50 || !sword.Repairable {
		t.Fatalf("expected sword durability 50/50 repairable true, got %d/%d %v", sword.Durability, sword.MaxDurability, sword.Repairable)
	}

	// 3. Set banker on chest
	Lookup("iset")(c, parse("iset chest banker on"))
	protoChest := w.Protos["item.chest"]
	if protoChest == nil || !protoChest.Banker {
		t.Fatalf("expected chest prototype banker true")
	}

	// 4. Set field repair kit
	Lookup("iset")(c, parse("iset sword kit 4 12 crafting 6"))
	if sword.RepairKit == nil || sword.RepairKit.Uses != 4 || sword.RepairKit.Amount != 12 || sword.RepairKit.Skill != "crafting" || sword.RepairKit.DC != 6 {
		t.Fatalf("expected repair kit configured, got %+v", sword.RepairKit)
	}

	// 5. Set salvage yields
	Lookup("iset")(c, parse("iset sword salvage add item.scrap 2"))
	Lookup("iset")(c, parse("iset sword salvage add item.wood 1"))
	if len(sword.Salvage) != 2 || sword.Salvage[0].Item != "item.scrap" || sword.Salvage[0].Count != 2 {
		t.Fatalf("expected salvage yields on sword, got %+v", sword.Salvage)
	}

	// 6. Verify prototype sync
	protoSword := w.Protos["item.sword"]
	if protoSword == nil || protoSword.Durability != 50 || protoSword.RepairKit == nil || len(protoSword.Salvage) != 2 {
		t.Fatalf("expected sword prototype synced, got %+v", protoSword)
	}
	if sword.RepairKit == protoSword.RepairKit || &sword.Salvage[0] == &protoSword.Salvage[0] {
		t.Fatal("instance shares kit or salvage storage with the prototype")
	}
}

func TestGsetBuilder(t *testing.T) {
	c, w, _, _ := newTestOLCContext()

	// 1. Add extra scenery
	Lookup("extra")(c, parse("extra add brambles | Thick thorny brambles."))
	var extra *world.Entity
	for _, kid := range w.Children("room.square") {
		if kid.HasKeyword("brambles") {
			extra = kid
			break
		}
	}
	if extra == nil {
		t.Fatalf("expected brambles extra scenery")
	}

	// 2. Set gather node on extra
	Lookup("gset")(c, parse("gset node brambles item hollow.branch max 4 regen 30 skill notice dc 6 bonus 9"))
	if extra.Gather == nil || extra.Gather.Resource != "hollow.branch" || extra.Gather.Max != 4 || extra.Gather.Difficulty != 6 {
		t.Fatalf("expected gather node on extra, got %+v", extra.Gather)
	}

	// 3. Set gather node on room
	Lookup("gset")(c, parse("gset room item hollow.stone max 6 regen 50 skill notice dc 5 bonus 8"))
	room := w.Get("room.square")
	if room.Gather == nil || room.Gather.Resource != "hollow.stone" || room.Gather.Max != 6 {
		t.Fatalf("expected gather node on room, got %+v", room.Gather)
	}

	// 4. Clear gather node
	Lookup("gset")(c, parse("gset clear room"))
	if room.Gather != nil {
		t.Fatalf("expected room gather node to be cleared")
	}

	Lookup("gset")(c, parse("gset room item hollow.branch 5 30 8"))
	if room.Gather == nil || room.Gather.Max != 5 || room.Gather.RegenEvery != 30 || room.Gather.Difficulty != 8 || room.Gather.BonusDC != 9 {
		t.Fatalf("positional numbers should be max, regen, dc, bonus: %+v", room.Gather)
	}
}

func TestRecipeBuilder(t *testing.T) {
	c, _, p, _ := newTestOLCContext()

	// 1. Add recipe
	Lookup("recipe")(c, parse("recipe add craft_dagger item.dagger name \"steel dagger\" station anvil skill smithing dc 7 min 1"))
	if len(p.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(p.Recipes))
	}
	r := p.Recipes[0]
	if r.ID != "craft_dagger" || r.Produces != "item.dagger" || r.Name != "steel dagger" || r.Station != "anvil" || r.Skill != "smithing" || r.Difficulty != 7 || r.MinSkill != 1 {
		t.Fatalf("unexpected recipe fields: %+v", r)
	}

	// 2. Add ingredients
	Lookup("recipe")(c, parse("recipe req craft_dagger item.iron 2"))
	Lookup("recipe")(c, parse("recipe req craft_dagger item.wood 1"))
	if len(p.Recipes[0].Requires) != 2 {
		t.Fatalf("expected 2 ingredients, got %d", len(p.Recipes[0].Requires))
	}
	if p.Recipes[0].Requires[0].Item != "item.iron" || p.Recipes[0].Requires[0].Count != 2 {
		t.Fatalf("unexpected requirement 0: %+v", p.Recipes[0].Requires[0])
	}

	Lookup("recipe")(c, parse("recipe add craft_dagger item.spear"))
	r = p.Recipes[0]
	if r.Produces != "item.spear" || r.Station != "anvil" || r.Name != "steel dagger" || r.Skill != "smithing" || r.MinSkill != 1 {
		t.Fatalf("update should keep omitted fields, got %+v", r)
	}
	locked := false
	p.Recipes[0].Discovered = &locked
	Lookup("recipe")(c, parse("recipe add craft_dagger item.spear disc grant"))
	if p.Recipes[0].Discovered == nil || *p.Recipes[0].Discovered || p.Recipes[0].Produces != "item.spear" {
		t.Fatalf("disc grant should not change the recipe, got %+v", p.Recipes[0])
	}
	Lookup("recipe")(c, parse("recipe add craft_dagger item.dagger disc off"))
	if p.Recipes[0].Discovered == nil || *p.Recipes[0].Discovered || p.Recipes[0].Produces != "item.dagger" || p.Recipes[0].Station != "anvil" {
		t.Fatalf("disc off should lock the recipe and update the product, got %+v", p.Recipes[0])
	}

	// 3. Delete recipe
	Lookup("recipe")(c, parse("recipe del craft_dagger"))
	if len(p.Recipes) != 0 {
		t.Fatalf("expected 0 recipes after delete, got %d", len(p.Recipes))
	}
}

func TestOLCSavePackRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c, w, p, _ := newTestOLCContext()
	p.Dir = dir

	c.SavePack = func() error {
		p.SyncFromWorld(w)
		return p.WriteWorld()
	}

	// Apply OLC modifications
	Lookup("mset")(c, parse("mset bram trainer on"))
	Lookup("mset")(c, parse("mset bram shop currency coin"))
	Lookup("mset")(c, parse("mset bram shop purse 75"))
	Lookup("mset")(c, parse("mset bram topic guards \"We keep the peace.\""))
	Lookup("mset")(c, parse("mset bram loot add item.sword 100 1"))
	Lookup("iset")(c, parse("iset sword durability 35"))
	Lookup("iset")(c, parse("iset sword kit 3 15 crafting 7"))
	Lookup("iset")(c, parse("iset sword salvage add item.scrap 2"))
	Lookup("extra")(c, parse("extra add well | An old stone well."))
	Lookup("gset")(c, parse("gset node well item item.water max 5 regen 20 skill notice dc 5 bonus 8"))
	Lookup("recipe")(c, parse("recipe add craft_torch item.torch name \"pitch torch\" station workbench skill crafting dc 6"))
	Lookup("recipe")(c, parse("recipe req craft_torch item.wood 1"))

	// Save pack
	Lookup("savepack")(c, parse("savepack"))

	// Reload pack from disk
	loaded, err := pack.Load(dir)
	if err != nil {
		t.Fatalf("failed to load saved pack: %v", err)
	}

	// Verify NPCs
	if len(loaded.NPCs) != 2 {
		t.Fatalf("expected 2 NPCs, got %d", len(loaded.NPCs))
	}
	var loadedBram *pack.NPCYAML
	for i := range loaded.NPCs {
		if loaded.NPCs[i].ID == "mob.bram" {
			loadedBram = &loaded.NPCs[i]
			break
		}
	}
	if loadedBram == nil || !loadedBram.Trainer || loadedBram.Shop == nil || loadedBram.Shop.Purse != 75 || len(loadedBram.Loot) != 1 {
		t.Fatalf("loaded bram not properly persisted: %+v", loadedBram)
	}

	// Verify Items
	var loadedSword *pack.ItemYAML
	for i := range loaded.Items {
		if loaded.Items[i].ID == "item.sword" {
			loadedSword = &loaded.Items[i]
			break
		}
	}
	if loadedSword == nil || loadedSword.Durability != 35 || loadedSword.RepairKit == nil || len(loadedSword.Salvage) != 1 {
		t.Fatalf("loaded sword not properly persisted: %+v", loadedSword)
	}

	// Verify Recipes
	if len(loaded.Recipes) != 1 || loaded.Recipes[0].ID != "craft_torch" || len(loaded.Recipes[0].Requires) != 1 {
		t.Fatalf("loaded recipes not properly persisted: %+v", loaded.Recipes)
	}

	// Verify Rooms and Extras Gather Node
	if len(loaded.Rooms) != 1 || len(loaded.Rooms[0].Extras) != 1 {
		t.Fatalf("expected 1 room with 1 extra, got %+v", loaded.Rooms)
	}
	ex := loaded.Rooms[0].Extras[0]
	if ex.Gather == nil || ex.Gather.Resource != "item.water" || ex.Gather.Max != 5 {
		t.Fatalf("loaded extra gather node not properly persisted: %+v", ex.Gather)
	}
}
