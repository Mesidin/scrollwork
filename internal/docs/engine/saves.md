# Saves

Sudengine stores a **full snapshot** of the live world: you, rooms, NPCs, items, combat, tick count.

```
save              write slot "autosave"
save attic        write slot "attic"
quit              autosave, then exit
```

Files land in `saves/<pack-id>/<slot>.json`.

The intro screen lists those slots under **Continue**. Picking one restores that snapshot exactly.

## New game vs continue

- **New game** instantiates the pack from YAML (plus character creation if the pack enables it).
- **Continue** does not re-read the pack's rooms; it loads the snapshot.

Updating a pack later will not upgrade old saves. Start a new game to see pack edits. Build mode writes the pack, not a playthrough.

## Slots

Use short names (`autosave`, `before-cistern`). There is no autosave-on-timer in v1 — only `save` and `quit`.
