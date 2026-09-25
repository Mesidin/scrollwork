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

An attack may set `hit` and `miss` lines. `{Name}` is the attacker, `{name}` the target, `{damage}` the amount, `{resource}` the bar's label. `skill: melee` adds damage when that skill has `damage_every`. `oppose.escape` and `oppose.hit` are the difficulties this mobile imposes. `xp` is awarded if the pack turned advancement on. `trainer: true` marks someone who sells skill ranks.

## Checks, noticing, advancement

See `help systems` for the roll, hidden things, and levels. Short version: omit `checks`, `feedback`, and `advancement` and the game does not roll, does not print brackets, and does not level. A worn item's `mods` map feeds any check that lists that key under `gear`. Hidden extras and items use `notice` and `search`.

`stats`, `inventory`, and `equipment` are the sheet. After a level, `improve` and `raise` spend points. `train` pays a trainer.

## Systems (rpg.yaml)

The engine has no baked-in D&D. This pack defines named resources and traits. `lexicon.yaml` is what the player reads (hp → Grit). See `help systems`.

## Lua (optional)

Most content needs zero Lua. For a whispering portrait or a boss phase, add `scripts/foo.lua` and set `scripts: foo.lua` on the entity.

Sandbox: no `io` or `os`. Hooks: `on_look`, `on_use`, `on_ask`, `on_enter`, `on_leave`, `on_death`, `on_tick`. Return `true` from `on_use` / `on_ask` to skip the YAML default. Then `reload` in build mode.

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
name <title>
desc                  then type lines, end with .
desc <one line>
rflags dark
extra add portrait | The eyes follow you.
ai rat aggressive
iset lamp use light
iset knife damage 1d6+1
proto item lamp a brass lamp
proto npc rat a rat
spawn rat
goto house.foyer
rooms
save pack             write world YAML (not help, intro, or Lua)
reload                reread Lua
```

`save pack` rewrites world YAML (comments in those files may be lost). It does not rewrite `help/`, `intro.txt`, or Lua.

## Check your work

Play it. `look`, `get`, `use`, `ask`, `kill`. `help` should list your `help/*.md` under Game. When it feels right, zip the folder — that folder is the game.
