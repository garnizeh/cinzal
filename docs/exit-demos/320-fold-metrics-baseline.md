# #320's own baseline: cmd/simulate's fold duration and allocation-share reference

**Issue:** [#320](https://github.com/garnizeh/cinzal/issues/320)
**Milestone:** M3 — Persistence

**Not the milestone exit demonstration.** [#331](https://github.com/garnizeh/cinzal/issues/331) is M3's own fourth exit criterion — both processes' numbers, a proper [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md)-scale sweep, and the "within an order of magnitude → file a decision" rule applied. This document is #320's own acceptance-criterion bullet ("a short document... records the baseline"), satisfied for the one process that exists today — `cmd/replay` is #322's own still-open task ([CLAUDE.md](../../CLAUDE.md)'s repository-state paragraph: `cmd/replay` is a `doc.go` plus an empty `main`). Once #322 builds it, #331 records `cmd/replay`'s own "fold-only reference" number alongside this one and runs the auto-file rule over both.

## Provenance

| | |
|---|---|
| Git SHA | `5e9cb1e77f00111abe485802f9f1d5319f14bb2b` (regenerated after CodeRabbit's review findings — raised on PR #398, but landed in the follow-up PR #400 after a staging mistake dropped them from #398's own merge — for `percentile`'s ceil-based nearest-rank, `StartHeapChurnSampler`'s stop-takes-a-final-sample, and this file's own HTML `Label`) |
| Root seed (default) | `cinzal-simulate-default-root-seed-v1` (SHA-256: `e4c50a633bfa5326029d36fcc00ea91af9510b523942539d4f89ed107866aa09`) |
| Matches | 3,000 |
| Players | 4 |
| Bot tier | Drifter |
| `--workers` | 1 (see "Concurrent workers materially inflate the tail" below) |
| Heap-churn sample interval | 10ms (`opsmetrics.StartHeapChurnSampler`, started automatically by `--fold-metrics-html`) |

```bash
bin/simulate --matches 3000 --players 4 --bots drifter --sweep LeaseCostPerBlock=3 \
  --out docs/exit-demos/320/p4-drifter-seed1.csv \
  --fold-metrics-html docs/exit-demos/320/cmd-simulate-fold-metrics.html \
  --workers 1
```

CSV and rendered dashboard artefact in [`320/`](320/): `p4-drifter-seed1.csv`, `cmd-simulate-fold-metrics.html`.

## The numbers

| metric | value | §7.3 threshold | verdict |
|---|---|---|---|
| fold duration p50 | 8.66 ms | — | — |
| fold duration p99 | 48.68 ms | 50 ms | PASS, by 1.32 ms |
| fold allocation share (cmd/simulate — bot+telemetry-diluted reference) | 94.24% | 20% (deferred to M5, D51) | reference only — not compared |

Fold duration here is `internal/rules.NewMatch` (standing in for RFC §7.1's `initial()`) plus the sum of all 15 `internal/rules.Resolve` calls per match — explicitly **not** `RunMatch`'s bot `Decide` time, which has no equivalent inside a real fold at all (a replay reconstructs state from an existing order log; nothing decides an order inside it). `cmd/simulate/driver.go`'s `RunMatch` accumulates these two spans separately for exactly this reason.

Allocation share is `opsmetrics.EstimateFoldBytes` summed over all 3,000 matches, divided by the process-wide `/gc/heap/allocs:bytes` delta sampled every 10ms for the sweep's duration (`opsmetrics.StartHeapChurnSampler`) — D45's exact computation, D51's exact labeling.

## What changed since the first capture

CodeRabbit's review of PR #398 found three real bugs in the measurement code itself. A staging mistake dropped the fixes from #398's own merge commit despite its message describing them (see #399); they actually landed in the follow-up PR #400. Either way, this is why this document's numbers moved slightly from their first-committed values (p50 8.98ms→8.66ms, p99 52.45ms→48.68ms, allocation share 94.26%→94.24%) and this artifact was regenerated rather than left stale:

- `opsmetrics.percentile` truncated (`int(p*n)`) instead of applying the ceil-based nearest-rank formula its own doc comment already specified — selecting the wrong sample rank by one position (e.g. p99 over 100 samples returned the maximum instead of the 99th-smallest). Fixed; see `internal/opsmetrics/stats_test.go`'s regression cases.
- `StartHeapChurnSampler`'s `stop` only signaled its goroutine to exit — it took no final sample — so a sweep finishing before the first 10ms tick would report zero heap-churn samples despite real fold allocations. `stop` now blocks until one final synchronous sample lands. This baseline's own sweep ran long enough that the bug never triggered here, but `cmd/simulate/run.go` also now calls `stop` explicitly before rendering rather than only via `defer` at function return, closing the gap for a shorter sweep.
- The HTML `Label` cmd/simulate passes carried only D51's "bot+telemetry-diluted reference" qualifier, not the process name — ambiguous when the artifact is viewed without this Markdown file. Now reads "cmd/simulate — bot+telemetry-diluted reference".

None of these fixes changes this document's own conclusions below — the duration finding and the allocation-share finding are both still real, both still #331's to weigh, not #320's.

## Two genuine findings, not a clean baseline — both belong to #331, not #320

**p99 duration sits close to the 50ms line in either direction, not comfortably clear of it.** Three runs of the identical deterministic match sequence (same root seed, same 3,000 derived match seeds, `--workers 1`) on the same machine read 52.45ms, 49.94ms, and 48.68ms (this document's own recorded value, after the `percentile` ceil-based nearest-rank fix — see "What changed" below; the fix moves which sample rank is selected by one position out of 3,000, not the wall-clock noise between runs). All three runs replay byte-identical game content; the spread is wall-clock measurement noise between runs on a shared machine (CPU scheduling, thermal state), not a difference in what was measured. Every reading places p99 within roughly 5% of the threshold, nowhere close to "an order of magnitude" — squarely inside #331's own auto-file trigger once it runs its own larger, more controlled measurement, regardless of which side of the line any single run lands on.

**Concurrent workers materially inflate the tail further.** The same sweep with `cmd/simulate`'s default worker count (`GOMAXPROCS`, 28 on the machine that produced this baseline) reported p50 = 32.7ms and p99 = 275.4ms — five times over threshold. That gap is goroutine scheduling contention among 28 concurrently-folding matches sharing a fixed core count, not a fact about one fold's own cost; `--workers 1` isolates the latter, which is what §7.3's "one fold" arithmetic is actually about. But it means neither number here describes what a busy `cmd/simulate` sweep (or a busy production server handling many concurrent folds) would report for the same match distribution — worth someone's attention once `cmd/server` exists in M5.

**The tail is plausibly driven by map-generation retry variance, a cost this repository has already documented elsewhere.** `internal/rules/gen/bench_test.go`'s own comment on `assignNodeTypes`: "1-66 tries, measured across ~9000 topologies... not a function of the algorithm's speed, only of which specific graph got generated." A scratch measurement taken while sizing `opsmetrics.BytesPerInitialCall` (seed=1..5, players=4, not committed — see that constant's own doc comment in `internal/opsmetrics/estimate.go`) found `NewMatch`'s own allocation ranging from 747KB to 4.66MB across five seeds purely from retry-count variance — a 6x spread from the map generator alone, before Resolve's own cost is added. A p99 over 3,000 matches is exactly the kind of measurement built to catch that tail; this run did.

**94.24% allocation share is far higher than D51 itself anticipated for cmd/simulate.** D51's own text: *"cmd/simulate spends most of its allocation on bot Decide calls and internal/telemetry's CSV rows, so its share is small for reasons that have nothing to do with fold cost"* and *"cmd/simulate's share, labeled 'bot+telemetry-diluted reference' — expected far smaller [than cmd/replay's ~1.0]."* The measured number here is close to 1.0, not "far smaller." A plausible explanation, not yet verified: `opsmetrics.BytesPerInitialCall` was itself measured from a GC-disabled, retry-heavy `NewMatch` call (see the finding above) and is large enough (4.66MB) that `EstimateFoldBytes`'s numerator — one `BytesPerInitialCall` plus 15×`BytesPerResolveCall` per match — may already be comparable to or larger than what bot `Decide` and telemetry actually allocate per match, which would make the "diluted" label's own premise (fold is the minority of the denominator) not hold at players=4 with Drifter, the cheapest tier. **This is exactly the kind of surprising real number a baseline document exists to surface rather than smooth over**, and it is #331's call whether it changes how the two labels are read, not #320's.

**Neither finding is #320's to resolve.** #331's own acceptance criteria already state the duration rule this triggers: *"If either measurement is within an order of magnitude of its threshold, that is called out as a finding and filed as a decision."* The allocation-share finding has no equivalent auto-file rule (D51 explicitly took share comparisons against 20% out of scope for M3), but is recorded here for whoever picks up #331 to weigh. This document records what #320's own wiring produced and flags both findings forward rather than hiding or resolving them.

## What this document does not do

- **Does not sweep 2, 3, or 5 players.** #331's own baseline should cover GDD §6.1's full player range; this document exists to prove #320's mechanism produces real numbers, not to pre-empt #331's own measurement plan.
- **Does not use D35's 10,000-match scale.** 3,000 was chosen to keep this baseline reproducible in under a minute at `--workers 1`; #331 should use whatever scale its own measurement plan calls for.
- **Does not record `cmd/replay`'s own "fold-only reference" number.** That process does not exist yet (#322).
