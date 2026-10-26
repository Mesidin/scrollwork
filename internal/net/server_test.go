package net

import (
	"context"
	"errors"
	stdnet "net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sudengine/internal/client"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/server"
)

func findOldHouse(t *testing.T) string {
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

// Test 1: A test listener accepts one framed connection, sends a room event,
// and reads one command frame back onto the actor loop.
func TestListenerAcceptsFramedConnectionAndCommand(t *testing.T) {
	packDir := findOldHouse(t)

	srv, err := server.LaunchServer(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModePlay,
	})
	if err != nil {
		t.Fatalf("LaunchServer failed: %v", err)
	}

	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Run(ctx)
	go func() {
		_ = Serve(ctx, srv, ln)
	}()

	conn, err := stdnet.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send connect frame
	connectPayload, err := EncodeConnect("Ada", "", "")
	if err != nil {
		t.Fatalf("EncodeConnect failed: %v", err)
	}
	if err := WriteFrame(conn, connectPayload); err != nil {
		t.Fatalf("WriteFrame connect failed: %v", err)
	}

	// Read frames until we receive a room event
	var roomReceived *protocol.RoomEvent
	deadline := time.Now().Add(3 * time.Second)
	_ = conn.SetReadDeadline(deadline)

	for time.Now().Before(deadline) {
		payload, err := ReadFrame(conn)
		if err != nil {
			t.Fatalf("ReadFrame failed: %v", err)
		}
		ev, err := DecodeEvent(payload)
		if err != nil {
			t.Fatalf("DecodeEvent failed: %v", err)
		}
		if r, ok := ev.(protocol.RoomEvent); ok {
			roomReceived = &r
			break
		}
	}

	if roomReceived == nil {
		t.Fatalf("did not receive RoomEvent from server")
	}
	if roomReceived.Title != "Front Porch" {
		t.Fatalf("expected room Title 'Front Porch', got %q", roomReceived.Title)
	}

	// Send a command frame back onto the actor loop
	cmdPayload, err := EncodeCommand("look")
	if err != nil {
		t.Fatalf("EncodeCommand failed: %v", err)
	}
	if err := WriteFrame(conn, cmdPayload); err != nil {
		t.Fatalf("WriteFrame command failed: %v", err)
	}

	// Read responses until prompt is received
	promptReceived := false
	for time.Now().Before(deadline) {
		payload, err := ReadFrame(conn)
		if err != nil {
			break
		}
		ev, err := DecodeEvent(payload)
		if err != nil {
			continue
		}
		if _, ok := ev.(protocol.PromptEvent); ok {
			promptReceived = true
			break
		}
	}

	if !promptReceived {
		t.Fatalf("actor loop did not process command and reply with prompt")
	}
}

