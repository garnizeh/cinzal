package rules

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// fogTestSight sets seat's Fog to fog for every node named — the minimum
// setup Project needs to include a node in Nodes at all (trailTestState
// leaves every seat's Fog at the zero value, FogHidden, which Project
// correctly renders as an absent map key).
func fogTestSight(s *MatchState, seat game.SeatID, fog game.FogState, nodes ...game.NodeID) {
	for _, n := range nodes {
		s.Players[seat].Fog[n] = fog
	}
}

func hasAnchor(anchors []game.Anchor, kind game.EventKind, node game.NodeID) bool {
	for _, a := range anchors {
		if a.Kind == kind && a.Node == node {
			return true
		}
	}
	return false
}

func findAnchor(t *testing.T, anchors []game.Anchor, kind game.EventKind, node game.NodeID) game.Anchor {
	t.Helper()
	for _, a := range anchors {
		if a.Kind == kind && a.Node == node {
			return a
		}
	}
	t.Fatalf("anchors = %+v, want one of kind %v at node %d", anchors, kind, node)
	return game.Anchor{}
}

func hasTrailEntry(trail []game.TrailEntry, kind game.EventKind, node game.NodeID) bool {
	for _, e := range trail {
		if e.Kind == kind && e.Node == node {
			return true
		}
	}
	return false
}

// --- Rows 2-4, 11, 13-16: global, sourced from MatchState.RoundAnchors ---

func TestProjectRow2DeliveryPresentWhenAnchored(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.RoundAnchors = []game.Anchor{{Kind: game.EventDelivered, Round: 5, Node: 3, Actor: seatPtr(0), Tier: 1}}

	v := Project(s, 1)
	a := findAnchor(t, v.Anchors, game.EventDelivered, 3)
	if a.Actor == nil || *a.Actor != 0 || a.Tier != 1 {
		t.Errorf("Delivered anchor = %+v, want Actor 0, Tier 1", a)
	}
}

func TestProjectRow2DeliveryAbsentWithoutAnchor(t *testing.T) {
	s := trailTestState(5, 0, 3)

	v := Project(s, 1)
	if hasAnchor(v.Anchors, game.EventDelivered, 3) {
		t.Errorf("Anchors = %+v, want no Delivered anchor with an empty RoundAnchors", v.Anchors)
	}
}

func TestProjectRow3PostStakedPresentWhenAnchored(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.RoundAnchors = []game.Anchor{{Kind: game.EventPostStaked, Round: 5, Node: 2, Actor: seatPtr(1)}}

	v := Project(s, 0)
	if !hasAnchor(v.Anchors, game.EventPostStaked, 2) {
		t.Errorf("Anchors = %+v, want a PostStaked anchor at node 2", v.Anchors)
	}
}

func TestProjectRow3PostStakedAbsentWithoutAnchor(t *testing.T) {
	s := trailTestState(5, 0, 3)

	v := Project(s, 0)
	if hasAnchor(v.Anchors, game.EventPostStaked, 2) {
		t.Errorf("Anchors = %+v, want no PostStaked anchor", v.Anchors)
	}
}

func TestProjectRow4LeaseExpiredPresentWhenAnchored(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.RoundAnchors = []game.Anchor{{Kind: game.EventLeaseExpired, Round: 5, Node: 2, Actor: seatPtr(1)}}

	v := Project(s, 0)
	a := findAnchor(t, v.Anchors, game.EventLeaseExpired, 2)
	if a.Actor == nil || *a.Actor != 1 {
		t.Errorf("LeaseExpired anchor Actor = %v, want the owner (seat 1)", a.Actor)
	}
}

func TestProjectRow4LeaseExpiredAbsentWithoutAnchor(t *testing.T) {
	s := trailTestState(5, 0, 3)

	v := Project(s, 0)
	if hasAnchor(v.Anchors, game.EventLeaseExpired, 2) {
		t.Errorf("Anchors = %+v, want no LeaseExpired anchor", v.Anchors)
	}
}

func TestProjectRow11InformantsPresentWhenAnchored(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.RoundAnchors = []game.Anchor{{Kind: game.EventInformants, Round: 5, Node: 0, Actor: seatPtr(0)}}

	v := Project(s, 1)
	if !hasAnchor(v.Anchors, game.EventInformants, 0) {
		t.Errorf("Anchors = %+v, want an Informants anchor at node 0", v.Anchors)
	}
}

