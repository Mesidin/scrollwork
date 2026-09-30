package weather

import (
	"math/rand"
	"strings"
	"testing"
)

func TestTimeCalculation(t *testing.T) {
	// 250ms tick = 4 ticks/sec. 24 minutes/day = 5760 ticks/day.
	// 1 hour = 240 ticks. 1 minute = 4 ticks.
	cfg := TimeConfig{
		Level:         1,
		MinutesPerDay: 24,
		StartHour:     8, // Starts at 8:00 AM
	}
	env := New(250, cfg, WeatherConfig{})

	// Tick 0: 8:00 AM Day 1
	ti := env.GetTime(0)
	if ti.Hour != 8 || ti.Minute != 0 || ti.Day != 1 || ti.Phase != PhaseMorning {
		t.Fatalf("expected 8:00 AM Day 1 Morning, got: %+v", ti)
	}

	// 4 ticks = 1 minute -> 8:01 AM
	ti = env.GetTime(4)
	if ti.Hour != 8 || ti.Minute != 1 || ti.Day != 1 {
		t.Fatalf("expected 8:01 AM, got: %+v", ti)
	}

	// 240 ticks = 1 hour -> 9:00 AM
	ti = env.GetTime(240)
	if ti.Hour != 9 || ti.Minute != 0 || ti.Day != 1 {
		t.Fatalf("expected 9:00 AM, got: %+v", ti)
	}

	// 12 hours later (2880 ticks) -> 20:00 (8:00 PM, Night)
	ti = env.GetTime(2880)
	if ti.Hour != 20 || ti.Phase != PhaseNight {
		t.Fatalf("expected 8:00 PM Night, got: %+v", ti)
	}

	// 24 hours later (5760 ticks) -> Day 2, 8:00 AM
	ti = env.GetTime(5760)
	if ti.Day != 2 || ti.Hour != 8 || ti.Minute != 0 {
		t.Fatalf("expected Day 2 8:00 AM, got: %+v", ti)
	}
}

func TestPhaseTransitions(t *testing.T) {
	cfg := TimeConfig{
		Level:         1,
		MinutesPerDay: 24,
		StartHour:     5, // Dawn
	}
	env := New(250, cfg, WeatherConfig{})
	rng := rand.New(rand.NewSource(1))

	// Initial phase is dawn. Tick forward until morning (hour 6 = 240 ticks)
	broadcasts := env.Tick(240, rng)
	if len(broadcasts) == 0 {
		t.Fatalf("expected phase transition broadcast when reaching morning")
	}
	if broadcasts[0].OutdoorText == "" {
		t.Fatalf("expected outdoor broadcast text for morning")
	}
}

func TestAmbientDarknessByLevel(t *testing.T) {
	outdoorFlags := map[string]bool{"outdoors": true}
	hasOutdoor := func(f string) bool { return outdoorFlags[f] }

	// Level 0: Night should NOT make outdoor dark
	env0 := New(250, TimeConfig{Level: 0, StartHour: 23}, WeatherConfig{Level: 0})
	env0.CurrentTick = 0
	if env0.IsAmbientDark(hasOutdoor) {
		t.Fatalf("level 0 time should never set ambient dark")
	}

	// Level 1: Night should still NOT affect lighting (cosmetic only)
	env1 := New(250, TimeConfig{Level: 1, StartHour: 23}, WeatherConfig{Level: 1})
	env1.CurrentTick = 0
	if env1.IsAmbientDark(hasOutdoor) {
		t.Fatalf("level 1 time should not affect lighting")
	}

	// Level 2: Night SHOULD make outdoor dark
	env2 := New(250, TimeConfig{Level: 2, StartHour: 23}, WeatherConfig{Level: 2})
	env2.CurrentTick = 0
	if !env2.IsAmbientDark(hasOutdoor) {
		t.Fatalf("level 2 time at hour 23 should make outdoor dark")
	}

	// Level 2: Daytime should be lit
	env2Day := New(250, TimeConfig{Level: 2, StartHour: 10}, WeatherConfig{Level: 2})
	env2Day.CurrentTick = 0
	if env2Day.IsAmbientDark(hasOutdoor) {
		t.Fatalf("level 2 time at hour 10 should be naturally lit")
	}

	// Level 2: Storm should make daytime dark
	env2Storm := New(250, TimeConfig{Level: 2, StartHour: 10}, WeatherConfig{Level: 2, Initial: "storm"})
	env2Storm.CurrentTick = 0
	if !env2Storm.IsAmbientDark(hasOutdoor) {
		t.Fatalf("level 2 storm should darken daylight")
	}

	// Level 2: Lit flag prevents darkness even at night
	litOutdoorFlags := map[string]bool{"outdoors": true, "lit": true}
	if env2.IsAmbientDark(func(f string) bool { return litOutdoorFlags[f] }) {
		t.Fatalf("room with 'lit' flag should never be ambiently dark")
	}
}

