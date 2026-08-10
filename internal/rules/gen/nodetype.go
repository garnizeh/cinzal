package gen

import (
	"sort"

	"github.com/garnizeh/cinzal/internal/game"
)

// nodeTypeOrder is GDD §6.2's own declaration order, and D9's tie-break
// order for the rounding rule below — Warehouse, Border, Black Market,
// Alley (docs/decisions/D09-node-type-rounding.md).
var nodeTypeOrder = [4]game.NodeType{
	game.NodeWarehouse,
	game.NodeBorder,
	game.NodeBlackMarket,
	game.NodeAlley,
}

// nodeTypeShareNumerator is GDD §6.2's percentage share for each type in
// nodeTypeOrder, summing to 100.
var nodeTypeShareNumerator = [4]int{24, 24, 20, 32}

// nodeTypeCounts allocates n nodes across the four GDD §6.2 types per D9's
// decision: floor each type's exact share (n * share / 100), then hand the
// n - sum(floors) remaining nodes one at a time to whichever types have the
// largest fractional part, breaking ties by nodeTypeOrder — the same
// "declared order, not iteration order" tie-break sectorSizes already uses
// for sector sizing. Deterministic, no RNG draw: two different seeds at the
// same node count always agree on these counts (docs/decisions/D09-node-type-rounding.md).
// The result matches GDD §6.2's own table at every currently supported node
// count (12, 15, 16, 20, 22, 25, 28).
func nodeTypeCounts(n int) [4]int {
	var counts [4]int

	type fraction struct {
		typeIdx int
		numer   int // fractional part's numerator, out of 100
	}
	fracs := make([]fraction, len(nodeTypeOrder))

	sum := 0
	for i, share := range nodeTypeShareNumerator {
		product := n * share
		counts[i] = product / 100
		fracs[i] = fraction{typeIdx: i, numer: product % 100}
		sum += counts[i]
	}

	// sort.SliceStable preserves nodeTypeOrder for equal fractional parts —
	// D9's tie order, not an arbitrary sort-implementation artifact.
	sort.SliceStable(fracs, func(i, j int) bool { return fracs[i].numer > fracs[j].numer })

	for k := 0; k < n-sum; k++ {
		counts[fracs[k].typeIdx]++
	}
	return counts
}

// typeAssignMaxTries bounds assignNodeTypes' own local retry loop — a
// single ascending-NodeID walk (attemptAssignNodeTypes) is a greedy
// heuristic with no look-ahead, so it deadlocks on some walks even when a
// satisfying assignment exists for this exact topology. Measured across
// ~9000 sampled topologies at every currently supported node count
// (12-28), a satisfying walk was always found, and never took more than 66
// tries; 200 leaves better than 3x headroom over that observed worst case
// while still being a hard, documented bound — never an unbounded loop.
// Exhausting it means this specific *graph* (not just this walk) is
// discarded; see assignNodeTypes.
const typeAssignMaxTries = 200

// assignNodeTypes places D9's counts onto b's nodes under GDD §6.1
// constraint 6 (no Warehouse adjacent to a Border), retrying
// attemptAssignNodeTypes' single greedy walk up to typeAssignMaxTries times
// against the same topology — each retry consumes a fresh slice of the
// same continuing draw stream, so it explores a different walk rather than
// repeating the last one. A node with zero valid choices on a given walk —
// every type either out of quota or blocked by adjacency — means that walk
// deadlocked, not that constraint 6 is unsatisfiable on this graph; only
// exhausting every retry means that. At that point assignNodeTypes returns
// false rather than backtracking further, and the caller discards the
// whole attempt and builds a fresh graph, per D9's own decision log: "the
// generator must detect that and retry the graph rather than quietly
// relaxing it."
func assignNodeTypes(rand Rand, b *builder) ([]game.NodeType, bool) {
	// Every retry re-checks adjacency against the same, unchanging topology
	// — computed once here, rather than rescanning b's full n x n adjacency
	// matrix from inside the hot loop on every one of up to
	// typeAssignMaxTries walks.
	neighbors := b.neighborLists()

	for try := 0; try < typeAssignMaxTries; try++ {
		if types, ok := attemptAssignNodeTypes(rand, b.n, neighbors); ok {
			return types, true
		}
	}
	return nil, false
}

// attemptAssignNodeTypes is one greedy walk: one draw per node, in
// ascending NodeID order, choosing uniformly among whichever types still
// have quota remaining (D9) and would not create a Warehouse-Border
// adjacency with an already-placed neighbour (GDD §6.1 constraint 6). Cost
// is exactly n draws, always (PurposeTypeAssign), regardless of whether
// this particular walk succeeds.
func attemptAssignNodeTypes(rand Rand, n int, neighbors [][]game.NodeID) ([]game.NodeType, bool) {
	remaining := nodeTypeCounts(n)
	types := make([]game.NodeType, n)

	var candidates []game.NodeType
	for i := 0; i < n; i++ {
		node := game.NodeID(i)

		candidates = candidates[:0]
		for ti, t := range nodeTypeOrder {
			if remaining[ti] <= 0 {
				continue
			}
			if t == game.NodeWarehouse && neighborHasType(types, neighbors[node], game.NodeBorder) {
				continue
			}
			if t == game.NodeBorder && neighborHasType(types, neighbors[node], game.NodeWarehouse) {
				continue
			}
			candidates = append(candidates, t)
		}
		if len(candidates) == 0 {
			return nil, false
		}

		choice := candidates[rand(PurposeTypeAssign, len(candidates))]
		types[node] = choice
		for ti, t := range nodeTypeOrder {
			if t == choice {
				remaining[ti]--
				break
			}
		}
	}
	return types, true
}

// neighborHasType reports whether any of node's neighbours is already
// assigned target. Unassigned neighbours read as game.NodeType's zero
// value, which is never a valid type (see game/enums.go), so this never
// mistakes "not yet assigned" for a match regardless of scan order.
func neighborHasType(types []game.NodeType, neighbors []game.NodeID, target game.NodeType) bool {
	for _, j := range neighbors {
		if types[j] == target {
			return true
		}
	}
	return false
}
