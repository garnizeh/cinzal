package fold

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/opsmetrics"
)

// BenchmarkFold and BenchmarkFoldMeasured exist side by side so
// benchstat/eyeballing shows FoldMeasured's own overhead over bare Fold
// directly — #320's acceptance criterion that "a benchmark shows the
// instrumented fold's own overhead against the uninstrumented one." Both
// share the same fixed 15-round, 2-player idle log built once outside the
// timed loop.
//
// These are ordinary wall-clock benchmarks (unlike
// internal/rules/resolve_bench_test.go's TestResolveAllocationBudget, which
// needs a GC-disabled, noise-free byte count) — run with `go test -bench`,
// not part of `go test`'s default run, and not gated by bench-compare
// (§7.3's arithmetic doesn't require internal/rules/gen-style regression
// tracking here; this benchmark exists for a human to read the overhead
// off, per the acceptance criterion's own wording).
//
// Measured at authoring time (`go test -run '^$' -bench . -benchtime 200x
// -count 3 ./internal/match/fold/...`, amd64, GOMAXPROCS=28): BenchmarkFold
// ≈ 2.08-2.10 ms/op across three runs, BenchmarkFoldMeasured ≈ 1.98-2.12
// ms/op — the two ranges overlap, so FoldMeasured's own overhead (one
// time.Now()/time.Since() pair, one mutex-guarded reservoir append, one
// atomic add) is not distinguishable from run-to-run noise at this scale,
// comfortably inside any reasonable "instrumentation costs more than a
// stated fraction of the fold" budget. An earlier run at -benchtime 20x (too
// few iterations) read as a spurious ~9% overhead — the kind of noise a
// small sample produces, not a real cost; re-run with more iterations before
// trusting a single benchstat comparison here.

func BenchmarkFold(b *testing.B) {
	cfg := game.DefaultConfig()
	seed := [32]byte{11}
	players := 2
	log := idleFullLog(cfg, players)

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := Fold(seed, cfg, players, log); err != nil {
			b.Fatalf("Fold() = %v", err)
		}
	}
}

// BenchmarkFoldMeasured times FoldMeasured against an isolated
// opsmetrics.FoldStats (via SetDefault) rather than the shared package-level
// Default, so a benchmark run does not leave thousands of samples behind in
// a Default any other test in the same binary might read — and so repeated
// b.Loop() iterations don't grow the reservoir past its cap mid-benchmark in
// a way that would change Observe's own cost partway through (append vs.
// reservoir-replacement is not the same number of instructions).
func BenchmarkFoldMeasured(b *testing.B) {
	cfg := game.DefaultConfig()
	seed := [32]byte{11}
	players := 2
	log := idleFullLog(cfg, players)

	scoped := opsmetrics.NewFoldStats()
	restore := opsmetrics.SetDefault(scoped)
	defer restore()

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := FoldMeasured(seed, cfg, players, log); err != nil {
			b.Fatalf("FoldMeasured() = %v", err)
		}
	}
}
