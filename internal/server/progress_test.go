package server

import (
	"path/filepath"
	"strings"
	"testing"

	"sudengine/internal/protocol"
	"sudengine/internal/world"
)

func TestFleeUsesEscapeCheck(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, _, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada", RoleID: "careful"})
	if err != nil {
		t.Fatal(err)
	}
	_ = srv.HandleLine("n")
	pl := srv.World.Player()
	maid := findMob(srv, "maid")
	if maid == nil {
		t.Fatal("maid")
	}
	if pl.Attrs["will"] != 2 {
		t.Fatalf("careful will %d", pl.Attrs["will"])
	}
	pl.Combat = &world.Combat{Target: maid.ID}
	ch := srv.Pack.RPG.Checks["escape"]
	ch.Difficulty = 99
	ch.Roll = "2d6"
	srv.Pack.RPG.Checks["escape"] = ch
	out := texts(srv.HandleLine("flee"))
	if !strings.Contains(strings.ToLower(out), "clear") {
		t.Fatalf("hard escape should fail:\n%s", out)
	}
	if srv.World.RoomOf(pl).ID != "house.foyer" || pl.Combat.Target == "" {
		t.Fatal("failed flee should leave you in the fight")
	}
	ch.Difficulty = 2
	srv.Pack.RPG.Checks["escape"] = ch
	out = texts(srv.HandleLine("flee"))
	if srv.World.RoomOf(pl).ID == "house.foyer" {
		t.Fatalf("easy escape should leave the foyer:\n%s", out)
	}
}

func TestGreenHollowSheet(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, _, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada", RoleID: "warden"})
	if err != nil {
		t.Fatal(err)
	}
	pl := srv.World.Player()
	if pl.Level != 1 || pl.Attrs["body"] < 2 || pl.Res("vigor").Max != 24 {
		t.Fatalf("warden level %d body %d vigor %d", pl.Level, pl.Attrs["body"], pl.Res("vigor").Max)
	}
	out := texts(srv.HandleLine("stats"))
	if !strings.Contains(out, "Level 1") || !strings.Contains(out, "Vigor") {
		t.Fatalf("stats:\n%s", out)
	}
	for _, skill := range []string{"Melee", "Footwork", "Notice"} {
		if !strings.Contains(out, skill) {
			t.Fatalf("stats should list %s:\n%s", skill, out)
		}
	}
	out = texts(srv.HandleLine("train"))
	if !strings.Contains(out, "Melee") || !strings.Contains(strings.ToLower(out), "coin") {
		t.Fatalf("train list:\n%s", out)
	}
	_ = srv.HandleLine("e")
	out = texts(srv.HandleLine("get ring"))
	if !strings.Contains(strings.ToLower(out), "don't see") && !strings.Contains(strings.ToLower(out), "you don't see") {
		// ring is in the thicket, not the lane
	}
	_ = srv.HandleLine("n")
	pl.Found = nil
	out = texts(srv.HandleLine("get ring"))
	if !strings.Contains(strings.ToLower(out), "don't see") {
		t.Fatalf("hidden ring should not be taken yet:\n%s", out)
	}
	pl.Attrs["mind"] = 5
	pl.Skills = map[string]int{"notice": 5}
	out = texts(srv.HandleLine("search"))
	if !strings.Contains(strings.ToLower(out), "ring") {
		t.Fatalf("search:\n%s", out)
	}
	out = texts(srv.HandleLine("get ring"))
	if !strings.Contains(strings.ToLower(out), "take") {
		t.Fatalf("get ring:\n%s", out)
	}
	out = texts(srv.HandleLine("inventory"))
	if !strings.Contains(strings.ToLower(out), "ring") {
		t.Fatalf("inventory:\n%s", out)
	}

	wolf := findMob(srv, "wolf")
	if wolf == nil || wolf.XP != 8 {
		t.Fatal("wolf xp")
	}
	wolf.SetRes("vigor", world.Resource{Current: 0, Max: 10})
	wolf.XP = 40
	srv.reapDead()
	if pl.Level < 2 || pl.SkillPoints < 1 {
		t.Fatalf("expected a level, got %d points %d xp %d", pl.Level, pl.SkillPoints, pl.XP)
	}
	out = texts(srv.HandleLine("improve melee"))
	if pl.Skills["melee"] != 1 {
		t.Fatalf("improve:\n%s skills %v", out, pl.Skills)
	}
	out = texts(srv.HandleLine("raise body"))
	if strings.Contains(strings.ToLower(out), "rises") {
		t.Fatal("raise should refuse without an attribute point")
	}
	_ = srv.HandleLine("s")
	_ = srv.HandleLine("w")
	out = texts(srv.HandleLine("train footwork"))
	if pl.Skills["footwork"] != 1 {
		t.Fatalf("train:\n%s", out)
	}
	if pl.Res("coin").Current >= 12 {
		t.Fatalf("training should spend coin, have %d", pl.Res("coin").Current)
	}
}
