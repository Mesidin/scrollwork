# Weather and the clock

One weather state covers the whole pack. Packs opt in with `environment.yaml`. `weather.yaml` is read only when `environment.yaml` is missing or leaves both levels at 0. Old House omits both files, so `time` and `weather` stay quiet and night does not darken rooms.

## Scales

Each of `time.level` and `weather.level` is 0–3.

| Level | Time | Weather |
| --- | --- | --- |
| 0 | Off | Off |
| 1 | Clock, phase lines, `time` | State changes, echoes, `weather` / `sky` / `look sky` |
| 2 | Outdoor night is dark; daylight fills a dark window | `light_mod` can darken outdoors; `notice_mod` shifts outdoor notice and search |
| 3 | Outdoor notice and search take `night_notice` at night (default -2) | Outdoor `hit` checks take `combat_mod`; `extinguish` blows out toggle-lights |

`combat_mod` and `notice_mod` are negative when the roll should be harder. The check runner raises an over target and lowers an under ceiling, so the sign means the same thing in both modes. A pack with no `hit` check still lands every blow. A pack with no `notice` check still reveals every hidden thing.

Ambient echoes roll once every 80 ticks, with a 20% chance on that tick. `change_interval` (default 240 ticks) is how often the weather may change.

## Pack file

```yaml
time:
  level: 1
  minutes_per_day: 24
  start_hour: 8
  night_notice: -2
  phases:
    night:
      outdoor: Night falls, and the open dark closes in.
      window: The window goes dark.

weather:
  level: 1
  initial: clear
  change_interval: 240
  states:
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
      indoor_echoes: []
      arrive_outdoor: A cold mist rolls in.
      arrive_window: Mist gathers outside the window.
      arrive_indoor: ""
      transitions:
        clear: 1
```

Leaving `states` out keeps the built-in table: clear, overcast, fog, rain, storm, snow, blizzard. Supplying `states` replaces that table. Storm and blizzard set `extinguish`. Empty `arrive_*` text falls back to the built-in sentence for those names. `phases` replaces the built-in line for that phase only. Phase names are `midnight`, `dawn`, `morning`, `midday`, `afternoon`, `dusk`, and `night`.

`sky_desc` is the `weather` / `look sky` sentence. `sky_desc_night` replaces it at night when the clock is on. Outdoor lines are capitalized. A window is prefixed with "Through the window, ".

## Rooms

* `outdoor` or `outdoors` — open-air echoes, night darkness, notice and hit modifiers, flames.
* `windowed` or `sheltered` — quieter echoes. At time scale 2, daylight lights a `dark` room unless `light_mod` is -2 or lower.
* `underground` — no sky, no daylight, no weather echoes.
* `lit` — an outdoor room stays bright at night (the square lamps stay on).

A lamp the player can relight uses `use.toggle_flag: light`. Scale 3 weather with `extinguish: true` clears that flag in outdoor rooms and refuses to light it again until the weather eases. A permanent `flags: [light]` entry is left alone. `lit` on the room is left alone.

## Saves

The world snapshot stores `weather_state` and the world tick. Launch and load copy the tick into the environment before the first look, so sky text and scale-2 darkness match the saved hour, and the first heartbeat does not announce a phase that already happened. Echo timers restart from that tick.

## Code

* `internal/weather` — clock, states, modifiers, sky text.
* `internal/server` — tick, alerts, save restore, outdoor hit penalty, dousing flames.
* `internal/commands` — `time`, `weather`, `sky`, notice penalty, relight refusal.
* `internal/combat` — `HitMod` on the hit check.
* `internal/world` — `Outdoor`, `Windowed`, `Underground`, `ExtinguishToggleLights`.
* Help: `internal/docs/engine/systems.md` and `building.md`.

Green Hollow is scale 1. The square is `outdoor` and `lit`, the cottage is `windowed`, and `hollow.lantern` is on the lane, so raising the levels does not black out the whole map.
