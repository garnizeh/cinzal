package gen

import (
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestLayoutDeterministic is issue #60's acceptance criterion "A test folds
// the same seed twice and asserts identical coordinates": two Generate()
// calls from the same seed must agree on every node's (X, Y), not just its
// topology (TestGenerateDeterministic already covers topology; this pins
// coordinates specifically).
func TestLayoutDeterministic(t *testing.T) {
	for _, tc := range propertyCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, seed := range []int{0, 1, 42, 12345} {
				g1, err := Generate(newTestRand(seed), tc.params)
				if err != nil {
					t.Fatalf("seed=%d: first Generate() = %v", seed, err)
				}
				g2, err := Generate(newTestRand(seed), tc.params)
				if err != nil {
					t.Fatalf("seed=%d: second Generate() = %v", seed, err)
				}
				for i := range g1.Nodes {
					if g1.Nodes[i].X != g2.Nodes[i].X || g1.Nodes[i].Y != g2.Nodes[i].Y {
						t.Fatalf("seed=%d: node %d coordinates differ across two Generate() calls from the same seed: (%d,%d) vs (%d,%d)",
							seed, i, g1.Nodes[i].X, g1.Nodes[i].Y, g2.Nodes[i].X, g2.Nodes[i].Y)
					}
				}
			}
		})
	}
}

// TestLayoutStableAfterSimulatedBridgeDown is issue #60's acceptance
// criterion "asserts coordinates are unchanged after a simulated Bridge
// Down": removing an edge — the only mutation Bridge Down (GDD §14.2) ever
// makes to the map — must never touch a node's (X, Y), because gen computes
// layout independently of the live edge set (D10) and nothing recomputes it
// afterward.
func TestLayoutStableAfterSimulatedBridgeDown(t *testing.T) {
	params := testParams(4)
	g, err := Generate(newTestRand(7), params)
	if err != nil {
		t.Fatalf("Generate() = %v", err)
	}

	before := make([]struct{ x, y int }, len(g.Nodes))
	for i, n := range g.Nodes {
		before[i] = struct{ x, y int }{n.X, n.Y}
	}

	// Simulate Bridge Down: destroy one edge, exactly as rules.Resolve would
	// (remove it from both endpoints' adjacency lists) — never touching X
	// or Y, because those live on the same Node value but are never written
	// by an edge-removal operation.
	a := g.Nodes[0].ID
	c := g.Nodes[0].Edges[0]
	removeEdge(&g, a, c)

	for i, n := range g.Nodes {
		if n.X != before[i].x || n.Y != before[i].y {
			t.Fatalf("node %d coordinates changed after simulated Bridge Down: (%d,%d) -> (%d,%d)",
				n.ID, before[i].x, before[i].y, n.X, n.Y)
		}
	}
}

// removeEdge deletes the edge between a and c from both endpoints'
// adjacency lists, in place — the same mutation Bridge Down performs on
// rules.Graph, reproduced here on gen.Graph's own shape to test layout
// stability without needing internal/rules (which this package must not
// import; see doc.go).
func removeEdge(g *Graph, a, c game.NodeID) {
	g.Nodes[a].Edges = removeFromSlice(g.Nodes[a].Edges, c)
	g.Nodes[c].Edges = removeFromSlice(g.Nodes[c].Edges, a)
}

func removeFromSlice(edges []game.NodeID, target game.NodeID) []game.NodeID {
	out := edges[:0]
	for _, e := range edges {
		if e != target {
			out = append(out, e)
		}
	}
	return out
}

// TestCanvasSizeIsCompileTimeConstant is issue #60's acceptance criterion
// "the canvas bounds are a compile-time constant and do not vary with node
// count or player count": CanvasSize is a Go const, which is checked at
// compile time by definition, and this test additionally confirms every
// generated graph, at every supported node/player count, honours the same
// bound.
func TestCanvasSizeIsCompileTimeConstant(t *testing.T) {
	// This assignment only compiles if CanvasSize is a constant expression.
	const mustBeConst = CanvasSize
	if mustBeConst != 1000 {
		t.Fatalf("CanvasSize = %d, want 1000 (D10)", mustBeConst)
	}

	for _, tc := range propertyCases() {
		g, err := Generate(newTestRand(0), tc.params)
		if err != nil {
			t.Fatalf("%s: Generate() = %v", tc.name, err)
		}
		for _, n := range g.Nodes {
			if n.X < 0 || n.X >= CanvasSize || n.Y < 0 || n.Y >= CanvasSize {
				t.Fatalf("%s: node %d coordinates (%d, %d) out of the fixed [0, %d) bound", tc.name, n.ID, n.X, n.Y, CanvasSize)
			}
		}
	}
}

