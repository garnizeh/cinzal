package rules

import (
	"os"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// This file is issue #79's cross-round counter suite (RFC §16.1): one test
// per row of RFC §6.6's table, each framed as "fires on round N, not N-1 or
// N+1" — not merely that the counter moved. #74 already built the *upkeep*
// half (which rows Upkeep clears and when); this is the *consumption* half.

// --- LastEndNode / LoiteringStreak ---

// crossRoundLineGraph is a straight line of eight Alley nodes (Loitering
// has no exemption for Alley — GDD §9.1's exemption is Warehouse/Border
// only), chosen so a test can pick two "last ending node" candidates that
// are provably non-adjacent to each other: if evaluateLoitering (trail.go)
// ever read a value more than one round stale, this graph makes that
// observable rather than accidentally still adjacent.
func crossRoundLineGraph() Graph {
	nodes := make([]Node, 9)
	for i := range nodes {
		nodes[i] = Node{ID: game.NodeID(i), Type: game.NodeAlley}
		if i > 0 {
			nodes[i].Edges = append(nodes[i].Edges, game.NodeID(i-1))
		}
		if i < len(nodes)-1 {
			nodes[i].Edges = append(nodes[i].Edges, game.NodeID(i+1))
		}
	}
	return Graph{Nodes: nodes}
}

func crossRoundLineState(round game.RoundNumber, seatPos game.NodeID) MatchState {
	return MatchState{
		Round: round,
		Graph: crossRoundLineGraph(),
		Players: []Player{
			{Seat: 0, Position: seatPos, LastEndNode: seatPos, Fog: make([]game.FogState, 9)},
		},
	}
}

// TestCrossRoundLastEndNodeReadsOnlyThePreviousRound is RFC §6.6's
// LastEndNode row: Loitering's 1-step radius test must compare this round's
// ending position against *last* round's, never anything staler. Round 1
// ends at node 0 (streak 1). Round 2 moves to node 4 — not adjacent to node
// 0 — which both breaks the streak and sets LastEndNode to 4. Round 3 moves
// to node 1: adjacent to node 0 (round 1's value) but *not* adjacent to
// node 4 (round 2's value) and not equal to it either. A correct read of
// "the previous round" (4) reports no loitering in round 3; a stale
// two-rounds-back read (0) would incorrectly restart the streak.
func TestCrossRoundLastEndNodeReadsOnlyThePreviousRound(t *testing.T) {
	s := crossRoundLineState(1, 0)
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionNothing}}}
	rng := NewRNG(testSeed(0x79), 1)

	// Round 1: stay at node 0 — within one step of the seeded LastEndNode
	// (0 itself) — loitering, streak 1.
	writeTrail(&s, validated, bySeat(s), trailTestWalks(0), nil, nil, globalEventContext{}, incidentContext{}, rng)
	if s.Players[0].LoiteringStreak != 1 {
		t.Fatalf("round 1: LoiteringStreak = %d, want 1", s.Players[0].LoiteringStreak)
	}
	if s.Players[0].LastEndNode != 0 {
		t.Fatalf("round 1: LastEndNode = %d, want 0", s.Players[0].LastEndNode)
	}

	// Round 2: move to node 4 — not adjacent to node 0 — breaks the streak.
	s.Round = 2
	s.Players[0].Position = 4
	writeTrail(&s, validated, bySeat(s), trailTestWalks(4), nil, nil, globalEventContext{}, incidentContext{}, rng)
	if s.Players[0].LoiteringStreak != 0 {
		t.Fatalf("round 2: LoiteringStreak = %d, want 0 (node 4 is not within one step of node 0)", s.Players[0].LoiteringStreak)
	}
	if s.Players[0].LastEndNode != 4 {
		t.Fatalf("round 2: LastEndNode = %d, want 4", s.Players[0].LastEndNode)
	}

	// Round 3: move to node 1 — adjacent to node 0 (round 1's stale value)
	// but not to node 4 (round 2's real value, and not equal to it).
	s.Round = 3
	s.Players[0].Position = 1
	writeTrail(&s, validated, bySeat(s), trailTestWalks(1), nil, nil, globalEventContext{}, incidentContext{}, rng)
	if s.Players[0].LoiteringStreak != 0 {
		t.Fatalf("round 3: LoiteringStreak = %d, want 0 — LastEndNode must read round 2's value (4), not round 1's stale value (0)", s.Players[0].LoiteringStreak)
	}
}

