package rules

import (
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// findScavengeSeed searches small integer seeds for one whose scavenge.d6
// roll — r.Next(scavenge.d6, 6) + 1 — falls in [lo, hi], returning the seed
// and the roll it produced. This avoids a hand-computed HMAC digest for
// what these tests need: rng_test.go's TestNextDerivationIsHMAC already
// pins the digest itself, so a fixture test only needs *a* seed landing in
// the range under test, not a specific one.
func findScavengeSeed(t *testing.T, lo, hi int) ([32]byte, int) {
	t.Helper()
	for i := range 1000 {
		seed := seedFromInt(i)
		roll := NewRNG(seed, 1).Next(PurposeScavengeD6, 6) + 1
		if roll >= lo && roll <= hi {
			return seed, roll
		}
	}
	t.Fatalf("no seed found producing a scavenge.d6 roll in [%d, %d] within 1000 tries", lo, hi)
	return [32]byte{}, 0
}

// --- crossingNode / detectCrossings (GDD §15a) ---

// TestCrossingNodeFourAggressiveCombinations is the issue's own acceptance
// criterion, verbatim: "all four Aggressive combinations, including the
// lower-index fallback."
func TestCrossingNodeFourAggressiveCombinations(t *testing.T) {
	const fromA, fromB = game.NodeID(3), game.NodeID(7)

	cases := []struct {
		name             string
		stanceA, stanceB game.Stance
		want             game.NodeID
	}{
		{"A aggressive, B not", game.StanceAggressive, game.StanceNeutral, fromA},
		{"B aggressive, A not", game.StanceNeutral, game.StanceAggressive, fromB},
		{"neither aggressive", game.StanceNeutral, game.StanceEvasive, fromA}, // lower-indexed endpoint
		{"both aggressive", game.StanceAggressive, game.StanceAggressive, fromA},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := crossingNode(fromA, fromB, c.stanceA, c.stanceB); got != c.want {
				t.Errorf("crossingNode(%d, %d, %s, %s) = %d, want %d", fromA, fromB, c.stanceA, c.stanceB, got, c.want)
			}
		})
	}
}

