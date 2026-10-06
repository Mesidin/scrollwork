package economy

import (
	"fmt"

	"sudengine/internal/world"
)

// Condition returns a textual representation of an item's durability condition.
func Condition(durability, maxDurability int, broken bool) string {
	if maxDurability <= 0 {
		return "indestructible"
	}
	if broken || durability <= 0 {
		return "broken"
	}
	ratio := float64(durability) / float64(maxDurability)
	switch {
	case ratio >= 0.99:
		return "pristine"
	case ratio >= 0.70:
		return "good"
	case ratio >= 0.35:
		return "worn"
	default:
		return "damaged"
	}
}

// DurabilityTag returns a short bracketed tag, e.g. "(45/50)" or "(broken)".
func DurabilityTag(it *world.Entity) string {
	if it == nil || it.MaxDurability <= 0 {
		return ""
	}
	if it.HasFlag("broken") || it.Durability <= 0 {
		return "(broken)"
	}
	return fmt.Sprintf("(%d/%d)", it.Durability, it.MaxDurability)
}

// Degrade decrements an item's durability by amount, marking it broken if it reaches 0.
// Returns true if the item broke on this hit.
func Degrade(it *world.Entity, amount int) (brokenNow bool) {
	if it == nil || it.MaxDurability <= 0 || it.HasFlag("broken") {
		return false
	}
	if amount <= 0 {
		amount = 1
	}
	it.Durability -= amount
	if it.Durability <= 0 {
		it.Durability = 0
		if it.Flags == nil {
			it.Flags = map[string]bool{}
		}
		it.Flags["broken"] = true
		return true
	}
	return false
}

// RepairCost is the currency to restore an item to full durability.
// A valued item costs (missing * value) / (max * 2). Anything else costs half the missing points.
func RepairCost(it *world.Entity) int {
	if it == nil || it.MaxDurability <= 0 {
		return 0
	}
	missing := it.MaxDurability - it.Durability
	if missing <= 0 {
		return 0
	}
	if it.Value > 0 {
		cost := (missing * it.Value) / (it.MaxDurability * 2)
		if cost < 1 {
			cost = 1
		}
		return cost
	}
	cost := missing / 2
	if cost < 1 {
		cost = 1
	}
	return cost
}

// Repair restores an item to its maximum durability and clears the broken flag.
func Repair(it *world.Entity) bool {
	if it == nil || it.MaxDurability <= 0 {
		return false
	}
	it.Durability = it.MaxDurability
	if it.Flags != nil {
		delete(it.Flags, "broken")
	}
	return true
}