// TestCrossRoundLoiteringStreakEscalatesAtExactlyTwoAndThree is the same
// row RFC §16.1 names by example ("Loitering escalates at exactly 2 and
// 3"). See also TestWriteTrailLoiteringEscalation (trail_test.go), which
// this test deliberately does not duplicate in depth — it exists here to
// keep the "one test per §6.6 row" mapping visible in this file, framed
// explicitly around the firing round rather than the archive-vs-Anchor
// distinction that test's own doc comment focuses on.
func TestCrossRoundLoiteringStreakEscalatesAtExactlyTwoAndThree(t *testing.T) {
	s := crossRoundLineState(1, 2)
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionNothing}}}

	for round := game.RoundNumber(1); round <= 3; round++ {
		s.Round = round
		events := writeTrail(&s, validated, bySeat(s), trailTestWalks(2), nil, nil, globalEventContext{}, incidentContext{}, NewRNG(testSeed(1), int(round)))
		fired := hasEvent(events, game.EventLoitering, 0)

		switch round {
		case 1:
			if fired {
				t.Error("round 1 (streak 1): EventLoitering fired, want silence")
			}
		case 2, 3:
			if !fired {
				t.Errorf("round %d (streak %d): EventLoitering did not fire, want it to", round, round)
			}
		}
	}
}

// --- LooseCrateHeldRounds ---

// TestCrossRoundLooseCrateHeldRoundsFiresOnSecondConsecutiveRound is RFC
// §6.6's LooseCrateHeldRounds row, at the Player field itself rather than
// NextLooseCrateHeldRounds/LooseCrateHeatFires in isolation (cargo_test.go
// already pins those pure functions) — this confirms the field is actually
// wired into writeTrail's per-round call, firing the global announcement on
// the second consecutive round held, not the first, and resetting the
// moment the crate is no longer carried.
func TestCrossRoundLooseCrateHeldRoundsFiresOnSecondConsecutiveRound(t *testing.T) {
	s := crossRoundLineState(1, 2)
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false} // a loose crate
	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionNothing}}}

	events := writeTrail(&s, validated, bySeat(s), trailTestWalks(2), nil, nil, globalEventContext{}, incidentContext{}, NewRNG(testSeed(1), 1))
	if s.Players[0].LooseCrateHeldRounds != 1 {
		t.Fatalf("round 1: LooseCrateHeldRounds = %d, want 1", s.Players[0].LooseCrateHeldRounds)
	}
	if hasEvent(events, game.EventLooseCrateHeld, 0) {
		t.Error("round 1: EventLooseCrateHeld fired, want silence (held only 1 round)")
	}

	s.Round = 2
	events = writeTrail(&s, validated, bySeat(s), trailTestWalks(2), nil, nil, globalEventContext{}, incidentContext{}, NewRNG(testSeed(1), 2))
	if s.Players[0].LooseCrateHeldRounds != 2 {
		t.Fatalf("round 2: LooseCrateHeldRounds = %d, want 2", s.Players[0].LooseCrateHeldRounds)
	}
	if !hasEvent(events, game.EventLooseCrateHeld, 0) {
		t.Error("round 2: EventLooseCrateHeld did not fire, want the second-consecutive-round announcement")
	}

	// Round 3: the crate is dropped — the streak must reset, not merely
	// hold at 2.
	s.Round = 3
	s.Players[0].Cargo = nil
	writeTrail(&s, validated, bySeat(s), trailTestWalks(2), nil, nil, globalEventContext{}, incidentContext{}, NewRNG(testSeed(1), 3))
	if s.Players[0].LooseCrateHeldRounds != 0 {
		t.Fatalf("round 3 (dropped): LooseCrateHeldRounds = %d, want 0", s.Players[0].LooseCrateHeldRounds)
	}
}

// --- Flagged ---

