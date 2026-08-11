package gen

import (
	"slices"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// testOpeningBand is Tier I's contract band as GDD §8.3 states it, spelled
// out here rather than read from game.DefaultConfig(): these are unit tests
// of the predicate, and a band that moved out from under them silently would
// make the fixtures below mean something other than what their comments say.
// The tests that must track the real config — the property sweep — read it
// from cfg.Contracts[0] instead (see propertyCases).
const (
	testOpeningMin = 3
	testOpeningMax = 4
)

// warehouseCycle builds the 8-cycle these tests share: one Warehouse at node
// 0 and, unless border is false, one Border at node 4 — distance 4 from the
// Warehouse, inside Tier I's band, which is what makes node 0 contractable
// under GDD §6.1 constraint 7 as D24 strengthened it.
//
// Distances from node 0 around the cycle are 0,1,2,3,4,3,2,1, so exactly
// nodes {0,1,2,6,7} have the Warehouse within maxStartWarehouseDistance —
// the fixture is not symmetric, and the only pair among them at distance >=
// minStartDistance is {2,6}.
func warehouseCycle(border bool) (*builder, []game.NodeType) {
	sectors := make([]game.Sector, 8)
	for i := range sectors {
		sectors[i] = game.SectorOldDocks
	}
	b := newBuilder(8, sectors)
	for i := range 8 {
		b.connect(game.NodeID(i), game.NodeID((i+1)%8))
	}

	types := make([]game.NodeType, 8)
	for i := range types {
		types[i] = game.NodeAlley
	}
	types[0] = game.NodeWarehouse
	if border {
		types[4] = game.NodeBorder
	}
	return b, types
}

// TestQualifyingStartsNeedsContractableWarehouse checks the hoisted
// per-attempt pass on a path of 8 nodes, where distance from node 0 is a
// node's own index: the Warehouse at node 0 only makes its neighbourhood
// {0, 1, 2} eligible when some Border sits inside the band. A Border below
// the band (the distance-2 pair D24's counterexample found) and one above it
// must both leave every node unqualified — the pre-D24 predicate qualified
// all three in every one of these cases.
func TestQualifyingStartsNeedsContractableWarehouse(t *testing.T) {
	sectors := make([]game.Sector, 8)
	for i := range sectors {
		sectors[i] = game.SectorOldDocks
	}
	b := newBuilder(8, sectors)
	for i := range 7 {
		b.connect(game.NodeID(i), game.NodeID(i+1))
	}

	cases := []struct {
		name     string
		borderAt int
		want     []game.NodeID // nodes expected to qualify; nil for none
		why      string
	}{
		{"border below the band", 2, nil, "distance 2 is under every tier's minimum (GDD §8.3)"},
		{"border at the band's floor", 3, []game.NodeID{0, 1, 2}, ""},
		{"border at the band's ceiling", 4, []game.NodeID{0, 1, 2}, ""},
		{"border above the band", 5, nil, "distance 5 is outside Tier I, the only tier a seat is eligible for at Infamy 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			types := make([]game.NodeType, 8)
			for i := range types {
				types[i] = game.NodeAlley
			}
			types[0] = game.NodeWarehouse
			types[tc.borderAt] = game.NodeBorder

			got := qualifyingStarts(b, types, testOpeningMin, testOpeningMax)
			for node := range got {
				want := slices.Contains(tc.want, game.NodeID(node))
				if got[node] != want {
					t.Errorf("qualifyingStarts()[%d] = %v, want %v (Warehouse at 0, Border at %d) %s", node, got[node], want, tc.borderAt, tc.why)
				}
			}
		})
	}
}

