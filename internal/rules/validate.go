package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// validate is Resolve's Step 0 (GDD §15.0, RFC §6.7): every submitted order
// is checked against s — the live match state, with entry's frozen
// Balance/Position standing in for "what this order was validated against"
// (RFC §6.6) — and comes out one of three ways: unchanged, degraded, or
// discarded wholesale for the absence default (GDD §18). It returns one
// order per seat in seats, plus every game.Event Step 0 produced ("Orders
// never silently fail").
//
// A missing entry in orders is not an error — it is GDD §18's ordinary
// absent-player case (a deadline that expired with nothing submitted) —
// and resolves straight to absenceDefault with no further checking.
//
// Reject vs. degrade is decided by *order of checks*, not by two
// independently-sourced views of the world: MatchState keeps exactly one
// live Graph.Nodes[i].Edges, so there is no separate "what the client's
// form was rendered against" copy to compare live state to, and a missing
// edge cannot be told apart from "never adjacent" by data alone. This
// function always resolves that ambiguity toward degradation — a route is
// walked against live edges and truncated at the first missing one before
// Legal ever runs — because a truncation is strictly less destructive than
// a reject, and GDD §15.0 names exactly this case (Bridge Down) as its own
// canonical degradation example. Legal then runs on whatever survived the
// truncation: a failure there is a property of a strict prefix of what was
// submitted, so it was illegal from the start and the whole order is
// rejected; a pass means the order degrades (truncated route, Action and
// PushingOn both cleared — PushingOn too, or a route that lost its Hidden
// ending to truncation would fail Legal's own Hidden-must-be-last check
// and a legitimate degrade would be misread as a reject).
//
// Two further degradation checks have no Legal equivalent at all, by
// Legal's own documentation (legal.go): a Stake Post target already owned,
// and a Pickup with neither GDD §9.2 source available — no matching
// Warehouse contract and no collectible dropped cargo (pickupAvailable,
// cargo.go). Both are checked here, against live state, after the
// route/Legal pass.
//
// Not handled here: GDD §15.0's third named example, "funds went to a
// stake that was lost". Balance is untouched at Step 0 — nothing has spent
// it yet, since resolveAddons (#70) runs after actions and deliveries —
// so there is nothing to degrade against yet; that check belongs to #70.
//
// blockedBorders is passed separately from ctx (rather than read off
// ctx.sealedBorders directly) because it is a wider set than Dragnet
// alone: Resolve computes it as Dragnet's seal unioned with two-player
// rotation's inactive set, D28's safety valve applied (borders.go). ctx
// keeps sealedBorders' own narrower, Dragnet-only meaning.
func validate(s MatchState, entry EntrySnapshot, seats []game.SeatID, orders map[game.SeatID]game.Order, cfg game.Config, ctx globalEventContext, blockedBorders []game.NodeID) (map[game.SeatID]game.Order, []game.Event) {
	validated := make(map[game.SeatID]game.Order, len(seats))
	var events []game.Event

	for _, seat := range seats {
		o, submitted := orders[seat]
		if !submitted {
			validated[seat] = absenceDefault(s, seat)
			continue
		}

		truncated, cut := truncateAtDestroyedEdge(s, seat, o.Route)
		candidate := o
		candidate.Route = truncated
		if cut {
			candidate.Action = game.ActionOrder{Kind: game.ActionNothing}
			candidate.PushingOn = game.PushingOn{}
		}

		// Curfew (issue #72, GDD §14.2/§15.0): checked only when the edge
		// truncation above did not already fire — a route already cut for
		// a destroyed edge is re-evaluated against Legal below exactly as
		// before, and does not also need the Curfew comparison. See
		// truncateForCurfew's own doc for why this degrades rather than
		// rejects.
		curfewCut := false
		if !cut && ctx.curfewActive {
			var curfewTruncated []game.NodeID
			curfewTruncated, curfewCut = truncateForCurfew(s, entry, seat, candidate.Route, cfg)
			if curfewCut {
				candidate.Route = curfewTruncated
				candidate.Action = game.ActionOrder{Kind: game.ActionNothing}
				candidate.PushingOn = game.PushingOn{}
			}
		}

		if err := Legal(legalView(s, entry, seat, ctx.curfewActive), candidate, cfg); err != nil {
			validated[seat] = absenceDefault(s, seat)
			events = append(events, game.Event{Kind: game.EventOrderRejected, Round: s.Round, Seat: seat})
			continue
		}

		if cut {
			validated[seat] = candidate
			events = append(events, game.Event{
				Kind:  game.EventRouteTruncated,
				Round: s.Round,
				Seat:  seat,
				Node:  routeEndingNode(s, seat, truncated),
			})
			continue
		}

		if curfewCut {
			validated[seat] = candidate
			events = append(events, game.Event{
				Kind:  game.EventCurfewTruncated,
				Round: s.Round,
				Seat:  seat,
				Node:  routeEndingNode(s, seat, candidate.Route),
			})
			continue
		}

		degraded, reason, ok := checkActionDegradation(s, seat, candidate, blockedBorders)
		if !ok {
			validated[seat] = degraded
			events = append(events, reason)
			continue
		}

		validated[seat] = candidate
	}

	return validated, events
}

