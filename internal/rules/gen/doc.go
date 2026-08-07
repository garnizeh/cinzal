// Package gen generates the match map: an undirected graph satisfying every
// constraint in GDD §6, plus a deterministic 2D layout.
//
// It is part of the pure rules core and carries the same import restrictions:
// no I/O, no clock, no ambient randomness. All draws go through the seeded RNG
// threaded in by the caller.
//
// The layout is generated here rather than at render time, and that is
// deliberate. GDD §7.1 gives a Rumoured node a position on the map but no
// edges, so coordinates are part of what the projection discloses. They must be
// derived from the seed and stable across fog states, or a node's dot would
// move when it went Rumoured to Known (docs/decisions, D10 — still open).
package gen
