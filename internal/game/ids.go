package game

// Identifier types. Each wraps a plain Go type so the fog boundary is a type
// boundary too: a NodeID that were a bare int could end up assigned from a
// seat index or a round number by accident, and the compiler would not
// notice. See D01 and issue #53.

// MatchID identifies a match. It is opaque outside internal/store, which owns
// how it is generated and persisted (RFC §7.2). It is a string rather than a
// third-party UUID type because internal/game may import nothing outside the
// standard library (D01) — the leaf rule that keeps render and web unable to
// name the match state.
type MatchID string

// SeatID identifies a seat at the table, 0-indexed. RFC §6.5 uses seat index
// as the default fairness ordering, and the schema keys match_players and
// orders on (match_id, seat) (RFC §7.2) — so SeatID is a small integer, not a
// generated identifier.
type SeatID int

// NodeID identifies a node in the generated map graph, 0-indexed. RFC §6.4
// and §6.5 sort candidate sets ascending by NodeID as the canonical
// tie-break; that ordering only means something if NodeID is a stable index
// rather than an opaque token.
type NodeID int

// ContractID identifies a contract instance held by a seat. GDD §8.1 caps a
// player at 2 active contracts and RFC §6.7 orders upkeep "a contract's
// assigned ID across a seat's own two slots" — contracts are per-player
// instances (GDD §8.4), not match-global objects, so ContractID is scoped to
// one seat's slots, not unique across the whole match.
type ContractID int

// RoundNumber identifies a round, 1-indexed (GDD §4: 15 rounds per match). It
// is also the staleness field on Order (RFC §11.1a): the round the order form
// was rendered for, checked against the round the order resolves in.
type RoundNumber int

// ItemID identifies one of the eight Black Market items (GDD §12). Unlike the
// other identifiers in this file, its value space is closed and fixed rather
// than generated per match — items in hand are tracked by type, never by a
// unique instance — but it is grouped here because it is exactly the same
// shape of type: a named identifier, never a bare int.
type ItemID uint8

// The zero value of ItemID is deliberately invalid (see enums.go for the same
// convention applied to the other enums in this package): a zero-valued
// ItemDiscard.Item that was never explicitly set is rejected rather than
// silently read as Shiv.
const (
	_ ItemID = iota

	ItemShiv              // Cr$ 4  — discard: +3 in one confrontation (GDD §12)
	ItemMuscle            // Cr$ 7  — permanent: +1 in all confrontations, lost on a loss (GDD §12)
	ItemPoliceBand        // Cr$ 3  — discard: full sight of one node this round (GDD §12)
	ItemCirculationPermit // Cr$ 5  — discard: immune to this round's Sector Incident and gate fee (GDD §12)
	ItemTornMap           // Cr$ 3  — discard: reveal 4 random Hidden nodes as Known (GDD §12)
	ItemDecoy             // Cr$ 5  — discard: plant a false "Cargo left here" trace (GDD §12, D12)
	ItemBoltHole          // Cr$ 5  — discard: on losing a confrontation, retreat to a declared node (GDD §12)
	ItemGuardContact      // Cr$ 6  — discard: −3 Infamy immediately (GDD §12)
)

// String returns the item's GDD §12 name, or "ItemID(n)" for an invalid value.
func (i ItemID) String() string {
	switch i {
	case ItemShiv:
		return "Shiv"
	case ItemMuscle:
		return "Muscle"
	case ItemPoliceBand:
		return "Police Band"
	case ItemCirculationPermit:
		return "Circulation Permit"
	case ItemTornMap:
		return "Torn Map"
	case ItemDecoy:
		return "Decoy"
	case ItemBoltHole:
		return "Bolt Hole"
	case ItemGuardContact:
		return "Guard Contact"
	default:
		return invalidEnumString("ItemID", int(i))
	}
}
