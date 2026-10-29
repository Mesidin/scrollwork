# Sudengine Progress & State Log

This document records the chronological development history, verified features, and current system state of the Sudengine (Erickson Stories) project.

---

## Current Status (as of Oct 27, 2026)

* **Engine**: Go 1.22+ Bubble Tea MUD engine with runtime YAML+Lua pack loading, framed TCP multiplayer (`serve`/`connect`), Telnet (`127.0.0.1:4023`), account authentication, paired atomic world/player saves, link-dead grace periods, and server-driven chargen.
* **Test Suite**: Stage 4/5 review fixes passed `go test -count=1 ./internal/save ./internal/net ./cmd/sudengine ./internal/server ./internal/commands ./internal/pack` on Oct 27, 2026. The race detector was not run (no gcc). A full `go test ./...` was not re-run in that pass.
* **Working Tree**: Stage 4 and 5 review fixes are uncommitted. All 5 multiplayer stages are in the tree.
* **Sample Packs**:
  * `games/old-house`: Core story regression pack.
  * `games/green-hollow`: Extended fantasy drill demonstrating levels, training, skills, AI, economy, quests, and companion interactions.
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

### Milestone 10: Quest-Driven Mob Relocation & Lua Teleportation (Oct 6, 2026)
* **Declarative Quest Mob Relocation**:
  * Added `Relocation` (`Mob`, `To`, `UpdateHome`, `Message`) to `quest.Rewards`.
  * Relocate runs after `on_quest_complete`. The pack `message` is printed only when that move succeeds. A missing or ambiguous mobile or room is reported, and the quest stays completed.
  * When several mobiles share a prototype, the one in the player's room is used. Any remaining tie moves nobody.
  * `update_home` sets `HomeRoom` and adds the room to a restricted leash. A one-shot move leaves both alone. Death copies the leash onto the respawn task.
  * Pack validation in `internal/pack/validate.go` validates that `relocate.mob` exists in NPCs and `relocate.to` exists in rooms.
* **Lua Teleportation & Quest State Functions**:
  * Added `teleport(who, where, [update_home])` (and alias `move`). It moves a mobile, item, or the player into a room, and refuses to reparent a room.
  * Moving the player ends the fight and runs the same leave/enter path as walking. Other teleports stay quiet.
  * `update_home` uses the same home and leash helper as quest relocate.
  * `is_quest_completed(id, [who])`, `is_quest_active(id, [who])`, and `has_quest(id, [who])`. `has_quest` is true for an active or completed quest. `who` is an id, prototype, or keyword.
* **Quest Completion Lua Hook**:
  * `on_quest_complete(actor, self, quest_id)` runs after the quest is marked completed and before relocate, on the turn-in mobile or, when that mobile has no script, the giver's live instance. Optional `quest.script` runs as well.
* **Verification & Documentation**:
  * Unit test `TestQuestRelocation` in `internal/quest/quest_test.go`.
  * Sandbox unit test `TestLuaTeleportAndQuestStatus` in `internal/script/script_test.go`.
  * Integration test `TestQuestRelocationIntegration` in `internal/server/quest_test.go`.
  * Updated `internal/docs/engine/systems.md` and `internal/docs/engine/building.md`.

### Milestone 11: Followers, Parties, Companions & Pet Taming (Oct 7-8, 2026)
* **Follower Graph & Movement Propagation**:
  * Added `Leader`, `Followers`, and `IsFollowing` graph traversal to `internal/world`.
  * Implemented acyclic following guards in `World.StartFollowing` and safe detaching in `World.StopFollowing` and `World.Destroy`.
  * Movement propagation: `moveFollowers` propagates active followers through exits whenever their leader transitions rooms (supporting players, companion chains, and wandering AI).
  * Player commands: `follow [target]` (lists status or begins following), `unfollow` (clears leader), `lose <target>` (shakes off a specific follower).
* **Party Management & Grouping**:
  * Added `PartyLeader` and `PartyInvites` to `Entity` with `World.PartyMembers` retrieval.
  * Player commands: `group` / `party` (displays member levels and vitals), `invite <target>` (invites characters; companions and mobs flagged `party_member: true` automatically accept), `join <target>` (accepts invites or joins open parties flagged `open_party: true`), and leaving/disbanding.
