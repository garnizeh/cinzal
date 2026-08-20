package gen

import (
	"math/rand/v2"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// newTestRand builds a Rand from a plain integer seed, for this package's
// own property tests. It has no relationship to *rules.RNG's HMAC
// derivation — that composition is checked separately, end to end, by
// internal/rules's TestGenerateWiredToRealRNGIsDeterministic — this is only
// gen's own promise that Generate is a pure function of whatever sequence
// its Rand produces.
func newTestRand(seed int) Rand {
	r := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b97f4a7c15))
	return func(purpose string, n int) int {
		return r.IntN(n)
	}
}

// propertyCase is one node-count/player-count combination this test suite
// verifies GDD §6.1 constraints 1-5 against.
type propertyCase struct {
	name   string
	params Params
	seeds  int
}

// propertyCases covers every node count GDD §6.1 and D8 name: the four
// per-player-count rows of §6.1's own table (game.DefaultConfig's
// MapByPlayers), plus the two scenario sizes D8's decision log singles out
// as the tightest ones to validate (12 and 16 — issue #59's acceptance
// criterion "Sector sizing follows #46 at 12, 15 and 16 nodes, and those
// sizes generate without exhausting retries"). GDD §6.1's own table has no
// edge-count row for 12 or 16 nodes — those only ever appear as GDD §19.1
// scenario node counts, whose edge range is M7's job to fix, not this
// issue's — so the two extra rows use a range derived from the same
// 2.8-3.2 average-degree band every other row in the table falls into
// (edges ~= 1.4-1.5 * nodes), not a number this repository has committed to
// anywhere. players=2 for both: GDD §19.1 names them as small-scenario
// (First Run) and single-district (The Trail) maps, not larger-player
// tables.
func propertyCases() []propertyCase {
	cfg := game.DefaultConfig()
	var cases []propertyCase
	for _, players := range []int{2, 3, 4, 5} {
		spec := cfg.MapByPlayers[players]
		cases = append(cases, propertyCase{
			name: "players=" + itoa(players),
			params: Params{
				Nodes:              spec.Nodes,
				MinEdges:           spec.MinEdges,
				MaxEdges:           spec.MaxEdges,
				Players:            players,
				MaxAttempts:        cfg.MaxGenAttempts,
				OpeningMinDistance: cfg.Contracts[0].MinDistance,
				OpeningMaxDistance: cfg.Contracts[0].MaxDistance,
			},
			seeds: 1000,
		})
	}
	cases = append(cases,
		propertyCase{
			name:   "D8-nodes=12",
			params: withOpeningBand(Params{Nodes: 12, MinEdges: 17, MaxEdges: 19, Players: 2, MaxAttempts: cfg.MaxGenAttempts}, cfg),
			seeds:  1000,
		},
		propertyCase{
			name:   "D8-nodes=16",
			params: withOpeningBand(Params{Nodes: 16, MinEdges: 23, MaxEdges: 25, Players: 2, MaxAttempts: cfg.MaxGenAttempts}, cfg),
			seeds:  1000,
		},
	)
	return cases
}

