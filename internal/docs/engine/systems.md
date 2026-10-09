# Systems

Sudengine does not ship a fixed RPG. Each pack declares what exists.

## Resources

Bars that go up and down: Grit, Calm, HP, Blood, Sanity — whatever `rpg.yaml` names.

```
resources:
  - key: hp
    label: Grit
    max: 24
    regen: 1
    regen_every: 20
```

`key` is the engine name. `label` (and `lexicon.yaml`) is what you see. Combat usually spends the pack's `primary_resource` (here `hp`, shown as Grit).

`show` decides how loud a resource is:

- `bar` draws a meter on the status line and in the side pane. The primary pool does this when `show` is omitted.
- `side` keeps it in the side pane only, as current / max, or as an amount when `pile: true`.
- `sheet` leaves it on `stats` alone.

A pile is not a pool. Coin and gold use `pile: true`. They do not regen, and they are not drawn as a meter.

`stats` lists every skill the pack defined, including ones you have not trained. The number is the effective rank (trained ranks plus the parent attribute). `train` with no skill prints the same list and the price. `improve` with no skill prints it and your unspent points.

## Traits

Attributes (`body`, `will`, …) are numbers used later by formulas and scripts. Skills are ranks. Neither is required.

## Origin and role

Optional lists in `rpg.yaml`. If present (or `chargen.enabled: true`), New Game asks for a name and those picks. A role can bump resources (Careful gets more Calm; Reckless more Grit).

## Actions

Abilities in `abilities/*.yaml` become extra verbs. Example: `steady` spends Calm and helps Grit. Cost, cooldown, and effects are data. Lua is only for weird cases.

## Combat

`kill <who>` starts a fight. Both sides auto-attack on the tick. Your swings are plain text; theirs are colored. `flee` leaves through an open exit. Death (by default) drops you at the start room with half of the primary resource.

Hostile NPCs: `ai.profile: aggressive`. They attack if you share a room. `aggro: look` or `aggro: flag` waits until you examine them or a story event sets `hostile`. `rooms` limits where they wander and how far they chase. `pursue: true` follows you out of the room and stops at the edge of that list.

## Checks

A check is a named roll in `rpg.yaml`. The usual shape is `2d6` plus a few modifiers, over a difficulty. `mode: under` is there for a pack that wants a percentile roll. If the check is absent, the verb does not roll: `flee` always works, a blow always lands, and `search` finds every hidden thing.

`actor` lists attributes and skills (`attr.will`, `skill.melee`). A skill's parent attribute is already inside the skill, so do not list both. `gear` adds the mods of worn items. An opponent's `oppose` number replaces the default difficulty when they have one.

`feedback.rolls` appends the arithmetic. `feedback.numbers` appends `[-4 Grit]` after a templated blow. Both default off.

## Noticing

An extra or item with `hidden: true` is left out of the room text. `notice` is the difficulty on the way in, when `checks.notice` exists. `search` is the difficulty for `search` and for `look <word>`. A success is remembered. With no notice check, `search` reveals everything and does not roll.

## Advancement

Leave `advancement` out and there are no levels. When it is on, a mobile's `xp` is awarded on death. Reaching a level grants skill points and, every `attribute_every` levels, one attribute point. `resources` raises those maximums.

`improve <skill>` spends a skill point. `raise <attribute>` spends an attribute point. Attributes are not for sale. `train <skill>` pays a trainer (`trainer: true`) in the pack's currency and buys a skill rank. A skill can name a `parent` attribute and a `max`. `damage_every` adds damage on attacks that name that skill. An ability can `require` a rating.

## Items in the system

Keys match an exit's `key:` field. A lamp with `use.toggle_flag: light` is how you see in `dark` rooms. Clothes use `wearable` + `slot`. None of that is hard-coded to fantasy names.

## Environment: Weather & Time

Packs turn this on in `environment.yaml`. `weather.yaml` is read when that file is missing or leaves both levels at 0. Leave both out and the clock and the sky stay still.

