package gen

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// minStartDistance is GDD §6.1 constraint 5: "Minimum graph distance of 4
// between any two starting positions."
const minStartDistance = 4

// maxStartWarehouseDistance is GDD §6.1 constraint 7: "Every starting node
// has at least one Warehouse within 2 steps that itself has a Border at a
// distance inside Tier I's contract band (3-4 steps, §8.3)." GDD §8.1
// explains why: at setup a player Knows only their start and the Warehouses
// within 2 steps of it (D23), so any Warehouse further away could not
// originate a contract without exploring first — and a Warehouse with no
// Border at a contractable distance cannot originate one either, which is
// the half of the guarantee D24 added. This constant is the origin half; the
// destination half's band arrives on Params, from GDD §8.3's own table,
// because gen must not import game.Config (D24).
const maxStartWarehouseDistance = 2

// selectStartPositions chooses players nodes satisfying both GDD §6.1
// constraint 5 (pairwise graph distance >= minStartDistance) and constraint
// 7 (a contractable Warehouse within maxStartWarehouseDistance steps):
// every node is shuffled once (fullShuffle, PurposeStartSelect, n-1 draws,
// always), then the shuffled order is walked greedily, accepting a candidate
// only if it has a nearby contractable Warehouse and its graph distance to
// every already-accepted node is at least minStartDistance, until players
// are accepted or the order is exhausted. The second return is false if
// fewer than players nodes could be found — the caller treats that as this
// attempt's constraint-5/7 failure, not an error, and retries a fresh
// attempt rather than redrawing within this one.
//
// Which nodes satisfy constraint 7 is computed once per attempt, before the
// walk, not once per candidate: the answer depends only on the graph and its
// node types, both fixed by the time this runs, so the per-candidate shape
// would repeat the same BFS work for every node the walk touches (D24
// measured the two shapes at +22-40% and +6-13% on BenchmarkGenerate). It
// draws nothing, so it cannot move where the shuffle's draws land, and the
// shuffle runs unconditionally even when no node qualifies at all —
// PurposeStartSelect consumes exactly Nodes-1 whether this attempt succeeds
// or fails (RFC-001 §6.4).
//
// This is the one selection in the package whose RNG cost does not bound
// its own success: a shuffle-once-then-greedily-walk approach never redraws
// (keeping the cost a pure function of Nodes, matching every other draw in
// this package), but it also is not the only possible greedy walk that
// could find a valid set from the same shuffle — it is simply the one this
// package commits to, so two implementations of this rule can never
// desynchronise against each other (RFC-001 §6.4).
func selectStartPositions(rand Rand, b *builder, players int, types []game.NodeType, openingMin, openingMax int) ([]game.NodeID, bool) {
	qualified := qualifyingStarts(b, types, openingMin, openingMax)

	order := b.nodesSlice()
	fullShuffle(rand, PurposeStartSelect, order)

	var selected []game.NodeID
	for _, candidate := range order {
		if len(selected) >= players {
			break
		}
		if !qualified[candidate] {
			continue
		}
		if farEnoughFromAll(b.bfsDistances(candidate), selected) {
			selected = append(selected, candidate)
		}
	}

	if len(selected) < players {
		return nil, false
	}

	slices.Sort(selected)
	return selected, true
}

// qualifyingStarts reports, per node, whether that node satisfies GDD §6.1
// constraint 7 as D24 strengthened it: some Warehouse within
// maxStartWarehouseDistance steps of it has a Border at a graph distance
// inside [openingMin, openingMax], Tier I's band (GDD §8.3, arriving via
// Params — see D24).
//
// It is written from the Warehouses outward rather than from the candidates
// in, which is the same set — graph distance is symmetric, so "a contractable
// Warehouse within 2 steps of the candidate" and "the candidate within 2
// steps of a contractable Warehouse" are one condition — reached with one
// BFS per Warehouse, ~24% of nodes (GDD §6.2), instead of one per candidate.
// The candidate-side scan is then a slice index, and the walk in
// selectStartPositions needs no BFS at all for a node it is going to reject.
func qualifyingStarts(b *builder, types []game.NodeType, openingMin, openingMax int) []bool {
	qualified := make([]bool, b.n)
	for w := range b.n {
		if types[w] != game.NodeWarehouse {
			continue
		}
		dist := b.bfsDistances(game.NodeID(w))
		if !reachesBorderInBand(dist, types, openingMin, openingMax) {
			continue
		}
		for node, d := range dist {
			if d >= 0 && d <= maxStartWarehouseDistance {
				qualified[node] = true
			}
		}
	}
	return qualified
}

// reachesBorderInBand reports whether dist (one Warehouse's distances to
// every other node) reaches a Border inside [openingMin, openingMax] — what
// makes that Warehouse able to originate an opening contract at all, and so
// the half of constraint 7 the pre-D24 predicate never checked. An
// unreachable node carries -1, which openingMin >= 1 (Params.validate)
// keeps out of the band on its own.
func reachesBorderInBand(dist []int, types []game.NodeType, openingMin, openingMax int) bool {
	for node, d := range dist {
		if d >= openingMin && d <= openingMax && types[node] == game.NodeBorder {
			return true
		}
	}
	return false
}

// farEnoughFromAll reports whether dist (a candidate's own distances to
// every other node) places it at least minStartDistance from every node
// already in selected (GDD §6.1 constraint 5).
func farEnoughFromAll(dist []int, selected []game.NodeID) bool {
	for _, s := range selected {
		if d := dist[s]; d == -1 || d < minStartDistance {
			return false
		}
	}
	return true
}
