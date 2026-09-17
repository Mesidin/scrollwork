package commands

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"sudengine/internal/dice"
	"sudengine/internal/docs"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/rpg"
	"sudengine/internal/script"
	"sudengine/internal/world"
)

type Context struct {
	World   *world.World
	Actor   *world.Entity
	Pack    *pack.Pack
	Mode    protocol.Mode
	Scripts *script.Host
	Tick    int64
	RNG     *rand.Rand

	Tell          func(channel protocol.Channel, text string)
	Quit          func()
	SaveSnap      func(slot string) error
	LoadSnap      func(slot string) error
	SavePack      func() error
	ReloadScripts func() error
	BuildWalk     *bool
	OnMoveFail    func(dir string) bool // buildwalk hook; return true if handled
	After         func()                // push UI
	BeginCapture  func(done func(text string))
}

func (c *Context) Print(format string, args ...any) {
	c.Tell(protocol.ChanNarrative, fmt.Sprintf(format, args...))
}

func (c *Context) System(format string, args ...any) {
	c.Tell(protocol.ChanSystem, fmt.Sprintf(format, args...))
}

func (c *Context) Combat(format string, args ...any) {
	c.Tell(protocol.ChanCombat, fmt.Sprintf(format, args...))
}

func (c *Context) Say(format string, args ...any) {
	c.Tell(protocol.ChanSay, fmt.Sprintf(format, args...))
}

func (c *Context) RoomTitle(name string) {
	c.Tell(protocol.ChanRoom, name)
}

func (c *Context) Alert(format string, args ...any) {
	c.Tell(protocol.ChanAlert, fmt.Sprintf(format, args...))
}

type Handler func(*Context, Parsed)

var handlers = map[string]Handler{}

func init() {
	register("look", look)
	register("examine", examine)
	register("inventory", inventory)
	register("equipment", equipment)
	register("exits", exitsCmd)
	register("score", score)
	register("help", help)
	register("get", get)
	register("drop", drop)
	register("put", put)
	register("give", give)
	register("wear", wear)
	register("remove", remove)
	register("use", use)
	register("open", openCmd)
	register("close", closeCmd)
	register("lock", lockCmd)
	register("unlock", unlockCmd)
	register("kill", kill)
	register("flee", flee)
	register("say", say)
	register("emote", emote)
	register("ask", ask)
	register("talk", talk)
	register("list", listShop)
	register("buy", buy)
	register("sell", sell)
	register("go", goCmd)
	register("save", saveCmd)
	register("quit", quitCmd)
	register("who", who)
	for _, d := range []string{"north", "south", "east", "west", "up", "down", "northeast", "northwest", "southeast", "southwest", "in", "out"} {
		dir := d
		register(dir, func(c *Context, _ Parsed) { move(c, dir) })
	}
}

func register(name string, h Handler) {
	handlers[name] = h
}

func Lookup(verb string) Handler {
	return handlers[verb]
}

func Dispatch(c *Context, p Parsed) bool {
	if p.Verb == "" {
		return true
	}
	if h := Lookup(p.Verb); h != nil {
		h(c, p)
		return true
	}
	if a := c.Pack.RPG.AbilityByVerb(p.Verb); a != nil {
		useAbility(c, a, p)
		return true
	}
	return false
}

func look(c *Context, p Parsed) {
	if p.Rest != "" {
		examine(c, p)
		return
	}
	DescribeRoom(c, true)
}

func DescribeRoom(c *Context, verbose bool) {
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		c.Print("You are nowhere.")
		return
	}
	c.RoomTitle(room.Name)
	lit := c.World.RoomIsLit(room)
	if !lit {
		c.Print("It's pitch black. You can't see a thing.")
		return
	}
	if verbose && room.Long != "" {
		c.Print("%s", strings.TrimSpace(room.Long))
	}
	_, _ = c.Scripts.Call("on_look", c.Actor, room)
	c.Print("%s", "")
	hadContents := false
	for _, e := range c.World.Children(room.ID) {
		if e.ID == c.Actor.ID {
			continue
		}
		switch e.Kind {
		case world.KindItem, world.KindScenery:
			c.Print("%s is here.", e.CapDisplay())
			hadContents = true
		case world.KindMobile, world.KindPlayer:
			c.Print("%s is here.", e.CapDisplay())
			hadContents = true
		}
	}
	if hadContents {
		c.Print("%s", "")
	}
	c.Print("Obvious exits: %s.", exitList(room))
}

