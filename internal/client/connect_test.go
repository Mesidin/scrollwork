package client

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	snet "sudengine/internal/net"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/save"
	"sudengine/internal/server"
)

func testOldHouse(t *testing.T) string {
	t.Helper()
	for _, c := range []string{
		filepath.Join("..", "..", "games", "old-house"),
		filepath.Join("games", "old-house"),
	} {
		p, err := pack.Load(c)
		if err == nil {
			return p.Dir
		}
	}
	t.Fatal("failed to locate old-house pack")
	return ""
}

func TestBookmarkWaitsForWorldAndDenialStaysPut(t *testing.T) {
	origRoot := save.Root
	save.Root = t.TempDir()
	defer func() { save.Root = origRoot }()
	origCost := save.BcryptCost
	save.BcryptCost = 4
	defer func() { save.BcryptCost = origCost }()

	packDir := testOldHouse(t)
	srv, err := server.LaunchServer(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModePlay,
		Persist: true,
		Seed:    1,
	})
	if err != nil {
		t.Fatal(err)
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

	m := New(filepath.Dir(packDir))
	m = m.openRemoteLogin(ln.Addr().String(), snet.ServerInfo{
		Type:    "server_info",
		Persist: true,
		Title:   "The Old House",
		PackID:  "old-house",
	})
	m.rlAccount = "kid3"
	m.rlPassword = "secret123"
	m.rlStep = rlStepCharName

	next, cmd := m.finishRemoteLogin("NoRole", "", "", nil)
	if cmd == nil {
		t.Fatal("expected a dial command")
	}
	if list, _ := save.LoadConnections(); len(list) != 0 {
		t.Fatal("bookmark was written before the dial returned")
	}
	updated, _ := next.(Model).Update(cmd())
	m = updated.(Model)
	if m.state != stateGame {
		t.Fatalf("dial state %v err %s", m.state, m.errLine)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && m.state == stateGame {
		m.sess.Flush()
		select {
		case ev := <-m.sess.Out:
			updated, _ = m.Update(ev)
			m = updated.(Model)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if m.state != stateRemoteLogin {
		t.Fatalf("denial left state %v err %q", m.state, m.errLine)
	}
	if !strings.Contains(strings.ToLower(m.errLine), "role") {
		t.Fatalf("expected the role error on the character step, got %q", m.errLine)
	}
	if list, _ := save.LoadConnections(); len(list) != 0 {
		t.Fatalf("bookmark saved after refusal: %+v", list)
	}

	next, cmd = m.finishRemoteLogin("Alicia", "", "reckless", nil)
	updated, _ = next.(Model).Update(cmd())
	m = updated.(Model)
	if m.state != stateGame {
		t.Fatalf("second dial state %v err %s", m.state, m.errLine)
	}
	joined := false
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !joined {
		if m.sess == nil {
			break
		}
		m.sess.Flush()
		select {
		case ev := <-m.sess.Out:
			if _, ok := ev.(protocol.RoomEvent); ok {
				joined = true
			}
			updated, _ = m.Update(ev)
			m = updated.(Model)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !joined || m.state != stateGame {
		t.Fatalf("expected Alicia to enter, state %v err %q", m.state, m.errLine)
	}
	list, err := save.LoadConnections()
	if err != nil || len(list) != 1 || list[0].Character != "Alicia" {
		t.Fatalf("bookmark after join: %+v err %v", list, err)
	}
}
