package economy

import (
	"fmt"
	"math/rand"
	"strings"

	"sudengine/internal/rpg"
	"sudengine/internal/world"

	"gopkg.in/yaml.v3"
)

// Ingredient specifies a required material item and quantity.
type Ingredient struct {
	Item  string `json:"item" yaml:"item"`   // Keyword or prototype ID
	Count int    `json:"count" yaml:"count"` // Quantity required (default 1)
}

// Recipe defines requirements and outputs for crafting an item.
type Recipe struct {
	ID         string       `json:"id" yaml:"id"`
	Name       string       `json:"name" yaml:"name"`
	Station    string       `json:"station,omitempty" yaml:"station,omitempty"` // Keyword or flag required in room (e.g. anvil, forge)
	Tools      []string     `json:"tools,omitempty" yaml:"tools,omitempty"`     // Tool keywords required carried or equipped
	Requires   []Ingredient `json:"requires" yaml:"requires"`
	Produces   string       `json:"produces" yaml:"produces"` // Prototype ID of crafted item
	Count      int          `json:"count,omitempty" yaml:"count,omitempty"`
	Skill      string       `json:"skill,omitempty" yaml:"skill,omitempty"`
	Difficulty int          `json:"difficulty,omitempty" yaml:"difficulty,omitempty"`
	MinSkill   int          `json:"min_skill,omitempty" yaml:"min_skill,omitempty"`
	Discovered *bool        `json:"discovered,omitempty" yaml:"discovered,omitempty"`
}

// IsKnown reports whether the actor knows the recipe.
// If Discovered is nil or true, it is known by default.
// If Discovered is explicitly false, actor must have learned it.
func (r Recipe) IsKnown(actor *world.Entity) bool {
	if r.Discovered == nil || *r.Discovered {
		return true
	}
	if actor == nil {
		return false
	}
	return actor.KnowsRecipe(r.ID)
}