func exitList(room *world.Entity) string {
	if room == nil || len(room.Exits) == 0 {
		return "none"
	}
	dirs := make([]string, 0, len(room.Exits))
	for d, ex := range room.Exits {
		s := d
		if ex.Closed {
			s += " (closed)"
		}
		if ex.Locked {
			s += " (locked)"
		}
		dirs = append(dirs, s)
	}
	sort.Strings(dirs)
	return strings.Join(dirs, ", ")
}

func examine(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Examine what?")
		return
	}
	e, err := resolve(c, p.Rest, world.ScopeInventory, world.ScopeEquipment, world.ScopeRoom, world.ScopeSelf)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if e.Long != "" {
		c.Print("%s", strings.TrimSpace(e.Long))
	} else {
		c.Print("You see nothing special about %s.", e.Display())
	}
	if e.Container {
		if e.Closed {
			c.Print("It is closed.")
		} else {
			kids := c.World.Children(e.ID)
			if len(kids) == 0 {
				c.Print("It is empty.")
			} else {
				var names []string
				for _, k := range kids {
					names = append(names, k.Display())
				}
				c.Print("It contains: %s.", strings.Join(names, ", "))
			}
		}
	}
	_, _ = c.Scripts.Call("on_look", c.Actor, e)
}

func resolve(c *Context, token string, scopes ...world.Scope) (*world.Entity, error) {
	one, many := c.World.Match(c.Actor, firstWord(token), scopes...)
	if one != nil {
		return one, nil
	}
	if len(many) > 1 {
		var names []string
		for _, m := range many {
			names = append(names, m.Display())
		}
		return nil, fmt.Errorf("which one? %s", strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("you don't see a %q here", token)
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

func inventory(c *Context, _ Parsed) {
	kids := c.World.Children(c.Actor.ID)
	if len(kids) == 0 {
		c.Print("You are carrying nothing.")
		return
	}
	c.Print("You are carrying:")
	for _, k := range kids {
		c.Print("  %s", k.Display())
	}
}

func equipment(c *Context, _ Parsed) {
	if len(c.Actor.Equipment) == 0 {
		c.Print("You aren't wearing anything special.")
		return
	}
	slots := append([]string{}, c.Pack.RPG.Slots...)
	if len(slots) == 0 {
		for s := range c.Actor.Equipment {
			slots = append(slots, s)
		}
		sort.Strings(slots)
	}
	c.Print("You are wearing:")
	for _, s := range slots {
		id, ok := c.Actor.Equipment[s]
		if !ok {
			continue
		}
		it := c.World.Get(id)
		if it == nil {
			continue
		}
		c.Print("  %s: %s", s, it.Display())
	}
}

func exitsCmd(c *Context, _ Parsed) {
	room := c.World.RoomOf(c.Actor)
	c.Print("Obvious exits: %s.", exitList(room))
}

func score(c *Context, _ Parsed) {
	c.Print("%s", c.Actor.Name)
	if c.Actor.OriginName != "" || c.Actor.RoleName != "" {
		var bits []string
		if c.Actor.OriginName != "" {
			bits = append(bits, c.Actor.OriginName)
		}
		if c.Actor.RoleName != "" {
			bits = append(bits, c.Actor.RoleName)
		}
		c.Print("  %s", strings.Join(bits, " · "))
	}
	keys := make([]string, 0, len(c.Actor.Resources))
	for k := range c.Actor.Resources {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r := c.Actor.Resources[k]
		c.Print("  %-12s %d / %d", c.Pack.Lexicon.Label(k), r.Current, r.Max)
	}
	if len(c.Actor.Attrs) > 0 {
		ak := make([]string, 0, len(c.Actor.Attrs))
		for k := range c.Actor.Attrs {
			ak = append(ak, k)
		}
		sort.Strings(ak)
		c.Print("%s", "")
		c.Print("Traits")
		for _, k := range ak {
			c.Print("  %-12s %d", c.Pack.Lexicon.Label(k), c.Actor.Attrs[k])
		}
	}
	if len(c.Actor.Skills) > 0 {
		sk := make([]string, 0, len(c.Actor.Skills))
		for k := range c.Actor.Skills {
			sk = append(sk, k)
		}
		sort.Strings(sk)
		c.Print("%s", "")
		c.Print("Skills")
		for _, k := range sk {
			c.Print("  %-12s %d", c.Pack.Lexicon.Label(k), c.Actor.Skills[k])
		}
	}
	if c.Pack != nil && len(c.Pack.RPG.Abilities) > 0 {
		c.Print("%s", "")
		c.Print("Actions")
		for _, a := range c.Pack.RPG.Abilities {
			verb := a.Verb
			if verb == "" {
				verb = a.ID
			}
			extra := ""
			if len(a.Cost) > 0 {
				var bits []string
				for res, n := range a.Cost {
					bits = append(bits, fmt.Sprintf("%s %d", c.Pack.Lexicon.Label(res), n))
				}
				sort.Strings(bits)
				extra = "  (" + strings.Join(bits, ", ") + ")"
			}
			name := a.Name
			if name == "" {
				name = verb
			}
			c.Print("  %-12s %s%s", verb, name, extra)
		}
	}
	c.Print("%s", "")
	c.Print("Type `help systems` for what these numbers mean.")
}

func help(c *Context, p Parsed) {
	topic := strings.ToLower(strings.TrimSpace(p.Rest))
	packHelp := map[string]string{}
	if c.Pack != nil {
		packHelp = c.Pack.Help
	}
	if topic == "" || topic == "topics" {
		c.Print("Type help <topic>. Start with: playing, systems, items, building.")
		var game, engine []string
		for _, t := range docs.Merge(packHelp) {
			line := fmt.Sprintf("  %-12s  %s", t.Key, t.Title)
			if t.Source == docs.SourceGame {
				game = append(game, line)
			} else {
				engine = append(engine, line)
			}
		}
		if len(game) > 0 {
			c.Print("%s", "")
			c.Print("Game")
			for _, l := range game {
				c.Print("%s", l)
			}
		}
		if len(engine) > 0 {
			c.Print("%s", "")
			c.Print("Engine")
			for _, l := range engine {
				c.Print("%s", l)
			}
		}
		if c.Mode == protocol.ModeBuild {
			c.Print("%s", "")
			c.Print("You are in build mode. Read `help building`, then try: dig, desc, proto, spawn, save pack.")
		}
		return
	}
	t, ok := docs.Lookup(packHelp, topic)
	if !ok {
		c.Print("No help on %q. Type `help` for topics.", topic)
		return
	}
	c.Print("%s", docs.Format(t.Body))
}

func get(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Get what?")
		return
	}
	// get X from Y
	from := ""
	what := p.Rest
	if i := strings.Index(strings.ToLower(p.Rest), " from "); i >= 0 {
		what = strings.TrimSpace(p.Rest[:i])
		from = strings.TrimSpace(p.Rest[i+6:])
	}
	var e *world.Entity
	if from != "" {
		cont, err2 := resolve(c, from, world.ScopeRoom, world.ScopeInventory)
		if err2 != nil {
			c.Print("%s", err2.Error())
			return
		}
		if !cont.Container {
			c.Print("%s isn't a container.", cont.CapDisplay())
			return
		}
		if cont.Closed {
			c.Print("%s is closed.", cont.CapDisplay())
			return
		}
		e = nil
		for _, ch := range c.World.Children(cont.ID) {
			if ch.HasKeyword(firstWord(what)) {
				if e != nil {
					c.Print("which one?")
					return
				}
				e = ch
			}
		}
		if e == nil {
			c.Print("you don't see a %q in %s", what, cont.Display())
			return
		}
	} else {
		var err error
		e, err = resolve(c, what, world.ScopeRoom)
		if err != nil {
			c.Print("%s", err.Error())
			return
		}
	}
	if !e.Takeable || e.Kind != world.KindItem {
		c.Print("You can't take %s.", e.Display())
		return
	}
	if err := c.World.Move(e.ID, c.Actor.ID); err != nil {
		c.Print("%s", err.Error())
		return
	}
	c.Print("You take %s.", e.Display())
}

func drop(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Drop what?")
		return
	}
	e, err := resolve(c, p.Rest, world.ScopeInventory)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	room := c.World.RoomOf(c.Actor)
	unequipIfWorn(c, e)
	if err := c.World.Move(e.ID, room.ID); err != nil {
		c.Print("%s", err.Error())
		return
	}
	c.Print("You drop %s.", e.Display())
}

func put(c *Context, p Parsed) {
	low := strings.ToLower(p.Rest)
	i := strings.Index(low, " in ")
	if i < 0 {
		c.Print("Put what in what?")
		return
	}
	what := strings.TrimSpace(p.Rest[:i])
	into := strings.TrimSpace(p.Rest[i+4:])
	item, err := resolve(c, what, world.ScopeInventory)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	cont, err := resolve(c, into, world.ScopeInventory, world.ScopeRoom)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if !cont.Container {
		c.Print("%s isn't a container.", cont.CapDisplay())
		return
	}
	if cont.Closed {
		c.Print("It's closed.")
		return
	}
	unequipIfWorn(c, item)
	if err := c.World.Move(item.ID, cont.ID); err != nil {
		c.Print("%s", err.Error())
		return
	}
	c.Print("You put %s in %s.", item.Display(), cont.Display())
}

func give(c *Context, p Parsed) {
	low := strings.ToLower(p.Rest)
	i := strings.Index(low, " to ")
	var what, whom string
	if i >= 0 {
		what = strings.TrimSpace(p.Rest[:i])
		whom = strings.TrimSpace(p.Rest[i+4:])
	} else if len(p.Args) >= 2 {
		what = p.Args[0]
		whom = strings.Join(p.Args[1:], " ")
	} else {
		c.Print("Give what to whom?")
		return
	}
	item, err := resolve(c, what, world.ScopeInventory)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	npc, err := resolve(c, whom, world.ScopeRoom)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if npc.Kind != world.KindMobile && npc.Kind != world.KindPlayer {
		c.Print("You can't give things to %s.", npc.Display())
		return
	}
	unequipIfWorn(c, item)
	if err := c.World.Move(item.ID, npc.ID); err != nil {
		c.Print("%s", err.Error())
		return
	}
	c.Print("You give %s to %s.", item.Display(), npc.Display())
}

func wear(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Wear what?")
		return
	}
	e, err := resolve(c, p.Rest, world.ScopeInventory)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if !e.Wearable || e.Slot == "" {
		c.Print("You can't wear %s.", e.Display())
		return
	}
	if c.Actor.Equipment == nil {
		c.Actor.Equipment = map[string]world.ID{}
	}
	if old, ok := c.Actor.Equipment[e.Slot]; ok {
		if it := c.World.Get(old); it != nil {
			c.Print("You remove %s.", it.Display())
		}
		delete(c.Actor.Equipment, e.Slot)
	}
	c.Actor.Equipment[e.Slot] = e.ID
	c.Print("You wear %s.", e.Display())
}

