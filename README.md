# Sudengine

A single-player MUD / Zork-style engine: rooms, objects, NPCs, real-time combat, pack-defined RPG stats, Lua hooks, in-engine building, and a multi-pane TUI.

**Naming (working):** games ship under **Erickson Stories**. The engine repo, Go module, and binary stay **sudengine** until a rename is worth the churn. The launcher shows Erickson Stories; `help building` is still the pack-author guide.

Content is not compiled into the binary. A game is a folder of YAML, Markdown, and optional Lua.

## Run

Go 1.22+ (developed on 1.27). Run these commands from the project directory so the engine can find `games/`.

```bash
go test ./...
```

On macOS and Linux, build a binary named `sudengine`:

```bash
go build -o sudengine ./cmd/sudengine

./sudengine                 # launcher
./sudengine play old-house      # story sample
./sudengine play green-hollow   # fantasy sample: levels, training, checks
./sudengine build old-house # in-engine workshop
./sudengine new mygame      # blank pack under games/mygame
./sudengine validate old-house
./sudengine install path/to/pack.zip
```

On Windows, build `sudengine.exe`. Windows runs a program when its name ends with an extension from `PATHEXT`, which includes `.exe`. A file named `sudengine` has no extension, so Windows asks which app should open it. Close that dialog. PowerShell, Windows Terminal, and Alacritty all run the game when you type the command in the shell:

```powershell
go build -o sudengine.exe ./cmd/sudengine

.\sudengine.exe                 # launcher
.\sudengine.exe play old-house      # story sample
.\sudengine.exe play green-hollow   # fantasy sample: levels, training, checks
.\sudengine.exe build old-house # in-engine workshop
.\sudengine.exe new mygame      # blank pack under games/mygame
.\sudengine.exe validate old-house
.\sudengine.exe install path/to/pack.zip
```

`go run` starts the engine on either platform and does not write a binary:

```bash
go run ./cmd/sudengine
go run ./cmd/sudengine play old-house
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

`help` opens a manual. Game rules and engine guides (playing, building, commands) are listed apart. A long guide such as building is split into sections. Page Up and Page Down scroll the page. The intro Help menu opens the same manual.

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
| `games/old-house` | story sample |
| `games/green-hollow` | fantasy sample (levels, training, checks) |

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

1. **CI** — `go test` on push.
2. **Multiplayer** — listen on the existing session protocol; accounts later.
3. **Name** — keep protocol/pack keys generic so “Erickson Stories” vs `sudengine` is a display/module change, not a content rewrite.
4. **Richer OLC** — comment-preserving YAML; shops in the sample pack if a story wants one.

## Docs map

| Audience | Where |
|----------|--------|
| Players / builders in-game | `help <topic>` (engine + pack Markdown, TUI-formatted) |
| Pack authors | `internal/docs/engine/building.md` (same text as `help building`) |
| Humans on GitHub | this README |
| Coding agents | [`AGENTS.md`](AGENTS.md) |

## License

[MIT](LICENSE) © 2026 Bradley Erickson.
