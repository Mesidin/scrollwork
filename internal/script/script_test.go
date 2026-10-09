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

func TestLuaPartyAndCompanion(t *testing.T) {
	w := world.New()
	room := &world.Entity{ID: "room", Kind: world.KindRoom}
	actor := &world.Entity{ID: "player", Kind: world.KindPlayer, Name: "Hero"}
	wolf := &world.Entity{ID: "wolf", Kind: world.KindMobile, Name: "grey wolf", Keywords: []string{"wolf"}, Scripts: "wolf.lua"}
	ranger := &world.Entity{ID: "ranger", Kind: world.KindMobile, Name: "ranger", Keywords: []string{"ranger"}, Scripts: "ranger.lua"}
	for _, e := range []*world.Entity{room, actor, wolf, ranger} {
		w.Add(e)
	}
	_ = w.Move(actor.ID, room.ID)
	_ = w.Move(wolf.ID, room.ID)
	_ = w.Move(ranger.ID, room.ID)

	wolfLua := `
function on_use(actor, self)
    assert(set_companion(self, actor, 80, "defend") == true)
    assert(get_loyalty(self) == 80)
    set_loyalty(self, 95)
    assert(get_loyalty(self) == 95)
    assert(get_stance(self) == "defend")
    set_stance(self, "assist")
    assert(get_stance(self) == "assist")
    return true
end
`
	rangerLua := `
function on_ask(actor, self, topic)
    if topic == "party" then
        assert(join_party(self, actor) == true)
        return true
    elseif topic == "invite" then
        assert(invite_party(self, actor) == true)
        return true
    elseif topic == "leave" then
        assert(leave_party(self) == true)
        return true
    end
    return false
end
`
	h := New(map[string]string{
		"wolf.lua":   wolfLua,
		"ranger.lua": rangerLua,
	})
	h.World = w

	// 1. Companion wolf test
	ok, err := h.Call("on_use", actor, wolf)
	if err != nil || !ok {
		t.Fatalf("wolf lua error: %v, ok=%v", err, ok)
	}
	if !wolf.Companion || wolf.Owner != actor.ID || wolf.Loyalty != 95 || wolf.Stance != "assist" {
		t.Fatalf("wolf companion state mismatch: %+v", wolf)
	}
	if wolf.Leader != actor.ID {
		t.Fatalf("expected wolf to be following actor")
	}

	// 2. Ranger join party test
	ok, err = h.Call("on_ask", actor, ranger, "party")
	if err != nil || !ok {
		t.Fatalf("ranger join party error: %v", err)
	}
	if ranger.PartyLeader != actor.ID || ranger.Leader != actor.ID {
		t.Fatalf("ranger party state mismatch: %+v", ranger)
	}

	// 3. Ranger leave party test
	ok, err = h.Call("on_ask", actor, ranger, "leave")
	if err != nil || !ok {
		t.Fatalf("ranger leave party error: %v", err)
	}
	if ranger.PartyLeader != "" || ranger.Leader != "" {
		t.Fatalf("ranger should have left party: %+v", ranger)
	}

	// 4. Ranger invite player test
	ok, err = h.Call("on_ask", actor, ranger, "invite")
	if err != nil || !ok {
		t.Fatalf("ranger invite error: %v", err)
	}
	found := false
	for _, inv := range actor.PartyInvites {
		if inv == ranger.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected actor to have party invite from ranger")
	}
}

func TestLuaLearnRecipeMethod(t *testing.T) {
	w := world.New()
	room := &world.Entity{ID: "room", Kind: world.KindRoom}
	actor := &world.Entity{ID: "player", Kind: world.KindPlayer, Name: "Hero"}
	npc := &world.Entity{ID: "sage", Kind: world.KindMobile, Name: "sage", Scripts: "sage.lua"}
	w.Add(room)
	w.Add(actor)
	w.Add(npc)
	_ = w.Move(actor.ID, room.ID)
	_ = w.Move(npc.ID, room.ID)

	sageLua := `
function on_ask(actor, self, topic)
    if topic == "alchemy" then
        learn_recipe("brew_heal", actor, "grant")
        assert(get_recipe_method("brew_heal", actor) == "grant")
        return true
    elseif topic == "trade" then
        learn_recipe("iron_dagger", actor, "buy")
        assert(get_recipe_method("iron_dagger", actor) == "buy")
        return true
    end
    return false
end
`
	h := New(map[string]string{
		"sage.lua": sageLua,
	})
	h.World = w

	ok, err := h.Call("on_ask", actor, npc, "alchemy")
	if err != nil || !ok {
		t.Fatalf("alchemy on_ask failed: %v", err)
	}
	if !actor.KnowsRecipe("brew_heal") || actor.RecipeMethod("brew_heal") != "grant" {
		t.Fatalf("expected brew_heal granted, got method %s", actor.RecipeMethod("brew_heal"))
	}

	ok, err = h.Call("on_ask", actor, npc, "trade")
	if err != nil || !ok {
		t.Fatalf("trade on_ask failed: %v", err)
	}
	if !actor.KnowsRecipe("iron_dagger") || actor.RecipeMethod("iron_dagger") != "buy" {
		t.Fatalf("expected iron_dagger bought, got method %s", actor.RecipeMethod("iron_dagger"))
	}
}


func TestLuaLeaderLeaveAndJoinReleasesMembers(t *testing.T) {
	w := world.New()
	room := &world.Entity{ID: "room", Kind: world.KindRoom}
	actor := &world.Entity{ID: "player", Kind: world.KindPlayer, Name: "Hero"}
	wolf := &world.Entity{ID: "wolf", Kind: world.KindMobile, Name: "wolf", Keywords: []string{"wolf"}, Scripts: "wolf.lua"}
	ranger := &world.Entity{ID: "ranger", Kind: world.KindMobile, Name: "ranger"}
	for _, e := range []*world.Entity{room, actor, wolf, ranger} {
		w.Add(e)
	}
	actor.PartyLeader = actor.ID
	wolf.PartyLeader = actor.ID
	_ = w.StartFollowing(wolf, actor)

	h := New(map[string]string{"wolf.lua": `
function on_use(actor, self)
    return leave_party(actor)
end
function on_ask(actor, self, topic)
    return join_party(self, actor)
end
`})
	h.World = w
	ok, err := h.Call("on_use", actor, wolf)
	if err != nil || !ok {
		t.Fatalf("leave: %v ok=%v", err, ok)
	}
	if actor.PartyLeader != "" || wolf.PartyLeader != "" {
		t.Fatalf("leader leave should clear the party, actor %s wolf %s", actor.PartyLeader, wolf.PartyLeader)
	}

	wolf.PartyLeader = wolf.ID
	ranger.PartyLeader = wolf.ID
	ok, err = h.Call("on_ask", actor, wolf, "party")
	if err != nil || !ok {
		t.Fatalf("join: %v ok=%v", err, ok)
	}
	if wolf.PartyLeader != actor.ID {
		t.Fatalf("wolf joined %s", wolf.PartyLeader)
	}
	if ranger.PartyLeader != "" {
		t.Fatalf("old member still on wolf's party: %s", ranger.PartyLeader)
	}
}
