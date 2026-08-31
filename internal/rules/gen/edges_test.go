package gen

import (
	"slices"
	"sort"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

func TestNewEdgeCandidateNormalizesOrder(t *testing.T) {
	c1 := newEdgeCandidate(3, 1)
	c2 := newEdgeCandidate(1, 3)

	if c1 != c2 {
		t.Fatalf("newEdgeCandidate(3, 1) = %v, newEdgeCandidate(1, 3) = %v — want equal regardless of argument order", c1, c2)
	}
	if c1.a != 1 || c1.c != 3 {
		t.Fatalf("newEdgeCandidate did not normalize to (a < c): got %v", c1)
	}
}

func TestSortCandidatesAscending(t *testing.T) {
	candidates := []edgeCandidate{
		{a: 2, c: 3},
		{a: 0, c: 5},
		{a: 0, c: 1},
		{a: 1, c: 2},
	}
	sortCandidates(candidates)

	want := []edgeCandidate{
		{a: 0, c: 1},
		{a: 0, c: 5},
		{a: 1, c: 2},
		{a: 2, c: 3},
	}
	if !slices.Equal(candidates, want) {
		t.Fatalf("sortCandidates() = %v, want %v", candidates, want)
	}
}

func TestIntraCandidates(t *testing.T) {
	// Sector OldDocks = {0, 1, 2}; 0-1 already connected, 0-2 and 1-2 not.
	b := newBuilder(4, testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorIronLow))
	b.connect(0, 1)

	got := intraCandidates(b, game.SectorOldDocks)
	want := []edgeCandidate{{a: 0, c: 2}, {a: 1, c: 2}}
	if !slices.Equal(got, want) {
		t.Fatalf("intraCandidates(OldDocks) = %v, want %v", got, want)
	}

	// A sector with only one node has no intra-sector candidate at all.
	if got := intraCandidates(b, game.SectorIronLow); got != nil {
		t.Fatalf("intraCandidates(IronLow, 1 node) = %v, want nil", got)
	}
}

func TestCrossCandidates(t *testing.T) {
	// OldDocks = {0, 1}, IronLow = {2, 3}; 0-2 already connected.
	b := newBuilder(4, testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorIronLow, game.SectorIronLow))
	b.connect(0, 2)

	got := crossCandidates(b, game.SectorOldDocks, game.SectorIronLow)
	want := []edgeCandidate{{a: 0, c: 3}, {a: 1, c: 2}, {a: 1, c: 3}}
	if !slices.Equal(got, want) {
		t.Fatalf("crossCandidates(OldDocks, IronLow) = %v, want %v", got, want)
	}
}

func TestDegreeNeed(t *testing.T) {
	b := newBuilder(4, testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks))
	// Node 0 stays at degree 0 (needs one more to reach minDegree=2).
	b.connect(0, 1) // node 0: degree 1, node 1: degree 1
	b.connect(1, 2) // node 1: degree 2 (satisfied), node 2: degree 1
	b.connect(1, 3) // node 1: degree 3, node 3: degree 1

	if got := degreeNeed(b, edgeCandidate{a: 0, c: 2}); got != 2 {
		t.Errorf("degreeNeed(0-2) = %d, want 2 (both endpoints below minDegree)", got)
	}
	if got := degreeNeed(b, edgeCandidate{a: 2, c: 3}); got != 2 {
		t.Errorf("degreeNeed(2-3) = %d, want 2 (both at degree 1, below minDegree)", got)
	}
	if got := degreeNeed(b, edgeCandidate{a: 1, c: 2}); got != 1 {
		t.Errorf("degreeNeed(1-2) = %d, want 1 (node 1 already at minDegree, node 2 below)", got)
	}
}

func TestSortByDegreeNeedIsStableAndDescending(t *testing.T) {
	b := newBuilder(4, testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks))
	// All nodes start at degree 0, so every candidate below has degreeNeed
	// == 2 except the ones naming node 1, once it is bumped up.
	b.connect(1, 2)
	b.connect(1, 3) // node 1 now at degree 2 (satisfied); 2 and 3 at degree 1.

	candidates := []edgeCandidate{
		{a: 0, c: 1}, // need 1 (node 0 below, node 1 satisfied)
		{a: 0, c: 2}, // need 2 (both below)
		{a: 2, c: 3}, // need 2 (both below)
		{a: 1, c: 0}, // duplicate shape of the first, to check stability
	}
	original := slices.Clone(candidates)
	sortByDegreeNeed(b, candidates)

	if degreeNeed(b, candidates[0]) < degreeNeed(b, candidates[len(candidates)-1]) {
		t.Fatalf("sortByDegreeNeed did not sort descending: %v", candidates)
	}
	// Verify it is a stable sort: elements with equal degreeNeed keep their
	// relative input order.
	sort.SliceStable(original, func(i, j int) bool {
		return degreeNeed(b, original[i]) > degreeNeed(b, original[j])
	})
	if !slices.Equal(candidates, original) {
		t.Fatalf("sortByDegreeNeed(candidates) = %v, want the same stable-sort result %v", candidates, original)
	}
}

func TestNodeSpanningTreeSingleNodeIsNoOp(t *testing.T) {
	rand, count := countingRand(0)
	b := newBuilder(1, testSectors(game.SectorOldDocks))
	nodeSpanningTree(rand, b, b.nodesInSector(game.SectorOldDocks))

	if *count != 0 {
		t.Fatalf("nodeSpanningTree on 1 node consumed %d draws, want 0", *count)
	}
	if b.edgeCount() != 0 {
		t.Fatalf("nodeSpanningTree on 1 node created %d edges, want 0", b.edgeCount())
	}
}