// UnmarshalYAML accepts produces as an item id or as {item, count}.
func (r *Recipe) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		ID         string       `yaml:"id"`
		Name       string       `yaml:"name"`
		Station    string       `yaml:"station"`
		Tools      []string     `yaml:"tools"`
		Requires   []Ingredient `yaml:"requires"`
		Produces   yaml.Node    `yaml:"produces"`
		Count      int          `yaml:"count"`
		Skill      string       `yaml:"skill"`
		Difficulty int          `yaml:"difficulty"`
		MinSkill   int          `yaml:"min_skill"`
		Discovered *bool        `yaml:"discovered"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*r = Recipe{
		ID: raw.ID, Name: raw.Name, Station: raw.Station, Tools: raw.Tools,
		Requires: raw.Requires, Count: raw.Count, Skill: raw.Skill, Difficulty: raw.Difficulty,
		MinSkill: raw.MinSkill, Discovered: raw.Discovered,
	}
	if raw.Produces.Kind == 0 {
		return nil
	}
	if raw.Produces.Kind == yaml.ScalarNode {
		r.Produces = raw.Produces.Value
		return nil
	}
	var prod struct {
		Item  string `yaml:"item"`
		Count int    `yaml:"count"`
	}
	if err := raw.Produces.Decode(&prod); err != nil {
		return err
	}
	r.Produces = prod.Item
	if r.Count <= 0 && prod.Count > 0 {
		r.Count = prod.Count
	}
	return nil
}

// CheckStation verifies that the room contains the required crafting station.
func CheckStation(w *world.World, room *world.Entity, station string) (*world.Entity, bool) {
	if station == "" {
		return nil, true
	}
	if room == nil {
		return nil, false
	}
	station = strings.ToLower(station)
	if room.HasFlag(station) {
		return room, true
	}
	for _, id := range room.Contents {
		e := w.Get(id)
		if e != nil && (e.HasKeyword(station) || e.HasFlag(station)) {
			return e, true
		}
	}
	return nil, false
}

// CheckTools verifies that the actor carries or equips all required tools.
func CheckTools(w *world.World, actor *world.Entity, tools []string) (missing string, ok bool) {
	if len(tools) == 0 {
		return "", true
	}
	carried := w.Children(actor.ID)
	hasTool := func(tool string) bool {
		tool = strings.ToLower(tool)
		for _, it := range carried {
			if it.HasKeyword(tool) {
				return true
			}
		}
		if actor.Equipment != nil {
			for _, eqID := range actor.Equipment {
				if eq := w.Get(eqID); eq != nil && eq.HasKeyword(tool) {
					return true
				}
			}
		}
		return false
	}
	for _, t := range tools {
		if !hasTool(t) {
			return t, false
		}
	}
	return "", true
}

// FindIngredients locates the required entities in the actor's inventory.
func FindIngredients(w *world.World, actor *world.Entity, requires []Ingredient) (toConsume []*world.Entity, err error) {
	carried := w.Children(actor.ID)
	used := map[world.ID]bool{}
	worn := map[world.ID]bool{}
	if actor != nil && actor.Equipment != nil {
		for _, id := range actor.Equipment {
			worn[id] = true
		}
	}

	for _, req := range requires {
		needed := req.Count
		if needed <= 0 {
			needed = 1
		}
		var found []*world.Entity
		target := strings.ToLower(req.Item)
		for _, it := range carried {
			if used[it.ID] || worn[it.ID] {
				continue
			}
			if strings.EqualFold(string(it.PrototypeID), target) || it.HasKeyword(target) {
				found = append(found, it)
				used[it.ID] = true
				if len(found) == needed {
					break
				}
			}
		}
		if len(found) < needed {
			return nil, fmt.Errorf("you need %d %s (have %d)", needed, req.Item, len(found))
		}
		toConsume = append(toConsume, found...)
	}
	return toConsume, nil
}

// Craft executes a recipe for an actor, consuming materials and spawning the product.
func Craft(w *world.World, actor *world.Entity, r Recipe, rng *rand.Rand, rules *rpg.Schema) (*world.Entity, error) {
	room := w.RoomOf(actor)
	if room == nil {
		return nil, fmt.Errorf("you are nowhere")
	}

	if !r.IsKnown(actor) {
		return nil, fmt.Errorf("you haven't learned how to craft that yet")
	}

	if r.MinSkill > 0 && r.Skill != "" {
		rank := 0
		if actor.Skills != nil {
			rank = actor.Skills[r.Skill]
		}
		if rank < r.MinSkill {
			return nil, fmt.Errorf("you need at least rank %d in %s to craft that (you have %d)", r.MinSkill, r.Skill, rank)
		}
	}

	if r.Station != "" {
		if _, ok := CheckStation(w, room, r.Station); !ok {
			return nil, fmt.Errorf("you need a %s nearby to craft that", r.Station)
		}
	}

	if missing, ok := CheckTools(w, actor, r.Tools); !ok {
		return nil, fmt.Errorf("you need a %s to work", missing)
	}

	toConsume, err := FindIngredients(w, actor, r.Requires)
	if err != nil {
		return nil, err
	}

	superior := false
	if r.Skill != "" {
		if rules == nil {
			return nil, fmt.Errorf("this recipe needs a %s check, and this game has no such check", r.Skill)
		}
		res, ok := rules.Resolve(rng, w, actor, r.Skill, r.Difficulty, r.Difficulty > 0)
		if !ok {
			return nil, fmt.Errorf("this recipe needs a %s check, and this game has no such check", r.Skill)
		}
		if !res.Success {
			if len(toConsume) > 0 {
				roll := rng
				if roll == nil {
					roll = rand.New(rand.NewSource(1))
				}
				w.Destroy(toConsume[roll.Intn(len(toConsume))].ID)
			}
			return nil, fmt.Errorf("you fumble the crafting process and ruin some material")
		}
		if res.Total >= res.Target+3 {
			superior = true
		}
	}

	if strings.TrimSpace(r.Produces) == "" || w.Protos[world.ID(r.Produces)] == nil {
		return nil, fmt.Errorf("there is no pattern for that")
	}

	for _, it := range toConsume {
		w.Destroy(it.ID)
	}

	count := r.Count
	if count <= 0 {
		count = 1
	}

	protoID := world.ID(r.Produces)
	var first *world.Entity
	for i := 0; i < count; i++ {
		crafted, err := w.Spawn(protoID, actor.ID)
		if err != nil {
			return nil, err
		}
		if superior && crafted.MaxDurability > 0 {
			bonus := crafted.MaxDurability / 5
			if bonus < 5 {
				bonus = 5
			}
			crafted.MaxDurability += bonus
			crafted.Durability = crafted.MaxDurability
			crafted.SetFlag("superior", true)
		}
		if first == nil {
			first = crafted
		}
	}

	return first, nil
}
