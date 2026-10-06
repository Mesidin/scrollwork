package economy

import (
	"sudengine/internal/world"
)

// HasFinitePurse reports a till that stays limited when the balance is zero.
// Finite is set from a purse or max_purse key. Dynamic and max_purse count on their own.
func HasFinitePurse(npc *world.Entity) bool {
	if npc == nil || npc.Shop == nil {
		return false
	}
	s := npc.Shop
	return s.Finite || s.Dynamic || s.MaxPurse > 0
}

// VendorPurse returns the vendor's current purse and whether it is finite.
func VendorPurse(npc *world.Entity) (int, bool) {
	if !HasFinitePurse(npc) {
		return 0, false
	}
	return npc.Shop.Purse, true
}

// CanVendorAfford reports whether the merchant has enough funds to purchase an item for price.
func CanVendorAfford(npc *world.Entity, price int) bool {
	if !HasFinitePurse(npc) {
		return true
	}
	return npc.Shop.Purse >= price
}

// VendorPay deducts price from vendor's purse if finite.
func VendorPay(npc *world.Entity, price int) {
	if HasFinitePurse(npc) {
		npc.Shop.Purse -= price
		if npc.Shop.Purse < 0 {
			npc.Shop.Purse = 0
		}
	}
}

// VendorReceive adds price to vendor's purse if finite.
func VendorReceive(npc *world.Entity, price int) {
	if HasFinitePurse(npc) {
		npc.Shop.Purse += price
		if npc.Shop.MaxPurse > 0 && npc.Shop.Purse > npc.Shop.MaxPurse {
			npc.Shop.Purse = npc.Shop.MaxPurse
		}
	}
}

// BuyPrice calculates the price a player pays to buy an item from a vendor.
func BuyPrice(npc *world.Entity, item *world.Entity) int {
	base := item.Value
	if base < 1 {
		base = 1
	}
	if npc != nil && npc.Shop != nil && npc.Shop.Markup > 0 {
		base += (base * npc.Shop.Markup) / 100
	}
	return base
}

// SellPrice is what a vendor pays for an item. Zero means the vendor will not buy it.
// Damaged goods scale with remaining durability. A broken piece at zero durability pays nothing.
func SellPrice(item *world.Entity) int {
	if item == nil || item.Value < 1 {
		return 0
	}
	base := item.Value
	if item.MaxDurability > 0 && item.Durability < item.MaxDurability {
		base = (base * item.Durability) / item.MaxDurability
	}
	if base < 1 {
		return 0
	}
	if item.HasFlag("broken") {
		base = base / 4
		if base < 1 {
			return 0
		}
	}
	return base
}
