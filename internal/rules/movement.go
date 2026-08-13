package rules

import "github.com/garnizeh/cinzal/internal/game"

// confrontation is one node's worth of contesting seats, detected during a
// single movement step (GDD §15a crossing, §15b collision) — the output of
// detectCrossings and detectCollisions, merged and node-ordered
// (mergeConfrontations, reusing #58's orderConfrontationsByNode) into
// resolveConfrontations' (#69) own input. Seats is seat-ordered
// (filterBySeat) — the shape every RNG batch over a confrontation's
// participants needs (RFC §6.5).
type confrontation struct {
	Node  game.NodeID
	Seats []game.SeatID
}

// transition is one seat's movement during a single step: From is its
// position before the step, To is its position after. To == From for a seat
// that did not move this step — route exhausted, Pushing On exhausted or
// stopped early (no legal edge), or simply not this seat's turn to move yet
// because a longer route elsewhere is what set movementSteps' floor.
type transition struct {
	Seat game.SeatID
	From game.NodeID
	To   game.NodeID
}

// seatWalk is per-seat, per-round movement bookkeeping with nowhere else to
// live: it exists only to support Pushing On's "exclude the node you just
// came from" rule (GDD §9.1 item 5) across steps, and it is entirely local
// to one call to Resolve — never part of MatchState, never persisted, never
// read outside this file.
type seatWalk struct {
	// Previous is the seat's position one step before its current one — the
	// node a Pushing On ladder must not backtrack into. It always trails
	// Position by exactly one step: advance updates it, from the position
	// held at the *start* of the step just processed, after computing that
	// step's destination but before applying it — so a blind step's own
	// ladder call always reads the predecessor of its own From, never of
	// itself.
	Previous game.NodeID
}

// newSeatWalks seeds one seatWalk per seat in seats at s's current
// positions — the correct start-of-round value: a seat whose route is empty
// and whose Pushing On begins immediately has "come from" nowhere yet this
// round, and Previous == Position makes the ladder's backtrack exclusion a
// harmless no-op for exactly that first blind step (a node is never its own
// neighbour).
func newSeatWalks(s MatchState, seats []game.SeatID) map[game.SeatID]*seatWalk {
	walks := make(map[game.SeatID]*seatWalk, len(seats))
	for _, seat := range seats {
		walks[seat] = &seatWalk{Previous: s.Players[seat].Position}
	}
	return walks
}

// advance applies step (1-based) of every seat's validated order: an
// ordinary route step while the seat is still inside its declared Route, a
// blind Pushing On step once the route is exhausted (GDD §9.1), or nothing
// at all once both are exhausted or the walk has already stopped early. It
// mutates s in place — Position, Balance (Scavenging's Cr$3), and Fog (a
// visited node becomes Known, GDD §7.2, and a natural 6 reveals its
// neighbours) — and returns each seat's (from, to) transition for this
// step, the shape detectCrossings needs; detectCollisions does not, since it
// reads final position directly off s.
//
// seats must be bySeat(s)'s own order (RFC §6.5): within one step, a seat's
// Pushing On draws (pushon.edge, scavenge.d6) are an RNG batch, and a batch
// draws in seat order, never map order (CLAUDE.md forbids ranging over maps
// in resolution).
//
// Gas Leak's "nobody may end their route in this sector" truncation (GDD
// §14.3) is not implemented here: no code anywhere yet sets a sector
// Unstable for the round (that is #73's Sector Incident deck), so there is
// no live state to truncate against and nothing this issue's acceptance
// criteria asks it to do. #73 is the natural place to add that check once
// the state it reads exists.
func advance(s *MatchState, walks map[game.SeatID]*seatWalk, validated map[game.SeatID]game.Order, seats []game.SeatID, step int, r *RNG) map[game.SeatID]transition {
	transitions := make(map[game.SeatID]transition, len(seats))

	for _, seat := range seats {
		from := s.Players[seat].Position
		o := validated[seat]
		walk := walks[seat]

		to := from
		switch {
		case step <= len(o.Route):
			to = o.Route[step-1]
		case step <= len(o.Route)+o.PushingOn.Steps:
			to = pushOnStep(s.Graph, from, walk.Previous, o.PushingOn.Bias, r)
		}

		walk.Previous = from

		if to != from {
			s.Players[seat].Position = to
			scavenge(s, seat, to, r)
		}

		transitions[seat] = transition{Seat: seat, From: from, To: to}
	}

	return transitions
}

