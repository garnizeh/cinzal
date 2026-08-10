package gen

import (
	"slices"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestViolationsCleanGraph checks the negative case first: a graph built to
// satisfy every constraint reports no violations at all.
func TestViolationsCleanGraph(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks)
	b := newBuilder(4, sectors)
	b.connect(0, 1)
	b.connect(1, 2)
	b.connect(2, 3)
	b.connect(3, 0) // a 4-cycle: every node at degree 2, connected, one sector

	if got := b.violations(nil, 4, 4); got != nil {
		t.Fatalf("violations() on a clean 4-cycle = %v, want nil", got)
	}
}

func TestViolationsDegree(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks)
	b := newBuilder(3, sectors)
	b.connect(0, 1) // node 2 stays at degree 0 — below minDegree

	got := b.violations(nil, 1, 5)
	if !slices.Contains(got, constraintDegree) {
		t.Fatalf("violations() = %v, want it to contain %q", got, constraintDegree)
	}
}

func TestViolationsSectorConnected(t *testing.T) {
	// Sector OldDocks = {0, 1, 2}, but only 0-1 is connected — node 2 is
	// internally disconnected from its own sector, even though an edge to
	// another sector (added below) keeps the *global* graph connected.
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorIronLow)
	b := newBuilder(4, sectors)
	b.connect(0, 1)
	b.connect(1, 3)
	b.connect(2, 3) // node 2 only reaches the rest of the graph via IronLow

	got := b.violations(nil, 1, 5)
	if !slices.Contains(got, constraintSectorConnected) {
		t.Fatalf("violations() = %v, want it to contain %q", got, constraintSectorConnected)
	}
	if slices.Contains(got, constraintGraphConnected) {
		t.Fatalf("violations() = %v, want it NOT to contain %q — the whole graph is connected, only the sector isn't", got, constraintGraphConnected)
	}
}

func TestViolationsGraphConnected(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorIronLow, game.SectorIronLow)
	b := newBuilder(4, sectors)
	b.connect(0, 1)
	b.connect(2, 3)
	// Two disconnected components: {0,1} and {2,3}.

	got := b.violations(nil, 1, 5)
	if !slices.Contains(got, constraintGraphConnected) {
		t.Fatalf("violations() = %v, want it to contain %q", got, constraintGraphConnected)
	}
}

func TestViolationsChokepointRange(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks)
	b := newBuilder(2, sectors)
	b.connect(0, 1)

	cases := map[string][]int{
		"too few":  {2},
		"too many": {6},
	}
	for name, counts := range cases {
		t.Run(name, func(t *testing.T) {
			got := b.violations(counts, 1, 5)
			if !slices.Contains(got, constraintChokepointRange) {
				t.Errorf("violations(chokepointCounts=%v) = %v, want it to contain %q", counts, got, constraintChokepointRange)
			}
		})
	}

	if got := b.violations([]int{3, 4, 5}, 1, 5); slices.Contains(got, constraintChokepointRange) {
		t.Errorf("violations(chokepointCounts=[3,4,5]) = %v, want it NOT to contain %q — every count is in [3, 5]", got, constraintChokepointRange)
	}
}

func TestViolationsEdgeCount(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks)
	b := newBuilder(4, sectors)
	b.connect(0, 1)
	b.connect(1, 2)
	b.connect(2, 3)
	b.connect(3, 0) // 4 edges

	if got := b.violations(nil, 5, 10); !slices.Contains(got, constraintEdgeCount) {
		t.Fatalf("violations(minEdges=5) = %v, want it to contain %q (graph only has 4 edges)", got, constraintEdgeCount)
	}
	if got := b.violations(nil, 1, 3); !slices.Contains(got, constraintEdgeCount) {
		t.Fatalf("violations(maxEdges=3) = %v, want it to contain %q (graph has 4 edges)", got, constraintEdgeCount)
	}
	if got := b.violations(nil, 4, 4); slices.Contains(got, constraintEdgeCount) {
		t.Fatalf("violations(minEdges=maxEdges=4) = %v, want it NOT to contain %q", got, constraintEdgeCount)
	}
}

// TestViolationsReportsEveryFailureAtOnce is the ExhaustedError diagnostic's
// own load-bearing assumption: a single attempt failing more than one
// constraint must have all of them reported, not just the first check that
// happened to run.
func TestViolationsReportsEveryFailureAtOnce(t *testing.T) {
	sectors := testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorIronLow, game.SectorIronLow)
	b := newBuilder(4, sectors)
	// No edges at all: every node below minDegree, every sector
	// disconnected internally (each has 2 nodes, 0 edges), and the whole
	// graph is disconnected too.

	got := b.violations(nil, 1, 5)
	for _, want := range []string{constraintDegree, constraintSectorConnected, constraintGraphConnected} {
		if !slices.Contains(got, want) {
			t.Errorf("violations() = %v, want it to also contain %q", got, want)
		}
	}
}