func TestProjectRow11InformantsAbsentWithoutAnchor(t *testing.T) {
	s := trailTestState(5, 0, 3)

	v := Project(s, 1)
	if hasAnchor(v.Anchors, game.EventInformants, 0) {
		t.Errorf("Anchors = %+v, want no Informants anchor", v.Anchors)
	}
}

func TestProjectRow13DeadRunnerCratePresentWhenAnchored(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.RoundAnchors = []game.Anchor{{Kind: game.EventDeadRunnerCrate, Round: 5, Node: 4}}

	v := Project(s, 0)
	a := findAnchor(t, v.Anchors, game.EventDeadRunnerCrate, 4)
	if a.Actor != nil {
		t.Errorf("DeadRunnerCrate anchor Actor = %v, want nil (node only)", a.Actor)
	}
}

func TestProjectRow13DeadRunnerCrateAbsentWithoutAnchor(t *testing.T) {
	s := trailTestState(5, 0, 3)

	v := Project(s, 0)
	if hasAnchor(v.Anchors, game.EventDeadRunnerCrate, 4) {
		t.Errorf("Anchors = %+v, want no DeadRunnerCrate anchor", v.Anchors)
	}
}

func TestProjectRow14FenceWindfallPresentWhenAnchored(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.RoundAnchors = []game.Anchor{{Kind: game.EventFenceWindfallAnnounced, Round: 5, Node: 0}}

	v := Project(s, 1)
	if !hasAnchor(v.Anchors, game.EventFenceWindfallAnnounced, 0) {
		t.Errorf("Anchors = %+v, want a FenceWindfallAnnounced anchor at node 0", v.Anchors)
	}
}

func TestProjectRow14FenceWindfallAbsentWithoutAnchor(t *testing.T) {
	s := trailTestState(5, 0, 3)

	v := Project(s, 1)
	if hasAnchor(v.Anchors, game.EventFenceWindfallAnnounced, 0) {
		t.Errorf("Anchors = %+v, want no FenceWindfallAnnounced anchor", v.Anchors)
	}
}

// --- Headline.ActiveBorders (GDD §6.3): the standing "new PlayerView
// field needs a negative fog test" obligation (CONTRIBUTING.md) ---

func bordersFogTestState(players int) MatchState {
	s := bordersFixture(players)
	s.Round = 1
	for i := range s.Players {
		s.Players[i].Fog = make([]game.FogState, len(s.Graph.Nodes))
	}
	return s
}

// TestProjectHeadlineActiveBordersAtTwoPlayers checks the positive case:
// at 2 players, every seat's Headline carries the upcoming round's active
// Border set, matching activeBordersForRound directly.
func TestProjectHeadlineActiveBordersAtTwoPlayers(t *testing.T) {
	s := bordersFogTestState(2)

	want := activeBordersForRound(s.Graph, s.Round+1, 2)
	if len(want) == 0 {
		t.Fatalf("test fixture produced an empty active set — strengthen it")
	}

	for _, seat := range []game.SeatID{0, 1} {
		v := Project(s, seat)
		if !slices.Equal(v.Headline.ActiveBorders, want) {
			t.Errorf("seat %d: Headline.ActiveBorders = %v, want %v", seat, v.Headline.ActiveBorders, want)
		}
	}
}

// TestProjectHeadlineActiveBordersNilAboveTwoPlayers is the negative half
// this obligation exists for: rotation must never leak into a 3+ player
// table (GDD §6.3, issue #76's own acceptance criterion).
func TestProjectHeadlineActiveBordersNilAboveTwoPlayers(t *testing.T) {
	for players := 3; players <= 5; players++ {
		s := bordersFogTestState(players)

		v := Project(s, 0)
		if v.Headline.ActiveBorders != nil {
			t.Errorf("players=%d: Headline.ActiveBorders = %v, want nil", players, v.Headline.ActiveBorders)
		}
	}
}

// --- Headline.Sector / Headline.Category under D11's Suppress.Incidents
// and Suppress.Events (issue #158) ---

