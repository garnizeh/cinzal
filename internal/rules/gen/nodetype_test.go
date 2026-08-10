package gen

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestNodeTypeCountsMatchesD9Table is issue #60's acceptance criterion
// "Type counts match #47's table exactly at 12, 15, 16, 20, 22, 25 and 28
// nodes" — the exact table from D9's decision log and GDD §6.2.
func TestNodeTypeCountsMatchesD9Table(t *testing.T) {
	// index order: Warehouse, Border, Black Market, Alley (nodeTypeOrder).
	cases := map[int][4]int{
		12: {3, 3, 2, 4},
		15: {4, 3, 3, 5},
		16: {4, 4, 3, 5},
		20: {5, 5, 4, 6},
		22: {5, 5, 5, 7},
		25: {6, 6, 5, 8},
		28: {7, 7, 5, 9},
	}
	for n, want := range cases {
		got := nodeTypeCounts(n)
		if got != want {
			t.Errorf("nodeTypeCounts(%d) = %v, want %v (D9, GDD §6.2)", n, got, want)
		}
		sum := 0
		for _, c := range got {
			sum += c
		}
		if sum != n {
			t.Errorf("nodeTypeCounts(%d) sums to %d, want %d", n, sum, n)
		}
	}
}

// completeGraphNeighbors builds the neighbor lists for K_n — every node
// adjacent to every other — the densest possible topology, used below to
// force a guaranteed constraint-6 deadlock regardless of which physical
// nodes end up Warehouse and Border.
func completeGraphNeighbors(n int) [][]game.NodeID {
	neighbors := make([][]game.NodeID, n)
	for i := range n {
		for j := range n {
			if i != j {
				neighbors[i] = append(neighbors[i], game.NodeID(j))
			}
		}
	}
	return neighbors
}

func TestNeighborHasType(t *testing.T) {
	types := []game.NodeType{game.NodeWarehouse, game.NodeBorder, game.NodeAlley, 0 /* unassigned */}

	if !neighborHasType(types, []game.NodeID{2, 0}, game.NodeWarehouse) {
		t.Error("neighborHasType(neighbors={2,0}, Warehouse) = false, want true (node 0 is a Warehouse)")
	}
	if neighborHasType(types, []game.NodeID{1, 2}, game.NodeWarehouse) {
		t.Error("neighborHasType(neighbors={1,2}, Warehouse) = true, want false (neither is a Warehouse)")
	}
	// An unassigned neighbour (zero-valued NodeType) must never match a
	// real target type such as Warehouse or Border — this is the whole
	// reason NodeType reserves its zero value as invalid (game/enums.go),
	// and the only way neighborHasType is ever actually called.
	if neighborHasType(types, []game.NodeID{3}, game.NodeWarehouse) {
		t.Error("neighborHasType matched an unassigned neighbour against Warehouse — the zero value must never be treated as a real type")
	}
	if neighborHasType(types, nil, game.NodeWarehouse) {
		t.Error("neighborHasType(nil neighbors, Warehouse) = true, want false")
	}
}

// TestAttemptAssignNodeTypesSucceedsOnSparseGraph checks the success path
// directly: on a sparse graph with plenty of room, one walk should place
// exactly nodeTypeCounts(n), respect constraint 6, and consume exactly n
// draws — regardless of which seed drives the choices.
func TestAttemptAssignNodeTypesSucceedsOnSparseGraph(t *testing.T) {
	n := 10
	// A 10-cycle: every node has exactly 2 neighbours, giving each choice
	// plenty of unblocked alternatives.
	neighbors := make([][]game.NodeID, n)
	for i := range n {
		neighbors[i] = []game.NodeID{game.NodeID((i - 1 + n) % n), game.NodeID((i + 1) % n)}
	}

	for seed := 0; seed < 20; seed++ {
		rand, count := countingRand(seed)
		types, ok := attemptAssignNodeTypes(rand, n, neighbors)
		if !ok {
			continue // a single walk is allowed to deadlock; assignNodeTypes retries
		}
		if *count != n {
			t.Fatalf("seed=%d: attemptAssignNodeTypes consumed %d draws, want exactly %d", seed, *count, n)
		}

		var counts [4]int
		for _, ty := range types {
			for ti, want := range nodeTypeOrder {
				if ty == want {
					counts[ti]++
				}
			}
		}
		if want := nodeTypeCounts(n); counts != want {
			t.Fatalf("seed=%d: type counts = %v, want %v", seed, counts, want)
		}
		for i, ty := range types {
			if ty != game.NodeWarehouse {
				continue
			}
			for _, j := range neighbors[i] {
				if types[j] == game.NodeBorder {
					t.Fatalf("seed=%d: Warehouse %d is adjacent to Border %d", seed, i, j)
				}
			}
		}
	}
}

