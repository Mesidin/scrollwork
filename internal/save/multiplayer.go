package save

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
	"sudengine/internal/world"
)

// BcryptCost is the bcrypt hashing cost used for account passwords.
var BcryptCost = bcrypt.DefaultCost

// ErrInvalidCredentials is returned when an account username or password does not match.
var ErrInvalidCredentials = errors.New("invalid username or password")

// ErrAccountExists is returned when creating an account that already exists.
var ErrAccountExists = errors.New("account already exists")

type Account struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	Characters   []string  `json:"characters,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type PlayerSave struct {
	Version   int                        `json:"version"`
	PackID    string                     `json:"pack_id"`
	SavedAt   time.Time                  `json:"saved_at"`
	Account   string                     `json:"account,omitempty"`
	Character string                     `json:"character"`
	PlayerID  world.ID                   `json:"player_id"`
	RoomID    world.ID                   `json:"room_id"`
	Player    *world.Entity              `json:"player"`
	Entities  map[world.ID]*world.Entity `json:"entities,omitempty"`
}

func AccountsDir(packID string) string {
	return filepath.Join(Dir(packID), "accounts")
}

func PlayersDir(packID string) string {
	return filepath.Join(Dir(packID), "players")
}

func WorldPath(packID string) string {
	return filepath.Join(Dir(packID), "world.json")
}

func AccountPath(packID, username string) (string, error) {
	slug, err := Slug(username)
	if err != nil {
		return "", err
	}
	return filepath.Join(AccountsDir(packID), slug+".json"), nil
}

func PlayerPath(packID, character string) (string, error) {
	slug, err := Slug(character)
	if err != nil {
		return "", err
	}
	return filepath.Join(PlayersDir(packID), slug+".json"), nil
}

// Slug normalizes an account or character name into a safe filename key.
func Slug(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty name")
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "", fmt.Errorf("invalid name %q", name)
	}
	return s, nil
}

func AccountExists(packID, username string) bool {
	p, err := AccountPath(packID, username)
	if err != nil {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func CreateAccount(packID, username, password string) (*Account, error) {
	username = strings.TrimSpace(username)
	id, err := Slug(username)
	if err != nil {
		return nil, err
	}
	if password == "" {
		return nil, fmt.Errorf("password cannot be empty")
	}
	if AccountExists(packID, username) {
		return nil, ErrAccountExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return nil, err
	}
	acc := &Account{
		ID:           id,
		Username:     username,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}
	if err := SaveAccount(packID, acc); err != nil {
		return nil, err
	}
	return acc, nil
}

func SaveAccount(packID string, acc *Account) error {
	if acc == nil {
		return fmt.Errorf("nil account")
	}
	path, err := AccountPath(packID, acc.Username)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(acc, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func LoadAccount(packID, username string) (*Account, error) {
	path, err := AccountPath(packID, username)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var acc Account
	if err := json.Unmarshal(b, &acc); err != nil {
		return nil, err
	}
	if acc.ID == "" || acc.PasswordHash == "" {
		return nil, fmt.Errorf("invalid account file")
	}
	return &acc, nil
}

func AuthenticateAccount(packID, username, password string) (*Account, error) {
	acc, err := LoadAccount(packID, username)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return acc, nil
}

func AddCharacterToAccount(packID, username, charName string) error {
	acc, err := LoadAccount(packID, username)
	if err != nil {
		return err
	}
	charName = strings.TrimSpace(charName)
	for _, existing := range acc.Characters {
		if strings.EqualFold(existing, charName) {
			return nil
		}
	}
	acc.Characters = append(acc.Characters, charName)
	return SaveAccount(packID, acc)
}

func (a *Account) OwnsCharacter(charName string) bool {
	if a == nil {
		return false
	}
	for _, c := range a.Characters {
		if strings.EqualFold(c, charName) {
			return true
		}
	}
	return false
}

// ExtractPlayerSubgraph clones the given player and all descendant entities
// carried in their inventory tree without including any other player's items
// or shared world rooms/mobiles.
func ExtractPlayerSubgraph(packID string, w *world.World, player *world.Entity) (*PlayerSave, error) {
	if w == nil || player == nil {
		return nil, fmt.Errorf("missing world or player")
	}
	if player.Kind != world.KindPlayer {
		return nil, fmt.Errorf("entity %s is not a player", player.ID)
	}
	roomID := player.Parent
	if room := w.RoomOf(player); room != nil {
		roomID = room.ID
	}

	plCopy := player.Clone()
	plCopy.Leader = ""
	plCopy.Followers = nil
	plCopy.PartyLeader = ""
	plCopy.PartyInvites = nil
	if plCopy.Combat != nil {
		plCopy.Combat.Target = ""
		plCopy.Combat.NextSwing = 0
	}
	if plCopy.Flags != nil {
		delete(plCopy.Flags, "linkdead")
	}

	descendants := map[world.ID]*world.Entity{}
	var collect func(parentID world.ID)
	collect = func(parentID world.ID) {
		p := w.Get(parentID)
		if p == nil {
			return
		}
		for _, cid := range p.Contents {
			child := w.Get(cid)
			if child == nil || child.Kind == world.KindPlayer || child.Kind == world.KindRoom {
				continue
			}
			if _, seen := descendants[cid]; seen {
				continue
			}
			descendants[cid] = child.Clone()
			collect(cid)
		}
	}
	collect(player.ID)

	charName := player.Name
	if charName == "" {
		charName = string(player.ID)
	}

	return &PlayerSave{
		Version:   1,
		PackID:    packID,
		SavedAt:   time.Now().UTC(),
		Character: charName,
		PlayerID:  player.ID,
		RoomID:    roomID,
		Player:    plCopy,
		Entities:  descendants,
	}, nil
}

// CutWorldWithoutPlayers returns a cloned World with all player entities and
// their carried inventory subgraphs removed.
func CutWorldWithoutPlayers(w *world.World) *world.World {
	if w == nil {
		return world.New()
	}
	excluded := map[world.ID]bool{}
	var markSubgraph func(id world.ID)
	markSubgraph = func(id world.ID) {
		if excluded[id] {
			return
		}
		excluded[id] = true
		e := w.Get(id)
		if e == nil {
			return
		}
		for _, cid := range e.Contents {
			markSubgraph(cid)
		}
	}
	for id, e := range w.Entities {
		if e != nil && e.Kind == world.KindPlayer {
			markSubgraph(id)
		}
	}

	cut := &world.World{
		Entities:      make(map[world.ID]*world.Entity, len(w.Entities)),
		Protos:        make(map[world.ID]*world.Entity, len(w.Protos)),
		PlayerID:      "",
		StartRoom:     w.StartRoom,
		Tick:          w.Tick,
		NextSeq:       w.NextSeq,
		PackID:        w.PackID,
		WeatherState:  w.WeatherState,
		Respawns:      append([]world.RespawnTask{}, w.Respawns...),
		IsAmbientDark: w.IsAmbientDark,
		IsAmbientLit:  w.IsAmbientLit,
	}
	for id, proto := range w.Protos {
		if proto != nil {
			cut.Protos[id] = proto.Clone()
		}
	}
	for id, e := range w.Entities {
		if e == nil || excluded[id] {
			continue
		}
		cp := e.Clone()
		if len(cp.Contents) > 0 {
			filtered := cp.Contents[:0]
			for _, cid := range cp.Contents {
				if !excluded[cid] {
					filtered = append(filtered, cid)
				}
			}
			cp.Contents = filtered
		}
		if len(cp.Followers) > 0 {
			filtered := cp.Followers[:0]
			for _, fid := range cp.Followers {
				if !excluded[fid] {
					filtered = append(filtered, fid)
				}
			}
			cp.Followers = filtered
		}
		if len(cp.PartyInvites) > 0 {
			filtered := cp.PartyInvites[:0]
			for _, pid := range cp.PartyInvites {
				if !excluded[pid] {
					filtered = append(filtered, pid)
				}
			}
			cp.PartyInvites = filtered
		}
		if excluded[cp.Leader] {
			cp.Leader = ""
		}
		if excluded[cp.PartyLeader] {
			cp.PartyLeader = ""
		}
		if cp.Combat != nil && excluded[cp.Combat.Target] {
			cp.Combat.Target = ""
			cp.Combat.NextSwing = 0
		}
		cut.Entities[id] = cp
	}
	return cut
}

type stagedFile struct {
	target    string
	tmp       string
	bak       string
	hadBak    bool
	published bool
	data      []byte
}

// pairMarker is written before any live rename and removed only after every
// rename has succeeded. A marker left on disk means the pair is unfinished.
type pairMarker struct {
	Fresh []string `json:"fresh,omitempty"`
}

func pairMarkerPath(packID string) string {
	return filepath.Join(Dir(packID), "paired.commit")
}

// WritePaired writes every live player file and world.json as one pair.
func WritePaired(packID string, w *world.World, players ...*world.Entity) error {
	return writePaired(packID, w, nil, nil, players...)
}

// WritePairedOwned is WritePaired, recording accounts on the player files in that same pair.
func WritePairedOwned(packID string, w *world.World, accounts map[world.ID]string, players ...*world.Entity) error {
	return writePaired(packID, w, nil, accounts, players...)
}

// WritePairedWithHook is WritePaired with a per-file hook for tests.
// The hook runs before each temporary file is written. Player files come first, world.json last.
func WritePairedWithHook(packID string, w *world.World, hook func(index int, path string) error, players ...*world.Entity) error {
	return writePaired(packID, w, hook, nil, players...)
}

func writePaired(packID string, w *world.World, hook func(index int, path string) error, accounts map[world.ID]string, players ...*world.Entity) error {
	if w == nil {
		return fmt.Errorf("nil world")
	}
	if err := RecoverPaired(packID); err != nil {
		return err
	}
	if err := os.MkdirAll(Dir(packID), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(PlayersDir(packID), 0o755); err != nil {
		return err
	}

	var staged []stagedFile
	seenPaths := map[string]bool{}
	for _, pl := range pairedPlayers(w, players) {
		pSave, err := ExtractPlayerSubgraph(packID, w, pl)
		if err != nil {
			return err
		}
		if accounts != nil && accounts[pl.ID] != "" {
			pSave.Account = accounts[pl.ID]
		} else if existing, err := readPlayerFile(packID, pl.Name); err == nil && existing != nil && existing.Account != "" {
			pSave.Account = existing.Account
		}
		pPath, err := PlayerPath(packID, pl.Name)
		if err != nil {
			return err
		}
		if seenPaths[pPath] {
			continue
		}
		seenPaths[pPath] = true
		data, err := json.MarshalIndent(pSave, "", "  ")
		if err != nil {
			return err
		}
		staged = append(staged, stagedFile{
			target: pPath,
			tmp:    pPath + ".tmp",
			bak:    pPath + ".bak",
			data:   data,
		})
	}

	worldCut := CutWorldWithoutPlayers(w)
	worldSnap := Snapshot{
		Version: 1,
		PackID:  packID,
		SavedAt: time.Now().UTC(),
		Slot:    "world",
		World:   worldCut,
	}
	worldData, err := json.MarshalIndent(worldSnap, "", "  ")
	if err != nil {
		return err
	}
	wPath := WorldPath(packID)
	staged = append(staged, stagedFile{
		target: wPath,
		tmp:    wPath + ".tmp",
		bak:    wPath + ".bak",
		data:   worldData,
	})

	removeTmp := func() {
		for _, sf := range staged {
			_ = os.Remove(sf.tmp)
		}
	}
	removeBaks := func() {
		for _, sf := range staged {
			_ = os.Remove(sf.bak)
		}
	}

	for i, sf := range staged {
		if hook != nil {
			if err := hook(i, sf.target); err != nil {
				removeTmp()
				return err
			}
		}
		if err := os.WriteFile(sf.tmp, sf.data, 0o644); err != nil {
			removeTmp()
			return err
		}
	}

	var fresh []string
	for i := range staged {
		sf := &staged[i]
		existing, err := os.ReadFile(sf.target)
		if err != nil {
			if os.IsNotExist(err) {
				fresh = append(fresh, sf.target)
				continue
			}
			removeTmp()
			removeBaks()
			return err
		}
		if err := os.WriteFile(sf.bak, existing, 0o644); err != nil {
			removeTmp()
			removeBaks()
			return err
		}
		sf.hadBak = true
	}

	if err := writePairMarker(packID, fresh); err != nil {
		removeTmp()
		removeBaks()
		return err
	}

	var renameErr error
	for i := range staged {
		if err := os.Rename(staged[i].tmp, staged[i].target); err != nil {
			renameErr = err
			break
		}
		staged[i].published = true
	}
	if renameErr != nil {
		removeTmp()
		if err := restoreStaged(staged); err != nil {
			return fmt.Errorf("%w (rollback: %v)", renameErr, err)
		}
		if err := os.Remove(pairMarkerPath(packID)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("%w (clear marker: %v)", renameErr, err)
		}
		return renameErr
	}

	if err := os.Remove(pairMarkerPath(packID)); err != nil && !os.IsNotExist(err) {
		if rerr := restoreStaged(staged); rerr != nil {
			return fmt.Errorf("clear marker: %v (rollback: %v)", err, rerr)
		}
		return err
	}
	removeBaks()
	return nil
}

func pairedPlayers(w *world.World, extra []*world.Entity) []*world.Entity {
	seen := map[world.ID]bool{}
	var out []*world.Entity
	add := func(e *world.Entity) {
		if e == nil || e.Kind != world.KindPlayer {
			return
		}
		if e.ID != "" {
			if seen[e.ID] {
				return
			}
			seen[e.ID] = true
		}
		out = append(out, e)
	}
	if w != nil {
		for _, e := range w.Entities {
			add(e)
		}
	}
	for _, e := range extra {
		add(e)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func writePairMarker(packID string, fresh []string) error {
	body, err := json.Marshal(pairMarker{Fresh: fresh})
	if err != nil {
		return err
	}
	path := pairMarkerPath(packID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// restoreStaged puts published files back. A .bak is removed only after that file is restored.
func restoreStaged(staged []stagedFile) error {
	var errs []error
	for i := range staged {
		sf := &staged[i]
		if sf.hadBak {
			if err := replaceWithBak(sf.bak, sf.target); err != nil {
				errs = append(errs, err)
				continue
			}
			sf.hadBak = false
			continue
		}
		if sf.published {
			if err := os.Remove(sf.target); err != nil && !os.IsNotExist(err) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func replaceWithBak(bak, target string) error {
	if err := os.Rename(bak, target); err == nil {
		return nil
	}
	data, err := os.ReadFile(bak)
	if err != nil {
		return err
	}
	tmp := target + ".restore"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Remove(bak); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RecoverPaired finishes or undoes an interrupted paired write.
// A paired.commit file means the rename did not finish, so live files are restored from .bak.
// With no marker, leftover .bak and .tmp files are discarded and the live pair is kept.
func RecoverPaired(packID string) error {
	path := pairMarkerPath(packID)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sweepPairLeftovers(packID)
		}
		return err
	}
	var marker pairMarker
	if len(bytesTrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &marker); err != nil {
			return err
		}
	}
	if err := rollbackPair(packID, marker.Fresh); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return sweepPairLeftovers(packID)
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func rollbackPair(packID string, fresh []string) error {
	var errs []error
	restore := func(target string) {
		bak := target + ".bak"
		if _, err := os.Stat(bak); err != nil {
			if !os.IsNotExist(err) {
				errs = append(errs, err)
			}
			return
		}
		if err := replaceWithBak(bak, target); err != nil {
			errs = append(errs, err)
		}
	}
	restore(WorldPath(packID))
	entries, err := os.ReadDir(PlayersDir(packID))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".bak") {
			continue
		}
		target := strings.TrimSuffix(filepath.Join(PlayersDir(packID), name), ".bak")
		restore(target)
	}
	if joined := errors.Join(errs...); joined != nil {
		return joined
	}
	for _, target := range fresh {
		if target == "" {
			continue
		}
		if _, err := os.Stat(target + ".bak"); err == nil {
			continue
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func sweepPairLeftovers(packID string) error {
	var errs []error
	cleanDir := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			errs = append(errs, err)
			return
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".bak") || strings.HasSuffix(name, ".restore") {
				if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
					errs = append(errs, err)
				}
			}
		}
	}
	cleanDir(Dir(packID))
	cleanDir(PlayersDir(packID))
	return errors.Join(errs...)
}

func HasWorld(packID string) bool {
	if err := RecoverPaired(packID); err != nil {
		return false
	}
	st, err := os.Stat(WorldPath(packID))
	return err == nil && !st.IsDir()
}

func ReadWorld(packID string) (*world.World, error) {
	if err := RecoverPaired(packID); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(WorldPath(packID))
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil, err
	}
	if snap.World == nil {
		return nil, fmt.Errorf("empty world snapshot")
	}
	if snap.World.Entities == nil {
		snap.World.Entities = map[world.ID]*world.Entity{}
	}
	if snap.World.Protos == nil {
		snap.World.Protos = map[world.ID]*world.Entity{}
	}
	return snap.World, nil
}

func HasPlayer(packID, character string) bool {
	if err := RecoverPaired(packID); err != nil {
		return false
	}
	p, err := PlayerPath(packID, character)
	if err != nil {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func ReadPlayer(packID, character string) (*PlayerSave, error) {
	if err := RecoverPaired(packID); err != nil {
		return nil, err
	}
	return readPlayerFile(packID, character)
}

func readPlayerFile(packID, character string) (*PlayerSave, error) {
	p, err := PlayerPath(packID, character)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var ps PlayerSave
	if err := json.Unmarshal(b, &ps); err != nil {
		return nil, err
	}
	if ps.Player == nil {
		return nil, fmt.Errorf("empty player save")
	}
	return &ps, nil
}

// AttachPlayer inserts a saved player and their carried entity subgraph into w.
func AttachPlayer(w *world.World, ps *PlayerSave) (*world.Entity, error) {
	if w == nil || ps == nil || ps.Player == nil {
		return nil, fmt.Errorf("missing world or player save")
	}
	pl := ps.Player.Clone()
	pl.Kind = world.KindPlayer
	if pl.Flags != nil {
		delete(pl.Flags, "linkdead")
	}

	// Ensure player ID does not collide with another player already in w.
	origID := pl.ID
	if origID == "" {
		slug, _ := Slug(ps.Character)
		if slug == "" {
			slug = "you"
		}
		origID = world.ID("player." + slug)
	}
	newID := origID
	if existing := w.Get(newID); existing != nil && !strings.EqualFold(existing.Name, pl.Name) {
		slug, _ := Slug(pl.Name)
		if slug == "" {
			slug = "player"
		}
		newID = world.ID("player." + slug)
		count := 1
		for {
			ex := w.Get(newID)
			if ex == nil || strings.EqualFold(ex.Name, pl.Name) {
				break
			}
			count++
			newID = world.ID(fmt.Sprintf("player.%s-%d", slug, count))
		}
	}
	pl.ID = newID

	// Remove any previous copy of this player subgraph before attaching.
	if w.Get(pl.ID) != nil {
		RemovePlayerSubgraph(w, pl.ID)
	}

	// Remap item IDs if any collide with existing entities in w.
	idRemap := map[world.ID]world.ID{}
	if origID != newID {
		idRemap[origID] = newID
	}
	bumpSeqFromID(w, pl.ID)
	for oldItemID, itemEnt := range ps.Entities {
		if itemEnt == nil {
			continue
		}
		bumpSeqFromID(w, oldItemID)
		if w.Get(oldItemID) != nil {
			w.NextSeq++
			base := string(oldItemID)
			if idx := strings.Index(base, "#"); idx >= 0 {
				base = base[:idx]
			}
			idRemap[oldItemID] = world.ID(fmt.Sprintf("%s#%d", base, w.NextSeq))
		}
	}

	remapID := func(id world.ID) world.ID {
		if mapped, ok := idRemap[id]; ok {
			return mapped
		}
		return id
	}

	for i, cid := range pl.Contents {
		pl.Contents[i] = remapID(cid)
	}
	for slot, eqID := range pl.Equipment {
		pl.Equipment[slot] = remapID(eqID)
	}
	pl.Parent = ""
	w.Add(pl)

	for _, itemEnt := range ps.Entities {
		if itemEnt == nil {
			continue
		}
		cp := itemEnt.Clone()
		cp.ID = remapID(cp.ID)
		cp.Parent = remapID(cp.Parent)
		for i, cid := range cp.Contents {
			cp.Contents[i] = remapID(cid)
		}
		w.Entities[cp.ID] = cp
	}

	destRoom := ps.RoomID
	if destRoom == "" || w.Get(destRoom) == nil {
		destRoom = w.StartRoom
	}
	if destRoom != "" && w.Get(destRoom) != nil {
		_ = w.Move(pl.ID, destRoom)
	}
	if w.PlayerID == "" {
		w.PlayerID = pl.ID
	}
	return pl, nil
}

// RemovePlayerSubgraph detaches and removes a player and their carried items from w.
func RemovePlayerSubgraph(w *world.World, playerID world.ID) {
	if w == nil || playerID == "" {
		return
	}
	pl := w.Get(playerID)
	if pl == nil {
		return
	}
	for _, other := range w.Entities {
		if other == nil {
			continue
		}
		if other.Combat != nil && other.Combat.Target == playerID {
			other.Combat.Target = ""
			other.Combat.NextSwing = 0
		}
		if other.Leader == playerID {
			other.Leader = ""
		}
		if other.PartyLeader == playerID {
			other.PartyLeader = ""
		}
		if len(other.Followers) > 0 {
			n := other.Followers[:0]
			for _, fid := range other.Followers {
				if fid != playerID {
					n = append(n, fid)
				}
			}
			other.Followers = n
		}
		if len(other.PartyInvites) > 0 {
			n := other.PartyInvites[:0]
			for _, pid := range other.PartyInvites {
				if pid != playerID {
					n = append(n, pid)
				}
			}
			other.PartyInvites = n
		}
	}
	w.Destroy(playerID)
	if w.PlayerID == playerID {
		w.PlayerID = ""
		for id, e := range w.Entities {
			if e != nil && e.Kind == world.KindPlayer {
				w.PlayerID = id
				break
			}
		}
	}
}

func bumpSeqFromID(w *world.World, id world.ID) {
	s := string(id)
	idx := strings.LastIndex(s, "#")
	if idx < 0 {
		return
	}
	if n, err := strconv.Atoi(s[idx+1:]); err == nil && n > w.NextSeq {
		w.NextSeq = n
	}
}
