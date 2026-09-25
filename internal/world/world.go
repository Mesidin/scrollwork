// Package world is the live object tree: rooms, items, mobiles, the player.
package world

import (
	"fmt"
	"strings"
)

type ID string

type Kind string

const (
	KindRoom    Kind = "room"
	KindItem    Kind = "item"
	KindMobile  Kind = "mobile"
	KindPlayer  Kind = "player"
	KindScenery Kind = "scenery"
)

type Resource struct {
	Current int `json:"current" yaml:"current"`
	Max     int `json:"max" yaml:"max"`
}

type Exit struct {
	Dir    string `json:"dir" yaml:"dir"`
	To     ID     `json:"to" yaml:"to"`
	Door   bool   `json:"door,omitempty" yaml:"door,omitempty"`
	Closed bool   `json:"closed,omitempty" yaml:"closed,omitempty"`
	Locked bool   `json:"locked,omitempty" yaml:"locked,omitempty"`
	Key    string `json:"key,omitempty" yaml:"key,omitempty"`
}

type Attack struct {
	Name   string `json:"name" yaml:"name"`
	Damage string `json:"damage" yaml:"damage"`
	Skill  string `json:"skill,omitempty" yaml:"skill,omitempty"`
	Hit    string `json:"hit,omitempty" yaml:"hit,omitempty"`
	Miss   string `json:"miss,omitempty" yaml:"miss,omitempty"`
}

type AI struct {
	Profile     string   `json:"profile,omitempty" yaml:"profile,omitempty"`
	Wander      bool     `json:"wander,omitempty" yaml:"wander,omitempty"`
	AggroRange  int      `json:"aggro_range,omitempty" yaml:"aggro_range,omitempty"`
	AssistTags  []string `json:"assist_tags,omitempty" yaml:"assist_tags,omitempty"`
	WanderEvery int      `json:"wander_every,omitempty" yaml:"wander_every,omitempty"`
	// Rooms limits wander and pursuit to these room ids. Empty means the whole map.
	Rooms []ID `json:"rooms,omitempty" yaml:"rooms,omitempty"`
	// Aggro is when a fight starts: enter (default), look, or flag.
	Aggro string `json:"aggro,omitempty" yaml:"aggro,omitempty"`
	// AggroFlag is the flag that wakes aggro look/flag. Default "hostile".
	AggroFlag string `json:"aggro_flag,omitempty" yaml:"aggro_flag,omitempty"`
	// Pursue follows a combat target through an open exit, still inside Rooms.
	Pursue bool `json:"pursue,omitempty" yaml:"pursue,omitempty"`
}

type Combat struct {
	RoundSpeed int      `json:"round_speed,omitempty" yaml:"round_speed,omitempty"`
	Attacks    []Attack `json:"attacks,omitempty" yaml:"attacks,omitempty"`
	Boss       bool     `json:"boss,omitempty" yaml:"boss,omitempty"`
	Target     ID       `json:"target,omitempty" yaml:"target,omitempty"`
	NextSwing  int64    `json:"next_swing,omitempty" yaml:"next_swing,omitempty"`
}

type UseEffect struct {
	ToggleFlag string `json:"toggle_flag,omitempty" yaml:"toggle_flag,omitempty"`
	Message    string `json:"message,omitempty" yaml:"message,omitempty"`
	MessageOff string `json:"message_off,omitempty" yaml:"message_off,omitempty"`
}

type Weapon struct {
	Damage string `json:"damage,omitempty" yaml:"damage,omitempty"`
	Verb   string `json:"verb,omitempty" yaml:"verb,omitempty"`
	Skill  string `json:"skill,omitempty" yaml:"skill,omitempty"`
	Hit    string `json:"hit,omitempty" yaml:"hit,omitempty"`
	Miss   string `json:"miss,omitempty" yaml:"miss,omitempty"`
}

type Armor struct {
	Soak int `json:"soak,omitempty" yaml:"soak,omitempty"`
}

type Shop struct {
	Currency string   `json:"currency,omitempty" yaml:"currency,omitempty"`
	Markup   int      `json:"markup,omitempty" yaml:"markup,omitempty"` // percent added when player buys
	Stock    []string `json:"stock,omitempty" yaml:"stock,omitempty"`
}

