package rpg

import "testing"

func testSchema() Schema {
	return Schema{
		Attributes: []AttrDef{
			{Key: "body", Label: "Body", Base: 1, Max: 4},
			{Key: "will", Label: "Will", Base: 1, Max: 4},
		},
		Races: []Bundle{{
			ID:    "local",
			Name:  "Local",
			Attrs: map[string]int{"will": 1},
		}},
		Roles: []Bundle{{
			ID:   "careful",
			Name: "Careful",
		}},
		Chargen: Chargen{Enabled: true, PointBuy: 2},
	}
}

func TestAcceptChargenRequiresKnownChoicesAndCapsSpends(t *testing.T) {
	s := testSchema()

	if _, _, _, err := s.AcceptChargen("", "careful", nil, true); err == nil {
		t.Fatal("expected a missing origin to fail for a saved character")
	}
	race, role, spend, err := s.AcceptChargen("Local", "careful", map[string]int{"body": 1}, true)
	if err != nil {
		t.Fatal(err)
	}
	if race != "local" || role != "careful" || spend["body"] != 1 {
		t.Fatalf("got race %q role %q spend %v", race, role, spend)
	}

	if _, _, _, err := s.AcceptChargen("local", "careful", map[string]int{"body": 3}, true); err == nil {
		t.Fatal("expected body over max to fail")
	}
	if _, _, _, err := s.AcceptChargen("local", "careful", map[string]int{"body": 1, "will": 2}, true); err == nil {
		t.Fatal("expected a spend over the budget to fail")
	}
	if _, _, _, err := s.AcceptChargen("local", "careful", map[string]int{"luck": 1}, true); err == nil {
		t.Fatal("expected an unknown attribute to fail")
	}
	if _, _, _, err := s.AcceptChargen("local", "careful", map[string]int{"body": -1}, true); err == nil {
		t.Fatal("expected a negative spend to fail")
	}

	guestRace, guestRole, _, err := s.AcceptChargen("", "", nil, false)
	if err != nil || guestRace != "" || guestRole != "" {
		t.Fatalf("guest omit: race %q role %q err %v", guestRace, guestRole, err)
	}
	if _, _, _, err := s.AcceptChargen("nope", "", nil, false); err == nil {
		t.Fatal("expected an unknown origin to fail even for a guest")
	}
}

func TestAcceptChargenDropsSpendsWhenBudgetIsZero(t *testing.T) {
	s := testSchema()
	s.Chargen.PointBuy = 0
	s.Chargen.Points = 0
	_, _, spend, err := s.AcceptChargen("local", "careful", map[string]int{"body": 40}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(spend) != 0 {
		t.Fatalf("expected spends to be dropped, got %v", spend)
	}
}
