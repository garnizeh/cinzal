# D45 — What emits the fold metrics, and what is "a dashboard" in a one-binary topology?

**Status:** decided
**Blocks:** [#320](https://github.com/garnizeh/cinzal/issues/320) (fold-metrics instrumentation), [#322](https://github.com/garnizeh/cinzal/issues/322) (`cmd/replay`), and M3's fourth exit criterion ([#331](https://github.com/garnizeh/cinzal/issues/331))
**Decided:** 2026-08-26
**Issue:** [#308](https://github.com/garnizeh/cinzal/issues/308)

## The question

RFC §7.3 declines to build a snapshot layer and stakes that refusal on two numbers: p99 fold duration (trigger at 50 ms) and fold allocation as a share of heap churn (trigger at 20%). RFC §17 names both as metrics that matter and names no metrics system to carry them. RFC §18 names the entire production topology as one binary and one Postgres — a dashboard is not in it. Roadmap M3 requires both metrics "wired from day one," and its fourth exit criterion requires them "visible on a dashboard, with the §7.3 thresholds marked."

Three sub-questions, per the issue: what emits the metrics; what "a dashboard" means without a third piece of infrastructure; and how "fold allocation as a share of heap churn" — a ratio, not a timer or a gauge — is actually computed without a naive `ReadMemStats` stopping the world on the read path.

## Why it is open

**These two metrics are the entire mitigation for risk P7** (roadmap §6: *"Fold performance forces a snapshot layer under production pressure … Mitigation: metrics from M3, thresholds pre-declared."*) and the sole falsifiability trigger for §7.3's no-snapshot decision. If M3 ships without a real reading of both numbers, that mitigation is a paragraph, not a fact.

**Choosing the exporter is choosing the deployment.** §18's argument against Redis (§4, D20) is the same argument against Prometheus + Grafana to reach two numbers: it doubles the operational surface of a project whose one-binary-one-Postgres story is a selling point.

**The allocation ratio has no obvious correct implementation**, and the three off-the-shelf options each answer a different question than the one §7.3 asks: `testing.AllocsPerRun` in a benchmark measures the right thing but never runs in production, which is exactly where P7 fires; `runtime/metrics` sampling with the fold's own allocation attributed by a delta around the call is production-safe but its attribution is approximate the moment two matches are folding concurrently, which in production is the normal case, not an edge case; a `pprof`-based periodic profile is heavier and samples rather than measures.

**And there is a fourth constraint the issue doesn't name but the repository already answers: there is no HTTP server yet.** `internal/web` and `cmd/server` are `doc.go` and an empty `main` until M5. M3's own exit demonstration ([#331](https://github.com/garnizeh/cinzal/issues/331)) already resolves this in its own text — the numbers come from `cmd/simulate` and `cmd/replay` sweeps, not from live traffic, and the artefact must be "committed or reachable," not merely observed once. Any answer to "what is a dashboard" that assumes a running server is answering a question M3 does not have.

## Options considered for the emitter

- **Prometheus `client_golang` + `/metrics`.** Real dependency, and adopting the exposition format telegraphs a scrape target and eventually a Prometheus server — the exact "third piece of infrastructure" §18 argues against, for a two-number problem.
- **OpenTelemetry.** Heavier SDK, exporter configuration, and the same eventual-collector assumption as Prometheus, with more moving parts.
- **`slog` lines aggregated downstream.** RFC §17 already commits to `slog`-to-stdout-as-JSON for logging, so this is the option with the lowest apparent cost. But computing a running p50/p99 or an allocation ratio "downstream" means some other system does the aggregation — reintroducing infrastructure outside "one binary, one Postgres" that RFC §18 never named, and it cannot produce the reproducible, offline-generatable artefact M3's exit demo requires: a log platform query is not something `cmd/simulate --sweep` can emit on its own.
- **stdlib `expvar`.** Closest in spirit to the answer below, but it only solves "publish an already-computed number as JSON at an HTTP path" — a serving problem M3 does not have (no server exists). It does nothing for the actual hard parts: percentile estimation and the allocation-share computation. Adopting the package buys a global string-keyed registry this decision does not need.
- **A small dedicated package, stdlib-only.** Solves exactly the two hard parts (percentile estimation, allocation-share computation) and nothing else; adds no exposition format, no exporter, no scrape target, and no assumption about how or whether it is ever served over HTTP. Chosen — see Decision.

## Decision

**A new leaf package, `internal/opsmetrics`.** It imports nothing but the standard library (`sync/atomic`, `time`, `runtime/metrics`, `html/template`, `sort`) — no `internal/game`, no `internal/rules`. This is deliberate: the package can never see a `MatchState`, a `PlayerView`, or a `match_id`, so it needs no entry in `scripts/check-fog-boundary.sh`'s `FORBIDDEN` set and is safe for `internal/web` to import directly once M5 exists, the same way `internal/match` already is under D01. It holds two things:

**1. `FoldStats` — an aggregator, one instance per process.**

```go
func (s *FoldStats) Observe(dur time.Duration, estimatedBytes uint64)
func (s *FoldStats) Snapshot() FoldSnapshot   // p50, p99, count, allocation share, both §7.3 thresholds
```

Duration samples are kept in a fixed-capacity reservoir (10,000 slots, standard reservoir sampling once the count exceeds capacity) so `Snapshot` sorts a bounded slice for exact p50/p99 rather than maintaining an unbounded one in a long-running process. `internal/opsmetrics` carries no determinism obligation — it never influences game state, is never imported by `rules` or `bots`, and RFC §6.3's purity requirement does not reach it — so the reservoir's random replacement can use whatever the standard library's `math/rand` offers with no seeding concern.

`FoldStats` also runs one internal ticker (started explicitly via `opsmetrics.StartHeapChurnSampler(interval)`, never on package `init`) that reads the cumulative `/gc/heap/allocs:bytes` sample from `runtime/metrics.Read` — documented by the standard library as cheap to read repeatedly, unlike `runtime.ReadMemStats`, which briefly stops the world — and records the delta since the last tick as that window's total heap churn.

**2. Two named constants, citing RFC §7.3 directly in their doc comments:** `FoldDurationP99Threshold = 50 * time.Millisecond` and `FoldAllocShareThreshold = 0.20`.

**Fold duration** is timed around the whole `internal/match.Fold(...)` call — start before it is invoked, stop when it returns — never around the individual `rules.Resolve` calls inside it. This covers `initial()` plus every `Resolve` call in one number, matching what §7.3's own arithmetic table treats as one fold, and matching what a caller actually experiences as latency. `Fold`'s own signature keeps the guarantee #319 requires — no `Effects`, no `*Store`, no provider — so the timer cannot live inside it; instead `internal/match` gains a second, thin function:

```go
// internal/match
func FoldMeasured(seed [32]byte, cfg game.Config, players int, log rules.OrderLog) (rules.MatchState, []game.Event, error) {
    start := time.Now()
    state, events, err := Fold(seed, cfg, players, log)
    if err == nil {
        opsmetrics.Default.Observe(time.Since(start), opsmetrics.EstimateFoldBytes(len(log)))
    }
    return state, events, err
}
```

This fits `internal/match`'s already-stated charter — it is the one package that owns every effect in the system (§7.4), and recording an ops metric is exactly that kind of effect, the same category as the telemetry write M4's tick already owns. `cmd/replay` calls `FoldMeasured`, never `Fold`, wherever it needs a dump. M4's tick, once it exists, does the same with no new decision required — the wiring built here is reused, not re-derived.

**Fold allocation share is never measured in situ**, for the reason named above: a before/after `runtime/metrics` delta around one fold is polluted by every other goroutine's allocation during that window, and in production multiple matches ticking concurrently is the ordinary case. Instead:

```text
estimatedFoldBytes(log) = BytesPerInitialCall + len(log) × BytesPerResolveCall
share(window)           = Σ estimatedFoldBytes over folds observed in window
                           ────────────────────────────────────────────────
                                  Δ(/gc/heap/allocs:bytes) over the same window
```

`BytesPerInitialCall` and `BytesPerResolveCall` are constants in `internal/opsmetrics`, sourced from a **deterministic, noise-free measurement**, not a timing benchmark: a new `internal/rules` test (`resolve_bench_test.go`, alongside a `BenchmarkResolve` for human eyeballing) that disables the GC for the duration of the measurement (`debug.SetGCPercent(-1)`, restored after), calls `initial`/`Resolve` some fixed number of times in a single goroutine, and reads `runtime.MemStats.TotalAlloc` before and after. Allocation *count and bytes* are deterministic given deterministic code — unlike wall-clock time, this needs no `bench-compare`-style noise tolerance. The benchmark cases mirror `internal/rules/gen/bench_test.go`'s own pattern: one row per player count (2p–5p), with parameters pinned to literal values rather than read live from `game.DefaultConfig()`, for the same reason that file already states — a future `Config` tuning pass must not silently move this baseline underneath an unrelated PR.

**The self-check that keeps the constant honest:** `TestResolveAllocationBudget` in the same file asserts the freshly measured bytes-per-call figure is within 10% of the hardcoded `BytesPerResolveCall`/`BytesPerInitialCall` constants, and **fails**, not skips, on drift — the PR that changed `Resolve`'s allocation shape is the PR that must update the constant, in the same diff, the same discipline `scripts/bots-isolation-allowlist.txt` already enforces for a different hazard.

The numerator's `len(log)` is exact — the caller already knows how many rounds it asked `Fold` to replay, no instrumentation of `Resolve` itself is needed. The denominator is exact and process-wide. Neither side of the ratio depends on attributing bytes to one goroutine among several, which is the property the naive approach cannot offer.

**What "a dashboard" means for M3's exit criterion.** Not a live HTTP page — none can exist before M5's `internal/web`, and [#331](https://github.com/garnizeh/cinzal/issues/331) already frames the exit demonstration around `cmd/simulate`/`cmd/replay` sweeps, an artefact "committed or reachable," reproducible by rerunning a stated command. `internal/opsmetrics` therefore also exports a pure renderer:

```go
func (s FoldSnapshot) WriteHTML(w io.Writer) error
```

Built on stdlib `html/template`, not `templ` — `make generate`'s `templ` step is a no-op until M5 (CONTRIBUTING.md), and pulling it forward for a two-number report would be a dependency this milestone does not need. The rendered page shows p50/p99 fold duration and the allocation share, each next to its RFC §7.3 threshold with a pass/fail mark — never wired to an alert or a page, matching §17's statement that a determinism mismatch is the only page-worthy event. `cmd/simulate` and `cmd/replay` each gain a flag (e.g. `--fold-metrics-html out.html`) that calls this after a run; the M3 exit demonstration commits its output to the evidence directory alongside the sweep command that produced it, per #331's own requirement that the artefact be reproducible, not a screenshot.

**`cmd/simulate` and `cmd/replay` both emit the metrics, through the identical package.** `cmd/replay` reaches `internal/opsmetrics` transitively via `internal/match.FoldMeasured`. `cmd/simulate` cannot import `internal/match` — the simulate-dependency gate (#199) restricts it to `rules`, `bots`, `game`, `internal/telemetry` — so it calls `opsmetrics.Default.Observe` directly, once per simulated match, timing that match's own sequence of `Resolve` calls the same way `FoldMeasured` times `Fold`'s. This is the one required change to an existing gate: `scripts/check-simulate-deps.sh`'s exact allowed set gains `internal/opsmetrics`, in the same PR that wires this in, with the reason stated inline the way the bots-isolation allow-list already requires for its own widenings. `internal/opsmetrics` was deliberately not folded into `internal/telemetry` (already on `cmd/simulate`'s allowed list) to avoid this gate edit — but `internal/telemetry` is explicitly forbidden to `internal/web` by the fog gate (its own doc comment: "never imported by internal/render or internal/web"), and the whole point of a dashboard artefact is that `internal/web` (in M5) or a CLI report renderer (now) must be able to reach it. A single new leaf package that both `cmd/simulate` and `internal/web` can import cleanly costs one gate-file edit; reusing `internal/telemetry` would have cost a permanent, awkward carve-out in a fog gate whose whole value is having no carve-outs.

**The forward-looking auth story, decided now so it is not invented under pressure once M5 exists.** If/when `internal/web` exposes `FoldSnapshot` live, it is reached at an operator-only path, authenticated by a single shared bearer token compared in constant time against an `OPS_METRICS_TOKEN` env var — not the player-facing OTP/session system (§12), which is the wrong trust domain for an operator credential and doesn't yet have any concept of an operator role. The page carries only the two aggregate numbers and their thresholds, never a `match_id`, a seat, or any other per-match label — by construction, since `internal/opsmetrics` has no type capable of holding one. This closes the issue's own concern directly: a per-match latency histogram is a cardinality bomb and a disclosure surface; a process-wide aggregate that structurally cannot be labelled by match is neither. Building the live page itself is not part of M3 and needs no new decision when M5 gets there — it is a small task reusing what this decision already specifies.

## Reasoning

**Why not reuse an existing package instead of adding one.** `internal/telemetry` was the closest candidate — already on `cmd/simulate`'s allowed list, already the "one computation, three sinks" precedent (D34) — but it is fog-gate-forbidden to `internal/web`/`internal/render` because it can reach `rules.MatchState`. Ops metrics need the opposite property: reachable from the one layer telemetry is barred from, and never reachable from `rules`/`bots` at all. Those are different placement constraints, not the same one twice, so this needed its own package rather than an extension of D34's.

**Why the allocation ratio splits into an offline-measured constant times an exact count, rather than any form of in-situ sampling.** Every in-situ option — a before/after `runtime/metrics` delta, `ReadMemStats`, a periodic pprof profile — measures "how much did the heap grow while this fold ran," which in a concurrent process is not the same question as "how much did this fold itself allocate." The gap between those two questions is exactly the attribution problem the issue raises, and it does not shrink by sampling harder or reading a cheaper counter — it is structural. Moving the per-call cost to an isolated, deterministic, single-goroutine measurement removes the concurrency variable from that half of the computation entirely, leaving only two exact quantities (a call count, a process-wide counter) to divide.

**Why duration gets simple in-situ timing but allocation does not.** Wall-clock duration is a fact about one goroutine's own execution and is unaffected by what other goroutines are doing at the same moment — `time.Since(start)` around one call is exact regardless of concurrency. Allocation is a fact about a shared heap that every goroutine mutates, so measuring it the same way (a before/after read) inherits noise duration never has. The issue's own third sub-question — "this is the one metric in the list that is not a timer or a gauge" — is precisely this asymmetry, and the decision treats the two metrics differently because they are different kinds of quantity, not out of inconsistency.

**Why a static, reproducible artefact rather than waiting for a live dashboard.** The exit criterion's wording ("visible on a dashboard") reads as if a server already exists; the roadmap's own dependency graph (§7) says it does not until M5, three milestones later. Reading the criterion literally would either block M3 on standing up `internal/web` early — reordering the roadmap's own stated build order (§21: rules → bots/simulation → persistence → round lifecycle → playable web, riskiest unknowns first) — or invent a throwaway HTTP server whose only job is to die once M5's real one exists. #331 already resolves this the right way: the numbers come from `cmd/simulate`/`cmd/replay` sweeps, and the artefact has to be reproducible, not perpetually running. This decision's renderer serves both that near-term need and the eventual M5 live page from the same `FoldSnapshot`, so nothing built now is thrown away later.

## Consequences

- New package `internal/opsmetrics`: `FoldStats`, `FoldSnapshot`, the two threshold constants, `EstimateFoldBytes`, `StartHeapChurnSampler`, and `WriteHTML`. Stdlib-only; no entry needed in the fog gate's `FORBIDDEN` set.
- `internal/match` gains `FoldMeasured`, a thin wrapper around `Fold` that owns the metrics-recording effect the same way it owns every other effect; `Fold` itself is untouched, preserving #319's no-provider-reachable guarantee.
- `internal/rules` gains `resolve_bench_test.go`: a `BenchmarkResolve` (2p–5p, pinned parameters, mirroring `internal/rules/gen/bench_test.go`'s existing pattern) and `TestResolveAllocationBudget`, a deterministic, GC-disabled byte-count measurement that fails the build if `BytesPerInitialCall`/`BytesPerResolveCall` have drifted by more than 10% from the hardcoded constants — the PR that changes `Resolve`'s allocation shape must update the constant in the same diff.
- `scripts/check-simulate-deps.sh`'s allowed dependency set gains `internal/opsmetrics`, alongside the reasoning this document gives, in the PR that wires #320.
- `cmd/replay` and `cmd/simulate` each gain a flag that renders a `FoldSnapshot` to a static HTML file via `WriteHTML`; the M3 exit demonstration commits that output, plus the command that generated it, to the evidence directory.
- No live HTTP surface is built in M3. When M5 builds `internal/web`, exposing `FoldSnapshot` live needs only a handler and the `OPS_METRICS_TOKEN` auth this document already specifies — no new decision.
- Reversible at low cost: `internal/opsmetrics` has no dependents outside `internal/match` and the two `cmd/*` binaries, and nothing it stores is authoritative — losing its in-memory state loses only the current window's numbers, never anything the order log or a replay depends on.
