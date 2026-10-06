package commands

import (
	"strings"
	"testing"

	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/quest"
	"sudengine/internal/script"
	"sudengine/internal/world"
)

func TestQuestHookRunsBeforeRelocate(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom, Name: "Hall"}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom, Name: "Yard"}
	player := &world.Entity{ID: "player", Kind: world.KindPlayer, Name: "Ada"}
	bram := &world.Entity{
		ID: "bram#1", PrototypeID: "hollow.bram", Kind: world.KindMobile,
		Name: "Bram", Keywords: []string{"bram"}, Scripts: "bram.lua",
	}
	w.Add(roomA)
	w.Add(roomB)
	w.Add(player)
	w.Add(bram)
	w.PlayerID = player.ID
	_ = w.Move(player.ID, roomA.ID)
	_ = w.Move(bram.ID, roomA.ID)

	var lines []string
	var heardIn world.ID
	h := script.New(map[string]string{"bram.lua": `
function on_quest_complete(actor, self, quest_id)
    echo_room("Bram raises his hand in a sharp salute.")
    if is_quest_completed(quest_id) then
        echo("The quest is already written down.")
    end
    return true
end
`})
	h.World = w
	h.Echo = func(actor *world.Entity, text string) {
		if actor != nil && actor.ID == player.ID {
			lines = append(lines, text)
		}
	}
	h.EchoRoom = func(room *world.Entity, text string, skip *world.Entity) {
		if room != nil && w.RoomOf(player) == room {
			heardIn = room.ID
			lines = append(lines, text)
		}
	}

	q := quest.Quest{
		ID:     "reloc_test",
		Title:  "Move Bram",
		TurnIn: "hollow.bram",
		Rewards: quest.Rewards{
			Relocate: []quest.Relocation{{
				Mob:     "hollow.bram",
				To:      "roomB",
				Message: "Bram marches south into the cottage.",
			}},
		},
	}
	quest.Start(player, q)
	c := &Context{
		World:   w,
		Actor:   player,
		Pack:    &pack.Pack{Quests: []quest.Quest{q}},
		Scripts: h,
		Tell: func(ch protocol.Channel, text string) {
			lines = append(lines, text)
		},
	}
	questCmd(c, Parse("quest complete reloc_test", nil))

	if heardIn != roomA.ID {
		t.Fatalf("room echo fired in %s", heardIn)
	}
	joined := strings.Join(lines, "\n")
	salute := strings.Index(joined, "Bram raises his hand in a sharp salute.")
	march := strings.Index(joined, "Bram marches south into the cottage.")
	if salute < 0 || march < 0 || salute > march {
		t.Fatalf("expected the salute before the move, got:\n%s", joined)
	}
	if !strings.Contains(joined, "The quest is already written down.") {
		t.Fatalf("hook should see the quest completed, got:\n%s", joined)
	}
	if bram.Parent != roomB.ID {
		t.Fatalf("bram parent %s", bram.Parent)
	}
}

func TestQuestGiverScriptFallback(t *testing.T) {
	w := world.New()
	room := &world.Entity{ID: "room", Kind: world.KindRoom, Name: "Hall"}
	player := &world.Entity{ID: "player", Kind: world.KindPlayer, Name: "Ada"}
	clerk := &world.Entity{ID: "clerk#1", PrototypeID: "hollow.clerk", Kind: world.KindMobile, Keywords: []string{"clerk"}, Name: "clerk"}
	bram := &world.Entity{
		ID: "bram#1", PrototypeID: "hollow.bram", Kind: world.KindMobile,
		Name: "Bram", Scripts: "bram.lua",
	}
	w.Add(room)
	w.Add(player)
	w.Add(clerk)
	w.Add(bram)
	_ = w.Move(player.ID, room.ID)
	_ = w.Move(clerk.ID, room.ID)
	_ = w.Move(bram.ID, room.ID)

	h := script.New(map[string]string{"bram.lua": `
function on_quest_complete(actor, self, quest_id)
    echo("Bram speaks for the clerk.")
    return true
end
`})
	h.World = w
	q := quest.Quest{
		ID:     "papers",
		Title:  "Papers",
		Giver:  "hollow.bram",
		TurnIn: "hollow.clerk",
	}
	quest.Start(player, q)
	lines := runQuest(w, player, h, q, "quest complete papers")
	if !strings.Contains(strings.Join(lines, "\n"), "Bram speaks for the clerk.") {
		t.Fatalf("giver script did not run:\n%s", strings.Join(lines, "\n"))
	}
}

func TestQuestRelocateFailureIsVisible(t *testing.T) {
	w := world.New()
	room := &world.Entity{ID: "room", Kind: world.KindRoom, Name: "Hall"}
	player := &world.Entity{ID: "player", Kind: world.KindPlayer, Name: "Ada"}
	bram := &world.Entity{ID: "bram#1", PrototypeID: "hollow.bram", Kind: world.KindMobile, Keywords: []string{"bram"}, Name: "Bram"}
	w.Add(room)
	w.Add(player)
	w.Add(bram)
	_ = w.Move(player.ID, room.ID)
	_ = w.Move(bram.ID, room.ID)

	q := quest.Quest{
		ID:     "gone",
		Title:  "Gone",
		TurnIn: "hollow.bram",
		Rewards: quest.Rewards{
			Relocate: []quest.Relocation{{
				Mob:     "mob.missing",
				To:      "room",
				Message: "They slip out the back.",
			}},
		},
	}
	quest.Start(player, q)
	lines := runQuest(w, player, nil, q, "quest complete gone")
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "They slip out the back.") {
		t.Fatalf("missing relocate was narrated as success:\n%s", joined)
	}
	if !strings.Contains(joined, "No mobile matches mob.missing") {
		t.Fatalf("expected a visible failure, got:\n%s", joined)
	}
	if !quest.IsCompleted(player, q) {
		t.Fatal("quest should still be completed")
	}
}

func runQuest(w *world.World, player *world.Entity, h *script.Host, q quest.Quest, line string) []string {
	var lines []string
	if h != nil && h.Echo == nil {
		h.Echo = func(actor *world.Entity, text string) {
			if actor != nil && actor.ID == player.ID {
				lines = append(lines, text)
			}
		}
	}
	if h != nil && h.EchoRoom == nil {
		h.EchoRoom = func(room *world.Entity, text string, skip *world.Entity) {
			if room != nil && w.RoomOf(player) == room {
				lines = append(lines, text)
			}
		}
	}
	c := &Context{
		World:   w,
		Actor:   player,
		Pack:    &pack.Pack{Quests: []quest.Quest{q}},
		Scripts: h,
		Tell: func(ch protocol.Channel, text string) {
			lines = append(lines, text)
		},
	}
	questCmd(c, Parse(line, nil))
	return lines
}
