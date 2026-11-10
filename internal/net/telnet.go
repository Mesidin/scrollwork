package net

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	stdnet "net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/rpg"
	"sudengine/internal/save"
	"sudengine/internal/server"
	"sudengine/internal/theme"
)

// DefaultTelnetPort is the loopback port for Telnet clients.
const DefaultTelnetPort = 4023

// Telnet protocol bytes (RFC 854 / RFC 857).
const (
	IAC     byte = 255 // 0xFF Interpret As Command
	SE      byte = 240 // 0xF0 Subnegotiation End
	NOP     byte = 241 // 0xF1 No Operation
	SB      byte = 250 // 0xFA Subnegotiation Begin
	WILL    byte = 251 // 0xFB
	WONT    byte = 252 // 0xFC
	DO      byte = 253 // 0xFD
	DONT    byte = 254 // 0xFE
	OptEcho byte = 1   // 0x01 Echo
)

// SuppressEcho sends IAC WILL ECHO so the Telnet client disables local echo
// while the user types a password.
func SuppressEcho(w io.Writer) error {
	_, err := w.Write([]byte{IAC, WILL, OptEcho})
	return err
}

// RestoreEcho sends IAC WONT ECHO so the Telnet client resumes local echo
// after password entry completes.
func RestoreEcho(w io.Writer) error {
	_, err := w.Write([]byte{IAC, WONT, OptEcho})
	return err
}

type iacState int

const (
	iacNormal iacState = iota
	iacCmd
	iacOpt
	iacSub
	iacSubIAC
)

// TelnetReader reads lines from a byte stream while stripping Telnet IAC
// command and subnegotiation sequences so they are never dispatched as commands.
type TelnetReader struct {
	r     io.Reader
	buf   [512]byte
	pos   int
	end   int
	state iacState
}

func NewTelnetReader(r io.Reader) *TelnetReader {
	return &TelnetReader{r: r}
}

func (tr *TelnetReader) nextByte() (byte, error) {
	if tr.pos >= tr.end {
		n, err := tr.r.Read(tr.buf[:])
		if n > 0 {
			tr.pos = 0
			tr.end = n
		}
		if err != nil && n == 0 {
			return 0, err
		}
	}
	b := tr.buf[tr.pos]
	tr.pos++
	return b, nil
}

// ErrLineTooLong is returned when a Telnet line exceeds MaxFrameSize.
// The partial line is discarded and must not be enqueued.
var ErrLineTooLong = errors.New("telnet line too long")

// ReadLine reads until '\n', stripping Telnet IAC sequences, '\r', and NUL bytes.
// A line of only IAC bytes is skipped. The line fails once it exceeds MaxFrameSize.
func (tr *TelnetReader) ReadLine() (string, error) {
	var line []byte
	hadIAC := false
	for {
		b, err := tr.nextByte()
		if err != nil {
			if len(line) > 0 && !errors.Is(err, ErrLineTooLong) {
				return string(line), nil
			}
			return "", err
		}
		switch tr.state {
		case iacNormal:
			if b == IAC {
				tr.state = iacCmd
				hadIAC = true
				continue
			}
			if b == '\r' || b == 0 {
				continue
			}
			if b == '\n' {
				if len(line) == 0 && hadIAC {
					hadIAC = false
					continue
				}
				return string(line), nil
			}
			if b == 0x08 || b == 0x7f {
				if len(line) > 0 {
					line = line[:len(line)-1]
				}
				continue
			}
			var addErr error
			line, addErr = appendTelnetByte(line, b)
			if addErr != nil {
				return "", addErr
			}
		case iacCmd:
			switch b {
			case WILL, WONT, DO, DONT:
				tr.state = iacOpt
			case SB:
				tr.state = iacSub
			case IAC:
				tr.state = iacNormal
				var addErr error
				line, addErr = appendTelnetByte(line, IAC)
				if addErr != nil {
					return "", addErr
				}
			default:
				tr.state = iacNormal
			}
		case iacOpt:
			tr.state = iacNormal
		case iacSub:
			if b == IAC {
				tr.state = iacSubIAC
			}
		case iacSubIAC:
			if b == SE {
				tr.state = iacNormal
			} else {
				// IAC IAC inside a subnegotiation is a literal 0xFF, not a new command.
				tr.state = iacSub
			}
		}
	}
}

