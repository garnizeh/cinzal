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
// field. step is resolve.go's own movement loop index (1-based, advance's
// same parameter) — carried through to haltStepsUnspent so every
// EventRouteHalted this call produces can report how much of the halted
// seat's declared plan was still outstanding (D39, GDD §22 row 1).
func resolveConfrontations(s *MatchState, pending []confrontation, validated map[game.SeatID]game.Order, walks map[game.SeatID]*seatWalk, step int, cfg game.Config, r *RNG) []game.Event {
	var events []game.Event
	for _, c := range pending {
		events = append(events, resolveOneConfrontation(s, c, validated, walks, step, cfg, r)...)
	}
	return events
}

// resolveOneConfrontation resolves a single node's confrontation: correct
// crossing positions, roll and total every participant (seat order — RFC
// §6.5: confrontation dice are an RNG batch, not a fairness-key selection),
// then apply the tie or decisive outcome.
func resolveOneConfrontation(s *MatchState, c confrontation, validated map[game.SeatID]game.Order, walks map[game.SeatID]*seatWalk, step int, cfg game.Config, r *RNG) []game.Event {
	corrected := correctCrossingPositions(s, c, walks)

	infamy := make(map[game.SeatID]int, len(c.Seats))
	for _, seat := range c.Seats {
		infamy[seat] = s.Players[seat].Infamy
	}
	lowest := lowestInfamy(infamy, c.Seats)

	totals := make(map[game.SeatID]int, len(c.Seats))
	for _, seat := range c.Seats {
		totals[seat] = confrontationTotal(s, seat, c.Node, validated[seat], infamy[seat], lowest, walks[seat], cfg, r)
	}

	winner, tie := determineOutcome(c.Seats, totals)
	if tie {
		haltEvents := resolveTie(s, c, validated, walks, step)
		applyMuscleLoss(s, c.Seats, nil, true)
		return append(haltEvents, tieEvents(s.Round, c.Node, c.Seats, validated)...)
	}

	losers := orderLosersForPushback(*s, losersExcept(c.Seats, winner))
	events := resolveDecisive(s, c, winner, corrected[winner], losers, validated, walks, step, cfg, r)
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
func confrontationTotal(s *MatchState, seat game.SeatID, node game.NodeID, o game.Order, infamy, lowest int, walk *seatWalk, cfg game.Config, r *RNG) int {
	roll := r.Next(PurposeConfrontD6, 6) + 1

	total := roll + infamyTierIndex(infamy, cfg) + stanceModifier(o.Stance.Stance)

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
	return hasDiscard(o, game.ItemShiv)
}

// hasDiscard reports whether o declares item as a Field 4 discard this
// round — the general form of hasShivDiscard above, needed by issue #73's
// Circulation Permit immunity check (incidents.go's incidentEligible) and
// Gas Leak's own immunity check (movement.go's advance).
func hasDiscard(o game.Order, item game.ItemID) bool {
	return slices.ContainsFunc(o.Items, func(d game.ItemDiscard) bool {
		return d.Item == item
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
// declared route and action this round are forfeit regardless: the GDD
// does not restate that for a tie the way it does for a Loser, but it is
// an implementation necessity — advance() (movement.go) trusts
// Route[step-1] to be adjacent to the seat's actual current position, and
// a reverted position generally invalidates whatever the rest of a
// multi-step route assumed. Since D39, the round's remaining steps are not
// lost with it: haltOrConvertMovement below turns any unspent budget into a
// blind Pushing On walk from dest, and Previous is pointed at c.Node — not
// dest's own predecessor — so that walk's first step cannot re-enter the
// node the tie happened at (§9.1's ladder level 5 does the rest). Returns
// one EventRouteHalted per participant, HaltCauseTie and each one's own
// HaltStepsUnspent (D39; step is resolve.go's movement loop index, the same
// parameter advance takes) — unchanged in meaning by D39's continuation:
// still how many steps of the declared plan were cut short, whether or not
// they survive as blind ones.
func resolveTie(s *MatchState, c confrontation, validated map[game.SeatID]game.Order, walks map[game.SeatID]*seatWalk, step int) []game.Event {
	events := make([]game.Event, 0, len(c.Seats))
	for _, seat := range c.Seats {
		dest := walks[seat].Previous
		s.Players[seat].Position = dest
		walks[seat].Path = []game.NodeID{dest}
		unspent := haltOrConvertMovement(validated, seat, step)
		if unspent > 0 {
			walks[seat].Previous = c.Node
		}
		events = append(events, game.Event{Kind: game.EventRouteHalted, Round: s.Round, Node: c.Node, Seat: seat, HaltCause: game.HaltCauseTie, HaltStepsUnspent: unspent})
	}
	return events
}

// tieEvents records that a confrontation happened here even though nobody
// won it — GDD §15's displacement discussion: "a confrontation writes a
// public trace naming both parties at that node." A tie has no winner/loser
// pairing to name, so this emits one EventConfrontation per adjacent pair
// in seat order (K-1 events for K tied participants, one event for the
// common two-way case) rather than every combinatorial pair.
func tieEvents(round game.RoundNumber, node game.NodeID, seats []game.SeatID, validated map[game.SeatID]game.Order) []game.Event {
	events := make([]game.Event, 0, len(seats)-1)
	for i := 0; i+1 < len(seats); i++ {
		events = append(events, game.Event{
			Kind:   game.EventConfrontation,
			Round:  round,
			Node:   node,
			Seat:   seats[i],
			Target: seats[i+1],
			Stance: validated[seats[i+1]].Stance.Stance,
			// Decisive left false: a tie has no winner/loser pairing to
			// name (D33 row 9) — see tieEvents' own doc above.
		})
	}
	return events
}

// resolveDecisive applies GDD §15's Winner and Loser sections once a unique
// top scorer is known. winnerCorrected is correctCrossingPositions' report
// for winner: true only for the rare crossing case where the winner's own
// pre-fight position was the "blocked" side (their attempted move undone to
// reach c.Node) — their remaining route no longer starts from where it
// assumed, so their declared route and action are forfeit exactly like a
// loser's despite winning, though since D39 their remaining steps are not:
// haltOrConvertMovement below turns any unspent budget into a blind Pushing
// On walk from c.Node, the node the winner is already standing on — no
// walks[winner].Previous write and no exclusion needed, unlike the loser and
// tie cases, since there is nowhere behind them to backtrack into. Every
// other case leaves the winner's route untouched, "route continues" exactly
// as written. step is resolve.go's movement loop index, needed by
// haltOrConvertMovement for every EventRouteHalted this call (and
// resolveLoser's) produces (D39).
func resolveDecisive(s *MatchState, c confrontation, winner game.SeatID, winnerCorrected bool, losers []game.SeatID, validated map[game.SeatID]game.Order, walks map[game.SeatID]*seatWalk, step int, cfg game.Config, r *RNG) []game.Event {
	s.Players[winner].Infamy = ApplyInfamyDelta(s.Players[winner].Infamy, InfamyGainConfrontationWin)
	var haltEvents []game.Event
	if winnerCorrected {
		unspent := haltOrConvertMovement(validated, winner, step)
		haltEvents = append(haltEvents, game.Event{Kind: game.EventRouteHalted, Round: s.Round, Node: c.Node, Seat: winner, HaltCause: game.HaltCauseCorrectedWinner, HaltStepsUnspent: unspent})
	}

	var eligible []game.SeatID
	stakeCollected, shakedownCollected := 0, 0
	for _, seat := range losers {
		share, shaken, forfeit, haltEvent := resolveLoser(s, c, seat, validated, walks, step, cfg, r)
		stakeCollected += share
		shakedownCollected += shaken
		haltEvents = append(haltEvents, haltEvent)
		if forfeit {
			eligible = append(eligible, seat)
		}
	}
	s.Players[winner].Balance += stakeCollected + shakedownCollected

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

	return append(haltEvents, decisiveEvents(s.Round, c.Node, winner, losers, validated)...)
}

// resolveLoser applies GDD §15's Loser consequences for seat, in order:
// stake, cargo (shakedown or forfeiture), Deadline Pause, Infamy, then
// pushback and converting the remainder of their round into blind steps
// (D39). Returns the Cr$ the winner collects from this loser's stake (half
// of what was actually forfeited, rounded down — never half of the declared
// stake: legalBalance (legal.go) deliberately excludes Aggressive stake from
// the submission-time affordability check, so a stake can legally outgrow
// balance by the time it resolves), the Cr$ shakedown this loser paid — GDD
// §15: "pay the Cr$4 shakedown to the winner", so the caller must credit it
// there too, exactly like the stake share — whether this seat's cargo is
// forfeit and still theirs to give — the caller decides whether the winner
// takes it or it drops at the node — and this loser's own EventRouteHalted,
// HaltCauseDecisiveLoser and its HaltStepsUnspent (D39; step is resolve.go's
// movement loop index) — unchanged in meaning by D39's continuation: still
// how many steps of the declared plan were cut short, whether or not they
// survive as blind ones.
func resolveLoser(s *MatchState, c confrontation, seat game.SeatID, validated map[game.SeatID]game.Order, walks map[game.SeatID]*seatWalk, step int, cfg game.Config, r *RNG) (stakeShare, shakedownPaid int, forfeitCargo bool, haltEvent game.Event) {
	p := &s.Players[seat]
	o := validated[seat]

	staked := min(o.Stance.Stake, p.Balance)
	p.Balance -= staked

	// GDD §15: "If Evasive: −1 step next round" — unconditional on the
	// stance, not on whether the shakedown below succeeds or cargo was
	// even carried. Consumed from the entry snapshot by steps.go, cleared
	// at the top of the following Resolve call (resetRoundFlags), exactly
	// like Flagged (RFC §6.6).
	if o.Stance.Stance == game.StanceEvasive {
		p.EvasiveStepPenalty = true
	}

	if p.Cargo != nil {
		protected := false
		if o.Stance.Stance == game.StanceEvasive {
			shakedownPaid = Shakedown(p.Balance, cfg.ShakedownCost)
			p.Balance -= shakedownPaid
			protected = shakedownPaid == cfg.ShakedownCost
		}

		if p.Cargo.Bound {
			applyDeadlinePause(p)
		}

		forfeitCargo = !protected
	}

	p.Infamy = ApplyInfamyDelta(p.Infamy, InfamyLossConfrontationLoss)

	pushback(s, c, seat, o.Stance.Stance == game.StanceEvasive, walks, r)
	unspent := haltOrConvertMovement(validated, seat, step)
	if unspent > 0 {
		// The loser now stands at the pushback destination, not at c.Node —
		// walk.Previous would otherwise still hold whatever ordinary
		// predecessor advance() last recorded, and the exclusion has to
		// name the confrontation node specifically (D39).
		walks[seat].Previous = c.Node
	}
	haltEvent = game.Event{Kind: game.EventRouteHalted, Round: s.Round, Node: c.Node, Seat: seat, HaltCause: game.HaltCauseDecisiveLoser, HaltStepsUnspent: unspent}

	return staked / 2, shakedownPaid, forfeitCargo, haltEvent
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
	dropped := Cargo{Node: node, Bound: p.Cargo.Bound, SpilledLoad: p.Cargo.SpilledLoad}

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
// pushback from the right history. dest becomes Known in seat's own Fog
// regardless of how it was reached — GDD §7.2's "a visited node becomes
// Known permanently" is unconditional, and the Displacement rule (GDD §15)
// states sight follows "wherever you actually end the round... however you
// got there." No Scavenging roll fires here even when dest was genuinely
// Hidden: Scavenging (movement.go's scavenge) is tied to a deliberate,
// step-spending exploration choice (GDD §9.1), which an involuntary
// displacement is not.
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

	p := &s.Players[seat]
	p.Position = dest
	markNodeKnown(p, dest)
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
// route and their action." Since D39, this is no longer every confrontation
// halt site's whole story — see haltOrConvertMovement below, which calls
// this only for the case D39 left unchanged: a seat with no budget left to
// convert.
func haltMovement(validated map[game.SeatID]game.Order, seat game.SeatID) {
	o := validated[seat]
	o.Route = nil
	o.PushingOn = game.PushingOn{}
	o.Action = game.ActionOrder{Kind: game.ActionNothing}
	validated[seat] = o
}

// haltOrConvertMovement is D39's replacement for a bare haltMovement call at
// all three of resolveConfrontations' halt sites (resolveTie, resolveLoser,
// resolveDecisive's corrected winner): "any confrontation participant whose
// declared route can no longer be walked from where they now stand keeps
// the round's remaining steps, and spends them as GDD §9.1 blind steps."
// The declared route and action are forfeit exactly as haltMovement always
// made them — Route nil, Action Nothing — but when o's declared plan (Route
// plus Pushing On, read before this call mutates anything) still had a step
// left unspent at movement step step, that budget is not thrown away: Route
// is truncated to its already-walked prefix (so advance's own
// `step <= len(o.Route)` check reads it as exhausted from the very next
// step onward) and PushingOn.Steps is set to the unspent count, turning the
// remainder into a fresh blind walk from wherever the seat now stands. A
// seat with no budget left (unspent <= 0) gets exactly haltMovement's old
// behaviour — there is nothing to continue.
//
// This does not set walk.Previous — the exclusion GDD §9.1's ladder level 5
// needs to keep that blind walk's first step from re-entering the
// confrontation node is the caller's own job, since resolveTie and
// resolveLoser stand somewhere other than c.Node by the time this runs
// while resolveDecisive's corrected winner stands on c.Node itself and
// needs no exclusion at all (D39).
//
// Returns the same unspent count haltStepsUnspent already computed —
// unchanged in meaning by this rule: still how many steps of the declared
// plan were cut short, whether or not they now survive as blind ones (D39,
// GDD §22 row 1's numerator) — for the caller's own EventRouteHalted.
func haltOrConvertMovement(validated map[game.SeatID]game.Order, seat game.SeatID, step int) int {
	o := validated[seat]
	unspent := haltStepsUnspent(o, step)
	if unspent <= 0 {
		haltMovement(validated, seat)
		return 0
	}

	walked := min(step, len(o.Route))
	o.Route = o.Route[:walked]
	o.PushingOn.Steps = unspent
	o.Action = game.ActionOrder{Kind: game.ActionNothing}
	validated[seat] = o
	return unspent
}

// haltStepsUnspent returns how many steps of o's declared plan — Route and
// Pushing On combined — were still unspent at movement step step (1-based,
// resolve.go's own loop index, advance's same parameter): advance consumes
// exactly one step of Route then Pushing On per loop iteration (movement.go),
// so step steps of the combined len(o.Route)+o.PushingOn.Steps plan have
// been used by the time this step's confrontations resolve, regardless of
// whether this particular seat moved this step — GDD §15b's collision check
// evaluates every seat's position "whether or not they moved." Floored at 0
// for a seat whose plan had already run out before this step. Must be
// called with o read before haltMovement or haltOrConvertMovement mutates
// it (D39, GDD §22 row 1's "at least one step ... unspent" numerator).
func haltStepsUnspent(o game.Order, step int) int {
	if left := len(o.Route) + o.PushingOn.Steps - step; left > 0 {
		return left
	}
	return 0
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
// naming both parties at that node." Stance names each loser's declared
// stance and Decisive is always true here — GDD §22's "confrontations won
// against an Evasive loser" (D33 row 9).
func decisiveEvents(round game.RoundNumber, node game.NodeID, winner game.SeatID, losers []game.SeatID, validated map[game.SeatID]game.Order) []game.Event {
	events := make([]game.Event, 0, len(losers))
	for _, loser := range losers {
		events = append(events, game.Event{
			Kind:     game.EventConfrontation,
			Round:    round,
			Node:     node,
			Seat:     winner,
			Target:   loser,
			Stance:   validated[loser].Stance.Stance,
			Decisive: true,
		})
	}
	return events
}
