package olc

import (
	"fmt"
	"strings"

	"sudengine/internal/commands"
	"sudengine/internal/protocol"
	"sudengine/internal/world"
)

func Lookup(verb string) commands.Handler {
	switch verb {
	case "dig":
		return dig
	case "buildwalk":
		return buildwalk
	case "desc":
		return desc
	case "name":
		return name
	case "rflags":
		return rflags
	case "extra":
		return extra
	case "ai":
		return setAI
	case "iset":
		return iset
	case "spawn":
		return spawn
	case "place":
		return spawn
	case "proto":
		return proto
	case "goto":
		return gotoRoom
	case "rooms":
		return rooms
	case "savepack", "export":
		return savePack
	case "reload":
		return reload
	default:
		return nil
	}
}

func dig(c *commands.Context, p commands.Parsed) {
	if p.Rest == "" {
		c.Print("dig <dir> [name]")
		return
	}
	dir := world.CanonicalDir(p.Args[0])
	title := strings.TrimSpace(strings.TrimPrefix(p.Rest, p.Args[0]))
	title = strings.Trim(title, `"'`)
	if title == "" {
		title = "New Room"
	}
	Dig(c, dir, title)
}

func Dig(c *commands.Context, dir, title string) {
	dir = world.CanonicalDir(dir)
	here := c.World.RoomOf(c.Actor)
	if here == nil {
		c.Print("You aren't in a room.")
		return
	}
	if _, exists := here.Exits[dir]; exists {
		c.Print("There's already an exit %s.", dir)
		return
	}
	opp := world.Opposite(dir)
	id := world.ID(c.Pack.Meta.ID + "." + world.Slug(title))
	base := id
	n := 2
	for c.World.Get(id) != nil {
		id = world.ID(fmt.Sprintf("%s-%d", base, n))
		n++
	}
	room := &world.Entity{
		ID:        id,
		Kind:      world.KindRoom,
		Name:      title,
		Short:     title,
		Long:      "This room has no description yet.",
		Exits:     map[string]world.Exit{},
		HasCoords: here.HasCoords,
		Keywords:  []string{world.Slug(title)},
	}
	if dx, dy, dz, ok := world.DirDelta(dir); ok && here.HasCoords {
		room.X = here.X + dx
		room.Y = here.Y + dy
		room.Z = here.Z + dz
		room.HasCoords = true
	}
	c.World.Add(room)
	if here.Exits == nil {
		here.Exits = map[string]world.Exit{}
	}
	here.Exits[dir] = world.Exit{Dir: dir, To: room.ID}
	if opp != "" {
		room.Exits[opp] = world.Exit{Dir: opp, To: here.ID}
	}
	_ = c.World.Move(c.Actor.ID, room.ID)
	c.Tell(protocol.ChanBuild, fmt.Sprintf("Dug %s (%s) to the %s.", title, id, dir))
	commands.Lookup("look")(c, commands.Parsed{})
}

func buildwalk(c *commands.Context, p commands.Parsed) {
	if c.BuildWalk == nil {
		c.Print("Buildwalk isn't available.")
		return
	}
	arg := strings.ToLower(p.Rest)
	switch arg {
	case "on", "1", "true":
		*c.BuildWalk = true
	case "off", "0", "false":
		*c.BuildWalk = false
	default:
		*c.BuildWalk = !*c.BuildWalk
	}
	if *c.BuildWalk {
		c.Print("Buildwalk on. Walking into a void will dig a room.")
	} else {
		c.Print("Buildwalk off.")
	}
}

func desc(c *commands.Context, p commands.Parsed) {
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		c.Print("You aren't in a room.")
		return
	}
	if p.Rest == "" {
		c.Print("Current desc:\n%s", room.Long)
		c.Print("Type the new description. End with a line containing only .")
		if c.BeginCapture == nil {
			c.Print("Or: desc <one line>")
			return
		}
		c.BeginCapture(func(text string) {
			room.Long = strings.TrimSpace(text)
			c.Tell(protocol.ChanBuild, "Description set.")
		})
		return
	}
	room.Long = p.Rest
	c.Tell(protocol.ChanBuild, "Description set.")
}

