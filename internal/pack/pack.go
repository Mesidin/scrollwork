package pack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sudengine/internal/rpg"
	"sudengine/internal/world"

	"gopkg.in/yaml.v3"
)

type Intro struct {
	Art      string `yaml:"art"`
	Subtitle string `yaml:"subtitle"`
	Blurb    string `yaml:"blurb"`
}

type Meta struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Author      string   `yaml:"author"`
	TickMS      int      `yaml:"tick_ms"`
	CombatEvery int      `yaml:"combat_every"`
	StartRoom   string   `yaml:"start_room"`
	UI          UIConfig `yaml:"ui"`
	Intro       Intro    `yaml:"intro"`
}

type UIConfig struct {
	Panes []string `yaml:"panes"`
}

type Lexicon struct {
	Labels   map[string]string `yaml:"labels"`
	Commands map[string]string `yaml:"commands"` // canonical -> player verb
}

func (l Lexicon) PlayerVerb(canonical string) string {
	if l.Commands != nil {
		if v, ok := l.Commands[canonical]; ok && v != "" {
			return v
		}
	}
	return canonical
}

func (l Lexicon) CanonicalVerb(player string) string {
	player = strings.ToLower(player)
	if l.Commands != nil {
		for canon, disp := range l.Commands {
			if strings.EqualFold(disp, player) {
				return canon
			}
		}
	}
	return player
}

func (l Lexicon) Label(key string) string {
	if l.Labels != nil {
		if v, ok := l.Labels[key]; ok && v != "" {
			return v
		}
	}
	return key
}

type ExitYAML struct {
	To     string `yaml:"to"`
	Door   bool   `yaml:"door,omitempty"`
	Closed *bool  `yaml:"closed,omitempty"`
	Locked bool   `yaml:"locked,omitempty"`
	Key    string `yaml:"key,omitempty"`
}

type ExtraYAML struct {
	Keywords []string `yaml:"keywords"`
	Name     string   `yaml:"name"`
	Short    string   `yaml:"short"`
	Long     string   `yaml:"long"`
	Scripts  string   `yaml:"scripts"`
}

type RoomYAML struct {
	ID      string              `yaml:"id"`
	Name    string              `yaml:"name"`
	Short   string              `yaml:"short"`
	Long    string              `yaml:"long"`
	X       int                 `yaml:"x,omitempty"`
	Y       int                 `yaml:"y,omitempty"`
	Z       int                 `yaml:"z,omitempty"`
	Coords  *bool               `yaml:"coords,omitempty"`
	Exits   map[string]ExitYAML `yaml:"exits"`
	Items   []string            `yaml:"items"`
	NPCs    []string            `yaml:"npcs"`
	Extras  []ExtraYAML         `yaml:"extras"`
	Flags   []string            `yaml:"flags"`
	Scripts string              `yaml:"scripts"`
}

type ItemYAML struct {
	ID        string           `yaml:"id"`
	Keywords  []string         `yaml:"keywords"`
	Name      string           `yaml:"name"`
	Short     string           `yaml:"short"`
	Long      string           `yaml:"long"`
	Takeable  *bool            `yaml:"takeable"`
	Container bool             `yaml:"container"`
	Wearable  bool             `yaml:"wearable"`
	Slot      string           `yaml:"slot"`
	Closed    bool             `yaml:"closed"`
	Locked    bool             `yaml:"locked"`
	Key       string           `yaml:"key"`
	Use       *world.UseEffect `yaml:"use"`
	Flags     []string         `yaml:"flags"`
	Scripts   string           `yaml:"scripts"`
	Contains  []string         `yaml:"contains"`
}

type NPCYAML struct {
	ID        string                    `yaml:"id"`
	Keywords  []string                  `yaml:"keywords"`
	Name      string                    `yaml:"name"`
	Short     string                    `yaml:"short"`
	Long      string                    `yaml:"long"`
	AI        *world.AI                 `yaml:"ai"`
	Combat    *world.Combat             `yaml:"combat"`
	Topics    map[string]string         `yaml:"topics"`
	Attrs     map[string]int            `yaml:"attrs"`
	Resources map[string]world.Resource `yaml:"resources"`
	Flags     []string                  `yaml:"flags"`
	Tags      []string                  `yaml:"tags"`
	Scripts   string                    `yaml:"scripts"`
}

