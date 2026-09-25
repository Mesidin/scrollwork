package client

import (
	"strings"
	"testing"

	"sudengine/internal/protocol"
)

func TestPileResourceIsAValue(t *testing.T) {
	m := New(t.TempDir())
	m.width = 80
	m.vitals.Resources = []protocol.ResourceView{
		{Label: "Vigor", Current: 10, Max: 20, Show: "bar"},
		{Label: "Focus", Current: 4, Max: 8, Show: "side"},
		{Label: "Coin", Current: 27, Max: 12, Pile: true, Show: "side"},
	}
	got := m.statusLine()
	if !strings.Contains(got, "Vigor") || !strings.Contains(got, "█") || !strings.Contains(got, "10/20") {
		t.Fatalf("status bar: %s", got)
	}
	if strings.Contains(got, "Focus") || strings.Contains(got, "Coin") {
		t.Fatalf("quiet resources leaked onto the status line: %s", got)
	}
	if m.vitals.Resources[2].Text() != "Coin 27" {
		t.Fatal(m.vitals.Resources[2].Text())
	}
}

func TestBuildSideMapAndCommands(t *testing.T) {
	m := New(t.TempDir())
	m.mode = protocol.ModeBuild
	m.layout = []string{"output"}
	if !m.showSide() {
		t.Fatal("build mode should keep a side pane")
	}
	m.room = protocol.RoomEvent{ID: "house.kitchen", Title: "Kitchen"}
	m.cmap.Text = "  @  "
	got := m.buildSide()
	for _, want := range []string{"Kitchen", "house.kitchen", "@ this room", "  @  ", "dig n Name", "save pack", "buildwalk", "help building"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "INVENTORY") || strings.Contains(got, "IN COMBAT") {
		t.Fatalf("play panes leaked into build pane:\n%s", got)
	}

	m.mode = protocol.ModePlay
	if m.showSide() {
		t.Fatal("play mode with only the output pane should not open a side pane")
	}
}
