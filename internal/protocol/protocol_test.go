package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestJSONRoundTrip(t *testing.T) {
	ev := RoomEvent{
		ID:          "foyer",
		Title:       "The Foyer",
		Description: "A dusty foyer.",
		Exits:       []ExitInfo{{Dir: "north", Closed: true, Locked: true}},
		Occupants:   []string{"Ada (fighting)"},
		Items:       []string{"brass key"},
		X:           1,
		Y:           2,
		Z:           0,
		HasCoords:   true,
		Dark:        false,
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var round RoomEvent
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(ev, round) {
		t.Fatalf("expected %+v, got %+v", ev, round)
	}
}
