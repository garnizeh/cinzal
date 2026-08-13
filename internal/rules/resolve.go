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
	// enforces that floor. walks is #68's own per-round movement
	// bookkeeping (seatWalk, movement.go) — local to this call, never part
	// of MatchState.
	walks := newSeatWalks(next, seats)
	for step := 1; step <= movementSteps(seats, validated); step++ {
		transitions := advance(&next, walks, validated, seats, step, r)
		crossings := detectCrossings(transitions, seats, validated)
		collisions := detectCollisions(next, seats)
		pending := mergeConfrontations(next, crossings, collisions)
		events = append(events, resolveConfrontations(&next, pending, validated, walks, cfg, r)...)
	}

	events = append(events, resolveActions(&next, validated, seats, cfg, r)...)
	events = append(events, resolveDeliveries(&next, validated, cfg)...)
	events = append(events, resolveAddons(&next, validated, entry, cfg)...)
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
//
// Ledger shares the same "next round only, consumed once" lifecycle (D1,
// #70): a seat's Ledger purchase is only ever meant to inform the very
// next order phase, so it is cleared here too, before this round's own
// resolveAddons (addons.go) can write a fresh one.
func resetRoundFlags(players []Player) {
	for i := range players {
		players[i].Flagged = false
		players[i].EvasiveStepPenalty = false
		players[i].Ledger = nil
	}
}

// movementSteps is the movement loop's upper bound: the longest validated
// order's route plus its Pushing On extension, floored at 1 so the
// collision check still runs once on a round where every route is empty
// (RFC §6.7, GDD §15). Split out from Resolve's loop so that floor is
// directly unit-testable without instrumenting the movement stubs below.
// Iterates seats rather than ranging validated directly (CLAUDE.md:
// resolution must not range over maps) — today's computation is a maximum,
// so iteration order can't affect the result, but keeping the stable order
// here means this stays true if the loop ever grows a step that isn't.
func movementSteps(seats []game.SeatID, validated map[game.SeatID]game.Order) int {
	steps := 1
	for _, seat := range seats {
		o := validated[seat]
		if n := len(o.Route) + o.PushingOn.Steps; n > steps {
			steps = n
		}
	}
	return steps
}

// advance, detectCrossings, detectCollisions, and mergeConfrontations are
// issue #68's synchronized-movement, crossing, and collision detection —
// see movement.go.

// resolveConfrontations (issue #69) is defined in confront.go: the
// confrontation dice roll, pushback, and displacement, for every group
// #68's detection (advance, detectCrossings, detectCollisions,
// mergeConfrontations, movement.go) already located and node-ordered (GDD
// §15; RFC §6.5).

// resolveActions (issue #70) is defined in actions.go: Step N+1's six
// contended/uncontended actions (Pickup, Stake Post, Deal, Vanish,
// Surveil, Nothing), resolved in ascending Infamy order.

// resolveDeliveries (issue #70) is defined in deliveries.go: Step N+2's
// Deliver, payments, Infamy, RP, and the global delivery announcement.

// resolveAddons (issue #70) is defined in addons.go: Step N+3's Ledger and
// lease renewals.

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
