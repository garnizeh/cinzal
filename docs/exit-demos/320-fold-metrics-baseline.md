# #320's own baseline: cmd/simulate's fold duration and allocation-share reference

**Issue:** [#320](https://github.com/garnizeh/cinzal/issues/320)
**Milestone:** M3 — Persistence

**Not the milestone exit demonstration.** [#331](https://github.com/garnizeh/cinzal/issues/331) is M3's own fourth exit criterion — both processes' numbers, a proper [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md)-scale sweep, and the "within an order of magnitude → file a decision" rule applied. This document is #320's own acceptance-criterion bullet ("a short document... records the baseline"), satisfied for the one process that exists today — `cmd/replay` is #322's own still-open task ([CLAUDE.md](../../CLAUDE.md)'s repository-state paragraph: `cmd/replay` is a `doc.go` plus an empty `main`). Once #322 builds it, #331 records `cmd/replay`'s own "fold-only reference" number alongside this one and runs the auto-file rule over both.

## Provenance

| | |
|---|---|
| Git SHA | `12a25ca1f1bc409b1f0ba05e9f663ec98ad7178d` |
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
| fold duration p50 | 8.98 ms | — | — |
| fold duration p99 | 52.45 ms | 50 ms | FAIL, by 2.45 ms |
| fold allocation share (bot+telemetry-diluted reference) | 94.26% | 20% (deferred to M5, D51) | reference only — not compared |

Fold duration here is `internal/rules.NewMatch` (standing in for RFC §7.1's `initial()`) plus the sum of all 15 `internal/rules.Resolve` calls per match — explicitly **not** `RunMatch`'s bot `Decide` time, which has no equivalent inside a real fold at all (a replay reconstructs state from an existing order log; nothing decides an order inside it). `cmd/simulate/driver.go`'s `RunMatch` accumulates these two spans separately for exactly this reason.

Allocation share is `opsmetrics.EstimateFoldBytes` summed over all 3,000 matches, divided by the process-wide `/gc/heap/allocs:bytes` delta sampled every 10ms for the sweep's duration (`opsmetrics.StartHeapChurnSampler`) — D45's exact computation, D51's exact labeling.

## Two genuine findings, not a clean baseline — both belong to #331, not #320

**p99 duration lands just over the 50ms line, not comfortably under it.** A second run of the identical deterministic match sequence (same root seed, same 3,000 derived match seeds, `--workers 1`) read p99 = 49.94ms — under threshold by 0.06ms — while the run recorded above read 52.45ms. Both runs replay byte-identical game content; the difference is wall-clock measurement noise between two runs on a shared machine (CPU scheduling, thermal state), not a difference in what was measured. Either reading places p99 within roughly 5% of the threshold, nowhere close to "an order of magnitude" — squarely inside #331's own auto-file trigger once it runs its own larger, more controlled measurement.

**Concurrent workers materially inflate the tail further.** The same sweep with `cmd/simulate`'s default worker count (`GOMAXPROCS`, 28 on the machine that produced this baseline) reported p50 = 32.7ms and p99 = 275.4ms — five times over threshold. That gap is goroutine scheduling contention among 28 concurrently-folding matches sharing a fixed core count, not a fact about one fold's own cost; `--workers 1` isolates the latter, which is what §7.3's "one fold" arithmetic is actually about. But it means neither number here describes what a busy `cmd/simulate` sweep (or a busy production server handling many concurrent folds) would report for the same match distribution — worth someone's attention once `cmd/server` exists in M5.

**The tail is plausibly driven by map-generation retry variance, a cost this repository has already documented elsewhere.** `internal/rules/gen/bench_test.go`'s own comment on `assignNodeTypes`: "1-66 tries, measured across ~9000 topologies... not a function of the algorithm's speed, only of which specific graph got generated." A scratch measurement taken while sizing `opsmetrics.BytesPerInitialCall` (seed=1..5, players=4, not committed — see that constant's own doc comment in `internal/opsmetrics/estimate.go`) found `NewMatch`'s own allocation ranging from 747KB to 4.66MB across five seeds purely from retry-count variance — a 6x spread from the map generator alone, before Resolve's own cost is added. A p99 over 3,000 matches is exactly the kind of measurement built to catch that tail; this run did.

**94.26% allocation share is far higher than D51 itself anticipated for cmd/simulate.** D51's own text: *"cmd/simulate spends most of its allocation on bot Decide calls and internal/telemetry's CSV rows, so its share is small for reasons that have nothing to do with fold cost"* and *"cmd/simulate's share, labeled 'bot+telemetry-diluted reference' — expected far smaller [than cmd/replay's ~1.0]."* The measured number here is close to 1.0, not "far smaller." A plausible explanation, not yet verified: `opsmetrics.BytesPerInitialCall` was itself measured from a GC-disabled, retry-heavy `NewMatch` call (see the finding above) and is large enough (4.66MB) that `EstimateFoldBytes`'s numerator — one `BytesPerInitialCall` plus 15×`BytesPerResolveCall` per match — may already be comparable to or larger than what bot `Decide` and telemetry actually allocate per match, which would make the "diluted" label's own premise (fold is the minority of the denominator) not hold at players=4 with Drifter, the cheapest tier. **This is exactly the kind of surprising real number a baseline document exists to surface rather than smooth over**, and it is #331's call whether it changes how the two labels are read, not #320's.

**Neither finding is #320's to resolve.** #331's own acceptance criteria already state the duration rule this triggers: *"If either measurement is within an order of magnitude of its threshold, that is called out as a finding and filed as a decision."* The allocation-share finding has no equivalent auto-file rule (D51 explicitly took share comparisons against 20% out of scope for M3), but is recorded here for whoever picks up #331 to weigh. This document records what #320's own wiring produced and flags both findings forward rather than hiding or resolving them.

## What this document does not do

- **Does not sweep 2, 3, or 5 players.** #331's own baseline should cover GDD §6.1's full player range; this document exists to prove #320's mechanism produces real numbers, not to pre-empt #331's own measurement plan.
- **Does not use D35's 10,000-match scale.** 3,000 was chosen to keep this baseline reproducible in under a minute at `--workers 1`; #331 should use whatever scale its own measurement plan calls for.
- **Does not record `cmd/replay`'s own "fold-only reference" number.** That process does not exist yet (#322).
