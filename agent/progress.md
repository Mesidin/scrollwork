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
  * Green Hollow keeps the square lit, adds a windowed cottage, and leaves a lantern on the lane.

### Milestone 5: Expanded Economy, Durability & Crafting Pipeline (Oct 2, 2026)
* **Durability & Wear/Tear Mechanics**:
  * Added `internal/economy` package with condition tags (`pristine`, `good`, `worn`, `damaged`, `broken`).
  * Weapons and armor take one point of wear on a landed hit.
  * Broken items trigger alerts and apply mechanical penalties (broken weapons fall back to 1d2 improvised damage; broken armor grants 0 soak).
  * Added `repair` verb allowing players to inspect damaged gear and pay NPC repairers (`repairer: true`) to restore items.
  * Durability-based sell price scaling: damaged items sell at a proportional discount based on remaining durability.
* **Dynamic Merchant Purses & Commerce**:
  * Supported finite merchant purses (`shop.purse`, `shop.max_purse`, `shop.dynamic: true`).
  * `list` command reveals vendor's till amount.
  * `sell` enforces merchant affordability and deducts from vendor purse; `buy` transfers player currency into vendor purse.
* **Crafting Pipeline**:
  * Implemented `recipes.yaml` loading and validation in `internal/pack`.
  * Support for room station checks (e.g. `workbench`, `anvil`), tool requirements, and ingredient consumption.
  * Player commands: `recipes` (alias `rlist`) and `craft` (alias `make`). `list` alias `lshop`.
* **Pack Demo & Tests**:
  * Updated `games/green-hollow` with item durability (`sword`, `cloak`), Sergeant Bram as a repairer with dynamic purse, a workbench in the cottage, and craftable cudgel and pitch torch recipes (`recipes.yaml`).
  * Integration tests cover one repair, one sale the till can afford, and one craft with the station and materials in hand (`internal/server/economy_test.go`). Unit tests cover durability, a dynamic purse, and the craft pipeline (`internal/economy/economy_test.go`).
  * Updated engine documentation in `internal/docs/engine/commands.md` and `systems.md`.
* **Follow-up (Oct 5, 2026)**:
  * Selling pays a pile past its starting amount, and only charges the till for coins the player receives.
  * A purse set with `purse`, `max_purse`, or `dynamic` stays finite at zero.
  * `repair all` continues past unaffordable pieces. Only `repairer` NPCs mend gear.
  * A recipe skill must name a real check. A missing pattern is refused. Worn gear is not an ingredient. A shattered item does not sell.
  * `recipes` marks the station, tools, and ingredients. `lshop` and `rlist` are registered. `repairable` defaults to false.

### Milestone 6: Banking, Gathering, Recipe Discovery & Crafting Progression (Oct 6, 2026)
* **Banking & Town Vaults**:
  * Implemented vault banking via `Entity.Bank map[string]int` and `Banker bool`.
  * Verbs: `deposit <amount|all>`, `withdraw <amount|all>`, and `balance` (aliases: `bank`, `account`).
  * Stored currency in vaults is preserved across character deaths.
  * Added `hollow.strongbox` common vault to the cottage in `green-hollow`.
* **Recipe Discovery & Progression**:
  * Added `discovered: false` flag to lock recipes until discovered or learned.
  * NPC recipe teaching: `NPCYAML.teach_recipes` allows NPCs to grant recipes during conversation (`ask bram about crafting/cudgel`).
  * Item recipe manuals: `use.learn_recipe` allows reading notes or manuals to learn crafting recipes (`use manual`).
  * Added Lua host binding: `learn_recipe(recipeID, [actor])`.
  * `recipes` command filters to only known/discovered recipes; `craft` prevents crafting unlearned recipes.
* **Crafting Skill & Quality Bonuses**:
  * Added `crafting` skill and check to `green-hollow` (`rpg.yaml`), trainable at Bram.
  * Recipes support `min_skill` (minimum skill rank required).
  * High crafting check rolls exceeding difficulty by 3+ grant a superior craftsmanship quality bonus (+20% durability and `superior` flag).
