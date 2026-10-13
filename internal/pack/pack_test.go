package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sudengine/internal/quest"
	"sudengine/internal/world"
)

func TestLoadOldHouse(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Meta.ID != "old-house" {
		t.Fatalf("id %s", p.Meta.ID)
	}
	w, err := p.Instantiate()
	if err != nil {
		t.Fatal(err)
	}
	if w.Get("house.foyer") == nil {
		t.Fatal("missing foyer")
	}
	SpawnPlayer(w, p, "you", "", "")
	if w.Player() == nil {
		t.Fatal("no player")
	}
	if w.RoomOf(w.Player()).ID != "house.porch" {
		t.Fatalf("start %s", w.RoomOf(w.Player()).ID)
	}
	if p.IntroArt == "" {
		t.Fatal("expected intro art")
	}
	if !p.RPG.ChargenEnabled() {
		t.Fatal("chargen should be on")
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCatchesBadExit(t *testing.T) {
	p := Blank("t", t.TempDir())
	p.Rooms[0].Exits = map[string]ExitYAML{"north": {To: "nowhere"}}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nowhere") {
		t.Fatal(err)
	}
}

func TestShopStockAllowsNPC(t *testing.T) {
	p := Blank("t", t.TempDir())
	p.Items = []ItemYAML{{ID: "lead", Name: "a lead"}}
	p.NPCs = []NPCYAML{
		{
			ID:   "trainer",
			Name: "trainer",
			Shop: &world.Shop{Currency: "hp", Stock: []string{"puppy", "lead", "missing"}},
		},
		{ID: "puppy", Name: "puppy"},
	}
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), "puppy") || strings.Contains(err.Error(), "lead") {
		t.Fatalf("known stock was rejected: %v", err)
	}
	p.NPCs[0].Shop.Stock = []string{"puppy", "lead"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRecipesLoadAndRepairableDefaultsOff(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Recipes) != 3 {
		t.Fatalf("recipes %d", len(p.Recipes))
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	e := itemFromYAML(ItemYAML{ID: "x", Name: "x", Durability: 3, MaxDurability: 3})
	if e.Repairable {
		t.Fatal("repairable should default off")
	}

	wrapped := t.TempDir()
	if err := os.WriteFile(filepath.Join(wrapped, "pack.yaml"), []byte("id: wrap\ntitle: Wrap\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "recipes:\n  - id: cup\n    name: clay cup\n    produces:\n      item: item.cup\n      count: 2\n    skill: pottery\n"
	if err := os.WriteFile(filepath.Join(wrapped, "recipes.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	wp, err := Load(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if len(wp.Recipes) != 1 || wp.Recipes[0].Produces != "item.cup" || wp.Recipes[0].Count != 2 {
		t.Fatalf("%+v", wp.Recipes)
	}
	verr := wp.Validate()
	if verr == nil || !strings.Contains(verr.Error(), "item.cup") || !strings.Contains(verr.Error(), "pottery") {
		t.Fatal(verr)
	}

	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "pack.yaml"), []byte("id: bad\ntitle: Bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "recipes.yaml"), []byte("recipes: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Fatal("broken recipes.yaml should fail load")
	}
}

func TestInstallDir(t *testing.T) {
	src := filepath.Join("..", "..", "games", "old-house")
	destRoot := t.TempDir()
	info, err := Install(src, destRoot)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "old-house" {
		t.Fatal(info.ID)
	}
	if _, err := Load(info.Dir); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyRecipeFileIsCleared(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "recipes.yaml"), []byte("- id: old\n  name: old\n  produces: item.old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &Pack{Dir: dir, Meta: Meta{ID: "t", Title: "T", StartRoom: "room"}}
	if err := p.WriteWorld(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "recipes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("deleted recipes should clear the file, got %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "quests.yaml")); !os.IsNotExist(err) {
		t.Fatal("quests.yaml should not be created for a pack that never had one")
	}
}

func TestValidateUnreachableRooms(t *testing.T) {
	p := Blank("t", t.TempDir())
	p.Rooms = append(p.Rooms, RoomYAML{
		ID:   "t.island",
		Name: "Island Room",
	})
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "room t.island: unreachable from start_room") {
		t.Fatalf("expected unreachable error, got: %v", err)
	}

	// Exit connects start to island
	p.Rooms[0].Exits = map[string]ExitYAML{"east": {To: "t.island"}}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected reachable via exit, got: %v", err)
	}

	// Island is unreachable via exits, but reached via quest relocate
	p.Rooms[0].Exits = nil
	p.Quests = []quest.Quest{
		{
			ID:    "q1",
			Title: "Travel",
			Rewards: quest.Rewards{
				Relocate: []quest.Relocation{
					{Mob: "puppy", To: "t.island"},
				},
			},
		},
	}
	p.NPCs = []NPCYAML{{ID: "puppy", Name: "puppy"}}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected reachable via quest relocate, got: %v", err)
	}

	// A longer id that merely contains the room id is not an entrance.
	p.Quests = nil
	p.NPCs = nil
	p.Scripts = map[string]string{
		"portal.lua": `teleport(player, "t.island_west")`,
	}
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), "room t.island: unreachable from start_room") {
		t.Fatalf("expected substring script id to stay unreachable, got: %v", err)
	}

	// Island is reachable via script mention
	p.Scripts = map[string]string{
		"portal.lua": `teleport(player, "t.island")`,
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected reachable via script mention, got: %v", err)
	}
}