* **Companions, Pets & Taming**:
  * Added `Companion`, `Owner`, `Loyalty`, `MaxLoyalty`, and `Stance` (`assist`, `defend`, `passive`, `stay`) to `world.Entity` and `NPCYAML` with pack YAML persistence.
  * Commerce: purchasing companion mobiles from vendor shopkeepers (`buy <pet> from <vendor>`) automatically assigns ownership and initiates following.
  * Feeding & Taming: `feed <pet> with <food>` consumes edible items, boosts loyalty (+15), and restores health. Wild beasts flagged `tameable` or `pet` convert to loyal companions at $\ge 30$ loyalty.
  * Orders: `order <pet> <follow|stay|stance|attack|dismiss>`. Pets below 20 loyalty have a chance to stubbornly refuse orders.
* **Combat Stances & Progression Sharing**:
  * Integrated `combat.Engine.CheckAssists`: nearby followers respond based on combat stances (`assist` engages target; `defend` engages only when leader is targeted).
  * Kill credit and progression sharing: `reapDead()` attributes kills by companions or party members to the player for quest objective updates and experience rewards.
* **Lua Scripting Bindings**:
  * Exposed `follow`, `unfollow`, `set_companion`, `get_loyalty`, `set_loyalty`, `get_stance`, `set_stance`, `join_party`, `invite_party`, and `leave_party`.
* **Testing & Documentation**:
  * Unit tests in `internal/commands/follow_test.go`, `internal/world/world_test.go`, and `internal/script/script_test.go`.
  * Integration tests in `internal/server/follow_test.go` verifying following, combat assists, and quest kill attribution.
  * Updated player manuals in `internal/docs/engine/commands.md` and `internal/docs/engine/systems.md`.
* **Review fixes**:
  * Owned, following, and `stay` mobiles skip aggro, wander, and pursue. Taming and buying break a fight with the owner.
  * `kill` recruits assists before the mob retaliates. Defend joins only when the leader is under attack.
  * Shop stock is the vendor's goods, not a loose copy or a pet the buyer already owns.
  * A party leader who joins another party disbands the old one. `PartyMembers` ignores a leader who has already left. Lua `leave_party` on a leader clears every member.
  * Kill credit is snapshotted before corpses are destroyed, so a companion who dies on the same tick still counts.
  * Follower moves share `World.PullFollowers`. A following player gets a normal room entry. Script teleports, pursuit, and quest relocation pull followers in the departure room. Death brings owned companions to the start; `stay` companions are left behind.
### Milestone 12: Supply-and-Demand Price Curves & Recipe Discovery Methods (Oct 9, 2026)
* **Floating Supply-and-Demand Price Curves**:
  * Added `PriceCurve bool` to `world.Shop` (`shop.price_curve: true` in YAML/JSON).
  * Implemented `StockCounts` and `ScalePrice` in `internal/economy/shop.go` using the floating curve formula:
    $$\text{Effective Price} = \text{Base Price} \times \left(1 + \frac{\text{Base Stock} - \text{Current Stock}}{\text{Base Stock} + 5}\right)$$
  * Supported inventory scarcity scaling (rare or bought-out goods increase in price) and surplus flooding (flooding vendor inventory drops buy-price to nominal scrap value of 1 coin).
  * Updated `BuyPrice` and `SellPrice` to dynamically scale based on stock when `PriceCurve` is enabled on the merchant.
  * Buying from vendor stock matches identical items without false "Which one?" prompt.
  * OLC: `mset <mob> shop curve <on|off>` and display in `mset <mob> show`.
