# Building

You never recompile to add content. A pack is a folder of YAML, Markdown, and optional Lua.

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

AI profiles: `sentinel`, `wander`, `aggressive`, `coward`. Combat block is optional. `ask maid about key` uses `topics`.

## Systems (rpg.yaml)

The engine has no baked-in D&D. This pack defines named resources and traits. `lexicon.yaml` is what the player reads (hp → Grit). See `help systems`.

## Lua (optional)

Most content needs zero Lua. For a whispering portrait or a boss phase, add `scripts/foo.lua` and set `scripts: foo.lua` on the entity.

Sandbox: no `io` or `os`. Hooks: `on_look`, `on_use`, `on_ask`, `on_enter`, `on_leave`, `on_death`, `on_tick`. Return `true` from `on_use` / `on_ask` to skip the YAML default. Then `reload` in build mode.

## In-engine commands

`sudengine build <id>`:

```
dig north Kitchen     create + link + walk
buildwalk             walking into a void digs
name / desc / rflags
proto item lamp a brass lamp
proto npc rat a rat
spawn rat
goto house.foyer
rooms
save pack             write YAML back to disk
reload                reread Lua
```

`save pack` may drop comments in YAML. Lua files are never rewritten.

## Check your work

Play it. `look`, `get`, `use`, `ask`, `kill`. `help` should list your `help/*.md` under Game. When it feels right, zip the folder — that folder is the game.
