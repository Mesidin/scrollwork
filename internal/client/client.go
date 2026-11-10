package client

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"sudengine/internal/docs"
	snet "sudengine/internal/net"
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
	stateConnect
	stateRemoteLogin
	stateGame
)

type Model struct {
	state  state
	width  int
	height int

	games      []pack.Info
	savedConns []save.Connection
	cursor     int
	gamesDir   string

	discovered    []snet.ServerInfo
	scanning      bool
	connectTyping bool

	remoteAddr   string
	remoteInfo   snet.ServerInfo
	rlStep       int
	rlAccount    string
	rlPassword   string
	rlCharName   string
	rlRace       int
	rlRole       int
	rlPoints     int
	rlAttrs      map[string]int
	rlAttrIdx    int
	dialing      bool
	dialNote     string
	dialGen      int
	pendingConn  *save.Connection
	remoteJoined bool
	remoteNotice string
	dialHome     state
	dialStep     int

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

	helpTopics   []docs.Topic
	helpTopic    int
	helpSection  int
	helpBodyY    int
	helpMenuY    int
	helpFromGame bool

	cgStep int // 0 name, 1 race, 2 role
	cgName string
	cgRace int
	cgRole int

	theme *theme.Palette

	pwStep    int
	pwOld     string
	pwPending string
	pwPrompt  string
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
	conns, _ := save.LoadConnections()
	m := Model{
		state:      stateLauncher,
		games:      games,
		savedConns: conns,
		gamesDir:   gamesDir,
		input:      ti,
		log:        log,
		side:       side,
		layout:     []string{"output", "map", "vitals"},
		prompt:     "> ",
		title:      "Erickson Stories",
		theme:      theme.Load(),
		helpTopic:  -1,
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
	case serversScannedMsg:
		m.scanning = false
		m.discovered = msg.servers
		return m, nil
	case probeResultMsg:
		if msg.gen != m.dialGen {
			return m, nil
		}
		m.finishNetOp()
		if msg.err != nil {
			m.errLine = fmt.Sprintf("Could not reach %s: %v", msg.addr, msg.err)
			return m, nil
		}
		m = m.openRemoteLogin(msg.addr, msg.info)
		return m, m.input.Focus()
	case authResultMsg:
		if msg.gen != m.dialGen {
			return m, nil
		}
		m.finishNetOp()
		if msg.err != nil {
			m.errLine = msg.err.Error()
			return m, m.input.Focus()
		}
		m.input.EchoMode = textinput.EchoNormal
		m.rlPassword = msg.password
		m.remoteInfo = msg.info
		m.errLine = ""
		if len(msg.info.Characters) > 0 {
			m.rlStep = rlStepCharSelect
			m.menuCursor = 0
			return m, nil
		}
		m.rlStep = rlStepCharName
		m.input.Prompt = "Character name: "
		m.input.Placeholder = "who are you?"
		m.input.Reset()
		return m, m.input.Focus()
	case remoteDialMsg:
		if msg.gen != m.dialGen {
			if msg.cleanup != nil {
				msg.cleanup()
			}
			if msg.cancel != nil {
				msg.cancel()
			}
			return m, nil
		}
		m.dialing = false
		m.dialNote = ""
		if msg.err != nil {
			if msg.cancel != nil {
				msg.cancel()
			}
			m.cancel = nil
			m.pendingConn = nil
			m.errLine = fmt.Sprintf("Connect failed (%s): %v", m.remoteAddr, msg.err)
			return m, nil
		}
		parent := msg.cancel
		cleanup := msg.cleanup
		m.cancel = func() {
			if cleanup != nil {
				cleanup()
			}
			if parent != nil {
				parent()
			}
		}
		m.srv = nil
		m.sess = msg.sess
		m.state = stateGame
		m.remoteJoined = false
		m.lines = nil
		m.room = protocol.RoomEvent{}
		m.log.SetContent("")
		m.input.EchoMode = textinput.EchoNormal
		m.input.Prompt = "> "
		m.input.Placeholder = "look, n, get lamp..."
		m.input.Reset()
		m.layoutPanes()
		return m, tea.Batch(waitEvent(msg.sess), m.input.Focus())
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layoutPanes()
		if m.state == stateHelp {
			m.revealHelpMenu()
			m = m.scrollHelpBody(0)
		}
		return m, nil
	case tea.MouseWheelMsg:
		if m.state == stateHelp {
			return m.wheelHelp(msg)
		}
		if m.state == stateGame {
			return m.wheelGame(msg)
		}
		return m, nil
	case tea.MouseClickMsg:
		if m.state == stateHelp {
			return m.clickHelp(msg)
		}
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
		case stateConnect:
			return m.updateConnect(msg)
		case stateRemoteLogin:
			return m.updateRemoteLogin(msg)
		default:
			return m.updateGameKey(msg)
		}
	case protocol.TextEvent:
		if m.pendingConn != nil && !m.remoteJoined && msg.Text != "" {
			m.remoteNotice = msg.Text
		}
		m = m.notePasswordUpdated(msg)
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
		m.noteRemoteJoined()
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
		m.layoutPanes()
		return m, waitEvent(m.sess)
	case protocol.TitleEvent:
		m.title = msg.Title
		return m, waitEvent(m.sess)
	case protocol.DisconnectEvent:
		if m.pendingConn != nil && !m.remoteJoined {
			return m.refuseRemote(msg.Reason)
		}
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
	if m.dialing {
		return m.cancelNetOp(msg)
	}
	items := m.launcherItems()
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(items)-1 {
			m.cursor++
		}
	case "c":
		return m.openConnect()
	case "d", "x", "delete", "backspace":
		if m.cursor >= 0 && m.cursor < len(items) && items[m.cursor].kind == launcherSaved {
			_ = save.RemoveConnection(items[m.cursor].conn)
			m.savedConns, _ = save.LoadConnections()
			newItems := m.launcherItems()
			if m.cursor >= len(newItems) {
				m.cursor = len(newItems) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
		}
	case "enter", "p":
		if m.cursor < 0 || m.cursor >= len(items) {
			return m, nil
		}
		it := items[m.cursor]
		switch it.kind {
		case launcherPack:
			return m.openIntro(it.pack, protocol.ModePlay)
		case launcherSaved:
			return m.connectSaved(it.conn)
		case launcherConnect:
			return m.openConnect()
		}
	case "b":
		if m.cursor >= 0 && m.cursor < len(items) && items[m.cursor].kind == launcherPack {
			return m.openIntro(items[m.cursor].pack, protocol.ModeBuild)
		}
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
	if m.pwStep != 0 {
		return m.updatePasswordKey(msg)
	}
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
		if next, handled := m.interceptPassword(line); handled {
			return next, nil
		}
		m.history = append(m.history, line)
		m.histIdx = len(m.history)
		m.appendChan(protocol.ChanSystem, "> "+line)
		if topic, section, ok := helpQuery(line); ok {
			return m.openHelpQuery(topic, section)
		}
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

func (m Model) interceptPassword(line string) (Model, bool) {
	if protocol.IsPasswordChange(line) {
		m.appendChan(protocol.ChanSystem, "Type password and press enter. You will be asked for the current and new passwords.")
		return m, true
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return m, false
	}
	verb := strings.ToLower(fields[0])
	if verb != "password" && verb != "passwd" {
		return m, false
	}
	m.appendChan(protocol.ChanSystem, "> password")
	if len(fields) > 1 {
		m.appendChan(protocol.ChanSystem, "Type password and press enter. You will be asked for the current and new passwords.")
		return m, true
	}
	if m.pwPending != "" {
		m.appendChan(protocol.ChanSystem, "A password change is already in progress.")
		return m, true
	}
	if m.sess != nil && m.sess.Account != "" {
		m.pwPrompt = m.input.Prompt
		m.pwStep = 1
		m.pwOld = ""
		m.input.EchoMode = textinput.EchoPassword
		m.input.Prompt = "Current password: "
		m.input.Placeholder = ""
		m.input.Reset()
		return m, true
	}
	if m.sess != nil {
		m.sess.In <- "password"
	}
	return m, true
}

func (m Model) updatePasswordKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.pwOld = ""
		m.pwStep = 0
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
	case "esc":
		m = m.endPasswordPrompt()
		m.appendChan(protocol.ChanSystem, "Password change cancelled.")
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		m.input.Reset()
		if value == "" {
			m.appendChan(protocol.ChanSystem, "Password cannot be empty.")
			return m, nil
		}
		if m.pwStep == 1 {
			m.pwOld = value
			m.pwStep = 2
			m.input.Prompt = "New password: "
			return m, nil
		}
		old := m.pwOld
		m = m.endPasswordPrompt()
		encoded, err := protocol.EncodePasswordChange(old, value)
		if err != nil || m.sess == nil {
			m.appendChan(protocol.ChanSystem, "Password change failed.")
			return m, nil
		}
		m.pwPending = value
		m.sess.In <- encoded
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) endPasswordPrompt() Model {
	m.pwStep = 0
	m.pwOld = ""
	m.input.EchoMode = textinput.EchoNormal
	if m.pwPrompt != "" {
		m.input.Prompt = m.pwPrompt
	} else {
		m.input.Prompt = "> "
	}
	m.input.Placeholder = "look, n, get lamp..."
	m.input.Reset()
	return m
}

func (m Model) notePasswordUpdated(msg protocol.TextEvent) Model {
	if m.pwPending == "" || msg.Channel != protocol.ChanSystem {
		return m
	}
	if msg.Text == protocol.PasswordUpdated {
		next := m.pwPending
		m.pwPending = ""
		m.rlPassword = next
		account := m.rlAccount
		if m.sess != nil && m.sess.Account != "" {
			account = m.sess.Account
		}
		if account == "" {
			return m
		}
		_ = save.UpdateConnectionPasswords(account, next)
		for i := range m.savedConns {
			if strings.EqualFold(strings.TrimSpace(m.savedConns[i].Account), account) {
				m.savedConns[i].Password = next
			}
		}
		if m.pendingConn != nil && strings.EqualFold(strings.TrimSpace(m.pendingConn.Account), account) {
			m.pendingConn.Password = next
		}
		return m
	}
	switch {
	case strings.HasPrefix(msg.Text, "Password change failed"),
		strings.HasPrefix(msg.Text, "Password changes are only"),
		strings.HasPrefix(msg.Text, "New password cannot"),
		strings.HasPrefix(msg.Text, "Wait a moment before trying"):
		m.pwPending = ""
	}
	return m
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
		if m.mode == protocol.ModeBuild && sideW < 32 {
			sideW = 32
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
	if m.mode == protocol.ModeBuild {
		return true
	}
	for _, p := range m.layout {
		if p == "map" || p == "vitals" || p == "inventory" || p == "combat" || p == "exits" {
			return true
		}
	}
	return false
}

func (m *Model) refreshSide() {
	if m.mode == protocol.ModeBuild {
		m.side.SetContent(m.buildSide())
		m.side.GotoTop()
		return
	}
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
		for _, r := range hudResources(m.vitals.Resources, false) {
			b.WriteString(r.Text() + "\n")
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

func (m Model) buildSide() string {
	var b strings.Builder
	b.WriteString(m.section("build"))
	if m.room.Title != "" {
		b.WriteString(m.room.Title + "\n")
	}
	if m.room.ID != "" {
		b.WriteString(m.room.ID + "\n")
	}
	b.WriteString("\n")
	b.WriteString(m.section("map"))
	b.WriteString("@ this room\n")
	if m.cmap.Text != "" {
		b.WriteString(m.cmap.Text)
		b.WriteByte('\n')
	} else {
		b.WriteString("No map. Set x, y, z on rooms.\n")
	}
	b.WriteString("\n")
	b.WriteString(m.section("commands"))
	for _, line := range []string{
		"dig n Name",
		"buildwalk",
		"name <title>",
		"desc <line>",
		"rflags dark",
		"goto <id>",
		"rooms",
		"spawn <id>",
		"save pack",
		"help building",
	} {
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) wheelGame(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	dir := 0
	switch msg.Button {
	case tea.MouseWheelUp:
		dir = -1
	case tea.MouseWheelDown:
		dir = 1
	default:
		return m, nil
	}
	step := 3
	overSide := m.showSide() && m.side.Width() > 0 && msg.X >= m.log.Width()
	if overSide {
		if dir < 0 {
			m.side.ScrollUp(step)
		} else {
			m.side.ScrollDown(step)
		}
		return m, nil
	}
	if dir < 0 {
		m.log.ScrollUp(step)
	} else {
		m.log.ScrollDown(step)
	}
	return m, nil
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
	case stateConnect:
		body = m.viewConnect()
	case stateRemoteLogin:
		body = m.viewRemoteLogin()
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
	b.WriteString("A text world engine. Enter play, c connect, b build, n new pack, q quit.\n\n")
	if len(m.games) == 0 {
		b.WriteString("No local game packs in " + m.gamesDir + ". Press n to create one.\n")
	}
	idx := 0
	for _, g := range m.games {
		cur := "  "
		if idx == m.cursor {
			cur = "> "
		}
		line := fmt.Sprintf("%s%s  (%s)", cur, g.Title, g.ID)
		if idx == m.cursor {
			line = m.theme.Cursor.Render(line)
		}
		b.WriteString(line + "\n")
		idx++
	}
	if len(m.savedConns) > 0 {
		b.WriteString("\n" + m.section("saved servers") + "\n")
		for _, c := range m.savedConns {
			cur := "  "
			if idx == m.cursor {
				cur = "> "
			}
			line := fmt.Sprintf("%s%s  (%s)", cur, c.Label(), c.Addr)
			if idx == m.cursor {
				line = m.theme.Cursor.Render(line)
			}
			b.WriteString(line + "\n")
			idx++
		}
	}
	b.WriteString("\n")
	cur := "  "
	if idx == m.cursor {
		cur = "> "
	}
	connLine := cur + "Connect to server..."
	if idx == m.cursor {
		connLine = m.theme.Cursor.Render(connLine)
	}
	b.WriteString(connLine + "\n")

	if m.dialing {
		b.WriteString("\n" + m.theme.Muted.Render(m.dialStatus()) + "\nesc cancel\n")
	}
	if m.errLine != "" {
		b.WriteString("\n" + m.theme.Error.Render(m.errLine) + "\n")
	}
	if len(m.savedConns) > 0 {
		b.WriteString("\n↑↓ move   enter play/connect   c connect menu   d remove saved   q quit\n")
	} else {
		b.WriteString("\n")
	}
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
	for _, r := range hudResources(m.vitals.Resources, true) {
		rest = append(rest, r.Text())
	}
	if m.mode == protocol.ModeBuild {
		rest = append(rest, "BUILD")
	}
	if m.theme == nil {
		return strings.Join(append([]string{m.room.Title}, rest...), "  │  ")
	}
	return m.theme.StatusLine(m.width, m.room.Title, strings.Join(rest, "  │  "), m.vitals.InCombat)
}

// hudResources keeps bar resources on the status line.
// The side pane also keeps show:side. sheet stays on stats.
func hudResources(rs []protocol.ResourceView, status bool) []protocol.ResourceView {
	var out []protocol.ResourceView
	for _, r := range rs {
		if status {
			if r.Show == "bar" {
				out = append(out, r)
			}
			continue
		}
		if r.Show == "sheet" {
			continue
		}
		out = append(out, r)
	}
	return out
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
		sess.Flush()
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

// ConnectPlay creates a client Model ready to play over an established remote session.
func ConnectPlay(sess *server.Session, cancel context.CancelFunc) Model {
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

	m := Model{
		state:     stateGame,
		input:     ti,
		log:       log,
		side:      side,
		layout:    []string{"output", "map", "vitals"},
		prompt:    "> ",
		title:     "Erickson Stories",
		theme:     theme.Load(),
		helpTopic: -1,
		sess:      sess,
		cancel:    cancel,
	}
	m.layoutPanes()
	return m
}

// CurrentRoom returns the room most recently received by the client model.
func (m Model) CurrentRoom() protocol.RoomEvent {
	return m.room
}
