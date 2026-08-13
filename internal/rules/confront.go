package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// resolveConfrontations resolves every confrontation #68's detection located
// and node-ordered this movement step (GDD §15): the TOTAL formula, the
// winner/loser/tie split, cargo and stake consequences, Muscle's loss
// condition (D14), and the pushback table including displacement.
//
// validated is mutated in place — a loser's (or a tie's, or a crossing-
// corrected winner's) remaining Route/PushingOn/Action is cleared so later
// steps in this same call's movement loop stop moving them ("loses the
// remainder of their route and their action"). walks carries this round's
// per-seat traversed-node history (seatWalk.Path, movement.go) and Shiv
// usage (seatWalk.ShivFired) — both threaded through the whole movement
// loop, never part of MatchState, exactly like Pushing On's own Previous
// field.
func resolveConfrontations(s *MatchState, pending []confrontation, validated map[game.SeatID]game.Order, walks map[game.SeatID]*seatWalk, cfg game.Config, r *RNG) []game.Event {
	var events []game.Event
	for _, c := range pending {
		events = append(events, resolveOneConfrontation(s, c, validated, walks, cfg, r)...)
	}
	return events
}

// resolveOneConfrontation resolves a single node's confrontation: correct
// crossing positions, roll and total every participant (seat order — RFC
// §6.5: confrontation dice are an RNG batch, not a fairness-key selection),
// then apply the tie or decisive outcome.
func resolveOneConfrontation(s *MatchState, c confrontation, validated map[game.SeatID]game.Order, walks map[game.SeatID]*seatWalk, cfg game.Config, r *RNG) []game.Event {
	corrected := correctCrossingPositions(s, c, walks)

	infamy := make(map[game.SeatID]int, len(c.Seats))
	for _, seat := range c.Seats {
		infamy[seat] = s.Players[seat].Infamy
	}
	lowest := lowestInfamy(infamy, c.Seats)

	totals := make(map[game.SeatID]int, len(c.Seats))
	for _, seat := range c.Seats {
		totals[seat] = confrontationTotal(s, seat, c.Node, validated[seat], infamy[seat], lowest, walks[seat], r)
	}

	winner, tie := determineOutcome(c.Seats, totals)
	if tie {
		resolveTie(s, c, validated, walks)
		applyMuscleLoss(s, c.Seats, nil, true)
		return tieEvents(s.Round, c.Node, c.Seats)
	}

	losers := orderLosersForPushback(*s, losersExcept(c.Seats, winner))
	events := resolveDecisive(s, c, winner, corrected[winner], losers, validated, walks, cfg, r)
	applyMuscleLoss(s, c.Seats, &winner, false)
	return events
}

// correctCrossingPositions snaps every participant's live Position to c.Node
// — a no-op for a collision (already true) and for whichever crossing
// party's own destination this step already was Node, but a genuine
// correction for the other: advance() (movement.go) already carried both
// crossing parties to their own (swapped) destinations before detection and
// resolution run, and GDD §15a resolves the fight "at the origin node of
// whichever player is Aggressive" — one specific node, not wherever advance
// happened to leave each of them. The corrected set is returned so the
// caller can halt a corrected winner's now-invalid remaining route (see
// resolveDecisive) — the one case where "route continues" doesn't literally
// hold.
func correctCrossingPositions(s *MatchState, c confrontation, walks map[game.SeatID]*seatWalk) map[game.SeatID]bool {
	corrected := make(map[game.SeatID]bool, len(c.Seats))
	for _, seat := range c.Seats {
		if s.Players[seat].Position == c.Node {
			continue
		}
		s.Players[seat].Position = c.Node
		fixPathTail(walks[seat], c.Node)
		corrected[seat] = true
	}
	return corrected
}

