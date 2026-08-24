# Exit demo: R1 under the changed confrontation rule

**Issue:** [#260](https://github.com/garnizeh/cinzal/issues/260), re-run again by [#267](https://github.com/garnizeh/cinzal/issues/267)
**Milestone:** M2 — Bots and simulation
**Method:** [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md) — 10,000 matches per configuration, `mean ± 1.96 · s / √n` over the per-match vector, action only when the whole interval sits on the failing side. Both root seeds — [#205](https://github.com/garnizeh/cinzal/issues/205)'s own two, `#205`'s own harness invocation, verbatim — algebraically combined into one pooled `(mean, half_width, n=20,000)` summary before any verdict is read (see Provenance below for the exact method; each seed's own unrounded CSV values are the inputs, not the two-decimal figures printed here).

## Update (#267): row 1's numerator fixed, re-run again

The section below this one (**"What this demonstration was asked to prove, and what it found instead"** onward) is the record of the *previous* re-run, under [#268](https://github.com/garnizeh/cinzal/issues/268): D39's rule (§15, option E+) confirmed working, but `RoutesCancelledMidRoute` itself measuring the wrong thing — still counting a halt as cancelled whenever the seat's declared plan was cut short at the moment of the first halt, whether or not those steps went on to be spent as a D39 blind walk. That gap was filed as [#267](https://github.com/garnizeh/cinzal/issues/267) and is what this update re-runs against.

**#267's fix is not a chain-aware read of `HaltStepsUnspent` — it's the removal of `EventRouteHalted` from row 1's numerator entirely.** Investigating #267 found that `haltOrConvertMovement` (D39, `confront.go`) always either converts a halt's unspent budget into further blind Pushing On steps or has nothing left to convert (`unspent <= 0`, `haltMovement`'s own only remaining call site) — so no `EventRouteHalted` this package produces can still distinguish "converted and later spent" from "genuinely never spent." A temporary ground-truth probe (not merged, the D37/D38/D39 pattern), built to measure the residual directly against final match state rather than events, found it to be 0.18% (31 of 17,167 submitted routes with at least one halt, 300 four-player Operator matches) — attributable to map dead-ends and incident truncation of an already-converted blind walk, neither observable from `EventRouteHalted`'s existing fields. The redefined `routesCancelledMidRoute` (`internal/telemetry/match.go`) reads that as 0 for every match, honestly, rather than approximating it with a signal proven (by the same investigation) to overcount by roughly two orders of magnitude.

Investigating #267 also surfaced and fixed a second, previously-hidden defect this measurement depends on: `haltOrConvertMovement`'s own boundary calculation (`len(Route)+PushingOn.Steps`, the absolute step `advance()` and `movementSteps()` read a halted seat's remaining budget against) silently shrank on any second halt against the same seat in the same round, or a first halt landing mid the seat's own declared Pushing On — costing real steps D39 was supposed to preserve, and potentially shortening the round's own shared movement-loop bound for other seats too. Filed and fixed together as [#269](https://github.com/garnizeh/cinzal/issues/269), landing in the same PR as #267 ([#270](https://github.com/garnizeh/cinzal/pull/270)) — #267's acceptance criterion can't be measured against a boundary that's still wrong.

### Provenance (this re-run)

| | |
|---|---|
| Git SHA | `8d67b08a4bcbfe5530959475b79eb206174d2eb4` — `issue-267-r1-halt-chain`, includes #270 (#267 + #269). Working tree clean at measurement time. |
| Root seed 1 (default) | `cinzal-simulate-default-root-seed-v1` → `e4c50a633bfa5326029d36fcc00ea91af9510b523942539d4f89ed107866aa09` — same seed #260/#268 used |
| Root seed 2 (independent) | `cinzal-simulate-205-second-seed-aa742df36a7e1d86944d2dc9cee43022` → `6e9de6981c839e3f965a2ec1cc24b8f4d0b27fcf8ab0a2fc65a61c53d066d98e` — same seed #260/#268 used |
| Config | `game.DefaultConfig()`, unmodified |
| Matches per configuration | 10,000 (20,000 pooled per cell); all 16 configurations report `status=ok`, `matches_completed=10000` |
| Player counts | 2, 3, 4, 5 |
| Bot tiers | Drifter and Operator, both reported for every cell (D35) |

Raw CSVs for all 16 configurations are in [`267/`](267/), two files each (`--out` and `--breakdown`), the identical harness invocation `#260`/`#268` used:

```bash
simulate --matches 10000 --players <N> --bots <drifter|operator> \
  --sweep Rounds=15 --seed <root-seed-string> \
  --out       267/p<N>-<tier>-<seed-label>.csv \
  --breakdown 267/p<N>-<tier>-<seed-label>-breakdown.csv
```

Every file carries `git_sha=8d67b08a4bcbfe5530959475b79eb206174d2eb4` and `root_seed` matching one of the two hexes above, checked directly.

### Row 1 — the acceptance criterion, re-measured

| | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| **Operator, measured** | 0.00% [0.00%, 0.00%] | 0.00% [0.00%, 0.00%] | 0.00% [0.00%, 0.00%] | 0.00% [0.00%, 0.00%] |
| **Operator, D39 predicted** | 1.65% [1.62%, 1.69%] | 2.85% [2.81%, 2.88%] | 3.51% [3.47%, 3.54%] | 4.75% [4.71%, 4.78%] |
| **Drifter, measured** | 0.00% [0.00%, 0.00%] | 0.00% [0.00%, 0.00%] | 0.00% [0.00%, 0.00%] | 0.00% [0.00%, 0.00%] |
| **Drifter, D39 predicted** | 0.99% [0.96%, 1.02%] | 1.71% [1.68%, 1.74%] | 2.16% [2.14%, 2.19%] | 3.46% [3.43%, 3.49%] |

The zero-width interval is exact, not rounded: `RoutesCancelledMidRoute_mean` reads `0.000000` in every one of the 32 CSV rows (16 configurations × `--out`/`--breakdown`), with an empty half-width field — `cmd/simulate`'s own "a zero-width interval is a fact, not a measurement" handling (`#249`), for a metric that is a genuine constant across every match under the current definition, not an artifact of nothing being measured (`N` is `10000` at every cell, the full submitted-route population).

**Acceptance criterion, read literally: still does not land inside D39's predicted intervals, at every cell, on both tiers — but on the opposite side from `#268`'s finding.** Every predicted interval's own lower bound (0.99%–4.75% across the 8 cells) sits strictly above the measured 0.00%. Per `#267`'s own acceptance criterion ("if it still does not [land inside], that gap is reported rather than assumed away, the same standard `#260` itself was held to"), this is that report: the gap is real, and it is not an implementation shortfall on this side — every path in `internal/rules/confront.go` that could still leave a submitted route's declared plan short of its full budget by round's end goes through `haltOrConvertMovement`, and that function converts whatever's left rather than dropping it, unconditionally. Nothing observable from the event stream distinguishes "genuinely never spent" from "converted and spent" any more, because under the fixed rule there is close to nothing left in the first category (the 0.18% ground-truth figure above).

**Reading this as "R1 passes with an even larger margin than D39 predicted" is the correct reading, not a technicality.** R1's own text is a `< 15%` threshold, action above it; 0.00% clears that threshold by more room than any option D39 measured, including E+ itself. The gap from D39's own predicted 1.65%–4.75% most likely reflects D39's external probe modeling the round's movement-loop bound less generously than `internal/rules`' actual `movementSteps` (`resolve.go`) does — a static per-round cap rather than the dynamic, per-seat-preserving recomputation the shipped code performs — though that is not independently confirmed here, since the probe's own source was never part of any merged diff (D39's own Method section).

### Corroborating rows, re-checked against this fix

Every row `#268` read as independent of `HaltStepsUnspent` still lands where it did — confirming the rules fix (`#269`) changed RNG consumption (golden fixtures moved) without changing match outcomes in any way these rows would catch. Operator, per player count, seed 1 vs. seed 2 (both from the same `267/` CSVs above):

| Row | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| 9 — confrontations won against an Evasive loser | 43.77% / 43.39% | 43.75% / 44.12% | 45.80% / 45.65% | 44.84% / 44.84% |
| 10/11 — confrontations per match | 5.7392 / 5.7537 | 11.6742 / 11.6447 | 17.3347 / 17.3658 | 26.1108 / 26.1888 |
| 20 — confrontations in the final 3 rounds | 20.36% / 19.97% | 20.36% / 20.57% | 21.03% / 21.21% | 20.25% / 20.26% |
| 2 — deliveries per player | 1.5036 / 1.5109 | 1.2545 / 1.2546 | 0.9799 / 0.9740 | 0.7949 / 0.7927 |
| 17 — rounds flagged Loitering | 3.90% / 3.93% | 4.24% / 4.24% | 4.15% / 4.20% | 4.94% / 5.01% |

Every figure is within a few tenths of a percentage point of `#268`'s own measured values at the same cells (Operator, 4p: row 9 was 45.51% [45.28%, 45.73%], row 10/11 was 17.4158 [17.3604, 17.4712], row 20 was 21.19% [21.06%, 21.31%]) — the same "the rule itself is confirmed working" verdict `#268` reached, undisturbed by the boundary fix.

## What lands, and what does not (updated)

**R1's underlying rule is confirmed working, again, and now so is its telemetry.** Row 1 reads exactly 0.00% at every cell, `N=10000` throughout — a genuine measurement, not an unmeasured gap — clearing the `< 15%` threshold with more margin than D39's own E+ prediction, for the reason explained above. `#267` is resolved; `#269`, the boundary defect it surfaced, is resolved with it. GDD §20's R1 entry and §22 row 1 can both be read against this figure going forward, in preference to every number in the sections below, which predate the numerator fix and the boundary fix alike.

---

## What this demonstration was asked to prove, and what it found instead

[D39](../decisions/D39-r1-confrontation-softening.md) decided §15's confrontation-softening rule (option E+) and predicted, from a probe built outside the engine, what GDD §22 row 1 would read once that rule shipped. [#258](https://github.com/garnizeh/cinzal/issues/258) and [#259](https://github.com/garnizeh/cinzal/issues/259) (landing as [#261](https://github.com/garnizeh/cinzal/pull/261), [#262](https://github.com/garnizeh/cinzal/pull/262) and [#266](https://github.com/garnizeh/cinzal/pull/266)) shipped it. This is the re-run.

**Row 1 does not land inside D39's predicted intervals, at any of the 8 (player count × tier) cells, and the gap is 5×–11×, not noise.** The confrontation rule itself is faithfully implemented — every other row D39 predicted would hold steady (§9's Evasive band, confrontations per match, the final-3-rounds share) reproduces to within a few tenths of a percentage point. What does not reproduce is row 1's own telemetry: `RoutesCancelledMidRoute` still counts a route as "cancelled" whenever the seat's *declared* plan was cut short at the moment of the first halt, whether or not — under the rule D39 just shipped — those steps go on to be spent as a blind walk instead of being lost. That is an implementation divergence from D39's own adopted definition, not a probe error, and it is the finding this section exists to state plainly before the numbers below make the same point at length.

## Provenance

| | |
|---|---|
| Git SHA | `7b20415c3eef49b785e10fadc4f6735a47ab1317` — current `main`, includes #261/#262/#264/#266. Working tree clean at measurement time. |
| Root seed 1 (default) | string `cinzal-simulate-default-root-seed-v1` → `e4c50a633bfa5326029d36fcc00ea91af9510b523942539d4f89ed107866aa09` — matches `#205`'s and D39's own hex exactly |
| Root seed 2 (independent) | string `cinzal-simulate-205-second-seed-aa742df36a7e1d86944d2dc9cee43022` → `6e9de6981c839e3f965a2ec1cc24b8f4d0b27fcf8ab0a2fc65a61c53d066d98e` |
| Config | `game.DefaultConfig()`, unmodified |
| Matches per configuration | 10,000 (20,000 pooled per cell) |
| Player counts | 2, 3, 4, 5 |
| Bot tiers | Drifter and Operator, both reported for every cell (D35) |

Raw CSVs for all 16 configurations are in [`260/`](260/), two files each (`--out` and `--breakdown`), produced by the harness invocation `#260` names verbatim — `#205`'s own:

```bash
simulate --matches 10000 --players <N> --bots <drifter|operator> \
  --sweep Rounds=15 [--seed <root-seed-string>] \
  --out       260/p<N>-<tier>-<seed-label>.csv \
  --breakdown 260/p<N>-<tier>-<seed-label>-breakdown.csv
```

Every one of the 32 files carries `git_sha=7b20415c3eef49b785e10fadc4f6735a47ab1317` in its provenance line and `root_seed` matching the hex above — checked directly, not assumed. Pooling follows `#205`'s own algebraic method: each seed's own `(mean, half_width, n)` triple converted back to a sample standard deviation (`s = half_width · √n / 1.96`), combined via the standard two-group pooled-variance formula (within-group sum of squares plus the between-group mean-difference term), then re-expressed as one `(mean, half_width, n=20000)` triple. Both seeds agree with each other and with the pooled figure on every cell — the largest single-seed gap on row 1 anywhere in the sweep is 0.20 percentage points (5-player Drifter, 19.995% vs 20.178%), against half-widths of ~0.11 pp — so nothing below is one seed's artifact.

## Row 1 — the acceptance criterion

| | 2p | 3p | 4p | 5p |
|---|---|---|---|---|
| **Operator, measured** | 13.40% [13.29%, 13.50%] | 17.93% [17.84%, 18.03%] | 20.09% [20.01%, 20.18%] | 24.03% [23.95%, 24.11%] |
| **Operator, D39 predicted** | 1.65% [1.62%, 1.69%] | 2.85% [2.81%, 2.88%] | 3.51% [3.47%, 3.54%] | 4.75% [4.71%, 4.78%] |
| **Drifter, measured** | 11.27% [11.17%, 11.37%] | 13.95% [13.87%, 14.04%] | 15.15% [15.07%, 15.23%] | 20.09% [20.01%, 20.16%] |
| **Drifter, D39 predicted** | 0.99% [0.96%, 1.02%] | 1.71% [1.68%, 1.74%] | 2.16% [2.14%, 2.19%] | 3.46% [3.43%, 3.49%] |

No measured interval comes anywhere near its predicted one — the closest cell (Operator, 5 players) is still 5.1× the prediction; the widest (Drifter, 2 players) is 11.4×. **Acceptance criterion: fails, at every cell, on both tiers.**

Read against the literal GDD §22 row 1 line — *"if more than 15% of submitted routes are cancelled mid-route"* — rather than against D39's prediction, the *measured* figures above still trip **ACTION** at 5 of the 8 cells (Operator at 3/4/5 players, Drifter at 4/5 players), with the whole interval on the failing side every time. Read naively, this demonstration would say the confrontation rule needs softening *again*. That reading is wrong, and the next section is why.

## The finding: row 1's numerator was never updated for what D39 actually shipped

D39 decided row 1's numerator in two parts that have to be read together (`D39 §Decision`, part 1 and 2):

1. Count **routes**, not halt events: one per `(round, seat)` whose first halt that round left at least one step of the declared plan unspent.
2. Change §15 so that a halted participant's unspent steps are **not lost** — they become GDD §9.1 blind steps from wherever the seat now stands, spent rather than thrown away.

Part 1 shipped first, in [#262](https://github.com/garnizeh/cinzal/pull/262) — before part 2 existed. At that point "unspent at the moment of the halt" and "genuinely lost" were the same fact, because nothing yet converted a halt's remainder into anything. `EventRouteHalted.HaltStepsUnspent` (added in [#261](https://github.com/garnizeh/cinzal/pull/261)) and `internal/telemetry`'s `routesCancelledMidRoute` (rewritten in #262) both encode that equivalence directly:

```go
// internal/telemetry/match.go
halts := firstHaltUnspent(events)
n := 0
for key := range submitted {
    if unspent, ok := halts[key]; ok && unspent > 0 {
        n++
    }
}
```

Part 2 shipped second, in [#266](https://github.com/garnizeh/cinzal/pull/266) — `haltOrConvertMovement` in `internal/rules/confront.go`, replacing the bare `haltMovement` call at all three of D39's call sites. It is exactly what D39 asked for: a seat with unspent budget keeps it, converted to a blind walk. But it explicitly declines to touch what "unspent" *means* for the metric that reads it:

```go
// internal/rules/confront.go, haltOrConvertMovement's own doc comment
// Returns the same unspent count haltStepsUnspent already computed —
// unchanged in meaning by this rule: still how many steps of the declared
// plan were cut short, whether or not they now survive as blind ones (D39,
// GDD §22 row 1's numerator) — for the caller's own EventRouteHalted.
```

**That "(D39, ...)" attribution does not have a source in D39's own text.** `docs/decisions/D39-r1-confrontation-softening.md` never states that a step converted to a blind walk should still count as cancelled — searched directly, the phrase does not appear there. What D39's own text says, instead, is that the whole *point* of the rule change is that a shortfall is no longer thrown away (`D39 §Decision`, part 2: *"not the round's remaining steps"*), and its own measured prediction for the corrected reading under E+ — 1.65%/2.85%/3.51%/4.75% (Operator) — is roughly an order of magnitude below the pre-D39 shipped rule's own 17.32%/22.64%/24.93%/29.13%. A metric that still treats every converted shortfall as a cancellation cannot produce that drop; a metric that only counts a shortfall *still outstanding once the round finishes* can, because under D39's own rule almost every first-halt remainder gets spent by round's end rather than surviving to be measured as lost.

The row 9, row 10/11 and row 20 figures below confirm this is the right diagnosis and not a guess: they are computed independently of `HaltStepsUnspent` entirely, and they land inside or right beside D39's predictions. The rule that was shipped behaves the way D39 predicted it would. **The one thing that does not behave as predicted is the field that was never asked to change meaning when the rule underneath it did.**

### What this is not

This is not a case for reopening D39 — nothing here suggests the E+ rule itself was the wrong choice, or that GDD §15's text needs to move again. It is not a probe error either: `#245`'s own probe reproduced `#205`'s published pre-D39 figures to six decimal places (D39 §Method), and the corroborating rows below reproduce D39's *post*-rule predictions to a similar tightness — the probe measured what it said it measured, on both sides of the rule change. The gap sits specifically between D39's decided numerator definition (part 1) and the code that computes it, which was never revisited once part 2 shipped a rule the definition's own wording ("unspent... route or Pushing On") already anticipated but its implementation did not.

## Also re-read: does anything else move against D39's prediction?

Operator, 4 players unless noted, from the same pooled 20,000-match cells. These do not depend on `HaltStepsUnspent`.

| Row | D39 predicted | Measured | Verdict |
|---|---|---|---|
| 9 — confrontations won against an Evasive loser | 45.73% (band 20–40%, fails > 55%) | 45.51% [45.28%, 45.73%] | **Matches.** Interval brackets both D39's shipped-baseline (45.50%) and predicted (45.73%) figures — E+ leaves it exactly where D39 said it would. |
| 10/11 — confrontations per match | 5.7465 (2p), 17.3502 (4p), 26.1498 (5p) | 5.7813 [5.7455, 5.8171] (2p); 17.4158 [17.3604, 17.4712] (4p); 26.1435 [26.0814, 26.2056] (5p) | **Matches, with one point outside by a hair.** The 2p and 5p predictions sit inside their measured intervals; at 4p the predicted point (17.3502) sits 0.0102 below the interval's own lower bound (17.3604) — the smallest gap anywhere in this table, closer than D39's own two-seed reproducibility noise, but not technically inside. |
| 17 — rounds flagged Loitering | 4.17% (4p), 4.98% (5p) — point predictions, not intervals; fails > 15% | 4.25% [4.20%, 4.30%] (4p); 5.12% [5.07%, 5.17%] (5p) | **Small, consistent miss.** D39's own table gives no interval for this row, so the honest comparison is against the point: the measured interval's own lower bound clears the predicted point by 0.03 pp (4p) and 0.09 pp (5p), and the measured mean by 0.08 pp and 0.14 pp — small but reproducible gaps, well inside the target band and nowhere near the 15% failing line. Possibly the same second-order rule-timing sensitivity row 1's own residual gap shows between the shipped-old and E+-predicted readings (see below) — not investigated further here, since it changes no verdict. |
| 20 — confrontations in the final 3 rounds | ~21%, unmoved (band < 30%, fails > 45%) | 21.19% [21.06%, 21.31%] | **Matches.** |
| 2 — deliveries per player | 0.9769 (4p), up ~20% from shipped's 0.8138 | 0.9664 | **Matches direction and magnitude** — up 18.8% from the shipped baseline. |
| 3 — winner's RP lead over last place | 124.21% (4p), down ~13 pp from shipped's 137.35% | 125.39% [124.68%, 126.10%] | **Matches direction**; measured drop is 11.96 pp against a predicted ~13 pp, both far outside the row's own `< 40%` band regardless (§10.4's question, not R1's, per D39). |

Row 9 — the one D39 called out as mattering most, since it is why B/B′ were rejected in favour of E+ — is unmoved to within a third of a percentage point, exactly as claimed.

## What lands, and what does not

**R1's underlying rule is confirmed working as D39 predicted.** Every metric independent of `EventRouteHalted.HaltStepsUnspent` reproduces D39's post-rule numbers closely enough that this demonstration has no basis to doubt option E+ was implemented faithfully.

**Row 1 itself is not confirmed, and cannot be, on the current telemetry.** `RoutesCancelledMidRoute` measures a quantity D39 stopped intending the moment part 2 shipped: whether a seat's declared plan was interrupted, not whether the interruption cost them anything. Reading the *current* CSVs against the literal 15% line would report ACTION at 5 of 8 cells — the opposite of what the corroborating rows say actually happened.

**Disposition, per `#260`'s own instructions:** the demonstration does not land inside D39's predicted intervals, so GDD §20's R1 entry needs a note recording that gap rather than being re-opened as a decision, and the gap itself is a new task — filed as [#267](https://github.com/garnizeh/cinzal/issues/267), blocked by nothing, blocking a re-run of this same sweep once it lands. §20's own resolution of *which rule* to ship stands; what remains is making row 1 measure it.

## Carried forward, again

`#205`'s own figures — the pre-D39 30.06%/40.51%/45.70%/56.36% (Operator) — are the last ones measured under both the superseded event-count numerator and the pre-remedy rule; neither the 2.0×–3.8× multiple `#205` quotes nor the 17.32%–29.13% "corrected shipped" figures D39 measured describe the game as it ships today, and this demonstration's own headline numbers (13.40%–24.03%, Operator) do not describe it either, for the separate reason above. None of the three readings — `#205`'s, D39's corrected-shipped, or this demonstration's as-measured — should be read forward as "what row 1 is" once #267 lands; only its own re-run will be.
