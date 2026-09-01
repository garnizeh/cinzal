// Package opsmetrics carries the two metrics RFC-001 §7.3 stakes its
// no-snapshot decision on: p50/p99 fold duration and fold allocation as a
// share of process heap churn. §7.3 declines to build a snapshot layer and
// names these two numbers as the falsifiability trigger — "if p99 fold
// duration passes 50 ms, or fold allocation exceeds 20% of total heap
// churn... it gets built." Without a real reading of both, that mitigation
// (roadmap §6, risk P7) is a paragraph, not a fact.
//
// # Why this package exists rather than reusing one that already reads
// # match state
//
// [D45] chose a new, stdlib-only leaf package over Prometheus, OpenTelemetry,
// slog-aggregated-downstream, or expvar — every one of those either
// telegraphs a scrape target and a third piece of infrastructure §18's
// one-binary-one-Postgres topology deliberately avoids, or solves the wrong
// half of the problem (publishing an already-computed number, not computing
// percentiles or an allocation ratio). This package imports nothing but the
// standard library — no internal/game, no internal/rules — so it can never
// hold a MatchState, a PlayerView, or a match_id. That is deliberate and
// structural, not a convention: internal/web may import this package
// directly once M5 exists, the same way internal/match already is under D01,
// with no fog-gate entry required, because there is no rules type reachable
// from anywhere in this package for the gate to forbid.
//
// # What FoldStats measures, and why the two metrics are computed
// # differently
//
// Fold duration is a fact about one goroutine's own execution and is exact
// regardless of what else the process is doing — time.Since(start) around
// one call needs no correction for concurrency. Fold allocation is a fact
// about a shared heap every goroutine mutates, so an in-situ
// before/after read (runtime.ReadMemStats, or a runtime/metrics delta around
// one call) is polluted by every other goroutine's allocation during that
// same window — in production, multiple matches folding concurrently is the
// ordinary case, not an edge case. [D45] resolves this by splitting the
// ratio into two exact quantities that never need in-situ attribution:
//
//	estimatedFoldBytes(resolveCalls) = BytesPerInitialCall + resolveCalls × BytesPerResolveCall
//	share(window)                    = Σ estimatedFoldBytes over folds observed in window
//	                                    ────────────────────────────────────────────────
//	                                           Δ(/gc/heap/allocs:bytes) over the same window
//
// BytesPerInitialCall/BytesPerResolveCall are offline-measured constants
// (internal/rules/resolve_alloc_budget_amd64_test.go — amd64-only, issue
// #352), cross-checked by a test that fails the build on drift rather than
// silently accepting a stale figure. The
// numerator's call count is exact — the caller already knows how many rounds
// it asked Fold to replay. The denominator, /gc/heap/allocs:bytes, is a
// cumulative process-wide counter, cheap to read repeatedly per the standard
// library's own documentation (unlike runtime.ReadMemStats, which briefly
// stops the world) — sampled by a ticker (StartHeapChurnSampler), never on
// every fold.
//
// # What "process-wide" means for the number this package reports
//
// [D51] found the denominator above is a property of the *process*, not of
// fold cost alone — cmd/replay does almost nothing but fold (share ≈ 1.0,
// four to five times over §7.3's 20% trigger, by construction) and
// cmd/simulate spends most of its allocation on bot Decide calls and
// telemetry CSV rows (a small share, for reasons unrelated to fold cost).
// Neither is the production server §7.3's 20% figure was chosen against —
// that process does not exist until M5. So FoldSnapshot.WriteHTML never
// draws a pass/fail line against FoldAllocShareThreshold; it renders the
// share as a labeled reference measurement instead ("fold-only reference"
// for cmd/replay's own snapshot, "bot+telemetry-diluted reference" for
// cmd/simulate's), and the real comparison is deferred to M5. Duration keeps
// its pass/fail mark unchanged — the two-process problem does not touch a
// quantity that is exact per-goroutine regardless of what else shares the
// process.
//
// # No determinism obligation
//
// This package is never imported by internal/rules or internal/bots, never
// influences game state, and RFC §6.3's purity/determinism requirement does
// not reach it. FoldStats' reservoir sampling (once its duration buffer
// exceeds capacity) uses the standard library's unseeded math/rand with no
// determinism concern — replaying the same {seed, config, players, orderLog}
// still reproduces byte-identical game state; it may simply land in a
// different slot of an ops-metrics reservoir that no replay ever inspects.
//
// [D45]: docs/decisions/D45-fold-metrics-emitter-and-dashboard.md
// [D49]: docs/decisions/D49-fold-package-boundary.md (moved FoldMeasured to
// internal/match/fold; this package's own design is untouched by that move)
// [D51]: docs/decisions/D51-fold-allocation-share-denominator-scope.md
package opsmetrics