// TestProjectHeadlineSectorAndCategoryPopulatedWithoutSuppression is the
// positive baseline the two suppressed tests below are a negation of: it
// confirms initial() and Project actually produce a live Headline in the
// ordinary, unsuppressed case, so "always nil" under suppression is a real
// behaviour change and not merely projectHeadline never firing at all.
func TestProjectHeadlineSectorAndCategoryPopulatedWithoutSuppression(t *testing.T) {
	cfg := game.DefaultConfig()
	s, err := initial(testSeed(72), cfg, 4)
	if err != nil {
		t.Fatalf("initial() = %v", err)
	}

	s.Round = 2 // Headline announces round 3's Unstable Sector.
	if v := Project(s, 0); v.Headline.Sector == nil {
		t.Fatal("Headline.Sector = nil, want non-nil (round 3's draw, announced at round 2)")
	}

	s.Round = 3 // Headline announces round 4's event category.
	if v := Project(s, 0); v.Headline.Category == nil {
		t.Fatal("Headline.Category = nil, want non-nil (round 4's card, announced at round 3)")
	}
}

// TestProjectHeadlineSilentUnderSuppressIncidents is D11's consequence for
// Suppress.Incidents, checked end-to-end through Project: with initial()'s
// round-3 draw skipped, s.UnstableSector stays nil for the whole match, so
// projectHeadline's existing "s.UnstableSector != nil" early-out —
// unmodified by this PR — correctly keeps the Headline's Unstable Sector
// line silent in every round, not merely before round 3.
func TestProjectHeadlineSilentUnderSuppressIncidents(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Suppress.Incidents = true
	s, err := initial(testSeed(70), cfg, 4)
	if err != nil {
		t.Fatalf("initial() = %v", err)
	}

	for round := 0; round <= cfg.Rounds; round++ {
		s.Round = game.RoundNumber(round)
		v := Project(s, 0)
		if v.Headline.Sector != nil {
			t.Errorf("round %d: Headline.Sector = %v, want nil under Suppress.Incidents", round, *v.Headline.Sector)
		}
	}
}

// TestProjectHeadlineSilentUnderSuppressEvents is the same shape for
// Suppress.Events: with buildEventDeck skipped, s.Graph.EventDeck stays
// nil, so projectHeadline's existing eventCardThisRound peek — unmodified
// by this PR — reports live=false in every round, keeping the Headline's
// event-category line silent throughout rounds 4-15 too.
func TestProjectHeadlineSilentUnderSuppressEvents(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Suppress.Events = true
	s, err := initial(testSeed(71), cfg, 4)
	if err != nil {
		t.Fatalf("initial() = %v", err)
	}

	for round := 0; round <= cfg.Rounds; round++ {
		s.Round = game.RoundNumber(round)
		v := Project(s, 0)
		if v.Headline.Category != nil {
			t.Errorf("round %d: Headline.Category = %v, want nil under Suppress.Events", round, *v.Headline.Category)
		}
	}
}

func TestProjectRoundAnchorsIdenticalAcrossEverySeat(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.RoundAnchors = []game.Anchor{{Kind: game.EventDelivered, Round: 5, Node: 3, Actor: seatPtr(0), Tier: 1}}

	v0 := Project(s, 0)
	v1 := Project(s, 1)
	if len(v0.Anchors) != 1 || len(v1.Anchors) != 1 {
		t.Fatalf("Anchors = %+v / %+v, want exactly one global anchor in each view", v0.Anchors, v1.Anchors)
	}
	a0, a1 := v0.Anchors[0], v1.Anchors[0]
	if a0.Kind != a1.Kind || a0.Round != a1.Round || a0.Node != a1.Node || a0.Tier != a1.Tier ||
		(a0.Actor == nil) != (a1.Actor == nil) || (a0.Actor != nil && *a0.Actor != *a1.Actor) {
		t.Errorf("global anchor differs by viewer: %+v vs %+v — rows 2-4, 11, 13-16 reach every seat unconditionally", a0, a1)
	}
	if a0.Actor == a1.Actor {
		t.Errorf("both views share the same *SeatID allocation, want each view's Anchor to own a fresh copy")
	}
}

// --- Rows 5, 6, 9, 10: global, derived from live Player state ---

func TestProjectRow5LoiteringPresentAtThreeRounds(t *testing.T) {
	s := trailTestState(5, 2, 3)
	s.Players[0].LoiteringStreak = 3

	v := Project(s, 1)
	a := findAnchor(t, v.Anchors, game.EventLoitering, 2)
	if a.Actor != nil {
		t.Errorf("Loitering anchor Actor = %v, want nil — row 5 is node-only, never named", a.Actor)
	}
}

