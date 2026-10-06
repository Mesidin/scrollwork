package script

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"sudengine/internal/world"

	lua "github.com/yuin/gopher-lua"
)

type EchoFunc func(actor *world.Entity, text string)
type EchoRoomFunc func(room *world.Entity, text string, skip *world.Entity)
type MoveFunc func(ent, dest *world.Entity) error
type DamageFunc func(att, def *world.Entity, amount int)

type Host struct {
	mu       sync.Mutex
	files    map[string]string
	World    *world.World
	Echo     EchoFunc
	EchoRoom EchoRoomFunc
	Move     MoveFunc
	Damage   DamageFunc
	Now      func() int64
}

func New(files map[string]string) *Host {
	if files == nil {
		files = map[string]string{}
	}
	return &Host{files: files}
}

func (h *Host) Reload(files map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.files = files
}

func (h *Host) Call(hook string, actor, self *world.Entity, extra ...string) (bool, error) {
	if self == nil || self.Scripts == "" {
		return false, nil
	}
	h.mu.Lock()
	src, ok := h.lookupLocked(self.Scripts)
	h.mu.Unlock()
	if !ok {
		return false, fmt.Errorf("script %q not found", self.Scripts)
	}
	return h.run(src, hook, actor, self, extra...)
}

func (h *Host) CallFile(file, hook string, actor, self *world.Entity, extra ...string) (bool, error) {
	h.mu.Lock()
	src, ok := h.lookupLocked(file)
	h.mu.Unlock()
	if !ok {
		return false, fmt.Errorf("script %q not found", file)
	}
	return h.run(src, hook, actor, self, extra...)
}

func (h *Host) lookupLocked(name string) (string, bool) {
	name = strings.TrimPrefix(name, "scripts/")
	if src, ok := h.files[name]; ok {
		return src, true
	}
	if src, ok := h.files[name+".lua"]; ok {
		return src, true
	}
	return "", false
}

func (h *Host) run(src, hook string, actor, self *world.Entity, extra ...string) (bool, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	openSafe(L)
	h.bind(L, actor, self)

	if err := L.DoString(src); err != nil {
		return false, err
	}
	fn := L.GetGlobal(hook)
	if fn == lua.LNil {
		return false, nil
	}
	L.Push(fn)
	n := 2
	L.Push(entityTable(L, actor))
	L.Push(entityTable(L, self))
	for _, a := range extra {
		L.Push(lua.LString(a))
		n++
	}
	if err := L.PCall(n, lua.MultRet, nil); err != nil {
		return true, err
	}
	if L.GetTop() >= 1 {
		return L.ToBool(-1), nil
	}
	return false, nil
}

func openSafe(L *lua.LState) {
	for _, pair := range []struct {
		n string
		f lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(pair.f))
		L.Push(lua.LString(pair.n))
		L.Call(1, 0)
	}
	for _, name := range []string{"dofile", "loadfile", "load", "loadstring", "collectgarbage"} {
		L.SetGlobal(name, lua.LNil)
	}
}

func (h *Host) bind(L *lua.LState, actor, self *world.Entity) {
	L.SetGlobal("echo", L.NewFunction(func(L *lua.LState) int {
		msg := L.ToString(1)
		who := actor
		if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
			if id, ok := tableID(L.Get(2)); ok {
				who = h.World.Get(world.ID(id))
			}
		}
		if h.Echo != nil {
			h.Echo(who, msg)
		}
		return 0
	}))
	L.SetGlobal("echo_room", L.NewFunction(func(L *lua.LState) int {
		msg := L.ToString(1)
		room := h.World.RoomOf(self)
		if h.EchoRoom != nil && room != nil {
			h.EchoRoom(room, msg, nil)
		}
		return 0
	}))
	L.SetGlobal("damage", L.NewFunction(func(L *lua.LState) int {
		n := L.ToInt(1)
		def := self
		att := actor
		if L.GetTop() >= 2 {
			if id, ok := tableID(L.Get(2)); ok {
				def = h.World.Get(world.ID(id))
			}
		}
		if h.Damage != nil && def != nil {
			h.Damage(att, def, n)
		}
		return 0
	}))
	L.SetGlobal("print", L.GetGlobal("echo"))
	L.SetGlobal("set_flag", L.NewFunction(func(L *lua.LState) int {
		h.setFlag(L, self)
		return 0
	}))
	L.SetGlobal("learn_recipe", L.NewFunction(func(L *lua.LState) int {
		recID := L.ToString(1)
		target := actor
		if L.GetTop() >= 2 {
			if id, ok := tableID(L.Get(2)); ok {
				target = h.World.Get(world.ID(id))
			}
		}
		if target != nil && recID != "" {
			target.LearnRecipe(recID)
		}
		return 0
	}))
	L.SetGlobal("teleport", L.NewFunction(func(L *lua.LState) int {
		return h.luaMove(L, actor, self)
	}))
	L.SetGlobal("move", L.GetGlobal("teleport"))
	L.SetGlobal("is_quest_completed", L.NewFunction(func(L *lua.LState) int {
		return h.luaQuestStatus(L, actor, "completed")
	}))
	L.SetGlobal("is_quest_active", L.NewFunction(func(L *lua.LState) int {
		return h.luaQuestStatus(L, actor, "active")
	}))
	L.SetGlobal("has_quest", L.NewFunction(func(L *lua.LState) int {
		return h.luaHasQuest(L, actor)
	}))
}

