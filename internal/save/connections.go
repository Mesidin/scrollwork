package save

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Connection stores a saved remote server bookmark and character login so
// the launcher can reconnect directly to a world as that character.
type Connection struct {
	Addr      string    `json:"addr"`
	PackID    string    `json:"pack_id,omitempty"`
	Title     string    `json:"title"`
	Account   string    `json:"account"`
	Password  string    `json:"password,omitempty"`
	Character string    `json:"character"`
	Race      string    `json:"race,omitempty"`
	Role      string    `json:"role,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Label returns the launcher display label: "<World Title> — <Character>".
func (c Connection) Label() string {
	worldName := strings.TrimSpace(c.Title)
	if worldName == "" {
		worldName = strings.TrimSpace(c.PackID)
	}
	if worldName == "" {
		worldName = c.Addr
	}
	if strings.TrimSpace(c.Character) != "" {
		return worldName + " — " + c.Character
	}
	return worldName
}

func connectionsPath() string {
	return filepath.Join(Root, "connections.json")
}

// LoadConnections reads saved remote connections from saves/connections.json.
func LoadConnections() ([]Connection, error) {
	b, err := os.ReadFile(connectionsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var list []Connection
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// SaveConnection inserts or updates a saved connection entry at the top of saves/connections.json.
func SaveConnection(entry Connection) error {
	entry.Addr = strings.TrimSpace(entry.Addr)
	entry.Account = strings.TrimSpace(entry.Account)
	entry.Character = strings.TrimSpace(entry.Character)
	if entry.Addr == "" || entry.Character == "" {
		return nil
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = time.Now().UTC()
	}
	existing, _ := LoadConnections()
	out := make([]Connection, 0, len(existing)+1)
	out = append(out, entry)
	for _, c := range existing {
		if sameConnection(c, entry) {
			continue
		}
		out = append(out, c)
	}
	return writeConnections(out)
}

// RemoveConnection deletes a saved connection matching target from saves/connections.json.
func RemoveConnection(target Connection) error {
	existing, err := LoadConnections()
	if err != nil || len(existing) == 0 {
		return err
	}
	out := make([]Connection, 0, len(existing))
	for _, c := range existing {
		if sameConnection(c, target) {
			continue
		}
		out = append(out, c)
	}
	return writeConnections(out)
}

func sameConnection(a, b Connection) bool {
	return strings.EqualFold(strings.TrimSpace(a.Addr), strings.TrimSpace(b.Addr)) &&
		strings.EqualFold(strings.TrimSpace(a.Account), strings.TrimSpace(b.Account)) &&
		strings.EqualFold(strings.TrimSpace(a.Character), strings.TrimSpace(b.Character))
}

func writeConnections(list []Connection) error {
	if err := os.MkdirAll(Root, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	path := connectionsPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := restrictPrivateFile(tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return restrictPrivateFile(path)
}
