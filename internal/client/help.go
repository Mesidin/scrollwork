package client

import (
	"fmt"
	"strings"

	"sudengine/internal/docs"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Help is a two-pane manual. The menu lists game rules and engine guides.
// A topic with ## headings expands into pages. Each pane scrolls inside the
// window; the terminal scrollbar is not part of reading help.

type helpItem struct {
	header  bool
	topic   int
	section int
	depth   int
	label   string
}

type helpLayout struct {
	w, h, inner int
	split       bool
	menuW       int
	bodyW       int
}

func (m Model) openHelp() (tea.Model, tea.Cmd) {
	packHelp := map[string]string{}
	if m.pack != nil {
		packHelp = m.pack.Help
	}
	m.helpTopics = docs.Merge(packHelp)
	m.helpTopic = 0
	m.helpSection = 0
	m.helpBodyY = 0
	m.helpMenuY = 0
	m.helpFromGame = false
	m.errLine = ""
	m.menuCursor = 0
	if len(m.helpTopics) > 0 {
		m.menuCursor = m.helpItemIndex(0, 0)
		m.revealHelpMenu()
	}
	m.state = stateHelp
	return m, nil
}

func (m Model) openHelpQuery(topic, section string) (tea.Model, tea.Cmd) {
	next, cmd := m.openHelp()
	hm := next.(Model)
	hm.helpFromGame = true
	if topic == "" {
		return hm, cmd
	}
	idx := -1
	for i, t := range hm.helpTopics {
		if strings.EqualFold(t.Key, topic) {
			idx = i
			break
		}
	}
	if idx < 0 {
		hm.errLine = fmt.Sprintf("No help on %q.", topic)
		return hm, cmd
	}
	secs := docs.Sections(hm.helpTopics[idx].Body)
	hm = hm.activateHelp(idx, docs.MatchSection(secs, section))
	return hm, cmd
}

func helpQuery(line string) (topic, section string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", "", false
	}
	verb := strings.ToLower(fields[0])
	if verb != "help" && verb != "manual" {
		return "", "", false
	}
	if len(fields) >= 2 && strings.ToLower(fields[1]) != "topics" {
		topic = strings.ToLower(fields[1])
	}
	if len(fields) >= 3 {
		section = strings.Join(fields[2:], " ")
	}
	return topic, section, true
}

func (m Model) closeHelp() (tea.Model, tea.Cmd) {
	fromGame := m.helpFromGame
	m.errLine = ""
	m.helpFromGame = false
	if fromGame {
		m.state = stateGame
		return m, m.input.Focus()
	}
	m.state = stateIntro
	m.menuCursor = 0
	return m, nil
}

func (m Model) updateHelp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		return m.closeHelp()
	case "up", "k":
		return m.moveHelp(-1), nil
	case "down", "j":
		return m.moveHelp(1), nil
	case "pgup":
		return m.pageHelpBody(-1), nil
	case "pgdown":
		return m.pageHelpBody(1), nil
	case "home":
		return m.scrollHelpEdge(true), nil
	case "end":
		return m.scrollHelpEdge(false), nil
	case "enter":
		return m, nil
	}
	return m, nil
}

func (m Model) wheelHelp(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	dir := 0
	switch msg.Button {
	case tea.MouseWheelUp:
		dir = -1
	case tea.MouseWheelDown:
		dir = 1
	default:
		return m, nil
	}
	L := m.helpLayout()
	if L.split && msg.X < L.menuW {
		return m.moveHelp(dir), nil
	}
	return m.scrollHelpBody(dir * 3), nil
}

func (m Model) clickHelp(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	L := m.helpLayout()
	if !L.split || msg.X >= L.menuW {
		return m, nil
	}
	row := msg.Y - 1
	if row < 0 || row >= L.inner {
		return m, nil
	}
	items := m.helpItems()
	idx := m.helpMenuOrigin(len(items), L.inner) + row
	if idx < 0 || idx >= len(items) || items[idx].header {
		return m, nil
	}
	return m.activateHelp(items[idx].topic, items[idx].section), nil
}