func appendTelnetByte(line []byte, b byte) ([]byte, error) {
	if len(line) >= MaxFrameSize {
		return nil, ErrLineTooLong
	}
	return append(line, b), nil
}

// ListenTelnet creates a TCP listener on 127.0.0.1 (defaulting to port 4023).
func ListenTelnet(addr string) (stdnet.Listener, error) {
	if addr == "" {
		addr = fmt.Sprintf("127.0.0.1:%d", DefaultTelnetPort)
	}
	return Listen(addr)
}

// ListenAndServeTelnet listens on addr and serves Telnet connections until ctx is canceled.
func ListenAndServeTelnet(ctx context.Context, srv *server.Server, addr string) error {
	ln, err := ListenTelnet(addr)
	if err != nil {
		return err
	}
	return ServeTelnet(ctx, srv, ln)
}

// ServeTelnet accepts Telnet connections on ln until ctx is canceled or ln is closed.
func ServeTelnet(ctx context.Context, srv *server.Server, ln stdnet.Listener) error {
	defer ln.Close()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, stdnet.ErrClosed) {
				return nil
			}
			fmt.Fprintf(os.Stderr, "telnet accept: %v\n", err)
			timer := time.NewTimer(20 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			continue
		}
		go ServeTelnetConn(ctx, srv, conn)
	}
}

// FormatTelnetEvent renders one protocol event as ANSI-16 text.
func FormatTelnetEvent(pal *theme.Palette, ev protocol.Event) string {
	if pal == nil {
		pal = theme.ANSI16()
	}
	switch e := ev.(type) {
	case protocol.RoomEvent:
		var b strings.Builder
		if e.Title != "" {
			b.WriteString(strings.ReplaceAll(pal.RoomTitle(stripESC(e.Title)), "\n", "\r\n"))
			b.WriteString("\r\n")
		}
		if e.Dark {
			b.WriteString("It's pitch black. You can't see a thing.\r\n")
			return b.String()
		}
		if desc := strings.TrimSpace(stripESC(e.Description)); desc != "" {
			b.WriteString(strings.ReplaceAll(desc, "\n", "\r\n"))
			b.WriteString("\r\n")
		}
		for _, it := range e.Items {
			b.WriteString(capFirst(stripESC(it)) + " is here.\r\n")
		}
		for _, occ := range e.Occupants {
			b.WriteString(capFirst(stripESC(occ)) + " is here.\r\n")
		}
		b.WriteString("Obvious exits: " + formatExitList(e.Exits) + ".\r\n")
		return b.String()
	case protocol.PromptEvent:
		if e.Room != "" || e.Status != "" {
			return pal.Prompt(stripESC(e.Room), stripESC(e.Status), e.Build)
		}
		if e.Text != "" {
			return stripESC(e.Text)
		}
		return "> "
	case protocol.VitalsEvent:
		var parts []string
		for _, r := range e.Resources {
			if r.Show == "bar" || (r.Show == "" && !r.Pile) {
				parts = append(parts, stripESC(r.Text()))
			}
		}
		if len(parts) == 0 {
			return "> "
		}
		return pal.Muted.Render(strings.Join(parts, " ")) + " > "
	case protocol.TextEvent:
		if e.Text == "" {
			return "\r\n"
		}
		rendered := pal.Line(e.Channel, stripESC(e.Text))
		return strings.ReplaceAll(rendered, "\n", "\r\n") + "\r\n"
	case protocol.DisconnectEvent:
		if e.Reason != "" {
			return pal.Line(protocol.ChanSystem, "Disconnected: "+stripESC(e.Reason)) + "\r\n"
		}
		return ""
	default:
		return ""
	}
}

// stripESC drops ESC bytes in text that has not yet been theme-colored.
func stripESC(s string) string {
	if strings.IndexByte(s, 0x1b) < 0 {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b = append(b, s[i])
		}
	}
	return string(b)
}

