package quest

import (
	"errors"
	"fmt"
	"strings"

	"sudengine/internal/world"
)

type Status string

const (
	StatusNotStarted Status = "not_started"
	StatusActive     Status = "active"
	StatusReady      Status = "ready"
	StatusCompleted  Status = "completed"
)

type ObjectiveKind string

const (
	KindKill    ObjectiveKind = "kill"
	KindCollect ObjectiveKind = "collect"
	KindVisit   ObjectiveKind = "visit"
	KindTalk    ObjectiveKind = "talk"
)

type Objective struct {
	ID      string        `json:"id" yaml:"id"`
	Desc    string        `json:"desc" yaml:"desc"`
	Kind    ObjectiveKind `json:"kind" yaml:"kind"`
	Target  string        `json:"target" yaml:"target"`
	Count   int           `json:"count,omitempty" yaml:"count,omitempty"`
	Consume bool          `json:"consume,omitempty" yaml:"consume,omitempty"`
}

type Relocation struct {
	Mob        string `json:"mob" yaml:"mob"`
	To         string `json:"to" yaml:"to"`
	UpdateHome bool   `json:"update_home,omitempty" yaml:"update_home,omitempty"`
	Message    string `json:"message,omitempty" yaml:"message,omitempty"`
}

type Rewards struct {
	XP       int          `json:"xp,omitempty" yaml:"xp,omitempty"`
	Currency string       `json:"currency,omitempty" yaml:"currency,omitempty"`
	Amount   int          `json:"amount,omitempty" yaml:"amount,omitempty"`
	Items    []string     `json:"items,omitempty" yaml:"items,omitempty"`
	Recipes  []string     `json:"recipes,omitempty" yaml:"recipes,omitempty"`
	Relocate []Relocation `json:"relocate,omitempty" yaml:"relocate,omitempty"`
	Message  string       `json:"message,omitempty" yaml:"message,omitempty"`
}

type Quest struct {
	ID            string      `json:"id" yaml:"id"`
	Title         string      `json:"title" yaml:"title"`
	Desc          string      `json:"desc" yaml:"desc"`
	Giver         string      `json:"giver,omitempty" yaml:"giver,omitempty"`
	TurnIn        string      `json:"turnin,omitempty" yaml:"turnin,omitempty"`
	AutoStart     bool        `json:"auto_start,omitempty" yaml:"auto_start,omitempty"`
	RequiresQuest []string    `json:"requires_quest,omitempty" yaml:"requires_quest,omitempty"`
	Script        string      `json:"script,omitempty" yaml:"script,omitempty"`
	Objectives    []Objective `json:"objectives" yaml:"objectives"`
	Rewards       Rewards     `json:"rewards,omitempty" yaml:"rewards,omitempty"`
}

type Notice struct {
	QuestID       string
	QuestTitle    string
	ObjectiveDesc string
	Current       int
	Needed        int
	ObjectiveDone bool
	QuestReady    bool
}

func (n Notice) String() string {
	if n.QuestReady {
		return fmt.Sprintf("[Quest Ready for Turn-in: %s]", n.QuestTitle)
	}
	if n.ObjectiveDone {
		return fmt.Sprintf("[Objective Complete: %s (%d/%d)]", n.ObjectiveDesc, n.Current, n.Needed)
	}
	return fmt.Sprintf("[Objective Progress: %s (%d/%d)]", n.ObjectiveDesc, n.Current, n.Needed)
}

func IsCompleted(actor *world.Entity, q Quest) bool {
	if actor == nil || actor.Quests == nil {
		return false
	}
	st, ok := actor.Quests[q.ID]
	return ok && st != nil && st.Status == "completed"
}

func IsActive(actor *world.Entity, q Quest) bool {
	if actor == nil || actor.Quests == nil {
		return false
	}
	st, ok := actor.Quests[q.ID]
	return ok && st != nil && st.Status == "active"
}

func IsAvailable(actor *world.Entity, q Quest, all []Quest) bool {
	if actor == nil || IsActive(actor, q) || IsCompleted(actor, q) {
		return false
	}
	for _, reqID := range q.RequiresQuest {
		reqQ := FindQuest(all, reqID)
		if reqQ == nil || !IsCompleted(actor, *reqQ) {
			return false
		}
	}
	return true
}

// FindQuest returns a quest whose id or title matches exactly.
// Requirement ids use this. Typed commands use MatchQuests.
func FindQuest(all []Quest, idOrTitle string) *Quest {
	idOrTitle = strings.TrimSpace(idOrTitle)
	if idOrTitle == "" {
		return nil
	}
	for i := range all {
		if strings.EqualFold(all[i].ID, idOrTitle) || strings.EqualFold(all[i].Title, idOrTitle) {
			return &all[i]
		}
	}
	return nil
}