func TestProjectRow5LoiteringAbsentBelowThreeRounds(t *testing.T) {
	s := trailTestState(5, 2, 3)
	s.Players[0].LoiteringStreak = 2

	v := Project(s, 1)
	if hasAnchor(v.Anchors, game.EventLoitering, 2) {
		t.Errorf("Anchors = %+v, want no Loitering anchor below 3 consecutive rounds", v.Anchors)
	}
}

func TestProjectRow6LooseCrateHeldPresentAtTwoRounds(t *testing.T) {
	s := trailTestState(5, 2, 3)
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false}
	s.Players[0].LooseCrateHeldRounds = 2

	v := Project(s, 1)
	a := findAnchor(t, v.Anchors, game.EventLooseCrateHeld, 2)
	if a.Actor != nil {
		t.Errorf("LooseCrateHeld anchor Actor = %v, want nil — row 6 is node-only, never named", a.Actor)
	}
}

func TestProjectRow6LooseCrateHeldAbsentBelowTwoRounds(t *testing.T) {
	s := trailTestState(5, 2, 3)
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false}
	s.Players[0].LooseCrateHeldRounds = 1

	v := Project(s, 1)
	if hasAnchor(v.Anchors, game.EventLooseCrateHeld, 2) {
		t.Errorf("Anchors = %+v, want no LooseCrateHeld anchor below 2 consecutive rounds", v.Anchors)
	}
}

func TestProjectRow6LooseCrateHeldAbsentWhenBound(t *testing.T) {
	s := trailTestState(5, 2, 3)
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true}
	s.Players[0].LooseCrateHeldRounds = 2

	v := Project(s, 1)
	if hasAnchor(v.Anchors, game.EventLooseCrateHeld, 2) {
		t.Errorf("Anchors = %+v, want no LooseCrateHeld anchor for bound cargo (still tied to its contract, GDD §8.4)", v.Anchors)
	}
}

func TestProjectRow9FearedRevealAtInfamySix(t *testing.T) {
	s := trailTestState(5, 2, 3)
	s.Players[0].Infamy = 6

	v := Project(s, 1)
	a := findAnchor(t, v.Anchors, game.EventTierFeared, 2)
	if a.Actor == nil || *a.Actor != 0 {
		t.Errorf("TierFeared anchor Actor = %v, want seat 0", a.Actor)
	}
}

func TestProjectRow9FearedAbsentAtInfamyFive(t *testing.T) {
	s := trailTestState(5, 2, 3)
	s.Players[0].Infamy = 5

	v := Project(s, 1)
	if hasAnchor(v.Anchors, game.EventTierFeared, 2) {
		t.Errorf("Anchors = %+v, want no TierFeared anchor below Infamy 6 (Known, GDD §11)", v.Anchors)
	}
}

func TestProjectRow10LegendBroadcastAtInfamyNine(t *testing.T) {
	s := trailTestState(5, 2, 3)
	s.Players[0].Infamy = 9

	v := Project(s, 1)
	a := findAnchor(t, v.Anchors, game.EventTierLegend, 2)
	if a.Actor == nil || *a.Actor != 0 {
		t.Errorf("TierLegend anchor Actor = %v, want seat 0", a.Actor)
	}
	// Legend also still satisfies row 9's own Feared-and-up threshold.
	if !hasAnchor(v.Anchors, game.EventTierFeared, 2) {
		t.Errorf("Anchors = %+v, want the Legend seat's TierFeared anchor too (Feared-and-up, GDD §11)", v.Anchors)
	}
}

func TestProjectRow10LegendAbsentAtInfamyEight(t *testing.T) {
	s := trailTestState(5, 2, 3)
	s.Players[0].Infamy = 8

	v := Project(s, 1)
	if hasAnchor(v.Anchors, game.EventTierLegend, 2) {
		t.Errorf("Anchors = %+v, want no TierLegend anchor below Infamy 9 (Feared, GDD §11)", v.Anchors)
	}
}

// --- Rows 1, 7, 8, 12: sight-gated, sourced from the seat's own Archive.Trail ---

func stampedTrail(round game.RoundNumber, kind game.EventKind, node game.NodeID, actor *game.SeatID) game.StampedTrailEntry {
	return game.StampedTrailEntry{TrailEntry: game.TrailEntry{Kind: kind, Node: node, Actor: actor}, Round: round}
}