// TestQualifyingStartsIgnoresUnreachableBorder guards the -1 that
// bfsDistances uses for an unreachable node: it must not be read as a
// distance, so a Border on the far side of a disconnected graph cannot make
// a Warehouse contractable, and an unreachable node cannot count as being
// within 2 steps of one.
func TestQualifyingStartsIgnoresUnreachableBorder(t *testing.T) {
	sectors := make([]game.Sector, 4)
	for i := range sectors {
		sectors[i] = game.SectorOldDocks
	}
	b := newBuilder(4, sectors)
	b.connect(0, 1) // nodes 2 and 3 form a separate component
	b.connect(2, 3)

	types := []game.NodeType{game.NodeWarehouse, game.NodeAlley, game.NodeAlley, game.NodeBorder}

	for node, q := range qualifyingStarts(b, types, testOpeningMin, testOpeningMax) {
		if q {
			t.Errorf("qualifyingStarts()[%d] = true, want false — the only Border is unreachable from the only Warehouse", node)
		}
	}
}

func TestReachesBorderInBand(t *testing.T) {
	types := []game.NodeType{game.NodeWarehouse, game.NodeBorder, game.NodeAlley, game.NodeBorder, game.NodeBlackMarket}

	cases := []struct {
		name string
		dist []int
		want bool
	}{
		{"border inside the band", []int{0, 3, 1, 9, 2}, true},
		{"border at the ceiling", []int{0, 4, 1, 9, 2}, true},
		{"border below the band", []int{0, 2, 1, 9, 2}, false},
		{"border above the band", []int{0, 5, 1, 9, 2}, false},
		{"border unreachable", []int{0, -1, 1, -1, 2}, false},
		{"only a non-border sits in the band", []int{0, 9, 3, 9, 4}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reachesBorderInBand(tc.dist, types, testOpeningMin, testOpeningMax); got != tc.want {
				t.Errorf("reachesBorderInBand(%v, types, %d, %d) = %v, want %v", tc.dist, testOpeningMin, testOpeningMax, got, tc.want)
			}
		})
	}
}

func TestFarEnoughFromAll(t *testing.T) {
	// dist is the candidate's own distance to every other node; node 5 is
	// unreachable.
	dist := []int{0, 1, 2, 3, 4, -1}

	cases := []struct {
		name     string
		selected []game.NodeID
		want     bool
	}{
		{"no one selected yet", nil, true},
		{"far enough from everyone", []game.NodeID{4}, true},
		{"too close to one", []game.NodeID{1, 4}, false},
		{"unreachable counts as too close", []game.NodeID{5}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := farEnoughFromAll(dist, tc.selected); got != tc.want {
				t.Errorf("farEnoughFromAll(dist, %v) = %v, want %v", tc.selected, got, tc.want)
			}
		})
	}
}

// TestSelectStartPositionsFindsValidSet builds a small, fully-known graph —
// a cycle of 8 nodes with one Warehouse and one Border — where exactly two
// nodes are both far enough apart (distance 4 on an 8-cycle) and have the
// Warehouse within 2 steps, and checks selectStartPositions actually finds
// that pair rather than only being exercised indirectly through the full
// property test.
func TestSelectStartPositionsFindsValidSet(t *testing.T) {
	b, types := warehouseCycle(true)

	rand, _ := countingRand(0)
	starts, ok := selectStartPositions(rand, b, 2, types, testOpeningMin, testOpeningMax)
	if !ok {
		t.Fatal("selectStartPositions() = false, want a valid pair to exist (an 8-cycle has antipodal nodes at distance 4)")
	}
	if len(starts) != 2 {
		t.Fatalf("selectStartPositions returned %d starts, want 2", len(starts))
	}
	if !slices.IsSorted(starts) {
		t.Fatalf("selectStartPositions returned %v, want ascending NodeID", starts)
	}

	dist := b.bfsDistances(starts[0])
	if d := dist[starts[1]]; d < minStartDistance {
		t.Fatalf("selected starts %v are distance %d apart, want >= %d", starts, d, minStartDistance)
	}
	qualified := qualifyingStarts(b, types, testOpeningMin, testOpeningMax)
	for _, s := range starts {
		if !qualified[s] {
			t.Fatalf("selected start %d has no contractable Warehouse within %d steps", s, maxStartWarehouseDistance)
		}
	}
}

