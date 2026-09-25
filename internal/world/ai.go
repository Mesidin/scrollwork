package world

import "strings"

// Allows reports whether this mobile may enter dest.
// An empty rooms list means every room.
func (a *AI) Allows(dest ID) bool {
	if a == nil || len(a.Rooms) == 0 {
		return true
	}
	for _, id := range a.Rooms {
		if id == dest {
			return true
		}
	}
	return false
}

// HostileFlag is the flag that wakes a delayed attack. Default is "hostile".
func (a *AI) HostileFlag() string {
	if a == nil || strings.TrimSpace(a.AggroFlag) == "" {
		return "hostile"
	}
	return strings.TrimSpace(a.AggroFlag)
}

// AttacksOnSight is true when sharing a room is enough to start a fight.
func (a *AI) AttacksOnSight() bool {
	if a == nil || a.Profile != "aggressive" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(a.Aggro)) {
	case "", "enter", "sight":
		return true
	default:
		return false
	}
}

// WakesOnLook is true when examining the mobile should start a fight.
func (a *AI) WakesOnLook() bool {
	if a == nil || a.Profile != "aggressive" {
		return false
	}
	return strings.ToLower(strings.TrimSpace(a.Aggro)) == "look"
}

// Awake reports whether an aggressive mobile will fight right now.
func (a *AI) Awake(e *Entity) bool {
	if a == nil || e == nil || a.Profile != "aggressive" {
		return false
	}
	if a.AttacksOnSight() {
		return true
	}
	return e.HasFlag(a.HostileFlag())
}