// TestAttemptAssignNodeTypesDeadlocksOnCompleteGraph forces a guaranteed
// failure: on K4 (every one of 4 nodes adjacent to every other),
// nodeTypeCounts(4) requires exactly one Warehouse and one Border among the
// 4 nodes — and any two distinct nodes on K4 are adjacent, so a Warehouse
// and a Border can never avoid being adjacent to each other. This must fail
// for every possible sequence of random choices, not just some.
func TestAttemptAssignNodeTypesDeadlocksOnCompleteGraph(t *testing.T) {
	n := 4
	if counts := nodeTypeCounts(n); counts[0] == 0 || counts[1] == 0 {
		t.Fatalf("test premise broken: nodeTypeCounts(%d) = %v needs both a Warehouse and a Border for K4 to force a deadlock", n, counts)
	}
	neighbors := completeGraphNeighbors(n)

	for seed := 0; seed < 50; seed++ {
		rand, _ := countingRand(seed)
		if _, ok := attemptAssignNodeTypes(rand, n, neighbors); ok {
			t.Fatalf("seed=%d: attemptAssignNodeTypes succeeded on K4, want failure — a Warehouse and Border can never be non-adjacent there", seed)
		}
	}
}

// TestAssignNodeTypesRetriesThenGivesUp checks assignNodeTypes' own wrapper
// behaviour: it must retry attemptAssignNodeTypes rather than propagating a
// single walk's failure, but still return false (never panic or loop
// forever) once the topology is truly unsatisfiable.
func TestAssignNodeTypesRetriesThenGivesUp(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks)
	b := newBuilder(4, sectors)
	// K4: connect every pair.
	for i := range 4 {
		for j := i + 1; j < 4; j++ {
			b.connect(game.NodeID(i), game.NodeID(j))
		}
	}

	rand, count := countingRand(0)
	types, ok := assignNodeTypes(rand, b)
	if ok {
		t.Fatalf("assignNodeTypes succeeded on K4 with types=%v, want failure", types)
	}
	if types != nil {
		t.Fatalf("assignNodeTypes returned %v on failure, want nil", types)
	}
	// Every walk on K4 deadlocks at the very last node: the first n-1 nodes
	// always have a safe BM/Alley fallback, so exactly n-1 draws are spent
	// before the last node finds zero candidates and returns without
	// drawing — a property specific to this K4 test fixture, not a general
	// guarantee attemptAssignNodeTypes makes.
	if want := typeAssignMaxTries * (b.n - 1); *count != want {
		t.Fatalf("assignNodeTypes consumed %d draws before giving up, want exactly %d (typeAssignMaxTries * (n-1))", *count, want)
	}
}

// TestAssignNodeTypesSucceedsEventually checks the retry loop's positive
// case: a topology where a single walk sometimes deadlocks (so the retry
// path is genuinely exercised) but a satisfying assignment exists, so
// assignNodeTypes must find it well within typeAssignMaxTries.
func TestAssignNodeTypesSucceedsEventually(t *testing.T) {
	for _, tc := range propertyCases() {
		t.Run(tc.name, func(t *testing.T) {
			for seed := 0; seed < 50; seed++ {
				rand := newTestRand(seed)
				sizes := sectorSizes(tc.params.Nodes)
				sector := assignSectors(rand, tc.params.Nodes, sizes)
				b := newBuilder(tc.params.Nodes, sector)
				for _, s := range sectorOrder {
					nodeSpanningTree(rand, b, b.nodesInSector(s))
				}
				pairs := sectorAdjacency(rand)
				target := tc.params.MinEdges + rand(PurposeEdgeCount, tc.params.MaxEdges-tc.params.MinEdges+1)
				chokepointCounts := buildChokepoints(rand, b, pairs)
				fillEdges(rand, b, pairs, chokepointCounts, target)
				if failed := b.violations(chokepointCounts, tc.params.MinEdges, tc.params.MaxEdges); len(failed) > 0 {
					continue
				}

				if _, ok := assignNodeTypes(rand, b); !ok {
					t.Fatalf("seed=%d: assignNodeTypes exhausted %d tries on a structurally valid graph", seed, typeAssignMaxTries)
				}
			}
		})
	}
}
