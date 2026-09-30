# Feature Specification: Expanded Economy, Crafting & Durability

## 1. Overview & Goals

Standard MUD economies often suffer from run-away inflation: merchants have infinite currency, buy endless player vendor-trash, and items last forever. This specification outlines an expanded, closed-loop economy system for Sudengine that introduces currency sinks, dynamic merchant trading, equipment wear-and-tear, and crafting pipelines.

---

## 2. Dynamic Merchants & Supply/Demand

### A. Merchant Purses & Local Stock
Instead of static buy/sell tables, merchants can behave as active economic agents:
* **Vendor Purse**: Merchants hold currency resources (e.g. `currency: 150` in their entity attributes or resources).
* **Inventory Depletion**: Selling an item to a merchant adds it to their stock; buying an item removes it from their stock.
* **Refusal / Empty Purse**: If a merchant has insufficient currency, they refuse to buy high-value goods until they make sales.

### B. Price Scaling (Floating Supply & Demand)
* Price curves based on stock count:
  $$\text{Effective Price} = \text{Base Price} \times \left(1 + \frac{\text{Base Stock} - \text{Current Stock}}{\text{Base Stock} + 5}\right)$$
* Flooding a vendor with 15 daggers drops the vendor's buy-price down to nominal scrap value.
* Rare goods purchased by the player become increasingly expensive to replace.

---

## 3. Item Durability & Repair Sinks

A primary gold and material sink in MUDs is equipment degradation.

### A. Durability Attributes
Items in `world/items.yaml` can define durability fields:
```yaml
id: hollow.iron_sword
name: an iron broadsword
kind: item
slots: [wield]
damage: 2d6
durability: 50
max_durability: 50
repairable: true
```

### B. Wear in Combat
* In [`internal/combat/combat.go`](file:///C:/dev/projects/scrollwork/internal/combat/combat.go), round strikes decrement wielded weapon durability on critical rolls or every $N$ successful strikes.
* Defending armor takes durability loss when soaking heavy hits.
* **Degraded States**:
  * Durability $> 50\%$: Full combat effectiveness.
  * Durability $\le 20\%$: *"The blade is nicked and dull"* (damage or soak penalty).
  * Durability $= 0$: Broken (`flags: [broken]`). The item cannot be wielded until repaired.

### C. Repair Mechanic
* **NPC Blacksmith**: `repair <item>` verb checks NPC role `repairer` or shop type. Deducts pack currency from the player and restores durability.
* **Field Repair Kits**: Consumable items with limited charges allowing partial restoration in the field.

---

## 4. Crafting & Material Pipelines

```
[ Harvesting / Nodes ]  ──►  [ Processing / Refining ]  ──►  [ Assembly / Smithing ]
   (Iron Ore Vein)                (Forge / Smelter)              (Anvil / Workbench)
         │                               │                               │
         ▼                               ▼                               ▼
    "mine vein"                    "smelt ore"                    "craft sword"
   -> Raw Ore                      -> Iron Ingot                 -> Iron Broadsword
```

### A. Recipe Schema (`recipes.yaml`)
```yaml
recipes:
  - id: forge_iron_sword
    name: "iron broadsword"
    station: anvil
    tools: [hollow.hammer]
    requires:
      - item: hollow.iron_ingot
        count: 2
      - item: hollow.leather_strip
        count: 1
    skill: smithing
    difficulty: 12
    produces:
      item: hollow.iron_sword
      count: 1
```

### B. Crafting Verbs
* `recipes` / `recipes <category>`: Lists known or discoverable blueprints.
* `craft <recipe>`: Checks nearby station entity (e.g. an anvil in the room), required tools in inventory, and consumes reagents.
* Resolves a skill check ([`rpg.Check`](file:///C:/dev/projects/scrollwork/internal/rpg/check.go)) against character skill:
  * Critical success: Yields a masterwork item (bonus damage or durability).
  * Failure: Consumes partial materials; item not created.

---

## 5. Currency Realism & Physical Money

* **Multi-Denomination**: Packs can define fractional coinage (e.g., 100 copper = 10 silver = 1 gold).
* **Coin Encumbrance**: Currency optionally incurs weight. Large wealth requires bank vaults or conversion to high-value gems/bullion.
* **Banking / Safes**: Storage entities (`container` with `locked: true`) in inns or banks where players deposit surplus funds or rare equipment.

---

## 6. Implementation Stages

* **Stage 1 (Durability & Repair)**: Add item durability fields, combat round wear checks, and the `repair` verb for blacksmith NPCs.
* **Stage 2 (Merchant Stock & Purses)**: Finite vendor currency and inventory transfer on transaction.
* **Stage 3 (Crafting Engine)**: Station entities, recipe definitions, consumption checks, and skill-based production.
