package server

import (
	"path/filepath"
	"testing"

	"sudengine/internal/protocol"
	"sudengine/internal/world"
)

func TestRatStaysInCellar(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, _, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Open the cellar door, then leave it open while the rat wanders.
	for _, line := range []string{"n", "n", "get key", "s", "unlock down", "open down"} {
		_ = srv.HandleLine(line)
	}
	for i := 0; i < 400; i++ {
		srv.tick()
	}
	rat := findMob(srv, "rat")
	if rat == nil {
		t.Fatal("missing rat")
	}
	room := srv.World.RoomOf(rat)
	if room == nil || (room.ID != "house.wine" && room.ID != "house.cellar") {
		id := ""
		if room != nil {
			id = string(room.ID)
		}
		t.Fatalf("rat left the cellar: %s", id)
	}
	player := srv.World.Player()
	if player != nil && player.InCombat() {
		t.Fatal("rat attacked upstairs")
	}
}

func TestThingWaitsForALook(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, _, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"n", "n", "get key", "s", "e", "get lamp", "use lamp", "w",
		"unlock down", "open down", "d", "w",
	} {
		_ = srv.HandleLine(line)
	}
	for i := 0; i < 8; i++ {
		srv.tick()
	}
	player := srv.World.Player()
	thing := findMob(srv, "thing")
	if thing == nil || player == nil {
		t.Fatal("missing thing or player")
	}
	if player.InCombat() || thing.InCombat() {
		t.Fatal("thing attacked on entering the cistern")
	}
	out := texts(srv.HandleLine("look thing"))
	if !player.InCombat() || !thing.InCombat() {
		t.Fatalf("look should wake the thing:\n%s", out)
	}
	// Step onto the cellar stairs. Pursuit should follow. The foyer is outside its rooms.
	_ = srv.World.Move(player.ID, "house.cellar")
	for i := 0; i < 8; i++ {
		srv.tick()
	}
	where := srv.World.RoomOf(thing)
	if where == nil || where.ID != "house.cellar" {
		id := ""
		if where != nil {
			id = string(where.ID)
		}
		t.Fatalf("thing did not follow onto the stairs: %s", id)
	}
	_ = srv.World.Move(player.ID, "house.foyer")
	for i := 0; i < 20; i++ {
		srv.tick()
	}
	where = srv.World.RoomOf(thing)
	if where == nil || (where.ID != "house.cellar" && where.ID != "house.cistern") {
		t.Fatalf("thing chased upstairs: %s", where.ID)
	}
}

func findMob(srv *Server, keyword string) *world.Entity {
	for _, e := range srv.World.Entities {
		if e.Kind == world.KindMobile && e.HasKeyword(keyword) {
			return e
		}
	}
	return nil
}
