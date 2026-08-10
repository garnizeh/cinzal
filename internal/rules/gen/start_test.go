package gen

import (
	"slices"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

func TestHasNearbyWarehouse(t *testing.T) {
	types := []game.NodeType{game.NodeAlley, game.NodeWarehouse, game.NodeBorder, game.NodeAlley, game.NodeWarehouse}

	cases := []struct {
		name string
		dist []int
		want bool
	}{
		{"warehouse at distance 0", []int{0, -1, -1, -1, -1}, false}, // node 0 itself is an Alley
		{"warehouse at distance 2", []int{2, 5, 5, 5, 2}, true},
		{"warehouse just out of range", []int{5, 3, 5, 5, 3}, false},
		{"warehouse unreachable", []int{0, -1, 5, 5, -1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasNearbyWarehouse(tc.dist, types); got != tc.want {
				t.Errorf("hasNearbyWarehouse(%v, %v) = %v, want %v", tc.dist, types, got, tc.want)
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
// a cycle of 8 nodes, one Warehouse — where exactly two nodes are far
// enough apart (distance 4 on an 8-cycle) and both happen to have a nearby
// Warehouse, and checks selectStartPositions actually finds a satisfying
// pair rather than only being exercised indirectly through the full
// property test.
func TestSelectStartPositionsFindsValidSet(t *testing.T) {
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
	types[0] = game.NodeWarehouse // within 2 steps of every node on this 8-cycle

	rand, _ := countingRand(0)
	starts, ok := selectStartPositions(rand, b, 2, types)
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
	for _, s := range starts {
		if !hasNearbyWarehouse(b.bfsDistances(s), types) {
			t.Fatalf("selected start %d has no Warehouse within %d steps", s, maxStartWarehouseDistance)
		}
	}
}

// TestSelectStartPositionsFailsWhenImpossible checks the false path: asking
// for more mutually-far-apart, Warehouse-adjacent starts than a tiny graph
// can ever provide must report failure, not a short or invalid slice.
func TestSelectStartPositionsFailsWhenImpossible(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks)
	b := newBuilder(3, sectors)
	b.connect(0, 1)
	b.connect(1, 2)

	types := []game.NodeType{game.NodeWarehouse, game.NodeAlley, game.NodeAlley}

	rand, _ := countingRand(0)
	starts, ok := selectStartPositions(rand, b, 2, types)
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
	for i := 0; i < 9; i++ {
		b.connect(game.NodeID(i), game.NodeID(i+1))
	}

	types := make([]game.NodeType, 10)
	for i := range types {
		types[i] = game.NodeAlley // no Warehouse anywhere on the map
	}

	rand, _ := countingRand(0)
	if _, ok := selectStartPositions(rand, b, 2, types); ok {
		t.Fatal("selectStartPositions() = true, want false — no node in the graph has a nearby Warehouse")
	}
}
