package pack

import (
	"fmt"
	"strings"

	"sudengine/internal/quest"
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
		for _, loot := range n.Loot {
			if _, ok := items[loot.Item]; !ok {
				errs = append(errs, fmt.Sprintf("npc %s: loot item %q is not an item", n.ID, loot.Item))
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
		for _, sal := range it.Salvage {
			if _, ok := items[sal.Item]; !ok {
				errs = append(errs, fmt.Sprintf("item %s: salvage item %q is not an item", it.ID, sal.Item))
			}
		}
		if it.Gather != nil && it.Gather.Resource != "" {
			if _, ok := items[it.Gather.Resource]; !ok {
				errs = append(errs, fmt.Sprintf("item %s: gather item %q is not an item", it.ID, it.Gather.Resource))
			}
		}
	}
	for _, r := range p.Rooms {
		if r.Gather != nil && r.Gather.Resource != "" {
			if _, ok := items[r.Gather.Resource]; !ok {
				errs = append(errs, fmt.Sprintf("room %s: gather item %q is not an item", r.ID, r.Gather.Resource))
			}
		}
		for _, ex := range r.Extras {
			if ex.Gather != nil && ex.Gather.Resource != "" {
				if _, ok := items[ex.Gather.Resource]; !ok {
					errs = append(errs, fmt.Sprintf("room %s extra %s: gather item %q is not an item", r.ID, ex.Name, ex.Gather.Resource))
				}
			}
		}
	}
	if p.RPG.PrimaryResource != "" && !hasResource(p, p.RPG.PrimaryResource) {
		errs = append(errs, fmt.Sprintf("rpg.yaml: primary_resource %q is not in resources", p.RPG.PrimaryResource))
	}
	for name, ch := range p.RPG.Checks {
		for _, ref := range ch.Actor {
			if !knownRating(p, ref) {
				errs = append(errs, fmt.Sprintf("rpg.yaml: check %s actor %q is not an attribute or skill", name, ref))
			}
		}
	}
	for _, sk := range p.RPG.Skills {
		if sk.Parent != "" && p.RPG.Attr(sk.Parent) == nil {
			errs = append(errs, fmt.Sprintf("rpg.yaml: skill %s parent %q is not an attribute", sk.Key, sk.Parent))
		}
	}
	recipeIDs := map[string]bool{}
	for _, rec := range p.Recipes {
		if rec.ID == "" {
			errs = append(errs, "recipes.yaml: a recipe has no id")
			continue
		}
		recipeIDs[rec.ID] = true
		if strings.TrimSpace(rec.Produces) == "" {
			errs = append(errs, fmt.Sprintf("recipe %s: produces is empty", rec.ID))
		} else if _, ok := items[rec.Produces]; !ok {
			errs = append(errs, fmt.Sprintf("recipe %s: produces %q which is not an item", rec.ID, rec.Produces))
		}
		if rec.Skill != "" {
			if _, ok := p.RPG.Check(rec.Skill); !ok {
				errs = append(errs, fmt.Sprintf("recipe %s: skill %q is not a check", rec.ID, rec.Skill))
			}
		}
		for _, req := range rec.Requires {
			if _, ok := items[req.Item]; !ok {
				errs = append(errs, fmt.Sprintf("recipe %s: requires %q which is not an item", rec.ID, req.Item))
			}
		}
	}

	questIDs := map[string]quest.Quest{}
	for _, q := range p.Quests {
		if q.ID == "" {
			errs = append(errs, "quests.yaml: a quest has no id")
			continue
		}
		if _, ok := questIDs[q.ID]; ok {
			errs = append(errs, fmt.Sprintf("duplicate quest id %q", q.ID))
		}
		if strings.TrimSpace(q.Title) == "" {
			errs = append(errs, fmt.Sprintf("quest %s: title is empty", q.ID))
		}
		if q.Giver != "" {
			if _, ok := npcs[q.Giver]; !ok {
				errs = append(errs, fmt.Sprintf("quest %s: giver %q is not an npc", q.ID, q.Giver))
			}
		}
		if q.TurnIn != "" {
			if _, ok := npcs[q.TurnIn]; !ok {
				errs = append(errs, fmt.Sprintf("quest %s: turnin %q is not an npc", q.ID, q.TurnIn))
			}
		}
		for _, obj := range q.Objectives {
			if obj.ID == "" {
				errs = append(errs, fmt.Sprintf("quest %s: an objective has no id", q.ID))
			}
			switch obj.Kind {
			case quest.KindKill, quest.KindCollect, quest.KindVisit, quest.KindTalk:
			default:
				errs = append(errs, fmt.Sprintf("quest %s: objective %s has unknown kind %q", q.ID, obj.ID, obj.Kind))
			}
			if obj.Kind == quest.KindCollect && obj.Target != "" {
				if _, ok := items[obj.Target]; !ok {
					errs = append(errs, fmt.Sprintf("quest %s: objective %s collect target %q is not an item", q.ID, obj.ID, obj.Target))
				}
			}
			if obj.Kind == quest.KindVisit && obj.Target != "" {
				if _, ok := rooms[obj.Target]; !ok {
					errs = append(errs, fmt.Sprintf("quest %s: objective %s visit target %q is not a room", q.ID, obj.ID, obj.Target))
				}
			}
		}
		for _, itemID := range q.Rewards.Items {
			if _, ok := items[itemID]; !ok {
				errs = append(errs, fmt.Sprintf("quest %s: reward item %q is not an item", q.ID, itemID))
			}
		}
		for _, recID := range q.Rewards.Recipes {
			if !recipeIDs[recID] {
				errs = append(errs, fmt.Sprintf("quest %s: reward recipe %q is not a recipe", q.ID, recID))
			}
		}
		if q.Rewards.Currency != "" && !hasResource(p, q.Rewards.Currency) {
			errs = append(errs, fmt.Sprintf("quest %s: reward currency %q is not in resources", q.ID, q.Rewards.Currency))
		}
		if q.Script != "" {
			name := strings.TrimPrefix(q.Script, "scripts/")
			_, ok1 := p.Scripts[name]
			_, ok2 := p.Scripts[name+".lua"]
			if !ok1 && !ok2 {
				errs = append(errs, fmt.Sprintf("quest %s: script %q not found", q.ID, q.Script))
			}
		}
		for _, rel := range q.Rewards.Relocate {
			if rel.Mob == "" {
				errs = append(errs, fmt.Sprintf("quest %s: relocate entry missing mob", q.ID))
			} else if _, ok := npcs[rel.Mob]; !ok {
				errs = append(errs, fmt.Sprintf("quest %s: relocate mob %q is not an npc", q.ID, rel.Mob))
			}
			if rel.To == "" {
				errs = append(errs, fmt.Sprintf("quest %s: relocate entry missing destination 'to'", q.ID))
			} else if _, ok := rooms[rel.To]; !ok {
				errs = append(errs, fmt.Sprintf("quest %s: relocate destination room %q is not a room", q.ID, rel.To))
			}
		}
		questIDs[q.ID] = q
	}
	for _, q := range p.Quests {
		for _, reqID := range q.RequiresQuest {
			if _, ok := questIDs[reqID]; !ok {
				errs = append(errs, fmt.Sprintf("quest %s: requires_quest %q does not exist", q.ID, reqID))
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func knownRating(p *Pack, ref string) bool {
	key := ref
	if i := strings.LastIndex(ref, "."); i >= 0 {
		key = ref[i+1:]
	}
	return p.RPG.Attr(key) != nil || p.RPG.Skill(key) != nil
}

func hasResource(p *Pack, key string) bool {
	for _, r := range p.RPG.Resources {
		if r.Key == key {
			return true
		}
	}
	return false
}