func extra(c *commands.Context, p commands.Parsed) {
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		c.Print("You aren't in a room.")
		return
	}
	args := p.Args
	if len(args) == 0 || strings.EqualFold(args[0], "list") {
		c.Print("Extras in this room:")
		n := 0
		for _, e := range c.World.Children(room.ID) {
			if e.Kind == world.KindScenery {
				c.Print("  %s  (%s)", e.Display(), strings.Join(e.Keywords, ", "))
				n++
			}
		}
		if n == 0 {
			c.Print("  (none)")
		}
		c.Print("Usage: extra add <keywords> | <look text>")
		return
	}
	if !strings.EqualFold(args[0], "add") {
		c.Print("extra list | extra add <keywords> | <look text>")
		return
	}
	rest := strings.TrimSpace(strings.TrimPrefix(p.Rest, p.Args[0]))
	kw, text, ok := strings.Cut(rest, "|")
	if !ok {
		c.Print("extra add <keywords> | <look text>")
		return
	}
	kws := strings.Fields(strings.ToLower(kw))
	text = strings.TrimSpace(text)
	if len(kws) == 0 || text == "" {
		c.Print("extra add <keywords> | <look text>")
		return
	}
	id := world.ID(fmt.Sprintf("%s.scenery.%s", room.ID, world.Slug(kws[0])))
	n := 2
	base := id
	for c.World.Get(id) != nil {
		id = world.ID(fmt.Sprintf("%s-%d", base, n))
		n++
	}
	e := &world.Entity{
		ID:       id,
		Kind:     world.KindScenery,
		Keywords: kws,
		Name:     kws[0],
		Short:    kws[0],
		Long:     text,
		Takeable: false,
	}
	c.World.Add(e)
	_ = c.World.Move(e.ID, room.ID)
	c.Tell(protocol.ChanBuild, fmt.Sprintf("Added extra %q.", kws[0]))
}

func setAI(c *commands.Context, p commands.Parsed) {
	if p.Rest == "" {
		c.Print("ai [target] <sentinel|wander|aggressive|coward>")
		return
	}
	profile := strings.ToLower(p.Args[len(p.Args)-1])
	switch profile {
	case "sentinel", "wander", "aggressive", "coward":
	default:
		c.Print("Profiles: sentinel, wander, aggressive, coward")
		return
	}
	var mob *world.Entity
	var err error
	if len(p.Args) >= 2 {
		token := strings.Join(p.Args[:len(p.Args)-1], " ")
		mob, err = findBuildTarget(c, token, world.KindMobile)
		if err != nil {
			c.Print("%s", err.Error())
			return
		}
	} else {
		mob = onlyMobile(c)
		if mob == nil {
			c.Print("ai <target> %s", profile)
			return
		}
	}
	if mob.AI == nil {
		mob.AI = &world.AI{}
	}
	mob.AI.Profile = profile
	mob.AI.Wander = profile == "wander" || profile == "aggressive"
	if proto := c.World.Protos[mob.PrototypeID]; proto != nil {
		if proto.AI == nil {
			proto.AI = &world.AI{}
		}
		proto.AI.Profile = profile
		proto.AI.Wander = mob.AI.Wander
	}
	c.Tell(protocol.ChanBuild, fmt.Sprintf("%s AI = %s", mob.Display(), profile))
}

func iset(c *commands.Context, p commands.Parsed) {
	if len(p.Args) < 2 {
		c.Print("iset <item> use <flag> | iset <item> damage <dice> | iset <item> value <n>")
		return
	}
	item, err := findBuildTarget(c, p.Args[0], world.KindItem)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	field := strings.ToLower(p.Args[1])
	rest := strings.TrimSpace(strings.Join(p.Args[2:], " "))
	switch field {
	case "use":
		if rest == "" {
			c.Print("iset %s use <flag>", p.Args[0])
			return
		}
		item.Use = &world.UseEffect{ToggleFlag: rest, Message: "You use it.", MessageOff: "You stop using it."}
		syncProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s use toggle %s", item.Display(), rest))
	case "damage":
		if rest == "" {
			c.Print("iset %s damage 1d6", p.Args[0])
			return
		}
		verb := "hit"
		item.Weapon = &world.Weapon{Damage: first(rest), Verb: verb}
		if item.Slot == "" {
			item.Slot = "wield"
			item.Wearable = true
		}
		syncProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s weapon %s", item.Display(), item.Weapon.Damage))
	case "value":
		n := 0
		fmt.Sscanf(rest, "%d", &n)
		item.Value = n
		syncProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s value %d", item.Display(), n))
	default:
		c.Print("iset fields: use, damage, value")
	}
}

func findBuildTarget(c *commands.Context, token string, kind world.Kind) (*world.Entity, error) {
	if proto := c.World.Protos[world.ID(token)]; proto != nil && proto.Kind == kind {
		return proto, nil
	}
	for _, proto := range c.World.Protos {
		if proto.Kind == kind && proto.HasKeyword(token) {
			return proto, nil
		}
	}
	one, many := c.World.Match(c.Actor, token, world.ScopeRoom, world.ScopeInventory)
	if one != nil && one.Kind == kind {
		return one, nil
	}
	if len(many) > 0 {
		return nil, fmt.Errorf("which one?")
	}
	return nil, fmt.Errorf("no %s %q here (try a prototype id)", kind, token)
}

func onlyMobile(c *commands.Context) *world.Entity {
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		return nil
	}
	var found *world.Entity
	for _, e := range c.World.Children(room.ID) {
		if e.Kind == world.KindMobile {
			if found != nil {
				return nil
			}
			found = e
		}
	}
	return found
}