func TestWindowIllumination(t *testing.T) {
	windowFlags := map[string]bool{"windowed": true, "dark": true}
	hasWindow := func(f string) bool { return windowFlags[f] }

	// Daytime: Window admits daylight (Level 2)
	env := New(250, TimeConfig{Level: 2, StartHour: 12}, WeatherConfig{Level: 2, Initial: "clear"})
	env.CurrentTick = 0
	if !env.IsAmbientLit(hasWindow) {
		t.Fatalf("windowed room should be ambiently lit by midday sun")
	}

	// Night: Window does not admit daylight
	envNight := New(250, TimeConfig{Level: 2, StartHour: 1}, WeatherConfig{Level: 2, Initial: "clear"})
	envNight.CurrentTick = 0
	if envNight.IsAmbientLit(hasWindow) {
		t.Fatalf("windowed room should not be lit at night")
	}
}

func TestModifiersByLevel(t *testing.T) {
	// Level 1: No modifiers
	env1 := New(250, TimeConfig{Level: 1}, WeatherConfig{Level: 1, Initial: "storm"})
	if env1.NoticeModifier(true) != 0 {
		t.Fatalf("level 1 weather should have 0 notice mod")
	}
	if env1.CombatModifier(true) != 0 {
		t.Fatalf("level 1 weather should have 0 combat mod")
	}

	// Level 2: Notice modifier, but 0 combat mod
	env2 := New(250, TimeConfig{Level: 2}, WeatherConfig{Level: 2, Initial: "storm"})
	if env2.NoticeModifier(true) >= 0 {
		t.Fatalf("level 2 storm should penalize notice checks")
	}
	if env2.CombatModifier(true) != 0 {
		t.Fatalf("level 2 storm should not penalize combat")
	}

	// Level 3: Both notice and combat modifiers
	env3 := New(250, TimeConfig{Level: 3}, WeatherConfig{Level: 3, Initial: "storm"})
	if env3.CombatModifier(true) >= 0 {
		t.Fatalf("level 3 storm should penalize ranged combat")
	}
	// Indoor check: modifiers should not apply indoors
	if env3.NoticeModifier(false) != 0 || env3.CombatModifier(false) != 0 {
		t.Fatalf("indoor checks should not have weather modifiers")
	}
}

func TestDescribeSky(t *testing.T) {
	env := New(250, TimeConfig{Level: 1, StartHour: 10}, WeatherConfig{Level: 1, Initial: "clear"})
	env.CurrentTick = 0

	// Outdoors
	desc, ok := env.DescribeSky(true, false)
	if !ok || desc == "" {
		t.Fatalf("expected sky description for outdoor room")
	}

	// Windowed
	desc, ok = env.DescribeSky(false, true)
	if !ok || desc == "" {
		t.Fatalf("expected sky description for windowed room")
	}

	// Indoors without windows
	desc, ok = env.DescribeSky(false, false)
	if ok {
		t.Fatalf("indoors room should return false for DescribeSky")
	}
	outdoor, _ := env.DescribeSky(true, false)
	if outdoor != "The sky is wide, clear, and bright." {
		t.Fatalf("daytime clear sky: %q", outdoor)
	}
	if !strings.Contains(desc, "cannot see the sky") {
		t.Fatalf("indoor sky: %q", desc)
	}
}

func TestSyncKeepsPhaseQuiet(t *testing.T) {
	env := New(250, TimeConfig{Level: 1, MinutesPerDay: 24, StartHour: 8}, WeatherConfig{Level: 1, Initial: "clear"})
	env.Sync(2880) // 8:00 plus 12 hours = 20:00 night
	if env.GetTime(env.CurrentTick).Phase != PhaseNight {
		t.Fatalf("synced tick should be night, got %+v", env.GetTime(env.CurrentTick))
	}
	got, _ := env.DescribeSky(true, false)
	if got != "The sky is clear and dark beneath the stars." {
		t.Fatalf("night sky: %q", got)
	}
	broadcasts := env.Tick(2881, rand.New(rand.NewSource(1)))
	if len(broadcasts) != 0 {
		t.Fatalf("loading a night tick should not announce a new phase: %+v", broadcasts)
	}
}