// fixPathTail rewrites walk's last traversed-node entry to node, for the
// crossing correction above: advance() already appended this step's raw
// (uncorrected) destination to Path, and the fight's real location
// overrides it. If that leaves the last two entries identical, the
// redundant one is dropped, keeping Path a list of genuinely distinct
// traversed nodes — the shape pushback (below) needs.
func fixPathTail(walk *seatWalk, node game.NodeID) {
	walk.Path[len(walk.Path)-1] = node
	if n := len(walk.Path); n >= 2 && walk.Path[n-2] == node {
		walk.Path = walk.Path[:n-1]
	}
}

// lowestInfamy returns the minimum Infamy among seats, per the infamy map
// built from live, instant-of-resolution values (GDD §11.1) — the
// "underestimated" modifier's own reference point.
func lowestInfamy(infamy map[game.SeatID]int, seats []game.SeatID) int {
	lowest := infamy[seats[0]]
	for _, seat := range seats[1:] {
		if v := infamy[seat]; v < lowest {
			lowest = v
		}
	}
	return lowest
}

// confrontationTotal computes one participant's GDD §15 TOTAL and consumes
// their one PurposeConfrontD6 draw. infamy and lowest are the confrontation's
// own instant-of-resolution snapshot (GDD §11.1) — never a live re-read once
// this confrontation's own Infamy deltas start applying, and never the entry
// snapshot. walk is this seat's own seatWalk (movement.go): ShivFired gates
// Shiv's "fires on your first confrontation this round" (GDD §9.4).
func confrontationTotal(s *MatchState, seat game.SeatID, node game.NodeID, o game.Order, infamy, lowest int, walk *seatWalk, r *RNG) int {
	roll := r.Next(PurposeConfrontD6, 6) + 1

	total := roll + infamyTierIndex(infamy) + stanceModifier(o.Stance.Stance)

	if o.Stance.Stance == game.StanceAggressive && s.Graph.Nodes[node].Type == game.NodeAlley {
		total++
	}

	if hasShivDiscard(o) && !walk.ShivFired {
		total += ShivBonus
		walk.ShivFired = true
	}
	if slices.Contains(s.Players[seat].Items, game.ItemMuscle) {
		total += MuscleBonus
	}

	total += min(o.Stance.Stake/3, 2)

	if infamy == lowest {
		total++
	}

	return total
}

// stanceModifier is GDD §15's stance term: +1 Aggressive, 0 Neutral, −1
// Evasive.
func stanceModifier(stance game.Stance) int {
	switch stance {
	case game.StanceAggressive:
		return 1
	case game.StanceEvasive:
		return -1
	default:
		return 0
	}
}

// hasShivDiscard reports whether o declares Shiv as a Field 4 discard this
// round (GDD §9.4) — read from the order, not from Player.Items, since
// whether an Armed discard has actually been removed from hand is #70's
// concern (resolveActions), not this one's.
func hasShivDiscard(o game.Order) bool {
	return slices.ContainsFunc(o.Items, func(d game.ItemDiscard) bool {
		return d.Item == game.ItemShiv
	})
}

// determineOutcome returns the unique highest-TOTAL seat among seats (seat
// order, so the result is deterministic regardless of totals map iteration)
// and whether two or more seats share that highest total — GDD §15: "on a
// tie, nobody wins," which holds for the whole confrontation, not just the
// tied subset.
func determineOutcome(seats []game.SeatID, totals map[game.SeatID]int) (winner game.SeatID, tie bool) {
	best := totals[seats[0]]
	winner = seats[0]
	tieCount := 1
	for _, seat := range seats[1:] {
		switch t := totals[seat]; {
		case t > best:
			best = t
			winner = seat
			tieCount = 1
		case t == best:
			tieCount++
		}
	}
	return winner, tieCount > 1
}

// losersExcept returns every seat but winner, preserving seats' own order.
func losersExcept(seats []game.SeatID, winner game.SeatID) []game.SeatID {
	losers := make([]game.SeatID, 0, len(seats)-1)
	for _, seat := range seats {
		if seat != winner {
			losers = append(losers, seat)
		}
	}
	return losers
}

