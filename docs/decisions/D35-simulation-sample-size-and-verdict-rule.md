# D35 — How many matches per configuration, and what makes a threshold verdict actionable?

**Status:** decided
**Blocks:** [#200](https://github.com/garnizeh/cinzal/issues/200) (`cmd/simulate`'s CSV columns), [#203](https://github.com/garnizeh/cinzal/issues/203)/[#204](https://github.com/garnizeh/cinzal/issues/204)/[#205](https://github.com/garnizeh/cinzal/issues/205) (the exit demonstrations that read the CSV), and through them M2's exit criteria
**Decided:** 2026-08-18
**Issue:** [#189](https://github.com/garnizeh/cinzal/issues/189)

## The question

The roadmap's M2 exit criteria are seven GDD §22 thresholds, each with an action attached if it trips (`cinzal-implementation-plan.md:246-262`). None of them says how many matches produce the number, which bot tier the number is read against, or what makes a number near the line an answer rather than noise. Three things need deciding together: match count per configuration, tier baseline, and verdict rule.

## Why it is open

**The thresholds are point estimates and the roadmap demands a verdict.** *"> 12 confrontations per match → raise node count"* (R9, GDD §20) has no stated precision. A sweep returning 12.3 is either a rule change or nothing, and nothing in the spec says which.

**The GDD already set a precedent and never explained it.** §20's R2 entry reports 3,000 matches per configuration across four setups and treats the result as decisive enough to reverse a design worry outright (`cinzal-gdd.md:1463-1472`). Whether 3,000 is the right count for a different, more sensitive metric is a separate question the R2 figure doesn't answer.

**RFC §16.4 already contradicts the R2 precedent with a different number, and downstream work has already committed to it.** The RFC's own worked example is `simulate --matches 10000 --players 4 --bots operator --sweep LeaseCostPerBlock=1,2,3,4,5 --out sweep.csv` (`cinzal-architecture-rfc.md:1473-1474`) — 10,000, not 3,000. This is not a hypothetical tension: [#200](https://github.com/garnizeh/cinzal/issues/200) cites it as its spec anchor and acceptance criterion (*"the RFC §16.4 invocation runs verbatim"*), and [#204](https://github.com/garnizeh/cinzal/issues/204) reproduces the same `--matches 10000` line as *"the RFC's own invocation, run verbatim."* A decision that landed on 3,000 would not be filling a gap, it would be overriding two issues that already treat 10,000 as settled.

**The tier changes the number, and the exit criteria don't say which tier.** RFC §14.3: Drifter is *"uniform random legal order, baseline for statistics"*; Operator *"plays the game the way the GDD says the game is meant to be played"* and is *"deliberately not superhuman."* R2's own numbers are explicitly random-walk numbers, with GDD's own caveat that real play pulls the count in both directions (`cinzal-gdd.md:1472`). A threshold about the map (R9) and a threshold about strategy (R11) may not want the same baseline — and [#203](https://github.com/garnizeh/cinzal/issues/203) and [#205](https://github.com/garnizeh/cinzal/issues/205) already assume a per-metric split *"per D35"* without this document existing yet to justify it.

**A sweep is many configurations, and some thresholds will trip by chance if each is read independently.** `--sweep LeaseCostPerBlock=1,2,3,4,5` at four player counts is twenty cells; the milestone checks seven thresholds against some of them. Whether that matters depends on what a tripped threshold triggers — here, a design change.

**Matches within one configuration are not independent in every metric.** *"Live leases at final scoring, per player: 2–4"* and *"routes cancelled mid-route ÷ routes submitted"* are both computed from multiple observations inside the same match (RFC §16.4 lease-rate sweep, GDD §22). Treating every player-round as its own independent sample overstates precision — the issue's own Option B flags this and doesn't resolve it.

## Options

**A — Fixed match count (3,000, matching R2), thresholds read as point estimates.**
- For: simplest, precedented, directly comparable to the R2 table.
- Against: says nothing about 12.3. Already superseded in practice — [#200](https://github.com/garnizeh/cinzal/issues/200) and [#204](https://github.com/garnizeh/cinzal/issues/204) are written against `--matches 10000`, not 3,000.

**B — Fixed match count, interval reported per metric, verdict rule: action only when the interval clears the threshold.**
- For: makes a near-miss decidable in advance. [#200](https://github.com/garnizeh/cinzal/issues/200)'s own acceptance criteria already require *"every metric with its interval per D35"* — this option is what that line is waiting on.
- Against: needs a stated interval method, and needs the clustering problem above actually solved, not just named.

**C — Sequential: run until each metric's interval is narrower than a stated width, cap the total.**
- For: spends compute where the answer is unclear.
- Against: variable runtime, a stopping rule that needs its own justification, and — per [#200](https://github.com/garnizeh/cinzal/issues/200)'s CSV format (*"one row per configuration × metric set"*) — a sample size that varies by cell doesn't fit a table meant to be read as one.

## Decision

**B**, with four parts.

### 1. Match count: 10,000 per configuration

Not 3,000. This is a correction to the issue's own recommendation, not an endorsement of it — the issue proposed matching R2's 3,000, but that number is no longer available to pick: RFC §16.4's worked example and two already-filed exit demonstrations ([#200](https://github.com/garnizeh/cinzal/issues/200), [#204](https://github.com/garnizeh/cinzal/issues/204)) already run `--matches 10000` verbatim as their spec anchor. Reopening that number here would mean rewriting both issues to serve a decision they're currently blocked on.

It also isn't a costly correction. Every threshold in the exit-criteria table resolves to well under a tenth of a unit of precision at 10,000 matches (see Reasoning) — the statistics would have been comfortable at 3,000 too. The 3.3× cost over R2's precedent is negligible because a single match's `Resolve`/`Decide` loop is headless, pure-Go, and has no I/O of its own — the only I/O in the whole run is the one CSV write per configuration at the end (`cmd/simulate`'s `--out sweep.csv`, [#200](https://github.com/garnizeh/cinzal/issues/200)), not per match (RFC §16.4: *"a weekend of sweeps"* already budgeted for the full parameter space).

### 2. One interval method for every metric, computed at match granularity

**Scope: the seven exit-criteria rows this decision governs, not all twenty of GDD §22.** [D33](D33-telemetry-event-stream-coverage.md) already found that two rows (15, 16) defer to M5 UI instrumentation and one (18) has no operational definition to compute against — nothing here changes that, and this rule only applies once a row has a computable per-match statistic to reduce. `cmd/simulate`'s CSV carries an interval for every row it *does* compute; rows 15/16/18 stay absent from that CSV, unaffected by this decision, exactly as D33 left them.

For each of those rows, first reduce each match to **one number** — its own per-match statistic, with the reducer named explicitly per metric shape, because "whatever the row already is" hides a real choice for anything computed across multiple players or routes inside one match:

- **A metric that is already one value per match** (confrontation count, "did any player reach Infamy 9") needs no reduction.
- **A metric specified per player** (live leases at scoring) reduces to that match's **mean across its own players** — 2 to 5 numbers in, one out.
- **A metric specified as a ratio over events inside the match** (routes cancelled ÷ routes submitted, incidents hitting a player ÷ incidents drawn) reduces to the match's own **pooled ratio** — total numerator over total denominator for that match, not a mean of per-route or per-incident indicators weighted some other way. A match that submits zero routes (or draws zero incidents) contributes no value for that metric and is excluded from that metric's vector, with the exclusion count reported alongside it in the CSV — the same "report the gap, don't paper over it" convention [#200](https://github.com/garnizeh/cinzal/issues/200)'s own fail-closed acceptance criterion already applies to a match that didn't reach `cfg.Rounds`.

This is the fix for the clustering problem the issue's own Option B left open: the sampling unit is always the match, never a player-round or a route, because players and routes inside the same match are not independent draws.

Then, across the resulting vector of (up to) 10,000 per-match numbers, report:

```text
mean ± 1.96 · s / √n
```

— a standard normal (Wald) interval for a mean, where `s` is the sample standard deviation of the per-match vector and `n` is the vector's length. One formula for every row, count or ratio or 0/1 indicator alike, so `cmd/simulate` implements one interval routine, not a per-shape branch. It is **not** a Wilson interval, and the difference isn't cosmetic: a Wald interval degenerates at the boundary — a per-match indicator with zero successes in the sample gives `s = 0` and a zero-width `[0, 0]` interval, which trivially "clears" any `< threshold%` action rule without the coverage a real 95% interval promises. None of the seven exit-criteria thresholds run near enough to 0 or 1 at their expected values for this to bite in practice (R7's 10% floor is the closest, and §Reasoning's computation already shows ±0.6pp of room around it) — but a sweep can walk a dial far enough that some cell's per-match indicator genuinely returns all-zero or all-one across 10,000 matches. **A reported interval of zero width at a sweep tail is a degenerate sample, not a finding**, and must be flagged and re-examined rather than read as a confident verdict.

### 3. Verdict rule

**Action only when the 95% interval lies entirely on the failing side of the threshold or band edge.** A point estimate crossing the line with the interval still straddling it is **watch, not act** — matching [#203](https://github.com/garnizeh/cinzal/issues/203)'s own wording, written before this document existed: *"a point estimate of 12.3 with an interval spanning 12 is not a finding."*

When a baseline (non-swept) exit-criterion check lands in "watch," re-run that configuration under a second, independently drawn root seed. **The two runs are then pooled, not compared as separate verdicts** — concatenate both 10,000-match vectors into one 20,000-match vector and recompute the same interval over it, then apply the same rule: action only if the pooled interval clears the threshold. Pooling is what settles every shape a second run could take (confirms the trip, moves to confident no-action, or still straddles) with the one rule instead of a case table for "both straddle" versus "only one clears" versus "they clear on opposite sides" — those are all just samples whose combination either narrows the interval past the threshold or doesn't. If the pooled interval still straddles, record "watch, unresolved at n = 20,000" and hand it to M5.5's human playtesting rather than inflating `n` further to manufacture false precision — this is what bounds the runtime that Option C left open-ended, without losing C's benefit of spending more effort where the answer is unclear. [#205](https://github.com/garnizeh/cinzal/issues/205) already does a second-seed re-run unconditionally for its three metrics; pooling still applies to that stricter case, it just runs regardless of whether the first draw alone would have triggered it.

For swept metrics (the lease rate, per [#204](https://github.com/garnizeh/cinzal/issues/204)), the "verdict" is a range, not a single accept/reject, and the sweep points are read **in dial order** — the order [#204](https://github.com/garnizeh/cinzal/issues/204) already sweeps them in (`LeaseCostPerBlock=1,2,3,4,5`, extended in both directions). Report the interval at every point and read the breakpoint as the region between the last in-order point whose interval sits fully inside the target band and the first whose interval sits fully outside it. Three cases fall outside that simple reading, and none may be silently collapsed to a single breakpoint: **more than one inside↔outside crossing** along the ordered sweep is reported as every transition region found, not just the first; **no point fully outside the band** is reported as "in band across the tested range," not as a missing breakpoint; **no point fully inside the band** is reported as "out of band across the tested range." Both of the last two are instructions to extend the sweep further in that direction, matching [#204](https://github.com/garnizeh/cinzal/issues/204)'s own standard: *"a sweep that never leaves the band has not found the shape."*

**No cross-metric multiplicity correction.** The seven exit-criteria thresholds are not one hypothesis family — they're seven separate, previously-separated design questions (R1, R9, the lease rate, R6, R7, R11, the two-player floor), each governing an independent lever. A Bonferroni-style joint correction would blur an R7 trip and an R1 trip together as if they said something about each other; they don't. Within a *sweep*, a threshold crossing at one configuration is not noise to correct away — it is the located flip point the sweep exists to find (RFC §16.4: *"a sweep tells you the shape of the parameter space — where a dial flips a strategy from dominant to dead"*). The CI-based verdict rule above is the deliberate, and only, safeguard against a chance trip.

### 4. Tier baseline, by threshold

Both tiers are run and reported for every metric — cheap, since the harness parameterises on tier already (RFC §16.4's `--bots` flag), and the Drifter/Operator gap is its own diagnostic (a metric where the two disagree sharply is a metric that rewards skill). But each threshold's **verdict** is read against one tier, chosen by whether the threshold is about map geometry or about strategy:

| Threshold | GDD ref | Read against | Why |
|---|---|---|---|
| Confrontations per match | R9 | **Drifter** | Map-shape question; matches R2's own random-walk methodology (`cinzal-gdd.md:1472`) |
| Two-player encounter rate under rotating borders | §6.3 | **Drifter** | Whether the border-rotation mechanic itself geometrically forces encounters, independent of strategy |
| Routes cancelled mid-route | R1 | **Operator** | *"About how the rules treat a player who is trying"* — a Drifter's cancellations measure the map, not the confrontation rule ([#205](https://github.com/garnizeh/cinzal/issues/205)) |
| Incidents actually hitting a player | R6 | **Operator** | Operator *"routes around unstable sectors weighted by the displayed deck counts"* (RFC §14.3); testing whether incidents still land against active avoidance is the real question |
| Matches reaching Infamy 9 | R7 | **Operator** | Depends on managed Infamy climb, which only Runner/Operator model (RFC §14.3) |
| Endgame camping | R11 | **Operator** | Explicitly a rational-incentive question, per the issue and [#203](https://github.com/garnizeh/cinzal/issues/203) |
| Lease rate | §10.4 | **Operator** | Leasing is a strategic choice; Runner *"never buys"* by definition and Drifter has no plan at all (RFC §14.3); [#204](https://github.com/garnizeh/cinzal/issues/204) already states this |

This table matches, and formalizes, what [#203](https://github.com/garnizeh/cinzal/issues/203), [#204](https://github.com/garnizeh/cinzal/issues/204) and [#205](https://github.com/garnizeh/cinzal/issues/205) already wrote against a D35 that didn't exist yet — this decision is confirming those three issues' assumption, not introducing a new split five metrics of seven end up on.

## Reasoning

**Why 10,000 is not statistical overkill, computed rather than assumed.** For a 0/1 per-match indicator near the tightest threshold in the table (R7, 10%): `SE = √(0.1·0.9/10000) ≈ 0.003`, a 95% half-width of **±0.6 percentage points** against a spec written in whole percentage points. For a count metric near its threshold (R9, confrontations/match ≈ 12, treating per-match variance as roughly Poisson so `s ≈ √12 ≈ 3.46`): `SE = 3.46/√10000 ≈ 0.035`, a half-width of **±0.07** against a threshold of 12. Both numbers would already have been tight at 3,000 (R7: ±1.1pp; R9: ±0.12) — the choice of 10,000 over 3,000 is not rescuing an underpowered design, it's matching the number two issues already depend on, at a cost increase (3.3×) that's free relative to a harness whose per-match cost is headless, pure-Go compute with no I/O of its own.

**Why one formula instead of a binomial-proportion / continuous-mean split.** The issue's Option B raised the clustering problem without solving it: *"matches within a configuration are not independent in every metric... requires care, not just a formula."* Reducing every row to one number per match first, then applying the same mean-and-SE formula, is that care — it makes the match the sampling unit unconditionally, so a per-player or per-route metric never borrows precision it doesn't have from counting the same match's players or routes as separate observations. It also means `cmd/simulate` implements exactly one interval routine, not one per metric shape.

**Why B over C.** C's stopping rule needs its own justification the issue never supplies, and per-cell variable sample sizes don't survive contact with [#200](https://github.com/garnizeh/cinzal/issues/200)'s own CSV contract — a fixed `match_count` column that means something different in every row is not what *"the CSV is a document, not a debug dump"* ([#200](https://github.com/garnizeh/cinzal/issues/200)) is asking for. The second-root-seed re-run for a straddling baseline check gets C's real benefit — more effort where the answer is unclear — without an open-ended runtime or a jagged table.

**Why the tier table doesn't need a rule beyond citing the specs.** Every row's answer was already implied by RFC §14.3's own tier descriptions once asked directly: a tier that *"never buys"* cannot answer a question about buying; a tier with *"no plan"* cannot answer a question about incentive. The interesting finding here isn't the split, it's that three exit-demonstration issues had already converged on it independently, which is itself evidence the split was the only defensible read of the specs, not a judgement call this document invented.

## Consequences

- **[#200](https://github.com/garnizeh/cinzal/issues/200)'s CSV gains a concrete interval column definition**, not just "an interval": mean and half-width computed by the §22 rule above, over the per-match-reduced vector, at `n = 10,000`. This is a fully specified acceptance criterion for a line [#200](https://github.com/garnizeh/cinzal/issues/200) already wrote, not new scope.
- **The interval is `cmd/simulate`'s computation, not `internal/telemetry`'s.** `telemetry.Match` ([D34](D34-telemetry-package-placement.md)) returns one `MatchSummary` per match; the mean/interval reduction happens once per configuration, across many `MatchSummary` values, entirely inside the harness that already owns the sweep loop. [#197](https://github.com/garnizeh/cinzal/issues/197)/[#198](https://github.com/garnizeh/cinzal/issues/198) need no interval-shaped field on `MatchSummary` itself.
- **[#203](https://github.com/garnizeh/cinzal/issues/203), [#204](https://github.com/garnizeh/cinzal/issues/204), [#205](https://github.com/garnizeh/cinzal/issues/205)** can now cite this document instead of a forward reference to a D35 that didn't exist when they were filed; no change to their own content, since they already wrote the tier split and the verdict wording this decision formalizes.
- **RFC §16.4 and GDD §21/§22 gain the sample-size and verdict-rule statement** — lands in [#202](https://github.com/garnizeh/cinzal/issues/202) (RFC/GDD catch-up to D32–D35), not here, matching D33/D34's own precedent of deferring spec-text edits to the implementing PR.
- **Reversible at low cost.** This is a documents-and-CSV-format decision; nothing in `internal/rules` or `internal/telemetry` depends on it. Changing the match count or the interval method later means re-running sweeps, not re-recording fixtures or replays — unlike D32/D33's `internal/rules` additions.
