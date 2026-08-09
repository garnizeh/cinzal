package game

import "strconv"

// invalidEnumString formats the fallback branch of every String() method in
// this package. Centralised so all of them fail the same recognisable way
// instead of each hand-rolling fmt.Sprintf.
func invalidEnumString(typeName string, v int) string {
	return typeName + "(" + strconv.Itoa(v) + ")"
}

// Every enum in this file reserves its zero value as invalid. A game.Order
// decoded from a malformed or partial payload — GDD §15.0's "illegal
// payload" case — must fail validation because a required field reads as
// unset, not silently succeed by reading as this package's first named
// constant.

// NodeType is a node's function on the map (GDD §6.2). Table order below
// matches D9's tie-break order for the allocation rounding rule.
type NodeType uint8

const (
	_ NodeType = iota

	NodeWarehouse   // 24% — Pickup (GDD §6.2, §9.2)
	NodeBorder      // 24% — Deliver, Cr$ 1 gate fee (GDD §6.2, §9.2)
	NodeBlackMarket // 20% — Deal, 3 rolled items (GDD §6.2, §9.2, §12)
	NodeAlley       // 32% — no economic function; +1 confrontation for an Aggressive occupant (GDD §6.2, §9.3)
)

// String returns the node type's GDD §6.2 name, or "NodeType(n)" for an
// invalid value.
func (t NodeType) String() string {
	switch t {
	case NodeWarehouse:
		return "Warehouse"
	case NodeBorder:
		return "Border"
	case NodeBlackMarket:
		return "Black Market"
	case NodeAlley:
		return "Alley"
	default:
		return invalidEnumString("NodeType", int(t))
	}
}

// Sector is one of the map's four districts (GDD §3). Order is ascending
// SectorID as D10 fixes it: assigned to canvas quadrants in reading order,
// top-left, top-right, bottom-left, bottom-right.
type Sector uint8

const (
	_ Sector = iota

	SectorOldDocks    // Blue-green, predominantly Warehouses (GDD §3)
	SectorIronLow     // Ochre, Alleys and Warehouses (GDD §3)
	SectorMistHeights // Violet, predominantly Black Markets (GDD §3)
	SectorNorthVale   // Dry red, predominantly Borders (GDD §3)
)

// String returns the sector's GDD §3 district name, or "Sector(n)" for an
// invalid value.
func (s Sector) String() string {
	switch s {
	case SectorOldDocks:
		return "Old Docks"
	case SectorIronLow:
		return "Iron Low"
	case SectorMistHeights:
		return "Mist Heights"
	case SectorNorthVale:
		return "North Vale"
	default:
		return invalidEnumString("Sector", int(s))
	}
}

// FogState is a player's discovery state for one node (GDD §7.1). Values are
// ordered Hidden < Rumoured < Known < InSight on purpose: fog only ever
// advances forward for a given player and node (GDD §7.2, "a visited node
// becomes Known permanently"), so callers can compare with >= rather than
// enumerating every state that qualifies.
type FogState uint8

const (
	_ FogState = iota

	FogHidden   // Nothing — not even that the node exists (GDD §7.1)
	FogRumoured // Name, type, sector, position — no edges (GDD §7.1)
	FogKnown    // Adds edges, post owner, lease expiry — not what happens there (GDD §7.1)
	FogInSight  // Adds this round's trail log, and market stock if a Black Market (GDD §7.1)
)

// String returns the fog state's GDD §7.1 name, or "FogState(n)" for an
// invalid value.
func (f FogState) String() string {
	switch f {
	case FogHidden:
		return "Hidden"
	case FogRumoured:
		return "Rumoured"
	case FogKnown:
		return "Known"
	case FogInSight:
		return "In sight"
	default:
		return invalidEnumString("FogState", int(f))
	}
}

// Stance is the one stance an Order declares, applying to any confrontation
// along the route (GDD §9.3). Order matches the GDD §9.3 table.
type Stance uint8

const (
	_ Stance = iota

	StanceAggressive // +1 confrontation, +1 more in an Alley; may stake Cr$ 0-6 (GDD §9.3)
	StanceNeutral    // No modifier; stake fixed at 0 (GDD §9.3)
	StanceEvasive    // −1 confrontation; keep cargo on a loss if the Cr$ 4 shakedown is paid (GDD §9.3)
)

// String returns the stance's GDD §9.3 name, or "Stance(n)" for an invalid
// value.
func (s Stance) String() string {
	switch s {
	case StanceAggressive:
		return "Aggressive"
	case StanceNeutral:
		return "Neutral"
	case StanceEvasive:
		return "Evasive"
	default:
		return invalidEnumString("Stance", int(s))
	}
}

// ActionKind is the action field of an Order (GDD §9.2). Order matches the
// GDD §9.2 table.
type ActionKind uint8

const (
	_ ActionKind = iota

	ActionPickup    // Warehouse matching a held contract, or dropped cargo you hold the contract for (GDD §9.2)
	ActionDeliver   // The destination Border, carrying the right cargo (GDD §9.2)
	ActionStakePost // Any unowned node, under your post cap (GDD §9.2, §10.3)
	ActionDeal      // Black Market — buy one of the three rolled items (GDD §9.2, §12)
	ActionVanish    // Anywhere — −2 Infamy, no "fresh tracks" trace this round (GDD §9.2)
	ActionSurveil   // Anywhere — sight of everything within 2 steps this round (GDD §9.2)
	ActionNothing   // No action taken; also the degradation fallback (GDD §9.2, §15.0)
)

// String returns the action's GDD §9.2 name, or "ActionKind(n)" for an
// invalid value.
func (a ActionKind) String() string {
	switch a {
	case ActionPickup:
		return "Pickup"
	case ActionDeliver:
		return "Deliver"
	case ActionStakePost:
		return "Stake Post"
	case ActionDeal:
		return "Deal"
	case ActionVanish:
		return "Vanish"
	case ActionSurveil:
		return "Surveil"
	case ActionNothing:
		return "Nothing"
	default:
		return invalidEnumString("ActionKind", int(a))
	}
}

// InfamyTier is the Infamy ladder's four titled bands (GDD §9.1, §8.2, §11).
// Order is ascending by Infamy so callers can compare with < and >=.
type InfamyTier uint8

const (
	_ InfamyTier = iota

	TierNobody // Infamy 0-2 (GDD §9.1)
	TierKnown  // Infamy 3-5 (GDD §9.1)
	TierFeared // Infamy 6-8 (GDD §9.1)
	TierLegend // Infamy 9-10 (GDD §9.1)
)

// String returns the tier's GDD §9.1 title, or "InfamyTier(n)" for an
// invalid value.
func (t InfamyTier) String() string {
	switch t {
	case TierNobody:
		return "Nobody"
	case TierKnown:
		return "Known"
	case TierFeared:
		return "Feared"
	case TierLegend:
		return "Legend"
	default:
		return invalidEnumString("InfamyTier", int(t))
	}
}
