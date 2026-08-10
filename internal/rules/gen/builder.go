package gen

import "github.com/garnizeh/cinzal/internal/game"

// maxDegree and minDegree are GDD §6.1 constraint 2, fixed by the rules —
// "no dead ends (frustrating), no super-hubs (dominant)".
const (
	minDegree = 2
	maxDegree = 4
)

// builder is one generation attempt's working graph: an adjacency matrix
// rather than per-node slices, so degree and edge-existence checks during
// construction are O(1) instead of a scan, and the final Node.Edges slices
// are built once, at the end, by ascending NodeID — never by ranging
// anything unordered (RFC-001 §6.3).
type builder struct {
	n      int
	sector []game.Sector // sector[i] is node i's sector
	adj    [][]bool      // adj[i][j] == adj[j][i]; adjacency matrix, no self-loops
	degree []int
}

func newBuilder(n int, sector []game.Sector) *builder {
	adj := make([][]bool, n)
	for i := range adj {
		adj[i] = make([]bool, n)
	}
	return &builder{n: n, sector: sector, adj: adj, degree: make([]int, n)}
}

// connected reports whether a and b are already joined by an edge.
func (b *builder) connected(a, c game.NodeID) bool {
	return b.adj[a][c]
}

// connect adds the edge (a, c) if it does not already exist. a == c is a
// no-op — this package never builds self-loops. Callers are responsible for
// checking degree headroom first; connect does not enforce maxDegree
// itself, so a caller that skips the check can violate it, which is exactly
// what the final verification pass in verify.go exists to catch.
func (b *builder) connect(a, c game.NodeID) {
	if a == c || b.adj[a][c] {
		return
	}
	b.adj[a][c] = true
	b.adj[c][a] = true
	b.degree[a]++
	b.degree[c]++
}

// edgeCount returns the total number of distinct edges added so far.
func (b *builder) edgeCount() int {
	total := 0
	for i := range b.n {
		total += b.degree[i]
	}
	return total / 2
}

// nodesInSector returns every node in sector s, ascending NodeID.
func (b *builder) nodesInSector(s game.Sector) []game.NodeID {
	var nodes []game.NodeID
	for i := range b.n {
		if b.sector[i] == s {
			nodes = append(nodes, game.NodeID(i))
		}
	}
	return nodes
}

// bfsDistances returns graph distance from src to every node, ascending
// NodeID, using only edges currently in the adjacency matrix. Unreachable
// nodes carry -1. Distance to itself is 0.
func (b *builder) bfsDistances(src game.NodeID) []int {
	dist := make([]int, b.n)
	for i := range dist {
		dist[i] = -1
	}
	dist[src] = 0

	queue := []game.NodeID{src}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for next := range b.n {
			if b.adj[cur][next] && dist[next] == -1 {
				dist[next] = dist[cur] + 1
				queue = append(queue, game.NodeID(next))
			}
		}
	}
	return dist
}

// connectedNodes returns every node reachable from src, including src
// itself, restricted to the induced subgraph on within (used to check GDD
// §6.1 constraint 3's per-sector internal connectivity, where only that
// sector's own nodes and edges count).
func (b *builder) connectedNodes(src game.NodeID, within map[game.NodeID]bool) map[game.NodeID]bool {
	seen := map[game.NodeID]bool{src: true}
	queue := []game.NodeID{src}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for next := range b.n {
			nid := game.NodeID(next)
			if within != nil && !within[nid] {
				continue
			}
			if b.adj[cur][nid] && !seen[nid] {
				seen[nid] = true
				queue = append(queue, nid)
			}
		}
	}
	return seen
}

// nodesSlice returns every node ID in this graph, ascending.
func (b *builder) nodesSlice() []game.NodeID {
	nodes := make([]game.NodeID, b.n)
	for i := range nodes {
		nodes[i] = game.NodeID(i)
	}
	return nodes
}

// Touched only to verify CodeRabbit review now reaches this package (see #108, #109).