// TestDetectCrossingsFindsOppositeDirectionSameEdge asserts GDD §15a's
// definition directly: two seats whose this-step transitions swap the same
// pair of nodes are a crossing, resolved at the Aggressive seat's own
// origin. A third, unrelated seat's transition must not interfere.
func TestDetectCrossingsFindsOppositeDirectionSameEdge(t *testing.T) {
	transitions := map[game.SeatID]transition{
		0: {Seat: 0, From: 1, To: 2},
		1: {Seat: 1, From: 2, To: 1},
		2: {Seat: 2, From: 5, To: 6},
	}
	seats := []game.SeatID{0, 1, 2}
	validated := map[game.SeatID]game.Order{
		0: {Stance: game.StanceOrder{Stance: game.StanceAggressive}},
		1: {Stance: game.StanceOrder{Stance: game.StanceNeutral}},
		2: {Stance: game.StanceOrder{Stance: game.StanceNeutral}},
	}

	got := detectCrossings(transitions, seats, validated)
	want := []confrontation{{Node: 1, Seats: []game.SeatID{0, 1}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("detectCrossings() = %+v, want %+v", got, want)
	}
}

// TestDetectCrossingsIgnoresSameDirectionAndStationary asserts two seats
// walking the same edge in the *same* direction, and a stationary seat, are
// never mistaken for a crossing.
func TestDetectCrossingsIgnoresSameDirectionAndStationary(t *testing.T) {
	transitions := map[game.SeatID]transition{
		0: {Seat: 0, From: 1, To: 2},
		1: {Seat: 1, From: 1, To: 2},
		2: {Seat: 2, From: 5, To: 5},
	}
	seats := []game.SeatID{0, 1, 2}
	validated := map[game.SeatID]game.Order{0: {}, 1: {}, 2: {}}

	got := detectCrossings(transitions, seats, validated)
	if len(got) != 0 {
		t.Fatalf("detectCrossings() = %v, want empty", got)
	}
}

// --- detectCollisions (GDD §15b) ---

// TestDetectCollisionsGroupsSharedNodeIncludingStationary is the issue's
// own acceptance criterion: "collision detection runs on stationary
// seats." Neither seat 0 nor seat 1 moved this step; they still collide.
func TestDetectCollisionsGroupsSharedNodeIncludingStationary(t *testing.T) {
	s := MatchState{Players: []Player{
		{Seat: 0, Position: 5},
		{Seat: 1, Position: 5},
		{Seat: 2, Position: 9},
	}}
	seats := []game.SeatID{0, 1, 2}

	got := detectCollisions(s, seats)
	want := []confrontation{{Node: 5, Seats: []game.SeatID{0, 1}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("detectCollisions() = %+v, want %+v", got, want)
	}
}

// TestDetectCollisionsEmptyWhenEverySeatAlone asserts a node with exactly
// one occupant produces no group at all, not a group of one.
func TestDetectCollisionsEmptyWhenEverySeatAlone(t *testing.T) {
	s := MatchState{Players: []Player{{Seat: 0, Position: 1}, {Seat: 1, Position: 2}}}
	seats := []game.SeatID{0, 1}

	got := detectCollisions(s, seats)
	if len(got) != 0 {
		t.Fatalf("detectCollisions() = %v, want empty", got)
	}
}

// TestCohabitationThreeCases is GDD §15's own table, verbatim: two seats
// displaced onto the same node resolve a round exactly per whether they
// separate, both hold, or one leaves — with no pre-movement check needed,
// since the collision rule evaluates every step's final position on its
// own (RFC §6.7).
func TestCohabitationThreeCases(t *testing.T) {
	g := buildGraph(3, map[game.NodeID][]game.NodeID{0: {1, 2}, 1: {0}, 2: {0}})
	seats := []game.SeatID{0, 1}
	fog := []game.FogState{game.FogKnown, game.FogKnown, game.FogKnown}

	newState := func() MatchState {
		return MatchState{
			Graph: g,
			Players: []Player{
				{Seat: 0, Position: 0, Fog: append([]game.FogState(nil), fog...)},
				{Seat: 1, Position: 0, Fog: append([]game.FogState(nil), fog...)},
			},
		}
	}

	cases := []struct {
		name      string
		validated map[game.SeatID]game.Order
		wantFight bool
	}{
		{
			name: "both leave in different directions: no fight",
			validated: map[game.SeatID]game.Order{
				0: {Route: []game.NodeID{1}},
				1: {Route: []game.NodeID{2}},
			},
			wantFight: false,
		},
		{
			name: "both stay: fight",
			validated: map[game.SeatID]game.Order{
				0: {},
				1: {},
			},
			wantFight: true,
		},
		{
			name: "one leaves, one stays: no fight",
			validated: map[game.SeatID]game.Order{
				0: {Route: []game.NodeID{1}},
				1: {},
			},
			wantFight: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newState()
			walks := newSeatWalks(s, seats)
			rng := NewRNG(testSeed(1), 1)
			advance(&s, walks, c.validated, seats, 1, incidentContext{}, rng)

			groups := detectCollisions(s, seats)
			fight := len(groups) > 0
			if fight != c.wantFight {
				t.Errorf("collision after step 1 = %v, want %v (groups=%v)", fight, c.wantFight, groups)
			}
		})
	}
}

// --- mergeConfrontations (#58's orderConfrontationsByNode) ---

// TestMergeConfrontationsOrdersByNodeID is the issue's own acceptance
// criterion, verbatim: "two confrontations at different nodes in the same
// step resolve in node ID order."
func TestMergeConfrontationsOrdersByNodeID(t *testing.T) {
	s := MatchState{Players: []Player{{Seat: 0}, {Seat: 1}, {Seat: 2}, {Seat: 3}}}
	crossings := []confrontation{{Node: 9, Seats: []game.SeatID{2, 3}}}
	collisions := []confrontation{{Node: 1, Seats: []game.SeatID{0, 1}}}

	got := mergeConfrontations(s, crossings, collisions)
	want := []confrontation{
		{Node: 1, Seats: []game.SeatID{0, 1}},
		{Node: 9, Seats: []game.SeatID{2, 3}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeConfrontations() = %+v, want %+v", got, want)
	}
}

// TestMergeConfrontationsUnionsCrossingAndCollisionAtSameNode asserts a
// node with both a crossing and a co-located collision becomes one
// confrontation with every participant's union, seat-ordered and
// deduplicated, not two separate resolutions of the same node.
func TestMergeConfrontationsUnionsCrossingAndCollisionAtSameNode(t *testing.T) {
	s := MatchState{Players: []Player{{Seat: 0}, {Seat: 1}, {Seat: 2}}}
	crossings := []confrontation{{Node: 4, Seats: []game.SeatID{0, 1}}}
	collisions := []confrontation{{Node: 4, Seats: []game.SeatID{1, 2}}}

	got := mergeConfrontations(s, crossings, collisions)
	want := []confrontation{{Node: 4, Seats: []game.SeatID{0, 1, 2}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeConfrontations() = %+v, want %+v", got, want)
	}
}

// --- Pushing On's priority ladder (GDD §9.1) ---

// TestPushOnLadderLevel2PrefersCloserOverFarther asserts level 2: node 1 is
// one step closer to the biased sector than node 0 itself, node 2 is one
// step farther — only node 1 may ever be chosen. cameFrom is a sentinel
// node absent from the graph, so the backtrack rule never interferes.
func TestPushOnLadderLevel2PrefersCloserOverFarther(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: 0, Edges: []game.NodeID{1, 2}, Sector: game.SectorOldDocks},
		{ID: 1, Edges: []game.NodeID{0, 3}, Sector: game.SectorOldDocks},
		{ID: 2, Edges: []game.NodeID{0}, Sector: game.SectorOldDocks},
		{ID: 3, Edges: []game.NodeID{1}, Sector: game.SectorMistHeights},
	}}
	bias := game.SectorMistHeights

	for seed := range 30 {
		rng := NewRNG(seedFromInt(seed), 1)
		if got := pushOnStep(g, 0, 99, &bias, rng); got != 1 {
			t.Fatalf("seed %d: pushOnStep() = %d, want 1 (node 2 is farther from the biased sector and must never be chosen when node 1 is closer)", seed, got)
		}
	}
}

