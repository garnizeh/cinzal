package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// buildGraph constructs a Graph of n nodes from an adjacency map, mirroring
// every edge both directions (the map's own convention — Node.Edges lists
// both endpoints' adjacency, GDD §6.2). Nodes not named in edges have none.
func buildGraph(n int, edges map[game.NodeID][]game.NodeID) Graph {
	nodes := make([]Node, n)
	for i := range nodes {
		nodes[i] = Node{ID: game.NodeID(i), Edges: edges[game.NodeID(i)]}
	}
	return Graph{Nodes: nodes}
}

// TestDistancesFromSrcIsZero pins the base case every other assertion here
// builds on.
func TestDistancesFromSrcIsZero(t *testing.T) {
	g := buildGraph(3, map[game.NodeID][]game.NodeID{0: {1}, 1: {0, 2}, 2: {1}})

	if got := g.distances(0)[0]; got != 0 {
		t.Errorf("distances(0)[0] = %d, want 0", got)
	}
}

// TestDistancesWalksShortestPath asserts a plain BFS shortest-path count on
// a line graph 0-1-2-3.
func TestDistancesWalksShortestPath(t *testing.T) {
	g := buildGraph(4, map[game.NodeID][]game.NodeID{
		0: {1},
		1: {0, 2},
		2: {1, 3},
		3: {2},
	})

	dist := g.distances(0)
	want := []int{0, 1, 2, 3}
	for i, w := range want {
		if dist[i] != w {
			t.Errorf("distances(0)[%d] = %d, want %d", i, dist[i], w)
		}
	}
}

// TestDistancesPrefersShortestOfSeveralPaths asserts BFS, not an arbitrary
// walk: node 3 is reachable in 2 steps (0-1-3) and in 3 (0-2-4-3); the
// shorter must win.
func TestDistancesPrefersShortestOfSeveralPaths(t *testing.T) {
	g := buildGraph(5, map[game.NodeID][]game.NodeID{
		0: {1, 2},
		1: {0, 3},
		2: {0, 4},
		3: {1},
		4: {2, 3},
	})

	if got := g.distances(0)[3]; got != 2 {
		t.Errorf("distances(0)[3] = %d, want 2 (via 0-1-3, shorter than 0-2-4-3)", got)
	}
}

// TestDistancesUnreachableIsNegativeOne asserts a disconnected node reports
// -1, not 0 or an out-of-range panic — GenerateOffer's distance-band check
// (contracts.go) depends on this sentinel to exclude it rather than
// mistaking it for "adjacent."
func TestDistancesUnreachableIsNegativeOne(t *testing.T) {
	g := buildGraph(3, map[game.NodeID][]game.NodeID{0: {1}, 1: {0}, 2: {}})

	if got := g.distances(0)[2]; got != -1 {
		t.Errorf("distances(0)[2] = %d, want -1 (node 2 has no edge to the rest of the graph)", got)
	}
}

// TestDistancesExcludesSinkholedNodes asserts GDD §9.1a item 0: the
// navigable graph excludes nodes SinkholeRounds > 0 makes impassable, not
// just edges Bridge Down destroys. A direct 0-1-2 path exists, but node 1
// is Sinkholed, so the only route left is the longer 0-3-2.
func TestDistancesExcludesSinkholedNodes(t *testing.T) {
	g := buildGraph(4, map[game.NodeID][]game.NodeID{
		0: {1, 3},
		1: {0, 2},
		2: {1, 3},
		3: {0, 2},
	})
	g.Nodes[1].SinkholeRounds = 3

	dist := g.distances(0)
	if dist[1] != -1 {
		t.Errorf("distances(0)[1] = %d, want -1 (node 1 is Sinkholed, so it is never entered even as itself reachable-but-blocking)", dist[1])
	}
	if dist[2] != 2 {
		t.Errorf("distances(0)[2] = %d, want 2 (via 0-3-2; the direct 0-1-2 path is blocked by node 1's Sinkhole)", dist[2])
	}
}

// TestDistancesFromSinkholedSrcNeverExpands asserts a Sinkholed src reports
// itself at 0 (never an out-of-range panic) but reaches nothing else — it
// must not expand to its neighbours even though its own distance is defined.
// contractCandidates (contracts.go) calls distances(origin.ID) once per
// Known Warehouse, which need not be the seat's own position: a Warehouse
// that became Sinkholed after the seat learned of it must not still look
// reachable to Borders through it.
func TestDistancesFromSinkholedSrcNeverExpands(t *testing.T) {
	g := buildGraph(3, map[game.NodeID][]game.NodeID{0: {1}, 1: {0, 2}, 2: {1}})
	g.Nodes[0].SinkholeRounds = 1

	dist := g.distances(0)
	if dist[0] != 0 {
		t.Errorf("distances(0)[0] = %d, want 0 (a node's distance to itself, even Sinkholed)", dist[0])
	}
	if dist[1] != -1 {
		t.Errorf("distances(0)[1] = %d, want -1 (Sinkholed src never expands to its neighbours)", dist[1])
	}
	if dist[2] != -1 {
		t.Errorf("distances(0)[2] = %d, want -1 (unreachable through a Sinkholed src)", dist[2])
	}
}

