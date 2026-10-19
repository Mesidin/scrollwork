# Multiplayer development guide

Sudengine is a single-player engine today. The TUI talks to an in-process server through `internal/protocol`. Multiplayer binds that same server to a port. UI types stay out of the server.

This file is the build order. The roadmap checklist in section 7 matches these stages. Do not start a later stage until the earlier stage's done check passes.

`play <pack>` stays an in-process, offline game. `serve` is a later command. Headless tests keep calling `commands.Dispatch` on a context for one actor. `go test ./internal/server` must still walk `games/old-house` after every stage.

## What the code does now

`Server` keeps a session map. `play` puts one local session in it. `Run` selects the clock and that session's `In`. `Session.Out` is a buffered channel of 128 events. Events that do not fit wait in a side queue. Vitals, map, combat, and prompt coalesce only there. `Send` does not read `Out`. Room text and command results are not dropped. Two ticks with `Out` still full disconnect that session. `Run` returns when the local session leaves.

`commands.Context` is built for that one session. `Actor` comes from `sess.PlayerID`, but several server paths still call `World.Player()`: combat start, assists, Lua `echo` / `echo_room`, `moveEntity`, `pushRoom`, `reapDead`, and companion recall. Combat `Emit` and script echoes deliver text only when the target is that one player.

`pushRoom` already lists both `KindMobile` and `KindPlayer` occupants, and it already skips items in a dark room. A second occupant renderer is not needed.

Saves are full JSON snapshots, `saves/<pack>/<slot>.json`, written by `save.Write`. There is no migration. A new game is not an old save.

`protocol.Event` is an interface. The concrete events (`TextEvent`, `RoomEvent`, `VitalsEvent`, and the rest in `internal/protocol/protocol.go`) have no JSON tags and no type field. A remote client cannot decode a bare `json.Marshal` of the interface.

`world.Opposite` maps the canonical directions, including `n`/`north`. Party membership, follower pull, and pet kill credit already exist. Player death already follows `rpg.yaml`: drop or keep items (`Death.PlayerKeepItems`), restore a percent of the primary resource, recall to `StartRoom`, and recall owned companions.

## Modes

| Command | When | What it does |
|---|---|---|
| `sudengine play <pack>` | Now, and after every stage | In-process TUI. One local session. Chargen stays on the client, then `SpawnPlayer`. |
| `sudengine serve <pack> [--port 4000]` | Stage 3 | Headless play-only server. No builder verbs. Listens on localhost until accounts exist. |
| `sudengine connect <host:port>` | Stage 3 | Bubble Tea client over the framed protocol. |
| Telnet on port 4023 | Stage 4 | ANSI rendering of the same events for Mudlet, TinTin++, and similar clients. |

`serve` does not load build mode. OLC stays on local `build`.

## The actor loop

Network reads run on the connection goroutine. They do not touch `world.World`, the RNG, or Lua. They enqueue a message. The tick loop is the only goroutine that mutates the world.

Connect, disconnect, and input are messages on one server channel, the same channel `Run` selects. The session map is touched only on that loop. There is no second lock around the world.

Each tick handles at most one queued command per session, then the heartbeat (weather, combat, AI, respawn). A fast client must not drain the queue ahead of the clock.

The world is single-threaded. Arrival order from the network is not deterministic, and the guide does not claim that it is.

`Session.Out` stays the write queue. Coalesce UI pushes (`VitalsEvent`, `MapEvent`, `CombatEvent`, `PromptEvent`) in the side queue: a newer push replaces one still waiting there. Do not read `Out` from the server. Room text and command results are not dropped. If `Out` stays full for two ticks, disconnect that session. Leave the world running unless the local session is the one that stalled or quit.

## Stage 1 — Who hears an event

Goal: two players in one process see the same room. No sockets.

Add `sendTo(player, event)` and `sendRoom(room, except, event)`. The local session is the only entry in the session map. `play` and the Old House tests use that one entry.