* **Resource Gathering & Perception**:
  * Added `GatherNode` configuration for room and scenery entities with finite harvest capacity (`remaining`, `max`) and periodic regeneration (`regen_every`).
  * Verbs: `gather` / `forage` (alias `harvest`).
  * Perception integration: rolls `notice` skill check; rolls meeting `bonus_dc` yield extra gathered resources.
### Milestone 7: Quests, Dialogue & Objectives Engine (Oct 6, 2026)
* **Quest Engine Architecture**:
  * Added `internal/quest` package with complete lifecycle management (`IsActive`, `IsReady`, `IsComplete`, `Accept`, `Complete`, `UpdateKill`, `UpdateVisit`, `UpdateTalk`, `ObjectiveProgress`).
  * Supports objectives: `kill` (mob prototypes/keywords), `collect` (gather items, optional `consume`), `visit` (room discovery), `talk` (conversations with NPCs).
  * Supports rewards: XP, currency/coin, item prototype grants, and recipe discovery unlocks (`recipes`).
  * Quest prerequisites: `requires_quest` prerequisite validation for quest lines.
* **World & Pack Integration**:
  * Added `QuestState` tracking to `internal/world` with deep copying in `Entity.Clone()`.
  * Added `Pack.Quests` loaded from `quests.yaml` / `world/quests.yaml`, validated in `validate.go`, and saved in `Save()`.
* **Player Commands & Dialogue Hooks**:
  * Verbs: `quests` (aliases: `tasks`, `journal`), `quest <id>`, `quest accept <id>`, `quest complete <id>`.
  * Integrated into NPC `talk` (showing offered quests and ready turn-ins), `ask` (talk objectives), `get` (collect updates), `move`/`showRoom` (visit & auto-start triggers), and `reapDead()` (kill updates).
* **Pack Content & Verification**:
  * Added 3-quest arc to `green-hollow` (`clear_wolves`, `lost_silver`, `scout_camp`).
  * Unit and integration test suite passing in `internal/server/quest_test.go` and `internal/quest/quest_test.go`.

### Milestone 8: Hunting, Loot Tables & Field Survival (Oct 6, 2026)
* **Loot Tables & Corpse Skinning**:
  * Added `LootDrop` (`Item`, `Chance`, `Count`, `SkinOnly`) to `world.Entity` and `NPCYAML`.
  * Upon mob death, non-skin loot drops to the room, while skin-only loot stays attached to the corpse.
  * Added `skin` command (alias `butcher`): skins corpses, rolls loot drop chances, marks corpse with `skinned` flag, updates short description to `(butchered)`, and triggers quest collect progress.
* **Salvage & Deconstruction**:
  * Added `SalvageYield` (`Item`, `Count`) to `world.Entity` and `ItemYAML`.
  * Added `salvage` command (aliases: `breakdown`, `dismantle`, `deconstruct`): dismantles inventory items into crafting components and destroys base item.
* **Field Repair Kits**:
  * Added `RepairKit` (`Uses`, `MaxUses`, `Skill`, `DC`, `Amount`) to `world.Entity` and `ItemYAML`.
  * Extended `repair` command: `repair <item> with <kit>` and automatic carried kit usage away from town repairers.
  * Extended `use` command: `use <kit> on <item>`.
  * Supports optional skill checks (e.g. `crafting`), restores durability, clears broken flags, decrements uses, and destroys exhausted kits.
* **Dynamic NPC Respawns**:
  * Added `RespawnEvery` and `HomeRoom` to `world.Entity` and `NPCYAML`.
  * Server heartbeat manages `World.Respawns`, spawning fresh instances at their home location and notifying players.
* **Pack Content & Verification**:
  * Added `hollow.pelt`, `hollow.whetstone`, `salvage` yields on cloak and club, loot on wolves and brigands in `green-hollow`.