func TestValidateScriptReferencesAndSyntax(t *testing.T) {
	p := Blank("t", t.TempDir())
	p.Rooms[0].Scripts = "missing_room.lua"
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), `room t.start: script "missing_room.lua" not found`) {
		t.Fatalf("expected missing script error on room, got: %v", err)
	}
	p.Rooms[0].Scripts = ""

	p.Rooms[0].Extras = []ExtraYAML{
		{Name: "statue", Scripts: "missing_extra.lua"},
	}
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), `room t.start extra statue: script "missing_extra.lua" not found`) {
		t.Fatalf("expected missing script error on extra, got: %v", err)
	}
	p.Rooms[0].Extras = nil

	p.Items = []ItemYAML{
		{ID: "scroll", Name: "scroll", Scripts: "missing_item.lua"},
	}
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), `item scroll: script "missing_item.lua" not found`) {
		t.Fatalf("expected missing script error on item, got: %v", err)
	}
	p.Items = nil

	p.NPCs = []NPCYAML{
		{ID: "guard", Name: "guard", Scripts: "missing_npc.lua"},
	}
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), `npc guard: script "missing_npc.lua" not found`) {
		t.Fatalf("expected missing script error on npc, got: %v", err)
	}
	p.NPCs = nil

	p.Quests = []quest.Quest{
		{ID: "q1", Title: "Quest", Script: "missing_quest.lua"},
	}
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), `quest q1: script "missing_quest.lua" not found`) {
		t.Fatalf("expected missing script error on quest, got: %v", err)
	}
	p.Quests = nil

	// Lua syntax error
	p.Scripts = map[string]string{
		"broken.lua": "function on_use(actor, self\n", // missing closing paren and end
	}
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), "script broken.lua: syntax error") {
		t.Fatalf("expected Lua syntax error, got: %v", err)
	}

	p.Scripts["broken.lua"] = "function on_use(actor, self) end\n"
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid Lua syntax, got: %v", err)
	}

	// A different body under the unsuffixed name is still parsed.
	p.Scripts = map[string]string{
		"broken":     "function on_use(actor, self\n",
		"broken.lua": "function on_use(actor, self) end\n",
	}
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), "script broken: syntax error") {
		t.Fatalf("expected syntax error in the unsuffixed body, got: %v", err)
	}
}

func TestValidateKeysAndLocks(t *testing.T) {
	p := Blank("t", t.TempDir())
	p.Rooms = append(p.Rooms, RoomYAML{ID: "t.vault", Name: "Vault"})
	p.Rooms[0].Exits = map[string]ExitYAML{"north": {To: "t.vault", Key: "vault.key"}}
	p.Rooms[1].Exits = map[string]ExitYAML{"south": {To: "t.start", Key: "other.key"}}

	// 1. Exit key not in items
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), `key "vault.key" is not an item`) {
		t.Fatalf("expected key not an item, got: %v", err)
	}

	p.Items = []ItemYAML{
		{ID: "vault.key", Name: "iron key", Keywords: []string{"key"}},
		{ID: "other.key", Name: "brass key", Keywords: []string{"key"}},
	}
	// 2. Conflicting door keys on reciprocal exits
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), "conflicting keys") {
		t.Fatalf("expected conflicting keys on door, got: %v", err)
	}

	// Fix reciprocal exit key
	p.Rooms[1].Exits["south"] = ExitYAML{To: "t.start", Key: "vault.key"}

	// 3. Key required by lock is not placed in the world
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), `key "vault.key" required by lock is not placed in the world`) {
		t.Fatalf("expected unplaced key error, got: %v", err)
	}
	// Also other.key is an orphaned key item
	if !strings.Contains(err.Error(), "item other.key: orphaned key does not unlock any door or container") {
		t.Fatalf("expected orphaned key error on other.key, got: %v", err)
	}

	// Place vault.key in the start room, and remove other.key
	p.Rooms[0].Items = []string{"vault.key"}
	p.Items = []ItemYAML{{ID: "vault.key", Name: "iron key", Keywords: []string{"key"}}}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid key setup, got: %v", err)
	}

	// 4. Container lock with missing key item
	p.Items = append(p.Items, ItemYAML{
		ID:        "chest",
		Name:      "chest",
		Container: true,
		Locked:    true,
		Key:       "nonexistent.key",
	})
	p.Rooms[0].Items = append(p.Rooms[0].Items, "chest")
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), `item chest: key "nonexistent.key" is not an item`) {
		t.Fatalf("expected container missing key item error, got: %v", err)
	}
}

