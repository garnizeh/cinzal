# D41 — Does D39 leave Operator's edge over Runner (#194) too narrow to test?

**Status:** decided
**Blocks:** [#272](https://github.com/garnizeh/cinzal/issues/272) — the follow-up task that softens `TestOperatorBeatsRunnerOverAThousandMatches`'s assertion
**Decided:** 2026-08-24
**Issue:** [#265](https://github.com/garnizeh/cinzal/issues/265)

## The question

[#259](https://github.com/garnizeh/cinzal/issues/259) (D39's engine implementation) found `TestOperatorBeatsRunnerOverAThousandMatches` — issue #194's own M2 acceptance criterion, *"Operator beats Runner over 1,000 matches at 4 players on the golden seed set, by mean RP"* — now fails on its committed golden seed (root `0xa0`): Operator mean RP 2.233, Runner 2.238, a margin of −0.005. A quick four-root probe in #259 found the sign isn't even stable (three of four roots showed Operator ahead by a comparable margin), pointing at noise rather than a code defect — both tiers resolve confrontations through the identical, symmetric path D39 changed. Left undisturbed pending this decision: does the test's strict pass/fail assertion still hold, and if not, what should it assert instead?

## Why it is open

- **The test asserts a strict pass/fail on a single 1,000-match sample with no interval.** [D36](D36-lease-rate-chokepoint-gate.md) already drew this exact distinction for the same test: *"that test's own smaller sample (1,000 matches, no formal interval) answers a different question — 'does Operator beat Runner at all' as a one-off acceptance check — not a §22 exit-criteria measurement."* A margin the issue's own probe already suspects is noise-level cannot be safely read at that sample size.
- **[D35](D35-simulation-sample-size-and-verdict-rule.md) already specifies exactly the rigor this question needs** — 10,000 matches, `mean ± 1.96·s/√n`, a straddling interval re-run under a second independent root and pooled — but has never been applied to this golden test, which predates D35 and uses its own ad hoc 1,000-match harness.
- **The four options in #265 trade off against each other** without a measurement to settle which is safe: swapping to a favorable seed risks picking the answer wanted rather than the true one; softening to report-only is only honest if the margin really is noise, not assumed to be; a properly-powered comparison answers that question directly but hadn't been run; and whether Operator's design itself needs attention is a separate, larger question this test alone can't answer.

## Method

D35's rigor, applied to this test's own paired design: `runCohort` already drives both cohorts from the same 1,000-seed set (`seed[31]=byte(i)`, `seed[30]=byte(i>>8)`), so the per-match statistic is the **paired margin** — `OperatorRP[i] − RunnerRP[i]` for the same seed `i` — not two independent means. Extended to `n = 10,000` per root, `game.DefaultConfig()`, 4 players, both tiers, git SHA `bb97a89`. First root: the committed `0xa0`. D35's straddle rule — re-run under a second independently drawn root, pool both vectors into one interval, act only if the pooled interval clears zero, otherwise record "watch, unresolved at n = 20,000" — governs from there.

A temporary probe (`internal/rules/probe_d41_test.go`, reusing the existing file's own unexported `runCohort`/`mean` helpers, adding paired-margin variance and a 95% CI) was built, run, and reverted immediately — the shape D37/D38/D39 established for a decision that has to measure something it hasn't yet decided to ship.

## What the measurement found

| root | n | mean margin | 95% CI |
|---|---|---|---|
| `0xa0` (committed) | 10,000 | +0.00857 | [−0.00452, +0.02167] |
| `0xb7` (second, independent) | 10,000 | −0.00492 | [−0.01802, +0.00817] |

Both intervals straddle zero — D35's "watch," not "act," on either root alone. Pooling both into one 20,000-match paired-margin vector (exact combination from each root's own `n`, mean, and sample SD — both roots returned `s = 0.66824`, and the pooled sample SD comes to the same value to five decimals, which reads as this margin's population variance being a fairly stable property of the match/config, not of which root was drawn):

**Pooled margin: +0.0018 [−0.0074, +0.0111], n = 20,000.**

The pooled interval still straddles zero. Per D35 §3, that is the terminal state for this rule: *"record 'watch, unresolved at n = 20,000' and hand to M5.5's human playtesting rather than inflating `n` further to manufacture false precision."*

## Options

Reprised from #265, read against the measurement above:

1. **Swap the golden seed to one where Operator wins.** Rejected — the pooled result shows the true margin is statistically indistinguishable from zero; picking a positive-margin root would report a real value as if it were a stable finding.
2. **Soften the assertion to report-only.** The measurement confirms this is the honest read, not just the cheap one.
3. **A properly-powered comparison, at D35's own rigor.** This is what Method/above did — it settles options 1 and 2 rather than standing beside them.
4. **Treat Operator's route-planning edge as an open bot-design question.** Not closed by this decision — see Consequences.

## Decision

**Option 2, on the strength of Option 3's own measurement**, which is the D35 procedure applied directly: the pooled 20,000-match interval straddles zero, so D35's own rule says record "watch, unresolved" rather than act. `TestOperatorBeatsRunnerOverAThousandMatches` no longer asserts `margin <= 0` as a failure. It keeps its two existing fail-closed checks — both cohorts must reach round 15, and the two cohorts' mean RP must not compare exactly equal — and keeps logging the margin every run (the test's own existing doc comment: *"failing to report the margin is not"* a result worth having). What changes is the strict pass/fail line: it is removed, because the measurement now on record says the signal it was asserting does not reliably exist at any sample size this test practically runs.

#194's stated acceptance criterion (*"Operator beats Runner over 1,000 matches... by mean RP"*) is retroactively unsupported as a strict signal: a single-1,000-match paired comparison has enough sampling noise, on this measured population SD, to land on either side by chance alone (±0.013 half-width against a true margin close to zero) — the criterion happened to read true on the committed seed pre-D39, not because the underlying edge was solid. D39 didn't break a working assertion; it moved a coin flip's outcome, on a margin that this decision's measurement shows was never far from zero to begin with.

## Reasoning

**Why D35's rule, not a fifth option.** D35 already exists precisely to answer "is a threshold crossing (or, here, a margin's sign) a finding or noise" — re-deriving a different verdict procedure for this test would duplicate D35 rather than apply it. The paired design this test already uses (shared seed set, per-match margin) is exactly D35's own reasoning for pairing the confrontation-load and lease-rate sweeps: it cancels map/contract-pool variance out of the margin instead of letting it swamp the signal.

**Why not push past n = 20,000 to force a verdict.** D35 §3 states this explicitly for a pooled interval that still straddles: further inflation manufactures false precision rather than finding a true answer faster. The pooled interval here is centered near zero with a half-width almost six times its own mean (+0.0018 ± 0.0093) — nothing in the shape of that number suggests a third root would resolve it rather than just narrow the same straddling interval further.

**Why the near-identical sample SD across roots (0.66824 both) doesn't change the verdict.** It says the *spread* of per-match margin is a stable property of this match configuration (players, config, tiers) rather than of the root chosen — consistent with the margin's *variance* being driven by map generation and event-deck randomness common to the config, while its *mean* is what actually shifts (and stays near zero) across independent draws. This is offered as a datum, not a proof; nothing here traces it to a specific mechanism, and it isn't load-bearing for the decision — the decision rests on the pooled mean and interval, not on why the two SDs matched.

**Why this doesn't reopen D39.** [#259](https://github.com/garnizeh/cinzal/issues/259)'s own framing — both tiers resolve confrontations through the identical, symmetric `resolveConfrontations` path D39 changed — still holds; nothing in this measurement finds a tier-specific defect. D39's softened rule narrowed whatever edge existed; this decision's job was only to find out how much of that edge was ever real enough to assert as a strict pass/fail, and the answer is: not enough, at any sample size practical for a per-PR test.

## Consequences

- **`TestOperatorBeatsRunnerOverAThousandMatches` loses its `margin <= 0` failure**, keeping its two fail-closed structural checks and its margin logging. This is a small, self-contained diff confined to `internal/rules/bots_operator_golden_external_test.go`'s existing assertions — filed as [#272](https://github.com/garnizeh/cinzal/issues/272) rather than included in this decision's own PR, matching D39's own precedent of a decision PR that carries no implementing diff.
- **#194's acceptance criterion is superseded for this test by this decision.** #194 itself stays closed; nothing here reopens M2's already-shipped Operator tier.
- **Option 4 is not closed by this decision.** Whether Operator's route-planning edge over Runner's simpler heuristic is *supposed* to still be measurable after D39 — a question about `internal/bots`' tier differentiation, not about this test — is real and unresolved. It is not filed as a new task here: D35's own pooled-and-straddling outcome is explicitly handed to M5.5's human playtesting (matching [D36](D36-lease-rate-chokepoint-gate.md)'s identical disposition for its own "not reachable by the tested lever, at the tested power" finding), and a bot-design task opened now would have nothing further to measure that this decision hasn't already measured.
- **Reversible at low cost.** Nothing in `internal/rules` or `internal/bots` changes here — only a test assertion, in its own follow-up PR. A future decision (e.g., after M5.5 human data, or after a change to Operator's route planning) means re-running this same paired measurement, not unwinding shipped behavior.
