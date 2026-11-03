package net_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"strings"
	"sudengine/internal/client"
	snet "sudengine/internal/net"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/save"
	"sudengine/internal/server"
)

func findOldHouseDir(t *testing.T) string {
	candidates := []string{
		filepath.Join("..", "..", "games", "old-house"),
		filepath.Join("games", "old-house"),
	}
	for _, c := range candidates {
		p, err := pack.Load(c)
		if err == nil {
			return p.Dir
		}
	}
	t.Fatalf("failed to locate old-house pack")
	return ""
}

// Test 2: connect to localhost shows the same room play shows for that pack.
func TestConnectShowsSameRoomAsPlay(t *testing.T) {
	packDir := findOldHouseDir(t)

	// 1. Launch in local single-player "play" mode to see what room play shows
	localSrv, localSess, err := server.Launch(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModePlay,
	})
	if err != nil {
		t.Fatalf("local Launch failed: %v", err)
	}
	ctxLocal, cancelLocal := context.WithCancel(context.Background())
	defer cancelLocal()
	go localSrv.Run(ctxLocal)

	var playRoom protocol.RoomEvent
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		localSess.Flush()
		select {
		case ev := <-localSess.Out:
			if r, ok := ev.(protocol.RoomEvent); ok {
				playRoom = r
				goto playDone
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
playDone:
	if playRoom.ID == "" {
		t.Fatalf("play mode did not emit RoomEvent")
	}

	// 2. Launch server mode and connect via remote Dial
	remoteSrv, err := server.LaunchServer(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModePlay,
	})
	if err != nil {
		t.Fatalf("LaunchServer failed: %v", err)
	}

	ln, err := snet.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	ctxRemote, cancelRemote := context.WithCancel(context.Background())
	defer cancelRemote()

	go remoteSrv.Run(ctxRemote)
	go func() {
		_ = snet.Serve(ctxRemote, remoteSrv, ln)
	}()

	clientSess, cleanup, err := snet.Dial(ctxRemote, ln.Addr().String(), "you", "", "")
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer cleanup()

	m := client.ConnectPlay(clientSess, cleanup)

	// Pump events into client model until it receives the room event
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		clientSess.Flush()
		select {
		case ev := <-clientSess.Out:
			updated, _ := m.Update(ev)
			m = updated.(client.Model)
			if m.CurrentRoom().ID != "" {
				goto connectDone
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
connectDone:
	remoteRoom := m.CurrentRoom()
	if remoteRoom.ID == "" {
		t.Fatalf("remote client did not receive room")
	}

	if remoteRoom.ID != playRoom.ID {
		t.Fatalf("room ID mismatch: play had %q, connect had %q", playRoom.ID, remoteRoom.ID)
	}
	if remoteRoom.Title != playRoom.Title {
		t.Fatalf("room Title mismatch: play had %q, connect had %q", playRoom.Title, remoteRoom.Title)
	}
	if remoteRoom.Description != playRoom.Description {
		t.Fatalf("room Description mismatch: play had %q, connect had %q", playRoom.Description, remoteRoom.Description)
	}
}

func TestFramedPersistentLoginAndSavedConnections(t *testing.T) {
	origRoot := save.Root
	save.Root = t.TempDir()
	defer func() { save.Root = origRoot }()

	origCost := save.BcryptCost
	save.BcryptCost = 4
	defer func() { save.BcryptCost = origCost }()

	packDir := findOldHouseDir(t)
	srv, err := server.LaunchServer(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModePlay,
		Persist: true,
		Seed:    1,
	})
	if err != nil {
		t.Fatalf("LaunchServer: %v", err)
	}

	ln, err := snet.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	go func() {
		_ = snet.Serve(ctx, srv, ln)
	}()

	addr := ln.Addr().String()

	// 1. Probe returns server info with Persist=true and Title="The Old House".
	info, err := snet.Probe(ctx, addr)
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if !info.Persist || info.Title != "The Old House" {
		t.Fatalf("unexpected Probe info: %+v", info)
	}

	// 2. Authenticate creates a new account, and rejects wrong password on second attempt.
	authInfo, err := snet.Authenticate(ctx, addr, "kid1", "secret123")
	if err != nil {
		t.Fatalf("Authenticate new account failed: %v", err)
	}
	if !authInfo.NewAccount {
		t.Fatalf("expected NewAccount=true on first auth")
	}
	if _, err := snet.Authenticate(ctx, addr, "kid1", "wrongpass"); err == nil {
		t.Fatalf("expected wrong password to fail")
	}

	// 3. DialWithLogin spawns character "Alicia" under account "kid1".
	sess, cleanup, err := snet.DialWithLogin(ctx, addr, snet.LoginOptions{
		Account:  "kid1",
		Password: "secret123",
		Name:     "Alicia",
		Role:     "reckless",
	})
	if err != nil {
		t.Fatalf("DialWithLogin failed: %v", err)
	}
	defer cleanup()

	deadline := time.Now().Add(3 * time.Second)
	gotRoom := false
	for time.Now().Before(deadline) {
		sess.Flush()
		select {
		case ev := <-sess.Out:
			if r, ok := ev.(protocol.RoomEvent); ok && r.Title == "Front Porch" {
				gotRoom = true
				goto entered
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
entered:
	if !gotRoom {
		t.Fatalf("expected Alicia to enter Front Porch via framed DialWithLogin")
	}

	// 4. Subsequent Authenticate lists "Alicia" in the account's characters.
	authAgain, err := snet.Authenticate(ctx, addr, "kid1", "secret123")
	if err != nil {
		t.Fatalf("Authenticate existing account failed: %v", err)
	}
	if len(authAgain.Characters) != 1 || authAgain.Characters[0] != "Alicia" {
		t.Fatalf("expected Characters=[Alicia], got %v", authAgain.Characters)
	}

	// 5. SaveConnection stores the bookmark and formats it as "The Old House — Alicia".
	connEntry := save.Connection{
		Addr:      addr,
		PackID:    info.PackID,
		Title:     info.Title,
		Account:   "kid1",
		Password:  "secret123",
		Character: "Alicia",
	}
	if err := save.SaveConnection(connEntry); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}
	loaded, err := save.LoadConnections()
	if err != nil || len(loaded) != 1 {
		t.Fatalf("LoadConnections: %v (%v)", loaded, err)
	}
	if loaded[0].Label() != "The Old House — Alicia" {
		t.Fatalf("expected label 'The Old House — Alicia', got %q", loaded[0].Label())
	}

	// 6. Launcher view shows "The Old House — Alicia" and "Connect to server...".
	m := client.New(filepath.Dir(packDir))
	view := m.View().Content
	if !strings.Contains(view, "The Old House — Alicia") {
		t.Fatalf("expected launcher view to list 'The Old House — Alicia', got:\n%s", view)
	}
	if !strings.Contains(view, "Connect to server...") {
		t.Fatalf("expected launcher view to list 'Connect to server...', got:\n%s", view)
	}
}

func TestFramedChargenRejectsSkippedRoleAndDropsUnbudgetedStats(t *testing.T) {
	origRoot := save.Root
	save.Root = t.TempDir()
	defer func() { save.Root = origRoot }()

	origCost := save.BcryptCost
	save.BcryptCost = 4
	defer func() { save.BcryptCost = origCost }()

	packDir := findOldHouseDir(t)
	srv, err := server.LaunchServer(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModePlay,
		Persist: true,
		Seed:    1,
	})
	if err != nil {
		t.Fatalf("LaunchServer: %v", err)
	}
	ln, err := snet.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	go func() { _ = snet.Serve(ctx, srv, ln) }()
	addr := ln.Addr().String()

	sess, cleanup, err := snet.DialWithLogin(ctx, addr, snet.LoginOptions{
		Account:  "kid2",
		Password: "secret123",
		Name:     "NoRole",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	reason := ""
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && reason == "" {
		sess.Flush()
		select {
		case ev := <-sess.Out:
			switch e := ev.(type) {
			case protocol.TextEvent:
				if strings.Contains(strings.ToLower(e.Text), "role") {
					reason = e.Text
				}
			case protocol.DisconnectEvent:
				if reason == "" {
					reason = e.Reason
				}
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !strings.Contains(strings.ToLower(reason), "role") {
		t.Fatalf("expected a missing role to be refused, got %q", reason)
	}
	if save.HasPlayer("old-house", "NoRole") {
		t.Fatal("refused character was spawned")
	}

	okSess, okCleanup, err := snet.DialWithLogin(ctx, addr, snet.LoginOptions{
		Account:  "kid2",
		Password: "secret123",
		Name:     "StatCheck",
		Role:     "reckless",
		Attrs:    map[string]int{"body": 40},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer okCleanup()
	gotRoom := false
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !gotRoom {
		okSess.Flush()
		select {
		case ev := <-okSess.Out:
			if _, ok := ev.(protocol.RoomEvent); ok {
				gotRoom = true
			}
			if d, ok := ev.(protocol.DisconnectEvent); ok {
				t.Fatalf("expected StatCheck to enter, got disconnect %q", d.Reason)
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !gotRoom {
		t.Fatal("expected StatCheck to enter the world")
	}
	ps, err := save.ReadPlayer("old-house", "StatCheck")
	if err != nil || ps.Player == nil {
		t.Fatalf("ReadPlayer: %v", err)
	}
	if ps.Player.Attrs["body"] == 40 {
		t.Fatal("unbudgeted body spend was applied")
	}
}
