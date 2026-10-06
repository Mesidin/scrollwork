package script

import (
	"testing"

	"sudengine/internal/world"
)

func TestSandboxBlocksOS(t *testing.T) {
	err := SandboxForbidden(`os.execute("echo hi")`)
	if err == nil {
		t.Fatal("expected os to be unavailable")
	}
}

func TestSandboxAllowsMath(t *testing.T) {
	err := SandboxForbidden(`assert(math.abs(-2) == 2)`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLuaTeleportAndQuestStatus(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom}
	actor := &world.Entity{
		ID:   "player",
		Kind: world.KindPlayer,
		Quests: map[string]*world.QuestState{
			"scout_camp": {ID: "scout_camp", Status: "active"},
			"old_job":    {ID: "old_job", Status: "completed"},
		},
	}
	scout := &world.Entity{
		ID:       "mob.scout",
		Kind:     world.KindMobile,
		Keywords: []string{"scout"},
		HomeRoom: roomA.ID,
		Scripts:  "test_script.lua",
	}
	w.Add(roomA)
	w.Add(roomB)
	w.Add(actor)
	w.Add(scout)
	_ = w.Move(actor.ID, roomA.ID)
	_ = w.Move(scout.ID, roomA.ID)

	luaCode := `
function on_quest_complete(actor, self, quest_id)
    assert(is_quest_active("scout_camp") == true)
    assert(is_quest_completed("scout_camp") == false)
    assert(has_quest("scout_camp") == true)
    assert(has_quest("old_job") == true)
    assert(is_quest_active("old_job") == false)
    assert(is_quest_completed("old_job") == true)
    assert(teleport("mob.scout", "roomB", true) == true)
    return true
end
`
	h := New(map[string]string{"test_script.lua": luaCode})
	h.World = w

	ok, err := h.Call("on_quest_complete", actor, scout, "scout_camp")
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if !ok {
		t.Fatalf("expected script to return true")
	}

	if scout.Parent != roomB.ID {
		t.Fatalf("expected scout parent %s, got %s", roomB.ID, scout.Parent)
	}
	if scout.HomeRoom != roomB.ID {
		t.Fatalf("expected scout home room %s, got %s", roomB.ID, scout.HomeRoom)
	}
}

func TestTeleportRefusesToReparentRooms(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom}
	scout := &world.Entity{ID: "mob.scout", Kind: world.KindMobile, Keywords: []string{"scout"}, Scripts: "t.lua"}
	w.Add(roomA)
	w.Add(roomB)
	w.Add(scout)
	_ = w.Move(scout.ID, roomA.ID)

	h := New(map[string]string{"t.lua": `
function on_use(actor, self)
    assert(teleport("roomA", "mob.scout") == false)
    assert(teleport("mob.scout", "mob.scout") == false)
    assert(teleport("roomA", "roomB") == false)
    return true
end
`})
	h.World = w
	if _, err := h.Call("on_use", scout, scout); err != nil {
		t.Fatal(err)
	}
	if scout.Parent != roomA.ID {
		t.Fatalf("scout moved to %s", scout.Parent)
	}
	if roomA.Parent != "" {
		t.Fatalf("roomA was reparented to %s", roomA.Parent)
	}
}

func TestTeleportHomeDoesNotWidenOnAOneShot(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom}
	scout := &world.Entity{
		ID: "mob.scout", Kind: world.KindMobile, HomeRoom: roomA.ID, Scripts: "t.lua",
		AI: &world.AI{Rooms: []world.ID{roomA.ID}},
	}
	w.Add(roomA)
	w.Add(roomB)
	w.Add(scout)
	_ = w.Move(scout.ID, roomA.ID)

	h := New(map[string]string{"t.lua": `
function on_use(actor, self)
    assert(teleport("mob.scout", "roomB", false) == true)
    return true
end
`})
	h.World = w
	if _, err := h.Call("on_use", scout, scout); err != nil {
		t.Fatal(err)
	}
	if scout.Parent != roomB.ID {
		t.Fatalf("parent %s", scout.Parent)
	}
	if scout.HomeRoom != roomA.ID || scout.AI.Allows(roomB.ID) {
		t.Fatalf("one-shot teleport changed home %s rooms %+v", scout.HomeRoom, scout.AI.Rooms)
	}
}

func TestTeleportPrefersPrototypeInTheActorsRoom(t *testing.T) {
	w := world.New()
	roomA := &world.Entity{ID: "roomA", Kind: world.KindRoom}
	roomB := &world.Entity{ID: "roomB", Kind: world.KindRoom}
	roomC := &world.Entity{ID: "roomC", Kind: world.KindRoom}
	actor := &world.Entity{ID: "player", Kind: world.KindPlayer, Scripts: "t.lua"}
	here := &world.Entity{ID: "mob.scout#1", PrototypeID: "mob.scout", Kind: world.KindMobile}
	away := &world.Entity{ID: "mob.scout#2", PrototypeID: "mob.scout", Kind: world.KindMobile}
	for _, e := range []*world.Entity{roomA, roomB, roomC, actor, here, away} {
		w.Add(e)
	}
	_ = w.Move(actor.ID, roomA.ID)
	_ = w.Move(here.ID, roomA.ID)
	_ = w.Move(away.ID, roomC.ID)

	h := New(map[string]string{"t.lua": `
function on_use(actor, self)
    assert(teleport("mob.scout", "roomB") == true)
    return true
end
`})
	h.World = w
	if _, err := h.Call("on_use", actor, actor); err != nil {
		t.Fatal(err)
	}
	if here.Parent != roomB.ID || away.Parent != roomC.ID {
		t.Fatalf("here %s away %s", here.Parent, away.Parent)
	}

	_ = w.Move(here.ID, roomA.ID)
	h2 := New(map[string]string{"t.lua": `
function on_use(actor, self)
    assert(teleport("mob.scout", "roomB") == false)
    return true
end
`})
	h2.World = w
	_ = w.Move(actor.ID, roomB.ID)
	if _, err := h2.Call("on_use", actor, actor); err != nil {
		t.Fatal(err)
	}
	if here.Parent != roomA.ID || away.Parent != roomC.ID {
		t.Fatal("ambiguous teleport moved a scout")
	}
}

func TestSetFlagPrefersMobileOverItemPrototype(t *testing.T) {
	w := world.New()
	room := &world.Entity{ID: "room", Kind: world.KindRoom}
	item := &world.Entity{ID: "item.token#1", PrototypeID: "token", Kind: world.KindItem}
	mob := &world.Entity{ID: "mob.scout", Kind: world.KindMobile, Keywords: []string{"token"}}
	actor := &world.Entity{ID: "player", Kind: world.KindPlayer, Scripts: "t.lua"}
	for _, e := range []*world.Entity{room, item, mob, actor} {
		w.Add(e)
	}
	_ = w.Move(actor.ID, room.ID)
	_ = w.Move(item.ID, room.ID)
	_ = w.Move(mob.ID, room.ID)

	h := New(map[string]string{"t.lua": `
function on_use(actor, self)
    set_flag("token", "marked")
    return true
end
`})
	h.World = w
	if _, err := h.Call("on_use", actor, actor); err != nil {
		t.Fatal(err)
	}
	if !mob.HasFlag("marked") {
		t.Fatal("expected the mobile keyword match to take the flag")
	}
	if item.HasFlag("marked") {
		t.Fatal("item prototype should not take a mobile keyword flag")
	}
}
