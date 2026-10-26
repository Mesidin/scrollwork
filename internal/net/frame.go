package net

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"sudengine/internal/protocol"
)

// MaxFrameSize bounds a single text event. The length is reserved before the
// body arrives, so a stalled peer must not pin a multi-megabyte buffer.
const MaxFrameSize = 256 * 1024

func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxFrameSize {
		return fmt.Errorf("payload length %d exceeds max frame size %d", len(payload), MaxFrameSize)
	}
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(payload)))
	copy(buf[4:], payload)
	_, err := w.Write(buf)
	return err
}

func ReadFrame(r io.Reader) ([]byte, error) {
	n, err := readHeader(r)
	if err != nil {
		return nil, err
	}
	return readPayload(r, n)
}

func readHeader(r io.Reader) (int, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, err
	}
	length := binary.BigEndian.Uint32(header[:])
	if length > MaxFrameSize {
		return 0, fmt.Errorf("frame length %d exceeds max frame size %d", length, MaxFrameSize)
	}
	return int(length), nil
}

func readPayload(r io.Reader, length int) ([]byte, error) {
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// readFrameBodyDeadline waits without a deadline for the length prefix, then
// requires the body within bodyWait. An idle player is not disconnected.
// The length is still rejected before the body buffer is allocated.
func readFrameBodyDeadline(conn interface {
	Read(b []byte) (int, error)
	SetReadDeadline(t time.Time) error
}, bodyWait time.Duration) ([]byte, error) {
	n, err := readHeader(conn)
	if err != nil {
		return nil, err
	}
	if bodyWait > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(bodyWait))
	}
	payload, err := readPayload(conn, n)
	_ = conn.SetReadDeadline(time.Time{})
	return payload, err
}

func EncodeEvent(ev protocol.Event) ([]byte, error) {
	switch e := ev.(type) {
	case protocol.TextEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.TextEvent
		}{Type: "text", TextEvent: e})
	case protocol.PromptEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.PromptEvent
		}{Type: "prompt", PromptEvent: e})
	case protocol.RoomEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.RoomEvent
		}{Type: "room", RoomEvent: e})
	case protocol.VitalsEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.VitalsEvent
		}{Type: "vitals", VitalsEvent: e})
	case protocol.InventoryEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.InventoryEvent
		}{Type: "inventory", InventoryEvent: e})
	case protocol.MapEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.MapEvent
		}{Type: "map", MapEvent: e})
	case protocol.CombatEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.CombatEvent
		}{Type: "combat", CombatEvent: e})
	case protocol.LayoutEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.LayoutEvent
		}{Type: "layout", LayoutEvent: e})
	case protocol.ModeEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.ModeEvent
		}{Type: "mode", ModeEvent: e})
	case protocol.TitleEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.TitleEvent
		}{Type: "title", TitleEvent: e})
	case protocol.DisconnectEvent:
		return json.Marshal(struct {
			Type string `json:"t"`
			protocol.DisconnectEvent
		}{Type: "disconnect", DisconnectEvent: e})
	default:
		return nil, fmt.Errorf("unknown event type: %T", ev)
	}
}

func DecodeEvent(data []byte) (protocol.Event, error) {
	var header struct {
		Type string `json:"t"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	switch header.Type {
	case "text":
		var ev protocol.TextEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	case "prompt":
		var ev protocol.PromptEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	case "room":
		var ev protocol.RoomEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	case "vitals":
		var ev protocol.VitalsEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	case "inventory":
		var ev protocol.InventoryEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	case "map":
		var ev protocol.MapEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	case "combat":
		var ev protocol.CombatEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	case "layout":
		var ev protocol.LayoutEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	case "mode":
		var ev protocol.ModeEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	case "title":
		var ev protocol.TitleEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	case "disconnect":
		var ev protocol.DisconnectEvent
		err := json.Unmarshal(data, &ev)
		return ev, err
	default:
		return nil, fmt.Errorf("unknown event type: %q", header.Type)
	}
}

type ClientFrame struct {
	Type    string `json:"t"`
	Command string `json:"command,omitempty"`
	Line    string `json:"line,omitempty"`
	Name    string `json:"name,omitempty"`
	Race    string `json:"race,omitempty"`
	Role    string `json:"role,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

func EncodeCommand(cmd string) ([]byte, error) {
	return json.Marshal(ClientFrame{
		Type:    "command",
		Command: cmd,
	})
}

func EncodeConnect(name, race, role string) ([]byte, error) {
	return json.Marshal(ClientFrame{
		Type: "connect",
		Name: name,
		Race: race,
		Role: role,
	})
}

func EncodeDisconnect(reason string) ([]byte, error) {
	return json.Marshal(ClientFrame{
		Type:   "disconnect",
		Reason: reason,
	})
}

func DecodeClientFrame(data []byte) (*ClientFrame, error) {
	var cf ClientFrame
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, err
	}
	if cf.Command == "" && cf.Line != "" {
		cf.Command = cf.Line
	}
	if cf.Type == "" && cf.Command != "" {
		cf.Type = "command"
	}
	return &cf, nil
}
