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

See `help items` and `help building`.
