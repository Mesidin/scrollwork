package server_test

import (
	"path/filepath"
	"strings"
	"testing"

	"sudengine/internal/protocol"
	"sudengine/internal/server"
	"sudengine/internal/world"
)

func drainEvents(sess *server.Session) []protocol.Event {
	var evs []protocol.Event
	for {
		select {
		case ev := <-sess.Out:
			evs = append(evs, ev)
		default:
			return evs
		}
	}
}

func filterTextEvents(evs []protocol.Event) []protocol.TextEvent {
	var out []protocol.TextEvent
	for _, ev := range evs {
		if t, ok := ev.(protocol.TextEvent); ok {
			out = append(out, t)
		}
	}
	return out
}

func filterRoomEvents(evs []protocol.Event) []protocol.RoomEvent {
	var out []protocol.RoomEvent
	for _, ev := range evs {
		if r, ok := ev.(protocol.RoomEvent); ok {
			out = append(out, r)
		}
	}
	return out
}

func TestStage1Multiplayer(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, sessAda, err := server.Launch(server.Options{
		PackDir:    dir,
		Mode:       protocol.ModePlay,
		Seed:       1,
		PlayerName: "Ada",
	})
	if err != nil {
		t.Fatalf("launch error: %v", err)
	}

	// 1. Spawn a second player session "Bob"
	sessBob, entBob := srv.SpawnPlayerSession("Bob", "", "")
	entAda := srv.World.Get(sessAda.PlayerID)
	if entAda == nil || entBob == nil {
		t.Fatalf("players not found in world: Ada=%v, Bob=%v", entAda, entBob)
	}

	// Drain initial greet events for both
	drainEvents(sessAda)
	drainEvents(sessBob)

	// Ensure both players start in the same room (Porch)
	roomAda := srv.World.RoomOf(entAda)
	roomBob := srv.World.RoomOf(entBob)
	if roomAda == nil || roomBob == nil || roomAda.ID != roomBob.ID {
		t.Fatalf("expected Ada and Bob in same room, got Ada in %v, Bob in %v", roomAda, roomBob)
	}

	// 2. Room occupant rendering: each player sees the other without seeing themselves
	adaRoomEvs := filterRoomEvents(srv.HandleSessionLine(sessAda, "look"))
	if len(adaRoomEvs) == 0 {
		t.Fatal("expected RoomEvent for Ada on look")
	}
	lastAdaRoom := adaRoomEvs[len(adaRoomEvs)-1]
	foundBob := false
	for _, occ := range lastAdaRoom.Occupants {
		if strings.Contains(occ, "Bob") {
			foundBob = true
		}
		if occ == "Ada" || occ == "you" {
			t.Fatalf("Ada should not see herself in occupants: %v", lastAdaRoom.Occupants)
		}
	}
	if !foundBob {
		t.Fatalf("Ada should see Bob in occupants: %v", lastAdaRoom.Occupants)
	}

	bobRoomEvs := filterRoomEvents(srv.HandleSessionLine(sessBob, "look"))
	if len(bobRoomEvs) == 0 {
		t.Fatal("expected RoomEvent for Bob on look")
	}
	lastBobRoom := bobRoomEvs[len(bobRoomEvs)-1]
	foundAda := false
	for _, occ := range lastBobRoom.Occupants {
		if strings.Contains(occ, "Ada") {
			foundAda = true
		}
		if occ == "Bob" {
			t.Fatalf("Bob should not see himself in occupants: %v", lastBobRoom.Occupants)
		}
	}
	if !foundAda {
		t.Fatalf("Bob should see Ada in occupants: %v", lastBobRoom.Occupants)
	}

	// 3. Say and Emote: speaker gets echo, other in room gets broadcast
	drainEvents(sessAda)
	drainEvents(sessBob)

	adaSayEvs := srv.HandleSessionLine(sessAda, "say hello friend")
	bobEvsAfterSay := drainEvents(sessBob)

	// Ada (speaker) should see "You say, \"hello friend\"" on ChanSay
	adaTexts := filterTextEvents(adaSayEvs)
	sawAdaSay := false
	for _, te := range adaTexts {
		if te.Channel == protocol.ChanSay && strings.Contains(te.Text, `You say, "hello friend"`) {
			sawAdaSay = true
		}
	}
	if !sawAdaSay {
		t.Fatalf("Ada did not receive correct say text: %+v", adaTexts)
	}

	// Bob should see "Ada says, \"hello friend\"" on ChanSay
	bobTexts := filterTextEvents(bobEvsAfterSay)
	sawBobHear := false
	for _, te := range bobTexts {
		if te.Channel == protocol.ChanSay && strings.Contains(te.Text, `Ada says, "hello friend"`) {
			sawBobHear = true
		}
	}
	if !sawBobHear {
		t.Fatalf("Bob did not hear Ada say: %+v", bobTexts)
	}

	// Emote: Ada emotes "smiles warmly." -> Both receive on ChanNarrative
	drainEvents(sessAda)
	drainEvents(sessBob)

	adaEmoteEvs := srv.HandleSessionLine(sessAda, "emote smiles warmly.")
	bobEvsAfterEmote := drainEvents(sessBob)

	sawAdaEmote := false
	for _, te := range filterTextEvents(adaEmoteEvs) {
		if te.Channel == protocol.ChanNarrative && strings.Contains(te.Text, "Ada smiles warmly.") {
			sawAdaEmote = true
		}
	}
	if !sawAdaEmote {
		t.Fatalf("Ada did not receive emote: %+v", adaEmoteEvs)
	}

	sawBobEmote := false
	for _, te := range filterTextEvents(bobEvsAfterEmote) {
		if te.Channel == protocol.ChanNarrative && strings.Contains(te.Text, "Ada smiles warmly.") {
			sawBobEmote = true
		}
	}
	if !sawBobEmote {
		t.Fatalf("Bob did not receive Ada's emote: %+v", bobEvsAfterEmote)
	}

	// 4. Movement: leaves <dir> in old room, arrives from <opposite> in dest room
	drainEvents(sessAda)
	drainEvents(sessBob)

	// Ada moves north into Foyer. Bob stays on Porch.
	_ = srv.HandleSessionLine(sessAda, "n")
	bobEvsAfterLeave := drainEvents(sessBob)

	sawAdaLeave := false
	for _, te := range filterTextEvents(bobEvsAfterLeave) {
		if te.Channel == protocol.ChanNarrative && strings.Contains(te.Text, "Ada leaves north.") {
			sawAdaLeave = true
		}
	}
	if !sawAdaLeave {
		t.Fatalf("Bob did not see Ada leave north: %+v", bobEvsAfterLeave)
	}

	// 5. Event isolation between different rooms
	// Ada is in Foyer; Bob is on Porch.
	drainEvents(sessAda)
	drainEvents(sessBob)

	srv.HandleSessionLine(sessAda, "say can you hear me through walls?")
	bobEvsIsolated := drainEvents(sessBob)
	for _, te := range filterTextEvents(bobEvsIsolated) {
		if strings.Contains(te.Text, "through walls") {
			t.Fatalf("Bob in another room should NOT hear Ada: %+v", te)
		}
	}

	// Ada moves east into Parlor. Bob on Porch should not hear leave east.
	srv.HandleSessionLine(sessAda, "e")
	bobEvsIsolatedMove := drainEvents(sessBob)
	for _, te := range filterTextEvents(bobEvsIsolatedMove) {
		if strings.Contains(te.Text, "leaves east") {
			t.Fatalf("Bob on Porch should NOT see Ada leave east in Foyer: %+v", te)
		}
	}

	// Move Ada back to Foyer, then south to Porch.
	_ = srv.HandleSessionLine(sessAda, "w")
	drainEvents(sessBob)

	// Ada arrives back on Porch from north. Bob should see arrive from north!
	_ = srv.HandleSessionLine(sessAda, "s")
	bobEvsAfterArrive := drainEvents(sessBob)

	sawAdaArrive := false
	for _, te := range filterTextEvents(bobEvsAfterArrive) {
		if te.Channel == protocol.ChanNarrative && strings.Contains(te.Text, "Ada arrives from north.") {
			sawAdaArrive = true
		}
	}
	if !sawAdaArrive {
		t.Fatalf("Bob did not see Ada arrive from north: %+v", bobEvsAfterArrive)
	}

	// 6. Dark room hides items and occupants
	darkRoom := &world.Entity{
		ID:    "dark.room",
		Kind:  world.KindRoom,
		Name:  "Dark Cellar",
		Long:  "A pitch black room.",
		Flags: map[string]bool{"dark": true},
	}
	itemInDark := &world.Entity{
		ID:    "dark.candle",
		Kind:  world.KindItem,
		Name:  "candle",
		Short: "a wax candle",
	}
	srv.World.Add(darkRoom)
	srv.World.Add(itemInDark)
	_ = srv.World.Move(itemInDark.ID, darkRoom.ID)
	_ = srv.World.Move(entAda.ID, darkRoom.ID)

	drainEvents(sessAda)
	darkEvs := filterRoomEvents(srv.HandleSessionLine(sessAda, "look"))
	if len(darkEvs) == 0 {
		t.Fatal("expected room event in dark room")
	}
	if !darkEvs[len(darkEvs)-1].Dark {
		t.Fatal("expected room event Dark to be true")
	}
	if len(darkEvs[len(darkEvs)-1].Items) > 0 {
		t.Fatalf("dark room must hide items, but got: %+v", darkEvs[len(darkEvs)-1].Items)
	}

	// 7. Occupant suffixes: (fighting) and (linkdead)
	// Move Ada and Bob back together in a lit room
	porch := srv.World.Get("house.porch")
	_ = srv.World.Move(entAda.ID, porch.ID)
	_ = srv.World.Move(entBob.ID, porch.ID)

	// Bob enters combat
	entBob.Combat = &world.Combat{Target: "some.target"}
	evsFighting := filterRoomEvents(srv.HandleSessionLine(sessAda, "look"))
	if len(evsFighting) == 0 {
		t.Fatal("expected room event for fighting check")
	}
	occFighting := strings.Join(evsFighting[len(evsFighting)-1].Occupants, ", ")
	if !strings.Contains(occFighting, "Bob (fighting)") {
		t.Fatalf("expected 'Bob (fighting)' in occupants, got: %s", occFighting)
	}

	// Bob is linkdead
	entBob.Combat.Target = ""
	entBob.SetFlag("linkdead", true)
	evsLinkdead := filterRoomEvents(srv.HandleSessionLine(sessAda, "look"))
	if len(evsLinkdead) == 0 {
		t.Fatal("expected room event for linkdead check")
	}
	occLinkdead := strings.Join(evsLinkdead[len(evsLinkdead)-1].Occupants, ", ")
	if !strings.Contains(occLinkdead, "Bob (linkdead)") {
		t.Fatalf("expected 'Bob (linkdead)' in occupants, got: %s", occLinkdead)
	}
}