// TestLayoutFieldsAreNeverFloat is issue #60's acceptance criterion "No
// coordinate is a float64 at any point, including intermediates": RFC §6.3
// forbids float64 anywhere in rules, gen included (D10). Node's own X, Y
// fields are declared int, checked here by reflection so a future edit that
// widens them to a float type fails this test rather than silently
// reintroducing the hazard D10 ruled out.
func TestLayoutFieldsAreNeverFloat(t *testing.T) {
	typ := reflect.TypeFor[Node]()
	for _, name := range []string{"X", "Y"} {
		f, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("Node has no field %q", name)
		}
		switch f.Type.Kind() {
		case reflect.Float32, reflect.Float64:
			t.Fatalf("Node.%s is %s, want an integer type (RFC §6.3, D10)", name, f.Type.Kind())
		}
	}
}

// TestComputeLayoutQuadrantsAndDrawCount is a direct unit test of
// computeLayout, independent of the full Generate() pipeline: builds a
// 12-node graph with a known, literal sector assignment (3 nodes per
// sector) and checks each node lands inside its sector's own quadrant, on
// one of the fixed lattice's cells, and that the draw count matches D10's
// "exactly n per sector" formula exactly.
func TestComputeLayoutQuadrantsAndDrawCount(t *testing.T) {
	var sectors []game.Sector
	for _, s := range sectorOrder {
		sectors = append(sectors, s, s, s)
	}
	b := newBuilder(len(sectors), sectors)

	rand, count := countingRand(0)
	x, y := computeLayout(rand, b)

	if *count != b.n {
		t.Fatalf("computeLayout consumed %d draws, want exactly %d (one per node, D10)", *count, b.n)
	}

	for qi, s := range sectorOrder {
		origin := quadrantOrigins[qi]
		for _, node := range b.nodesInSector(s) {
			nx, ny := x[node], y[node]
			if nx < origin.X || nx >= origin.X+quadrantSize || ny < origin.Y || ny >= origin.Y+quadrantSize {
				t.Fatalf("sector %s node %d at (%d, %d) is outside its quadrant %v..+%d", s, node, nx, ny, origin, quadrantSize)
			}

			onLattice := false
			for _, cell := range latticeCells {
				if nx == origin.X+cell.X && ny == origin.Y+cell.Y {
					onLattice = true
					break
				}
			}
			if !onLattice {
				t.Fatalf("sector %s node %d at (%d, %d) is not on the fixed 9-cell lattice", s, node, nx, ny)
			}
		}
	}
}

// TestComputeLayoutNoCollisionsWithinSector checks minimum separation by
// construction: a sector at D8's maximum size (8) exactly fills the 9-cell
// lattice minus one, so every node in it must land on a distinct cell.
func TestComputeLayoutNoCollisionsWithinSector(t *testing.T) {
	sizes := [4]int{8, 8, 8, 4} // 28 nodes total, matching the 5-player table
	var sectors []game.Sector
	for i, s := range sectorOrder {
		for range sizes[i] {
			sectors = append(sectors, s)
		}
	}
	b := newBuilder(len(sectors), sectors)

	rand, _ := countingRand(0)
	x, y := computeLayout(rand, b)

	for _, s := range sectorOrder {
		seen := map[[2]int]bool{}
		for _, node := range b.nodesInSector(s) {
			key := [2]int{x[node], y[node]}
			if seen[key] {
				t.Fatalf("sector %s: two nodes share coordinates (%d, %d)", s, key[0], key[1])
			}
			seen[key] = true
		}
	}
}

func testParams(players int) Params {
	cfg := game.DefaultConfig()
	spec := cfg.MapByPlayers[players]
	return Params{
		Nodes:       spec.Nodes,
		MinEdges:    spec.MinEdges,
		MaxEdges:    spec.MaxEdges,
		Players:     players,
		MaxAttempts: cfg.MaxGenAttempts,
	}
}
