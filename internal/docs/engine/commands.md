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

## Environment

```
time / clock            current time and period of day
weather / sky           weather conditions and sky view
look sky                observe the sky or through a window
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
talk <who>            list topics they might know
```

## Following & Parties

```
follow / fol [who]       follow someone or check who you follow and who follows you
unfollow / unfol         stop following your leader
lose / ditch / shake <who> shake off someone following you
group / party / gt       display active party members, health, and status
invite <who>             invite a companion or player to join your party
join <who>               join another party (if invited or open)
order / command <pet> <act> order a pet you own: follow, stay, stance, attack, dismiss
feed <pet> with <food>   feed a companion or tameable animal; builds loyalty and can tame
```

## Economy & Crafting

```
list / lshop         browse merchant inventory and till amount
buy <item|pet> [from <who>]
sell <item> [to <who>]
deposit <amount|all> store coin in a local bank or vault
withdraw <amount|all> retrieve coin from your vault
balance / bank / account check stored currency in your vault
repair               list damaged items and costs at a repairer
repair <item> / all  restore equipment you can afford; all names what is left
repair <item> with <kit> mend equipment in the field using a repair kit
recipes / rlist      list known recipes and station/material readiness
craft / make <recipe> craft an item at a required station
gather / forage [node] harvest natural resources; perception boosts yield
skin / butcher [corpse] harvest skin/pelts from fallen beasts
salvage <item>       break down an item into crafting components
```

## Quests

```
quests / tasks / journal     list active and completed quests with objective progress
quest <id|title>             show full quest details, objectives, and rewards
quest accept <id|title>      accept an offered quest from an NPC
quest complete <id|title>    turn in a completed quest to receive rewards
```

## Fighting

```
kill / attack / k <who>
flee
```

Pack abilities register as extra verbs (for example `steady` in The Old House).

## Meta

```
help
help <topic>
help <topic> <section>
search
stats
inventory
equipment
improve <skill>
raise <attribute>
train <skill>
save [slot]
quit
```

## Building (build mode only)

See `help building`. Shortcuts: `dig`, `buildwalk`, `desc`, `name`, `rflags`, `proto`, `spawn`, `goto`, `rooms`, `save pack`.