// pushOnStep computes and applies one blind Pushing On step from `from`,
// per GDD §9.1's priority ladder, returning the destination — from itself,
// unmoved, when no legal edge exists (GDD §9.1's own early-termination
// case, "the pattern used for pushback," §15). It consumes exactly one
// pushon.edge draw when at least one legal edge exists, and none at all
// when it doesn't — the RFC §6.4 lazy-draw rule this whole loop exists to
// protect.
//
// The ladder's levels 1-3 need "the shortest graph distance from the
// current node to the nearest node of the declared sector" (d) and, for
// each candidate edge, the identical distance computed from the
// candidate's own destination — both against g's navigable graph (GDD
// §9.1a item 0), both from distancesToSector, one multi-source BFS shared
// across every candidate rather than one BFS per candidate.
func pushOnStep(g Graph, from, cameFrom game.NodeID, bias *game.Sector, r *RNG) game.NodeID {
	candidates := pushOnCandidates(g, from, cameFrom)
	if len(candidates) == 0 {
		return from
	}

	pool := candidates
	if bias != nil {
		toSector := g.distancesToSector(*bias)
		if d := toSector[from]; d >= 0 {
			if closer := nodesCloserThanSector(candidates, toSector, d); len(closer) > 0 {
				pool = closer
			} else if level := nodesAtSectorDistance(candidates, toSector, d); len(level) > 0 {
				pool = level
			}
		}
	}

	return pool[r.Next(PurposePushonEdge, len(pool))]
}

// pushOnCandidates returns from's legal Pushing On destinations on g's
// navigable graph: Bridge-Down-destroyed edges are already absent from
// Node.Edges (see Node's own comment), and a Sinkholed destination is
// impassable and excluded here (GDD §9.1a item 0). cameFrom is excluded
// unless it is the only candidate left (GDD §9.1 item 5, "at every level"
// — applied once to the whole candidate set rather than level by level,
// since every level draws from the same pool).
func pushOnCandidates(g Graph, from, cameFrom game.NodeID) []game.NodeID {
	var all []game.NodeID
	for _, n := range g.Nodes[from].Edges {
		if g.Nodes[n].SinkholeRounds == 0 {
			all = append(all, n)
		}
	}

	var withoutBacktrack []game.NodeID
	for _, n := range all {
		if n != cameFrom {
			withoutBacktrack = append(withoutBacktrack, n)
		}
	}
	if len(withoutBacktrack) > 0 {
		return withoutBacktrack
	}
	return all
}

// nodesCloserThanSector returns candidates whose own distance to the biased
// sector (toSector) is strictly less than d — GDD §9.1's ladder level 2. A
// candidate with no path to the sector at all (toSector == -1) never
// qualifies, on either side of the comparison.
func nodesCloserThanSector(candidates []game.NodeID, toSector []int, d int) []game.NodeID {
	var out []game.NodeID
	for _, n := range candidates {
		if dist := toSector[n]; dist >= 0 && dist < d {
			out = append(out, n)
		}
	}
	return out
}

// nodesAtSectorDistance returns candidates whose own distance to the biased
// sector equals d exactly — GDD §9.1's ladder level 3.
func nodesAtSectorDistance(candidates []game.NodeID, toSector []int, d int) []game.NodeID {
	var out []game.NodeID
	for _, n := range candidates {
		if toSector[n] == d {
			out = append(out, n)
		}
	}
	return out
}

// scavenge applies GDD §9.1's Scavenging roll for seat entering node: a
// destination already Known to seat rolls nothing at all — "including one
// that became Known thirty seconds earlier because a 6 on the previous step
// revealed it" — checked against seat's live Fog at the moment of arrival,
// which advance guarantees by calling this immediately after each step's
// move, never batched. Otherwise it draws 1D6 (scavenge.d6): 1-3 nothing,
// 4-5 Cr$3, 6 reveals every node adjacent to node as Known. Entering node
// makes it Known to seat regardless of the roll — "a visited node becomes
// Known permanently" (GDD §7.2) is the reveal the roll only adds a bonus on
// top of, not a gate on.
func scavenge(s *MatchState, seat game.SeatID, node game.NodeID, r *RNG) {
	p := &s.Players[seat]
	if p.Fog[node] >= game.FogKnown {
		return
	}

	switch roll := r.Next(PurposeScavengeD6, 6) + 1; roll {
	case 4, 5:
		p.Balance += 3
	case 6:
		for _, adj := range s.Graph.Nodes[node].Edges {
			if p.Fog[adj] < game.FogKnown {
				p.Fog[adj] = game.FogKnown
			}
		}
	}

	p.Fog[node] = game.FogKnown
}