func remove(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Remove what?")
		return
	}
	e, err := resolve(c, p.Rest, world.ScopeInventory, world.ScopeEquipment)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if !unequipIfWorn(c, e) {
		c.Print("You aren't wearing %s.", e.Display())
		return
	}
	c.Print("You remove %s.", e.Display())
}

func unequipIfWorn(c *Context, e *world.Entity) bool {
	if c.Actor.Equipment == nil {
		return false
	}
	for slot, id := range c.Actor.Equipment {
		if id == e.ID {
			delete(c.Actor.Equipment, slot)
			return true
		}
	}
	return false
}

func use(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Use what?")
		return
	}
	e, err := resolve(c, p.Rest, world.ScopeInventory, world.ScopeEquipment, world.ScopeRoom)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	handled, err := c.Scripts.Call("on_use", c.Actor, e)
	if err != nil {
		c.Print("The world hiccups: %s", err.Error())
		return
	}
	if handled {
		return
	}
	if e.Use != nil && e.Use.ToggleFlag != "" {
		on := e.HasFlag(e.Use.ToggleFlag)
		e.SetFlag(e.Use.ToggleFlag, !on)
		if on {
			msg := e.Use.MessageOff
			if msg == "" {
				msg = fmt.Sprintf("You stop using %s.", e.Display())
			}
			c.Print("%s", msg)
		} else {
			msg := e.Use.Message
			if msg == "" {
				msg = fmt.Sprintf("You use %s.", e.Display())
			}
			c.Print("%s", msg)
		}
		return
	}
	if a := c.Pack.RPG.AbilityByVerb(p.Verb); a != nil {
		useAbility(c, a, p)
		return
	}
	c.Print("You're not sure how to use %s.", e.Display())
}