// writeTelnet writes s, doubling 0xFF so a data byte is not read as IAC.
func writeTelnet(w io.Writer, s string) error {
	if strings.IndexByte(s, 0xff) < 0 {
		_, err := io.WriteString(w, s)
		return err
	}
	buf := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		buf = append(buf, s[i])
		if s[i] == 0xff {
			buf = append(buf, 0xff)
		}
	}
	_, err := w.Write(buf)
	return err
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func formatExitList(exits []protocol.ExitInfo) string {
	if len(exits) == 0 {
		return "none"
	}
	dirs := make([]string, 0, len(exits))
	for _, ex := range exits {
		s := ex.Dir
		if ex.Closed {
			s += " (closed)"
		}
		if ex.Locked {
			s += " (locked)"
		}
		dirs = append(dirs, stripESC(s))
	}
	sort.Strings(dirs)
	return strings.Join(dirs, ", ")
}

func isDirectPlayVerb(line string) bool {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(line)))
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "look", "l", "who", "score", "inventory", "i", "inv", "exits", "time", "weather", "sky", "help", "say", "tell", "whisper", "whis", "shout", "yell", "gossip", "ooc", "quit", "n", "s", "e", "w", "u", "d", "north", "south", "east", "west", "up", "down":
		return true
	}
	return false
}

// ServeTelnetConn handles one Telnet connection: login/chargen (or direct guest command),
// IAC-aware command reading, and ANSI-16 event rendering.
func ServeTelnetConn(ctx context.Context, srv *server.Server, conn stdnet.Conn) {
	defer conn.Close()
	pal := theme.ANSI16()
	tr := NewTelnetReader(conn)

	title := "Sudengine"
	if srv != nil && srv.Pack != nil && srv.Pack.Meta.Title != "" {
		title = srv.Pack.Meta.Title
	}
	_, _ = io.WriteString(conn, pal.Title.Render(title)+"\r\n")

	connectMsg, ok := telnetLoginOrQuickStart(conn, tr, srv, pal)
	if !ok {
		return
	}

	sess := &server.Session{
		Account: connectMsg.Account,
		Persist: connectMsg.Persist,
		Out:     make(chan protocol.Event, 128),
		In:      make(chan string, 32),
	}
	connectMsg.Session = sess
	if !srv.EnqueueConnectChar(connectMsg) {
		return
	}

	writerDone := make(chan struct{})
	connClosed := make(chan struct{})
	var closeOnce sync.Once
	signalClose := func() {
		closeOnce.Do(func() { close(connClosed) })
	}

	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-connClosed:
		}
	}()

	go telnetWriteLoop(ctx, conn, sess, pal, connClosed, writerDone)

	reason := "closed"
	for {
		if sess.Closed() {
			break
		}
		line, err := tr.ReadLine()
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if rest, isPW := passwordVerb(line); isPW {
			if rest != "" {
				_, _ = io.WriteString(conn, "Type password and press enter. You will be asked for the current and new passwords.\r\n")
				continue
			}
			if sess.Account == "" {
				if !srv.Enqueue(sess, "password") {
					break
				}
				continue
			}
			oldPass, newPass, alive := readTelnetPasswordChange(conn, tr)
			if !alive {
				break
			}
			if oldPass == "" || newPass == "" {
				continue
			}
			encoded, err := protocol.EncodePasswordChange(oldPass, newPass)
			if err != nil || !srv.Enqueue(sess, encoded) {
				break
			}
			continue
		}
		if !srv.Enqueue(sess, line) {
			break
		}
		if strings.EqualFold(line, "quit") {
			reason = "quit"
			break
		}
	}

	_ = srv.EnqueueDisconnect(sess, reason)
	waitSessionClosed(srv, sess)
	signalClose()
	<-writerDone
}

func passwordVerb(line string) (rest string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", false
	}
	switch strings.ToLower(fields[0]) {
	case "password", "passwd":
	default:
		return "", false
	}
	return strings.TrimSpace(line[len(fields[0]):]), true
}

