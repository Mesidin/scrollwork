package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sudengine/internal/combat"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/script"
	"sudengine/internal/weather"
	"sudengine/internal/world"
)

func newTestServerWithEnv(timeLevel, weatherLevel int, initialWeather string, startHour int) (*Server, *Session) {
	w := world.New()
	p := &pack.Pack{
		Meta: pack.Meta{
			ID:          "test-pack",
			Title:       "Test World",
			TickMS:      50,
			StartRoom:   "room.square",
			CombatEvery: 4,
		},
		Lexicon: pack.Lexicon{Labels: map[string]string{}},
		Time: weather.TimeConfig{
			Level:         timeLevel,
			MinutesPerDay: 24,
			StartHour:     startHour,
		},
		Weather: weather.WeatherConfig{
			Level:          weatherLevel,
			Initial:        initialWeather,
			ChangeInterval: 100,
		},
	}

	square := &world.Entity{
		ID:        "room.square",
		Kind:      world.KindRoom,
		Name:      "Town Square",
		Long:      "A wide cobblestone town square open to the skies.",
		Flags:     map[string]bool{"outdoors": true},
		Exits:     map[string]world.Exit{"in": {To: "room.tavern"}},
		HasCoords: true,
	}
	tavern := &world.Entity{
		ID:        "room.tavern",
		Kind:      world.KindRoom,
		Name:      "Warm Tavern",
		Long:      "A cozy room with a small window looking outside.",
		Flags:     map[string]bool{"windowed": true, "dark": true},
		Exits:     map[string]world.Exit{"out": {To: "room.square"}, "down": {To: "room.cellar"}},
		HasCoords: true,
	}
	cellar := &world.Entity{
		ID:        "room.cellar",
		Kind:      world.KindRoom,
		Name:      "Deep Cellar",
		Long:      "A cold underground cellar.",
		Flags:     map[string]bool{"underground": true, "dark": true},
		Exits:     map[string]world.Exit{"up": {To: "room.tavern"}},
		HasCoords: true,
	}
	player := &world.Entity{
		ID:     "player",
		Kind:   world.KindPlayer,
		Name:   "Tester",
		Parent: "room.square",
		Attrs:  map[string]int{"body": 10},
		Resources: map[string]world.Resource{
			"hp": {Current: 20, Max: 20},
		},
	}

	w.Entities[square.ID] = square
	w.Entities[tavern.ID] = tavern
	w.Entities[cellar.ID] = cellar
	w.Entities[player.ID] = player
	w.PlayerID = player.ID
	w.StartRoom = square.ID

	sess := &Session{
		ID:       "test-session",
		Mode:     protocol.ModePlay,
		PlayerID: player.ID,
		Out:      make(chan protocol.Event, 256),
		In:       make(chan string, 32),
	}

	h := script.New(map[string]string{})
	h.World = w
	srv := &Server{
		Pack:    p,
		World:   w,
		Scripts: h,
		Combat:  &combat.Engine{Primary: "hp", Every: 4},
		Session: sess,
		Env:     weather.New(p.Meta.TickMS, p.Time, p.Weather),
	}
	w.IsAmbientDark = func(w *world.World, room *world.Entity) bool {
		return srv.Env.IsAmbientDark(room.HasFlag)
	}
	w.IsAmbientLit = func(w *world.World, room *world.Entity) bool {
		return srv.Env.IsAmbientLit(room.HasFlag)
	}

	return srv, sess
}

func drainLines(sess *Session) []string {
	var lines []string
	for {
		select {
		case ev := <-sess.Out:
			if te, ok := ev.(protocol.TextEvent); ok {
				lines = append(lines, te.Text)
			}
		default:
			return lines
		}
	}
}