func TestValidateKeyPlacementAndDoorPairs(t *testing.T) {
	p := Blank("t", t.TempDir())
	p.Rooms = append(p.Rooms, RoomYAML{ID: "t.vault", Name: "Vault"})
	p.Rooms[0].Exits = map[string]ExitYAML{
		"north": {To: "t.vault", Key: "vault.key"},
		"up":    {To: "t.vault", Key: "hatch.key"},
	}
	p.Rooms[1].Exits = map[string]ExitYAML{
		"south": {To: "t.start", Key: "vault.key"},
		"down":  {To: "t.start", Key: "hatch.key"},
	}
	p.Items = []ItemYAML{
		{ID: "vault.key", Name: "iron key"},
		{ID: "hatch.key", Name: "hatch key"},
		{ID: "lantern", Name: "lantern", Keywords: []string{"key"}, Flags: []string{"key"}},
	}
	p.Rooms[0].Items = []string{"vault.key", "hatch.key", "lantern"}
	if err := p.Validate(); err != nil {
		t.Fatalf("two consistent doors should pass, got: %v", err)
	}

	p.Rooms[1].Exits["down"] = ExitYAML{To: "t.start", Key: "vault.key"}
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), "conflicting keys") {
		t.Fatalf("expected the up/down pair to conflict, got: %v", err)
	}

	p.Rooms[0].Exits = map[string]ExitYAML{"north": {To: "t.vault", Key: "vault.key"}}
	p.Rooms[1].Exits = map[string]ExitYAML{"south": {To: "t.start", Key: "vault.key"}}
	p.Items = []ItemYAML{
		{ID: "vault.key", Name: "iron key"},
		{ID: "chest", Name: "chest", Contains: []string{"vault.key"}},
	}
	p.Rooms[0].Items = nil
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), `key "vault.key" required by lock is not placed in the world`) {
		t.Fatalf("unplaced chest should not place the key, got: %v", err)
	}

	p.Rooms[0].Items = []string{"chest"}
	if err := p.Validate(); err != nil {
		t.Fatalf("key inside a placed chest should count, got: %v", err)
	}

	p.Items[1].Key = "vault.key"
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), `key "vault.key" required by lock is not placed in the world`) {
		t.Fatalf("key locked inside its own chest should not count, got: %v", err)
	}

	p.Items = []ItemYAML{{ID: "vault.key", Name: "iron key"}}
	p.Rooms[0].Items = nil
	p.NPCs = []NPCYAML{{
		ID: "smith", Name: "smith",
		Shop: &world.Shop{Currency: "hp", Stock: []string{"vault.key"}},
	}}
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), `key "vault.key" required by lock is not placed in the world`) {
		t.Fatalf("unplaced shop should not place the key, got: %v", err)
	}
	p.Rooms[0].NPCs = []string{"smith"}
	if err := p.Validate(); err != nil {
		t.Fatalf("placed shop stock should count, got: %v", err)
	}

	p.Rooms[0].NPCs = nil
	p.NPCs = nil
	p.Scripts = map[string]string{"give.lua": `give(actor, "vault.keyring")`}
	err = p.Validate()
	if err == nil || !strings.Contains(err.Error(), `key "vault.key" required by lock is not placed in the world`) {
		t.Fatalf("longer script id should not place the key, got: %v", err)
	}
	p.Scripts["give.lua"] = `give(actor, "vault.key")`
	if err := p.Validate(); err != nil {
		t.Fatalf("whole script id should place the key, got: %v", err)
	}
}