// readTelnetPasswordChange asks for both passwords with local echo off.
// A false alive means the connection closed. Empty passwords were rejected
// and already explained to the client.
func readTelnetPasswordChange(conn stdnet.Conn, tr *TelnetReader) (oldPass, newPass string, alive bool) {
	_ = SuppressEcho(conn)
	_, _ = io.WriteString(conn, "Current password: ")
	oldLine, err := tr.ReadLine()
	_, _ = io.WriteString(conn, "\r\n")
	if err != nil {
		_ = RestoreEcho(conn)
		return "", "", false
	}
	_, _ = io.WriteString(conn, "New password: ")
	newLine, err := tr.ReadLine()
	_ = RestoreEcho(conn)
	_, _ = io.WriteString(conn, "\r\n")
	if err != nil {
		return "", "", false
	}
	oldPass = strings.TrimSpace(oldLine)
	newPass = strings.TrimSpace(newLine)
	if oldPass == "" || newPass == "" {
		_, _ = io.WriteString(conn, "Password cannot be empty.\r\n")
		return "", "", true
	}
	return oldPass, newPass, true
}

func telnetLoginOrQuickStart(conn stdnet.Conn, tr *TelnetReader, srv *server.Server, pal *theme.Palette) (server.ConnectMsg, bool) {
	packID := ""
	var p *pack.Pack
	if srv != nil && srv.Pack != nil {
		p = srv.Pack
		packID = p.Meta.ID
	}

	for attempts := 0; attempts < 3; attempts++ {
		_, _ = io.WriteString(conn, "Account: ")
		line, err := tr.ReadLine()
		if err != nil {
			return server.ConnectMsg{}, false
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// A play verb skips login only when this process is not saving characters.
		if srv != nil && !srv.Persist && isDirectPlayVerb(line) && (packID == "" || !save.AccountExists(packID, line)) {
			return server.ConnectMsg{
				Name:    "you",
				Command: line,
			}, true
		}

		username := line
		if _, err := save.Slug(username); err != nil {
			_, _ = io.WriteString(conn, "Invalid account name.\r\n")
			continue
		}

		var acc *save.Account
		if packID != "" && save.AccountExists(packID, username) {
			_ = SuppressEcho(conn)
			_, _ = io.WriteString(conn, "Password: ")
			pass, err := tr.ReadLine()
			_ = RestoreEcho(conn)
			_, _ = io.WriteString(conn, "\r\n")
			if err != nil {
				return server.ConnectMsg{}, false
			}
			acc, err = save.AuthenticateAccount(packID, username, pass)
			if err != nil {
				_, _ = io.WriteString(conn, pal.Error.Render("Invalid password.")+"\r\n")
				continue
			}
		} else {
			_ = SuppressEcho(conn)
			_, _ = io.WriteString(conn, "New account. Password: ")
			pass, err := tr.ReadLine()
			_ = RestoreEcho(conn)
			_, _ = io.WriteString(conn, "\r\n")
			if err != nil {
				return server.ConnectMsg{}, false
			}
			pass = strings.TrimSpace(pass)
			if pass == "" {
				_, _ = io.WriteString(conn, pal.Error.Render("Password cannot be empty.")+"\r\n")
				continue
			}
			if packID != "" {
				acc, err = save.CreateAccount(packID, username, pass)
				if err != nil {
					_, _ = io.WriteString(conn, pal.Error.Render(err.Error())+"\r\n")
					continue
				}
			} else {
				acc = &save.Account{ID: strings.ToLower(username), Username: username}
			}
		}

		charName, isExisting, ok := selectOrPromptCharacter(conn, tr, packID, acc)
		if !ok {
			return server.ConnectMsg{}, false
		}
		if isExisting {
			return server.ConnectMsg{
				Account: acc.ID,
				Persist: true,
				Name:    charName,
			}, true
		}

		raceID, roleID, attrs, ok := runTelnetChargen(conn, tr, p)
		if !ok {
			return server.ConnectMsg{}, false
		}
		return server.ConnectMsg{
			Account: acc.ID,
			Persist: true,
			Name:    charName,
			Race:    raceID,
			Role:    roleID,
			Attrs:   attrs,
		}, true
	}
	return server.ConnectMsg{}, false
}

func selectOrPromptCharacter(conn stdnet.Conn, tr *TelnetReader, packID string, acc *save.Account) (string, bool, bool) {
	for {
		if len(acc.Characters) > 0 {
			_, _ = io.WriteString(conn, "Characters:\r\n")
			for i, ch := range acc.Characters {
				_, _ = io.WriteString(conn, fmt.Sprintf("  %d) %s\r\n", i+1, ch))
			}
			_, _ = io.WriteString(conn, "Character (number, name, or new name): ")
		} else {
			_, _ = io.WriteString(conn, "Character name: ")
		}
		line, err := tr.ReadLine()
		if err != nil {
			return "", false, false
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if len(acc.Characters) > 0 {
				return acc.Characters[0], true, true
			}
			continue
		}
		if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(acc.Characters) {
			return acc.Characters[n-1], true, true
		}
		for _, ch := range acc.Characters {
			if strings.EqualFold(ch, line) {
				return ch, true, true
			}
		}
		if _, err := save.Slug(line); err != nil {
			_, _ = io.WriteString(conn, "Invalid character name.\r\n")
			continue
		}
		if packID != "" && save.HasPlayer(packID, line) {
			ps, err := save.ReadPlayer(packID, line)
			if err != nil || ps == nil || ps.Account == "" || !strings.EqualFold(ps.Account, acc.ID) {
				_, _ = io.WriteString(conn, "That character name is already taken.\r\n")
				continue
			}
			return ps.Character, true, true
		}
		return line, false, true
	}
}

func runTelnetChargen(conn io.Writer, tr *TelnetReader, p *pack.Pack) (string, string, map[string]int, bool) {
	if p == nil || !p.RPG.ChargenEnabled() {
		return "", "", nil, true
	}
	var raceID, roleID string
	if len(p.RPG.Races) > 0 {
		chosen, ok := promptBundleChoice(conn, tr, "Origin", p.RPG.Races)
		if !ok {
			return "", "", nil, false
		}
		raceID = chosen
	}
	if len(p.RPG.Roles) > 0 {
		chosen, ok := promptBundleChoice(conn, tr, "Role", p.RPG.Roles)
		if !ok {
			return "", "", nil, false
		}
		roleID = chosen
	}
	var attrs map[string]int
	budget := p.RPG.Chargen.Budget()
	if budget > 0 && len(p.RPG.Attributes) > 0 {
		var ok bool
		attrs, ok = promptPointBuy(conn, tr, p.RPG, raceID, roleID, budget)
		if !ok {
			return "", "", nil, false
		}
	}
	return raceID, roleID, attrs, true
}

func promptBundleChoice(conn io.Writer, tr *TelnetReader, label string, bundles []rpg.Bundle) (string, bool) {
	_, _ = io.WriteString(conn, fmt.Sprintf("Choose %s:\r\n", label))
	for i, b := range bundles {
		desc := ""
		if b.Description != "" {
			desc = " — " + b.Description
		}
		_, _ = io.WriteString(conn, fmt.Sprintf("  %d) %s%s\r\n", i+1, b.Name, desc))
	}
	for {
		_, _ = io.WriteString(conn, fmt.Sprintf("%s (1-%d): ", label, len(bundles)))
		line, err := tr.ReadLine()
		if err != nil {
			return "", false
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return bundles[0].ID, true
		}
		if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(bundles) {
			return bundles[n-1].ID, true
		}
		for _, b := range bundles {
			if strings.EqualFold(b.ID, line) || strings.EqualFold(b.Name, line) {
				return b.ID, true
			}
		}
	}
}

func promptPointBuy(conn io.Writer, tr *TelnetReader, schema rpg.Schema, raceID, roleID string, budget int) (map[string]int, bool) {
	bonuses := map[string]int{}
	baseVals := map[string]int{}
	for _, a := range schema.Attributes {
		baseVals[a.Key] = a.Base
	}
	if b := schema.Bundle(schema.Races, raceID); b != nil {
		for k, v := range b.Attrs {
			baseVals[k] += v
		}
	}
	if b := schema.Bundle(schema.Roles, roleID); b != nil {
		for k, v := range b.Attrs {
			baseVals[k] += v
		}
	}
	remaining := budget
	for remaining > 0 {
		_, _ = io.WriteString(conn, fmt.Sprintf("Attribute points remaining: %d\r\n", remaining))
		for i, a := range schema.Attributes {
			label := a.Label
			if label == "" {
				label = a.Key
			}
			cur := baseVals[a.Key] + bonuses[a.Key]
			maxNote := ""
			if a.Max > 0 {
				maxNote = fmt.Sprintf("/%d", a.Max)
			}
			_, _ = io.WriteString(conn, fmt.Sprintf("  %d) %s: %d%s\r\n", i+1, label, cur, maxNote))
		}
		_, _ = io.WriteString(conn, "Spend point on (number, name, or 'done'): ")
		line, err := tr.ReadLine()
		if err != nil {
			return nil, false
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "done") {
			break
		}
		idx := -1
		if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(schema.Attributes) {
			idx = n - 1
		} else {
			for i, a := range schema.Attributes {
				if strings.EqualFold(a.Key, line) || strings.EqualFold(a.Label, line) {
					idx = i
					break
				}
			}
		}
		if idx < 0 {
			_, _ = io.WriteString(conn, "Unknown attribute.\r\n")
			continue
		}
		attr := schema.Attributes[idx]
		cur := baseVals[attr.Key] + bonuses[attr.Key]
		if attr.Max > 0 && cur >= attr.Max {
			_, _ = io.WriteString(conn, fmt.Sprintf("%s is already at its maximum (%d).\r\n", attr.Label, attr.Max))
			continue
		}
		bonuses[attr.Key]++
		remaining--
	}
	return bonuses, true
}

func telnetWriteLoop(ctx context.Context, conn stdnet.Conn, sess *server.Session, pal *theme.Palette, connClosed <-chan struct{}, writerDone chan struct{}) {
	defer close(writerDone)
	defer conn.Close()
	tw := &telnetSessionWriter{w: conn, pal: pal, needPrompt: true}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		sess.Flush()
		select {
		case <-connClosed:
			drainTelnetSession(tw, sess)
			return
		case ev, ok := <-sess.Out:
			if !ok {
				return
			}
			if err := tw.writeEvent(ev); err != nil {
				return
			}
			if _, isDisc := ev.(protocol.DisconnectEvent); isDisc {
				return
			}
		case <-ticker.C:
			if ctx.Err() != nil && sess.Closed() && !sess.HasOutbound() {
				return
			}
		}
	}
}

