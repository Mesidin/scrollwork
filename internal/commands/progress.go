package commands

import (
	"fmt"
	"strings"

	"sudengine/internal/rpg"
	"sudengine/internal/world"
)

func hiddenFrom(actor, e *world.Entity) bool {
	if e == nil || !e.Hidden {
		return false
	}
	if actor == nil {
		return true
	}
	return !actor.HasFound(e.ID)
}

func noticeOnEnter(c *Context, room *world.Entity) {
	if c.Pack == nil || room == nil || !c.World.RoomIsLit(room) {
		return
	}
	if _, ok := c.Pack.RPG.Check("notice"); !ok {
		return
	}
	for _, e := range c.World.Children(room.ID) {
		if e.ID == c.Actor.ID || !e.Hidden || c.Actor.HasFound(e.ID) {
			continue
		}
		dc := e.Notice
		has := e.Notice != 0
		if !has {
			dc = 0
		}
		res, ok := c.Pack.RPG.Resolve(c.RNG, c.World, c.Actor, "notice", dc, has)
		if ok && res.Success {
			c.Actor.MarkFound(e.ID)
		}
	}
}

func revealHidden(c *Context, e *world.Entity) bool {
	if _, ok := c.Pack.RPG.Check("notice"); !ok {
		c.Actor.MarkFound(e.ID)
		return true
	}
	dc := e.SearchDC
	has := e.SearchDC != 0
	if !has {
		dc = e.Notice
		has = e.Notice != 0
	}
	res, ok := c.Pack.RPG.Resolve(c.RNG, c.World, c.Actor, "notice", dc, has)
	if !ok || !res.Success {
		return false
	}
	c.Actor.MarkFound(e.ID)
	return true
}

func searchCmd(c *Context, _ Parsed) {
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		c.Print("You find nothing.")
		return
	}
	if !c.World.RoomIsLit(room) {
		c.Print("It's pitch black. You can't search.")
		return
	}
	var hidden []*world.Entity
	for _, e := range c.World.Children(room.ID) {
		if e.Hidden && !c.Actor.HasFound(e.ID) {
			hidden = append(hidden, e)
		}
	}
	if len(hidden) == 0 {
		c.Print("You find nothing hidden.")
		return
	}
	if _, ok := c.Pack.RPG.Check("notice"); !ok {
		for _, e := range hidden {
			c.Actor.MarkFound(e.ID)
			c.Print("You find %s.", e.Display())
		}
		return
	}
	found := 0
	for _, e := range hidden {
		if revealHidden(c, e) {
			found++
			c.Print("You find %s.", e.Display())
		}
	}
	if found == 0 {
		c.Print("You find nothing hidden.")
	}
}

func rollCheck(c *Context, name string, opponent world.ID) (rpg.Result, bool) {
	if c.Pack == nil {
		return rpg.Result{}, false
	}
	opp, has := 0, false
	if tgt := c.World.Get(opponent); tgt != nil && tgt.Oppose != nil {
		if n, ok := tgt.Oppose[name]; ok {
			opp, has = n, true
		}
	}
	return c.Pack.RPG.Resolve(c.RNG, c.World, c.Actor, name, opp, has)
}

func improveCmd(c *Context, p Parsed) {
	if c.Pack == nil || !c.Pack.RPG.Advancement.On() {
		c.Print("There is nothing here to improve.")
		return
	}
	key := strings.ToLower(strings.TrimSpace(p.Rest))
	if key == "" {
		listSkills(c, "improve")
		return
	}
	if c.Pack.RPG.Attr(key) != nil && c.Pack.RPG.Skill(key) == nil {
		c.Print("Attributes are raised, not improved. Try raise %s.", key)
		return
	}
	sk := matchSkill(c, key)
	if sk == nil {
		c.Print("No skill %q.", key)
		return
	}
	if c.Actor.Skills == nil {
		c.Actor.Skills = map[string]int{}
	}
	if sk.Max > 0 && c.Actor.Skills[sk.Key] >= sk.Max {
		c.Print("You already know %s as well as you can.", c.Pack.Lexicon.Label(sk.Key))
		return
	}
	if c.Actor.SkillPoints < 1 {
		c.Print("You have no skill points. A trainer might still teach you.")
		return
	}
	c.Actor.Skills[sk.Key]++
	c.Actor.SkillPoints--
	c.Print("Your %s improves. (%d points left)", c.Pack.Lexicon.Label(sk.Key), c.Actor.SkillPoints)
}