func TestNodeSpanningTreeConnectsWithinDegreeCap(t *testing.T) {
	for seed := range 30 {
		rand, _ := countingRand(seed)
		sectors := make([]game.Sector, 8)
		for i := range sectors {
			sectors[i] = game.SectorOldDocks
		}
		b := newBuilder(8, sectors)
		nodes := b.nodesInSector(game.SectorOldDocks)

		nodeSpanningTree(rand, b, nodes)

		if got := b.edgeCount(); got != len(nodes)-1 {
			t.Fatalf("seed=%d: nodeSpanningTree produced %d edges, want %d (a tree over %d nodes)", seed, got, len(nodes)-1, len(nodes))
		}
		if !inducedConnected(Graph{Nodes: builderToNodes(b)}, nodes) {
			t.Fatalf("seed=%d: nodeSpanningTree left the sector disconnected", seed)
		}
		for _, n := range nodes {
			if b.degree[n] > maxDegree {
				t.Fatalf("seed=%d: node %d has degree %d after nodeSpanningTree, want <= %d", seed, n, b.degree[n], maxDegree)
			}
		}
	}
}

// builderToNodes converts b's current adjacency matrix into gen.Node
// values, for tests that want to reuse generate_test.go's inducedConnected
// helper against a builder mid-construction rather than a finished Graph.
func builderToNodes(b *builder) []Node {
	nodes := make([]Node, b.n)
	for i := range b.n {
		var edges []game.NodeID
		for j := range b.n {
			if b.adj[i][j] {
				edges = append(edges, game.NodeID(j))
			}
		}
		nodes[i] = Node{ID: game.NodeID(i), Sector: b.sector[i], Edges: edges}
	}
	return nodes
}

func TestLowestDegreeAmongTieBreaksUniformly(t *testing.T) {
	b := newBuilder(3, testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks))
	// All three nodes start at degree 0 — a three-way tie.
	added := []game.NodeID{0, 1, 2}

	seen := map[game.NodeID]bool{}
	for seed := range 20 {
		rand, count := countingRand(seed)
		got := lowestDegreeAmong(rand, "test", b, added)
		seen[got] = true
		if *count != 1 {
			t.Fatalf("seed=%d: lowestDegreeAmong consumed %d draws, want exactly 1", seed, *count)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("lowestDegreeAmong across 20 seeds only ever returned %v, want to see all of %v eventually", seen, added)
	}

	// Once node 0 is bumped above the others, it must never be returned.
	b.connect(0, 1)
	b.connect(0, 2)
	for seed := range 20 {
		rand, _ := countingRand(seed)
		if got := lowestDegreeAmong(rand, "test", b, added); got == 0 {
			t.Fatalf("seed=%d: lowestDegreeAmong returned node 0 (degree %d) over nodes at lower degree", seed, b.degree[0])
		}
	}
}

func TestBuildChokepointsRespectsTargetAndDegreeCap(t *testing.T) {
	// Two sectors of 4 nodes each, fully internally connected first so every
	// candidate's degreeNeed starts at 0 and the test isolates
	// buildChokepoints' own target/degree-cap behaviour.
	sectors := append(testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks),
		game.SectorIronLow, game.SectorIronLow, game.SectorIronLow, game.SectorIronLow)
	b := newBuilder(8, sectors)
	for _, e := range [][2]game.NodeID{{0, 1}, {1, 2}, {2, 3}, {4, 5}, {5, 6}, {6, 7}} {
		b.connect(e[0], e[1])
	}

	pairs := []sectorPair{newSectorPair(game.SectorOldDocks, game.SectorIronLow)}
	rand, _ := countingRand(0)
	counts := buildChokepoints(rand, b, pairs)

	if len(counts) != 1 {
		t.Fatalf("buildChokepoints returned %d counts, want 1 (one per pair)", len(counts))
	}
	if counts[0] < 3 || counts[0] > 5 {
		t.Fatalf("buildChokepoints accepted %d chokepoint edges, want [3, 5] (GDD §6.1 constraint 4)", counts[0])
	}
	for i := range b.n {
		if b.degree[i] > maxDegree {
			t.Fatalf("node %d has degree %d after buildChokepoints, want <= %d", i, b.degree[i], maxDegree)
		}
	}
}

func TestFillEdgesStopsAtTarget(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks)
	b := newBuilder(4, sectors)
	b.connect(0, 1)
	b.connect(1, 2)
	b.connect(2, 3) // 3 edges so far; every node degree 1 or 2

	rand, _ := countingRand(0)
	fillEdges(rand, b, nil, nil, 3) // already at target — must add nothing

	if got := b.edgeCount(); got != 3 {
		t.Fatalf("fillEdges with target already met changed edge count to %d, want unchanged 3", got)
	}
}

func TestFillEdgesReachesHigherTarget(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks)
	b := newBuilder(4, sectors)
	b.connect(0, 1)
	b.connect(1, 2)
	b.connect(2, 3)

	rand, _ := countingRand(0)
	fillEdges(rand, b, nil, nil, 4)

	if got := b.edgeCount(); got != 4 {
		t.Fatalf("fillEdges(target=4) reached %d edges, want exactly 4", got)
	}
	for i := range b.n {
		if b.degree[i] > maxDegree {
			t.Fatalf("node %d has degree %d after fillEdges, want <= %d", i, b.degree[i], maxDegree)
		}
	}
}
