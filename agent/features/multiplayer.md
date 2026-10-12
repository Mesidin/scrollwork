# Feature Specification: Networking & Multiplayer Architecture

## 1. Overview & Architectural Goals

Sudengine was originally built as a single-player MUD engine where an in-process server communicates with a Bubble Tea terminal client over typed protocol events (`internal/protocol`). 

As stated in `AGENTS.md`:
> *"Architecture: TUI talks to an in-process server via protocol. Future multiplayer binds that server to a port; do not leak UI types into the server."*

The goal of this specification is to pre-plan the complete architecture for turning Sudengine into a multi-user dungeon (MUD), supporting:
1. **Multiple concurrent players** roaming the same persistent world.
2. **Dual execution modes**:
   - `sudengine play <pack>`: In-process, single-player offline play (100% backwards compatible).
   - `sudengine serve <pack> [--port 4000] [--telnet 4023]`: Headless dedicated server hosting the world.
   - `sudengine connect <host:port>`: Networked Bubble Tea client connecting to a remote server.
3. **Multi-client connectivity**:
   - **Telnet / Raw TCP**: Standard MUD clients (Mudlet, TinTin++, terminal netcat) via ANSI text stream.
   - **Structured Protocol Stream**: JSON-framed `protocol.Event` stream powering remote Bubble Tea TUI or future web clients.
4. **Deterministic single-threaded game loop**:
   - Network I/O is asynchronous on dedicated connection goroutines.
   - Command dispatch and world mutation remain strictly single-threaded on the server tick loop, avoiding race conditions and locking overhead.

---

## 2. Server Concurrency & Connection Model

```
                    ┌───────────────────────────────┐
                    │       Dedicated Server        │
                    │                               │
 [Telnet Client]──► │ [Telnet Conn] ──┐             │
                    │                 ▼             │
 [TUI Client]   ──► │ [Proto Conn]  ──┼──► [ In Chan (Session, Command) ]
                    │                 ▲             │
 [Web/WSS Client]─► │ [WSS Conn]   ───┘             │
                    │                               ▼
                    │                  ┌─────────────────────────┐
                    │                  │  Server Main Tick Loop  │
                    │                  │  - Tick / Weather       │
                    │                  │  - Process Commands     │
                    │                  │  - Combat Rounds        │
                    │                  │  - AI / Respawns        │
                    │                  └────────────┬────────────┘
                    │                               │
                    │         Dispatch Events       │
                    │                               ▼
                    │              ┌─────────────────────────────────┐
                    │              │  Session Out Chans (Event Bus)  │
                    │              │  - Room broadcasts              │
                    │              │  - Targeted private tells       │
                    │              │  - Per-player UI vitals/map/inv │
                    │              └─────────────────────────────────┘
                    └───────────────────────────────┘
```

### A. The Actor-Loop Pattern
Rather than synchronizing `world.World` across dozens of concurrent goroutines with coarse read/write locks, Sudengine adopts the classic MUD actor loop:
* **Connection Goroutines**: Each network connection runs a lightweight read loop that parses lines/frames and enqueues a `SessionCommand{Session: sess, Input: line}` into `server.cmdChan`.
* **Main Server Loop**: The server loop consumes `cmdChan` sequentially between clock heartbeats (`ticker.C`).
* **Zero Locking Overhead**: Movement, item trades, combat swings, and Lua scripts execute sequentially in the main thread with guaranteed determinism and zero race conditions.
* **Non-Blocking Write Queues**: Each session maintains a buffered `Out chan protocol.Event` (e.g. buffer size 256). If a client stalls or disconnects, the server drops or disconnects without freezing the world loop.

---

## 3. Session & Connection Lifecycle

