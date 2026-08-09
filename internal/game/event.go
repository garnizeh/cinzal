package game

// EventKind identifies what happened during resolution. RFC §6.7 calls Event
// the shared substrate for six consumers — the trail, the recap, email
// bodies, the debug trace, telemetry, and the narrated resolution list — and
// GDD §7.3 names the eight trail archetypes seeded below. Resolution steps
// not yet built (movement sub-steps, degradation, global events, incidents,
// pressure, upkeep — RFC §6.7, §11.5) will add their own kinds when they
// land; this enum is deliberately left open for that rather than closed here.
type EventKind uint8

const (
	_ EventKind = iota

	// EventCargoTaken is GDD §7.3's "Cargo left here." Named iff the actor's
	// Infamy is >= 3 at the moment it resolves (RFC §9.1 row 1). D12's Decoy
	// item reuses this exact kind, self-named under the same gate — see
	// Seat and Node below.
	EventCargoTaken

	// EventFreshTracks is GDD §7.3's "Fresh tracks." Never named (RFC §9.1)
	// — it is position information with no name attached, by design.
	EventFreshTracks

	// EventConfrontation is GDD §7.3's "Blood and broken glass." Always
	// named, both parties (RFC §9.1 row 7) — see Seat and Target.
	EventConfrontation

	// EventPostStaked is GDD §7.3's "Territory marked." Always named (RFC
	// §9.1 row 3).
	EventPostStaked

	// EventLoitering is GDD §7.3's "Someone's been standing here" (2+
	// consecutive rounds) and its 3+ round global escalation (GDD §9.1).
	// Never named — a node-only fix (RFC §9.1 row 5).
	EventLoitering

	// EventLeaseExpired is GDD §7.3's "The corner went quiet." Names the
	// owner (RFC §9.1 row 4) — fires identically whether the lease expired
	// on its own or was surrendered to Debt (GDD §15 Upkeep step 2).
	EventLeaseExpired

	// EventDelivered is GDD §7.3's delivery entry. Always a global
	// announcement naming the actor, tier, and location (RFC §9.1 row 2).
	EventDelivered

	// EventItemPurchased is GDD §7.3's "Someone worked the counter." Named
	// iff the actor's Infamy is >= 6 (RFC §9.1 row 8) — see Item.
	EventItemPurchased
)

// String returns the event kind's name, or "EventKind(n)" for an invalid
// value.
func (k EventKind) String() string {
	switch k {
	case EventCargoTaken:
		return "CargoTaken"
	case EventFreshTracks:
		return "FreshTracks"
	case EventConfrontation:
		return "Confrontation"
	case EventPostStaked:
		return "PostStaked"
	case EventLoitering:
		return "Loitering"
	case EventLeaseExpired:
		return "LeaseExpired"
	case EventDelivered:
		return "Delivered"
	case EventItemPurchased:
		return "ItemPurchased"
	default:
		return invalidEnumString("EventKind", int(k))
	}
}

// Event is one fact produced during Resolve. RFC §11.5's rendering contract
// fixes the shape: "kind, params", structured, never a rendered string —
// localisation and fog filtering both belong at the render edge, with a
// PlayerView in hand, and neither has any business happening here.
//
// Event is built inside Resolve with the full match state in hand, so Seat
// and Target are always the true actors — nothing here is fog-filtered.
// Whether another player ever learns a name from this fact is decided later,
// by writeTrail and Project, from this same value. Which of the fields below
// are meaningful depends on Kind; see the comment on each EventKind constant.
type Event struct {
	Kind  EventKind
	Round RoundNumber
	Node  NodeID

	// Seat is the acting player for every kind above.
	Seat SeatID

	// Target is the second party. Populated only for EventConfrontation.
	Target SeatID

	// Item is the item bought. Populated only for EventItemPurchased.
	Item ItemID

	// Contract is the contract fulfilled. Populated only for EventDelivered.
	Contract ContractID
}
