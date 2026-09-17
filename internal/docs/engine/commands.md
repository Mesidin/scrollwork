# Commands

Builtin verbs. Packs can add more (abilities, Lua) and remap names in `lexicon.yaml`.

## Movement

```
n s e w u d          north south east west up down
ne nw se sw
go <dir>
exits
```

## Looking

```
look / l             the room
look <thing>         examine something nearby or carried
examine / exa / ex
inventory / i / inv
score / stats / sc
equipment / eq
```

## Things

```
get / take <item>
get <item> from <container>
drop <item>
put <item> in <container>
give <item> to <someone>
wear <item>
remove <item>
use <item>
light <item>         same as use
```

## Doors

```
open [dir|door]
close [dir|door]
unlock [dir]
lock [dir]
```

You need the matching key in inventory to lock or unlock.

## People

```
say <text>           or  'text
emote <text>         or  :text
ask <who> about <topic>
```

## Fighting

```
kill / attack / k <who>
flee
```

Pack abilities register as extra verbs (for example `steady` in The Old House).

## Meta

```
help [topic]
save [slot]
quit
```

## Building (build mode only)

See `help building`. Shortcuts: `dig`, `buildwalk`, `desc`, `name`, `rflags`, `proto`, `spawn`, `goto`, `rooms`, `save pack`.
