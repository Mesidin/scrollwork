package net

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	stdnet "net"
	"reflect"
	"strings"
	"testing"
	"time"

	"sudengine/internal/protocol"
)

func TestFrameReadWrite(t *testing.T) {
	var buf bytes.Buffer

	// Room descriptions containing newlines:
	original := []byte("Front Porch\nDust, a locked cellar, and a maid who will not go downstairs.\nLight a lamp.\n")
	if err := WriteFrame(&buf, original); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	read, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}

	if !bytes.Equal(original, read) {
		t.Fatalf("expected %q, got %q", string(original), string(read))
	}
}

func TestMultipleFramesInStream(t *testing.T) {
	var buf bytes.Buffer

	f1 := []byte("frame 1: room desc\nwith multiple\nlines")
	f2 := []byte("frame 2: prompt > ")
	f3 := []byte("frame 3: █░█░ meter")

	if err := WriteFrame(&buf, f1); err != nil {
		t.Fatal(err)
	}
	if err := WriteFrame(&buf, f2); err != nil {
		t.Fatal(err)
	}
	if err := WriteFrame(&buf, f3); err != nil {
		t.Fatal(err)
	}

	r1, err := ReadFrame(&buf)
	if err != nil || !bytes.Equal(r1, f1) {
		t.Fatalf("f1 mismatch: got %q, err: %v", string(r1), err)
	}

	r2, err := ReadFrame(&buf)
	if err != nil || !bytes.Equal(r2, f2) {
		t.Fatalf("f2 mismatch: got %q, err: %v", string(r2), err)
	}

	r3, err := ReadFrame(&buf)
	if err != nil || !bytes.Equal(r3, f3) {
		t.Fatalf("f3 mismatch: got %q, err: %v", string(r3), err)
	}
}

func TestEventsEncodingDecoding(t *testing.T) {
	events := []protocol.Event{
		protocol.TextEvent{Channel: protocol.ChanSay, Text: `Ada says, "Hello."`},
		protocol.PromptEvent{Text: "Foyer Grit:24/24 > ", Room: "foyer", Status: "Grit:24/24"},
		protocol.RoomEvent{
			ID:          "foyer",
			Title:       "The Foyer",
			Description: "A dusty foyer.\nWith tall ceilings.",
			Exits:       []protocol.ExitInfo{{Dir: "north", Closed: false}},
			Occupants:   []string{"Ada"},
			Items:       []string{"brass key"},
			X:           1,
			Y:           2,
			Z:           0,
			HasCoords:   true,
			Dark:        false,
		},
		protocol.VitalsEvent{
			Resources: []protocol.ResourceView{{Key: "grit", Label: "Grit", Current: 24, Max: 24, Show: "bar"}},
			Flags:     []string{"hidden"},
			InCombat:  true,
			Round:     3,
		},
		protocol.InventoryEvent{
			Items:     []string{"lantern"},
			Equipment: []protocol.EquipView{{Slot: "main_hand", Item: "iron sword"}},
		},
		protocol.MapEvent{
			Rooms:   []protocol.MapRoom{{X: 0, Y: 0, Name: "Foyer", Here: true}},
			PlayerX: 0,
			PlayerY: 0,
			Text:    "Map text",
		},
		protocol.CombatEvent{
			Active:   true,
			Fighters: []protocol.FighterView{{Name: "Goblin", HP: 10, Max: 10, IsPlayer: false}},
		},
		protocol.LayoutEvent{Panes: []string{"output", "vitals", "map"}},
		protocol.ModeEvent{Mode: protocol.ModePlay},
		protocol.TitleEvent{Title: "The Old House"},
		protocol.DisconnectEvent{Reason: "quit"},
	}

	expectedTypes := []string{
		"text", "prompt", "room", "vitals", "inventory",
		"map", "combat", "layout", "mode", "title", "disconnect",
	}

	for i, ev := range events {
		data, err := EncodeEvent(ev)
		if err != nil {
			t.Fatalf("EncodeEvent(%T) failed: %v", ev, err)
		}

		// Verify "t" field in JSON
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("JSON unmarshal error on encoded %T: %v", ev, err)
		}
		if raw["t"] != expectedTypes[i] {
			t.Fatalf("expected t=%q for %T, got %v in %s", expectedTypes[i], ev, raw["t"], string(data))
		}

		// Decode back to protocol.Event
		decoded, err := DecodeEvent(data)
		if err != nil {
			t.Fatalf("DecodeEvent failed for %T: %v", ev, err)
		}

		if !reflect.DeepEqual(ev, decoded) {
			t.Fatalf("roundtrip mismatch for %T:\nexpected: %+v\ngot:      %+v", ev, ev, decoded)
		}
	}
}

func TestClientFrameCodec(t *testing.T) {
	// 1. Command frame
	cmdData, err := EncodeCommand("look north")
	if err != nil {
		t.Fatal(err)
	}
	cf, err := DecodeClientFrame(cmdData)
	if err != nil {
		t.Fatal(err)
	}
	if cf.Type != "command" || cf.Command != "look north" {
		t.Fatalf("unexpected command frame: %+v", cf)
	}

	// 2. Connect frame
	connData, err := EncodeConnect("Ada", "human", "drifter")
	if err != nil {
		t.Fatal(err)
	}
	cf, err = DecodeClientFrame(connData)
	if err != nil {
		t.Fatal(err)
	}
	if cf.Type != "connect" || cf.Name != "Ada" || cf.Race != "human" || cf.Role != "drifter" {
		t.Fatalf("unexpected connect frame: %+v", cf)
	}

	// 3. Fallback without explicit "t"
	cf, err = DecodeClientFrame([]byte(`{"command":"get lamp"}`))
	if err != nil {
		t.Fatal(err)
	}
	if cf.Type != "command" || cf.Command != "get lamp" {
		t.Fatalf("unexpected fallback: %+v", cf)
	}
}

func TestOversizeFrameRejectedBeforeBody(t *testing.T) {
	var buf bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(MaxFrameSize+1))
	if _, err := buf.Write(hdr[:]); err != nil {
		t.Fatal(err)
	}
	_, err := ReadFrame(&buf)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversize rejection, got %v", err)
	}
}

func TestFrameBodyDeadline(t *testing.T) {
	a, b := stdnet.Pipe()
	defer a.Close()
	defer b.Close()
	go func() {
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], 64)
		_, _ = a.Write(hdr[:])
	}()
	_ = b.SetReadDeadline(time.Time{})
	_, err := readFrameBodyDeadline(b, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected the body read to time out")
	}
}

func TestExampleJSONFormat(t *testing.T) {
	// Spec exact example:
	// {"t":"text","channel":"say","text":"Ada says, \"Hello.\""}
	ev := protocol.TextEvent{Channel: protocol.ChanSay, Text: `Ada says, "Hello."`}
	data, err := EncodeEvent(ev)
	if err != nil {
		t.Fatal(err)
	}
	str := string(data)
	if !strings.Contains(str, `"t":"text"`) || !strings.Contains(str, `"channel":"say"`) || !strings.Contains(str, `"text":"Ada says, \"Hello.\""`) {
		t.Fatalf("JSON format did not match spec: %s", str)
	}
}