func (m Model) helpItems() []helpItem {
	var items []helpItem
	last := ""
	for i, t := range m.helpTopics {
		if t.Source != last {
			last = t.Source
			label := "Engine"
			if t.Source == docs.SourceGame {
				label = "Game"
			}
			items = append(items, helpItem{header: true, label: label})
		}
		label := t.Title
		if docs.Slug(t.Title) != t.Key && t.Key != "" {
			label = t.Key + "  " + t.Title
		}
		items = append(items, helpItem{topic: i, section: 0, label: label})
		if i != m.helpTopic {
			continue
		}
		secs := docs.Sections(t.Body)
		for s := 1; s < len(secs); s++ {
			items = append(items, helpItem{
				topic: i, section: s, depth: 1, label: secs[s].Title,
			})
		}
	}
	return items
}

func (m Model) helpItemIndex(topic, section int) int {
	for i, it := range m.helpItems() {
		if !it.header && it.topic == topic && it.section == section {
			return i
		}
	}
	for i, it := range m.helpItems() {
		if !it.header && it.topic == topic {
			return i
		}
	}
	return 0
}

func (m Model) activateHelp(topic, section int) Model {
	if topic < 0 || topic >= len(m.helpTopics) {
		return m
	}
	secs := docs.Sections(m.helpTopics[topic].Body)
	if section < 0 || section >= len(secs) {
		section = 0
	}
	changed := topic != m.helpTopic || section != m.helpSection
	m.helpTopic = topic
	m.helpSection = section
	m.menuCursor = m.helpItemIndex(topic, section)
	if changed {
		m.helpBodyY = 0
	}
	m.revealHelpMenu()
	return m
}

func (m Model) moveHelp(delta int) Model {
	if delta == 0 {
		return m
	}
	items := m.helpItems()
	i := m.menuCursor
	for {
		i += delta
		if i < 0 || i >= len(items) {
			return m
		}
		if items[i].header {
			continue
		}
		return m.activateHelp(items[i].topic, items[i].section)
	}
}

func (m Model) revealHelpMenu() {
	L := m.helpLayout()
	if L.inner < 1 {
		return
	}
	if m.menuCursor < m.helpMenuY {
		m.helpMenuY = m.menuCursor
	}
	if m.menuCursor >= m.helpMenuY+L.inner {
		m.helpMenuY = m.menuCursor - L.inner + 1
	}
	if m.helpMenuY < 0 {
		m.helpMenuY = 0
	}
}

func (m Model) helpLayout() helpLayout {
	L := helpLayout{w: m.width, h: m.height}
	if L.w < 1 || L.h < 1 {
		return L
	}
	L.inner = L.h - 2
	if L.inner < 0 {
		L.inner = 0
	}
	if L.w >= 48 && L.inner > 0 {
		L.menuW = 28
		if L.w < 80 {
			L.menuW = 22
		}
		if L.menuW > L.w-20 {
			L.menuW = L.w / 2
		}
		if L.menuW >= 16 {
			L.split = true
			L.bodyW = L.w - L.menuW
			return L
		}
	}
	L.menuW = 0
	L.bodyW = L.w
	return L
}

func (L helpLayout) textWidth() int {
	w := L.bodyW - 1
	if w < 1 {
		return 1
	}
	return w
}