// TestPushOnLadderLevel3PrefersEqualDistanceWhenCloserIsExcluded asserts
// level 3: from node 0 (distance 2 from the biased sector), node 1 sits one
// step closer (distance 1) but is excluded as the backtrack node; node 2
// sits at the same distance as node 0 (distance 2) and is the only
// remaining candidate — level 3's own case, never level 2's, since level 2
// has nothing left to offer once node 1 is off the table.
func TestPushOnLadderLevel3PrefersEqualDistanceWhenCloserIsExcluded(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: 0, Edges: []game.NodeID{1, 2}, Sector: game.SectorOldDocks},
		{ID: 1, Edges: []game.NodeID{0, 3}, Sector: game.SectorOldDocks},
		{ID: 2, Edges: []game.NodeID{0, 4}, Sector: game.SectorOldDocks},
		{ID: 3, Edges: []game.NodeID{1, 4}, Sector: game.SectorMistHeights},
		{ID: 4, Edges: []game.NodeID{2, 3}, Sector: game.SectorOldDocks},
	}}
	bias := game.SectorMistHeights

	for seed := range 30 {
		rng := NewRNG(seedFromInt(seed), 1)
		if got := pushOnStep(g, 0, 1, &bias, rng); got != 2 {
			t.Fatalf("seed %d: pushOnStep() = %d, want 2 (node 1 is closer but excluded as the backtrack node, so level 3 must pick node 2)", seed, got)
		}
	}
}

// TestPushOnLadderLevel4WhenBiasIsNone asserts declaring no bias degenerates
// to a plain random walk over every legal neighbour (GDD §9.1) — both of
// node 0's neighbours must be reachable across enough seeds, not just one.
func TestPushOnLadderLevel4WhenBiasIsNone(t *testing.T) {
	g := buildGraph(3, map[game.NodeID][]game.NodeID{0: {1, 2}, 1: {0}, 2: {0}})

	seen := map[game.NodeID]bool{}
	for seed := range 40 {
		rng := NewRNG(seedFromInt(seed), 1)
		got := pushOnStep(g, 0, 99, nil, rng)
		if got != 1 && got != 2 {
			t.Fatalf("seed %d: pushOnStep() = %d, want 1 or 2", seed, got)
		}
		seen[got] = true
	}
	if len(seen) != 2 {
		t.Fatalf("pushOnStep() with no bias only ever chose %v across 40 seeds, want both neighbours reachable", seen)
	}
}

