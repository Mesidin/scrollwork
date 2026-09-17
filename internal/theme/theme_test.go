package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sudengine/internal/protocol"
)

func TestRoomTitleAndAlert(t *testing.T) {
	p := defaults()
	got := p.RoomTitle("Foyer")
	if !strings.Contains(got, "Foyer") {
		t.Fatalf("%q", got)
	}
	if !strings.Contains(got, "─") {
		t.Fatal("expected underline")
	}
	alert := p.Line(protocol.ChanAlert, "a rat attacks you!")
	if !strings.Contains(alert, "a rat attacks you!") {
		t.Fatalf("%q", alert)
	}
	if !strings.Contains(alert, "▸") {
		t.Fatal("expected marker")
	}
}

func TestOmarchyOverlay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "colors.toml")
	src := `
mode = "dark"
accent = "#7aa2f7"
foreground = "#a9b1d6"
background = "#1a1b26"
muted = "#414868"
red = "#f7768e"
color9 = "#ff7a93"
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUDENGINE_THEME", path)
	t.Setenv("NO_COLOR", "")
	p := defaults()
	p.applyFiles([]string{path})
	room := p.Line(protocol.ChanRoom, "Foyer")
	if !strings.Contains(room, "Foyer") {
		t.Fatal(room)
	}
}

func TestNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	p := Load()
	if !p.NoColor {
		t.Fatal("expected NoColor")
	}
	if strings.Contains(p.Line(protocol.ChanAlert, "hi"), "\x1b") {
		t.Fatal("ansi leaked")
	}
}

func TestOverlaySemantics(t *testing.T) {
	out := overlaySemantics(map[string]string{
		"accent": "#abcabc",
		"red":    "#ff0000",
		"room":   "#00ff00",
	})
	if out["room"] != "#00ff00" {
		t.Fatalf("room %q", out["room"])
	}
	if out["combat"] != "#ff0000" {
		t.Fatalf("combat %q", out["combat"])
	}
	if out["accent"] != "#abcabc" {
		t.Fatalf("accent %q", out["accent"])
	}
}
