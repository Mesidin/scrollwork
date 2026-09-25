package client

import (
	"fmt"
	"strings"

	"sudengine/internal/docs"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/save"
	"sudengine/internal/server"

	tea "charm.land/bubbletea/v2"
)

type introAction int

const (
	actNew introAction = iota
	actContinue
	actEnterBuild
	actHelp
	actBack
)

type introItem struct {
	label  string
	action introAction
}

func (m Model) openIntro(info pack.Info, mode protocol.Mode) (tea.Model, tea.Cmd) {
	p, err := pack.Load(info.Dir)
	if err != nil {
		m.errLine = err.Error()
		return m, nil
	}
	saves, _ := save.ListInfo(p.Meta.ID)
	m.info = info
	m.pack = p
	m.saves = saves
	m.mode = mode
	m.state = stateIntro
	m.menuCursor = 0
	m.title = p.Meta.Title
	m.errLine = ""
	m.helpTopic = -1
	m.helpFromGame = false
	return m, nil
}

func (m Model) introItems() []introItem {
	if m.mode == protocol.ModeBuild {
		return []introItem{
			{label: "Enter builder", action: actEnterBuild},
			{label: "Help", action: actHelp},
			{label: "Back", action: actBack},
		}
	}
	items := []introItem{{label: "New game", action: actNew}}
	if len(m.saves) > 0 {
		items = append(items, introItem{
			label:  fmt.Sprintf("Continue (%d save%s)", len(m.saves), plural(len(m.saves))),
			action: actContinue,
		})
	}
	items = append(items,
		introItem{label: "Help", action: actHelp},
		introItem{label: "Back", action: actBack},
	)
	return items
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (m Model) updateIntro(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := m.introItems()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		return m.backToLauncher()
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j":
		if m.menuCursor < len(items)-1 {
			m.menuCursor++
		}
	case "n":
		if m.mode == protocol.ModePlay {
			return m.beginNew()
		}
	case "c":
		if m.mode == protocol.ModePlay && len(m.saves) > 0 {
			return m.openContinue()
		}
	case "h":
		return m.openHelp()
	case "enter":
		if m.menuCursor < 0 || m.menuCursor >= len(items) {
			return m, nil
		}
		switch items[m.menuCursor].action {
		case actNew:
			return m.beginNew()
		case actContinue:
			return m.openContinue()
		case actEnterBuild:
			return m.startGame(server.Options{PlayerName: "you"})
		case actHelp:
			return m.openHelp()
		case actBack:
			return m.backToLauncher()
		}
	}
	return m, nil
}

func (m Model) backToLauncher() (tea.Model, tea.Cmd) {
	m.state = stateLauncher
	m.pack = nil
	m.errLine = ""
	m.menuCursor = 0
	m.title = "Erickson Stories"
	games, _ := pack.Discover(m.gamesDir)
	m.games = games
	return m, nil
}

func (m Model) beginNew() (tea.Model, tea.Cmd) {
	if m.pack != nil && m.pack.RPG.ChargenEnabled() {
		return m.openChargen()
	}
	return m.startGame(server.Options{})
}

func (m Model) openContinue() (tea.Model, tea.Cmd) {
	m.state = stateContinue
	m.menuCursor = 0
	return m, nil
}

func (m Model) updateContinue(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.state = stateIntro
		m.menuCursor = 0
		return m, nil
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j":
		if m.menuCursor < len(m.saves)-1 {
			m.menuCursor++
		}
	case "enter":
		if m.menuCursor < 0 || m.menuCursor >= len(m.saves) {
			return m, nil
		}
		return m.startGame(server.Options{Slot: m.saves[m.menuCursor].Name})
	}
	return m, nil
}

func (m Model) openChargen() (tea.Model, tea.Cmd) {
	m.state = stateChargen
	m.cgStep = 0
	m.cgName = ""
	m.cgRace = 0
	m.cgRole = 0
	m.input.Reset()
	m.input.Prompt = "Name: "
	m.input.Placeholder = "who are you?"
	m.input.Focus()
	return m, m.input.Focus()
}

func (m Model) chargenSteps() []int {
	var steps []int
	steps = append(steps, 0) // name
	if m.pack != nil && len(m.pack.RPG.Races) > 0 {
		steps = append(steps, 1)
	}
	if m.pack != nil && len(m.pack.RPG.Roles) > 0 {
		steps = append(steps, 2)
	}
	return steps
}