func TestAmbientRollsOncePerInterval(t *testing.T) {
	env := New(250, TimeConfig{Level: 1, MinutesPerDay: 24, StartHour: 8}, WeatherConfig{Level: 1, Initial: "clear", ChangeInterval: 1000})
	rng := rand.New(rand.NewSource(1))
	for tick := int64(1); tick <= 80; tick++ {
		env.Tick(tick, rng)
	}
	for tick := int64(81); tick <= 159; tick++ {
		if n := len(env.Tick(tick, rng)); n != 0 {
			t.Fatalf("tick %d emitted %d echoes inside one ambient interval", tick, n)
		}
	}
}

func TestPackSkyAndArriveText(t *testing.T) {
	env := New(250, TimeConfig{
		Level:     1,
		StartHour: 10,
		Phases: map[string]PhaseEcho{
			"morning": {Outdoor: "Pack dawn line.", Window: "Pack window dawn."},
		},
	}, WeatherConfig{
		Level:          1,
		Initial:        "mist",
		ChangeInterval: 10,
		States: map[string]StateConfig{
			"mist": {
				Label:        "mist",
				SkyDesc:      "pack sky line.",
				SkyDescNight: "pack night line.",
				Transitions:  map[string]int{"clear": 1},
			},
			"clear": {
				Label:         "clear",
				SkyDesc:       "Open sky.",
				ArriveOutdoor: "Pack mist arrives.",
			},
		},
	})
	desc, ok := env.DescribeSky(true, false)
	if !ok || desc != "Pack sky line." {
		t.Fatalf("pack sky: %q", desc)
	}
	win, ok := env.DescribeSky(false, true)
	if !ok || win != "Through the window, pack sky line." {
		t.Fatalf("window sky: %q", win)
	}
	env.Sync(0)
	broadcasts := env.Tick(10, rand.New(rand.NewSource(1)))
	found := false
	for _, b := range broadcasts {
		if b.OutdoorText == "Pack mist arrives." {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pack arrival text, got %+v", broadcasts)
	}
}

func TestPhaseOverride(t *testing.T) {
	env := New(250, TimeConfig{
		Level:         1,
		MinutesPerDay: 24,
		StartHour:     5,
		Phases:        map[string]PhaseEcho{"morning": {Outdoor: "A pack morning.", Window: "Window morning."}},
	}, WeatherConfig{Level: 0})
	broadcasts := env.Tick(240, rand.New(rand.NewSource(1)))
	if len(broadcasts) == 0 || broadcasts[0].OutdoorText != "A pack morning." {
		t.Fatalf("phase override: %+v", broadcasts)
	}
}

func TestNightNoticeAndExtinguish(t *testing.T) {
	night := New(250, TimeConfig{Level: 3, StartHour: 23}, WeatherConfig{Level: 3, Initial: "clear"})
	if night.NoticeModifier(true) != -2 {
		t.Fatalf("default night notice %d", night.NoticeModifier(true))
	}
	if night.NoticeModifier(false) != 0 {
		t.Fatal("indoor night notice should be 0")
	}
	day := New(250, TimeConfig{Level: 3, StartHour: 10, NightNotice: -5}, WeatherConfig{Level: 3, Initial: "storm"})
	if day.NoticeModifier(true) != -4 {
		t.Fatalf("day storm notice should stay weather-only, got %d", day.NoticeModifier(true))
	}
	if !day.Extinguishes(true) || day.Extinguishes(false) {
		t.Fatal("storm scale 3 should extinguish outdoors only")
	}
	mild := New(250, TimeConfig{Level: 3, StartHour: 10}, WeatherConfig{Level: 2, Initial: "storm"})
	if mild.Extinguishes(true) {
		t.Fatal("scale 2 should not extinguish")
	}
	clearDay := New(250, TimeConfig{Level: 2, StartHour: 12}, WeatherConfig{Level: 2, Initial: "clear"})
	shelter := map[string]bool{"sheltered": true, "dark": true}
	if !clearDay.IsAmbientLit(func(f string) bool { return shelter[f] }) {
		t.Fatal("sheltered room should take daylight like a window")
	}
}