// TestPushOnLadderLevel4WhenSectorUnreachable asserts an unreachable
// declared sector falls through to level 4 exactly like no bias at all (GDD
// §9.1: "every neighbour has infinite distance, the ladder falls through to
// level 4").
func TestPushOnLadderLevel4WhenSectorUnreachable(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: 0, Edges: []game.NodeID{1, 2}, Sector: game.SectorOldDocks},
		{ID: 1, Edges: []game.NodeID{0}, Sector: game.SectorOldDocks},
		{ID: 2, Edges: []game.NodeID{0}, Sector: game.SectorOldDocks},
	}}
	bias := game.SectorNorthVale // no node anywhere belongs to this sector

	seen := map[game.NodeID]bool{}
	for seed := range 40 {
		rng := NewRNG(seedFromInt(seed), 1)
		seen[pushOnStep(g, 0, 99, &bias, rng)] = true
	}
	if len(seen) != 2 {
		t.Fatalf("pushOnStep() with an unreachable sector only ever chose %v across 40 seeds, want both neighbours reachable", seen)
	}
}

// TestPushOnStepStopsEarlyWithNoLegalEdge is GDD §9.1's other named
// terminating case: no edge leads anywhere legal, so the walk stops without
// moving and, per RFC §6.4's lazy-draw rule, consumes no pushon.edge index
// at all.
func TestPushOnStepStopsEarlyWithNoLegalEdge(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: 0, Edges: []game.NodeID{1}},
		{ID: 1, SinkholeRounds: 3},
	}}
	rng := NewRNG(testSeed(1), 1)

	if got := pushOnStep(g, 0, 0, nil, rng); got != 0 {
		t.Fatalf("pushOnStep() = %d, want 0 (unmoved — node 1, the only edge, is Sinkholed)", got)
	}
	if rng.Seq() != 0 {
		t.Fatalf("Seq() = %d, want 0 — no legal edge consumes no draw", rng.Seq())
	}
}

// TestPushOnCandidatesAllowsBacktrackWhenSoleOption is GDD §9.1 item 5's own
// exception: the node just come from is excluded "unless it is the only
// option."
func TestPushOnCandidatesAllowsBacktrackWhenSoleOption(t *testing.T) {
	g := buildGraph(2, map[game.NodeID][]game.NodeID{0: {1}, 1: {0}})

	got := pushOnCandidates(g, 0, 1)
	want := []game.NodeID{1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pushOnCandidates() = %v, want %v (the backtrack node is the only option)", got, want)
	}
}

// TestPushOnCandidatesExcludesCameFromWhenAlternativeExists asserts the
// ordinary case: the backtrack node is dropped whenever a real alternative
// exists.
func TestPushOnCandidatesExcludesCameFromWhenAlternativeExists(t *testing.T) {
	g := buildGraph(3, map[game.NodeID][]game.NodeID{0: {1, 2}, 1: {0}, 2: {0}})

	got := pushOnCandidates(g, 0, 1)
	want := []game.NodeID{2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pushOnCandidates() = %v, want %v", got, want)
	}
}

// TestPushOnCandidatesExcludesSinkholedNodes asserts GDD §9.1a item 0: a
// Sinkholed destination is impassable, excluded from candidates regardless
// of the backtrack rule.
func TestPushOnCandidatesExcludesSinkholedNodes(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: 0, Edges: []game.NodeID{1, 2}},
		{ID: 1, SinkholeRounds: 2},
		{ID: 2},
	}}

	got := pushOnCandidates(g, 0, 99)
	want := []game.NodeID{2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pushOnCandidates() = %v, want %v (node 1 is Sinkholed and impassable)", got, want)
	}
}

// --- Scavenging (GDD §9.1) ---

