package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// resolveActions is GDD §15's Step N+1: six of the seven §9.2 actions —
// Pickup, Stake Post, Deal, Vanish, Surveil, Nothing — resolved in ascending
// order of Infamy, the fairness key issue #58 built and this issue is its
// primary consumer of (byFairness, ordering.go). Deliver is deliberately not
// among them: GDD names it its own Step N+2 (resolveDeliveries,
// deliveries.go), and RFC §6.7 mirrors the split with two separate pipeline
// functions — Deliver is uncontested (nobody else can take your cargo out
// from under you), so it carries no fairness dimension at all.
//
// Every seat still carrying a non-Nothing Action at this point has a live
// Position equal to their route's true ending node: a confrontation loser's
// Action was already nulled by haltMovement (confront.go), so nothing here
// needs to re-derive "where did this seat actually end up" — s.Players
// [seat].Position already is that answer. This is what makes resolving one
// seat at a time, in fairness order, correct for the contended actions
// (Pickup, Stake Post, Deal): each seat's turn sees exactly what every
// earlier seat in this same order already did to s this round.
//
// The second return value is GDD §9.1's Vanish-qualifies-for-Loitering-
// exemption fact — "a Vanish qualifies only if it actually reduced your
// Infamy by at least 1" — keyed by seat, true only for a seat whose Action
// this round was Vanish and whose Infamy genuinely dropped (not floored at
// 0). RFC §6.6: "the delta is known in-flight" — this is that in-flight
// value, read once here where the mutation happens, rather than
// reconstructed later from a diff of before/after Infamy snapshots that
// would also have to account for this same round's confrontation deltas.
// writeTrail (#71) is its only consumer.
func resolveActions(s *MatchState, validated map[game.SeatID]game.Order, seats []game.SeatID, cfg game.Config, ctx globalEventContext, r *RNG) ([]game.Event, map[game.SeatID]bool) {
	var events []game.Event
	vanishReducedInfamy := map[game.SeatID]bool{}

	// Field 4 discard hand bookkeeping (confirmed minimal scope for this
	// issue): removing a declared discard from hand is what makes Deal's
	// own hand-limit precondition (legalHandLimit, legal.go — "len(v.You.
	// Items) - len(o.Items) + 1") hold at resolution as well as submission.
	// No item's discard *effect* (Guard Contact's Infamy loss, Torn Map's
	// reveal, Shiv/Circulation Permit/Bolt Hole arming beyond what
	// confront.go already reads directly, Decoy) fires here — that wiring
	// is out of this issue's scope. Order-independent: legalDiscards
	// already guarantees every declared discard names an item this seat
	// holds, so this can run before the fairness loop rather than
	// interleaved with it.
	for _, seat := range seats {
		applyDiscards(s, seat, validated[seat])
	}

	for _, seat := range byFairness(*s, r) {
		o := validated[seat]
		node := s.Players[seat].Position

		switch o.Action.Kind {
		case game.ActionPickup:
			if e := resolvePickup(s, seat, node, s.Round); e != nil {
				events = append(events, *e)
			}
			// Shipping Boom (GDD §14.2, issue #72): +Cr$5 on a Pickup at
			// this round's already-drawn overloaded Warehouse
			// (ctx.shippingBoomNode, buildGlobalEventContext) — fires
			// regardless of which Pickup source resolvePickup actually
			// used, matching the card text's plain "picking up there."
			if ctx.card == EventShippingBoom && ctx.live && node == ctx.shippingBoomNode && s.Players[seat].Cargo != nil {
				s.Players[seat].Balance += 5
			}

		case game.ActionStakePost:
			events = append(events, resolveStakePost(s, seat, node, o, cfg, ctx, s.Round)...)

		case game.ActionDeal:
			if e := resolveDeal(s, seat, node, o.Action.Item, s.Round); e != nil {
				events = append(events, *e)
			}

		case game.ActionVanish:
			p := &s.Players[seat]
			before := p.Infamy
			p.Infamy = ApplyInfamyDelta(p.Infamy, InfamyLossVanish)
			vanishReducedInfamy[seat] = p.Infamy < before

			// ActionSurveil and ActionNothing: no state mutation here.
			// Surveil's 2-step sight (GDD §7.2) is realized by writeTrail
			// (#71), reading validated[seat].Action.Kind directly — it
			// survives unchanged from here, so no MatchState field is
			// needed to carry it forward within this same Resolve call.
		}
	}

	return events, vanishReducedInfamy
}

// applyDiscards removes every item o declares as a Field 4 discard
// (game.ItemDiscard) from seat's hand, once per declared copy — the pure
// hand-bookkeeping half of GDD §9.4's discards. legalDiscards (legal.go)
// already guarantees every declared discard names an item this seat holds,
// no more times than the hand holds copies of it, so this never needs to
// guard against a missing entry.
func applyDiscards(s *MatchState, seat game.SeatID, o game.Order) {
	if len(o.Items) == 0 {
		return
	}
	p := &s.Players[seat]
	for _, d := range o.Items {
		p.Items = removeOneItem(p.Items, d.Item)
	}
}