// withOpeningBand fills constraint 7's Tier I band (D24) from cfg's own
// contract table rather than a literal 3/4 — the same reason gen.Params
// carries the band at all: a GDD §8.3 edit must move these tests with it.
func withOpeningBand(p Params, cfg game.Config) Params {
	p.OpeningMinDistance = cfg.Contracts[0].MinDistance
	p.OpeningMaxDistance = cfg.Contracts[0].MaxDistance
	return p
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestGenerateSatisfiesConstraints is issue #59's central acceptance
// criterion: a property test over >= 1000 seeds at every supported node
// count asserting every GDD §6.1 constraint 1-5 holds, every time. Every
// check here is independent of gen's own internal verify.go — it walks
// Graph's exported shape directly, so a bug in verify.go's own bookkeeping
// cannot hide a real constraint violation from this test.
//
// The Generate error check is load-bearing on its own, not just plumbing to
// reach verifyGraph: D24's strengthened constraint 7 rejects strictly more
// start-placement attempts than the old one, and this asserts that on none
// of these 6000 seeds does it reject all of them within MaxAttempts.
func TestGenerateSatisfiesConstraints(t *testing.T) {
	for _, tc := range propertyCases() {
		t.Run(tc.name, func(t *testing.T) {
			for seed := range tc.seeds {
				g, err := Generate(newTestRand(seed), tc.params)
				if err != nil {
					t.Fatalf("seed=%d: Generate() = %v", seed, err)
				}
				verifyGraph(t, seed, tc.params, g)
			}
		})
	}
}

// verifyGraph independently re-checks every GDD §6.1 constraint 1-5 against
// g's exported fields.
func verifyGraph(t *testing.T, seed int, p Params, g Graph) {
	t.Helper()

	if len(g.Nodes) != p.Nodes {
		t.Fatalf("seed=%d: len(Nodes) = %d, want %d", seed, len(g.Nodes), p.Nodes)
	}
	for i, n := range g.Nodes {
		if n.ID != game.NodeID(i) {
			t.Fatalf("seed=%d: Nodes[%d].ID = %d, want %d", seed, i, n.ID, i)
		}
	}

	verifyDegreeAndSymmetry(t, seed, g)
	verifySectors(t, seed, g)
	verifyChokepoints(t, seed, g)
	verifyEdgeCount(t, seed, p, g)
	verifyGlobalConnectivity(t, seed, g)
	verifyStartPositions(t, seed, p, g)
	verifyNodeTypes(t, seed, g)
	verifyNoWarehouseAdjacentBorder(t, seed, g)
	verifyStartsHaveContractableWarehouse(t, seed, p, g)
	verifyLayout(t, seed, g)
}

// verifyNodeTypes is issue #60's acceptance criterion "Type counts match
// #47's table exactly at 12, 15, 16, 20, 22, 25 and 28 nodes": every node
// carries a valid type, and the totals match nodeTypeCounts(len(g.Nodes))
// (D9) exactly.
func verifyNodeTypes(t *testing.T, seed int, g Graph) {
	t.Helper()

	var counts [4]int
	for _, n := range g.Nodes {
		found := false
		for ti, nt := range nodeTypeOrder {
			if n.Type == nt {
				counts[ti]++
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("seed=%d: node %d has invalid type %v", seed, n.ID, n.Type)
		}
	}

	want := nodeTypeCounts(len(g.Nodes))
	if counts != want {
		t.Fatalf("seed=%d: node type counts = %v, want %v (D9, GDD §6.2)", seed, counts, want)
	}
}

// verifyNoWarehouseAdjacentBorder is GDD §6.1 constraint 6: no Warehouse is
// adjacent to a Border.
func verifyNoWarehouseAdjacentBorder(t *testing.T, seed int, g Graph) {
	t.Helper()

	for _, n := range g.Nodes {
		if n.Type != game.NodeWarehouse {
			continue
		}
		for _, e := range n.Edges {
			if g.Nodes[e].Type == game.NodeBorder {
				t.Fatalf("seed=%d: Warehouse %d is adjacent to Border %d (GDD §6.1 constraint 6)", seed, n.ID, e)
			}
		}
	}
}

// verifyStartsHaveContractableWarehouse is GDD §6.1 constraint 7 as
// strengthened by D24: every starting node has at least one Warehouse within
// 2 steps that itself has a Border at a distance inside Tier I's contract
// band. Both halves are asserted — a Warehouse with no Border in the band
// satisfies the constraint's old text and fails its stated purpose, which is
// the gap D24 closed.
//
// The failure message names the Warehouses it did find and why each was
// rejected: a start with no Warehouse at all and a start whose only
// Warehouses are uncontractable are different bugs, and the pre-D24 code
// would have passed the second.
func verifyStartsHaveContractableWarehouse(t *testing.T, seed int, p Params, g Graph) {
	t.Helper()

	dist := bfsAll(g)
	for _, start := range g.StartPositions {
		var nearby []game.NodeID
		found := false
		for other, d := range dist[start] {
			if d < 0 || d > maxStartWarehouseDistance || g.Nodes[other].Type != game.NodeWarehouse {
				continue
			}
			w := game.NodeID(other)
			nearby = append(nearby, w)
			if borderInBand(p, g, dist[w]) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("seed=%d: start position %d has no Warehouse within %d steps with a Border at distance [%d, %d] (GDD §6.1 constraint 7, D24) — nearby Warehouses: %v",
				seed, start, maxStartWarehouseDistance, p.OpeningMinDistance, p.OpeningMaxDistance, nearby)
		}
	}
}

// borderInBand reports whether dist (one Warehouse's distances to every
// other node) reaches a Border inside [OpeningMinDistance,
// OpeningMaxDistance] — Tier I's band, the distances an opening contract
// from that Warehouse could be drawn at (GDD §8.3).
func borderInBand(p Params, g Graph, dist []int) bool {
	for other, d := range dist {
		if d >= p.OpeningMinDistance && d <= p.OpeningMaxDistance && g.Nodes[other].Type == game.NodeBorder {
			return true
		}
	}
	return false
}

// verifyLayout is issue #60's coordinate acceptance criteria: every node
// has coordinates, in bounds, and no coordinate is a float64 at any point —
// enforced by the type system, since Node.X and Node.Y are declared int.
func verifyLayout(t *testing.T, seed int, g Graph) {
	t.Helper()

	for _, n := range g.Nodes {
		if n.X < 0 || n.X >= CanvasSize || n.Y < 0 || n.Y >= CanvasSize {
			t.Fatalf("seed=%d: node %d coordinates (%d, %d) out of [0, %d) bounds (D10)", seed, n.ID, n.X, n.Y, CanvasSize)
		}
	}

	// No two nodes in the same sector share a coordinate — the fixed
	// 9-cell lattice's whole purpose is minimum separation by construction
	// (D10), so a collision would mean the lattice or its shuffle is wrong.
	type coord struct{ x, y int }
	bySector := map[game.Sector]map[coord]game.NodeID{}
	for _, n := range g.Nodes {
		if bySector[n.Sector] == nil {
			bySector[n.Sector] = map[coord]game.NodeID{}
		}
		c := coord{n.X, n.Y}
		if other, ok := bySector[n.Sector][c]; ok {
			t.Fatalf("seed=%d: nodes %d and %d in sector %s share coordinates (%d, %d) (D10)", seed, other, n.ID, n.Sector, n.X, n.Y)
		}
		bySector[n.Sector][c] = n.ID
	}
}

func verifyDegreeAndSymmetry(t *testing.T, seed int, g Graph) {
	t.Helper()

	adjBack := make([]map[game.NodeID]bool, len(g.Nodes))
	for i := range adjBack {
		adjBack[i] = map[game.NodeID]bool{}
	}

	for _, n := range g.Nodes {
		if len(n.Edges) < minDegree || len(n.Edges) > maxDegree {
			t.Fatalf("seed=%d: node %d has degree %d, want [%d, %d] (GDD §6.1 constraint 2)", seed, n.ID, len(n.Edges), minDegree, maxDegree)
		}
		for i, e := range n.Edges {
			if e == n.ID {
				t.Fatalf("seed=%d: node %d has a self-loop", seed, n.ID)
			}
			if i > 0 && n.Edges[i-1] >= e {
				t.Fatalf("seed=%d: node %d's Edges are not strictly ascending: %v", seed, n.ID, n.Edges)
			}
			adjBack[n.ID][e] = true
		}
	}
	for _, n := range g.Nodes {
		for _, e := range n.Edges {
			if !adjBack[e][n.ID] {
				t.Fatalf("seed=%d: edge %d->%d is not symmetric", seed, n.ID, e)
			}
		}
	}
}

func verifySectors(t *testing.T, seed int, g Graph) {
	t.Helper()

	bySector := map[game.Sector][]game.NodeID{}
	for _, n := range g.Nodes {
		bySector[n.Sector] = append(bySector[n.Sector], n.ID)
	}
	if len(bySector) != 4 {
		t.Fatalf("seed=%d: graph uses %d distinct sectors, want 4", seed, len(bySector))
	}

	for s, nodes := range bySector {
		if len(nodes) < 3 || len(nodes) > 8 {
			t.Fatalf("seed=%d: sector %s has %d nodes, want [3, 8] (D8)", seed, s, len(nodes))
		}
		if !inducedConnected(g, nodes) {
			t.Fatalf("seed=%d: sector %s is not internally connected (GDD §6.1 constraint 3)", seed, s)
		}
	}
}

// inducedConnected reports whether the subgraph induced on nodes, using
// only edges with both endpoints in nodes, is connected.
func inducedConnected(g Graph, nodes []game.NodeID) bool {
	within := map[game.NodeID]bool{}
	for _, n := range nodes {
		within[n] = true
	}

	seen := map[game.NodeID]bool{nodes[0]: true}
	queue := []game.NodeID{nodes[0]}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range g.Nodes[cur].Edges {
			if within[next] && !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return len(seen) == len(nodes)
}

func verifyChokepoints(t *testing.T, seed int, g Graph) {
	t.Helper()

	sectorOf := make(map[game.NodeID]game.Sector, len(g.Nodes))
	for _, n := range g.Nodes {
		sectorOf[n.ID] = n.Sector
	}

	counts := map[sectorPair]int{}
	for _, n := range g.Nodes {
		for _, e := range n.Edges {
			if e <= n.ID {
				continue // count each undirected edge once
			}
			sa, sb := sectorOf[n.ID], sectorOf[e]
			if sa == sb {
				continue
			}
			counts[newSectorPair(sa, sb)]++
		}
	}

	adjacent := 0
	for pair, c := range counts {
		adjacent++
		if c < 3 || c > 5 {
			t.Fatalf("seed=%d: sector pair %v has %d inter-sector edges, want [3, 5] (GDD §6.1 constraint 4)", seed, pair, c)
		}
	}
	if adjacent != 3 {
		t.Fatalf("seed=%d: %d sector pairs are adjacent, want exactly 3 (D8: sector count fixed at four, connected via a spanning tree)", seed, adjacent)
	}
}

func verifyEdgeCount(t *testing.T, seed int, p Params, g Graph) {
	t.Helper()

	total := 0
	for _, n := range g.Nodes {
		total += len(n.Edges)
	}
	total /= 2

	if total < p.MinEdges || total > p.MaxEdges {
		t.Fatalf("seed=%d: graph has %d edges, want [%d, %d] (GDD §6.1)", seed, total, p.MinEdges, p.MaxEdges)
	}
}

func verifyGlobalConnectivity(t *testing.T, seed int, g Graph) {
	t.Helper()

	all := make([]game.NodeID, len(g.Nodes))
	for i := range g.Nodes {
		all[i] = game.NodeID(i)
	}
	if !inducedConnected(g, all) {
		t.Fatalf("seed=%d: graph is not connected (GDD §6.1 constraint 1)", seed)
	}
}

func verifyStartPositions(t *testing.T, seed int, p Params, g Graph) {
	t.Helper()

	if len(g.StartPositions) != p.Players {
		t.Fatalf("seed=%d: len(StartPositions) = %d, want %d", seed, len(g.StartPositions), p.Players)
	}
	for i := 1; i < len(g.StartPositions); i++ {
		if g.StartPositions[i-1] >= g.StartPositions[i] {
			t.Fatalf("seed=%d: StartPositions not strictly ascending: %v", seed, g.StartPositions)
		}
	}

	dist := bfsAll(g)
	for i, a := range g.StartPositions {
		for _, b := range g.StartPositions[i+1:] {
			if d := dist[a][b]; d < minStartDistance {
				t.Fatalf("seed=%d: start positions %d and %d are distance %d apart, want >= %d (GDD §6.1 constraint 5)", seed, a, b, d, minStartDistance)
			}
		}
	}
}

// bfsAll returns every node's distance to every other node, computed
// independently of builder.bfsDistances.
func bfsAll(g Graph) [][]int {
	n := len(g.Nodes)
	dist := make([][]int, n)
	for src := range n {
		d := make([]int, n)
		for i := range d {
			d[i] = -1
		}
		d[src] = 0
		queue := []game.NodeID{game.NodeID(src)}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, next := range g.Nodes[cur].Edges {
				if d[next] == -1 {
					d[next] = d[cur] + 1
					queue = append(queue, next)
				}
			}
		}
		dist[src] = d
	}
	return dist
}

// TestGenerateDeterministic pins issue #59's other headline acceptance
// criterion: the same seed produces a byte-identical graph across runs.
func TestGenerateDeterministic(t *testing.T) {
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
				if !graphsIdentical(g1, g2) {
					t.Fatalf("seed=%d: two Generate() calls from the same seed produced different graphs", seed)
				}
			}
		})
	}
}

