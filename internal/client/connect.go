package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	snet "sudengine/internal/net"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/save"
	"sudengine/internal/server"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type launcherItemKind int

const (
	launcherPack launcherItemKind = iota
	launcherSaved
	launcherConnect
)

type launcherItem struct {
	kind launcherItemKind
	pack pack.Info
	conn save.Connection
}

func (m Model) launcherItems() []launcherItem {
	items := make([]launcherItem, 0, len(m.games)+len(m.savedConns)+1)
	for _, g := range m.games {
		items = append(items, launcherItem{kind: launcherPack, pack: g})
	}
	for _, c := range m.savedConns {
		items = append(items, launcherItem{kind: launcherSaved, conn: c})
	}
	items = append(items, launcherItem{kind: launcherConnect})
	return items
}

type serversScannedMsg struct {
	servers []snet.ServerInfo
}

func scanServersCmd(port int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		found := snet.DiscoverServers(ctx, port)
		return serversScannedMsg{servers: found}
	}
}

func (m Model) openConnect() (tea.Model, tea.Cmd) {
	m.state = stateConnect
	m.menuCursor = 0
	m.errLine = ""
	m.scanning = true
	m.connectTyping = false
	m.input.EchoMode = textinput.EchoNormal
	m.input.Prompt = "Server (host:port): "
	m.input.Placeholder = "127.0.0.1:4000 or 192.168.1.50:4000"
	m.input.Reset()
	return m, scanServersCmd(4000)
}

type connectOptionKind int

const (
	connOptServer connectOptionKind = iota
	connOptManual
	connOptRescan
	connOptBack
)

type connectOption struct {
	kind   connectOptionKind
	label  string
	server snet.ServerInfo
}

func (m Model) connectOptions() []connectOption {
	var opts []connectOption
	for _, s := range m.discovered {
		title := s.Title
		if title == "" {
			title = s.PackID
		}
		if title == "" {
			title = "Sudengine Server"
		}
		label := fmt.Sprintf("%s  (%s)", title, s.Addr)
		opts = append(opts, connectOption{
			kind:   connOptServer,
			label:  label,
			server: s,
		})
	}
	opts = append(opts,
		connectOption{kind: connOptManual, label: "Enter IP / host:port..."},
		connectOption{kind: connOptRescan, label: "Rescan local network"},
		connectOption{kind: connOptBack, label: "Back"},
	)
	return opts
}

func (m Model) updateConnect(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.dialing {
		return m.cancelNetOp(msg)
	}
	if m.connectTyping {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.connectTyping = false
			m.errLine = ""
			m.input.Reset()
			return m, nil
		case "enter":
			addr := snet.NormalizeClientAddr(m.input.Value())
			m.input.Reset()
			m.connectTyping = false
			return m.beginRemoteServer(addr, nil)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	opts := m.connectOptions()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		return m.backToLauncher()
	case "r":
		m.scanning = true
		m.errLine = ""
		return m, scanServersCmd(4000)
	case "t", "/", "i":
		m.connectTyping = true
		m.errLine = ""
		m.input.Prompt = "Server (host:port): "
		m.input.Placeholder = "127.0.0.1:4000 or 192.168.1.50:4000"
		m.input.Reset()
		m.input.Focus()
		return m, m.input.Focus()
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j":
		if m.menuCursor < len(opts)-1 {
			m.menuCursor++
		}
	case "enter":
		if m.menuCursor < 0 || m.menuCursor >= len(opts) {
			return m, nil
		}
		opt := opts[m.menuCursor]
		switch opt.kind {
		case connOptServer:
			info := opt.server
			return m.beginRemoteServer(info.Addr, &info)
		case connOptManual:
			m.connectTyping = true
			m.errLine = ""
			m.input.Prompt = "Server (host:port): "
			m.input.Placeholder = "127.0.0.1:4000 or 192.168.1.50:4000"
			m.input.Reset()
			m.input.Focus()
			return m, m.input.Focus()
		case connOptRescan:
			m.scanning = true
			m.errLine = ""
			return m, scanServersCmd(4000)
		case connOptBack:
			return m.backToLauncher()
		}
	}
	return m, nil
}

