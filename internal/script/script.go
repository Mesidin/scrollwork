package script

import (
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
			target = h.findEntity(L.ToString(1))
			name = L.ToString(2)
		}
	default:
		target = h.findEntity(L.ToString(1))
		name = L.ToString(2)
		on = L.ToBool(3)
	}
	if target == nil || name == "" {
		return
	}
	target.SetFlag(name, on)
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
