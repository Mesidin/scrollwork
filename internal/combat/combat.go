package combat

import (
	"fmt"
	"math/rand"

	"sudengine/internal/dice"
	"sudengine/internal/world"
)

type Emitter func(to *world.Entity, channel, text string)

type Engine struct {
	RNG     *rand.Rand
	Primary string
	Emit    Emitter
	Every   int
}

func (e *Engine) Start(a, b *world.Entity, now int64) {
	e.ensure(a)
	e.ensure(b)
	a.Combat.Target = b.ID
	b.Combat.Target = a.ID
	if a.Combat.NextSwing == 0 {
		a.Combat.NextSwing = now + 1
	}
	if b.Combat.NextSwing == 0 {
		b.Combat.NextSwing = now + 1
	}
}

func (e *Engine) ensure(ent *world.Entity) {
	if ent.Combat == nil {
		ent.Combat = &world.Combat{RoundSpeed: e.Every, Attacks: []world.Attack{{Name: "hit", Damage: "1d4"}}}
	}
	if ent.Combat.RoundSpeed <= 0 {
		ent.Combat.RoundSpeed = e.Every
		if ent.Combat.RoundSpeed <= 0 {
			ent.Combat.RoundSpeed = 4
		}
	}
	if len(ent.Combat.Attacks) == 0 {
		ent.Combat.Attacks = []world.Attack{{Name: "hit", Damage: "1d4"}}
	}
}

func (e *Engine) Stop(ent *world.Entity) {
	if ent != nil && ent.Combat != nil {
		ent.Combat.Target = ""
		ent.Combat.NextSwing = 0
	}
}

func (e *Engine) Tick(w *world.World, now int64) {
	primary := e.Primary
	if primary == "" {
		primary = "hp"
	}
	ids := make([]world.ID, 0, len(w.Entities))
	for id := range w.Entities {
		ids = append(ids, id)
	}
	for _, id := range ids {
		ent := w.Get(id)
		if ent == nil || ent.Combat == nil || ent.Combat.Target == "" {
			continue
		}
		if !ent.Alive(primary) {
			continue
		}
		tgt := w.Get(ent.Combat.Target)
		if tgt == nil || !tgt.Alive(primary) || w.RoomOf(ent) != w.RoomOf(tgt) {
			e.Stop(ent)
			continue
		}
		if now < ent.Combat.NextSwing {
			continue
		}
		e.swing(w, ent, tgt)
		spd := ent.Combat.RoundSpeed
		if spd < 1 {
			spd = 4
		}
		ent.Combat.NextSwing = now + int64(spd)
	}
}

func (e *Engine) swing(w *world.World, att, def *world.Entity) {
	primary := e.Primary
	if primary == "" {
		primary = "hp"
	}
	atk := att.Combat.Attacks[0]
	if len(att.Combat.Attacks) > 1 {
		atk = att.Combat.Attacks[e.RNG.Intn(len(att.Combat.Attacks))]
	}
	dmg := dice.MustRoll(e.RNG, atk.Damage)
	if dmg < 1 {
		dmg = 1
	}
	def.AdjustRes(primary, -dmg)
	msgAtt := fmt.Sprintf("You %s %s for %d.", atk.Name, def.Display(), dmg)
	msgDef := fmt.Sprintf("%s %ss you for %d.", att.CapDisplay(), atk.Name, dmg)
	msgRoom := fmt.Sprintf("%s %ss %s.", att.CapDisplay(), atk.Name, def.Display())
	if e.Emit != nil {
		if att.Kind == world.KindPlayer {
			e.Emit(att, "narrative", msgAtt)
		} else if def.Kind == world.KindPlayer {
			e.Emit(def, "combat", msgDef)
		} else {
			if p := w.Player(); p != nil && w.RoomOf(p) == w.RoomOf(att) {
				e.Emit(p, "narrative", msgRoom)
			}
		}
	}
	if !def.Alive(primary) {
		e.Stop(att)
		e.Stop(def)
		if e.Emit != nil {
			if def.Kind == world.KindPlayer {
				e.Emit(def, "alert", "You collapse.")
			} else if att.Kind == world.KindPlayer {
				e.Emit(att, "narrative", fmt.Sprintf("%s dies.", def.CapDisplay()))
			}
		}
	}
}

func (e *Engine) Damage(w *world.World, att, def *world.Entity, amount int, verb string) {
	primary := e.Primary
	if primary == "" {
		primary = "hp"
	}
	if amount < 0 {
		amount = 0
	}
	def.AdjustRes(primary, -amount)
	if e.Emit != nil && att != nil && att.Kind == world.KindPlayer {
		e.Emit(att, "narrative", fmt.Sprintf("You %s %s for %d.", verb, def.Display(), amount))
	}
	if !def.Alive(primary) {
		e.Stop(att)
		e.Stop(def)
	}
}
