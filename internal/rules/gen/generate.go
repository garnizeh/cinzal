package gen

import "github.com/garnizeh/cinzal/internal/game"

// Generate builds the match map graph under GDD §6.1 constraints 1-5 —
// connectivity, degree bounds, sector sizing and internal connectivity,
// chokepoint edge counts, and starting-position separation. Constraints 6
// and 7 (node type placement) are issue #60's job, layered on top of this
// graph.
//
// "The generator rejects and retries until all hold" (GDD §6.1): each
// attempt is built from scratch, verified against every constraint, and
// either accepted or discarded. A discarded attempt's draws are never
// rolled back — rand is threaded through every attempt without resetting,
// so the attempt count is part of the same index stream every other
// consumer in this engine shares, exactly as issue #59 requires. Exhausting
// Params.MaxAttempts returns *ExhaustedError, naming whichever constraint
// failed on the most attempts, never a partial graph.
func Generate(rand Rand, p Params) (Graph, error) {
	if err := p.validate(); err != nil {
		return Graph{}, err
	}

	sizes := sectorSizes(p.Nodes)
	failures := make(map[string]int)

	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		g, ok, failed := attemptGenerate(rand, p, sizes)
		if ok {
			return g, nil
		}
		for _, f := range failed {
			failures[f]++
		}
	}

	mostFailed := ""
	mostCount := -1
	for _, name := range []string{
		constraintDegree,
		constraintSectorConnected,
		constraintGraphConnected,
		constraintChokepointRange,
		constraintEdgeCount,
		constraintTypeAdjacency,
		constraintStartPlacement,
	} {
		if failures[name] > mostCount {
			mostCount = failures[name]
			mostFailed = name
		}
	}

	return Graph{}, &ExhaustedError{
		Attempts:   p.MaxAttempts,
		MostFailed: mostFailed,
		Failures:   failures,
	}
}

// attemptGenerate builds and verifies exactly one attempt. The third return
// is every constraint name that failed, non-nil only when ok is false.
func attemptGenerate(rand Rand, p Params, sizes [4]int) (Graph, bool, []string) {
	sector := assignSectors(rand, p.Nodes, sizes)
	b := newBuilder(p.Nodes, sector)

	for _, s := range sectorOrder {
		nodeSpanningTree(rand, b, b.nodesInSector(s))
	}

	pairs := sectorAdjacency(rand)

	target := p.MinEdges + rand(PurposeEdgeCount, p.MaxEdges-p.MinEdges+1)

	chokepointCounts := buildChokepoints(rand, b, pairs)
	fillEdges(rand, b, pairs, chokepointCounts, target)

	if failed := b.violations(chokepointCounts, p.MinEdges, p.MaxEdges); len(failed) > 0 {
		return Graph{}, false, failed
	}

	types, ok := assignNodeTypes(rand, b)
	if !ok {
		return Graph{}, false, []string{constraintTypeAdjacency}
	}

	starts, ok := selectStartPositions(rand, b, p.Players, types)
	if !ok {
		return Graph{}, false, []string{constraintStartPlacement}
	}

	x, y := computeLayout(rand, b)

	return buildGraph(b, types, x, y, starts), true, nil
}

// buildGraph converts b's finished adjacency matrix, plus the type and
// layout passes' own per-node output, into the exported Graph shape: each
// node's Edges slice is built once, by ascending NodeID on both the outer
// node and the adjacency scan — never by ranging anything unordered
// (RFC-001 §6.3).
func buildGraph(b *builder, types []game.NodeType, x, y []int, starts []game.NodeID) Graph {
	nodes := make([]Node, b.n)
	for i := range b.n {
		var edges []game.NodeID
		for j := range b.n {
			if b.adj[i][j] {
				edges = append(edges, game.NodeID(j))
			}
		}
		nodes[i] = Node{
			ID:     game.NodeID(i),
			Sector: b.sector[i],
			Type:   types[i],
			X:      x[i],
			Y:      y[i],
			Edges:  edges,
		}
	}
	return Graph{Nodes: nodes, StartPositions: starts}
}