// resolveTie applies GDD §15's tie outcome: every participant reverts to
// the node they held before this step (seatWalk.Previous, movement.go —
// well-defined even for a seat that did not move this step, where it
// already equals their current position) — no stake, cargo, or Infamy
// change, since a stake was never actually deducted at declaration
// ("stakes are returned" is therefore a no-op here, not a refund). Their
// remaining route and action this round are halted regardless: the GDD
// does not restate that for a tie the way it does for a Loser, but it is
// an implementation necessity — advance() (movement.go) trusts
// Route[step-1] to be adjacent to the seat's actual current position, and
// a reverted position generally invalidates whatever the rest of a
// multi-step route assumed.
func resolveTie(s *MatchState, c confrontation, validated map[game.SeatID]game.Order, walks map[game.SeatID]*seatWalk) {
	for _, seat := range c.Seats {
		dest := walks[seat].Previous
		s.Players[seat].Position = dest
		walks[seat].Path = []game.NodeID{dest}
		haltMovement(validated, seat)
	}
}

// tieEvents records that a confrontation happened here even though nobody
// won it — GDD §15's displacement discussion: "a confrontation writes a
// public trace naming both parties at that node." A tie has no winner/loser
// pairing to name, so this emits one EventConfrontation per adjacent pair
// in seat order (K-1 events for K tied participants, one event for the
// common two-way case) rather than every combinatorial pair.
func tieEvents(round game.RoundNumber, node game.NodeID, seats []game.SeatID) []game.Event {
	events := make([]game.Event, 0, len(seats)-1)
	for i := 0; i+1 < len(seats); i++ {
		events = append(events, game.Event{
			Kind:   game.EventConfrontation,
			Round:  round,
			Node:   node,
			Seat:   seats[i],
			Target: seats[i+1],
		})
	}
	return events
}

// resolveDecisive applies GDD §15's Winner and Loser sections once a unique
// top scorer is known. winnerCorrected is correctCrossingPositions' report
// for winner: true only for the rare crossing case where the winner's own
// pre-fight position was the "blocked" side (their attempted move undone to
// reach c.Node) — their remaining route no longer starts from where it
// assumed, so it is halted exactly like a loser's despite winning; every
// other case leaves it untouched, "route continues" exactly as written.
func resolveDecisive(s *MatchState, c confrontation, winner game.SeatID, winnerCorrected bool, losers []game.SeatID, validated map[game.SeatID]game.Order, walks map[game.SeatID]*seatWalk, cfg game.Config, r *RNG) []game.Event {
	s.Players[winner].Infamy = ApplyInfamyDelta(s.Players[winner].Infamy, InfamyGainConfrontationWin)
	if winnerCorrected {
		haltMovement(validated, winner)
	}

	var eligible []game.SeatID
	stakeCollected := 0
	for _, seat := range losers {
		share, forfeit := resolveLoser(s, c, seat, validated, walks, cfg, r)
		stakeCollected += share
		if forfeit {
			eligible = append(eligible, seat)
		}
	}
	s.Players[winner].Balance += stakeCollected

	taken, tookCargo := game.SeatID(0), false
	if s.Players[winner].Cargo == nil {
		if target, ok := selectDefaultMeleeCargoTarget(*s, eligible); ok {
			cargo := *s.Players[target].Cargo
			s.Players[winner].Cargo = &cargo
			s.Players[target].Cargo = nil
			taken, tookCargo = target, true
		}
	}

	for _, seat := range eligible {
		if tookCargo && seat == taken {
			continue
		}
		dropForfeitCargo(s, seat, c.Node)
	}

	return decisiveEvents(s.Round, c.Node, winner, losers)
}