// TestCrossRoundFlaggedCostsAStepOnlyOnTheEntrySnapshotRound is RFC §16.1's
// own example: "Flagged costs a step on exactly the next round and not the
// one after." validate (validate.go) reads Flagged from entry, the frozen
// EntrySnapshot taken at the top of the round it applies to — never from
// the live Player field — so this proves the -1 lands on exactly the round
// whose entry snapshot carried it. Pairs with
// TestResolveFreezesFlaggedAndEvasiveStepPenalty (resolve_test.go), which
// proves the live field is reset before any later round's own snapshot
// could see it — between the two, Flagged is shown to apply on round N+1
// only, never N+2.
func TestCrossRoundFlaggedCostsAStepOnlyOnTheEntrySnapshotRound(t *testing.T) {
	s := resolveTestState() // 0-1-2-3-4 line graph, seat 0 at node 0
	cfg := legalTestConfig()
	seats := []game.SeatID{0, 1}
	// A route exactly at the Nobody-tier allowance (4 steps) without any
	// modifier — 0 -> 1 -> 0 -> 1 -> 0, using the mutual 0<->1 edge so
	// length is the only thing that could trip the check.
	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1, 0, 1, 0}, Action: game.ActionOrder{Kind: game.ActionNothing}},
	}

	t.Run("Flagged this round: the 4-step route no longer fits", func(t *testing.T) {
		s.Players[0].Flagged = true
		entry := s.Snapshot()
		validated, events := validate(s, entry, seats, orders, cfg, globalEventContext{}, nil)

		if !hasEvent(events, game.EventOrderRejected, 0) {
			t.Fatal("no EventOrderRejected — want the route rejected once Flagged's -1 drops the allowance to 3")
		}
		if len(validated[0].Route) != 0 {
			t.Errorf("validated route = %v, want the absence default (empty route)", validated[0].Route)
		}
	})

	t.Run("not Flagged: the identical route is accepted unchanged", func(t *testing.T) {
		s.Players[0].Flagged = false
		entry := s.Snapshot()
		validated, events := validate(s, entry, seats, orders, cfg, globalEventContext{}, nil)

		if hasEvent(events, game.EventOrderRejected, 0) {
			t.Fatal("EventOrderRejected fired, want the 4-step route accepted at the full Nobody-tier allowance")
		}
		if got := validated[0].Route; len(got) != 4 {
			t.Errorf("validated route = %v, want the original 4-step route unchanged", got)
		}
	})
}

// --- EvasiveStepPenalty ---

// TestCrossRoundEvasiveStepPenaltyCostsAStepOnlyOnTheEntrySnapshotRound
// mirrors the Flagged test above for EvasiveStepPenalty — both are
// consumed from the same frozen snapshot, by the same formula (steps.go).
func TestCrossRoundEvasiveStepPenaltyCostsAStepOnlyOnTheEntrySnapshotRound(t *testing.T) {
	s := resolveTestState()
	cfg := legalTestConfig()
	seats := []game.SeatID{0, 1}
	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1, 0, 1, 0}, Action: game.ActionOrder{Kind: game.ActionNothing}},
	}

	s.Players[0].EvasiveStepPenalty = true
	entry := s.Snapshot()
	_, events := validate(s, entry, seats, orders, cfg, globalEventContext{}, nil)
	if !hasEvent(events, game.EventOrderRejected, 0) {
		t.Fatal("no EventOrderRejected — want the route rejected once EvasiveStepPenalty's -1 drops the allowance to 3")
	}
}

// TestCrossRoundEvasiveStepPenaltyDoesNotStack is RFC §16.1's own example:
// "EvasiveStepPenalty does not stack with itself." EvasiveStepPenalty is
// declared bool (state.go), not a counter, and Steps (steps.go) applies it
// as a single `if m.EvasiveStepPenalty { steps-- }` — never proportional to
// how many Evasive losses set it. This drives resolveLoser (confront.go)
// twice against the same seat in one round to prove the flag itself cannot
// accumulate — not reachable through the real pipeline, since
// haltMovement stops a loser after their first loss, but the field's own
// correctness must not depend on that being the only way to reach it.
func TestCrossRoundEvasiveStepPenaltyDoesNotStack(t *testing.T) {
	s := confrontTestState(12, 12)
	c := confrontation{Node: 12, Seats: []game.SeatID{0, 1}}
	validated := map[game.SeatID]game.Order{
		1: {Stance: game.StanceOrder{Stance: game.StanceEvasive}},
	}
	walks := confrontWalks(map[game.SeatID][]game.NodeID{0: {12}, 1: {12}})
	cfg := legalTestConfig()
	r := NewRNG(testSeed(0x79), int(s.Round))

	resolveLoser(&s, c, 1, validated, walks, 1, cfg, r)
	if !s.Players[1].EvasiveStepPenalty {
		t.Fatal("EvasiveStepPenalty = false after one Evasive loss, want true")
	}

	resolveLoser(&s, c, 1, validated, walks, 1, cfg, r)
	if !s.Players[1].EvasiveStepPenalty {
		t.Fatal("EvasiveStepPenalty = false after a second Evasive loss, want still true")
	}

	v := game.PlayerView{You: game.SelfState{StepModifiers: game.StepModifiers{EvasiveStepPenalty: true}}}
	if got, want := Steps(v, cfg), cfg.StepsByTier[0]-1; got != want {
		t.Errorf("Steps() with EvasiveStepPenalty set = %d, want %d — the penalty applies once regardless of how many losses set it", got, want)
	}
}

// --- LastOfferRound ---