// truncateForCurfew truncates route to this round's Curfew-reduced step
// allowance (GDD §14.2, §15.0) when route was legal against the allowance as
// it stood without Curfew — the allowance the player actually saw when they
// submitted — but exceeds it once Curfew's live -1 is applied. This is
// "legal at submission, the world moved under it" (a global event card only
// known once Resolve peeks it, after submission), not an illegal payload:
// GDD §15.0's ordinary "route longer than your step allowance" reject row is
// for a route that was never legal in the first place, which is exactly
// what the second comparison below still catches. Reports false — nothing
// to truncate — when route already fits the Curfew allowance, or still
// exceeds the allowance even without Curfew.
func truncateForCurfew(s MatchState, entry EntrySnapshot, seat game.SeatID, route []game.NodeID, cfg game.Config) ([]game.NodeID, bool) {
	withCurfew := Steps(legalView(s, entry, seat, true), cfg)
	if len(route) <= withCurfew {
		return route, false
	}
	withoutCurfew := Steps(legalView(s, entry, seat, false), cfg)
	if len(route) > withoutCurfew {
		return route, false
	}
	return route[:withCurfew], true
}

// absenceDefault is GDD §18's default order on absence, reused verbatim by
// Step 0 for the illegal-payload reject path too (GDD §15.0: "Rejection
// falls back to the absence default rather than to a partial execution"):
// an empty route (stay put), Deliver if canAutoDeliverHere finds this seat
// already standing on the right Border with the right cargo, Nothing
// otherwise, Evasive stance, and no add-ons.
func absenceDefault(s MatchState, seat game.SeatID) game.Order {
	action := game.ActionOrder{Kind: game.ActionNothing}
	if canAutoDeliverHere(s, seat) {
		action = game.ActionOrder{Kind: game.ActionDeliver}
	}
	return game.Order{
		Action: action,
		Stance: game.StanceOrder{Stance: game.StanceEvasive},
	}
}

// canAutoDeliverHere reports whether seat, standing exactly where it is
// right now, could Deliver: live-state mirror of legal.go's
// deliverableCargo, needed here because absenceDefault has no
// game.PlayerView to read — only live MatchState.
func canAutoDeliverHere(s MatchState, seat game.SeatID) bool {
	p := s.Players[seat]
	if s.Graph.Nodes[p.Position].Type != game.NodeBorder {
		return false
	}
	if p.Cargo == nil {
		return false
	}
	if !p.Cargo.Bound {
		return true
	}
	for _, c := range p.Contracts {
		if c.ID == p.Cargo.Contract && c.Destination == p.Position {
			return true
		}
	}
	return false
}