func (h *Host) luaMove(L *lua.LState, actor, self *world.Entity) int {
	fail := func() int {
		L.Push(lua.LFalse)
		return 1
	}
	top := L.GetTop()
	if top < 1 || h.World == nil {
		return fail()
	}
	prefer := h.preferRoom(actor, self)
	var target *world.Entity
	var dest *world.Entity
	updateHome := false

	if top == 1 {
		target = self
		dest = h.luaRoom(L.Get(1))
	} else {
		target = h.luaBeing(L.Get(1), prefer)
		dest = h.luaRoom(L.Get(2))
		if top >= 3 {
			updateHome = L.ToBool(3)
		}
	}

	if target == nil || dest == nil || target.Kind == world.KindRoom || dest.Kind != world.KindRoom {
		return fail()
	}

	var err error
	if h.Move != nil {
		err = h.Move(target, dest)
	} else {
		err = h.World.Move(target.ID, dest.ID)
	}
	if err != nil {
		return fail()
	}
	if updateHome {
		world.RememberHome(target, dest)
	}
	L.Push(lua.LTrue)
	return 1
}

func (h *Host) luaQuestStatus(L *lua.LState, actor *world.Entity, expectedStatus string) int {
	target, qID := h.questWho(L, actor)
	if target == nil || target.Quests == nil || qID == "" {
		L.Push(lua.LFalse)
		return 1
	}
	st := target.Quests[qID]
	L.Push(lua.LBool(st != nil && st.Status == expectedStatus))
	return 1
}

func (h *Host) luaHasQuest(L *lua.LState, actor *world.Entity) int {
	target, qID := h.questWho(L, actor)
	if target == nil || target.Quests == nil || qID == "" {
		L.Push(lua.LFalse)
		return 1
	}
	st := target.Quests[qID]
	ok := st != nil && (st.Status == "active" || st.Status == "completed")
	L.Push(lua.LBool(ok))
	return 1
}

func (h *Host) questWho(L *lua.LState, actor *world.Entity) (*world.Entity, string) {
	qID := L.ToString(1)
	target := actor
	if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
		target = h.luaBeing(L.Get(2), h.preferRoom(actor, nil))
	}
	return target, qID
}

func (h *Host) preferRoom(actor, self *world.Entity) world.ID {
	if h.World == nil {
		return ""
	}
	if room := h.World.RoomOf(actor); room != nil {
		return room.ID
	}
	if room := h.World.RoomOf(self); room != nil {
		return room.ID
	}
	return ""
}

func (h *Host) luaRoom(v lua.LValue) *world.Entity {
	if h.World == nil {
		return nil
	}
	if id, ok := tableID(v); ok {
		e := h.World.Get(world.ID(id))
		if e != nil && e.Kind == world.KindRoom {
			return e
		}
		return nil
	}
	s, ok := v.(lua.LString)
	if !ok {
		return nil
	}
	room, err := h.World.ResolveRoom(string(s))
	if err != nil {
		return nil
	}
	return room
}

func (h *Host) luaBeing(v lua.LValue, prefer world.ID) *world.Entity {
	if h.World == nil {
		return nil
	}
	if id, ok := tableID(v); ok {
		e := h.World.Get(world.ID(id))
		if e != nil && e.Kind != world.KindRoom {
			return e
		}
		return nil
	}
	s, ok := v.(lua.LString)
	if !ok {
		return nil
	}
	return h.lookup(string(s), prefer)
}

func (h *Host) setFlag(L *lua.LState, self *world.Entity) {
	top := L.GetTop()
	if top < 1 || h.World == nil {
		return
	}
	target := self
	name := L.ToString(1)
	on := true
	switch top {
	case 1:
		// set_flag("hostile")
	case 2:
		if L.Get(2).Type() == lua.LTBool {
			on = L.ToBool(2)
		} else {
			target = h.lookup(L.ToString(1), h.preferRoom(self, nil))
			name = L.ToString(2)
		}
	default:
		target = h.lookup(L.ToString(1), h.preferRoom(self, nil))
		name = L.ToString(2)
		on = L.ToBool(3)
	}
	if target == nil || name == "" {
		return
	}
	target.SetFlag(name, on)
}

func (h *Host) lookup(token string, prefer world.ID) *world.Entity {
	if h.World == nil || strings.TrimSpace(token) == "" {
		return nil
	}
	if e := h.World.Get(world.ID(token)); e != nil {
		return e
	}
	if mob, err := h.World.ResolveMobile(token, prefer); err == nil {
		return mob
	} else if errors.Is(err, world.ErrAmbiguous) {
		return nil
	}
	return h.findEntity(token)
}

func (h *Host) findEntity(token string) *world.Entity {
	if h.World == nil || strings.TrimSpace(token) == "" {
		return nil
	}
	if e := h.World.Get(world.ID(token)); e != nil {
		return e
	}
	var hit *world.Entity
	for _, e := range h.World.Entities {
		if e.HasKeyword(token) {
			hit = e
			if e.Kind == world.KindMobile {
				return e
			}
		}
	}
	return hit
}

func entityTable(L *lua.LState, e *world.Entity) lua.LValue {
	if e == nil {
		return lua.LNil
	}
	t := L.NewTable()
	t.RawSetString("id", lua.LString(e.ID))
	t.RawSetString("name", lua.LString(e.Name))
	t.RawSetString("kind", lua.LString(e.Kind))
	return t
}

func tableID(v lua.LValue) (string, bool) {
	tb, ok := v.(*lua.LTable)
	if !ok {
		return "", false
	}
	id := tb.RawGetString("id")
	s, ok := id.(lua.LString)
	return string(s), ok
}

func SandboxForbidden(src string) error {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	openSafe(L)
	return L.DoString(src)
}
