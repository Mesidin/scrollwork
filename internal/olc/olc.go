package olc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"sudengine/internal/commands"
	"sudengine/internal/economy"
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
	case "mset":
		return mset
	case "gset":
		return gset
	case "recipe", "rcset", "rclist":
		return recipeCmd
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
	mob.AI.Wander = profile == "wander"
	syncMobProto(c, mob)
	c.Tell(protocol.ChanBuild, fmt.Sprintf("%s AI = %s", mob.Display(), profile))
}

func mset(c *commands.Context, p commands.Parsed) {
	if len(p.Args) == 0 {
		c.Print("Usage: mset <target> <field> [value...]")
		c.Print("Fields: show, ai, trainer, repairer, banker, xp, respawn, shop, topic, teach, loot")
		return
	}

	var mob *world.Entity
	var field string
	var args []string

	targetMob, err := findBuildTarget(c, p.Args[0], world.KindMobile)
	if err == nil {
		mob = targetMob
		if len(p.Args) >= 2 {
			field = strings.ToLower(p.Args[1])
			args = p.Args[2:]
		} else {
			field = "show"
		}
	} else if single := onlyMobile(c); single != nil {
		mob = single
		field = strings.ToLower(p.Args[0])
		args = p.Args[1:]
	} else {
		c.Print("%s", err.Error())
		return
	}

	switch field {
	case "show", "stat", "stats":
		showMob(c, mob)

	case "trainer":
		val := true
		if len(args) > 0 {
			val = parseBool(args[0], !mob.Trainer)
		} else {
			val = !mob.Trainer
		}
		mob.Trainer = val
		syncMobProto(c, mob)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s trainer = %v", mob.Display(), val))

	case "repairer":
		val := true
		if len(args) > 0 {
			val = parseBool(args[0], !mob.Repairer)
		} else {
			val = !mob.Repairer
		}
		mob.Repairer = val
		syncMobProto(c, mob)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s repairer = %v", mob.Display(), val))

	case "banker":
		val := true
		if len(args) > 0 {
			val = parseBool(args[0], !mob.Banker)
		} else {
			val = !mob.Banker
		}
		mob.Banker = val
		syncMobProto(c, mob)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s banker = %v", mob.Display(), val))

	case "ai":
		if len(args) == 0 {
			c.Print("mset %s ai <sentinel|wander|aggressive|coward>", mob.Display())
			return
		}
		prof := strings.ToLower(args[0])
		switch prof {
		case "sentinel", "wander", "aggressive", "coward":
		default:
			c.Print("Profiles: sentinel, wander, aggressive, coward")
			return
		}
		if mob.AI == nil {
			mob.AI = &world.AI{}
		}
		wander := prof == "wander"
		if len(args) > 1 {
			switch strings.ToLower(args[1]) {
			case "wander", "on", "yes", "true":
				wander = true
			case "off", "no", "false", "stay":
				wander = false
			default:
				c.Print("wander is on or off")
				return
			}
		}
		mob.AI.Profile = prof
		mob.AI.Wander = wander
		syncMobProto(c, mob)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s AI = %s", mob.Display(), prof))

	case "xp":
		if len(args) == 0 {
			c.Print("mset %s xp <n>", mob.Display())
			return
		}
		n, _ := strconv.Atoi(args[0])
		mob.XP = n
		syncMobProto(c, mob)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s xp = %d", mob.Display(), n))

	case "respawn", "respawn_every":
		if len(args) == 0 {
			c.Print("mset %s respawn <ticks>", mob.Display())
			return
		}
		n, _ := strconv.Atoi(args[0])
		mob.RespawnEvery = n
		syncMobProto(c, mob)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s respawn_every = %d ticks", mob.Display(), n))

	case "shop":
		if len(args) == 0 {
			c.Print("mset %s shop currency|purse|maxpurse|dynamic|clear", mob.Display())
			return
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "clear":
			mob.Shop = nil
			syncMobProto(c, mob)
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s shop cleared", mob.Display()))
		case "currency":
			if len(args) < 2 {
				c.Print("mset %s shop currency <currency-name>", mob.Display())
				return
			}
			if mob.Shop == nil {
				mob.Shop = &world.Shop{Dynamic: true}
			}
			mob.Shop.Currency = args[1]
			syncMobProto(c, mob)
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s shop currency = %s", mob.Display(), mob.Shop.Currency))
		case "purse":
			if len(args) < 2 {
				c.Print("mset %s shop purse <n>", mob.Display())
				return
			}
			n, _ := strconv.Atoi(args[1])
			if mob.Shop == nil {
				mob.Shop = &world.Shop{Currency: "coin", Dynamic: true, Finite: true}
			}
			mob.Shop.Purse = n
			mob.Shop.Finite = true
			syncMobProto(c, mob)
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s shop purse = %d", mob.Display(), n))
		case "maxpurse", "max_purse":
			if len(args) < 2 {
				c.Print("mset %s shop maxpurse <n>", mob.Display())
				return
			}
			n, _ := strconv.Atoi(args[1])
			if mob.Shop == nil {
				mob.Shop = &world.Shop{Currency: "coin", Dynamic: true, Finite: true}
			}
			mob.Shop.MaxPurse = n
			mob.Shop.Finite = true
			syncMobProto(c, mob)
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s shop max_purse = %d", mob.Display(), n))
		case "dynamic":
			val := true
			if len(args) >= 2 {
				val = parseBool(args[1], true)
			} else if mob.Shop != nil {
				val = !mob.Shop.Dynamic
			}
			if mob.Shop == nil {
				mob.Shop = &world.Shop{Currency: "coin"}
			}
			mob.Shop.Dynamic = val
			if val {
				mob.Shop.Finite = true
			}
			syncMobProto(c, mob)
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s shop dynamic = %v", mob.Display(), val))
		default:
			c.Print("Shop subcommands: currency, purse, maxpurse, dynamic, clear")
		}

	case "topic":
		if len(args) < 1 {
			c.Print("mset %s topic <key> [response...]", mob.Display())
			return
		}
		key := strings.ToLower(args[0])
		resp := strings.TrimSpace(strings.Join(args[1:], " "))
		if resp == "" || resp == "clear" || resp == "none" {
			if mob.Topics != nil {
				delete(mob.Topics, key)
			}
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s topic %q cleared", mob.Display(), key))
		} else {
			if mob.Topics == nil {
				mob.Topics = make(map[string]string)
			}
			mob.Topics[key] = resp
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s topic %q set", mob.Display(), key))
		}
		syncMobProto(c, mob)

	case "teach":
		if len(args) < 1 {
			c.Print("mset %s teach <topic> [recipeID]", mob.Display())
			return
		}
		topic := strings.ToLower(args[0])
		recipeID := ""
		if len(args) >= 2 {
			recipeID = args[1]
		}
		if recipeID == "" || recipeID == "clear" || recipeID == "none" {
			if mob.TeachRecipes != nil {
				delete(mob.TeachRecipes, topic)
			}
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s teach recipe on %q cleared", mob.Display(), topic))
		} else {
			if mob.TeachRecipes == nil {
				mob.TeachRecipes = make(map[string]string)
			}
			mob.TeachRecipes[topic] = recipeID
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s teaches recipe %s on topic %q", mob.Display(), recipeID, topic))
		}
		syncMobProto(c, mob)

	case "loot":
		if len(args) == 0 || strings.EqualFold(args[0], "list") {
			if len(mob.Loot) == 0 {
				c.Print("Loot drops: (none)")
				return
			}
			c.Print("Loot drops on %s:", mob.Display())
			for i, l := range mob.Loot {
				skin := ""
				if l.SkinOnly {
					skin = " (skin only)"
				}
				c.Print("  [%d] %s x%d (%d%%)%s", i+1, l.Item, l.Count, l.Chance, skin)
			}
			return
		}
		sub := strings.ToLower(args[0])
		switch sub {
		case "clear":
			mob.Loot = nil
			syncMobProto(c, mob)
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s loot table cleared", mob.Display()))
		case "add":
			if len(args) < 2 {
				c.Print("mset %s loot add <item> [chance] [count] [skin]", mob.Display())
				return
			}
			itemID := args[1]
			chance := 100
			count := 1
			skinOnly := false
			parsedNums := 0
			for _, opt := range args[2:] {
				optL := strings.ToLower(opt)
				if optL == "skin" || optL == "skinonly" || optL == "skin_only" {
					skinOnly = true
					continue
				}
				if val, err := strconv.Atoi(opt); err == nil {
					if parsedNums == 0 {
						chance = val
						parsedNums++
					} else {
						count = val
						parsedNums++
					}
				}
			}
			mob.Loot = append(mob.Loot, world.LootDrop{
				Item:     itemID,
				Chance:   chance,
				Count:    count,
				SkinOnly: skinOnly,
			})
			syncMobProto(c, mob)
			c.Tell(protocol.ChanBuild, fmt.Sprintf("Added loot drop %s x%d (%d%%, skin=%v) to %s.", itemID, count, chance, skinOnly, mob.Display()))
		default:
			c.Print("Loot subcommands: list, add <item> [chance] [count] [skin], clear")
		}

	default:
		c.Print("Unknown mset field %q. Fields: show, ai, trainer, repairer, banker, xp, respawn, shop, topic, teach, loot", field)
	}
}