// TestCrossRoundContactCooldownOpensOnExactlyLastOfferRoundPlusCooldown is
// RFC §6.6's LastOfferRound row: the Contact Cooldown (GDD §8.2) must block
// through exactly LastOfferRound + cooldown(tier) - 1 and open at
// LastOfferRound + cooldown(tier), for every Infamy tier's own cooldown
// value (Config.CooldownByTier).
func TestCrossRoundContactCooldownOpensOnExactlyLastOfferRoundPlusCooldown(t *testing.T) {
	cfg := legalTestConfig()

	for tier, cooldown := range cfg.CooldownByTier {
		s := MatchState{
			Round:   10,
			Players: []Player{{Seat: 0, Infamy: tierFloorInfamy(tier), LastOfferRound: 5}},
		}

		s.Round = game.RoundNumber(5 + cooldown - 1)
		if offerDue(s, 0, cfg) {
			t.Errorf("tier %d: offerDue at round %d = true, want false (cooldown %d has not elapsed)", tier, s.Round, cooldown)
		}

		s.Round = game.RoundNumber(5 + cooldown)
		if !offerDue(s, 0, cfg) {
			t.Errorf("tier %d: offerDue at round %d = false, want true (cooldown %d has exactly elapsed)", tier, s.Round, cooldown)
		}
	}
}

// tierFloorInfamy returns an Infamy score falling inside
// Config.StepsByTier/CooldownByTier's tier index — 0 (Nobody), 3 (Known), 6
// (Feared), 9 (Legend) — matching TierOf's own boundaries (infamy.go).
func tierFloorInfamy(tierIndex int) int {
	return [4]int{0, 3, 6, 9}[tierIndex]
}

// --- DeadlinePauseUsed ---

// TestCrossRoundDeadlinePauseUsedAppliesOncePerContractLifetime is RFC
// §6.6's DeadlinePauseUsed row — contract-scoped, not round-scoped (its
// own "Lifetime: contract lifetime" per the table), so its "correct round"
// test is that a second qualifying loss on the *same* contract instance is
// a no-op, however many rounds separate the two losses.
func TestCrossRoundDeadlinePauseUsedAppliesOncePerContractLifetime(t *testing.T) {
	c := Contract{ID: 0, ExpiresRound: 10}

	first := c.WithDeadlinePause()
	if !first.DeadlinePauseUsed {
		t.Fatal("first WithDeadlinePause: DeadlinePauseUsed = false, want true")
	}
	if first.ExpiresRound != 11 {
		t.Fatalf("first WithDeadlinePause: ExpiresRound = %d, want 11", first.ExpiresRound)
	}

	second := first.WithDeadlinePause() // a later loss, same contract instance
	if second.ExpiresRound != 11 {
		t.Errorf("second WithDeadlinePause: ExpiresRound = %d, want unchanged 11 (once per contract)", second.ExpiresRound)
	}
}

// --- ConsecutiveDefaults ---

// TestCrossRoundConsecutiveDefaultsNeverWrittenByRules is RFC §6.6's
// ConsecutiveDefaults row, negative by necessity: RFC §8.2 is explicit that
// Autopilot is "a fact about the order log," derived at the match/tick
// layer from `source <> 'human'` — never a value Resolve computes or
// maintains. This field exists on Player (state.go) for RFC §6.6's table
// to name, but internal/rules itself must never write it — the same shape
// D5 already established for LastOfferRound
// (TestUpkeepNeverMutatesLastOfferRound, upkeep_test.go) and
// LooseCrateHeldRounds, generalised to the whole package rather than just
// upkeep() since this field has no writer anywhere in it at all.
//
// A structural, fail-closed scan (the same shape
// TestOnlyOrderingFileUsesSortSlice, ordering_test.go, already uses in this
// package) rather than a runtime assertion: there is no live code path to
// drive that would prove a negative, and the actual risk is a *future*
// change adding a write this package was never meant to own.
func TestCrossRoundConsecutiveDefaultsNeverWrittenByRules(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(.): %v", err)
	}

	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++

		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		// state.go's own field declaration and doc comment are the only
		// permitted mentions — any assignment operator following the field
		// name anywhere else is a write this row must never gain silently.
		if name == "state.go" {
			continue
		}
		if strings.Contains(string(data), "ConsecutiveDefaults") {
			t.Errorf("%s mentions ConsecutiveDefaults — RFC §8.2 places Autopilot derivation entirely outside internal/rules; a write here is a field silently migrating back into a package it was never meant to live in", name)
		}
	}

	if scanned == 0 {
		t.Fatal("scanned 0 non-test .go files in internal/rules — the check ran over nothing, which is not the same as passing")
	}
}
