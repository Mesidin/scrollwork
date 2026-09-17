package server

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"sudengine/internal/combat"
	"sudengine/internal/commands"
	"sudengine/internal/olc"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/render"
	"sudengine/internal/save"
	"sudengine/internal/script"
	"sudengine/internal/world"
)

type capture struct {
	buf  []string
	done func(text string)
}

type Session struct {
	ID        string
	Mode      protocol.Mode
	PlayerID  world.ID
	Out       chan protocol.Event
	In        chan string
	BuildWalk bool
	quit      bool
	cap       *capture
}

func (s *Session) Send(ev protocol.Event) {
	select {
	case s.Out <- ev:
	default:
		// drop if client is stalled
	}
}

type Server struct {
	Pack    *pack.Pack
	World   *world.World
	Scripts *script.Host
	Combat  *combat.Engine
	Session *Session
	rng     *rand.Rand
}

type Options struct {
	PackDir    string
	Mode       protocol.Mode
	Slot       string // if set, load snapshot instead of fresh instantiate
	Seed       int64
	PlayerName string
	RaceID     string
	RoleID     string
}

func Launch(opt Options) (*Server, *Session, error) {
	p, err := pack.Load(opt.PackDir)
	if err != nil {
		return nil, nil, err
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
	h := script.New(p.Scripts)
	h.World = w
	srv := &Server{
		Pack:    p,
		World:   w,
		Scripts: h,
		Session: sess,
		rng:     rand.New(rand.NewSource(seed)),
	}
	srv.Combat = &combat.Engine{
		RNG:     srv.rng,
		Primary: p.RPG.Primary(),
		Every:   p.Meta.CombatEvery,
		Emit: func(to *world.Entity, channel, text string) {
			if to != nil && to.ID == sess.PlayerID {
				sess.Send(protocol.TextEvent{Channel: protocol.Channel(channel), Text: text})
			}
		},
	}
	h.Echo = func(actor *world.Entity, text string) {
		if actor != nil && actor.ID == sess.PlayerID {
			sess.Send(protocol.TextEvent{Channel: protocol.ChanNarrative, Text: text})
		}
	}
	h.EchoRoom = func(room *world.Entity, text string, skip *world.Entity) {
		pl := w.Player()
		if pl != nil && w.RoomOf(pl) == room {
			sess.Send(protocol.TextEvent{Channel: protocol.ChanNarrative, Text: text})
		}
	}
	h.Damage = func(att, def *world.Entity, amount int) {
		srv.Combat.Damage(w, att, def, amount, "strike")
	}
	return srv, sess, nil
}

func (s *Server) Run(ctx context.Context) {
	tick := time.Duration(s.Pack.Meta.TickMS) * time.Millisecond
	if tick <= 0 {
		tick = 250 * time.Millisecond
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	s.greet()
	s.pushUI()

	for {
		if s.Session.quit {
			s.Session.Send(protocol.DisconnectEvent{Reason: "quit"})
			return
		}
		select {
		case <-ctx.Done():
			s.Session.Send(protocol.DisconnectEvent{Reason: "shutdown"})
			return
		case <-ticker.C:
			s.tick()
		case line, ok := <-s.Session.In:
			if !ok {
				return
			}
			s.handle(line)
		}
	}
}

func (s *Server) greet() {
	title := s.Pack.Meta.Title
	s.Session.Send(protocol.TitleEvent{Title: title})
	s.Session.Send(protocol.ModeEvent{Mode: s.Session.Mode})
	s.Session.Send(protocol.LayoutEvent{Panes: s.Pack.Meta.UI.Panes})
	banner := title
	if s.Session.Mode == protocol.ModeBuild {
		banner += "  [BUILD]"
	}
	s.Session.Send(protocol.TextEvent{Channel: protocol.ChanSystem, Text: banner})
	if s.Session.Mode == protocol.ModePlay {
		commands.DescribeRoom(s.ctx(), true)
	} else {
		s.Session.Send(protocol.TextEvent{Channel: protocol.ChanBuild, Text: "Builder mode. Try: dig, desc, extra, ai, iset, spawn, proto, save pack. help building"})
		commands.DescribeRoom(s.ctx(), true)
	}
}

func (s *Server) ctx() *commands.Context {
	sess := s.Session
	c := &commands.Context{
		World:     s.World,
		Actor:     s.World.Get(sess.PlayerID),
		Pack:      s.Pack,
		Mode:      sess.Mode,
		Scripts:   s.Scripts,
		Tick:      s.World.Tick,
		RNG:       s.rng,
		BuildWalk: &sess.BuildWalk,
		Tell: func(ch protocol.Channel, text string) {
			sess.Send(protocol.TextEvent{Channel: ch, Text: text})
		},
		Quit: func() { sess.quit = true },
		SaveSnap: func(slot string) error {
			return save.Write(s.Pack.Meta.ID, slot, s.World)
		},
		LoadSnap: func(slot string) error {
			w, err := save.Read(s.Pack.Meta.ID, slot)
			if err != nil {
				return err
			}
			s.World = w
			s.Scripts.World = w
			sess.PlayerID = w.PlayerID
			return nil
		},
		SavePack: func() error {
			s.Pack.SyncFromWorld(s.World)
			return s.Pack.WriteWorld()
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
	if s.Session.cap != nil {
		if strings.TrimSpace(line) == "." {
			text := strings.Join(s.Session.cap.buf, "\n")
			done := s.Session.cap.done
			s.Session.cap = nil
			if done != nil {
				done(text)
			}
			s.pushUI()
			return
		}
		s.Session.cap.buf = append(s.Session.cap.buf, line)
		return
	}
	line = strings.TrimSpace(line)
	c := s.ctx()
	if line == "" {
		s.pushUI()
		return
	}
	p := commands.Parse(line, s.Pack.Lexicon.CanonicalVerb)
	if commands.Dispatch(c, p) {
		s.afterCommand()
		s.pushUI()
		return
	}
	if s.Session.Mode == protocol.ModeBuild {
		if h := olc.Lookup(p.Verb); h != nil {
			h(c, p)
			s.afterCommand()
			s.pushUI()
			return
		}
	}
	c.Print("I don't understand %q.", p.Verb)
	s.pushUI()
}

func (s *Server) afterCommand() {
	s.reapDead()
}

func (s *Server) tick() {
	s.World.Tick++
	now := s.World.Tick
	s.regen()
	s.ai()
	s.Combat.Tick(s.World, now)
	s.reapDead()
	// only push vitals/combat on tick to avoid flooding the log
	s.pushVitals()
	s.pushCombat()
	s.pushPrompt()
}

func (s *Server) regen() {
	for _, e := range s.World.Entities {
		if e.Kind != world.KindPlayer && e.Kind != world.KindMobile {
			continue
		}
		for _, def := range s.Pack.RPG.Resources {
			if def.Regen == 0 || def.RegenEvery <= 0 {
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
}

func (s *Server) ai() {
	player := s.World.Player()
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
		prof := e.AI.Profile
		if player != nil && s.World.RoomOf(player) == room {
			if prof == "aggressive" && !e.InCombat() && player.Alive(primary) {
				s.Combat.Start(e, player, s.World.Tick)
				if player.ID == s.Session.PlayerID {
					s.Session.Send(protocol.TextEvent{
						Channel: protocol.ChanAlert,
						Text:    fmt.Sprintf("%s attacks you!", e.CapDisplay()),
					})
				}
			}
			if prof == "coward" {
				r := e.Res(primary)
				if r.Max > 0 && r.Current*100/r.Max < 30 && len(room.Exits) > 0 {
					for d, ex := range room.Exits {
						if !ex.Closed {
							_ = s.World.Move(e.ID, ex.To)
							if player != nil && s.World.RoomOf(player) == room {
								s.Session.Send(protocol.TextEvent{
									Channel: protocol.ChanNarrative,
									Text:    fmt.Sprintf("%s flees %s.", e.CapDisplay(), d),
								})
							}
							break
						}
					}
				}
			}
		}
		if e.InCombat() {
			continue
		}
		if e.AI.Wander || prof == "wander" || prof == "aggressive" {
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
				if !ex.Closed {
					dirs = append(dirs, ex)
				}
			}
			if len(dirs) == 0 {
				continue
			}
			ex := dirs[s.rng.Intn(len(dirs))]
			old := room
			_ = s.World.Move(e.ID, ex.To)
			if player != nil {
				if s.World.RoomOf(player) == old {
					s.Session.Send(protocol.TextEvent{
						Channel: protocol.ChanNarrative,
						Text:    fmt.Sprintf("%s leaves %s.", e.CapDisplay(), ex.Dir),
					})
				}
				if s.World.RoomOf(player) == s.World.Get(ex.To) {
					if prof == "aggressive" && player.Alive(primary) && !e.InCombat() {
						s.Combat.Start(e, player, s.World.Tick)
						s.Session.Send(protocol.TextEvent{
							Channel: protocol.ChanAlert,
							Text:    fmt.Sprintf("%s arrives — and attacks you!", e.CapDisplay()),
						})
					} else {
						s.Session.Send(protocol.TextEvent{
							Channel: protocol.ChanNarrative,
							Text:    fmt.Sprintf("%s arrives.", e.CapDisplay()),
						})
					}
					s.pushRoom()
				}
			}
		}
	}
}

func (s *Server) reapDead() {
	primary := s.Pack.RPG.Primary()
	player := s.World.Player()
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
		room := s.World.RoomOf(e)
		_, _ = s.Scripts.Call("on_death", player, e)
		if room != nil {
			if s.Pack.RPG.Death.NPCDrop() {
				for _, c := range append([]world.ID{}, e.Contents...) {
					_ = s.World.Move(c, room.ID)
				}
			}
			if s.Pack.RPG.Death.NPCCorpse() {
				corpse := &world.Entity{
					ID:       world.ID(fmt.Sprintf("corpse-%s", e.ID)),
					Kind:     world.KindItem,
					Name:     "corpse of " + e.Name,
					Short:    "the corpse of " + e.Display(),
					Long:     "It's dead.",
					Keywords: []string{"corpse"},
					Takeable: true,
				}
				s.World.Add(corpse)
				_ = s.World.Move(corpse.ID, room.ID)
			}
		}
		s.World.Destroy(id)
	}
	if player != nil && !player.Alive(primary) {
		s.Session.Send(protocol.TextEvent{Channel: protocol.ChanAlert, Text: "You die. The world fades... then you wake at the start."})
		pct := s.Pack.RPG.Death.PlayerPct()
		max := player.Res(primary).Max
		player.SetRes(primary, world.Resource{Current: max * pct / 100, Max: max})
		if player.Res(primary).Current < 1 && max > 0 {
			player.SetRes(primary, world.Resource{Current: 1, Max: max})
		}
		if player.Combat != nil {
			player.Combat.Target = ""
		}
		if !s.Pack.RPG.Death.PlayerKeepItems() {
			room := s.World.RoomOf(player)
			if room != nil {
				for _, it := range append([]*world.Entity{}, s.World.Children(player.ID)...) {
					_ = s.World.Move(it.ID, room.ID)
				}
			}
		}
		if s.World.StartRoom != "" {
			_ = s.World.Move(player.ID, s.World.StartRoom)
		}
		commands.DescribeRoom(s.ctx(), true)
	}
}

func (s *Server) pushUI() {
	s.pushRoom()
	s.pushVitals()
	s.pushInv()
	s.pushMap()
	s.pushCombat()
	s.pushPrompt()
}

func (s *Server) pushRoom() {
	pl := s.World.Player()
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
			case world.KindMobile, world.KindPlayer:
				ev.Occupants = append(ev.Occupants, e.Display())
			case world.KindItem, world.KindScenery:
				ev.Items = append(ev.Items, e.Display())
			}
		}
	}
	s.Session.Send(ev)
}

func (s *Server) pushVitals() {
	pl := s.World.Player()
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
		})
	}
	for f, on := range pl.Flags {
		if on {
			ev.Flags = append(ev.Flags, f)
		}
	}
	sort.Strings(ev.Flags)
	s.Session.Send(ev)
}

func (s *Server) pushInv() {
	pl := s.World.Player()
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
	s.Session.Send(ev)
}

func (s *Server) pushMap() {
	pl := s.World.Player()
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
	s.Session.Send(ev)
}

func (s *Server) pushCombat() {
	pl := s.World.Player()
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
	s.Session.Send(ev)
}

func (s *Server) pushPrompt() {
	pl := s.World.Player()
	room := s.World.RoomOf(pl)
	name := ""
	if room != nil {
		name = room.Name
	}
	primary := s.Pack.RPG.Primary()
	r := pl.Res(primary)
	label := s.Pack.Lexicon.Label(primary)
	mode := ""
	if s.Session.Mode == protocol.ModeBuild {
		mode = " [BUILD]"
	}
	status := fmt.Sprintf("%s:%d/%d", label, r.Current, r.Max)
	s.Session.Send(protocol.PromptEvent{
		Text:   fmt.Sprintf("%s %s%s > ", name, status, mode),
		Room:   name,
		Status: status,
		Build:  s.Session.Mode == protocol.ModeBuild,
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
	var out []protocol.Event
	for {
		select {
		case ev := <-s.Session.Out:
			out = append(out, ev)
		default:
			return out
		}
	}
}
