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
	Env     *weather.Environment
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
		Env:     weather.New(p.Meta.TickMS, p.Time, p.Weather),
		rng:     rand.New(rand.NewSource(seed)),
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
		Emit: func(to *world.Entity, channel, text string) {
			if to != nil && to.ID == sess.PlayerID {
				sess.Send(protocol.TextEvent{Channel: protocol.Channel(channel), Text: text})
			}
		},
	}
	srv.Combat.HitMod = srv.outdoorHitPenalty
	if srv.Env.Extinguishes(true) {
		srv.douseOutdoorFlames()
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
	h.Move = srv.moveEntity
	return srv, sess, nil
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
	player := s.World.Player()
	changed := old == nil || old.ID != dest.ID
	if player != nil && changed {
		movedPlayer := ent.ID == player.ID
		movedTarget := player.Combat != nil && player.Combat.Target == ent.ID
		if movedPlayer || movedTarget {
			endFight(s.World, player)
		}
		if movedPlayer && s.Session != nil {
			commands.Arrive(s.ctx(), old, dest)
		}
	}
	if changed && old != nil {
		s.moveFollowers(ent, old, dest, "")
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
		commands.EnterRoom(s.ctx(), true)
	} else {
		s.Session.Send(protocol.TextEvent{Channel: protocol.ChanBuild, Text: "Builder mode. The side pane shows the map and dig, desc, save pack. help building"})
		commands.EnterRoom(s.ctx(), true)
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
		Env:       s.Env,
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
			player := s.World.Player()
			if mob == nil || player == nil || mob.InCombat() || !player.Alive(s.Pack.RPG.Primary()) {
				return
			}
			s.Combat.Start(mob, player, s.World.Tick)
			s.Combat.CheckAssists(s.World, player, mob, s.World.Tick, false)
			sess.Send(protocol.TextEvent{
				Channel: protocol.ChanAlert,
				Text:    fmt.Sprintf("%s attacks you!", mob.CapDisplay()),
			})
		},
		Assist: func(target *world.Entity, leaderAttacking bool) {
			if target == nil || s.World == nil {
				return
			}
			player := s.World.Player()
			if player == nil {
				return
			}
			s.Combat.CheckAssists(s.World, player, target, s.World.Tick, leaderAttacking)
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
	s.environment()
	s.regen()
	s.ai()
	if s.Combat != nil {
		s.Combat.Tick(s.World, now)
	}
	s.reapDead()
	s.processRespawns()
	// only push vitals/combat on tick to avoid flooding the log
	s.pushVitals()
	s.pushCombat()
	s.pushPrompt()
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
				player := s.World.Player()
				if player != nil && s.World.RoomOf(player) == s.World.Get(task.RoomID) {
					s.Session.Send(protocol.TextEvent{
						Channel: protocol.ChanNarrative,
						Text:    fmt.Sprintf("%s appears.", mob.CapDisplay()),
					})
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
	pl := s.World.Player()
	here := s.World.RoomOf(pl)
	lostHere := false
	for _, e := range s.World.Entities {
		if e.Kind != world.KindRoom || e.Underground() || !e.Outdoor() {
			continue
		}
		gone := s.World.ExtinguishToggleLights(e)
		if here != nil && e.ID == here.ID && len(gone) > 0 {
			lostHere = true
		}
	}
	if lostHere && s.Session != nil {
		s.Session.Send(protocol.TextEvent{
			Channel: protocol.ChanAlert,
			Text:    weather.FlameOutLine,
		})
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
	pl := s.World.Player()
	if pl == nil {
		return
	}
	room := s.World.RoomOf(pl)
	if room == nil || room.Underground() {
		return
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
			s.Session.Send(protocol.TextEvent{
				Channel: protocol.ChanAlert,
				Text:    text,
			})
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
	player := s.World.Player()
	primary := s.Pack.RPG.Primary()
	for _, e := range s.World.Entities {
		if e.Kind != world.KindMobile || e.AI == nil {
			continue
		}
		if !e.Alive(primary) {
			continue
		}
		if strings.EqualFold(e.Stance, "stay") || (player != nil && (e.Owner == player.ID || e.Leader == player.ID)) {
			continue
		}
		room := s.World.RoomOf(e)
		if room == nil {
			continue
		}
		prof := e.AI.Profile
		if player != nil && s.World.RoomOf(player) == room {
			if e.AI.Awake(e) && !e.InCombat() && player.Alive(primary) {
				s.Combat.Start(e, player, s.World.Tick)
				s.Combat.CheckAssists(s.World, player, e, s.World.Tick, false)
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
							destRoom := s.World.Get(ex.To)
							watching := player != nil && s.World.RoomOf(player) == room
							_ = s.World.Move(e.ID, ex.To)
							if watching {
								s.Session.Send(protocol.TextEvent{
									Channel: protocol.ChanNarrative,
									Text:    fmt.Sprintf("%s flees %s.", e.CapDisplay(), d),
								})
							}
							s.moveFollowers(e, room, destRoom, d)
							break
						}
					}
				}
			}
		}
		if e.InCombat() {
			s.pursue(e, room, player)
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
			wasOld := player != nil && s.World.RoomOf(player) == old
			wasDest := player != nil && destRoom != nil && s.World.RoomOf(player) == destRoom
			_ = s.World.Move(e.ID, ex.To)
			came := s.moveFollowers(e, old, destRoom, ex.Dir)
			if player != nil && !came {
				if wasOld {
					s.Session.Send(protocol.TextEvent{
						Channel: protocol.ChanNarrative,
						Text:    fmt.Sprintf("%s leaves %s.", e.CapDisplay(), ex.Dir),
					})
				}
				if wasDest {
					if e.AI.Awake(e) && player.Alive(primary) && !e.InCombat() {
						s.Combat.Start(e, player, s.World.Tick)
						s.Combat.CheckAssists(s.World, player, e, s.World.Tick, false)
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

func (s *Server) pursue(e, room, player *world.Entity) {
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
	wasOld := player != nil && s.World.RoomOf(player) == old
	wasDest := player != nil && dest != nil && s.World.RoomOf(player) == dest
	_ = s.World.Move(e.ID, step.To)
	came := s.moveFollowers(e, old, dest, step.Dir)
	if player == nil || came {
		return
	}
	if wasOld {
		s.Session.Send(protocol.TextEvent{
			Channel: protocol.ChanNarrative,
			Text:    fmt.Sprintf("%s leaves %s.", e.CapDisplay(), step.Dir),
		})
	}
	if wasDest {
		s.Session.Send(protocol.TextEvent{
			Channel: protocol.ChanAlert,
			Text:    fmt.Sprintf("%s follows you.", e.CapDisplay()),
		})
		s.pushRoom()
	}
}

func (s *Server) moveFollowers(leader, old, dest *world.Entity, dir string) bool {
	if s == nil || s.World == nil || leader == nil || old == nil || dest == nil {
		return false
	}
	player := s.World.Player()
	came := false
	for _, hop := range s.World.PullFollowers(leader, old, dest) {
		f := hop.Follower
		if f == nil {
			continue
		}
		if player != nil && f.ID == player.ID {
			came = true
			s.Session.Send(protocol.TextEvent{
				Channel: protocol.ChanNarrative,
				Text:    commands.FollowYou(hop.Leader, dir),
			})
			commands.Arrive(s.ctx(), old, dest)
			s.pushRoom()
			continue
		}
		if player != nil && hop.Leader != nil && hop.Leader.ID == player.ID {
			s.Session.Send(protocol.TextEvent{
				Channel: protocol.ChanNarrative,
				Text:    commands.FollowsYou(f, dir),
			})
		}
		if s.Scripts != nil {
			_, _ = s.Scripts.Call("on_leave", f, old)
			_, _ = s.Scripts.Call("on_enter", f, dest)
		}
	}
	return came
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

func (s *Server) reapDead() {
	primary := s.Pack.RPG.Primary()
	player := s.World.Player()
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
		_, _ = s.Scripts.Call("on_death", player, e)
		isPlayerKill := false
		if player != nil && e.SlainBy == player.ID {
			isPlayerKill = true
		} else if player != nil && e.SlainBy != "" {
			if k, ok := credit[e.SlainBy]; ok {
				isPlayerKill = k.owner == player.ID || k.leader == player.ID ||
					(player.PartyLeader != "" && k.party == player.PartyLeader)
			}
		}
		if isPlayerKill && s.Pack != nil && len(s.Pack.Quests) > 0 {
			notices := quest.UpdateKill(player, e, s.Pack.Quests, s.World)
			for _, n := range notices {
				s.Session.Send(protocol.TextEvent{Channel: protocol.ChanAlert, Text: n.String()})
			}
		}
		if (isPlayerKill || e.SlainBy == "") && player != nil && e.XP > 0 && s.Pack.RPG.Advancement.On() {
			s.Session.Send(protocol.TextEvent{
				Channel: protocol.ChanNarrative,
				Text:    fmt.Sprintf("You gain %d experience.", e.XP),
			})
			msgs := s.Pack.RPG.Grant(player, e.XP)
			for _, msg := range msgs {
				s.Session.Send(protocol.TextEvent{Channel: protocol.ChanAlert, Text: msg})
			}
			if len(msgs) > 0 {
				s.Session.Send(protocol.TextEvent{
					Channel: protocol.ChanSystem,
					Text:    "improve <skill> spends a skill point. raise <attribute> spends an attribute point. train <skill> pays a trainer.",
				})
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
		s.recallCompanions(player)
		commands.EnterRoom(s.ctx(), true)
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
			s.Session.Send(protocol.TextEvent{
				Channel: protocol.ChanNarrative,
				Text:    fmt.Sprintf("%s stays behind.", e.CapDisplay()),
			})
			continue
		}
		endFight(s.World, e)
		if s.World.RoomOf(e) != start {
			_ = s.World.Move(e.ID, start.ID)
		}
		s.Session.Send(protocol.TextEvent{
			Channel: protocol.ChanNarrative,
			Text:    fmt.Sprintf("%s is with you.", e.CapDisplay()),
		})
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
			Pile: def.Pile, Show: s.Pack.RPG.Show(def.Key),
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
	if s.Pack.RPG.Pile(primary) {
		status = fmt.Sprintf("%s:%d", label, r.Current)
	}
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
