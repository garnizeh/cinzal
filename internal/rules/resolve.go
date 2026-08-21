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

	// Phase 2's own answer (D29): before validate, so a seat already
	// standing at a newly-accepted contract's origin can legally Pickup
	// this same round — applyContractChoices reads orders directly
	// (raw, not yet validated), applying whatever offer.go's
	// prepareNextRound staged onto PendingOffer at the tail of the
	// round that just closed (or, for round 1, at initial()'s own
	// bootstrap).
	applyContractChoices(&next, orders, cfg, next.Round)

	// Upkeep step 4's own entry snapshot (D5): s.NextRound as it stood
	// entering this round, before Phase 6's global event deck draw below
	// can set a fresh value for the round about to start. upkeep
	// (upkeep.go) clears only what this captures, never a same-round-fresh
	// flag — see that function's own doc comment and state.go's NextRound
	// comment for why an unconditional clear is wrong.
	entryNextRound := next.NextRound.clone()

	// Global event peek (issue #72, GDD §14.2): the deck's card for this
	// round is fixed at Setup, so buildGlobalEventContext peeks it — and,
	// for Dragnet only, draws its target set — before validate/movement
	// run, exactly the "decide early, reveal late" pattern the deck build
	// itself already established for the Headline paradox (RFC §6.4). Every
	// other card resolves entirely inside globalEvent, at its normal Phase
	// 6 call site below; see events.go's own doc comment for the full
	// grouping.
	ctx := buildGlobalEventContext(next, r)

	// Two-player rotating borders (GDD §6.3, D03) plus Dragnet's own seal
	// above, combined and D28-safety-valved: blockedBordersThisRound
	// (borders.go) is every Border this round's Deliver orders cannot
	// route to. Nil at 3+ players whenever Dragnet didn't fire — identical
	// to today's Dragnet-only behaviour, since rotation contributes
	// nothing there.
	blockedBorders := blockedBordersThisRound(next.Graph, next.Round, len(next.Players), ctx.sealedBorders)

	// Sector incident peek (issue #73, GDD §14.3): the flagged sector for
	// this round was already decided a round ago (MatchState.UnstableSector,
	// state.go) — incidentContext is a non-consuming re-read of it plus
	// this round's card, needed before movement (Gas Leak) and before
	// writeTrail (Riot, Distracted Guard's own-trace suppression), both of
	// which run before Phase 7's own sector-incident call site below.
	incCtx := buildIncidentContext(next)

	seats := bySeat(next)
	validated, events := validate(next, entry, seats, orders, cfg, ctx, blockedBorders)

	// Steps 1..N — synchronized movement (RFC §6.7). The collision check
	// runs at least once per round even when every route is empty (GDD
	// §15: a table where nobody moves still resolves) — movementSteps
	// enforces that floor. walks is #68's own per-round movement
	// bookkeeping (seatWalk, movement.go) — local to this call, never part
	// of MatchState.
	walks := newSeatWalks(next, seats)
	for step := 1; step <= movementSteps(seats, validated); step++ {
		transitions := advance(&next, walks, validated, seats, step, incCtx, r)
		crossings := detectCrossings(transitions, seats, validated)
		collisions := detectCollisions(next, seats)
		pending := mergeConfrontations(next, crossings, collisions)
		events = append(events, resolveConfrontations(&next, pending, validated, walks, step, cfg, r)...)
	}

	actionEvents, vanishReducedInfamy := resolveActions(&next, validated, seats, cfg, ctx, r)
	events = append(events, actionEvents...)
	events = append(events, resolveFenceWindfallClaim(&next, seats, r)...)
	events = append(events, resolveDeliveries(&next, validated, cfg, ctx)...)
	events = append(events, resolveAddons(&next, validated, entry, cfg, ctx)...)
	events = append(events, writeTrail(&next, validated, seats, walks, vanishReducedInfamy, events, ctx, incCtx, r)...)
	events = append(events, globalEvent(&next, ctx, events, r)...)
	events = append(events, incident(&next, incCtx, validated, seats, walks, cfg, r)...)
	events = append(events, pressure(&next, cfg, r)...)
	events = append(events, upkeep(&next, cfg, entryNextRound)...)

	// Phases 2 and 3 for the round about to begin (D29): staged here,
	// after upkeep and before RoundAnchors, using next — the fully-
	// closed state this call is about to return — and the same *RNG
	// parameter r every other draw in this package already uses. Skipped
	// entirely once there is no further round to prepare for, mirroring
	// nextUnstableSector's own guard (incidents.go).
	if int(next.Round) < cfg.Rounds {
		prepareNextRound(&next, cfg, r)
	}

	// RoundAnchors (#75, anchors.go): the round's own global,
	// unconditional position-writer facts, extracted from the full event
	// stream just before it leaves Resolve's hands — the only point in
	// the pipeline that ever sees every event this round produced. See
	// MatchState.RoundAnchors's own doc comment for why Project needs
	// this stored rather than reading events directly.
	next.RoundAnchors = buildRoundAnchors(events, next.Round)

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
		players[i].DistractedGuard = false
		players[i].StreetsBlocked = false
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

// writeTrail (issue #71) is defined in trail.go: Step N+4's Loitering
// evaluation, crate heat, per-node trail logs, and sight-gated distribution
// into every seat's SeatArchive.

// globalEvent (issue #72) is defined in events.go: Phase 6's 24-card global
// event deck — buildGlobalEventContext (also events.go) is this same
// issue's round-start peek, called above, before validate.

// incident and pressure (issue #73) are defined in incidents.go: Phase 7's
// 16-card sector incident deck and the Legend-tier Pressure check —
// buildIncidentContext (also incidents.go) is this same issue's round-start
// peek, called above, before validate.

// upkeep (issue #74) is defined in upkeep.go: Phase 8's four fixed,
// dependency-ordered steps (contract deadlines, leases, Sinkhole,
// next-round modifier clear).
