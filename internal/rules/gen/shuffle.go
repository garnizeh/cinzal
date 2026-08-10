package gen

import "slices"

// fullShuffle performs a complete Fisher-Yates shuffle of items in place,
// consuming exactly len(items)-1 draws under purpose (0 if len(items) <= 1)
// — RFC-001 §6.4's "n-1 draws for n items" full-shuffle shape, the same one
// D03 mandates for final deck order (docs/decisions/D03-rng-consumption-table.md).
// items must already be in a stable, deterministic starting order (ascending
// ID) — building it by ranging a map would be the RFC-001 §6.3 hazard
// wearing a new hat.
func fullShuffle[T any](rand Rand, purpose string, items []T) {
	for i := 0; i < len(items)-1; i++ {
		j := i + rand(purpose, len(items)-i)
		items[i], items[j] = items[j], items[i]
	}
}

// randomSpanningTree builds a random spanning tree over items (already in a
// stable, deterministic order), rooted at items[0]: shuffle items[1:]
// fully, then attach each in turn to a uniformly random member of the
// growing "already attached" set, calling connect(parent, child) once per
// edge — len(items)-1 edges, always. Cost: full shuffle of items[1:]
// (len(items)-2 draws, floor 0) plus one bounded draw per non-root item
// (len(items)-1 draws). This is RFC-001 §6.4's mandated selection shape,
// extended from "pick k of n" to "attach n-1 items to a growing tree" — the
// same extension edges.go's degree-aware sibling, nodeSpanningTree, makes
// again for node-level trees where a plain uniform pick would ignore GDD
// §6.1 constraint 2's degree cap.
func randomSpanningTree[T any](rand Rand, purpose string, items []T, connect func(parent, child T)) {
	if len(items) <= 1 {
		return
	}

	rest := slices.Clone(items[1:])
	fullShuffle(rand, purpose, rest)

	added := make([]T, 1, len(items))
	added[0] = items[0]
	for _, child := range rest {
		parent := added[rand(purpose, len(added))]
		connect(parent, child)
		added = append(added, child)
	}
}
