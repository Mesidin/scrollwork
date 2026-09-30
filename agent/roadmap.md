# Sudengine Project Roadmap & Backlog

Status Legend:
* `[ ]` Not started
* `[-]` In progress / Under design
* `[x]` Completed

---

## 1. Tooling & Infrastructure

* [ ] **GitHub Actions CI**: Automated `go test ./...` on push and pull request across Linux, macOS, and Windows.
* [ ] **YAML Comment Preservation**: Prevent `save pack` from stripping comments in world YAML files.
* [ ] **Pack Integrity Tools**: Extended validation checks for unreachable rooms, orphaned keys, and broken script references.

---

## 2. World & Environmental Systems

* [x] **Weather & Climate Simulation**: *(See design spec: [`features/weather.md`](features/weather.md))*
  * [x] Scaled system (Level 0: disabled, Level 1: cosmetic, Level 2: lighting/sensory, Level 3: mechanical).
  * [x] Room flags: `outdoor`/`outdoors`, `windowed`/`sheltered`, `underground`, `lit`.
  * [x] Pack environment definitions (`environment.yaml` / `weather.yaml` with states, transitions, intervals).
  * [x] Server tick integration emitting sensory alerts to outdoor/windowed/indoor rooms.
  * [x] Scale 2 light and notice modifiers. Scale 3 outdoor hit penalties, flame extinguishing, and a night notice penalty.
  * [x] Player commands: `weather`, `sky`, `look sky`.
* [x] **Day/Night & Solar Clock Cycle**:
  * [x] Scaled day/night system (Level 0: disabled, Level 1: cosmetic, Level 2: lighting, Level 3: mechanical).
  * [x] 24-minute real time = 24-hour MUD day ratio (configurable `minutes_per_day` and `start_hour`).
  * [x] Phases: Dawn, Morning, Midday, Afternoon, Dusk, Night, Midnight with transition echoes.
  * [x] Environmental lighting: night causes outdoor darkness; daylight illuminates windowed rooms.
  * [x] Player command: `time` (aliases `clock`, `date`).

---

## 3. Economy, Crafting & Durability

* [-] **Expanded Economy & Commerce**: *(See design spec: [`features/economy.md`](features/economy.md))*
  * [ ] Dynamic merchant inventory & purses (finite vendor gold, stock depletion, supply/demand pricing).
  * [ ] Item durability & equipment degradation during combat rounds.
  * [ ] Repair mechanics via blacksmith NPCs or repair kits.
  * [ ] Crafting system (gathering, refining, assembly via verbs or Lua recipes).
  * [ ] Tangible currencies (coin encumbrance/weight, bank vaults, money changers).

---

## 4. Content & Games

* [ ] **Develop `game-3`**: Turn the blank scaffold into a complete mini-adventure or specialized demo.
* [ ] **Expand `green-hollow`**: Add additional rooms, quest loops, deeper NPC conversations, and wilderness encounters.
* [ ] **OLC Enhancements**: In-engine builders for shops, loot tables, and dialogue trees.

---

## 5. Networking & Multiplayer (Long-Term)

* [ ] **Server Port Binding**: Bind `internal/server` protocol session handling to TCP / Telnet / SSH.
* [ ] **Multiplayer Session Management**: Player connection lifecycle, room broadcasts, and multi-user chat channels.
* [ ] **Account & Persistence Layer**: User credentials and character selection before world instantiation.