// truncateAtDestroyedEdge walks route from seat's current live position
// through s.Graph's live edges, stopping at the first step whose edge no
// longer exists. It returns the surviving prefix and whether truncation
// occurred. See validate's own doc comment for why this always means
// degradation, never rejection.
func truncateAtDestroyedEdge(s MatchState, seat game.SeatID, route []game.NodeID) ([]game.NodeID, bool) {
	current := s.Players[seat].Position
	for i, to := range route {
		if !slices.Contains(s.Graph.Nodes[current].Edges, to) {
			return route[:i], true
		}
		current = to
	}
	return route, false
}

// routeEndingNode returns where route leaves seat, or seat's own current
// position for an empty route — the same "ending node" convention
// legal.go's endingNode uses, needed here to name the truncation point on
// EventRouteTruncated.
func routeEndingNode(s MatchState, seat game.SeatID, route []game.NodeID) game.NodeID {
	if len(route) == 0 {
		return s.Players[seat].Position
	}
	return route[len(route)-1]
}

// checkActionDegradation applies GDD §15.0's two degradation checks that
// have no Legal equivalent (legal.go's own doc comments name both as
// Resolve-time concerns): a Stake Post target already owned by someone
// else, and a Pickup whose declared cargo is no longer on the ground. On
// either, it returns o with Action nulled to Nothing, the matching event,
// and false. Otherwise it returns o unchanged and true. The route is never
// touched here — only truncateAtDestroyedEdge truncates a route.
// blockedBorders is every Border not accepting deliveries this round
// (borders.go's blockedBordersThisRound): Dragnet's own seal (issue #72,
// GDD §14.2 — "every delivery must route to the ones that remain") unioned
// with two-player rotation's inactive set (GDD §6.3), D28's safety valve
// already applied. A declared Deliver at a blocked Border is legal payload
// shape (Deliver at a Border is otherwise always legal) that the world
// moved under, exactly like a Stake Post target someone else already
// claimed — degrade, don't reject.
func checkActionDegradation(s MatchState, seat game.SeatID, o game.Order, blockedBorders []game.NodeID) (game.Order, game.Event, bool) {
	node := routeEndingNode(s, seat, o.Route)

	switch o.Action.Kind {
	case game.ActionStakePost:
		if post := s.Graph.Nodes[node].Post; post != nil && post.Owner != seat {
			degraded := o
			degraded.Action = game.ActionOrder{Kind: game.ActionNothing}
			return degraded, game.Event{Kind: game.EventStakeTargetTaken, Round: s.Round, Seat: seat, Node: node}, false
		}

	case game.ActionPickup:
		if !pickupAvailable(s, seat, node) {
			degraded := o
			degraded.Action = game.ActionOrder{Kind: game.ActionNothing}
			return degraded, game.Event{Kind: game.EventPickupTargetGone, Round: s.Round, Seat: seat, Node: node}, false
		}

	case game.ActionDeliver:
		if slices.Contains(blockedBorders, node) {
			degraded := o
			degraded.Action = game.ActionOrder{Kind: game.ActionNothing}
			return degraded, game.Event{Kind: game.EventDeliveryBlocked, Round: s.Round, Seat: seat, Node: node}, false
		}
	}

	return o, game.Event{}, true
}