// TestScavengeSkipsAlreadyKnownNode is GDD §9.1's own line: "a node that is
// already Known when you arrive rolls no Scavenging."
func TestScavengeSkipsAlreadyKnownNode(t *testing.T) {
	s := MatchState{
		Graph:   buildGraph(2, map[game.NodeID][]game.NodeID{0: {1}, 1: {0}}),
		Players: []Player{{Seat: 0, Fog: []game.FogState{game.FogKnown, game.FogKnown}}},
	}
	rng := NewRNG(testSeed(1), 1)

	scavenge(&s, 0, 1, rng)
	if rng.Seq() != 0 {
		t.Fatalf("Seq() = %d, want 0 — an already-Known node rolls no Scavenging", rng.Seq())
	}
}

// TestScavengeMarksEnteredNodeKnownRegardlessOfRoll asserts GDD §7.2's
// reveal ("a visited node becomes Known permanently") happens on every
// roll, not only a favourable one.
func TestScavengeMarksEnteredNodeKnownRegardlessOfRoll(t *testing.T) {
	for seed := range 20 {
		s := MatchState{
			Graph:   buildGraph(2, map[game.NodeID][]game.NodeID{0: {1}, 1: {0}}),
			Players: []Player{{Seat: 0, Fog: []game.FogState{game.FogKnown, game.FogHidden}}},
		}
		rng := NewRNG(seedFromInt(seed), 1)

		scavenge(&s, 0, 1, rng)
		if s.Players[0].Fog[1] != game.FogKnown {
			t.Fatalf("seed %d: Fog[1] = %s, want Known after visiting, regardless of the roll", seed, s.Players[0].Fog[1])
		}
	}
}

// TestScavengeSkipsRumouredNodeButStillMarksKnown asserts GDD §9.1's roll
// is gated on genuinely Hidden, not "anything short of Known": a Rumoured
// node (name, type, sector already known, GDD §7.1) rolls no Scavenging
// and consumes no scavenge.d6 index — but entering it still upgrades it to
// Known, exactly like any other visited node (GDD §7.2).
func TestScavengeSkipsRumouredNodeButStillMarksKnown(t *testing.T) {
	s := MatchState{
		Graph:   buildGraph(2, map[game.NodeID][]game.NodeID{0: {1}, 1: {0}}),
		Players: []Player{{Seat: 0, Balance: 10, Fog: []game.FogState{game.FogKnown, game.FogRumoured}}},
	}
	rng := NewRNG(testSeed(1), 1)

	scavenge(&s, 0, 1, rng)

	if rng.Seq() != 0 {
		t.Errorf("Seq() = %d, want 0 — a Rumoured node is not Hidden, so it rolls no Scavenging", rng.Seq())
	}
	if s.Players[0].Balance != 10 {
		t.Errorf("Balance = %d, want unchanged 10", s.Players[0].Balance)
	}
	if s.Players[0].Fog[1] != game.FogKnown {
		t.Errorf("Fog[1] = %s, want Known after visiting, even though no roll fired", s.Players[0].Fog[1])
	}
}

// TestScavengeRollOneToThreeGrantsNothing checks the GDD §9.1 table's first
// row: nothing but rust.
func TestScavengeRollOneToThreeGrantsNothing(t *testing.T) {
	seed, roll := findScavengeSeed(t, 1, 3)
	s := MatchState{
		Graph:   buildGraph(2, map[game.NodeID][]game.NodeID{0: {1}, 1: {0}}),
		Players: []Player{{Seat: 0, Balance: 10, Fog: []game.FogState{game.FogKnown, game.FogHidden}}},
	}
	rng := NewRNG(seed, 1)

	scavenge(&s, 0, 1, rng)
	if s.Players[0].Balance != 10 {
		t.Fatalf("roll %d: Balance = %d, want unchanged 10", roll, s.Players[0].Balance)
	}
}

// TestScavengeGrantsCr3OnFourOrFive checks the GDD §9.1 table's middle row.
func TestScavengeGrantsCr3OnFourOrFive(t *testing.T) {
	seed, roll := findScavengeSeed(t, 4, 5)
	s := MatchState{
		Graph:   buildGraph(2, map[game.NodeID][]game.NodeID{0: {1}, 1: {0}}),
		Players: []Player{{Seat: 0, Balance: 10, Fog: []game.FogState{game.FogKnown, game.FogHidden}}},
	}
	rng := NewRNG(seed, 1)

	scavenge(&s, 0, 1, rng)
	if s.Players[0].Balance != 13 {
		t.Fatalf("roll %d: Balance = %d, want 13 (10 + Cr$3)", roll, s.Players[0].Balance)
	}
}