func useAbility(c *Context, a *rpg.Ability, p Parsed) {
	if c.Actor.Cooldown == nil {
		c.Actor.Cooldown = map[string]int64{}
	}
	if until := c.Actor.Cooldown[a.ID]; until > c.Tick {
		c.Print("That's still cooling down.")
		return
	}
	for res, cost := range a.Cost {
		if c.Actor.Res(res).Current < cost {
			c.Print("You don't have enough %s.", c.Pack.Lexicon.Label(res))
			return
		}
	}
	var tgt *world.Entity
	switch a.Target {
	case "self", "":
		tgt = c.Actor
	case "room":
		tgt = c.World.RoomOf(c.Actor)
	default:
		if p.Rest == "" {
			c.Print("%s whom?", a.Name)
			return
		}
		var err error
		tgt, err = resolve(c, p.Rest, world.ScopeRoom)
		if err != nil {
			c.Print("%s", err.Error())
			return
		}
	}
	for res, cost := range a.Cost {
		c.Actor.AdjustRes(res, -cost)
	}
	if a.Cooldown > 0 {
		c.Actor.Cooldown[a.ID] = c.Tick + int64(a.Cooldown)
	}
	if msg := a.Messages["self"]; msg != "" {
		c.Print("%s", msg)
	} else {
		c.Print("You use %s.", a.Name)
	}
	for _, ef := range a.Effects {
		if ef.Heal != nil && tgt != nil {
			n := 1
			if ef.Heal.Dice != "" && c.RNG != nil {
				if rolled, err := dice.Roll(c.RNG, ef.Heal.Dice); err == nil {
					n = rolled
				}
			}
			res := ef.Heal.Resource
			if res == "" {
				res = c.Pack.RPG.Primary()
			}
			tgt.AdjustRes(res, n)
		}
		if ef.SetFlag != "" && tgt != nil {
			tgt.SetFlag(ef.SetFlag, true)
		}
		if ef.ClearFlag != "" && tgt != nil {
			tgt.SetFlag(ef.ClearFlag, false)
		}
	}
	if a.Scripts != "" {
		_, _ = c.Scripts.CallFile(a.Scripts, "on_use", c.Actor, tgt)
	}
}

