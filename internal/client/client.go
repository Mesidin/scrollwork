package client

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"sudengine/internal/docs"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/save"
	"sudengine/internal/server"
	"sudengine/internal/theme"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type state int

const (
	stateLauncher state = iota
	stateIntro
	stateContinue
	stateChargen
	stateHelp
	stateGame
)

type Model struct {
	state  state
	width  int
	height int

	games    []pack.Info
	cursor   int
	gamesDir string

	input   textinput.Model
	log     viewport.Model
	side    viewport.Model
	lines   []string
	history []string
	histIdx int

	prompt string
	room   protocol.RoomEvent
	vitals protocol.VitalsEvent
	inv    protocol.InventoryEvent
	cmap   protocol.MapEvent
	combat protocol.CombatEvent
	layout []string
	mode   protocol.Mode
	title  string

	sess   *server.Session
	srv    *server.Server
	cancel context.CancelFunc

	info       pack.Info
	pack       *pack.Pack
	saves      []save.SlotInfo
	menuCursor int
	errLine    string

	helpTopics []docs.Topic
	helpBody   string

	cgStep int // 0 name, 1 race, 2 role
	cgName string
	cgRace int
	cgRole int

	theme *theme.Palette
}

func New(gamesDir string) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "look, n, get lamp..."
	ti.CharLimit = 240
	ti.Focus()

	log := viewport.New()
	log.SoftWrap = true
	log.MouseWheelEnabled = true
	side := viewport.New()
	side.SoftWrap = true

	games, _ := pack.Discover(gamesDir)
	m := Model{
		state:    stateLauncher,
		games:    games,
		gamesDir: gamesDir,
		input:    ti,
		log:      log,
		side:     side,
		layout:   []string{"output", "map", "vitals"},
		prompt:   "> ",
		title:    "Erickson Stories",
		theme:    theme.Load(),
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.input.Focus()}
	if m.sess != nil {
		cmds = append(cmds, waitEvent(m.sess))
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.theme != nil {
		m.theme.ReloadIfChanged()
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layoutPanes()
		return m, nil
	case tea.KeyPressMsg:
		switch m.state {
		case stateLauncher:
			return m.updateLauncher(msg)
		case stateIntro:
			return m.updateIntro(msg)
		case stateContinue:
			return m.updateContinue(msg)
		case stateChargen:
			return m.updateChargen(msg)
		case stateHelp:
			return m.updateHelp(msg)
		default:
			return m.updateGameKey(msg)
		}
	case protocol.TextEvent:
		m.appendChan(msg.Channel, msg.Text)
		return m, waitEvent(m.sess)
	case protocol.PromptEvent:
		m.prompt = msg.Text
		if m.theme != nil && msg.Room != "" {
			m.input.Prompt = m.theme.Prompt(msg.Room, msg.Status, msg.Build)
		} else {
			m.input.Prompt = msg.Text
		}
		return m, waitEvent(m.sess)
	case protocol.RoomEvent:
		m.room = msg
		m.refreshSide()
		return m, waitEvent(m.sess)
	case protocol.VitalsEvent:
		m.vitals = msg
		m.refreshSide()
		return m, waitEvent(m.sess)
	case protocol.InventoryEvent:
		m.inv = msg
		m.refreshSide()
		return m, waitEvent(m.sess)
	case protocol.MapEvent:
		m.cmap = msg
		m.refreshSide()
		return m, waitEvent(m.sess)
	case protocol.CombatEvent:
		m.combat = msg
		m.refreshSide()
		return m, waitEvent(m.sess)
	case protocol.LayoutEvent:
		if len(msg.Panes) > 0 {
			m.layout = msg.Panes
		}
		m.layoutPanes()
		return m, waitEvent(m.sess)
	case protocol.ModeEvent:
		m.mode = msg.Mode
		return m, waitEvent(m.sess)
	case protocol.TitleEvent:
		m.title = msg.Title
		return m, waitEvent(m.sess)
	case protocol.DisconnectEvent:
		m.append("Disconnected: " + msg.Reason)
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	if m.state == stateGame {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateLauncher(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.games)-1 {
			m.cursor++
		}
	case "enter", "p":
		if len(m.games) == 0 {
			return m, nil
		}
		return m.openIntro(m.games[m.cursor], protocol.ModePlay)
	case "b":
		if len(m.games) == 0 {
			return m, nil
		}
		return m.openIntro(m.games[m.cursor], protocol.ModeBuild)
	case "n":
		return m.newPack()
	}
	return m, nil
}

func (m Model) newPack() (tea.Model, tea.Cmd) {
	id := fmt.Sprintf("game-%d", len(m.games)+1)
	dir := filepath.Join(m.gamesDir, id)
	p := pack.Blank(id, dir)
	if err := p.Write(); err != nil {
		m.append("new pack failed: " + err.Error())
		return m, nil
	}
	games, _ := pack.Discover(m.gamesDir)
	m.games = games
	for i, g := range games {
		if g.ID == p.Meta.ID {
			m.cursor = i
			return m.openIntro(g, protocol.ModeBuild)
		}
	}
	return m, nil
}

func (m Model) startGame(opt server.Options) (tea.Model, tea.Cmd) {
	opt.PackDir = m.info.Dir
	opt.Mode = m.mode
	srv, sess, err := server.Launch(opt)
	if err != nil {
		m.errLine = err.Error()
		m.state = stateIntro
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.srv = srv
	m.sess = sess
	m.cancel = cancel
	m.state = stateGame
	m.title = m.info.Title
	m.errLine = ""
	m.lines = nil
	m.log.SetContent("")
	m.input.Prompt = "> "
	m.input.Placeholder = "look, n, get lamp..."
	m.input.Reset()
	go srv.Run(ctx)
	m.layoutPanes()
	return m, tea.Batch(waitEvent(sess), m.input.Focus())
}

func (m Model) updateGameKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.sess != nil {
			select {
			case m.sess.In <- "quit":
			default:
			}
		}
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	case "enter":
		line := strings.TrimSpace(m.input.Value())
		m.input.Reset()
		if line == "" {
			return m, nil
		}
		m.history = append(m.history, line)
		m.histIdx = len(m.history)
		m.appendChan(protocol.ChanSystem, "> "+line)
		if m.sess != nil {
			m.sess.In <- line
		}
		return m, nil
	case "up":
		if len(m.history) == 0 {
			break
		}
		if m.histIdx > 0 {
			m.histIdx--
		}
		m.input.SetValue(m.history[m.histIdx])
		m.input.CursorEnd()
		return m, nil
	case "down":
		if m.histIdx < len(m.history)-1 {
			m.histIdx++
			m.input.SetValue(m.history[m.histIdx])
		} else {
			m.histIdx = len(m.history)
			m.input.Reset()
		}
		m.input.CursorEnd()
		return m, nil
	case "pgup":
		m.log.ScrollUp(m.log.Height() / 2)
		return m, nil
	case "pgdown":
		m.log.ScrollDown(m.log.Height() / 2)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) append(text string) {
	m.appendChan(protocol.ChanNarrative, text)
}

func (m *Model) appendChan(ch protocol.Channel, text string) {
	rendered := text
	if m.theme != nil {
		rendered = m.theme.Line(ch, text)
	}
	if ch == protocol.ChanAlert && len(m.lines) > 0 && m.lines[len(m.lines)-1] != "" {
		m.lines = append(m.lines, "")
	}
	for _, line := range strings.Split(rendered, "\n") {
		m.lines = append(m.lines, line)
	}
	if len(m.lines) > 2000 {
		m.lines = m.lines[len(m.lines)-2000:]
	}
	atBottom := m.log.AtBottom()
	m.log.SetContent(strings.Join(m.lines, "\n"))
	if atBottom {
		m.log.GotoBottom()
	}
}

func (m *Model) layoutPanes() {
	if m.width < 1 || m.height < 1 {
		return
	}
	if m.state != stateGame {
		h := m.height - 3
		if h < 5 {
			h = 5
		}
		m.log.SetWidth(m.width)
		m.log.SetHeight(h)
		m.input.SetWidth(m.width - 1)
		return
	}
	status := 1
	prompt := 1
	innerH := m.height - status - prompt
	if innerH < 3 {
		innerH = 3
	}
	sideOn := m.showSide()
	sideW := 0
	if sideOn && m.width > 60 {
		sideW = m.width / 3
		if sideW < 24 {
			sideW = 24
		}
		if sideW > 36 {
			sideW = 36
		}
	}
	logW := m.width - sideW
	if logW < 20 {
		logW = m.width
		sideW = 0
	}
	m.log.SetWidth(logW)
	m.log.SetHeight(innerH)
	if sideW > 0 {
		m.side.SetWidth(sideW)
		m.side.SetHeight(innerH)
	}
	m.input.SetWidth(m.width - 1)
	m.refreshSide()
}

func (m Model) showSide() bool {
	for _, p := range m.layout {
		if p == "map" || p == "vitals" || p == "inventory" || p == "combat" || p == "exits" {
			return true
		}
	}
	return false
}

func (m *Model) refreshSide() {
	var b strings.Builder
	want := func(name string) bool {
		for _, p := range m.layout {
			if p == name {
				return true
			}
		}
		return false
	}
	if want("vitals") || want("output") {
		b.WriteString(m.section("status"))
		if m.room.Title != "" {
			b.WriteString(m.room.Title + "\n")
		}
		for _, r := range m.vitals.Resources {
			b.WriteString(fmt.Sprintf("%s %d/%d\n", r.Label, r.Current, r.Max))
		}
		if m.vitals.InCombat {
			b.WriteString("IN COMBAT\n")
		}
		if len(m.vitals.Flags) > 0 {
			b.WriteString(strings.Join(m.vitals.Flags, ", ") + "\n")
		}
	}
	if want("exits") {
		b.WriteString(m.section("exits"))
		if len(m.room.Exits) == 0 {
			b.WriteString("none\n")
		} else {
			for _, e := range m.room.Exits {
				extra := ""
				if e.Locked {
					extra = " locked"
				} else if e.Closed {
					extra = " closed"
				}
				b.WriteString(e.Dir + extra + "\n")
			}
		}
	}
	if want("map") {
		b.WriteString(m.section("map"))
		if m.cmap.Text != "" {
			b.WriteString(m.cmap.Text)
			b.WriteByte('\n')
		} else {
			b.WriteString("No map.\n")
		}
	}
	if want("inventory") {
		b.WriteString(m.section("inventory"))
		if len(m.inv.Items) == 0 {
			b.WriteString("(empty)\n")
		}
		for _, it := range m.inv.Items {
			b.WriteString("• " + it + "\n")
		}
	}
	if want("combat") && m.combat.Active {
		b.WriteString(m.section("combat"))
		for _, f := range m.combat.Fighters {
			mark := " "
			if f.IsPlayer {
				mark = "*"
			}
			b.WriteString(fmt.Sprintf("%s %s %d/%d\n", mark, f.Name, f.HP, f.Max))
		}
	}
	m.side.SetContent(strings.TrimRight(b.String(), "\n"))
}

func (m Model) section(title string) string {
	st := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	if m.theme != nil {
		st = m.theme.Accent
	}
	return st.Render(strings.ToUpper(title)) + "\n"
}

func (m Model) View() tea.View {
	var body string
	switch m.state {
	case stateLauncher:
		body = m.viewLauncher()
	case stateIntro:
		body = m.viewIntro()
	case stateContinue:
		body = m.viewContinue()
	case stateChargen:
		body = m.viewChargen()
	case stateHelp:
		body = m.viewHelp()
	default:
		body = m.viewGame()
	}
	v := tea.NewView(body)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.ReportFocus = true
	v.WindowTitle = m.title
	return v
}

func (m Model) viewLauncher() string {
	var b strings.Builder
	head := m.theme.Title.Render("ERICKSON STORIES")
	b.WriteString(head + "\n")
	b.WriteString("A text world engine. Enter play, b build, n new pack, q quit.\n\n")
	if len(m.games) == 0 {
		b.WriteString("No game packs in " + m.gamesDir + ".\nPress n to create one.\n")
	}
	for i, g := range m.games {
		cur := "  "
		if i == m.cursor {
			cur = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s  (%s)\n", cur, g.Title, g.ID))
	}
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewGame() string {
	status := m.statusLine()
	logView := m.log.View()
	if m.showSide() && m.side.Width() > 0 {
		left := paneStyle().Width(m.log.Width()).Height(m.log.Height()).Render(logView)
		right := m.sideStyle().Width(m.side.Width()).Height(m.side.Height()).Render(m.side.View())
		logView = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	prompt := m.input.View()
	return lipgloss.JoinVertical(lipgloss.Left, logView, status, prompt)
}

func (m Model) statusLine() string {
	var rest []string
	for _, r := range m.vitals.Resources {
		rest = append(rest, fmt.Sprintf("%s %d/%d", r.Label, r.Current, r.Max))
	}
	if m.mode == protocol.ModeBuild {
		rest = append(rest, "BUILD")
	}
	if m.theme == nil {
		return strings.Join(append([]string{m.room.Title}, rest...), "  │  ")
	}
	return m.theme.StatusLine(m.width, m.room.Title, strings.Join(rest, "  │  "), m.vitals.InCombat)
}

func paneStyle() lipgloss.Style {
	return lipgloss.NewStyle()
}

func (m Model) sideStyle() lipgloss.Style {
	st := lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).PaddingLeft(1)
	if m.theme != nil {
		st = st.BorderForeground(m.theme.Border.GetForeground())
	}
	return st
}

func waitEvent(sess *server.Session) tea.Cmd {
	if sess == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-sess.Out
		if !ok {
			return protocol.DisconnectEvent{Reason: "closed"}
		}
		return ev
	}
}

func DirectPlay(gamesDir, packID string, mode protocol.Mode) (Model, error) {
	games, err := pack.Discover(gamesDir)
	if err != nil {
		return Model{}, err
	}
	var info pack.Info
	found := false
	for _, g := range games {
		if g.ID == packID || g.Dir == packID {
			info = g
			found = true
			break
		}
	}
	if !found {
		p, err := pack.Load(packID)
		if err != nil {
			return Model{}, fmt.Errorf("unknown pack %q", packID)
		}
		info = pack.Info{ID: p.Meta.ID, Title: p.Meta.Title, Dir: p.Dir}
	}
	m := New(gamesDir)
	nm, _ := m.openIntro(info, mode)
	return nm.(Model), nil
}
