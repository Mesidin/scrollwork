package server

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"sudengine/internal/combat"
	"sudengine/internal/commands"
	"sudengine/internal/olc"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/quest"
	"sudengine/internal/render"
	"sudengine/internal/save"
	"sudengine/internal/script"
	"sudengine/internal/weather"
	"sudengine/internal/world"
)

type capture struct {
	buf  []string
	done func(text string)
}

type Session struct {
	ID           string
	Account      string
	Persist      bool
	Mode         protocol.Mode
	PlayerID     world.ID
	Out          chan protocol.Event
	In           chan string
	BuildWalk    bool
	quit         bool
	cap          *capture
	mu           sync.Mutex
	stalledTicks int
	pending      []protocol.Event
	closed       bool
	cmdQueue     []string
	inflight     int
	monitored    bool
	pumpStop     chan struct{}
	pwBusy       bool
	pwRetry      time.Time
}

// passwordRetryAfter is how long a session waits after a failed password check
// before another bcrypt compare is started.
const passwordRetryAfter = 2 * time.Second

// MaxQueuedCommands matches the local input buffer. A framed client must not
// grow the per-session queue without a bound.
const MaxQueuedCommands = 32

func isCoalescable(ev protocol.Event) bool {
	switch ev.(type) {
	case protocol.VitalsEvent, protocol.MapEvent, protocol.CombatEvent, protocol.PromptEvent:
		return true
	default:
		return false
	}
}

func sameEventType(a, b protocol.Event) bool {
	switch a.(type) {
	case protocol.VitalsEvent:
		_, ok := b.(protocol.VitalsEvent)
		return ok
	case protocol.MapEvent:
		_, ok := b.(protocol.MapEvent)
		return ok
	case protocol.CombatEvent:
		_, ok := b.(protocol.CombatEvent)
		return ok
	case protocol.PromptEvent:
		_, ok := b.(protocol.PromptEvent)
		return ok
	default:
		return false
	}
}

// Send queues ev for this session. Vitals, map, combat, and prompt events
// replace an older copy still waiting in the side queue. Room text and
// command results stay queued. Out is only written, never read, so a client
// receive cannot reorder the log.
func (s *Session) Send(ev protocol.Event) {
	if s == nil || s.Out == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.enqueueLocked(ev)
	s.flushLocked()
}

// Flush moves queued events onto Out. The client calls it before each read
// so a full buffer does not leave narrative stuck until the next Send.
func (s *Session) Flush() {
	if s == nil || s.Out == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked()
}

// Disconnect queues a final event and ignores later sends. Events already
// queued, including this one, stay available for Flush.
func (s *Session) Disconnect(reason string) {
	if s == nil || s.Out == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.enqueueLocked(protocol.DisconnectEvent{Reason: reason})
	s.closed = true
	s.flushLocked()
}

func (s *Session) Closed() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Session) enqueueLocked(ev protocol.Event) {
	if isCoalescable(ev) {
		for i := len(s.pending) - 1; i >= 0; i-- {
			if sameEventType(s.pending[i], ev) {
				s.pending[i] = ev
				return
			}
		}
	}
	s.pending = append(s.pending, ev)
}

func (s *Session) flushLocked() {
	if s.Out == nil {
		return
	}
	for len(s.pending) > 0 {
		select {
		case s.Out <- s.pending[0]:
			s.pending = s.pending[1:]
		default:
			return
		}
	}
}

func (s *Session) EnqueueCommand(line string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(s.cmdQueue) >= MaxQueuedCommands {
		return false
	}
	s.cmdQueue = append(s.cmdQueue, line)
	return true
}

// AcceptsCommand reports whether another command can be queued without
// passing MaxQueuedCommands. inflight counts commands accepted but not yet
// finished, including ones still on the server channel.
func (s *Session) AcceptsCommand() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	return len(s.cmdQueue) < MaxQueuedCommands && s.inflight < MaxQueuedCommands
}

// HasOutbound reports events waiting in the side queue or in Out.
func (s *Session) HasOutbound() bool {
	if s == nil || s.Out == nil {
		return false
	}
	s.mu.Lock()
	pending := len(s.pending) > 0
	s.mu.Unlock()
	return pending || len(s.Out) > 0
}

func (s *Session) trackCommand() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.inflight++
	s.mu.Unlock()
}

func (s *Session) finishCommand() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.inflight > 0 {
		s.inflight--
	}
	s.mu.Unlock()
}

func (s *Session) commandsInFlight() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inflight
}

func (s *Session) stopInput() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pumpStop == nil {
		return
	}
	select {
	case <-s.pumpStop:
	default:
		close(s.pumpStop)
	}
}

func (s *Session) PopCommand() (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cmdQueue) == 0 {
		return "", false
	}
	cmd := s.cmdQueue[0]
	s.cmdQueue = s.cmdQueue[1:]
	return cmd, true
}

func (s *Session) QueuedCommands() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.cmdQueue))
	copy(out, s.cmdQueue)
	return out
}

func (s *Session) CommandQueueLen() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cmdQueue)
}

type ServerMsg interface {
	isServerMsg()
}

type CommandMsg struct {
	Session *Session
	Line    string
}

func (CommandMsg) isServerMsg() {}

type ConnectMsg struct {
	Session *Session
	Account string
	Persist bool
	Name    string
	Race    string
	Role    string
	Attrs   map[string]int
	Command string
}

func (ConnectMsg) isServerMsg() {}

type DisconnectMsg struct {
	Session *Session
	Reason  string
}

func (DisconnectMsg) isServerMsg() {}

type passwordResultMsg struct {
	Session *Session
	Err     error
}

func (passwordResultMsg) isServerMsg() {}

type Server struct {
	Pack          *pack.Pack
	World         *world.World
	Scripts       *script.Host
	Combat        *combat.Engine
	Session       *Session
	Sessions      map[string]*Session
	In            chan ServerMsg
	Env           *weather.Environment
	rng           *rand.Rand
	localQuit     bool
	stopped       chan struct{}
	stopOnce      sync.Once
	Multiplayer   bool
	Persist       bool
	GraceTicks    int
	AutosaveTicks int
	Linkdead      map[world.ID]int64
	owners        map[world.ID]string
}

func (s *Server) sessionFor(player *world.Entity) *Session {
	if player == nil || s == nil {
		return nil
	}
	return s.sessionForID(player.ID)
}

func (s *Server) sessionForID(playerID world.ID) *Session {
	if s == nil || playerID == "" {
		return nil
	}
	for _, sess := range s.Sessions {
		if sess != nil && sess.PlayerID == playerID {
			return sess
		}
	}
	if s.Session != nil && s.Session.PlayerID == playerID {
		return s.Session
	}
	return nil
}

func (s *Server) allSessions() []*Session {
	if s == nil {
		return nil
	}
	if len(s.Sessions) == 0 {
		if s.Session != nil {
			return []*Session{s.Session}
		}
		return nil
	}
	seen := map[string]bool{}
	var out []*Session
	for _, sess := range s.Sessions {
		if sess != nil && !sess.Closed() && !seen[sess.ID] {
			seen[sess.ID] = true
			out = append(out, sess)
		}
	}
	if s.Session != nil && !s.Session.Closed() && !seen[s.Session.ID] {
		out = append(out, s.Session)
	}
	return out
}

func (s *Server) AttachSession(sess *Session) {
	if s == nil || sess == nil {
		return
	}
	if s.Sessions == nil {
		s.Sessions = make(map[string]*Session)
	}
	if s.In == nil {
		s.In = make(chan ServerMsg, 128)
	}
	s.Sessions[sess.ID] = sess
	s.monitorSessionIn(sess)
}

func (s *Server) monitorSessionIn(sess *Session) {
	if s == nil || sess == nil || sess.In == nil {
		return
	}
	sess.mu.Lock()
	if sess.monitored {
		sess.mu.Unlock()
		return
	}
	sess.monitored = true
	if sess.pumpStop == nil {
		sess.pumpStop = make(chan struct{})
	}
	stop := sess.pumpStop
	sess.mu.Unlock()
	go func() {
		for {
			select {
			case <-stop:
				for {
					select {
					case line, ok := <-sess.In:
						if !ok {
							return
						}
						s.Enqueue(sess, line)
					default:
						return
					}
				}
			case line, ok := <-sess.In:
				if !ok {
					return
				}
				s.Enqueue(sess, line)
			}
		}
	}()
}

func (s *Server) Enqueue(sess *Session, line string) bool {
	if s == nil || sess == nil {
		return false
	}
	if !s.waitCommandRoom(sess) {
		return false
	}
	sess.trackCommand()
	if !s.enqueueMsg(CommandMsg{Session: sess, Line: line}) {
		sess.finishCommand()
		return false
	}
	return true
}

func (s *Server) EnqueueConnect(sess *Session, name, race, role, command string) bool {
	if s == nil || sess == nil {
		return false
	}
	return s.enqueueMsg(ConnectMsg{
		Session: sess,
		Account: sess.Account,
		Persist: sess.Persist,
		Name:    name,
		Race:    race,
		Role:    role,
		Command: command,
	})
}

func (s *Server) EnqueueConnectChar(msg ConnectMsg) bool {
	if s == nil || msg.Session == nil {
		return false
	}
	return s.enqueueMsg(msg)
}

func (s *Server) EnqueueDisconnect(sess *Session, reason string) bool {
	if s == nil || sess == nil {
		return false
	}
	return s.enqueueMsg(DisconnectMsg{Session: sess, Reason: reason})
}

func (s *Server) Stopped() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.stopped
}