func doorTarget(c *Context, rest string) (room *world.Entity, dir string, ex world.Exit, ok bool) {
	room = c.World.RoomOf(c.Actor)
	if room == nil {
		return
	}
	dir = world.CanonicalDir(firstWord(rest))
	if rest == "" || rest == "door" {
		// unique door
		var found string
		for d, e := range room.Exits {
			if e.Door {
				if found != "" {
					c.Print("Which door? Try a direction.")
					return
				}
				found = d
			}
		}
		if found == "" {
			c.Print("There is no door here.")
			return
		}
		dir = found
	}
	ex, exists := room.Exits[dir]
	if !exists {
		c.Print("There is no exit %s.", dir)
		return
	}
	ok = true
	return
}

func setExit(room *world.Entity, dir string, ex world.Exit) {
	room.Exits[dir] = ex
	ex.Dir = dir
	room.Exits[dir] = ex
}

func openCmd(c *Context, p Parsed) {
	room, dir, ex, ok := doorTarget(c, p.Rest)
	if !ok {
		return
	}
	if !ex.Door {
		c.Print("There's no door that way.")
		return
	}
	if ex.Locked {
		c.Print("It's locked.")
		return
	}
	if !ex.Closed {
		c.Print("It's already open.")
		return
	}
	ex.Closed = false
	setExit(room, dir, ex)
	c.Print("You open the door to the %s.", dir)
}

