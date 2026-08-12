package rules

import "github.com/garnizeh/cinzal/internal/game"

// Resolve is the entire round pipeline (RFC §6.1): a pure function, same
// inputs and same outputs on any machine, forever (RFC §6.3). It is a fixed
// sequence of small, independently testable steps (RFC §6.7); this issue
// (#67) builds the skeleton, the entry-snapshot wiring, and validate — the
// steps after validate are no-op stubs, one per the issue that implements
// it for real (see each stub's own doc comment).
//
// No error path exists yet at this layer: cfg is already validated at
// initial(), and a seat missing from orders is not a caller mistake — it
// is GDD §18's ordinary "the deadline expired with nothing submitted" case,
// handled inside validate via the absence default.
func Resolve(s MatchState, orders map[game.SeatID]game.Order, cfg game.Config, r *RNG) (MatchState, []game.Event, error) {
	next := s.clone()
	next.Round = s.Round + 1

	// The entry snapshot (RFC §6.6): "end of the previous round" truth,
	// frozen here before anything below can mutate it. Flagged and
	// EvasiveStepPenalty are step-allowance inputs consumed from this same
	// snapshot, never from the live field — so the live copies are reset
	// to false in this same top-of-Resolve step, before any resolution
	// step (including a same-round Debt trigger or Evasive loss, once
	// #69/#70 exist) can write to either again.
	entry := next.Snapshot()
	resetRoundFlags(next.Players)

	seats := bySeat(next)
	validated, events := validate(next, entry, seats, orders, cfg)

	// Steps 1..N — synchronized movement (RFC §6.7). The collision check
	// runs at least once per round even when every route is empty (GDD
	// §15: a table where nobody moves still resolves) — movementSteps
	// enforces that floor.
	for step := 1; step <= movementSteps(validated); step++ {
		advance(&next, validated, step)
		events = append(events, detectCrossings(next, validated, step)...)
		events = append(events, detectCollisions(next, step)...)
		events = append(events, resolveConfrontations(&next, r)...)
	}

	events = append(events, resolveActions(&next, validated, seats, r)...)
	events = append(events, resolveDeliveries(&next, r)...)
	events = append(events, resolveAddons(&next, validated)...)
	events = append(events, writeTrail(&next)...)
	events = append(events, globalEvent(&next, r)...)
	events = append(events, incident(&next, r)...)
	events = append(events, pressure(&next, r)...)
	events = append(events, upkeep(&next)...)

	return next, events, nil
}

// resetRoundFlags clears Flagged and EvasiveStepPenalty on every live
// Player, at the one point in the round (top of Resolve, alongside the
// entry snapshot) that can distinguish "the value this round already
// used" from "a value this round just set for next round" (RFC §6.6,
// D5) — an Upkeep-phase clear cannot make that distinction, which is
// exactly why these two counters are not in upkeep()'s step 4 list.
func resetRoundFlags(players []Player) {
	for i := range players {
		players[i].Flagged = false
		players[i].EvasiveStepPenalty = false
	}
}

// movementSteps is the movement loop's upper bound: the longest validated
// order's route plus its Pushing On extension, floored at 1 so the
// collision check still runs once on a round where every route is empty
// (RFC §6.7, GDD §15). Split out from Resolve's loop so that floor is
// directly unit-testable without instrumenting the movement stubs below.
func movementSteps(validated map[game.SeatID]game.Order) int {
	steps := 1
	for _, o := range validated {
		if n := len(o.Route) + o.PushingOn.Steps; n > steps {
			steps = n
		}
	}
	return steps
}

// advance is a stub — issue #68 implements synchronized movement.
func advance(s *MatchState, validated map[game.SeatID]game.Order, step int) {}

// detectCrossings is a stub — issue #68 implements crossing detection
// (two players traversing the same edge in opposite directions).
func detectCrossings(s MatchState, validated map[game.SeatID]game.Order, step int) []game.Event {
	return nil
}

// detectCollisions is a stub — issue #68 implements collision detection
// (two or more players ending a step on the same node).
func detectCollisions(s MatchState, step int) []game.Event { return nil }

// resolveConfrontations is a stub — issue #69 implements the confrontation,
// pushback, and displacement.
func resolveConfrontations(s *MatchState, r *RNG) []game.Event { return nil }

// resolveActions is a stub — issue #70 implements Step N+1: actions,
// resolved in ascending Infamy order.
func resolveActions(s *MatchState, validated map[game.SeatID]game.Order, seats []game.SeatID, r *RNG) []game.Event {
	return nil
}

// resolveDeliveries is a stub — issue #70 implements Step N+2: payments,
// Infamy, RP, and global delivery announcements.
func resolveDeliveries(s *MatchState, r *RNG) []game.Event { return nil }

// resolveAddons is a stub — issue #70 implements Step N+3: the Ledger and
// lease renewals.
func resolveAddons(s *MatchState, validated map[game.SeatID]game.Order) []game.Event { return nil }

// writeTrail is a stub — issue #71 implements Step N+4: Loitering
// evaluation, crate heat, per-node logs, and sight-gated distribution.
func writeTrail(s *MatchState) []game.Event { return nil }

// globalEvent is a stub — issue #72 implements the 24-card global event
// deck.
func globalEvent(s *MatchState, r *RNG) []game.Event { return nil }

// incident is a stub — issue #73 implements the 16-card sector incident
// deck.
func incident(s *MatchState, r *RNG) []game.Event { return nil }

// pressure is a stub — issue #73 implements Phase 7's Legend-tier pressure
// check.
func pressure(s *MatchState, r *RNG) []game.Event { return nil }

// upkeep is a stub — issue #74 implements Phase 8's four fixed steps
// (contract deadlines, leases, Sinkhole, next-round modifier clear).
func upkeep(s *MatchState) []game.Event { return nil }