func (s *Server) ensureRuntime() {
	if s == nil {
		return
	}
	if s.In == nil {
		s.In = make(chan ServerMsg, 128)
	}
	if s.stopped == nil {
		s.stopped = make(chan struct{})
	}
	if s.Sessions == nil {
		s.Sessions = make(map[string]*Session)
	}
	if s.Linkdead == nil {
		s.Linkdead = make(map[world.ID]int64)
	}
}

func (s *Server) markStopped() {
	if s == nil {
		return
	}
	if s.stopped == nil {
		s.stopped = make(chan struct{})
	}
	s.stopOnce.Do(func() { close(s.stopped) })
}

func (s *Server) isStopped() bool {
	if s == nil || s.stopped == nil {
		return false
	}
	select {
	case <-s.stopped:
		return true
	default:
		return false
	}
}

// enqueueMsg hands msg to the actor. After Run has stopped, it returns
// instead of blocking on a channel nobody reads.
func (s *Server) enqueueMsg(msg ServerMsg) bool {
	if s == nil {
		return false
	}
	if s.In == nil {
		s.In = make(chan ServerMsg, 128)
	}
	if s.stopped == nil {
		s.In <- msg
		return true
	}
	select {
	case <-s.stopped:
		return false
	default:
	}
	select {
	case <-s.stopped:
		return false
	case s.In <- msg:
		return true
	}
}