func closeCmd(c *Context, p Parsed) {
	room, dir, ex, ok := doorTarget(c, p.Rest)
	if !ok {
		return
	}
	if !ex.Door {
		c.Print("There's no door that way.")
		return
	}
	if ex.Closed {
		c.Print("It's already closed.")
		return
	}
	ex.Closed = true
	setExit(room, dir, ex)
	c.Print("You close the door to the %s.", dir)
}

func lockCmd(c *Context, p Parsed) {
	room, dir, ex, ok := doorTarget(c, p.Rest)
	if !ok {
		return
	}
	if ex.Key == "" {
		c.Print("It doesn't have a lock.")
		return
	}
	if !hasKey(c, ex.Key) {
		c.Print("You don't have the key.")
		return
	}
	ex.Locked = true
	ex.Closed = true
	ex.Door = true
	setExit(room, dir, ex)
	c.Print("You lock the door to the %s.", dir)
}

func unlockCmd(c *Context, p Parsed) {
	room, dir, ex, ok := doorTarget(c, p.Rest)
	if !ok {
		return
	}
	if !ex.Locked {
		c.Print("It isn't locked.")
		return
	}
	if !hasKey(c, ex.Key) {
		c.Print("You don't have the key.")
		return
	}
	ex.Locked = false
	setExit(room, dir, ex)
	c.Print("You unlock the door to the %s.", dir)
}

func hasKey(c *Context, key string) bool {
	for _, it := range c.World.Children(c.Actor.ID) {
		if string(it.ID) == key || string(it.PrototypeID) == key || it.HasKeyword(key) {
			return true
		}
		if strings.HasPrefix(string(it.PrototypeID), key) || strings.HasPrefix(string(it.ID), key) {
			return true
		}
	}
	return false
}

func kill(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Kill whom?")
		return
	}
	e, err := resolve(c, p.Rest, world.ScopeRoom)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if e.Kind != world.KindMobile && e.Kind != world.KindPlayer {
		c.Print("You can't fight %s.", e.Display())
		return
	}
	if e.ID == c.Actor.ID {
		c.Print("You consider it, then think better of it.")
		return
	}
	if c.Actor.Combat == nil {
		c.Actor.Combat = &world.Combat{RoundSpeed: c.Pack.Meta.CombatEvery, Attacks: []world.Attack{{Name: "strike", Damage: "1d6"}}}
	}
	if e.Combat == nil {
		e.Combat = &world.Combat{RoundSpeed: c.Pack.Meta.CombatEvery, Attacks: []world.Attack{{Name: "hit", Damage: "1d4"}}}
	}
	c.Actor.Combat.Target = e.ID
	c.Actor.Combat.NextSwing = c.Tick + 1
	if e.Combat.Target == "" {
		e.Combat.Target = c.Actor.ID
		e.Combat.NextSwing = c.Tick + 1
	}
	c.Print("You attack %s!", e.Display())
}

func flee(c *Context, _ Parsed) {
	if c.Actor.Combat == nil || c.Actor.Combat.Target == "" {
		c.Print("You aren't fighting.")
		return
	}
	room := c.World.RoomOf(c.Actor)
	var dirs []string
	for d, ex := range room.Exits {
		if !ex.Closed {
			dirs = append(dirs, d)
		}
	}
	if len(dirs) == 0 {
		c.Print("There's nowhere to run!")
		return
	}
	c.Actor.Combat.Target = ""
	c.Actor.Combat.NextSwing = 0
	move(c, dirs[0])
	c.Print("You flee %s!", dirs[0])
}

func say(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Say what?")
		return
	}
	c.Say("You say, \"%s\"", p.Rest)
}

