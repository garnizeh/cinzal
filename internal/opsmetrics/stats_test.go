package opsmetrics

import (
	"sync"
	"testing"
	"time"
)

// TestSnapshotZeroSamples is #320's fails-closed acceptance criterion: a
// FoldStats that has recorded nothing must report that state explicitly
// (Count == 0, HasAllocShare == false) rather than a P99 of exactly 0 that a
// caller comparing against FoldDurationP99Threshold would read as a pass.
func TestSnapshotZeroSamples(t *testing.T) {
	s := NewFoldStats()
	snap := s.Snapshot()

	if snap.Count != 0 {
		t.Errorf("Count = %d, want 0", snap.Count)
	}
	if snap.P50 != 0 || snap.P99 != 0 {
		t.Errorf("P50/P99 = %v/%v, want 0/0 on an empty snapshot", snap.P50, snap.P99)
	}
	if snap.HasAllocShare {
		t.Error("HasAllocShare = true on a snapshot with no heap-churn samples ever recorded")
	}

	// The fails-closed property in practice: a caller must not be able to
	// mistake "no data" for "passed." WriteHTML's own test (snapshot_test.go)
	// checks this renders as "no samples," not PASS.
	if snap.Count > 0 && snap.P99 <= FoldDurationP99Threshold {
		t.Fatal("unreachable: Count is 0 above")
	}
}

// TestObserveAndSnapshotCount is the basic accounting property: every
// Observe call is counted, whether or not the reservoir has room to keep its
// own duration sample.
func TestObserveAndSnapshotCount(t *testing.T) {
	s := NewFoldStats()
	const n = 250
	for i := 0; i < n; i++ {
		s.Observe(time.Duration(i+1)*time.Millisecond, 1000)
	}

	snap := s.Snapshot()
	if snap.Count != n {
		t.Errorf("Count = %d, want %d", snap.Count, n)
	}
	if snap.P99 == 0 {
		t.Error("P99 = 0 after n observations with non-zero durations")
	}
}

// TestSnapshotPercentiles checks the nearest-rank computation against a
// small, fully-known dataset: 100 samples of 1ms..100ms. percentile uses the
// nearest-rank method, 1-based rank = max(1, ceil(p*n)), returned as
// sorted[rank-1]. p50: ceil(0.50*100)=50 → sorted[49] = 50ms. p99:
// ceil(0.99*100)=99 → sorted[98] = 99ms — the 99th-smallest of 100, not the
// maximum. Exact values documented here so a future change to the
// percentile method shows up as an intentional diff, not a silent drift.
func TestSnapshotPercentiles(t *testing.T) {
	s := NewFoldStats()
	for i := 1; i <= 100; i++ {
		s.Observe(time.Duration(i)*time.Millisecond, 0)
	}

	snap := s.Snapshot()
	if snap.Count != 100 {
		t.Fatalf("Count = %d, want 100", snap.Count)
	}

	wantP50 := 50 * time.Millisecond // rank = ceil(0.50*100) = 50 → sorted[49] = 50ms (0-indexed)
	wantP99 := 99 * time.Millisecond // rank = ceil(0.99*100) = 99 → sorted[98] = 99ms
	if snap.P50 != wantP50 {
		t.Errorf("P50 = %v, want %v", snap.P50, wantP50)
	}
	if snap.P99 != wantP99 {
		t.Errorf("P99 = %v, want %v", snap.P99, wantP99)
	}
}

// TestPercentileRegressionTwoSamplesP50 is CodeRabbit's own regression case
// on PR #398: with exactly 2 samples, p50's nearest rank is
// ceil(0.5*2)=1 — the lower sample. The pre-fix truncating formula
// (int(p*n), not ceil) computed rank=1 too here by coincidence at n=2, but
// diverges for other n (see TestSnapshotPercentiles above) — this case
// pins the n=2 boundary explicitly since it is the smallest n where p50 is
// ever ambiguous between "lower" and "upper".
func TestPercentileRegressionTwoSamplesP50(t *testing.T) {
	s := NewFoldStats()
	s.Observe(10*time.Millisecond, 0)
	s.Observe(20*time.Millisecond, 0)

	snap := s.Snapshot()
	want := 10 * time.Millisecond // rank = ceil(0.5*2) = 1 → sorted[0] = 10ms, the lower sample
	if snap.P50 != want {
		t.Errorf("P50 over 2 samples = %v, want %v (the lower sample)", snap.P50, want)
	}
}

