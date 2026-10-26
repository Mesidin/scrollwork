package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/weather"
	"sudengine/internal/world"
)

func TestCommandQueueCaps(t *testing.T) {
	sess := &Session{Out: make(chan protocol.Event, 1)}
	for i := 0; i < MaxQueuedCommands; i++ {
		if !sess.EnqueueCommand("look") {
			t.Fatalf("rejected command %d", i)
		}
	}
	if sess.EnqueueCommand("look") {
		t.Fatal("queue accepted a command past the cap")
	}
	if sess.CommandQueueLen() != MaxQueuedCommands {
		t.Fatalf("queue length %d", sess.CommandQueueLen())
	}
}

func TestEnqueueBlocksWhenCommandQueueIsFull(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, sess, err := Launch(Options{
		PackDir: dir,
		Mode:    protocol.ModePlay,
		Seed:    1,
		TickMS:  5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for i := 0; i < 100; i++ {
			if !srv.Enqueue(sess, "look") {
				return
			}
		}
	}()
	select {
	case <-finished:
		t.Fatal("queued 100 commands without blocking")
	case <-time.After(300 * time.Millisecond):
	}
	if n := sess.CommandQueueLen(); n > MaxQueuedCommands {
		t.Fatalf("queue length %d", n)
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("enqueue did not return after shutdown")
	}
}

func TestStartupDouseReachesAttachedSession(t *testing.T) {
	w := world.New()
	p := &pack.Pack{
		Meta: pack.Meta{ID: "storm-start", Title: "Storm", TickMS: 50, StartRoom: "room.square"},
		Time: weather.TimeConfig{Level: 3, MinutesPerDay: 24, StartHour: 12},
		Weather: weather.WeatherConfig{
			Level:          3,
			Initial:        "storm",
			ChangeInterval: 100,
		},
	}
	square := &world.Entity{
		ID: "room.square", Kind: world.KindRoom, Name: "Square",
		Flags: map[string]bool{"outdoors": true},
	}
	player := &world.Entity{ID: "player", Kind: world.KindPlayer, Name: "Ada"}
	lamp := &world.Entity{
		ID: "lamp", Kind: world.KindItem, Name: "lantern",
		Use: &world.UseEffect{ToggleFlag: "light"},
	}
	w.Add(square)
	w.Add(player)
	w.Add(lamp)
	w.PlayerID = player.ID
	w.StartRoom = square.ID
	if err := w.Move(player.ID, square.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.Move(lamp.ID, player.ID); err != nil {
		t.Fatal(err)
	}
	lamp.SetFlag("light", true)

	srv := newServerInstance(p, w, 1)
	if !lamp.HasFlag("light") {
		t.Fatal("startup extinguished the lamp before a session existed")
	}
	sess := &Session{
		ID:       "local",
		Mode:     protocol.ModePlay,
		PlayerID: player.ID,
		Out:      make(chan protocol.Event, 8),
		In:       make(chan string, 1),
	}
	srv.Sessions[sess.ID] = sess
	if !srv.Env.Extinguishes(true) {
		t.Fatal("storm at level 3 should extinguish outdoor flames")
	}
	srv.douseOutdoorFlames()
	if lamp.HasFlag("light") {
		t.Fatal("lamp stayed lit")
	}
	sess.Flush()
	select {
	case ev := <-sess.Out:
		txt, ok := ev.(protocol.TextEvent)
		if !ok || !strings.Contains(txt.Text, "blows the flame out") {
			t.Fatalf("expected the flame line, got %#v", ev)
		}
	default:
		t.Fatal("attached session did not receive the flame line")
	}
}