```yaml
time:
  level: 1              # 0 off, 1 cosmetic, 2 lighting, 3 night notice
  minutes_per_day: 24   # 24 real minutes = 24 game hours
  start_hour: 8
  night_notice: -2      # outdoor notice at night when time level is 3; 0 means -2
  phases:
    night:
      outdoor: Night falls, and the open dark closes in.
      window: The window goes dark.

weather:
  level: 1              # 0 off, 1 cosmetic, 2 light and notice, 3 combat and flames
  initial: clear
  change_interval: 240  # ticks between weather rolls
  states:               # omit to keep the built-in clear/rain/storm table
    mist:
      label: a cold mist
      sky_desc: A cold mist hides the hills.
      sky_desc_night: The mist glows under a hidden moon.
      light_mod: -1
      notice_mod: -2
      combat_mod: -1
      extinguish: false
      outdoor_echoes:
        - Damp mist drifts across the ground.
      window_echoes:
        - Mist presses against the window.
      arrive_outdoor: A cold mist rolls in.
      arrive_window: Mist gathers outside the window.
      transitions:
        clear: 1
```

`states` replaces the whole built-in table. The built-in names are clear, overcast, fog, rain, storm, snow, and blizzard. Storm and blizzard set `extinguish`.

- **Scale 0**: Off.
- **Scale 1 (Cosmetic)**: The clock advances. Weather changes. Outdoor rooms, windowed rooms, and (for thunder) other rooms hear an echo. `time`, `weather`, `sky`, and `look sky` work. Ambient lines are rolled once every 80 ticks.
- **Scale 2 (Light and senses)**: Night makes an outdoor room dark unless someone there has a light, or the room is `lit`. Daylight lights a dark windowed room unless `light_mod` is -2 or lower. Outdoor `notice` and `search` take `notice_mod`.
- **Scale 3 (Mechanical)**: Outdoor `hit` checks take `combat_mod` when the pack defines `hit`. A state with `extinguish: true` blows out lamps (`use.toggle_flag: light`) in outdoor rooms, and a new flame will not catch there until the weather eases. Time scale 3 adds `night_notice` to outdoor notice and search during the night.

`sky_desc` is the sentence for `weather` and `look sky`. `sky_desc_night` replaces it at night when the clock is on. `arrive_outdoor`, `arrive_window`, and `arrive_indoor` are the lines when that weather begins. `phases` replaces the built-in dawn-to-midnight lines for that phase.

One weather state covers the whole pack. The current state is stored on the save. Continuing a game keeps the hour and does not repeat the phase line for a change that already happened.

Room flags: `outdoor` or `outdoors`, `windowed` or `sheltered`, `underground`, `lit`. See `help building`.

## Economy, Durability & Crafting

Packs can enable tiered economy mechanics across equipment, commerce, and production:

### Durability & Repair
Items may define `durability` and `max_durability`. `repairable: true` opts an item into repair. Omitting it leaves the item unrepairable.
A landed hit wears the wielded weapon by one point, and wears every worn armor piece by one point.
Condition states range from *pristine* to *good*, *worn*, *damaged*, and *broken*.
A broken weapon still swings, for 1d2. Broken armor soaks nothing.
NPCs with `repairer: true` (or the `repairer` / `blacksmith` flag) mend gear for the shop currency, or the pack's advancement currency when the NPC has no shop (`repair`, `repair <item>`, `repair all`).
`repair all` fixes every piece you can afford and names the ones left undone.

### Merchant Purses & Commerce
`shop.purse` (including 0), `shop.max_purse`, or `shop.dynamic: true` makes the till finite, and it stays finite after the balance hits zero.
Merchants buy only what the till can cover. Buying from them refills the till, up to `max_purse` when that is set.
`shop.price_curve: true` enables floating supply-and-demand price curves:
- Price scales based on base stock versus current vendor stock:
  $$\text{Effective Price} = \text{Base Price} \times \left(1 + \frac{\text{Base Stock} - \text{Current Stock}}{\text{Base Stock} + 5}\right)$$