func graphsIdentical(a, b Graph) bool {
	if len(a.Nodes) != len(b.Nodes) || len(a.StartPositions) != len(b.StartPositions) {
		return false
	}
	for i := range a.Nodes {
		if a.Nodes[i].ID != b.Nodes[i].ID || a.Nodes[i].Sector != b.Nodes[i].Sector {
			return false
		}
		if a.Nodes[i].Type != b.Nodes[i].Type || a.Nodes[i].X != b.Nodes[i].X || a.Nodes[i].Y != b.Nodes[i].Y {
			return false
		}
		if len(a.Nodes[i].Edges) != len(b.Nodes[i].Edges) {
			return false
		}
		for j := range a.Nodes[i].Edges {
			if a.Nodes[i].Edges[j] != b.Nodes[i].Edges[j] {
				return false
			}
		}
	}
	for i := range a.StartPositions {
		if a.StartPositions[i] != b.StartPositions[i] {
			return false
		}
	}
	return true
}

// TestGenerateRejectsInvalidParams guards Params.validate()'s fail-closed
// behavior — a caller error, not a generation failure, never counts against
// MaxAttempts.
func TestGenerateRejectsInvalidParams(t *testing.T) {
	base := Params{Nodes: 15, MinEdges: 21, MaxEdges: 23, Players: 2, MaxAttempts: 100, OpeningMinDistance: 3, OpeningMaxDistance: 4}

	cases := map[string]Params{
		"too few nodes":        withNodes(base, 11),
		"min > max edges":      withEdges(base, 23, 21),
		"zero edges":           withEdges(base, 0, 0),
		"zero players":         withPlayers(base, 0),
		"zero max attempts":    withMaxAttempts(base, 0),
		"unset opening band":   withOpeningDistances(base, 0, 0),
		"min > max opening":    withOpeningDistances(base, 4, 3),
		"zero opening minimum": withOpeningDistances(base, 0, 4),
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Generate(newTestRand(0), p); err == nil {
				t.Fatal("Generate() = nil error, want one")
			}
		})
	}
}

