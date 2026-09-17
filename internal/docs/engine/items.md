# Items

Yes — you can pick up, wear, and use things.

```
get lamp
inventory
use lamp
light lamp          same as use
wear coat
wear knife            wield slot; combat uses its damage
remove coat
drop lamp
put lamp in chest
give lamp to maid
```

`look lamp` (or `examine lamp`) reads the long description.

## Usable gear

If an item defines `use:` in YAML, `use <item>` runs it. The Old House lamp toggles a `light` flag so dark cellar rooms become visible.

If nothing happens: you are not holding it, or it has no `use` block and no Lua `on_use`. The game will say you are not sure how to use it.

## Keys and doors

```
unlock down
open down
```

The key must be in your inventory. Its prototype id or keywords should match the exit's `key:` field.

## Building usable items

In `world/items.yaml`:

```
use:
  toggle_flag: light
  message: You light the lamp.
  message_off: You douse the lamp.
```

Or attach `scripts: lamp.lua` with `on_use` and `return true`. See `help building`.