// TestPercentileRegressionHundredSamplesP99 is CodeRabbit's own regression
// case on PR #398: with 100 samples, p99's nearest rank is
// ceil(0.99*100)=99 — the 99th-smallest, not the maximum (rank 100). The
// pre-fix truncating formula returned sorted[99], the maximum, for this
// exact n — the bug this fix and TestSnapshotPercentiles' updated
// expectation both correct.
func TestPercentileRegressionHundredSamplesP99(t *testing.T) {
	s := NewFoldStats()
	for i := 1; i <= 100; i++ {
		s.Observe(time.Duration(i)*time.Millisecond, 0)
	}

	snap := s.Snapshot()
	want := 99 * time.Millisecond // rank = ceil(0.99*100) = 99 → sorted[98] = 99ms, not the max (100ms)
	if snap.P99 != want {
		t.Errorf("P99 over 100 samples = %v, want %v (not the maximum, 100ms)", snap.P99, want)
	}
}

// TestReservoirCapBounded asserts the duration reservoir never grows past
// reservoirCapacity, even when Observe is called far more times than that —
// D45's own reason for a bounded reservoir: a long-running process must not
// grow this buffer without limit. Count still reflects the true total.
func TestReservoirCapBounded(t *testing.T) {
	s := NewFoldStats()
	const n = reservoirCapacity + 5_000
	for i := 0; i < n; i++ {
		s.Observe(time.Duration(i)*time.Nanosecond, 0)
	}

	if len(s.durations) != reservoirCapacity {
		t.Errorf("reservoir holds %d samples, want exactly %d (capped)", len(s.durations), reservoirCapacity)
	}

	snap := s.Snapshot()
	if snap.Count != n {
		t.Errorf("Count = %d, want %d (true total, independent of reservoir cap)", snap.Count, n)
	}
}

// TestObserveConcurrentSafe exercises Observe from many goroutines at once —
// cmd/simulate's RunMany calls it from worker goroutines, one per completed
// match. Run with -race; a data race here would otherwise show up only under
// production load.
func TestObserveConcurrentSafe(t *testing.T) {
	s := NewFoldStats()
	const goroutines = 50
	const perGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				s.Observe(time.Duration(i+1)*time.Microsecond, 100)
			}
		}(g)
	}
	wg.Wait()

	snap := s.Snapshot()
	want := goroutines * perGoroutine
	if snap.Count != want {
		t.Errorf("Count = %d, want %d", snap.Count, want)
	}
}

// TestAllocShareArithmetic checks Snapshot's division directly against a
// FoldStats whose heap-churn accumulator is set without going through the
// ticker (heapchurn_test.go covers the ticker itself).
func TestAllocShareArithmetic(t *testing.T) {
	s := NewFoldStats()
	s.Observe(time.Millisecond, 4_000)
	s.Observe(time.Millisecond, 6_000)
	s.heapChurn = 20_000 // Σ estimatedFoldBytes = 10_000; share = 0.5

	snap := s.Snapshot()
	if !snap.HasAllocShare {
		t.Fatal("HasAllocShare = false with a non-zero heap-churn accumulator")
	}
	const want = 0.5
	if snap.AllocShare != want {
		t.Errorf("AllocShare = %v, want %v", snap.AllocShare, want)
	}
}

// TestDefaultIsUsable is a smoke test that the package-level Default
// instance behaves like any other FoldStats — production code (FoldMeasured,
// cmd/simulate) observes into it directly.
func TestDefaultIsUsable(t *testing.T) {
	if Default == nil {
		t.Fatal("Default is nil")
	}
	before := Default.Snapshot().Count
	Default.Observe(time.Millisecond, 1)
	after := Default.Snapshot().Count
	if after != before+1 {
		t.Errorf("Default.Snapshot().Count = %d after one Observe, want %d", after, before+1)
	}
}