func TestProjectRow1CargoTakenPresentThisRound(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.Players[0].Archive.Trail = []game.StampedTrailEntry{stampedTrail(5, game.EventCargoTaken, 0, seatPtr(1))}

	v := Project(s, 0)
	if !hasTrailEntry(v.Trail, game.EventCargoTaken, 0) {
		t.Errorf("Trail = %+v, want a CargoTaken entry at node 0", v.Trail)
	}
}

func TestProjectRow1CargoTakenAbsentFromPriorRound(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.Players[0].Archive.Trail = []game.StampedTrailEntry{stampedTrail(4, game.EventCargoTaken, 0, seatPtr(1))}

	v := Project(s, 0)
	if hasTrailEntry(v.Trail, game.EventCargoTaken, 0) {
		t.Errorf("Trail = %+v, want no entry stamped from a prior round — Trail is this round only", v.Trail)
	}
	if len(v.Archive.Trail) != 1 {
		t.Errorf("Archive.Trail = %+v, want the prior-round entry to remain in the archive", v.Archive.Trail)
	}
}

func TestProjectRow7ConfrontationNamesBothParties(t *testing.T) {
	s := trailTestState(5, 0, 3)
	target := game.SeatID(1)
	entry := game.StampedTrailEntry{
		TrailEntry: game.TrailEntry{Kind: game.EventConfrontation, Node: 0, Actor: seatPtr(0), Target: &target},
		Round:      5,
	}
	s.Players[0].Archive.Trail = []game.StampedTrailEntry{entry}

	v := Project(s, 0)
	if !hasTrailEntry(v.Trail, game.EventConfrontation, 0) {
		t.Fatalf("Trail = %+v, want a Confrontation entry at node 0", v.Trail)
	}
	for _, e := range v.Trail {
		if e.Kind == game.EventConfrontation {
			if e.Actor == nil || e.Target == nil {
				t.Errorf("Confrontation entry = %+v, want both Actor and Target named", e)
			}
		}
	}
}

func TestProjectRow7ConfrontationAbsentFromPriorRound(t *testing.T) {
	s := trailTestState(5, 0, 3)
	target := game.SeatID(1)
	s.Players[0].Archive.Trail = []game.StampedTrailEntry{{
		TrailEntry: game.TrailEntry{Kind: game.EventConfrontation, Node: 0, Actor: seatPtr(0), Target: &target},
		Round:      4,
	}}

	v := Project(s, 0)
	if hasTrailEntry(v.Trail, game.EventConfrontation, 0) {
		t.Errorf("Trail = %+v, want no entry stamped from a prior round", v.Trail)
	}
}

func TestProjectRow8ItemPurchasedPresentThisRound(t *testing.T) {
	s := trailTestState(5, 0, 3)
	item := game.ItemID(1)
	s.Players[0].Archive.Trail = []game.StampedTrailEntry{{
		TrailEntry: game.TrailEntry{Kind: game.EventItemPurchased, Node: 0, Actor: seatPtr(0), Item: &item},
		Round:      5,
	}}

	v := Project(s, 0)
	if !hasTrailEntry(v.Trail, game.EventItemPurchased, 0) {
		t.Errorf("Trail = %+v, want an ItemPurchased entry at node 0", v.Trail)
	}
}

func TestProjectRow8ItemPurchasedAbsentFromPriorRound(t *testing.T) {
	s := trailTestState(5, 0, 3)
	item := game.ItemID(1)
	s.Players[0].Archive.Trail = []game.StampedTrailEntry{{
		TrailEntry: game.TrailEntry{Kind: game.EventItemPurchased, Node: 0, Actor: seatPtr(0), Item: &item},
		Round:      4,
	}}

	v := Project(s, 0)
	if hasTrailEntry(v.Trail, game.EventItemPurchased, 0) {
		t.Errorf("Trail = %+v, want no entry stamped from a prior round", v.Trail)
	}
}

