package opsmetrics

// BytesPerInitialCall and BytesPerResolveCall are the per-call allocation
// costs D45's allocation-share formula multiplies by an exact call count,
// rather than attempting to attribute bytes to one fold among several
// concurrently running ones (see this package's doc comment for why the
// latter is structurally the wrong measurement).
//
// Sourced from internal/rules/resolve_bench_test.go: a deterministic,
// GC-disabled measurement (debug.SetGCPercent(-1), runtime.MemStats.
// TotalAlloc before/after) of one fixed match — players=4, seed=[32]byte{1},
// game.DefaultConfig() — rather than a timing benchmark. Allocation byte
// count is exact given deterministic code, unlike wall-clock time, so this
// needs no bench-compare-style noise tolerance.
//
// BytesPerInitialCall is dominated by map generation (Generate's own retry
// loop for node-type assignment can run 1-66 tries per topology, per
// internal/rules/gen's own documented measurement; see that package's
// bench_test.go), which is why this number is far larger than
// BytesPerResolveCall despite both being "one call" in the formula above —
// that asymmetry is a real property of NewMatch's cost, not a measurement
// artifact. TestResolveAllocationBudget re-measures against these same
// pinned (seed, cfg, players) inputs on every test run and fails — not
// skips — if either constant has drifted by more than 10%: the PR that
// changes Resolve's or NewMatch's allocation shape is the PR that must
// update these constants, in the same diff.
//
// Both are measured over the same fixed match runner-generated on the
// commit that set them; re-measuring with a different seed or player count
// will read differently — this is a reference point for EstimateFoldBytes'
// order-of-magnitude estimate, not a claim that every fold costs exactly
// this much.
const (
	BytesPerInitialCall uint64 = 4_663_352
	BytesPerResolveCall uint64 = 42_765
)

// EstimateFoldBytes is D45's numerator: the exact call count (the caller
// already knows how many Resolve calls its own fold replayed — no
// instrumentation of Resolve itself is needed) times the two offline-measured
// per-call constants above, plus the one initial() call every fold makes
// exactly once.
//
// resolveCalls is the number of Resolve calls the fold being measured made —
// len(log) for internal/match/fold.Fold, or cfg.Rounds for cmd/simulate's own
// per-match sequence (RunMatch runs exactly cfg.Rounds Resolve calls for a
// match that reaches completion, the only kind RunMatch returns without
// error).
func EstimateFoldBytes(resolveCalls int) uint64 {
	if resolveCalls <= 0 {
		return BytesPerInitialCall
	}
	return BytesPerInitialCall + uint64(resolveCalls)*BytesPerResolveCall
}