// TestDistancesRespectsBridgeDown asserts a destroyed edge (simply absent
// from both endpoints' Edges, per Node's own convention — Bridge Down never
// leaves a one-directional stub) forces the longer remaining path, the same
// property GDD §9.1a item 0 requires for the live navigable graph as
// opposed to the static setup graph.
func TestDistancesRespectsBridgeDown(t *testing.T) {
	beforeBridgeDown := buildGraph(3, map[game.NodeID][]game.NodeID{
		0: {1, 2},
		1: {0},
		2: {0},
	})
	if got := beforeBridgeDown.distances(0)[1]; got != 1 {
		t.Fatalf("before Bridge Down: distances(0)[1] = %d, want 1", got)
	}

	// Bridge Down destroys the 0-1 edge: gone from both endpoints.
	afterBridgeDown := buildGraph(3, map[game.NodeID][]game.NodeID{
		0: {2},
		1: {2},
		2: {0, 1},
	})
	if got := afterBridgeDown.distances(0)[1]; got != 2 {
		t.Errorf("after Bridge Down: distances(0)[1] = %d, want 2 (only remaining path is 0-2-1)", got)
	}
}

// sectorGraph builds a Graph of n nodes from an adjacency map, exactly like
// buildGraph, but also stamps each node's Sector from sectors — the shape
// distancesToSector's multi-source BFS needs that a plain buildGraph graph
// does not carry. A node absent from sectors keeps FogState's zero-value
// analogue for Sector, an invalid value distinct from every real one (GDD
// §3 has exactly four), which is fine here: these tests only ever query
// distance to a sector some node explicitly claims.
func sectorGraph(n int, edges map[game.NodeID][]game.NodeID, sectors map[game.NodeID]game.Sector) Graph {
	g := buildGraph(n, edges)
	for id, sector := range sectors {
		g.Nodes[id].Sector = sector
	}
	return g
}

// TestDistancesToSectorZeroAtEverySectorMember asserts every node belonging
// to the target sector reports distance 0 — the multi-source BFS's base
// case, mirroring TestDistancesFromSrcIsZero's single-source one.
func TestDistancesToSectorZeroAtEverySectorMember(t *testing.T) {
	g := sectorGraph(3, map[game.NodeID][]game.NodeID{0: {1}, 1: {0, 2}, 2: {1}},
		map[game.NodeID]game.Sector{1: game.SectorMistHeights, 2: game.SectorMistHeights})

	dist := g.distancesToSector(game.SectorMistHeights)
	if dist[1] != 0 {
		t.Errorf("distancesToSector(MistHeights)[1] = %d, want 0 (node 1 is itself in the sector)", dist[1])
	}
	if dist[2] != 0 {
		t.Errorf("distancesToSector(MistHeights)[2] = %d, want 0 (node 2 is itself in the sector)", dist[2])
	}
	if dist[0] != 1 {
		t.Errorf("distancesToSector(MistHeights)[0] = %d, want 1 (one step from either sector member)", dist[0])
	}
}

// TestDistancesToSectorUnreachableIsNegativeOne asserts a sector with no
// member reachable at all reports -1 everywhere — the sentinel Pushing On's
// priority ladder reads as "sector unreachable, fall through to level 4"
// (GDD §9.1).
func TestDistancesToSectorUnreachableIsNegativeOne(t *testing.T) {
	g := sectorGraph(2, map[game.NodeID][]game.NodeID{0: {1}, 1: {0}}, nil)

	dist := g.distancesToSector(game.SectorNorthVale)
	for i, d := range dist {
		if d != -1 {
			t.Errorf("distancesToSector(NorthVale)[%d] = %d, want -1 (no node belongs to that sector)", i, d)
		}
	}
}

// TestDistancesToSectorRoutesAroundBridgeDownAndSinkhole is the issue's own
// acceptance criterion: "a test with a destroyed edge and a Sinkhole
// asserts the ladder routes around both." Node 3 is the only sector member,
// reachable two ways: the graph is already built with only currently-
// navigable edges (Bridge Down's own convention — a destroyed edge is
// simply absent, per Node's own comment, exactly as
// TestDistancesRespectsBridgeDown exercises for the single-source case),
// and the shorter of the two surviving paths, through node 1, is additionally
// blocked by node 1's Sinkhole — forcing the longer 0-2-4-3 detour.
func TestDistancesToSectorRoutesAroundBridgeDownAndSinkhole(t *testing.T) {
	g := sectorGraph(5, map[game.NodeID][]game.NodeID{
		0: {1, 2},
		1: {0, 3},
		2: {0, 4},
		3: {1, 4},
		4: {2, 3},
	}, map[game.NodeID]game.Sector{3: game.SectorMistHeights})
	g.Nodes[1].SinkholeRounds = 3

	dist := g.distancesToSector(game.SectorMistHeights)
	if dist[0] != 3 {
		t.Errorf("distancesToSector(MistHeights)[0] = %d, want 3 (via 0-2-4-3; node 1's Sinkhole blocks the shorter 0-1-3)", dist[0])
	}
	if dist[1] != -1 {
		t.Errorf("distancesToSector(MistHeights)[1] = %d, want -1 (node 1 is Sinkholed and never entered)", dist[1])
	}
}
