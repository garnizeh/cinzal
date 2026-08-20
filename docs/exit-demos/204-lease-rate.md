# Exit demo: the lease rate has a range, and the sweep shows where the dial breaks

**Issue:** [#204](https://github.com/garnizeh/cinzal/issues/204)
**Milestone:** M2 — Bots and simulation
**Method:** [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md) — 10,000 matches per configuration, `mean ± 1.96 · s / √n` over the per-match vector, action only when the interval clears the threshold; for a sweep, the verdict is a range read in dial order.

## Headline result

**No point in the tested space comes anywhere close to the 2–4/player target band, at any player count, on either bot tier, even at the most economically favorable configuration tested.** The single highest value found anywhere in this sweep is 0.0666 leases/player (Drifter, 4 players, `LeaseCostPerBlock=1, LeaseBlockRounds=15`) — 15× below GDD §22's `< 1` failing floor, and 30–60× below the target band itself. Every measured Operator point is at or below 0.0031.

This is not a "the dial needs retuning" result — it's **the dial the sweep was asked to turn is not the binding constraint.** §Root cause below identifies the actual bottleneck, which sits in `internal/bots`, outside `game.Config` and outside what `--sweep` can reach.

## Provenance

| | |
|---|---|
| Git SHA | `7ce0fd6c7b44e25704aac8001f481628938e2d98` |
| Root seed (default) | string `cinzal-simulate-default-root-seed-v1` → `e4c50a633bfa5326029d36fcc00ea91af9510b523942539d4f89ed107866aa09` |
| Config | `game.DefaultConfig()`, only `LeaseCostPerBlock`/`LeaseBlockRounds` swept |
| Matches per configuration | 10,000 |
| Player counts | 2, 3, 4, 5 |
| Bot tiers | Operator (D35's read tier for this row) and Drifter, both reported |
| Total configurations run | 103 (every row below has `status=ok`, `matches_completed=10000`) |

Every point below is a single-seed run — per D35, a swept metric's "verdict" is a range read in dial order across points, not a pooled accept/reject at one configuration, so the second-seed re-run D35 reserves for a straddling *baseline* check doesn't apply here. No re-run was needed regardless: every point sits so far outside the band that no interval anywhere is close to straddling a line.

Raw CSVs are in [`204/`](204/), produced by:

```bash
# primary curve: cost sweep at the default LeaseBlockRounds=3, one file per (players, tier)
simulate --matches 10000 --players <N> --bots <operator|drifter> \
  --sweep LeaseCostPerBlock=0,1,2,3,4,6,10 \
  --out 204/p<N>-<tier>-cost-sweep.csv

# second axis: rounds sweep at the default LeaseCostPerBlock=3
simulate --matches 10000 --players <N> --bots <operator|drifter> \
  --sweep LeaseBlockRounds=1,2,4,6 \
  --out 204/p<N>-<tier>-rounds-sweep.csv

# 2D cross-check: does cost/rounds collapse to one effective dial?
simulate --matches 10000 --players 4 --bots operator \
  --sweep LeaseCostPerBlock=1,3,6 --sweep LeaseBlockRounds=1,3,6 \
  --out 204/p4-operator-grid.csv

# supplementary points: practical ceiling and true collapse-to-zero
simulate --matches 10000 --players 4 --bots <operator|drifter> --sweep LeaseBlockRounds=15 --out 204/p4-<tier>-rounds-ceiling.csv
simulate --matches 10000 --players 4 --bots <operator|drifter> --sweep LeaseCostPerBlock=1 --sweep LeaseBlockRounds=15 --out 204/p4-<tier>-bestcase.csv
simulate --matches 10000 --players 4 --bots <operator|drifter> --sweep LeaseCostPerBlock=20 --out 204/p4-<tier>-highcost.csv
```

## The cost curve — `LeaseCostPerBlock`, `LeaseBlockRounds=3` (shipped default), one line per player count

All figures `mean [95% interval]`, live leases at final scoring per player.

### Operator (D35's read tier)

| `LeaseCostPerBlock` | 0 | 1 | 2 | 3 (default) | 4 | 6 | 10 |
|---|---|---|---|---|---|---|---|
| 2p | 0.0000 [0,0] | 0.0031 [.0023,.0039] | 0.0029 [.0021,.0036] | 0.0024 [.0017,.0031] | 0.0024 [.0017,.0030] | 0.0018 [.0012,.0024] | 0.0012 [.0007,.0017] |
| 3p | 0.0000 [0,0] | 0.0026 [.0020,.0031] | 0.0024 [.0018,.0030] | 0.0022 [.0017,.0027] | 0.0020 [.0015,.0025] | 0.0016 [.0012,.0021] | 0.0011 [.0007,.0014] |
| 4p | 0.0000 [0,0] | 0.0024 [.0019,.0029] | 0.0020 [.0016,.0024] | 0.0018 [.0014,.0022] | 0.0016 [.0012,.0020] | 0.0012 [.0009,.0015] | 0.0008 [.0005,.0011] |
| 5p | 0.0000 [0,0] | 0.0017 [.0013,.0020] | 0.0015 [.0011,.0018] | 0.0013 [.0010,.0017] | 0.0011 [.0008,.0014] | 0.0009 [.0006,.0011] | 0.0005 [.0003,.0006] |

### Drifter (reported alongside, per the issue's own request)

| `LeaseCostPerBlock` | 0 | 1 | 2 | 3 (default) | 4 | 6 | 10 |
|---|---|---|---|---|---|---|---|
| 2p | 0.0000 [0,0] | 0.0602 [.0567,.0637] | 0.0282 [.0258,.0305] | 0.0164 [.0147,.0182] | 0.0103 [.0089,.0116] | 0.0064 [.0053,.0075] | 0.0016 [.0011,.0022] |
| 3p | 0.0000 [0,0] | 0.0447 [.0423,.0472] | 0.0210 [.0194,.0226] | 0.0119 [.0107,.0132] | 0.0082 [.0071,.0092] | 0.0040 [.0033,.0047] | 0.0011 [.0007,.0015] |
| 4p | 0.0000 [0,0] | 0.0361 [.0341,.0381] | 0.0154 [.0142,.0166] | 0.0084 [.0075,.0093] | 0.0059 [.0052,.0067] | 0.0035 [.0029,.0040] | 0.0010 [.0007,.0013] |
| 5p | 0.0000 [0,0] | 0.0318 [.0301,.0334] | 0.0123 [.0113,.0133] | 0.0082 [.0074,.0090] | 0.0045 [.0039,.0051] | 0.0024 [.0019,.0028] | 0.0006 [.0004,.0009] |

**Verdict at the shipped default (`LeaseCostPerBlock=3`), read against Operator per D35's tier table: ACTION, unambiguously, at every player count.** Every interval's upper bound (max 0.0031, at 2p) sits over 300× below the `< 1` failing line — this is not a near-miss the interval could plausibly clear either way. The curve is monotonically decreasing in cost, as expected, and both tiers behave sensibly relative to each other (Drifter uniformly higher than Operator at every point, since Operator's stricter gating — see §Root cause — suppresses staking Drifter's uniform sampling doesn't). But sensible shape does not rescue the magnitude: nowhere in `[0, 10]` does either tier even reach the `< 1` failing floor, let alone the 2–4 band.

**The `LeaseCostPerBlock=0` column is a guard artifact, not the free-leasing limit.** `internal/rules/affordance.go:80` computes `MaxLeaseBlocks` only `if cfg.LeaseCostPerBlock > 0` — at cost 0 it's left at its zero default, so 0 is read by every consumer as "cannot afford any lease block," not "leasing is free." `internal/bots/operator.go:496` has the identical guard for Operator's fresh-stake path. This means the exact-zero point at cost 0 is not informative about the low end of the curve — the true near-zero-cost behavior is the `LeaseCostPerBlock=1` column instead, which is already the highest value found anywhere in the cost sweep and still far below `< 1`.

## The rounds curve — `LeaseBlockRounds`, `LeaseCostPerBlock=3` (shipped default)

All figures `mean [95% interval]`.

### Operator

| `LeaseBlockRounds` | 1 | 2 | 3 (default, from cost sweep) | 4 | 6 |
|---|---|---|---|---|---|
| 2p | 0.0000 [0,0] | 0.0020 [.0014,.0026] | 0.0024 [.0017,.0031] | 0.0024 [.0017,.0030] | 0.0027 [.0020,.0034] |
| 3p | 0.0000 [0,0] | 0.0019 [.0014,.0024] | 0.0022 [.0017,.0027] | 0.0022 [.0017,.0028] | 0.0024 [.0018,.0030] |
| 4p | 0.0000 [0,0] | 0.0016 [.0012,.0020] | 0.0018 [.0014,.0022] | 0.0019 [.0015,.0024] | 0.0020 [.0016,.0025] |
| 5p | 0.0000 [0,0] | 0.0011 [.0008,.0014] | 0.0013 [.0010,.0017] | 0.0014 [.0010,.0017] | 0.0015 [.0012,.0019] |

### Drifter

| `LeaseBlockRounds` | 1 | 2 | 3 (default, from cost sweep) | 4 | 6 |
|---|---|---|---|---|---|
| 2p | 0.0029 [.0022,.0036] | 0.0109 [.0094,.0123] | 0.0164 [.0147,.0182] | 0.0221 [.0200,.0242] | 0.0312 [.0288,.0337] |
| 3p | 0.0020 [.0015,.0025] | 0.0075 [.0065,.0084] | 0.0119 [.0107,.0132] | 0.0173 [.0158,.0188] | 0.0245 [.0227,.0262] |
| 4p | 0.0016 [.0012,.0020] | 0.0055 [.0048,.0062] | 0.0084 [.0075,.0093] | 0.0123 [.0112,.0134] | 0.0183 [.0169,.0196] |
| 5p | 0.0011 [.0008,.0013] | 0.0055 [.0048,.0061] | 0.0082 [.0074,.0090] | 0.0115 [.0105,.0124] | 0.0164 [.0152,.0175] |

**`LeaseBlockRounds=1` gives exactly 0.0000 for Operator at every player count — a precise, reproducible mechanism, not noise.** `internal/bots/operator.go:508` always funds a fresh stake with `const blocks = 1`. At `LeaseBlockRounds=1`, that stake has `RoundsRemaining=1` the moment it's placed; by the next round's `Decide`, upkeep has already expired and removed it from `v.You.Posts`. `internal/bots/runner.go:471-494`'s `maybeRenewLease` (the renewal heuristic Operator shares with Runner) only renews a post that's still *in* `v.You.Posts` with `RoundsRemaining <= 2` — it never sees the post again once it's gone, so Operator can never catch a one-round-duration lease before it lapses. This isn't cost-sensitive: the grid below shows the same exact-zero at `rounds=1` for `cost∈{1,3,6}`.

**Excluding `LeaseBlockRounds=1`, Operator's rounds curve is nearly flat (0.0011–0.0027); including that case, the measured range is 0.0000–0.0027 — pushing duration up does not close the gap.** Drifter's curve keeps climbing (0.003→0.031 at 2p from rounds 1→6) because it has no renewal-timing dependency, but even its best value in this sweep is 30× below the failing floor.

### Practical ceiling and true collapse-to-zero (supplementary points, 4p)

| Configuration | Operator | Drifter |
|---|---|---|
| `LeaseBlockRounds=15` (one stake covers the whole 15-round match) | 0.0021 [.0016,.0025] | 0.0278 [.0261,.0295] |
| `LeaseCostPerBlock=1, LeaseBlockRounds=15` (cheapest realistic cost × longest possible duration — the single most favorable point in the entire economic space) | 0.0026 [.0021,.0030] | 0.0666 [.0639,.0693] |
| `LeaseCostPerBlock=20` (roughly 1.7× `StartingBalance`) | 0.00028 [.00011,.00044] | 0.00005 [−0.00002,.00012] |

Pushing `LeaseBlockRounds` to 15 — a single lease that, once staked, would need no renewal for the rest of the match — moves Operator from 0.0018 (default) to 0.0021: essentially flat, confirming duration was never the constraint for Operator. Even stacking the single cheapest cost with the maximum duration only reaches 0.0026 (Operator) / 0.0666 (Drifter) — the best-case point in the whole space, still 15–380× below `< 1`. At the high end, `LeaseCostPerBlock=20` drives both tiers down to their lowest measured values, Drifter's interval reaching down to touch zero — but the sweep has no points between 10 and 20, so this is the first tested near-zero observation, not a located collapse threshold.

**The two named ends, as the issue requested:** `LeaseCostPerBlock=20` is the first tested near-zero point (roughly 1.7× `StartingBalance`, i.e. more than a fresh player can ever afford for one block) — narrowing exactly where between 10 and 20 the curve reaches zero would need intermediate points this sweep didn't run. The cost at which players hold the post cap **is not reached anywhere in this sweep** — not because the sweep didn't look, but because §Root cause identifies a gate upstream of the economic dial entirely that neither `LeaseCostPerBlock` nor `LeaseBlockRounds` can move past.

## 2D grid — does cost/rounds collapse to one effective dial? (4p, Operator)

| `LeaseCostPerBlock` | `LeaseBlockRounds` | effective cost/round | mean |
|---|---|---|---|
| 1 | 1 | 1.0 | 0.0000 |
| 3 | 3 | 1.0 | 0.0018 |
| 6 | 6 | 1.0 | 0.0015 |
| 1 | 3 | 0.33 | 0.0024 |
| 3 | 6 | 0.50 | 0.0020 |
| 1 | 6 | 0.17 | 0.0026 |
| 3 | 1 | 3.0 | 0.0000 |
| 6 | 3 | 2.0 | 0.0012 |
| 6 | 1 | 6.0 | 0.0000 |

**Cost and rounds do not collapse to one dial wearing two names — the issue's own hypothesis doesn't hold, and the `rounds=1` mechanism explains why.** The three cells sharing effective cost-per-round = 1.0 give 0.0000, 0.0018, and 0.0015 — not equal, and the outlier is entirely explained by the `LeaseBlockRounds=1` renewal-timing gap above, not by the ratio. Excluding every `rounds=1` cell (the degenerate case), the remaining points are consistent with cost and rounds acting as two independent, small, roughly-additive effects rather than one ratio: at fixed rounds, mean falls as cost rises (as the cost sweep already showed); at fixed cost, mean rises as rounds rises (as the rounds sweep already showed). Neither axis, alone or combined, produces a value that approaches the band.

## Root cause

**The bottleneck is not `LeaseCostPerBlock` or `LeaseBlockRounds` — it's `OperatorOptions.ChokepointTrafficRate` (0.98) and `ChokepointMinObserved` (9), a bots-package tuning pair outside `game.Config` and outside anything `--sweep` can reach.** Operator only ever *attempts* a fresh stake when `findChokepoint` (`internal/bots/operator.go:231-272`) identifies a node clearing 98% observed traffic rate over at least 9 observed rounds — a bar high enough that, per that function's own documented rationale:

> "The bar sits high enough that only a corridor near certainty ... clears it, so the behaviour fires rarely against real Heat Map data while remaining fully implemented" (`operator.go:85-88`)

That tuning was **already a deliberate, measured trade-off**, made before this issue and justified against a different metric entirely: the same comment states every lower threshold "measurably cost Operator mean RP against Runner." In other words, M2 already ran the experiment that set this gate high, optimized for RP performance, and that choice is what this sweep is now measuring the lease-rate consequence of. `LeaseCostPerBlock`/`LeaseBlockRounds` only govern what happens *after* Operator has already decided to stake — and Operator almost never decides to, so the economic dial has nothing to act on. This is consistent with Operator's curve being both an order of magnitude lower than Drifter's at every point (Drifter has no traffic-rate gate at all — it stakes uniformly whenever `ActionStakePost` happens to be drawn) and nearly flat against both cost and duration (neither can move a decision that's rarely being made).

## Verdict

**GDD §22's lease-rate exit criterion cannot be met by a `game.Config` edit.** The issue's own expected result — "a stated range of `LeaseCostPerBlock` × `LeaseBlockRounds` values that put live leases at scoring inside 2–4 per player ... with the shipped default either inside that range or changed to be" — presupposes a range exists inside the tested Config space. It does not: every one of the 103 configurations measured here, spanning cost 0–20 and duration 1–15 (the full sensible range — 15 is the whole match), stays at least an order of magnitude below even the `< 1` failing floor. No `Config` PR is being opened alongside this document, because none would be honest: changing `LeaseCostPerBlock` or `LeaseBlockRounds` to any value in the tested (or plausible untested) range does not move Operator's lease rate meaningfully, because the rate is gated upstream by `OperatorOptions`, not by `Config`.

This is the sweep finding its own shape, per D35's own framing for the "no point fully inside the band" case: *"reported as 'out of band across the tested range,' not as a missing breakpoint ... an instruction to extend the sweep further in that direction."* Here that direction is not further along `LeaseCostPerBlock`/`LeaseBlockRounds` — the practical ceiling and collapse-to-zero points above already bound that axis — it's outside `game.Config` entirely, in `internal/bots`'s own tuning.

**Recommendation:** this needs its own decision, not a unilateral fix inside this exit demo. The real choice is between (a) loosening `OperatorOptions.ChokepointTrafficRate`/`ChokepointMinObserved` and re-measuring the RP-performance cost that made the current values deliberate in the first place, (b) treating the lease-rate band itself as miscalibrated against what "a player who is trying" (D35's own Operator-tier rationale) actually does under the shipped rules, or (c) something in how `findChokepoint`'s heat-map read interacts with the `LeaseBlockRounds=1` renewal gap and other undiscovered mechanism-level gaps. Filing that as a decision (`docs/decisions/`) is a separate task from this measurement.

## Caveat, carried forward per the issue's own text

Bots lease for reasons a person may not share. Operator's chokepoint gate is a competent, narrower instruction than what a human posts for — a human might lease speculatively, for map control, or to deny a rival, none of which `findChokepoint`'s traffic-rate read models. The range (or absence of one) this sweep found is the range M5.5's human playtesting confirms or overturns, not the final answer — matching the roadmap's "it narrows the range that M5.5 then confirms."

## Scope note

`LeaseBlockRounds` was not swept below 1 (0 rounds held has no reading — a lease that expires before it's ever live) or above 15 (a single block already covers the entire 15-round match; nothing past that changes the mechanics). `LeaseCostPerBlock` was not swept below 0 (negative cost has no reading under GDD §5's non-negative-balance invariant) or meaningfully above 20 (already deep into the collapse-to-zero region on both tiers). The 2D grid was run at 4 players / Operator only, as a targeted cross-check of the "one dial, two names" hypothesis rather than a full grid at every player count — the hypothesis was rejected clearly enough at one player count that repeating it at 2/3/5 would not change the conclusion, and the compute would be better spent on whatever follow-up the root-cause finding leads to.
