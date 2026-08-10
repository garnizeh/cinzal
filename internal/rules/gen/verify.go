package gen

import "github.com/garnizeh/cinzal/internal/game"

// Constraint names for ExhaustedError.Failures — one per GDD §6.1
// constraint (or the sub-check that can fail within it) this package
// actually verifies. Every attempt.violations() call returns a subset of
// these; an attempt can fail more than one at once.
const (
	constraintDegree          = "degree (constraint 2: min 2, max 4)"
	constraintSectorConnected = "sector-connectivity (constraint 3)"
	constraintGraphConnected  = "connectivity (constraint 1)"
	constraintChokepointRange = "chokepoint-range (constraint 4: 3-5 edges)"
	constraintEdgeCount       = "edge-count (out of MinEdges/MaxEdges range)"
	constraintStartDistance   = "start-distance (constraint 5: min distance 4)"
)

// violations runs every structural check GDD §6.1 constraints 1-4 require
// against b (the graph, without regard to starting positions) and returns
// every constraint name that failed — never just the first, so
// ExhaustedError's failure tally reflects every reason an attempt was
// rejected, not only whichever check happened to run first.
func (b *builder) violations(chokepointCounts []int, minEdges, maxEdges int) []string {
	var failed []string

	for i := range b.n {
		if b.degree[i] < minDegree || b.degree[i] > maxDegree {
			failed = append(failed, constraintDegree)
			break
		}
	}

	for _, s := range sectorOrder {
		nodes := b.nodesInSector(s)
		if len(nodes) == 0 {
			continue
		}
		within := make(map[game.NodeID]bool, len(nodes))
		for _, n := range nodes {
			within[n] = true
		}
		reached := b.connectedNodes(nodes[0], within)
		if len(reached) != len(nodes) {
			failed = append(failed, constraintSectorConnected)
			break
		}
	}

	reached := b.connectedNodes(0, nil)
	if len(reached) != b.n {
		failed = append(failed, constraintGraphConnected)
	}

	for _, c := range chokepointCounts {
		if c < 3 || c > 5 {
			failed = append(failed, constraintChokepointRange)
			break
		}
	}

	if got := b.edgeCount(); got < minEdges || got > maxEdges {
		failed = append(failed, constraintEdgeCount)
	}

	return failed
}

// Touched only to verify CodeRabbit review now reaches this package (see #108, #109).
