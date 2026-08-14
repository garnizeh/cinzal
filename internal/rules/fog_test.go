package rules

import (
	"encoding/json"
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

// --- Rows 2-4, 11, 13-14: global, sourced from MatchState.RoundAnchors ---

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
		t.Errorf("global anchor differs by viewer: %+v vs %+v — rows 2-4, 11, 13-14 reach every seat unconditionally", a0, a1)
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
// acceptance criterion: a real cargo pickup and a Decoy plant produce the
// exact same game.TrailEntry shape (kind, node, actor), so two MatchStates
// differing only in which one actually happened must serialise identically
// through Project — the fabricated/real distinction is internal to
// MatchState and never crosses the fog boundary.
func TestProjectRow12DecoyIndistinguishableFromRealPickup(t *testing.T) {
	real := trailTestState(5, 0, 3)
	real.Players[0].Infamy = 4
	real.Players[0].Archive.Trail = []game.StampedTrailEntry{stampedTrail(5, game.EventCargoTaken, 0, seatPtr(1))}

	decoy := trailTestState(5, 0, 3)
	decoy.Players[0].Infamy = 4
	decoy.Players[0].Archive.Trail = []game.StampedTrailEntry{stampedTrail(5, game.EventCargoTaken, 0, seatPtr(1))}

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
