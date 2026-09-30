package weather

import "fmt"

// TimeLevel defines the scaled complexity of the day/night cycle.
type TimeLevel int

const (
	TimeNone       TimeLevel = 0 // Disabled (default)
	TimeCosmetic   TimeLevel = 1 // Ambient clock + phase transition echoes + 'time' command
	TimeLighting   TimeLevel = 2 // Level 1 + outdoor rooms become dark at night; windows admit daylight
	TimeMechanical TimeLevel = 3 // Level 2 + a night penalty on outdoor notice and search
)

// TimeConfig configures the in-game clock and day/night cycle.
type TimeConfig struct {
	Level         int   `json:"level" yaml:"level"`                     // 0 = none, 1 = cosmetic, 2 = lighting, 3 = mechanical
	MinutesPerDay int   `json:"minutes_per_day" yaml:"minutes_per_day"` // Real minutes per MUD day (default 24)
	StartHour     int   `json:"start_hour" yaml:"start_hour"`           // 0..23 (default 8 = 8:00 AM)
	EpochTick     int64 `json:"epoch_tick" yaml:"epoch_tick"`           // Tick offset
	// NightNotice is the outdoor notice modifier at night when level is 3.
	// Zero uses the default of -2.
	NightNotice int                  `json:"night_notice" yaml:"night_notice"`
	Phases      map[string]PhaseEcho `json:"phases" yaml:"phases"`
}

// PhaseEcho replaces the built-in line for one phase. The key is the phase name
// (dawn, morning, midday, afternoon, dusk, night, midnight).
type PhaseEcho struct {
	Outdoor string `json:"outdoor" yaml:"outdoor"`
	Window  string `json:"window" yaml:"window"`
}

// TimePhase represents a distinct portion of the day.
type TimePhase string

const (
	PhaseMidnight  TimePhase = "midnight"  // 00:00 - 03:59
	PhaseDawn      TimePhase = "dawn"      // 04:00 - 05:59
	PhaseMorning   TimePhase = "morning"   // 06:00 - 11:59
	PhaseMidday    TimePhase = "midday"    // 12:00 - 13:59
	PhaseAfternoon TimePhase = "afternoon" // 14:00 - 17:59
	PhaseDusk      TimePhase = "dusk"      // 18:00 - 19:59
	PhaseNight     TimePhase = "night"     // 20:00 - 23:59
)

// TimeInfo holds human-readable clock values.
type TimeInfo struct {
	Day      int
	Hour     int
	Minute   int
	Phase    TimePhase
	PhaseStr string
}

// IsDay reports whether this phase has natural daylight.
func (p TimePhase) IsDay() bool {
	switch p {
	case PhaseDawn, PhaseMorning, PhaseMidday, PhaseAfternoon, PhaseDusk:
		return true
	default:
		return false
	}
}

// IsNight reports whether this phase is dark outside.
func (p TimePhase) IsNight() bool {
	return !p.IsDay()
}

// FormatTime returns a 12-hour formatted time string, e.g. "8:15 AM (Morning), Day 1".
func (ti TimeInfo) String() string {
	h12 := ti.Hour % 12
	if h12 == 0 {
		h12 = 12
	}
	ampm := "AM"
	if ti.Hour >= 12 {
		ampm = "PM"
	}
	return fmt.Sprintf("%d:%02d %s (%s), Day %d", h12, ti.Minute, ampm, ti.PhaseStr, ti.Day)
}

// PhaseForHour returns the TimePhase for a 24-hour value.
func PhaseForHour(hour int) (TimePhase, string) {
	hour = hour % 24
	if hour < 0 {
		hour += 24
	}
	switch {
	case hour >= 4 && hour < 6:
		return PhaseDawn, "Dawn"
	case hour >= 6 && hour < 12:
		return PhaseMorning, "Morning"
	case hour >= 12 && hour < 14:
		return PhaseMidday, "Midday"
	case hour >= 14 && hour < 18:
		return PhaseAfternoon, "Afternoon"
	case hour >= 18 && hour < 20:
		return PhaseDusk, "Dusk"
	case hour >= 20:
		return PhaseNight, "Night"
	default:
		return PhaseMidnight, "Midnight"
	}
}

// PhaseTransitionEchoes provides default sensory descriptions when a new phase begins.
var PhaseTransitionEchoes = map[TimePhase]struct {
	Outdoor string
	Window  string
}{
	PhaseDawn: {
		Outdoor: "The first pale light of dawn touches the eastern horizon.",
		Window:  "Soft dawn light begins to seep through the window.",
	},
	PhaseMorning: {
		Outdoor: "The morning sun climbs steadily into the sky, warming the air.",
		Window:  "Morning sunlight streams in through the glass.",
	},
	PhaseMidday: {
		Outdoor: "The sun reaches its peak high in the midday sky.",
		Window:  "Bright midday glare reflects off the windowpane.",
	},
	PhaseAfternoon: {
		Outdoor: "The sun starts its slow afternoon slant toward the west.",
		Window:  "The sunlight slanting through the window turns warm and golden.",
	},
	PhaseDusk: {
		Outdoor: "The sun sinks below the horizon, and twilight gives way to dusk.",
		Window:  "Twilight fades outside the window as evening approaches.",
	},
	PhaseNight: {
		Outdoor: "Night falls, and the open dark closes in.",
		Window:  "Darkness gathers outside the window as night settles in.",
	},
	PhaseMidnight: {
		Outdoor: "The deep quiet of midnight settles across the landscape.",
		Window:  "The cold, quiet darkness of midnight watches outside.",
	},
}
