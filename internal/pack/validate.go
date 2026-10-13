package pack

import (
	"fmt"
	"sort"
	"strings"

	"sudengine/internal/quest"
	"sudengine/internal/world"

	"github.com/yuin/gopher-lua/parse"
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
		}
		for _, loot := range n.Loot {
			if _, ok := items[loot.Item]; !ok {
				errs = append(errs, fmt.Sprintf("npc %s: loot item %q is not an item", n.ID, loot.Item))
			}
		}
		if n.Scripts != "" && !hasScript(p, n.Scripts) {
			errs = append(errs, fmt.Sprintf("npc %s: script %q not found", n.ID, n.Scripts))
		}
	}
	for _, n := range p.NPCs {
		if n.Shop == nil {
			continue
		}
		for _, sid := range n.Shop.Stock {
			if _, ok := items[sid]; ok {
				continue
			}
			if _, ok := npcs[sid]; ok {
				continue
			}
			errs = append(errs, fmt.Sprintf("npc %s: shop stock %q is not an item or npc", n.ID, sid))
		}
	}
	for _, r := range p.Rooms {
		if r.Scripts != "" && !hasScript(p, r.Scripts) {
			errs = append(errs, fmt.Sprintf("room %s: script %q not found", r.ID, r.Scripts))
		}
		for _, ex := range r.Extras {
			if ex.Scripts != "" && !hasScript(p, ex.Scripts) {
				errs = append(errs, fmt.Sprintf("room %s extra %s: script %q not found", r.ID, ex.Name, ex.Scripts))
			}
		}
		for dir, ex := range r.Exits {
			if ex.To == "" {
				errs = append(errs, fmt.Sprintf("room %s: exit %s has no to:", r.ID, dir))
				continue
			}
			if _, ok := rooms[ex.To]; !ok {
				errs = append(errs, fmt.Sprintf("room %s: exit %s points at missing room %q", r.ID, dir, ex.To))
			}
			if ex.Key != "" {
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
		if it.Scripts != "" && !hasScript(p, it.Scripts) {
			errs = append(errs, fmt.Sprintf("item %s: script %q not found", it.ID, it.Scripts))
		}
		if it.Key != "" {
			if _, ok := items[it.Key]; !ok {
				errs = append(errs, fmt.Sprintf("item %s: key %q is not an item", it.ID, it.Key))
			}
		}
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
		if rec.Discovery != "" {
			switch strings.ToLower(rec.Discovery) {
			case "grant", "buy", "discover", "all":
			default:
				errs = append(errs, fmt.Sprintf("recipe %s: discovery %q must be grant, buy, discover, or all", rec.ID, rec.Discovery))
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
		if q.Script != "" && !hasScript(p, q.Script) {
			errs = append(errs, fmt.Sprintf("quest %s: script %q not found", q.ID, q.Script))
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

	validateScriptSyntax(p, &errs)
	validateReciprocalDoors(p, rooms, &errs)
	validateKeys(p, rooms, items, npcs, &errs)
	validateReachability(p, rooms, &errs)

	if len(errs) == 0 {
		return nil
	}
	return errs
}

func validateScriptSyntax(p *Pack, errs *ErrorList) {
	if len(p.Scripts) == 0 {
		return
	}
	checked := map[string]bool{}
	names := make([]string, 0, len(p.Scripts))
	for name := range p.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		src := p.Scripts[name]
		if checked[src] {
			continue
		}
		checked[src] = true
		if _, err := parse.Parse(strings.NewReader(src), name); err != nil {
			*errs = append(*errs, fmt.Sprintf("script %s: syntax error: %v", name, err))
		}
	}
}

func validateReciprocalDoors(p *Pack, rooms map[string]RoomYAML, errs *ErrorList) {
	seenPairs := map[string]bool{}
	for _, r := range p.Rooms {
		for dir, ex := range r.Exits {
			if ex.To == "" || ex.To == r.ID {
				continue
			}
			opp := world.Opposite(dir)
			if opp == "" {
				continue
			}
			dest, ok := rooms[ex.To]
			if !ok {
				continue
			}
			var backDir string
			var backEx ExitYAML
			found := false
			for d, be := range dest.Exits {
				if be.To == r.ID && world.CanonicalDir(d) == opp {
					backDir = d
					backEx = be
					found = true
					break
				}
			}
			if !found {
				continue
			}
			pairKey := doorPair(r.ID, dir, dest.ID, backDir)
			if seenPairs[pairKey] {
				continue
			}
			seenPairs[pairKey] = true
			if ex.Key != "" && backEx.Key != "" && ex.Key != backEx.Key {
				*errs = append(*errs, fmt.Sprintf("rooms %s and %s: conflicting keys %q and %q on door", r.ID, dest.ID, ex.Key, backEx.Key))
			}
		}
	}
}

func doorPair(aRoom, aDir, bRoom, bDir string) string {
	a := aRoom + " " + world.CanonicalDir(aDir)
	b := bRoom + " " + world.CanonicalDir(bDir)
	if a > b {
		a, b = b, a
	}
	return a + " <-> " + b
}

func validateKeys(p *Pack, rooms map[string]RoomYAML, items map[string]ItemYAML, npcs map[string]NPCYAML, errs *ErrorList) {
	requiredKeys := map[string]bool{}
	for _, r := range p.Rooms {
		for _, ex := range r.Exits {
			if ex.Key != "" {
				requiredKeys[ex.Key] = true
			}
		}
	}
	for _, it := range p.Items {
		if it.Key != "" {
			requiredKeys[it.Key] = true
		}
	}

	placedItems := map[string]bool{}
	for _, r := range p.Rooms {
		for _, id := range r.Items {
			placedItems[id] = true
		}
		if r.Gather != nil && r.Gather.Resource != "" {
			placedItems[r.Gather.Resource] = true
		}
		for _, ex := range r.Extras {
			if ex.Gather != nil && ex.Gather.Resource != "" {
				placedItems[ex.Gather.Resource] = true
			}
		}
		for _, nid := range r.NPCs {
			n, ok := npcs[nid]
			if !ok {
				continue
			}
			if n.Shop != nil {
				for _, sid := range n.Shop.Stock {
					placedItems[sid] = true
				}
			}
			for _, loot := range n.Loot {
				placedItems[loot.Item] = true
			}
		}
	}
	for _, q := range p.Quests {
		for _, id := range q.Rewards.Items {
			placedItems[id] = true
		}
	}
	for _, rec := range p.Recipes {
		if rec.Produces != "" {
			placedItems[rec.Produces] = true
		}
	}
	// Contents and gather nodes count only once their holder is already placed.
	// A key does not count when its only holder is the container that key locks.
	seen := map[string]bool{}
	var queue []string
	for id := range placedItems {
		queue = append(queue, id)
	}
	sort.Strings(queue)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		it, ok := items[id]
		if !ok {
			continue
		}
		if it.Gather != nil && it.Gather.Resource != "" && !placedItems[it.Gather.Resource] {
			placedItems[it.Gather.Resource] = true
			queue = append(queue, it.Gather.Resource)
		}
		for _, cid := range it.Contains {
			if cid == "" || cid == it.Key || placedItems[cid] {
				continue
			}
			placedItems[cid] = true
			queue = append(queue, cid)
		}
	}
	for keyID := range requiredKeys {
		if scriptMentions(p, keyID) {
			placedItems[keyID] = true
		}
	}

	// Unobtainable keys: a required key must be placed in the world or referenced in a script
	reqList := make([]string, 0, len(requiredKeys))
	for k := range requiredKeys {
		reqList = append(reqList, k)
	}
	sort.Strings(reqList)
	for _, k := range reqList {
		if !placedItems[k] {
			*errs = append(*errs, fmt.Sprintf("key %q required by lock is not placed in the world", k))
		}
	}

	// Orphaned keys: a defined key item must lock at least one exit or container, or be used in a script
	for _, it := range p.Items {
		if isKeyItem(it) {
			if !requiredKeys[it.ID] && !scriptMentions(p, it.ID) {
				*errs = append(*errs, fmt.Sprintf("item %s: orphaned key does not unlock any door or container", it.ID))
			}
		}
	}
}

func validateReachability(p *Pack, rooms map[string]RoomYAML, errs *ErrorList) {
	if p.Meta.StartRoom == "" || rooms[p.Meta.StartRoom].ID == "" {
		return
	}
	reachable := map[string]bool{p.Meta.StartRoom: true}
	queue := []string{p.Meta.StartRoom}

	for _, q := range p.Quests {
		for _, rel := range q.Rewards.Relocate {
			if rel.To != "" && !reachable[rel.To] {
				if _, ok := rooms[rel.To]; ok {
					reachable[rel.To] = true
					queue = append(queue, rel.To)
				}
			}
		}
	}

	for rID := range rooms {
		if !reachable[rID] && scriptMentions(p, rID) {
			reachable[rID] = true
			queue = append(queue, rID)
		}
	}

	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]
		r := rooms[currID]
		for _, ex := range r.Exits {
			if ex.To != "" && !reachable[ex.To] {
				if _, ok := rooms[ex.To]; ok {
					reachable[ex.To] = true
					queue = append(queue, ex.To)
				}
			}
		}
	}

	for _, r := range p.Rooms {
		if !reachable[r.ID] {
			*errs = append(*errs, fmt.Sprintf("room %s: unreachable from start_room %q", r.ID, p.Meta.StartRoom))
		}
	}
}

func hasScript(p *Pack, ref string) bool {
	if ref == "" {
		return true
	}
	if p.Scripts == nil {
		return false
	}
	name := strings.TrimPrefix(ref, "scripts/")
	if _, ok := p.Scripts[name]; ok {
		return true
	}
	if _, ok := p.Scripts[name+".lua"]; ok {
		return true
	}
	if _, ok := p.Scripts[strings.TrimSuffix(name, ".lua")]; ok {
		return true
	}
	return false
}

func scriptMentions(p *Pack, target string) bool {
	if target == "" || p.Scripts == nil {
		return false
	}
	for _, src := range p.Scripts {
		if tokenContains(src, target) {
			return true
		}
	}
	return false
}

// tokenContains reports whether target is a whole id in src.
// Letters, digits, '_', '.', and '-' stick to the id, so house.hall does not match house.hallway.
func tokenContains(src, target string) bool {
	for i := 0; i+len(target) <= len(src); {
		j := strings.Index(src[i:], target)
		if j < 0 {
			return false
		}
		j += i
		end := j + len(target)
		beforeOK := j == 0 || !isIDChar(src[j-1])
		afterOK := end == len(src) || !isIDChar(src[end])
		if beforeOK && afterOK {
			return true
		}
		i = j + 1
	}
	return false
}

func isIDChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '.' || b == '-'
}

func isKeyItem(it ItemYAML) bool {
	id := strings.ToLower(it.ID)
	return id == "key" || strings.HasSuffix(id, "-key") || strings.HasSuffix(id, "_key") || strings.HasSuffix(id, ".key")
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