Build `commands.Context` from the session that owns the command (`ctx(sess)`), and pass that actor into combat start, assists, movement, death, and Lua callbacks. `World.Player()` stays for single-player tests and for code that has not moved yet. New server, combat, and script paths take the acting entity. Do not look up "the player" with `World.Player()` on a path that can run for someone else.

`pushRoom`, `pushVitals`, `pushInv`, `pushMap`, and `pushCombat` take the player they describe. Each player in a room gets their own `RoomEvent` (they do not see themselves; darkness is already per room). Other players in the occupant list get a short suffix: ` (fighting)` or ` (linkdead)`. Items and scenery stay as they are.

Movement uses `world.Opposite`. The mover gets the normal arrival. Others in the room they left get `leaves <dir>`. Others in the room they entered get `arrives from <opposite>`. Followers already move with `moveFollowers`. A following player gets the normal room entry, once.

`say <text>` and `'<text>` go to `ChanSay` for every player in the room, including the speaker. `emote` / `:<text>` go to `ChanNarrative` the same way. Leave and arrive lines are the other Stage 1 broadcast.

Lua `echo` goes to that actor. `echo_room` goes to every player in the room except `skip`. Combat text is one room message: the actor does not also get a private copy of the same line. Player swings stay `ChanNarrative`. Enemy hits stay `ChanCombat`. A hostile arriving stays `ChanAlert`.

### Stage 1 done

- `go test ./internal/server` passes, including the Old House playthrough.
- A new test puts two sessions in one world. One moves, the other sees the leave and the arrive. One says, the other hears it. A dark room still hides items. An event for session A is not delivered to session B in another room.

## Stage 2 — Queue and a few verbs

Replace the single `case line := <-s.Session.In` with the server command channel. The local TUI and tests deliver input as a message carrying the session. `Dispatch` still accepts a context whose `Actor` is that session's player, so existing tests do not need a socket.

Add `tell <player> <text>`, `who`, and `gossip <text>` (alias `ooc`). `tell` is private `ChanSay` to the target, plus an echo to the speaker. `gossip` is `ChanSystem` to every logged-in player. `who` lists connected characters with the pack's role name and primary resource. There is no engine level stat. Do not print a level column.

`party <text>` (alias `gt`) talks to the current party, wherever they are. It uses `PartyLeader`. It does not invent a second group.

`whisper`, `shout`, and room `pvp` wait until after Stage 4. `kill <player>` refuses with a calm line everywhere until a room flag exists. That refusal can land in Stage 2 so two test characters cannot cut each other down by accident.

### Stage 2 done

- One command per session per tick is covered by a test that queues two lines and sees the second wait.
- `tell` does not leak to a third session. `who` lists both characters. `gossip` reaches both rooms.

## Stage 3 — Framed protocol

`sudengine serve <pack>` loads the pack, validates it, and listens on `127.0.0.1:4000` unless `--port` is set. It does not open a public address. `sudengine connect <host:port>` is the Bubble Tea client. Chargen for `connect` can send the same race and role the local launcher already collects. The server then calls `SpawnPlayer`. Full server-driven chargen is Stage 5.

Each frame is a length prefix plus a JSON object:

```json
{"t":"text","channel":"say","text":"Ada says, \"Hello.\""}
```

`t` is one of `text`, `prompt`, `room`, `vitals`, `inventory`, `map`, `combat`, `layout`, `mode`, `title`, `disconnect`. Add `json` tags on the protocol structs when the encoder is written. The in-process channel can keep passing the Go values. The encoder lives next to the listener (`internal/net`), not inside the tick loop.

Room descriptions contain newlines, so a newline-delimited stream is the wrong frame. One frame, one event.

The remote client renders the panes it already renders locally. It does not grow a second protocol.

### Stage 3 done

- A test listener accepts one framed connection, sends a `room` event, and reads one command frame back onto the actor loop.
- `connect` to localhost shows the same room `play` shows for that pack.
- `serve` rejects build mode.