// waitCommandRoom blocks until the session can take another command, or the
// actor has stopped. One producer per session keeps the cap check stable.
func (s *Server) waitCommandRoom(sess *Session) bool {
	for {
		if sess.Closed() || s.isStopped() {
			return false
		}
		if sess.AcceptsCommand() {
			return true
		}
		timer := time.NewTimer(10 * time.Millisecond)
		if s.stopped == nil {
			<-timer.C
			continue
		}
		select {
		case <-s.stopped:
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

func (s *Server) SpawnPlayerSession(name, raceID, roleID string) (*Session, *world.Entity) {
	pl := pack.SpawnPlayer(s.World, s.Pack, name, raceID, roleID)
	id := name
	if id == "" || (s.Sessions != nil && s.Sessions[id] != nil) {
		id = string(pl.ID)
	}
	sess := &Session{
		ID:       id,
		Mode:     protocol.ModePlay,
		PlayerID: pl.ID,
		Out:      make(chan protocol.Event, 128),
		In:       make(chan string, 32),
	}
	s.AttachSession(sess)
	return sess, pl
}

// bindConnectedPlayer spawns or reconnects a remote player onto an existing session shell.
// The actor calls it so the world and the session map stay single-threaded.
func (s *Server) bindConnectedPlayer(m ConnectMsg) error {
	sess := m.Session
	if sess == nil {
		return fmt.Errorf("missing session")
	}
	if s.World == nil || s.Pack == nil {
		return fmt.Errorf("server is not ready")
	}
	name := strings.TrimSpace(m.Name)
	if name == "" {
		name = "you"
	}
	if m.Account != "" {
		sess.Account = m.Account
	}
	if s.Persist && sess.Account == "" {
		return fmt.Errorf("login required")
	}
	sess.Persist = m.Persist || sess.Account != ""
	var pl *world.Entity
	if sess.Persist {
		if err := save.RecoverPaired(s.Pack.Meta.ID); err != nil {
			return err
		}
		if s.Sessions != nil {
			if existing := s.Sessions[name]; existing != nil && existing != sess && !existing.Closed() {
				return fmt.Errorf("%s is already connected", name)
			}
		}
		for _, e := range s.World.Entities {
			if e == nil || e.Kind != world.KindPlayer || !strings.EqualFold(e.Name, name) {
				continue
			}
			if live := s.sessionFor(e); live != nil && live != sess && !live.Closed() {
				return fmt.Errorf("%s is already connected", e.Name)
			}
			if !s.accountOwns(e, sess.Account) {
				return fmt.Errorf("that character belongs to another account")
			}
			pl = e
			pl.SetFlag("linkdead", false)
			if s.Linkdead != nil {
				delete(s.Linkdead, pl.ID)
			}
			s.rememberOwner(pl.ID, sess.Account)
			break
		}
	}

	if pl == nil && sess.Persist && save.HasPlayer(s.Pack.Meta.ID, name) {
		ps, err := save.ReadPlayer(s.Pack.Meta.ID, name)
		if err != nil {
			return err
		}
		if ps.Account == "" || !strings.EqualFold(ps.Account, sess.Account) {
			return fmt.Errorf("that character belongs to another account")
		}
		attached, err := save.AttachPlayer(s.World, ps)
		if err != nil {
			return err
		}
		pl = attached
		s.Pack.RPG.StampPiles(s.World)
		s.rememberOwner(pl.ID, ps.Account)
	}

	if pl == nil {
		// Guests may omit an origin or role. A saved character must pick each
		// list the pack offers. A zero point-buy budget drops client spends.
		race, role, spend, err := s.Pack.RPG.AcceptChargen(m.Race, m.Role, m.Attrs, sess.Persist)
		if err != nil {
			return err
		}
		pl = pack.SpawnPlayer(s.World, s.Pack, name, race, role)
		for k, v := range spend {
			if pl.Attrs == nil {
				pl.Attrs = map[string]int{}
			}
			pl.Attrs[k] += v
		}
		if sess.Account != "" {
			s.rememberOwner(pl.ID, sess.Account)
		}
		if sess.Persist {
			if err := s.SaveWorld(); err != nil {
				s.markLinkdead(pl)
				sess.PlayerID = pl.ID
				return fmt.Errorf("could not save character: %w", err)
			}
			if sess.Account != "" {
				if err := save.AddCharacterToAccount(s.Pack.Meta.ID, sess.Account, pl.Name); err != nil {
					sess.Send(protocol.TextEvent{
						Channel: protocol.ChanSystem,
						Text:    "Could not add character to account: " + err.Error(),
					})
				}
			}
		}
	}

	if sess.ID == "" {
		sess.ID = name
	}
	if s.Sessions != nil {
		if existing := s.Sessions[sess.ID]; existing != nil && existing != sess && !existing.Closed() {
			sess.ID = string(pl.ID)
		}
	}
	if sess.Mode == "" {
		sess.Mode = protocol.ModePlay
	}
	sess.PlayerID = pl.ID
	if sess.Out == nil {
		sess.Out = make(chan protocol.Event, 128)
	}
	if sess.In == nil {
		sess.In = make(chan string, 32)
	}
	return nil
}

func (s *Server) rememberOwner(id world.ID, account string) {
	if s == nil || id == "" || account == "" {
		return
	}
	if s.owners == nil {
		s.owners = map[world.ID]string{}
	}
	s.owners[id] = account
}

func (s *Server) accountOwns(pl *world.Entity, account string) bool {
	if pl == nil || account == "" {
		return false
	}
	if s.owners != nil {
		if own, ok := s.owners[pl.ID]; ok {
			return own != "" && strings.EqualFold(own, account)
		}
	}
	if s.Pack == nil {
		return false
	}
	ps, err := save.ReadPlayer(s.Pack.Meta.ID, pl.Name)
	if err != nil || ps == nil {
		return false
	}
	return ps.Account != "" && strings.EqualFold(ps.Account, account)
}

func (s *Server) sendTo(player *world.Entity, ev protocol.Event) {
	if player == nil {
		return
	}
	if sess := s.sessionFor(player); sess != nil {
		sess.Send(ev)
	}
}

func (s *Server) sendRoom(room *world.Entity, except *world.Entity, ev protocol.Event) {
	var skip []world.ID
	if except != nil {
		skip = []world.ID{except.ID}
	}
	s.sendRoomSkip(room, skip, ev)
}

func (s *Server) sendRoomSkip(room *world.Entity, skip []world.ID, ev protocol.Event) {
	if s == nil || s.World == nil || room == nil {
		return
	}
	skipSet := map[world.ID]bool{}
	for _, id := range skip {
		skipSet[id] = true
	}
	for _, child := range s.World.Children(room.ID) {
		if child.Kind == world.KindPlayer && !skipSet[child.ID] {
			s.sendTo(child, ev)
		}
	}
}

func (s *Server) playerInRoom(room *world.Entity) *world.Entity {
	if s == nil || s.World == nil || room == nil {
		return nil
	}
	primary := s.Pack.RPG.Primary()
	for _, child := range s.World.Children(room.ID) {
		if child.Kind == world.KindPlayer && child.Alive(primary) {
			return child
		}
	}
	return nil
}

type Options struct {
	PackDir       string
	Mode          protocol.Mode
	Slot          string // if set, load snapshot instead of fresh instantiate
	Seed          int64
	PlayerName    string
	RaceID        string
	RoleID        string
	TickMS        int
	Persist       bool
	GraceTicks    int
	AutosaveTicks int
}

func Launch(opt Options) (*Server, *Session, error) {
	p, err := pack.Load(opt.PackDir)
	if err != nil {
		return nil, nil, err
	}
	if opt.TickMS > 0 {
		p.Meta.TickMS = opt.TickMS
	}
	var w *world.World
	if opt.Mode != protocol.ModeBuild {
		if err := p.Validate(); err != nil {
			return nil, nil, err
		}
	}
	if opt.Slot != "" {
		w, err = save.Read(p.Meta.ID, opt.Slot)
		if err != nil {
			return nil, nil, err
		}
	} else {
		w, err = p.Instantiate()
		if err != nil {
			return nil, nil, err
		}
		name := opt.PlayerName
		if name == "" {
			name = "you"
		}
		pack.SpawnPlayer(w, p, name, opt.RaceID, opt.RoleID)
	}
	seed := opt.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	sess := &Session{
		ID:       "local",
		Mode:     opt.Mode,
		PlayerID: w.PlayerID,
		Out:      make(chan protocol.Event, 128),
		In:       make(chan string, 32),
	}
	if sess.Mode == "" {
		sess.Mode = protocol.ModePlay
	}
	srv := newServerInstance(p, w, seed)
	srv.Persist = opt.Persist
	srv.GraceTicks = opt.GraceTicks
	srv.AutosaveTicks = opt.AutosaveTicks
	srv.Session = sess
	srv.Sessions[sess.ID] = sess
	srv.monitorSessionIn(sess)
	// Douse after the local session is attached so the flame line is not dropped.
	if srv.Env != nil && srv.Env.Extinguishes(true) {
		srv.douseOutdoorFlames()
	}
	return srv, sess, nil
}

func LaunchServer(opt Options) (*Server, error) {
	if opt.Mode == protocol.ModeBuild {
		return nil, fmt.Errorf("build mode is not permitted in server mode")
	}
	p, err := pack.Load(opt.PackDir)
	if err != nil {
		return nil, err
	}
	if opt.TickMS > 0 {
		p.Meta.TickMS = opt.TickMS
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if opt.Persist && opt.Slot == "" {
		if err := save.RecoverPaired(p.Meta.ID); err != nil {
			return nil, err
		}
	}
	var w *world.World
	if opt.Slot != "" {
		w, err = save.Read(p.Meta.ID, opt.Slot)
		if err != nil {
			return nil, err
		}
	} else if opt.Persist && save.HasWorld(p.Meta.ID) {
		w, err = save.ReadWorld(p.Meta.ID)
		if err != nil {
			return nil, err
		}
		p.RPG.StampPiles(w)
	} else {
		w, err = p.Instantiate()
		if err != nil {
			return nil, err
		}
	}
	seed := opt.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	srv := newServerInstance(p, w, seed)
	srv.Multiplayer = true
	srv.Persist = opt.Persist
	srv.GraceTicks = opt.GraceTicks
	srv.AutosaveTicks = opt.AutosaveTicks
	// No player yet. Lights still go out; there is nobody to tell.
	if srv.Env != nil && srv.Env.Extinguishes(true) {
		srv.douseOutdoorFlames()
	}
	return srv, nil
}

func newServerInstance(p *pack.Pack, w *world.World, seed int64) *Server {
	h := script.New(p.Scripts)
	h.World = w
	srv := &Server{
		Pack:     p,
		World:    w,
		Scripts:  h,
		Sessions: make(map[string]*Session),
		Linkdead: make(map[world.ID]int64),
		owners:   make(map[world.ID]string),
		In:       make(chan ServerMsg, 128),
		stopped:  make(chan struct{}),
		Env:      weather.New(p.Meta.TickMS, p.Time, p.Weather),
		rng:      rand.New(rand.NewSource(seed)),
	}
	if w.WeatherState != "" {
		srv.Env.CurrentWeather = w.WeatherState
	}
	srv.Env.Sync(w.Tick)
	w.IsAmbientDark = func(w *world.World, room *world.Entity) bool {
		return srv.Env.IsAmbientDark(room.HasFlag)
	}
	w.IsAmbientLit = func(w *world.World, room *world.Entity) bool {
		return srv.Env.IsAmbientLit(room.HasFlag)
	}
	srv.Combat = &combat.Engine{
		RNG:     srv.rng,
		Primary: p.RPG.Primary(),
		Every:   p.Meta.CombatEvery,
		Rules:   &p.RPG,
		Emit: func(to *world.Entity, channel, text string, hear combat.Hear, alsoExcept ...*world.Entity) {
			if to == nil {
				return
			}
			ev := protocol.TextEvent{Channel: protocol.Channel(channel), Text: text}
			switch hear {
			case combat.HearPrivate:
				srv.sendTo(to, ev)
			case combat.HearOthers:
				room := w.RoomOf(to)
				if room == nil {
					return
				}
				skip := make([]world.ID, 0, 1+len(alsoExcept))
				skip = append(skip, to.ID)
				for _, ex := range alsoExcept {
					if ex != nil {
						skip = append(skip, ex.ID)
					}
				}
				srv.sendRoomSkip(room, skip, ev)
			default:
				if room := w.RoomOf(to); room != nil {
					srv.sendRoom(room, nil, ev)
					return
				}
				srv.sendTo(to, ev)
			}
		},
	}
	srv.Combat.HitMod = srv.outdoorHitPenalty
	h.Echo = func(actor *world.Entity, text string) {
		if actor != nil {
			srv.sendTo(actor, protocol.TextEvent{Channel: protocol.ChanNarrative, Text: text})
		}
	}
	h.EchoRoom = func(room *world.Entity, text string, skip *world.Entity) {
		srv.sendRoom(room, skip, protocol.TextEvent{Channel: protocol.ChanNarrative, Text: text})
	}
	h.Damage = func(att, def *world.Entity, amount int) {
		srv.harmEntity(att, def, amount, "strike")
	}
	h.Move = srv.moveEntity
	return srv
}

func (s *Server) moveEntity(ent, dest *world.Entity) error {
	if s == nil || s.World == nil || ent == nil || dest == nil {
		return fmt.Errorf("invalid entity or destination")
	}
	if dest.Kind != world.KindRoom {
		return fmt.Errorf("destination is not a room")
	}
	if ent.Kind == world.KindRoom {
		return fmt.Errorf("cannot move a room")
	}
	old := s.World.RoomOf(ent)
	if err := s.World.Move(ent.ID, dest.ID); err != nil {
		return err
	}
	changed := old == nil || old.ID != dest.ID
	if changed {
		if ent.Kind == world.KindPlayer {
			endFight(s.World, ent)
			if sess := s.sessionFor(ent); sess != nil {
				commands.Arrive(s.ctx(sess), old, dest)
			}
		}
		for _, e := range s.World.Entities {
			if e != nil && e.Combat != nil && e.Combat.Target == ent.ID {
				endFight(s.World, e)
			}
		}
		if old != nil {
			s.moveFollowers(ent, old, dest, "")
			s.pushRoomIn(old)
		}
		s.pushRoomIn(dest)
	}
	return nil
}

func endFight(w *world.World, e *world.Entity) {
	if w == nil || e == nil || e.Combat == nil || e.Combat.Target == "" {
		return
	}
	if tgt := w.Get(e.Combat.Target); tgt != nil && tgt.Combat != nil && tgt.Combat.Target == e.ID {
		tgt.Combat.Target = ""
		tgt.Combat.NextSwing = 0
	}
	e.Combat.Target = ""
	e.Combat.NextSwing = 0
}

func (s *Server) Run(ctx context.Context) {
	s.ensureRuntime()
	defer s.markStopped()
	if s.Session != nil && s.Sessions[s.Session.ID] == nil {
		s.Sessions[s.Session.ID] = s.Session
	}
	for _, sess := range s.allSessions() {
		s.monitorSessionIn(sess)
	}

	tick := time.Duration(s.Pack.Meta.TickMS) * time.Millisecond
	if tick <= 0 {
		tick = 250 * time.Millisecond
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	s.greet()
	s.pushUI()

	for {
		if s.reapQuits() {
			return
		}
		select {
		case <-ctx.Done():
			s.markStopped()
			if s.Persist {
				_ = s.SaveWorld()
			}
			s.shutdownSessions()
			return
		case <-ticker.C:
			s.tick()
		case msg, ok := <-s.In:
			if !ok {
				return
			}
			s.handleServerMsg(msg)
		}
	}
}

func (s *Server) handleServerMsg(msg ServerMsg) {
	switch m := msg.(type) {
	case ConnectMsg:
		if m.Session == nil {
			break
		}
		if m.Session.PlayerID == "" {
			if err := s.bindConnectedPlayer(m); err != nil {
				m.Session.Send(protocol.TextEvent{Channel: protocol.ChanSystem, Text: err.Error()})
				m.Session.Disconnect("denied")
				break
			}
		}
		s.AttachSession(m.Session)
		s.greet(m.Session)
		s.pushUI(m.Session)
		if pl := s.World.Get(m.Session.PlayerID); pl != nil {
			if room := s.World.RoomOf(pl); room != nil {
				s.pushRoomIn(room)
			}
		}
		if m.Command != "" {
			m.Session.trackCommand()
			if !m.Session.EnqueueCommand(m.Command) {
				m.Session.finishCommand()
			}
		}
	case DisconnectMsg:
		if m.Session != nil {
			s.disconnect(m.Session, m.Reason)
		}
	case CommandMsg:
		if m.Session == nil {
			break
		}
		if !m.Session.EnqueueCommand(m.Line) {
			m.Session.finishCommand()
		}
	case passwordResultMsg:
		s.finishPasswordChange(m.Session, m.Err)
	}
}

func (s *Server) reapQuits() bool {
	if s == nil {
		return false
	}
	var quitting []*Session
	for _, sess := range s.allSessions() {
		if sess != nil && sess.quit {
			quitting = append(quitting, sess)
		}
	}
	for _, sess := range quitting {
		s.disconnect(sess, "quit")
	}
	return s.localQuit
}

func (s *Server) shutdownSessions() {
	for _, sess := range append([]*Session{}, s.allSessions()...) {
		s.dropSession(sess, "shutdown")
	}
}

func (s *Server) dropSession(sess *Session, reason string) bool {
	if s == nil || sess == nil {
		return false
	}
	if s.Sessions != nil {
		delete(s.Sessions, sess.ID)
	}
	isLocal := s.Session == sess
	if isLocal {
		s.localQuit = true
	}
	sess.quit = false
	sess.Disconnect(reason)
	sess.stopInput()
	return isLocal
}

func (s *Server) graceTicks() int {
	if s != nil && s.GraceTicks > 0 {
		return s.GraceTicks
	}
	tickMS := 250
	if s != nil && s.Pack != nil && s.Pack.Meta.TickMS > 0 {
		tickMS = s.Pack.Meta.TickMS
	}
	// Default grace is 60 seconds.
	n := 60000 / tickMS
	if n < 1 {
		n = 1
	}
	return n
}

func (s *Server) autosaveEvery() int {
	if s != nil && s.AutosaveTicks > 0 {
		return s.AutosaveTicks
	}
	return s.graceTicks()
}

// SaveWorld writes every live player and world.json, including each owner's account.
func (s *Server) SaveWorld(players ...*world.Entity) error {
	if s == nil || s.Pack == nil || s.World == nil {
		return nil
	}
	return save.WritePairedOwned(s.Pack.Meta.ID, s.World, s.owners, players...)
}

func (s *Server) markLinkdead(pl *world.Entity) {
	if s == nil || pl == nil {
		return
	}
	if s.Linkdead == nil {
		s.Linkdead = make(map[world.ID]int64)
	}
	pl.SetFlag("linkdead", true)
	s.Linkdead[pl.ID] = s.World.Tick + int64(s.graceTicks())
	if room := s.World.RoomOf(pl); room != nil {
		s.pushRoomIn(room)
	}
}

func (s *Server) reapLinkdead() {
	if s == nil || s.World == nil || len(s.Linkdead) == 0 {
		return
	}
	now := s.World.Tick
	var expired []world.ID
	for pid, due := range s.Linkdead {
		if now >= due {
			expired = append(expired, pid)
		}
	}
	sort.Slice(expired, func(i, j int) bool { return expired[i] < expired[j] })
	for _, pid := range expired {
		pl := s.World.Get(pid)
		if pl == nil {
			delete(s.Linkdead, pid)
			continue
		}
		if s.sessionFor(pl) != nil {
			pl.SetFlag("linkdead", false)
			delete(s.Linkdead, pid)
			continue
		}
		if s.Persist {
			if err := s.SaveWorld(); err != nil {
				s.tellSessions(fmt.Sprintf("Could not save %s: %s", pl.Name, err.Error()))
				continue
			}
		}
		delete(s.Linkdead, pid)
		pl.SetFlag("linkdead", false)
		s.removePlayer(pl)
	}
}

func (s *Server) disconnect(sess *Session, reason string) {
	if s == nil || sess == nil || s.World == nil {
		return
	}
	isLocal := s.dropSession(sess, reason)
	pl := s.World.Get(sess.PlayerID)
	if pl == nil {
		return
	}
	if reason != "quit" || pl.InCombat() {
		if sess.Persist {
			s.Persist = true
		}
		s.markLinkdead(pl)
		return
	}
	if isLocal {
		return
	}
	if s.Persist || sess.Persist {
		if err := s.SaveWorld(); err != nil {
			s.markLinkdead(pl)
			s.tellSessions(fmt.Sprintf("Could not save %s: %s", pl.Name, err.Error()))
			return
		}
	}
	s.removePlayer(pl)
}

func (s *Server) removePlayer(pl *world.Entity) {
	if s == nil || s.World == nil || pl == nil {
		return
	}
	endFight(s.World, pl)
	room := s.World.RoomOf(pl)
	save.RemovePlayerSubgraph(s.World, pl.ID)
	if s.owners != nil {
		delete(s.owners, pl.ID)
	}
	if room != nil {
		s.pushRoomIn(room)
	}
}

func (s *Server) tellSessions(text string) {
	if s == nil || text == "" {
		return
	}
	ev := protocol.TextEvent{Channel: protocol.ChanSystem, Text: text}
	for _, sess := range s.allSessions() {
		if sess != nil {
			sess.Send(ev)
		}
	}
}

func (s *Server) greet(sessions ...*Session) {
	target := sessions
	if len(target) == 0 {
		target = s.allSessions()
	}
	for _, sess := range target {
		title := s.Pack.Meta.Title
		sess.Send(protocol.TitleEvent{Title: title})
		sess.Send(protocol.ModeEvent{Mode: sess.Mode})
		sess.Send(protocol.LayoutEvent{Panes: s.Pack.Meta.UI.Panes})
		banner := title
		if sess.Mode == protocol.ModeBuild {
			banner += "  [BUILD]"
		}
		sess.Send(protocol.TextEvent{Channel: protocol.ChanSystem, Text: banner})
		if sess.Mode == protocol.ModePlay {
			commands.EnterRoom(s.ctx(sess), true)
		} else {
			sess.Send(protocol.TextEvent{Channel: protocol.ChanBuild, Text: "Builder mode. The side pane shows the map and dig, desc, save pack. help building"})
			commands.EnterRoom(s.ctx(sess), true)
		}
	}
}

func (s *Server) ctx(session ...*Session) *commands.Context {
	var sess *Session
	if len(session) > 0 {
		sess = session[0]
	}
	if sess == nil {
		sess = s.Session
	}
	if sess == nil {
		return nil
	}
	actor := s.World.Get(sess.PlayerID)
	c := &commands.Context{
		World:     s.World,
		Actor:     actor,
		Pack:      s.Pack,
		Mode:      sess.Mode,
		Scripts:   s.Scripts,
		Tick:      s.World.Tick,
		RNG:       s.rng,
		Env:       s.Env,
		BuildWalk: &sess.BuildWalk,
		Tell: func(ch protocol.Channel, text string) {
			sess.Send(protocol.TextEvent{Channel: ch, Text: text})
		},
		TellRoom: func(room *world.Entity, ch protocol.Channel, text string, except ...*world.Entity) {
			var exc *world.Entity
			if len(except) > 0 {
				exc = except[0]
			}
			s.sendRoom(room, exc, protocol.TextEvent{Channel: ch, Text: text})
		},
		TellPlayer: func(player *world.Entity, ch protocol.Channel, text string) {
			s.sendTo(player, protocol.TextEvent{Channel: ch, Text: text})
		},
		Broadcast: func(ch protocol.Channel, text string) {
			ev := protocol.TextEvent{Channel: ch, Text: text}
			for _, ses := range s.allSessions() {
				ses.Send(ev)
			}
		},
		ConnectedPlayers: func() []*world.Entity {
			var out []*world.Entity
			for _, ses := range s.allSessions() {
				if pl := s.World.Get(ses.PlayerID); pl != nil {
					out = append(out, pl)
				}
			}
			return out
		},
		OnMove: func(act, old, dest *world.Entity, dir string) {
			if old != nil {
				s.pushRoomIn(old)
			}
			if dest != nil {
				s.pushRoomIn(dest)
			}
		},
		OnFollowerMove: func(f, old, dest, leader *world.Entity, dir string) {
			if f.Kind == world.KindPlayer {
				if sessF := s.sessionFor(f); sessF != nil {
					sessF.Send(protocol.TextEvent{
						Channel: protocol.ChanNarrative,
						Text:    commands.FollowYou(leader, dir),
					})
					commands.Arrive(s.ctx(sessF), old, dest)
					s.pushRoomFor(sessF)
				}
			}
		},
		Quit: func() { sess.quit = true },
		ChangePassword: func() func(oldPass, newPass string) error {
			if sess.Account == "" || s.Pack == nil || s.Pack.Meta.ID == "" {
				return nil
			}
			return func(oldPass, newPass string) error {
				s.startPasswordChange(sess, oldPass, newPass)
				return nil
			}
		}(),
		Harm: func(target *world.Entity, amount int, verb string) {
			if target == nil || actor == nil || s.Combat == nil {
				return
			}
			s.Combat.Damage(s.World, actor, target, amount, verb)
		},
		SaveSnap: func(slot string) error {
			if s.Persist || sess.Persist {
				if slot == "autosave" && actor != nil && actor.InCombat() {
					return nil
				}
				return s.SaveWorld()
			}
			return save.Write(s.Pack.Meta.ID, slot, s.World)
		},
		LoadSnap: func(slot string) error {
			w, err := save.Read(s.Pack.Meta.ID, slot)
			if err != nil {
				return err
			}
			s.World = w
			s.Pack.StampPiles(w)
			s.Scripts.World = w
			sess.PlayerID = w.PlayerID
			if s.Env != nil {
				if w.WeatherState != "" {
					s.Env.CurrentWeather = w.WeatherState
				} else {
					s.Env.CurrentWeather = s.Env.WeatherConfig.Initial
					if s.Env.CurrentWeather == "" {
						s.Env.CurrentWeather = "clear"
					}
				}
				s.Env.Sync(w.Tick)
			}
			w.IsAmbientDark = func(w *world.World, room *world.Entity) bool {
				return s.Env.IsAmbientDark(room.HasFlag)
			}
			w.IsAmbientLit = func(w *world.World, room *world.Entity) bool {
				return s.Env.IsAmbientLit(room.HasFlag)
			}
			if s.Env != nil && s.Env.Extinguishes(true) {
				s.douseOutdoorFlames()
			}
			return nil
		},
		SavePack: func() error {
			s.Pack.SyncFromWorld(s.World)
			return s.Pack.WriteWorld()
		},
		StartFight: func(mob *world.Entity) {
			if mob == nil || actor == nil || mob.InCombat() || !actor.Alive(s.Pack.RPG.Primary()) {
				return
			}
			s.Combat.Start(mob, actor, s.World.Tick)
			s.Combat.CheckAssists(s.World, actor, mob, s.World.Tick, false)
			sess.Send(protocol.TextEvent{
				Channel: protocol.ChanAlert,
				Text:    fmt.Sprintf("%s attacks you!", mob.CapDisplay()),
			})
		},
		Assist: func(target *world.Entity, leaderAttacking bool) {
			if target == nil || s.World == nil || actor == nil {
				return
			}
			s.Combat.CheckAssists(s.World, actor, target, s.World.Tick, leaderAttacking)
		},
		BeginCapture: func(done func(text string)) {
			sess.cap = &capture{done: done}
		},
		ReloadScripts: func() error {
			p, err := pack.Load(s.Pack.Dir)
			if err != nil {
				return err
			}
			s.Pack.Scripts = p.Scripts
			s.Scripts.Reload(p.Scripts)
			return nil
		},
	}
	c.OnMoveFail = func(dir string) bool {
		if sess.Mode != protocol.ModeBuild || !sess.BuildWalk {
			return false
		}
		olc.Dig(c, dir, "New Room")
		return true
	}
	return c
}

func (s *Server) handle(line string) {
	s.handleSession(s.Session, line)
}

// harmEntity applies script damage. Player-versus-player damage is refused
// unless the attacker's room allows pvp, and a new fight alerts the defender.
func (s *Server) harmEntity(att, def *world.Entity, amount int, verb string) {
	if s == nil || s.Combat == nil || def == nil {
		return
	}
	if amount < 0 {
		amount = 0
	}
	playerFight := att != nil && att.ID != def.ID && att.Kind == world.KindPlayer &&
		(def.Kind == world.KindPlayer || def.Kind == world.KindMobile)
	if playerFight && def.Kind == world.KindPlayer {
		room := s.World.RoomOf(att)
		if room == nil || !room.HasFlag("pvp") {
			s.sendTo(att, protocol.TextEvent{
				Channel: protocol.ChanNarrative,
				Text:    "You cannot attack other players here.",
			})
			return
		}
	}
	if playerFight {
		already := def.Combat != nil && def.Combat.Target == att.ID
		if !already && def.Kind == world.KindPlayer {
			s.sendTo(def, protocol.TextEvent{
				Channel: protocol.ChanAlert,
				Text:    fmt.Sprintf("%s attacks you!", att.CapDisplay()),
			})
		}
		if !already {
			s.Combat.Start(att, def, s.World.Tick)
		}
	}
	s.Combat.Damage(s.World, att, def, amount, verb)
}

func (s *Server) startPasswordChange(sess *Session, oldPass, newPass string) {
	if s == nil || sess == nil {
		return
	}
	if sess.Account == "" || s.Pack == nil || s.Pack.Meta.ID == "" {
		sess.Send(protocol.TextEvent{
			Channel: protocol.ChanSystem,
			Text:    "Password changes are only available when logged into a persistent server.",
		})
		return
	}
	oldPass = strings.TrimSpace(oldPass)
	newPass = strings.TrimSpace(newPass)
	if oldPass == "" || newPass == "" {
		sess.Send(protocol.TextEvent{
			Channel: protocol.ChanSystem,
			Text:    "Password change failed: password cannot be empty.",
		})
		return
	}
	if sess.pwBusy {
		sess.Send(protocol.TextEvent{
			Channel: protocol.ChanSystem,
			Text:    "A password change is already running.",
		})
		return
	}
	if time.Now().Before(sess.pwRetry) {
		sess.Send(protocol.TextEvent{
			Channel: protocol.ChanSystem,
			Text:    "Wait a moment before trying another password.",
		})
		return
	}
	sess.pwBusy = true
	packID := s.Pack.Meta.ID
	account := sess.Account
	go func() {
		err := save.ChangePassword(packID, account, oldPass, newPass)
		s.enqueueMsg(passwordResultMsg{Session: sess, Err: err})
	}()
}

func (s *Server) finishPasswordChange(sess *Session, err error) {
	if sess != nil {
		sess.pwBusy = false
		if err != nil {
			sess.pwRetry = time.Now().Add(passwordRetryAfter)
		}
	}
	if sess == nil {
		return
	}
	if err != nil {
		sess.Send(protocol.TextEvent{
			Channel: protocol.ChanSystem,
			Text:    "Password change failed: " + err.Error(),
		})
		return
	}
	sess.Send(protocol.TextEvent{Channel: protocol.ChanSystem, Text: protocol.PasswordUpdated})
}

func (s *Server) handleSession(sess *Session, line string) {
	if sess == nil {
		return
	}
	defer func() {
		if sess.quit {
			s.disconnect(sess, "quit")
		}
	}()
	if sess.cap != nil {
		if strings.TrimSpace(line) == "." {
			text := strings.Join(sess.cap.buf, "\n")
			done := sess.cap.done
			sess.cap = nil
			if done != nil {
				done(text)
			}
			s.pushUI(sess)
			return
		}
		sess.cap.buf = append(sess.cap.buf, line)
		return
	}
	line = strings.TrimSpace(line)
	c := s.ctx(sess)
	if line == "" {
		s.pushUI(sess)
		return
	}
	if oldPass, newPass, ok := protocol.DecodePasswordChange(line); ok {
		s.startPasswordChange(sess, oldPass, newPass)
		s.pushUI(sess)
		return
	}
	p := commands.Parse(line, s.Pack.Lexicon.CanonicalVerb)
	if commands.Dispatch(c, p) {
		s.afterCommand()
		s.pushUI(sess)
		return
	}
	if sess.Mode == protocol.ModeBuild {
		if h := olc.Lookup(p.Verb); h != nil {
			h(c, p)
			s.afterCommand()
			s.pushUI(sess)
			return
		}
	}
	c.Print("I don't understand %q.", p.Verb)
	s.pushUI(sess)
}

func (s *Server) afterCommand() {
	s.reapDead()
}

func (s *Server) tick() {
	s.World.Tick++
	now := s.World.Tick
	for _, sess := range s.allSessions() {
		if cmd, ok := sess.PopCommand(); ok {
			s.handleSession(sess, cmd)
			sess.finishCommand()
		}
	}
	var stalled []*Session
	for _, sess := range s.allSessions() {
		if sess.Out != nil && cap(sess.Out) > 0 && len(sess.Out) == cap(sess.Out) {
			sess.stalledTicks++
			if sess.stalledTicks >= 2 {
				stalled = append(stalled, sess)
			}
		} else {
			sess.stalledTicks = 0
		}
	}
	for _, sess := range stalled {
		s.disconnect(sess, "stalled")
	}
	s.environment()
	s.regen()
	s.ai()
	if s.Combat != nil {
		s.Combat.Tick(s.World, now)
	}
	s.reapDead()
	s.reapLinkdead()
	s.processRespawns()
	if s.Persist {
		every := s.autosaveEvery()
		if every > 0 && now%int64(every) == 0 {
			_ = s.SaveWorld()
		}
	}
	// only push vitals/combat on tick to avoid flooding the log
	s.pushVitals()
	s.pushCombat()
	s.pushPrompt()
}

// Tick lets tests step one tick without Run.
func (s *Server) Tick() {
	s.tick()
}

func (s *Server) processRespawns() {
	if s.World == nil || len(s.World.Respawns) == 0 {
		return
	}
	now := s.World.Tick
	remaining := s.World.Respawns[:0]
	for _, task := range s.World.Respawns {
		if now >= task.DueTick {
			mob, err := s.World.Spawn(task.ProtoID, task.RoomID)
			if err == nil && mob != nil {
				mob.HomeRoom = task.RoomID
				if mob.AI != nil && len(task.Rooms) > 0 {
					mob.AI.Rooms = append([]world.ID{}, task.Rooms...)
				}
				room := s.World.Get(task.RoomID)
				if room != nil {
					s.sendRoom(room, nil, protocol.TextEvent{
						Channel: protocol.ChanNarrative,
						Text:    fmt.Sprintf("%s appears.", mob.CapDisplay()),
					})
					s.pushRoomIn(room)
				}
			}
		} else {
			remaining = append(remaining, task)
		}
	}
	s.World.Respawns = remaining
}

func (s *Server) outdoorHitPenalty(att *world.Entity) int {
	if s == nil || s.Env == nil || s.World == nil || att == nil {
		return 0
	}
	room := s.World.RoomOf(att)
	if room == nil {
		return 0
	}
	return -s.Env.CombatModifier(room.Outdoor())
}

func (s *Server) douseOutdoorFlames() {
	if s == nil || s.Env == nil || s.World == nil || !s.Env.Extinguishes(true) {
		return
	}
	for _, e := range s.World.Entities {
		if e.Kind != world.KindRoom || e.Underground() || !e.Outdoor() {
			continue
		}
		gone := s.World.ExtinguishToggleLights(e)
		if len(gone) > 0 {
			s.sendRoom(e, nil, protocol.TextEvent{
				Channel: protocol.ChanAlert,
				Text:    weather.FlameOutLine,
			})
		}
	}
}

func (s *Server) environment() {
	if s.Env == nil {
		return
	}
	prev := s.World.WeatherState
	broadcasts := s.Env.Tick(s.World.Tick, s.rng)
	changed := prev != s.Env.CurrentWeather
	if changed {
		s.World.WeatherState = s.Env.CurrentWeather
	}
	if changed && s.Env.Extinguishes(true) {
		s.douseOutdoorFlames()
	}
	if len(broadcasts) == 0 {
		return
	}
	for _, sess := range s.allSessions() {
		pl := s.World.Get(sess.PlayerID)
		if pl == nil {
			continue
		}
		room := s.World.RoomOf(pl)
		if room == nil || room.Underground() {
			continue
		}
		isOutdoor := room.Outdoor()
		isWindowed := room.Windowed()

		for _, b := range broadcasts {
			var text string
			if isOutdoor && b.OutdoorText != "" {
				text = b.OutdoorText
			} else if isWindowed && b.WindowText != "" {
				text = b.WindowText
			} else if !isOutdoor && !isWindowed && b.IndoorText != "" {
				text = b.IndoorText
			}
			if text != "" {
				sess.Send(protocol.TextEvent{
					Channel: protocol.ChanAlert,
					Text:    text,
				})
			}
		}
	}
}

func (s *Server) regen() {
	for _, e := range s.World.Entities {
		if e.Kind != world.KindPlayer && e.Kind != world.KindMobile {
			continue
		}
		for _, def := range s.Pack.RPG.Resources {
			if def.Pile || def.Regen == 0 || def.RegenEvery <= 0 {
				continue
			}
			if s.World.Tick%int64(def.RegenEvery) != 0 {
				continue
			}
			if e.InCombat() {
				continue
			}
			r := e.Res(def.Key)
			if r.Current < r.Max {
				e.AdjustRes(def.Key, def.Regen)
			}
		}
	}
	for _, e := range s.World.Entities {
		if e.Gather != nil && e.Gather.Remaining < e.Gather.Max && e.Gather.RegenEvery > 0 {
			if s.World.Tick%int64(e.Gather.RegenEvery) == 0 {
				e.Gather.Remaining++
			}
		}
	}
}

func (s *Server) ai() {
	primary := s.Pack.RPG.Primary()
	for _, e := range s.World.Entities {
		if e.Kind != world.KindMobile || e.AI == nil {
			continue
		}
		if !e.Alive(primary) {
			continue
		}
		room := s.World.RoomOf(e)
		if room == nil {
			continue
		}
		if s.skipWildAI(e) {
			continue
		}
		targetPlayer := s.playerInRoom(room)
		prof := e.AI.Profile
		if targetPlayer != nil {
			if e.AI.Awake(e) && !e.InCombat() && targetPlayer.Alive(primary) {
				s.Combat.Start(e, targetPlayer, s.World.Tick)
				s.Combat.CheckAssists(s.World, targetPlayer, e, s.World.Tick, false)
				s.sendTo(targetPlayer, protocol.TextEvent{
					Channel: protocol.ChanAlert,
					Text:    fmt.Sprintf("%s attacks you!", e.CapDisplay()),
				})
			}
			if prof == "coward" {
				r := e.Res(primary)
				if r.Max > 0 && r.Current*100/r.Max < 30 && len(room.Exits) > 0 {
					for d, ex := range room.Exits {
						if !ex.Closed {
							destRoom := s.World.Get(ex.To)
							_ = s.World.Move(e.ID, ex.To)
							s.sendRoom(room, nil, protocol.TextEvent{
								Channel: protocol.ChanNarrative,
								Text:    fmt.Sprintf("%s flees %s.", e.CapDisplay(), d),
							})
							s.moveFollowers(e, room, destRoom, d)
							s.pushRoomIn(room)
							s.pushRoomIn(destRoom)
							break
						}
					}
				}
			}
		}
		if e.InCombat() {
			s.pursue(e, room)
			continue
		}
		if e.AI.Wander || prof == "wander" {
			every := e.AI.WanderEvery
			if every <= 0 {
				every = 12
			}
			if s.World.Tick%int64(every) != 0 {
				continue
			}
			if s.rng.Intn(100) > 40 {
				continue
			}
			var dirs []world.Exit
			for _, ex := range room.Exits {
				if ex.Closed || !e.AI.Allows(ex.To) {
					continue
				}
				dirs = append(dirs, ex)
			}
			if len(dirs) == 0 {
				continue
			}
			ex := dirs[s.rng.Intn(len(dirs))]
			old := room
			destRoom := s.World.Get(ex.To)
			_ = s.World.Move(e.ID, ex.To)
			skip := entityIDs(s.moveFollowers(e, old, destRoom, ex.Dir))
			s.sendRoom(old, nil, protocol.TextEvent{
				Channel: protocol.ChanNarrative,
				Text:    fmt.Sprintf("%s leaves %s.", e.CapDisplay(), ex.Dir),
			})
			destPlayer := s.aggroTarget(destRoom, skip)
			if destPlayer != nil && e.AI.Awake(e) && destPlayer.Alive(primary) && !e.InCombat() {
				s.Combat.Start(e, destPlayer, s.World.Tick)
				s.Combat.CheckAssists(s.World, destPlayer, e, s.World.Tick, false)
				s.sendTo(destPlayer, protocol.TextEvent{
					Channel: protocol.ChanAlert,
					Text:    fmt.Sprintf("%s arrives — and attacks you!", e.CapDisplay()),
				})
				s.sendRoomSkip(destRoom, append(skip, destPlayer.ID), protocol.TextEvent{
					Channel: protocol.ChanAlert,
					Text:    fmt.Sprintf("%s arrives.", e.CapDisplay()),
				})
			} else {
				s.sendRoomSkip(destRoom, skip, protocol.TextEvent{
					Channel: protocol.ChanNarrative,
					Text:    fmt.Sprintf("%s arrives.", e.CapDisplay()),
				})
			}
			s.pushRoomIn(old)
			s.pushRoomIn(destRoom)
		}
	}
}

func (s *Server) skipWildAI(e *world.Entity) bool {
	if e == nil {
		return false
	}
	if strings.EqualFold(e.Stance, "stay") {
		return true
	}
	if s == nil || s.World == nil {
		return false
	}
	if o := s.World.Get(e.Owner); o != nil && o.Kind == world.KindPlayer {
		return true
	}
	if l := s.World.Get(e.Leader); l != nil && l.Kind == world.KindPlayer {
		return true
	}
	return false
}

func (s *Server) aggroTarget(room *world.Entity, skip []world.ID) *world.Entity {
	if s == nil || s.World == nil || room == nil {
		return nil
	}
	skipSet := map[world.ID]bool{}
	for _, id := range skip {
		skipSet[id] = true
	}
	primary := s.Pack.RPG.Primary()
	for _, child := range s.World.Children(room.ID) {
		if child.Kind == world.KindPlayer && child.Alive(primary) && !skipSet[child.ID] {
			return child
		}
	}
	return nil
}

func entityIDs(ents []*world.Entity) []world.ID {
	if len(ents) == 0 {
		return nil
	}
	ids := make([]world.ID, 0, len(ents))
	for _, e := range ents {
		if e != nil {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

func (s *Server) pursue(e, room *world.Entity) {
	if e == nil || e.AI == nil || !e.AI.Pursue || e.Combat == nil {
		return
	}
	tgt := s.World.Get(e.Combat.Target)
	if tgt == nil {
		return
	}
	dest := s.World.RoomOf(tgt)
	if dest == nil || dest == room {
		return
	}
	var step *world.Exit
	for _, ex := range room.Exits {
		if ex.Closed || ex.To != dest.ID || !e.AI.Allows(ex.To) {
			continue
		}
		chosen := ex
		step = &chosen
		break
	}
	if step == nil {
		return
	}
	old := room
	_ = s.World.Move(e.ID, step.To)
	skip := entityIDs(s.moveFollowers(e, old, dest, step.Dir))
	s.sendRoom(old, nil, protocol.TextEvent{
		Channel: protocol.ChanNarrative,
		Text:    fmt.Sprintf("%s leaves %s.", e.CapDisplay(), step.Dir),
	})
	if tgt.Kind == world.KindPlayer {
		s.sendTo(tgt, protocol.TextEvent{
			Channel: protocol.ChanAlert,
			Text:    fmt.Sprintf("%s follows you.", e.CapDisplay()),
		})
		s.sendRoomSkip(dest, append(skip, tgt.ID), protocol.TextEvent{
			Channel: protocol.ChanAlert,
			Text:    fmt.Sprintf("%s arrives.", e.CapDisplay()),
		})
	} else {
		s.sendRoomSkip(dest, skip, protocol.TextEvent{
			Channel: protocol.ChanAlert,
			Text:    fmt.Sprintf("%s arrives.", e.CapDisplay()),
		})
	}
	s.pushRoomIn(old)
	s.pushRoomIn(dest)
}

func (s *Server) moveFollowers(leader, old, dest *world.Entity, dir string) []*world.Entity {
	if s == nil || s.World == nil || leader == nil || old == nil || dest == nil {
		return nil
	}
	var moved []*world.Entity
	for _, hop := range s.World.PullFollowers(leader, old, dest) {
		f := hop.Follower
		if f == nil {
			continue
		}
		moved = append(moved, f)
		if f.Kind == world.KindPlayer {
			if sess := s.sessionFor(f); sess != nil {
				sess.Send(protocol.TextEvent{
					Channel: protocol.ChanNarrative,
					Text:    commands.FollowYou(hop.Leader, dir),
				})
				commands.Arrive(s.ctx(sess), old, dest)
			}
			continue
		}
		if hop.Leader != nil && hop.Leader.Kind == world.KindPlayer {
			if sess := s.sessionFor(hop.Leader); sess != nil {
				sess.Send(protocol.TextEvent{
					Channel: protocol.ChanNarrative,
					Text:    commands.FollowsYou(f, dir),
				})
			}
		}
		if s.Scripts != nil {
			_, _ = s.Scripts.Call("on_leave", f, old)
			_, _ = s.Scripts.Call("on_enter", f, dest)
		}
	}
	return moved
}

// corpseID keeps the mob's dotted id out of the corpse id, so the last
// path segment is not a living keyword such as "wolf".
func corpseID(id world.ID) world.ID {
	safe := strings.NewReplacer(".", "-", "#", "-").Replace(string(id))
	return world.ID("corpse#" + safe)
}

type allyCredit struct {
	owner, leader, party world.ID
}

func (s *Server) creditPlayer(id world.ID, credit map[world.ID]allyCredit) *world.Entity {
	seen := map[world.ID]bool{}
	var walk func(world.ID) *world.Entity
	walk = func(id world.ID) *world.Entity {
		if id == "" || seen[id] {
			return nil
		}
		seen[id] = true
		if e := s.World.Get(id); e != nil && e.Kind == world.KindPlayer {
			return e
		}
		c, ok := credit[id]
		if !ok {
			return nil
		}
		if p := walk(c.owner); p != nil {
			return p
		}
		if p := walk(c.leader); p != nil {
			return p
		}
		if c.party == "" {
			return nil
		}
		if p := walk(c.party); p != nil {
			return p
		}
		return s.playerSharingParty(c.party)
	}
	return walk(id)
}

// solePlayer is the only character in a single-player world. An unattributed
// death can credit that character. A second character means nobody is assumed.
func (s *Server) solePlayer() *world.Entity {
	if s == nil || s.World == nil {
		return nil
	}
	var found *world.Entity
	for _, e := range s.World.Entities {
		if e == nil || e.Kind != world.KindPlayer {
			continue
		}
		if found != nil {
			return nil
		}
		found = e
	}
	return found
}

func (s *Server) playerSharingParty(partyID world.ID) *world.Entity {
	if s == nil || s.World == nil || partyID == "" {
		return nil
	}
	var fallback *world.Entity
	for _, e := range s.World.Entities {
		if e == nil || e.Kind != world.KindPlayer || e.PartyLeader != partyID {
			continue
		}
		if e.ID == partyID {
			return e
		}
		if fallback == nil || e.ID < fallback.ID {
			fallback = e
		}
	}
	return fallback
}

func (s *Server) killRecipients(killer, room *world.Entity, credit map[world.ID]allyCredit) []*world.Entity {
	if killer == nil {
		return nil
	}
	partyID := killer.PartyLeader
	if partyID == "" {
		partyID = credit[killer.ID].party
	}
	if partyID == "" {
		return []*world.Entity{killer}
	}
	var out []*world.Entity
	for _, e := range s.World.Entities {
		if e == nil || e.Kind != world.KindPlayer {
			continue
		}
		pLeader := e.PartyLeader
		if pLeader == "" {
			pLeader = credit[e.ID].party
		}
		if pLeader != partyID {
			continue
		}
		if room != nil && s.World.RoomOf(e) != room {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Server) reapDead() {
	primary := s.Pack.RPG.Primary()
	credit := map[world.ID]allyCredit{}
	for id, e := range s.World.Entities {
		if e == nil {
			continue
		}
		credit[id] = allyCredit{owner: e.Owner, leader: e.Leader, party: e.PartyLeader}
	}
	var dead []world.ID
	for _, e := range s.World.Entities {
		if e.Kind != world.KindMobile {
			continue
		}
		if !e.Alive(primary) {
			dead = append(dead, e.ID)
		}
	}
	for _, id := range dead {
		e := s.World.Get(id)
		if e == nil {
			continue
		}
		s.World.StopFollowing(e)
		for _, fID := range append([]world.ID{}, e.Followers...) {
			f := s.World.Get(fID)
			if f != nil {
				f.Leader = ""
			}
		}
		e.Followers = nil
		room := s.World.RoomOf(e)
		killer := s.creditPlayer(e.SlainBy, credit)
		unattributed := killer == nil && e.SlainBy == ""
		if unattributed {
			killer = s.solePlayer()
		}
		_, _ = s.Scripts.Call("on_death", killer, e)
		isPlayerKill := killer != nil && !unattributed
		recipients := s.killRecipients(killer, room, credit)
		if isPlayerKill && s.Pack != nil && len(s.Pack.Quests) > 0 {
			for _, rec := range recipients {
				notices := quest.UpdateKill(rec, e, s.Pack.Quests, s.World)
				for _, n := range notices {
					s.sendTo(rec, protocol.TextEvent{Channel: protocol.ChanAlert, Text: n.String()})
				}
			}
		}
		if (isPlayerKill || unattributed) && e.XP > 0 && s.Pack.RPG.Advancement.On() {
			for _, rec := range recipients {
				s.sendTo(rec, protocol.TextEvent{
					Channel: protocol.ChanNarrative,
					Text:    fmt.Sprintf("You gain %d experience.", e.XP),
				})
				msgs := s.Pack.RPG.Grant(rec, e.XP)
				for _, msg := range msgs {
					s.sendTo(rec, protocol.TextEvent{Channel: protocol.ChanAlert, Text: msg})
				}
				if len(msgs) > 0 {
					s.sendTo(rec, protocol.TextEvent{
						Channel: protocol.ChanSystem,
						Text:    "improve <skill> spends a skill point. raise <attribute> spends an attribute point. train <skill> pays a trainer.",
					})
				}
			}
		}
		if room != nil {
			if s.Pack.RPG.Death.NPCDrop() {
				for _, c := range append([]world.ID{}, e.Contents...) {
					_ = s.World.Move(c, room.ID)
				}
			}
			if len(e.Loot) > 0 {
				for _, loot := range e.Loot {
					if loot.SkinOnly {
						continue
					}
					chance := loot.Chance
					if chance <= 0 {
						chance = 100
					}
					if s.rng == nil || s.rng.Intn(100) < chance {
						count := loot.Count
						if count <= 0 {
							count = 1
						}
						for k := 0; k < count; k++ {
							_, _ = s.World.Spawn(world.ID(loot.Item), room.ID)
						}
					}
				}
			}
			if s.Pack.RPG.Death.NPCCorpse() {
				var skinLoot []world.LootDrop
				for _, loot := range e.Loot {
					if loot.SkinOnly {
						skinLoot = append(skinLoot, loot)
					}
				}
				corpse := &world.Entity{
					ID:       corpseID(e.ID),
					Kind:     world.KindItem,
					Name:     "corpse",
					Short:    "a corpse",
					Long:     "The remains of " + e.Display() + ".",
					Keywords: []string{"corpse"},
					Takeable: true,
					Loot:     skinLoot,
				}
				s.World.Add(corpse)
				_ = s.World.Move(corpse.ID, room.ID)
			}
		}
		if e.RespawnEvery > 0 {
			home := e.HomeRoom
			if home == "" && room != nil {
				home = room.ID
			}
			if home != "" {
				proto := e.PrototypeID
				if proto == "" {
					proto = e.ID
				}
				task := world.RespawnTask{
					ProtoID: proto,
					RoomID:  home,
					DueTick: s.World.Tick + int64(e.RespawnEvery),
				}
				if e.AI != nil && len(e.AI.Rooms) > 0 {
					task.Rooms = append([]world.ID{}, e.AI.Rooms...)
				}
				s.World.Respawns = append(s.World.Respawns, task)
			}
		}
		s.World.Destroy(id)
	}
	for _, ent := range s.World.Entities {
		if ent == nil || ent.Kind != world.KindPlayer || ent.Alive(primary) {
			continue
		}
		pDead := ent
		sess := s.sessionFor(pDead)
		if sess != nil {
			sess.Send(protocol.TextEvent{Channel: protocol.ChanAlert, Text: "You die. The world fades... then you wake at the start."})
		}
		fall := s.World.RoomOf(pDead)
		if fall != nil {
			s.sendRoom(fall, pDead, protocol.TextEvent{
				Channel: protocol.ChanNarrative,
				Text:    fmt.Sprintf("%s collapses.", pDead.CapDisplay()),
			})
		}
		pct := s.Pack.RPG.Death.PlayerPct()
		max := pDead.Res(primary).Max
		pDead.SetRes(primary, world.Resource{Current: max * pct / 100, Max: max})
		if pDead.Res(primary).Current < 1 && max > 0 {
			pDead.SetRes(primary, world.Resource{Current: 1, Max: max})
		}
		if pDead.Combat != nil {
			pDead.Combat.Target = ""
		}
		if !s.Pack.RPG.Death.PlayerKeepItems() {
			if fall != nil {
				for _, it := range append([]*world.Entity{}, s.World.Children(pDead.ID)...) {
					_ = s.World.Move(it.ID, fall.ID)
				}
			}
		}
		if fall != nil {
			s.pushRoomIn(fall)
		}
		var start *world.Entity
		if s.World.StartRoom != "" {
			start = s.World.Get(s.World.StartRoom)
			if start != nil {
				_ = s.World.Move(pDead.ID, start.ID)
			}
		}
		s.recallCompanions(pDead)
		if start != nil && (fall == nil || start.ID != fall.ID) {
			s.sendRoom(start, pDead, protocol.TextEvent{
				Channel: protocol.ChanNarrative,
				Text:    fmt.Sprintf("%s arrives.", pDead.CapDisplay()),
			})
		}
		if fall != nil {
			s.pushRoomIn(fall)
		}
		if start != nil && (fall == nil || start.ID != fall.ID) {
			s.pushRoomIn(start)
		}
		if sess != nil {
			commands.EnterRoom(s.ctx(sess), true)
			s.pushUI(sess)
		}
	}
}

func (s *Server) recallCompanions(player *world.Entity) {
	if s == nil || s.World == nil || player == nil || s.World.StartRoom == "" {
		return
	}
	start := s.World.Get(s.World.StartRoom)
	if start == nil {
		return
	}
	var ids []world.ID
	for id, e := range s.World.Entities {
		if e != nil && e.ID != player.ID && e.Owner == player.ID {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		e := s.World.Get(id)
		if e == nil {
			continue
		}
		if strings.EqualFold(e.Stance, "stay") {
			s.World.StopFollowing(e)
			s.sendTo(player, protocol.TextEvent{
				Channel: protocol.ChanNarrative,
				Text:    fmt.Sprintf("%s stays behind.", e.CapDisplay()),
			})
			continue
		}
		endFight(s.World, e)
		if s.World.RoomOf(e) != start {
			_ = s.World.Move(e.ID, start.ID)
		}
		s.sendTo(player, protocol.TextEvent{
			Channel: protocol.ChanNarrative,
			Text:    fmt.Sprintf("%s is with you.", e.CapDisplay()),
		})
	}
}

func (s *Server) pushUI(sessions ...*Session) {
	target := sessions
	if len(target) == 0 {
		target = s.allSessions()
	}
	for _, sess := range target {
		s.pushRoomFor(sess)
		s.pushVitalsFor(sess)
		s.pushInvFor(sess)
		s.pushMapFor(sess)
		s.pushCombatFor(sess)
		s.pushPromptFor(sess)
	}
}

func (s *Server) pushRoom(sessions ...*Session) {
	target := sessions
	if len(target) == 0 {
		target = s.allSessions()
	}
	for _, sess := range target {
		s.pushRoomFor(sess)
	}
}

func (s *Server) pushRoomIn(room *world.Entity) {
	if s == nil || s.World == nil || room == nil {
		return
	}
	for _, sess := range s.allSessions() {
		pl := s.World.Get(sess.PlayerID)
		if pl != nil && s.World.RoomOf(pl) == room {
			s.pushRoomFor(sess)
		}
	}
}

func (s *Server) pushRoomFor(sess *Session) {
	if sess == nil {
		return
	}
	pl := s.World.Get(sess.PlayerID)
	if pl == nil {
		return
	}
	room := s.World.RoomOf(pl)
	if room == nil {
		return
	}
	ev := protocol.RoomEvent{
		ID:          string(room.ID),
		Title:       room.Name,
		Description: room.Long,
		X:           room.X,
		Y:           room.Y,
		Z:           room.Z,
		HasCoords:   room.HasCoords,
		Dark:        !s.World.RoomIsLit(room),
	}
	for d, ex := range room.Exits {
		ev.Exits = append(ev.Exits, protocol.ExitInfo{Dir: d, Closed: ex.Closed, Locked: ex.Locked})
	}
	sort.Slice(ev.Exits, func(i, j int) bool { return ev.Exits[i].Dir < ev.Exits[j].Dir })
	if s.World.RoomIsLit(room) {
		for _, e := range s.World.Children(room.ID) {
			if e.ID == pl.ID {
				continue
			}
			switch e.Kind {
			case world.KindMobile:
				ev.Occupants = append(ev.Occupants, e.Display())
			case world.KindPlayer:
				name := e.Display()
				if e.InCombat() {
					name += " (fighting)"
				} else if e.HasFlag("linkdead") {
					name += " (linkdead)"
				}
				ev.Occupants = append(ev.Occupants, name)
			case world.KindItem, world.KindScenery:
				ev.Items = append(ev.Items, e.Display())
			}
		}
	}
	sess.Send(ev)
}

func (s *Server) pushVitals(sessions ...*Session) {
	target := sessions
	if len(target) == 0 {
		target = s.allSessions()
	}
	for _, sess := range target {
		s.pushVitalsFor(sess)
	}
}

func (s *Server) pushVitalsFor(sess *Session) {
	if sess == nil {
		return
	}
	pl := s.World.Get(sess.PlayerID)
	if pl == nil {
		return
	}
	ev := protocol.VitalsEvent{
		InCombat: pl.InCombat(),
		Round:    s.World.Tick,
	}
	for _, def := range s.Pack.RPG.Resources {
		r := pl.Res(def.Key)
		label := s.Pack.Lexicon.Label(def.Key)
		if label == def.Key && def.Label != "" {
			label = def.Label
		}
		ev.Resources = append(ev.Resources, protocol.ResourceView{
			Key: def.Key, Label: label, Current: r.Current, Max: r.Max,
			Pile: def.Pile, Show: s.Pack.RPG.Show(def.Key),
		})
	}
	for f, on := range pl.Flags {
		if on {
			ev.Flags = append(ev.Flags, f)
		}
	}
	sort.Strings(ev.Flags)
	sess.Send(ev)
}

func (s *Server) pushInv(sessions ...*Session) {
	target := sessions
	if len(target) == 0 {
		target = s.allSessions()
	}
	for _, sess := range target {
		s.pushInvFor(sess)
	}
}

func (s *Server) pushInvFor(sess *Session) {
	if sess == nil {
		return
	}
	pl := s.World.Get(sess.PlayerID)
	if pl == nil {
		return
	}
	ev := protocol.InventoryEvent{}
	for _, e := range s.World.Children(pl.ID) {
		ev.Items = append(ev.Items, e.Display())
	}
	for slot, id := range pl.Equipment {
		it := s.World.Get(id)
		name := string(id)
		if it != nil {
			name = it.Display()
		}
		ev.Equipment = append(ev.Equipment, protocol.EquipView{Slot: slot, Item: name})
	}
	sess.Send(ev)
}

func (s *Server) pushMap(sessions ...*Session) {
	target := sessions
	if len(target) == 0 {
		target = s.allSessions()
	}
	for _, sess := range target {
		s.pushMapFor(sess)
	}
}

func (s *Server) pushMapFor(sess *Session) {
	if sess == nil {
		return
	}
	pl := s.World.Get(sess.PlayerID)
	if pl == nil {
		return
	}
	room := s.World.RoomOf(pl)
	ev := protocol.MapEvent{}
	if room != nil && room.HasCoords {
		ev.PlayerX, ev.PlayerY = room.X, room.Y
		for _, e := range s.World.Entities {
			if e.Kind != world.KindRoom || !e.HasCoords || e.Z != room.Z {
				continue
			}
			if abs(e.X-room.X) > 4 || abs(e.Y-room.Y) > 4 {
				continue
			}
			mr := protocol.MapRoom{X: e.X, Y: e.Y, Name: e.Name, Here: e.ID == room.ID}
			for d := range e.Exits {
				mr.Exits = append(mr.Exits, d)
			}
			ev.Rooms = append(ev.Rooms, mr)
		}
	}
	if room != nil {
		ev.Text = render.Automap(s.World, pl, 2)
	}
	sess.Send(ev)
}

func (s *Server) pushCombat(sessions ...*Session) {
	target := sessions
	if len(target) == 0 {
		target = s.allSessions()
	}
	for _, sess := range target {
		s.pushCombatFor(sess)
	}
}

func (s *Server) pushCombatFor(sess *Session) {
	if sess == nil {
		return
	}
	pl := s.World.Get(sess.PlayerID)
	if pl == nil {
		return
	}
	ev := protocol.CombatEvent{Active: pl.InCombat()}
	if ev.Active {
		primary := s.Pack.RPG.Primary()
		add := func(e *world.Entity) {
			if e == nil {
				return
			}
			r := e.Res(primary)
			ev.Fighters = append(ev.Fighters, protocol.FighterView{
				Name: e.Display(), HP: r.Current, Max: r.Max, IsPlayer: e.Kind == world.KindPlayer,
			})
		}
		add(pl)
		add(s.World.Get(pl.Combat.Target))
	}
	sess.Send(ev)
}

func (s *Server) pushPrompt(sessions ...*Session) {
	target := sessions
	if len(target) == 0 {
		target = s.allSessions()
	}
	for _, sess := range target {
		s.pushPromptFor(sess)
	}
}

func (s *Server) pushPromptFor(sess *Session) {
	if sess == nil {
		return
	}
	pl := s.World.Get(sess.PlayerID)
	if pl == nil {
		return
	}
	room := s.World.RoomOf(pl)
	name := ""
	if room != nil {
		name = room.Name
	}
	primary := s.Pack.RPG.Primary()
	r := pl.Res(primary)
	label := s.Pack.Lexicon.Label(primary)
	mode := ""
	if sess.Mode == protocol.ModeBuild {
		mode = " [BUILD]"
	}
	status := fmt.Sprintf("%s:%d/%d", label, r.Current, r.Max)
	if s.Pack.RPG.Pile(primary) {
		status = fmt.Sprintf("%s:%d", label, r.Current)
	}
	sess.Send(protocol.PromptEvent{
		Text:   fmt.Sprintf("%s %s%s > ", name, status, mode),
		Room:   name,
		Status: status,
		Build:  sess.Mode == protocol.ModeBuild,
	})
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// HandleLine is for tests: process one command and drain events.
func (s *Server) HandleLine(line string) []protocol.Event {
	s.handle(line)
	return takeEvents(s.Session)
}

// HandleSessionLine is for tests: process one command for a given session and drain its events.
func (s *Server) HandleSessionLine(sess *Session, line string) []protocol.Event {
	s.handleSession(sess, line)
	return takeEvents(sess)
}

func takeEvents(sess *Session) []protocol.Event {
	if sess == nil {
		return nil
	}
	sess.Flush()
	var out []protocol.Event
	for {
		select {
		case ev, ok := <-sess.Out:
			if !ok {
				return out
			}
			out = append(out, ev)
		default:
			return out
		}
	}
}
