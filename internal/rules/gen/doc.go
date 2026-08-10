// Package gen generates the match map: an undirected graph satisfying every
// constraint in GDD §6, plus a deterministic 2D layout.
//
// It is part of the pure rules core and carries the same import restrictions as
// internal/rules: the standard library and internal/game only, and from the
// standard library nothing that performs I/O, tells the time, or generates
// randomness. All draws go through the seeded RNG threaded in by the caller.
//
// The layout is generated here rather than at render time, and that is
// deliberate. GDD §7.1 gives a Rumoured node a position on the map but no
// edges, so coordinates are part of what the projection discloses. They must be
// derived from the seed and stable across fog states, or a node's dot would
// move when it went Rumoured to Known (docs/decisions/D10-map-layout.md).
//
// This package does not import internal/rules, even though internal/rules
// will import this one: rules.MatchState's initial construction needs a
// generated Graph, so the dependency runs rules -> gen, and Go forbids the
// reverse. Concretely, that means this package cannot name *rules.RNG or
// rules.Purpose directly — Generate takes its randomness as the Rand
// function type instead (see rand.go), and internal/rules/rng_purpose.go
// re-exports this package's purpose strings as typed rules.Purpose
// constants once it wires a real *rules.RNG to them.
package gen

// Touched only to verify CodeRabbit review now reaches this package (see #108, #109).