```
[ New Connection ]
        │
        ▼
[ State: Handshake / Login ]
        ├─► Existing User ──► [ Password Check ] ──► [ Success ]
        └─► New User      ──► [ Account / Char Creation (Chargen) ]
                                                            │
                                                            ▼
                                                    [ State: InWorld ]
                                                            │
                                            ┌───────────────┴───────────────┐
                                            ▼                               ▼
                                    [ Clean 'quit' ]                [ Disconnect / Linkdead ]
                                            │                               │
                                            ▼                               ▼
                                    [ Save & Despawn ]              [ 60s Grace Period in Room ]
                                                                            │
                                                                    (Timeout or Slain)
                                                                            ▼
                                                                    [ Auto-Save & Despawn ]
```

### A. Session States
```go
type SessionState int

const (
    StateLogin SessionState = iota
    StateChargen
    StatePlaying
    StateLinkdead
    StateClosed
)
```

1. **Login State**:
   - Prompts for account or character name.
   - Enforces password authentication (hashed via Argon2 or bcrypt).
2. **Chargen State**:
   - Reuses existing `rpg.yaml` chargen rules: bundles, origins/races, roles/classes, and initial attribute point allocation.
   - Instantiates a new player entity into `world.Entities`.
3. **Playing State**:
   - Bound to an active `world.Entity` where `e.Kind == world.KindPlayer`.
   - Placed in `start_room` (or the room where the character previously saved).
4. **Linkdead State (Grace Period)**:
   - If a network socket drops during combat or traversal, the character does not vanish instantly (preventing combat combat-logging).
   - Marked `Linkdead: true` for a 60-second window before despawning.

---

## 4. World & Room Multiplayer Semantics

### A. Entity Representation
* `w.Entities` contains all players currently logged into the realm.
* `w.Player()` remains as a fallback pointing to `w.Entities[w.PlayerID]` for single-player tests and scripts.
* `w.Players()` returns all active player entities.
* `w.SessionFor(playerID)` returns the network session bound to that character.

### B. Room Visibility & Occupants
* In single-player, `RoomEvent.Occupants` only listed `KindMobile`.
* In multiplayer, `Occupants` includes other players in the room:
  ```go
  for _, id := range room.Contents {
      e := w.Get(id)
      if e == nil || e.ID == actor.ID {
          continue
      }
      if e.Kind == world.KindPlayer {
          status := ""
          if e.InCombat() {
              status = " (fighting)"
          } else if e.HasFlag("linkdead") {
              status = " (linkdead)"
          }
          ev.Occupants = append(ev.Occupants, fmt.Sprintf("%s%s", e.Display(), status))
      } else if e.Kind == world.KindMobile {
          ev.Occupants = append(ev.Occupants, e.Display())
      }
  }
  ```

### C. Movement Echoes
When player $A$ moves through exit `north` from room $R_1$ to room $R_2$:
* **Actor $A$**: Receives normal room description and arrival narrative.
* **Occupants of $R_1$**: Receive narrative echo: `"[Actor] leaves north."` (or `"leaves hurriedly."` if fleeing).
* **Occupants of $R_2$**: Receive narrative echo: `"[Actor] arrives from the south."`
* **Followers**: Multi-tier follower propagation pulls companions and party members across the boundary with follower echoes.

---

## 5. Communication Channels

| Command | Channel / Scope | Audience | Example Output |
| :--- | :--- | :--- | :--- |
| `say <text>` / `'<text>` | `ChanSay` (Local Room) | All players in the same room | `Ada says, "Greetings everyone."` |
| `emote <text>` / `:<text>` | `ChanNarrative` (Local Room) | All players in the same room | `Ada waves her hand warmly.` |
| `tell <player> <msg>` | `ChanSay` (Private) | Specific target player | `[To Bob] "Meet me at the square."` / `[From Ada] "Meet me at the square."` |
| `whisper <player> <msg>`| `ChanSay` (Quiet Room) | Target player in same room | `Ada whispers to you, "Be careful."` |
| `shout <text>` / `yell` | `ChanAlert` (Area/Outdoors) | All players in adjacent outdoor rooms | `Someone shouts from the east, "Watch out!"` |
| `gossip <text>` / `ooc` | `ChanSystem` (Global Realm) | All logged-in players | `[Gossip] Ada: "Anyone know where Sergeant Bram is?"` |
| `group <msg>` / `gt <msg>`| `ChanNarrative` (Party) | All party members anywhere | `[Party] Ada: "Fall back and heal!"` |
| `who` | `ChanSystem` (Roster) | Invoker | Displays online player list, levels, roles, and idle status |

