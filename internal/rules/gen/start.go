package gen

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// minStartDistance is GDD §6.1 constraint 5: "Minimum graph distance of 4
// between any two starting positions."
const minStartDistance = 4

// selectStartPositions chooses players nodes pairwise at least
// minStartDistance apart (GDD §6.1 constraint 5): every node is shuffled
// once (fullShuffle, PurposeStartSelect, n-1 draws, always), then the
// shuffled order is walked greedily, accepting a candidate only if its
// graph distance to every already-accepted node is at least
// minStartDistance, until players are accepted or the order is exhausted.
// The second return is false if fewer than players nodes could be found —
// the caller treats that as this attempt's constraint-5 failure, not an
// error, and retries a fresh attempt rather than redrawing within this one.
//
// This is the one selection in the package whose RNG cost does not bound
// its own success: a shuffle-once-then-greedily-walk approach never redraws
// (keeping the cost a pure function of Nodes, matching every other draw in
// this package), but it also is not the only possible greedy walk that
// could find a valid set from the same shuffle — it is simply the one this
// package commits to, so two implementations of this rule can never
// desynchronise against each other (RFC-001 §6.4).
func selectStartPositions(rand Rand, b *builder, players int) ([]game.NodeID, bool) {
	order := b.nodesSlice()
	fullShuffle(rand, PurposeStartSelect, order)

	var selected []game.NodeID
	for _, candidate := range order {
		if len(selected) >= players {
			break
		}
		if farEnoughFromAll(b, candidate, selected) {
			selected = append(selected, candidate)
		}
	}

	if len(selected) < players {
		return nil, false
	}

	slices.Sort(selected)
	return selected, true
}

func farEnoughFromAll(b *builder, candidate game.NodeID, selected []game.NodeID) bool {
	if len(selected) == 0 {
		return true
	}
	dist := b.bfsDistances(candidate)
	for _, s := range selected {
		if d := dist[s]; d == -1 || d < minStartDistance {
			return false
		}
	}
	return true
}
