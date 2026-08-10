package gen

import (
	"slices"
	"testing"
)

// countingRand wraps a Rand, counting how many draws it served — this
// package's own tests need to pin exact draw counts (RFC-001 §6.4's
// consumption formulas), independently of internal/rules' own *RNG-backed
// wiring test.
func countingRand(seed int) (Rand, *int) {
	base := newTestRand(seed)
	count := 0
	return func(purpose string, n int) int {
		count++
		return base(purpose, n)
	}, &count
}

func TestFullShuffleDrawCount(t *testing.T) {
	sizes := map[string]int{"empty": 0, "single": 1, "many": 7}

	for name, size := range sizes {
		rand, count := countingRand(0)
		items := make([]int, size)
		for i := range items {
			items[i] = i
		}
		fullShuffle(rand, "test", items)

		want := max(size-1, 0)
		if *count != want {
			t.Errorf("%s: fullShuffle consumed %d draws, want %d (len(items)-1, floor 0)", name, *count, want)
		}
	}
}

func TestFullShuffleIsAPermutation(t *testing.T) {
	rand, _ := countingRand(1)
	items := []int{0, 1, 2, 3, 4, 5, 6}
	original := slices.Clone(items)

	fullShuffle(rand, "test", items)

	slices.Sort(items)
	if !slices.Equal(items, original) {
		t.Fatalf("fullShuffle produced %v, which is not a permutation of %v", items, original)
	}
}

func TestPartialShuffleDrawCountAndSelection(t *testing.T) {
	rand, count := countingRand(2)
	items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8}
	k := 3

	partialShuffle(rand, "test", items, k)

	if *count != k {
		t.Fatalf("partialShuffle(k=%d) consumed %d draws, want %d", k, *count, k)
	}

	// The selected prefix and the untouched remainder together must still
	// be exactly the original set — partial shuffle swaps in place, it
	// never drops or duplicates an element.
	all := slices.Clone(items)
	slices.Sort(all)
	want := []int{0, 1, 2, 3, 4, 5, 6, 7, 8}
	if !slices.Equal(all, want) {
		t.Fatalf("partialShuffle(k=%d) result %v is not a permutation of %v", k, items, want)
	}

	// The selected prefix items[:k] must be pairwise distinct (selection
	// without replacement).
	seen := map[int]bool{}
	for _, v := range items[:k] {
		if seen[v] {
			t.Fatalf("partialShuffle(k=%d) selected prefix %v has a duplicate", k, items[:k])
		}
		seen[v] = true
	}
}

func TestPartialShuffleZeroIsNoOp(t *testing.T) {
	rand, count := countingRand(3)
	items := []int{0, 1, 2, 3}
	original := slices.Clone(items)

	partialShuffle(rand, "test", items, 0)

	if *count != 0 {
		t.Fatalf("partialShuffle(k=0) consumed %d draws, want 0", *count)
	}
	if !slices.Equal(items, original) {
		t.Fatalf("partialShuffle(k=0) mutated items: got %v, want unchanged %v", items, original)
	}
}

func TestRandomSpanningTreeSingleItemIsNoOp(t *testing.T) {
	rand, count := countingRand(4)
	called := false
	randomSpanningTree(rand, "test", []int{0}, func(parent, child int) { called = true })

	if called {
		t.Fatal("randomSpanningTree with one item called connect, want no-op")
	}
	if *count != 0 {
		t.Fatalf("randomSpanningTree with one item consumed %d draws, want 0", *count)
	}
}

func TestRandomSpanningTreeConnectsEveryItemOnce(t *testing.T) {
	rand, _ := countingRand(5)
	items := []int{0, 1, 2, 3, 4, 5}

	edges := 0
	seenAsChild := map[int]bool{}
	seenNodes := map[int]bool{items[0]: true}

	randomSpanningTree(rand, "test", items, func(parent, child int) {
		edges++
		if seenAsChild[child] {
			t.Fatalf("item %d was attached to the tree more than once", child)
		}
		seenAsChild[child] = true
		if !seenNodes[parent] {
			t.Fatalf("edge (%d -> %d) attaches to a parent not yet in the tree", parent, child)
		}
		seenNodes[child] = true
	})

	if edges != len(items)-1 {
		t.Fatalf("randomSpanningTree produced %d edges, want %d (len(items)-1)", edges, len(items)-1)
	}
	if len(seenNodes) != len(items) {
		t.Fatalf("randomSpanningTree left %d/%d items disconnected", len(items)-len(seenNodes), len(items))
	}
}
