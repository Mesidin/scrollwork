package weather

import (
	"math/rand"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Broadcast represents a sensory echo dispatched to players based on their room type.
type Broadcast struct {
	OutdoorText string
	WindowText  string
	IndoorText  string
}

// Environment manages in-game time and weather simulations according to their configured levels.
type Environment struct {
	TimeConfig    TimeConfig
	WeatherConfig WeatherConfig
	TickMS        int

	CurrentWeather  string
	LastWeatherTick int64
	LastPhase       TimePhase
	LastAmbientTick int64
	CurrentTick     int64
	clockReady      bool
}

const (
	ambientEvery  int64   = 80
	ambientChance float32 = 0.20
)

// New creates and initializes an Environment manager.
func New(tickMS int, timeCfg TimeConfig, weatherCfg WeatherConfig) *Environment {
	if tickMS <= 0 {
		tickMS = 250
	}
	if weatherCfg.Initial == "" {
		weatherCfg.Initial = "clear"
	}
	if len(weatherCfg.States) == 0 {
		weatherCfg.States = cloneStates(DefaultStates)
	}
	env := &Environment{
		TimeConfig:     timeCfg,
		WeatherConfig:  weatherCfg,
		TickMS:         tickMS,
		CurrentWeather: weatherCfg.Initial,
	}
	env.Sync(0)
	return env
}

// Sync sets the clock, sky, and echo timers to a world tick.
// Loading a save calls this so the first heartbeat does not announce a phase
// the world already passed, and so lighting matches that tick immediately.
func (e *Environment) Sync(tick int64) {
	if e == nil {
		return
	}
	if tick < 0 {
		tick = 0
	}
	e.CurrentTick = tick
	e.LastWeatherTick = tick
	e.LastAmbientTick = tick
	e.clockReady = true
	if e.TimeConfig.Level >= 1 {
		e.LastPhase = e.GetTime(tick).Phase
	}
}

// CurrentState returns the active weather StateConfig.
func (e *Environment) CurrentState() StateConfig {
	if st, ok := e.WeatherConfig.States[e.CurrentWeather]; ok {
		return st
	}
	if st, ok := DefaultStates[e.CurrentWeather]; ok {
		return st
	}
	return DefaultStates["clear"]
}

// GetTime computes TimeInfo from the given engine tick.
func (e *Environment) GetTime(tick int64) TimeInfo {
	minPerDay := e.TimeConfig.MinutesPerDay
	if minPerDay <= 0 {
		minPerDay = 24
	}
	ticksPerDay := int64(minPerDay*60*1000) / int64(e.TickMS)
	if ticksPerDay <= 0 {
		ticksPerDay = 5760
	}
	ticksPerHour := ticksPerDay / 24
	if ticksPerHour <= 0 {
		ticksPerHour = 240
	}
	ticksPerMin := ticksPerHour / 60
	if ticksPerMin <= 0 {
		ticksPerMin = 4
	}

	startHour := e.TimeConfig.StartHour
	if startHour < 0 || startHour > 23 {
		startHour = 8
	}
	offsetTicks := int64(startHour) * ticksPerHour
	totalTicks := tick + offsetTicks + e.TimeConfig.EpochTick
	if totalTicks < 0 {
		totalTicks = 0
	}

	day := int(totalTicks/ticksPerDay) + 1
	ticksIntoDay := totalTicks % ticksPerDay
	hour := int(ticksIntoDay / ticksPerHour)
	ticksIntoHour := ticksIntoDay % ticksPerHour
	minute := int(ticksIntoHour / ticksPerMin)

	phase, phaseStr := PhaseForHour(hour)
	return TimeInfo{
		Day:      day,
		Hour:     hour,
		Minute:   minute,
		Phase:    phase,
		PhaseStr: phaseStr,
	}
}

// Tick evaluates time and weather changes, returning any broadcasts to emit.
func (e *Environment) Tick(tick int64, rng *rand.Rand) []Broadcast {
	e.CurrentTick = tick
	var broadcasts []Broadcast

	if !e.clockReady {
		e.LastWeatherTick = tick
		e.LastAmbientTick = tick
		e.clockReady = true
	}

	// 1. Day / Night phase transitions (Level 1+)
	if e.TimeConfig.Level >= 1 {
		ti := e.GetTime(tick)
		if e.LastPhase == "" {
			e.LastPhase = ti.Phase
		} else if ti.Phase != e.LastPhase {
			out, win := e.phaseEcho(ti.Phase)
			if out != "" || win != "" {
				broadcasts = append(broadcasts, Broadcast{
					OutdoorText: out,
					WindowText:  win,
				})
			}
			e.LastPhase = ti.Phase
		}
	}

	// 2. Weather state transitions and ambient echoes (Level 1+)
	if e.WeatherConfig.Level >= 1 {
		interval := e.WeatherConfig.ChangeInterval
		if interval <= 0 {
			interval = 240
		}
		if tick-e.LastWeatherTick >= int64(interval) {
			st := e.CurrentState()
			next := pickNextState(st.Transitions, rng)
			if next != "" && next != e.CurrentWeather {
				out, win, in := e.arriveText(next)
				broadcasts = append(broadcasts, Broadcast{
					OutdoorText: out,
					WindowText:  win,
					IndoorText:  in,
				})
				e.CurrentWeather = next
			}
			e.LastWeatherTick = tick
		} else if rng != nil && tick-e.LastAmbientTick >= ambientEvery {
			e.LastAmbientTick = tick
			if rng.Float32() < ambientChance {
				st := e.CurrentState()
				var outEcho, winEcho, inEcho string
				if len(st.OutdoorEchoes) > 0 {
					outEcho = st.OutdoorEchoes[rng.Intn(len(st.OutdoorEchoes))]
				}
				if len(st.WindowEchoes) > 0 {
					winEcho = st.WindowEchoes[rng.Intn(len(st.WindowEchoes))]
				}
				if len(st.IndoorEchoes) > 0 {
					inEcho = st.IndoorEchoes[rng.Intn(len(st.IndoorEchoes))]
				}
				if outEcho != "" || winEcho != "" || inEcho != "" {
					broadcasts = append(broadcasts, Broadcast{
						OutdoorText: outEcho,
						WindowText:  winEcho,
						IndoorText:  inEcho,
					})
				}
			}
		}
	}

	return broadcasts
}

func pickNextState(transitions map[string]int, rng *rand.Rand) string {
	if len(transitions) == 0 || rng == nil {
		return ""
	}
	type pair struct {
		state  string
		weight int
	}
	var pairs []pair
	total := 0
	for s, w := range transitions {
		if w > 0 {
			pairs = append(pairs, pair{state: s, weight: w})
			total += w
		}
	}
	if total <= 0 {
		return ""
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].state < pairs[j].state })
	roll := rng.Intn(total)
	acc := 0
	for _, p := range pairs {
		acc += p.weight
		if roll < acc {
			return p.state
		}
	}
	return pairs[len(pairs)-1].state
}

