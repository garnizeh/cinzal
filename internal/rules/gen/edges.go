package gen

import (
	"slices"
	"sort"

	"github.com/garnizeh/cinzal/internal/game"
)

// edgeCandidate is one unordered node pair, always stored a < c so the same
// pair is never represented two different ways.
type edgeCandidate struct{ a, c game.NodeID }

func newEdgeCandidate(a, c game.NodeID) edgeCandidate {
	if a > c {
		a, c = c, a
	}
	return edgeCandidate{a: a, c: c}
}

// sortCandidates sorts candidates ascending by (a, c) — RFC-001 §6.4's
// canonical pair order, the same one Bridge Down's own candidate set uses
// (docs/decisions/D03-rng-consumption-table.md), so shuffling a candidate
// slice never depends on the order it happened to be appended in.
func sortCandidates(candidates []edgeCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].a != candidates[j].a {
			return candidates[i].a < candidates[j].a
		}
		return candidates[i].c < candidates[j].c
	})
}

// intraCandidates returns every not-yet-connected pair within sector s,
// ascending by (a, c) by construction (nodesInSector is already ascending,
// and the double loop below only ever increases both indices).
func intraCandidates(b *builder, s game.Sector) []edgeCandidate {
	nodes := b.nodesInSector(s)
	var candidates []edgeCandidate
	for i := range nodes {
		for j := i + 1; j < len(nodes); j++ {
			if !b.connected(nodes[i], nodes[j]) {
				candidates = append(candidates, edgeCandidate{a: nodes[i], c: nodes[j]})
			}
		}
	}
	return candidates
}

// crossCandidates returns every not-yet-connected pair with one endpoint in
// sector x and the other in sector y, sorted ascending by (a, c).
func crossCandidates(b *builder, x, y game.Sector) []edgeCandidate {
	xs := b.nodesInSector(x)
	ys := b.nodesInSector(y)

	candidates := make([]edgeCandidate, 0, len(xs)*len(ys))
	for _, a := range xs {
		for _, c := range ys {
			if !b.connected(a, c) {
				candidates = append(candidates, newEdgeCandidate(a, c))
			}
		}
	}
	sortCandidates(candidates)
	return candidates
}

// degreeNeed reports how many of candidate's two endpoints still sit below
// GDD §6.1 constraint 2's minimum degree — 0, 1, or 2.
func degreeNeed(b *builder, candidate edgeCandidate) int {
	need := 0
	if b.degree[candidate.a] < minDegree {
		need++
	}
	if b.degree[candidate.c] < minDegree {
		need++
	}
	return need
}

// sortByDegreeNeed stable-sorts candidates so pairs touching more
// below-minimum-degree nodes come first. It is a deterministic re-sort, not
// a draw: the point is to spend the mandatory chokepoint and fill edges on
// GDD §6.1 constraint 2's leftover degree-1 leaves wherever possible, rather
// than on edges that would have been added anyway, without paying for a
// second, separate repair pass. Stability preserves the shuffle's random
// order within each tier, since sort.SliceStable never reorders equal
// elements.
func sortByDegreeNeed(b *builder, candidates []edgeCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return degreeNeed(b, candidates[i]) > degreeNeed(b, candidates[j])
	})
}

// nodeSpanningTree builds a random spanning tree over a sector's nodes
// (ascending NodeID; nodes[0], the sector's lowest-ID node, is always the
// deterministic root — not itself a draw), preferring to attach each new
// node to a currently-lowest-degree already-added node so the tree does not
// concentrate degree on one hub before GDD §6.1 constraint 2's max-degree-4
// cap has any headroom left for the chokepoint and fill phases that follow.
// Cost: full shuffle of nodes[1:] (len(nodes)-2 draws, floor 0) plus one
// bounded draw per non-root node (len(nodes)-1 draws), all under
// PurposeSectorTree.
func nodeSpanningTree(rand Rand, b *builder, nodes []game.NodeID) {
	if len(nodes) <= 1 {
		return
	}

	rest := slices.Clone(nodes[1:])
	fullShuffle(rand, PurposeSectorTree, rest)

	added := make([]game.NodeID, 1, len(nodes))
	added[0] = nodes[0]
	for _, child := range rest {
		parent := lowestDegreeAmong(rand, PurposeSectorTree, b, added)
		b.connect(parent, child)
		added = append(added, child)
	}
}