// resolveLoser applies GDD §15's Loser consequences for seat, in order:
// stake, cargo (shakedown or forfeiture), Deadline Pause, Infamy, then
// pushback and halting the remainder of their round. Returns the Cr$ the
// winner collects from this loser's stake (half of what was actually
// forfeited, rounded down — never half of the declared stake: legalBalance
// (legal.go) deliberately excludes Aggressive stake from the submission-time
// affordability check, so a stake can legally outgrow balance by the time it
// resolves) and whether this seat's cargo is forfeit and still theirs to
// give — the caller decides whether the winner takes it or it drops at the
// node.
func resolveLoser(s *MatchState, c confrontation, seat game.SeatID, validated map[game.SeatID]game.Order, walks map[game.SeatID]*seatWalk, cfg game.Config, r *RNG) (stakeShare int, forfeitCargo bool) {
	p := &s.Players[seat]
	o := validated[seat]

	staked := min(o.Stance.Stake, p.Balance)
	p.Balance -= staked

	if p.Cargo != nil {
		protected := false
		if o.Stance.Stance == game.StanceEvasive {
			paid := Shakedown(p.Balance, cfg.ShakedownCost)
			p.Balance -= paid
			protected = paid == cfg.ShakedownCost
		}

		if p.Cargo.Bound {
			applyDeadlinePause(p)
		}

		forfeitCargo = !protected
	}

	p.Infamy = ApplyInfamyDelta(p.Infamy, InfamyLossConfrontationLoss)

	pushback(s, c, seat, o.Stance.Stance == game.StanceEvasive, walks, r)
	haltMovement(validated, seat)

	return staked / 2, forfeitCargo
}

// applyDeadlinePause finds p's contract matching its carried cargo and
// applies Contract.WithDeadlinePause() (cargo.go) — a no-op past the first
// call on that contract. Only meaningful when p.Cargo.Bound; the caller
// gates that.
func applyDeadlinePause(p *Player) {
	for i := range p.Contracts {
		if p.Contracts[i].ID == p.Cargo.Contract {
			p.Contracts[i] = p.Contracts[i].WithDeadlinePause()
			return
		}
	}
}

// dropForfeitCargo drops seat's carried cargo at node, still bound to its
// original contract's origin/destination pair — GDD §15: "it does not
// become a loose crate" — for a loser whose cargo the winner didn't take,
// couldn't take (full slot already), or wasn't chosen over another eligible
// loser's. A carried loose crate (Bound false) simply becomes a loose crate
// on the ground again, since it never had a pair to preserve.
func dropForfeitCargo(s *MatchState, seat game.SeatID, node game.NodeID) {
	p := &s.Players[seat]
	dropped := Cargo{Node: node, Bound: p.Cargo.Bound}

	if p.Cargo.Bound {
		for _, c := range p.Contracts {
			if c.ID == p.Cargo.Contract {
				dropped.Origin = c.Origin
				dropped.Destination = c.Destination
				break
			}
		}
	}

	s.Graph.Cargo = append(s.Graph.Cargo, dropped)
	p.Cargo = nil
}

// pushback computes and applies GDD §15's pushback table for a confrontation
// loser at seat, whose current Position is c.Node (after any crossing
// correction) and whose seatWalk.Path (movement.go) ends there too: falling
// back along the route actually traversed this round, 1 node (2 if
// Evasive), per the table's own edge cases — exhausted-route stop, a
// stationary random hop (a second, harder-excluded hop if Evasive), and an
// early stop the moment any hop has no legal destination, displacing the
// loser as far as the graph allows and no further. Path is truncated (or
// reset) to match the new Position, so a later confrontation this round —
// reachable per RFC §6.5's Bounty worked case, where a pushed loser is
// caught again by someone else's later movement step — starts its own
// pushback from the right history.
func pushback(s *MatchState, c confrontation, seat game.SeatID, evasive bool, walks map[game.SeatID]*seatWalk, r *RNG) {
	hops := 1
	if evasive {
		hops = 2
	}

	walk := walks[seat]
	traversed := len(walk.Path) - 1

	var dest game.NodeID
	switch {
	case traversed >= hops:
		dest = walk.Path[len(walk.Path)-1-hops]
		walk.Path = walk.Path[:len(walk.Path)-hops]

	case traversed == 1:
		// Pushed 2 but only 1 traversed: the route is exhausted, not
		// extrapolated — stops at the starting node.
		dest = walk.Path[0]
		walk.Path = walk.Path[:1]

	default: // traversed == 0: stationary.
		dest = c.Node
		if first := neighborsExcluding(s.Graph, c.Node); len(first) > 0 {
			dest = first[r.Next(PurposePushbackHop, len(first))]

			if evasive {
				// Must not re-enter the confrontation node, and hop one's
				// origin is the confrontation node — so "must not
				// immediately backtrack" and "must not re-enter" are the
				// same single exclusion here. No "unless it's the only
				// candidate" fallback: GDD §15 is explicit this one is
				// load-bearing. A boxed-in second hop simply doesn't
				// happen — no draw consumed (RFC §6.4's lazy-draw rule).
				if second := neighborsExcluding(s.Graph, dest, c.Node); len(second) > 0 {
					dest = second[r.Next(PurposePushbackHop, len(second))]
				}
			}
		}
		walk.Path = []game.NodeID{dest}
	}

	s.Players[seat].Position = dest
}