func TestStage1QueueCoalescing(t *testing.T) {
	sess := &server.Session{
		ID:       "test-coalesce",
		Mode:     protocol.ModePlay,
		PlayerID: "player",
		Out:      make(chan protocol.Event, 4),
	}

	sess.Send(protocol.TextEvent{Channel: protocol.ChanNarrative, Text: "Line 1"})
	sess.Send(protocol.VitalsEvent{Round: 1})
	sess.Send(protocol.MapEvent{PlayerX: 1, PlayerY: 1})
	sess.Send(protocol.PromptEvent{Status: "HP:10/10"})

	if len(sess.Out) != 4 {
		t.Fatalf("expected queue length 4, got %d", len(sess.Out))
	}

	// The buffer is committed to the client. Later UI events coalesce in the
	// side queue, and narrative waits there instead of dropping a pane event.
	sess.Send(protocol.VitalsEvent{Round: 2})
	sess.Send(protocol.VitalsEvent{Round: 3})
	sess.Send(protocol.TextEvent{Channel: protocol.ChanNarrative, Text: "Line 2"})

	committed := drainEvents(sess)
	var committedText []string
	for _, ev := range committed {
		switch te := ev.(type) {
		case protocol.TextEvent:
			committedText = append(committedText, te.Text)
		case protocol.VitalsEvent:
			if te.Round != 1 {
				t.Fatalf("vitals already on Out stay at round 1, got %d", te.Round)
			}
		}
	}
	if len(committedText) != 1 || committedText[0] != "Line 1" {
		t.Fatalf("committed text = %v", committedText)
	}

	sess.Flush()
	rest := drainEvents(sess)
	var texts []string
	var rounds []int64
	for _, ev := range rest {
		switch te := ev.(type) {
		case protocol.TextEvent:
			texts = append(texts, te.Text)
		case protocol.VitalsEvent:
			rounds = append(rounds, te.Round)
		}
	}
	if len(texts) != 1 || texts[0] != "Line 2" {
		t.Fatalf("backlog text = %v", texts)
	}
	if len(rounds) != 1 || rounds[0] != 3 {
		t.Fatalf("coalesced vitals = %v", rounds)
	}

	full := &server.Session{
		ID:  "text-only",
		Out: make(chan protocol.Event, 2),
	}
	full.Send(protocol.TextEvent{Text: "a"})
	full.Send(protocol.TextEvent{Text: "b"})
	full.Send(protocol.TextEvent{Text: "c"})
	got := drainEvents(full)
	full.Flush()
	got = append(got, drainEvents(full)...)
	var all []string
	for _, ev := range got {
		if te, ok := ev.(protocol.TextEvent); ok {
			all = append(all, te.Text)
		}
	}
	if strings.Join(all, ",") != "a,b,c" {
		t.Fatalf("full text queue dropped or reordered: %v", all)
	}
}
