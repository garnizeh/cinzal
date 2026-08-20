# Exit demo: 4-player node count raised — R9 clears; 5 players hits the generator's ceiling

**Issue:** [#229](https://github.com/garnizeh/cinzal/issues/229)
**Milestone:** M2 — Bots and simulation
**Method:** [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md) — 10,000 matches per configuration, `mean ± 1.96 · s / √n` over the per-match vector, action only when the whole interval clears the threshold.

## Provenance

| | |
|---|---|
| Git SHA (base, before this PR's own commit) | `085d32e23c86e253c88a6faf469ec142598d29e6` |
| Root seed 1 (default) | string `cinzal-simulate-default-root-seed-v1` |
| Root seed 2 (independent) | string `cinzal-simulate-203-second-seed-e442e1e2347aede2a75412c37b179846` |
| Matches per configuration | 10,000 |
| Bot tier | Drifter — R9 is a map-geometry question, [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md)'s read tier |

CSVs in [`229/`](229/). `p4-drifter-seed*.csv` measures the shipped fix, produced the same way [#203](203-confrontation-load.md) was:

```bash
simulate --matches 10000 --players 4 --bots drifter \
  --sweep Rounds=15 --seed <root-seed-string> \
  --out 229/p4-drifter-seed<N>.csv
```

`p5-drifter-32nodes-seed*.csv` is evidence for the 5-player ceiling below, not a shipped configuration — `MapByPlayers[5]` was set to `{Nodes: 32, MinEdges: 46, MaxEdges: 51}` locally for this run only and reverted immediately after; `--sweep` has no path to a `MapByPlayers` edit (`cmd/simulate`'s reflection-based `--sweep` rejects composite-kind fields, `sweep_spec.go`), so this pair of CSVs was produced by editing `internal/game/config.go` in place, building, running, and discarding the edit — the same one-off shape [#229](https://github.com/garnizeh/cinzal/issues/229)'s own exploration used, just at full D35 sample size and both root seeds.

## What changed

`game.DefaultConfig().MapByPlayers[4]` raised from `{Nodes: 25, MinEdges: 36, MaxEdges: 40}` to `{Nodes: 28, MinEdges: 41, MaxEdges: 45}` — GDD §6.1's 4-player row. `MapByPlayers[5]` is unchanged; see "The 5-player ceiling" below.

## 4 players — confirms the fix

[#203](203-confrontation-load.md) measured 13.35 / 13.38 confrontations per match (seed 1 / seed 2) at the old 25-node map — clearing R9's `> 12` action threshold on both independently-drawn seeds. Re-measured at the new 28-node map:

| Seed | Confrontations/match |
|---|---|
| 1 | 11.81 [11.74, 11.89] |
| 2 | 11.75 [11.68, 11.83] |

**Verdict: fixed.** Both seeds' intervals sit entirely below 12, back inside the 4-12 target band, with room to spare.

## The 5-player ceiling

`MapByPlayers[5]` stays at `{Nodes: 28, MinEdges: 40, MaxEdges: 45}` — [#203](203-confrontation-load.md) measured 19.10 / 19.12 confrontations per match here, over threshold by a wider margin than the 4-player row was. Raising node count, the same lever that fixed 4 players, does not close this gap.

[D8](../decisions/D08-sector-size-constraint.md) fixes the map generator at exactly four sectors, each holding 3-8 nodes (`sectorSizes()`, `internal/rules/gen/sector.go`). That arithmetic puts the design ceiling at **32 nodes**: at 33, the split produces a 9-node sector, outside D8's stated range. 32 is therefore the highest node count `MapByPlayers[5]` can be raised to without reopening D8.

> **Correction ([D37](../decisions/D37-five-player-confrontation-load.md)):** the paragraph above originally added that 33 nodes is "rejected by `Params.validate()`." It is not. `validate()` bounds `Nodes` only from below, `sectorSizes()` has no cap, and D8's `[3, 8]` range is asserted only in tests — 33-36 nodes generate valid graphs today. The generator's real ceiling is **36**; at 37, the first 10-node sector overruns [D10](../decisions/D10-map-layout.md)'s fixed 9-cell layout lattice and `computeLayout` panics rather than failing validation ([#239](https://github.com/garnizeh/cinzal/issues/239)). This changes nothing about the measurement below — 32 remains the last node count inside D8's documented range, which is the number this section was choosing — only about what enforces it.
>
> **Update ([#239](https://github.com/garnizeh/cinzal/issues/239), 2026-08-20):** enforced. `Params.validate()` now bounds `Nodes` from above at four times the tighter of D8's per-sector cap and D10's lattice size — **32** today — so 33 really is rejected, by a returned error naming whichever limit bound. The original sentence describes today's behaviour again; it was aspirational when written.

Measured at that ceiling (Drifter, 10,000 matches, both root seeds, `Nodes: 32, MinEdges: 46, MaxEdges: 51` — average degree 2.88-3.19, the same band the shipped rows use):

| Seed | Confrontations/match |
|---|---|
| 1 | 16.54 [16.45, 16.62] |
| 2 | 16.57 [16.49, 16.66] |

**Verdict: raising node count cannot close the 5-player gap within D8's stated range.** At the largest map that range allows, the rate stays far above 12.

> **Follow-up ([D37](../decisions/D37-five-player-confrontation-load.md)):** continuing the curve *past* that range — which needs D8's per-sector ceiling and D10's layout lattice both raised — the threshold does eventually clear, at **46 nodes** (11.59 / 11.62, both intervals below 12). D37 declines to pay for it: by 46 nodes, §22's share-of-map-under-sight row has fallen out of its 30-55% band (0.281) and its Heat-Map-low-confidence row has risen out of its `< 40%` target (0.445), which is the "Board goes unused" leading indicator this document's next section is about. The 5-player row is deferred to M5.5.

A smaller-scale check (3,000 matches, single seed) also tried edge density outside the shipped ~2.8-3.2 average-degree band at 32 nodes, in both directions — sparser (~2.0) and denser (~4.0). Both made match generation pathologically slow: constraint 4's chokepoint band (3-5 edges between adjacent sectors) and constraint 2's degree cap (2-4) leave very little slack outside the documented band, which is consistent with why the shipped table never strayed from it. Edge-density tuning is not a lever this generator has room for either.

Closing the 5-player gap needs an architecture-level decision, not a `Config` edit — filed as [#232](https://github.com/garnizeh/cinzal/issues/232) rather than folded into this task, per this repository's own task-vs-decision discipline (CLAUDE.md: *"A task that can't cite a GDD/RFC section is really a decision — file it as one"*). That issue was later decided as [D37](../decisions/D37-five-player-confrontation-load.md).

## A gap in the leading indicator

R9's own GDD text (§20) and §22's confrontations-per-match row both name "the Board going unused" as the leading indicator to watch as node count rises — the failure mode where a bigger map clears the confrontation threshold by making most of it irrelevant rather than by giving players room to actually avoid each other. Checked against `internal/telemetry`'s current metric set while doing this task's own scope bullet ("check... before adding a new one"): no such row exists, in §22's table or in the code. This sweep did not read it, because there is nothing yet to read. Filed separately as [#233](https://github.com/garnizeh/cinzal/issues/233) rather than defined ad hoc here. ([D37](../decisions/D37-five-player-confrontation-load.md) later had to read it through the two nearest existing §22 rows — share of map under sight, and Heat Map entries at low confidence — which is a workaround for the missing row, not a substitute for it.)

> **Update ([D38](../decisions/D38-board-going-unused-indicator.md), 2026-08-20):** `#233` is decided, and this section's framing is corrected by it in two places. **The Board is §7.5's deduction UI, not the map** — "going unused" means players stop consulting the Log, Attribution and the Heat Map because collisions have made deduction unnecessary, which is §22's rows 15 and 16 and is UI instrumentation no bot sweep can produce. So this section's paraphrase — "a bigger map clears the threshold by making most of it irrelevant" — describes a real failure, but a *different* one: the remedy overshooting, which §22 rows 8 and 19 already measure headlessly. Those are the rows D37 read, and they were the right ones for the question it was asking, not a workaround.

## Standing caveat

Bot play is not human play (RFC §16.4, carried forward from [#203](203-confrontation-load.md)): Drifter's uniform-random legal-order draw is the statistical baseline this map-geometry question is read against, not a model of how a human avoids or seeks out rivals. This sweep says the shipped 4-player map's *shape* no longer forces collisions past the target rate under that baseline; it does not say what a human table feels like, which is M5.5's question.
