# Playing

Sudengine is a single-player MUD: you type commands, the world answers, and time still passes if you sit at the prompt.

## Starting

Pick a pack from the launcher (`sudengine`) or run `sudengine play <pack>`.

The intro screen shows that game's art and a short blurb. From there:

- **New game** — start a fresh world. If the pack enables character creation, you will name yourself and pick any race/role it defines.
- **Continue** — restore a snapshot (only listed when saves exist).
- **Help** — read game and engine manuals without entering the world.

## The screen

The prompt sits at the bottom. The current **room name** is the accent-colored title in the log and on the prompt. Hostile arrivals and first strikes show as alert lines (a `▸` marker). Combat swings use a separate combat color.

The TUI follows your terminal palette (ANSI-16). On Omarchy it also reads the active theme:

```
~/.local/state/omarchy/current/theme/colors.toml
~/.local/state/omarchy/current/theme/sudengine.toml   # optional overlay
```

Portable overrides (any OS): `~/.config/sudengine/theme.toml` or `SUDENGINE_THEME=/path/to/file.toml`. `NO_COLOR` disables color.

Packs may also show a map, vitals, exits, inventory, and combat. Those panes are defined in `pack.yaml` under `ui.panes`.

Status on the bottom bar is the room name plus your primary resources (whatever the pack calls them — HP, Grit, Blood, …).

## Talking to the parser

Commands are MUD-style, not English puzzles:

```
look
n
get lamp
ask maid about key
kill rat
```

Abbreviations work (`l`, `i`, `n/s/e/w/u/d`). See `help commands`.

## Time and combat

The world ticks in real time. NPCs wander. Aggressive ones attack if you share a room. `kill <who>` starts a fight; both sides auto-attack until someone dies or you `flee`.

## Saving

`save` writes a full snapshot (default slot `autosave`). `quit` saves autosave and exits. See `help saves`.