// IsAmbientDark reports whether environmental conditions render the room dark.
func (e *Environment) IsAmbientDark(hasFlag func(string) bool) bool {
	if hasFlag("underground") {
		return false
	}
	if flagSet(hasFlag, "outdoors", "outdoor") {
		if hasFlag("lit") {
			return false
		}
		if e.TimeConfig.Level >= 2 {
			ti := e.GetTime(e.CurrentTick)
			if ti.Phase.IsNight() {
				return true
			}
		}
		if e.WeatherConfig.Level >= 2 {
			st := e.CurrentState()
			if st.LightMod <= -2 {
				return true
			}
		}
	}
	return false
}

// IsAmbientLit reports whether daylight illuminates a dark indoor room via a window.
func (e *Environment) IsAmbientLit(hasFlag func(string) bool) bool {
	if flagSet(hasFlag, "windowed", "sheltered") && e.TimeConfig.Level >= 2 {
		ti := e.GetTime(e.CurrentTick)
		if ti.Phase.IsDay() {
			st := e.CurrentState()
			if st.LightMod > -2 {
				return true
			}
		}
	}
	return false
}

// NoticeModifier returns the outdoor perception modifier.
// Negative numbers are harder. Weather applies from scale 2.
// Time scale 3 adds a night penalty (default -2, or night_notice).
func (e *Environment) NoticeModifier(isOutdoor bool) int {
	if e == nil || !isOutdoor {
		return 0
	}
	n := 0
	if e.WeatherConfig.Level >= 2 {
		n += e.CurrentState().NoticeMod
	}
	if e.TimeConfig.Level >= 3 {
		ti := e.GetTime(e.CurrentTick)
		if ti.Phase.IsNight() {
			pen := e.TimeConfig.NightNotice
			if pen == 0 {
				pen = -2
			}
			n += pen
		}
	}
	return n
}

