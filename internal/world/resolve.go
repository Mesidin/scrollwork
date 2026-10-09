package world

import (
	"errors"
	"sort"
	"strings"
)

// ErrNotFound and ErrAmbiguous come from ResolveMobile, ResolveRoom, and ResolveNonMobile.
// An ambiguous match is left unpicked so a map walk cannot move a random body.
var (
	ErrNotFound  = errors.New("not found")
	ErrAmbiguous = errors.New("ambiguous")
)

// ResolveMobile finds one mobile. An exact entity id wins. Otherwise prototype,
// id, and keyword matches are collected. When several match, the single one
// standing in prefer is used. Any other tie is ErrAmbiguous.
func (w *World) ResolveMobile(token string, prefer ID) (*Entity, error) {
	token = strings.TrimSpace(token)
	if w == nil || token == "" {
		return nil, ErrNotFound
	}
	if e := w.Get(ID(token)); e != nil && e.Kind == KindMobile {
		return e, nil
	}
	var cands []*Entity
	seen := map[ID]bool{}
	for _, e := range w.Entities {
		if e == nil || e.Kind != KindMobile || seen[e.ID] {
			continue
		}
		if strings.EqualFold(string(e.PrototypeID), token) || strings.EqualFold(string(e.ID), token) || e.HasKeyword(token) {
			seen[e.ID] = true
			cands = append(cands, e)
		}
	}
	return pickOne(w, cands, prefer)
}

// ResolveRoom finds one room by id, prototype, or keyword.
func (w *World) ResolveRoom(token string) (*Entity, error) {
	token = strings.TrimSpace(token)
	if w == nil || token == "" {
		return nil, ErrNotFound
	}
	if e := w.Get(ID(token)); e != nil && e.Kind == KindRoom {
		return e, nil
	}
	var cands []*Entity
	seen := map[ID]bool{}
	for _, e := range w.Entities {
		if e == nil || e.Kind != KindRoom || seen[e.ID] {
			continue
		}
		if strings.EqualFold(string(e.PrototypeID), token) || strings.EqualFold(string(e.ID), token) || e.HasKeyword(token) {
			seen[e.ID] = true
			cands = append(cands, e)
		}
	}
	return pickOne(w, cands, "")
}

// ResolveNonMobile finds one item, room, player, or other non-mobile.
// An exact id wins. Otherwise prototype, id, and keyword matches use pickOne.
// A tie is ErrAmbiguous, including a unique item when the caller already
// refused an ambiguous mobile.
func (w *World) ResolveNonMobile(token string, prefer ID) (*Entity, error) {
	token = strings.TrimSpace(token)
	if w == nil || token == "" {
		return nil, ErrNotFound
	}
	if e := w.Get(ID(token)); e != nil && e.Kind != KindMobile {
		return e, nil
	}
	var cands []*Entity
	seen := map[ID]bool{}
	for _, e := range w.Entities {
		if e == nil || e.Kind == KindMobile || seen[e.ID] {
			continue
		}
		if strings.EqualFold(string(e.PrototypeID), token) || strings.EqualFold(string(e.ID), token) || e.HasKeyword(token) {
			seen[e.ID] = true
			cands = append(cands, e)
		}
	}
	return pickOne(w, cands, prefer)
}

func pickOne(w *World, cands []*Entity, prefer ID) (*Entity, error) {
	if len(cands) == 0 {
		return nil, ErrNotFound
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].ID < cands[j].ID })
	if len(cands) == 1 {
		return cands[0], nil
	}
	if prefer != "" && w != nil {
		var in []*Entity
		for _, e := range cands {
			if room := w.RoomOf(e); room != nil && room.ID == prefer {
				in = append(in, e)
			}
		}
		if len(in) == 1 {
			return in[0], nil
		}
	}
	return nil, ErrAmbiguous
}

// RememberHome sticks respawn, and a restricted leash, to dest.
// A one-shot move should not call it.
func RememberHome(e, dest *Entity) {
	if e == nil || dest == nil || dest.Kind != KindRoom {
		return
	}
	e.HomeRoom = dest.ID
	if e.AI == nil || len(e.AI.Rooms) == 0 || e.AI.Allows(dest.ID) {
		return
	}
	e.AI.Rooms = append(e.AI.Rooms, dest.ID)
}