// TestScavengeRevealsAdjacentNodesOnSix checks the GDD §9.1 table's last
// row: a Rumoured neighbour is upgraded exactly like a Hidden one — the
// rule reveals every adjacent node "as Known," not merely nodes strictly
// below Known.
func TestScavengeRevealsAdjacentNodesOnSix(t *testing.T) {
	seed, _ := findScavengeSeed(t, 6, 6)
	s := MatchState{
		Graph: buildGraph(4, map[game.NodeID][]game.NodeID{0: {1}, 1: {0, 2, 3}, 2: {1}, 3: {1}}),
		Players: []Player{{Seat: 0, Fog: []game.FogState{
			game.FogKnown, game.FogHidden, game.FogHidden, game.FogRumoured,
		}}},
	}
	rng := NewRNG(seed, 1)

	scavenge(&s, 0, 1, rng)
	want := []game.FogState{game.FogKnown, game.FogKnown, game.FogKnown, game.FogKnown}
	if !reflect.DeepEqual(s.Players[0].Fog, want) {
		t.Fatalf("Fog = %v, want %v (a 6 reveals every adjacent node as Known)", s.Players[0].Fog, want)
	}
}

// --- advance: orchestration, ordinary routes, and lazy RNG interleaving ---

// TestAdvanceOrdinaryRouteStepDrawsNothing asserts a fully-Known, player-
// declared route never touches the RNG at all — only Pushing On's blind
// steps draw pushon.edge, and only a newly-explored node draws scavenge.d6.
func TestAdvanceOrdinaryRouteStepDrawsNothing(t *testing.T) {
	s := MatchState{
		Graph:   buildGraph(3, map[game.NodeID][]game.NodeID{0: {1}, 1: {0, 2}, 2: {1}}),
		Players: []Player{{Seat: 0, Position: 0, Fog: []game.FogState{game.FogKnown, game.FogKnown, game.FogKnown}}},
	}
	seats := []game.SeatID{0}
	validated := map[game.SeatID]game.Order{0: {Route: []game.NodeID{1, 2}}}
	walks := newSeatWalks(s, seats)
	rng := NewRNG(testSeed(1), 1)

	advance(&s, walks, validated, seats, 1, incidentContext{}, rng)
	advance(&s, walks, validated, seats, 2, incidentContext{}, rng)

	if s.Players[0].Position != 2 {
		t.Fatalf("Position = %d, want 2", s.Players[0].Position)
	}
	if rng.Seq() != 0 {
		t.Fatalf("Seq() = %d, want 0 — an ordinary, fully-Known route draws nothing", rng.Seq())
	}
}

// TestAdvanceStationarySeatNeverMoves asserts a seat with no route and no
// Pushing On simply stays put, step after step, with From == To.
func TestAdvanceStationarySeatNeverMoves(t *testing.T) {
	s := MatchState{
		Graph:   buildGraph(2, map[game.NodeID][]game.NodeID{0: {1}, 1: {0}}),
		Players: []Player{{Seat: 0, Position: 0, Fog: []game.FogState{game.FogKnown, game.FogKnown}}},
	}
	seats := []game.SeatID{0}
	validated := map[game.SeatID]game.Order{0: {}}
	walks := newSeatWalks(s, seats)
	rng := NewRNG(testSeed(1), 1)

	tr := advance(&s, walks, validated, seats, 1, incidentContext{}, rng)
	if s.Players[0].Position != 0 {
		t.Fatalf("Position = %d, want 0 (stationary)", s.Players[0].Position)
	}
	if tr[0].From != tr[0].To {
		t.Fatalf("transition = %+v, want From == To", tr[0])
	}
}

