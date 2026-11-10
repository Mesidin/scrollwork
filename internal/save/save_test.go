package save

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"sudengine/internal/world"
)

func TestUpdateConnectionPasswordsRewritesEveryBookmark(t *testing.T) {
	origRoot := Root
	Root = t.TempDir()
	defer func() { Root = origRoot }()

	entries := []Connection{
		{Addr: "127.0.0.1:4000", Account: "Ada", Password: "old", Character: "One", Title: "House"},
		{Addr: "10.0.0.8:4000", Account: "ada", Password: "old", Character: "Two", Title: "House"},
		{Addr: "127.0.0.1:4000", Account: "Bob", Password: "keep", Character: "Three", Title: "House"},
	}
	for _, entry := range entries {
		if err := SaveConnection(entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := UpdateConnectionPasswords("Ada", "new secret"); err != nil {
		t.Fatal(err)
	}
	list, err := LoadConnections()
	if err != nil {
		t.Fatal(err)
	}
	var ada, bob int
	for _, c := range list {
		switch strings.ToLower(c.Account) {
		case "ada":
			ada++
			if c.Password != "new secret" {
				t.Fatalf("bookmark %s kept %q", c.Character, c.Password)
			}
		case "bob":
			bob++
			if c.Password != "keep" {
				t.Fatalf("bob bookmark changed to %q", c.Password)
			}
		}
	}
	if ada != 2 || bob != 1 {
		t.Fatalf("expected 2 ada bookmarks and 1 bob, got %+v", list)
	}
}

func TestAccountsBcryptAndMultipleCharacters(t *testing.T) {
	origRoot := Root
	Root = t.TempDir()
	defer func() { Root = origRoot }()

	origCost := BcryptCost
	BcryptCost = 4
	defer func() { BcryptCost = origCost }()

	acc, err := CreateAccount("old-house", "AdaAccount", "secret-pass")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acc.PasswordHash == "secret-pass" || !strings.HasPrefix(acc.PasswordHash, "$2") {
		t.Fatalf("expected bcrypt hash, got %q", acc.PasswordHash)
	}

	// Duplicate account creation must fail
	if _, err := CreateAccount("old-house", "adaaccount", "other"); !errors.Is(err, ErrAccountExists) {
		t.Fatalf("expected ErrAccountExists, got %v", err)
	}

	// Wrong password fails
	if _, err := AuthenticateAccount("old-house", "AdaAccount", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	// Correct password succeeds
	loaded, err := AuthenticateAccount("old-house", "AdaAccount", "secret-pass")
	if err != nil || loaded.Username != "AdaAccount" {
		t.Fatalf("AuthenticateAccount failed: %v", err)
	}

	// One account can own multiple characters
	if err := AddCharacterToAccount("old-house", "AdaAccount", "Ada"); err != nil {
		t.Fatal(err)
	}
	if err := AddCharacterToAccount("old-house", "AdaAccount", "Beatrice"); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadAccount("old-house", "AdaAccount")
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.OwnsCharacter("Ada") || !loaded.OwnsCharacter("Beatrice") || len(loaded.Characters) != 2 {
		t.Fatalf("expected 2 characters on account, got %v", loaded.Characters)
	}
}

func TestCreateAccountExclusive(t *testing.T) {
	origRoot := Root
	Root = t.TempDir()
	defer func() { Root = origRoot }()

	origCost := BcryptCost
	BcryptCost = 4
	defer func() { BcryptCost = origCost }()

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	wins := make([]*Account, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			acc, err := CreateAccount("old-house", "Racer", "secret-pass")
			wins[i] = acc
			errs[i] = err
		}()
	}
	wg.Wait()

	ok := 0
	for i, err := range errs {
		if err == nil {
			ok++
			if wins[i] == nil || wins[i].PasswordHash == "" || len(wins[i].Characters) != 0 {
				t.Fatalf("winner %d has an incomplete account: %+v", i, wins[i])
			}
			continue
		}
		if !errors.Is(err, ErrAccountExists) {
			t.Fatalf("loser %d: %v", i, err)
		}
	}
	if ok != 1 {
		t.Fatalf("expected one creator to win, got %d", ok)
	}
	loaded, err := AuthenticateAccount("old-house", "Racer", "secret-pass")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Characters) != 0 || loaded.PasswordHash == "" {
		t.Fatalf("raced create rewrote the account: %+v", loaded)
	}
}

// Stage 5 Done Check 1: Two characters save and load without either file containing the other's inventory.
func TestStage5TwoCharactersSaveAndLoadWithoutCrossInventory(t *testing.T) {
	origRoot := Root
	Root = t.TempDir()
	defer func() { Root = origRoot }()

	w := world.New()
	w.PackID = "old-house"
	w.StartRoom = "house.porch"
	porch := &world.Entity{ID: "house.porch", Kind: world.KindRoom, Name: "Front Porch"}
	foyer := &world.Entity{ID: "house.foyer", Kind: world.KindRoom, Name: "Foyer"}
	w.Add(porch)
	w.Add(foyer)

	ada := &world.Entity{ID: "player.ada", Kind: world.KindPlayer, Name: "Ada", Short: "Ada"}
	bob := &world.Entity{ID: "player.bob", Kind: world.KindPlayer, Name: "Bob", Short: "Bob"}
	w.Add(ada)
	w.Add(bob)
	_ = w.Move(ada.ID, porch.ID)
	_ = w.Move(bob.ID, foyer.ID)

	adaSword := &world.Entity{ID: "item.ada_sword#1", Kind: world.KindItem, Name: "silver rapier", Short: "a silver rapier", Takeable: true}
	bobShield := &world.Entity{ID: "item.bob_shield#2", Kind: world.KindItem, Name: "oak buckler", Short: "an oak buckler", Takeable: true}
	roomBench := &world.Entity{ID: "item.bench#3", Kind: world.KindScenery, Name: "wooden bench", Short: "a wooden bench"}
	w.Add(adaSword)
	w.Add(bobShield)
	w.Add(roomBench)
	_ = w.Move(adaSword.ID, ada.ID)
	_ = w.Move(bobShield.ID, bob.ID)
	_ = w.Move(roomBench.ID, porch.ID)

	if err := WritePaired("old-house", w, ada, bob); err != nil {
		t.Fatalf("WritePaired: %v", err)
	}

	adaPath, _ := PlayerPath("old-house", "Ada")
	bobPath, _ := PlayerPath("old-house", "Bob")
	wPath := WorldPath("old-house")

	adaRaw, err := os.ReadFile(adaPath)
	if err != nil {
		t.Fatalf("read ada.json: %v", err)
	}
	bobRaw, err := os.ReadFile(bobPath)
	if err != nil {
		t.Fatalf("read bob.json: %v", err)
	}
	worldRaw, err := os.ReadFile(wPath)
	if err != nil {
		t.Fatalf("read world.json: %v", err)
	}

	// Verify Ada's file has Ada's sword and nothing of Bob's
	if !strings.Contains(string(adaRaw), "silver rapier") {
		t.Fatalf("ada.json missing silver rapier:\n%s", string(adaRaw))
	}
	if strings.Contains(string(adaRaw), "oak buckler") || strings.Contains(string(adaRaw), "player.bob") {
		t.Fatalf("ada.json leaked Bob's inventory:\n%s", string(adaRaw))
	}

	// Verify Bob's file has Bob's shield and nothing of Ada's
	if !strings.Contains(string(bobRaw), "oak buckler") {
		t.Fatalf("bob.json missing oak buckler:\n%s", string(bobRaw))
	}
	if strings.Contains(string(bobRaw), "silver rapier") || strings.Contains(string(bobRaw), "player.ada") {
		t.Fatalf("bob.json leaked Ada's inventory:\n%s", string(bobRaw))
	}

	// Verify world.json has rooms and scenery, and neither player nor their carried items
	for _, forbidden := range []string{"silver rapier", "oak buckler", "player.ada", "player.bob"} {
		if strings.Contains(string(worldRaw), forbidden) {
			t.Fatalf("world.json leaked player entity/item %q:\n%s", forbidden, string(worldRaw))
		}
	}
	if !strings.Contains(string(worldRaw), "wooden bench") {
		t.Fatalf("world.json missing room scenery:\n%s", string(worldRaw))
	}

	// Now load world.json and re-attach both characters
	loadedWorld, err := ReadWorld("old-house")
	if err != nil {
		t.Fatalf("ReadWorld: %v", err)
	}
	adaSave, err := ReadPlayer("old-house", "Ada")
	if err != nil {
		t.Fatalf("ReadPlayer Ada: %v", err)
	}
	bobSave, err := ReadPlayer("old-house", "Bob")
	if err != nil {
		t.Fatalf("ReadPlayer Bob: %v", err)
	}

	adaLoaded, err := AttachPlayer(loadedWorld, adaSave)
	if err != nil {
		t.Fatalf("AttachPlayer Ada: %v", err)
	}
	bobLoaded, err := AttachPlayer(loadedWorld, bobSave)
	if err != nil {
		t.Fatalf("AttachPlayer Bob: %v", err)
	}

	if adaLoaded.Parent != "house.porch" || bobLoaded.Parent != "house.foyer" {
		t.Fatalf("expected Ada in porch and Bob in foyer, got %s and %s", adaLoaded.Parent, bobLoaded.Parent)
	}
	adaItems := loadedWorld.Children(adaLoaded.ID)
	bobItems := loadedWorld.Children(bobLoaded.ID)
	if len(adaItems) != 1 || adaItems[0].Name != "silver rapier" {
		t.Fatalf("unexpected Ada inventory after load: %+v", adaItems)
	}
	if len(bobItems) != 1 || bobItems[0].Name != "oak buckler" {
		t.Fatalf("unexpected Bob inventory after load: %+v", bobItems)
	}
}

// Stage 5 Done Check 2: A failed second write leaves the previous pair in place.
func TestStage5FailedSecondWriteLeavesPreviousPairInPlace(t *testing.T) {
	origRoot := Root
	Root = t.TempDir()
	defer func() { Root = origRoot }()

	w := world.New()
	w.PackID = "old-house"
	w.StartRoom = "house.porch"
	w.Tick = 10
	porch := &world.Entity{ID: "house.porch", Kind: world.KindRoom, Name: "Front Porch"}
	w.Add(porch)

	ada := &world.Entity{ID: "player.ada", Kind: world.KindPlayer, Name: "Ada", Short: "Ada"}
	ada.SetRes("coin", world.Resource{Current: 25, Max: 100, Pile: true})
	w.Add(ada)
	_ = w.Move(ada.ID, porch.ID)

	// 1. Initial successful paired write (Ada has 25 coins, World tick is 10).
	if err := WritePaired("old-house", w, ada); err != nil {
		t.Fatalf("initial WritePaired failed: %v", err)
	}

	// 2. Mutate state (Ada has 999 coins, World tick is 99), then fail on the second file write.
	ada.SetRes("coin", world.Resource{Current: 999, Max: 100, Pile: true})
	w.Tick = 99

	err := WritePairedWithHook("old-house", w, func(index int, path string) error {
		if index == 1 {
			return errors.New("simulated disk failure on second write")
		}
		return nil
	}, ada)
	if err == nil {
		t.Fatalf("expected WritePairedWithHook to fail on second write")
	}

	// 3. Verify previous pair is still intact on disk (25 coins and tick 10).
	ps, err := ReadPlayer("old-house", "Ada")
	if err != nil {
		t.Fatalf("ReadPlayer after failed write: %v", err)
	}
	if got := ps.Player.Res("coin").Current; got != 25 {
		t.Fatalf("expected previous player coin=25 to remain in place, got %d", got)
	}
	wLoaded, err := ReadWorld("old-house")
	if err != nil {
		t.Fatalf("ReadWorld after failed write: %v", err)
	}
	if wLoaded.Tick != 10 {
		t.Fatalf("expected previous world tick=10 to remain in place, got %d", wLoaded.Tick)
	}
}

// Stage 5 Done Check 3: An old saves/<pack>/<slot>.json still loads in play.
func TestStage5OldSinglePlayerSlotStillLoads(t *testing.T) {
	origRoot := Root
	Root = t.TempDir()
	defer func() { Root = origRoot }()

	// Write a classic single-player slot snapshot
	singleW := world.New()
	singleW.PackID = "old-house"
	singleW.StartRoom = "house.porch"
	porch := &world.Entity{ID: "house.porch", Kind: world.KindRoom, Name: "Front Porch"}
	pl := &world.Entity{ID: "player", Kind: world.KindPlayer, Name: "SoloHero", Short: "SoloHero"}
	key := &world.Entity{ID: "house.key#1", Kind: world.KindItem, Name: "brass key", Short: "a brass key", Takeable: true}
	singleW.Add(porch)
	singleW.Add(pl)
	singleW.Add(key)
	singleW.PlayerID = pl.ID
	_ = singleW.Move(pl.ID, porch.ID)
	_ = singleW.Move(key.ID, pl.ID)

	if err := Write("old-house", "slot1", singleW); err != nil {
		t.Fatalf("Write slot1: %v", err)
	}

	// Also write multiplayer accounts, players, and world.json beside it
	multiW := world.New()
	multiW.PackID = "old-house"
	multiW.StartRoom = "house.porch"
	multiW.Add(porch.Clone())
	mpPlayer := &world.Entity{ID: "player.ada", Kind: world.KindPlayer, Name: "Ada"}
	multiW.Add(mpPlayer)
	_ = multiW.Move(mpPlayer.ID, porch.ID)
	if err := WritePaired("old-house", multiW, mpPlayer); err != nil {
		t.Fatalf("WritePaired: %v", err)
	}

	// List(packID) for single-player Continue menu must only list "slot1" (not "world", "accounts", or "players")
	slots, err := List("old-house")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(slots) != 1 || slots[0] != "slot1" {
		t.Fatalf("expected single-player slots=[slot1], got %v", slots)
	}

	// Read("old-house", "slot1") must restore SoloHero and their key
	restored, err := Read("old-house", "slot1")
	if err != nil {
		t.Fatalf("Read slot1: %v", err)
	}
	if restored.Player() == nil || restored.Player().Name != "SoloHero" {
		t.Fatalf("expected SoloHero in restored single-player slot, got %+v", restored.Player())
	}
	kids := restored.Children(restored.PlayerID)
	if len(kids) != 1 || kids[0].Name != "brass key" {
		t.Fatalf("expected brass key in SoloHero inventory, got %+v", kids)
	}
}

func TestWritePairedOnePlayerStillSavesEveryCharacter(t *testing.T) {
	origRoot := Root
	Root = t.TempDir()
	defer func() { Root = origRoot }()

	w, porch, ada, bob, coin := twoPlayersAndCoin()
	_ = w.Move(coin.ID, ada.ID)
	if err := WritePaired("old-house", w, bob); err != nil {
		t.Fatalf("WritePaired: %v", err)
	}
	adaSave, err := ReadPlayer("old-house", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	if !saveHasItem(adaSave, "coin") {
		t.Fatal("Ada's file was not written when only Bob was passed")
	}
	worldSaved, err := ReadWorld("old-house")
	if err != nil {
		t.Fatal(err)
	}
	if saveWorldHasItem(worldSaved, "coin") {
		t.Fatal("coin in Ada's inventory was also left in world.json")
	}

	_ = w.Move(coin.ID, porch.ID)
	if err := WritePaired("old-house", w, bob); err != nil {
		t.Fatal(err)
	}
	adaSave, err = ReadPlayer("old-house", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	if saveHasItem(adaSave, "coin") {
		t.Fatal("dropped coin stayed in Ada's file")
	}
	worldSaved, err = ReadWorld("old-house")
	if err != nil {
		t.Fatal(err)
	}
	if !saveWorldHasItem(worldSaved, "coin") {
		t.Fatal("dropped coin was missing from world.json")
	}
	bobSave, err := ReadPlayer("old-house", "Bob")
	if err != nil {
		t.Fatal(err)
	}
	if saveHasItem(bobSave, "coin") {
		t.Fatal("Ada's coin was written into Bob's file")
	}
}

func TestRecoverPairedRestoresWhenMarkerRemains(t *testing.T) {
	origRoot := Root
	Root = t.TempDir()
	defer func() { Root = origRoot }()

	w, _, ada, _, _ := twoPlayersAndCoin()
	ada.SetRes("coin", world.Resource{Current: 25, Max: 100, Pile: true})
	w.Tick = 10
	if err := WritePaired("old-house", w, ada); err != nil {
		t.Fatal(err)
	}
	adaPath, err := PlayerPath("old-house", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	oldAda, err := os.ReadFile(adaPath)
	if err != nil {
		t.Fatal(err)
	}
	oldWorld, err := os.ReadFile(WorldPath("old-house"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(adaPath+".bak", oldAda, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(WorldPath("old-house")+".bak", oldWorld, 0o644); err != nil {
		t.Fatal(err)
	}
	ada.SetRes("coin", world.Resource{Current: 999, Max: 100, Pile: true})
	w.Tick = 99
	if err := os.WriteFile(adaPath, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(WorldPath("old-house"), []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pairMarkerPath("old-house"), []byte(`{"fresh":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	ps, err := ReadPlayer("old-house", "Ada")
	if err != nil {
		t.Fatalf("ReadPlayer after recover: %v", err)
	}
	if got := ps.Player.Res("coin").Current; got != 25 {
		t.Fatalf("expected restored coin 25, got %d", got)
	}
	loaded, err := ReadWorld("old-house")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tick != 10 {
		t.Fatalf("expected restored tick 10, got %d", loaded.Tick)
	}
	if _, err := os.Stat(pairMarkerPath("old-house")); !os.IsNotExist(err) {
		t.Fatalf("marker still present: %v", err)
	}
}

func TestRecoverPairedKeepsLivePairWhenMarkerIsGone(t *testing.T) {
	origRoot := Root
	Root = t.TempDir()
	defer func() { Root = origRoot }()

	w, _, ada, _, _ := twoPlayersAndCoin()
	ada.SetRes("coin", world.Resource{Current: 25, Max: 100, Pile: true})
	if err := WritePaired("old-house", w, ada); err != nil {
		t.Fatal(err)
	}
	adaPath, err := PlayerPath("old-house", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	oldAda, err := os.ReadFile(adaPath)
	if err != nil {
		t.Fatal(err)
	}
	ada.SetRes("coin", world.Resource{Current: 999, Max: 100, Pile: true})
	if err := WritePaired("old-house", w, ada); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(adaPath+".bak", oldAda, 0o644); err != nil {
		t.Fatal(err)
	}
	ps, err := ReadPlayer("old-house", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	if got := ps.Player.Res("coin").Current; got != 999 {
		t.Fatalf("leftover bak replaced the live save, coin=%d", got)
	}
	if _, err := os.Stat(adaPath + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("leftover bak was kept: %v", err)
	}
}

func TestRestoreStagedLeavesBakWhenRestoreFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing", "world.json")
	bak := filepath.Join(dir, "world.json.bak")
	if err := os.WriteFile(bak, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := restoreStaged([]stagedFile{{
		target:    target,
		bak:       bak,
		hadBak:    true,
		published: true,
	}})
	if err == nil {
		t.Fatal("expected restore to fail when the target directory is missing")
	}
	body, readErr := os.ReadFile(bak)
	if readErr != nil {
		t.Fatalf("backup removed after a failed restore: %v", readErr)
	}
	if string(body) != "old" {
		t.Fatalf("backup changed to %q", body)
	}
}

func twoPlayersAndCoin() (*world.World, *world.Entity, *world.Entity, *world.Entity, *world.Entity) {
	w := world.New()
	w.PackID = "old-house"
	w.StartRoom = "house.porch"
	porch := &world.Entity{ID: "house.porch", Kind: world.KindRoom, Name: "Front Porch"}
	w.Add(porch)
	ada := &world.Entity{ID: "player.ada", Kind: world.KindPlayer, Name: "Ada", Short: "Ada"}
	bob := &world.Entity{ID: "player.bob", Kind: world.KindPlayer, Name: "Bob", Short: "Bob"}
	coin := &world.Entity{ID: "coin#1", Kind: world.KindItem, Name: "coin", Takeable: true}
	w.Add(ada)
	w.Add(bob)
	w.Add(coin)
	_ = w.Move(ada.ID, porch.ID)
	_ = w.Move(bob.ID, porch.ID)
	return w, porch, ada, bob, coin
}

func saveHasItem(ps *PlayerSave, name string) bool {
	if ps == nil {
		return false
	}
	for _, e := range ps.Entities {
		if e != nil && e.Name == name {
			return true
		}
	}
	return false
}

func saveWorldHasItem(w *world.World, name string) bool {
	if w == nil {
		return false
	}
	for _, e := range w.Entities {
		if e != nil && e.Name == name {
			return true
		}
	}
	return false
}
