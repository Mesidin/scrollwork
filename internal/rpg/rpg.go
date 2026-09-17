package rpg

import "sudengine/internal/world"

type AttrDef struct {
	Key   string `yaml:"key"`
	Label string `yaml:"label"`
}

type ResDef struct {
	Key        string `yaml:"key"`
	Label      string `yaml:"label"`
	Max        int    `yaml:"max"`
	Regen      int    `yaml:"regen"`
	RegenEvery int    `yaml:"regen_every"`
}

type SkillDef struct {
	Key   string `yaml:"key"`
	Label string `yaml:"label"`
}

type Bundle struct {
	ID          string         `yaml:"id"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Attrs       map[string]int `yaml:"attrs"`
	Resources   map[string]int `yaml:"resources"`
	Skills      map[string]int `yaml:"skills"`
	Flags       []string       `yaml:"flags"`
	Abilities   []string       `yaml:"abilities"`
}

type Effect struct {
	Heal      *ResDice `yaml:"heal"`
	Damage    *ResDice `yaml:"damage"`
	SetFlag   string   `yaml:"set_flag"`
	ClearFlag string   `yaml:"clear_flag"`
}

type ResDice struct {
	Resource string `yaml:"resource"`
	Dice     string `yaml:"dice"`
}

type Ability struct {
	ID       string            `yaml:"id"`
	Name     string            `yaml:"name"`
	Verb     string            `yaml:"verb"`
	Cost     map[string]int    `yaml:"cost"`
	Cooldown int               `yaml:"cooldown"`
	Target   string            `yaml:"target"` // self, room, any
	Effects  []Effect          `yaml:"effects"`
	Messages map[string]string `yaml:"messages"`
	Scripts  string            `yaml:"scripts"`
}

type Chargen struct {
	Enabled bool `yaml:"enabled"`
	AskName bool `yaml:"ask_name"`
}

type Schema struct {
	Attributes      []AttrDef  `yaml:"attributes"`
	Resources       []ResDef   `yaml:"resources"`
	Skills          []SkillDef `yaml:"skills"`
	Races           []Bundle   `yaml:"races"`
	Roles           []Bundle   `yaml:"roles"`
	Slots           []string   `yaml:"slots"`
	PrimaryResource string     `yaml:"primary_resource"`
	Chargen         Chargen    `yaml:"chargen"`
	Abilities       []Ability  `yaml:"-"`
}

func (s Schema) ChargenEnabled() bool {
	if s.Chargen.Enabled {
		return true
	}
	return len(s.Races) > 0 || len(s.Roles) > 0
}

func (s Schema) Bundle(list []Bundle, id string) *Bundle {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

func (s Schema) Primary() string {
	if s.PrimaryResource != "" {
		return s.PrimaryResource
	}
	if len(s.Resources) > 0 {
		return s.Resources[0].Key
	}
	return "hp"
}

func (s Schema) Label(key string, lexicon map[string]string) string {
	if lexicon != nil {
		if l, ok := lexicon[key]; ok && l != "" {
			return l
		}
	}
	for _, a := range s.Attributes {
		if a.Key == key && a.Label != "" {
			return a.Label
		}
	}
	for _, r := range s.Resources {
		if r.Key == key && r.Label != "" {
			return r.Label
		}
	}
	for _, sk := range s.Skills {
		if sk.Key == key && sk.Label != "" {
			return sk.Label
		}
	}
	return key
}

func ApplyDefaults(e *world.Entity, s Schema) {
	if e.Attrs == nil {
		e.Attrs = map[string]int{}
	}
	if e.Resources == nil {
		e.Resources = map[string]world.Resource{}
	}
	if e.Skills == nil {
		e.Skills = map[string]int{}
	}
	for _, a := range s.Attributes {
		if _, ok := e.Attrs[a.Key]; !ok {
			e.Attrs[a.Key] = 0
		}
	}
	for _, r := range s.Resources {
		if _, ok := e.Resources[r.Key]; !ok {
			e.Resources[r.Key] = world.Resource{Current: r.Max, Max: r.Max}
		} else {
			cur := e.Resources[r.Key]
			if cur.Max == 0 {
				cur.Max = r.Max
			}
			if cur.Current == 0 && cur.Max > 0 {
				cur.Current = cur.Max
			}
			e.Resources[r.Key] = cur
		}
	}
}

func ApplyBundle(e *world.Entity, b Bundle) {
	if e.Attrs == nil {
		e.Attrs = map[string]int{}
	}
	if e.Resources == nil {
		e.Resources = map[string]world.Resource{}
	}
	if e.Skills == nil {
		e.Skills = map[string]int{}
	}
	for k, v := range b.Attrs {
		e.Attrs[k] += v
	}
	for k, v := range b.Resources {
		r := e.Resources[k]
		r.Max = v
		r.Current = v
		e.Resources[k] = r
	}
	for k, v := range b.Skills {
		e.Skills[k] += v
	}
	for _, f := range b.Flags {
		e.SetFlag(f, true)
	}
}

func (s Schema) AbilityByVerb(verb string) *Ability {
	for i := range s.Abilities {
		a := &s.Abilities[i]
		if a.Verb == verb || a.ID == verb {
			return a
		}
	}
	return nil
}

func (s Schema) AbilityByID(id string) *Ability {
	for i := range s.Abilities {
		if s.Abilities[i].ID == id {
			return &s.Abilities[i]
		}
	}
	return nil
}
