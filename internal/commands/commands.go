package commands

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	"sudengine/internal/dice"
	"sudengine/internal/docs"
	"sudengine/internal/economy"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/quest"
	"sudengine/internal/rpg"
	"sudengine/internal/script"
	"sudengine/internal/weather"
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
	Env     *weather.Environment

	Tell          func(channel protocol.Channel, text string)
	Quit          func()
	SaveSnap      func(slot string) error
	LoadSnap      func(slot string) error
	SavePack      func() error
	ReloadScripts func() error
	StartFight    func(mob *world.Entity)
	Assist        func(target *world.Entity, leaderAttacking bool)
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
	register("search", searchCmd)
	register("improve", improveCmd)
	register("raise", raiseCmd)
	register("train", trainCmd)
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
	register("time", timeCmd)
	register("weather", weatherCmd)
	register("sky", weatherCmd)
	register("repair", repairCmd)
	register("recipes", recipesCmd)
	register("craft", craftCmd)
	register("deposit", depositCmd)
	register("withdraw", withdrawCmd)
	register("balance", balanceCmd)
	register("gather", gatherCmd)
	register("quests", questsCmd)
	register("quest", questCmd)
	register("skin", skinCmd)
	register("salvage", salvageCmd)
	register("follow", followCmd)
	register("unfollow", unfollowCmd)
	register("lose", loseCmd)
	register("group", groupCmd)
	register("invite", inviteCmd)
	register("join", joinCmd)
	register("order", orderCmd)
	register("feed", feedCmd)
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

// EnterRoom is DescribeRoom plus a notice roll for hidden things.
func EnterRoom(c *Context, verbose bool) {
	showRoom(c, verbose, true)
}

func DescribeRoom(c *Context, verbose bool) {
	showRoom(c, verbose, false)
}

