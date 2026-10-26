// Package protocol is the client/server session contract.
// The TUI speaks only these types; later a network transport can JSON-encode them.
package protocol

import (
	"fmt"
	"strings"
)

type Mode string

const (
	ModeLaunch Mode = "launch"
	ModePlay   Mode = "play"
	ModeBuild  Mode = "build"
)

type Channel string

const (
	ChanNarrative Channel = "narrative"
	ChanCombat    Channel = "combat"
	ChanAlert     Channel = "alert" // hostile arrival / first strike
	ChanSay       Channel = "say"
	ChanSystem    Channel = "system"
	ChanBuild     Channel = "build"
	ChanRoom      Channel = "room" // room title
)

// Event is a server-to-client message. Concrete structs below all implement it.
type Event interface {
	isEvent()
}

type TextEvent struct {
	Channel Channel `json:"channel"`
	Text    string  `json:"text"`
}

type PromptEvent struct {
	Text   string `json:"text"` // plain fallback: "Foyer Grit:24/24 > "
	Room   string `json:"room,omitempty"`
	Status string `json:"status,omitempty"`
	Build  bool   `json:"build,omitempty"`
}

type RoomEvent struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Exits       []ExitInfo `json:"exits,omitempty"`
	Occupants   []string   `json:"occupants,omitempty"`
	Items       []string   `json:"items,omitempty"`
	X           int        `json:"x"`
	Y           int        `json:"y"`
	Z           int        `json:"z"`
	HasCoords   bool       `json:"has_coords,omitempty"`
	Dark        bool       `json:"dark,omitempty"`
}

type ExitInfo struct {
	Dir    string `json:"dir"`
	Closed bool   `json:"closed,omitempty"`
	Locked bool   `json:"locked,omitempty"`
}

type ResourceView struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Current int    `json:"current"`
	Max     int    `json:"max"`
	Pile    bool   `json:"pile,omitempty"` // not a capped pool; show the amount alone
	Show    string `json:"show,omitempty"` // bar, side, or sheet
}

// Text is one resource for a pane. A bar is a meter. A pile is an amount.
func (r ResourceView) Text() string {
	if r.Pile {
		return fmt.Sprintf("%s %d", r.Label, r.Current)
	}
	if r.Show == "bar" {
		return fmt.Sprintf("%s %s %d/%d", r.Label, Meter(r.Current, r.Max, 10), r.Current, r.Max)
	}
	return fmt.Sprintf("%s %d/%d", r.Label, r.Current, r.Max)
}

// Meter draws a filled bar of width cells.
func Meter(current, max, width int) string {
	if width < 1 {
		width = 10
	}
	filled := 0
	if max > 0 && current > 0 {
		filled = current * width / max
		if filled < 1 {
			filled = 1
		}
		if filled > width {
			filled = width
		}
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

type VitalsEvent struct {
	Resources []ResourceView `json:"resources,omitempty"`
	Flags     []string       `json:"flags,omitempty"`
	InCombat  bool           `json:"in_combat,omitempty"`
	Round     int64          `json:"round,omitempty"`
}

type EquipView struct {
	Slot string `json:"slot"`
	Item string `json:"item"`
}

type InventoryEvent struct {
	Items     []string    `json:"items,omitempty"`
	Equipment []EquipView `json:"equipment,omitempty"`
}

type MapRoom struct {
	X     int      `json:"x"`
	Y     int      `json:"y"`
	Name  string   `json:"name"`
	Here  bool     `json:"here,omitempty"`
	Exits []string `json:"exits,omitempty"`
}

type MapEvent struct {
	Rooms   []MapRoom `json:"rooms,omitempty"`
	PlayerX int       `json:"player_x"`
	PlayerY int       `json:"player_y"`
	Text    string    `json:"text,omitempty"`
}

type FighterView struct {
	Name     string `json:"name"`
	HP       int    `json:"hp"`
	Max      int    `json:"max"`
	IsPlayer bool   `json:"is_player,omitempty"`
}

type CombatEvent struct {
	Active   bool          `json:"active"`
	Fighters []FighterView `json:"fighters,omitempty"`
}

type LayoutEvent struct {
	Panes []string `json:"panes"`
}

type ModeEvent struct {
	Mode Mode `json:"mode"`
}

type TitleEvent struct {
	Title string `json:"title"`
}

type DisconnectEvent struct {
	Reason string `json:"reason"`
}

func (TextEvent) isEvent()       {}
func (PromptEvent) isEvent()     {}
func (RoomEvent) isEvent()       {}
func (VitalsEvent) isEvent()     {}
func (InventoryEvent) isEvent()  {}
func (MapEvent) isEvent()        {}
func (CombatEvent) isEvent()     {}
func (LayoutEvent) isEvent()     {}
func (ModeEvent) isEvent()       {}
func (TitleEvent) isEvent()      {}
func (DisconnectEvent) isEvent() {}
