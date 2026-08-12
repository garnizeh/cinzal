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
// and a Pickup whose declared cargo is no longer on the ground. Both are
// checked here, against live state, after the route/Legal pass.
//
// Not handled here: GDD §15.0's third named example, "funds went to a
// stake that was lost". Balance is untouched at Step 0 — nothing has spent
// it yet, since resolveAddons (#70) runs after actions and deliveries —
// so there is nothing to degrade against yet; that check belongs to #70.
func validate(s MatchState, entry EntrySnapshot, seats []game.SeatID, orders map[game.SeatID]game.Order, cfg game.Config) (map[game.SeatID]game.Order, []game.Event) {
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

		if err := Legal(legalView(s, entry, seat), candidate, cfg); err != nil {
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

		degraded, reason, ok := checkActionDegradation(s, seat, candidate)
		if !ok {
			validated[seat] = degraded
			events = append(events, reason)
			continue
		}

		validated[seat] = candidate
	}

	return validated, events
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
func checkActionDegradation(s MatchState, seat game.SeatID, o game.Order) (game.Order, game.Event, bool) {
	node := routeEndingNode(s, seat, o.Route)

	switch o.Action.Kind {
	case game.ActionStakePost:
		if s.Graph.Nodes[node].Post != nil {
			degraded := o
			degraded.Action = game.ActionOrder{Kind: game.ActionNothing}
			return degraded, game.Event{Kind: game.EventStakeTargetTaken, Round: s.Round, Seat: seat, Node: node}, false
		}

	case game.ActionPickup:
		if !cargoAvailable(s, seat, node) {
			degraded := o
			degraded.Action = game.ActionOrder{Kind: game.ActionNothing}
			return degraded, game.Event{Kind: game.EventPickupTargetGone, Round: s.Round, Seat: seat, Node: node}, false
		}
	}

	return o, game.Event{}, true
}

// cargoAvailable reports whether node currently holds cargo seat may
// collect (GDD §8.4), reusing CanCollect (cargo.go) — the same Bound/loose
// eligibility rule Legal's own deliverableCargo uses for Deliver, applied
// here to Pickup instead, which Legal deliberately does not check at all.
func cargoAvailable(s MatchState, seat game.SeatID, node game.NodeID) bool {
	for _, c := range s.Graph.Cargo {
		if c.Node == node && CanCollect(s.Players[seat].Contracts, c) {
			return true
		}
	}
	return false
}

// legalView builds the minimal game.PlayerView Legal actually reads,
// directly from live MatchState plus entry's frozen Balance/Position —
// standing in for Project (#75, not built yet and not a blocker of this
// issue). Legal reads only v.You (Position, Balance, Items, Posts,
// Contracts, Cargo) and v.Nodes (fog-filtered); v.Others is read only for
// its length, by legalPostCap. Everything else on game.PlayerView (Trail,
// Anchors, Headline, Deck, Archive, NodeStats) is Project's job, not
// Legal's, and is left zero-valued here.
func legalView(s MatchState, entry EntrySnapshot, seat game.SeatID) game.PlayerView {
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

	return game.PlayerView{
		You: game.SelfState{
			Position:  snap.Position,
			Balance:   snap.Balance,
			Cargo:     p.Cargo,
			Contracts: contractsInHand(p.Contracts),
			Items:     p.Items,
			Posts:     postsHeld(s, p.Posts),
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
