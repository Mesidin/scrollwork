# Agent guide — Sudengine

This file is for coding agents (and humans pairing with them). Read it before changing the engine.

## What you are working on

A Go MUD engine with a Bubble Tea TUI. Games are **packs** (YAML + Markdown + optional Lua) loaded at runtime. There is no content compiler.

Sample pack: `games/old-house`. Treat it as the regression world. `go test ./internal/server` walks it.

## Commands

```bash
go test ./...
go test ./internal/server ./internal/pack ./internal/docs
go build -o sudengine ./cmd/sudengine
./sudengine play old-house
./sudengine build old-house
```

Do not commit `sudengine`, `saves/`, or `.DS_Store`. They are gitignored.

## Layout

```
cmd/sudengine/          main
internal/protocol/      session events only — no Bubble Tea types
internal/server/        tick, dispatch, UI push
internal/client/        TUI (launcher, intro, chargen, game)
internal/world/         entity tree
internal/commands/      play verbs + parser
internal/olc/           build verbs
internal/pack/          pack load/write
internal/rpg/           schema, bundles, abilities
internal/combat/
internal/script/        gopher-lua sandbox
internal/theme/         ANSI-16 + Omarchy
internal/docs/engine/   embedded manuals (help <topic>)
games/<id>/             a game
```

## Locked decisions (do not casually undo)

- Parser: MUD verb-first (`get lamp`, `n`), not Infocom English.
- Time: real-time heartbeat; combat auto-attacks.
- Language: Go engine, Lua for pack specials, YAML for data.
- RPG: engine is a skeleton (keys, resources, abilities). Packs supply names and numbers. No baked-in D&D.
- Saves: full world snapshot, not pack-delta.
- Architecture: TUI talks to an in-process server via `protocol`. Future multiplayer binds that server to a port; do not leak UI types into the server.
- IDs: strings (`house.kitchen`), not vnums.
- Lua: no `io` / `os` / `dofile`. Hooks return `true` to consume `on_use` / `on_ask`.

## Where to change what

| If you want to… | Touch |
|-----------------|--------|
| Add a play command | `internal/commands`, parser aliases, `internal/docs/engine/commands.md` |
| Add a builder command | `internal/olc` |
| Change look/combat copy | `commands`, `combat`, `server` AI — keep player swings narrative, enemy hits `ChanCombat`, hostiles arriving `ChanAlert` |
| Change TUI layout / intro | `internal/client` |
| Change colors | `internal/theme` (ANSI defaults + Omarchy overlay) |
| Change engine manuals | `internal/docs/engine/*.md` (embedded; `docs.Format` strips Markdown for the TUI) |
| Change the sample game | `games/old-house/**` only |
| Change the session contract | `internal/protocol` and **every** producer/consumer |

Player-facing help is Markdown. After editing `internal/docs/engine/*.md`, `help <topic>` picks it up on next binary build (embed). Pack `help/*.md` is picked up on pack load (no rebuild).

## Content vs engine

Never put Old House fiction, Grit/Calm labels, or room text in Go. Those belong in the pack (`lexicon.yaml`, `rpg.yaml`, `world/`, `help/`). The engine may print generic words (`help`, `look`) and pack labels via the lexicon.

`save pack` round-trips YAML (comments may be lost). It must not rewrite Lua.

## Tests

- Parser, world graph, dice, Lua sandbox, pack load, docs format, Old House playthrough (`internal/server/play_test.go`).
- After world/command/combat/pack changes, run `go test ./internal/server ./internal/pack ./internal/commands`.
- Headless tests drain `Session.Out` before asserting on `help` (the tick loop can fill the buffer).
- No browser. Verification is `go test` plus playing the TUI.

## Style

- Match neighboring code. Small files, no unused exports.
- Protocol events stay JSON-tag friendly.
- Do not add Infocom parsing, networking, or a Lua editor in the TUI unless the user asked.
- Do not “upgrade” snapshots when a pack changes; document that new game ≠ old save.

## Gaps (not implemented — don’t assume they exist)

- CI on push (run `go test ./...` locally).
- Crafting, weather, economy beyond a shop NPC.
- Networked multiplayer.
- Infocom-style parser.
- YAML comment preservation on `save pack` (help/intro/Lua are left alone).

## In-game docs vs this file

`help building` / `internal/docs/engine/building.md` teaches **pack authors**.

This file teaches **people changing the Go engine**.
