# Sudengine Progress & State Log

This document records the chronological development history, verified features, and current system state of the Sudengine (Erickson Stories) project.

---

## Current Status (as of Sep 29, 2026)

* **Engine**: Go 1.22+ Bubble Tea MUD engine with runtime YAML+Lua pack loading.
* **Test Suite**: `go test -count=1 ./...` passing 100% across all packages.
* **Working Tree**: Clean on `master` branch.
* **Sample Packs**:
  * `games/old-house`: Core story regression pack.
  * `games/green-hollow`: Extended fantasy drill demonstrating levels, training, skills, AI, and shop interactions.
  * `games/game-3`: Pack stub initialized via `sudengine new game-3`.

---

## Development Milestones

### Milestone 1: Initial Sudengine Engine & Old House (Sep 16, 2026 — `d1a5029`)
* Established core client/server architecture communicating via `internal/protocol`.
* Bubble Tea multi-pane TUI (`internal/client`) supporting output, map, vitals, and commands.
* Entity graph (`internal/world`) for rooms, items, and NPCs.
* Parser and standard MUD verbs (`internal/commands`).
* In-engine building prototype (`internal/olc`).
* Sandboxed Lua integration (`internal/script` via `gopher-lua`).
* Initial `old-house` story pack with simple puzzles, keys, and scripts.

### Milestone 2: Pack Tooling, Richer OLC & Combat Enhancements (Sep 16, 2026 — `34825e1`)
* Pack validation (`sudengine validate`) and zip installation (`sudengine install`).
* World graph integrity validation before play mode.
* Richer builder tools: multiline descriptions (`.`), `extra`, `ai`, and `iset` (use/damage/value).
* Combat updates: wielded weapons, worn soak armor, 3rd-person combat verb conjugation, and NPC corpses.
* NPC interactions: `talk` command and shopkeeper transactions.
* Stats display showing origin and role choices from chargen.

### Milestone 3: Progression, Perception, AI & Interactive Help (Sep 25, 2026 — `e8e91aa`)
* **Character Progression**:
  * Added `rpg.Advancement` schema (XP thresholds, skill points, attribute points, trainer costs).
  * Added player commands: `improve <skill>`, `raise <attr>`, and `train <skill>` (with NPC trainer and currency checks).
* **Perception & Checks**:
  * Check resolution system with dice rolls, opposing gear/attributes, and DC thresholds.
  * Added passive `notice` rolls on entering rooms and active `search` command to find hidden entities.
* **World & AI Upgrades**:
  * Door mirroring and state synchronization across room boundaries (`MirrorExit`, `SyncDoors`).
  * NPC roaming boundaries (`Allows`) and aggression triggers (`sight`, `enter`, `look`, hostility flags).
* **Documentation & TUI Help**:
  * Built an interactive two-pane manual viewer with topic headers, section pagination, and PageUp/PageDown scrolling.
  * Created `green-hollow` sample pack implementing roles, trainers, combat, and hidden items.
  * Windows build instructions and binary handling (`sudengine.exe`).

### Milestone 4: Scaled Weather & Day/Night Clock Systems (Sep 30, 2026)
* **Scaled Architecture**:
  * Designed tiered simulation levels (Level 0: disabled, Level 1: cosmetic, Level 2: lighting/sensory, Level 3: mechanical).
  * Implemented `internal/weather` package with `time.go`, `weather.go`, and `env.go`.
* **Day/Night Cycle**:
  * 24-minute real time = 24-hour MUD day ratio (configurable `minutes_per_day` and `start_hour`).
  * 7 day phases: Midnight, Dawn, Morning, Midday, Afternoon, Dusk, Night with transition echoes.
  * Dynamic ambient lighting: night renders outdoor rooms dark unless carrying light; daylight illuminates dark windowed rooms.
  * Player command: `time` (aliases `clock`, `date`).
* **Weather Simulation**:
  * Weather state machine with customizable states (clear, overcast, fog, rain, storm, snow, blizzard) and transition weights.
  * Room flag filtering: outdoor vs windowed vs underground sensory echoes.
  * Mechanical modifiers: storms dim outdoor light; rain/fog/storms penalize perception checks (`notice`, `search`).
  * Player commands: `weather`, `sky`, `look sky`.
* **Sample Packs & Documentation**:
  * Added `games/green-hollow/environment.yaml` showcasing Level 1 environment.
  * Documented commands and systems in `internal/docs/engine/commands.md` and `systems.md`.
  * Verified 100% test coverage with unit and integration tests.
* **Follow-up (Sep 30, 2026)**:
  * Saves restore the hour before the first look, and do not repeat a phase line for weather the world is already in.
  * Ambient echoes roll once per 80 ticks.
  * Scale 3 applies outdoor hit penalties, blows out toggle-lights in storms and blizzards, and adds a night notice penalty.
  * `sky_desc`, arrival lines, and phase lines come from the pack when the pack sets them.
  * Green Hollow keeps the square lit, adds a windowed cottage, and leaves a lantern on the lane.