func syncProto(c *commands.Context, e *world.Entity) {
	pid := e.PrototypeID
	if pid == "" {
		pid = e.ID
	}
	if proto := c.World.Protos[pid]; proto != nil {
		if e.Use != nil {
			u := *e.Use
			proto.Use = &u
		}
		if e.Weapon != nil {
			w := *e.Weapon
			proto.Weapon = &w
		}
		proto.Value = e.Value
		proto.Wearable = e.Wearable
		proto.Slot = e.Slot
	}
}

func name(c *commands.Context, p commands.Parsed) {
	if p.Rest == "" {
		c.Print("name <room title>")
		return
	}
	room := c.World.RoomOf(c.Actor)
	room.Name = p.Rest
	room.Short = p.Rest
	c.Tell(protocol.ChanBuild, "Name set.")
}

func rflags(c *commands.Context, p commands.Parsed) {
	room := c.World.RoomOf(c.Actor)
	if p.Rest == "" {
		c.Print("Flags: %v", room.Flags)
		c.Print("Usage: rflags <flag>   (toggles)")
		return
	}
	f := strings.ToLower(first(p.Rest))
	on := !room.HasFlag(f)
	room.SetFlag(f, on)
	c.Tell(protocol.ChanBuild, fmt.Sprintf("Flag %s = %v", f, on))
}

func spawn(c *commands.Context, p commands.Parsed) {
	if p.Rest == "" {
		c.Print("spawn <prototype-id>")
		return
	}
	id := world.ID(first(p.Rest))
	room := c.World.RoomOf(c.Actor)
	ent, err := c.World.Spawn(id, room.ID)
	if err != nil {
		c.Print("%s", err.Error())
		return
	}
	c.Tell(protocol.ChanBuild, fmt.Sprintf("Spawned %s (%s).", ent.Display(), ent.ID))
}

func proto(c *commands.Context, p commands.Parsed) {
	// proto item lamp "a brass lamp"  OR proto npc rat "a rat"
	if len(p.Args) < 2 {
		c.Print("proto item|npc <id> [short name]")
		return
	}
	kind := strings.ToLower(p.Args[0])
	id := world.ID(p.Args[1])
	short := strings.TrimSpace(strings.Join(p.Args[2:], " "))
	if short == "" {
		short = string(id)
	}
	e := &world.Entity{
		ID:       id,
		Name:     short,
		Short:    short,
		Long:     "Undescribed.",
		Keywords: []string{world.Slug(short), string(id)},
	}
	switch kind {
	case "item", "object", "obj":
		e.Kind = world.KindItem
		e.Takeable = true
	case "npc", "mob", "mobile":
		e.Kind = world.KindMobile
		e.AI = &world.AI{Profile: "sentinel"}
		e.Combat = &world.Combat{RoundSpeed: 4, Attacks: []world.Attack{{Name: "hit", Damage: "1d4"}}}
		e.Resources = map[string]world.Resource{"hp": {Current: 10, Max: 10}}
	default:
		c.Print("proto item|npc ...")
		return
	}
	c.World.Protos[id] = e
	c.Tell(protocol.ChanBuild, fmt.Sprintf("Prototype %s (%s) created. Use spawn %s.", id, kind, id))
}

func gotoRoom(c *commands.Context, p commands.Parsed) {
	if p.Rest == "" {
		c.Print("goto <room-id>")
		return
	}
	dest := c.World.Get(world.ID(p.Rest))
	if dest == nil || dest.Kind != world.KindRoom {
		c.Print("No room %q.", p.Rest)
		return
	}
	_ = c.World.Move(c.Actor.ID, dest.ID)
	commands.Lookup("look")(c, commands.Parsed{})
}

func rooms(c *commands.Context, _ commands.Parsed) {
	c.Print("Rooms:")
	for _, e := range c.World.Entities {
		if e.Kind == world.KindRoom {
			here := ""
			if c.World.RoomOf(c.Actor) == e {
				here = "  <- you"
			}
			c.Print("  %s  %s%s", e.ID, e.Name, here)
		}
	}
}

func savePack(c *commands.Context, _ commands.Parsed) {
	if c.SavePack == nil {
		c.Print("Pack save isn't available.")
		return
	}
	if err := c.SavePack(); err != nil {
		c.Print("Pack save failed: %s", err.Error())
		return
	}
	c.Tell(protocol.ChanBuild, "Pack written to disk.")
}

func reload(c *commands.Context, p commands.Parsed) {
	if c.ReloadScripts == nil {
		c.Print("Reload isn't available.")
		return
	}
	if err := c.ReloadScripts(); err != nil {
		c.Print("Reload failed: %s", err.Error())
		return
	}
	c.Tell(protocol.ChanBuild, "Scripts reloaded.")
}

func first(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}
