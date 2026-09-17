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

`stats` or `score` prints current / max. They regen on the tick while you are not fighting.

## Traits

Attributes (`body`, `will`, …) are numbers used later by formulas and scripts. Skills are ranks. Neither is required.

## Origin and role

Optional lists in `rpg.yaml`. If present (or `chargen.enabled: true`), New Game asks for a name and those picks. A role can bump resources (Careful gets more Calm; Reckless more Grit).

## Actions

Abilities in `abilities/*.yaml` become extra verbs. Example: `steady` spends Calm and helps Grit. Cost, cooldown, and effects are data. Lua is only for weird cases.

## Combat

`kill <who>` starts a fight. Both sides auto-attack on the tick. Your swings are plain text; theirs are colored. `flee` leaves through an open exit. Death (by default) drops you at the start room with half of the primary resource.

Hostile NPCs: `ai.profile: aggressive`. They attack if they share your room.

## Items in the system

Keys match an exit's `key:` field. A lamp with `use.toggle_flag: light` is how you see in `dark` rooms. Clothes use `wearable` + `slot`. None of that is hard-coded to fantasy names.

See `help items` and `help building`.