- Rare or bought-out goods become more expensive.
- Flooding a merchant with surplus items drops their buy-price down to nominal scrap value (1 coin).
Damaged goods sell for value scaled by remaining durability. An item with nothing left does not sell.
A pile resource, such as coin, can grow past its starting amount.

### Banking & Vaults
NPCs or scenery items marked with `banker: true` (or the `bank` flag) act as common vaults.
- `deposit <amount|all> [currency]` moves a loose pile (or the advancement currency) into the vault. `deposit all coin` names the pile.
- `withdraw <amount|all> [currency]` brings it back. A capped pool that cannot hold the whole withdrawal leaves the rest in the vault.
- `balance` (or `account`, `bank`) displays all stored currencies.
Funds in your bank are safely preserved across character deaths.

### Crafting Pipeline & Skill Progression
Packs list recipes in `recipes.yaml` (a bare list, or a `recipes:` list). Each recipe has:
- `id` and `name`
- `station` (optional keyword or flag in the room, such as `anvil` or `workbench`)
- `tools` (carried or worn items required to craft, not consumed)
- `requires` (ingredient prototype ids and counts; worn items are not consumed; an omitted count means 1)
- `produces` (an item id, or `{item, count}`)
- `discovered` (optional boolean, defaults to true). When false, the recipe is locked until it is taught or learned.
- `discovery` (optional method: `grant`, `buy`, `discover`, or `all`). `all` starts unlocked; `grant`, `buy`, or `discover` locks the recipe and records the learning method on the character.
- `skill` (optional check in `rpg.yaml`). A set `difficulty` replaces the check's difficulty. A failed roll ruins one ingredient. A success at least 3 above the target that was rolled produces a superior item with bonus durability.
- `min_skill` (optional minimum trained rank in the named skill). The parent attribute does not count.

