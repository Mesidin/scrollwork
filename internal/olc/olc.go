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
	if p.Rest == "" {
		c.Print("Current desc:\n%s", room.Long)
		c.Print("Usage: desc <text>")
		return
	}
	room.Long = p.Rest
	c.Tell(protocol.ChanBuild, "Description set.")
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
