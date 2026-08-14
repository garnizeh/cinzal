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

	// The four kinds below are not in GDD §7.3's trail table — RFC §9.1
	// sources them from elsewhere (loose crate holding, the Infamy ladder,
	// and the Informants event card) and lists them as rows 6, 9, 10 and
	// 11 of the position-writer table. They land here ahead of the
	// resolution logic that will eventually produce them (#63, #65, #72),
	// because #75's Project — and the Anchor type in view.go — depends on
	// this vocabulary existing, not on that logic being built yet.

	// EventLooseCrateHeld is GDD §8.4's "run hot" escalation: a loose crate
	// held for a second consecutive round triggers a global announcement
	// of the holder's node (RFC §9.1 row 6). Never named, the same shape
	// as EventLoitering.
	EventLooseCrateHeld

	// EventTierFeared is the Feared tier's automatic position reveal,
	// fired at the end of every round Infamy sits at 6-8 (GDD §11, RFC
	// §9.1 row 9).
	EventTierFeared

	// EventTierLegend is the Legend tier's automatic position reveal,
	// fired for the whole order phase whenever Infamy sits at 9-10 (GDD
	// §11, RFC §9.1 row 10) — the ladder's steepest exposure cost.
	EventTierLegend

	// EventInformants is the Underworld global event card: every player's
	// current position is revealed to everyone, once (GDD §14.2, RFC §9.1
	// row 11).
	EventInformants

	// The four kinds below are Resolve's Step 0 vocabulary (GDD §15.0,
	// issue #67): "Orders never silently fail" — every rejection or
	// degradation produces one of these rather than a silent substitution.
	// None of the four is in RFC §9.1's authorised-writer table: they are
	// producer-side bookkeeping for the acting seat's own order, not a
	// fact about another seat's position, so no fog question arises for
	// them the way it does for the kinds above.

	// EventOrderRejected is Step 0's response to an illegal payload (GDD
	// §15.0's reject table): the whole order was discarded and the
	// absence default (§18) applied instead. Named — Seat only.
	EventOrderRejected

	// EventRouteTruncated is Step 0 degradation: a route step's edge no
	// longer exists (e.g. a prior round's Bridge Down). The route
	// truncated at Node, the last node still reachable, and the action
	// became Nothing (GDD §15.0 "Step 0").
	EventRouteTruncated

	// EventStakeTargetTaken is Step 0 degradation: a declared Stake Post
	// at Node is no longer legal because someone else already holds it.
	// The action became Nothing; the route is unaffected.
	EventStakeTargetTaken

	// EventPickupTargetGone is Step 0 degradation: a declared Pickup at
	// Node no longer has matching cargo on the ground. The action became
	// Nothing; the route is unaffected.
	EventPickupTargetGone

	// EventCurfewTruncated is Step 0 degradation (issue #72): the Curfew
	// global event card (GDD §14.2) reduces this round's step allowance by
	// 1, and the route submitted before that was known no longer fits.
	// The route truncated at Node, the last node still reachable under the
	// reduced allowance; the action became Nothing — the same shape as
	// EventRouteTruncated, a distinct kind because the cause (a step-
	// allowance change, not a destroyed edge) is a different "specific
	// reason" for the player to be notified of (GDD §15.0).
	EventCurfewTruncated

	// EventDeliveryBlocked is Step 0 degradation (issue #72): the Dragnet
	// global event card (GDD §14.2) seals Node, a Border, for this round —
	// a declared Deliver there cannot complete. The action became Nothing;
	// the route is unaffected.
	EventDeliveryBlocked

	// EventDeadRunnerCrate is the Underworld global event card's own
	// public announcement (GDD §14.2: "A crate appears at a random node,
	// announced publicly") — an Anchor-shaped fact, not a sight-gated
	// Trail row, naming Node only.
	EventDeadRunnerCrate

	// EventFenceWindfallAnnounced is the Economy global event card's own
	// public announcement (GDD §14.2: "One random Black Market,
	// announced publicly") — Node only, fired once when the standing
	// offer opens, independent of whoever eventually claims it.
	EventFenceWindfallAnnounced

	// EventSpilledLoadCrate is the Spilled Load sector incident's own
	// public announcement (GDD §14.3: "A crate appears at a random node
	// in the sector... Announced publicly") — an Anchor-shaped fact, not
	// a sight-gated Trail row, naming Node only. Mirrors
	// EventDeadRunnerCrate; kept a distinct kind because the two loose
	// crates carry different flat payouts (Cargo.SpilledLoad, cargo.go)
	// and a recap/telemetry consumer needs to tell them apart.
	EventSpilledLoadCrate

	// EventInformantRing is the Informant Ring sector incident (GDD
	// §14.3): every player ending in the flagged sector has their
	// position revealed publicly — the same position-reveal shape as
	// EventInformants (row 11), kept a distinct kind so recap/telemetry
	// can attribute the reveal to the incident rather than the global
	// event card.
	EventInformantRing
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
	case EventLooseCrateHeld:
		return "LooseCrateHeld"
	case EventTierFeared:
		return "TierFeared"
	case EventTierLegend:
		return "TierLegend"
	case EventInformants:
		return "Informants"
	case EventOrderRejected:
		return "OrderRejected"
	case EventRouteTruncated:
		return "RouteTruncated"
	case EventStakeTargetTaken:
		return "StakeTargetTaken"
	case EventPickupTargetGone:
		return "PickupTargetGone"
	case EventCurfewTruncated:
		return "CurfewTruncated"
	case EventDeliveryBlocked:
		return "DeliveryBlocked"
	case EventDeadRunnerCrate:
		return "DeadRunnerCrate"
	case EventFenceWindfallAnnounced:
		return "FenceWindfallAnnounced"
	case EventSpilledLoadCrate:
		return "SpilledLoadCrate"
	case EventInformantRing:
		return "InformantRing"
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

	// Tier is the contract tier delivered, 0-3 indexing Config.Contracts.
	// Populated only for EventDelivered — the delivery announcement always
	// names tier alongside actor and location (GDD §7.3), matching
	// Anchor's own Tier field (view.go), which this is the source of.
	Tier int
}
