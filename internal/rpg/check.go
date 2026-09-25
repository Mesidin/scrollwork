package rpg

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"sudengine/internal/dice"
	"sudengine/internal/world"
)

// Check is one named roll. An omitted check means the verb does not roll.
type Check struct {
	Roll       string   `yaml:"roll"`
	Mode       string   `yaml:"mode"` // over (default) or under
	Actor      []string `yaml:"actor"`
	Gear       []string `yaml:"gear"`
	Difficulty int      `yaml:"difficulty"`
}

// Feedback controls whether the player sees the arithmetic.
type Feedback struct {
	Rolls   bool `yaml:"rolls"`
	Numbers bool `yaml:"numbers"`
}

// Advancement is off unless Enabled. A story pack leaves it out.
type Advancement struct {
	Enabled        bool           `yaml:"enabled"`
	XP             int            `yaml:"xp"`
	XPStep         int            `yaml:"xp_step"`
	SkillPoints    int            `yaml:"skill_points"`
	AttributeEvery int            `yaml:"attribute_every"`
	Resources      map[string]int `yaml:"resources"`
	TrainCost      int            `yaml:"train_cost"`
	Currency       string         `yaml:"currency"`
}

func (a Advancement) On() bool { return a.Enabled }

// Cost is the XP required to leave the given level.
// Level 1 costs XP. Later levels cost XP + (level-1)*XPStep.
func (a Advancement) Cost(level int) int {
	xp := a.XP
	if xp < 1 {
		xp = 20
	}
	if level < 1 {
		level = 1
	}
	if level == 1 || a.XPStep == 0 {
		return xp
	}
	return xp + (level-1)*a.XPStep
}

func (a Advancement) TrainPrice() int {
	if a.TrainCost < 1 {
		return 10
	}
	return a.TrainCost
}

func (a Advancement) Purse() string {
	if a.Currency == "" {
		return "coin"
	}
	return a.Currency
}

// Result is one resolved check.
type Result struct {
	Rolled  bool
	Success bool
	Natural int
	Mod     int
	Total   int
	Target  int
	Expr    string
	Mode    string
}

func (s Schema) Check(name string) (Check, bool) {
	if s.Checks == nil {
		return Check{}, false
	}
	ch, ok := s.Checks[name]
	return ch, ok
}

func (s Schema) Skill(key string) *SkillDef {
	for i := range s.Skills {
		if s.Skills[i].Key == key {
			return &s.Skills[i]
		}
	}
	return nil
}

func (s Schema) Attr(key string) *AttrDef {
	for i := range s.Attributes {
		if s.Attributes[i].Key == key {
			return &s.Attributes[i]
		}
	}
	return nil
}

// Effective is the number a check uses for a key.
// A skill's parent attribute is included once, here.
func (s Schema) Effective(e *world.Entity, ref string) int {
	if e == nil {
		return 0
	}
	kind, key := splitRef(ref)
	if kind == "" {
		if s.Skill(key) != nil {
			kind = "skill"
		} else {
			kind = "attr"
		}
	}
	switch kind {
	case "skill":
		n := 0
		if e.Skills != nil {
			n = e.Skills[key]
		}
		if sk := s.Skill(key); sk != nil && sk.Parent != "" && e.Attrs != nil {
			n += e.Attrs[sk.Parent]
		}
		return n
	default:
		if e.Attrs == nil {
			return 0
		}
		return e.Attrs[key]
	}
}

// DamageBonus is the flat damage added when an attack names a skill.
// damage_every 2 means +1 per two points of effective skill. 0 adds nothing.
func (s Schema) DamageBonus(e *world.Entity, skill string) int {
	if skill == "" || e == nil {
		return 0
	}
	sk := s.Skill(skill)
	if sk == nil || sk.DamageEvery < 1 {
		return 0
	}
	eff := s.Effective(e, "skill."+skill)
	if eff < 1 {
		return 0
	}
	return eff / sk.DamageEvery
}

