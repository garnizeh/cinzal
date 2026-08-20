# D36 — The lease-rate exit criterion can't be met by a `Config` edit: loosen the chokepoint gate, or recalibrate the band?

**Status:** decided
**Blocks:** M2's exit-criteria closure for the lease-rate row (GDD §22); any future `LeaseCostPerBlock`/`LeaseBlockRounds` retuning task
**Decided:** 2026-08-20
**Issue:** [#231](https://github.com/garnizeh/cinzal/issues/231)

## The question

[#204](https://github.com/garnizeh/cinzal/issues/204)'s exit demonstration swept `LeaseCostPerBlock` (0–20) × `LeaseBlockRounds` (1–15) at 2/3/4/5 players, both bot tiers, 10,000 matches per configuration (106 configurations total, written up in `docs/exit-demos/204-lease-rate.md`). Every measured point stays at least an order of magnitude below GDD §22's `< 1` failing floor, let alone the 2–4/player target band. No `Config` edit closes this gap: the root cause `#204` identified is `OperatorOptions.ChokepointTrafficRate` (0.98) / `ChokepointMinObserved` (9) (`internal/bots/operator.go`) — a bots-package tuning pair outside `game.Config` and outside anything `cmd/simulate --sweep` can reach — which makes Operator attempt a stake so rarely that the economic dial has almost nothing to act on.

That gate was already a deliberate choice, made before `#204`, justified against a different metric: `operator.go`'s own comment states every looser threshold "measurably cost Operator mean RP against Runner." `#231` is the one that conflicts with `#204`'s finding.

## Why it is open

- **Loosening the gate to hit the lease-rate band probably costs RP performance again** — the same trade-off that set it high in the first place, now read in the other direction. `#231` itself states this isn't known without re-measuring.
- **The 2–4/player band may itself be uncalibrated against what Operator actually models.** [D35](D35-simulation-sample-size-and-verdict-rule.md)'s own tier table reads the lease rate against Operator because leasing is "a strategic choice" — but Operator's specific strategy (heat-map chokepoint reading) is a narrower, more conservative model of "buying a post" than a human's, and GDD §10.1's own worked example already frames a post's payoff as "clearly a loss on points... you're buying the vision and the sector majority, not the two points."
- **A secondary, distinct mechanism also caps the low end regardless of the gate's threshold:** `LeaseBlockRounds=1` gives Operator exactly zero live leases at final scoring, at every player count and every cost tested, because a one-round lease always expires before `maybeRenewLease`'s "about to lapse" check gets a second look at it (`internal/bots/runner.go:471-494`). Doesn't affect the shipped default (`LeaseBlockRounds: 3`); tracked separately as [#236](https://github.com/garnizeh/cinzal/issues/236), not part of this decision.

## Options

**A — Loosen `ChokepointTrafficRate`/`ChokepointMinObserved`, re-measure the RP-performance cost.** Directly addresses the lease-rate band, if the band is reachable at some point on that dial. Risk stated in the issue: re-opens the RP-performance question the current values were tuned to close, and needs its own sweep to quantify the cost before committing.

**B — Recalibrate GDD §22's 2–4/player target against Operator's actual (conservative) leasing model**, treating the measured range as the answer rather than a gap to close. Cheapest option; risk stated in the issue: quietly validates an overly cautious Operator rather than checking whether the caution itself is warranted.

**C — Leave both as they are, and let M5.5's human playtesting be the actual read on the lease rate**, per `#204`'s own carried-forward caveat that Operator's chokepoint gate is "a narrower instruction than what a human does with a post." Defers the decision instead of making it; leaves an M2 exit criterion formally unmet in the meantime, per the issue's own framing.

## Decision

**C**, on the strength of a direct measurement of Option A that the issue asked for but did not yet have.

Option A is closed empirically, not just judged too risky to attempt. Option B's premise — that the measured range reflects a calibrated, if conservative, read of "a rational actor's leasing frequency" that GDD §22 should simply adopt — turns out not to survive the same measurement. Both are decided by one paired-cohort sweep run against this decision (see Reasoning), following the exact methodology `internal/rules/bots_operator_golden_external_test.go`'s `TestOperatorBeatsRunnerOverAThousandMatches` already established for reading Operator's RP margin against Runner.

## Reasoning

**Measurement.** 1,000-match paired cohorts (golden root `0xa0...`, 4 players, `game.DefaultConfig()`, one Runner cohort shared as the fixed comparison baseline), one Operator cohort per `(ChokepointTrafficRate, ChokepointMinObserved)` pair, from the shipped default down to the gate fully open (`0, 0` — the most permissive setting expressible in `OperatorOptions`; `findChokepoint`'s `ObservedRounds <= 0` check is unconditional and cannot be loosened further). Git SHA `22ff49f`.

| `ChokepointTrafficRate`, `ChokepointMinObserved` | Operator mean RP | Margin vs. Runner (1.855) | Mean leases/player at scoring |
|---|---|---|---|
| 0.98, 9 (shipped default) | 1.903 | +0.049 | 0.0020 |
| 0.90, 6 | 1.898 | +0.043 | 0.0158 |
| 0.80, 5 | 1.867 | +0.012 | 0.0983 |
| 0.70, 4 | 1.821 | −0.033 | 0.1675 |
| 0.60, 3 | 1.735 | −0.119 | 0.2545 |
| 0.50, 3 | 1.732 | −0.122 | 0.2662 |
| 0.0, 0 (gate off entirely) | 1.645 | −0.209 | 0.3033 |

**Option A is closed, not merely expensive.** Even with the chokepoint gate removed outright — the absolute ceiling reachable through `OperatorOptions`, and a setting no shipped tuning would ever choose — the lease rate reaches only 0.303 leases/player: still under a third of GDD §22's `< 1` *failing* floor, 6–13× below the 2–4 target band, at a cost that has already flipped Operator from beating Runner (+0.049) to losing to it by more than four times that margin (−0.209). There is no point on this dial, at any RP cost up to and including deleting the mechanism's own gate, that reaches the band. `#231`'s own framing — "loosening the gate probably costs RP performance again... how much, and whether that's acceptable, isn't known without re-measuring" — is now answered: the cost is real and it still doesn't buy the outcome.

**This also undercuts Option B's premise, not just Option A's viability.** Loosening the gate from its default (rate 0.98) all the way to off (rate 0.0) moves the lease rate by roughly 150×, but the absolute distance covered (0.002 → 0.303) is still nowhere near the 2–4 band another 6–13× away. If the shipped gate's specific threshold were the thing calibrating Operator's leasing frequency to "a rational, conservative actor" per D35's own reasoning for reading this row against Operator, removing that threshold entirely should have moved the needle far more than it did. It didn't, which means something else inside Operator's action-priority ordering — not the traffic-rate/observation gate `#204` identified — is the dominant constraint on how often a stake is even attempted. Recalibrating GDD §22's band to match the *shipped default's* measured range (Option B) would be encoding that other, unidentified mechanical ceiling as if it were a deliberate model of human leasing behavior, which is exactly the risk `#231`'s own text flagged for B ("quietly validating an overly cautious Operator rather than checking whether the caution itself is warranted") — now with evidence that the caution isn't the load-bearing constraint, so validating it wouldn't even be validating the right thing.

**Why C over reopening A with a different dial.** Having ruled out the two `OperatorOptions` fields `#204` and `#231` already named, the next candidate would be Operator's action-priority ordering itself — a materially larger change than a tuning-pair sweep, touching behavior the golden RP-margin test and `operator_test.go`'s own hand-constructed cases already cover. That is not a decision to make unilaterally inside this document; it is its own task, scoped and reviewed like any other change to `internal/bots`, and only worth taking on if a human-play signal from M5.5 says the lease rate is actually a problem worth chasing. Filed as [#237](https://github.com/garnizeh/cinzal/issues/237), explicitly blocked on M5.5's read rather than opened now.

**Why C is procedurally sound despite `cinzal-implementation-plan.md`'s "exit criteria are numbers, not code."** That line (§2, P1) guards against skipping M2's *measurement*, not against a row whose measurement is complete and whose answer is "not reachable by simulation." The roadmap already has this shape precedented three ways: row 13 of GDD §22 "ships without a precise answer and says so" (`cinzal-implementation-plan.md`, M2 deliverables); rows 15/16/18 defer to M5/M5.5 per [D33](D33-telemetry-event-stream-coverage.md); and the M2 exit-criteria section's own closing caveat states a sweep "tells you the shape of the parameter space... not the exact value. It narrows the range that M5.5 then confirms." The lease-rate row has, in fact, been narrowed — from "unknown" to "not reachable by any bot-simulation lever tested, at any RP cost" — which is the shape of answer M2's own caveat anticipates for a row like this.

## Consequences

- **`game.Config` and `OperatorOptions` ship unchanged.** No PR retunes `LeaseCostPerBlock`, `LeaseBlockRounds`, `ChokepointTrafficRate`, or `ChokepointMinObserved` off this decision.
- **The lease-rate row of M2's exit-criteria table closes as "measured, out of band across the tested range including the gate's own removal, deferred to M5.5"** — not as "watch" (D35's watch state is for a straddling interval; every interval measured here, in both `#204` and this decision's own sweep, sits fully outside the band) and not as silently met.
- **GDD §22's 2–4/player band is not edited.** Per the Reasoning above, this decision does not have grounds to say what the right number is — only that neither the current bot simulation nor its most permissive reachable variant can answer that question, which is a different finding from "the band is wrong."
- **[#236](https://github.com/garnizeh/cinzal/issues/236)** (the `LeaseBlockRounds=1` renewal-timing gap) and **[#237](https://github.com/garnizeh/cinzal/issues/237)** (Operator's action-priority ordering as the real constraint on stake attempts, blocked on M5.5) are filed as follow-up tasks, independent of this decision closing.
- **Reversible at low cost.** Nothing in `internal/rules` or shipped `internal/bots` tuning changes here; a future decision revisiting this one (e.g., after M5.5 human data) means re-running sweeps against a possibly-changed Operator, not unwinding a merged code change.
