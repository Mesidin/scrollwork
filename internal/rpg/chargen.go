package rpg

import (
	"fmt"
	"strings"
)

// AcceptChargen checks a new character's origin, role, and point-buy spends.
// requireChoice is set for a saved character: each list the pack offers must
// be answered. A guest may leave them blank. A chosen id must still be known.
// A zero budget drops spends. A positive budget rejects unknown keys, negative
// spends, a total over the budget, and a total over an attribute max.
// The returned ids are the pack's canonical ids. The spend map is nil when
// nothing should be added on top of the origin and role.
func (s Schema) AcceptChargen(raceID, roleID string, attrs map[string]int, requireChoice bool) (string, string, map[string]int, error) {
	if !s.ChargenEnabled() {
		return "", "", nil, nil
	}
	race, err := canonicalBundle(s.Races, raceID, "origin", requireChoice)
	if err != nil {
		return "", "", nil, err
	}
	role, err := canonicalBundle(s.Roles, roleID, "role", requireChoice)
	if err != nil {
		return "", "", nil, err
	}
	if s.Chargen.Budget() <= 0 {
		return race, role, nil, nil
	}
	spend, err := acceptSpends(s, race, role, attrs, s.Chargen.Budget())
	if err != nil {
		return "", "", nil, err
	}
	return race, role, spend, nil
}

func canonicalBundle(list []Bundle, id, label string, required bool) (string, error) {
	id = strings.TrimSpace(id)
	if len(list) == 0 {
		if id != "" {
			return "", fmt.Errorf("unknown %s", label)
		}
		return "", nil
	}
	if id == "" {
		if required {
			return "", fmt.Errorf("choose a %s", label)
		}
		return "", nil
	}
	for _, b := range list {
		if strings.EqualFold(b.ID, id) || (b.Name != "" && strings.EqualFold(b.Name, id)) {
			return b.ID, nil
		}
	}
	return "", fmt.Errorf("unknown %s", label)
}

func acceptSpends(s Schema, raceID, roleID string, attrs map[string]int, budget int) (map[string]int, error) {
	known := make(map[string]AttrDef, len(s.Attributes))
	base := make(map[string]int, len(s.Attributes))
	for _, a := range s.Attributes {
		known[a.Key] = a
		base[a.Key] = a.Base
	}
	if b := s.Bundle(s.Races, raceID); b != nil {
		for k, v := range b.Attrs {
			base[k] += v
		}
	}
	if b := s.Bundle(s.Roles, roleID); b != nil {
		for k, v := range b.Attrs {
			base[k] += v
		}
	}
	if len(attrs) == 0 {
		return nil, nil
	}
	out := map[string]int{}
	sum := 0
	for k, v := range attrs {
		if v == 0 {
			continue
		}
		if v < 0 {
			return nil, fmt.Errorf("attribute %s cannot be negative", k)
		}
		def, ok := known[k]
		if !ok {
			return nil, fmt.Errorf("unknown attribute %s", k)
		}
		if def.Max > 0 && base[k]+v > def.Max {
			name := def.Label
			if name == "" {
				name = def.Key
			}
			return nil, fmt.Errorf("%s exceeds its maximum of %d", name, def.Max)
		}
		sum += v
		out[k] = v
	}
	if sum > budget {
		return nil, fmt.Errorf("attribute points exceed the budget of %d", budget)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
