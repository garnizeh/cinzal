package rules

import (
	"fmt"
	"sort"
)

// PartialFisherYates performs RFC-001 §6.4's mandated bounded selection: a
// partial Fisher-Yates over candidates, which the caller must already have
// sorted by a stable, deterministic key (NodeID, EdgeID, ...) — building
// that slice by ranging a map is the §6.3 hazard wearing a new hat.
//
// It shuffles the first min(k, len(candidates)) positions of candidates in
// place and returns that prefix, consuming exactly that many purpose draws.
// This is the same method Torn Map, node layout, and every D03 selection
// row use: inventing a second correct-looking selection method anywhere in
// this package would recreate the desync risk this shape exists to
// prevent.
//
// The returned slice aliases candidates' backing array. Callers must pass a
// freshly built slice — never one shared with State — and must not retain
// the result past the resolution step that produced it.
func PartialFisherYates[T any](rng *RNG, purpose Purpose, candidates []T, k int) []T {
	n := min(k, len(candidates))
	for i := 0; i < n; i++ {
		j := i + rng.Next(purpose, len(candidates)-i)
		candidates[i], candidates[j] = candidates[j], candidates[i]
	}
	return candidates[:n]
}

// ShuffleConstrained builds a category-quota-constrained deck (RFC-001
// §6.4, completed by D03 — docs/decisions/D03-rng-consumption-table.md): it
// selects exactly quota[c] items of each category c from pool via
// PartialFisherYates (at selectPurpose, one call per category, candidates
// in each category taken in pool order), then fully shuffles the selected
// set into final round order — a complete Fisher-Yates, len(selected)-1
// draws, at orderPurpose.
//
// D03 split selection and ordering into two purposes deliberately: a
// category-quota bug and an ordering bug must be distinguishable from the
// trace alone. pool must already be sorted by canonical card order; the
// per-category split preserves that order until the final shuffle
// randomizes it. Categories are visited in ascending order, not map
// iteration order, so the draw sequence stays deterministic regardless of
// how quota's keys happen to hash.
//
// A quota is a fixed contract, not a best-effort target: a category with
// fewer than quota[c] items panics rather than silently returning a short
// deck. PartialFisherYates's own min(k, n) truncation is correct for a
// bounded selection like Torn Map, where the candidate pool can vary; a
// short deck here would both violate D03's exact per-category counts and
// consume fewer draws than the §6.4 table predicts, desynchronising a
// replay in a later round with no obvious cause at the point it surfaces.
func ShuffleConstrained[T any](rng *RNG, selectPurpose, orderPurpose Purpose, pool []T, categoryOf func(T) int, quota map[int]int) []T {
	byCategory := make(map[int][]T, len(quota))
	for _, item := range pool {
		c := categoryOf(item)
		byCategory[c] = append(byCategory[c], item)
	}

	categories := make([]int, 0, len(quota))
	total := 0
	for c, k := range quota {
		categories = append(categories, c)
		total += k
	}
	sort.Ints(categories)

	selected := make([]T, 0, total)
	for _, c := range categories {
		if len(byCategory[c]) < quota[c] {
			panic(fmt.Sprintf("rules: ShuffleConstrained(%q): category %d has %d items, quota requires %d", selectPurpose, c, len(byCategory[c]), quota[c]))
		}
		selected = append(selected, PartialFisherYates(rng, selectPurpose, byCategory[c], quota[c])...)
	}

	for i := 0; i < len(selected)-1; i++ {
		j := i + rng.Next(orderPurpose, len(selected)-i)
		selected[i], selected[j] = selected[j], selected[i]
	}

	return selected
}