// legalView builds the minimal game.PlayerView Legal actually reads,
// directly from live MatchState plus entry's frozen Balance/Position —
// standing in for Project (#75, not built yet and not a blocker of this
// issue). Legal reads only v.You (Position, Balance, Items, Posts,
// Contracts, Cargo) and v.Nodes (fog-filtered); v.Others is read only for
// its length, by legalPostCap. Everything else on game.PlayerView (Trail,
// Anchors, Headline, Deck, Archive, NodeStats) is Project's job, not
// Legal's, and is left zero-valued here.
//
// You.Infamy and You.StepModifiers.Flagged/EvasiveStepPenalty complete a
// wiring gap this function always had: EntrySnapshot has carried Infamy,
// Flagged and EvasiveStepPenalty since #67, but nothing ever copied them
// into SelfState, so Steps (steps.go), which Legal calls, silently computed
// every legality check against Infamy 0 and no modifiers. curfewActive
// (issue #72, GDD §14.2) is this round's peeked Curfew card — see
// resolve.go's globalEventContext — and Scaffolding/Retainer read
// s.NextRound directly (also #72): Scaffolding is sector-scoped against
// snap.Position (the same frozen position every other entry-snapshot
// consumer here already uses), Retainer against live p.Cargo, which
// movement has not yet touched at the point validate calls this.
//
// TODO(#75): remove this second MatchState -> game.PlayerView construction
// once Project exists, and have Legal read Project's output instead — two
// fog-filter implementations can otherwise drift apart.
func legalView(s MatchState, entry EntrySnapshot, seat game.SeatID, curfewActive bool) game.PlayerView {
	p := s.Players[seat]
	snap := entry.Seats[seat]

	nodes := make(map[game.NodeID]game.NodeView, len(p.Fog))
	for i, fog := range p.Fog {
		if fog < game.FogRumoured {
			continue
		}
		n := s.Graph.Nodes[i]
		view := game.NodeView{Fog: fog, Name: n.Name, Type: n.Type, Sector: n.Sector, X: n.X, Y: n.Y}
		if fog >= game.FogKnown {
			view.Edges = n.Edges
		}
		nodes[game.NodeID(i)] = view
	}

	scaffolding := s.NextRound.Scaffolding != nil && *s.NextRound.Scaffolding == s.Graph.Nodes[snap.Position].Sector

	return game.PlayerView{
		You: game.SelfState{
			Position:  snap.Position,
			Balance:   snap.Balance,
			Infamy:    snap.Infamy,
			Cargo:     p.Cargo,
			Contracts: contractsInHand(p.Contracts),
			Items:     p.Items,
			Posts:     postsHeld(s, p.Posts),
			StepModifiers: game.StepModifiers{
				Flagged:            snap.Flagged,
				EvasiveStepPenalty: snap.EvasiveStepPenalty,
				Curfew:             curfewActive,
				Scaffolding:        scaffolding,
				Retainer:           s.NextRound.Retainer && p.Cargo == nil,
				DistractedGuard:    snap.DistractedGuard,
				StreetsBlocked:     snap.StreetsBlocked,
			},
			DockersStrike: s.NextRound.DockersStrike,
		},
		Others: make([]game.OpponentView, len(s.Players)-1),
		Nodes:  nodes,
	}
}

// postsHeld converts a seat's held node list (rules.Player.Posts) to the
// game.Post shape Legal's legalPostCap reads — it only ever asks for the
// slice's length, but the field type is fixed by game.PlayerView.
func postsHeld(s MatchState, held []game.NodeID) []game.Post {
	if held == nil {
		return nil
	}
	posts := make([]game.Post, len(held))
	for i, node := range held {
		rounds := 0
		if p := s.Graph.Nodes[node].Post; p != nil {
			rounds = p.RoundsRemaining
		}
		posts[i] = game.Post{Node: node, RoundsRemaining: rounds}
	}
	return posts
}

// contractsInHand converts rules.Contract (the authoritative, per-seat
// instance) to game.ContractInHand (the fog-facing shape) — a plain field
// copy, since Contract carries nothing MatchState needs to hide from its
// own owning seat.
func contractsInHand(contracts []Contract) []game.ContractInHand {
	if contracts == nil {
		return nil
	}
	hand := make([]game.ContractInHand, len(contracts))
	for i, c := range contracts {
		hand[i] = game.ContractInHand{
			ID:                c.ID,
			Origin:            c.Origin,
			Destination:       c.Destination,
			Tier:              c.Tier,
			ExpiresRound:      c.ExpiresRound,
			DeadlinePauseUsed: c.DeadlinePauseUsed,
		}
	}
	return hand
}