* **Follow-up (Oct 6, 2026)**:
  * Quest XP is granted once, through level-up when advancement is on.
  * Kill credit requires the player to land the blow. Green Hollow's two wolves do not respawn, so one wolf cannot be farmed for the thicket quest.
  * Skinning rolls only `skin_only` rows. A corpse is keyword `corpse` and does not answer to the living mob.
  * A field kit uses its own difficulty. A missing named check refuses the repair.
  * `min_skill` is trained rank. A superior craft is 3 above the target the roll used.
  * `craft` ignores unlearned matches before it asks which recipe.
  * The vault takes loose piles, and `deposit all coin` names the pile. A withdrawal the player cannot hold goes back to the vault.
  * Asking a partial topic no longer teaches a recipe. Quest names need an exact title or one whole word.
  * Builder purses stay finite. Prototype kits, shops, and gather nodes are copied onto instances. Positional `gset` numbers stay in order. `recipe add` changes only the fields on the command line. Deleting the last recipe or quest rewrites the file as an empty list.
  * Added unit and integration tests in `internal/server/hunting_test.go` (100% passing).

### Milestone 9: In-Engine OLC World Builder Tools (Oct 6, 2026)
* **Mobile Configuration (`mset`)**:
  * Implemented `mset <mob> <field> [args...]`:
    * Inspection: `show` (displays AI, XP, respawn ticks, service flags, shops, topics, recipes taught, and loot tables).
    * AI & Progression: `ai <profile> [wander]`, `xp <amount>`, `respawn <ticks>`.
    * Services: `trainer <on|off>`, `repairer <on|off>`, `banker <on|off>`.
    * Shopkeeper: `shop currency <id>`, `shop purse <amount>`, `shop maxpurse <amount>`, `shop dynamic <on|off>`, `shop clear`.
    * Dialogue & Progression: `topic <keyword> <text>`, `teach <topic> <recipe>`.
    * Loot Tables: `loot list`, `loot add <item> [chance] [count] [skin]`, `loot clear`.
* **Item Configuration (`iset`)**:
  * Implemented `iset <item> <field> [args...]`:
    * Inspection: `show`.
    * Combat & Gear: `use <flag>`, `damage <dice>`, `armor <soak>`, `slot <name>`, `value <n>`, `durability <n>`, `repairable <on|off>`, `banker <on|off>`.
    * Survival & Kits: `kit <uses> [amount] [skill] [dc]`, `kit clear`.
    * Salvage & Deconstruction: `salvage list`, `salvage add <item> [count]`, `salvage clear`.
* **Gathering Nodes Configuration (`gset`)**:
  * Implemented `gset <subcommand> [args...]`:
    * Inspection & Clearing: `show`, `clear`.
    * Room Binding: `room item <id> [max] [regen] [skill] [dc] [bonus_dc]`.
    * Extra / Scenery Binding: `node <extra|item> [max] [regen] [skill] [dc] [bonus_dc]`.
  * Added `Gather *world.GatherNode` support to `RoomYAML` and `ExtraYAML` with full serialization in `SyncFromWorld` and validation in `validate.go`.
* **Crafting Recipe Builder (`recipe` / `rcset`)**:
  * Implemented `recipe` / `rcset` commands:
    * `list`: lists all registered crafting recipes in the active pack.
    * `show <id>`: displays output, name, required station, skill, DC, min rank, discovery method, and ingredients.
    * `add <id> <produces> [name] [station <s>] [skill <s>] [dc <n>] [min <n>] [disc <grant|buy|discover|all>]`: creates or updates recipe.
    * `req <id> <item> [count]`: adds or updates ingredient requirements.
    * `del <id>`: deletes recipe from pack.
* **Prototype Synchronization & Persistence**:
  * Live updates synchronize both active instances and prototype templates (`syncItemProto`, `syncMobProto`).
  * `save pack` round-trips all newly configured entities, gathering nodes, loot tables, and `recipes.yaml`.
* **Testing & Documentation**:
  * Added comprehensive OLC test suite in `internal/olc/olc_test.go` verifying `mset`, `iset`, `gset`, `recipe`, and full `save pack` reload round-tripping.
  * Updated builder documentation in `internal/docs/engine/building.md`.