// CombatModifier returns the outdoor hit-check modifier. Negative is harder.
// It applies at weather scale 3.
func (e *Environment) CombatModifier(isOutdoor bool) int {
	if !isOutdoor || e.WeatherConfig.Level < 3 {
		return 0
	}
	return e.CurrentState().CombatMod
}

// Extinguishes reports whether outdoor toggle-lights should go out.
// Weather scale 3 and a state with extinguish set.
func (e *Environment) Extinguishes(isOutdoor bool) bool {
	if e == nil || !isOutdoor || e.WeatherConfig.Level < 3 {
		return false
	}
	return e.CurrentState().Extinguish
}

func (e *Environment) phaseEcho(p TimePhase) (outdoor, window string) {
	if e.TimeConfig.Phases != nil {
		if pe, ok := e.TimeConfig.Phases[string(p)]; ok {
			return pe.Outdoor, pe.Window
		}
	}
	if echo, ok := PhaseTransitionEchoes[p]; ok {
		return echo.Outdoor, echo.Window
	}
	return "", ""
}

func (e *Environment) arriveText(next string) (outdoor, window, indoor string) {
	st, ok := e.WeatherConfig.States[next]
	if !ok {
		st = DefaultStates[next]
	}
	if st.ArriveOutdoor != "" || st.ArriveWindow != "" || st.ArriveIndoor != "" {
		return st.ArriveOutdoor, st.ArriveWindow, st.ArriveIndoor
	}
	return WeatherTransitionMessage(e.CurrentWeather, next)
}

// DescribeSky produces the sky sentence from the active state's sky text.
func (e *Environment) DescribeSky(isOutdoor, isWindowed bool) (string, bool) {
	if !isOutdoor && !isWindowed {
		return "You cannot see the sky from here.", false
	}
	if e.TimeConfig.Level == 0 && e.WeatherConfig.Level == 0 {
		return "The sky is calm and unchanging.", true
	}
	text := e.skyText()
	if isWindowed && !isOutdoor {
		return "Through the window, " + lowerFirst(text), true
	}
	return text, true
}

func (e *Environment) skyText() string {
	st := e.CurrentState()
	text := strings.TrimSpace(st.SkyDesc)
	if e.TimeConfig.Level >= 1 {
		ti := e.GetTime(e.CurrentTick)
		if ti.Phase.IsNight() && strings.TrimSpace(st.SkyDescNight) != "" {
			text = strings.TrimSpace(st.SkyDescNight)
		}
	}
	return capitalize(text)
}

func capitalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "The sky is calm."
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size <= 1 && !utf8.ValidString(s) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

func lowerFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToLower(r)) + s[size:]
}

func flagSet(hasFlag func(string) bool, names ...string) bool {
	if hasFlag == nil {
		return false
	}
	for _, n := range names {
		if hasFlag(n) {
			return true
		}
	}
	return false
}
