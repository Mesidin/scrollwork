package weather

// WeatherLevel defines the scaled complexity of the weather simulation.
type WeatherLevel int

const (
	WeatherNone       WeatherLevel = 0 // Disabled (default)
	WeatherCosmetic   WeatherLevel = 1 // Ambient transitions & echoes + 'weather'/'sky' commands
	WeatherSensory    WeatherLevel = 2 // Level 1 + storm light dimming + perception (notice/search) modifiers
	WeatherMechanical WeatherLevel = 3 // Level 2 + outdoor hit penalty + flames blown out
)

// WeatherConfig configures the weather simulation.
type WeatherConfig struct {
	Level          int                    `json:"level" yaml:"level"`
	ChangeInterval int                    `json:"change_interval" yaml:"change_interval"` // ticks between transition checks (default 240)
	Initial        string                 `json:"initial" yaml:"initial"`                 // initial state (default "clear")
	States         map[string]StateConfig `json:"states" yaml:"states"`
}

// StateConfig defines attributes and sensory text for a weather state.
type StateConfig struct {
	Label         string         `json:"label" yaml:"label"`
	SkyDesc       string         `json:"sky_desc" yaml:"sky_desc"`
	SkyDescNight  string         `json:"sky_desc_night" yaml:"sky_desc_night"`
	LightMod      int            `json:"light_mod" yaml:"light_mod"`
	NoticeMod     int            `json:"notice_mod" yaml:"notice_mod"`
	CombatMod     int            `json:"combat_mod" yaml:"combat_mod"`
	Extinguish    bool           `json:"extinguish" yaml:"extinguish"`
	OutdoorEchoes []string       `json:"outdoor_echoes" yaml:"outdoor_echoes"`
	WindowEchoes  []string       `json:"window_echoes" yaml:"window_echoes"`
	IndoorEchoes  []string       `json:"indoor_echoes" yaml:"indoor_echoes"`
	ArriveOutdoor string         `json:"arrive_outdoor" yaml:"arrive_outdoor"`
	ArriveWindow  string         `json:"arrive_window" yaml:"arrive_window"`
	ArriveIndoor  string         `json:"arrive_indoor" yaml:"arrive_indoor"`
	Transitions   map[string]int `json:"transitions" yaml:"transitions"` // target state -> weight
}

const (
	FlameOutLine   = "The wind blows the flame out."
	FlameSnuffLine = "The wind snuffs the flame before it catches."
)

// DefaultStates provides rich default weather patterns for packs that don't supply custom tables.
var DefaultStates = map[string]StateConfig{
	"clear": {
		Label:        "clear skies",
		SkyDesc:      "The sky is wide, clear, and bright.",
		SkyDescNight: "The sky is clear and dark beneath the stars.",
		LightMod:     0,
		NoticeMod:    0,
		CombatMod:    0,
		OutdoorEchoes: []string{
			"A gentle breeze drifts across the open ground.",
			"The clear air feels pleasant and still.",
		},
		WindowEchoes: []string{
			"Pleasant daylight gleams against the window.",
		},
		Transitions: map[string]int{
			"overcast": 65,
			"fog":      35,
		},
	},
	"overcast": {
		Label:        "overcast",
		SkyDesc:      "A solid blanket of dull grey clouds covers the sky.",
		SkyDescNight: "The night sky is obscured by heavy, pitch-black clouds.",
		LightMod:     -1,
		NoticeMod:    0,
		CombatMod:    0,
		OutdoorEchoes: []string{
			"Grey clouds hang heavy and low above.",
			"The sky darkens slightly under the unbroken cloud cover.",
		},
		WindowEchoes: []string{
			"Muted grey daylight filters through the glass.",
		},
		Transitions: map[string]int{
			"clear": 35,
			"rain":  50,
			"fog":   15,
		},
	},
	"fog": {
		Label:        "dense fog",
		SkyDesc:      "A cold, thick fog rolls in, obscuring everything beyond a few paces.",
		SkyDescNight: "A cold, dense fog rolls through the nighttime shadows.",
		LightMod:     -1,
		NoticeMod:    -3,
		CombatMod:    -1,
		OutdoorEchoes: []string{
			"Tendrils of damp mist coil along the ground.",
			"The thick fog muffles nearby sounds and limits your sight.",
		},
		WindowEchoes: []string{
			"A wall of white fog presses against the windowpane.",
		},
		Transitions: map[string]int{
			"clear":    45,
			"overcast": 40,
			"rain":     15,
		},
	},
	"rain": {
		Label:        "steady rain",
		SkyDesc:      "Rain falls steadily from dark, leaden clouds.",
		SkyDescNight: "Cold rain falls through the darkness of the night.",
		LightMod:     -1,
		NoticeMod:    -2,
		CombatMod:    -1,
		OutdoorEchoes: []string{
			"Rain patters steadily into the puddles and damp earth.",
			"A cool, steady shower washes over the area.",
		},
		WindowEchoes: []string{
			"Raindrops streak diagonally down the windowpane.",
		},
		Transitions: map[string]int{
			"overcast": 45,
			"storm":    30,
			"clear":    25,
		},
	},
	"storm": {
		Label:        "a violent thunderstorm",
		SkyDesc:      "Fierce winds whip through driving rain as lightning cuts across the black sky.",
		SkyDescNight: "Jagged flashes of lightning briefly illuminate the stormy night sky.",
		LightMod:     -2,
		Extinguish:   true,
		NoticeMod:    -4,
		CombatMod:    -2,
		OutdoorEchoes: []string{
			"Lightning flashes violently across the churning sky!",
			"Thunder crashes violently overhead, vibrating through the ground.",
			"Driving sheets of rain whip sideways in the howling gale.",
		},
		WindowEchoes: []string{
			"A sudden flash of lightning illuminates the room through the window!",
			"The window shudders against the heavy wind and thunder.",
		},
		IndoorEchoes: []string{
			"A deep roll of thunder rumbles through the building.",
		},
		Transitions: map[string]int{
			"rain":     70,
			"overcast": 30,
		},
	},
	"snow": {
		Label:        "gentle snowfall",
		SkyDesc:      "Soft white snowflakes drift silently down from a pale sky.",
		SkyDescNight: "Pale flakes of snow drift down through the silent dark.",
		LightMod:     0,
		NoticeMod:    -1,
		CombatMod:    0,
		OutdoorEchoes: []string{
			"Flakes of snow drift quietly to the ground.",
			"The cold air turns crisp as snow settles on every surface.",
		},
		WindowEchoes: []string{
			"Snowflakes melt against the cold glass of the window.",
		},
		Transitions: map[string]int{
			"overcast": 60,
			"blizzard": 20,
			"clear":    20,
		},
	},
	"blizzard": {
		Label:        "a howling blizzard",
		SkyDesc:      "A blinding whiteout of freezing wind and stinging snow obliterates all visibility.",
		SkyDescNight: "A howling blizzard whips stinging snow through the freezing darkness.",
		LightMod:     -2,
		Extinguish:   true,
		NoticeMod:    -5,
		CombatMod:    -3,
		OutdoorEchoes: []string{
			"Freezing wind howls around you, whipping snow into a blinding whiteout!",
			"Stinging ice particles bite into your face as the gale roars.",
		},
		WindowEchoes: []string{
			"The window rattles violently under a crust of windblown frost and snow.",
		},
		IndoorEchoes: []string{
			"The bitter wind howls outside against the walls.",
		},
		Transitions: map[string]int{
			"snow":     70,
			"overcast": 30,
		},
	},
}

