package gen

import (
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// testSectors builds a per-node sector assignment from a literal list, one
// entry per node — a small, readable stand-in for assignSectors' shuffled
// output when a test wants a fixed, known layout instead.
func testSectors(sectors ...game.Sector) []game.Sector { return sectors }

func TestBuilderConnectAndConnected(t *testing.T) {
	b := newBuilder(3, testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks))

	if b.connected(0, 1) {
		t.Fatal("connected(0, 1) = true before any connect() call")
	}

	b.connect(0, 1)
	if !b.connected(0, 1) || !b.connected(1, 0) {
		t.Fatal("connect(0, 1) did not make the edge visible from both endpoints")
	}
	if b.degree[0] != 1 || b.degree[1] != 1 {
		t.Fatalf("degree after one connect = [%d, %d], want [1, 1]", b.degree[0], b.degree[1])
	}

	// A repeated connect is a no-op: it must not double-count degree.
	b.connect(0, 1)
	if b.degree[0] != 1 || b.degree[1] != 1 {
		t.Fatalf("degree after a duplicate connect = [%d, %d], want [1, 1] (connect must be idempotent)", b.degree[0], b.degree[1])
	}

	// A self-loop is always a no-op — this package never builds one.
	b.connect(2, 2)
	if b.connected(2, 2) || b.degree[2] != 0 {
		t.Fatalf("connect(2, 2) created a self-loop: connected=%v degree=%d", b.connected(2, 2), b.degree[2])
	}
}

func TestBuilderEdgeCount(t *testing.T) {
	b := newBuilder(4, testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks))
	if got := b.edgeCount(); got != 0 {
		t.Fatalf("edgeCount() on an empty builder = %d, want 0", got)
	}

	b.connect(0, 1)
	b.connect(1, 2)
	b.connect(2, 3)
	if got := b.edgeCount(); got != 3 {
		t.Fatalf("edgeCount() = %d, want 3", got)
	}

	// Reconnecting an existing edge must not inflate the count.
	b.connect(0, 1)
	if got := b.edgeCount(); got != 3 {
		t.Fatalf("edgeCount() after a duplicate connect = %d, want 3", got)
	}
}

func TestBuilderNodesInSector(t *testing.T) {
	b := newBuilder(5, testSectors(
		game.SectorOldDocks, game.SectorIronLow, game.SectorOldDocks, game.SectorMistHeights, game.SectorOldDocks,
	))

	got := b.nodesInSector(game.SectorOldDocks)
	want := []game.NodeID{0, 2, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nodesInSector(OldDocks) = %v, want %v (ascending NodeID)", got, want)
	}

	if got := b.nodesInSector(game.SectorNorthVale); got != nil {
		t.Fatalf("nodesInSector(NorthVale) = %v, want nil (no nodes in that sector)", got)
	}
}

func TestBuilderBFSDistances(t *testing.T) {
	// A 4-node path: 0 - 1 - 2 - 3.
	b := newBuilder(5, testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks))
	b.connect(0, 1)
	b.connect(1, 2)
	b.connect(2, 3)
	// Node 4 is deliberately left isolated.

	dist := b.bfsDistances(0)
	want := []int{0, 1, 2, 3, -1}
	if !reflect.DeepEqual(dist, want) {
		t.Fatalf("bfsDistances(0) = %v, want %v", dist, want)
	}

	// Distance to self is always 0, even for an isolated node.
	if d := b.bfsDistances(4)[4]; d != 0 {
		t.Fatalf("bfsDistances(4)[4] = %d, want 0 (distance to self)", d)
	}
	if d := b.bfsDistances(4)[0]; d != -1 {
		t.Fatalf("bfsDistances(4)[0] = %d, want -1 (unreachable)", d)
	}
}

func TestBuilderConnectedNodes(t *testing.T) {
	// 0 - 1 - 2, and separately 3 - 4, all edges added so a global BFS from
	// 0 would reach everything except node 5 — connectedNodes' within
	// restriction is what should keep it scoped to {0,1,2} regardless.
	b := newBuilder(6, testSectors(
		game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks,
		game.SectorIronLow, game.SectorIronLow, game.SectorMistHeights,
	))
	b.connect(0, 1)
	b.connect(1, 2)
	b.connect(2, 3) // crosses into the "within" boundary below
	b.connect(3, 4)

	within := map[game.NodeID]bool{0: true, 1: true, 2: true}
	got := b.connectedNodes(0, within)
	want := map[game.NodeID]bool{0: true, 1: true, 2: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("connectedNodes(0, within={0,1,2}) = %v, want %v (must not cross into node 3)", got, want)
	}

	// within == nil means unrestricted: every node reachable through any
	// edge, including the one that crosses into {3, 4}.
	got = b.connectedNodes(0, nil)
	want = map[game.NodeID]bool{0: true, 1: true, 2: true, 3: true, 4: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("connectedNodes(0, nil) = %v, want %v", got, want)
	}
	if got[5] {
		t.Fatal("connectedNodes(0, nil) reached isolated node 5")
	}
}

func TestBuilderNeighborLists(t *testing.T) {
	b := newBuilder(4, testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks))
	b.connect(0, 1)
	b.connect(0, 2)
	b.connect(2, 3)

	got := b.neighborLists()
	want := [][]game.NodeID{
		{1, 2},
		{0},
		{0, 3},
		{2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("neighborLists() = %v, want %v", got, want)
	}
}

func TestBuilderNodesSlice(t *testing.T) {
	b := newBuilder(3, testSectors(game.SectorOldDocks, game.SectorOldDocks, game.SectorOldDocks))
	got := b.nodesSlice()
	want := []game.NodeID{0, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nodesSlice() = %v, want %v", got, want)
	}
}
