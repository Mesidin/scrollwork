# Building

You never recompile to add content. A pack is a folder of YAML, Markdown, and optional Lua.

In the manual, each heading below is its own page. Up and down move between those pages. Page Up and Page Down scroll the page.

There are two ways to work:

- **Files** (this page) — edit `games/<id>/` in a text editor, then `sudengine play <id>`.
- **In-engine** — `sudengine build <id>` and type `dig`, `desc`, `spawn`. Lua still lives in files.

Create a blank pack: `sudengine new mygame`

## File walkthrough

1. Copy `games/old-house` or start from `sudengine new`.
2. Edit `pack.yaml` (title, start room, intro blurb, which UI panes).
3. Edit `rpg.yaml` (what numbers exist: Grit, Calm, Blood, whatever the fiction needs).
4. List rooms in `world/rooms.yaml`, items in `world/items.yaml`, people in `world/npcs.yaml`.
5. Put player-facing manuals in `help/*.md` (`help house` loads `help/house.md`).
6. Optional: `intro.txt` ASCII art, `scripts/*.lua` for unique behavior.
7. Run `sudengine play <id>` and walk it. Fix YAML. Repeat.

Room IDs are strings (`house.kitchen`), not numbers. Set `x`, `y`, `z` on rooms if you want the automap.

## Pack layout

```
games/<id>/
  pack.yaml           id, title, tick, start room, ui panes, intro
  lexicon.yaml        labels (hp → Grit) and extra command words
  rpg.yaml            attributes, resources, races, roles, chargen
  intro.txt           ASCII art for the intro screen
  world/rooms.yaml
  world/items.yaml
  world/npcs.yaml
  abilities/*.yaml    extra verbs (steady, rites, hacks)
  scripts/*.lua
  help/*.md
```

## Rooms

```
- id: house.kitchen
  name: Kitchen
  long: A black iron stove and a dry sink.
  x: 1
  y: 0
  z: 0
  flags: [indoor]
  items: [house.lamp]
  npcs: []
  exits:
    west: { to: house.foyer }
```

Doors: `door: true`, `locked: true`, `key: house.cellar-key`. Dark rooms: `flags: [dark]` — the player needs a light (`use lamp`).

Weather flags, when the pack turns the clock or the sky on: `outdoor` (or `outdoors`) hears the open-air lines. `windowed` (or `sheltered`) hears the quieter lines and, at time scale 2, lets daylight into a dark room. `underground` ignores the sky. `lit` keeps an outdoor room bright at night. See `help systems`.

## Items

Takeable things, clothes, keys, and usable gear.

```
- id: house.lamp
  keywords: [lamp, lantern]
  name: brass lamp
  short: a brass lamp
  long: The wick still looks willing.
  takeable: true
  use:
    toggle_flag: light
    message: You light the lamp.
    message_off: You douse the lamp.
```

Players: `get lamp`, `use lamp` (or `light lamp`), `wear coat`, `inventory`.
See `help items`.

## People

```
- id: house.maid
  keywords: [maid, marta]
  name: the maid
  short: a tired maid
  long: She will not look at the cellar door.
  ai: { profile: sentinel }
  topics:
    key: I dropped it in the parlor.
```

AI profiles: `sentinel` (stays), `wander` (moves), `aggressive` (attacks), `coward` (runs when hurt). An aggressive mobile stays in its room unless you also set `wander: true`.

`rooms` keeps both wander and pursuit inside those room ids. `aggro: look` waits until the player examines it. `aggro: flag` waits until the `hostile` flag is set (from `set_flag` in Lua, or any other story event). `pursue: true` follows the player through an open exit and stops at the edge of `rooms`.

```
ai:
  profile: aggressive
  wander: true
  rooms: [house.wine, house.cellar]
  pursue: true
```

A creature that should not attack the moment you walk in:

```
ai:
  profile: aggressive
  aggro: look
  pursue: true
  rooms: [house.cistern, house.cellar]
```

Combat block is optional. `ask maid about key` uses `topics`.

