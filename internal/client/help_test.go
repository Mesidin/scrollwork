package client

import (
	"path/filepath"
	"strings"
	"testing"

	"sudengine/internal/docs"
	"sudengine/internal/pack"

	"github.com/charmbracelet/x/ansi"
)

func testHelp(t *testing.T) Model {
	t.Helper()
	m := New(filepath.Join("..", "..", "games"))
	p, err := pack.Load(filepath.Join("..", "..", "games", "old-house"))
	if err != nil {
		t.Fatal(err)
	}
	m.pack = p
	m.width = 80
	m.height = 24
	next, _ := m.openHelp()
	return next.(Model)
}

func TestHelpViewFitsWindow(t *testing.T) {
	m := testHelp(t)
	assertHelpFrame(t, m, 80, 24)
	m.width, m.height = 80, 10
	assertHelpFrame(t, m, 80, 10)
	if !strings.Contains(m.viewHelp(), "█") {
		t.Fatal("short window should show a scrollbar thumb")
	}
	m.width, m.height = 40, 20
	assertHelpFrame(t, m, 40, 20)
}

func TestHelpBuildingSection(t *testing.T) {
	m := testHelp(t)
	next, _ := m.openHelpQuery("building", "rooms")
	hm := next.(Model)
	if hm.helpFromGame != true {
		t.Fatal("expected game-origin help")
	}
	view := hm.viewHelp()
	if !strings.Contains(view, "kitchen") {
		t.Fatalf("rooms page missing example:\n%s", view)
	}
	if strings.Contains(view, "on_tick") {
		t.Fatal("lua page leaked into rooms")
	}
	var gameSystems, engineSystems, building bool
	for _, topic := range hm.helpTopics {
		if topic.Key == "systems" && topic.Source == docs.SourceGame {
			gameSystems = true
		}
		if topic.Key == "systems" && topic.Source == docs.SourceEngine {
			engineSystems = true
		}
		if topic.Key == "building" && topic.Source == docs.SourceEngine {
			building = true
		}
	}
	if !gameSystems || !engineSystems || !building {
		t.Fatalf("game systems %v engine systems %v building %v", gameSystems, engineSystems, building)
	}
	low := strings.ToLower(view)
	if !strings.Contains(low, "game") || !strings.Contains(low, "engine") {
		t.Fatal("menu missing game and engine groups")
	}
}

func TestHelpQuery(t *testing.T) {
	topic, section, ok := helpQuery("help building rooms")
	if !ok || topic != "building" || section != "rooms" {
		t.Fatalf("%q %q %v", topic, section, ok)
	}
	if _, _, ok := helpQuery("look"); ok {
		t.Fatal("look is not help")
	}
}

func assertHelpFrame(t *testing.T, m Model, width, height int) {
	t.Helper()
	m.width = width
	m.height = height
	view := m.viewHelp()
	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Fatalf("%dx%d help is %d lines", width, height, len(lines))
	}
	for i, ln := range lines {
		if w := ansi.StringWidth(ln); w > width {
			t.Fatalf("line %d width %d: %q", i, w, ln)
		}
	}
}
