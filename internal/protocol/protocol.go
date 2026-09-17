// Package protocol is the client/server session contract.
// The TUI speaks only these types; later a network transport can JSON-encode them.
package protocol

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
	Channel Channel
	Text    string
}

type PromptEvent struct {
	Text   string // plain fallback: "Foyer Grit:24/24 > "
	Room   string
	Status string // "Grit:24/24"
	Build  bool
}

type RoomEvent struct {
	ID          string
	Title       string
	Description string
	Exits       []ExitInfo
	Occupants   []string
	Items       []string
	X, Y, Z     int
	HasCoords   bool
	Dark        bool
}

type ExitInfo struct {
	Dir    string
	Closed bool
	Locked bool
}

type ResourceView struct {
	Key          string
	Label        string
	Current, Max int
}

type VitalsEvent struct {
	Resources []ResourceView
	Flags     []string
	InCombat  bool
	Round     int64
}

type EquipView struct {
	Slot string
	Item string
}

type InventoryEvent struct {
	Items     []string
	Equipment []EquipView
}

type MapRoom struct {
	X, Y  int
	Name  string
	Here  bool
	Exits []string
}

type MapEvent struct {
	Rooms            []MapRoom
	PlayerX, PlayerY int
	Text             string
}

type FighterView struct {
	Name     string
	HP, Max  int
	IsPlayer bool
}

type CombatEvent struct {
	Active   bool
	Fighters []FighterView
}

type LayoutEvent struct {
	Panes []string
}

type ModeEvent struct {
	Mode Mode
}

type TitleEvent struct {
	Title string
}

type DisconnectEvent struct {
	Reason string
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