An attack may set `hit` and `miss` lines. `{Name}` is the attacker, `{name}` the target, `{damage}` the amount, `{resource}` the bar's label. `hit_room` and `miss_room` are that same swing for everyone else in the room. The fighter still sees `hit` and `miss`. Leave the room lines out and bystanders get a plain third-person sentence. A weapon uses the same four fields. `skill: melee` adds damage when that skill has `damage_every`. `oppose.escape` and `oppose.hit` are the difficulties this mobile imposes. `xp` is awarded if the pack turned advancement on. `trainer: true` marks someone who sells skill ranks.

## Checks, noticing, advancement

See `help systems` for the roll, hidden things, and levels. Short version: omit `checks`, `feedback`, and `advancement` and the game does not roll, does not print brackets, and does not level. A worn item's `mods` map feeds any check that lists that key under `gear`. Hidden extras and items use `notice` and `search`.

`stats`, `inventory`, and `equipment` are the sheet. After a level, `improve` and `raise` spend points. `train` pays a trainer.

## Systems (rpg.yaml)

The engine has no baked-in D&D. This pack defines named resources and traits. `lexicon.yaml` is what the player reads (hp → Grit). See `help systems`.

## Lua (optional)

Most content needs zero Lua. For a whispering portrait or a boss phase, add `scripts/foo.lua` and set `scripts: foo.lua` on the entity.

Sandbox: no `io` or `os`. Hooks: `on_look`, `on_use`, `on_ask`, `on_enter`, `on_leave`, `on_death`, `on_tick`, `on_quest_complete`. Builtins: `echo`, `echo_room`, `teleport` (or `move`), `damage`, `set_flag`, `learn_recipe`, `has_quest`, `is_quest_completed`, `is_quest_active`. `on_quest_complete` runs after the quest is marked completed and before `relocate`, so `echo_room` still reaches the room the player is standing in. `has_quest(id, [who])` is true when that quest is active or completed. `is_quest_completed` and `is_quest_active` take the same optional who (an id, prototype, or keyword). `teleport(who, where, [update_home])` moves a mobile, item, or the player into a room. `update_home` sets the respawn room and adds it to a restricted leash; leaving it off is a one-shot move. Return `true` from `on_use` / `on_ask` to skip the YAML default. Then `reload` in build mode.

## Settings and rules

The builder edits the world: rooms, exits, descriptions, prototypes, and who is standing in a room. It does not edit the rules of the whole game.

Change those in a text editor, then start the session again.

- `pack.yaml` — title, author, tick, start room, intro, which panes show in play
- `lexicon.yaml` — the words a player reads (hp shown as Grit)
- `rpg.yaml` — resources, attributes, roles, character creation
- `abilities/*.yaml` — extra verbs such as steady
- `help/*.md` — the game manual
- `scripts/*.lua` — special behavior

`save pack` writes rooms, items, and people. It also rewrites `pack.yaml`, `lexicon.yaml`, and `rpg.yaml` from the copy loaded at startup, so comments in those files can disappear. It leaves `help/`, `intro.txt`, and Lua alone.

## In-engine commands

`sudengine build <id>`. The side pane shows a map centered on the room you are in (`@`), then the commands you type most often.

```
dig north Kitchen     create + link + walk
buildwalk             walking into a void digs
name <title>          set current room title
desc                  then type lines, end with .
desc <one line>       set room description directly
rflags dark           toggle room flags (dark, outdoor, lit, underground, windowed)
extra add portrait | The eyes follow you.
proto item lamp a brass lamp
proto npc rat a rat
spawn rat             instantiate mobile in current room
goto house.foyer      teleport to room
rooms                 list all world rooms
save pack             write world YAML and recipes
reload                reread Lua scripts
```

## Mobile builder (mset)

Configure mobiles and prototypes directly in-engine:

