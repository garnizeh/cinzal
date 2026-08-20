package gen

import (
	"fmt"

	"github.com/garnizeh/cinzal/internal/game"
)

// Node is one node's topology and placement as gen constructs it: identity,
// sector membership, adjacency, GDD §6.2 type, and canvas coordinates (D10).
// Fog seeding — which nodes begin Known to which player under GDD §6.1
// constraint 7 — is internal/rules' job at Setup: this package only
// guarantees the structural property (within 2 steps of every starting
// position, a Warehouse that itself has a Border inside Tier I's contract
// band) that makes that seeding useful — see D24.
type Node struct {
	ID     game.NodeID
	Sector game.Sector

	// Type is this node's GDD §6.2 function, assigned under D9's counts and
	// GDD §6.1 constraint 6 (no Warehouse adjacent to a Border) — see
	// nodetype.go.
	Type game.NodeType

	// X, Y are canvas coordinates, 0-1000, generated once here and never
	// recomputed per viewer (D10) — see layout.go.
	X, Y int

	// Edges is this node's adjacency list, ascending game.NodeID — the same
	// convention rules.Node.Edges already uses, so converting gen's output
	// into a rules.Node costs nothing but copying the extra fields rules.Node
	// adds on top (Post, SinkholeRounds, name).
	Edges []game.NodeID
}

// Graph is the generated map in full: every node's sector, type, layout,
// and adjacency, plus the player-count-many starting positions GDD §6.1
// constraints 5 and 7 require — mutual graph distance >= 4 apart, and each
// within 2 steps of a Warehouse that itself has a Border inside Tier I's
// contract band (D24).
type Graph struct {
	// Nodes is indexed by NodeID — Nodes[i].ID == game.NodeID(i) always,
	// matching rules.Node's own convention (RFC-001 §6.4/§6.5's
	// NodeID-ascending default).
	Nodes []Node

	// StartPositions is one node per player, ascending game.NodeID,
	// pairwise graph distance >= 4 apart (GDD §6.1 constraint 5).
	StartPositions []game.NodeID
}

// Params is everything Generate needs beyond the seeded draw source. Nodes,
// MinEdges and MaxEdges come from GDD §6.1's per-player-count table
// (game.Config.MapByPlayers); Players sets how many starting positions
// constraint 5 must place; MaxAttempts bounds the reject-and-retry loop GDD
// §6.1 specifies ("The generator rejects and retries until all hold") —
// game.Config.MaxGenAttempts, per D9's decision log.
type Params struct {
	Nodes    int
	MinEdges int
	MaxEdges int
	Players  int

	MaxAttempts int

	// OpeningMinDistance and OpeningMaxDistance are Tier I's contract band
	// (GDD §8.3): constraint 7 requires a Border at a graph distance within
	// the inclusive range [OpeningMinDistance, OpeningMaxDistance] of a
	// Warehouse near a starting position, for that Warehouse to be able to
	// originate the opening contract offer (D24).
	//
	// They arrive as parameters rather than gen constants for the same
	// reason every other number in this struct does: gen must not import
	// game.Config — that boundary is why Params exists — and a hard-coded
	// 3/4 would let a §8.3 band edit silently stop satisfying the
	// constraint that exists to serve it. internal/rules fills them from
	// cfg.Contracts[0].MinDistance/.MaxDistance.
	OpeningMinDistance int
	OpeningMaxDistance int
}

// minSupportedNodes is D8's four-sectors-of-at-least-three floor (D8,
// docs/decisions/D08-sector-size-constraint.md): four sectors, three nodes
// each, is the smallest map Generate can ever produce a legal graph for.
const minSupportedNodes = len(sectorOrder) * minSectorNodes