func showMob(c *commands.Context, mob *world.Entity) {
	c.Print("Mobile: %s (id: %s, proto: %s)", mob.Display(), mob.ID, mob.PrototypeID)
	c.Print("Keywords: %s", strings.Join(mob.Keywords, ", "))
	prof := "(none)"
	if mob.AI != nil && mob.AI.Profile != "" {
		prof = mob.AI.Profile
		if mob.AI.Wander {
			prof += " (wanders)"
		}
	}
	c.Print("AI Profile: %s", prof)
	c.Print("XP: %d | Respawn Every: %d ticks", mob.XP, mob.RespawnEvery)
	c.Print("Services: trainer=%v, repairer=%v, banker=%v", mob.Trainer, mob.Repairer, mob.Banker)
	if mob.Shop != nil {
		c.Print("Shop: currency=%s, purse=%d, max_purse=%d, dynamic=%v", mob.Shop.Currency, mob.Shop.Purse, mob.Shop.MaxPurse, mob.Shop.Dynamic)
	} else {
		c.Print("Shop: (none)")
	}
	if len(mob.Topics) > 0 {
		var topics []string
		for k := range mob.Topics {
			topics = append(topics, k)
		}
		sort.Strings(topics)
		c.Print("Topics: %s", strings.Join(topics, ", "))
	}
	if len(mob.TeachRecipes) > 0 {
		var teaches []string
		for k, v := range mob.TeachRecipes {
			teaches = append(teaches, fmt.Sprintf("%s -> %s", k, v))
		}
		sort.Strings(teaches)
		c.Print("Teach Recipes: %s", strings.Join(teaches, ", "))
	}
	if len(mob.Loot) > 0 {
		c.Print("Loot drops:")
		for _, l := range mob.Loot {
			skin := ""
			if l.SkinOnly {
				skin = " (skin only)"
			}
			c.Print("  %s x%d (%d%%)%s", l.Item, l.Count, l.Chance, skin)
		}
	}
}

