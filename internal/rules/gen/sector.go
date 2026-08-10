package gen

import (
	"sort"

	"github.com/garnizeh/cinzal/internal/game"
)

// sectorOrder is D8/D10's fixed sector declaration order — the same
// "declared order, not iteration order" tie-break D3 and D9 already
// establish for their own fixed orderings.
var sectorOrder = [4]game.Sector{
	game.SectorOldDocks,
	game.SectorIronLow,
	game.SectorMistHeights,
	game.SectorNorthVale,
}

// sectorSizes splits n nodes into exactly four sector sizes, each within D8's
// [3, 8] range: base = n/4 nodes each, the n%4 remainder handed out one at a
// time in sectorOrder — the same largest-remainder-flavored, fixed-tie-break
// shape D9 already established for node types
// (docs/decisions/D09-node-type-rounding.md). Deterministic: no RNG draw, so
// two different seeds at the same node count always agree on sizes and only
// disagree on which physical nodes fill them (assignSectors). Every
// currently supported node count (12, 15, 16, 20, 22, 25, 28) produces sizes
// within [3, 8]; Params.validate's minSupportedNodes floor is what keeps
// that true for any future caller.
func sectorSizes(n int) [4]int {
	base := n / 4
	rem := n % 4

	var sizes [4]int
	for i := range sizes {
		sizes[i] = base
		if i < rem {
			sizes[i]++
		}
	}
	return sizes
}

// assignSectors performs GDD §6.1's node-to-sector assignment: a full
// Fisher-Yates shuffle of every NodeID (fullShuffle, PurposeSectorAssign),
// then sliced sequentially into the four sectors per sizes, in sectorOrder.
// Cost: exactly n-1 draws, always — deterministic in count, not in outcome.
func assignSectors(rand Rand, n int, sizes [4]int) []game.Sector {
	order := make([]game.NodeID, n)
	for i := range order {
		order[i] = game.NodeID(i)
	}
	fullShuffle(rand, PurposeSectorAssign, order)

	sector := make([]game.Sector, n)
	idx := 0
	for s, size := range sizes {
		for range size {
			sector[order[idx]] = sectorOrder[s]
			idx++
		}
	}
	return sector
}

// sectorPair is one unordered pair of distinct sectors, always stored with
// A < B so two pairs describing the same two sectors compare equal and sort
// identically regardless of discovery order.
type sectorPair struct{ a, b game.Sector }

func newSectorPair(x, y game.Sector) sectorPair {
	if x > y {
		x, y = y, x
	}
	return sectorPair{a: x, b: y}
}

// sectorAdjacency chooses which three of the six possible sector pairs
// carry GDD §6.1 constraint 4's chokepoint edges: a random spanning tree
// over the four sectors (randomSpanningTree, PurposeAdjacency), which both
// guarantees the sector-level "meta-graph" is connected — the property that
// makes constraint 1 (the whole graph connected) follow from constraint 3
// (each sector internally connected) without a separate global check — and
// keeps the inter-sector edge budget to the minimum GDD §6.1 constraint 4
// can produce (3 pairs, not up to 6), which D8's own follow-up flags as the
// difference between a satisfiable and an unsatisfiable chokepoint budget at
// the tightest node counts. Cost is fixed at 5 draws, always: D8 fixes
// sector count at exactly four, so this never varies with Nodes or the
// seed's later choices. Returned pairs are sorted ascending by (a, b) so
// every later phase processes them in a fixed, seed-independent order.
func sectorAdjacency(rand Rand) []sectorPair {
	var pairs []sectorPair
	randomSpanningTree(rand, PurposeAdjacency, sectorOrder[:], func(parent, child game.Sector) {
		pairs = append(pairs, newSectorPair(parent, child))
	})

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].a != pairs[j].a {
			return pairs[i].a < pairs[j].a
		}
		return pairs[i].b < pairs[j].b
	})
	return pairs
}

// Touched only to verify CodeRabbit review now reaches this package (see #108, #109).
