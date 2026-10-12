# Sudengine Project Roadmap & Backlog

Status Legend:
* `[ ]` Not started
* `[-]` In progress / Under design
* `[x]` Completed

---

## 1. Tooling & Infrastructure

* [x] **Pack Integrity Tools**: Extended validation checks for unreachable rooms, orphaned keys, and broken script references.
* [x] **YAML Comment Preservation**: Prevent `save pack` from stripping comments in world YAML files.

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

* [x] **Expanded Economy & Commerce**: *(See design spec: [`features/economy.md`](features/economy.md))*
  * [x] Dynamic merchant inventory & purses (finite vendor gold till, dynamic refill on player purchases, affordability check on sell).
  * [x] Item durability & equipment degradation during combat rounds (weapon and armor wear on a landed hit, broken states).
  * [x] Repair mechanics via blacksmith NPCs (`repair`, `repair <item>`, `repair all`, durability discount on sale).
  * [x] Crafting system (`recipes.yaml` schema, room crafting station checks, tool requirements, material consumption, `recipes` & `craft` verbs).
  * [x] Recipe progression (discovery locks `discovered: false`, NPC topic teaching `teach_recipes`, item `use.learn_recipe`, Lua API `learn_recipe`).
  * [x] Crafting skill integration (minimum skill requirement `min_skill`, crafting rolls, and superior craftsmanship quality bonuses).
  * [x] Resource gathering (`gather` / `forage`, gathering nodes with capacity and regeneration, perception DC checks with bonus resource discoverability).
  * [x] Banking & town vaults (`banker: true`, `deposit`, `withdraw`, `balance` preserved safely across character deaths).
  * [x] Hunting & loot tables (`loot` schema, non-skin drops on death, `skin` / `butcher` corpse harvesting).
  * [x] Salvage & deconstruction (`salvage` / `breakdown` items into components).
  * [x] Field survival repair kits (`repair_kit` schema, `repair <item> with <kit>`, `use <kit> on <item>`).
  * [x] Dynamic mob respawning (`respawn_every` tick heartbeat).
  * [x] Supply-and-demand price curves (`shop.price_curve: true`, `ScalePrice` floating curves based on stock).
  * [x] Storing and persisting recipe discovery methods (`grant`, `buy`, `discover`, `all` in `Recipe.Discovery` and `Entity.RecipeMethods`). `buy` is recorded when the character reads that recipe's manual. Carrying the manual or buying the finished good does not teach it.
  * [x] Review fixes for stock identity, NPC shop stock, ask/talk replies, follow-loop purchases and invites, loyalty caps, dismiss, and unambiguous script lookup.

---

## 4. Quests & Narrative Objectives

* [x] **Quests, Dialogue & Objectives**:
  * [x] Data-driven quest schema (`quests.yaml` / `world/quests.yaml`).
  * [x] Objectives engine (`kill`, `collect` with item consumption, `visit`, `talk`).
  * [x] Reward engine (XP, currency, item prototype grants, recipe unlocks).
  * [x] Prerequisite quest chains (`requires_quest`).
  * [x] Player commands (`quests` / `tasks` / `journal`, `quest <id>`, `quest accept`, `quest complete`).
  * [x] Dialogue integration: NPC `talk` lists offered quests and ready turn-ins; `ask` triggers talk objectives.
  * [x] Quest-driven NPC relocation: declarative `relocate:` rewards (`mob`, `to`, `update_home`, `message`) run after the turn-in script, and only a move that happened is narrated.
  * [x] Lua quest hooks & teleportation: `on_quest_complete` sees a completed quest before relocate. `teleport` / `move`, `has_quest`, `is_quest_completed`, and `is_quest_active`.
  * [x] Demonstrated with 3-quest progression line in `games/green-hollow`.

---

## 5. Content, Building & OLC Tools

* [ ] **Develop `game-3`**: Reserved for user. Do not edit or touch.
* [x] **Expand `green-hollow`**: Quests, trainer, combat, gathering, hunting, repair, and crafting loops added.
* [x] **In-Engine OLC World Builder Tools**:
  * [x] Shop & purse builders (`mset shop ...`).
  * [x] Service NPC flags (`mset repairer`, `mset banker`, `mset trainer`).
  * [x] Mobile loot table & respawn builders (`mset loot`, `mset respawn`).
  * [x] Item salvage & repair kit builders (`iset salvage`, `iset kit`).
  * [x] Gathering node builders (`gset` / `node`).
  * [x] Crafting recipe builders (`recipe` / `rcset`).
  * [x] Live prototype synchronization and `save pack` YAML round-trip persistence.

---

## 6. Followers, Parties & Companions

* [x] **Follower Graph & Movement Propagation**:
  * [x] Multi-tier follower propagation across room transitions (`moveFollowers`) for players and wandering mobs.
  * [x] Loop detection preventing cyclic following (`StartFollowing`, `StopFollowing`).
  * [x] Player commands: `follow` (alias `fol`), `unfollow` (aliases `unfol`, `stopfollow`), `lose` (aliases `ditch`, `shake`).
* [x] **Party Management & Grouping**:
  * [x] Group roster display showing levels and primary resource vitals (`group` / `party` / `gt`).
  * [x] Invitation and joining system (`invite`, `join`). NPC acceptance via `party_member: true` or companion flag; open parties via `open_party: true`.
  * [x] Leaving and disbanding parties.
* [x] **Companions, Pets & Taming**:
  * [x] Pack schema for NPCs (`companion`, `owner`, `loyalty`, `max_loyalty`, `stance`, `value`).
  * [x] Shopkeeper pet purchases (`buy <pet> from <vendor>`).
  * [x] Pet feeding and loyalty progression (`feed <pet> with <food>`). Taming wild animals flagged `tameable` or `pet` at loyalty >= 30.
  * [x] Directing pets via `order` (`follow`, `stay`, `stance`, `attack`, `dismiss`). Stubborn refusal below 20 loyalty.
* [x] **Combat Stances & Progression Sharing**:
  * [x] Stance behavior in combat (`assist`, `defend`, `passive`, `stay`) via `CheckAssists`.
  * [x] Quest kill objective credit and experience points shared when pets/companions land the killing blow.
* [x] **Scripting API**:
  * [x] Lua bindings: `follow`, `unfollow`, `set_companion`, `get_loyalty`, `set_loyalty`, `get_stance`, `set_stance`, `join_party`, `invite_party`, `leave_party`.
* [x] **Review fixes**: tame AI, `kill` assists, shop stock, party disband, same-tick kill credit, room entry for followers, death recall, pet-only orders, and exact food matching.

---

## 7. Networking & Multiplayer *(See architectural spec: [`features/multiplayer.md`](features/multiplayer.md))*

* [ ] **Multi-Session Core Engine**: Multi-session management in `internal/server`, serialized command loop, and room/global broadcast channels (`say`, `emote`, `tell`, `who`, `gossip`).
* [ ] **Network Gateways & Port Binding**: Bind `internal/server` protocol sessions to Telnet (ANSI text) and structured protocol streaming (TCP/WebSocket for Bubble Tea remote clients).
* [ ] **Account & Persistence Layer**: User credentials, character selection/creation, isolated per-player saves, and link-dead grace periods.
