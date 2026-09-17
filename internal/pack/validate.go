package pack

import (
	"fmt"
	"strings"
)

type ErrorList []string

func (e ErrorList) Error() string {
	if len(e) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d pack problem(s):", len(e)))
	for i, s := range e {
		b.WriteString(fmt.Sprintf("\n  %d. %s", i+1, s))
	}
	return b.String()
}

func (p *Pack) Validate() error {
	var errs ErrorList
	if p.Meta.ID == "" {
		errs = append(errs, "pack.yaml: id is empty")
	}
	if p.Meta.StartRoom == "" {
		errs = append(errs, "pack.yaml: start_room is missing")
	}
	rooms := map[string]RoomYAML{}
	for _, r := range p.Rooms {
		if r.ID == "" {
			errs = append(errs, "world/rooms.yaml: a room has no id")
			continue
		}
		if _, ok := rooms[r.ID]; ok {
			errs = append(errs, fmt.Sprintf("duplicate room id %q", r.ID))
		}
		if strings.TrimSpace(r.Name) == "" {
			errs = append(errs, fmt.Sprintf("room %s: name is empty", r.ID))
		}
		rooms[r.ID] = r
	}
	if p.Meta.StartRoom != "" {
		if _, ok := rooms[p.Meta.StartRoom]; !ok {
			errs = append(errs, fmt.Sprintf("start_room %q does not exist", p.Meta.StartRoom))
		}
	}
	items := map[string]ItemYAML{}
	for _, it := range p.Items {
		if it.ID == "" {
			errs = append(errs, "world/items.yaml: an item has no id")
			continue
		}
		if _, ok := items[it.ID]; ok {
			errs = append(errs, fmt.Sprintf("duplicate item id %q", it.ID))
		}
		items[it.ID] = it
	}
	npcs := map[string]NPCYAML{}
	for _, n := range p.NPCs {
		if n.ID == "" {
			errs = append(errs, "world/npcs.yaml: an npc has no id")
			continue
		}
		if _, ok := npcs[n.ID]; ok {
			errs = append(errs, fmt.Sprintf("duplicate npc id %q", n.ID))
		}
		npcs[n.ID] = n
		if n.Shop != nil {
			cur := n.Shop.Currency
			if cur == "" {
				errs = append(errs, fmt.Sprintf("npc %s: shop.currency is empty", n.ID))
			} else if !hasResource(p, cur) {
				errs = append(errs, fmt.Sprintf("npc %s: shop currency %q is not in rpg.yaml resources", n.ID, cur))
			}
			for _, sid := range n.Shop.Stock {
				if _, ok := items[sid]; !ok {
					errs = append(errs, fmt.Sprintf("npc %s: shop stock %q is not an item", n.ID, sid))
				}
			}
		}
	}
	for _, r := range p.Rooms {
		for dir, ex := range r.Exits {
			if ex.To == "" {
				errs = append(errs, fmt.Sprintf("room %s: exit %s has no to:", r.ID, dir))
				continue
			}
			if _, ok := rooms[ex.To]; !ok {
				errs = append(errs, fmt.Sprintf("room %s: exit %s points at missing room %q", r.ID, dir, ex.To))
			}
			if ex.Locked && ex.Key != "" {
				if _, ok := items[ex.Key]; !ok {
					errs = append(errs, fmt.Sprintf("room %s: exit %s key %q is not an item", r.ID, dir, ex.Key))
				}
			}
		}
		for _, id := range r.Items {
			if _, ok := items[id]; !ok {
				errs = append(errs, fmt.Sprintf("room %s: item %q is not in world/items.yaml", r.ID, id))
			}
		}
		for _, id := range r.NPCs {
			if _, ok := npcs[id]; !ok {
				errs = append(errs, fmt.Sprintf("room %s: npc %q is not in world/npcs.yaml", r.ID, id))
			}
		}
	}
	for _, it := range p.Items {
		for _, cid := range it.Contains {
			if _, ok := items[cid]; !ok {
				errs = append(errs, fmt.Sprintf("item %s: contains %q which is not an item", it.ID, cid))
			}
		}
	}
	if p.RPG.PrimaryResource != "" && !hasResource(p, p.RPG.PrimaryResource) {
		errs = append(errs, fmt.Sprintf("rpg.yaml: primary_resource %q is not in resources", p.RPG.PrimaryResource))
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func hasResource(p *Pack, key string) bool {
	for _, r := range p.RPG.Resources {
		if r.Key == key {
			return true
		}
	}
	return false
}