type Entity struct {
	ID          ID       `json:"id" yaml:"id"`
	PrototypeID ID       `json:"prototype_id,omitempty" yaml:"prototype_id,omitempty"`
	Kind        Kind     `json:"kind" yaml:"kind"`
	Keywords    []string `json:"keywords,omitempty" yaml:"keywords,omitempty"`
	Name        string   `json:"name" yaml:"name"`
	Short       string   `json:"short,omitempty" yaml:"short,omitempty"`
	Long        string   `json:"long,omitempty" yaml:"long,omitempty"`
	Parent      ID       `json:"parent,omitempty" yaml:"parent,omitempty"`
	Contents    []ID     `json:"contents,omitempty" yaml:"contents,omitempty"`

	X         int             `json:"x,omitempty" yaml:"x,omitempty"`
	Y         int             `json:"y,omitempty" yaml:"y,omitempty"`
	Z         int             `json:"z,omitempty" yaml:"z,omitempty"`
	HasCoords bool            `json:"has_coords,omitempty" yaml:"has_coords,omitempty"`
	Exits     map[string]Exit `json:"exits,omitempty" yaml:"exits,omitempty"`

	Takeable  bool           `json:"takeable,omitempty" yaml:"takeable,omitempty"`
	Container bool           `json:"container,omitempty" yaml:"container,omitempty"`
	Wearable  bool           `json:"wearable,omitempty" yaml:"wearable,omitempty"`
	Slot      string         `json:"slot,omitempty" yaml:"slot,omitempty"`
	Closed    bool           `json:"item_closed,omitempty" yaml:"item_closed,omitempty"`
	Locked    bool           `json:"item_locked,omitempty" yaml:"item_locked,omitempty"`
	Key       string         `json:"item_key,omitempty" yaml:"item_key,omitempty"`
	Use       *UseEffect     `json:"use,omitempty" yaml:"use,omitempty"`
	Weapon    *Weapon        `json:"weapon,omitempty" yaml:"weapon,omitempty"`
	Armor     *Armor         `json:"armor,omitempty" yaml:"armor,omitempty"`
	Value     int            `json:"value,omitempty" yaml:"value,omitempty"`
	Mods      map[string]int `json:"mods,omitempty" yaml:"mods,omitempty"`
	Hidden    bool           `json:"hidden,omitempty" yaml:"hidden,omitempty"`
	Notice    int            `json:"notice,omitempty" yaml:"notice,omitempty"`
	SearchDC  int            `json:"search,omitempty" yaml:"search,omitempty"`

	AI     *AI               `json:"ai,omitempty" yaml:"ai,omitempty"`
	Combat *Combat           `json:"combat,omitempty" yaml:"combat,omitempty"`
	Topics map[string]string `json:"topics,omitempty" yaml:"topics,omitempty"`

	Attrs     map[string]int      `json:"attrs,omitempty" yaml:"attrs,omitempty"`
	Resources map[string]Resource `json:"resources,omitempty" yaml:"resources,omitempty"`
	Skills    map[string]int      `json:"skills,omitempty" yaml:"skills,omitempty"`
	Oppose    map[string]int      `json:"oppose,omitempty" yaml:"oppose,omitempty"`
	Flags     map[string]bool     `json:"flags,omitempty" yaml:"flags,omitempty"`
	Found     map[string]bool     `json:"found,omitempty" yaml:"found,omitempty"`
	Tags      []string            `json:"tags,omitempty" yaml:"tags,omitempty"`
	Equipment map[string]ID       `json:"equipment,omitempty" yaml:"equipment,omitempty"`
	Cooldown  map[string]int64    `json:"cooldown,omitempty" yaml:"cooldown,omitempty"`

	Scripts string `json:"scripts,omitempty" yaml:"scripts,omitempty"`
	Shop    *Shop  `json:"shop,omitempty" yaml:"shop,omitempty"`

	OriginID   string `json:"origin_id,omitempty" yaml:"origin_id,omitempty"`
	OriginName string `json:"origin_name,omitempty" yaml:"origin_name,omitempty"`
	RoleID     string `json:"role_id,omitempty" yaml:"role_id,omitempty"`
	RoleName   string `json:"role_name,omitempty" yaml:"role_name,omitempty"`

	XP          int  `json:"xp,omitempty" yaml:"xp,omitempty"`
	Level       int  `json:"level,omitempty" yaml:"level,omitempty"`
	SkillPoints int  `json:"skill_points,omitempty" yaml:"skill_points,omitempty"`
	AttrPoints  int  `json:"attr_points,omitempty" yaml:"attr_points,omitempty"`
	Trainer     bool `json:"trainer,omitempty" yaml:"trainer,omitempty"`
}