type Pack struct {
	Dir       string
	Meta      Meta
	Lexicon   Lexicon
	RPG       rpg.Schema
	Rooms     []RoomYAML
	Items     []ItemYAML
	NPCs      []NPCYAML
	Abilities []rpg.Ability
	Scripts   map[string]string // relative path -> source
	Help      map[string]string
	IntroArt  string
}

type Info struct {
	ID    string
	Title string
	Dir   string
}

func Discover(gamesDir string) ([]Info, error) {
	entries, err := os.ReadDir(gamesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(gamesDir, e.Name())
		b, err := os.ReadFile(filepath.Join(dir, "pack.yaml"))
		if err != nil {
			continue
		}
		var m Meta
		if err := yaml.Unmarshal(b, &m); err != nil {
			continue
		}
		if m.ID == "" {
			m.ID = e.Name()
		}
		if m.Title == "" {
			m.Title = m.ID
		}
		out = append(out, Info{ID: m.ID, Title: m.Title, Dir: dir})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}

func FindGamesDir() string {
	if wd, err := os.Getwd(); err == nil {
		p := filepath.Join(wd, "games")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "games")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return "games"
}

func Load(dir string) (*Pack, error) {
	p := &Pack{
		Dir:     dir,
		Scripts: map[string]string{},
		Help:    map[string]string{},
	}
	if err := unmarshalFile(filepath.Join(dir, "pack.yaml"), &p.Meta); err != nil {
		return nil, fmt.Errorf("pack.yaml: %w", err)
	}
	if p.Meta.ID == "" {
		p.Meta.ID = filepath.Base(dir)
	}
	if p.Meta.TickMS <= 0 {
		p.Meta.TickMS = 250
	}
	if p.Meta.CombatEvery <= 0 {
		p.Meta.CombatEvery = 4
	}
	if len(p.Meta.UI.Panes) == 0 {
		p.Meta.UI.Panes = []string{"output", "map", "vitals"}
	}
	_ = unmarshalFile(filepath.Join(dir, "lexicon.yaml"), &p.Lexicon)
	_ = unmarshalFile(filepath.Join(dir, "rpg.yaml"), &p.RPG)
	if p.Lexicon.Labels == nil {
		p.Lexicon.Labels = map[string]string{}
	}
	if p.Lexicon.Commands == nil {
		p.Lexicon.Commands = map[string]string{}
	}

	if err := loadList(filepath.Join(dir, "world", "rooms.yaml"), &p.Rooms); err != nil {
		return nil, err
	}
	if err := loadList(filepath.Join(dir, "world", "items.yaml"), &p.Items); err != nil {
		return nil, err
	}
	if err := loadList(filepath.Join(dir, "world", "npcs.yaml"), &p.NPCs); err != nil {
		return nil, err
	}

	abDir := filepath.Join(dir, "abilities")
	if entries, err := os.ReadDir(abDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			var list []rpg.Ability
			if err := loadList(filepath.Join(abDir, e.Name()), &list); err != nil {
				return nil, err
			}
			p.Abilities = append(p.Abilities, list...)
		}
	}
	p.RPG.Abilities = p.Abilities

	scriptDir := filepath.Join(dir, "scripts")
	_ = filepath.Walk(scriptDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".lua") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(scriptDir, path)
		rel = filepath.ToSlash(rel)
		p.Scripts[rel] = string(b)
		p.Scripts[strings.TrimSuffix(rel, ".lua")] = string(b)
		return nil
	})

	helpDir := filepath.Join(dir, "help")
	if entries, err := os.ReadDir(helpDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(helpDir, e.Name()))
			if err != nil {
				continue
			}
			key := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			p.Help[strings.ToLower(key)] = string(b)
		}
	}
	p.IntroArt = loadIntroArt(dir, p.Meta.Intro.Art)
	return p, nil
}

func loadIntroArt(dir, name string) string {
	if name == "" {
		name = "intro.txt"
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\n")
}

func unmarshalFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, v)
}

func loadList[T any](path string, dest *[]T) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return yaml.Unmarshal(b, dest)
}

