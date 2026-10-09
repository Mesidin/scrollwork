package economy

import (
	"math"
	"strings"

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

// StockCounts calculates base stock and current stock for an item at a vendor.
// Base stock comes from npc.Shop.Stock. Current stock counts items carried by npc in w.
func StockCounts(w *world.World, npc *world.Entity, item *world.Entity) (baseStock, currentStock int) {
	if npc == nil || npc.Shop == nil || item == nil {
		return 0, 0
	}
	for _, sid := range npc.Shop.Stock {
		if listedAs(item, sid) {
			baseStock++
		}
	}
	if w != nil {
		for _, child := range w.Children(npc.ID) {
			if matchesItem(child, item) {
				currentStock++
			}
		}
	}
	return baseStock, currentStock
}

// listedAs reports whether a stock id names this good by prototype, instance id,
// or an exact keyword. A name or short that merely contains the id does not count.
func listedAs(item *world.Entity, sid string) bool {
	sid = strings.TrimSpace(sid)
	if item == nil || sid == "" {
		return false
	}
	if item.PrototypeID != "" && strings.EqualFold(string(item.PrototypeID), sid) {
		return true
	}
	if strings.EqualFold(string(item.ID), sid) {
		return true
	}
	for _, k := range item.Keywords {
		if strings.EqualFold(k, sid) {
			return true
		}
	}
	return false
}

func matchesItem(a, b *world.Entity) bool {
	if a == nil || b == nil {
		return false
	}
	if a.PrototypeID != "" && b.PrototypeID != "" {
		return a.PrototypeID == b.PrototypeID
	}
	if a.ID == b.ID {
		return true
	}
	if a.Name != "" && b.Name != "" && strings.EqualFold(a.Name, b.Name) {
		return true
	}
	return false
}

// ScalePrice applies the supply-and-demand price curve formula:
// Effective Price = Base Price * (1 + (Base Stock - Current Stock) / (Base Stock + 5))
// If the calculated price drops below 1, it is clamped to a nominal scrap value of 1.
func ScalePrice(basePrice int, baseStock, currentStock int) int {
	if basePrice <= 0 {
		return 0
	}
	num := float64(baseStock - currentStock)
	den := float64(baseStock + 5)
	multiplier := 1.0 + (num / den)
	scaled := int(math.Round(float64(basePrice) * multiplier))
	if scaled < 1 {
		scaled = 1
	}
	return scaled
}

// BuyPrice calculates the price a player pays to buy an item from a vendor.
// If npc.Shop.PriceCurve is enabled and a World is provided, supply-and-demand scaling is applied.
func BuyPrice(npc *world.Entity, item *world.Entity, optWorld ...*world.World) int {
	base := item.Value
	if base < 1 {
		base = 1
	}
	if npc != nil && npc.Shop != nil && npc.Shop.Markup > 0 {
		base += (base * npc.Shop.Markup) / 100
	}
	var w *world.World
	if len(optWorld) > 0 {
		w = optWorld[0]
	}
	if npc != nil && npc.Shop != nil && npc.Shop.PriceCurve && w != nil {
		baseStock, currentStock := StockCounts(w, npc, item)
		base = ScalePrice(base, baseStock, currentStock)
	}
	return base
}

// SellPrice is what a vendor pays for an item. Zero means the vendor will not buy it.
// Damaged goods scale with remaining durability. A broken piece at zero durability pays nothing.
// If an NPC vendor with PriceCurve and World are provided, supply-and-demand scaling is applied.
func SellPrice(item *world.Entity, opt ...any) int {
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
	var npc *world.Entity
	var w *world.World
	for _, o := range opt {
		switch v := o.(type) {
		case *world.Entity:
			npc = v
		case *world.World:
			w = v
		}
	}
	if npc != nil && npc.Shop != nil && npc.Shop.PriceCurve && w != nil {
		baseStock, currentStock := StockCounts(w, npc, item)
		base = ScalePrice(base, baseStock, currentStock)
	}
	return base
}