// TestSelectStartPositionsFailsWhenImpossible checks the false path: asking
// for more mutually-far-apart, contractable-Warehouse-adjacent starts than a
// tiny graph can ever provide must report failure, not a short or invalid
// slice.
func TestSelectStartPositionsFailsWhenImpossible(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks)
	b := newBuilder(3, sectors)
	b.connect(0, 1)
	b.connect(1, 2)

	types := []game.NodeType{game.NodeWarehouse, game.NodeAlley, game.NodeAlley}

	rand, _ := countingRand(0)
	starts, ok := selectStartPositions(rand, b, 2, types, testOpeningMin, testOpeningMax)
	if ok {
		t.Fatalf("selectStartPositions() = %v, true — want false, a 3-node path cannot fit 2 nodes at distance >= %d", starts, minStartDistance)
	}
	if starts != nil {
		t.Fatalf("selectStartPositions returned %v on failure, want nil", starts)
	}
}

// TestSelectStartPositionsFailsWithNoWarehouse checks that constraint 7
// alone — no Warehouse anywhere near enough — is also detected as failure,
// even when constraint 5's distance requirement would otherwise be easy.
func TestSelectStartPositionsFailsWithNoWarehouse(t *testing.T) {
	sectors := make([]game.Sector, 10)
	for i := range sectors {
		sectors[i] = game.SectorOldDocks
	}
	b := newBuilder(10, sectors)
	for i := range 9 {
		b.connect(game.NodeID(i), game.NodeID(i+1))
	}

	types := make([]game.NodeType, 10)
	for i := range types {
		types[i] = game.NodeAlley // no Warehouse anywhere on the map
	}

	rand, _ := countingRand(0)
	if _, ok := selectStartPositions(rand, b, 2, types, testOpeningMin, testOpeningMax); ok {
		t.Fatal("selectStartPositions() = true, want false — no node in the graph has a nearby Warehouse")
	}
}

// TestSelectStartPositionsRejectsUncontractableWarehouse is D24's own
// regression test at the unit level: the same 8-cycle that succeeds with a
// Border at distance 4 must fail with no Border at all. Every node still has
// a Warehouse within 2 steps under the constraint's pre-D24 text, so this
// fixture is exactly the map that satisfied constraint 7's test while
// failing its stated purpose.
func TestSelectStartPositionsRejectsUncontractableWarehouse(t *testing.T) {
	b, types := warehouseCycle(false)

	rand, _ := countingRand(0)
	starts, ok := selectStartPositions(rand, b, 2, types, testOpeningMin, testOpeningMax)
	if ok {
		t.Fatalf("selectStartPositions() = %v, true — want false, the only Warehouse has no Border inside [%d, %d] (GDD §6.1 constraint 7, D24)", starts, testOpeningMin, testOpeningMax)
	}
}

// TestSelectStartPositionsDrawCountIsFixed pins the RNG cost D24 promised it
// did not change: exactly Nodes-1 draws, whether the walk succeeds or the
// strengthened constraint rejects every candidate. A predicate that let the
// function return before the shuffle — the obvious optimisation when no
// Warehouse qualifies — would desynchronise every later draw in the match
// (RFC-001 §6.4).
func TestSelectStartPositionsDrawCountIsFixed(t *testing.T) {
	cases := map[string]bool{"succeeds": true, "rejected by constraint 7": false}

	for name, border := range cases {
		t.Run(name, func(t *testing.T) {
			b, types := warehouseCycle(border)

			rand, count := countingRand(0)
			if _, ok := selectStartPositions(rand, b, 2, types, testOpeningMin, testOpeningMax); ok != border {
				t.Fatalf("selectStartPositions() = %v, want %v", ok, border)
			}
			if want := b.n - 1; *count != want {
				t.Errorf("selectStartPositions drew %d times, want exactly %d (one full shuffle, PurposeStartSelect)", *count, want)
			}
		})
	}
}