* **Recipe Discovery Methods & Persistence**:
  * Extended `economy.Recipe` with `Discovery` method (`grant`, `buy`, `discover`, `all`), while preserving backwards compatibility with boolean `discovered`.
  * Extended `world.Entity` with `RecipeMethods map[string]string` and methods `LearnRecipe(id, method ...string)` and `RecipeMethod(id)`.
  * Integration across learning channels:
    * NPC dialogue (`teach_recipes`): recorded as `grant`.
    * Quest rewards (`rewards.recipes`): recorded as `grant`.
    * Item use (`use.learn_recipe`): recorded as `discover` (or from recipe's declared `Discovery`).
    * Lua sandbox: `learn_recipe(recipeID, [actor], [method])` and `get_recipe_method(recipeID, [actor])`.
  * OLC: `recipe add <id> ... [disc <grant|buy|discover|all|on|off>]` and `recipe show <id>` displaying discovery method.
  * Player `recipes` command displays learned method tags.
* **Testing & Documentation**:
  * Unit tests in `internal/economy/economy_test.go` (`TestPriceCurveCalculations`, `TestRecipeDiscoveryMethods`).
  * Unit tests in `internal/script/script_test.go` (`TestLuaLearnRecipeMethod`).
  * OLC builder tests in `internal/olc/olc_test.go` (`TestMsetBuilder`, `TestRecipeBuilder`).
  * Server integration test in `internal/server/economy_test.go` (`TestPriceCurveServerIntegration`).
  * Documentation updated in `internal/docs/engine/building.md`, `internal/docs/engine/systems.md`, `AGENTS.md`, and `agent/roadmap.md`.
* **Review fixes (Oct 9, 2026)**:
  * Shop stock counts match a prototype id, an instance id, or an exact keyword. A name that merely contains the stock id, such as "earring" against `ring`, does not make the good look scarce.
  * `shop.stock` may name an item or an NPC. A missing id still fails validation.
  * `ask` teaches a recipe and completes a talk objective only after one real reply. Several topic matches are ambiguous and do not teach or advance the quest. An unanswered question does not count as talking to that NPC.
  * Buying or inviting a mobile refunds the purchase, or leaves the party unchanged, when `StartFollowing` would make a loop.
  * A purchased companion with unset loyalty fills to `max_loyalty` when that is already set, and never stays above the max.
  * `order dismiss` also releases the player when the player is following that pet, and clears a party link that points at the pet.
  * Script lookup returns nothing when several mobiles or several other bodies match. It does not walk the entity map and edit the last hit.
  * `discovery: buy` is the method stored when the character reads that recipe's manual (`use`). Carrying the manual, or buying the finished good, does not teach it.

### Milestone 13: Pack Integrity Tools & Static Analysis (Oct 12, 2026)
* **Room Reachability Graph Analysis**:
  * Added BFS graph traversal in `internal/pack/validate.go` from `start_room` across exits.
  * Supported entry points from quest relocations (`relocate.to`) and destination rooms referenced in Lua scripts.
  * Catches and reports isolated/unreachable rooms that the player cannot reach.
* **Key & Lock Integrity**:
  * Unobtainable key detection: validates that every key required by a locked exit or container is actually obtainable in the world (placed in a room, inside a container, in NPC shop stock, in mob loot, as a quest reward, produced by a recipe, harvestable from a gather node, or referenced in a script).
  * Orphaned key detection: identifies defined key items that never lock any exit or container and are not referenced in scripts.
  * Reciprocal door consistency: detects conflicting keys specified on opposite sides of reciprocal doors.
  * Container key validation: verifies that container `key` properties reference valid items.
* **Script References & Lua Syntax Validation**:
  * Enforces that script references across rooms, scenery extras, items, NPCs, and quests point to existing files in `scripts/`.
  * Integrated `github.com/yuin/gopher-lua/parse` to validate the syntax of all Lua scripts in the pack during `sudengine validate` and pack load.
* **Testing & Documentation**:
  * Added unit test suites in `internal/pack/pack_test.go`: `TestValidateUnreachableRooms`, `TestValidateScriptReferencesAndSyntax`, and `TestValidateKeysAndLocks`.
  * Updated builder documentation in `internal/docs/engine/building.md` and agent tracking in `agent/roadmap.md`.

### Milestone 14: YAML Comment Preservation on Save Pack (Oct 12, 2026)
* **Non-Destructive World YAML Serialization**:
  * Upgraded `writeYAML` in `internal/pack/pack.go` to parse existing YAML files into AST `yaml.Node` structures before writing.
  * Implemented AST comment reconciliation (`transferComments`):
    * File-level and section-level header/foot comments are preserved.
    * Sequence elements (rooms, items, NPCs, recipes, quests) are matched by identifier (`id`, `key`, `name`, `item`) to carry over entry and field comments.
    * Mapping keys and values retain their `HeadComment`, `LineComment`, and `FootComment` through in-engine edits and round-tripping.
  * Configured YAML encoder with 2-space indentation matching existing pack style.
* **Verification & Documentation**:
  * Added unit tests in `internal/pack/pack_test.go`: `TestYAMLCommentPreservation` (rooms list with inline, block, and section comments) and `TestYAMLCommentPreservationMapping` (mapping configuration comments).
  * Verified OLC round-trip compatibility in `internal/olc/olc_test.go` (`TestOLCSavePackRoundTrip`).
  * Updated `AGENTS.md` and `agent/roadmap.md`.

### Review fixes (Oct 13, 2026)
* Script mentions and reachability match a whole id. `house.hall` does not match `house.hallway`.
* A key counts as placed from a room, a placed container, or a placed NPC. A container that is not in the world does not count, and a key does not count when it only sits inside the container it locks.
* Reciprocal keys are compared per opposite direction, so north/south and up/down are different doors.
* Every distinct Lua body is parsed, including an unsuffixed name whose `.lua` sibling has different text.
* An item is an orphan key only when its id is `key` or ends in `.key`, `_key`, or `-key`.
* List comments match `id` before `name`. A wrapped file with several lists uses the list named for that file. A failed read no longer replaces the YAML.

### Multiplayer guide (Oct 13, 2026)
* Rewrote `agent/features/multiplayer.md` as the build order. Stage 1 is who hears an event. Sockets, Telnet, and accounts come after a two-session test. `play` keeps full JSON snapshots. A shared world save is a paired write of `players/*.json` and `world.json`. Roadmap section 7 matches those five stages.

## Milestone 15: Multiplayer Stage 1 — Who hears an event (Oct 16, 2026)
* **Session Map & Routing (`internal/server/server.go`)**:
  * Added `Sessions map[string]*Session` to `Server`, initialized with the local session on `Launch`.
  * Added helper methods `sendTo(player, ev)` and `sendRoom(room, except, ev)`.
  * Output queue: vitals, map, combat, and prompt coalesce in a side queue. `Send` never reads `Out`. Room text and command results stay queued. A session whose `Out` stays full for two ticks is disconnected. `Run` returns only when the local session leaves.
* **Per-Player UI Pushes**:
  * Refactored `pushRoom`, `pushVitals`, `pushInv`, `pushMap`, `pushCombat`, and `pushPrompt` to accept variadic target sessions.
  * `pushRoomFor` renders occupants with ` (fighting)` and ` (linkdead)` suffixes, excludes the player themselves, and hides occupants and items when dark.
  * Added `pushRoomIn(room)` to broadcast room pane updates to all occupants on enter/leave.
* **Movement & Broadcasts (`internal/commands/commands.go`, `internal/server/server.go`)**:
  * Movement emits `leaves <dir>` to the old room and `arrives from <opposite>` to the destination room on `ChanNarrative`.
  * `say` emits `You say, "..."` to the speaker and `<Actor> says, "..."` to others in the room on `ChanSay`.
  * `emote` emits `<Actor> <text>` to all players in the room on `ChanNarrative`.
  * Followers pull and enter rooms once, receiving narrative follow echoes and room entry.
  * Combat swings, enemy hits, and Lua echoes (`h.Echo`, `h.EchoRoom`) route through room broadcasts without duplicate private echoes.
  * `SpawnPlayer` supports spawning multiple players with unique IDs.
* **Verification**:
  * Added `internal/server/multiplayer_test.go` (`TestStage1Multiplayer`, `TestStage1QueueCoalescing`) covering two sessions in one world, leave/arrive opposing broadcasts, say/emote echoes, room isolation, dark room item hiding, fighting/linkdead occupant tags, and write queue coalescing.
  * All tests pass: `go test -count=1 ./...` including `TestOldHousePlaythrough`.
* **Review fixes (Oct 19, 2026)**:
  * A full `Out` no longer drops narrative or reorders events under the TUI reader. The client flushes the side queue before each read.
  * A stalled or quitting second session is removed and sent `DisconnectEvent`. The local session is the only quit that stops `Run`.
  * Companion and stay mobiles skip wild AI when their owner or leader is any player, including a pet left behind or standing beside someone else.
  * Wander and pursue still tell the old room and the destination when a player follows. A following player gets one `on_enter`.
  * Death credit walks owner, leader, and party to a player. An empty `SlainBy` credits experience only when one character exists. Player death tells the fall room and the start room and refreshes both panes.
  * Private combat lines (gear break, collapse, assist) are marked at the `Emit` call. At this commit a hit line was still one room message. The swing split below gives that line to the fighter only.
  * `go test ./...` passed on Oct 19, 2026.
* **Swing lines (Oct 19, 2026)**:
  * Each swing gives the fighter the existing second-person line in private. Everyone else gets `hit_room` / `miss_room`, or a plain third-person sentence when those are omitted. The fighter does not also hear the room line. Two players in the same swing each get a second-person line, and the room line skips both.
  * Gear breaks, collapse, and assist calls stay private. `{Name} dies.` stays one room line. `Emit` takes an audience (`HearPrivate`, `HearRoom`, `HearOthers`) instead of a bool. Do not classify lines by their wording.
  * Sample attacks in Old House and Green Hollow set the room lines. `go test ./...` passed on Oct 19, 2026.

## Milestone 16: Multiplayer Stage 2 — Command queue & multiplayer verbs (Oct 21, 2026)
* **Server Command Channel & Rate Limiting (`internal/server/server.go`)**:
  * Replaced `case line := <-s.Session.In` with unified server message channel `s.In chan ServerMsg`.
  * Defined `ServerMsg` interface with `CommandMsg`, `ConnectMsg`, and `DisconnectMsg`.
  * Added `cmdQueue []string` to `Session` with `EnqueueCommand(line)`, `PopCommand()`, `CommandQueueLen()`, and `QueuedCommands()`.
  * Added session input pump (`monitorSessionIn`) so writes to `sess.In` automatically forward to `s.Enqueue(sess, line)`.
  * Rate-limiting: `s.tick()` dequeues at most one command per session before running the heartbeat (weather, regen, AI, combat, respawns). Commands queued beyond one per tick wait for subsequent ticks.
  * Added exported `s.Tick()` method for synchronous test execution.
  * Added `TickMS int` override to `Options` and initialized server message channels and session pumps on `Run()`.
* **Multiplayer Communication Verbs (`internal/commands/commands.go`, `internal/commands/parser.go`)**:
  * `tell <player> <text>`: private message on `ChanSay` to target player (`<Speaker> tells you, "..."`) with echo to speaker (`You tell <Target>, "..."`). Does not leak to the room or third sessions. Handles self-tells and unknown players gracefully.
  * `who`: lists all connected characters (`<Name> - <Role> (<Primary>:<Cur>/<Max>)`) with role name and primary resource. Omits any level column per design rules. Deterministic sorting by character name.
  * `gossip <text>` (alias `ooc`): global system message on `ChanSystem` delivered to every connected session across all rooms (`<Speaker> gossips, "..."`).
  * `party <text>` (alias `gt`): party chat on `ChanSay` delivered to all party members across rooms (`<Speaker> tells the party, "..."`) with echo to speaker (`You tell the party, "..."`). Uses `PartyLeader` without inventing a second group. Typing `party` without arguments displays the roster; party management subcommands (`invite`, `join`, `leave`, `disband`) continue to work.
  * `kill <player>` refusal: attacking any entity with `Kind == world.KindPlayer` is calmly refused with `"You cannot attack other players here."`, preventing unintended PvP until room PvP flags exist.
* **Documentation**:
  * Added `tell`, `who`, `gossip` / `ooc`, and `party` / `gt` to engine player manual in `internal/docs/engine/commands.md`.
* **Verification**:
  * Added test suites in `internal/server/multiplayer_test.go`:
    * `TestStage2CommandQueueRateLimit`: queues two commands, asserts first runs on tick 1, second waits, and second runs on tick 2.
    * `TestStage2TellPrivacy`: verifies private delivery to target, echo to sender, privacy from third sessions, self-tell and unknown player handling.
    * `TestStage2Who`: verifies all connected players listed with role name and primary resource, and confirms absence of a level column.
    * `TestStage2Gossip`: verifies global broadcast across different rooms and tests `ooc` alias.
    * `TestStage2PartyChat`: verifies party communication across rooms, `gt` alias, echo, isolation from non-party members, and non-party refusal.
    * `TestStage2KillPlayerRefusal`: verifies calm refusal when attempting `kill <player>` and confirms neither player enters combat.
  * All tests pass: `go test -count=1 ./...` including Old House playthrough (`TestOldHousePlaythrough`).
## Milestone 17: Multiplayer Stage 3 — Framed Protocol (`serve` & `connect`) (Oct 23, 2026)
* **Protocol JSON Struct Tags (`internal/protocol/protocol.go`)**:
  * Added `json:"..."` struct tags across all 11 `protocol.Event` structures and subviews (`TextEvent`, `PromptEvent`, `RoomEvent`, `ExitInfo`, `ResourceView`, `VitalsEvent`, `EquipView`, `InventoryEvent`, `MapRoom`, `MapEvent`, `FighterView`, `CombatEvent`, `LayoutEvent`, `ModeEvent`, `TitleEvent`, `DisconnectEvent`).
  * In-process channels continue to pass native Go structs without overhead, while remote network sessions serialize and deserialize cleanly.
* **Length-Prefixed Framed Transport & Codec (`internal/net/frame.go`)**:
  * Implemented binary framing with a 4-byte big-endian `uint32` length prefix (`WriteFrame`, `ReadFrame`). A frame may be at most 256KB. The length is rejected before the body is allocated, and the body read has a deadline so a stalled peer cannot sit on that buffer. An idle connection is not timed out while it is waiting for the next length prefix.
  * Preserves newlines safely inside room descriptions, dialogue, and art banners without line-splitting ambiguity.
  * Event encoder and decoder (`EncodeEvent`, `DecodeEvent`) wraps each event in a typed envelope with `"t"`:
    `{"t":"text", ...}`, `{"t":"prompt", ...}`, `{"t":"room", ...}`, `{"t":"vitals", ...}`, `{"t":"inventory", ...}`, `{"t":"map", ...}`, `{"t":"combat", ...}`, `{"t":"layout", ...}`, `{"t":"mode", ...}`, `{"t":"title", ...}`, `{"t":"disconnect", ...}`.
  * Client frame codec (`EncodeCommand`, `EncodeConnect`, `EncodeDisconnect`, `DecodeClientFrame`) supports command lines, connect handshakes with character metadata, and disconnect notifications.
* **Server-Side Networking & Dedicated Server Mode (`internal/net/server.go`, `internal/server/server.go`)**:
  * Added `server.LaunchServer(Options)` for starting headless multiplayer servers without an initial local player session. Extracted shared server subsystem wiring into `newServerInstance`.
  * `LaunchServer` rejects build mode (`protocol.ModeBuild`) and validates the pack before instantiating the world.
  * `SpawnPlayerSession` still builds a session for in-process tests. A framed connection does not call it. `ConnectMsg` carries the name, race, role, and optional first command. The actor spawns the entity, picks a free session id, and attaches the session before greet. Two connects with the same name no longer share one map slot.
  * Implemented `NormalizeAddr` and `Listen` enforcing loopback-only binding (`127.0.0.1:<port>`) and refusing public or wildcard hostnames.
  * `Serve` and `ServeConn` manage connection lifecycles. The connection goroutine only enqueues. One accept error is logged and the listener stays up. The outbound loop flushes the side queue, so a full `Out` still delivers `DisconnectEvent`. Cancelling the serve context closes the socket. Sends onto `Server.In` return once `Run` has stopped.
  * `sudengine serve` returns the listen error. `--port` must be in `1..65535` and must have a value. The "Serving" line is printed after the bind succeeds.
  * Remote command queues stop at 32, the same cap as the local input buffer.
  * Startup extinguish runs after the local session is attached, so the flame line is delivered. A headless `LaunchServer` still puts the lights out with nobody to tell.
  * `Dial` closes its wake channel once, so client cleanup and the reader can both run.
* **Remote Client & Bubble Tea Integration (`internal/net/client.go`, `internal/client/client.go`)**:
  * Implemented `net.Dial(ctx, addr, name, race, role)` connecting to remote servers, sending initial connect handshake frame, and returning a connected `server.Session`.
  * Added `client.ConnectPlay(sess, cancel)`: boots the Bubble Tea TUI in `stateGame` using the remote session.
  * Remote client renders the exact same UI panes (output log, side status, room description, exits, vitals, inventory, map, combat) as local `play`.
* **CLI Subcommands (`cmd/sudengine/main.go`)**:
  * `sudengine serve <pack> [--port 4000]`: loads, validates, and serves pack on `127.0.0.1:4000` (or custom `--port`). Rejects build mode.
  * `sudengine connect <host:port> [--name <name>] [--race <race>] [--role <role>]`: launches Bubble Tea client connecting over framed protocol.
  * Updated engine help documentation.
* **Verification & Test Suite**:
  * `internal/protocol/protocol_test.go`: verified JSON serialization/deserialization round-tripping.
  * `internal/net/frame_test.go`: verified length-prefix framing, multi-frame streaming with newlines, event encoding/decoding across all 11 types, client frame codec, and exact spec JSON envelope structure (`{"t":"text","channel":"say","text":"..."}`).
  * `internal/net/server_test.go`:
    * `TestListenerAcceptsFramedConnectionAndCommand`: accepts framed connection on ephemeral port, emits `RoomEvent`, and reads command frame back onto actor loop.
    * `TestConnectShowsSameRoomAsPlay`: verifies remote `connect` to localhost receives the exact same room ID, title, and description as local `play`.
    * `TestServeRejectsBuildMode`: verifies `serve` refuses build mode.
    * `TestMultipleFramedConnections`: verifies multiple concurrent TCP clients can interact in real-time (`say`, `tell`).
  * `cmd/sudengine/main_test.go`: verifies CLI argument validation and build rejection.
  * All tests pass: `go test -count=1 ./...` including Old House playthrough (`TestOldHousePlaythrough`).
* **Review fixes (Oct 26, 2026)**:
  * Tests cover an oversize frame, a stalled body, an accept error, an unknown handshake, two players with the same name, idempotent `Dial` cleanup, a disconnect flushed from a full `Out`, the command cap, enqueue unblocking after shutdown, startup flame delivery, a bad `--port`, and a failed bind.
  * `go test -count=1 ./internal/server ./internal/commands ./internal/pack ./internal/client ./internal/net ./cmd/sudengine` passed. The race detector was not run: this machine has no gcc, and `-race` needs cgo.

## Milestone 18: Multiplayer Stage 4 — Telnet (`internal/net/telnet.go`) (Oct 27, 2026)
* **Telnet Listener & Server Integration (`internal/net/telnet.go`, `cmd/sudengine/main.go`)**:
  * Added `DefaultTelnetPort = 4023`, `ListenTelnet`, `ListenAndServeTelnet`, `ServeTelnet`, and `ServeTelnetConn`.
  * `sudengine serve <pack>` now starts the Telnet listener on `127.0.0.1:4023` (configurable via `--telnet <port>`, or disabled with `--telnet 0`) alongside the framed listener on `127.0.0.1:4000`.
* **IAC-Aware Byte Stream Reader (`TelnetReader`)**:
  * Implemented an RFC 854/857 state machine (`TelnetReader`) that strips `IAC WILL/WONT/DO/DONT <opt>`, `IAC SB ... IAC SE` subnegotiations (even when subnegotiation payloads contain `0x0A`), 2-byte `IAC` commands, `\r`, `\x00`, and handles backspace (`0x08` / `0x7F`) without using `bufio.Scanner`.
  * Ensures IAC negotiation bytes are never dispatched as game commands.
* **Password Echo Suppression**:
  * Implemented `SuppressEcho` (`IAC WILL ECHO`, `0xFF 0xFB 0x01`) before password entry and `RestoreEcho` (`IAC WONT ECHO`, `0xFF 0xFC 0x01`) immediately after password entry, never echoing password characters back over the socket.
* **ANSI-16 Event Rendering (`FormatTelnetEvent`, `theme.ANSI16()`)**:
  * Exported `theme.ANSI16()` for pure 16-color terminal output without Omarchy truecolor overrides.
  * Renders `RoomEvent` (title, description, items, occupants, exits), `PromptEvent`, `VitalsEvent`, `TextEvent` (channel-colored lines), and `DisconnectEvent` while omitting TUI-only `MapEvent`, `CombatEvent`, and `InventoryEvent` panes.
* **Verification**:
  * Added `internal/net/telnet_test.go`:
    * `TestTelnetRawLookReceivesRoomTitle`: raw TCP connection sends `look\n` and receives the formatted room title and exits.
    * `TestTelnetIACSequenceConsumedNotDispatchedAsCommand`: unit and end-to-end TCP tests verifying IAC negotiations and subnegotiations are stripped and not treated as commands.
    * `TestTelnetEventFormatting`: verifies ANSI-16 rendering of room, prompt, vitals, and text events and omission of map/combat/inventory events.

## Milestone 19: Multiplayer Stage 5 — Accounts, Paired Saves, Link-Dead & Server Chargen (Oct 27, 2026)
* **Accounts & Bcrypt Password Hashing (`internal/save/multiplayer.go`)**:
  * Added `Account` persisted at `saves/<pack>/accounts/<account_id>.json` with `golang.org/x/crypto/bcrypt` password hashes (`CreateAccount`, `AuthenticateAccount`, `LoadAccount`, `SaveAccount`, `AddCharacterToAccount`).
  * One account can own and select among multiple characters.
* **Paired Atomic World & Player Saves (`internal/save/multiplayer.go`, `internal/save/save.go`)**:
  * Added `PlayerSave` persisted at `saves/<pack>/players/<character>.json` containing only that character's entity subgraph (character + carried/equipped inventory tree) and current room ID.
  * Added `CutWorldWithoutPlayers` and `WritePaired` / `WritePairedWithHook` writing `saves/<pack>/world.json` (shared world state with all player entities and their inventory subgraphs stripped) alongside `saves/<pack>/players/*.json`.
  * `paired.commit` is written before the live renames and removed only after all of them succeed. A leftover marker restores each file from `.bak`. A `.bak` is deleted only after that restore succeeds. With no marker, leftover `.bak` and `.tmp` files are discarded.
  * Single-player `play` continues to use `save.Write` and `saves/<pack>/<slot>.json`; `save.ListInfo` skips `world.json` so single-player slot lists remain clean.
  * Server autosaves on tick intervals (`AutosaveTicks`) and on server shutdown when `Persist` is enabled.
* **Link-Dead Grace Period & Combat `quit` (`internal/server/server.go`)**:
  * Dropped sockets (`closed` / `stalled`) and typing `quit` while in combat (`pl.InCombat()`) both enter the same link-dead grace period (`GraceTicks`, default 60s), setting the `linkdead` flag and leaving the character in the room and in the fight.
  * Reconnecting during the grace period resumes that body only when the session account owns it.
  * When the grace period expires, `reapLinkdead` runs after `reapDead`: if the character died during the grace period, death rules run first (recalling them to `StartRoom` and restoring HP). The character is saved and despawned only after that save succeeds. A failed save leaves them in the room and retries.
* **Same-Room Party Kill Credit & XP Sharing (`internal/server/server.go`)**:
  * Updated `killRecipients` in `reapDead` so party members present in the same room when a mob dies share quest kill credit and XP, while party members in other rooms receive neither. Pet and companion credit attribution remains intact.
* **Server-Driven Chargen for Telnet (`internal/net/telnet.go`, `internal/rpg/rpg.go`, `internal/server/server.go`)**:
  * Added `Chargen.PointBuy` / `Chargen.Budget()` support in `internal/rpg` and `ConnectMsg.Attrs` application in `bindConnectedPlayer`.
  * Telnet login walks new characters through origin/race bundles, role bundles, and point-buy attribute allocation before spawning into the world.
* **Verification**:
  * Added unit tests in `internal/save/save_test.go`: `TestMultiplayerTwoCharactersSaveAndLoadWithoutCrossInventory`, `TestMultiplayerFailedSecondWriteLeavesPreviousPairInPlace`, `TestSinglePlayerOldSlotCoexistsWithMultiplayerSaves`, and `TestAccountBcryptAndMultipleCharacters`.
  * Added integration tests in `internal/server/multiplayer_test.go`: `TestStage5LinkDeadStaysInRoomAndDespawnsAfterGrace`, `TestStage5CombatQuitSharesGraceAndDeathRecallsBeforeSave`, `TestStage5OldSlotStillLoadsInPlay`, and `TestStage5PartyKillCreditAndXPInSameRoomOnly`.
  * Added end-to-end Telnet account/echo-suppression/chargen test `TestTelnetAccountLoginEchoSuppressionAndChargen` in `internal/net/telnet_test.go`.
* **Review fixes (Oct 27, 2026)**:
  * A paired write always stages every live character with `world.json`. One character quitting no longer duplicates or drops another character's items.
  * Link-dead reap and a clean remote quit leave the character in the room when the save fails. Process shutdown drops sessions without starting grace. A client disconnect reason of `shutdown` starts grace.
  * `sudengine serve` rejects an anonymous connect. A saved or link-dead body resumes only for the owning account. An empty account on a character file cannot be claimed. Guest connections on a non-persistent server can still share a display name.
  * Telnet `IAC IAC` inside a subnegotiation stays inside it, and a data `IAC IAC` becomes a literal `0xFF`. Output doubles `0xFF` and strips ESC from text before theme colors are added. `ReadLine` fails at 256KB. A failed bind on the default Telnet port is returned. `--telnet 0` still disables Telnet.
  * `golang.org/x/crypto` is a direct module requirement.
  * `go test -count=1 ./internal/save ./internal/net ./cmd/sudengine ./internal/server ./internal/commands ./internal/pack` passed. The race detector was not run.
