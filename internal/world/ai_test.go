package world

import "testing"

func TestAIRoomsAndAggro(t *testing.T) {
	ai := &AI{Profile: "aggressive", Rooms: []ID{"house.wine", "house.cellar"}}
	if !ai.Allows("house.wine") || ai.Allows("house.foyer") {
		t.Fatal(ai.Rooms)
	}
	if !ai.AttacksOnSight() || ai.WakesOnLook() {
		t.Fatal("default aggressive should attack on sight")
	}
	ai.Aggro = "look"
	mob := &Entity{}
	if ai.Awake(mob) || !ai.WakesOnLook() {
		t.Fatal("look aggro should sleep until the flag")
	}
	mob.SetFlag(ai.HostileFlag(), true)
	if !ai.Awake(mob) {
		t.Fatal("hostile flag should wake it")
	}
}

func TestSyncDoorsCopiesLock(t *testing.T) {
	w := New()
	up := &Entity{ID: "cellar", Kind: KindRoom, Exits: map[string]Exit{
		"up": {Dir: "up", To: "foyer", Door: true},
	}}
	foyer := &Entity{ID: "foyer", Kind: KindRoom, Exits: map[string]Exit{
		"down": {Dir: "down", To: "cellar", Door: true, Closed: true, Locked: true, Key: "key"},
	}}
	w.Add(up)
	w.Add(foyer)
	SyncDoors(w)
	ex := up.Exits["up"]
	if !ex.Locked || !ex.Closed || ex.Key != "key" {
		t.Fatalf("cellar side did not inherit the lock: %+v", ex)
	}
	ex.Locked = false
	ex.Closed = false
	MirrorExit(w, up, "up", ex)
	if foyer.Exits["down"].Locked || foyer.Exits["down"].Closed {
		t.Fatalf("unlock did not mirror: %+v", foyer.Exits["down"])
	}
}