// Test 2: connect to localhost shows the same room play shows for that pack.
func TestConnectShowsSameRoomAsPlay(t *testing.T) {
	packDir := findOldHouse(t)

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

	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	ctxRemote, cancelRemote := context.WithCancel(context.Background())
	defer cancelRemote()

	go remoteSrv.Run(ctxRemote)
	go func() {
		_ = Serve(ctxRemote, remoteSrv, ln)
	}()

	clientSess, cleanup, err := Dial(ctxRemote, ln.Addr().String(), "you", "", "")
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

// Test 3: serve rejects build mode.
func TestServeRejectsBuildMode(t *testing.T) {
	packDir := findOldHouse(t)

	_, err := server.LaunchServer(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModeBuild,
	})
	if err == nil {
		t.Fatalf("expected LaunchServer with ModeBuild to return error, got nil")
	}
	if !strings.Contains(err.Error(), "build mode is not permitted") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// Test 4: Multiple clients connected simultaneously over TCP can interact.
func TestMultipleFramedConnections(t *testing.T) {
	packDir := findOldHouse(t)

	srv, err := server.LaunchServer(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModePlay,
	})
	if err != nil {
		t.Fatalf("LaunchServer failed: %v", err)
	}

	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Run(ctx)
	go func() {
		_ = Serve(ctx, srv, ln)
	}()

	// Connect Ada
	sessAda, cleanupAda, err := Dial(ctx, ln.Addr().String(), "Ada", "", "")
	if err != nil {
		t.Fatalf("Dial Ada failed: %v", err)
	}
	defer cleanupAda()

	// Connect Bob
	sessBob, cleanupBob, err := Dial(ctx, ln.Addr().String(), "Bob", "", "")
	if err != nil {
		t.Fatalf("Dial Bob failed: %v", err)
	}
	defer cleanupBob()

	// Drain initial greets
	time.Sleep(300 * time.Millisecond)

	// Ada says something in the room
	sessAda.In <- `say Hello there, Bob`

	// Bob should receive ChanSay
	deadline := time.Now().Add(2 * time.Second)
	heardSay := false
	for time.Now().Before(deadline) {
		sessBob.Flush()
		select {
		case ev := <-sessBob.Out:
			if txt, ok := ev.(protocol.TextEvent); ok {
				if txt.Channel == protocol.ChanSay && strings.Contains(txt.Text, "Hello there, Bob") {
					heardSay = true
					goto heard
				}
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
heard:
	if !heardSay {
		t.Fatalf("Bob did not hear Ada's say over framed protocol")
	}

	// Bob whispers/tells Ada private message
	sessBob.In <- `tell Ada nice weather`
	heardTell := false
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sessAda.Flush()
		select {
		case ev := <-sessAda.Out:
			if txt, ok := ev.(protocol.TextEvent); ok {
				if txt.Channel == protocol.ChanSay && strings.Contains(txt.Text, "nice weather") {
					heardTell = true
					goto tellHeard
				}
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
tellHeard:
	if !heardTell {
		t.Fatalf("Ada did not hear Bob's tell over framed protocol")
	}
}

type failOnceListener struct {
	stdnet.Listener
	left int
}

func (f *failOnceListener) Accept() (stdnet.Conn, error) {
	if f.left > 0 {
		f.left--
		return nil, errors.New("reset")
	}
	return f.Listener.Accept()
}

func TestAcceptErrorDoesNotStopServe(t *testing.T) {
	packDir := findOldHouse(t)
	srv, err := server.LaunchServer(server.Options{PackDir: packDir, Mode: protocol.ModePlay, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	go func() {
		_ = Serve(ctx, srv, &failOnceListener{Listener: ln, left: 1})
	}()

	conn, err := stdnet.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	payload, err := EncodeConnect("Ada", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFrame(conn, payload); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	gotRoom := false
	for !gotRoom {
		body, err := ReadFrame(conn)
		if err != nil {
			t.Fatal(err)
		}
		ev, err := DecodeEvent(body)
		if err != nil {
			continue
		}
		if _, ok := ev.(protocol.RoomEvent); ok {
			gotRoom = true
		}
	}
}

func TestUnknownHandshakeDoesNotSpawn(t *testing.T) {
	packDir := findOldHouse(t)
	srv, err := server.LaunchServer(server.Options{PackDir: packDir, Mode: protocol.ModePlay, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	go func() { _ = Serve(ctx, srv, ln) }()

	bad, err := stdnet.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFrame(bad, []byte(`{"t":"disconnect","reason":"nope"}`)); err != nil {
		t.Fatal(err)
	}
	_ = bad.Close()

	sess, cleanup, err := Dial(ctx, ln.Addr().String(), "Ada", "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		sess.Flush()
		select {
		case ev := <-sess.Out:
			if _, ok := ev.(protocol.RoomEvent); ok {
				goto ready
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("Ada did not enter")
ready:
	sess.In <- "who"
	deadline = time.Now().Add(3 * time.Second)
	var lines []string
	for time.Now().Before(deadline) {
		sess.Flush()
		select {
		case ev := <-sess.Out:
			if txt, ok := ev.(protocol.TextEvent); ok {
				lines = append(lines, txt.Text)
				if strings.Contains(txt.Text, "Ada -") {
					goto listed
				}
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatalf("who did not list Ada: %v", lines)
listed:
	for _, line := range lines {
		if strings.Contains(line, "you -") {
			t.Fatalf("unknown handshake spawned a player: %v", lines)
		}
	}
}

func TestSameNameConnectionsBothSpawn(t *testing.T) {
	packDir := findOldHouse(t)
	srv, err := server.LaunchServer(server.Options{PackDir: packDir, Mode: protocol.ModePlay, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	go func() { _ = Serve(ctx, srv, ln) }()

	a, cleanA, err := Dial(ctx, ln.Addr().String(), "Sam", "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanA()
	b, cleanB, err := Dial(ctx, ln.Addr().String(), "Sam", "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanB()
	waitRoom := func(sess *server.Session) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			sess.Flush()
			select {
			case ev := <-sess.Out:
				if _, ok := ev.(protocol.RoomEvent); ok {
					return
				}
			case <-time.After(20 * time.Millisecond):
			}
		}
		t.Fatal("missing room")
	}
	waitRoom(a)
	waitRoom(b)
	a.In <- "who"
	deadline := time.Now().Add(3 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		a.Flush()
		select {
		case ev := <-a.Out:
			if txt, ok := ev.(protocol.TextEvent); ok && strings.Contains(txt.Text, "Sam -") {
				got += txt.Text + "\n"
			}
			if strings.Count(got, "Sam -") >= 2 {
				return
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatalf("expected two Sams, got %q", got)
}

func TestCleanupTwiceDoesNotPanic(t *testing.T) {
	ln, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		close(accepted)
		_, _ = ReadFrame(conn)
	}()
	_, cleanup, err := Dial(context.Background(), ln.Addr().String(), "Ada", "", "")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("accept timed out")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		cleanup()
	}()
	go func() {
		defer wg.Done()
		cleanup()
	}()
	wg.Wait()
	cleanup()
}

func TestWriterFlushesDisconnectWhenOutIsFull(t *testing.T) {
	client, serverConn := stdnet.Pipe()
	defer client.Close()
	sess := &server.Session{
		ID:  "you",
		Out: make(chan protocol.Event, 1),
		In:  make(chan string, 1),
	}
	sess.Send(protocol.TextEvent{Channel: protocol.ChanNarrative, Text: "one"})
	sess.Send(protocol.TextEvent{Channel: protocol.ChanNarrative, Text: "two"})
	sess.Disconnect("stalled")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writerDone := make(chan struct{})
	connClosed := make(chan struct{})
	go writeLoop(ctx, serverConn, sess, connClosed, writerDone)

	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	saw := false
	for i := 0; i < 4; i++ {
		payload, err := ReadFrame(client)
		if err != nil {
			t.Fatal(err)
		}
		ev, err := DecodeEvent(payload)
		if err != nil {
			t.Fatal(err)
		}
		if d, ok := ev.(protocol.DisconnectEvent); ok {
			if d.Reason != "stalled" {
				t.Fatalf("reason %q", d.Reason)
			}
			saw = true
			break
		}
	}
	if !saw {
		t.Fatal("disconnect stayed in the side queue")
	}
	close(connClosed)
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("writer did not exit")
	}
}