func (m Model) helpBodyLines(width int) []string {
	if m.helpTopic < 0 || m.helpTopic >= len(m.helpTopics) {
		return []string{"No manuals."}
	}
	secs := docs.Sections(m.helpTopics[m.helpTopic].Body)
	if len(secs) == 0 {
		return nil
	}
	sec := m.helpSection
	if sec < 0 || sec >= len(secs) {
		sec = 0
	}
	text := docs.FormatWidth(secs[sec].Body, width)
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func (m Model) scrollHelpBody(delta int) Model {
	L := m.helpLayout()
	lines := m.helpBodyLines(L.textWidth())
	window := L.inner
	if window < 1 {
		window = 1
	}
	maxOff := len(lines) - window
	if maxOff < 1 {
		m.helpBodyY = 0
		if delta == 0 {
			return m
		}
		step := window / 2
		if step < 1 {
			step = 1
		}
		dir := 1
		if delta < 0 {
			dir = -1
		}
		return m.jumpHelp(dir * step)
	}
	next := m.helpBodyY + delta
	if next < 0 {
		next = 0
	}
	if next > maxOff {
		next = maxOff
	}
	m.helpBodyY = next
	return m
}

func (m Model) pageHelpBody(dir int) Model {
	L := m.helpLayout()
	step := L.inner / 2
	if step < 1 {
		step = 1
	}
	return m.scrollHelpBody(dir * step)
}

func (m Model) scrollHelpEdge(top bool) Model {
	if top {
		m.helpBodyY = 0
		return m
	}
	L := m.helpLayout()
	lines := m.helpBodyLines(L.textWidth())
	maxOff := len(lines) - L.inner
	if maxOff < 0 {
		maxOff = 0
	}
	m.helpBodyY = maxOff
	return m
}

func (m Model) jumpHelp(steps int) Model {
	dir := 1
	if steps < 0 {
		dir = -1
		steps = -steps
	}
	for i := 0; i < steps; i++ {
		next := m.moveHelp(dir)
		if next.menuCursor == m.menuCursor && next.helpTopic == m.helpTopic && next.helpSection == m.helpSection {
			break
		}
		m = next
	}
	return m
}

func (m Model) viewHelp() string {
	if m.width < 1 || m.height < 1 {
		return "Help"
	}
	L := m.helpLayout()
	lines := make([]string, 0, L.h)
	lines = append(lines, m.helpTitle(L.w))
	if L.inner > 0 {
		bodySrc := m.helpBodyLines(L.textWidth())
		bodyOff := clampInt(m.helpBodyY, 0, max(0, len(bodySrc)-L.inner))
		body := drawPane(bodySrc, bodyOff, L.bodyW, L.inner, m.barCell)
		if !L.split {
			lines = append(lines, body...)
		} else {
			items := m.helpItems()
			menuOff := m.helpMenuOrigin(len(items), L.inner)
			menu := m.drawMenu(items, menuOff, L.menuW, L.inner)
			for i := 0; i < L.inner; i++ {
				left, right := "", ""
				if i < len(menu) {
					left = menu[i]
				}
				if i < len(body) {
					right = body[i]
				}
				lines = append(lines, left+right)
			}
		}
	}
	if len(lines) < L.h {
		lines = append(lines, m.helpHint(L.w))
	}
	return strings.Join(exactLines(lines, L.w, L.h), "\n")
}

func (m Model) helpMenuOrigin(n, window int) int {
	off := m.helpMenuY
	maxOff := n - window
	if maxOff < 0 {
		maxOff = 0
	}
	if off < 0 {
		off = 0
	}
	if off > maxOff {
		off = maxOff
	}
	return off
}

func (m Model) helpTitle(width int) string {
	label := "Help"
	src := ""
	if m.helpTopic >= 0 && m.helpTopic < len(m.helpTopics) {
		t := m.helpTopics[m.helpTopic]
		label = t.Title
		secs := docs.Sections(t.Body)
		if m.helpSection >= 0 && m.helpSection < len(secs) && secs[m.helpSection].Title != "Overview" {
			label = t.Title + "  ·  " + secs[m.helpSection].Title
		}
		src = t.Source
	}
	left := ansi.Truncate(label, width, "")
	if src != "" && ansi.StringWidth(left)+1+ansi.StringWidth(src) <= width {
		gap := width - ansi.StringWidth(left) - ansi.StringWidth(src)
		titleSt := lipgloss.NewStyle()
		muted := titleSt
		if m.theme != nil {
			titleSt = m.theme.Title
			muted = m.theme.Muted
		}
		return padCells(titleSt.Render(left)+strings.Repeat(" ", gap)+muted.Render(src), width)
	}
	st := lipgloss.NewStyle()
	if m.theme != nil {
		st = m.theme.Title
	}
	return m.paint(st, label, width)
}

func (m Model) helpHint(width int) string {
	text := "↑↓ menu   pgup/pgdn scroll   esc back"
	st := lipgloss.NewStyle()
	if m.errLine != "" {
		text = m.errLine
		if m.theme != nil {
			st = m.theme.Error
		}
	} else if m.theme != nil {
		st = m.theme.Muted
	}
	return m.paint(st, text, width)
}

func (m Model) drawMenu(items []helpItem, offset, width, height int) []string {
	src := make([]string, height)
	for i := 0; i < height; i++ {
		idx := offset + i
		if idx < 0 || idx >= len(items) {
			continue
		}
		it := items[idx]
		text := it.label
		if !it.header {
			prefix := "  "
			if it.depth > 0 {
				prefix = "    "
			}
			if idx == m.menuCursor {
				if it.depth > 0 {
					prefix = "  > "
				} else {
					prefix = "> "
				}
			}
			text = prefix + it.label
		}
		st := lipgloss.NewStyle()
		if m.theme != nil && it.header {
			st = m.theme.Accent
		} else if m.theme != nil && idx == m.menuCursor {
			st = m.theme.Cursor
		}
		inner := width - 1
		if inner < 1 {
			inner = 1
		}
		src[i] = m.paint(st, text, inner)
	}
	bar := scrollMarks(height, len(items), offset)
	out := make([]string, height)
	for i := 0; i < height; i++ {
		cell := " "
		if i < len(bar) {
			cell = m.barCell(bar[i])
		}
		line := ""
		if i < len(src) {
			line = src[i]
		}
		out[i] = padCells(line, max(width-1, 0)) + cell
	}
	return out
}

func drawPane(src []string, offset, width, height int, barFn func(rune) string) []string {
	inner := width - 1
	if inner < 1 {
		inner = 1
	}
	bar := scrollMarks(height, len(src), offset)
	out := make([]string, height)
	for i := 0; i < height; i++ {
		line := ""
		j := offset + i
		if j >= 0 && j < len(src) {
			line = src[j]
		}
		cell := " "
		if i < len(bar) {
			cell = barFn(bar[i])
		}
		out[i] = padCells(ansi.Truncate(line, inner, ""), inner) + cell
	}
	return out
}

func (m Model) barCell(r rune) string {
	if r == ' ' {
		return " "
	}
	st := lipgloss.NewStyle()
	if m.theme != nil {
		if r == '█' {
			st = m.theme.Accent
		} else {
			st = m.theme.Muted
		}
	}
	return st.Render(string(r))
}

// scrollMarks builds a one-column bar. A space means the pane fits.
// '█' is the thumb and '│' is the track.
func scrollMarks(height, total, offset int) []rune {
	col := make([]rune, height)
	for i := range col {
		col[i] = ' '
	}
	if height <= 0 || total <= height {
		return col
	}
	thumb := height * height / total
	if thumb < 1 {
		thumb = 1
	}
	if thumb > height {
		thumb = height
	}
	maxOff := total - height
	if offset < 0 {
		offset = 0
	}
	if offset > maxOff {
		offset = maxOff
	}
	top := 0
	travel := height - thumb
	if maxOff > 0 && travel > 0 {
		top = offset * travel / maxOff
	}
	for i := range col {
		col[i] = '│'
	}
	for i := top; i < top+thumb && i < height; i++ {
		col[i] = '█'
	}
	return col
}

func (m Model) paint(st lipgloss.Style, s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "")
	rendered := s
	if m.theme != nil {
		rendered = st.Render(s)
	}
	if ansi.StringWidth(rendered) > width {
		rendered = ansi.Truncate(rendered, width, "")
	}
	return padCells(rendered, width)
}

func padCells(s string, width int) string {
	if width <= 0 {
		return ""
	}
	gap := width - ansi.StringWidth(s)
	if gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

func exactLines(lines []string, width, height int) []string {
	out := make([]string, height)
	for i := 0; i < height; i++ {
		s := ""
		if i < len(lines) {
			s = lines[i]
		}
		if ansi.StringWidth(s) > width {
			s = ansi.Truncate(s, width, "")
		}
		out[i] = padCells(s, width)
	}
	return out
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