func iset(c *commands.Context, p commands.Parsed) {
	if len(p.Args) < 2 {
		c.Print("Usage: iset <item> <field> [value...]")
		c.Print("Fields: show, use, damage, armor, slot, value, durability, repairable, banker, kit, salvage")
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
	case "show", "stat", "stats":
		showItem(c, item)

	case "use":
		if rest == "" {
			c.Print("iset %s use <flag>", p.Args[0])
			return
		}
		item.Use = &world.UseEffect{ToggleFlag: rest, Message: "You use it.", MessageOff: "You stop using it."}
		syncItemProto(c, item)
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
		syncItemProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s weapon %s", item.Display(), item.Weapon.Damage))

	case "armor":
		if rest == "" {
			c.Print("iset %s armor <soak>", p.Args[0])
			return
		}
		n, _ := strconv.Atoi(first(rest))
		item.Armor = &world.Armor{Soak: n}
		item.Wearable = true
		if item.Slot == "" {
			item.Slot = "body"
		}
		syncItemProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s armor soak = %d", item.Display(), n))

	case "slot":
		if rest == "" {
			c.Print("iset %s slot <body|head|wield|feet|...>", p.Args[0])
			return
		}
		item.Slot = rest
		item.Wearable = true
		syncItemProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s slot = %s", item.Display(), rest))

	case "value":
		n := 0
		fmt.Sscanf(rest, "%d", &n)
		item.Value = n
		syncItemProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s value %d", item.Display(), n))

	case "durability":
		if rest == "" {
			c.Print("iset %s durability <max>", p.Args[0])
			return
		}
		n, _ := strconv.Atoi(first(rest))
		item.MaxDurability = n
		item.Durability = n
		item.Repairable = true
		syncItemProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s durability = %d/%d", item.Display(), n, n))

	case "repairable":
		val := true
		if rest != "" {
			val = parseBool(rest, !item.Repairable)
		} else {
			val = !item.Repairable
		}
		item.Repairable = val
		syncItemProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s repairable = %v", item.Display(), val))

	case "banker", "vault":
		val := true
		if rest != "" {
			val = parseBool(rest, !item.Banker)
		} else {
			val = !item.Banker
		}
		item.Banker = val
		syncItemProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s banker = %v", item.Display(), val))

	case "kit":
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			c.Print("iset %s kit <uses> [amount] [skill] [dc] | iset %s kit clear", p.Args[0], p.Args[0])
			return
		}
		if strings.EqualFold(fields[0], "clear") {
			item.RepairKit = nil
			syncItemProto(c, item)
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s repair kit cleared", item.Display()))
			return
		}
		uses, _ := strconv.Atoi(fields[0])
		amount := 0
		skill := ""
		dc := 7
		if len(fields) >= 2 {
			amount, _ = strconv.Atoi(fields[1])
		}
		if len(fields) >= 3 {
			skill = fields[2]
		}
		if len(fields) >= 4 {
			dc, _ = strconv.Atoi(fields[3])
		}
		item.RepairKit = &world.RepairKit{
			Uses:    uses,
			MaxUses: uses,
			Amount:  amount,
			Skill:   skill,
			DC:      dc,
		}
		syncItemProto(c, item)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("%s configured as field repair kit (uses %d, amount %d, %s DC %d).", item.Display(), uses, amount, skill, dc))

	case "salvage":
		fields := strings.Fields(rest)
		if len(fields) == 0 || strings.EqualFold(fields[0], "list") {
			if len(item.Salvage) == 0 {
				c.Print("Salvage yields: (none)")
				return
			}
			c.Print("Salvage yields for %s:", item.Display())
			for i, s := range item.Salvage {
				c.Print("  [%d] %s x%d", i+1, s.Item, s.Count)
			}
			return
		}
		sub := strings.ToLower(fields[0])
		switch sub {
		case "clear":
			item.Salvage = nil
			syncItemProto(c, item)
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s salvage cleared", item.Display()))
		case "add":
			if len(fields) < 2 {
				c.Print("iset %s salvage add <item> [count]", p.Args[0])
				return
			}
			yieldItem := fields[1]
			count := 1
			if len(fields) >= 3 {
				count, _ = strconv.Atoi(fields[2])
				if count <= 0 {
					count = 1
				}
			}
			item.Salvage = append(item.Salvage, world.SalvageYield{Item: yieldItem, Count: count})
			syncItemProto(c, item)
			c.Tell(protocol.ChanBuild, fmt.Sprintf("%s yields %s x%d on salvage", item.Display(), yieldItem, count))
		default:
			c.Print("Salvage subcommands: list, add <item> [count], clear")
		}

	default:
		c.Print("iset fields: show, use, damage, armor, slot, value, durability, repairable, banker, kit, salvage")
	}
}