// MatchQuests returns exact id or title matches, or every quest where the
// query is one whole word of the title or id. Several matches are ambiguous.
func MatchQuests(all []Quest, query string) []*Quest {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	var exact []*Quest
	for i := range all {
		if strings.EqualFold(all[i].ID, query) || strings.EqualFold(all[i].Title, query) {
			exact = append(exact, &all[i])
		}
	}
	if len(exact) > 0 {
		return exact
	}
	low := strings.ToLower(query)
	var words []*Quest
	for i := range all {
		title := strings.ToLower(all[i].Title)
		id := strings.ToLower(strings.ReplaceAll(all[i].ID, "_", " "))
		if questWord(title, low) || questWord(id, low) {
			words = append(words, &all[i])
		}
	}
	return words
}

func questWord(name, word string) bool {
	for _, f := range strings.Fields(name) {
		if f == word {
			return true
		}
	}
	return false
}

func ObjectiveProgress(actor *world.Entity, w *world.World, q Quest, obj Objective) (cur, needed int, done bool) {
	needed = obj.Count
	if needed <= 0 {
		needed = 1
	}
	if actor == nil {
		return 0, needed, false
	}

	if obj.Kind == KindCollect {
		count := 0
		if w != nil {
			for _, cid := range actor.Contents {
				it := w.Get(cid)
				if it != nil && (strings.EqualFold(string(it.PrototypeID), obj.Target) ||
					it.HasKeyword(strings.ToLower(obj.Target)) || strings.EqualFold(string(it.ID), obj.Target)) {
					count++
				}
			}
		}
		return count, needed, count >= needed
	}

	if actor.Quests != nil && actor.Quests[q.ID] != nil && actor.Quests[q.ID].Progress != nil {
		cur = actor.Quests[q.ID].Progress[obj.ID]
	}
	return cur, needed, cur >= needed
}

func IsReady(actor *world.Entity, w *world.World, q Quest) bool {
	if !IsActive(actor, q) {
		return false
	}
	for _, obj := range q.Objectives {
		_, _, done := ObjectiveProgress(actor, w, q, obj)
		if !done {
			return false
		}
	}
	return true
}

func Start(actor *world.Entity, q Quest) bool {
	if actor == nil || IsActive(actor, q) || IsCompleted(actor, q) {
		return false
	}
	if actor.Quests == nil {
		actor.Quests = map[string]*world.QuestState{}
	}
	actor.Quests[q.ID] = &world.QuestState{
		ID:       q.ID,
		Status:   "active",
		Progress: map[string]int{},
	}
	return true
}

func Complete(actor *world.Entity, w *world.World, q Quest, now int64) error {
	if !IsReady(actor, w, q) {
		return fmt.Errorf("objectives are not yet complete")
	}

	// Consume collect items if requested
	if w != nil {
		for _, obj := range q.Objectives {
			if obj.Kind == KindCollect && obj.Consume {
				needed := obj.Count
				if needed <= 0 {
					needed = 1
				}
				consumed := 0
				for _, cid := range append([]world.ID{}, actor.Contents...) {
					it := w.Get(cid)
					if it != nil && (strings.EqualFold(string(it.PrototypeID), obj.Target) ||
						it.HasKeyword(strings.ToLower(obj.Target)) || strings.EqualFold(string(it.ID), obj.Target)) {
						w.Destroy(it.ID)
						consumed++
						if consumed >= needed {
							break
						}
					}
				}
			}
		}
	}

	// Currency, items, and recipes land here. XP is applied by the command
	// so a level-up walks once.
	if q.Rewards.Amount > 0 {
		cur := q.Rewards.Currency
		if cur == "" {
			cur = "coin"
		}
		actor.AdjustRes(cur, q.Rewards.Amount)
	}
	if w != nil {
		for _, itemID := range q.Rewards.Items {
			_, _ = w.Spawn(world.ID(itemID), actor.ID)
		}
	}
	for _, recID := range q.Rewards.Recipes {
		actor.LearnRecipe(recID)
	}

	// Update status
	if actor.Quests == nil {
		actor.Quests = map[string]*world.QuestState{}
	}
	st := actor.Quests[q.ID]
	if st == nil {
		st = &world.QuestState{ID: q.ID}
		actor.Quests[q.ID] = st
	}
	st.Status = "completed"
	st.CompletedAt = now

	return nil
}

// MoveResult is one relocate entry after the attempt. Message is the pack
// line and is only for a move that happened. Reason is the player-facing
// failure.
type MoveResult struct {
	Mob     string
	To      string
	Message string
	OK      bool
	Reason  string
}

// ApplyRelocations moves each reward entry. prefer is the actor's room, used
// when several mobiles share a prototype. Home and leash change only after a
// successful move whose entry sets UpdateHome.
func ApplyRelocations(w *world.World, prefer world.ID, rels []Relocation) []MoveResult {
	if len(rels) == 0 {
		return nil
	}
	out := make([]MoveResult, 0, len(rels))
	for _, rel := range rels {
		res := MoveResult{Mob: rel.Mob, To: rel.To, Message: rel.Message}
		if w == nil {
			res.Reason = fmt.Sprintf("No mobile matches %s; nobody moved.", rel.Mob)
			out = append(out, res)
			continue
		}
		mob, err := w.ResolveMobile(rel.Mob, prefer)
		if err != nil {
			res.Reason = relocateFailure(rel.Mob, err, true)
			out = append(out, res)
			continue
		}
		dest, err := w.ResolveRoom(rel.To)
		if err != nil {
			res.Reason = relocateFailure(rel.To, err, false)
			out = append(out, res)
			continue
		}
		if err := w.Move(mob.ID, dest.ID); err != nil {
			res.Reason = fmt.Sprintf("%s could not enter %s.", rel.Mob, rel.To)
			out = append(out, res)
			continue
		}
		if rel.UpdateHome {
			world.RememberHome(mob, dest)
		}
		res.OK = true
		out = append(out, res)
	}
	return out
}