// Resolve rolls a named check. hasOppose uses oppose even when it is 0.
// The bool is false when the pack did not define the check.
func (s Schema) Resolve(rng *rand.Rand, w *world.World, actor *world.Entity, name string, oppose int, hasOppose bool) (Result, bool) {
	ch, ok := s.Check(name)
	if !ok {
		return Result{}, false
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	expr := strings.TrimSpace(ch.Roll)
	if expr == "" {
		expr = "2d6"
	}
	mode := strings.ToLower(strings.TrimSpace(ch.Mode))
	if mode == "" {
		mode = "over"
	}
	mod := 0
	for _, ref := range ch.Actor {
		mod += s.Effective(actor, ref)
	}
	mod += GearBonus(w, actor, ch.Gear)
	natural := dice.MustRoll(rng, expr)
	target := ch.Difficulty
	if hasOppose {
		target = oppose
	}
	total := natural + mod
	success := total >= target
	if mode == "under" {
		// A bonus widens the roll-under ceiling.
		target += mod
		total = natural
		success = natural <= target
	}
	return Result{
		Rolled:  true,
		Success: success,
		Natural: natural,
		Mod:     mod,
		Total:   total,
		Target:  target,
		Expr:    expr,
		Mode:    mode,
	}, true
}

// GearBonus sums worn item mods for the listed keys.
func GearBonus(w *world.World, e *world.Entity, keys []string) int {
	if w == nil || e == nil || len(keys) == 0 || len(e.Equipment) == 0 {
		return 0
	}
	n := 0
	for _, id := range e.Equipment {
		it := w.Get(id)
		if it == nil {
			continue
		}
		for _, k := range keys {
			n += it.Mods[k]
		}
	}
	return n
}

// Decorate appends a roll and a damage bracket when the pack asked for them.
// damageToken means the sentence already includes the amount.
func (s Schema) Decorate(line string, damage int, damageToken bool, resLabel string, result *Result) string {
	if s.Feedback.Rolls && result != nil && result.Rolled {
		line += " " + result.Note()
	}
	if s.Feedback.Numbers && damage > 0 && !damageToken && resLabel != "" {
		line += fmt.Sprintf(" [-%d %s]", damage, resLabel)
	}
	return line
}

// Note is the parenthetical a pack shows when feedback.rolls is on.
func (r Result) Note() string {
	if r.Mode == "under" {
		return fmt.Sprintf("(%s = %d vs %d)", r.Expr, r.Natural, r.Target)
	}
	return fmt.Sprintf("(%s%+d = %d vs %d)", r.Expr, r.Mod, r.Total, r.Target)
}

// Fill substitutes {name}, {Name}, {damage}, and {resource}.
func Fill(tmpl, name, cap, resource string, damage int) (string, bool) {
	used := strings.Contains(tmpl, "{damage}")
	r := strings.NewReplacer(
		"{name}", name,
		"{Name}", cap,
		"{damage}", strconv.Itoa(damage),
		"{resource}", resource,
	)
	return r.Replace(tmpl), used
}

func splitRef(ref string) (kind, key string) {
	ref = strings.TrimSpace(ref)
	if i := strings.Index(ref, "."); i >= 0 {
		return strings.ToLower(ref[:i]), strings.TrimSpace(ref[i+1:])
	}
	return "", ref
}

// Grant adds XP and applies every level crossed. Messages describe the new level.
func (s Schema) Grant(e *world.Entity, amount int) []string {
	if e == nil || amount <= 0 || !s.Advancement.On() {
		return nil
	}
	if e.Level < 1 {
		e.Level = 1
	}
	e.XP += amount
	var out []string
	for {
		need := s.Advancement.Cost(e.Level)
		if e.XP < need {
			break
		}
		e.XP -= need
		e.Level++
		e.SkillPoints += s.Advancement.SkillPoints
		if s.Advancement.AttributeEvery > 0 && e.Level%s.Advancement.AttributeEvery == 0 {
			e.AttrPoints++
		}
		for key, n := range s.Advancement.Resources {
			r := e.Res(key)
			r.Max += n
			r.Current += n
			e.SetRes(key, r)
		}
		out = append(out, fmt.Sprintf("You reach level %d. Skill points: %d. Attribute points: %d.", e.Level, e.SkillPoints, e.AttrPoints))
	}
	return out
}