type World struct {
	Entities  map[ID]*Entity `json:"entities"`
	Protos    map[ID]*Entity `json:"protos"`
	PlayerID  ID             `json:"player_id"`
	StartRoom ID             `json:"start_room"`
	Tick      int64          `json:"tick"`
	NextSeq   int            `json:"next_seq"`
	PackID    string         `json:"pack_id"`
}

func New() *World {
	return &World{
		Entities: map[ID]*Entity{},
		Protos:   map[ID]*Entity{},
	}
}

func (e *Entity) Display() string {
	if e.Short != "" {
		return e.Short
	}
	return e.Name
}

func (e *Entity) CapDisplay() string {
	s := e.Display()
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (e *Entity) HasKeyword(token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	if strings.EqualFold(string(e.ID), token) {
		return true
	}
	if e.PrototypeID != "" && strings.EqualFold(string(e.PrototypeID), token) {
		return true
	}
	for _, k := range e.Keywords {
		if strings.EqualFold(k, token) {
			return true
		}
	}
	// last path segment of id: house.lamp -> lamp
	if i := strings.LastIndex(string(e.ID), "."); i >= 0 {
		rest := string(e.ID)[i+1:]
		if j := strings.Index(rest, "#"); j >= 0 {
			rest = rest[:j]
		}
		if strings.EqualFold(rest, token) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(e.Name), token) ||
		strings.Contains(strings.ToLower(e.Short), token)
}

func (e *Entity) HasFlag(name string) bool {
	return e.Flags != nil && e.Flags[name]
}

func (e *Entity) HasFound(id ID) bool {
	return e != nil && e.Found != nil && e.Found[string(id)]
}

func (e *Entity) MarkFound(id ID) {
	if e == nil {
		return
	}
	if e.Found == nil {
		e.Found = map[string]bool{}
	}
	e.Found[string(id)] = true
}

func (e *Entity) SetFlag(name string, v bool) {
	if e.Flags == nil {
		e.Flags = map[string]bool{}
	}
	if v {
		e.Flags[name] = true
	} else {
		delete(e.Flags, name)
	}
}

func (e *Entity) HasTag(tag string) bool {
	for _, t := range e.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func (e *Entity) Res(key string) Resource {
	if e.Resources == nil {
		return Resource{}
	}
	return e.Resources[key]
}

func (e *Entity) SetRes(key string, r Resource) {
	if e.Resources == nil {
		e.Resources = map[string]Resource{}
	}
	if r.Current < 0 {
		r.Current = 0
	}
	if r.Max > 0 && r.Current > r.Max {
		r.Current = r.Max
	}
	e.Resources[key] = r
}

func (e *Entity) AdjustRes(key string, delta int) int {
	r := e.Res(key)
	r.Current += delta
	e.SetRes(key, r)
	return e.Res(key).Current
}

func (e *Entity) Alive(primary string) bool {
	if primary == "" {
		primary = "hp"
	}
	r, ok := e.Resources[primary]
	if !ok {
		return true
	}
	return r.Current > 0
}

func (e *Entity) InCombat() bool {
	return e.Combat != nil && e.Combat.Target != ""
}

func (e *Entity) Clone() *Entity {
	cp := *e
	cp.Keywords = append([]string{}, e.Keywords...)
	cp.Tags = append([]string{}, e.Tags...)
	if e.Exits != nil {
		cp.Exits = make(map[string]Exit, len(e.Exits))
		for k, v := range e.Exits {
			cp.Exits[k] = v
		}
	}
	if e.Topics != nil {
		cp.Topics = make(map[string]string, len(e.Topics))
		for k, v := range e.Topics {
			cp.Topics[k] = v
		}
	}
	if e.Attrs != nil {
		cp.Attrs = make(map[string]int, len(e.Attrs))
		for k, v := range e.Attrs {
			cp.Attrs[k] = v
		}
	}
	if e.Resources != nil {
		cp.Resources = make(map[string]Resource, len(e.Resources))
		for k, v := range e.Resources {
			cp.Resources[k] = v
		}
	}
	if e.Skills != nil {
		cp.Skills = make(map[string]int, len(e.Skills))
		for k, v := range e.Skills {
			cp.Skills[k] = v
		}
	}
	if e.Mods != nil {
		cp.Mods = make(map[string]int, len(e.Mods))
		for k, v := range e.Mods {
			cp.Mods[k] = v
		}
	}
	if e.Oppose != nil {
		cp.Oppose = make(map[string]int, len(e.Oppose))
		for k, v := range e.Oppose {
			cp.Oppose[k] = v
		}
	}
	if e.Found != nil {
		cp.Found = make(map[string]bool, len(e.Found))
		for k, v := range e.Found {
			cp.Found[k] = v
		}
	}
	if e.Flags != nil {
		cp.Flags = make(map[string]bool, len(e.Flags))
		for k, v := range e.Flags {
			cp.Flags[k] = v
		}
	}
	if e.Equipment != nil {
		cp.Equipment = make(map[string]ID, len(e.Equipment))
		for k, v := range e.Equipment {
			cp.Equipment[k] = v
		}
	}
	if e.Cooldown != nil {
		cp.Cooldown = make(map[string]int64, len(e.Cooldown))
		for k, v := range e.Cooldown {
			cp.Cooldown[k] = v
		}
	}
	if e.AI != nil {
		ai := *e.AI
		ai.AssistTags = append([]string{}, e.AI.AssistTags...)
		ai.Rooms = append([]ID{}, e.AI.Rooms...)
		cp.AI = &ai
	}
	if e.Combat != nil {
		c := *e.Combat
		c.Attacks = append([]Attack{}, e.Combat.Attacks...)
		c.Target = ""
		c.NextSwing = 0
		cp.Combat = &c
	}
	if e.Use != nil {
		u := *e.Use
		cp.Use = &u
	}
	if e.Weapon != nil {
		w := *e.Weapon
		cp.Weapon = &w
	}
	if e.Armor != nil {
		a := *e.Armor
		cp.Armor = &a
	}
	if e.Shop != nil {
		s := *e.Shop
		s.Stock = append([]string{}, e.Shop.Stock...)
		cp.Shop = &s
	}
	cp.Contents = append([]ID{}, e.Contents...)
	return &cp
}

func (w *World) Add(e *Entity) {
	if e.Exits == nil && e.Kind == KindRoom {
		e.Exits = map[string]Exit{}
	}
	w.Entities[e.ID] = e
}

func (w *World) Get(id ID) *Entity {
	return w.Entities[id]
}

func (w *World) Player() *Entity {
	return w.Get(w.PlayerID)
}

func (w *World) RoomOf(e *Entity) *Entity {
	if e == nil {
		return nil
	}
	cur := e
	for i := 0; i < 16 && cur != nil; i++ {
		if cur.Kind == KindRoom {
			return cur
		}
		cur = w.Get(cur.Parent)
	}
	return nil
}

func (w *World) Children(id ID) []*Entity {
	e := w.Get(id)
	if e == nil {
		return nil
	}
	out := make([]*Entity, 0, len(e.Contents))
	for _, cid := range e.Contents {
		if c := w.Get(cid); c != nil {
			out = append(out, c)
		}
	}
	return out
}

func (w *World) Detach(id ID) {
	e := w.Get(id)
	if e == nil || e.Parent == "" {
		return
	}
	parent := w.Get(e.Parent)
	if parent != nil {
		n := parent.Contents[:0]
		for _, c := range parent.Contents {
			if c != id {
				n = append(n, c)
			}
		}
		parent.Contents = n
	}
	e.Parent = ""
}

func (w *World) Move(id, dest ID) error {
	e := w.Get(id)
	if e == nil {
		return fmt.Errorf("no such entity %s", id)
	}
	d := w.Get(dest)
	if d == nil {
		return fmt.Errorf("no such destination %s", dest)
	}
	w.Detach(id)
	e.Parent = dest
	d.Contents = append(d.Contents, id)
	return nil
}

func (w *World) Destroy(id ID) {
	e := w.Get(id)
	if e == nil {
		return
	}
	for _, c := range append([]ID{}, e.Contents...) {
		w.Destroy(c)
	}
	w.Detach(id)
	delete(w.Entities, id)
}

func (w *World) Spawn(protoID ID, dest ID) (*Entity, error) {
	p := w.Protos[protoID]
	if p == nil {
		return nil, fmt.Errorf("unknown prototype %s", protoID)
	}
	w.NextSeq++
	inst := p.Clone()
	inst.PrototypeID = protoID
	inst.ID = ID(fmt.Sprintf("%s#%d", protoID, w.NextSeq))
	inst.Parent = ""
	inst.Contents = nil
	w.Add(inst)
	if dest != "" {
		if err := w.Move(inst.ID, dest); err != nil {
			return nil, err
		}
	}
	return inst, nil
}

type Scope int

const (
	ScopeRoom Scope = iota
	ScopeInventory
	ScopeEquipment
	ScopeHeldAndRoom
	ScopeSelf
)

func (w *World) Match(actor *Entity, token string, scopes ...Scope) (*Entity, []*Entity) {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return nil, nil
	}
	var cands []*Entity
	seen := map[ID]bool{}
	add := func(e *Entity) {
		if e == nil || seen[e.ID] {
			return
		}
		if actor != nil && e.ID == actor.ID && token != "me" && token != "self" {
			if !e.HasKeyword(token) {
				return
			}
		}
		if e.HasKeyword(token) {
			seen[e.ID] = true
			cands = append(cands, e)
		}
	}
	if actor != nil && (token == "me" || token == "self") {
		return actor, []*Entity{actor}
	}
	room := w.RoomOf(actor)
	for _, s := range scopes {
		switch s {
		case ScopeSelf:
			add(actor)
		case ScopeInventory:
			if actor != nil {
				for _, c := range w.Children(actor.ID) {
					add(c)
					if c.Container && !c.Closed {
						for _, inner := range w.Children(c.ID) {
							add(inner)
						}
					}
				}
			}
		case ScopeEquipment:
			if actor != nil {
				for _, id := range actor.Equipment {
					add(w.Get(id))
				}
			}
		case ScopeRoom:
			if room != nil {
				for _, c := range w.Children(room.ID) {
					if actor != nil && c.ID == actor.ID {
						continue
					}
					add(c)
					if c.Container && !c.Closed {
						for _, inner := range w.Children(c.ID) {
							add(inner)
						}
					}
				}
			}
		case ScopeHeldAndRoom:
			// handled by combining
		}
	}
	if len(cands) == 1 {
		return cands[0], cands
	}
	if len(cands) == 0 {
		return nil, nil
	}
	return nil, cands
}

func (w *World) CarryingLight(e *Entity) bool {
	if e == nil {
		return false
	}
	if e.HasFlag("light") {
		return true
	}
	for _, c := range w.Children(e.ID) {
		if c.HasFlag("light") {
			return true
		}
	}
	for _, id := range e.Equipment {
		if it := w.Get(id); it != nil && it.HasFlag("light") {
			return true
		}
	}
	return false
}

func (w *World) RoomIsLit(room *Entity) bool {
	if room == nil {
		return true
	}
	if !room.HasFlag("dark") {
		return true
	}
	for _, c := range w.Children(room.ID) {
		if w.CarryingLight(c) {
			return true
		}
	}
	return false
}

func Opposite(dir string) string {
	switch CanonicalDir(dir) {
	case "north":
		return "south"
	case "south":
		return "north"
	case "east":
		return "west"
	case "west":
		return "east"
	case "up":
		return "down"
	case "down":
		return "up"
	case "northeast":
		return "southwest"
	case "northwest":
		return "southeast"
	case "southeast":
		return "northwest"
	case "southwest":
		return "northeast"
	case "in":
		return "out"
	case "out":
		return "in"
	default:
		return ""
	}
}

func CanonicalDir(dir string) string {
	switch strings.ToLower(dir) {
	case "n", "north":
		return "north"
	case "s", "south":
		return "south"
	case "e", "east":
		return "east"
	case "w", "west":
		return "west"
	case "u", "up":
		return "up"
	case "d", "down":
		return "down"
	case "ne", "northeast":
		return "northeast"
	case "nw", "northwest":
		return "northwest"
	case "se", "southeast":
		return "southeast"
	case "sw", "southwest":
		return "southwest"
	case "in":
		return "in"
	case "out":
		return "out"
	default:
		return strings.ToLower(dir)
	}
}

func DirDelta(dir string) (dx, dy, dz int, ok bool) {
	switch CanonicalDir(dir) {
	case "north":
		return 0, 1, 0, true
	case "south":
		return 0, -1, 0, true
	case "east":
		return 1, 0, 0, true
	case "west":
		return -1, 0, 0, true
	case "up":
		return 0, 0, 1, true
	case "down":
		return 0, 0, -1, true
	case "northeast":
		return 1, 1, 0, true
	case "northwest":
		return -1, 1, 0, true
	case "southeast":
		return 1, -1, 0, true
	case "southwest":
		return -1, -1, 0, true
	default:
		return 0, 0, 0, false
	}
}

func IsDir(s string) bool {
	c := CanonicalDir(s)
	switch c {
	case "north", "south", "east", "west", "up", "down",
		"northeast", "northwest", "southeast", "southwest", "in", "out":
		return c == strings.ToLower(s) || CanonicalDir(s) != s || len(s) <= 2
	}
	// still accept if canonical maps known
	_, _, _, ok := DirDelta(c)
	if ok {
		return true
	}
	return c == "in" || c == "out"
}

func Slug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "room"
	}
	return s
}