Learning recipes:
- **Taught by NPCs**: `NPCYAML.teach_recipes` maps dialogue topics to recipe IDs (recorded as `grant`).
- **Quest Rewards**: Quests award recipes upon turn-in (recorded as `grant`).
- **Learned from Items**: Items with `use.learn_recipe: "<id>"` teach the recipe when `use` is performed (recorded as `discover` or the recipe's declared method).
- **Lua Scripts**: Scripts can call `learn_recipe("id", [actor], [method])` and check `get_recipe_method("id", [actor])`.

Commands:
- `recipes` (alias `rlist`) lists only your known recipes and marks whether station, tools, and materials are on hand.
- `craft` (alias `make`) matches an exact name, id, or whole word to craft the item.

### Resource Gathering & Perception
Rooms or scenery items can define a `gather:` node:
- `resource` (prototype ID spawned upon harvesting)
- `name` (display name of the resource)
- `skill` (check to roll, defaulting to `notice`)
- `difficulty` (difficulty threshold to find resources)
- `bonus_dc` (higher DC threshold granting extra yields for sharp perception)
- `remaining` and `max` (harvest capacity)
- `regen_every` (ticks between node replenishment)

Commands:
- `gather` / `forage` harvests available materials. A perception roll exceeding `bonus_dc` awards bonus resources. Depleted nodes regenerate over time.

## Quests, Dialogue & Objectives

Packs define adventures and storyline tasks in `quests.yaml` (or `world/quests.yaml`).

### Schema
Each quest includes:
- `id`: unique identifier string
- `title`: display name of the quest
- `desc`: narrative briefing / description
- `giver`: prototype ID of the NPC who offers the quest
- `turnin`: prototype ID of the NPC to whom completed tasks are turned in (defaults to `giver`)
- `auto_start`: boolean (optional). Automatically starts when available without needing manual acceptance.
- `requires_quest`: list of quest IDs that must be completed before this quest unlocks.
- `objectives`: list of task objectives:
  - `id`: identifier for progress tracking
  - `desc`: human-readable objective description
  - `kind`: `kill`, `collect`, `visit`, or `talk`
  - `target`: prototype ID, room ID, keyword, or NPC name
  - `count`: quantity required (defaults to 1)
  - `consume`: boolean (optional, for `collect`). Consumes required items upon completion.
- `rewards`:
  - `xp`: experience points granted
  - `currency`: currency key (defaults to pack primary advancement currency or `coin`)
  - `amount`: currency amount awarded
  - `items`: list of item prototype IDs granted to player inventory
  - `recipes`: list of recipe IDs taught upon completion
  - `relocate`: list of NPC moves upon completion (`mob`, `to`, `update_home`, `message`)
  - `message`: dialogue response or closing narrative text

Quests can also specify an optional `script: scripts/foo.lua`. Turning a quest in marks it completed, then runs `on_quest_complete(actor, self, quest_id)` on the turn-in mobile's script. If that mobile has no script, the giver's live instance is used (a prototype id such as `hollow.bram` matches the spawned body). The quest script runs after that. `relocate` happens after the hook, and each `message` is shown only when that move succeeds. A missing or ambiguous mobile or room is reported, and the quest stays completed.

`teleport(who, where, [update_home])` (alias `move`) moves a mobile, item, or the player into a room. It will not move a room, or move something into a non-room. When the player is moved, the fight ends and the normal leave/enter path runs, including visit objectives. Other teleports stay quiet unless the script echoes. `update_home` sets `HomeRoom` and adds the room to a restricted `ai.rooms` leash; that leash is copied onto the respawn. Omit `update_home` for a one-shot move.

`is_quest_completed(quest_id, [who])` and `is_quest_active(quest_id, [who])` read one status. `who` defaults to the actor and may be an id, a prototype, or a keyword. `has_quest(quest_id, [who])` is true when the quest is active or already completed. The completion hook sees status `completed`.

### Commands
- `quests` (or `tasks`, `journal`): list active quests with objective progress, and completed quests.
- `quest <id>`: display detailed briefing, objectives status, and rewards.
- `quest accept <id>`: accept a quest from an NPC present in the room.
- `quest complete <id>`: turn in finished objectives to the recipient and claim rewards.
- `talk <npc>`: displays offered quests and ready turn-ins on the NPC.

## Hunting, Loot Tables & Field Survival

Packs define mobile loot tables, salvaging, field repair kits, and dynamic mob respawns in `world/npcs.yaml` and `world/items.yaml`.

### Loot Drops & Corpse Skinning
NPCs can define `loot`:
- `item`: prototype ID of the dropped item.
- `chance`: percentage chance to drop (1-100, default 100).
- `count`: number of items dropped (default 1).
- `skin_only`: boolean. If false (default), drops directly into the room on death. If true, remains on the corpse and must be harvested using `skin corpse` or `butcher corpse`.
- When butchered, corpses gain the `skinned` flag, update their short description to `(butchered)`, and cannot be skinned a second time.

### Salvage & Deconstruction
Items can define `salvage` in `world/items.yaml`:
- `item`: component prototype ID produced.
- `count`: quantity produced.
The `salvage <item>` command (aliases `breakdown`, `dismantle`, `deconstruct`) dismantles an item in inventory into its constituent components and consumes the item.

### Field Repair Kits
Items can function as field maintenance tools (e.g. whetstones, sewing kits, leather patches) by defining `repair_kit`:
- `uses`: remaining uses before the kit is consumed.
- `max_uses`: maximum capacity.
- `amount`: durability restored per application (0 restores fully).
- `skill`: optional skill check key required (e.g. `crafting`).
- `dc`: difficulty check (defaults to 7).
Field repair kits allow adventurers to repair equipment out in the wild using `repair <item> with <kit>` or `use <kit> on <item>`. When away from repairers, `repair <item>` also attempts to use a carried repair kit.

### Dynamic NPC Respawns
NPCs can define `respawn_every: <ticks>` in `world/npcs.yaml`.
When slain, the server schedules a respawn task. Once the interval expires, a fresh instance is spawned at the mobile's original home room and an arrival notification is sent to any players in that room.

## Followers, Parties & Companions

Packs can configure companion mobiles, pets, and party members in `world/npcs.yaml` and via Lua scripts.

### Following
- `follow <who>` (alias `fol`): Begin following another character or mobile. If no argument is given, lists current leader and followers. Loops (e.g. A following B following A) are refused.
- `unfollow` (aliases `unfol`, `stopfollow`): Stop following your current leader.
- `lose <who>` (aliases `ditch`, `shake`): Remove someone currently following you.
- When a leader changes rooms (walk, flee, wander, pursue, scripted teleport, or quest relocation), followers still in that room come along. A player who is following gets a normal room entry. `stay`, immobile, and fighting followers stay behind and do not catch up on a later exit.
- On death, owned companions are brought to the start room. A companion told to `stay` is left behind and stops following.

### Parties
- `group` (aliases `party`, `gt`): Displays active party members, leader, levels, and primary resource status.
- `invite <who>`: Invite another player or mobile into your party. Companions and NPCs with `party_member: true` automatically accept.
- `join <who>`: Accept an outstanding party invitation or join an NPC whose party is flagged `open_party: true`.
- Party leaders can disband or remove members; members can leave using `group leave`.

### Companions & Pets
NPCs can declare companion and loyalty attributes in `world/npcs.yaml`:
- `companion`: boolean. Marks the NPC as an ally or pet.
- `owner`: entity ID of the owner.
- `loyalty` and `max_loyalty`: numeric loyalty pool (defaults to 100).
- `stance`: combat and movement demeanor (`assist`, `defend`, `passive`, or `stay`).
- `value`: purchase cost when stocked by an NPC shopkeeper.

#### Pet Commands & Care
- **Acquisition**: Can be purchased from vendors (`buy puppy from vendor`) or tamed in the wild.
- **Feeding**: `feed <pet> with <food>` (or `feed <food> to <pet>`) works on a companion or an animal flagged `tameable` or `pet`. Food is an item flagged `food` or `edible`, tagged as food, or carrying an exact food keyword. It restores loyalty (+15) and heals a portion of the primary resource. Animals flagged `tameable` or `pet` become loyal companions once their loyalty reaches 30.
- **Orders**: `order <pet> <action>` (alias `command`) requires a pet you own, or an unowned companion already following you. Party membership alone does not grant orders.
  - `follow`: Resumes following and restores the stance from before `stay`.
  - `stay`: Holds position in the current room and stops following. The previous stance is remembered.
  - `stance <assist|defend|passive|stay>`: Configures combat engagement stance.
  - `attack <target>` (or `kill`): Commands pet to attack a specific mobile.
  - `dismiss`: Releases companion, clearing ownership and following links.
  - *Loyalty Check*: If loyalty drops below 20, pets may stubbornly growl and ignore orders.

### Combat Stances & Assist
When a leader enters combat or is attacked, nearby companions and party members respond based on their stance:
- `assist` (default): Immediately engages whoever the leader attacks or is attacked by.
- `defend`: Engages only enemies that actively attack or target the leader.
- `passive`: Refrains from entering combat unless explicitly ordered to attack.
- `stay`: Remains in place and does not join battles. Owned, following, and `stay` mobiles also skip aggro, wander, and pursue.

### Quest Objectives & Experience
- Kills landed by pets, companions, or party members count toward the player's active `kill` quest objectives.
- Defeating enemies alongside companions grants the player their full earned experience.

### Scripting API
Lua scripts can manipulate companions and parties using sandbox bindings:
- `set_companion(mob, [owner], [loyalty], [stance])`
- `get_loyalty(mob)` / `set_loyalty(mob, amount)`
- `get_stance(mob)` / `set_stance(mob, stance)`
- `follow(follower, leader)` / `unfollow(follower)`
- `join_party(member, leader)`, `invite_party(inviter, invitee)`, `leave_party(member)`


