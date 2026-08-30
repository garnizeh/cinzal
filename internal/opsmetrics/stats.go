package opsmetrics

import (
	"math"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// reservoirCapacity bounds FoldStats' duration sample buffer. Snapshot sorts
// a bounded slice for exact p50/p99 rather than an unbounded one in a
// long-running process — a server that folds for weeks must not grow this
// buffer without limit (D45).
const reservoirCapacity = 10_000

// FoldStats aggregates fold duration and allocation-share inputs for one
// process. One instance per process; Default (below) is the package-level
// instance every production caller uses — internal/match/fold.FoldMeasured,
// and cmd/simulate directly, since it cannot import internal/match (D45).
//
// The uint64 fields are declared first for sync/atomic's alignment
// requirement on 32-bit platforms (sync/atomic's own doc: "the first word in
// an allocated struct, array, or slice can be relied upon to be 64-bit
// aligned").
type FoldStats struct {
	allocBytes uint64 // atomic: Σ estimatedFoldBytes across every Observe call
	heapChurn  uint64 // atomic: Σ heap-churn deltas StartHeapChurnSampler has recorded

	mu        sync.Mutex
	durations []time.Duration // reservoir, len <= reservoirCapacity
	seen      uint64          // total Observe calls ever made, including ones the reservoir discarded
}

// NewFoldStats returns a fresh, empty aggregator. Production code uses the
// package-level Default; tests that must not share state with other tests
// (or with a concurrently running production caller) construct their own.
func NewFoldStats() *FoldStats {
	return &FoldStats{}
}

// Default is the package-level aggregator every production caller observes
// into. internal/opsmetrics has no game/rules dependency and can never hold
// a match_id, so one process-wide instance carries no disclosure risk the
// way a per-match one would.
var Default = NewFoldStats()

// SetDefault replaces the package-level Default instance and returns a
// function that restores whatever was installed before this call. Exists so
// a test that needs to assert something precise about "the one sample
// FoldMeasured just recorded" can do so against an isolated FoldStats
// instead of sharing Default's reservoir with every other test in the same
// binary — production code never calls this. Not safe against a concurrent
// Observe call landing mid-swap; callers rely on Go's default sequential
// test execution (no t.Parallel) the same way the rest of this repository's
// test suites do for shared package-level state.
func SetDefault(s *FoldStats) (restore func()) {
	prev := Default
	Default = s
	return func() { Default = prev }
}

// Observe records one completed fold's duration and estimated allocation.
// Safe for concurrent use: cmd/simulate calls this from RunMany's worker
// goroutines, one call per completed match; internal/match/fold.FoldMeasured
// calls it once per Fold invocation.
func (s *FoldStats) Observe(dur time.Duration, estimatedBytes uint64) {
	s.mu.Lock()
	n := s.seen
	s.seen++
	switch {
	case len(s.durations) < reservoirCapacity:
		s.durations = append(s.durations, dur)
	default:
		// Standard reservoir sampling: the (n+1)-th observation (0-indexed
		// n) replaces a uniformly-chosen existing slot with probability
		// reservoirCapacity/(n+1), keeping every observation seen so far
		// equally likely to survive in the final reservoir. This package
		// carries no RFC §6.3 determinism obligation (doc.go) — it never
		// influences game state and is never imported by rules or bots — so
		// an unseeded math/rand draw here needs no consumption-table row.
		j := rand.Int63n(int64(n) + 1)
		if j < reservoirCapacity {
			s.durations[j] = dur
		}
	}
	s.mu.Unlock()

	atomic.AddUint64(&s.allocBytes, estimatedBytes)
}

// FoldSnapshot is a point-in-time read of a FoldStats instance: the two RFC
// §7.3 numbers, ready to render or assert against. Label distinguishes which
// process produced it, per D51 — "fold-only reference" for cmd/replay's own
// snapshot, "bot+telemetry-diluted reference" for cmd/simulate's — set by the
// caller, never by FoldStats itself, which has no notion of which binary it
// is running inside.
type FoldSnapshot struct {
	Label string

	Count int // number of Observe calls this snapshot's percentiles are drawn from
	P50   time.Duration
	P99   time.Duration

	// AllocShare is Σ estimatedFoldBytes / Δheap-churn over this stats
	// instance's whole lifetime (StartHeapChurnSampler's first tick through
	// Snapshot's own call). Zero, with HasAllocShare false, if the heap-churn
	// sampler was never started or has not ticked yet — a computed 0.0 here
	// would be indistinguishable from "fold is free," which is not a claim
	// this package makes.
	AllocShare    float64
	HasAllocShare bool
}

// Snapshot reads the current aggregate state. Percentiles are computed by
// sorting a copy of the duration reservoir — O(n log n) in reservoirCapacity
// at most, and Snapshot is called at most once per report, never per fold.
//
// Fails closed by construction: Count is the literal number of Observe
// calls seen (capped display-wise at nothing — it is the true total, even
// past reservoirCapacity), so a caller asserting "metrics were actually
// recorded" checks Count > 0 rather than trusting a non-zero P99, which a
// snapshot over zero samples would report as exactly 0 — comfortably, and
// misleadingly, under FoldDurationP99Threshold.
func (s *FoldStats) Snapshot() FoldSnapshot {
	s.mu.Lock()
	n := s.seen
	durations := make([]time.Duration, len(s.durations))
	copy(durations, s.durations)
	s.mu.Unlock()

	snap := FoldSnapshot{Count: int(n)}

	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		snap.P50 = percentile(durations, 0.50)
		snap.P99 = percentile(durations, 0.99)
	}

	churn := atomic.LoadUint64(&s.heapChurn)
	if churn > 0 {
		alloc := atomic.LoadUint64(&s.allocBytes)
		snap.AllocShare = float64(alloc) / float64(churn)
		snap.HasAllocShare = true
	}

	return snap
}

// percentile returns the p-th percentile (0 <= p <= 1) of a sorted,
// non-empty duration slice, using the nearest-rank method: the smallest
// value at or past ceil(p*n) samples. This is exact given the reservoir's
// contents — no interpolation, which would invent a duration nothing was
// ever observed to take. rank is 1-based per the nearest-rank definition
// (max(1, ceil(p*n))); sorted is 0-indexed, hence rank-1 below. Previously
// this truncated (int(p*n)) instead of applying ceil, which silently
// selected the wrong sample — e.g. p50 over 2 samples chose the upper one
// instead of the lower.
func percentile(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	rank := int(math.Ceil(p * float64(n)))
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return sorted[rank-1]
}