---

## 6. Combat, Groups & PvP Rules

### A. Cooperative PvE Combat
* Multiple players can engage the same mobile simultaneously.
* Combat rounds broadcast attack narratives and damage rolls to all players in the room.
* Experience points and quest objective kill credit are shared across party members present in the room when the target falls.

### B. PvP Flagging & Arenas
* **Default**: Peaceful PvE. Players cannot attack other players (`kill <player>` refuses with `"You cannot attack other players here."`).
* **Arena / PvP Rooms**: Rooms flagged `flags: [pvp]` or `flags: [arena]` allow mutual combat between players.
* **Opt-In Stance**: Optional player flag (`pvp on/off`) in packs that support open wilderness fighting.

---

## 7. Account & Character Persistence

### A. Directory Structure
```
saves/<pack>/
  accounts/
    <account_id>.json         # Account credentials (username, password_hash, created_at)
  players/
    <player_name>.yaml        # Full character snapshot (attributes, resources, inventory, quests, recipes, location)
  world_state.yaml            # Shared persistent world state (vault till, merchant purse, gathering node timers)
```

### B. Isolated Player Saves
* In single-player, saving writes the entire world into `saves/<pack>/<slot>.yaml`.
* In multiplayer:
  - Each character is stored in its own YAML file (`saves/<pack>/players/<player>.yaml`).
  - Player saving does not overwrite or block other players.
  - The shared world state (vendor purse, banks, persistent world items) is saved independently on an autosave interval (e.g. every 5 minutes) or server shutdown.

---

## 8. Network Transports & Prototypes

### A. Transport 1: Telnet / Raw TCP (Port 4023)
* Standard line-oriented TCP server using `bufio.Scanner`.
* Translates incoming ASCII/UTF-8 commands into `server.cmdChan`.
* Formats outgoing `protocol.Event` structs into colored ANSI-16 text:
  - `RoomEvent` -> Renders room title, description, exits, items, and occupants.
  - `VitalsEvent` -> Renders prompt or prompt status string.
  - `TextEvent` -> Prints directly with channel ANSI color coding.
* Telnet negotiation: IAC WILL ECHO for password suppression during login.

### B. Transport 2: Framed Protocol (TCP / WebSocket Port 4000)
* Transmits length-prefixed JSON frames or WebSocket text frames representing exact `protocol.Event` JSON payloads.
* Enables the native `sudengine connect <host:4000>` client to render the complete multi-pane Bubble Tea interface (dedicated map pane, real-time vitals meters, combat roster, and narrative output) across the network.

---

## 9. Staged Implementation Plan

* **Stage 1: Multi-Session Engine Core**
  - Refactor `internal/server/server.go` to maintain `map[string]*Session` and a thread-safe connection register.
  - Route incoming session commands into `s.cmdChan`.
  - Implement room broadcasting for `say`, `emote`, and room arrival/departure.
  - Implement `who`, `tell`, and `gossip` verbs.
* **Stage 2: Network Gateways (`internal/net`)**
  - Implement TCP Telnet gateway (`internal/net/telnet.go`).
  - Implement protocol streaming gateway (`internal/net/stream.go`).
  - Add CLI subcommands: `sudengine serve <pack>` and `sudengine connect <host:port>`.
* **Stage 3: Accounts & Character Persistence**
  - Login state machine and password hashing.
  - Individual character snapshot storage in `saves/<pack>/players/`.
  - Link-dead detection, auto-reconnect, and graceful disconnects.