func withNodes(p Params, n int) Params       { p.Nodes = n; return p }
func withEdges(p Params, lo, hi int) Params  { p.MinEdges, p.MaxEdges = lo, hi; return p }
func withPlayers(p Params, n int) Params     { p.Players = n; return p }
func withMaxAttempts(p Params, n int) Params { p.MaxAttempts = n; return p }
func withOpeningDistances(p Params, lo, hi int) Params {
	p.OpeningMinDistance, p.OpeningMaxDistance = lo, hi
	return p
}

// TestGenerateExhaustsAndNamesFailedConstraint is issue #59's acceptance
// criterion: "Exhausted retries return an error naming the constraint that
// failed most often, not a partial graph." Players=28 on a 28-node graph
// can never place 28 mutually->=4-apart starting positions — constraint 5
// alone is unsatisfiable on any connected graph with minimum degree 2,
// regardless of topology — while the graph's own structure (GDD §6.1's
// real 28-node, 41-45-edge table row) succeeds often enough that most
// attempts reach and fail constraint 5 specifically, rather than an earlier
// structural check.
func TestGenerateExhaustsAndNamesFailedConstraint(t *testing.T) {
	p := Params{Nodes: 28, MinEdges: 41, MaxEdges: 45, Players: 28, MaxAttempts: 50, OpeningMinDistance: 3, OpeningMaxDistance: 4}

	g, err := Generate(newTestRand(0), p)
	if err == nil {
		t.Fatalf("Generate() = %+v, nil error; want ExhaustedError", g)
	}

	var exhausted *ExhaustedError
	if !asExhausted(err, &exhausted) {
		t.Fatalf("Generate() error = %v (%T), want *ExhaustedError", err, err)
	}
	if exhausted.Attempts != p.MaxAttempts {
		t.Errorf("Attempts = %d, want %d", exhausted.Attempts, p.MaxAttempts)
	}
	if exhausted.MostFailed != constraintStartPlacement {
		t.Errorf("MostFailed = %q, want %q — Failures: %v", exhausted.MostFailed, constraintStartPlacement, exhausted.Failures)
	}
	if exhausted.Failures[constraintStartPlacement] == 0 {
		t.Errorf("Failures[%q] = 0, want > 0", constraintStartPlacement)
	}
}

func asExhausted(err error, target **ExhaustedError) bool {
	e, ok := err.(*ExhaustedError)
	if ok {
		*target = e
	}
	return ok
}
