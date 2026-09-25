package world

// MirrorExit writes ex on room and copies the door state onto the reverse exit.
func MirrorExit(w *World, room *Entity, dir string, ex Exit) {
	if room == nil {
		return
	}
	if room.Exits == nil {
		room.Exits = map[string]Exit{}
	}
	ex.Dir = dir
	room.Exits[dir] = ex
	if w == nil || (!ex.Door && !ex.Closed && !ex.Locked) {
		return
	}
	dest := w.Get(ex.To)
	opp, back, ok := reverseExit(dest, room.ID)
	if !ok {
		return
	}
	back.Door = back.Door || ex.Door
	back.Closed = ex.Closed
	back.Locked = ex.Locked
	if back.Key == "" {
		back.Key = ex.Key
	}
	back.Dir = opp
	dest.Exits[opp] = back
}

// SyncDoors makes both sides of a door agree. A locked or closed side wins,
// and a key on either side is copied across.
func SyncDoors(w *World) {
	if w == nil {
		return
	}
	seen := map[string]bool{}
	for _, room := range w.Entities {
		if room == nil || room.Kind != KindRoom {
			continue
		}
		for dir, ex := range room.Exits {
			if !ex.Door && !ex.Closed && !ex.Locked {
				continue
			}
			dest := w.Get(ex.To)
			if dest == nil {
				continue
			}
			a := string(room.ID) + ">" + string(dest.ID)
			b := string(dest.ID) + ">" + string(room.ID)
			if seen[a] || seen[b] {
				continue
			}
			seen[a] = true
			opp, back, ok := reverseExit(dest, room.ID)
			if !ok {
				continue
			}
			locked := ex.Locked || back.Locked
			closed := locked || ex.Closed || back.Closed
			door := ex.Door || back.Door || closed || locked
			key := ex.Key
			if key == "" {
				key = back.Key
			}
			ex.Dir = dir
			ex.Door, ex.Closed, ex.Locked, ex.Key = door, closed, locked, key
			room.Exits[dir] = ex
			back.Dir = opp
			back.Door, back.Closed, back.Locked, back.Key = door, closed, locked, key
			dest.Exits[opp] = back
		}
	}
}

func reverseExit(dest *Entity, backTo ID) (string, Exit, bool) {
	if dest == nil {
		return "", Exit{}, false
	}
	for dir, ex := range dest.Exits {
		if ex.To == backTo {
			return dir, ex, true
		}
	}
	return "", Exit{}, false
}
