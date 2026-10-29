package server_test

import (
	"path/filepath"
	"strings"
	"testing"

	"sudengine/internal/protocol"
	"sudengine/internal/quest"
	"sudengine/internal/save"
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

func findLastRoomEvent(evs []protocol.Event) *protocol.RoomEvent {
	rooms := filterRoomEvents(evs)
	if len(rooms) == 0 {
		return nil
	}
	last := rooms[len(rooms)-1]
	return &last
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

func TestStage2CommandQueueRateLimit(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, sess, err := server.Launch(server.Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(sess)

	// Queue two commands: first "look", second "say second command"
	sess.EnqueueCommand("look")
	sess.EnqueueCommand("say second command")

	if len(sess.QueuedCommands()) != 2 {
		t.Fatalf("expected 2 queued commands, got %d", len(sess.QueuedCommands()))
	}

	// First tick: only "look" should be executed
	srv.Tick()

	if len(sess.QueuedCommands()) != 1 {
		t.Fatalf("expected 1 command remaining in queue after tick 1, got %d", len(sess.QueuedCommands()))
	}
	evs1 := drainEvents(sess)
	roomEvs1 := filterRoomEvents(evs1)
	if len(roomEvs1) == 0 {
		t.Fatal("expected RoomEvent from 'look' on tick 1")
	}
	for _, te := range filterTextEvents(evs1) {
		if strings.Contains(te.Text, "second command") {
			t.Fatal("second command ran prematurely on tick 1")
		}
	}

	// Second tick: "say second command" should now be executed
	srv.Tick()

	if len(sess.QueuedCommands()) != 0 {
		t.Fatalf("expected 0 commands remaining in queue after tick 2, got %d", len(sess.QueuedCommands()))
	}
	evs2 := drainEvents(sess)
	foundSay := false
	for _, te := range filterTextEvents(evs2) {
		if strings.Contains(te.Text, "second command") {
			foundSay = true
			break
		}
	}
	if !foundSay {
		t.Fatal("expected 'second command' text event on tick 2")
	}
}

func TestStage2TellPrivacy(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, ada, err := server.Launch(server.Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	bob, _ := srv.SpawnPlayerSession("Bob", "", "")
	charlie, _ := srv.SpawnPlayerSession("Charlie", "", "")
	drainEvents(ada)
	drainEvents(bob)
	drainEvents(charlie)

	// Ada tells Bob
	adaEvs := filterTextEvents(srv.HandleSessionLine(ada, "tell Bob meet me in the cellar"))

	// Bob hears the tell on ChanSay
	bobEvs := filterTextEvents(drainEvents(bob))
	if len(bobEvs) != 1 {
		t.Fatalf("expected Bob to receive 1 event, got %d: %v", len(bobEvs), bobEvs)
	}
	if bobEvs[0].Channel != protocol.ChanSay || !strings.Contains(bobEvs[0].Text, "Ada tells you") || !strings.Contains(bobEvs[0].Text, "meet me in the cellar") {
		t.Fatalf("unexpected message to Bob: %v", bobEvs[0])
	}

	// Ada receives the echo on ChanSay
	if len(adaEvs) != 1 {
		t.Fatalf("expected Ada to receive 1 echo, got %d: %v", len(adaEvs), adaEvs)
	}
	if adaEvs[0].Channel != protocol.ChanSay || !strings.Contains(adaEvs[0].Text, "You tell Bob") || !strings.Contains(adaEvs[0].Text, "meet me in the cellar") {
		t.Fatalf("unexpected echo to Ada: %v", adaEvs[0])
	}

	// Charlie hears nothing
	charlieEvs := drainEvents(charlie)
	if len(charlieEvs) != 0 {
		t.Fatalf("Charlie should not hear private tell, got: %v", charlieEvs)
	}

	// Self-tell
	adaSelf := filterTextEvents(srv.HandleSessionLine(ada, "tell Ada hello"))
	if len(adaSelf) == 0 || !strings.Contains(adaSelf[0].Text, "Talking to yourself?") {
		t.Fatalf("expected self-tell refusal, got: %v", adaSelf)
	}

	// Unknown player
	adaUnknown := filterTextEvents(srv.HandleSessionLine(ada, "tell Nobody hello"))
	if len(adaUnknown) == 0 || !strings.Contains(adaUnknown[0].Text, "Nobody by that name is connected.") {
		t.Fatalf("expected unknown player error, got: %v", adaUnknown)
	}

	bob.Disconnect("test")
	adaGone := filterTextEvents(srv.HandleSessionLine(ada, "tell Bob still there?"))
	if len(adaGone) == 0 || !strings.Contains(adaGone[0].Text, "Nobody by that name is connected.") || strings.Contains(adaGone[0].Text, "You tell") {
		t.Fatalf("disconnected tell should not claim success, got: %v", adaGone)
	}

	adaSelfID := filterTextEvents(srv.HandleSessionLine(ada, "tell player hello"))
	if len(adaSelfID) == 0 || !strings.Contains(adaSelfID[0].Text, "Talking to yourself?") {
		t.Fatalf("tell by own id should refuse, got: %v", adaSelfID)
	}
}

func TestStage2Who(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, ada, err := server.Launch(server.Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada", RoleID: "careful"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = srv.SpawnPlayerSession("Bob", "", "reckless")
	drainEvents(ada)

	evs := filterTextEvents(srv.HandleSessionLine(ada, "who"))
	var all []string
	for _, te := range evs {
		all = append(all, te.Text)
	}
	output := strings.Join(all, "\n")

	// Both characters listed
	if !strings.Contains(output, "Ada") || !strings.Contains(output, "Bob") {
		t.Fatalf("expected both Ada and Bob in who output, got:\n%s", output)
	}

	// Role name shown
	if !strings.Contains(output, "Careful") || !strings.Contains(output, "Reckless") {
		t.Fatalf("expected role names in who output, got:\n%s", output)
	}

	// Primary resource shown (Calm or Grit)
	if !strings.Contains(output, "Calm:") && !strings.Contains(output, "Grit:") {
		t.Fatalf("expected primary resource in who output, got:\n%s", output)
	}

	// No level column
	if strings.Contains(strings.ToLower(output), "level") || strings.Contains(strings.ToLower(output), "lvl") {
		t.Fatalf("who output must not include a level column, got:\n%s", output)
	}
}

func TestStage2Gossip(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, ada, err := server.Launch(server.Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	bob, entBob := srv.SpawnPlayerSession("Bob", "", "")
	_ = srv.World.Move(entBob.ID, "house.foyer") // Bob in another room
	drainEvents(ada)
	drainEvents(bob)

	adaEvs := filterTextEvents(srv.HandleSessionLine(ada, "gossip the sun is rising"))

	// Both hear gossip on ChanSystem
	if len(adaEvs) != 1 || adaEvs[0].Channel != protocol.ChanSystem || !strings.Contains(adaEvs[0].Text, "Ada gossips") {
		t.Fatalf("unexpected gossip for Ada: %v", adaEvs)
	}
	bobEvs := filterTextEvents(drainEvents(bob))
	if len(bobEvs) != 1 || bobEvs[0].Channel != protocol.ChanSystem || !strings.Contains(bobEvs[0].Text, "Ada gossips") {
		t.Fatalf("unexpected gossip for Bob across rooms: %v", bobEvs)
	}

	// Test ooc alias
	_ = srv.HandleSessionLine(bob, "ooc hello all")
	adaOOC := filterTextEvents(drainEvents(ada))
	if len(adaOOC) != 1 || adaOOC[0].Channel != protocol.ChanSystem || !strings.Contains(adaOOC[0].Text, "Bob gossips") {
		t.Fatalf("unexpected ooc for Ada: %v", adaOOC)
	}
}

func TestStage2PartyChat(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, ada, err := server.Launch(server.Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	bob, entBob := srv.SpawnPlayerSession("Bob", "", "")
	charlie, _ := srv.SpawnPlayerSession("Charlie", "", "")
	entAda := srv.World.Get(ada.PlayerID)

	// Bob is in Ada's party, but in a different room
	entAda.PartyLeader = entAda.ID
	entBob.PartyLeader = entAda.ID
	_ = srv.World.Move(entBob.ID, "house.foyer")
	drainEvents(ada)
	drainEvents(bob)
	drainEvents(charlie)

	// Ada sends party chat via 'party'
	adaEvs := filterTextEvents(srv.HandleSessionLine(ada, "party we move at dawn"))

	// Ada receives echo on ChanSay
	if len(adaEvs) != 1 || adaEvs[0].Channel != protocol.ChanSay || !strings.Contains(adaEvs[0].Text, "You tell the party") {
		t.Fatalf("unexpected party echo for Ada: %v", adaEvs)
	}

	// Bob receives party message across rooms on ChanSay
	bobEvs := filterTextEvents(drainEvents(bob))
	if len(bobEvs) != 1 || bobEvs[0].Channel != protocol.ChanSay || !strings.Contains(bobEvs[0].Text, "Ada tells the party") {
		t.Fatalf("unexpected party message for Bob: %v", bobEvs)
	}

	// Charlie is not in the party and hears nothing
	charlieEvs := drainEvents(charlie)
	if len(charlieEvs) != 0 {
		t.Fatalf("Charlie should not hear party chat, got: %v", charlieEvs)
	}

	// Test 'gt' alias
	_ = srv.HandleSessionLine(bob, "gt understood leader")
	adaGT := filterTextEvents(drainEvents(ada))
	if len(adaGT) != 1 || !strings.Contains(adaGT[0].Text, "Bob tells the party") {
		t.Fatalf("unexpected gt message for Ada: %v", adaGT)
	}

	// Non-party player using party chat gets refusal
	charlieMsg := filterTextEvents(srv.HandleSessionLine(charlie, "gt hello?"))
	if len(charlieMsg) == 0 || !strings.Contains(charlieMsg[0].Text, "not in a party") {
		t.Fatalf("expected non-party refusal for Charlie, got: %v", charlieMsg)
	}

	// gt is chat. It does not run party subcommands.
	bobGT := filterTextEvents(srv.HandleSessionLine(bob, "gt leave"))
	if len(bobGT) == 0 || !strings.Contains(bobGT[0].Text, "You tell the party") {
		t.Fatalf("gt leave should be chat, got: %v", bobGT)
	}
	if entBob.PartyLeader != entAda.ID {
		t.Fatalf("gt leave changed the party, leader %s", entBob.PartyLeader)
	}
	adaLeave := filterTextEvents(drainEvents(ada))
	if len(adaLeave) == 0 || !strings.Contains(adaLeave[0].Text, "leave") {
		t.Fatalf("Ada should hear the word leave, got: %v", adaLeave)
	}

	left := filterTextEvents(srv.HandleSessionLine(bob, "party leave"))
	if entBob.PartyLeader != "" {
		t.Fatalf("party leave should drop Bob, leader %s, lines %v", entBob.PartyLeader, left)
	}
}

func TestStage2PartyNeedsAConnectedMember(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, ada, err := server.Launch(server.Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	bob, entBob := srv.SpawnPlayerSession("Bob", "", "")
	entAda := srv.World.Get(ada.PlayerID)
	entAda.PartyLeader = entAda.ID
	entBob.PartyLeader = entAda.ID
	bob.Disconnect("test")
	drainEvents(ada)

	evs := filterTextEvents(srv.HandleSessionLine(ada, "party hello"))
	if len(evs) == 0 || !strings.Contains(evs[0].Text, "Nobody in the party is connected.") || strings.Contains(evs[0].Text, "You tell the party") {
		t.Fatalf("party chat should not claim a disconnected member heard it, got: %v", evs)
	}
}

func TestStage2TellIsUnambiguous(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, ada, err := server.Launch(server.Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	bob, entBob := srv.SpawnPlayerSession("Bob", "", "")
	charlie, entCharlie := srv.SpawnPlayerSession("Charlie", "", "")
	entBob.Name = "Sam"
	entCharlie.Name = "Sam"
	drainEvents(ada)
	drainEvents(bob)
	drainEvents(charlie)

	adaEvs := filterTextEvents(srv.HandleSessionLine(ada, "tell Sam hello"))
	if len(adaEvs) == 0 || !strings.Contains(adaEvs[0].Text, "More than one player matches.") || strings.Contains(adaEvs[0].Text, "You tell") {
		t.Fatalf("ambiguous tell should not send, got: %v", adaEvs)
	}
	if len(drainEvents(bob)) != 0 || len(drainEvents(charlie)) != 0 {
		t.Fatal("ambiguous tell reached a Sam")
	}
}

func TestStage2KillPlayerRefusal(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, ada, err := server.Launch(server.Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1, PlayerName: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	bob, entBob := srv.SpawnPlayerSession("Bob", "", "")
	entAda := srv.World.Get(ada.PlayerID)
	drainEvents(ada)
	drainEvents(bob)

	// Ada attempts to attack Bob
	evs := filterTextEvents(srv.HandleSessionLine(ada, "kill Bob"))
	if len(evs) == 0 || !strings.Contains(evs[0].Text, "You cannot attack other players here.") {
		t.Fatalf("expected calm attack refusal on player, got: %v", evs)
	}

	// Neither is in combat
	if entAda.InCombat() || entBob.InCombat() {
		t.Fatalf("neither player should be in combat: Ada=%v, Bob=%v", entAda.InCombat(), entBob.InCombat())
	}
}

// Stage 5 Done Check 4: A link-dead character is still in the room on the next tick, and is gone after the grace period.
func TestStage5LinkDeadStaysInRoomNextTickAndDespawnsAfterGrace(t *testing.T) {
	origRoot := save.Root
	save.Root = t.TempDir()
	defer func() { save.Root = origRoot }()

	dir := filepath.Join("..", "..", "games", "old-house")
	srv, ada, err := server.Launch(server.Options{
		PackDir:    dir,
		Mode:       protocol.ModePlay,
		Seed:       1,
		PlayerName: "Ada",
		Persist:    true,
		GraceTicks: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	bob, entBob := srv.SpawnPlayerSession("Bob", "", "")
	bob.Persist = true
	bobID := entBob.ID

	// Simulate Bob's socket dropping (stalled after two full ticks).
	for i := 0; i < cap(bob.Out); i++ {
		bob.Send(protocol.TextEvent{Channel: protocol.ChanNarrative, Text: "pad"})
	}
	srv.Tick() // stalledTicks = 1
	srv.Tick() // stalledTicks = 2 -> Bob disconnected as stalled and marked linkdead (due at tick + 3)

	if !bob.Closed() {
		t.Fatalf("expected Bob's session to be closed")
	}

	// Next tick: Bob is STILL in the room and flagged linkdead.
	drainEvents(ada)
	srv.Tick() // 1 tick into 3-tick grace period
	if srv.World.Get(bobID) == nil {
		t.Fatalf("expected link-dead Bob to still be in the world on the next tick")
	}
	if !srv.World.Get(bobID).HasFlag("linkdead") {
		t.Fatalf("expected link-dead Bob to have linkdead flag set")
	}
	adaLook := srv.HandleSessionLine(ada, "look")
	roomEv := findLastRoomEvent(adaLook)
	if roomEv == nil {
		t.Fatalf("expected RoomEvent for Ada")
	}
	foundLinkdeadBob := false
	for _, occ := range roomEv.Occupants {
		if occ == "Bob (linkdead)" {
			foundLinkdeadBob = true
		}
	}
	if !foundLinkdeadBob {
		t.Fatalf("expected Ada to see 'Bob (linkdead)' in room occupants, got %v", roomEv.Occupants)
	}

	// Advance remaining grace ticks (2 more ticks to reach dueTick).
	srv.Tick()
	srv.Tick()

	// After the grace period: Bob is saved to disk and despawned from the world and room.
	if srv.World.Get(bobID) != nil {
		t.Fatalf("expected Bob to be despawned after grace period expired")
	}
	if !save.HasPlayer(srv.Pack.Meta.ID, "Bob") {
		t.Fatalf("expected Bob to be saved to players/bob.json when grace period expired")
	}
	adaLookAfter := srv.HandleSessionLine(ada, "look")
	roomEvAfter := findLastRoomEvent(adaLookAfter)
	for _, occ := range roomEvAfter.Occupants {
		if strings.Contains(occ, "Bob") {
			t.Fatalf("expected Bob to be gone from room occupants after grace period, got %v", roomEvAfter.Occupants)
		}
	}
}

func TestStage5CombatQuitSharesGraceAndDeathRecallsBeforeSave(t *testing.T) {
	origRoot := save.Root
	save.Root = t.TempDir()
	defer func() { save.Root = origRoot }()

	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, ada, err := server.Launch(server.Options{
		PackDir:    dir,
		Mode:       protocol.ModePlay,
		Seed:       1,
		PlayerName: "Ada",
		Persist:    true,
		GraceTicks: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	bob, entBob := srv.SpawnPlayerSession("Bob", "", "warden")
	bob.Persist = true
	bobID := entBob.ID

	// Move Bob out of StartRoom into hollow.lane and start combat with a wolf.
	lane := srv.World.Get("hollow.lane")
	if lane == nil {
		t.Fatal("missing hollow.lane")
	}
	_ = srv.World.Move(entBob.ID, lane.ID)
	wolf, err := srv.World.Spawn("hollow.wolf", lane.ID)
	if err != nil {
		t.Fatal(err)
	}
	wolf.AI = nil
	srv.Combat.Start(entBob, wolf, srv.World.Tick)
	if !entBob.InCombat() {
		t.Fatal("expected Bob to be in combat")
	}

	// Bob types "quit" during combat -> session closes, but Bob stays in the room and in the fight!
	_ = srv.HandleSessionLine(bob, "quit")
	if !bob.Closed() {
		t.Fatal("expected Bob session to close on quit")
	}
	if srv.World.Get(bobID) == nil {
		t.Fatal("combat quit must not immediately despawn Bob before grace period")
	}
	if !entBob.HasFlag("linkdead") || !entBob.InCombat() {
		t.Fatalf("expected Bob to be linkdead and still in combat, linkdead=%v combat=%v", entBob.HasFlag("linkdead"), entBob.InCombat())
	}

	// On the next tick, Bob is still in hollow.lane.
	srv.Tick() // 1 tick into 3-tick grace
	if srv.World.Get(bobID) == nil || entBob.Parent != lane.ID {
		t.Fatalf("expected Bob still in hollow.lane on next tick, got %v", entBob.Parent)
	}

	// During the grace period, Bob dies in combat!
	primary := srv.Pack.RPG.Primary()
	entBob.SetRes(primary, world.Resource{Current: 0, Max: 24})

	// Advance remaining grace ticks: death rules run first (moving Bob to StartRoom), then grace ends and saves Bob at StartRoom.
	srv.Tick()
	srv.Tick()

	if srv.World.Get(bobID) != nil {
		t.Fatalf("expected Bob to be despawned after grace period")
	}
	ps, err := save.ReadPlayer(srv.Pack.Meta.ID, "Bob")
	if err != nil {
		t.Fatalf("ReadPlayer Bob: %v", err)
	}
	if ps.RoomID != srv.World.StartRoom {
		t.Fatalf("expected Bob who died during grace period to be saved at StartRoom %q, got %q", srv.World.StartRoom, ps.RoomID)
	}
	if !ps.Player.Alive(primary) {
		t.Fatalf("expected saved Bob to have restored primary resource after death rules")
	}
	_ = ada
}

func TestStage5OldSlotStillLoadsInPlay(t *testing.T) {
	origRoot := save.Root
	save.Root = t.TempDir()
	defer func() { save.Root = origRoot }()

	dir := filepath.Join("..", "..", "games", "old-house")
	srv1, sess1, err := server.Launch(server.Options{
		PackDir:    dir,
		Mode:       protocol.ModePlay,
		Seed:       1,
		PlayerName: "SoloPlayer",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = srv1.HandleSessionLine(sess1, "n")
	_ = srv1.HandleSessionLine(sess1, "n")
	_ = srv1.HandleSessionLine(sess1, "get key")
	_ = srv1.HandleSessionLine(sess1, "save myslot")

	// Also write paired multiplayer files in the same pack save folder.
	if err := save.WritePaired(srv1.Pack.Meta.ID, srv1.World); err != nil {
		t.Fatal(err)
	}

	// Now launch play mode loading "myslot".
	srv2, sess2, err := server.Launch(server.Options{
		PackDir: dir,
		Mode:    protocol.ModePlay,
		Slot:    "myslot",
		Seed:    1,
	})
	if err != nil {
		t.Fatalf("Launch with old slot failed: %v", err)
	}
	invEvs := filterTextEvents(srv2.HandleSessionLine(sess2, "inventory"))
	var invText string
	for _, te := range invEvs {
		invText += te.Text + "\n"
	}
	if !strings.Contains(invText, "brass key") {
		t.Fatalf("expected loaded single-player slot to have brass key in inventory, got:\n%s", invText)
	}
}

func TestStage5PartyKillCreditAndXPInSameRoomOnly(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "green-hollow")
	srv, adaSess, err := server.Launch(server.Options{
		PackDir:    dir,
		Mode:       protocol.ModePlay,
		Seed:       1,
		PlayerName: "Ada",
	})
	if err != nil {
		t.Fatal(err)
	}
	ada := srv.World.Get(adaSess.PlayerID)
	_, bob := srv.SpawnPlayerSession("Bob", "", "")
	_, cara := srv.SpawnPlayerSession("Cara", "", "")

	// Put Ada, Bob, and Cara in the same party.
	ada.PartyLeader = ada.ID
	bob.PartyLeader = ada.ID
	cara.PartyLeader = ada.ID

	// Ada and Bob are in hollow.square; Cara is in hollow.lane (a different room).
	_ = srv.World.Move(cara.ID, "hollow.lane")

	// Accept the wolf-hunting quest for all three players.
	if len(srv.Pack.Quests) > 0 {
		q := srv.Pack.Quests[0]
		quest.Start(ada, q)
		quest.Start(bob, q)
		quest.Start(cara, q)
	}

	wolf, err := srv.World.Spawn("hollow.wolf", ada.Parent)
	if err != nil {
		t.Fatal(err)
	}
	wolf.AI = nil
	wolf.XP = 10
	primary := srv.Pack.RPG.Primary()
	wolf.SetRes(primary, world.Resource{Current: 0, Max: 10})
	wolf.SlainBy = ada.ID

	srv.Tick()

	// Ada and Bob (both in the room when the wolf died) gain XP and quest kill credit.
	if ada.XP != 10 {
		t.Fatalf("expected Ada to gain 10 XP, got %d", ada.XP)
	}
	if bob.XP != 10 {
		t.Fatalf("expected party member Bob in the same room to gain 10 XP, got %d", bob.XP)
	}
	// Cara (in another room) gets neither XP nor quest kill credit.
	if cara.XP != 0 {
		t.Fatalf("expected Cara in another room to gain 0 XP, got %d", cara.XP)
	}
	if len(srv.Pack.Quests) > 0 {
		q := srv.Pack.Quests[0]
		obj := q.Objectives[0]
		adaCur, _, _ := quest.ObjectiveProgress(ada, srv.World, q, obj)
		bobCur, _, _ := quest.ObjectiveProgress(bob, srv.World, q, obj)
		caraCur, _, _ := quest.ObjectiveProgress(cara, srv.World, q, obj)
		if adaCur != 1 || bobCur != 1 {
			t.Fatalf("expected Ada and Bob quest kill progress=1, got Ada=%d Bob=%d", adaCur, bobCur)
		}
		if caraCur != 0 {
			t.Fatalf("expected Cara in another room to have quest kill progress=0, got %d", caraCur)
		}
	}
}