// TestProjectRow12DecoyIndistinguishableFromRealPickup is D12's own
// acceptance criterion, exercised through the real production path rather
// than two hand-built archives that merely assert the tautology "identical
// input produces identical output": writeTrail (trail.go) builds a real
// EventCargoTaken trail entry from roundEvents, and a Decoy entry from a
// declared ItemDecoy discard, through the shared cargoTakenEntry helper
// (addCargoTakenAndDecoy, trail.go) — this drives both of writeTrail's own
// inputs independently, for the same seat/node/Infamy, and asserts the two
// resulting MatchStates project to byte-identical PlayerViews. Neither
// MatchState nor game.TrailEntry stores a fabricated/real marker; this is
// what makes that true from the production code, not from the test's own
// setup.
func TestProjectRow12DecoyIndistinguishableFromRealPickup(t *testing.T) {
	real := trailTestState(5, 2)
	real.Players[0].Infamy = 4
	realOrders := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionNothing}}}
	realEvents := []game.Event{{Kind: game.EventCargoTaken, Round: 5, Node: 2, Seat: 0}}
	writeTrail(&real, realOrders, bySeat(real), trailTestWalks(2), nil, realEvents, globalEventContext{}, incidentContext{}, NewRNG(testSeed(1), int(real.Round)))

	decoy := trailTestState(5, 2)
	decoy.Players[0].Infamy = 4
	decoyOrders := map[game.SeatID]game.Order{0: {
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Items:  []game.ItemDiscard{{Item: game.ItemDecoy, Target: 2}},
	}}
	writeTrail(&decoy, decoyOrders, bySeat(decoy), trailTestWalks(2), nil, nil, globalEventContext{}, incidentContext{}, NewRNG(testSeed(1), int(decoy.Round)))

	if !hasTrailEntry(projectTrail(real.Players[0].Archive, 5), game.EventCargoTaken, 2) {
		t.Fatalf("real setup produced no CargoTaken entry at node 2, want writeTrail to have written one")
	}
	if !hasTrailEntry(projectTrail(decoy.Players[0].Archive, 5), game.EventCargoTaken, 2) {
		t.Fatalf("decoy setup produced no CargoTaken entry at node 2, want writeTrail to have written one")
	}

	realJSON, err := json.Marshal(Project(real, 0))
	if err != nil {
		t.Fatalf("Marshal(real) failed: %v", err)
	}
	decoyJSON, err := json.Marshal(Project(decoy, 0))
	if err != nil {
		t.Fatalf("Marshal(decoy) failed: %v", err)
	}
	if string(realJSON) != string(decoyJSON) {
		t.Errorf("real pickup and Decoy plant serialise differently:\nreal:  %s\ndecoy: %s", realJSON, decoyJSON)
	}
}

// TestProjectRow12DecoyUnnamedBelowInfamyThree is row 1/12's shared Infamy
// gate (RFC §9.1), exercised through the planter's own Archive.Trail: a
// Nobody-tier plant is never named, exactly like a Nobody-tier real pickup
// (writeTrail, trail.go, already enforces the gate before the entry ever
// reaches the archive — this asserts Project does not re-introduce a name
// that was never written).
func TestProjectRow12DecoyUnnamedBelowInfamyThree(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.Players[0].Infamy = 2
	s.Players[0].Archive.Trail = []game.StampedTrailEntry{stampedTrail(5, game.EventCargoTaken, 0, nil)}

	v := Project(s, 0)
	for _, e := range v.Trail {
		if e.Kind == game.EventCargoTaken && e.Actor != nil {
			t.Errorf("CargoTaken entry = %+v, want unnamed below Infamy 3", e)
		}
	}
}

// --- Known-tier pickup: the issue's own named acceptance criterion ---

// TestProjectKnownTierPickupNamesWithoutRevealingPosition is RFC §9.1's
// "most important line": a Known-tier (Infamy 3-5) player taking cargo
// leaves a named trace at a specific Warehouse while their position is
// otherwise entirely hidden from every rival — the primary disclosure
// below Infamy 6, and GDD §7.4's worked example.
func TestProjectKnownTierPickupNamesWithoutRevealingPosition(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.Players[0].Infamy = 4
	fogTestSight(&s, 1, game.FogInSight, 0)
	s.Players[1].Archive.Trail = []game.StampedTrailEntry{stampedTrail(5, game.EventCargoTaken, 0, seatPtr(0))}

	v := Project(s, 1)

	entry := struct{ found bool }{}
	for _, e := range v.Trail {
		if e.Kind == game.EventCargoTaken && e.Node == 0 {
			entry.found = true
			if e.Actor == nil || *e.Actor != 0 {
				t.Errorf("CargoTaken entry Actor = %v, want seat 0 named (Infamy 3-5 is Known)", e.Actor)
			}
		}
	}
	if !entry.found {
		t.Fatalf("Trail = %+v, want the Known-tier pickup entry", v.Trail)
	}

	// OpponentView has no Position field at all — this is the exhaustive
	// check that nothing else in the view names seat 0's position either.
	if hasAnchor(v.Anchors, game.EventTierFeared, 0) || hasAnchor(v.Anchors, game.EventTierLegend, 0) {
		t.Errorf("Anchors = %+v, want no position-reveal anchor for a Known-tier (Infamy 4) seat", v.Anchors)
	}
}