func (m Model) viewConnect() string {
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("CONNECT TO SERVER") + "\n")
	b.WriteString("Pick a discovered server on your network, or enter an IP / host:port.\n\n")
	if m.dialing {
		b.WriteString(m.theme.Muted.Render(m.dialStatus()) + "\n\nesc cancel\n")
		return b.String()
	}

	if m.scanning {
		b.WriteString(m.theme.Muted.Render("Scanning local network for servers on port 4000...") + "\n\n")
	} else if len(m.discovered) == 0 {
		b.WriteString(m.theme.Muted.Render("No servers found automatically on port 4000.") + "\n\n")
	}

	if m.connectTyping {
		b.WriteString("Enter server address (port defaults to 4000 if omitted):\n\n")
		b.WriteString(m.input.View() + "\n")
		if m.errLine != "" {
			b.WriteString("\n" + m.theme.Error.Render(m.errLine) + "\n")
		}
		b.WriteString("\nenter connect   esc cancel\n")
		return b.String()
	}

	opts := m.connectOptions()
	for i, opt := range opts {
		cur := "  "
		if i == m.menuCursor {
			cur = "> "
		}
		line := cur + opt.label
		if i == m.menuCursor {
			line = m.theme.Cursor.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if m.errLine != "" {
		b.WriteString("\n" + m.theme.Error.Render(m.errLine) + "\n")
	}
	b.WriteString("\n↑↓ move   enter select   t type IP:port   r rescan   esc back\n")
	return b.String()
}

const (
	rlStepAccount = iota
	rlStepPassword
	rlStepCharSelect
	rlStepCharName
	rlStepRace
	rlStepRole
	rlStepPointBuy
)

func (m Model) beginRemoteServer(addr string, known *snet.ServerInfo) (tea.Model, tea.Cmd) {
	addr = snet.NormalizeClientAddr(addr)
	if known != nil && known.Type == "server_info" {
		info := *known
		info.Addr = addr
		m = m.openRemoteLogin(addr, info)
		return m, m.input.Focus()
	}
	ctx, gen := m.beginNetOp("Contacting server...")
	return m, func() tea.Msg {
		pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		info, err := snet.Probe(pctx, addr)
		return probeResultMsg{addr: addr, info: info, err: err, gen: gen}
	}
}

func (m Model) openRemoteLogin(addr string, info snet.ServerInfo) Model {
	m.dialing = false
	m.dialNote = ""
	m.remoteAddr = addr
	m.remoteInfo = info
	m.state = stateRemoteLogin
	m.rlStep = rlStepAccount
	m.rlAccount = ""
	m.rlPassword = ""
	m.rlCharName = ""
	m.rlRace = 0
	m.rlRole = 0
	m.rlPoints = info.PointBuy
	m.rlAttrs = map[string]int{}
	m.rlAttrIdx = 0
	m.menuCursor = 0
	m.errLine = ""
	if info.Title != "" {
		m.title = info.Title
	}
	m.input.EchoMode = textinput.EchoNormal
	m.input.Prompt = "Account: "
	m.input.Placeholder = "username"
	m.input.Reset()
	m.input.Focus()
	return m
}

func (m Model) remoteCreationSteps() []int {
	steps := []int{rlStepCharName}
	if len(m.remoteInfo.Races) > 0 {
		steps = append(steps, rlStepRace)
	}
	if len(m.remoteInfo.Roles) > 0 {
		steps = append(steps, rlStepRole)
	}
	if m.remoteInfo.PointBuy > 0 && len(m.remoteInfo.Attributes) > 0 {
		steps = append(steps, rlStepPointBuy)
	}
	return steps
}

func (m Model) updateRemoteLogin(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.dialing {
		return m.cancelNetOp(msg)
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.input.EchoMode = textinput.EchoNormal
		m.errLine = ""
		switch m.rlStep {
		case rlStepPassword:
			m.rlStep = rlStepAccount
			m.input.Prompt = "Account: "
			m.input.Placeholder = "username"
			m.input.SetValue(m.rlAccount)
			m.input.CursorEnd()
			return m, m.input.Focus()
		case rlStepCharSelect:
			m.rlStep = rlStepAccount
			m.input.Prompt = "Account: "
			m.input.Placeholder = "username"
			m.input.SetValue(m.rlAccount)
			m.input.CursorEnd()
			return m, m.input.Focus()
		case rlStepCharName:
			if len(m.remoteInfo.Characters) > 0 {
				m.rlStep = rlStepCharSelect
				m.menuCursor = 0
				return m, nil
			}
			m.rlStep = rlStepAccount
			m.input.Prompt = "Account: "
			m.input.Placeholder = "username"
			m.input.SetValue(m.rlAccount)
			m.input.CursorEnd()
			return m, m.input.Focus()
		case rlStepRace, rlStepRole, rlStepPointBuy:
			m.rlStep = rlStepCharName
			m.input.Prompt = "Character name: "
			m.input.Placeholder = "who are you?"
			m.input.SetValue(m.rlCharName)
			m.input.CursorEnd()
			return m, m.input.Focus()
		default:
			return m.openConnect()
		}
	}

	switch m.rlStep {
	case rlStepAccount:
		if msg.String() == "enter" {
			acc := strings.TrimSpace(m.input.Value())
			if acc == "" {
				m.errLine = "Please enter an account username."
				return m, nil
			}
			if _, err := save.Slug(acc); err != nil {
				m.errLine = "Account name must contain letters or numbers."
				return m, nil
			}
			m.rlAccount = acc
			m.errLine = ""
			m.rlStep = rlStepPassword
			m.input.Reset()
			m.input.EchoMode = textinput.EchoPassword
			m.input.Prompt = "Password: "
			m.input.Placeholder = "password"
			return m, m.input.Focus()
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case rlStepPassword:
		if msg.String() == "enter" {
			pass := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			if pass == "" {
				m.errLine = "Password cannot be empty."
				return m, nil
			}
			ctx, gen := m.beginNetOp("Signing in...")
			account := m.rlAccount
			addr := m.remoteAddr
			return m, func() tea.Msg {
				pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				info, err := snet.Authenticate(pctx, addr, account, pass)
				return authResultMsg{info: info, password: pass, err: err, gen: gen}
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case rlStepCharSelect:
		total := len(m.remoteInfo.Characters) + 1 // +1 for "New character..."
		switch msg.String() {
		case "up", "k":
			if m.menuCursor > 0 {
				m.menuCursor--
			}
		case "down", "j":
			if m.menuCursor < total-1 {
				m.menuCursor++
			}
		case "n":
			m.rlStep = rlStepCharName
			m.errLine = ""
			m.input.Prompt = "Character name: "
			m.input.Placeholder = "who are you?"
			m.input.Reset()
			return m, m.input.Focus()
		case "enter":
			if m.menuCursor >= 0 && m.menuCursor < len(m.remoteInfo.Characters) {
				charName := m.remoteInfo.Characters[m.menuCursor]
				return m.finishRemoteLogin(charName, "", "", nil)
			}
			m.rlStep = rlStepCharName
			m.errLine = ""
			m.input.Prompt = "Character name: "
			m.input.Placeholder = "who are you?"
			m.input.Reset()
			return m, m.input.Focus()
		}
		return m, nil

	case rlStepCharName:
		if msg.String() == "enter" {
			name := strings.TrimSpace(m.input.Value())
			if name == "" {
				m.errLine = "Please enter a character name."
				return m, nil
			}
			if _, err := save.Slug(name); err != nil {
				m.errLine = "Character name must contain letters or numbers."
				return m, nil
			}
			m.rlCharName = name
			m.input.Reset()
			m.errLine = ""
			return m.advanceRemoteCreationStep(rlStepCharName)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case rlStepRace:
		switch msg.String() {
		case "up", "k":
			if m.rlRace > 0 {
				m.rlRace--
			}
		case "down", "j":
			if m.rlRace < len(m.remoteInfo.Races)-1 {
				m.rlRace++
			}
		case "enter":
			return m.advanceRemoteCreationStep(rlStepRace)
		}
		return m, nil

	case rlStepRole:
		switch msg.String() {
		case "up", "k":
			if m.rlRole > 0 {
				m.rlRole--
			}
		case "down", "j":
			if m.rlRole < len(m.remoteInfo.Roles)-1 {
				m.rlRole++
			}
		case "enter":
			return m.advanceRemoteCreationStep(rlStepRole)
		}
		return m, nil

	case rlStepPointBuy:
		switch msg.String() {
		case "up", "k":
			if m.rlAttrIdx > 0 {
				m.rlAttrIdx--
			}
		case "down", "j":
			if m.rlAttrIdx < len(m.remoteInfo.Attributes)-1 {
				m.rlAttrIdx++
			}
		case "enter", "+", "right", "l":
			if m.rlAttrIdx >= 0 && m.rlAttrIdx < len(m.remoteInfo.Attributes) && m.rlPoints > 0 {
				attr := m.remoteInfo.Attributes[m.rlAttrIdx]
				if attr.Max > 0 && m.pointBuyTotal(attr) >= attr.Max {
					label := attr.Label
					if label == "" {
						label = attr.ID
					}
					m.errLine = fmt.Sprintf("%s is already at its maximum (%d).", label, attr.Max)
					return m, nil
				}
				if m.rlAttrs == nil {
					m.rlAttrs = map[string]int{}
				}
				m.rlAttrs[attr.ID]++
				m.rlPoints--
				m.errLine = ""
				if m.rlPoints <= 0 {
					return m.advanceRemoteCreationStep(rlStepPointBuy)
				}
			}
		case "-", "left", "h":
			if m.rlAttrIdx >= 0 && m.rlAttrIdx < len(m.remoteInfo.Attributes) {
				key := m.remoteInfo.Attributes[m.rlAttrIdx].ID
				if m.rlAttrs[key] > 0 {
					m.rlAttrs[key]--
					if m.rlAttrs[key] == 0 {
						delete(m.rlAttrs, key)
					}
					m.rlPoints++
					m.errLine = ""
				}
			}
		case "d":
			m.errLine = ""
			return m.advanceRemoteCreationStep(rlStepPointBuy)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) advanceRemoteCreationStep(current int) (tea.Model, tea.Cmd) {
	steps := m.remoteCreationSteps()
	idx := indexOf(steps, current)
	if idx >= 0 && idx+1 < len(steps) {
		next := steps[idx+1]
		m.rlStep = next
		if next == rlStepPointBuy {
			m.rlPoints = m.remoteInfo.PointBuy
			m.rlAttrs = map[string]int{}
			m.rlAttrIdx = 0
		}
		return m, nil
	}
	raceID := ""
	if len(m.remoteInfo.Races) > 0 && m.rlRace >= 0 && m.rlRace < len(m.remoteInfo.Races) {
		raceID = m.remoteInfo.Races[m.rlRace].ID
	}
	roleID := ""
	if len(m.remoteInfo.Roles) > 0 && m.rlRole >= 0 && m.rlRole < len(m.remoteInfo.Roles) {
		roleID = m.remoteInfo.Roles[m.rlRole].ID
	}
	return m.finishRemoteLogin(m.rlCharName, raceID, roleID, m.rlAttrs)
}

func (m Model) finishRemoteLogin(charName, raceID, roleID string, attrs map[string]int) (tea.Model, tea.Cmd) {
	entry := save.Connection{
		Addr:      m.remoteAddr,
		PackID:    m.remoteInfo.PackID,
		Title:     m.remoteInfo.Title,
		Account:   m.rlAccount,
		Password:  m.rlPassword,
		Character: charName,
		Race:      raceID,
		Role:      roleID,
	}
	step := m.rlStep
	if step == rlStepPointBuy || step == rlStepRace || step == rlStepRole {
		step = rlStepCharName
	}
	return m.startRemoteDial(m.remoteAddr, m.remoteInfo.Title, snet.LoginOptions{
		Account:  m.rlAccount,
		Password: m.rlPassword,
		Name:     charName,
		Race:     raceID,
		Role:     roleID,
		Attrs:    attrs,
	}, &entry, stateRemoteLogin, step)
}

func (m Model) connectSaved(conn save.Connection) (tea.Model, tea.Cmd) {
	m.remoteAddr = conn.Addr
	entry := conn
	return m.startRemoteDial(conn.Addr, conn.Label(), snet.LoginOptions{
		Account:  conn.Account,
		Password: conn.Password,
		Name:     conn.Character,
		Race:     conn.Race,
		Role:     conn.Role,
	}, &entry, stateLauncher, 0)
}

func (m Model) startRemoteDial(addr, title string, opt snet.LoginOptions, entry *save.Connection, home state, step int) (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.dialGen++
	gen := m.dialGen
	m.cancel = cancel
	m.dialing = true
	m.dialNote = "Connecting..."
	m.remoteJoined = false
	m.remoteNotice = ""
	m.pendingConn = entry
	m.dialHome = home
	m.dialStep = step
	m.errLine = ""
	m.remoteAddr = addr
	if title != "" {
		m.title = title
	}
	return m, func() tea.Msg {
		sess, cleanup, err := snet.DialWithLogin(ctx, addr, opt)
		return remoteDialMsg{sess: sess, cleanup: cleanup, err: err, gen: gen, cancel: cancel}
	}
}

func (m Model) viewRemoteLogin() string {
	var b strings.Builder
	title := m.remoteInfo.Title
	if title == "" {
		title = m.remoteAddr
	}
	b.WriteString(m.theme.Title.Render(title) + "  " + m.theme.Muted.Render("("+m.remoteAddr+")") + "\n")
	if m.remoteInfo.Subtitle != "" {
		b.WriteString(m.remoteInfo.Subtitle + "\n")
	}
	b.WriteString("\n")
	if m.dialing {
		b.WriteString(m.theme.Muted.Render(m.dialStatus()) + "\n\nesc cancel\n")
		return b.String()
	}

	switch m.rlStep {
	case rlStepAccount:
		b.WriteString("Sign in or create an account on this server:\n\n")
		b.WriteString(m.input.View() + "\n")
		if m.errLine != "" {
			b.WriteString("\n" + m.theme.Error.Render(m.errLine) + "\n")
		}
		b.WriteString("\nenter next   esc back\n")

	case rlStepPassword:
		b.WriteString(fmt.Sprintf("Account: %s\n\n", m.rlAccount))
		b.WriteString(m.input.View() + "\n")
		if m.errLine != "" {
			b.WriteString("\n" + m.theme.Error.Render(m.errLine) + "\n")
		}
		b.WriteString("\nenter sign in   esc back\n")

	case rlStepCharSelect:
		b.WriteString(fmt.Sprintf("Account: %s — Choose a character:\n\n", m.rlAccount))
		for i, ch := range m.remoteInfo.Characters {
			cur := "  "
			if i == m.menuCursor {
				cur = "> "
			}
			line := fmt.Sprintf("%s%s — %s", cur, title, ch)
			if i == m.menuCursor {
				line = m.theme.Cursor.Render(line)
			}
			b.WriteString(line + "\n")
		}
		newIdx := len(m.remoteInfo.Characters)
		cur := "  "
		if m.menuCursor == newIdx {
			cur = "> "
		}
		newLine := cur + "New character..."
		if m.menuCursor == newIdx {
			newLine = m.theme.Cursor.Render(newLine)
		}
		b.WriteString(newLine + "\n")
		if m.errLine != "" {
			b.WriteString("\n" + m.theme.Error.Render(m.errLine) + "\n")
		}
		b.WriteString("\n↑↓ move   enter play   n new character   esc back\n")

	case rlStepCharName:
		b.WriteString("New character — What do they call you?\n\n")
		b.WriteString(m.input.View() + "\n")
		if m.errLine != "" {
			b.WriteString("\n" + m.theme.Error.Render(m.errLine) + "\n")
		}
		b.WriteString("\nenter next   esc back\n")

	case rlStepRace:
		b.WriteString("Choose an origin:\n\n")
		for i, r := range m.remoteInfo.Races {
			cur := "  "
			if i == m.rlRace {
				cur = "> "
			}
			line := cur + r.Label
			if r.Help != "" {
				line += "  —  " + r.Help
			}
			if i == m.rlRace {
				line = m.theme.Cursor.Render(line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n↑↓ move   enter next   esc back\n")

	case rlStepRole:
		b.WriteString("Choose a role:\n\n")
		for i, r := range m.remoteInfo.Roles {
			cur := "  "
			if i == m.rlRole {
				cur = "> "
			}
			line := cur + r.Label
			if r.Help != "" {
				line += "  —  " + r.Help
			}
			if i == m.rlRole {
				line = m.theme.Cursor.Render(line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n↑↓ move   enter next   esc back\n")

	case rlStepPointBuy:
		b.WriteString(fmt.Sprintf("Spend attribute points (%d remaining):\n\n", m.rlPoints))
		for i, a := range m.remoteInfo.Attributes {
			cur := "  "
			if i == m.rlAttrIdx {
				cur = "> "
			}
			label := a.Label
			if label == "" {
				label = a.ID
			}
			total := m.pointBuyTotal(a)
			value := fmt.Sprintf("%d", total)
			if a.Max > 0 {
				value = fmt.Sprintf("%d/%d", total, a.Max)
			}
			spent := ""
			if m.rlAttrs[a.ID] > 0 {
				spent = fmt.Sprintf("  (+%d)", m.rlAttrs[a.ID])
			}
			line := fmt.Sprintf("%s%s  %s%s", cur, label, value, spent)
			if i == m.rlAttrIdx {
				line = m.theme.Cursor.Render(line)
			}
			b.WriteString(line + "\n")
		}
		if m.errLine != "" {
			b.WriteString("\n" + m.theme.Error.Render(m.errLine) + "\n")
		}
		b.WriteString("\n↑↓ move   +/- spend   d done   esc back\n")
	}

	return b.String()
}

// ConnectAddress opens the remote login form for addr.
// The caller is outside the TUI loop, so the probe runs here.
func ConnectAddress(gamesDir, addr string) (Model, error) {
	m := New(gamesDir)
	addr = snet.NormalizeClientAddr(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	info, err := snet.Probe(ctx, addr)
	if err != nil {
		return Model{}, fmt.Errorf("could not reach %s: %w", addr, err)
	}
	return m.openRemoteLogin(addr, info), nil
}

type probeResultMsg struct {
	addr string
	info snet.ServerInfo
	err  error
	gen  int
}

type authResultMsg struct {
	info     snet.ServerInfo
	password string
	err      error
	gen      int
}

type remoteDialMsg struct {
	sess    *server.Session
	cleanup context.CancelFunc
	err     error
	gen     int
	cancel  context.CancelFunc
}

func (m *Model) beginNetOp(note string) (context.Context, int) {
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.dialGen++
	m.dialing = true
	m.dialNote = note
	m.errLine = ""
	return ctx, m.dialGen
}

func (m *Model) finishNetOp() {
	m.dialing = false
	m.dialNote = ""
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

func (m Model) dialStatus() string {
	if m.dialNote != "" {
		return m.dialNote
	}
	return "Contacting server..."
}

func (m Model) cancelNetOp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	case "esc":
		m.dialGen++
		m.dialing = false
		m.dialNote = ""
		m.pendingConn = nil
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		m.errLine = "Canceled."
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) noteRemoteJoined() {
	if m.remoteJoined || m.pendingConn == nil {
		return
	}
	m.remoteJoined = true
	entry := *m.pendingConn
	m.pendingConn = nil
	_ = save.SaveConnection(entry)
	if list, err := save.LoadConnections(); err == nil {
		m.savedConns = list
	}
}

func (m Model) refuseRemote(reason string) (tea.Model, tea.Cmd) {
	detail := m.remoteNotice
	if detail == "" {
		detail = reason
	}
	if detail == "" || detail == "denied" {
		detail = "The server refused that character."
	}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.sess = nil
	m.pendingConn = nil
	m.remoteJoined = false
	m.remoteNotice = ""
	m.dialing = false
	m.dialNote = ""
	m.lines = nil
	m.room = protocol.RoomEvent{}
	m.log.SetContent("")
	home := m.dialHome
	step := m.dialStep
	m.state = home
	m.errLine = detail
	if home == stateRemoteLogin {
		m.rlStep = step
		if step == rlStepCharName || step == rlStepAccount || step == rlStepPassword {
			return m, m.input.Focus()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) pointBuyTotal(a snet.BundleOption) int {
	total := a.Base + m.rlAttrs[a.ID]
	if len(m.remoteInfo.Races) > 0 && m.rlRace >= 0 && m.rlRace < len(m.remoteInfo.Races) {
		total += m.remoteInfo.Races[m.rlRace].Attrs[a.ID]
	}
	if len(m.remoteInfo.Roles) > 0 && m.rlRole >= 0 && m.rlRole < len(m.remoteInfo.Roles) {
		total += m.remoteInfo.Roles[m.rlRole].Attrs[a.ID]
	}
	return total
}