func emote(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Emote what?")
		return
	}
	c.Say("%s %s", c.Actor.Name, p.Rest)
}

func talk(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Talk to whom?")
		return
	}
	npc, err := resolve(c, p.Rest, world.ScopeRoom)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if npc.Kind != world.KindMobile && npc.Kind != world.KindPlayer {
		c.Print("You get no answer.")
		return
	}
	if len(npc.Topics) == 0 {
		c.Print("%s has nothing particular to say. Try ask %s about <topic> if you learn a word.", npc.CapDisplay(), firstWord(npc.Name))
		return
	}
	keys := make([]string, 0, len(npc.Topics))
	for k := range npc.Topics {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	c.Print("%s might talk about: %s.", npc.CapDisplay(), strings.Join(keys, ", "))
	c.Print("Ask with: ask %s about %s", firstWord(p.Rest), keys[0])
}

func listShop(c *Context, p Parsed) {
	npc, err := shopkeep(c, p.Rest)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	goods := c.World.Children(npc.ID)
	if len(goods) == 0 {
		c.Print("%s has nothing for sale.", npc.CapDisplay())
		return
	}
	cur := shopCurrency(npc)
	c.Print("%s offers:", npc.CapDisplay())
	for _, it := range goods {
		c.Print("  %s  (%d %s)", it.Display(), shopPrice(npc, it), c.Pack.Lexicon.Label(cur))
	}
	c.Print("Your %s: %d", c.Pack.Lexicon.Label(cur), c.Actor.Res(cur).Current)
}

func buy(c *Context, p Parsed) {
	rest := p.Rest
	from := ""
	what := rest
	low := strings.ToLower(rest)
	if i := strings.Index(low, " from "); i >= 0 {
		what = strings.TrimSpace(rest[:i])
		from = strings.TrimSpace(rest[i+6:])
	}
	npc, err := shopkeep(c, from)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if what == "" {
		c.Print("Buy what?")
		return
	}
	var item *world.Entity
	for _, it := range c.World.Children(npc.ID) {
		if it.HasKeyword(firstWord(what)) {
			if item != nil {
				c.Print("Which one?")
				return
			}
			item = it
		}
	}
	if item == nil {
		c.Print("%s doesn't have a %q.", npc.CapDisplay(), what)
		return
	}
	cur := shopCurrency(npc)
	price := shopPrice(npc, item)
	if c.Actor.Res(cur).Current < price {
		c.Print("You can't afford that (%d %s).", price, c.Pack.Lexicon.Label(cur))
		return
	}
	c.Actor.AdjustRes(cur, -price)
	if err := c.World.Move(item.ID, c.Actor.ID); err != nil {
		c.Actor.AdjustRes(cur, price)
		c.Print("%s", err.Error())
		return
	}
	c.Print("You buy %s for %d %s.", item.Display(), price, c.Pack.Lexicon.Label(cur))
}

func sell(c *Context, p Parsed) {
	rest := p.Rest
	to := ""
	what := rest
	low := strings.ToLower(rest)
	if i := strings.Index(low, " to "); i >= 0 {
		what = strings.TrimSpace(rest[:i])
		to = strings.TrimSpace(rest[i+4:])
	}
	npc, err := shopkeep(c, to)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	item, err := resolve(c, what, world.ScopeInventory)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	cur := shopCurrency(npc)
	price := item.Value
	if price < 1 {
		c.Print("%s isn't interested in %s.", npc.CapDisplay(), item.Display())
		return
	}
	unequipIfWorn(c, item)
	if err := c.World.Move(item.ID, npc.ID); err != nil {
		c.Print("%s", err.Error())
		return
	}
	c.Actor.AdjustRes(cur, price)
	c.Print("You sell %s for %d %s.", item.Display(), price, c.Pack.Lexicon.Label(cur))
}