// maxSupportedNodes reports the largest Nodes value Generate can serve, and
// names the constraint that sets it. sectorSizes splits n into four
// near-equal parts, so the ceiling is four times the most nodes a single
// sector may hold — and two independent limits set that, at two different
// numbers: D8 caps a sector at maxSectorNodes (8), while D10's fixed layout
// lattice offers len(latticeCells) (9) cells for a sector's nodes to land on
// (docs/decisions/D10-map-layout.md). The tighter of the two binds, and the
// error says which.
//
// Both are computed rather than written out as one literal because they can
// move apart: raising D8's cap alone would re-point the ceiling at the
// lattice, and overrunning the lattice is not a rejected map — it is a panic
// three frames down in partialShuffle, naming neither Nodes nor D8 nor D10
// (#239).
func maxSupportedNodes() (nodes int, limit string) {
	perSector := maxSectorNodes
	limit = fmt.Sprintf("D8: each sector holds at most %d nodes", maxSectorNodes)
	if len(latticeCells) < perSector {
		perSector = len(latticeCells)
		limit = fmt.Sprintf("D10: a sector's nodes must fit the %d-cell layout lattice", len(latticeCells))
	}
	return len(sectorOrder) * perSector, limit
}

// validate reports whether p is well-formed enough to attempt generation at
// all — this is a caller-error check, not a generation failure, so it never
// counts against MaxAttempts.
func (p Params) validate() error {
	if p.Nodes < minSupportedNodes {
		return fmt.Errorf("gen: Nodes must be at least %d (D8: four sectors of at least three nodes each), got %d", minSupportedNodes, p.Nodes)
	}
	// The ceiling is here for the same reason the OpeningMinDistance band
	// below is, except that its failure is worse than an exhausted
	// MaxAttempts: sectorSizes happily splits a too-large n into oversized
	// sectors, and the first sector wider than D10's layout lattice panics
	// inside computeLayout's partialShuffle rather than failing any
	// constraint. It fails here instead, loudly and before the first attempt
	// (#239).
	if maxNodes, limit := maxSupportedNodes(); p.Nodes > maxNodes {
		return fmt.Errorf("gen: Nodes must be at most %d (%s), got %d", maxNodes, limit, p.Nodes)
	}
	if p.MinEdges < 1 || p.MaxEdges < p.MinEdges {
		return fmt.Errorf("gen: MinEdges/MaxEdges must satisfy 1 <= MinEdges <= MaxEdges, got [%d, %d]", p.MinEdges, p.MaxEdges)
	}
	if p.Players < 1 {
		return fmt.Errorf("gen: Players must be at least 1, got %d", p.Players)
	}
	if p.MaxAttempts < 1 {
		return fmt.Errorf("gen: MaxAttempts must be at least 1, got %d", p.MaxAttempts)
	}
	// An unset band would make constraint 7 unsatisfiable (no Border sits at
	// distance <= 0 from a Warehouse), which would surface as an exhausted
	// MaxAttempts — a generation failure reported for a caller error. It
	// fails here instead, loudly and before the first attempt (D24).
	if p.OpeningMinDistance < 1 || p.OpeningMaxDistance < p.OpeningMinDistance {
		return fmt.Errorf("gen: OpeningMinDistance/OpeningMaxDistance must satisfy 1 <= OpeningMinDistance <= OpeningMaxDistance, got [%d, %d]", p.OpeningMinDistance, p.OpeningMaxDistance)
	}
	return nil
}

// ExhaustedError is returned when Generate could not produce a graph
// satisfying every GDD §6.1 constraint within Params.MaxAttempts tries. GDD
// §6.1's algorithm is "rejects and retries until all hold"; D9's decision
// log requires this be a returned error — never a partial graph, never an
// infinite loop.
type ExhaustedError struct {
	Attempts int

	// MostFailed is the constraint name (see the constraint* names in
	// verify.go) that caused a rejection on the most attempts.
	MostFailed string

	// Failures maps every constraint name that caused at least one
	// rejection to how many attempts it failed on. A single attempt can
	// fail more than one constraint at once, so these counts do not need to
	// sum to Attempts.
	Failures map[string]int
}

func (e *ExhaustedError) Error() string {
	return fmt.Sprintf("gen: exhausted %d attempts without a graph satisfying every GDD §6.1 constraint; most frequent failure: %q (%d/%d attempts) — full breakdown: %v",
		e.Attempts, e.MostFailed, e.Failures[e.MostFailed], e.Attempts, e.Failures)
}
