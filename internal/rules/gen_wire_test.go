package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules/gen"
)

// TestGenPurposeStringsMatchDeclaredConstants guards against the two
// packages' purpose strings drifting apart. rules.Purpose's gen* constants
// are declared as literals, not as a conversion of gen's own constants
// (TestPurposeTableMatchesDeclaredConstants only extracts *ast.BasicLit
// string values, so a conversion expression would silently escape that
// test's coverage) — this test is what actually enforces the two files stay
// in lockstep.
func TestGenPurposeStringsMatchDeclaredConstants(t *testing.T) {
	pairs := map[Purpose]string{
		PurposeGenSectorAssign:     gen.PurposeSectorAssign,
		PurposeGenSectorTree:       gen.PurposeSectorTree,
		PurposeGenAdjacency:        gen.PurposeAdjacency,
		PurposeGenChokepointCount:  gen.PurposeChokepointCount,
		PurposeGenChokepointSelect: gen.PurposeChokepointSelect,
		PurposeGenEdgeCount:        gen.PurposeEdgeCount,
		PurposeGenFillEdge:         gen.PurposeFillEdge,
		PurposeGenStartSelect:      gen.PurposeStartSelect,
	}
	for rulesPurpose, genPurpose := range pairs {
		if string(rulesPurpose) != genPurpose {
			t.Errorf("rules.%s = %q, gen constant = %q — the two packages' purpose strings have drifted apart", rulesPurpose, rulesPurpose, genPurpose)
		}
	}
}

// genRand adapts an *RNG into a gen.Rand closure, the shape #59's
// architecture note (internal/rules/gen/doc.go) requires: gen must not
// import internal/rules, so the caller supplies the seeded draw as a plain
// function value instead of the concrete *RNG type.
func genRand(rng *RNG) gen.Rand {
	return func(purpose string, n int) int {
		return rng.Next(Purpose(purpose), n)
	}
}

func testGenParams(players int) gen.Params {
	spec := game.DefaultConfig().MapByPlayers[players]
	return gen.Params{
		Nodes:       spec.Nodes,
		MinEdges:    spec.MinEdges,
		MaxEdges:    spec.MaxEdges,
		Players:     players,
		MaxAttempts: game.DefaultConfig().MaxGenAttempts,
	}
}

// TestGenerateWiredToRealRNGIsDeterministic is the end-to-end check behind
// issue #59's acceptance criterion "the same seed produces a byte-identical
// graph across runs": two *RNG instances built from the same seed and round,
// each driving a fresh gen.Generate call, must produce identical graphs.
func TestGenerateWiredToRealRNGIsDeterministic(t *testing.T) {
	var seed [32]byte
	copy(seed[:], "issue-59-determinism-seed------")

	for _, players := range []int{2, 3, 4, 5} {
		params := testGenParams(players)

		rng1 := NewRNG(seed, 0)
		g1, err := gen.Generate(genRand(rng1), params)
		if err != nil {
			t.Fatalf("players=%d: first Generate() = %v", players, err)
		}

		rng2 := NewRNG(seed, 0)
		g2, err := gen.Generate(genRand(rng2), params)
		if err != nil {
			t.Fatalf("players=%d: second Generate() = %v", players, err)
		}

		if !graphsEqual(g1, g2) {
			t.Fatalf("players=%d: two RNGs built from the same seed produced different graphs", players)
		}
		if rng1.Seq() != rng2.Seq() {
			t.Fatalf("players=%d: rng1.Seq()=%d != rng2.Seq()=%d for the same seed", players, rng1.Seq(), rng2.Seq())
		}
	}
}

func graphsEqual(a, b gen.Graph) bool {
	if len(a.Nodes) != len(b.Nodes) || len(a.StartPositions) != len(b.StartPositions) {
		return false
	}
	for i := range a.Nodes {
		if a.Nodes[i].ID != b.Nodes[i].ID || a.Nodes[i].Sector != b.Nodes[i].Sector {
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

// TestGenerateWiredToRealRNGRecordsConsumption is issue #59's other
// end-to-end acceptance criterion: index consumption per generation attempt
// is recorded and asserted against the #41/D03 table. It does not
// re-derive an independent formula (edges.go's own comments already state
// each purpose's exact or bounding cost); it asserts that every purpose
// gen.Generate can consume actually shows up in *RNG's own ledger, which is
// what RFC-001 §16.2's invariant ("rng.seq consumed == predicted, per the
// §6.4 table, asserted per round") reads from at every other call site in
// this package.
func TestGenerateWiredToRealRNGRecordsConsumption(t *testing.T) {
	var seed [32]byte
	copy(seed[:], "issue-59-consumption-seed------")

	params := testGenParams(4)
	rng := NewRNG(seed, 0)
	if _, err := gen.Generate(genRand(rng), params); err != nil {
		t.Fatalf("Generate() = %v", err)
	}

	// *RNG.Consumed is cumulative across every attempt Generate made before
	// it succeeded (rejected attempts' draws are never rolled back — see
	// generate.go), and PurposeGenStartSelect only fires on an attempt that
	// already cleared every structural check (constraints 1-4), which can
	// be fewer attempts than PurposeGenSectorAssign saw (constraints 1-4
	// can reject an attempt before it ever reaches start selection). So
	// each purpose's total is checked against its own known per-attempt
	// cost as a multiple, not against one shared attempt count.
	assertMultiple(t, rng, PurposeGenSectorAssign, params.Nodes-1, "Nodes-1")
	assertMultiple(t, rng, PurposeGenStartSelect, params.Nodes-1, "Nodes-1")

	// PurposeGenAdjacency and PurposeGenChokepointCount are fixed by D8's
	// four-sector guarantee: 5 draws and 3 draws per attempt, always,
	// regardless of Nodes or the seed.
	assertMultiple(t, rng, PurposeGenAdjacency, 5, "5 (D8 fixes sector count at four)")
	assertMultiple(t, rng, PurposeGenChokepointCount, 3, "3 (one draw per adjacent sector pair)")

	// Every other gen purpose is data-dependent (candidate pool sizes,
	// which vary with the sector assignment this attempt drew) but must
	// still have fired at least once.
	for _, p := range []Purpose{PurposeGenSectorTree, PurposeGenChokepointSelect, PurposeGenEdgeCount, PurposeGenFillEdge} {
		if rng.Consumed(p) == 0 {
			t.Errorf("Consumed(%s) = 0, want > 0 — this purpose should have drawn at least once", p)
		}
	}
}

// assertMultiple checks that purpose's total consumption is a positive
// multiple of perAttempt — the shape every fixed-per-attempt gen purpose
// takes once rejected attempts' draws are folded in (see the comment on
// TestGenerateWiredToRealRNGRecordsConsumption).
func assertMultiple(t *testing.T, rng *RNG, purpose Purpose, perAttempt int, label string) {
	t.Helper()
	got := rng.Consumed(purpose)
	if got == 0 || got%perAttempt != 0 {
		t.Errorf("Consumed(%s) = %d, want a positive multiple of %s (%d)", purpose, got, label, perAttempt)
	}
}
