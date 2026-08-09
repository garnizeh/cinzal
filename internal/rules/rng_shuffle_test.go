package rules

import (
	"slices"
	"testing"
)

// TestPartialFisherYatesConsumesExactlyMinKLen pins the index-cost
// guarantee RFC-001 §6.4 mandates for Torn Map, node layout, and every D03
// selection row: exactly min(k, len(candidates)) draws, no more.
func TestPartialFisherYatesConsumesExactlyMinKLen(t *testing.T) {
	cases := []struct {
		name       string
		candidates int
		k          int
		want       int
	}{
		{"k under pool size", 10, 4, 4},
		{"k equals pool size", 4, 4, 4},
		{"k exceeds pool size (truncation)", 3, 4, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rng := NewRNG(testSeed(9), 1)
			candidates := make([]int, tc.candidates)
			for i := range candidates {
				candidates[i] = i
			}

			result := PartialFisherYates(rng, PurposeItemTornMap, candidates, tc.k)

			if len(result) != tc.want {
				t.Fatalf("len(result) = %d, want %d", len(result), tc.want)
			}
			if got := rng.Consumed(PurposeItemTornMap); got != tc.want {
				t.Fatalf("Consumed(item.tornmap) = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestPartialFisherYatesSelectsWithoutDuplication asserts the selected
// prefix is a genuine subset of the candidate set — every element unique,
// none invented.
func TestPartialFisherYatesSelectsWithoutDuplication(t *testing.T) {
	rng := NewRNG(testSeed(10), 1)
	candidates := []int{100, 101, 102, 103, 104, 105}

	result := PartialFisherYates(rng, PurposeEventDragnet, candidates, 4)

	seen := map[int]bool{}
	for _, v := range result {
		if seen[v] {
			t.Fatalf("result %v contains duplicate value %d", result, v)
		}
		seen[v] = true
		if !slices.Contains([]int{100, 101, 102, 103, 104, 105}, v) {
			t.Fatalf("result contains %d, which is not one of the original candidates", v)
		}
	}
}

// TestPartialFisherYatesDeterministic asserts the same seed/round/purpose
// against the same candidate slice always yields the same selection.
func TestPartialFisherYatesDeterministic(t *testing.T) {
	seed := testSeed(11)

	run := func() []int {
		rng := NewRNG(seed, 4)
		candidates := []int{0, 1, 2, 3, 4, 5, 6, 7}
		out := PartialFisherYates(rng, PurposeEventFestival, candidates, 3)
		cp := make([]int, len(out))
		copy(cp, out)
		return cp
	}

	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("length mismatch: %v vs %v", a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("result diverged at %d: %v vs %v", i, a, b)
		}
	}
}

// TestShuffleConstrainedRespectsQuota asserts each category contributes
// exactly its quota, and nothing else.
func TestShuffleConstrainedRespectsQuota(t *testing.T) {
	type card struct {
		id       int
		category int
	}

	pool := make([]card, 0, 24)
	for cat := range 4 {
		for i := range 6 {
			pool = append(pool, card{id: cat*6 + i, category: cat})
		}
	}

	rng := NewRNG(testSeed(12), 0)
	quota := map[int]int{0: 3, 1: 3, 2: 3, 3: 3}

	selected := ShuffleConstrained(rng, PurposeDeckEventSelect, PurposeDeckEventOrder, pool, func(c card) int { return c.category }, quota)

	if len(selected) != 12 {
		t.Fatalf("len(selected) = %d, want 12", len(selected))
	}

	counts := map[int]int{}
	seen := map[int]bool{}
	for _, c := range selected {
		if seen[c.id] {
			t.Fatalf("card %d selected twice", c.id)
		}
		seen[c.id] = true
		counts[c.category]++
	}
	for cat, want := range quota {
		if counts[cat] != want {
			t.Errorf("category %d contributed %d cards, want %d", cat, counts[cat], want)
		}
	}
}

// TestShuffleConstrainedIndexCost pins D03's decomposed cost: selection
// draws sum to the total quota (partial Fisher-Yates per category, k == pool
// size per category here so no truncation), and ordering draws are exactly
// len(selected)-1 (a full Fisher-Yates).
func TestShuffleConstrainedIndexCost(t *testing.T) {
	type card struct{ category int }

	pool := []card{{0}, {0}, {0}, {1}, {1}, {1}, {1}, {1}}
	quota := map[int]int{0: 3, 1: 4}

	rng := NewRNG(testSeed(13), 0)
	selected := ShuffleConstrained(rng, PurposeDeckIncidentSelect, PurposeDeckIncidentOrder, pool, func(c card) int { return c.category }, quota)

	if len(selected) != 7 {
		t.Fatalf("len(selected) = %d, want 7", len(selected))
	}
	if got := rng.Consumed(PurposeDeckIncidentSelect); got != 7 {
		t.Errorf("Consumed(deck.incident.select) = %d, want 7 (3 + 4)", got)
	}
	if got := rng.Consumed(PurposeDeckIncidentOrder); got != 6 {
		t.Errorf("Consumed(deck.incident.order) = %d, want 6 (n-1 for n=7)", got)
	}
}

// TestShuffleConstrainedDeterministic asserts the same seed reproduces the
// same final deck order.
func TestShuffleConstrainedDeterministic(t *testing.T) {
	type card struct{ id, category int }

	newPool := func() []card {
		return []card{{0, 0}, {1, 0}, {2, 0}, {3, 1}, {4, 1}, {5, 1}}
	}
	quota := map[int]int{0: 2, 1: 2}
	seed := testSeed(14)

	run := func() []int {
		rng := NewRNG(seed, 7)
		selected := ShuffleConstrained(rng, PurposeDeckEventSelect, PurposeDeckEventOrder, newPool(), func(c card) int { return c.category }, quota)
		ids := make([]int, len(selected))
		for i, c := range selected {
			ids[i] = c.id
		}
		return ids
	}

	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("result diverged at %d: %v vs %v", i, a, b)
		}
	}
}
