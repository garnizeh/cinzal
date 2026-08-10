package gen

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// minStartDistance is GDD §6.1 constraint 5: "Minimum graph distance of 4
// between any two starting positions."
const minStartDistance = 4

// maxStartWarehouseDistance is GDD §6.1 constraint 7: "Every starting node
// has at least one Warehouse within 2 steps." GDD §8.1 explains why: at
// setup a player Knows only their start and its neighbours, so any
// Warehouse further away could not be reached without exploring first —
// and the opening contract offer needs a Known origin to exist at all.
const maxStartWarehouseDistance = 2

// selectStartPositions chooses players nodes satisfying both GDD §6.1
// constraint 5 (pairwise graph distance >= minStartDistance) and constraint
// 7 (a Warehouse within maxStartWarehouseDistance steps): every node is
// shuffled once (fullShuffle, PurposeStartSelect, n-1 draws, always), then
// the shuffled order is walked greedily, accepting a candidate only if it
// has a nearby Warehouse and its graph distance to every already-accepted
// node is at least minStartDistance, until players are accepted or the
// order is exhausted. The second return is false if fewer than players
// nodes could be found — the caller treats that as this attempt's
// constraint-5/7 failure, not an error, and retries a fresh attempt rather
// than redrawing within this one.
//
// This is the one selection in the package whose RNG cost does not bound
// its own success: a shuffle-once-then-greedily-walk approach never redraws
// (keeping the cost a pure function of Nodes, matching every other draw in
// this package), but it also is not the only possible greedy walk that
// could find a valid set from the same shuffle — it is simply the one this
// package commits to, so two implementations of this rule can never
// desynchronise against each other (RFC-001 §6.4).
func selectStartPositions(rand Rand, b *builder, players int, types []game.NodeType) ([]game.NodeID, bool) {
	order := b.nodesSlice()
	fullShuffle(rand, PurposeStartSelect, order)

	var selected []game.NodeID
	for _, candidate := range order {
		if len(selected) >= players {
			break
		}
		dist := b.bfsDistances(candidate)
		if !hasNearbyWarehouse(dist, types) {
			continue
		}
		if farEnoughFromAll(dist, selected) {
			selected = append(selected, candidate)
		}
	}

	if len(selected) < players {
		return nil, false
	}

	slices.Sort(selected)
	return selected, true
}

// hasNearbyWarehouse reports whether dist (a candidate's own distances to
// every other node) reaches a Warehouse within maxStartWarehouseDistance
// steps (GDD §6.1 constraint 7).
func hasNearbyWarehouse(dist []int, types []game.NodeType) bool {
	for node, d := range dist {
		if d >= 0 && d <= maxStartWarehouseDistance && types[node] == game.NodeWarehouse {
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
