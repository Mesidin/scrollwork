package server

import (
	"testing"

	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/quest"
	"sudengine/internal/script"
	"sudengine/internal/world"
)

func TestPlayerTeleportEntersAndEndsCombat(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom, Name: "Hall", Exits: map[string]world.Exit{}}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom, Name: "Yard", Exits: map[string]world.Exit{}}
	player := &world.Entity{
		ID: "player", Kind: world.KindPlayer, Name: "Ada",
		Combat: &world.Combat{Target: "wolf"},
	}
	wolf := &world.Entity{
		ID: "wolf", Kind: world.KindMobile, Name: "wolf",
		Combat: &world.Combat{Target: "player"},
	}
	w.Add(roomA)
	w.Add(roomB)
	w.Add(player)
	w.Add(wolf)
	w.PlayerID = player.ID
	_ = w.Move(player.ID, roomA.ID)
	_ = w.Move(wolf.ID, roomA.ID)

	q := quest.Quest{
		ID:    "see_yard",
		Title: "See the Yard",
		Objectives: []quest.Objective{{
			ID: "go", Kind: quest.KindVisit, Target: "roomB", Count: 1, Desc: "Visit the yard",
		}},
	}
	quest.Start(player, q)

	sess := &Session{Out: make(chan protocol.Event, 32), PlayerID: player.ID}
	srv := &Server{
		Pack:    &pack.Pack{Quests: []quest.Quest{q}, Meta: pack.Meta{Title: "T"}},
		World:   w,
		Scripts: script.New(nil),
		Session: sess,
	}
	if err := srv.moveEntity(player, roomB); err != nil {
		t.Fatal(err)
	}
	if player.Parent != roomB.ID {
		t.Fatalf("player parent %s", player.Parent)
	}
	if player.InCombat() || wolf.InCombat() {
		t.Fatalf("fight should end when the player is moved, player %q wolf %q", player.Combat.Target, wolf.Combat.Target)
	}
	if !quest.IsReady(player, w, q) {
		t.Fatal("visit objective should advance when teleport runs the enter path")
	}
	var sawRoom bool
	for {
		select {
		case ev := <-sess.Out:
			if text, ok := ev.(protocol.TextEvent); ok && text.Channel == protocol.ChanRoom && text.Text == "Yard" {
				sawRoom = true
			}
		default:
			goto done
		}
	}
done:
	if !sawRoom {
		t.Fatal("expected the destination room title")
	}

	before := len(sess.Out)
	if err := srv.moveEntity(wolf, roomA); err != nil {
		t.Fatal(err)
	}
	if wolf.Parent != roomA.ID {
		t.Fatalf("wolf parent %s", wolf.Parent)
	}
	if len(sess.Out) != before {
		t.Fatal("moving an NPC should stay quiet")
	}
}

func TestMoveEntityRefusesRooms(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom}
	mob := &world.Entity{ID: "mob", Kind: world.KindMobile}
	w.Add(roomA)
	w.Add(roomB)
	w.Add(mob)
	srv := &Server{World: w, Pack: &pack.Pack{}, Scripts: script.New(nil), Session: &Session{Out: make(chan protocol.Event, 4)}}
	if err := srv.moveEntity(roomA, mob); err == nil {
		t.Fatal("expected a room destination to be refused")
	}
	if err := srv.moveEntity(roomA, roomB); err == nil {
		t.Fatal("expected moving a room to be refused")
	}
	if roomA.Parent != "" {
		t.Fatalf("roomA parent %s", roomA.Parent)
	}
}

func TestRespawnKeepsWidenedLeash(t *testing.T) {
	w := world.New()
	clearing := &world.Entity{ID: "clearing", Kind: world.KindRoom, Name: "Clearing"}
	lane := &world.Entity{ID: "lane", Kind: world.KindRoom, Name: "Lane"}
	proto := &world.Entity{
		ID: "mob.wolf", Kind: world.KindMobile, Name: "wolf",
		RespawnEvery: 5, HomeRoom: clearing.ID,
		AI:        &world.AI{Profile: "wander", Rooms: []world.ID{clearing.ID}},
		Resources: map[string]world.Resource{"hp": {Current: 10, Max: 10}},
	}
	w.Add(clearing)
	w.Add(lane)
	w.Protos = map[world.ID]*world.Entity{proto.ID: proto}
	wolf, err := w.Spawn(proto.ID, clearing.ID)
	if err != nil {
		t.Fatal(err)
	}
	wolf.RespawnEvery = 5
	wolf.HomeRoom = clearing.ID
	_ = w.Move(wolf.ID, lane.ID)
	world.RememberHome(wolf, lane)

	hp := wolf.Resources["hp"]
	hp.Current = 0
	wolf.Resources["hp"] = hp

	player := &world.Entity{ID: "player", Kind: world.KindPlayer, Name: "Ada"}
	w.Add(player)
	w.PlayerID = player.ID
	_ = w.Move(player.ID, clearing.ID)

	srv := &Server{
		Pack:    &pack.Pack{Meta: pack.Meta{Title: "T"}},
		World:   w,
		Scripts: script.New(nil),
		Session: &Session{Out: make(chan protocol.Event, 8), PlayerID: player.ID},
	}
	srv.reapDead()
	if len(w.Respawns) != 1 {
		t.Fatalf("respawns: %+v", w.Respawns)
	}
	task := w.Respawns[0]
	if task.RoomID != lane.ID {
		t.Fatalf("respawn room %s", task.RoomID)
	}
	if !containsID(task.Rooms, lane.ID) || !containsID(task.Rooms, clearing.ID) {
		t.Fatalf("respawn leash %+v", task.Rooms)
	}
	w.Respawns[0].DueTick = w.Tick
	srv.processRespawns()

	var born *world.Entity
	for _, e := range w.Entities {
		if e.Kind == world.KindMobile && e.PrototypeID == proto.ID {
			born = e
			break
		}
	}
	if born == nil {
		t.Fatal("expected a respawned wolf")
	}
	if born.HomeRoom != lane.ID {
		t.Fatalf("home %s", born.HomeRoom)
	}
	if born.AI == nil || !born.AI.Allows(lane.ID) || !born.AI.Allows(clearing.ID) {
		t.Fatalf("leash %+v", born.AI)
	}
}

func containsID(ids []world.ID, want world.ID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