// --- Structural rules: Hidden absent, Rumoured no edges ---

func TestProjectNodesHiddenAbsentRumouredNoEdges(t *testing.T) {
	s := trailTestState(5, 0, 3)
	fogTestSight(&s, 0, game.FogRumoured, 1)
	fogTestSight(&s, 0, game.FogKnown, 2)
	// Node 4 stays FogHidden (the zero value trailTestState seeds).

	v := Project(s, 0)

	if _, ok := v.Nodes[4]; ok {
		t.Errorf("Nodes[4] = %+v, want an absent key for a Hidden node", v.Nodes[4])
	}
	rumoured, ok := v.Nodes[1]
	if !ok {
		t.Fatalf("Nodes[1] missing, want a Rumoured entry")
	}
	if rumoured.Edges != nil {
		t.Errorf("Nodes[1].Edges = %v, want nil — Rumoured carries no topology", rumoured.Edges)
	}
	known, ok := v.Nodes[2]
	if !ok {
		t.Fatalf("Nodes[2] missing, want a Known entry")
	}
	if known.Edges == nil {
		t.Errorf("Nodes[2].Edges = nil, want populated from Known onward")
	}
}

// --- SeatArchive isolation ---

func TestProjectArchiveIsolatedPerSeat(t *testing.T) {
	s := trailTestState(5, 0, 3)
	s.Players[0].Archive.Trail = []game.StampedTrailEntry{stampedTrail(5, game.EventCargoTaken, 0, seatPtr(0))}
	s.Players[0].Archive.Sight = map[game.NodeID]game.RoundSet{0: game.RoundSet(0).With(5)}
	s.Players[1].Archive.Trail = nil
	s.Players[1].Archive.Sight = nil

	v1 := Project(s, 1)
	if len(v1.Archive.Trail) != 0 || len(v1.Archive.Sight) != 0 {
		t.Errorf("seat 1's projected Archive = %+v, want seat 0's archive to be unreachable", v1.Archive)
	}
	if len(v1.NodeStats) != 0 {
		t.Errorf("seat 1's projected NodeStats = %+v, want none derived from seat 0's archive", v1.NodeStats)
	}
}

// --- Byte-level: a hidden node's ID never appears in the serialised view ---

func TestProjectSerialisationOmitsHiddenNodeID(t *testing.T) {
	s := trailTestState(5, 0, 3)
	fogTestSight(&s, 0, game.FogRumoured, 1)
	// Node 4 (deliberately distinctive, unlikely to collide with any other
	// small integer already present elsewhere in the payload) stays Hidden.

	v := Project(s, 0)
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if strings.Contains(string(b), `"4":`) || strings.Contains(string(b), strconv.Itoa(4)+`"`) {
		t.Errorf("serialised view contains node 4's ID, want it absent: %s", b)
	}
	if _, ok := v.Nodes[4]; ok {
		t.Fatalf("test setup error: node 4 should be Hidden")
	}
}

// --- Purity: Project never aliases MatchState memory ---

func TestProjectDoesNotAliasMatchState(t *testing.T) {
	s := trailTestState(5, 0, 3)
	fogTestSight(&s, 0, game.FogKnown, 1)
	s.Players[0].Items = []game.ItemID{1, 2}
	s.Players[0].Archive.Trail = []game.StampedTrailEntry{stampedTrail(5, game.EventCargoTaken, 0, seatPtr(0))}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal(s) failed: %v", err)
	}

	v := Project(s, 0)

	// Mutate everything the view exposes a handle into.
	v.You.Items[0] = 99
	if n, ok := v.Nodes[1]; ok && len(n.Edges) > 0 {
		n.Edges[0] = 99
	}
	for i := range v.Archive.Trail {
		if v.Archive.Trail[i].Actor != nil {
			*v.Archive.Trail[i].Actor = 99
		}
	}

	after, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal(s) failed: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("mutating the projected view changed the source MatchState — Project aliased its memory")
	}
}
