# D42 — R7's 2-player Infamy-9 climb: which of GDD §20's three named remedies fixes it?

**Status:** decided
**Blocks:** nothing directly — closes [#280](https://github.com/garnizeh/cinzal/issues/280); [#241](https://github.com/garnizeh/cinzal/issues/241) (the M5.5 hand-off) is updated to carry this decision's finding forward
**Decided:** 2026-08-24
**Issue:** [#280](https://github.com/garnizeh/cinzal/issues/280)

## The question

[#277](https://github.com/garnizeh/cinzal/issues/277)/[#281](https://github.com/garnizeh/cinzal/pull/281) confirmed, on a fresh independently-drawn seed pair, that [D39](D39-r1-confrontation-softening.md)'s confrontation-softening rule change measurably perturbed R7's 2-player cell: matches where a player reaches Infamy 9 (GDD §22 row 4, read against Operator) dropped from `11.02% [10.59%, 11.45%]` pre-D39 to `8.93% [8.53%, 9.33%]` post-D39 — below the `< 10%` action threshold, on both seeds independently as well as pooled. 3, 4 and 5 players are unaffected and stay well inside the `> 20%` target band.

GDD §20's R7 paragraph names a remedy for exactly this threshold, in a stated order:

> Watch for Tier IV contracts being accepted and then abandoned. If nobody voluntarily crosses Infamy 8, the dials are: extend the Tier IV deadline to 7 rounds; raise Tier IV to Cr$ 35 / 10 RP; or raise Legend to 3 steps. Try them in that order — the step-table change is the coupled one, because it moves every deadline in §8.3 with it.

Which of these three closes the 2-player gap — and does the third, coupled one need to be reached at all?

## Why it is open

None of the three is a documentation change or a `Config` default anyone has picked. #280's own filing named three reasons this needs settling rather than picking the first item off the list: the step-table change is explicitly coupled to every deadline in §8.3; the stated order is a guess, the same hedge shape ("most likely by") D39 found did not survive contact with R1's actual mechanism; and the trip is a single player count with a thin margin, while `Config`'s `Contracts` and `StepsByTier` fields apply globally, not per player count, so a 2-player-only fix may not be expressible without a new field.

## Options

- **A. Extend Tier IV's deadline** (`Contracts[3].Deadline`, 6 → 7 rounds). §20's own first-choice dial, aimed at the abandonment §280's own filing re-confirmed (65.3% of settled Tier IV contracts abandoned at 2 players, post-D39).
- **B. Raise Tier IV's reward** (`Contracts[3].Payment`/`RP`, Cr$ 30/8 → Cr$ 35/10). §20's second choice, aimed at making the crossing worth taking.
- **C. Raise Legend's step allowance** (`StepsByTier[3]`, 2 → 3). §20's third and explicitly coupled choice — every tier's deadline in §8.3 was calibrated against the current step table, so this option was never going to be a one-field sweep.
- **D. None of the three — reject on measurement, defer the row to M5.5.**

## Decision

**D. None of GDD §20's three named dials is adopted.** Options A and B were measured directly and rejected; option C was never run, because the same defect that sank A and B is visible in its own definition before a single match executes. R7's 2-player cell is deferred to M5.5's human playtesting, the same disposition already carried by R7's own 3/4/5-player Tier IV-abandonment note and by [D36](D36-lease-rate-chokepoint-gate.md)'s lease rate — recorded in [#241](https://github.com/garnizeh/cinzal/issues/241), not reopened here.

## Reasoning

### The measurement

A D35-rigor paired-cohort sweep: same two fresh, independently-drawn root seeds, same 10,000 matches per configuration, 2 players, Operator (R7's read tier), run against the shipped default and each candidate `Config` in turn. Pairing on identical seeds isolates the config edit's own effect from seed-to-seed noise, the same design D36 used for its own chokepoint-gate sweep. Git SHA `9020ebaac21b49bb1b450ccff767c8273e8f177e`, working tree clean before and after each run (`internal/game/config.go`'s tier-table edit made, built, measured, and `git checkout`-reverted immediately, per this repository's own established one-off-composite-field method — see `docs/exit-demos/229-node-count-raise.md`'s `MapByPlayers` precedent, `--sweep` cannot reach an array-of-struct field like `Contracts` either).

| Config | Seed 1 (`AnyPlayerReachedInfamy9`) | Seed 2 | Tier IV abandon rate (settled), seed 1 / seed 2 |
|---|---|---|---|
| Shipped default | 8.81% [8.25%, 9.37%] | 8.46% [7.91%, 9.01%] | 75.0% (24/32) / 56.5% (13/23) |
| **A** — Deadline 6→7 | 8.85% [8.29%, 9.41%] | 8.47% [7.92%, 9.02%] | 52.2% (12/23) / 38.9% (7/18) |
| **B** — Cr$ 35/10 RP | 8.81% [8.25%, 9.37%] | 8.46% [7.91%, 9.01%] | — (delivery reward, not deadline; abandonment mechanism untouched) |

**Option A moves the abandonment rate it was aimed at — the settled Tier IV abandon rate roughly halves, both seeds — and moves the climb-to-9 headline by nothing.** The paired deltas (+0.04pp seed 1, +0.01pp seed 2) sit inside a tenth of each seed's own ±0.56pp half-width. This is not "a smaller effect than hoped"; it is noise.

**Option B moves nothing at all.** Both seeds read bit-identical to the shipped baseline to four decimal places (`0.088100`/`0.084600` in both rows of the raw CSV). A reward paid on delivery cannot influence a decision made before the contract is even offered.

### Why, structurally, not just empirically

Both null results have the same cause, and it rules out option C without needing to run it. **Every one of the three named dials only takes effect for a player who has already reached Infamy 9:**

- Tier IV itself requires `InfamyRequired: 9` to be offered at all (GDD §8.3; enforced in `internal/rules/contracts.go`'s `tier.InfamyRequired <= infamy` eligibility check, `contracts.go:40`). A contract's deadline and reward — options A and B — cannot influence any decision made *before* that contract exists to be reasoned about.
- `StepsByTier[3]` (option C) is indexed by `infamyTierIndex`, which only returns 3 once Infamy is 9 or 10 (GDD §11's ladder: Legend is 9–10, and it is the *last* tier). A Legend's own mobility cannot affect whether a Nobody, Known, or Feared player crosses into Legend — it only affects what a player already there can do with it.

GDD §20's own framing treats this as a rational-actor question: *"if nobody voluntarily crosses Infamy 8"* is a bet that a bot (or a player) weighs the post-crossing trade — Tier IV's reward and deadline pressure, Legend's mobility — against the cost of becoming Legend (half the steps, Pressure, permanent exposure) *before* deciding whether to keep pushing toward 9. That lookahead is what all three dials are meant to sweeten. **None of the three bot tiers implement it.** Some existing code does read these fields, but only for the tier a bot is already in or has already been offered, never for one it has not reached: `internal/bots/operator.go` — R7's own read tier — decides Vanish timing off `InfamyComfortBand` (current Infamy only) and scores threats off `ThreatInfamyFloor` (opponent's current Infamy only), and neither `operator.go` nor `drifter.go` references `Contracts[]` or `StepsByTier` at all. `runner.go`'s `runnerOfferReachable` does read `cfg.Contracts[offer.Tier].Deadline` (`runner.go:590`) — but only when deciding whether to *accept an offer already on the table*, which for `offer.Tier == 3` cannot happen before Infamy 9 is already reached (the rules-level gate in `contracts.go:40` sees to that); and `internal/rules/steps.go`'s `Steps` indexes `StepsByTier[infamyTierIndex(v.You.Infamy, cfg)]` off the seat's *current* Infamy, so it only ever reaches index 3 for a seat that is already Legend. Both are existing-state reads of a tier already reached, not evaluations of a tier not yet reached — the distinction this decision turns on. Infamy 9 is reached, in every bot tier, as a side effect of the round-to-round heuristic (mostly by winning confrontations — 75.6% of 2-player crossings, per #280's own filing), never as the output of a multi-round plan that anticipates what Legend is worth.

This means M2's simulation harness cannot validate or invalidate any of GDD §20's three named remedies for this specific row, regardless of which values are chosen: the mechanism the remedy needs to move — a bot reasoning about a tier it has not yet reached — does not exist in any of the three bot tiers, at any difficulty setting `Config` can express. A dial aimed at Tier IV or Legend's own mechanics changes what happens after the crossing R7 measures, never the crossing itself.

### Why not C anyway, or a fourth dial

Running option C would cost real scope — it is the one GDD itself calls coupled, since every deadline in §8.3 was set against the current step table, so raising `StepsByTier[3]` without recalibrating Contracts I–IV's deadlines would be a different, unbudgeted change. Spending that cost to confirm a result the structural argument above already predicts (no effect on the pre-9 climb, since `StepsByTier[3]` is exactly as gated as the other two) is not a measurement worth taking. Inventing a fourth, untested dial — something that actually reaches the pre-9 climb, e.g. Contact Cooldown at low tiers, or the confrontation-win Infamy grant — is also declined here: GDD §20 names three specific things and this decision answers the question it was asked, not a broader step-gradient redesign. That redesign, if warranted, is its own decision with its own measurement, on the same "a task that can't cite a GDD/RFC section is really a decision" discipline this repository already holds itself to.

### The caveat this inherits

This decision's own issue, #280's filing, already carried forward the standing caveat from #205/#277: *"a better planner completes more Tier IV runs than Operator does, which would move the settled-abandonment number without any rule changing"* — this decision sharpens rather than resolves that caveat. It is not just that Operator might complete more Tier IV runs with a better heuristic; it is that **no simulated bot can exercise the specific judgement GDD §20's watch condition describes**, because none of them plan against a tier they have not yet reached. That is exactly the kind of forward-looking, cost/benefit reasoning a human table brings and no bot tier here was built to model — GDD §20's own R7 text calls it *"a statement about player judgement,"* and M5.5 is the venue built for reading player judgement against a bot baseline.

## Consequences

- **`game.Config` ships unchanged.** No PR retunes `Contracts[3].Deadline`, `Contracts[3].Payment`/`RP`, or `StepsByTier[3]` off this decision.
- **GDD moves to v2.30.** §20's R7 paragraph gains a correction noting the three named dials are rejected on measurement and structural grounds, and that the 2-player cell is deferred to M5.5, carried in #241 — mirroring how D39 corrected §20's R1 paragraph in place rather than leaving a since-falsified remedy standing unqualified. §22's row 4 band, threshold and read tier are all unchanged; this decision does not claim the band is wrong, only that the three named levers cannot move a bot's reading of it. Companion RFC moves to r36 (pointer only — GDD §20 prose is GDD-owned data, not an architecture concern).
- **[#241](https://github.com/garnizeh/cinzal/issues/241) gains a third deferred row.** #241's own text flagged this in advance: *"A third row landing here would be worth treating as evidence about the method rather than about the row."* Unlike the lease rate (D36) and 5-player R9 (D37) — both cases where the named lever was swept to its own ceiling and still fell short by a wide margin — this row's finding is sharper: the named levers were never reachable by bot simulation at all, for a structural reason (all three gate behind the very crossing event being measured) that a wider sweep of the same dials could not have changed. That is new information about the method, not a repeat of the first two: it says bot simulation cannot validate *incentive-shaped* remedies for a threshold that depends on a bot anticipating a game state it has not yet reached, no matter how the incentive is tuned.
- **[#207](https://github.com/garnizeh/cinzal/issues/207) (M2 tracking) is updated.** #280's row is checked off with this decision's outcome, and the tracking issue's own "None became a third deferral" sentence — written when only R1 and R6 had closed — is corrected: R7 is now that third deferral, with a pointer to this document's finding about what that means for the method.
- **What would reopen this.** A future bot tier that plans multiple rounds ahead — weighing Tier IV's reward or Legend's mobility against the cost of crossing before deciding whether to keep pushing toward Infamy 9 — would give M2's harness a mechanism these three dials could actually move, and re-running this sweep against it would be a fair test GDD §20's guess never got. Building that bot is a bots-package task with its own scope and its own review, not a consequence of this decision to take on unilaterally. Separately, if M5.5's human data shows the 2-player crossing rate reads as fine to actual players, that is evidence the `< 10%` action threshold itself is calibrated too high at 2 players specifically — a band question this decision explicitly declines to make on bot evidence alone, the same restraint D36 and D38 both held.
- **Reversible at no cost.** Documentation only. Nothing in `internal/rules`, `internal/bots`, or `internal/game`'s behaviour changes.