// WeatherTransitionMessages provides atmospheric descriptions when transitioning from one weather state to another.
func WeatherTransitionMessage(from, to string) (outdoor, window, indoor string) {
	switch {
	case to == "storm":
		return "Dark storm clouds gather and lightning splits the sky as a tempest breaks loose!",
			"Lightning flashes and heavy rain begins lashing violently against the window!",
			"A sudden crack of thunder echoes through the structure."
	case from == "storm" && to == "rain":
		return "The violent thunder and lightning pass, leaving a steady, soaking rain.",
			"The thunder subsides as rain continues to drum against the glass.",
			""
	case to == "rain":
		return "Rain begins to fall, first as a light spatter, then settling into a steady shower.",
			"Raindrops begin to patter against the windowpane.",
			""
	case from == "rain" && (to == "overcast" || to == "clear"):
		return "The rain slows to a gentle mist, then tapers off completely.",
			"The rain outside stops, leaving beads of water trickling down the window.",
			""
	case to == "fog":
		return "A cool, dense fog creeps across the ground, swallowing the horizon.",
			"Mist begins gathering outside, obscuring the view beyond the glass.",
			""
	case from == "fog":
		return "The thick fog begins to thin and disperse into the air.",
			"The fog outside begins to lift.",
			""
	case to == "blizzard":
		return "The wind shrieks as a brutal blizzard descends in a blinding whiteout!",
			"Howling winds drive thick snow against the rattling windowpane!",
			"Bitter wind groans against the outside of the building."
	case to == "snow":
		return "The air grows cold, and soft flakes of white snow begin to drift from above.",
			"Snow begins to fall quietly outside the window.",
			""
	case to == "clear":
		return "The clouds break and drift apart, revealing open skies.",
			"Bright light breaks through outside as the cloud cover clears.",
			""
	case to == "overcast":
		return "A dull layer of grey clouds rolls in, obscuring the sky.",
			"The light outside dulls as clouds cover the sky.",
			""
	default:
		return "The weather outside shifts.", "The weather outside shifts.", ""
	}
}

func cloneStates(in map[string]StateConfig) map[string]StateConfig {
	out := make(map[string]StateConfig, len(in))
	for k, st := range in {
		st.OutdoorEchoes = append([]string(nil), st.OutdoorEchoes...)
		st.WindowEchoes = append([]string(nil), st.WindowEchoes...)
		st.IndoorEchoes = append([]string(nil), st.IndoorEchoes...)
		if st.Transitions != nil {
			tr := make(map[string]int, len(st.Transitions))
			for tk, tv := range st.Transitions {
				tr[tk] = tv
			}
			st.Transitions = tr
		}
		out[k] = st
	}
	return out
}
