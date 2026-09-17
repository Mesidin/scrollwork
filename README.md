# Sudengine

A single-player MUD / Zork-style engine: rooms, objects, NPCs, real-time combat, pack-defined RPG stats, Lua hooks, in-engine building, and a multi-pane TUI.

**Naming (working):** games ship under **Erickson Stories**. The engine repo, Go module, and binary stay **sudengine** until a rename is worth the churn. The launcher shows Erickson Stories; `help building` is still the pack-author guide.

Content is not compiled into the binary. A game is a folder of YAML, Markdown, and optional Lua.

## Run

Go 1.22+ (developed on 1.27).

```bash
go test ./...
go build -o sudengine ./cmd/sudengine

./sudengine                 # launcher
./sudengine play old-house  # sample pack
./sudengine build old-house # in-engine workshop
./sudengine new mygame      # blank pack under games/mygame
```

Packs live in `games/<id>/`. Snapshots live in `saves/<pack-id>/` (gitignored).

## Play (Old House)

Intro screen: New game (name + temperament), Continue if saves exist, Help.

```
n                    foyer
ask maid about key
n                    parlor
get key
get coat
wear coat
s
e                    kitchen
get lamp
use lamp             (or: light lamp)
w
unlock down
open down
d
stats                Grit / Calm
help systems
help building
```

In-game manuals: `help`, `help playing`, `help systems`, `help items`, `help building`. The same files appear on the intro Help menu.

## Architecture

One process, two sides. The TUI never mutates the world; it sends input and renders events. That is the seam for later multiplayer.

```
TUI (Bubble Tea)  ← session events →  game server (tick, parser, world, Lua)
                                              ↓
                                   pack on disk (YAML + Lua)
                                   snapshot saves
```

| Path | Role |
|------|------|
| `cmd/sudengine` | CLI + TUI entry |
| `internal/protocol` | client ↔ server events |
| `internal/server` | tick loop, dispatch |
| `internal/world` | entity tree (rooms / items / mobiles) |
| `internal/commands` | MUD parser + play verbs |
| `internal/olc` | builder verbs |
| `internal/pack` | load / write packs |
| `internal/rpg` | generic attrs, resources, abilities |
| `internal/combat` | rounds, damage |
| `internal/script` | sandboxed Lua |
| `internal/theme` | ANSI-16 + Omarchy overlay |
| `internal/docs/engine` | engine manuals (embedded) |
| `games/old-house` | sample pack |

Locked v1 choices: MUD commands (not Infocom English), real-time ticks, Go + Lua, full-world snapshots, engine RPG skeleton only (packs name Grit vs HP), local client/server from day one.

## Content

Do not recompile to add rooms. See **`help building`** in the TUI, or `internal/docs/engine/building.md`.

Short version: edit `games/<id>/`, then play. Lua is optional and sandboxed (no `io` / `os`).

## Theming

Default styles are ANSI-16 (follow the terminal, including Omarchy retints). Optional hex overlay:

- `~/.local/state/omarchy/current/theme/colors.toml`
- `~/.local/state/omarchy/current/theme/sudengine.toml`
- `~/.config/sudengine/theme.toml`
- `SUDENGINE_THEME=/path/to.toml`
- `NO_COLOR=1`

## Next steps

Rough order, not a promise:

1. **Packs as products** — zip/install a folder; validate YAML with useful errors; keep Old House as the regression pack.
2. **Builder** — multiline `desc`, edit extras/AI/use from OLC, don’t clobber Lua or comments more than we have to.
3. **Play loop** — weapons/armor in the damage pipeline, combat grammar, corpse/loot rules in data, shops/dialogue beyond `ask`.
4. **Character** — persist origin/role on the player; `stats` should show what you picked.
5. **CI** — `go test` on push.
6. **Multiplayer** — listen on the existing session protocol; accounts later.
7. **Name** — keep protocol/pack keys generic so “Erickson Stories” vs `sudengine` is a display/module change, not a content rewrite.

## Docs map

| Audience | Where |
|----------|--------|
| Players / builders in-game | `help <topic>` (engine + pack Markdown, TUI-formatted) |
| Pack authors | `internal/docs/engine/building.md` (same text as `help building`) |
| Humans on GitHub | this README |
| Coding agents | [`AGENTS.md`](AGENTS.md) |

## License

[MIT](LICENSE) © 2026 Bradley Erickson.
