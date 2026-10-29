package net

import (
	"bytes"
	"context"
	"errors"
	stdnet "net"
	"strings"
	"testing"
	"time"

	"sudengine/internal/protocol"
	"sudengine/internal/save"
	"sudengine/internal/server"
	"sudengine/internal/theme"
)

// Stage 4 Done Check 1: A raw TCP test sends "look\n" and receives the room title.
func TestTelnetRawLookReceivesRoomTitle(t *testing.T) {
	packDir := findOldHouse(t)
	srv, err := server.LaunchServer(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModePlay,
		Seed:    1,
	})
	if err != nil {
		t.Fatalf("LaunchServer: %v", err)
	}

	ln, err := ListenTelnet("127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenTelnet: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Run(ctx)
	go func() {
		_ = ServeTelnet(ctx, srv, ln)
	}()

	conn, err := stdnet.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("look\n")); err != nil {
		t.Fatalf("Write look: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var received bytes.Buffer
	buf := make([]byte, 1024)
	for !strings.Contains(received.String(), "Obvious exits:") {
		n, err := conn.Read(buf)
		if n > 0 {
			received.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	plain := StripANSI(received.String())
	if !strings.Contains(plain, "Front Porch") {
		t.Fatalf("expected Telnet output to contain room title 'Front Porch', got:\n%s", plain)
	}
	if !strings.Contains(plain, "Obvious exits:") {
		t.Fatalf("expected Telnet output to contain exits line, got:\n%s", plain)
	}
}

// Stage 4 Done Check 2: An IAC sequence in the byte stream is consumed and is not dispatched as a command.
func TestTelnetIACSequenceConsumedNotDispatchedAsCommand(t *testing.T) {
	// 1. Unit test TelnetReader with various IAC negotiations and subnegotiations.
	stream := bytes.NewBuffer(nil)
	// IAC WILL ECHO, IAC DO SGA, IAC SB NAWS 0 80 0 24 (with embedded \n = 0x0A!) IAC SE, standalone line, then look\n
	stream.Write([]byte{IAC, WILL, OptEcho, IAC, DO, 3})
	stream.Write([]byte{IAC, SB, 31, 0, 80, 0, 10, IAC, SE, '\n'})
	stream.Write([]byte{IAC, WONT, 24})
	stream.WriteString("look\r\n")

	tr := NewTelnetReader(stream)
	line, err := tr.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine failed: %v", err)
	}
	if line != "look" {
		t.Fatalf("expected IAC-stripped line 'look', got %q", line)
	}

	// 2. End-to-end TCP test sending IAC bytes before and around a command.
	packDir := findOldHouse(t)
	srv, err := server.LaunchServer(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModePlay,
		Seed:    1,
	})
	if err != nil {
		t.Fatalf("LaunchServer: %v", err)
	}

	ln, err := ListenTelnet("127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenTelnet: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Run(ctx)
	go func() {
		_ = ServeTelnet(ctx, srv, ln)
	}()

	conn, err := stdnet.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Send pure IAC line followed by IAC-prefixed look\n and IAC-prefixed who\n.
	var payload bytes.Buffer
	payload.Write([]byte{IAC, WILL, 1, IAC, DO, 3, '\r', '\n'})
	payload.Write([]byte{IAC, SB, 31, 0, 80, 0, 24, IAC, SE})
	payload.WriteString("look\r\n")
	payload.Write([]byte{IAC, NOP})
	payload.WriteString("who\r\n")
	if _, err := conn.Write(payload.Bytes()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var received bytes.Buffer
	buf := make([]byte, 1024)
	for !strings.Contains(received.String(), "Connected Players:") {
		n, err := conn.Read(buf)
		if n > 0 {
			received.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	plain := StripANSI(received.String())
	if !strings.Contains(plain, "Front Porch") {
		t.Fatalf("expected room title 'Front Porch', got:\n%s", plain)
	}
	if !strings.Contains(plain, "Connected Players:") {
		t.Fatalf("expected 'Connected Players:' from who command, got:\n%s", plain)
	}
	if strings.Contains(plain, "I don't understand") {
		t.Fatalf("IAC sequence leaked as unknown command:\n%s", plain)
	}
}

func TestTelnetEventFormatting(t *testing.T) {
	pal := theme.ANSI16()

	roomOut := FormatTelnetEvent(pal, protocol.RoomEvent{
		ID:          "house.porch",
		Title:       "Front Porch",
		Description: "Rain drips from the eaves.",
		Exits:       []protocol.ExitInfo{{Dir: "north", Closed: false}},
		Items:       []string{"a brass key"},
		Occupants:   []string{"Ada"},
	})
	plainRoom := StripANSI(roomOut)
	for _, want := range []string{"Front Porch", "Rain drips from the eaves.", "A brass key is here.", "Ada is here.", "Obvious exits: north."} {
		if !strings.Contains(plainRoom, want) {
			t.Fatalf("RoomEvent missing %q in:\n%s", want, plainRoom)
		}
	}

	promptOut := StripANSI(FormatTelnetEvent(pal, protocol.PromptEvent{
		Room:   "Front Porch",
		Status: "Grit:24/24",
	}))
	if !strings.Contains(promptOut, "Front Porch") || !strings.Contains(promptOut, "Grit:24/24") || !strings.HasSuffix(promptOut, "> ") {
		t.Fatalf("unexpected PromptEvent format: %q", promptOut)
	}

	sayOut := FormatTelnetEvent(pal, protocol.TextEvent{
		Channel: protocol.ChanSay,
		Text:    `Ada says, "Hello."`,
	})
	if !strings.Contains(StripANSI(sayOut), `Ada says, "Hello."`) {
		t.Fatalf("unexpected TextEvent format: %q", sayOut)
	}

	// Map, combat, and inventory events should be omitted.
	for _, omitted := range []protocol.Event{
		protocol.MapEvent{Text: "@"},
		protocol.CombatEvent{Active: true},
		protocol.InventoryEvent{Items: []string{"lamp"}},
	} {
		if got := FormatTelnetEvent(pal, omitted); got != "" {
			t.Fatalf("expected %T to be omitted on Telnet, got %q", omitted, got)
		}
	}
}

func TestTelnetAccountLoginEchoSuppressionAndChargen(t *testing.T) {
	origRoot := save.Root
	save.Root = t.TempDir()
	defer func() { save.Root = origRoot }()

	origCost := save.BcryptCost
	save.BcryptCost = 4 // MinCost for fast test execution
	defer func() { save.BcryptCost = origCost }()

	packDir := findOldHouse(t)
	srv, err := server.LaunchServer(server.Options{
		PackDir: packDir,
		Mode:    protocol.ModePlay,
		Persist: true,
		Seed:    1,
	})
	if err != nil {
		t.Fatalf("LaunchServer: %v", err)
	}
	// Enable a 2-point point buy on Old House to test all server-driven chargen steps.
	srv.Pack.RPG.Chargen.PointBuy = 2

	ln, err := ListenTelnet("127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenTelnet: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Run(ctx)
	go func() {
		_ = ServeTelnet(ctx, srv, ln)
	}()

	conn, err := stdnet.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	readUntil := func(substr string) []byte {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var out bytes.Buffer
		buf := make([]byte, 512)
		for !strings.Contains(StripANSI(out.String()), substr) {
			n, err := conn.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
			}
			if err != nil {
				t.Fatalf("readUntil(%q) failed: %v\ncollected:\n%s", substr, err, StripANSI(out.String()))
			}
		}
		return out.Bytes()
	}

	// 1. Account prompt
	readUntil("Account:")
	_, _ = conn.Write([]byte("alice\r\n"))

	// 2. Password prompt must include IAC WILL ECHO (0xff 0xfb 0x01)
	passPrompt := readUntil("Password:")
	if !bytes.Contains(passPrompt, []byte{IAC, WILL, OptEcho}) {
		t.Fatalf("expected IAC WILL ECHO before password entry, got %v", passPrompt)
	}
	_, _ = conn.Write([]byte("hunter2\r\n"))

	// 3. After password entry, server must send IAC WONT ECHO (0xff 0xfc 0x01) and never echo password
	charPrompt := readUntil("Character name:")
	if !bytes.Contains(charPrompt, []byte{IAC, WONT, OptEcho}) {
		t.Fatalf("expected IAC WONT ECHO after password entry, got %v", charPrompt)
	}
	if bytes.Contains(charPrompt, []byte("hunter2")) {
		t.Fatalf("server echoed password back to client!")
	}

	// 4. Enter new character name -> triggers server-driven chargen (Role + Point Buy)
	_, _ = conn.Write([]byte("Alicia\r\n"))
	readUntil("Role (1-2):")
	_, _ = conn.Write([]byte("2\r\n")) // Reckless (hp: 30)

	// Point buy step 1
	readUntil("Spend point on")
	_, _ = conn.Write([]byte("body\r\n"))
	// Point buy step 2
	readUntil("Spend point on")
	_, _ = conn.Write([]byte("will\r\n"))

	// 5. Character enters Front Porch
	enterOut := StripANSI(string(readUntil("Front Porch")))
	if !strings.Contains(enterOut, "Front Porch") {
		t.Fatalf("expected Alicia to enter Front Porch, got:\n%s", enterOut)
	}

	// Verify account and character files exist on disk
	acc, err := save.AuthenticateAccount(srv.Pack.Meta.ID, "alice", "hunter2")
	if err != nil {
		t.Fatalf("AuthenticateAccount failed: %v", err)
	}
	if !acc.OwnsCharacter("Alicia") {
		t.Fatalf("expected account alice to own character Alicia, got %v", acc.Characters)
	}
	ps, err := save.ReadPlayer(srv.Pack.Meta.ID, "Alicia")
	if err != nil {
		t.Fatalf("ReadPlayer Alicia failed: %v", err)
	}
	if ps.Player.RoleID != "reckless" || ps.Player.Attrs["body"] < 1 || ps.Player.Attrs["will"] < 2 {
		t.Fatalf("unexpected chargen stats on saved Alicia: role=%q attrs=%v", ps.Player.RoleID, ps.Player.Attrs)
	}
	if ps.Account == "" {
		t.Fatal("expected Alicia's save to record the owning account")
	}
}

func TestTelnetSubnegotiationEscapedIACDoesNotEndEarly(t *testing.T) {
	stream := bytes.NewBuffer(nil)
	stream.Write([]byte{IAC, SB, 31, IAC, IAC, SE, 'n', 'o', IAC, SE})
	stream.WriteString("look\n")
	tr := NewTelnetReader(stream)
	line, err := tr.ReadLine()
	if err != nil {
		t.Fatal(err)
	}
	if line != "look" {
		t.Fatalf("expected look after an escaped IAC SE inside SB, got %q", line)
	}
}

func TestTelnetDataIACBecomesLiteralFF(t *testing.T) {
	stream := bytes.NewBuffer([]byte{'a', IAC, IAC, 'b', '\n'})
	tr := NewTelnetReader(stream)
	line, err := tr.ReadLine()
	if err != nil {
		t.Fatal(err)
	}
	if line != "a\xffb" {
		t.Fatalf("expected a literal 0xFF in the line, got %q", line)
	}
}

func TestTelnetReadLineRejectsOverlongLine(t *testing.T) {
	payload := append(bytes.Repeat([]byte{'a'}, MaxFrameSize+1), '\n')
	tr := NewTelnetReader(bytes.NewReader(payload))
	line, err := tr.ReadLine()
	if !errors.Is(err, ErrLineTooLong) {
		t.Fatalf("expected ErrLineTooLong, got line %q err %v", line, err)
	}
	if line != "" {
		t.Fatalf("overlong line was returned: %q", line)
	}
}

func TestTelnetOutputDoublesIACAndStripsPlayerESC(t *testing.T) {
	pal := theme.ANSI16()
	formatted := FormatTelnetEvent(pal, protocol.TextEvent{
		Channel: protocol.ChanSay,
		Text:    "hi\x1b[2J\xff",
	})
	if strings.Contains(formatted, "\x1b[2J") {
		t.Fatalf("player ESC survived theme coloring: %q", formatted)
	}
	if !strings.Contains(formatted, "\x1b[") {
		t.Fatal("theme color was removed with the player ESC")
	}
	var buf bytes.Buffer
	if err := writeTelnet(&buf, formatted); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte{0xff, 0xff}) {
		t.Fatalf("0xFF was not doubled on output: %v", buf.Bytes())
	}
	if bytes.Count(buf.Bytes(), []byte{0xff})%2 != 0 {
		t.Fatal("odd number of 0xFF bytes on the wire")
	}
}
