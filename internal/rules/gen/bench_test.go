package gen

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// benchCases is a fixed snapshot of propertyCases()'s shape — GDD §6.1's
// four per-player-count rows plus D8's two scenario sizes — pinned to
// literal Params rather than read live from game.DefaultConfig(). Every
// benchmark below reuses one fixed topology per case across every run (see
// benchTopology), built once from Generate(seed=0); AssignNodeTypes'
// per-topology retry count varies hugely by design (typeAssignMaxTries'
// own doc comment: 1-66 tries, measured across ~9000 topologies) and is not
// a function of the algorithm's speed, only of which specific graph got
// generated. A MapByPlayers edit (like issue #229's 4-player raise) changes
// propertyCases()'s Nodes/edges, which would silently swap in a different
// seed=0 topology and make bench-compare's base/candidate comparison read
// as a "regression" or "improvement" that is really just two different
// graphs needing different retry counts — found the hard way on #229's own
// PR. Pinning these values here, independent of game.DefaultConfig(), means
// a future MapByPlayers edit does not move this file's own benchmark
// baseline; only a deliberate edit to this table does, and that edit is
// then visible in the same diff as whatever changed the retry-sensitive
// code being measured. propertyCases() itself stays wired to
// game.DefaultConfig() — the property tests it feeds run 1000 seeds per
// case and check correctness (does a satisfying graph exist), not one
// topology's timing, so they are not exposed to this failure mode and
// should keep tracking the shipped table automatically.
func benchCases() []propertyCase {
	return []propertyCase{
		{name: "players=2", params: Params{Nodes: 15, MinEdges: 21, MaxEdges: 23, Players: 2, MaxAttempts: 1000, OpeningMinDistance: 3, OpeningMaxDistance: 4}, seeds: 1000},
		{name: "players=3", params: Params{Nodes: 22, MinEdges: 31, MaxEdges: 35, Players: 3, MaxAttempts: 1000, OpeningMinDistance: 3, OpeningMaxDistance: 4}, seeds: 1000},
		{name: "players=4", params: Params{Nodes: 28, MinEdges: 41, MaxEdges: 45, Players: 4, MaxAttempts: 1000, OpeningMinDistance: 3, OpeningMaxDistance: 4}, seeds: 1000},
		{name: "players=5", params: Params{Nodes: 28, MinEdges: 40, MaxEdges: 45, Players: 5, MaxAttempts: 1000, OpeningMinDistance: 3, OpeningMaxDistance: 4}, seeds: 1000},
		{name: "D8-nodes=12", params: Params{Nodes: 12, MinEdges: 17, MaxEdges: 19, Players: 2, MaxAttempts: 1000, OpeningMinDistance: 3, OpeningMaxDistance: 4}, seeds: 1000},
		{name: "D8-nodes=16", params: Params{Nodes: 16, MinEdges: 23, MaxEdges: 25, Players: 2, MaxAttempts: 1000, OpeningMinDistance: 3, OpeningMaxDistance: 4}, seeds: 1000},
	}
}

// BenchmarkGenerate is issue #112's headline benchmark: the full Generate()
// pipeline, parameterised over every benchCases() entry — 2/3/4/5 players
// at GDD §6.1's own node counts (pinned, see benchCases), plus the two D8
// scenario sizes (12, 16). This is the number a regression would actually
// be felt as ("CI feels sluggish"), so it stays the primary signal;
// BenchmarkAssignNodeTypes and BenchmarkComputeLayout below exist to
// attribute a regression here to a specific phase rather than only to
// Generate() as a whole.
//
// Each b.Loop() iteration advances to a fresh seed rather than reusing one:
// the known cost driver here is retries (30-80 full regenerations per seed
// to satisfy constraints 5+7 on the 28-node/5-player map), and that count is
// seed-dependent, so averaging over many seeds is what makes this number
// representative rather than an artifact of one lucky or unlucky draw.
func BenchmarkGenerate(b *testing.B) {
	for _, tc := range benchCases() {
		b.Run(tc.name, func(b *testing.B) {
			seed := 0
			for b.Loop() {
				if _, err := Generate(newTestRand(seed), tc.params); err != nil {
					b.Fatalf("seed=%d: Generate() = %v", seed, err)
				}
				seed++
			}
		})
	}
}

// benchTopology builds one constraint-satisfying graph via Generate (seed 0)
// and reconstructs a *builder around its adjacency and sector assignment.
// BenchmarkAssignNodeTypes and BenchmarkComputeLayout use it so the timed
// loop measures only the phase under test — never Generate's own retry cost
// — against a topology that is realistic (it is one Generate actually
// produced) rather than a synthetic fixture.
func benchTopology(tb testing.TB, tc propertyCase) *builder {
	tb.Helper()

	g, err := Generate(newTestRand(0), tc.params)
	if err != nil {
		tb.Fatalf("Generate() = %v", err)
	}

	sector := make([]game.Sector, len(g.Nodes))
	for i, n := range g.Nodes {
		sector[i] = n.Sector
	}
	bld := newBuilder(len(g.Nodes), sector)
	for _, n := range g.Nodes {
		for _, e := range n.Edges {
			bld.connect(n.ID, e)
		}
	}
	return bld
}

// BenchmarkAssignNodeTypes isolates #60's most retry-sensitive internal: up
// to typeAssignMaxTries greedy walks against one fixed, already-valid
// topology (built once per case, outside the timed loop). rand is shared
// across b.Loop() iterations rather than reset each time, so each call
// continues drawing from the same stream — the same threading discipline
// Generate itself uses — rather than replaying one identical walk N times.
func BenchmarkAssignNodeTypes(b *testing.B) {
	for _, tc := range benchCases() {
		b.Run(tc.name, func(b *testing.B) {
			bld := benchTopology(b, tc)
			rand := newTestRand(1)
			for b.Loop() {
				if _, ok := assignNodeTypes(rand, bld); !ok {
					b.Fatalf("assignNodeTypes exhausted %d tries on a structurally valid graph", typeAssignMaxTries)
				}
			}
		})
	}
}

// BenchmarkComputeLayout isolates D10's layout pass. Unlike assignNodeTypes,
// computeLayout's cost is fixed at exactly one draw per node — no rejection
// loop — so this benchmark's job is to catch a regression that adds one
// (e.g. a lattice-collision retry), not to characterise existing variance.
func BenchmarkComputeLayout(b *testing.B) {
	for _, tc := range benchCases() {
		b.Run(tc.name, func(b *testing.B) {
			bld := benchTopology(b, tc)
			rand := newTestRand(1)
			for b.Loop() {
				computeLayout(rand, bld)
			}
		})
	}
}
