# D51 — D45's allocation share has a process-wide denominator, and M3's two processes make it meaningless

**Status:** decided
**Blocks:** [#320](https://github.com/garnizeh/cinzal/issues/320) (fold-metrics instrumentation) and M3's fourth exit demonstration ([#331](https://github.com/garnizeh/cinzal/issues/331))
**Decided:** 2026-08-27
**Issue:** [#348](https://github.com/garnizeh/cinzal/issues/348)

## The question

[D45](D45-fold-metrics-emitter-and-dashboard.md) defines

```text
share(window) = Σ estimatedFoldBytes over folds observed in window
                 ────────────────────────────────────────────────
                     Δ(/gc/heap/allocs:bytes) over the same window
```

and calls both sides exact — true, but the denominator is `/gc/heap/allocs:bytes`, a **process-wide** counter, and D45 also names the only two M3 processes that will ever compute this ratio: `cmd/replay` and `cmd/simulate`. Neither is the process RFC §7.3's 20%-of-heap-churn trigger was written about. `cmd/replay` does almost nothing but fold, so its share sits near 1.0 by construction — four to five times over the line. `cmd/simulate` spends most of its allocation on bot `Decide` calls and `internal/telemetry`'s CSV rows, so its share is small for reasons that have nothing to do with fold cost. **What does M3 report, and does it get compared to 20% at all?**

## Why it is open

**§7.3's threshold means "fold allocation as a share of a production server's heap churn."** That server — `internal/web`, `cmd/server` — does not exist until M5 (§21's build order: rules → bots/simulation → persistence → round lifecycle → playable web). Every M3 process is either fold-only or fold-plus-unrelated-simulation-work; neither mixture is the one §7.3's 20 % figure was chosen against.

**Reading `cmd/replay`'s ~1.0 against 20% trips [#331](https://github.com/garnizeh/cinzal/issues/331)'s own auto-file rule on the first run, for the wrong reason.** #331: *"If either measurement is within an order of magnitude of its threshold, that is called out as a finding and filed as a decision."* A replay-process ratio near 1.0 clears that bar by construction, every time, regardless of how cheap or expensive folding actually is — the rule exists to catch a genuine finding, not a property of which binary produced the number.

**Reading `cmd/simulate`'s diluted share against 20% is the same failure in the other direction, and quieter.** It passes comfortably, but for a reason unrelated to fold cost: most of the denominator is bot decision-making and CSV writing. A pass/fail mark on that number would read as "folding is cheap in production" when it measures "simulation does other things too" — exactly the "we noticed and moved on" outcome #331 refuses.

**Averaging or blending the two invents the missing workload rather than admitting it's missing.** There is no principled weight between "100% fold" and "mostly not fold" without first deciding what fraction of a real request is fold — which is the RFC §18 stack's own allocation profile (HTTP handling, `templ` rendering, `pgx` queries, SSE), none of which exists yet to measure.

## Options considered

- **Compare `cmd/replay`'s share to 20% as-is.** Trips #331's auto-file rule on every run for a number that is a fact about `cmd/replay`, not about fold cost. Rejected.
- **Compare `cmd/simulate`'s share to 20% as-is.** Passes trivially and misreports why. Rejected.
- **Blend or weight the two into one number.** Requires guessing the real server's fold-vs-other allocation mix before `internal/web` exists — inventing the very quantity that's missing, with false precision attached. Rejected.
- **Measure over a synthetic mixed workload** (fold interleaved with a stated amount of non-fold allocation approximating M5's server) so the ratio has a defined meaning now. Considered directly — see Reasoning for why it is declined for M3, not dismissed outright.
- **Report both processes' shares as separately labeled bounds; compare only the duration threshold to §7.3 in M3; defer the 20% comparison to M5.** Chosen.

## Decision

**1. M3's dashboard artefact reports allocation share as two labeled numbers, never one, and neither is compared to §7.3's 20% line.**

- `cmd/replay`'s share, labeled **"fold-only ceiling"** — expected near 1.0, because a replay process does almost nothing else.
- `cmd/simulate`'s share, labeled **"bot+telemetry-diluted floor"** — expected far smaller, because its denominator includes bot `Decide` calls and CSV/telemetry writes a production request never makes.

Both go into the same baseline artefact #331 already requires, with the labels living in the same file as the numbers — the "measurement conditions travel with the measurement" discipline #331 already states, applied to what the *denominator* means as well as to the sweep's parameters.

**2. `FoldSnapshot.WriteHTML` (D45) renders the 50 ms duration line with its existing pass/fail mark; it renders both allocation-share numbers with no pass/fail mark against 20% — just the two labeled bounds and a one-line caption that the production ratio is unmeasured until M5.** This amends D45's rendering description (*"shows p50/p99 fold duration and the allocation share, each next to its RFC §7.3 threshold with a pass/fail mark"*) for the allocation-share half only; the duration half is untouched, for the reason D45 already gave — duration is a fact about one goroutine's own execution and is exact regardless of concurrency or process topology, so nothing about the two-process problem touches it.

**3. #331's "within an order of magnitude → file a decision" rule applies to the duration threshold only in M3.** The allocation-share comparison against 20% is out of scope for that rule until M5, because in M3 the comparison is never drawn — there is nothing for the rule to trip on. This is written into #331's acceptance criteria directly (see Consequences), so nobody re-derives it under pressure when `cmd/replay`'s ~1.0 shows up on the first run.

**4. The synthetic-mixed-workload option is declined for M3.** See Reasoning.

**5. The real comparison happens at M5**, once `cmd/server` exists and a single process actually mixes fold work with HTTP handling, `templ` rendering, `pgx` queries, and SSE — the workload §7.3's 20% figure was chosen against. M3's two bounds are not thrown away then: the real M5 ratio is expected to land between the floor and the ceiling recorded here, and a real ratio outside that bracket is itself a finding.

## Reasoning

**Why not the synthetic workload.** Constructing "a stated amount of non-fold allocation" means deciding, in advance of writing any of `internal/web`, how much a board render's HTTP handling, `templ` rendering, `pgx` query, and SSE push allocate relative to one fold — precisely the RFC §18 stack's own profile, which is unmeasurable before that code exists. A number assembled that way carries the same false precision as blending the two real bounds, just dressed as a controlled experiment instead of an average. Two honestly-labeled real bounds, both measured against a real process rather than an invented one, are more useful to a future reader than one synthetic number that looks calibrated and isn't. This is reversible: nothing here forecloses building a synthetic-workload harness later if M5 is delayed and an earlier read is needed — it is simply not the cheapest or most honest answer available now, with M5 three build-order steps away rather than open-ended.

**Why the two real bounds are worth keeping rather than reporting nothing.** They are not the production number, but they are not noise either: `cmd/replay`'s ~1.0 is a true fact about a fold-only process and an honest upper bound on what any process containing a fold could show; `cmd/simulate`'s number is a true fact about a process that does a specific, named set of other things. Both bracket the unknown production ratio without asserting where in that bracket it falls — which is exactly the caveat #331 already requires for the *duration* baseline's bot-driven population (*"they bound the shape, not the production value"*), extended to the allocation-share axis instead of dropped there.

**Why duration keeps its threshold comparison and allocation share loses it.** D45 already drew this line for measurement technique — duration is exact per-goroutine regardless of concurrency, allocation is a shared-heap fact polluted by every other goroutine's work in the same window. The same asymmetry reappears one level up, at the process-topology question this decision answers: a 50 ms trigger means the same thing whichever M3 process measures it, because nothing about *which other work shares the process* changes how long one fold call takes. A 20% trigger does not have that property — it is a ratio, and the other term of that ratio is exactly the thing that differs between `cmd/replay`, `cmd/simulate`, and the M5 server. Treating the two thresholds differently here is the same reasoning D45 already applied, not a new inconsistency.

## Consequences

- **[#331](https://github.com/garnizeh/cinzal/issues/331)'s acceptance criteria are edited in place** (per this repository's practice of correcting an existing exit-demo/task issue directly rather than leaving a stale spec standing, [D43](D43-row-1-unmeasurable-post-d39.md)'s precedent): the allocation-share bullet is replaced with "both `cmd/replay`'s and `cmd/simulate`'s shares are recorded, each labeled per D51 (ceiling / floor), with neither compared to the 20% line"; the "within an order of magnitude → file a decision" bullet is scoped to state it applies to the 50 ms duration threshold only in M3.
- **[#320](https://github.com/garnizeh/cinzal/issues/320)'s acceptance criteria gain the same scoping**: "the two §7.3 thresholds appear as named constants … surfaced in the dashboard artefact" is unchanged for duration; for allocation share the task implements the two-labeled-numbers, no-pass/fail rendering this decision specifies, not the single-number-with-mark D45's prose originally described.
- **D45's `WriteHTML` description is amended, not rewritten**: duration's pass/fail rendering stands as D45 specified it; allocation share's rendering is what this document's Decision §2 states. `internal/opsmetrics.FoldSnapshot` itself is untouched by this decision — no code exists yet for either issue, so there is nothing to file as a follow-up task the way D43 filed #289; the two blocked issues carry the updated spec directly.
- **No RFC or GDD text changes.** §7.3's stated policy — *"if p99 fold duration passes 50 ms, or fold allocation exceeds 20% of total heap churn … it gets built"* — is unamended and remains the M5 comparison; this decision is about how M3 measures and reports toward it, the same category of decision D45 itself was, which also needed no RFC/GDD edit. Companion doc stays at RFC r46 / GDD v2.32.
- **What would reopen this.** `cmd/server` existing and folding real traffic makes the M5 comparison possible and this decision's deferral moot for anything past that point; nothing here blocks that — it is the outcome this decision is deliberately waiting for. If M5 slips far enough that the no-snapshot decision needs re-examination before a real server exists, the synthetic-workload option considered and declined above is the documented fallback, not a new question.
- **Reversible at documentation cost only.** No code, golden fixture, or RNG index moves; the two labeled bounds this decision asks for are a superset of what D45's original single-number spec would have produced, so implementing #320 under this decision costs nothing to later collapse back to one number if a future decision decides the bracket isn't worth keeping.
