package save

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"sudengine/internal/world"
)

type Snapshot struct {
	Version int          `json:"version"`
	PackID  string       `json:"pack_id"`
	SavedAt time.Time    `json:"saved_at"`
	Slot    string       `json:"slot"`
	World   *world.World `json:"world"`
}

// Root is the directory under which per-pack save folders are created.
var Root = "saves"

func Dir(packID string) string {
	return filepath.Join(Root, packID)
}

func Path(packID, slot string) string {
	return filepath.Join(Dir(packID), slot+".json")
}

func Write(packID, slot string, w *world.World) error {
	if err := os.MkdirAll(Dir(packID), 0o755); err != nil {
		return err
	}
	snap := Snapshot{
		Version: 1,
		PackID:  packID,
		SavedAt: time.Now().UTC(),
		Slot:    slot,
		World:   w,
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(packID, slot), b, 0o644)
}

func Read(packID, slot string) (*world.World, error) {
	b, err := os.ReadFile(Path(packID, slot))
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil, err
	}
	if snap.World == nil {
		return nil, fmt.Errorf("empty snapshot")
	}
	if snap.World.Entities == nil {
		snap.World.Entities = map[world.ID]*world.Entity{}
	}
	if snap.World.Protos == nil {
		snap.World.Protos = map[world.ID]*world.Entity{}
	}
	return snap.World, nil
}

type SlotInfo struct {
	Name    string
	ModTime time.Time
}

func List(packID string) ([]string, error) {
	infos, err := ListInfo(packID)
	if err != nil {
		return nil, err
	}
	slots := make([]string, len(infos))
	for i, s := range infos {
		slots[i] = s.Name
	}
	return slots, nil
}

func ListInfo(packID string) ([]SlotInfo, error) {
	entries, err := os.ReadDir(Dir(packID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var slots []SlotInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		info, err := e.Info()
		mod := time.Time{}
		if err == nil {
			mod = info.ModTime()
		}
		slots = append(slots, SlotInfo{Name: name[:len(name)-5], ModTime: mod})
	}
	sort.Slice(slots, func(i, j int) bool {
		return slots[i].ModTime.After(slots[j].ModTime)
	})
	return slots, nil
}
