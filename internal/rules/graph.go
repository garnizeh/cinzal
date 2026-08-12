package rules

import "github.com/garnizeh/cinzal/internal/game"

// distances returns g's BFS shortest-path distance from src to every node,
// on the currently navigable graph — GDD §9.1a item 0: "the base graph minus
// edges destroyed by Bridge Down and minus nodes made impassable by
// Sinkhole." Node.Edges already omits Bridge-Down-destroyed edges (see
// Node's own comment), so the only additional exclusion this walk needs is
// any node with SinkholeRounds > 0, which it never enters or exits.
//
// distances[n] is -1 when n is unreachable from src. distances[src] is
// always 0 — even when src itself is Sinkholed, since "unreachable from
// itself" would be a stranger answer than "0 steps away" — but a Sinkholed
// src never expands to its neighbours: contractCandidates (contracts.go)
// calls this once per Known Warehouse, which is not necessarily the seat's
// own position, so a Warehouse that became Sinkholed after the seat learned
// of it must not still originate reachable-looking Border candidates.
//
// This is the one BFS implementation both D23's fog seeding (initial.go, the
// setup graph) and the contract tier distance bands (contracts.go, the live
// graph) call — internal/rules/gen's own bfsDistances (gen/builder.go)
// cannot be reused directly: it walks a different Node/Graph shape that
// carries no SinkholeRounds, because Sinkholes cannot exist before Setup.
func (g Graph) distances(src game.NodeID) []int {
	dist := make([]int, len(g.Nodes))
	for i := range dist {
		dist[i] = -1
	}
	dist[src] = 0
	if g.Nodes[src].SinkholeRounds > 0 {
		return dist
	}

	queue := []game.NodeID{src}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, next := range g.Nodes[cur].Edges {
			if g.Nodes[next].SinkholeRounds > 0 || dist[next] != -1 {
				continue
			}
			dist[next] = dist[cur] + 1
			queue = append(queue, next)
		}
	}

	return dist
}
