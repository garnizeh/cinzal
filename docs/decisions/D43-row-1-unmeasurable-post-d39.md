# D43 — GDD §22 row 1 is a hard-coded 0 after D39: instrument the residual, declare the row unmeasurable, or fix only the documents?

**Status:** decided
**Blocks:** M2's close — R1 is one of the seven exit-criteria rows, and its verdict is currently read two incompatible ways on `main`; closes [#283](https://github.com/garnizeh/cinzal/issues/283), unblocks [#287](https://github.com/garnizeh/cinzal/issues/287)'s R1 cell
**Decided:** 2026-08-25
**Issue:** [#283](https://github.com/garnizeh/cinzal/issues/283)

## The question

Since [#270](https://github.com/garnizeh/cinzal/pull/270) (#267 + #269), `telemetry.routesCancelledMidRoute` (`internal/telemetry/match.go:177-183`) returns a literal constant:

```go
func routesCancelledMidRoute(log rules.OrderLog) Rate {
	submitted := nonEmptyRoutes(log)
	if len(submitted) == 0 {
		return Rate{}
	}
	return Rate{Value: 0, N: len(submitted)}
}
```

The order log is read for the denominator only. GDD §22 row 1 — the row carrying R1's `> 15%` **action** threshold, the one that already forced a GDD §15 rule change — is structurally incapable of being non-zero, for any config, any seed, any player count, forever. **What should row 1 be?**

## Why it is open

**The harness and the exit demonstration disagree about what this row means, and both are on `main`.**

[#249](https://github.com/garnizeh/cinzal/issues/249) built exactly the right gate and it fires correctly. In every CSV of the #267 re-run (`docs/exit-demos/267/`):

```
RoutesCancelledMidRoute_mean       = 0.000000
RoutesCancelledMidRoute_half_width =            <-- empty
RoutesCancelledMidRoute_n          = 10000
RoutesCancelledMidRoute_excluded   = 0
```

An empty `half_width` is `cmd/simulate` declaring, in `run.go`'s own words, *"its absence is what tells a reader this is not a measurement to read a threshold verdict off of"* — the second half of §22's own reading rule: *"A zero-width interval is a degenerate sample, not a finding. Flag it and re-examine, never read it as a confident verdict."*

But `docs/exit-demos/260-r1-confrontation-softening.md` reads the same four bytes the opposite way:

> Row 1 reads exactly 0.00% at every cell, `N=10000` throughout — **a genuine measurement, not an unmeasured gap** — clearing the `< 15%` threshold with more margin than D39's own E+ prediction […] GDD §20's R1 entry and §22 row 1 can both be read against this figure going forward.

And [#241](https://github.com/garnizeh/cinzal/issues/241) lists R1 under **"closed, no action"** alongside R6 — a disposition that rests on that reading. One of these is wrong. A metric that cannot take any value other than 0 has not cleared a threshold; it has become unfalsifiable.

**The residual is real and uninstrumented.** #267's own investigation measured it with a throwaway probe (not merged — the D37/D38/D39 pattern): **0.18%**, 31 of 17,167 submitted routes with at least one halt, across 300 four-player Operator matches — map dead-ends and incident truncation of an already-converted blind walk. Well under the 15% line, but it is a real number that the shipped row reports as `0` and the shipped harness reports as unmeasured.

**The follow-up this needs was named in code and never filed.** `internal/telemetry/match.go:174-176`: *"Reading that residual precisely needs new instrumentation at the point a converted walk actually stalls, which is a distinct, separately-scoped task from this one."* No such issue existed before #283.

**Related, and part of whichever option wins.** `Event.HaltCause` and `Event.HaltStepsUnspent` ([#261](https://github.com/garnizeh/cinzal/pull/261)) are now **write-only**: `internal/rules/confront.go` populates them at all three halt sites (`confront.go:241`, `:289`, `:383`), and no production consumer reads either field — `internal/telemetry` mentions them only in comments explaining why it does not. They were added for row 1's numerator, and row 1 no longer reads them. `HaltCause`'s own doc comment (`internal/game/enums.go:269-272`) still describes itself as *"D39's split of GDD §22 row 1's numerator"*, which stopped being true at #270.

## Options

- **A. Instrument the stall point and make row 1 measure the residual.** A new `EventKind` (or field) emitted where a converted D39 blind walk actually stalls short — the dead-end and incident-truncation cases the probe found. Row 1's numerator counts those. Costs a new event kind appended to `game.EventKind`'s iota block, golden-fixture regeneration across `internal/rules/gen`, and a `bench-compare` run — for a quantity measured at 0.18% against a 15% line.
- **B. Declare row 1 structurally unmeasurable post-D39 and defer the read to M5.5.** The row keeps its definition, its band and its threshold; `telemetry` returns an empty `Rate` — no value rather than a false 0 — taking the same D35 exclusion path an empty denominator already takes. §22 marks the row the way rows 15/16/18 are marked. R1's verdict moves from "closed, no action" to the M5.5 deferral list in #241.
- **C. Keep the constant 0 and fix only the documents.** Correct `260-r1-confrontation-softening.md`'s reading and #241's disposition to say "no measurement, no verdict", and leave the code alone.

## Decision

**B, with C's document corrections folded in.** GDD §22 row 1 is recorded as **structurally unmeasurable by bot simulation post-D39**: `internal/telemetry` reports no value and no verdict for it, the exit demonstration and #241 are corrected to say the same, and the row's human read is deferred to M5.5 alongside rows 15 and 18. R1's rule change — D39's own subject — stands on the evidence that actually confirmed it (rows 2, 9, 10/11, 17 and 20 all reproduced D39's prediction closely); what is withdrawn is only the claim that row 1 itself *measured* anything after that rule shipped.

**`Event.HaltCause` and `Event.HaltStepsUnspent` are kept**, with their doc comments corrected to name their real consumer: RFC §11.3's narrated resolution list (M5) and §15.1's debug panel, not §22 row 1. They are not removed.

## Reasoning

### Why the numerator cannot come back, and it is D39 that removed it

This is not an instrumentation bug that a better read of the event stream fixes. `haltOrConvertMovement` (`internal/rules/confront.go`, #266) has three call sites — `resolveTie`, `resolveLoser`, and `resolveDecisive`'s corrected winner — and every one of them either converts the halt's unspent budget into further blind Pushing On steps (`unspent > 0`) or has nothing left to convert (`unspent == 0`, `haltMovement`'s only remaining call site). Once the boundary invariant #269 fixed holds — `len(Route)+PushingOn.Steps` lands at `step+unspent`, so a repeat halt in the same round cannot quietly shrink the movement loop's shared bound — that conversion always gets to run its course. **No `EventRouteHalted` the engine can emit distinguishes "converted and later fully spent" from "genuinely never spent" any more**, because under the shipped rule there is almost nothing in the second category to distinguish.

That is D39 working exactly as adopted. GDD §15 now says a participant whose declared route can no longer be walked keeps the round's remaining steps and spends them as §9.1 blind steps, losing the route and the action. Row 1 counts *routes cancelled mid-route*; the rule removed the cancellation and left only the loss of the declared plan. The row's numerator did not break — its subject was legislated away.

### The constant 0 is not a pass, and the two readings are not both defensible

D35's fourth bullet, and §22's own copy of it, is unambiguous: *"A zero-width interval is a degenerate sample, not a finding."* `interval()` (`cmd/simulate/stats.go:26-47`) returns `ok = halfWidth > 0` precisely so that a constant vector cannot be read as a verdict, and `run.go:248-259` writes the mean but suppresses `half_width` on that path. The harness has been printing "no verdict" for row 1 since #249 landed; `260-r1-confrontation-softening.md` read the printed `0.000000` as a measured pass anyway, and #241 recorded a disposition off that reading. **The document is wrong and the harness is right** — not the other way round, and not a wash. This is exactly the failure mode this repository already refuses in its gates: a routine reporting success having measured nothing.

Under B the harness stops printing even the mean: with `Rate{}` on every match, `rateMetric`'s `r.N > 0` excludes every match, so the row reports `_n = 0`, `_excluded = 10000`, and both `_mean` and `_half_width` empty. That is a stronger and less mistakable statement than the current output, and it comes from the exclusion path D35 already specifies, with no new mechanism.

### Why not A — the row's threshold is a trigger for a remedy that has already been applied

R1's threshold does not describe a target; it describes an *action*: **"if more than 15% of submitted routes are cancelled mid-route, the confrontation rule gets softened"** (GDD §20). That rule was softened, at M2, by D39, on the strength of the very measurement that tripped the threshold (17.3%–29.1% at Operator, every player count). Re-reading the same threshold after the remedy has shipped asks "should we soften the confrontation rule?" of a game whose confrontation rule is already the softened one — and the softening is *what erased the numerator*. There is no verdict left for the row to deliver in M2 that D39 has not already delivered.

Against that, option A buys precision on a residual measured at 0.18% — roughly 80× inside a line that no longer has a remedy waiting behind it — at the cost of a new `EventKind`, golden-fixture regeneration, and a `bench-compare` run. It would also be measuring a genuinely different quantity from the one §22 row 1 names: not "your plan was cancelled" but "your converted blind walk ran out of graph", which is a map-geometry and incident-deck fact, not a confrontation fact. Building a headless row for it is defensible work; doing it *as row 1*, under R1's threshold, would attach a confrontation-softening action to a number that confrontations no longer drive.

The residual is not thereby dismissed. It is real, its two causes are known (map dead-ends; incident truncation of an already-converted walk), and the question it actually raises — does a human player experience that stall as the lottery R1 named? — is a human question, which is why it lands with M5.5's read rather than in a new event kind now.

### Why not C alone

C's corrections are necessary and are adopted. C *by itself* is not sufficient: it leaves `Rate{Value: 0, N: len(submitted)}` — a literal, unreachable-by-any-input zero — standing in a metrics package as the permanent answer to an exit criterion, and leaves `cmd/simulate` printing a mean for it. A reader six months out finds a row that always reads 0, a CSV column that always says `0.000000`, and no statement in the code that this is intended. The empty `Rate` says it in the one place the number is produced.

### The `HaltCause` / `HaltStepsUnspent` disposition

Kept, for three reasons.

1. **They have a named consumer that is not row 1.** RFC §11.3 renders each round as a narrated list of events projected through the fog, and §11.5's first rendering guarantee is that an `Event` carries structured params, never prose — a halt narrated without *why* it halted, and without whether anything was left to convert, is exactly the kind of pre-rendered flattening §11.5 forbids the engine from doing on the render edge's behalf. The same stream feeds §13's `round_resolved` email and §15.1's debug panel.
2. **Removing them is not free.** Both fields are asserted at all three halt sites by `internal/rules/confront_test.go`, and `HaltCause` has a pinned `String()` table (`internal/game/enums_test.go`). Deleting a populated `Event` field to make a comment true is churn against a struct M5 is about to read.
3. **What is actually wrong is the comment, not the field.** `enums.go:269-272` and `event.go:341-355` both still explain these fields as row 1's numerator split. That statement is false post-#270 and is corrected — this decision's own "grep for the wrong statement elsewhere" obligation, since the same claim appears in two files.

## Consequences

- **`internal/telemetry` reports no value for row 1.** `routesCancelledMidRoute` returns `Rate{}` unconditionally; `nonEmptyRoutes`/`routeHaltKey` go with it, since nothing else reads them. `MatchSummary.RoutesCancelledMidRoute` keeps its field and its GDD-row comment, rewritten to state that the row reports no measurement and why. Filed as **[#289](https://github.com/garnizeh/cinzal/issues/289)**, which also carries the `HaltCause`/`HaltStepsUnspent` comment corrections, `cmd/simulate`'s row-1 breakdown block, and the package doc comment's absent-rows list. Not done here: this document is the decision, per the log's own "decisions produce a document, not code."
- **`telemetry.Match`'s `log` parameter keeps D34's fixed signature but loses its last row-level reader.** The order log is still validated structurally (`len(log) == 0`, and every round key inside `1..cfg.Rounds` — #263's check), and `cmd/simulate`'s `Breakdown` still reads it directly for `RoutesSubmittedByRound`, which remains a true order-log fact. The roadmap's own reason for the parameter — *"row 1's denominator needs order-log access regardless"* (§3's D33 note) — no longer holds, and #289 records that in the signature's own comment rather than changing the signature.
- **GDD moves to v2.31.** §22's framing count moves from seventeen headless rows to sixteen; row 1 is marked with the milestone that now produces its read (M5.5), the way rows 15, 16 and 18 already are; §20's R1 paragraph gains the same correction D42 gave §20's R7 paragraph — the threshold is recorded as spent, not as cleared. The band, the threshold, the denominator and the read tier are all unchanged: this decision does not claim `< 15%` was wrong, only that no post-D39 bot measurement can be read against it. Companion RFC moves to **r37** (pointer only — §17's "one computation, three sinks" and §16.4's harness description both stand as written).
- **The roadmap's "17 of 20" becomes "16 of 20"** in §1's deliverable table and M2's deliverable list. M2's exit-criteria table itself is left to [#287](https://github.com/garnizeh/cinzal/issues/287), which owns filling all seven cells and is blocked on this document for R1's; R1's cell reads **no measurement, deferred to M5.5**, not met and not failed.
- **`docs/exit-demos/260-r1-confrontation-softening.md` is corrected in place**, not rewritten: its "genuine measurement" reading of the `0.000000` column is withdrawn and replaced with this decision's reading. `205-r1-r6-r7.md`'s R1 sentence, which cites that document as authoritative for R1's verdict, is corrected with it. Both keep their numbers — the CSVs are not in question, only what was read off them.
- **[#241](https://github.com/garnizeh/cinzal/issues/241) gains a fourth row, of a fourth shape.** R1 moves out of "closed, no action" into the M5.5 hand-off. The three rows already there are "the named lever was swept to its ceiling and still fell short" (lease rate, R9 at 5 players) and "the named levers were never reachable by bot simulation at all" (R7 at 2 players, D42). R1's is a third shape and the most specific of the four: **the remedy shipped, and shipping it removed the row's own numerator.** M5.5's question for it is not "is the rate under 15%" — no bot run will ever say otherwise — but whether human players experience D39's converted blind walk, and the ~0.18% of it that stalls on a dead end or an incident, as the lottery R1 named.
- **What would reopen this.** A rule change that reintroduces a genuine mid-route cancellation path — anything that stops converting a halt's unspent budget — puts a real numerator back in reach and makes option A cheap and correct; the row's definition, band and threshold are all preserved here precisely so that reversal is a code change and not another decision. Independently, if M5.5's human read finds the residual stall *is* felt as R1 described, that is the evidence that justifies A's event kind, with a known question to point it at.
- **Reversible at documentation cost, plus one small diff.** Nothing in `internal/rules`, `internal/bots` or `internal/game`'s behaviour changes; no golden fixture moves; no RNG index moves. #289's telemetry diff is a few lines and is itself reversible.