// lowestDegreeAmong returns a member of added chosen uniformly at random
// among those currently at added's lowest degree, consuming exactly one
// draw under purpose regardless of how many are tied.
func lowestDegreeAmong(rand Rand, purpose string, b *builder, added []game.NodeID) game.NodeID {
	lowest := b.degree[added[0]]
	for _, n := range added[1:] {
		if b.degree[n] < lowest {
			lowest = b.degree[n]
		}
	}

	var tied []game.NodeID
	for _, n := range added {
		if b.degree[n] == lowest {
			tied = append(tied, n)
		}
	}
	return tied[rand(purpose, len(tied))]
}

// buildChokepoints adds GDD §6.1 constraint 4's chokepoint edges for every
// sector pair sectorAdjacency chose, returning each pair's actual accepted
// count (index-aligned with pairs) for the fill phase and final
// verification to read. For each pair: one draw picks the target count, 3
// to 5 (PurposeChokepointCount); the pair's full candidate pool is shuffled
// once (PurposeChokepointSelect) and stable-sorted by degree need, then
// walked in that order, skipping any candidate that would exceed GDD §6.1
// constraint 2's degree-4 cap, until the target is reached or the pool is
// exhausted. A pool exhausted before reaching 3 is a constraint-4 failure
// the caller's verification step catches — this function itself never
// retries or redraws.
func buildChokepoints(rand Rand, b *builder, pairs []sectorPair) []int {
	counts := make([]int, len(pairs))
	for i, p := range pairs {
		target := 3 + rand(PurposeChokepointCount, 3) // 3, 4, or 5

		candidates := crossCandidates(b, p.a, p.b)
		fullShuffle(rand, PurposeChokepointSelect, candidates)
		sortByDegreeNeed(b, candidates)

		accepted := 0
		for _, c := range candidates {
			if accepted >= target {
				break
			}
			if b.connected(c.a, c.c) || b.degree[c.a] >= maxDegree || b.degree[c.c] >= maxDegree {
				continue
			}
			b.connect(c.a, c.c)
			accepted++
		}
		counts[i] = accepted
	}
	return counts
}

// fillCandidate is one fillEdges pool entry: the edge itself, and which
// chokepoint pair it counts against (index into pairs/chokepointCounts), or
// -1 for an intra-sector candidate, which has no such cap.
type fillCandidate struct {
	edge edgeCandidate
	pair int
}

// fillEdges adds edges until the graph reaches target total edges (GDD §6.1's
// per-player-count edge range) or no valid candidate remains. The pool is
// every remaining intra-sector pair, plus every remaining pair within a
// chosen chokepoint sector-pair that has not yet hit GDD §6.1 constraint 4's
// 5-edge cap. Shuffled once, then stable-sorted by degree need — the same
// leftover-leaf-preferring shape buildChokepoints uses — under one purpose,
// PurposeFillEdge, regardless of pool size; chokepointCounts is updated in
// place as inter-sector candidates are accepted, so the 5-edge cap is
// enforced live rather than only at the end.
func fillEdges(rand Rand, b *builder, pairs []sectorPair, chokepointCounts []int, target int) {
	if b.edgeCount() >= target {
		return
	}

	var pool []fillCandidate
	for _, s := range sectorOrder {
		for _, c := range intraCandidates(b, s) {
			pool = append(pool, fillCandidate{edge: c, pair: -1})
		}
	}
	for i, p := range pairs {
		for _, c := range crossCandidates(b, p.a, p.b) {
			pool = append(pool, fillCandidate{edge: c, pair: i})
		}
	}
	sort.Slice(pool, func(i, j int) bool {
		if pool[i].edge.a != pool[j].edge.a {
			return pool[i].edge.a < pool[j].edge.a
		}
		return pool[i].edge.c < pool[j].edge.c
	})

	fullShuffle(rand, PurposeFillEdge, pool)

	sort.SliceStable(pool, func(i, j int) bool {
		return degreeNeed(b, pool[i].edge) > degreeNeed(b, pool[j].edge)
	})

	for _, fc := range pool {
		if b.edgeCount() >= target {
			return
		}
		if b.connected(fc.edge.a, fc.edge.c) || b.degree[fc.edge.a] >= maxDegree || b.degree[fc.edge.c] >= maxDegree {
			continue
		}
		if fc.pair >= 0 && chokepointCounts[fc.pair] >= 5 {
			continue
		}
		b.connect(fc.edge.a, fc.edge.c)
		if fc.pair >= 0 {
			chokepointCounts[fc.pair]++
		}
	}
}

// Touched only to verify CodeRabbit review now reaches this package (see #108, #109).