func TestYAMLCommentPreservation(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	if err := os.MkdirAll(worldDir, 0o755); err != nil {
		t.Fatal(err)
	}

	initialRoomsYAML := `# World rooms for test pack
# Note: do not delete the porch

- id: test.porch # Starting location
  name: Front Porch
  # Peeling paint description
  long: Weathered boards.
  exits:
    north:
      to: test.foyer # Leads into foyer

# The foyer connects to other areas
- id: test.foyer
  name: Foyer
  long: A narrow hall.
`
	roomsPath := filepath.Join(worldDir, "rooms.yaml")
	if err := os.WriteFile(roomsPath, []byte(initialRoomsYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Blank("test", dir)
	p.Rooms = []RoomYAML{
		{
			ID:   "test.porch",
			Name: "Grand Porch", // updated name
			Long: "Weathered boards.",
			Exits: map[string]ExitYAML{
				"north": {To: "test.foyer"},
			},
		},
		{
			ID:   "test.foyer",
			Name: "Foyer",
			Long: "A narrow hall.",
		},
	}

	if err := p.WriteWorld(); err != nil {
		t.Fatalf("WriteWorld failed: %v", err)
	}

	saved, err := os.ReadFile(roomsPath)
	if err != nil {
		t.Fatal(err)
	}
	savedStr := string(saved)

	if !strings.Contains(savedStr, "name: Grand Porch") {
		t.Fatalf("expected updated name 'Grand Porch', got:\n%s", savedStr)
	}

	for _, expectedComment := range []string{
		"# World rooms for test pack",
		"# Note: do not delete the porch",
		"# Starting location",
		"# Peeling paint description",
		"# Leads into foyer",
		"# The foyer connects to other areas",
	} {
		if !strings.Contains(savedStr, expectedComment) {
			t.Errorf("missing comment %q in saved rooms.yaml:\n%s", expectedComment, savedStr)
		}
	}
}

func TestYAMLCommentPreservationMapping(t *testing.T) {
	dir := t.TempDir()
	initialPackYAML := `# Pack metadata configuration
id: test # Primary pack identifier
title: Test Pack # Human readable title
tick_ms: 250
combat_every: 4
start_room: test.start # Entry room
`
	packPath := filepath.Join(dir, "pack.yaml")
	if err := os.WriteFile(packPath, []byte(initialPackYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Blank("test", dir)
	p.Meta.Title = "Updated Test Pack"
	if err := p.WriteWorld(); err != nil {
		t.Fatalf("WriteWorld failed: %v", err)
	}

	saved, err := os.ReadFile(packPath)
	if err != nil {
		t.Fatal(err)
	}
	savedStr := string(saved)

	if !strings.Contains(savedStr, "title: Updated Test Pack") {
		t.Fatalf("expected updated title, got:\n%s", savedStr)
	}

	for _, expectedComment := range []string{
		"# Pack metadata configuration",
		"# Primary pack identifier",
		"# Human readable title",
		"# Entry room",
	} {
		if !strings.Contains(savedStr, expectedComment) {
			t.Errorf("missing comment %q in saved pack.yaml:\n%s", expectedComment, savedStr)
		}
	}
}

func TestYAMLCommentMatchesIDBeforeName(t *testing.T) {
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "world")
	if err := os.MkdirAll(worldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	initial := `notes:
  - id: noise # wrong list
rooms:
  - name: Front Porch
    id: test.porch # keep me
    long: Weathered boards.
  - name: Front Porch
    id: test.porch # second porch
    long: Weathered boards.
`
	roomsPath := filepath.Join(worldDir, "rooms.yaml")
	if err := os.WriteFile(roomsPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Blank("test", dir)
	p.Rooms = []RoomYAML{
		{ID: "test.porch", Name: "Grand Porch", Long: "Weathered boards."},
		{ID: "test.porch", Name: "Side Porch", Long: "Weathered boards."},
	}
	if err := p.WriteWorld(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(roomsPath)
	if err != nil {
		t.Fatal(err)
	}
	savedStr := string(saved)
	if strings.Contains(savedStr, "wrong list") {
		t.Fatalf("notes list donated its comment:\n%s", savedStr)
	}
	for _, comment := range []string{"# keep me", "# second porch"} {
		if !strings.Contains(savedStr, comment) {
			t.Errorf("missing %q in:\n%s", comment, savedStr)
		}
	}
	if !strings.Contains(savedStr, "name: Grand Porch") || !strings.Contains(savedStr, "name: Side Porch") {
		t.Fatalf("names were not updated:\n%s", savedStr)
	}
}

func TestWriteYAMLReadErrorDoesNotReplace(t *testing.T) {
	dir := t.TempDir()
	err := writeYAML(dir, map[string]string{"a": "b"})
	if err == nil {
		t.Fatal("expected a read error for a directory")
	}
	info, statErr := os.Stat(dir)
	if statErr != nil || !info.IsDir() {
		t.Fatalf("directory was replaced: %v", statErr)
	}
}