func (p *Pack) Instantiate() (*world.World, error) {
	w := world.New()
	w.PackID = p.Meta.ID
	w.StartRoom = world.ID(p.Meta.StartRoom)

	for _, it := range p.Items {
		e := itemFromYAML(it)
		w.Protos[e.ID] = e
	}
	for _, n := range p.NPCs {
		e := npcFromYAML(n)
		w.Protos[e.ID] = e
	}

	for _, r := range p.Rooms {
		e := roomFromYAML(r)
		w.Add(e)
	}
	if p.Meta.StartRoom != "" && w.Get(world.ID(p.Meta.StartRoom)) == nil {
		return nil, fmt.Errorf("start_room %q not found", p.Meta.StartRoom)
	}

	for _, r := range p.Rooms {
		roomID := world.ID(r.ID)
		for _, extra := range r.Extras {
			sc := extraToEntity(r.ID, extra)
			w.Add(sc)
			_ = w.Move(sc.ID, roomID)
		}
		for _, id := range r.Items {
			if _, err := w.Spawn(world.ID(id), roomID); err != nil {
				return nil, fmt.Errorf("room %s item %s: %w", r.ID, id, err)
			}
		}
		for _, id := range r.NPCs {
			if _, err := w.Spawn(world.ID(id), roomID); err != nil {
				return nil, fmt.Errorf("room %s npc %s: %w", r.ID, id, err)
			}
		}
	}

	// nested contains on item protos: spawn into first matching instance
	for _, it := range p.Items {
		if len(it.Contains) == 0 {
			continue
		}
		for _, e := range w.Entities {
			if e.PrototypeID == world.ID(it.ID) && e.Kind == world.KindItem {
				for _, cid := range it.Contains {
					if _, err := w.Spawn(world.ID(cid), e.ID); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return w, nil
}

func SpawnPlayer(w *world.World, p *Pack, name, raceID, roleID string) *world.Entity {
	if name == "" {
		name = "you"
	}
	e := &world.Entity{
		ID:       "player",
		Kind:     world.KindPlayer,
		Name:     name,
		Short:    name,
		Long:     "That's you.",
		Keywords: []string{"me", "self", strings.ToLower(name)},
	}
	rpg.ApplyDefaults(e, p.RPG)
	if b := p.RPG.Bundle(p.RPG.Races, raceID); b != nil {
		rpg.ApplyBundle(e, *b)
	}
	if b := p.RPG.Bundle(p.RPG.Roles, roleID); b != nil {
		rpg.ApplyBundle(e, *b)
	}
	if e.Combat == nil {
		e.Combat = &world.Combat{
			RoundSpeed: p.Meta.CombatEvery,
			Attacks:    []world.Attack{{Name: "strike", Damage: "1d6"}},
		}
	}
	w.Add(e)
	w.PlayerID = e.ID
	if w.StartRoom != "" {
		_ = w.Move(e.ID, w.StartRoom)
	}
	return e
}

func itemFromYAML(it ItemYAML) *world.Entity {
	take := true
	if it.Takeable != nil {
		take = *it.Takeable
	}
	e := &world.Entity{
		ID:        world.ID(it.ID),
		Kind:      world.KindItem,
		Keywords:  it.Keywords,
		Name:      it.Name,
		Short:     it.Short,
		Long:      it.Long,
		Takeable:  take,
		Container: it.Container,
		Wearable:  it.Wearable,
		Slot:      it.Slot,
		Closed:    it.Closed,
		Locked:    it.Locked,
		Key:       it.Key,
		Use:       it.Use,
		Scripts:   it.Scripts,
		Flags:     map[string]bool{},
	}
	if e.Short == "" {
		e.Short = e.Name
	}
	for _, f := range it.Flags {
		e.Flags[f] = true
	}
	return e
}

func npcFromYAML(n NPCYAML) *world.Entity {
	e := &world.Entity{
		ID:        world.ID(n.ID),
		Kind:      world.KindMobile,
		Keywords:  n.Keywords,
		Name:      n.Name,
		Short:     n.Short,
		Long:      n.Long,
		AI:        n.AI,
		Combat:    n.Combat,
		Topics:    n.Topics,
		Attrs:     n.Attrs,
		Resources: n.Resources,
		Tags:      n.Tags,
		Scripts:   n.Scripts,
		Flags:     map[string]bool{},
		Takeable:  false,
	}
	if e.Short == "" {
		e.Short = e.Name
	}
	for _, f := range n.Flags {
		e.Flags[f] = true
	}
	if e.Combat != nil && e.Combat.RoundSpeed == 0 {
		e.Combat.RoundSpeed = 4
	}
	return e
}

func roomFromYAML(r RoomYAML) *world.Entity {
	e := &world.Entity{
		ID:        world.ID(r.ID),
		Kind:      world.KindRoom,
		Name:      r.Name,
		Short:     r.Short,
		Long:      r.Long,
		X:         r.X,
		Y:         r.Y,
		Z:         r.Z,
		HasCoords: true,
		Exits:     map[string]world.Exit{},
		Flags:     map[string]bool{},
		Scripts:   r.Scripts,
		Keywords:  []string{world.Slug(r.Name)},
	}
	if r.Coords != nil {
		e.HasCoords = *r.Coords
	}
	if e.Short == "" {
		e.Short = e.Name
	}
	for dir, ex := range r.Exits {
		closed := false
		if ex.Closed != nil {
			closed = *ex.Closed
		} else if ex.Locked {
			closed = true
		}
		e.Exits[world.CanonicalDir(dir)] = world.Exit{
			Dir:    world.CanonicalDir(dir),
			To:     world.ID(ex.To),
			Door:   ex.Door || ex.Locked,
			Closed: closed,
			Locked: ex.Locked,
			Key:    ex.Key,
		}
	}
	for _, f := range r.Flags {
		e.Flags[f] = true
	}
	return e
}

func extraToEntity(roomID string, extra ExtraYAML) *world.Entity {
	id := world.ID(fmt.Sprintf("%s.scenery.%s", roomID, world.Slug(first(extra.Keywords, extra.Name))))
	name := extra.Name
	if name == "" {
		name = first(extra.Keywords, "something")
	}
	short := extra.Short
	if short == "" {
		short = name
	}
	return &world.Entity{
		ID:       id,
		Kind:     world.KindScenery,
		Keywords: extra.Keywords,
		Name:     name,
		Short:    short,
		Long:     extra.Long,
		Scripts:  extra.Scripts,
		Takeable: false,
	}
}

func first(ss []string, fallback string) string {
	if len(ss) > 0 {
		return ss[0]
	}
	return fallback
}

func Blank(id, dir string) *Pack {
	id = world.Slug(id)
	start := id + ".start"
	return &Pack{
		Dir: dir,
		Meta: Meta{
			ID:          id,
			Title:       id,
			TickMS:      250,
			CombatEvery: 4,
			StartRoom:   start,
			UI:          UIConfig{Panes: []string{"output", "map", "vitals"}},
		},
		Lexicon: Lexicon{Labels: map[string]string{}, Commands: map[string]string{}},
		RPG: rpg.Schema{
			PrimaryResource: "hp",
			Resources: []rpg.ResDef{
				{Key: "hp", Label: "HP", Max: 20},
			},
			Attributes: []rpg.AttrDef{
				{Key: "body", Label: "Body"},
			},
			Slots: []string{"wield", "body"},
		},
		Rooms: []RoomYAML{
			{
				ID:   start,
				Name: "Empty Room",
				Long: "A blank room waiting to be described.",
			},
		},
		Scripts: map[string]string{},
		Help: map[string]string{
			"welcome": "# Welcome\n\nThis is a blank pack. Type `help building` for engine builder docs, then `dig`, `desc`, and `save pack`.\n",
		},
		IntroArt: defaultBlankArt(id),
	}
}

func defaultBlankArt(id string) string {
	return "  +------------------+\n  |   " + id + "\n  |   a new world    |\n  +------------------+"
}

func (p *Pack) Write() error {
	dir := p.Dir
	dirs := []string{
		dir,
		filepath.Join(dir, "world"),
		filepath.Join(dir, "scripts"),
		filepath.Join(dir, "abilities"),
		filepath.Join(dir, "help"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := writeYAML(filepath.Join(dir, "pack.yaml"), p.Meta); err != nil {
		return err
	}
	if err := writeYAML(filepath.Join(dir, "lexicon.yaml"), p.Lexicon); err != nil {
		return err
	}
	if err := writeYAML(filepath.Join(dir, "rpg.yaml"), p.RPG); err != nil {
		return err
	}
	if err := writeYAML(filepath.Join(dir, "world", "rooms.yaml"), p.Rooms); err != nil {
		return err
	}
	if err := writeYAML(filepath.Join(dir, "world", "items.yaml"), p.Items); err != nil {
		return err
	}
	if err := writeYAML(filepath.Join(dir, "world", "npcs.yaml"), p.NPCs); err != nil {
		return err
	}
	if len(p.Abilities) > 0 {
		if err := writeYAML(filepath.Join(dir, "abilities", "abilities.yaml"), p.Abilities); err != nil {
			return err
		}
	}
	for k, body := range p.Help {
		name := k + ".md"
		if err := os.WriteFile(filepath.Join(dir, "help", name), []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
			return err
		}
	}
	if p.IntroArt != "" {
		artName := p.Meta.Intro.Art
		if artName == "" {
			artName = "intro.txt"
		}
		if err := os.WriteFile(filepath.Join(dir, artName), []byte(p.IntroArt+"\n"), 0o644); err != nil {
			return err
		}
	}
	// Lua files are never rewritten here.
	return nil
}

func writeYAML(path string, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// SyncFromWorld writes live rooms back into pack room list (build-mode save).
func (p *Pack) SyncFromWorld(w *world.World) {
	rooms := make([]RoomYAML, 0)
	for _, e := range w.Entities {
		if e.Kind != world.KindRoom {
			continue
		}
		ry := RoomYAML{
			ID:      string(e.ID),
			Name:    e.Name,
			Short:   e.Short,
			Long:    e.Long,
			X:       e.X,
			Y:       e.Y,
			Z:       e.Z,
			Exits:   map[string]ExitYAML{},
			Flags:   flagList(e),
			Scripts: e.Scripts,
		}
		c := true
		ry.Coords = &c
		if e.HasCoords {
			// keep
		} else {
			f := false
			ry.Coords = &f
		}
		for dir, ex := range e.Exits {
			closed := ex.Closed
			ry.Exits[dir] = ExitYAML{
				To:     string(ex.To),
				Door:   ex.Door,
				Closed: &closed,
				Locked: ex.Locked,
				Key:    ex.Key,
			}
		}
		for _, c := range w.Children(e.ID) {
			switch c.Kind {
			case world.KindItem:
				pid := string(c.PrototypeID)
				if pid == "" {
					pid = string(c.ID)
				}
				ry.Items = append(ry.Items, pid)
			case world.KindMobile:
				pid := string(c.PrototypeID)
				if pid == "" {
					pid = string(c.ID)
				}
				ry.NPCs = append(ry.NPCs, pid)
			case world.KindScenery:
				ry.Extras = append(ry.Extras, ExtraYAML{
					Keywords: c.Keywords,
					Name:     c.Name,
					Short:    c.Short,
					Long:     c.Long,
					Scripts:  c.Scripts,
				})
			}
		}
		rooms = append(rooms, ry)
	}
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].ID < rooms[j].ID })
	p.Rooms = rooms

	p.Items = p.Items[:0]
	p.NPCs = p.NPCs[:0]
	var itemIDs, npcIDs []string
	for id, e := range w.Protos {
		switch e.Kind {
		case world.KindItem:
			itemIDs = append(itemIDs, string(id))
		case world.KindMobile:
			npcIDs = append(npcIDs, string(id))
		}
	}
	sort.Strings(itemIDs)
	sort.Strings(npcIDs)
	for _, id := range itemIDs {
		e := w.Protos[world.ID(id)]
		take := e.Takeable
		p.Items = append(p.Items, ItemYAML{
			ID:        string(e.ID),
			Keywords:  e.Keywords,
			Name:      e.Name,
			Short:     e.Short,
			Long:      e.Long,
			Takeable:  &take,
			Container: e.Container,
			Wearable:  e.Wearable,
			Slot:      e.Slot,
			Closed:    e.Closed,
			Locked:    e.Locked,
			Key:       e.Key,
			Use:       e.Use,
			Flags:     flagList(e),
			Scripts:   e.Scripts,
		})
	}
	for _, id := range npcIDs {
		e := w.Protos[world.ID(id)]
		p.NPCs = append(p.NPCs, NPCYAML{
			ID:        string(e.ID),
			Keywords:  e.Keywords,
			Name:      e.Name,
			Short:     e.Short,
			Long:      e.Long,
			AI:        e.AI,
			Combat:    e.Combat,
			Topics:    e.Topics,
			Attrs:     e.Attrs,
			Resources: e.Resources,
			Flags:     flagList(e),
			Tags:      e.Tags,
			Scripts:   e.Scripts,
		})
	}
}

func flagList(e *world.Entity) []string {
	var out []string
	for k, v := range e.Flags {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