func showRoom(c *Context, verbose, enter bool) {
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		c.Print("You are nowhere.")
		return
	}
	c.RoomTitle(room.Name)
	if enter {
		blowOutFlames(c)
		checkAutoStarts(c)
		if c.Pack != nil && len(c.Pack.Quests) > 0 {
			notices := quest.UpdateVisit(c.Actor, room.ID, c.Pack.Quests, c.World)
			for _, n := range notices {
				c.Alert("%s", n.String())
			}
		}
	}
	lit := c.World.RoomIsLit(room)
	if !lit {
		c.Print("It's pitch black. You can't see a thing.")
		return
	}
	if verbose && room.Long != "" {
		c.Print("%s", strings.TrimSpace(room.Long))
	}
	if enter {
		noticeOnEnter(c, room)
	}
	_, _ = c.Scripts.Call("on_look", c.Actor, room)
	c.Print("%s", "")
	hadContents := false
	for _, e := range c.World.Children(room.ID) {
		if e.ID == c.Actor.ID {
			continue
		}
		if hiddenFrom(c.Actor, e) {
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
	if strings.EqualFold(p.Rest, "sky") {
		weatherCmd(c, p)
		return
	}
	e, err := resolve(c, p.Rest, world.ScopeInventory, world.ScopeEquipment, world.ScopeRoom, world.ScopeSelf)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if unseenInDark(c, e) {
		c.Print("It's pitch black. You can't see a thing.")
		return
	}
	if e.Hidden && !c.Actor.HasFound(e.ID) {
		if !revealHidden(c, e) {
			c.Print("You can't make it out.")
			return
		}
	}
	if e.Long != "" {
		c.Print("%s", strings.TrimSpace(e.Long))
	} else {
		c.Print("You see nothing special about %s.", e.Display())
	}
	if e.MaxDurability > 0 {
		cond := economy.Condition(e.Durability, e.MaxDurability, e.HasFlag("broken"))
		tag := economy.DurabilityTag(e)
		c.Print("Condition: %s %s.", cond, tag)
	}
	if e.Use != nil && e.Use.LearnRecipe != "" {
		if c.Actor.KnowsRecipe(e.Use.LearnRecipe) {
			c.Print("You have already mastered the crafting instructions in this.")
		} else if len(e.Keywords) > 0 {
			c.Print("It contains crafting instructions. Type 'use %s' to study and learn it.", e.Keywords[0])
		} else {
			c.Print("It contains crafting instructions. Type 'use' to study and learn it.")
		}
	}
	if e.Gather != nil {
		if e.Gather.Remaining > 0 {
			resName := e.Gather.Name
			if resName == "" {
				resName = "resources"
			}
			c.Print("It looks like you could gather %s here. Type 'gather' to forage.", resName)
		} else {
			c.Print("It looks picked clean for now.")
		}
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
	if e.AI != nil && e.AI.WakesOnLook() {
		e.SetFlag(e.AI.HostileFlag(), true)
		if c.StartFight != nil && e.AI.Awake(e) {
			c.StartFight(e)
		}
	}
}

func unseenInDark(c *Context, e *world.Entity) bool {
	if e == nil || c.Actor == nil || c.World == nil {
		return false
	}
	if e.Parent == c.Actor.ID {
		return false
	}
	room := c.World.RoomOf(c.Actor)
	if room == nil || c.World.RoomOf(e) != room {
		return false
	}
	return !c.World.RoomIsLit(room)
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
		tag := economy.DurabilityTag(it)
		if tag != "" {
			c.Print("  %s: %s %s", s, it.Display(), tag)
		} else {
			c.Print("  %s: %s", s, it.Display())
		}
	}
}

func exitsCmd(c *Context, _ Parsed) {
	room := c.World.RoomOf(c.Actor)
	c.Print("Obvious exits: %s.", exitList(room))
}

func score(c *Context, _ Parsed) {
	c.Print("%s", c.Actor.Name)
	if c.Pack != nil && c.Pack.RPG.Advancement.On() {
		lvl := c.Actor.Level
		if lvl < 1 {
			lvl = 1
		}
		need := c.Pack.RPG.Advancement.Cost(lvl)
		c.Print("  Level %d    experience %d / %d", lvl, c.Actor.XP, need)
		if c.Actor.SkillPoints > 0 || c.Actor.AttrPoints > 0 {
			c.Print("  Skill points %d    attribute points %d", c.Actor.SkillPoints, c.Actor.AttrPoints)
			c.Print("  improve <skill>    raise <attribute>    train <skill>")
		}
	}
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
		if c.Pack.RPG.Pile(k) {
			c.Print("  %-12s %d", c.Pack.Lexicon.Label(k), r.Current)
			continue
		}
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
	if c.Pack != nil && len(c.Pack.RPG.Skills) > 0 {
		c.Print("%s", "")
		c.Print("Skills")
		for _, sk := range c.Pack.RPG.Skills {
			c.Print("%s", skillLine(c, &sk))
		}
		if c.Pack.RPG.Advancement.On() {
			c.Print("  train with no skill lists these. improve spends a point. train pays a trainer.")
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
	if e.Hidden && !c.Actor.HasFound(e.ID) {
		c.Print("You don't see that here.")
		return
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
	if c.Pack != nil {
		for _, q := range c.Pack.Quests {
			if !quest.IsActive(c.Actor, q) {
				continue
			}
			for _, obj := range q.Objectives {
				if obj.Kind == quest.KindCollect {
					match := strings.EqualFold(string(e.PrototypeID), obj.Target) ||
						e.HasKeyword(strings.ToLower(obj.Target)) ||
						strings.EqualFold(string(e.ID), obj.Target)
					if match {
						cur, need, done := quest.ObjectiveProgress(c.Actor, c.World, q, obj)
						if done && quest.IsReady(c.Actor, c.World, q) {
							c.Alert("[Quest Ready for Turn-in: %s]", q.Title)
						} else if done {
							c.Alert("[Objective Complete: %s (%d/%d)]", obj.Desc, cur, need)
						} else {
							c.Alert("[Objective Progress: %s (%d/%d)]", obj.Desc, cur, need)
						}
					}
				}
			}
		}
	}
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
	if strings.Contains(strings.ToLower(p.Rest), " on ") {
		parts := strings.SplitN(p.Rest, " on ", 2)
		kitStr := strings.TrimSpace(parts[0])
		itemStr := strings.TrimSpace(parts[1])
		kit, err := resolve(c, kitStr, world.ScopeInventory)
		if err == nil && kit.RepairKit != nil {
			it, err := resolve(c, itemStr, world.ScopeInventory, world.ScopeEquipment)
			if err != nil {
				c.Print("%s", err.Error())
				return
			}
			repairWithKit(c, it, kit)
			return
		}
	}
	e, err := resolve(c, p.Rest, world.ScopeInventory, world.ScopeEquipment, world.ScopeRoom)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if e.RepairKit != nil {
		c.Print("Use %s on what? (Try: use %s on <item>, or repair <item> with %s)", e.Display(), firstWord(e.Name), firstWord(e.Name))
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
	if e.Use != nil && e.Use.LearnRecipe != "" {
		recipeID := e.Use.LearnRecipe
		method := "discover"
		if c.Pack != nil {
			for _, r := range c.Pack.Recipes {
				if strings.EqualFold(r.ID, recipeID) {
					if r.Discovery != "" && r.Discovery != "all" {
						method = r.Discovery
					}
					break
				}
			}
		}
		learned := c.Actor.LearnRecipe(recipeID, method)
		recipeName := recipeID
		if c.Pack != nil {
			for _, r := range c.Pack.Recipes {
				if strings.EqualFold(r.ID, recipeID) {
					recipeName = r.Name
					break
				}
			}
		}
		if learned {
			msg := e.Use.Message
			if msg == "" {
				msg = fmt.Sprintf("You study %s and learn how to craft %s.", e.Display(), recipeName)
			}
			c.Print("%s", msg)
		} else {
			c.Print("You already know how to craft %s.", recipeName)
		}
		return
	}
	if e.Use != nil && e.Use.ToggleFlag != "" {
		on := e.HasFlag(e.Use.ToggleFlag)
		if !on && e.Use.ToggleFlag == "light" && flameWontCatch(c) {
			c.Print("%s", weather.FlameSnuffLine)
			return
		}
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
	for key, need := range a.Requires {
		if c.Pack.RPG.Effective(c.Actor, key) < need {
			c.Print("You need more %s for that.", c.Pack.Lexicon.Label(key))
			return
		}
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

func setExit(c *Context, room *world.Entity, dir string, ex world.Exit) {
	world.MirrorExit(c.World, room, dir, ex)
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
	setExit(c, room, dir, ex)
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
	setExit(c, room, dir, ex)
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
	setExit(c, room, dir, ex)
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
	setExit(c, room, dir, ex)
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
	opened := e.Combat.Target == ""
	c.Actor.Combat.Target = e.ID
	c.Actor.Combat.NextSwing = c.Tick + 1
	if c.Assist != nil {
		c.Assist(e, true)
	}
	if opened {
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
	if res, ok := rollCheck(c, "escape", c.Actor.Combat.Target); ok && !res.Success {
		line := c.Pack.RPG.Decorate("You can't get clear.", 0, false, "", &res)
		c.Print("%s", line)
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
	hasQuestOffers := false
	if c.Pack != nil {
		for _, q := range c.Pack.Quests {
			turnInID := q.TurnIn
			if turnInID == "" {
				turnInID = q.Giver
			}
			isTurnIn := strings.EqualFold(string(npc.ID), turnInID) || strings.EqualFold(string(npc.PrototypeID), turnInID) || npc.HasKeyword(strings.ToLower(turnInID))
			isGiver := strings.EqualFold(string(npc.ID), q.Giver) || strings.EqualFold(string(npc.PrototypeID), q.Giver) || npc.HasKeyword(strings.ToLower(q.Giver))

			if isTurnIn && quest.IsActive(c.Actor, q) && quest.IsReady(c.Actor, c.World, q) {
				c.Print("[Quest Ready for Turn-in: %s] (Type: quest complete %s)", q.Title, q.ID)
				hasQuestOffers = true
			} else if isGiver && quest.IsAvailable(c.Actor, q, c.Pack.Quests) {
				c.Print("[Quest Available: %s] (Type: quest accept %s)", q.Title, q.ID)
				hasQuestOffers = true
			}
		}
	}
	if len(npc.Topics) == 0 {
		if !hasQuestOffers {
			c.Print("%s has nothing particular to say. Try ask %s about <topic> if you learn a word.", npc.CapDisplay(), firstWord(npc.Name))
		}
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
	cur := shopCurrency(c, npc)
	if len(goods) == 0 {
		c.Print("%s has nothing for sale.", npc.CapDisplay())
		if purse, finite := economy.VendorPurse(npc); finite {
			c.Print("%s has %d %s in the till.", npc.CapDisplay(), purse, c.Pack.Lexicon.Label(cur))
		}
		c.Print("Your %s: %d", c.Pack.Lexicon.Label(cur), c.Actor.Res(cur).Current)
		return
	}
	c.Print("%s offers:", npc.CapDisplay())
	for _, it := range goods {
		c.Print("  %s  (%d %s)", it.Display(), shopPrice(c, npc, it), c.Pack.Lexicon.Label(cur))
	}
	if purse, finite := economy.VendorPurse(npc); finite {
		c.Print("%s has %d %s in the till.", npc.CapDisplay(), purse, c.Pack.Lexicon.Label(cur))
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
				if (item.PrototypeID == "" || item.PrototypeID != it.PrototypeID) || item.Name != it.Name || item.Value != it.Value {
					c.Print("Which one?")
					return
				}
				continue
			}
			item = it
		}
	}
	if item == nil {
		room := c.World.RoomOf(npc)
		if room != nil {
			for _, rid := range room.Contents {
				it := c.World.Get(rid)
				if it == nil || it.ID == npc.ID || it.ID == c.Actor.ID {
					continue
				}
				if it.Owner != npc.ID || it.Companion {
					continue
				}
				if it.HasKeyword(firstWord(what)) {
					if item != nil {
						if (item.PrototypeID == "" || item.PrototypeID != it.PrototypeID) || item.Name != it.Name || item.Value != it.Value {
							c.Print("Which one?")
							return
						}
						continue
					}
					item = it
				}
			}
		}
	}
	if item == nil {
		c.Print("%s doesn't have a %q.", npc.CapDisplay(), what)
		return
	}
	if item.Owner == c.Actor.ID {
		c.Print("You already own %s.", item.Display())
		return
	}
	cur := shopCurrency(c, npc)
	price := shopPrice(c, npc, item)
	if c.Actor.Res(cur).Current < price {
		c.Print("You can't afford that (%d %s).", price, c.Pack.Lexicon.Label(cur))
		return
	}
	c.Actor.AdjustRes(cur, -price)
	if item.Kind == world.KindMobile || item.Companion || item.HasFlag("pet") {
		if err := c.World.StartFollowing(item, c.Actor); err != nil {
			c.Actor.AdjustRes(cur, price)
			c.Print("%s", err.Error())
			return
		}
		room := c.World.RoomOf(c.Actor)
		if room != nil && item.Parent != room.ID {
			_ = c.World.Move(item.ID, room.ID)
		}
		item.Kind = world.KindMobile
		item.Companion = true
		item.Owner = c.Actor.ID
		if item.MaxLoyalty <= 0 {
			item.MaxLoyalty = 100
		}
		if item.Loyalty <= 0 {
			item.Loyalty = item.MaxLoyalty
		}
		if item.Loyalty > item.MaxLoyalty {
			item.Loyalty = item.MaxLoyalty
		}
		if item.Stance == "" {
			item.Stance = "assist"
		}
		world.BreakFight(item, c.Actor)
		economy.VendorReceive(npc, price)
		c.Print("You buy %s for %d %s. %s is now following you!", item.Display(), price, c.Pack.Lexicon.Label(cur), item.CapDisplay())
		return
	}
	if err := c.World.Move(item.ID, c.Actor.ID); err != nil {
		c.Actor.AdjustRes(cur, price)
		c.Print("%s", err.Error())
		return
	}
	economy.VendorReceive(npc, price)
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
	cur := shopCurrency(c, npc)
	price := economy.SellPrice(item, npc, c.World)
	label := c.Pack.Lexicon.Label(cur)
	if price < 1 {
		c.Print("%s isn't interested in %s.", npc.CapDisplay(), item.Display())
		return
	}
	before := c.Actor.Res(cur).Current
	c.Actor.AdjustRes(cur, price)
	gained := c.Actor.Res(cur).Current - before
	if gained < 1 {
		c.Print("You can't hold any more %s.", label)
		return
	}
	if !economy.CanVendorAfford(npc, gained) {
		c.Actor.AdjustRes(cur, -gained)
		purse, _ := economy.VendorPurse(npc)
		c.Print("%s only has %d %s and cannot afford %s.", npc.CapDisplay(), purse, label, item.Display())
		return
	}
	unequipIfWorn(c, item)
	if err := c.World.Move(item.ID, npc.ID); err != nil {
		c.Actor.AdjustRes(cur, -gained)
		c.Print("%s", err.Error())
		return
	}
	economy.VendorPay(npc, gained)
	c.Print("You sell %s for %d %s.", item.Display(), gained, label)
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

func shopCurrency(c *Context, npc *world.Entity) string {
	if npc != nil && npc.Shop != nil && npc.Shop.Currency != "" {
		return npc.Shop.Currency
	}
	if c != nil && c.Pack != nil {
		if cur := c.Pack.RPG.Advancement.Purse(); cur != "" {
			return cur
		}
	}
	return "coin"
}

func shopPrice(c *Context, npc, item *world.Entity) int {
	return economy.BuyPrice(npc, item, c.World)
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
	teachIfAny := func(matchedTopic string) {
		if npc.TeachRecipes == nil {
			return
		}
		for k, recID := range npc.TeachRecipes {
			if strings.EqualFold(matchedTopic, k) || strings.EqualFold(topic, k) {
				if c.Actor.LearnRecipe(recID, "grant") {
					recName := recID
					if c.Pack != nil {
						for _, r := range c.Pack.Recipes {
							if strings.EqualFold(r.ID, recID) {
								recName = r.Name
								break
							}
						}
					}
					c.Print("You learned how to craft %s!", recName)
				}
			}
		}
	}
	checkQuestTalk := func(matchedTopic string) {
		if c.Pack != nil && len(c.Pack.Quests) > 0 {
			notices := quest.UpdateTalk(c.Actor, npc, matchedTopic, c.Pack.Quests, c.World)
			for _, n := range notices {
				c.Alert("%s", n.String())
			}
		}
	}
	if npc.Topics != nil {
		topicLower := strings.ToLower(topic)
		if resp, ok := npc.Topics[topicLower]; ok {
			c.Say("%s says, \"%s\"", npc.CapDisplay(), resp)
			teachIfAny(topicLower)
			checkQuestTalk(topicLower)
			return
		}
		var hits []string
		for k := range npc.Topics {
			lk := strings.ToLower(k)
			if strings.Contains(topicLower, lk) || strings.Contains(lk, topicLower) {
				hits = append(hits, k)
			}
		}
		sort.Strings(hits)
		switch len(hits) {
		case 0:
		case 1:
			k := hits[0]
			c.Say("%s says, \"%s\"", npc.CapDisplay(), npc.Topics[k])
			teachIfAny(k)
			checkQuestTalk(k)
			return
		default:
			c.Print("%s isn't sure which you mean: %s.", npc.CapDisplay(), strings.Join(hits, ", "))
			return
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
	Arrive(c, old, dest)
	moveFollowers(c, c.Actor, old, dest, dir)
}

// Arrive runs the leave and enter hooks after the actor is already in dest.
func Arrive(c *Context, old, dest *world.Entity) {
	if c == nil || c.Actor == nil || dest == nil {
		return
	}
	if c.Scripts != nil && old != nil && old.ID != dest.ID {
		_, _ = c.Scripts.Call("on_leave", c.Actor, old)
		_, _ = c.Scripts.Call("on_enter", c.Actor, dest)
	}
	EnterRoom(c, true)
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

func blowOutFlames(c *Context) {
	if c == nil || c.Env == nil || c.World == nil || c.Actor == nil {
		return
	}
	room := c.World.RoomOf(c.Actor)
	if room == nil || !c.Env.Extinguishes(room.Outdoor()) {
		return
	}
	if len(c.World.ExtinguishToggleLights(room)) > 0 {
		c.Alert("%s", weather.FlameOutLine)
	}
}

func flameWontCatch(c *Context) bool {
	if c == nil || c.Env == nil || c.World == nil || c.Actor == nil {
		return false
	}
	room := c.World.RoomOf(c.Actor)
	return room != nil && c.Env.Extinguishes(room.Outdoor())
}

func who(c *Context, _ Parsed) {
	c.Print("Players: %s", c.Actor.Name)
}

func timeCmd(c *Context, _ Parsed) {
	if c.Env == nil || c.Env.TimeConfig.Level == 0 {
		c.Print("Time has no meaning here.")
		return
	}
	ti := c.Env.GetTime(c.Tick)
	c.Print("It is %s.", ti.String())
}

func weatherCmd(c *Context, _ Parsed) {
	if c.Env == nil || c.Env.WeatherConfig.Level == 0 {
		c.Print("The weather is calm and unchanging.")
		return
	}
	room := c.World.RoomOf(c.Actor)
	isOutdoor := room != nil && room.Outdoor()
	isWindowed := room != nil && room.Windowed()
	desc, ok := c.Env.DescribeSky(isOutdoor, isWindowed)
	if !ok {
		c.Print("%s", desc)
		return
	}
	st := c.Env.CurrentState()
	c.Print("%s (%s)", desc, st.Label)
}

func repairCmd(c *Context, p Parsed) {
	if strings.Contains(strings.ToLower(p.Rest), " with ") {
		parts := strings.SplitN(p.Rest, " with ", 2)
		itemStr := strings.TrimSpace(parts[0])
		kitStr := strings.TrimSpace(parts[1])
		it, err := resolve(c, itemStr, world.ScopeInventory, world.ScopeEquipment)
		if err != nil {
			c.Print("%s", err.Error())
			return
		}
		kit, err := resolve(c, kitStr, world.ScopeInventory)
		if err != nil {
			c.Print("%s", err.Error())
			return
		}
		if kit.RepairKit == nil {
			c.Print("%s is not a repair kit.", kit.Display())
			return
		}
		repairWithKit(c, it, kit)
		return
	}

	repairer := repairerHere(c)
	if repairer == nil {
		if p.Rest != "" {
			kit := findKitInInventory(c)
			if kit != nil {
				it, err := resolve(c, p.Rest, world.ScopeInventory, world.ScopeEquipment)
				if err == nil {
					repairWithKit(c, it, kit)
					return
				}
			}
		}
		c.Print("No one here can repair your gear.")
		return
	}
	cur := shopCurrency(c, repairer)

	carried := c.World.Children(c.Actor.ID)
	var damaged []*world.Entity
	seen := map[world.ID]bool{}
	checkDamaged := func(it *world.Entity) {
		if it != nil && it.MaxDurability > 0 && it.Durability < it.MaxDurability && it.Repairable && !seen[it.ID] {
			damaged = append(damaged, it)
			seen[it.ID] = true
		}
	}
	for _, it := range carried {
		checkDamaged(it)
	}
	if c.Actor.Equipment != nil {
		for _, id := range c.Actor.Equipment {
			checkDamaged(c.World.Get(id))
		}
	}

	target := strings.TrimSpace(p.Rest)
	if target == "" {
		if len(damaged) == 0 {
			c.Print("All of your equipment is in pristine condition.")
			return
		}
		c.Print("%s can repair:", repairer.CapDisplay())
		total := 0
		for _, it := range damaged {
			cost := economy.RepairCost(it)
			total += cost
			c.Print("  %s %s — %d %s", it.Display(), economy.DurabilityTag(it), cost, c.Pack.Lexicon.Label(cur))
		}
		c.Print("Total repair cost: %d %s (You have: %d). Type 'repair all' or 'repair <item>' to fix.", total, c.Pack.Lexicon.Label(cur), c.Actor.Res(cur).Current)
		return
	}

	if strings.EqualFold(target, "all") {
		if len(damaged) == 0 {
			c.Print("All of your equipment is in pristine condition.")
			return
		}
		repaired := 0
		totalSpent := 0
		var skipped []string
		for _, it := range damaged {
			cost := economy.RepairCost(it)
			if c.Actor.Res(cur).Current < cost {
				skipped = append(skipped, it.Display())
				continue
			}
			c.Actor.AdjustRes(cur, -cost)
			economy.VendorReceive(repairer, cost)
			economy.Repair(it)
			totalSpent += cost
			repaired++
		}
		if repaired == 0 {
			c.Print("You can't afford to repair any of your equipment.")
			return
		}
		c.Print("%s repairs %d item(s) for a total of %d %s.", repairer.CapDisplay(), repaired, totalSpent, c.Pack.Lexicon.Label(cur))
		if len(skipped) > 0 {
			c.Print("Still needs repair: %s (not enough %s).", strings.Join(skipped, ", "), c.Pack.Lexicon.Label(cur))
		}
		return
	}

	it, err := resolve(c, target, world.ScopeInventory, world.ScopeEquipment)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if it.MaxDurability <= 0 || !it.Repairable {
		c.Print("%s cannot be repaired.", it.Display())
		return
	}
	if it.Durability >= it.MaxDurability && !it.HasFlag("broken") {
		c.Print("%s is already in pristine condition.", it.Display())
		return
	}
	cost := economy.RepairCost(it)
	if c.Actor.Res(cur).Current < cost {
		c.Print("You need %d %s to repair %s.", cost, c.Pack.Lexicon.Label(cur), it.Display())
		return
	}
	c.Actor.AdjustRes(cur, -cost)
	economy.VendorReceive(repairer, cost)
	economy.Repair(it)
	c.Print("%s repairs %s back to pristine condition for %d %s.", repairer.CapDisplay(), it.Display(), cost, c.Pack.Lexicon.Label(cur))
}

func repairerHere(c *Context) *world.Entity {
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		return nil
	}
	for _, e := range c.World.Children(room.ID) {
		if e.Repairer || e.HasFlag("repairer") || e.HasFlag("blacksmith") {
			return e
		}
	}
	return nil
}

func recipesCmd(c *Context, _ Parsed) {
	if c.Pack == nil || len(c.Pack.Recipes) == 0 {
		c.Print("You don't know any crafting recipes.")
		return
	}
	var known []economy.Recipe
	for _, r := range c.Pack.Recipes {
		if r.IsKnown(c.Actor) {
			known = append(known, r)
		}
	}
	if len(known) == 0 {
		c.Print("You haven't discovered or learned any crafting recipes yet.")
		return
	}
	c.Print("Crafting Recipes:")
	room := c.World.RoomOf(c.Actor)
	for _, r := range known {
		var reqs []string
		for _, req := range r.Requires {
			n := req.Count
			if n <= 0 {
				n = 1
			}
			reqs = append(reqs, fmt.Sprintf("%d %s", n, req.Item))
		}
		var notes []string
		if r.Station != "" {
			if _, ok := economy.CheckStation(c.World, room, r.Station); ok {
				notes = append(notes, r.Station+" here")
			} else {
				notes = append(notes, "need "+r.Station)
			}
		}
		if len(r.Tools) > 0 {
			if missing, ok := economy.CheckTools(c.World, c.Actor, r.Tools); ok {
				notes = append(notes, "tools here")
			} else {
				notes = append(notes, "need "+missing)
			}
		}
		if _, err := economy.FindIngredients(c.World, c.Actor, r.Requires); err == nil {
			notes = append(notes, "materials here")
		} else {
			notes = append(notes, "missing materials")
		}
		if m := c.Actor.RecipeMethod(r.ID); m != "" {
			notes = append(notes, m)
		} else if r.Discovery != "" && r.Discovery != "all" {
			notes = append(notes, r.Discovery)
		}
		c.Print("  %s — %s [%s]", r.Name, strings.Join(reqs, ", "), strings.Join(notes, ", "))
	}
}

func matchRecipes(recipes []economy.Recipe, what string) []*economy.Recipe {
	what = strings.TrimSpace(what)
	low := strings.ToLower(what)
	if low == "" {
		return nil
	}
	var exact, words []*economy.Recipe
	for i := range recipes {
		r := &recipes[i]
		name := strings.ToLower(r.Name)
		id := strings.ToLower(strings.ReplaceAll(r.ID, "_", " "))
		if strings.EqualFold(r.Name, what) || strings.EqualFold(r.ID, what) {
			exact = append(exact, r)
			continue
		}
		if hasWord(name, low) || hasWord(id, low) {
			words = append(words, r)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return words
}

func hasWord(name, word string) bool {
	for _, f := range strings.Fields(name) {
		if f == word {
			return true
		}
	}
	return false
}

func craftCmd(c *Context, p Parsed) {
	if c.Pack == nil || len(c.Pack.Recipes) == 0 {
		c.Print("You don't know any crafting recipes.")
		return
	}
	what := strings.TrimSpace(p.Rest)
	if what == "" {
		c.Print("Craft what? Type 'recipes' to see known blueprints.")
		return
	}
	var known []*economy.Recipe
	for _, r := range matchRecipes(c.Pack.Recipes, what) {
		if r.IsKnown(c.Actor) {
			known = append(known, r)
		}
	}
	if len(known) == 0 {
		c.Print("You don't know how to craft %q.", what)
		return
	}
	if len(known) > 1 {
		names := make([]string, 0, len(known))
		for _, r := range known {
			names = append(names, r.Name)
		}
		c.Print("Which one? %s.", strings.Join(names, ", "))
		return
	}
	crafted, err := economy.Craft(c.World, c.Actor, *known[0], c.RNG, &c.Pack.RPG)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if crafted.HasFlag("superior") {
		c.Print("Your superior craftsmanship produces %s of exceptional quality!", crafted.Display())
	} else {
		c.Print("You craft %s.", crafted.Display())
	}
}

func bankerHere(c *Context) *world.Entity {
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		return nil
	}
	if room.Banker || room.HasFlag("bank") || room.HasFlag("banker") {
		return room
	}
	for _, e := range c.World.Children(room.ID) {
		if e.Banker || e.HasFlag("bank") || e.HasFlag("banker") || e.HasKeyword("banker") || e.HasKeyword("vault") || e.HasKeyword("strongbox") {
			return e
		}
	}
	return nil
}

func bankCurrency(c *Context, banker *world.Entity) string {
	if banker != nil && banker.Shop != nil && banker.Shop.Currency != "" {
		return banker.Shop.Currency
	}
	if c.Pack != nil && c.Pack.RPG.Advancement.Currency != "" {
		return c.Pack.RPG.Advancement.Currency
	}
	if c.Actor != nil && c.Actor.Resources != nil {
		for _, key := range []string{"coin", "gold", "copper", "silver", "credits"} {
			if _, ok := c.Actor.Resources[key]; ok {
				return key
			}
		}
		for k, r := range c.Actor.Resources {
			if r.Pile {
				return k
			}
		}
	}
	return "coin"
}

func resourceLabel(c *Context, key string) string {
	if c.Pack == nil {
		return key
	}
	return c.Pack.Lexicon.Label(key)
}

func isPurse(c *Context, key string) bool {
	if c.Pack == nil {
		return strings.EqualFold(key, "coin")
	}
	return strings.EqualFold(key, c.Pack.RPG.Advancement.Purse())
}

func canBank(c *Context, key string, allowStored bool) bool {
	if c.Actor == nil {
		return false
	}
	if r, ok := c.Actor.Resources[key]; ok && (r.Pile || isPurse(c, key)) {
		return true
	}
	if isPurse(c, key) {
		return true
	}
	return allowStored && c.Actor.BankBalance(key) > 0
}

func matchBankKey(c *Context, token string, allowStored bool) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" || c.Actor == nil {
		return "", false
	}
	named := func(key string) bool {
		return strings.EqualFold(key, token) || strings.EqualFold(resourceLabel(c, key), token)
	}
	if c.Actor.Resources != nil {
		for k, r := range c.Actor.Resources {
			if !r.Pile && !isPurse(c, k) {
				continue
			}
			if named(k) {
				return k, true
			}
		}
	}
	if allowStored && c.Actor.Bank != nil {
		for k, n := range c.Actor.Bank {
			if n <= 0 || !named(k) {
				continue
			}
			return k, true
		}
	}
	return "", false
}

func depositCmd(c *Context, p Parsed) {
	banker := bankerHere(c)
	if banker == nil {
		c.Print("There is no bank or vault here to conduct business.")
		return
	}
	cur := bankCurrency(c, banker)
	label := c.Pack.Lexicon.Label(cur)

	target := strings.TrimSpace(p.Rest)
	if target == "" {
		c.Print("Deposit how much %s?", label)
		return
	}

	fields := strings.Fields(target)
	all := strings.EqualFold(fields[0], "all")
	amount := 0
	if all {
		if len(fields) > 1 {
			k, ok := matchBankKey(c, fields[1], false)
			if !ok {
				c.Print("The vault only holds loose piles.")
				return
			}
			cur = k
		}
	} else {
		n, err := strconv.Atoi(fields[0])
		if err != nil || n <= 0 {
			c.Print("Deposit how much %s?", label)
			return
		}
		amount = n
		if len(fields) > 1 {
			k, ok := matchBankKey(c, fields[1], false)
			if !ok {
				c.Print("The vault only holds loose piles.")
				return
			}
			cur = k
		}
	}
	if !canBank(c, cur, false) {
		c.Print("The vault only holds loose piles.")
		return
	}
	label = resourceLabel(c, cur)
	if all {
		amount = c.Actor.Res(cur).Current
	}

	if amount <= 0 {
		c.Print("You don't have any %s to deposit.", label)
		return
	}
	have := c.Actor.Res(cur).Current
	if have < amount {
		c.Print("You only have %d %s (cannot deposit %d).", have, label, amount)
		return
	}

	c.Actor.AdjustRes(cur, -amount)
	bal := c.Actor.AdjustBank(cur, amount)
	c.Print("You deposit %d %s into your vault (vault balance: %d %s).", amount, label, bal, label)
}

func withdrawCmd(c *Context, p Parsed) {
	banker := bankerHere(c)
	if banker == nil {
		c.Print("There is no bank or vault here to conduct business.")
		return
	}
	cur := bankCurrency(c, banker)
	label := c.Pack.Lexicon.Label(cur)

	target := strings.TrimSpace(p.Rest)
	if target == "" {
		c.Print("Withdraw how much %s?", label)
		return
	}

	fields := strings.Fields(target)
	all := strings.EqualFold(fields[0], "all")
	amount := 0
	if all {
		if len(fields) > 1 {
			k, ok := matchBankKey(c, fields[1], true)
			if !ok {
				c.Print("The vault only holds loose piles.")
				return
			}
			cur = k
		}
	} else {
		n, err := strconv.Atoi(fields[0])
		if err != nil || n <= 0 {
			c.Print("Withdraw how much %s?", label)
			return
		}
		amount = n
		if len(fields) > 1 {
			k, ok := matchBankKey(c, fields[1], true)
			if !ok {
				c.Print("The vault only holds loose piles.")
				return
			}
			cur = k
		}
	}
	if !canBank(c, cur, true) {
		c.Print("The vault only holds loose piles.")
		return
	}
	label = resourceLabel(c, cur)
	if all {
		amount = c.Actor.BankBalance(cur)
	}

	if amount <= 0 {
		c.Print("You don't have any %s in your vault to withdraw.", label)
		return
	}
	inBank := c.Actor.BankBalance(cur)
	if inBank < amount {
		c.Print("You only have %d %s in your vault (cannot withdraw %d).", inBank, label, amount)
		return
	}

	before := c.Actor.Res(cur).Current
	c.Actor.AdjustBank(cur, -amount)
	after := c.Actor.AdjustRes(cur, amount)
	gained := after - before
	if gained < amount {
		c.Actor.AdjustBank(cur, amount-gained)
	}
	bal := c.Actor.BankBalance(cur)
	if gained < 1 {
		c.Print("You can't hold any more %s.", label)
		return
	}
	c.Print("You withdraw %d %s from your vault (vault balance: %d %s).", gained, label, bal, label)
}

func balanceCmd(c *Context, _ Parsed) {
	banker := bankerHere(c)
	if banker == nil {
		c.Print("There is no bank or vault here to conduct business.")
		return
	}
	if len(c.Actor.Bank) == 0 {
		c.Print("Your vault account is currently empty.")
		return
	}
	total := 0
	var lines []string
	for cur, amt := range c.Actor.Bank {
		if amt > 0 {
			lines = append(lines, fmt.Sprintf("  %d %s", amt, c.Pack.Lexicon.Label(cur)))
			total += amt
		}
	}
	if len(lines) == 0 {
		c.Print("Your vault account is currently empty.")
		return
	}
	c.Print("Vault Account Balance:")
	for _, l := range lines {
		c.Print("%s", l)
	}
}

func gatherCmd(c *Context, p Parsed) {
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		c.Print("You are nowhere.")
		return
	}
	var nodeEnt *world.Entity
	if p.Rest != "" {
		e, err := resolve(c, p.Rest, world.ScopeRoom)
		if err == nil && e.Gather != nil {
			nodeEnt = e
		}
	}
	if nodeEnt == nil {
		for _, e := range c.World.Children(room.ID) {
			if e.Gather != nil {
				nodeEnt = e
				break
			}
		}
	}
	if nodeEnt == nil && room.Gather != nil {
		nodeEnt = room
	}
	if nodeEnt == nil || nodeEnt.Gather == nil {
		c.Print("There is nothing here to gather or forage.")
		return
	}

	node := nodeEnt.Gather
	if node.Remaining <= 0 {
		msg := node.DepletedMsg
		if msg == "" {
			msg = fmt.Sprintf("The %s has been picked clean for now. Check back later.", nodeEnt.Display())
		}
		c.Print("%s", msg)
		return
	}

	skill := node.Skill
	if skill == "" {
		skill = "notice"
	}
	diff := node.Difficulty

	var res rpg.Result
	var ok bool
	if c.Pack != nil && len(c.Pack.RPG.Checks) > 0 {
		res, ok = c.Pack.RPG.Resolve(c.RNG, c.World, c.Actor, skill, diff, diff > 0)
	}

	if ok && !res.Success {
		c.Print("You search carefully around %s, but find nothing usable.", nodeEnt.Display())
		return
	}

	protoID := world.ID(node.Resource)
	if c.World.Protos[protoID] == nil {
		c.Print("You couldn't gather anything: there is no pattern for %s.", node.Resource)
		return
	}

	node.Remaining--
	first, err := c.World.Spawn(protoID, c.Actor.ID)
	if err != nil {
		node.Remaining++
		c.Print("You couldn't gather anything: %s", err.Error())
		return
	}

	itemName := first.Display()
	bonus := false
	if ok && node.BonusDC > 0 && res.Total >= node.BonusDC {
		if _, err := c.World.Spawn(protoID, c.Actor.ID); err == nil {
			bonus = true
		}
	}

	if bonus {
		c.Print("You gather %s from %s. Your keen perception spots an extra piece!", itemName, nodeEnt.Display())
	} else {
		c.Print("You gather %s from %s.", itemName, nodeEnt.Display())
	}
}

func questsCmd(c *Context, p Parsed) {
	if c.Pack == nil || len(c.Pack.Quests) == 0 {
		c.Print("There are no quests in this world.")
		return
	}
	if p.Rest != "" {
		q := pickQuest(c, p.Rest)
		if q == nil {
			return
		}
		showQuestDetails(c, q)
		return
	}

	var active, completed []quest.Quest
	for _, q := range c.Pack.Quests {
		if quest.IsActive(c.Actor, q) {
			active = append(active, q)
		} else if quest.IsCompleted(c.Actor, q) {
			completed = append(completed, q)
		}
	}

	if len(active) == 0 && len(completed) == 0 {
		c.Print("You have no active or completed quests.")
		c.Print("Talk to people in the world to find quests.")
		return
	}

	if len(active) > 0 {
		c.Print("Active Quests:")
		for _, q := range active {
			statusTag := ""
			if quest.IsReady(c.Actor, c.World, q) {
				statusTag = " [Ready for Turn-in!]"
			}
			c.Print("  %s (%s)%s", q.Title, q.ID, statusTag)
			for _, obj := range q.Objectives {
				cur, need, done := quest.ObjectiveProgress(c.Actor, c.World, q, obj)
				mark := ""
				if done {
					mark = " [Done]"
				}
				c.Print("    - %s (%d/%d)%s", obj.Desc, cur, need, mark)
			}
		}
	}

	if len(completed) > 0 {
		if len(active) > 0 {
			c.Print("%s", "")
		}
		c.Print("Completed Quests:")
		for _, q := range completed {
			c.Print("  - %s (%s)", q.Title, q.ID)
		}
	}
}

func showQuestDetails(c *Context, q *quest.Quest) {
	c.Print("%s (%s)", q.Title, q.ID)
	if q.Desc != "" {
		c.Print("  %s", q.Desc)
	}
	c.Print("Objectives:")
	for _, obj := range q.Objectives {
		cur, need, done := quest.ObjectiveProgress(c.Actor, c.World, *q, obj)
		mark := ""
		if done {
			mark = " [Done]"
		}
		c.Print("  - %s (%d/%d)%s", obj.Desc, cur, need, mark)
	}
	var rewParts []string
	if q.Rewards.XP > 0 {
		rewParts = append(rewParts, fmt.Sprintf("%d XP", q.Rewards.XP))
	}
	if q.Rewards.Amount > 0 {
		curName := q.Rewards.Currency
		if curName == "" {
			curName = "coin"
		}
		rewParts = append(rewParts, fmt.Sprintf("%d %s", q.Rewards.Amount, c.Pack.Lexicon.Label(curName)))
	}
	for _, it := range q.Rewards.Items {
		itemProto := c.World.Protos[world.ID(it)]
		name := it
		if itemProto != nil {
			name = itemProto.Display()
		}
		rewParts = append(rewParts, name)
	}
	for _, rec := range q.Rewards.Recipes {
		recName := rec
		for _, r := range c.Pack.Recipes {
			if strings.EqualFold(r.ID, rec) {
				recName = r.Name
				break
			}
		}
		rewParts = append(rewParts, fmt.Sprintf("%s recipe", recName))
	}
	if len(rewParts) > 0 {
		c.Print("Rewards: %s", strings.Join(rewParts, ", "))
	}
	if quest.IsCompleted(c.Actor, *q) {
		c.Print("Status: Completed")
	} else if quest.IsActive(c.Actor, *q) {
		if quest.IsReady(c.Actor, c.World, *q) {
			c.Print("Status: Ready to Turn-in (Type: quest complete %s)", q.ID)
		} else {
			c.Print("Status: Active")
		}
	} else if quest.IsAvailable(c.Actor, *q, c.Pack.Quests) {
		c.Print("Status: Available (Type: quest accept %s)", q.ID)
	} else {
		c.Print("Status: Locked")
	}
}

func questCmd(c *Context, p Parsed) {
	if c.Pack == nil || len(c.Pack.Quests) == 0 {
		c.Print("There are no quests in this world.")
		return
	}
	if len(p.Args) == 0 {
		questsCmd(c, p)
		return
	}

	sub := strings.ToLower(p.Args[0])
	switch sub {
	case "accept", "take", "start":
		if len(p.Args) < 2 {
			c.Print("Accept which quest? Type: quest accept <id|name>")
			return
		}
		target := strings.TrimSpace(p.Rest[len(p.Args[0]):])
		q := pickQuest(c, target)
		if q == nil {
			return
		}
		if quest.IsActive(c.Actor, *q) {
			c.Print("You are already on '%s'.", q.Title)
			return
		}
		if quest.IsCompleted(c.Actor, *q) {
			c.Print("You have already completed '%s'.", q.Title)
			return
		}
		if !quest.IsAvailable(c.Actor, *q, c.Pack.Quests) {
			c.Print("You do not meet the requirements to start '%s'.", q.Title)
			return
		}
		if q.Giver != "" {
			giver := npcInRoom(c, q.Giver)
			if giver == nil {
				c.Print("The quest giver is not here.")
				return
			}
		}
		if quest.Start(c.Actor, *q) {
			c.Print("You accepted quest: %s.", q.Title)
			if q.Desc != "" {
				c.Print("%s", q.Desc)
			}
			room := c.World.RoomOf(c.Actor)
			if room != nil {
				notices := quest.UpdateVisit(c.Actor, room.ID, []quest.Quest{*q}, c.World)
				for _, n := range notices {
					c.Alert("%s", n.String())
				}
			}
		}
	case "complete", "finish", "turnin", "turn-in":
		if len(p.Args) < 2 {
			c.Print("Complete which quest? Type: quest complete <id|name>")
			return
		}
		target := strings.TrimSpace(p.Rest[len(p.Args[0]):])
		q := pickQuest(c, target)
		if q == nil {
			return
		}
		if !quest.IsActive(c.Actor, *q) {
			if quest.IsCompleted(c.Actor, *q) {
				c.Print("You have already completed '%s'.", q.Title)
			} else {
				c.Print("You are not on that quest.")
			}
			return
		}
		turnInID := q.TurnIn
		if turnInID == "" {
			turnInID = q.Giver
		}
		var turnInNPC *world.Entity
		if turnInID != "" {
			turnInNPC = npcInRoom(c, turnInID)
			if turnInNPC == nil {
				c.Print("The turn-in contact is not here.")
				return
			}
		}
		if !quest.IsReady(c.Actor, c.World, *q) {
			c.Print("You haven't finished all the objectives for '%s' yet.", q.Title)
			return
		}
		if err := quest.Complete(c.Actor, c.World, *q, c.Tick); err != nil {
			c.Print("Cannot complete quest: %s", err.Error())
			return
		}
		c.Print("Quest Completed: %s!", q.Title)
		if q.Rewards.Message != "" {
			c.Print("%s", q.Rewards.Message)
		}
		runQuestScripts(c, q, turnInNPC)
		prefer := world.ID("")
		if room := c.World.RoomOf(c.Actor); room != nil {
			prefer = room.ID
		}
		for _, mv := range quest.ApplyRelocations(c.World, prefer, q.Rewards.Relocate) {
			if mv.OK {
				if mv.Message != "" {
					c.Print("%s", mv.Message)
				}
				arriveWithLeader(c, mv)
				continue
			}
			if mv.Reason != "" {
				c.Print("%s", mv.Reason)
			}
		}
		var rewardsList []string
		if q.Rewards.XP > 0 {
			rewardsList = append(rewardsList, fmt.Sprintf("%d XP", q.Rewards.XP))
			if c.Pack.RPG.Advancement.On() {
				msgs := c.Pack.RPG.Grant(c.Actor, q.Rewards.XP)
				for _, msg := range msgs {
					c.Alert("%s", msg)
				}
			} else {
				c.Actor.XP += q.Rewards.XP
			}
		}
		if q.Rewards.Amount > 0 {
			cur := q.Rewards.Currency
			if cur == "" {
				cur = "coin"
			}
			rewardsList = append(rewardsList, fmt.Sprintf("%d %s", q.Rewards.Amount, c.Pack.Lexicon.Label(cur)))
		}
		for _, it := range q.Rewards.Items {
			p := c.World.Protos[world.ID(it)]
			name := it
			if p != nil {
				name = p.Display()
			}
			rewardsList = append(rewardsList, name)
		}
		for _, rec := range q.Rewards.Recipes {
			recName := rec
			for _, r := range c.Pack.Recipes {
				if strings.EqualFold(r.ID, rec) {
					recName = r.Name
					break
				}
			}
			rewardsList = append(rewardsList, fmt.Sprintf("%s recipe", recName))
		}
		if len(rewardsList) > 0 {
			c.Print("Rewards: %s", strings.Join(rewardsList, ", "))
		}
		checkAutoStarts(c)
	default:
		matches := quest.MatchQuests(c.Pack.Quests, p.Rest)
		switch len(matches) {
		case 1:
			showQuestDetails(c, matches[0])
		default:
			if len(matches) > 1 {
				names := make([]string, 0, len(matches))
				for _, q := range matches {
					names = append(names, q.Title)
				}
				c.Print("Which quest? %s.", strings.Join(names, ", "))
				return
			}
			c.Print("Unknown quest command: '%s'. Try 'quest accept <id>', 'quest complete <id>', or 'quests'.", sub)
		}
	}
}

func runQuestScripts(c *Context, q *quest.Quest, turnIn *world.Entity) {
	if c == nil || c.Scripts == nil || q == nil {
		return
	}
	self := turnIn
	if self == nil || self.Scripts == "" {
		self = giverWithScript(c, q.Giver)
	}
	if self != nil && self.Scripts != "" {
		if _, err := c.Scripts.Call("on_quest_complete", c.Actor, self, q.ID); err != nil {
			c.Print("[Script error: %v]", err)
		}
	}
	if q.Script != "" {
		if _, err := c.Scripts.CallFile(q.Script, "on_quest_complete", c.Actor, turnIn, q.ID); err != nil {
			c.Print("[Script error: %v]", err)
		}
	}
}

func giverWithScript(c *Context, giverID string) *world.Entity {
	if c == nil || c.World == nil || giverID == "" {
		return nil
	}
	prefer := world.ID("")
	if room := c.World.RoomOf(c.Actor); room != nil {
		prefer = room.ID
	}
	giver, err := c.World.ResolveMobile(giverID, prefer)
	if err != nil {
		if errors.Is(err, world.ErrAmbiguous) {
			c.Print("More than one %s matches; the quest script did not run.", giverID)
		}
		return nil
	}
	if giver == nil || giver.Scripts == "" {
		return nil
	}
	return giver
}

func pickQuest(c *Context, query string) *quest.Quest {
	matches := quest.MatchQuests(c.Pack.Quests, query)
	switch len(matches) {
	case 0:
		c.Print("No such quest: %q.", strings.TrimSpace(query))
		return nil
	case 1:
		return matches[0]
	default:
		names := make([]string, 0, len(matches))
		for _, q := range matches {
			names = append(names, q.Title)
		}
		c.Print("Which quest? %s.", strings.Join(names, ", "))
		return nil
	}
}

func checkAutoStarts(c *Context) {
	if c == nil || c.Pack == nil || c.Actor == nil {
		return
	}
	for _, q := range c.Pack.Quests {
		if q.AutoStart && quest.IsAvailable(c.Actor, q, c.Pack.Quests) {
			if quest.Start(c.Actor, q) {
				c.Alert("[New Quest Started: %s]", q.Title)
				if q.Desc != "" {
					c.Print("%s", q.Desc)
				}
			}
		}
	}
}

func npcInRoom(c *Context, npcID string) *world.Entity {
	if c == nil || c.Actor == nil || c.World == nil || npcID == "" {
		return nil
	}
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		return nil
	}
	for _, kid := range c.World.Children(room.ID) {
		if kid.Kind != world.KindMobile && kid.Kind != world.KindPlayer {
			continue
		}
		if strings.EqualFold(string(kid.ID), npcID) || strings.EqualFold(string(kid.PrototypeID), npcID) || kid.HasKeyword(strings.ToLower(npcID)) {
			return kid
		}
	}
	return nil
}

func isCorpse(e *world.Entity) bool {
	if e == nil {
		return false
	}
	id := string(e.ID)
	return e.HasKeyword("corpse") || strings.HasPrefix(id, "corpse-") || strings.HasPrefix(id, "corpse#")
}

func findKitInInventory(c *Context) *world.Entity {
	if c.Actor == nil || c.World == nil {
		return nil
	}
	for _, id := range c.Actor.Contents {
		it := c.World.Get(id)
		if it != nil && it.RepairKit != nil && it.RepairKit.Uses > 0 {
			return it
		}
	}
	return nil
}

func repairWithKit(c *Context, it *world.Entity, kit *world.Entity) {
	if it == nil || kit == nil {
		return
	}
	if it.MaxDurability <= 0 || !it.Repairable {
		c.Print("%s cannot be repaired.", it.Display())
		return
	}
	if it.Durability >= it.MaxDurability && !it.HasFlag("broken") {
		c.Print("%s is already in pristine condition.", it.Display())
		return
	}
	if kit.RepairKit == nil || kit.RepairKit.Uses <= 0 {
		c.Print("%s has no uses remaining.", kit.Display())
		return
	}

	if kit.RepairKit.Skill != "" {
		if c.Pack == nil {
			c.Print("This repair needs a %s check, and this game has no such check.", kit.RepairKit.Skill)
			return
		}
		dc := kit.RepairKit.DC
		if dc <= 0 {
			dc = 7
		}
		res, ok := c.Pack.RPG.Resolve(c.RNG, c.World, c.Actor, kit.RepairKit.Skill, dc, true)
		if !ok {
			c.Print("This repair needs a %s check, and this game has no such check.", kit.RepairKit.Skill)
			return
		}
		if !res.Success {
			kit.RepairKit.Uses--
			c.Print("You struggle with %s and fail to mend %s properly. One use was wasted (%d left).", kit.Display(), it.Display(), kit.RepairKit.Uses)
			if kit.RepairKit.Uses <= 0 {
				c.Print("Your %s is used up.", kit.Display())
				c.World.Destroy(kit.ID)
			}
			return
		}
	}

	amount := kit.RepairKit.Amount
	if amount <= 0 {
		it.Durability = it.MaxDurability
	} else {
		it.Durability += amount
		if it.Durability > it.MaxDurability {
			it.Durability = it.MaxDurability
		}
	}
	it.SetFlag("broken", false)
	kit.RepairKit.Uses--
	c.Print("You use %s to repair %s (%d/%d durability).", kit.Display(), it.Display(), it.Durability, it.MaxDurability)
	if kit.RepairKit.Uses <= 0 {
		c.Print("Your %s is used up.", kit.Display())
		c.World.Destroy(kit.ID)
	}
}

func skinCmd(c *Context, p Parsed) {
	target := strings.TrimSpace(p.Rest)
	var corpse *world.Entity
	if target == "" || strings.EqualFold(target, "corpse") {
		room := c.World.RoomOf(c.Actor)
		if room != nil {
			for _, kid := range c.World.Children(room.ID) {
				if isCorpse(kid) {
					corpse = kid
					break
				}
			}
		}
		if corpse == nil {
			for _, cid := range c.Actor.Contents {
				it := c.World.Get(cid)
				if isCorpse(it) {
					corpse = it
					break
				}
			}
		}
		if corpse == nil {
			c.Print("There is no corpse here to skin.")
			return
		}
	} else {
		var err error
		corpse, err = resolve(c, target, world.ScopeRoom, world.ScopeInventory)
		if err != nil {
			c.Print("%s", err.Error())
			return
		}
	}

	if !isCorpse(corpse) {
		c.Print("You can only skin or butcher corpses.")
		return
	}
	if corpse.HasFlag("skinned") {
		c.Print("%s has already been butchered.", corpse.CapDisplay())
		return
	}

	var skinLoot []world.LootDrop
	for _, loot := range corpse.Loot {
		if !loot.SkinOnly {
			continue
		}
		if c.World.Protos[world.ID(loot.Item)] == nil {
			c.Print("You can't harvest %s: there is no pattern for %s.", corpse.Display(), loot.Item)
			return
		}
		skinLoot = append(skinLoot, loot)
	}

	corpse.SetFlag("skinned", true)
	if !strings.Contains(corpse.Short, "(butchered)") && !strings.Contains(corpse.Short, "(skinned)") {
		corpse.Short = corpse.Short + " (butchered)"
	}

	var harvested []string
	if len(skinLoot) > 0 {
		for _, loot := range skinLoot {
			chance := loot.Chance
			if chance <= 0 {
				chance = 100
			}
			if c.RNG != nil && c.RNG.Intn(100) >= chance {
				continue
			}
			count := loot.Count
			if count <= 0 {
				count = 1
			}
			for i := 0; i < count; i++ {
				sp, err := c.World.Spawn(world.ID(loot.Item), c.Actor.ID)
				if err == nil && sp != nil {
					harvested = append(harvested, sp.Display())
					if c.Pack != nil {
						for _, q := range c.Pack.Quests {
							if !quest.IsActive(c.Actor, q) {
								continue
							}
							for _, obj := range q.Objectives {
								if obj.Kind == quest.KindCollect {
									match := strings.EqualFold(string(sp.PrototypeID), obj.Target) ||
										sp.HasKeyword(strings.ToLower(obj.Target)) ||
										strings.EqualFold(string(sp.ID), obj.Target)
									if match {
										cur, need, done := quest.ObjectiveProgress(c.Actor, c.World, q, obj)
										if done && quest.IsReady(c.Actor, c.World, q) {
											c.Alert("[Quest Ready for Turn-in: %s]", q.Title)
										} else if done {
											c.Alert("[Objective Complete: %s (%d/%d)]", obj.Desc, cur, need)
										} else {
											c.Alert("[Objective Progress: %s (%d/%d)]", obj.Desc, cur, need)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if len(harvested) > 0 {
		c.Print("You butcher %s and collect: %s.", corpse.Display(), strings.Join(harvested, ", "))
	} else {
		c.Print("You butcher %s, but find nothing usable.", corpse.Display())
	}
}

func salvageCmd(c *Context, p Parsed) {
	if p.Rest == "" {
		c.Print("Salvage what? Type: salvage <item>")
		return
	}
	it, err := resolve(c, p.Rest, world.ScopeInventory)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if len(it.Salvage) == 0 {
		c.Print("You can't salvage anything useful from %s.", it.Display())
		return
	}

	for _, sy := range it.Salvage {
		if c.World.Protos[world.ID(sy.Item)] == nil {
			c.Print("You can't salvage %s: there is no pattern for %s.", it.Display(), sy.Item)
			return
		}
	}

	unequipIfWorn(c, it)
	itName := it.Display()
	salvageYields := append([]world.SalvageYield{}, it.Salvage...)
	c.World.Destroy(it.ID)

	var obtained []string
	for _, sy := range salvageYields {
		count := sy.Count
		if count <= 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			sp, err := c.World.Spawn(world.ID(sy.Item), c.Actor.ID)
			if err == nil && sp != nil {
				obtained = append(obtained, sp.Display())
			}
		}
	}

	if len(obtained) > 0 {
		c.Print("You carefully break down %s into: %s.", itName, strings.Join(obtained, ", "))
	} else {
		c.Print("You break down %s, but nothing was recovered.", itName)
	}
}

func moveFollowers(c *Context, leader, old, dest *world.Entity, dir string) {
	if c == nil || c.World == nil || leader == nil || old == nil || dest == nil {
		return
	}
	for _, hop := range c.World.PullFollowers(leader, old, dest) {
		f := hop.Follower
		if f == nil {
			continue
		}
		if c.Actor != nil && f.ID == c.Actor.ID {
			c.Tell(protocol.ChanNarrative, FollowYou(hop.Leader, dir))
			Arrive(c, old, dest)
			continue
		}
		if c.Actor != nil && hop.Leader != nil && hop.Leader.ID == c.Actor.ID {
			c.Print("%s", FollowsYou(f, dir))
		}
		if c.Scripts != nil {
			_, _ = c.Scripts.Call("on_leave", f, old)
			_, _ = c.Scripts.Call("on_enter", f, dest)
		}
	}
}

func FollowYou(leader *world.Entity, dir string) string {
	name := "them"
	if leader != nil {
		name = leader.Display()
	}
	if dir == "" {
		return fmt.Sprintf("You follow %s.", name)
	}
	return fmt.Sprintf("You follow %s %s.", name, dir)
}

func FollowsYou(f *world.Entity, dir string) string {
	name := "Someone"
	if f != nil {
		name = f.Display()
	}
	if dir == "" {
		return fmt.Sprintf("%s follows you.", name)
	}
	return fmt.Sprintf("%s follows you %s.", name, dir)
}

func arriveWithLeader(c *Context, mv quest.MoveResult) {
	if c == nil || c.World == nil || len(mv.Pulled) == 0 {
		return
	}
	old := c.World.Get(world.ID(mv.FromID))
	dest := c.World.Get(world.ID(mv.DestID))
	for _, id := range mv.Pulled {
		f := c.World.Get(world.ID(id))
		if f == nil {
			continue
		}
		if c.Actor != nil && f.ID == c.Actor.ID {
			c.Print("You follow along.")
			Arrive(c, old, dest)
			continue
		}
		if c.Scripts != nil && old != nil && dest != nil {
			_, _ = c.Scripts.Call("on_leave", f, old)
			_, _ = c.Scripts.Call("on_enter", f, dest)
		}
		c.Print("%s follows along.", f.CapDisplay())
	}
}

func followCmd(c *Context, p Parsed) {
	if len(p.Args) == 0 {
		if c.Actor.Leader != "" {
			leader := c.World.Get(c.Actor.Leader)
			if leader != nil {
				c.Print("You are following %s.", leader.Display())
			} else {
				c.World.StopFollowing(c.Actor)
				c.Print("You are not following anyone.")
			}
		} else {
			c.Print("You are not following anyone.")
		}

		if len(c.Actor.Followers) > 0 {
			var names []string
			for _, fID := range c.Actor.Followers {
				f := c.World.Get(fID)
				if f != nil {
					names = append(names, f.Display())
				}
			}
			if len(names) > 0 {
				c.Print("Following you: %s.", strings.Join(names, ", "))
			} else {
				c.Print("No one is following you.")
			}
		} else {
			c.Print("No one is following you.")
		}
		return
	}

	targetName := strings.ToLower(strings.TrimSpace(p.Rest))
	if targetName == "self" || targetName == "none" || targetName == "stop" {
		if c.Actor.Leader == "" {
			c.Print("You are not following anyone.")
			return
		}
		oldLeader := c.World.Get(c.Actor.Leader)
		c.World.StopFollowing(c.Actor)
		if oldLeader != nil {
			c.Print("You stop following %s.", oldLeader.Display())
		} else {
			c.Print("You stop following your leader.")
		}
		return
	}

	room := c.World.RoomOf(c.Actor)
	if room == nil {
		c.Print("You can't follow anyone from nowhere.")
		return
	}

	target, _ := c.World.Match(c.Actor, targetName, world.ScopeRoom)
	if target == nil {
		c.Print("You don't see %q here.", targetName)
		return
	}
	if target.ID == c.Actor.ID {
		c.Print("You cannot follow yourself.")
		return
	}
	if target.Kind != world.KindMobile && target.Kind != world.KindPlayer {
		c.Print("You cannot follow %s.", target.Display())
		return
	}
	if target.AI != nil && target.AI.Profile == "aggressive" && target.AI.Awake(target) {
		c.Print("%s won't let you follow them!", target.Display())
		return
	}
	if target.IsFollowing(c.Actor.ID, c.World) {
		c.Print("%s is already following you! You cannot form a loop.", target.Display())
		return
	}

	if err := c.World.StartFollowing(c.Actor, target); err != nil {
		c.Print("Cannot follow: %s.", err.Error())
		return
	}
	c.Print("You begin following %s.", target.Display())
}

func unfollowCmd(c *Context, p Parsed) {
	if len(p.Args) == 0 {
		if c.Actor.Leader == "" {
			c.Print("You are not following anyone.")
			return
		}
		oldLeader := c.World.Get(c.Actor.Leader)
		c.World.StopFollowing(c.Actor)
		if oldLeader != nil {
			c.Print("You stop following %s.", oldLeader.Display())
		} else {
			c.Print("You stop following your leader.")
		}
		return
	}
	loseCmd(c, p)
}

func loseCmd(c *Context, p Parsed) {
	if len(p.Args) == 0 {
		c.Print("Lose whom? Type 'lose <target>' or 'lose all'.")
		return
	}
	targetName := strings.ToLower(strings.TrimSpace(p.Rest))
	if targetName == "all" {
		if len(c.Actor.Followers) == 0 {
			c.Print("No one is following you.")
			return
		}
		for _, fID := range append([]world.ID{}, c.Actor.Followers...) {
			f := c.World.Get(fID)
			if f != nil {
				c.World.StopFollowing(f)
			}
		}
		c.Print("You shake off all your followers.")
		return
	}

	var found *world.Entity
	for _, fID := range c.Actor.Followers {
		f := c.World.Get(fID)
		if f != nil && (strings.EqualFold(string(f.ID), targetName) || strings.EqualFold(string(f.PrototypeID), targetName) || f.HasKeyword(targetName)) {
			found = f
			break
		}
	}
	if found == nil {
		c.Print("No follower named %q is following you.", targetName)
		return
	}
	c.World.StopFollowing(found)
	c.Print("You shake off %s.", found.Display())
}

func groupCmd(c *Context, p Parsed) {
	sub := ""
	if len(p.Args) > 0 {
		sub = strings.ToLower(p.Args[0])
	}
	switch sub {
	case "invite":
		if len(p.Args) < 2 {
			c.Print("Invite whom? Type: group invite <name>")
			return
		}
		inviteTarget(c, strings.TrimSpace(p.Rest[len(p.Args[0]):]))
	case "join":
		if len(p.Args) < 2 {
			c.Print("Join whose party? Type: group join <name>")
			return
		}
		joinTarget(c, strings.TrimSpace(p.Rest[len(p.Args[0]):]))
	case "leave":
		leaveParty(c)
	case "disband", "disown":
		disbandParty(c)
	default:
		showParty(c)
	}
}

func showParty(c *Context) {
	leaderID := c.Actor.PartyLeader
	if leaderID == "" {
		leaderID = c.Actor.ID
	}
	leader := c.World.Get(leaderID)
	if leader == nil {
		leader = c.Actor
	}

	members := c.World.PartyMembers(leaderID)
	if len(members) == 0 {
		members = []*world.Entity{c.Actor}
	}

	c.Print("Party of %s:", leader.Display())
	for _, m := range members {
		roleBadge := ""
		if m.ID == leaderID {
			roleBadge = " [Leader]"
		}
		resStr := ""
		if c.Pack != nil && len(m.Resources) > 0 {
			key := c.Pack.RPG.Primary()
			if r, ok := m.Resources[key]; ok {
				resStr = fmt.Sprintf(" | %s: %d/%d", c.Pack.Lexicon.Label(key), r.Current, r.Max)
			}
		}
		stanceStr := ""
		if m.Stance != "" {
			stanceStr = fmt.Sprintf(" [%s]", m.Stance)
		}
		loyaltyStr := ""
		if m.Companion || m.Owner != "" || m.Loyalty > 0 {
			maxL := m.MaxLoyalty
			if maxL <= 0 {
				maxL = 100
			}
			loyaltyStr = fmt.Sprintf(" (Loyalty: %d/%d)", m.Loyalty, maxL)
		}
		c.Print("  %s (Lvl %d)%s%s%s%s", m.Display(), m.Level, roleBadge, resStr, stanceStr, loyaltyStr)
	}

	var extraFollowers []*world.Entity
	for _, fID := range c.Actor.Followers {
		f := c.World.Get(fID)
		if f != nil && f.PartyLeader != leaderID {
			extraFollowers = append(extraFollowers, f)
		}
	}
	if len(extraFollowers) > 0 {
		c.Print("Followers:")
		for _, f := range extraFollowers {
			stance := "assist"
			if f.Stance != "" {
				stance = f.Stance
			}
			loyaltyStr := ""
			if f.Companion || f.Owner != "" || f.Loyalty > 0 {
				maxL := f.MaxLoyalty
				if maxL <= 0 {
					maxL = 100
				}
				loyaltyStr = fmt.Sprintf(" (Loyalty: %d/%d)", f.Loyalty, maxL)
			}
			c.Print("  (Following) %s [%s]%s", f.Display(), stance, loyaltyStr)
		}
	}
}

func inviteCmd(c *Context, p Parsed) {
	if len(p.Args) == 0 {
		c.Print("Invite whom? Type: invite <name>")
		return
	}
	inviteTarget(c, p.Rest)
}

func inviteTarget(c *Context, name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		c.Print("Invite whom?")
		return
	}
	target, _ := c.World.Match(c.Actor, name, world.ScopeRoom)
	if target == nil {
		c.Print("You don't see %q here.", name)
		return
	}
	if target.ID == c.Actor.ID {
		c.Print("You cannot invite yourself.")
		return
	}
	if target.Kind != world.KindMobile && target.Kind != world.KindPlayer {
		c.Print("You cannot invite %s.", target.Display())
		return
	}

	leaderID := c.Actor.PartyLeader
	if leaderID != "" && leaderID != c.Actor.ID {
		c.Print("Only the party leader can invite new members.")
		return
	}
	madeLeader := false
	if leaderID == "" {
		c.Actor.PartyLeader = c.Actor.ID
		leaderID = c.Actor.ID
		madeLeader = true
	}

	if target.PartyLeader == leaderID {
		c.Print("%s is already in your party.", target.Display())
		return
	}

	if target.Companion || target.Owner == c.Actor.ID {
		if err := c.World.StartFollowing(target, c.Actor); err != nil {
			if madeLeader {
				c.Actor.PartyLeader = ""
			}
			c.Print("%s", err.Error())
			return
		}
		target.PartyLeader = leaderID
		c.Print("%s joins your party!", target.Display())
		return
	}

	if c.Scripts != nil {
		handled, _ := c.Scripts.Call("on_invite", c.Actor, target)
		if handled {
			return
		}
	}

	if target.HasFlag("party_member") || target.HasFlag("companion") || target.HasFlag("follower") {
		if err := c.World.StartFollowing(target, c.Actor); err != nil {
			if madeLeader {
				c.Actor.PartyLeader = ""
			}
			c.Print("%s", err.Error())
			return
		}
		target.PartyLeader = leaderID
		c.Print("%s accepts your invitation and joins your party!", target.Display())
		return
	}

	if target.Kind == world.KindMobile {
		c.Print("%s declines to join your party.", target.Display())
		return
	}

	target.PartyInvites = append(target.PartyInvites, c.Actor.ID)
	c.Print("You invite %s to join your party.", target.Display())
}

func joinCmd(c *Context, p Parsed) {
	if len(p.Args) == 0 {
		c.Print("Join whose party? Type: join <name>")
		return
	}
	joinTarget(c, p.Rest)
}

func joinTarget(c *Context, name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	target, _ := c.World.Match(c.Actor, name, world.ScopeRoom)
	if target == nil {
		c.Print("You don't see %q here.", name)
		return
	}

	invited := false
	targetLeader := target.PartyLeader
	if targetLeader == "" {
		targetLeader = target.ID
	}
	for i, inv := range c.Actor.PartyInvites {
		if inv == target.ID || inv == targetLeader {
			invited = true
			c.Actor.PartyInvites = append(c.Actor.PartyInvites[:i], c.Actor.PartyInvites[i+1:]...)
			break
		}
	}
	if !invited && (target.Companion || target.Owner == c.Actor.ID || target.HasFlag("open_party") || target.HasFlag("party_leader")) {
		invited = true
	}

	if !invited && c.Scripts != nil {
		handled, _ := c.Scripts.Call("on_join", c.Actor, target)
		if handled {
			return
		}
	}

	if !invited {
		c.Print("You do not have an invite to join %s's party.", target.Display())
		return
	}

	releaseOwnParty(c)

	if target.PartyLeader == "" {
		target.PartyLeader = target.ID
		targetLeader = target.ID
	}
	c.Actor.PartyLeader = targetLeader
	_ = c.World.StartFollowing(c.Actor, target)
	c.Print("You have joined %s's party!", target.Display())
}

func releaseOwnParty(c *Context) {
	if c == nil || c.Actor == nil || c.Actor.PartyLeader == "" {
		return
	}
	if c.Actor.PartyLeader != c.Actor.ID {
		leaveParty(c)
		return
	}
	others := false
	for _, m := range c.World.PartyMembers(c.Actor.ID) {
		if m != nil && m.ID != c.Actor.ID {
			others = true
			break
		}
	}
	if others {
		disbandParty(c)
		return
	}
	c.Actor.PartyLeader = ""
}

func leaveParty(c *Context) {
	if c.Actor.PartyLeader == "" {
		c.Print("You are not in a party.")
		return
	}
	if c.Actor.PartyLeader == c.Actor.ID {
		disbandParty(c)
		return
	}
	oldLeader := c.World.Get(c.Actor.PartyLeader)
	c.Actor.PartyLeader = ""
	c.World.StopFollowing(c.Actor)
	if oldLeader != nil {
		c.Print("You leave %s's party.", oldLeader.Display())
	} else {
		c.Print("You leave the party.")
	}
}

func disbandParty(c *Context) {
	if c.Actor.PartyLeader == "" {
		c.Print("You are not in a party.")
		return
	}
	if c.Actor.PartyLeader != c.Actor.ID {
		c.Print("Only the party leader can disband the party.")
		return
	}
	c.World.ClearParty(c.Actor.ID)
	c.Print("You disband the party.")
}

func isFood(item *world.Entity) bool {
	if item == nil {
		return false
	}
	if item.HasFlag("food") || item.HasFlag("edible") {
		return true
	}
	for _, t := range item.Tags {
		if strings.EqualFold(t, "food") || strings.EqualFold(t, "ration") || strings.EqualFold(t, "meat") || strings.EqualFold(t, "bread") {
			return true
		}
	}
	for _, kw := range []string{"food", "ration", "meat", "bread", "apple", "fish", "treat", "berry", "berries", "herb", "grain", "bone", "cheese"} {
		for _, k := range item.Keywords {
			if strings.EqualFold(k, kw) {
				return true
			}
		}
	}
	return false
}

func takesOrders(actor, pet *world.Entity) bool {
	if actor == nil || pet == nil || pet.Kind != world.KindMobile {
		return false
	}
	if pet.Owner == actor.ID {
		return true
	}
	return pet.Companion && pet.Owner == "" && pet.Leader == actor.ID
}

func rememberStay(pet *world.Entity) {
	if pet == nil {
		return
	}
	if !strings.EqualFold(pet.Stance, "stay") {
		pet.RestStance = pet.Stance
		if pet.RestStance == "" {
			pet.RestStance = "assist"
		}
	}
	pet.Stance = "stay"
}

func resumeFollowStance(pet *world.Entity) {
	if pet == nil || !strings.EqualFold(pet.Stance, "stay") {
		return
	}
	next := pet.RestStance
	if next == "" || strings.EqualFold(next, "stay") {
		next = "assist"
	}
	pet.Stance = next
	pet.RestStance = ""
}

func orderCmd(c *Context, p Parsed) {
	fields := strings.Fields(p.Rest)
	if len(fields) < 2 {
		c.Print("Order whom to do what? Syntax: order <pet> <follow|stay|stance <assist|defend|passive>|attack <target>|dismiss>")
		return
	}

	orderVerbs := map[string]bool{
		"follow": true, "stay": true, "stance": true,
		"assist": true, "defend": true, "passive": true,
		"attack": true, "kill": true, "dismiss": true,
		"to": true,
	}

	var pet *world.Entity
	actionIdx := 1
	for i := len(fields) - 1; i >= 1; i-- {
		wLow := strings.ToLower(fields[i])
		if orderVerbs[wLow] {
			candidate := strings.Join(fields[:i], " ")
			if m, err := resolve(c, candidate, world.ScopeRoom); err == nil && m.Kind == world.KindMobile {
				pet = m
				actionIdx = i
				break
			}
		}
	}
	if pet == nil {
		if m, err := resolve(c, fields[0], world.ScopeRoom); err == nil && m.Kind == world.KindMobile {
			pet = m
			actionIdx = 1
		} else {
			c.Print("You don't see %q here.", fields[0])
			return
		}
	}

	if !takesOrders(c.Actor, pet) {
		c.Print("%s does not take orders from you.", pet.CapDisplay())
		return
	}

	if pet.MaxLoyalty > 0 && pet.Loyalty < 20 {
		r := 0
		if c.RNG != nil {
			r = c.RNG.Intn(100)
		} else {
			r = rand.Intn(100)
		}
		if r < 50 {
			c.Print("%s growls stubbornly and ignores your command. Its loyalty is too low (%d/%d).", pet.CapDisplay(), pet.Loyalty, pet.MaxLoyalty)
			return
		}
	}

	actionTokens := fields[actionIdx:]
	if len(actionTokens) > 0 && strings.EqualFold(actionTokens[0], "to") {
		actionTokens = actionTokens[1:]
	}
	if len(actionTokens) == 0 {
		c.Print("Order %s to do what?", pet.Display())
		return
	}

	act := strings.ToLower(actionTokens[0])
	switch act {
	case "follow":
		resumeFollowStance(pet)
		if err := c.World.StartFollowing(pet, c.Actor); err != nil {
			c.Print("%s cannot follow you: %v", pet.CapDisplay(), err)
			return
		}
		c.Print("%s begins following you.", pet.CapDisplay())

	case "stay":
		c.World.StopFollowing(pet)
		rememberStay(pet)
		c.Print("%s stays put.", pet.CapDisplay())

	case "stance":
		if len(actionTokens) < 2 {
			st := pet.Stance
			if st == "" {
				st = "assist"
			}
			c.Print("%s's stance is currently '%s'. Options: assist, defend, passive, stay.", pet.CapDisplay(), st)
			return
		}
		mode := strings.ToLower(actionTokens[1])
		switch mode {
		case "assist", "defend", "passive":
			pet.Stance = mode
			c.Print("%s's stance is now %s.", pet.CapDisplay(), mode)
		case "stay":
			c.World.StopFollowing(pet)
			rememberStay(pet)
			c.Print("%s stays put (stance: stay).", pet.CapDisplay())
		default:
			c.Print("Unknown stance %q. Options: assist, defend, passive, stay.", mode)
		}

	case "assist", "defend", "passive":
		pet.Stance = act
		c.Print("%s's stance is now %s.", pet.CapDisplay(), act)

	case "attack", "kill":
		if len(actionTokens) < 2 {
			c.Print("Order %s to attack whom?", pet.Display())
			return
		}
		targetName := strings.Join(actionTokens[1:], " ")
		target, err := resolve(c, targetName, world.ScopeRoom)
		if err != nil {
			c.Print("%s", err.Error())
			return
		}
		if target.Kind != world.KindMobile && target.Kind != world.KindPlayer {
			c.Print("%s cannot fight %s.", pet.CapDisplay(), target.Display())
			return
		}
		if target.ID == pet.ID || target.ID == c.Actor.ID {
			c.Print("%s refuses to attack that.", pet.CapDisplay())
			return
		}
		every := 4
		if c.Pack != nil && c.Pack.Meta.CombatEvery > 0 {
			every = c.Pack.Meta.CombatEvery
		}
		if pet.Combat == nil {
			pet.Combat = &world.Combat{RoundSpeed: every, Attacks: []world.Attack{{Name: "bite", Damage: "1d4"}}}
		}
		if target.Combat == nil {
			target.Combat = &world.Combat{RoundSpeed: every, Attacks: []world.Attack{{Name: "hit", Damage: "1d4"}}}
		}
		pet.Combat.Target = target.ID
		pet.Combat.NextSwing = c.Tick + 1
		if target.Combat.Target == "" {
			target.Combat.Target = pet.ID
			target.Combat.NextSwing = c.Tick + 1
		}
		c.Print("You order %s to attack %s! %s leaps into combat!", pet.Display(), target.Display(), pet.CapDisplay())

	case "dismiss":
		c.World.StopFollowing(pet)
		if c.Actor.Leader == pet.ID {
			c.World.StopFollowing(c.Actor)
		}
		c.World.ClearParty(pet.ID)
		if pet.PartyLeader == c.Actor.PartyLeader || pet.PartyLeader == c.Actor.ID {
			pet.PartyLeader = ""
		}
		pet.Owner = ""
		pet.Companion = false
		pet.Stance = ""
		pet.RestStance = ""
		c.Print("You dismiss %s. It wanders off.", pet.CapDisplay())

	default:
		c.Print("Unknown order %q. Options: follow, stay, stance, attack, dismiss.", act)
	}
}

func feedCmd(c *Context, p Parsed) {
	rest := strings.TrimSpace(p.Rest)
	if rest == "" {
		c.Print("Feed whom with what? Syntax: feed <pet> with <item>")
		return
	}
	var petStr, foodStr string
	low := strings.ToLower(rest)
	if idx := strings.Index(low, " to "); idx >= 0 {
		foodStr = strings.TrimSpace(rest[:idx])
		petStr = strings.TrimSpace(rest[idx+4:])
	} else if idx := strings.Index(low, " with "); idx >= 0 {
		petStr = strings.TrimSpace(rest[:idx])
		foodStr = strings.TrimSpace(rest[idx+6:])
	} else {
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			c.Print("Feed whom with what? Syntax: feed <pet> with <item>")
			return
		}
		if m, err := resolve(c, fields[0], world.ScopeRoom); err == nil && m.Kind == world.KindMobile {
			petStr = fields[0]
			foodStr = strings.Join(fields[1:], " ")
		} else {
			foodStr = fields[0]
			petStr = strings.Join(fields[1:], " ")
		}
	}

	pet, err := resolve(c, petStr, world.ScopeRoom)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	if pet.Kind != world.KindMobile {
		c.Print("You can't feed %s.", pet.Display())
		return
	}

	food, err := resolve(c, foodStr, world.ScopeInventory)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}

	if !isFood(food) {
		c.Print("%s sniffs %s with disinterest. That's not food.", pet.CapDisplay(), food.Display())
		return
	}
	if pet.Owner != c.Actor.ID && !pet.Companion && !pet.HasFlag("tameable") && !pet.HasFlag("pet") {
		c.Print("%s will not take food from you.", pet.CapDisplay())
		return
	}

	c.World.Destroy(food.ID)
	if pet.MaxLoyalty <= 0 {
		pet.MaxLoyalty = 100
	}
	pet.Loyalty += 15
	if pet.Loyalty > pet.MaxLoyalty {
		pet.Loyalty = pet.MaxLoyalty
	}
	if c.Pack != nil {
		primary := c.Pack.RPG.Primary()
		if primary != "" {
			pet.AdjustRes(primary, 5)
		}
	}

	c.Print("You feed %s to %s. %s happily devours it! (Loyalty: %d/%d)", food.Display(), pet.Display(), pet.CapDisplay(), pet.Loyalty, pet.MaxLoyalty)

	if pet.Owner == "" && (pet.HasFlag("tameable") || pet.HasFlag("pet") || pet.Companion) {
		if pet.Loyalty >= 30 {
			pet.Owner = c.Actor.ID
			pet.Companion = true
			if pet.Stance == "" {
				pet.Stance = "assist"
			}
			_ = c.World.StartFollowing(pet, c.Actor)
			world.BreakFight(pet, c.Actor)
			c.Print("%s has grown fond of you and is now your companion!", pet.CapDisplay())
		}
	}
}
