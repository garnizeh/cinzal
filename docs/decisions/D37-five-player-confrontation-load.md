# D37 — The 5-player R9 threshold can't be met by node count alone: raise the per-sector ceiling, add a fifth sector, or accept the row unmet?

**Status:** decided
**Blocks:** M2's exit-criteria closure for R9 at 5 players (GDD §22, R9/§20); any future `MapByPlayers[5]` retuning task
**Decided:** 2026-08-20
**Issue:** [#232](https://github.com/garnizeh/cinzal/issues/232)

## The question

[#229](https://github.com/garnizeh/cinzal/issues/229) raised `MapByPlayers[4]`'s node count (25→28) to clear R9's `> 12` confrontations-per-match threshold, per the roadmap's own action table ("raise node count before touching anything else"). The same fix does not close the 5-player row: [#203](../exit-demos/203-confrontation-load.md) measured 19.10 / 19.12 confrontations per match at 5 players against the shipped 28-node map (Drifter, both root seeds), over threshold by a wider margin than the 4-player row was, and `#229`'s own follow-up measurement found the rate still at 16.5 at what it believed was the generator's ceiling.

Closing that gap looked architectural rather than tunable — every candidate touches either [D8](D08-sector-size-constraint.md)'s four-sector, 3-8-nodes-per-sector rule or another §6.1 constraint the GDD states as fixed — so `#232` filed it as a decision rather than folding it into `#229`.

## Why it is open

- **The fix, if it exists, is architectural, not a `Config` edit.** Every option below touches a rule GDD §6.1 or §6.2 states as fixed.
- **5-player is the top of the player-count range** — nothing above it forces a decision here on behalf of some larger future count.
- **No measurement yet showed what a larger sector cap or a fifth sector would do to R9, or to anything else §6.1's constraints protect.** Each option is a real generator change, not a parameter sweep, and needs its own implementation before it can be measured.

## Options

**A — Raise the per-sector ceiling above 8 nodes, at 5 players only.** Keeps four sectors, so the Unstable Sector rotation, sector-majority scoring, and D8's own reasoning about chokepoint edge budgets carry forward unchanged at 2-4 players; only 5p's sector size range moves. `#232` judged this "likely the safer of the two structural options," on D8's own note that larger sectors are *easier* on the chokepoint budget than smaller ones — but flagged that a materially higher node count changes match pacing in ways R9 does not capture alone.

**B — Add a fifth sector at 5 players only.** Larger structural change: GDD §6.2 names exactly four sectors (Old Docks, Iron Low, Mist Heights, North Vale) with display identity, and sector-majority scoring and the Unstable Sector flag are both built on that count.

**C — Accept a formally unmet exit criterion at 5 players, pending human data.** GDD §22's R9 threshold is a design guess validated by bot measurement; M5.5 confirms or corrects bot-measured bands against real play regardless. Cheapest option; costs an M2 exit criterion staying formally unmet, the same shape [D36](D36-lease-rate-chokepoint-gate.md) accepted for the lease rate.

## Decision

**C**, on the strength of a direct measurement of Option A that `#232` asked for but did not yet have.

Option A is closed on its merits, not on its cost. The R9 threshold *is* reachable by raising node count — it clears at **46 nodes**, measured, at D35's own rigor. It is closed because of what the map looks like by the time it gets there: at every tested node count that clears R9 — 46, 48 and 52, the three the sweep measured — two other GDD §22 rows have already left their own target bands, in the direction R9's own text names as the thing to watch ("Watch for the Board going unused as the leading indicator," §20). Buying the confrontation row by thinning the Board past its own indicator is the failure R9 exists to catch, not a fix for it.

Option B is not measured here and stays unmeasured for the reason `#232` gave, plus one it did not have: a fifth sector has nowhere to be drawn (see Reasoning).

## Reasoning

**First: the ceiling `#232` and `#229` both cite is wrong, in a way that changes what Option A costs to test.** Both state that the generator caps at 32 nodes because 33 forces a 9-node sector "which `Params.validate()` and `sectorSizes()`'s own split arithmetic cannot produce." Neither is true. `sectorSizes(33)` returns `{9, 8, 8, 8}` without complaint — it is a plain `n/4`-plus-remainder split with no cap — and `Params.validate()` bounds `Nodes` only from below (`minSupportedNodes = 12`). D8's `[3, 8]` range is asserted in tests (`sector_test.go:30`, `generate_test.go:318`) at the node counts those tests enumerate, and nowhere in production code. Probed directly at 5 players with edges inside the shipped 2.8-3.2 average-degree band, 50 seeds per count:

| Nodes | `sectorSizes` | Result |
|---|---|---|
| 32 | `{8,8,8,8}` | 50/50 generated |
| 33-35 | `{9,8,8,8}` … `{9,9,9,8}` | 50/50 generated |
| 36 | `{9,9,9,9}` | 50/50 generated |
| 37 | `{10,9,9,9}` | **panic** — `computeLayout` → `partialShuffle` → `rand(purpose, 0)` |

The generator's real ceiling today is **36 nodes**, not 32, and the wall at 37 is [D10](D10-map-layout.md)'s fixed 9-cell layout lattice failing as a panic three frames down, not a validation rejection. Filed as [#239](https://github.com/garnizeh/cinzal/issues/239); the three merged places that restate the wrong figure are corrected in this decision's own PR. This matters here for one reason beyond accuracy: it means Option A's first four nodes of headroom need no code change at all, and everything past 36 needs a change to D10 — a document `#232`'s option list never mentioned.

**Method.** Since this decision governs one of D35's seven §22 exit-criteria rows, the points are measured at [D35](D35-simulation-sample-size-and-verdict-rule.md)'s stated rigor: 10,000 matches per configuration, `mean ± 1.96·s/√n` over the per-match vector, two independently-drawn root seeds, Drifter — R9's own tier, a map-geometry question. Git SHA `c379552`, `game.DefaultConfig()`, `--sweep Rounds=15`, the same invocation shape `#203` and `#229` used. `--sweep` has no path to a `MapByPlayers` edit (`cmd/simulate`'s reflection-based sweep rejects composite-kind fields, `sweep_spec.go`), so each point is a local edit to `internal/game/config.go`, built, run, and reverted — the one-off shape `#229` established. Node counts above 36 additionally need D10's lattice enlarged from a 3×3 grid of 9 cells to a 4×4 grid of 16, which was applied for every point in the sweep including the controls.

**Why enlarging the lattice does not contaminate the measurement.** The lattice is display-only in the strict sense: `Node.X`/`Y` reach `game.NodeView` (`fog.go:128`) and nothing else in `internal/rules` reads them. More importantly it does not perturb the RNG stream. `RNG.Next` derives its digest from `(round, seq, purpose)` and uses `n` only as a modulus on the result (`rng.go:63-82`), and `partialShuffle` consumes exactly `k` draws whether the lattice holds 9 cells or 16 (`shuffle.go:28-33`) — so `seq` advances identically and every downstream draw in the match is bit-identical. A 16-cell lattice changes which canvas cell a node is drawn on, and nothing else. The 32-node control confirms this empirically: measured under the enlarged lattice it reproduces `#229`'s independently-produced 32-node figures to four decimal places (16.5381 / 16.5745 here against 16.54 / 16.57 there).

**The curve.** 5 players, Drifter, 10,000 matches per configuration per seed. Edge ranges chosen to hold average degree inside the 2.8-3.2 band every shipped §6.1 row uses. The 28-node row is `#203`'s measurement of the shipped map, unchanged.

| Nodes | `sectorSizes` | Confrontations/match, seed 1 | seed 2 | R9 (`> 12`) |
|---|---|---|---|---|
| 28 *(shipped)* | `{7,7,7,7}` | 19.0986 [19.0109, 19.1863] | 19.1177 [19.0313, 19.2041] | trips |
| 32 | `{8,8,8,8}` | 16.5381 [16.4535, 16.6227] | 16.5745 [16.4883, 16.6607] | trips |
| 36 | `{9,9,9,9}` | 14.7754 [14.6903, 14.8605] | 14.6741 [14.5902, 14.7580] | trips |
| 40 | `{10,10,10,10}` | 13.2530 [13.1703, 13.3357] | 13.2288 [13.1459, 13.3117] | trips |
| 44 | `{11,11,11,11}` | 12.0826 [12.0009, 12.1643] | 12.1472 [12.0653, 12.2291] | trips |
| **46** | `{12,12,11,11}` | **11.5935 [11.5136, 11.6734]** | **11.6172 [11.5357, 11.6987]** | **clears** |
| 48 | `{12,12,12,12}` | 11.1175 [11.0377, 11.1973] | 11.1015 [11.0220, 11.1810] | clears |
| 52 | `{13,13,13,13}` | 10.3278 [10.2496, 10.4060] | 10.2555 [10.1768, 10.3342] | clears |

The crossing is between 44 and 46: at 44 both seeds' whole intervals sit above 12, at 46 both sit entirely below it. Per D35's verdict rule this is an action either way, not a "watch" — no interval on this curve straddles the threshold.

**What the map looks like at 46 nodes.** GDD §22 has no computed row for R9's own leading indicator — "the Board going unused" is named in §20 and §22's R9 row and is measured nowhere, which is what [#233](https://github.com/garnizeh/cinzal/issues/233) exists to fix. The two nearest existing rows are proxies for it rather than the thing itself, and both leave their target bands well before R9 clears:

> **Update ([D38](D38-board-going-unused-indicator.md), 2026-08-20):** the two rows read below are not proxies for the question this section is actually asking. "The Board" is §7.5's deduction UI, and *its* usage is §22 rows 15 and 16 — UI instrumentation no bot sweep can produce. Rows 8 and 19 are the headless guardrail on R9's **remedy**: whether raising node count has thinned the Board's own data past usefulness. That is exactly what this section measures, so the numbers and the conclusion below stand unchanged; only the label "proxy" was wrong.

| Nodes | Share of map under sight, final third *(row 8, target 30-55%)* | Heat Map entries at low confidence *(row 19, target < 40%)* |
|---|---|---|
| 28 *(shipped)* | 0.4095 [0.4084, 0.4106] | 0.3698 [0.3687, 0.3709] |
| 32 | 0.3712 [0.3702, 0.3723] | 0.3924 [0.3913, 0.3936] |
| 36 | 0.3391 [0.3381, 0.3401] | **0.4119** [0.4108, 0.4131] |
| 40 | 0.3127 [0.3119, 0.3136] | 0.4256 [0.4244, 0.4268] |
| 44 | **0.2901** [0.2893, 0.2910] | 0.4385 [0.4372, 0.4397] |
| 46 | 0.2813 [0.2804, 0.2821] | 0.4451 [0.4439, 0.4463] |
| 52 | 0.2556 [0.2549, 0.2564] | 0.4594 [0.4581, 0.4606] |

(Seed 1 shown; across the whole sweep seed 2 differs from it by at most 0.0012 on row 8 and 0.0007 on row 19, so the second seed changes nothing here and is omitted for readability.) For reference, the 4-player 28-node map `#229` shipped — the configuration this repository has accepted as correct — measures 0.4328 and 0.3634 on these two rows.

Row 19 leaves its `< 40%` target between 32 and 36 nodes; row 8 leaves its 30-55% band between 40 and 44. Both are outside their targets at every tested node count that clears R9 (46, 48, 52); the sweep did not measure the counts between and past them, and this decision claims nothing about those beyond the direction the eight measured points move. **Neither reaches its own documented *failing* line** — row 8's stated action is `> 65%` ("post sight still too generous") and has no threshold defined on the low side at all, and row 19's is `> 60%`. So this is not a second tripped exit criterion, and this decision does not claim one; it is two independent measures of observation coverage moving monotonically the wrong way, out of the bands the design chose, to buy the confrontation row. That is the pattern R9's own text instructs a reader to watch for, read through the only proxies §22 currently has.

**What Option A would actually cost, stated fully.** `#232` scoped it as a change to D8 and framed the rest as carrying forward unchanged. At the node count that actually clears R9 it is larger than that:

- **[D8](D08-sector-size-constraint.md) / GDD §6.1 constraint 3**: the per-sector range goes from `3-8` to `3-12`. D8's Reasoning is about the *lower* bound — whether a 3-node sector can absorb constraint 4's chokepoint budget under constraint 2's degree cap — and says nothing about a ceiling of 12.
- **[D10](D10-map-layout.md) / RFC §6.4 / RFC §11.2**: the 9-cell lattice has to grow. D10 chose Option C specifically because it "has no failure mode, because the lattice is sized (9) with headroom above the largest possible sector (8, per D8) before a single index is drawn" — a proof derived from the number Option A moves. A 4×4 lattice on the same fixed 1000×1000 canvas (RFC §11.2's `viewBox` is a literal constant, deliberately) cuts D10's minimum separation from 175 units within a sector and 150 across a quadrant boundary to 130 and 110. That is a player-facing rendering property, not an internal one.
- **GDD §6.1's table**: the 5-player map becomes 46 nodes against the 4-player row's 28 — 64% larger, where the two rows are identical in node count today and differ only in edge range. The largest step the table has ever held between adjacent player counts is 47% (15→22, and that one is a deliberate two-player special case, §6.3).

None of these is a reason on its own; together they are the shape of change that wants a design conversation and a human read, not a decision document acting alone — which is the same conclusion the leading-indicator measurement reaches by a different route.

**Option B lands on the same wall, plus one of its own.** Beyond the reasons `#232` gave — §6.2's four named sectors, sector-majority scoring, the Unstable Sector rotation — a fifth sector has nowhere to be drawn. D10 partitions the canvas into exactly four 500×500 quadrants indexed positionally against `sectorOrder` (`layout.go:23-28`, `computeLayout`'s `for qi, s := range sectorOrder`); a fifth sector has no quadrant, and giving it one means re-deriving the whole partition, not extending an array. Whether a fifth sector would move R9 at all is also unknown: it changes the chokepoint structure rather than the node count directly, and the mechanism by which that would reduce confrontation load is not obvious enough to assume. Measuring it means building it first. This decision does not rule Option B wrong — it records that it is unmeasured, that its cost is strictly larger than A's, and that A's own measurement gives no reason to expect B to escape the same trade.

**Why C is procedurally sound despite `cinzal-implementation-plan.md`'s "exit criteria are numbers, not code."** That line guards against skipping M2's *measurement*, not against a row whose measurement is complete. This row has numbers now — more of them than the criterion asked for: the full curve from the shipped map to well past the crossing, at D35 rigor, on two root seeds, with the two nearest leading-indicator proxies read alongside. The roadmap's own M2 caveat anticipates exactly this outcome: a sweep "tells you the shape of the parameter space... not the exact value. It narrows the range that M5.5 then confirms." The shape here is unusually well determined — R9 clears at 46 nodes and the Board thins out of two bands before it does.

**This is the second M2 exit-criteria row deferred to M5.5, and that is worth saying plainly rather than letting it accumulate quietly.** [D36](D36-lease-rate-chokepoint-gate.md) deferred the lease rate a day earlier, on a structurally similar finding: the criterion is unreachable by the levers bot simulation has, and the cheapest honest answer is to say so rather than to move the design until the number complies. Two of seven is a fact about how much of GDD §22's calibration bot play can actually carry, not a pattern of avoidance — but a third would be worth treating as evidence about the method rather than about the row.

**What would reopen this.** M5.5 reading the 5-player table as *actually* too collision-dense in human play — as opposed to formally over a bot-measured threshold — is the signal that makes Option A's cost worth paying, and at that point the node count is already known (46) rather than needing to be searched for again. The converse signal, a 5-player table that plays well at 28 nodes, means GDD §22's R9 band wants recalibrating at the top of the player-count range, which is a GDD edit and not a generator change. This decision deliberately makes neither call: nothing measured here says which of those two the human data will show.

## Consequences

- **`game.Config` ships unchanged.** `MapByPlayers[5]` stays `{Nodes: 28, MinEdges: 40, MaxEdges: 45}`. No PR retunes it off this decision.
- **[D8](D08-sector-size-constraint.md) and [D10](D10-map-layout.md) are not reopened**, and GDD §6.1's constraint 3, its node/edge table, and §6.2's four named sectors are unedited.
- **R9's 5-player row of M2's exit-criteria table closes as "measured across 28-52 nodes; clears only at 46, past the point two other §22 rows leave their target bands; deferred to M5.5"** — not as "watch" (D35's watch state is for a straddling interval; no interval on this curve straddles 12) and not as silently met. The 4-player row is unaffected and stays closed as met by `#229`.
- **GDD §22's 4-12 band and R9's `> 12` threshold are not edited.** As with D36, this decision has grounds to say the criterion is not reachable at an acceptable cost by the lever the roadmap names — not grounds to say the band is wrong.
- **The "generator caps at 32 nodes, enforced by `Params.validate()`" claim is corrected** in `docs/exit-demos/229-node-count-raise.md`, `internal/game/config.go`'s `MapByPlayers` comment, and GDD's v2.25 changelog entry, in this decision's PR. The real ceiling is 36 and nothing enforces it.
- **[#239](https://github.com/garnizeh/cinzal/issues/239)** is filed as a follow-up task: `gen.Params.validate` has no upper bound on `Nodes`, and 37+ panics in `computeLayout` rather than failing validation. Independent of this decision closing — it is a fail-closed gap, not a balance question.
- **[#233](https://github.com/garnizeh/cinzal/issues/233) is now load-bearing rather than tidy-up.** This decision had to read R9's own leading indicator through two proxies because the indicator itself has no computed row. A future revisit — M5.5's or otherwise — should have the real row to read.
  > **Update ([D38](D38-board-going-unused-indicator.md), 2026-08-20):** `#233` is decided, and no such row is coming. R9's indicator is the Board's *usage* — §22 rows 15 and 16 — which is M5/M5.5 UI instrumentation and structurally unavailable to bot simulation. The revisit gets it from human play, not from a telemetry task; rows 8 and 19, read above, are the headless guardrail on this decision's own question and were the right rows for it.
- **Reversible at low cost.** Nothing in `internal/rules`, `internal/game`, or the GDD's rules changes here; revisiting this decision after human data means re-running a sweep whose recipe is written down above, not unwinding a merged change.