// resolvePickup applies GDD §9.2's Pickup at node for seat, trying the
// ground-cargo source before the Warehouse source (groundCargoIndex,
// warehousePickupContract — cargo.go) so a genuine dropped crate never
// lingers uncollected under an inexhaustible Warehouse source sitting on
// the same node. Returns nil — no state change, no event — when the seat's
// cargo slot is already full, or neither source has anything for this seat:
// the latter is the contested case ("two players contest a single dropped
// crate: the fairness key decides") for a seat who loses the race to an
// earlier seat this same fairness order already served.
func resolvePickup(s *MatchState, seat game.SeatID, node game.NodeID, round game.RoundNumber) *game.Event {
	p := &s.Players[seat]
	if p.Cargo != nil {
		return nil
	}

	if idx, ok := groundCargoIndex(*s, seat, node); ok {
		ground := s.Graph.Cargo[idx]
		cargo := game.CarriedCargo{Bound: ground.Bound, SpilledLoad: ground.SpilledLoad}
		if ground.Bound {
			for _, c := range p.Contracts {
				if c.Origin == ground.Origin && c.Destination == ground.Destination {
					cargo.Contract = c.ID
					break
				}
			}
		}
		p.Cargo = &cargo
		s.Graph.Cargo = slices.Delete(s.Graph.Cargo, idx, idx+1)
		return &game.Event{Kind: game.EventCargoTaken, Round: round, Node: node, Seat: seat}
	}

	if contract, ok := warehousePickupContract(*s, seat, node); ok {
		p.Cargo = &game.CarriedCargo{Bound: true, Contract: contract.ID}
		return &game.Event{Kind: game.EventCargoTaken, Round: round, Node: node, Seat: seat}
	}

	return nil
}

// resolveStakePost applies GDD §9.2/§10.4's Stake Post at node for seat.
// o.AddOns.RenewBlocks/RenewPost do double duty (affordance.go): on a
// StakePost order they name this fresh post's own block count, fully
// consumed here — resolveAddons (addons.go) skips any seat whose Action
// this round was StakePost, so the same declaration is never processed
// twice.
//
// Re-verifies ownership and the post cap against live state rather than
// trusting Step 0's pre-movement check (validate.go): another seat earlier
// in this same fairness order may have staked node this round, or (in
// principle) the cap could differ from what Legal saw at submission. A
// target already taken reuses EventStakeTargetTaken — the same meaning as
// Step 0's own degradation event, just detected at Step N+1 instead — and
// a seat at the cap gets silent no-op, matching D14's "Open Doors"
// precedent for a legal declaration whose target moved under it
// (docs/decisions/D14-five-resolution-gaps.md).
func resolveStakePost(s *MatchState, seat game.SeatID, node game.NodeID, o game.Order, cfg game.Config, ctx globalEventContext, round game.RoundNumber) []game.Event {
	if s.Graph.Nodes[node].Post != nil {
		return []game.Event{{Kind: game.EventStakeTargetTaken, Round: round, Node: node, Seat: seat}}
	}

	if _, reached := PostCapReached(len(s.Players[seat].Posts), len(s.Players), cfg); reached {
		return nil
	}

	// cost: leaseCostPerBlock (events.go) applies Permit Auction's
	// discounted rate (GDD §14.2, issue #72) when live this round — "lease
	// blocks," not "renewals only," so a fresh stake's own block count is
	// discounted identically to resolveRenewal's (addons.go).
	cost := o.AddOns.RenewBlocks * leaseCostPerBlock(cfg, ctx)
	_, debtEvent := applyDebt(s, seat, cost, round)

	// Read after applyDebt, not before: a Debt-surrendered lease (GDD §13
	// step 3) can be the seat's only other post in this sector, which
	// would flip this from false to true — computing it earlier could
	// wrongly deny GDD §11's "stake your first post in a sector +1".
	firstInSector := FirstPostInSector(*s, seat, node)

	rounds := RenewedRoundsRemaining(0, o.AddOns.RenewBlocks, cfg)
	s.Graph.Nodes[node].Post = &Post{Owner: seat, RoundsRemaining: rounds}

	p := &s.Players[seat]
	p.Posts = append(p.Posts, node)
	slices.Sort(p.Posts) // Player.Posts: "ascending NodeID" (state.go)
	if firstInSector {
		p.Infamy = ApplyInfamyDelta(p.Infamy, InfamyGainFirstPost)
	}

	events := []game.Event{{Kind: game.EventPostStaked, Round: round, Node: node, Seat: seat}}
	if debtEvent != nil {
		events = append(events, *debtEvent)
	}
	return events
}

// resolveDeal applies GDD §9.2/§12's Deal at node for seat: buying item from
// the Black Market's current stock. Skips silently — no purchase, no event,
// no Debt (confirmed: Deal is a discretionary purchase, absent from GDD
// §13's Debt-trigger list) — when item is no longer in the market's stock
// (an earlier seat this same fairness order already bought it: D14's "Open
// Doors" precedent again) or seat's balance can no longer cover the price.
func resolveDeal(s *MatchState, seat game.SeatID, node game.NodeID, item game.ItemID, round game.RoundNumber) *game.Event {
	market := s.Graph.Nodes[node].Market
	idx := slices.Index(market, item)
	if idx < 0 {
		return nil
	}

	price := itemPrice(item)
	p := &s.Players[seat]
	if p.Balance < price {
		return nil
	}

	s.Graph.Nodes[node].Market = slices.Delete(slices.Clone(market), idx, idx+1)
	p.Balance -= price
	p.Items = append(p.Items, item)

	return &game.Event{Kind: game.EventItemPurchased, Round: round, Node: node, Seat: seat, Item: item}
}