func raiseCmd(c *Context, p Parsed) {
	if c.Pack == nil || !c.Pack.RPG.Advancement.On() {
		c.Print("There is nothing here to raise.")
		return
	}
	key := strings.ToLower(strings.TrimSpace(p.Rest))
	if key == "" {
		c.Print("Raise which attribute?")
		return
	}
	if c.Pack.RPG.Skill(key) != nil && c.Pack.RPG.Attr(key) == nil {
		c.Print("Skills are improved or trained. Try improve %s.", key)
		return
	}
	at := matchAttr(c, key)
	if at == nil {
		c.Print("No attribute %q.", key)
		return
	}
	if c.Actor.Attrs == nil {
		c.Actor.Attrs = map[string]int{}
	}
	if at.Max > 0 && c.Actor.Attrs[at.Key] >= at.Max {
		c.Print("Your %s cannot go any higher.", c.Pack.Lexicon.Label(at.Key))
		return
	}
	if c.Actor.AttrPoints < 1 {
		c.Print("You have no attribute points. They come every few levels, and they are not for sale.")
		return
	}
	c.Actor.Attrs[at.Key]++
	c.Actor.AttrPoints--
	c.Print("Your %s rises. (%d points left)", c.Pack.Lexicon.Label(at.Key), c.Actor.AttrPoints)
}

func trainCmd(c *Context, p Parsed) {
	if c.Pack == nil || !c.Pack.RPG.Advancement.On() {
		c.Print("No one here can train you.")
		return
	}
	key := strings.ToLower(strings.TrimSpace(p.Rest))
	if key == "" {
		listSkills(c, "train")
		return
	}
	if c.Pack.RPG.Attr(key) != nil && c.Pack.RPG.Skill(key) == nil {
		c.Print("Attributes are not for sale.")
		return
	}
	sk := matchSkill(c, key)
	if sk == nil {
		c.Print("No skill %q.", key)
		return
	}
	trainer := trainerHere(c)
	if trainer == nil {
		c.Print("You need a trainer for that.")
		return
	}
	if c.Actor.Skills == nil {
		c.Actor.Skills = map[string]int{}
	}
	if sk.Max > 0 && c.Actor.Skills[sk.Key] >= sk.Max {
		c.Print("%s has nothing more to teach you about %s.", trainer.CapDisplay(), c.Pack.Lexicon.Label(sk.Key))
		return
	}
	cur := c.Pack.RPG.Advancement.Purse()
	price := c.Pack.RPG.Advancement.TrainPrice()
	if c.Actor.Res(cur).Current < price {
		c.Print("Training costs %d %s.", price, c.Pack.Lexicon.Label(cur))
		return
	}
	c.Actor.AdjustRes(cur, -price)
	c.Actor.Skills[sk.Key]++
	c.Print("%s drills you in %s. You pay %d %s.", trainer.CapDisplay(), c.Pack.Lexicon.Label(sk.Key), price, c.Pack.Lexicon.Label(cur))
}

func listSkills(c *Context, verb string) {
	if c.Pack == nil || len(c.Pack.RPG.Skills) == 0 {
		c.Print("This world has no skills.")
		return
	}
	c.Print("Skills")
	for i := range c.Pack.RPG.Skills {
		c.Print("%s", skillLine(c, &c.Pack.RPG.Skills[i]))
	}
	switch verb {
	case "train":
		price := c.Pack.RPG.Advancement.TrainPrice()
		cur := c.Pack.Lexicon.Label(c.Pack.RPG.Advancement.Purse())
		if trainerHere(c) == nil {
			c.Print("Find a trainer, then train <skill> for %d %s.", price, cur)
		} else {
			c.Print("train <skill> pays %d %s.", price, cur)
		}
	default:
		c.Print("You have %d skill points. improve <skill> spends one.", c.Actor.SkillPoints)
	}
}

func skillLine(c *Context, sk *rpg.SkillDef) string {
	ranks := 0
	if c.Actor.Skills != nil {
		ranks = c.Actor.Skills[sk.Key]
	}
	eff := c.Pack.RPG.Effective(c.Actor, "skill."+sk.Key)
	label := c.Pack.Lexicon.Label(sk.Key)
	cap := ""
	if sk.Max > 0 {
		cap = fmt.Sprintf(" / %d", sk.Max)
	}
	from := ""
	if sk.Parent != "" {
		from = "  from " + c.Pack.Lexicon.Label(sk.Parent)
	}
	return fmt.Sprintf("  %-12s %d   trained %d%s%s", label, eff, ranks, cap, from)
}

func trainerHere(c *Context) *world.Entity {
	room := c.World.RoomOf(c.Actor)
	if room == nil {
		return nil
	}
	for _, e := range c.World.Children(room.ID) {
		if e.Trainer {
			return e
		}
	}
	return nil
}

func matchSkill(c *Context, token string) *rpg.SkillDef {
	token = strings.ToLower(token)
	for i := range c.Pack.RPG.Skills {
		sk := &c.Pack.RPG.Skills[i]
		if sk.Key == token || strings.EqualFold(sk.Label, token) || strings.EqualFold(c.Pack.Lexicon.Label(sk.Key), token) {
			return sk
		}
	}
	return nil
}

func matchAttr(c *Context, token string) *rpg.AttrDef {
	token = strings.ToLower(token)
	for i := range c.Pack.RPG.Attributes {
		at := &c.Pack.RPG.Attributes[i]
		if at.Key == token || strings.EqualFold(at.Label, token) || strings.EqualFold(c.Pack.Lexicon.Label(at.Key), token) {
			return at
		}
	}
	return nil
}