func showItem(c *commands.Context, it *world.Entity) {
	c.Print("Item: %s (id: %s, proto: %s)", it.Display(), it.ID, it.PrototypeID)
	c.Print("Keywords: %s", strings.Join(it.Keywords, ", "))
	c.Print("Takeable: %v | Value: %d", it.Takeable, it.Value)
	if it.Wearable {
		c.Print("Wearable: true (slot: %s)", it.Slot)
	}
	if it.Weapon != nil {
		c.Print("Weapon: damage=%s, verb=%s", it.Weapon.Damage, it.Weapon.Verb)
	}
	if it.Armor != nil {
		c.Print("Armor: soak=%d", it.Armor.Soak)
	}
	if it.MaxDurability > 0 {
		c.Print("Durability: %d/%d (repairable: %v)", it.Durability, it.MaxDurability, it.Repairable)
	}
	if it.Banker {
		c.Print("Banker/Vault: true")
	}
	if it.RepairKit != nil {
		c.Print("Repair Kit: uses=%d/%d, amount=%d, skill=%s, dc=%d", it.RepairKit.Uses, it.RepairKit.MaxUses, it.RepairKit.Amount, it.RepairKit.Skill, it.RepairKit.DC)
	}
	if len(it.Salvage) > 0 {
		c.Print("Salvage yields:")
		for _, s := range it.Salvage {
			c.Print("  %s x%d", s.Item, s.Count)
		}
	}
	if it.Gather != nil {
		c.Print("Gather node: resource=%s, capacity=%d/%d, regen=%d, skill=%s, dc=%d, bonus_dc=%d",
			it.Gather.Resource, it.Gather.Remaining, it.Gather.Max, it.Gather.RegenEvery, it.Gather.Skill, it.Gather.Difficulty, it.Gather.BonusDC)
	}
	if it.Use != nil {
		c.Print("Use: toggle_flag=%s", it.Use.ToggleFlag)
	}
}