// neighborsExcluding returns node's neighbors on the navigable graph
// (Bridge-Down-destroyed edges already absent from Node.Edges; a Sinkholed
// neighbor is impassable and excluded, matching pushOnCandidates'
// (movement.go) identical navigability rule, GDD §9.1a item 0) that are not
// among exclude. Unlike pushOnCandidates' own backtrack rule, this has no
// "unless it's the only candidate" fallback — an empty result is a real,
// meaningful outcome the caller must handle (a boxed-in hop).
func neighborsExcluding(g Graph, node game.NodeID, exclude ...game.NodeID) []game.NodeID {
	var out []game.NodeID
	for _, n := range g.Nodes[node].Edges {
		if g.Nodes[n].SinkholeRounds != 0 || slices.Contains(exclude, n) {
			continue
		}
		out = append(out, n)
	}
	return out
}

// haltMovement clears validated[seat]'s remaining Route and Pushing On so
// advance() (movement.go) never moves this seat again this round, and
// forces its Action to Nothing — GDD §15: "loses the remainder of their
// route and their action." Applied to every confrontation participant whose
// final position this step isn't a straightforward continuation of their
// own plan: losers, tie participants, and the rare crossing-corrected
// winner (resolveDecisive).
func haltMovement(validated map[game.SeatID]game.Order, seat game.SeatID) {
	o := validated[seat]
	o.Route = nil
	o.PushingOn = game.PushingOn{}
	o.Action = game.ActionOrder{Kind: game.ActionNothing}
	validated[seat] = o
}

// applyMuscleLoss applies D14's ruling (MuscleLoss, items.go) to every
// participant: winner is nil for a tie (MuscleLoss(false, true) is always
// false, so this is a no-op walk in that case, kept for symmetry with the
// decisive path) and otherwise names the one seat MuscleLoss must treat as
// the winner.
func applyMuscleLoss(s *MatchState, seats []game.SeatID, winner *game.SeatID, tie bool) {
	for _, seat := range seats {
		isWinner := winner != nil && seat == *winner
		if !MuscleLoss(isWinner, tie) {
			continue
		}
		s.Players[seat].Items = removeOneItem(s.Players[seat].Items, game.ItemMuscle)
	}
}

// removeOneItem removes the first occurrence of item from items, leaving
// any further copies untouched — losing Muscle costs exactly one held
// instance, not the whole hand.
func removeOneItem(items []game.ItemID, item game.ItemID) []game.ItemID {
	if i := slices.Index(items, item); i >= 0 {
		return slices.Delete(items, i, i+1)
	}
	return items
}

// decisiveEvents records one EventConfrontation per loser, Seat the winner
// and Target that loser (RFC §9.1 row 7: "always named, both parties") —
// GDD §15's displacement discussion: "a confrontation writes a public trace
// naming both parties at that node."
func decisiveEvents(round game.RoundNumber, node game.NodeID, winner game.SeatID, losers []game.SeatID) []game.Event {
	events := make([]game.Event, 0, len(losers))
	for _, loser := range losers {
		events = append(events, game.Event{
			Kind:   game.EventConfrontation,
			Round:  round,
			Node:   node,
			Seat:   winner,
			Target: loser,
		})
	}
	return events
}
