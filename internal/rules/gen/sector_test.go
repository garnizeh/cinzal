package gen

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

func TestSectorSizes(t *testing.T) {
	cases := map[int][4]int{
		12: {3, 3, 3, 3},
		13: {4, 3, 3, 3},
		14: {4, 4, 3, 3},
		15: {4, 4, 4, 3},
		16: {4, 4, 4, 4},
		22: {6, 6, 5, 5},
		25: {7, 6, 6, 6},
		28: {7, 7, 7, 7},
	}
	for n, want := range cases {
		got := sectorSizes(n)
		if got != want {
			t.Errorf("sectorSizes(%d) = %v, want %v", n, got, want)
		}
		sum := got[0] + got[1] + got[2] + got[3]
		if sum != n {
			t.Errorf("sectorSizes(%d) sums to %d, want %d", n, sum, n)
		}
		for i, size := range got {
			if size < 3 || size > 8 {
				t.Errorf("sectorSizes(%d)[%d] = %d, out of D8's [3, 8] range", n, i, size)
			}
		}
	}
}

func TestAssignSectorsDrawCountAndSizes(t *testing.T) {
	rand, count := countingRand(0)
	n := 15
	sizes := sectorSizes(n)

	sector := assignSectors(rand, n, sizes)

	if *count != n-1 {
		t.Fatalf("assignSectors consumed %d draws, want %d (Nodes-1)", *count, n-1)
	}
	if len(sector) != n {
		t.Fatalf("assignSectors returned %d entries, want %d", len(sector), n)
	}

	got := map[game.Sector]int{}
	for _, s := range sector {
		got[s]++
	}
	for i, s := range sectorOrder {
		if got[s] != sizes[i] {
			t.Errorf("sector %s has %d nodes, want %d (sectorSizes)", s, got[s], sizes[i])
		}
	}
}

func TestNewSectorPairNormalizesOrder(t *testing.T) {
	p1 := newSectorPair(game.SectorNorthVale, game.SectorOldDocks)
	p2 := newSectorPair(game.SectorOldDocks, game.SectorNorthVale)

	if p1 != p2 {
		t.Fatalf("newSectorPair(NorthVale, OldDocks) = %v, newSectorPair(OldDocks, NorthVale) = %v — want equal regardless of argument order", p1, p2)
	}
	if p1.a != game.SectorOldDocks || p1.b != game.SectorNorthVale {
		t.Fatalf("newSectorPair did not normalize to (a < b): got %v", p1)
	}
}

// TestSectorAdjacencyShape pins the structural guarantees sectorAdjacency
// promises regardless of seed: exactly 3 pairs (D8's four-sector spanning
// tree), sorted ascending, every pair a genuine two-sector pair (never a
// sector paired with itself), and exactly 5 draws consumed.
func TestSectorAdjacencyShape(t *testing.T) {
	for seed := range 50 {
		rand, count := countingRand(seed)
		pairs := sectorAdjacency(rand)

		if *count != 5 {
			t.Fatalf("seed=%d: sectorAdjacency consumed %d draws, want exactly 5 (D8 fixes sector count at four)", seed, *count)
		}
		if len(pairs) != 3 {
			t.Fatalf("seed=%d: sectorAdjacency returned %d pairs, want exactly 3", seed, len(pairs))
		}
		for i, p := range pairs {
			if p.a == p.b {
				t.Fatalf("seed=%d: pair %d is a sector paired with itself: %v", seed, i, p)
			}
			if i > 0 {
				prev := pairs[i-1]
				if p.a < prev.a || (p.a == prev.a && p.b < prev.b) {
					t.Fatalf("seed=%d: pairs not sorted ascending: %v then %v", seed, prev, p)
				}
			}
		}

		// The 3 pairs must form a connected meta-graph over all 4 sectors
		// (sectorAdjacency is built from a spanning tree), so every sector
		// must appear in at least one pair.
		seen := map[game.Sector]bool{}
		for _, p := range pairs {
			seen[p.a], seen[p.b] = true, true
		}
		if len(seen) != 4 {
			t.Fatalf("seed=%d: pairs %v touch %d distinct sectors, want all 4", seed, pairs, len(seen))
		}
	}
}
