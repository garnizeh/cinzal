package gen

import (
	"fmt"

	"github.com/garnizeh/cinzal/internal/game"
)

// Node is one node's topology as gen constructs it: identity, sector
// membership, and its adjacency list. Node type (Warehouse/Border/...),
// starting-position fog seeding, and 2D layout are issue #60's job, added
// once this graph exists — this package's own scope is GDD §6.1 constraints
// 1-5 only; constraints 6-7 are placement questions #60 owns.
type Node struct {
	ID     game.NodeID
	Sector game.Sector

	// Edges is this node's adjacency list, ascending game.NodeID — the same
	// convention rules.Node.Edges already uses, so converting gen's output
	// into a rules.Node costs nothing but copying the extra fields rules.Node
	// adds on top (Post, SinkholeRounds, layout, name).
	Edges []game.NodeID
}

// Graph is the generated topology: every node with its sector and
// adjacency, and the player-count-many starting positions GDD §6.1
// constraint 5 requires (mutual graph distance >= 4, GDD §6.1). Node type
// assignment, fog seeding and 2D layout are added on top of this by #60.
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
}

// minSupportedNodes is D8's four-sectors-of-at-least-three floor (D8,
// docs/decisions/D08-sector-size-constraint.md): four sectors, three nodes
// each, is the smallest map Generate can ever produce a legal graph for.
const minSupportedNodes = 12

// validate reports whether p is well-formed enough to attempt generation at
// all — this is a caller-error check, not a generation failure, so it never
// counts against MaxAttempts.
func (p Params) validate() error {
	if p.Nodes < minSupportedNodes {
		return fmt.Errorf("gen: Nodes must be at least %d (D8: four sectors of at least three nodes each), got %d", minSupportedNodes, p.Nodes)
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
