# Exit demo: confrontation load has numbers — R9, the 2-player floor, R11

**Issue:** [#203](https://github.com/garnizeh/cinzal/issues/203)
**Milestone:** M2 — Bots and simulation
**Method:** [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md) — 10,000 matches per configuration, `mean ± 1.96 · s / √n` over the per-match vector, action only when the whole interval clears the threshold.

## Provenance

| | |
|---|---|
| Git SHA | `3c10114f3eca42b6abd4a7af9e3524787a50572e` |
| Root seed 1 (default) | string `cinzal-simulate-default-root-seed-v1` → `e4c50a633bfa5326029d36fcc00ea91af9510b523942539d4f89ed107866aa09` |
| Root seed 2 (independent) | string `cinzal-simulate-203-second-seed-e442e1e2347aede2a75412c37b179846` → `4fe7d4f25ae24809558d7bf6d89f43c24bd6679bf9c84feec97c0c50b2a5b4a1` |
| Config | `game.DefaultConfig()`, unmodified — the shipped default |
| Matches per configuration | 10,000 |
| Player counts | 2, 3, 4, 5 |
| Bot tiers | Drifter and Operator, both reported for every cell |

Raw CSVs for all 16 configurations (4 player counts × 2 tiers × 2 seeds) are in [`203/`](203/), produced by:

```
simulate --matches 10000 --players <N> --bots <drifter|operator> \
  --sweep Rounds=15 --seed <root-seed-string> \
  --out 203/p<N>-<tier>-<seed-label>.csv
```

`--sweep` is a required flag on `cmd/simulate` (at least one dimension), so `Rounds=15` is used as a no-op dimension — 15 is already `DefaultConfig()`'s own value, so every run's `Config` is the unmodified shipped default; the `sweep.Rounds` column in each CSV is constant and not a real sweep axis.

## Results

Both tiers ran at every player count; each threshold's verdict below is read against the tier D35 assigns it (Drifter for the two map-geometry questions, Operator for the strategy question). All figures are `mean [95% interval]` across `n = 10,000` matches, with the two independently-seeded runs shown side by side.

### R9 — confrontations per match, 4–5 players (Drifter). Target 4–12, action if the interval clears **> 12**.

| Players | Seed 1 | Seed 2 |
|---|---|---|
| 4 | 13.35 [13.28, 13.43] | 13.38 [13.31, 13.46] |
| 5 | 19.10 [19.01, 19.19] | 19.12 [19.03, 19.20] |

**Verdict: ACTION — raise node count before touching anything else.** Both player counts clear 12 by a wide margin, on both root seeds independently (no pooling needed — neither interval straddles the line). At 5 players the point estimate is over 1.5× the threshold. Per R9's own text, watch the Board-going-unused leading indicator as this gets addressed; that indicator has no computed row in this sweep and needs its own read.

### Two-player floor under rotating borders (Drifter, §6.3). Target 4–12, action if the interval clears **< 4**.

| Seed 1 | Seed 2 |
|---|---|
| 4.51 [4.46, 4.55] | 4.47 [4.42, 4.51] |

**Verdict: NO ACTION — the interval sits entirely above 4** on both seeds. The floor is not breached.

**This is the finding worth flagging, not the verdict.** GDD §6.3 itself states the fix (rotating borders + the 15-node map) puts the two-player rate "into the 9–12 band, comfortably inside target." Measured here, at the shipped default `Config` and against the tier D35 reads this threshold against, it is **4.5** — barely clear of the `< 4` failing line, not anywhere near 9–12. The exit criterion itself passes (4.5 > 4), so this is not an action item under the stated verdict rule, but the GDD's own narrative claim in §6.3 is not what this measurement shows, and that section should be corrected or re-checked against whatever produced "9–12" — a documentation task, separate from this demonstration's own scope.

### R11 — endgame camping, confrontations in the final 3 rounds ÷ all confrontations (Operator). Target < 30%, action if the interval clears **> 45%**.

| Players | Seed 1 | Seed 2 |
|---|---|---|
| 2 | 21.1% [20.8%, 21.5%] | 21.3% [20.9%, 21.7%] |
| 3 | 21.0% [20.8%, 21.3%] | 20.9% [20.7%, 21.1%] |
| 4 | 20.8% [20.7%, 21.0%] | 20.7% [20.6%, 20.9%] |
| 5 | 20.5% [20.4%, 20.7%] | 20.5% [20.3%, 20.6%] |

**Verdict: NO ACTION at every player count.** Every interval sits comfortably inside the < 30% target band, nowhere near the 45% failing line — the tightest margin (5 players) still clears the target band by almost 10 points. Per R11's own instruction ("do not pre-emptively patch this — verify it happens first"), the candidate remedies (halved confrontation Infamy in the final two rounds; stakes returned once contracts can no longer be delivered) stay unwritten.

**Standing caveat, carried forward per RFC §16.4 and the roadmap:** R11 is a strategic-incentive question, and Operator has no concept of being out of contention — it will never rationally farm a chokepoint out of spite the way a losing human might. A low number here against bots is weak evidence of a low number against humans. This result says the mechanic doesn't manufacture endgame camming on its own against a bot that's still nominally trying; it does not clear R11 for human play, and M5.5 should re-check it.

### Second-seed confirmation

Every verdict above is unchanged between the two independently-drawn root seeds — R9 clears at both 4p and 5p on both seeds, the 2-player floor clears (no action) on both seeds, and R11 stays under target at every player count on both seeds. No configuration landed in "watch" on either draw, so D35's pooling step was never triggered.

## R2 sanity check

GDD §20's R2 entry reports encounters under a pure random walk, 3,000 matches per setup, against the map shape that predates the 2-player fix (19 nodes, no rotating borders):

| Setup | R2 (random walk, pre-2p-fix) | Drifter here (shipped default, 10,000 matches, mean of both seeds) |
|---|---|---|
| 2p / 15 nodes, rotating borders / 4 steps *(R2's setup: 19 nodes, no rotation)* | 4.5 | 4.49 |
| 3p / 22 nodes / 4 steps | 11.3 | 8.25 |
| 4p / 25 nodes / 4 steps | 19.5 | 13.37 |
| 5p / 28 nodes / 3 steps *(current default uses 4 steps at every player count)* | 21.2 | 19.11 |

Every Drifter figure lands within the same order of magnitude as R2's — the harness is measuring the game, not itself, which is the bar this comparison exists to clear. The 2-player row's near-exact match is coincidental: R2's 4.5 was measured on the pre-fix 19-node map with no rotation, ours on the shipped 15-node rotating-border map; two different mechanisms landing on close numbers is not evidence either is wrong, just a reminder the comparison isn't apples-to-apples at that row.

The 3p/4p/5p rows run lower than R2 by 15–30%, not an order of magnitude. Divergence is expected, not a defect: Drifter draws uniformly over the *full legal-order space* (actions, stances, contract choices), not a pure positional random walk the way R2's model did — GDD §20 itself names this as the likely direction ("real players actively avoid each other, which pulls the number down"), and Drifter's stance/action noise plausibly does some of the same work. A gap this size, in the direction the spec already anticipated, is a result, not a bug.

## Scope note

3-player confrontation load was measured (drifter 8.24, operator 12.52) for the R2 comparison above, but no roadmap exit criterion governs 3 players on its own — R9 is scoped to 4–5 players and the floor threshold is 2-players-only. The 3p operator figure clearing 12 is informational, not a verdict: R9's threshold is read against Drifter, not Operator, by D35's own table.
