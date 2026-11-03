package server

import (
	"os"
	"path/filepath"
	"testing"

	"sudengine/internal/protocol"
	"sudengine/internal/save"
	"sudengine/internal/world"
)

func TestClientShutdownReasonStartsGrace(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, _, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	bob, ent := srv.SpawnPlayerSession("Bob", "", "")
	ent.Combat = &world.Combat{Target: "house.shadow"}
	srv.disconnect(bob, "shutdown")
	if srv.World.Get(ent.ID) == nil {
		t.Fatal("client shutdown removed Bob")
	}
	if !ent.HasFlag("linkdead") || !ent.InCombat() {
		t.Fatalf("expected link-dead combat, linkdead=%v combat=%v", ent.HasFlag("linkdead"), ent.InCombat())
	}
	if _, ok := srv.Linkdead[ent.ID]; !ok {
		t.Fatal("client shutdown did not start grace")
	}
}

func TestProcessShutdownDoesNotLinkdead(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, sess, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	pl := srv.World.Get(sess.PlayerID)
	srv.shutdownSessions()
	if pl.HasFlag("linkdead") {
		t.Fatal("process shutdown marked the player link-dead")
	}
	if srv.World.Get(pl.ID) == nil {
		t.Fatal("process shutdown removed the player")
	}
	if !sess.Closed() {
		t.Fatal("process shutdown left the session open")
	}
}

func TestFailedQuitSaveDoesNotDespawn(t *testing.T) {
	orig := save.Root
	base := t.TempDir()
	block := filepath.Join(base, "block")
	if err := os.WriteFile(block, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	save.Root = filepath.Join(block, "saves")
	defer func() { save.Root = orig }()

	dir := filepath.Join("..", "..", "games", "old-house")
	srv, _, err := Launch(Options{
		PackDir:    dir,
		Mode:       protocol.ModePlay,
		Seed:       1,
		PlayerName: "Ada",
		Persist:    true,
		GraceTicks: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	bob, ent := srv.SpawnPlayerSession("Bob", "", "")
	bob.Persist = true
	srv.disconnect(bob, "quit")
	if srv.World.Get(ent.ID) == nil {
		t.Fatal("failed quit save removed Bob")
	}
	if !ent.HasFlag("linkdead") {
		t.Fatal("failed quit save did not leave Bob link-dead")
	}
	if _, ok := srv.Linkdead[ent.ID]; !ok {
		t.Fatal("failed quit save dropped the grace timer")
	}

	srv.Tick()
	if srv.World.Get(ent.ID) == nil {
		t.Fatal("failed grace save removed Bob")
	}
	if _, ok := srv.Linkdead[ent.ID]; !ok {
		t.Fatal("failed grace save dropped Bob")
	}
}

func TestPersistConnectRequiresMatchingAccount(t *testing.T) {
	orig := save.Root
	save.Root = t.TempDir()
	defer func() { save.Root = orig }()

	dir := filepath.Join("..", "..", "games", "old-house")
	srv, err := LaunchServer(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	anon := &Session{Out: make(chan protocol.Event, 8), In: make(chan string, 4)}
	if err := srv.bindConnectedPlayer(ConnectMsg{Session: anon, Name: "Alicia"}); err == nil {
		t.Fatal("expected an anonymous connect to be rejected")
	}
	if anon.PlayerID != "" {
		t.Fatal("anonymous connect spawned a player")
	}

	alice := &Session{Out: make(chan protocol.Event, 32), In: make(chan string, 4)}
	if err := srv.bindConnectedPlayer(ConnectMsg{Session: alice, Account: "alice", Persist: true, Name: "Alicia", Role: "reckless"}); err != nil {
		t.Fatal(err)
	}
	srv.AttachSession(alice)
	ps, err := save.ReadPlayer(srv.Pack.Meta.ID, "Alicia")
	if err != nil {
		t.Fatal(err)
	}
	if ps.Account != "alice" {
		t.Fatalf("expected account alice on the character file, got %q", ps.Account)
	}

	bob := &Session{Out: make(chan protocol.Event, 8), In: make(chan string, 4)}
	if err := srv.bindConnectedPlayer(ConnectMsg{Session: bob, Account: "bob", Persist: true, Name: "Alicia"}); err == nil {
		t.Fatal("expected a second live login to be rejected")
	}
	srv.disconnect(alice, "closed")
	if got := srv.World.Get(alice.PlayerID); got == nil || !got.HasFlag("linkdead") {
		t.Fatal("expected Alicia to stay link-dead")
	}
	if err := srv.bindConnectedPlayer(ConnectMsg{Session: bob, Account: "bob", Persist: true, Name: "Alicia"}); err == nil {
		t.Fatal("expected another account to be rejected")
	}
	again := &Session{Out: make(chan protocol.Event, 8), In: make(chan string, 4)}
	if err := srv.bindConnectedPlayer(ConnectMsg{Session: again, Account: "alice", Persist: true, Name: "Alicia"}); err != nil {
		t.Fatal(err)
	}
	if again.PlayerID != alice.PlayerID {
		t.Fatalf("expected the same body, got %s", again.PlayerID)
	}
}

func TestUnownedCharacterCannotBeClaimed(t *testing.T) {
	orig := save.Root
	save.Root = t.TempDir()
	defer func() { save.Root = orig }()

	w := world.New()
	w.PackID = "old-house"
	w.StartRoom = "house.porch"
	porch := &world.Entity{ID: "house.porch", Kind: world.KindRoom, Name: "Front Porch"}
	pl := &world.Entity{ID: "player.alicia", Kind: world.KindPlayer, Name: "Alicia"}
	w.Add(porch)
	w.Add(pl)
	_ = w.Move(pl.ID, porch.ID)
	if err := save.WritePaired("old-house", w, pl); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join("..", "..", "games", "old-house")
	srv, err := LaunchServer(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	sess := &Session{Out: make(chan protocol.Event, 8), In: make(chan string, 4)}
	if err := srv.bindConnectedPlayer(ConnectMsg{Session: sess, Account: "alice", Persist: true, Name: "Alicia"}); err == nil {
		t.Fatal("expected an unowned character file to be rejected")
	}
	if sess.PlayerID != "" {
		t.Fatal("unowned file was attached")
	}
}