// detectCrossings returns, for step's transitions, one seat group per node
// where two seats traversed the same edge in opposite directions this step
// (GDD §15a): "they meet mid-edge." The group is keyed by the confrontation
// location crossingNode resolves — the four Aggressive combinations the
// issue names explicitly.
//
// seats must be bySeat's own order: pairs are scanned lowest-seat-first so
// that when more than one pair resolves to the same node, the seats append
// to that node's group in a stable order before mergeConfrontations imposes
// its own final seat ordering — a merge input this function does not itself
// need to keep sorted, but must not need a second, ad-hoc sort to fix later.
func detectCrossings(transitions map[game.SeatID]transition, seats []game.SeatID, validated map[game.SeatID]game.Order) map[game.NodeID][]game.SeatID {
	groups := map[game.NodeID][]game.SeatID{}

	for i, a := range seats {
		ta := transitions[a]
		if ta.From == ta.To {
			continue
		}

		for _, b := range seats[i+1:] {
			tb := transitions[b]
			if tb.From != ta.To || tb.To != ta.From {
				continue
			}

			node := crossingNode(ta.From, tb.From, validated[a].Stance.Stance, validated[b].Stance.Stance)
			groups[node] = append(groups[node], a, b)
		}
	}

	return groups
}

// crossingNode resolves GDD §15a's confrontation location: the origin node
// of whichever traveler is Aggressive; the lower-indexed of the two
// endpoints if neither or both are. fromA and fromB are the two travelers'
// own origins — also the crossed edge's two endpoints, since a crossing is
// only ever detected on a shared edge in the first place.
func crossingNode(fromA, fromB game.NodeID, stanceA, stanceB game.Stance) game.NodeID {
	aggressiveA := stanceA == game.StanceAggressive
	aggressiveB := stanceB == game.StanceAggressive

	switch {
	case aggressiveA && !aggressiveB:
		return fromA
	case aggressiveB && !aggressiveA:
		return fromB
	default:
		return min(fromA, fromB)
	}
}

// detectCollisions returns one seat group per node occupied by two or more
// seats at the end of this step (GDD §15b) — "evaluates ALL positions,
// stationary seats included" (RFC §6.7): it reads s.Players directly rather
// than a transition set, so a seat that did not move this step, or this
// round, is still counted. Running this every step, on final position
// alone, is what makes GDD §15's three cohabitation cases self-resolving
// without a dedicated pre-movement check (GDD §15, "nothing special is
// needed to end it").
//
// seats must be bySeat's own order, so each node's group is already seat-
// ordered on the way in — mergeConfrontations still reorders explicitly via
// filterBySeat rather than trusting that as a given, since a crossing group
// merged into the same node need not itself be seat-ordered going in.
func detectCollisions(s MatchState, seats []game.SeatID) map[game.NodeID][]game.SeatID {
	groups := map[game.NodeID][]game.SeatID{}
	for _, seat := range seats {
		node := s.Players[seat].Position
		groups[node] = append(groups[node], seat)
	}

	for node, group := range groups {
		if len(group) < 2 {
			delete(groups, node)
		}
	}

	return groups
}

// mergeConfrontations combines detectCrossings' and detectCollisions'
// groupings into node-ordered confrontations (orderConfrontationsByNode,
// #58): "two confrontations at different nodes in the same step resolve in
// node ID order." A node with both a crossing and a co-located collision
// becomes one confrontation with every participant's union, not two
// separate resolutions of the same node — Seats is deduplicated and
// seat-ordered via filterBySeat, which also folds a seat's own duplicate
// membership across more than one contributing group into a single entry.
func mergeConfrontations(s MatchState, crossings, collisions map[game.NodeID][]game.SeatID) []confrontation {
	merged := map[game.NodeID][]game.SeatID{}
	for node, group := range crossings {
		merged[node] = append(merged[node], group...)
	}
	for node, group := range collisions {
		merged[node] = append(merged[node], group...)
	}

	nodes := make([]game.NodeID, 0, len(merged))
	for node := range merged {
		nodes = append(nodes, node)
	}

	result := make([]confrontation, 0, len(merged))
	for _, node := range orderConfrontationsByNode(nodes) {
		result = append(result, confrontation{Node: node, Seats: filterBySeat(s, merged[node])})
	}
	return result
}