func TestTimeAndWeatherCommands(t *testing.T) {
	srv, sess := newTestServerWithEnv(1, 1, "clear", 8) // Level 1 (Cosmetic), 8:00 AM Clear
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	// 'time' command
	sess.In <- "time"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "8:00 AM") && strings.Contains(l, "Morning") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 8:00 AM Morning in time output, got: %v", lines)
	}

	// 'weather' command outdoors
	sess.In <- "weather"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	found = false
	for _, l := range lines {
		if strings.Contains(l, "clear skies") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected clear skies in weather output, got: %v", lines)
	}

	// Move into tavern (windowed room)
	sess.In <- "in"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	sess.In <- "look sky"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	found = false
	for _, l := range lines {
		if strings.Contains(l, "window") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected window sky view from tavern, got: %v", lines)
	}

	// Move down to cellar (underground, no window)
	sess.In <- "down"
	time.Sleep(30 * time.Millisecond)
	drainLines(sess)

	sess.In <- "sky"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	found = false
	for _, l := range lines {
		if strings.Contains(l, "cannot see the sky") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cannot see the sky in cellar, got: %v", lines)
	}
}

func TestNightDarknessLevel2(t *testing.T) {
	// Start at 23:00 (11:00 PM Night) with Level 2
	srv, sess := newTestServerWithEnv(2, 2, "clear", 23)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	// Since it's night and outdoors without a light source, looking should reveal pitch black
	sess.In <- "look"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "pitch black") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected outdoor room to be pitch black at night under Level 2, got: %v", lines)
	}
}

func TestDisabledTimeAndWeatherLevel0(t *testing.T) {
	// Level 0 (None)
	srv, sess := newTestServerWithEnv(0, 0, "", 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	drainLines(sess)

	sess.In <- "time"
	time.Sleep(30 * time.Millisecond)
	lines := drainLines(sess)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "Time has no meaning here") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'Time has no meaning here' for level 0, got: %v", lines)
	}

	sess.In <- "weather"
	time.Sleep(30 * time.Millisecond)
	lines = drainLines(sess)
	found = false
	for _, l := range lines {
		if strings.Contains(l, "unchanging") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'weather is calm and unchanging' for level 0, got: %v", lines)
	}
}

func TestStormBlowsOutLantern(t *testing.T) {
	srv, sess := newTestServerWithEnv(3, 3, "storm", 10)
	if err := srv.World.Move("player", "room.square"); err != nil {
		t.Fatal(err)
	}
	lamp := &world.Entity{
		ID: "lamp", Kind: world.KindItem, Name: "lantern", Keywords: []string{"lantern"},
		Use: &world.UseEffect{ToggleFlag: "light"},
	}
	srv.World.Add(lamp)
	if err := srv.World.Move(lamp.ID, "player"); err != nil {
		t.Fatal(err)
	}
	lamp.SetFlag("light", true)
	srv.douseOutdoorFlames()
	if lamp.HasFlag("light") {
		t.Fatal("storm should blow the lantern out")
	}
	if !strings.Contains(strings.Join(drainLines(sess), "\n"), "blows the flame out") {
		t.Fatal("expected the flame to be reported")
	}
	out := texts(srv.HandleLine("use lantern"))
	if lamp.HasFlag("light") || !strings.Contains(out, "snuffs") {
		t.Fatalf("lantern should not catch outdoors in a storm:\n%s", out)
	}
}

func TestGreenHollowCanFaceNight(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, _, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada", RoleID: "warden"})
	if err != nil {
		t.Fatal(err)
	}
	square := srv.World.Get("hollow.square")
	cottage := srv.World.Get("hollow.cottage")
	if square == nil || !square.Outdoor() || !square.HasFlag("lit") {
		t.Fatal("square should stay lit outdoors")
	}
	if cottage == nil || !cottage.Windowed() {
		t.Fatal("cottage should be windowed")
	}
	var lantern *world.Entity
	for _, e := range srv.World.Entities {
		if e.PrototypeID == "hollow.lantern" {
			lantern = e
			break
		}
	}
	if lantern == nil || lantern.Use == nil || lantern.Use.ToggleFlag != "light" {
		t.Fatalf("lantern should toggle light, got %#v", lantern)
	}
	if srv.Env == nil || srv.Env.CurrentTick != srv.World.Tick {
		t.Fatal("environment clock should match the world tick at launch")
	}
}