func drainTelnetSession(tw *telnetSessionWriter, sess *server.Session) {
	sess.Flush()
	for {
		select {
		case ev, ok := <-sess.Out:
			if !ok {
				return
			}
			if err := tw.writeEvent(ev); err != nil {
				return
			}
			sess.Flush()
			if _, isDisc := ev.(protocol.DisconnectEvent); isDisc {
				return
			}
		default:
			return
		}
	}
}

type telnetSessionWriter struct {
	w               io.Writer
	pal             *theme.Palette
	printedRoomText bool
	lastRoomID      string
	needPrompt      bool
}

func (tw *telnetSessionWriter) writeEvent(ev protocol.Event) error {
	switch e := ev.(type) {
	case protocol.TextEvent:
		if e.Channel == protocol.ChanRoom {
			tw.printedRoomText = true
		}
		tw.needPrompt = true
		out := FormatTelnetEvent(tw.pal, e)
		if out == "" {
			return nil
		}
		return writeTelnet(tw.w, out)
	case protocol.RoomEvent:
		if !tw.printedRoomText && e.ID != tw.lastRoomID {
			tw.lastRoomID = e.ID
			tw.needPrompt = true
			out := FormatTelnetEvent(tw.pal, e)
			if out != "" {
				return writeTelnet(tw.w, out)
			}
		}
		tw.lastRoomID = e.ID
		return nil
	case protocol.PromptEvent:
		if !tw.needPrompt {
			return nil
		}
		tw.needPrompt = false
		tw.printedRoomText = false
		out := FormatTelnetEvent(tw.pal, e)
		if out == "" {
			return nil
		}
		return writeTelnet(tw.w, out)
	case protocol.DisconnectEvent:
		out := FormatTelnetEvent(tw.pal, e)
		if out == "" {
			return nil
		}
		return writeTelnet(tw.w, out)
	default:
		return nil
	}
}

// StripANSI removes ANSI CSI escape sequences for plain-text assertions in tests.
func StripANSI(s string) string {
	var b bytes.Buffer
	inEsc := false
	for i := 0; i < len(s); i++ {
		if !inEsc {
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
				inEsc = true
				i++
				continue
			}
			b.WriteByte(s[i])
			continue
		}
		if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
			inEsc = false
		}
	}
	return b.String()
}