```
mset <mob> show
mset <mob> ai <profile> [wander on|off]   set AI profile (sentinel, wander, aggressive, coward)
mset <mob> trainer <on|off>           enable skill trainer service
mset <mob> repairer <on|off>          enable equipment repair service
mset <mob> banker <on|off>            enable vault deposit/withdraw service
mset <mob> xp <amount>                experience awarded on defeat
mset <mob> respawn <ticks>            automatic respawn interval (0 = off)
mset <mob> shop currency <id>         merchant coin currency ID
mset <mob> shop purse <amount>        merchant active till
mset <mob> shop maxpurse <amount>     merchant maximum purse capacity
mset <mob> shop dynamic <on|off>      finite till that refills as players buy
mset <mob> shop curve <on|off>        floating supply-and-demand price curves based on stock
mset <mob> shop clear                 remove merchant shopkeeper role
mset <mob> topic <key> <text>         dialogue topic response
mset <mob> teach <topic> <recipe>     teach crafting recipe on dialogue topic
mset <mob> loot add <item> [chance] [count] [skin]   add loot drop or skinning yield
mset <mob> loot list                  display mobile loot table
mset <mob> loot clear                 wipe all loot drops
```

## Item builder (iset)

Inspect and configure item properties, combat stats, durability, and survival tools:

```
iset <item> show
iset <item> use <flag>                toggle use flag effect
iset <item> damage <dice>             weapon damage dice (e.g. 1d6+1, slot wield)
iset <item> armor <soak>              armor soak value (slot body)
iset <item> slot <body|head|wield...> equipment slot
iset <item> value <coins>             base economic value in coins
iset <item> durability <n>            maximum and current durability
iset <item> repairable <on|off>       allow repair at blacksmith/artisan
iset <item> banker <on|off>           mark container as vault bank
iset <item> kit <uses> [amount] [skill] [dc]   field repair kit configuration
iset <item> kit clear                 clear repair kit properties
iset <item> salvage add <item> [count]          add salvage yield material
iset <item> salvage list              show salvage yields
iset <item> salvage clear             clear salvage yields
```

## Gathering builder (gset)

Configure resource nodes on rooms, scenery extras, or standalone items:

```
gset show                             display gathering configuration for room
gset clear                            remove gathering node from room
gset room item <id> [max] [regen] [skill] [dc] [bonus_dc]
                                      attach resource node to current room
gset node <extra|item> [max] [regen] [skill] [dc] [bonus_dc]
                                      attach resource node to scenery extra or item
```

## Recipe builder (recipe)

Manage the pack's `recipes.yaml` crafting registry:

```
recipe list / rclist                  show all registered crafting recipes
recipe show <id>                      display recipe requirements, station, DC, and discovery
recipe add <id> <item> [name <name>] [station <s>] [skill <s>] [dc <n>] [min <n>] [disc <grant|buy|discover|all|on|off>]
                                      create a recipe, or change only the fields you name
recipe req <id> <item> [count]        add required ingredient to recipe
recipe del <id>                       delete recipe from pack
```

`disc` sets how a character learns the recipe. `all` (or `on`) means the recipe starts known. `grant`, `buy`, or `discover` (or `off`) locks it until the character learns it. A quest reward or an NPC topic records `grant`. Using an item with `use.learn_recipe` records that recipe's method (`buy`, `discover`, or `grant`). Buying the finished good, or carrying the manual without using it, does not teach the recipe.

## Check your work

Validate your pack with `sudengine validate <pack>`. The validator performs comprehensive static integrity checks:
- **Graph reachability**: All rooms must be reachable from `start_room` (via exits, quest relocations, or a whole room id in a script).
- **Key & lock integrity**: A required key must be an item the player can reach: in a room, inside a placed container that this key does not itself lock, on a placed NPC's stock or loot, from a quest, recipe, or gather node, or as a whole id in a script. Opposite sides of a door must use the same key when both sides set one. An item whose id is `key` or ends in `.key`, `_key`, or `-key` must unlock something or appear as a whole id in a script.
- **Script references & syntax**: Every script referenced by a room, scenery extra, item, NPC, or quest must exist in `scripts/`, and all Lua files are parsed to verify valid syntax.

Play it: `sudengine play <pack>`. `look`, `get`, `use`, `ask`, `kill`. `help` should list your `help/*.md` under Game. When it feels right, zip the folder — that folder is the game.