func findBuildTarget(c *commands.Context, token string, kind world.Kind) (*world.Entity, error) {
	if proto := c.World.Protos[world.ID(token)]; proto != nil && (kind == "" || proto.Kind == kind) {
		return proto, nil
	}
	for _, proto := range c.World.Protos {
		if (kind == "" || proto.Kind == kind) && proto.HasKeyword(token) {
			return proto, nil
		}
	}
	one, many := c.World.Match(c.Actor, token, world.ScopeRoom, world.ScopeInventory)
	if one != nil && (kind == "" || one.Kind == kind) {
		return one, nil
	}
	if len(many) > 0 {
		return nil, fmt.Errorf("which one?")
	}
	if kind != "" {
		return nil, fmt.Errorf("no %s %q here (try a prototype id)", kind, token)
	}
	return nil, fmt.Errorf("no entity %q here", token)
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

func syncItemProto(c *commands.Context, item *world.Entity) {
	pid := item.PrototypeID
	if pid == "" {
		pid = item.ID
	}
	proto := c.World.Protos[pid]
	if proto == nil {
		return
	}
	if item.Use != nil {
		u := *item.Use
		proto.Use = &u
	} else {
		proto.Use = nil
	}
	if item.Weapon != nil {
		w := *item.Weapon
		proto.Weapon = &w
	} else {
		proto.Weapon = nil
	}
	if item.Armor != nil {
		a := *item.Armor
		proto.Armor = &a
	} else {
		proto.Armor = nil
	}
	proto.Value = item.Value
	proto.Wearable = item.Wearable
	proto.Slot = item.Slot
	proto.Durability = item.Durability
	proto.MaxDurability = item.MaxDurability
	proto.Repairable = item.Repairable
	proto.Banker = item.Banker
	if item.RepairKit != nil {
		rk := *item.RepairKit
		proto.RepairKit = &rk
	} else {
		proto.RepairKit = nil
	}
	if item.Salvage != nil {
		proto.Salvage = append([]world.SalvageYield{}, item.Salvage...)
	} else {
		proto.Salvage = nil
	}
	if item.Gather != nil {
		gn := *item.Gather
		proto.Gather = &gn
	} else {
		proto.Gather = nil
	}

	if item == proto {
		syncInstance := func(ent *world.Entity) {
			if ent == proto || (ent.PrototypeID != pid && ent.ID != pid) {
				return
			}
			ent.Use = cloneUse(proto.Use)
			ent.Weapon = cloneWeapon(proto.Weapon)
			ent.Armor = cloneArmor(proto.Armor)
			ent.Value = proto.Value
			ent.Wearable = proto.Wearable
			ent.Slot = proto.Slot
			ent.Durability = proto.Durability
			ent.MaxDurability = proto.MaxDurability
			ent.Repairable = proto.Repairable
			ent.Banker = proto.Banker
			ent.RepairKit = cloneKit(proto.RepairKit)
			ent.Salvage = cloneSalvage(proto.Salvage)
			ent.Gather = cloneGather(proto.Gather)
		}
		room := c.World.RoomOf(c.Actor)
		if room != nil {
			for _, cid := range c.World.Children(room.ID) {
				if cid.Kind == world.KindItem {
					syncInstance(cid)
				}
			}
		}
		for _, cid := range c.Actor.Contents {
			if it := c.World.Get(cid); it != nil && it.Kind == world.KindItem {
				syncInstance(it)
			}
		}
	}
}

func syncMobProto(c *commands.Context, mob *world.Entity) {
	pid := mob.PrototypeID
	if pid == "" {
		pid = mob.ID
	}
	proto := c.World.Protos[pid]
	if proto == nil {
		return
	}
	proto.AI = cloneAI(mob.AI)
	proto.XP = mob.XP
	proto.Trainer = mob.Trainer
	proto.Repairer = mob.Repairer
	proto.Banker = mob.Banker
	proto.RespawnEvery = mob.RespawnEvery
	proto.Shop = cloneShop(mob.Shop)
	if mob.Topics != nil {
		topics := make(map[string]string, len(mob.Topics))
		for k, v := range mob.Topics {
			topics[k] = v
		}
		proto.Topics = topics
	} else {
		proto.Topics = nil
	}
	if mob.TeachRecipes != nil {
		teaches := make(map[string]string, len(mob.TeachRecipes))
		for k, v := range mob.TeachRecipes {
			teaches[k] = v
		}
		proto.TeachRecipes = teaches
	} else {
		proto.TeachRecipes = nil
	}
	if mob.Loot != nil {
		proto.Loot = append([]world.LootDrop{}, mob.Loot...)
	} else {
		proto.Loot = nil
	}

	if mob == proto {
		room := c.World.RoomOf(c.Actor)
		if room != nil {
			for _, cid := range c.World.Children(room.ID) {
				if cid == proto || cid.Kind != world.KindMobile || (cid.PrototypeID != pid && cid.ID != pid) {
					continue
				}
				cid.AI = cloneAI(proto.AI)
				cid.XP = proto.XP
				cid.Trainer = proto.Trainer
				cid.Repairer = proto.Repairer
				cid.Banker = proto.Banker
				cid.RespawnEvery = proto.RespawnEvery
				cid.Shop = cloneShop(proto.Shop)
				cid.Topics = cloneStringMap(proto.Topics)
				cid.TeachRecipes = cloneStringMap(proto.TeachRecipes)
				cid.Loot = cloneLoot(proto.Loot)
			}
		}
	}
}

func syncProto(c *commands.Context, e *world.Entity) {
	if e == nil {
		return
	}
	if e.Kind == world.KindItem {
		syncItemProto(c, e)
	} else if e.Kind == world.KindMobile {
		syncMobProto(c, e)
	}
}

func cloneUse(src *world.UseEffect) *world.UseEffect {
	if src == nil {
		return nil
	}
	u := *src
	return &u
}

func cloneWeapon(src *world.Weapon) *world.Weapon {
	if src == nil {
		return nil
	}
	w := *src
	return &w
}

func cloneArmor(src *world.Armor) *world.Armor {
	if src == nil {
		return nil
	}
	a := *src
	return &a
}

func cloneKit(src *world.RepairKit) *world.RepairKit {
	if src == nil {
		return nil
	}
	rk := *src
	return &rk
}

func cloneGather(src *world.GatherNode) *world.GatherNode {
	if src == nil {
		return nil
	}
	g := *src
	return &g
}

func cloneShop(src *world.Shop) *world.Shop {
	if src == nil {
		return nil
	}
	s := *src
	if src.Stock != nil {
		s.Stock = append([]string{}, src.Stock...)
	}
	return &s
}

func cloneAI(src *world.AI) *world.AI {
	if src == nil {
		return nil
	}
	ai := *src
	ai.AssistTags = append([]string{}, src.AssistTags...)
	ai.Rooms = append([]world.ID{}, src.Rooms...)
	return &ai
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneSalvage(src []world.SalvageYield) []world.SalvageYield {
	if src == nil {
		return nil
	}
	return append([]world.SalvageYield{}, src...)
}

func cloneLoot(src []world.LootDrop) []world.LootDrop {
	if src == nil {
		return nil
	}
	return append([]world.LootDrop{}, src...)
}

func parseBool(s string, def bool) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "on", "true", "1", "yes":
		return true
	case "off", "false", "0", "no":
		return false
	default:
		return def
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

func gset(c *commands.Context, p commands.Parsed) {
	if len(p.Args) == 0 {
		c.Print("Usage: gset show [target]")
		c.Print("       gset clear [target]")
		c.Print("       gset node <extra|item> item <itemProto> [max] [regen] [skill] [dc] [bonus_dc]")
		c.Print("       gset room item <itemProto> [max] [regen] [skill] [dc] [bonus_dc]")
		return
	}

	action := strings.ToLower(p.Args[0])
	switch action {
	case "show", "stat", "stats":
		var target *world.Entity
		if len(p.Args) >= 2 {
			var err error
			target, err = findBuildTarget(c, p.Args[1], "")
			if err != nil {
				c.Print("%s", err.Error())
				return
			}
		} else {
			room := c.World.RoomOf(c.Actor)
			if room != nil && room.Gather != nil {
				target = room
			} else if room != nil {
				for _, kid := range c.World.Children(room.ID) {
					if kid.Gather != nil {
						target = kid
						break
					}
				}
			}
			if target == nil {
				target = room
			}
		}
		if target == nil || target.Gather == nil {
			c.Print("No gathering node configured.")
			return
		}
		gn := target.Gather
		c.Print("Gather node on %s:", target.Display())
		c.Print("  Resource: %s", gn.Resource)
		c.Print("  Capacity: %d/%d (regen every %d ticks)", gn.Remaining, gn.Max, gn.RegenEvery)
		c.Print("  Check: %s (DC: %d, Bonus DC: %d)", gn.Skill, gn.Difficulty, gn.BonusDC)

	case "clear":
		var target *world.Entity
		if len(p.Args) >= 2 {
			if strings.EqualFold(p.Args[1], "room") {
				target = c.World.RoomOf(c.Actor)
			} else {
				var err error
				target, err = findBuildTarget(c, p.Args[1], "")
				if err != nil {
					c.Print("%s", err.Error())
					return
				}
			}
		} else {
			target = c.World.RoomOf(c.Actor)
		}
		if target != nil {
			target.Gather = nil
			if target.Kind == world.KindItem {
				syncItemProto(c, target)
			}
			c.Tell(protocol.ChanBuild, fmt.Sprintf("Gather node cleared on %s.", target.Display()))
		}

	case "room":
		room := c.World.RoomOf(c.Actor)
		if room == nil {
			c.Print("You aren't in a room.")
			return
		}
		setGatherNode(c, room, p.Args[1:])

	case "node":
		if len(p.Args) < 2 {
			c.Print("gset node <extra|item> item <itemProto> [max] [regen] [skill] [dc] [bonus_dc]")
			return
		}
		targetName := p.Args[1]
		target, err := findBuildTarget(c, targetName, "")
		if err != nil {
			c.Print("%s", err.Error())
			return
		}
		setGatherNode(c, target, p.Args[2:])

	default:
		target, err := findBuildTarget(c, p.Args[0], "")
		if err == nil {
			setGatherNode(c, target, p.Args[1:])
			return
		}
		c.Print("Unknown gset action %q. Usage: gset show | clear | node <target> item <id> | room item <id>", p.Args[0])
	}
}

func setGatherNode(c *commands.Context, target *world.Entity, args []string) {
	if len(args) == 0 {
		c.Print("gset ... item <itemProto> [max] [regen] [skill] [dc] [bonus_dc]")
		return
	}
	itemProto := ""
	max := 3
	regen := 30
	skill := "notice"
	dc := 6
	bonusDC := 9
	setMax, setRegen, setDC, setBonus, setSkill := false, false, false, false, false
	var positional []int
	posSkill := ""

	for i := 0; i < len(args); i++ {
		switch strings.ToLower(args[i]) {
		case "item":
			if i+1 < len(args) {
				itemProto = args[i+1]
				i++
			}
		case "max", "capacity":
			if i+1 < len(args) {
				max, _ = strconv.Atoi(args[i+1])
				setMax = true
				i++
			}
		case "regen", "regen_every":
			if i+1 < len(args) {
				regen, _ = strconv.Atoi(args[i+1])
				setRegen = true
				i++
			}
		case "skill":
			if i+1 < len(args) {
				skill = args[i+1]
				setSkill = true
				i++
			}
		case "dc", "diff":
			if i+1 < len(args) {
				dc, _ = strconv.Atoi(args[i+1])
				setDC = true
				i++
			}
		case "bonus_dc", "bonus":
			if i+1 < len(args) {
				bonusDC, _ = strconv.Atoi(args[i+1])
				setBonus = true
				i++
			}
		default:
			if itemProto == "" {
				if _, err := strconv.Atoi(args[i]); err != nil {
					itemProto = args[i]
					continue
				}
			}
			if n, err := strconv.Atoi(args[i]); err == nil {
				positional = append(positional, n)
				continue
			}
			if !setSkill && posSkill == "" {
				posSkill = args[i]
			}
		}
	}
	applyPos := func(set bool, dst *int) {
		if set || len(positional) == 0 {
			return
		}
		*dst = positional[0]
		positional = positional[1:]
	}
	applyPos(setMax, &max)
	applyPos(setRegen, &regen)
	applyPos(setDC, &dc)
	applyPos(setBonus, &bonusDC)
	if !setSkill && posSkill != "" {
		skill = posSkill
	}

	if itemProto == "" {
		c.Print("You must specify a harvestable item prototype. (e.g. item <itemProto>)")
		return
	}
	if max <= 0 {
		max = 1
	}

	target.Gather = &world.GatherNode{
		Resource:   itemProto,
		Remaining:  max,
		Max:        max,
		RegenEvery: regen,
		Skill:      skill,
		Difficulty: dc,
		BonusDC:    bonusDC,
	}
	if target.Kind == world.KindItem {
		syncItemProto(c, target)
	}
	c.Tell(protocol.ChanBuild, fmt.Sprintf("Gather node on %s set to harvest %s (max %d, regen %d, %s DC %d / bonus %d).",
		target.Display(), itemProto, max, regen, skill, dc, bonusDC))
}

func recipeCmd(c *commands.Context, p commands.Parsed) {
	if c.Pack == nil {
		c.Print("No active pack.")
		return
	}
	if len(p.Args) == 0 || strings.EqualFold(p.Args[0], "list") {
		if len(c.Pack.Recipes) == 0 {
			c.Print("No recipes defined in pack.")
			return
		}
		c.Print("Recipes (%d):", len(c.Pack.Recipes))
		for _, r := range c.Pack.Recipes {
			st := r.Station
			if st == "" {
				st = "none"
			}
			sk := r.Skill
			if sk == "" {
				sk = "none"
			}
			c.Print("  %s (%s) -> produces %s (station: %s, skill: %s DC %d)", r.ID, r.Name, r.Produces, st, sk, r.Difficulty)
		}
		c.Print("Commands: recipe show <id> | recipe add <id> ... | recipe req <id> <item> [count] | recipe del <id>")
		return
	}

	sub := strings.ToLower(p.Args[0])
	switch sub {
	case "show":
		if len(p.Args) < 2 {
			c.Print("recipe show <id>")
			return
		}
		id := p.Args[1]
		var found *economy.Recipe
		for i := range c.Pack.Recipes {
			if strings.EqualFold(c.Pack.Recipes[i].ID, id) {
				found = &c.Pack.Recipes[i]
				break
			}
		}
		if found == nil {
			c.Print("Recipe %q not found.", id)
			return
		}
		c.Print("Recipe: %s (%s)", found.ID, found.Name)
		c.Print("  Produces: %s (count: %d)", found.Produces, found.Count)
		c.Print("  Station: %s | Tools: %s", found.Station, strings.Join(found.Tools, ", "))
		c.Print("  Skill: %s (DC: %d, Min: %d)", found.Skill, found.Difficulty, found.MinSkill)
		disc := true
		if found.Discovered != nil {
			disc = *found.Discovered
		}
		c.Print("  Discovered by default: %v", disc)
		if len(found.Requires) > 0 {
			c.Print("  Ingredients:")
			for _, req := range found.Requires {
				c.Print("    %s x%d", req.Item, req.Count)
			}
		} else {
			c.Print("  Ingredients: (none)")
		}

	case "del", "remove", "delete":
		if len(p.Args) < 2 {
			c.Print("recipe del <id>")
			return
		}
		id := p.Args[1]
		idx := -1
		for i := range c.Pack.Recipes {
			if strings.EqualFold(c.Pack.Recipes[i].ID, id) {
				idx = i
				break
			}
		}
		if idx == -1 {
			c.Print("Recipe %q not found.", id)
			return
		}
		c.Pack.Recipes = append(c.Pack.Recipes[:idx], c.Pack.Recipes[idx+1:]...)
		c.Tell(protocol.ChanBuild, fmt.Sprintf("Recipe %q deleted.", id))

	case "req", "require", "ingredient":
		if len(p.Args) < 3 {
			c.Print("recipe req <recipeID> <itemID> [count]")
			return
		}
		id := p.Args[1]
		var found *economy.Recipe
		for i := range c.Pack.Recipes {
			if strings.EqualFold(c.Pack.Recipes[i].ID, id) {
				found = &c.Pack.Recipes[i]
				break
			}
		}
		if found == nil {
			c.Print("Recipe %q not found.", id)
			return
		}
		reqItem := p.Args[2]
		count := 1
		if len(p.Args) >= 4 {
			count, _ = strconv.Atoi(p.Args[3])
			if count <= 0 {
				count = 1
			}
		}
		updated := false
		for k := range found.Requires {
			if strings.EqualFold(found.Requires[k].Item, reqItem) {
				found.Requires[k].Count = count
				updated = true
				break
			}
		}
		if !updated {
			found.Requires = append(found.Requires, economy.Ingredient{Item: reqItem, Count: count})
		}
		c.Tell(protocol.ChanBuild, fmt.Sprintf("Recipe %s requires %s x%d.", found.ID, reqItem, count))

	case "add", "set":
		if len(p.Args) < 3 {
			c.Print("recipe add <id> <producesItem> [name <name>] [station <station>] [skill <skill>] [dc <n>] [min <n>]")
			return
		}
		id := p.Args[1]
		produces := p.Args[2]
		name := id
		station := ""
		skill := ""
		dc := 7
		minSkill := 0
		disc := true
		nameSet, stationSet, skillSet, dcSet, minSet, discSet := false, false, false, false, false, false

		args := p.Args[3:]
		for i := 0; i < len(args); i++ {
			switch strings.ToLower(args[i]) {
			case "name":
				var nameParts []string
				for i+1 < len(args) {
					next := strings.ToLower(args[i+1])
					if next == "station" || next == "skill" || next == "dc" || next == "difficulty" || next == "min" || next == "min_skill" || next == "disc" || next == "discovered" {
						break
					}
					nameParts = append(nameParts, args[i+1])
					i++
				}
				if len(nameParts) > 0 {
					name = strings.Trim(strings.Join(nameParts, " "), `"'`)
					nameSet = true
				}
			case "station":
				if i+1 < len(args) {
					station = args[i+1]
					stationSet = true
					i++
				}
			case "skill":
				if i+1 < len(args) {
					skill = args[i+1]
					skillSet = true
					i++
				}
			case "dc", "difficulty":
				if i+1 < len(args) {
					dc, _ = strconv.Atoi(args[i+1])
					dcSet = true
					i++
				}
			case "min", "min_skill":
				if i+1 < len(args) {
					minSkill, _ = strconv.Atoi(args[i+1])
					minSet = true
					i++
				}
			case "disc", "discovered":
				if i+1 >= len(args) {
					c.Print("disc is on or off. Discovery methods are not stored yet.")
					return
				}
				switch strings.ToLower(args[i+1]) {
				case "on", "true", "1", "yes":
					disc = true
					discSet = true
				case "off", "false", "0", "no":
					disc = false
					discSet = true
				default:
					c.Print("disc is on or off. Discovery methods are not stored yet.")
					return
				}
				i++
			}
		}

		var existing *economy.Recipe
		for i := range c.Pack.Recipes {
			if strings.EqualFold(c.Pack.Recipes[i].ID, id) {
				existing = &c.Pack.Recipes[i]
				break
			}
		}
		if existing != nil {
			existing.Produces = produces
			if nameSet {
				existing.Name = name
			}
			if stationSet {
				existing.Station = station
			}
			if skillSet {
				existing.Skill = skill
			}
			if dcSet {
				existing.Difficulty = dc
			}
			if minSet {
				existing.MinSkill = minSkill
			}
			if discSet {
				b := disc
				existing.Discovered = &b
			}
			c.Tell(protocol.ChanBuild, fmt.Sprintf("Recipe %s updated (produces: %s, station: %s).", existing.ID, existing.Produces, existing.Station))
			return
		}
		b := disc
		c.Pack.Recipes = append(c.Pack.Recipes, economy.Recipe{
			ID:         id,
			Name:       name,
			Produces:   produces,
			Count:      1,
			Station:    station,
			Skill:      skill,
			Difficulty: dc,
			MinSkill:   minSkill,
			Discovered: &b,
		})
		c.Tell(protocol.ChanBuild, fmt.Sprintf("Recipe %s created (produces: %s, station: %s).", id, produces, station))

	default:
		c.Print("recipe list | recipe show <id> | recipe add <id> <produces> ... | recipe req <id> <item> [count] | recipe del <id>")
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
