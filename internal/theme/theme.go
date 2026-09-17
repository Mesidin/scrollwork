// Package theme styles the TUI. ANSI-16 is the default so a terminal (and
// Omarchy's retint) already restyles the app. Hex from an Omarchy colors.toml
// or a sudengine.toml is an overlay on top of that.
package theme

import (
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
	"sudengine/internal/protocol"
)

type Palette struct {
	mu      sync.Mutex
	NoColor bool
	Mode    string // "dark" or "light"

	Room     lipgloss.Style
	RoomRule lipgloss.Style
	Alert    lipgloss.Style
	Combat   lipgloss.Style
	Say      lipgloss.Style
	System   lipgloss.Style
	Build    lipgloss.Style
	Title    lipgloss.Style
	Accent   lipgloss.Style
	Muted    lipgloss.Style
	Cursor   lipgloss.Style
	Status   lipgloss.Style
	Error    lipgloss.Style
	Banner   lipgloss.Style
	Border   lipgloss.Style

	sources []watched
}

type watched struct {
	path  string
	mtime time.Time
}

func Load() *Palette {
	p := defaults()
	p.applyFiles(discoverFiles())
	return p
}

func defaults() *Palette {
	p := &Palette{Mode: "dark"}
	p.applyHex(map[string]string{
		// ANSI-16 indexes: follow the terminal (and Omarchy retint) for free.
		"room":      "6",
		"alert":     "9",
		"combat":    "1",
		"say":       "5",
		"system":    "8",
		"build":     "3",
		"title":     "5",
		"accent":    "6",
		"muted":     "8",
		"cursor":    "6",
		"status_fg": "0",
		"status_bg": "6",
		"error":     "1",
		"banner":    "5",
		"border":    "8",
	})
	return p
}

func (p *Palette) applyHex(keys map[string]string) {
	if p.NoColor {
		plain := lipgloss.NewStyle()
		p.Room, p.RoomRule = plain, plain
		p.Alert, p.Combat, p.Say = plain, plain, plain
		p.System, p.Build, p.Title = plain, plain, plain
		p.Accent, p.Muted, p.Cursor = plain, plain, plain
		p.Error, p.Banner, p.Border = plain, plain, plain
		p.Status = plain
		return
	}
	fg := func(key string, extra ...func(lipgloss.Style) lipgloss.Style) lipgloss.Style {
		st := lipgloss.NewStyle()
		if c, ok := keys[key]; ok && c != "" {
			st = st.Foreground(lipgloss.Color(c))
		}
		for _, fn := range extra {
			st = fn(st)
		}
		return st
	}
	bold := func(s lipgloss.Style) lipgloss.Style { return s.Bold(true) }
	p.Room = fg("room", bold)
	p.RoomRule = fg("room")
	p.Alert = fg("alert", bold)
	p.Combat = fg("combat", bold)
	p.Say = fg("say")
	p.System = fg("system")
	p.Build = fg("build")
	p.Title = fg("title", bold)
	p.Accent = fg("accent", bold)
	p.Muted = fg("muted")
	p.Cursor = fg("cursor", bold)
	p.Error = fg("error", bold)
	p.Banner = fg("banner")
	p.Border = fg("border")
	status := lipgloss.NewStyle()
	if !p.NoColor {
		if c := keys["status_fg"]; c != "" {
			status = status.Foreground(lipgloss.Color(c))
		}
		if c := keys["status_bg"]; c != "" {
			status = status.Background(lipgloss.Color(c))
		}
	}
	p.Status = status
}

func (p *Palette) RoomTitle(name string) string {
	if name == "" {
		return ""
	}
	title := p.Room.Render(name)
	n := runewidth.StringWidth(name)
	if n < 3 {
		n = 3
	}
	if n > 40 {
		n = 40
	}
	rule := p.RoomRule.Render(strings.Repeat("─", n))
	return title + "\n" + rule
}

func (p *Palette) Line(ch protocol.Channel, text string) string {
	if text == "" {
		return ""
	}
	switch ch {
	case protocol.ChanRoom:
		return p.RoomTitle(text)
	case protocol.ChanAlert:
		return p.Alert.Render("▸ " + text)
	case protocol.ChanCombat:
		return p.Combat.Render(text)
	case protocol.ChanSay:
		return p.Say.Render(text)
	case protocol.ChanSystem:
		return p.System.Render(text)
	case protocol.ChanBuild:
		return p.Build.Render(text)
	default:
		return text
	}
}

func (p *Palette) Prompt(room, status string, build bool) string {
	var b strings.Builder
	if room != "" {
		b.WriteString(p.Room.Render(room))
		b.WriteString("  ")
	}
	if status != "" {
		b.WriteString(p.Muted.Render(status))
	}
	if build {
		b.WriteString(p.Build.Render(" [BUILD]"))
	}
	b.WriteString(" > ")
	return b.String()
}

func (p *Palette) StatusLine(width int, room, rest string, inCombat bool) string {
	var bits []string
	if room != "" {
		bits = append(bits, room)
	}
	if rest != "" {
		bits = append(bits, rest)
	}
	if inCombat {
		bits = append(bits, "COMBAT")
	}
	line := strings.Join(bits, "  │  ")
	st := p.Status
	if inCombat && !p.NoColor {
		st = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("9"))
	}
	if width > 0 {
		st = st.Width(width)
	}
	return st.Render(line)
}

func (p *Palette) ReloadIfChanged() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !anyChanged(p.sources) {
		return false
	}
	fresh := defaults()
	fresh.applyFiles(discoverFiles())
	p.NoColor = fresh.NoColor
	p.Mode = fresh.Mode
	p.Room = fresh.Room
	p.RoomRule = fresh.RoomRule
	p.Alert = fresh.Alert
	p.Combat = fresh.Combat
	p.Say = fresh.Say
	p.System = fresh.System
	p.Build = fresh.Build
	p.Title = fresh.Title
	p.Accent = fresh.Accent
	p.Muted = fresh.Muted
	p.Cursor = fresh.Cursor
	p.Status = fresh.Status
	p.Error = fresh.Error
	p.Banner = fresh.Banner
	p.Border = fresh.Border
	p.sources = fresh.sources
	return true
}
