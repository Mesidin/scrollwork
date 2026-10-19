package combat

import (
	"fmt"
	"math/rand"
	"strings"

	"sudengine/internal/dice"
	"sudengine/internal/economy"
	"sudengine/internal/rpg"
	"sudengine/internal/world"
)

// Hear chooses who receives one combat line.
// HearPrivate is only `to`. HearRoom is every player in `to`'s room.
// HearOthers is that room except `to` and alsoExcept.
type Hear int

const (
	HearPrivate Hear = iota
	HearRoom
	HearOthers
)

type Emitter func(to *world.Entity, channel, text string, hear Hear, alsoExcept ...*world.Entity)

type Engine struct {
	RNG     *rand.Rand
	Primary string
	Emit    Emitter
	Every   int
	Rules   *rpg.Schema
	// HitMod is a positive penalty added to outdoor hit checks. Nil adds nothing.
	HitMod func(att *world.Entity) int
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

// CheckAssists recruits allies in the leader's room.
// leaderAttacking is set when the leader opened the fight. Defend joins only
// when the leader is the one under attack, which is not visible from the
// target's combat field once that target has been pointed at the leader.
func (e *Engine) CheckAssists(w *world.World, leader, target *world.Entity, now int64, leaderAttacking bool) {
	if e == nil || w == nil || leader == nil || target == nil {
		return
	}
	room := w.RoomOf(leader)
	if room == nil {
		return
	}
	for _, kidID := range room.Contents {
		kid := w.Get(kidID)
		if kid == nil || kid.ID == leader.ID || kid.ID == target.ID {
			continue
		}
		if kid.InCombat() || strings.EqualFold(kid.Stance, "stay") || strings.EqualFold(kid.Stance, "passive") {
			continue
		}
		if kid.MaxLoyalty > 0 && kid.Loyalty <= 0 {
			continue
		}
		isAlly := kid.Leader == leader.ID || kid.Owner == leader.ID ||
			(leader.PartyLeader != "" && kid.PartyLeader == leader.PartyLeader)
		if !isAlly {
			continue
		}
		stance := strings.ToLower(kid.Stance)
		if stance == "" {
			stance = "assist"
		}
		if stance == "assist" || (stance == "defend" && !leaderAttacking) {
			e.ensure(kid)
			kid.Combat.Target = target.ID
			if kid.Combat.NextSwing == 0 {
				kid.Combat.NextSwing = now + 1
			}
			if e.Emit != nil {
				e.Emit(leader, "combat", fmt.Sprintf("%s rushes to your aid against %s!", kid.Display(), target.Display()), HearPrivate)
			}
		}
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

func (e *Engine) hitPenalty(att *world.Entity) int {
	if e == nil || e.HitMod == nil || att == nil {
		return 0
	}
	return e.HitMod(att)
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
	atk := attackOf(w, att, e)
	var hit *rpg.Result
	rules := e.Rules
	if rules != nil {
		if _, ok := rules.Checks["hit"]; ok {
			opp, has := 0, false
			if def.Oppose != nil {
				if n, ok := def.Oppose["hit"]; ok {
					opp, has = n, true
				}
			}
			res, ok := rules.ResolveHarder(e.RNG, w, att, "hit", opp, has, e.hitPenalty(att))
			if ok && !res.Success {
				e.report(w, att, def, atk, 0, &res, true)
				return
			}
			if ok {
				hit = &res
			}
		}
	}
	dmg := dice.MustRoll(e.RNG, atk.Damage)
	if rules != nil {
		dmg += rules.DamageBonus(att, atk.Skill)
	}
	soak := armorSoak(w, def)
	dmg -= soak
	if dmg < 1 {
		dmg = 1
	}
	def.AdjustRes(primary, -dmg)

	if att.Equipment != nil {
		if id, ok := att.Equipment["wield"]; ok {
			if wep := w.Get(id); wep != nil && wep.MaxDurability > 0 {
				if economy.Degrade(wep, 1) && e.Emit != nil && att.Kind == world.KindPlayer {
					e.Emit(att, "alert", fmt.Sprintf("Your %s breaks!", gearName(wep)), HearPrivate)
				}
			}
		}
	}
	if def.Equipment != nil {
		for slot, id := range def.Equipment {
			if slot == "wield" {
				continue
			}
			if arm := w.Get(id); arm != nil && arm.Armor != nil && arm.MaxDurability > 0 {
				if economy.Degrade(arm, 1) && e.Emit != nil && def.Kind == world.KindPlayer {
					e.Emit(def, "alert", fmt.Sprintf("Your %s breaks!", gearName(arm)), HearPrivate)
				}
			}
		}
	}

	e.report(w, att, def, atk, dmg, hit, false)
	if !def.Alive(primary) {
		if att != nil {
			def.SlainBy = att.ID
		}
		e.Stop(att)
		e.Stop(def)
		if e.Emit != nil {
			if def.Kind == world.KindPlayer {
				e.Emit(def, "alert", "You collapse.", HearPrivate)
			} else if att.Kind == world.KindPlayer {
				e.Emit(att, "narrative", fmt.Sprintf("%s dies.", def.CapDisplay()), HearRoom)
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
		e.Emit(att, "narrative", fmt.Sprintf("You %s %s for %d.", verb, def.Display(), amount), HearPrivate)
		roomLine := fmt.Sprintf("%s %s %s for %d.", att.CapDisplay(), ThirdPerson(verb), def.Display(), amount)
		if def.Kind == world.KindPlayer {
			e.Emit(def, "combat", fmt.Sprintf("%s %s you for %d.", att.CapDisplay(), ThirdPerson(verb), amount), HearPrivate)
			e.Emit(att, "narrative", roomLine, HearOthers, def)
		} else {
			e.Emit(att, "narrative", roomLine, HearOthers)
		}
	}
	if !def.Alive(primary) {
		if att != nil {
			def.SlainBy = att.ID
		}
		e.Stop(att)
		e.Stop(def)
	}
}

// report gives the fighter the second-person line and everyone else a third-person line.
// The fighter does not also hear the room line. Mob versus mob is one room line.
func (e *Engine) report(w *world.World, att, def *world.Entity, atk world.Attack, dmg int, hit *rpg.Result, miss bool) {
	if e.Emit == nil || att == nil || def == nil {
		return
	}
	label := e.Primary
	if e.Rules != nil {
		label = e.Rules.Label(e.Primary, nil)
	}
	playerAtt := att.Kind == world.KindPlayer
	playerDef := def.Kind == world.KindPlayer
	you, youToken, youTmpl := swingYou(att, def, atk, dmg, label, miss)
	room, roomToken, roomTmpl := swingRoom(att, def, atk, dmg, label, miss)
	if playerAtt || playerDef {
		if youTmpl && e.Rules != nil {
			you = e.Rules.Decorate(you, dmg, youToken, label, hit)
		}
		if roomTmpl && e.Rules != nil {
			room = e.Rules.Decorate(room, dmg, roomToken, label, hit)
		}
	}
	switch {
	case playerAtt && playerDef:
		e.Emit(att, "narrative", you, HearPrivate)
		ch := "combat"
		if miss {
			ch = "narrative"
		}
		e.Emit(def, ch, plainTarget(att, def, atk, dmg, miss), HearPrivate)
		e.Emit(att, "narrative", room, HearOthers, def)
	case playerAtt:
		e.Emit(att, "narrative", you, HearPrivate)
		e.Emit(att, "narrative", room, HearOthers)
	case playerDef:
		ch := "combat"
		if miss {
			ch = "narrative"
		}
		e.Emit(def, ch, you, HearPrivate)
		e.Emit(def, ch, room, HearOthers)
	default:
		e.Emit(att, "narrative", room, HearRoom)
	}
}

func swingYou(att, def *world.Entity, atk world.Attack, dmg int, label string, miss bool) (string, bool, bool) {
	tmpl := atk.Hit
	if miss {
		tmpl = atk.Miss
	}
	if tmpl != "" {
		line, token := rpg.Fill(tmpl, def.Display(), att.CapDisplay(), label, dmg)
		return line, token, true
	}
	verb := atk.Name
	if verb == "" {
		verb = "hit"
	}
	if miss {
		if att.Kind == world.KindPlayer {
			return fmt.Sprintf("You miss %s.", def.Display()), false, false
		}
		return fmt.Sprintf("%s misses you.", att.CapDisplay()), false, false
	}
	if att.Kind == world.KindPlayer {
		return fmt.Sprintf("You %s %s for %d.", verb, def.Display(), dmg), false, false
	}
	if def.Kind == world.KindPlayer {
		return fmt.Sprintf("%s %s you for %d.", att.CapDisplay(), ThirdPerson(verb), dmg), false, false
	}
	return fmt.Sprintf("%s %s %s.", att.CapDisplay(), ThirdPerson(verb), def.Display()), false, false
}

func swingRoom(att, def *world.Entity, atk world.Attack, dmg int, label string, miss bool) (string, bool, bool) {
	tmpl := atk.HitRoom
	if miss {
		tmpl = atk.MissRoom
	}
	if tmpl != "" {
		line, token := rpg.Fill(tmpl, def.Display(), att.CapDisplay(), label, dmg)
		return line, token, true
	}
	verb := atk.Name
	if verb == "" {
		verb = "hit"
	}
	if miss {
		return fmt.Sprintf("%s misses %s.", att.CapDisplay(), def.Display()), false, false
	}
	return fmt.Sprintf("%s %s %s for %d.", att.CapDisplay(), ThirdPerson(verb), def.Display(), dmg), false, false
}

func plainTarget(att, def *world.Entity, atk world.Attack, dmg int, miss bool) string {
	if miss {
		return fmt.Sprintf("%s misses you.", att.CapDisplay())
	}
	verb := atk.Name
	if verb == "" {
		verb = "hit"
	}
	return fmt.Sprintf("%s %s you for %d.", att.CapDisplay(), ThirdPerson(verb), dmg)
}

func attackOf(w *world.World, att *world.Entity, e *Engine) world.Attack {
	if att.Equipment != nil {
		if id, ok := att.Equipment["wield"]; ok {
			if it := w.Get(id); it != nil && it.Weapon != nil && it.Weapon.Damage != "" {
				if it.HasFlag("broken") || (it.MaxDurability > 0 && it.Durability <= 0) {
					return world.Attack{Name: "hit", Damage: "1d2"}
				}
				verb := it.Weapon.Verb
				if verb == "" {
					verb = "hit"
				}
				return world.Attack{
					Name: verb, Damage: it.Weapon.Damage, Skill: it.Weapon.Skill,
					Hit: it.Weapon.Hit, Miss: it.Weapon.Miss,
					HitRoom: it.Weapon.HitRoom, MissRoom: it.Weapon.MissRoom,
				}
			}
		}
	}
	if att.Combat != nil && len(att.Combat.Attacks) > 0 {
		atks := att.Combat.Attacks
		if len(atks) > 1 && e.RNG != nil {
			return atks[e.RNG.Intn(len(atks))]
		}
		return atks[0]
	}
	return world.Attack{Name: "hit", Damage: "1d4"}
}

func armorSoak(w *world.World, def *world.Entity) int {
	if def.Equipment == nil {
		return 0
	}
	n := 0
	for _, id := range def.Equipment {
		if it := w.Get(id); it != nil && it.Armor != nil {
			if it.HasFlag("broken") || (it.MaxDurability > 0 && it.Durability <= 0) {
				continue
			}
			n += it.Armor.Soak
		}
	}
	return n
}

func gearName(e *world.Entity) string {
	if e == nil {
		return "gear"
	}
	if e.Name != "" {
		return e.Name
	}
	return e.Display()
}