func (m Model) updateChargen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	steps := m.chargenSteps()
	cur := m.cgStep
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.input.Prompt = "> "
		m.input.Placeholder = "look, n, get lamp..."
		m.input.Reset()
		m.state = stateIntro
		m.menuCursor = 0
		return m, nil
	case "enter":
		if cur == 0 {
			name := strings.TrimSpace(m.input.Value())
			if name == "" {
				name = "you"
			}
			m.cgName = name
			m.input.Reset()
		}
		idx := indexOf(steps, cur)
		if idx < 0 || idx+1 >= len(steps) {
			return m.finishChargen()
		}
		m.cgStep = steps[idx+1]
		if m.cgStep == 0 {
			m.input.Prompt = "Name: "
		} else {
			m.input.Prompt = ""
		}
		return m, nil
	case "up", "k":
		if cur == 1 && m.cgRace > 0 {
			m.cgRace--
			return m, nil
		}
		if cur == 2 && m.cgRole > 0 {
			m.cgRole--
			return m, nil
		}
	case "down", "j":
		if cur == 1 && m.pack != nil && m.cgRace < len(m.pack.RPG.Races)-1 {
			m.cgRace++
			return m, nil
		}
		if cur == 2 && m.pack != nil && m.cgRole < len(m.pack.RPG.Roles)-1 {
			m.cgRole++
			return m, nil
		}
	}
	if cur == 0 {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) finishChargen() (tea.Model, tea.Cmd) {
	opt := server.Options{PlayerName: m.cgName}
	if m.pack != nil {
		if len(m.pack.RPG.Races) > 0 && m.cgRace >= 0 && m.cgRace < len(m.pack.RPG.Races) {
			opt.RaceID = m.pack.RPG.Races[m.cgRace].ID
		}
		if len(m.pack.RPG.Roles) > 0 && m.cgRole >= 0 && m.cgRole < len(m.pack.RPG.Roles) {
			opt.RoleID = m.pack.RPG.Roles[m.cgRole].ID
		}
	}
	m.input.Prompt = "> "
	m.input.Placeholder = "look, n, get lamp..."
	m.input.Reset()
	return m.startGame(opt)
}

func indexOf(steps []int, v int) int {
	for i, s := range steps {
		if s == v {
			return i
		}
	}
	return -1
}

func (m Model) viewIntro() string {
	var b strings.Builder
	art := ""
	if m.pack != nil && m.pack.IntroArt != "" {
		art = m.pack.IntroArt
	} else {
		art = docs.EngineBanner()
	}
	b.WriteString(m.theme.Banner.Render(art))
	b.WriteString("\n\n")
	title := m.title
	if m.pack != nil && m.pack.Meta.Title != "" {
		title = m.pack.Meta.Title
	}
	b.WriteString(m.theme.Title.Render(title))
	if m.pack != nil && m.pack.Meta.Intro.Subtitle != "" {
		b.WriteString("  —  " + m.pack.Meta.Intro.Subtitle)
	}
	if m.mode == protocol.ModeBuild {
		b.WriteString(m.theme.Build.Render("  [BUILD]"))
	}
	b.WriteString("\n")
	if m.pack != nil && m.pack.Meta.Author != "" {
		b.WriteString("by " + m.pack.Meta.Author + "\n")
	}
	if m.pack != nil && strings.TrimSpace(m.pack.Meta.Intro.Blurb) != "" {
		b.WriteString("\n" + strings.TrimSpace(m.pack.Meta.Intro.Blurb) + "\n")
	}
	b.WriteString("\n")
	items := m.introItems()
	for i, it := range items {
		cur := "  "
		if i == m.menuCursor {
			cur = "> "
		}
		line := cur + it.label
		if i == m.menuCursor {
			line = m.theme.Cursor.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if m.errLine != "" {
		b.WriteString("\n" + m.theme.Error.Render(m.errLine) + "\n")
	}
	b.WriteString("\n↑↓ move   enter select   h help   esc back\n")
	return b.String()
}

func (m Model) viewContinue() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("Continue") + "\n\n")
	if len(m.saves) == 0 {
		b.WriteString("No saves.\n")
	}
	for i, s := range m.saves {
		cur := "  "
		if i == m.menuCursor {
			cur = "> "
		}
		when := ""
		if !s.ModTime.IsZero() {
			when = "  " + s.ModTime.Format("2006-01-02 15:04")
		}
		line := fmt.Sprintf("%s%s%s", cur, s.Name, when)
		if i == m.menuCursor {
			line = m.theme.Cursor.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nenter load   esc back\n")
	return b.String()
}

func (m Model) viewChargen() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("New character") + "\n\n")
	switch m.cgStep {
	case 0:
		b.WriteString("What do they call you?\n\n")
		b.WriteString(m.input.View() + "\n")
	case 1:
		b.WriteString("Choose a " + raceWord(m.pack) + ":\n\n")
		if m.pack != nil {
			for i, r := range m.pack.RPG.Races {
				cur := "  "
				if i == m.cgRace {
					cur = "> "
				}
				line := cur + r.Name
				if r.Description != "" {
					line += "  —  " + r.Description
				}
				if i == m.cgRace {
					line = m.theme.Cursor.Render(line)
				}
				b.WriteString(line + "\n")
			}
		}
	case 2:
		b.WriteString("Choose a " + roleWord(m.pack) + ":\n\n")
		if m.pack != nil {
			for i, r := range m.pack.RPG.Roles {
				cur := "  "
				if i == m.cgRole {
					cur = "> "
				}
				line := cur + r.Name
				if r.Description != "" {
					line += "  —  " + r.Description
				}
				if i == m.cgRole {
					line = m.theme.Cursor.Render(line)
				}
				b.WriteString(line + "\n")
			}
		}
	}
	b.WriteString("\nenter next   esc back\n")
	return b.String()
}

func raceWord(p *pack.Pack) string {
	if p == nil {
		return "origin"
	}
	if l := p.Lexicon.Label("race"); l != "race" {
		return strings.ToLower(l)
	}
	return "origin"
}

func roleWord(p *pack.Pack) string {
	if p == nil {
		return "temperament"
	}
	if l := p.Lexicon.Label("class"); l != "class" {
		return strings.ToLower(l)
	}
	if l := p.Lexicon.Label("role"); l != "role" {
		return strings.ToLower(l)
	}
	return "temperament"
}