## Stage 4 — Telnet

Telnet is a lossy view of the same events, on port 4023. It is not a second game loop.

Read with a small IAC-aware scanner. `bufio.Scanner` splits on newlines and will break on Telnet negotiation bytes. During password entry (Stage 5), send `IAC WILL ECHO` so the client stops local echo, and do not write those bytes back. After login, restore client echo.

Map events to ANSI-16 the way `internal/theme` already does:

| Event | Telnet output |
|---|---|
| `room` | Title, description, exits, items, occupants |
| `vitals` / `prompt` | A one-line prompt |
| `text` | The line, colored by channel |
| `map`, `combat`, `inventory` | Omitted, or a one-line summary when the player asks |

### Stage 4 done

- A raw TCP test sends `look\n` and receives the room title.
- An IAC sequence in the byte stream is consumed and is not dispatched as a command.

## Stage 5 — Accounts, saves, link-dead

Login is an account, then a character. The character name is not the password identity. Hash passwords with bcrypt (`golang.org/x/crypto/bcrypt`). One account can own more than one character.

The live world stays one entity tree. Players are entities in rooms, and their inventory is children of those entities, as it is today. The on-disk cut is taken from that tree:

```
saves/<pack>/
  <slot>.json                         # unchanged single-player snapshot
  accounts/<account_id>.json          # username, password hash, created time
  players/<character>.json            # that player's entity subgraph
  world.json                          # the shared tree with player entities removed
```

Write `players/*.json` and `world.json` to temp files and rename them together. A crash between the two must not leave a coin in both places or in neither. `play` keeps using `save.Write` and the slot JSON. Do not read an old slot as an account, and do not rewrite old snapshots.

Autosave on an interval and on shutdown uses that same paired write. Player death uses the existing `rpg.yaml` death rules for that player: items drop or stay, the body recalls to `StartRoom`, companions recall. Credit and XP for a mob go to party members who are in the room when it falls, and the existing pet and companion credit stays.

A dropped socket sets the character link-dead and leaves them in the room and in the fight. `quit` during combat starts that same grace period. The default grace is 60 seconds. When it ends, save and despawn. If they die during the grace, the death rules run first, then the save stores them at the start room.

Server-driven chargen for Telnet reuses `rpg.yaml` bundles, origins, roles, and point buy. The local `play` launcher can keep doing chargen before it attaches.

### Stage 5 done

- Two characters save and load without either file containing the other's inventory.
- A failed second write leaves the previous pair in place.
- An old `saves/<pack>/<slot>.json` still loads in `play`.
- A link-dead character is still in the room on the next tick, and is gone after the grace period.

## Combat and PvP, after Stage 4

Several players can fight one mobile. The room sees each swing once. Quest kill credit and XP go to party members in the room when the mobile dies. Someone in another room gets neither. Pet and companion credit is unchanged.

Default play refuses `kill <player>`. A later pack flag `pvp` on a room allows it. An open-wilderness stance waits until a pack asks for it. One switch is enough until then.

## Chat, by stage

| Command | Stage | Who hears it |
|---|---|---|
| `say`, `'`, `emote`, `:` | 1 | Players in the room |
| Leave / arrive | 1 | Players in the rooms on either side of the move |
| `tell` | 2 | Speaker and target |
| `who` | 2 | The asker |
| `gossip`, `ooc` | 2 | Every connected player |
| `party`, `gt` | 2 | Current party members |
| `whisper` | later | One player in the same room |
| `shout`, `yell` | later | Players in adjacent rooms that are outdoor |

`shout` uses the outdoor flag the weather system already understands. Adjacent means one exit step.

## Not in these stages

- WebSocket or a browser client. Add it after the framed protocol is stable.
- A public bind before accounts.
- Builder verbs on `serve`.
- Rewriting single-player saves into the account layout.
- A second party system beside `PartyLeader`.
- Baked-in levels, classes, or PvP balance. Packs keep supplying those names and numbers.