func relocateFailure(token string, err error, mobile bool) string {
	if errors.Is(err, world.ErrAmbiguous) {
		if mobile {
			return fmt.Sprintf("More than one %s matches; nobody moved.", token)
		}
		return fmt.Sprintf("More than one room matches %s; nobody moved.", token)
	}
	if mobile {
		return fmt.Sprintf("No mobile matches %s; nobody moved.", token)
	}
	return fmt.Sprintf("No room matches %s; nobody moved.", token)
}

func UpdateKill(actor *world.Entity, target *world.Entity, quests []Quest, w *world.World) []Notice {
	if actor == nil || target == nil || len(quests) == 0 {
		return nil
	}
	var notices []Notice
	for _, q := range quests {
		if !IsActive(actor, q) {
			continue
		}
		for _, obj := range q.Objectives {
			if obj.Kind != KindKill {
				continue
			}
			match := strings.EqualFold(string(target.PrototypeID), obj.Target) ||
				target.HasKeyword(strings.ToLower(obj.Target)) ||
				strings.EqualFold(string(target.ID), obj.Target)
			if !match {
				continue
			}
			cur, needed, done := ObjectiveProgress(actor, w, q, obj)
			if done {
				continue
			}
			cur++
			if actor.Quests[q.ID].Progress == nil {
				actor.Quests[q.ID].Progress = map[string]int{}
			}
			actor.Quests[q.ID].Progress[obj.ID] = cur
			isDone := cur >= needed
			isReady := isDone && IsReady(actor, w, q)
			notices = append(notices, Notice{
				QuestID:       q.ID,
				QuestTitle:    q.Title,
				ObjectiveDesc: obj.Desc,
				Current:       cur,
				Needed:        needed,
				ObjectiveDone: isDone,
				QuestReady:    isReady,
			})
		}
	}
	return notices
}

func UpdateVisit(actor *world.Entity, roomID world.ID, quests []Quest, w *world.World) []Notice {
	if actor == nil || len(quests) == 0 {
		return nil
	}
	var notices []Notice
	for _, q := range quests {
		if !IsActive(actor, q) {
			continue
		}
		for _, obj := range q.Objectives {
			if obj.Kind != KindVisit {
				continue
			}
			room := w.Get(roomID)
			match := strings.EqualFold(string(roomID), obj.Target) ||
				(room != nil && (strings.EqualFold(string(room.PrototypeID), obj.Target) || room.HasKeyword(strings.ToLower(obj.Target))))
			if !match {
				continue
			}
			cur, needed, done := ObjectiveProgress(actor, w, q, obj)
			if done {
				continue
			}
			cur = needed
			if actor.Quests[q.ID].Progress == nil {
				actor.Quests[q.ID].Progress = map[string]int{}
			}
			actor.Quests[q.ID].Progress[obj.ID] = cur
			isReady := IsReady(actor, w, q)
			notices = append(notices, Notice{
				QuestID:       q.ID,
				QuestTitle:    q.Title,
				ObjectiveDesc: obj.Desc,
				Current:       cur,
				Needed:        needed,
				ObjectiveDone: true,
				QuestReady:    isReady,
			})
		}
	}
	return notices
}

func UpdateTalk(actor *world.Entity, npc *world.Entity, topic string, quests []Quest, w *world.World) []Notice {
	if actor == nil || len(quests) == 0 {
		return nil
	}
	var notices []Notice
	for _, q := range quests {
		if !IsActive(actor, q) {
			continue
		}
		for _, obj := range q.Objectives {
			if obj.Kind != KindTalk {
				continue
			}
			npcMatch := npc != nil && (strings.EqualFold(string(npc.ID), obj.Target) ||
				strings.EqualFold(string(npc.PrototypeID), obj.Target) ||
				npc.HasKeyword(strings.ToLower(obj.Target)))
			topicMatch := strings.EqualFold(topic, obj.Target)
			if !npcMatch && !topicMatch {
				continue
			}
			cur, needed, done := ObjectiveProgress(actor, w, q, obj)
			if done {
				continue
			}
			cur = needed
			if actor.Quests[q.ID].Progress == nil {
				actor.Quests[q.ID].Progress = map[string]int{}
			}
			actor.Quests[q.ID].Progress[obj.ID] = cur
			isReady := IsReady(actor, w, q)
			notices = append(notices, Notice{
				QuestID:       q.ID,
				QuestTitle:    q.Title,
				ObjectiveDesc: obj.Desc,
				Current:       cur,
				Needed:        needed,
				ObjectiveDone: true,
				QuestReady:    isReady,
			})
		}
	}
	return notices
}