func shopkeep(c *Context, token string) (*world.Entity, error) {
	if token != "" {
		e, err := resolve(c, token, world.ScopeRoom)
		if err != nil {
			return nil, err
		}
		if e.Shop == nil {
			return nil, fmt.Errorf("%s isn't selling anything", e.CapDisplay())
		}
		return e, nil
	}
	room := c.World.RoomOf(c.Actor)
	var found *world.Entity
	for _, e := range c.World.Children(room.ID) {
		if e.Shop != nil {
			if found != nil {
				return nil, fmt.Errorf("which shopkeeper?")
			}
			found = e
		}
	}
	if found == nil {
		return nil, fmt.Errorf("no one here is buying or selling")
	}
	return found, nil
}

func shopCurrency(npc *world.Entity) string {
	if npc.Shop != nil && npc.Shop.Currency != "" {
		return npc.Shop.Currency
	}
	return "gold"
}

func shopPrice(npc, item *world.Entity) int {
	n := item.Value
	if n < 1 {
		n = 1
	}
	if npc.Shop != nil && npc.Shop.Markup > 0 {
		n += n * npc.Shop.Markup / 100
	}
	return n
}

func ask(c *Context, p Parsed) {
	low := strings.ToLower(p.Rest)
	i := strings.Index(low, " about ")
	if i < 0 {
		talk(c, p)
		return
	}
	whom := strings.TrimSpace(p.Rest[:i])
	topic := strings.TrimSpace(p.Rest[i+7:])
	npc, err := resolve(c, whom, world.ScopeRoom)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	handled, err := c.Scripts.Call("on_ask", c.Actor, npc, topic)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if handled {
		return
	}
	if npc.Topics != nil {
		if resp, ok := npc.Topics[strings.ToLower(topic)]; ok {
			c.Say("%s says, \"%s\"", npc.CapDisplay(), resp)
			return
		}
		for k, resp := range npc.Topics {
			if strings.Contains(strings.ToLower(topic), k) || strings.Contains(k, strings.ToLower(topic)) {
				c.Say("%s says, \"%s\"", npc.CapDisplay(), resp)
				return
			}
		}
	}
	c.Print("%s doesn't know about that.", npc.CapDisplay())
}

func goCmd(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Go where?")
		return
	}
	move(c, world.CanonicalDir(p.Rest))
}

func move(c *Context, dir string) {
	dir = world.CanonicalDir(dir)
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		c.Print("You can't go anywhere from nowhere.")
		return
	}
	ex, ok := room.Exits[dir]
	if !ok {
		if c.OnMoveFail != nil && c.OnMoveFail(dir) {
			return
		}
		c.Print("You can't go that way.")
		return
	}
	if ex.Closed {
		c.Print("The way %s is closed.", dir)
		return
	}
	dest := c.World.Get(ex.To)
	if dest == nil {
		c.Print("The exit leads nowhere.")
		return
	}
	if c.Actor.InCombat() {
		c.Print("You can't just walk away from a fight. Try flee.")
		return
	}
	old := room
	_ = c.World.Move(c.Actor.ID, dest.ID)
	_, _ = c.Scripts.Call("on_leave", c.Actor, old)
	_, _ = c.Scripts.Call("on_enter", c.Actor, dest)
	DescribeRoom(c, true)
}

func saveCmd(c *Context, p Parsed) {
	if c.Mode == protocol.ModeBuild && (p.Rest == "pack" || strings.HasPrefix(strings.ToLower(p.Rest), "pack ")) {
		if c.SavePack == nil {
			c.Print("Pack save isn't available.")
			return
		}
		if err := c.SavePack(); err != nil {
			c.Print("Pack save failed: %s", err.Error())
			return
		}
		c.System("Pack written to disk.")
		return
	}
	slot := p.Rest
	if slot == "" {
		slot = "autosave"
	}
	if c.SaveSnap == nil {
		c.Print("Saving isn't available.")
		return
	}
	if err := c.SaveSnap(slot); err != nil {
		c.Print("Save failed: %s", err.Error())
		return
	}
	c.System("Saved (%s).", slot)
}

func quitCmd(c *Context, _ Parsed) {
	if c.SaveSnap != nil {
		_ = c.SaveSnap("autosave")
	}
	c.System("Goodbye.")
	if c.Quit != nil {
		c.Quit()
	}
}

func who(c *Context, _ Parsed) {
	c.Print("Players: %s", c.Actor.Name)
}