// TestAdvanceInterleavesPushOnAndScavengeLazily is RFC §6.4's own worked
// case and the issue's own acceptance criterion: "a route with 2 blind
// steps onto two previously-Hidden nodes consumes 4 indices — pushon.edge,
// scavenge.d6, pushon.edge, scavenge.d6, interleaved in execution order,
// not batched by kind."
//
// The reference sequence calls pushOnStep and scavenge directly — the exact
// primitives advance is built from — in that mandated order, against a
// fresh RNG. advance is then run through the identical two steps against a
// second, identically-seeded RNG. If advance batched its draws by kind
// instead of interleaving them (both pushon.edge calls before either
// scavenge.d6 call), the second scavenge.d6 draw would land at a different
// seq and, per HMAC(seed, round||seq||purpose), (almost certainly) produce
// a different roll — so equality here is a genuine order check, not just a
// count check.
func TestAdvanceInterleavesPushOnAndScavengeLazily(t *testing.T) {
	g := buildGraph(5, map[game.NodeID][]game.NodeID{
		0: {1, 2},
		1: {0, 3},
		2: {0, 4},
		3: {1},
		4: {2},
	})
	fog := []game.FogState{game.FogKnown, game.FogHidden, game.FogHidden, game.FogHidden, game.FogHidden}
	seed, round := testSeed(11), 7

	// Reference: pushOnStep then scavenge, twice, interleaved.
	ref := NewRNG(seed, round)
	refState := MatchState{Graph: g, Players: []Player{{Position: 0, Fog: append([]game.FogState(nil), fog...)}}}
	step1 := pushOnStep(g, 0, 0, nil, ref)
	scavenge(&refState, 0, step1, ref)
	step2 := pushOnStep(g, step1, 0, nil, ref)
	scavenge(&refState, 0, step2, ref)

	// Production: advance, two steps of a pure Pushing On order.
	s := MatchState{Graph: g, Players: []Player{{Position: 0, Fog: append([]game.FogState(nil), fog...)}}}
	seats := []game.SeatID{0}
	validated := map[game.SeatID]game.Order{0: {PushingOn: game.PushingOn{Steps: 2}}}
	walks := newSeatWalks(s, seats)
	rng := NewRNG(seed, round)
	advance(&s, walks, validated, seats, 1, incidentContext{}, rng)
	advance(&s, walks, validated, seats, 2, incidentContext{}, rng)

	if s.Players[0].Position != step2 {
		t.Errorf("advance() final position = %d, want %d (pushOnStep's own second-step result)", s.Players[0].Position, step2)
	}
	if s.Players[0].Balance != refState.Players[0].Balance {
		t.Errorf("advance() balance = %d, want %d", s.Players[0].Balance, refState.Players[0].Balance)
	}
	if !reflect.DeepEqual(s.Players[0].Fog, refState.Players[0].Fog) {
		t.Errorf("advance() fog = %v, want %v", s.Players[0].Fog, refState.Players[0].Fog)
	}

	if got := rng.Consumed(PurposePushonEdge); got != 2 {
		t.Errorf("Consumed(pushon.edge) = %d, want 2", got)
	}
	if got := rng.Consumed(PurposeScavengeD6); got != 2 {
		t.Errorf("Consumed(scavenge.d6) = %d, want 2", got)
	}
	if rng.Seq() != 4 {
		t.Errorf("Seq() = %d, want 4", rng.Seq())
	}
}

// TestAdvanceSkipsScavengeWhenBlindStepLandsOnKnownNode is the issue's
// third named lazy-draw truncation point: "a blind step landing on an
// already-Known node" consumes that step's own pushon.edge but zero
// scavenge.d6.
func TestAdvanceSkipsScavengeWhenBlindStepLandsOnKnownNode(t *testing.T) {
	g := buildGraph(2, map[game.NodeID][]game.NodeID{0: {1}, 1: {0}})
	s := MatchState{
		Graph:   g,
		Players: []Player{{Position: 0, Fog: []game.FogState{game.FogKnown, game.FogKnown}}},
	}
	seats := []game.SeatID{0}
	validated := map[game.SeatID]game.Order{0: {PushingOn: game.PushingOn{Steps: 1}}}
	walks := newSeatWalks(s, seats)
	rng := NewRNG(testSeed(1), 1)

	advance(&s, walks, validated, seats, 1, incidentContext{}, rng)

	if got := rng.Consumed(PurposePushonEdge); got != 1 {
		t.Errorf("Consumed(pushon.edge) = %d, want 1", got)
	}
	if got := rng.Consumed(PurposeScavengeD6); got != 0 {
		t.Errorf("Consumed(scavenge.d6) = %d, want 0 — the blind step landed on an already-Known node", got)
	}
}
