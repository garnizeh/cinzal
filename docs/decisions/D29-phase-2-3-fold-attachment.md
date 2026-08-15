# D29 — Where do Phases 2 and 3 attach to the fold, and what carries the contract choice?

**Status:** decided
**Blocks:** [#77](https://github.com/garnizeh/cinzal/issues/77) (determinism suite), [#80](https://github.com/garnizeh/cinzal/issues/80) (golden replay exit demo), [#81](https://github.com/garnizeh/cinzal/issues/81) (RNG index exit demo), [#84](https://github.com/garnizeh/cinzal/issues/84) (full match exit demo), and the two tasks this decision splits out: [#163](https://github.com/garnizeh/cinzal/issues/163), [#164](https://github.com/garnizeh/cinzal/issues/164)
**Decided:** 2026-08-15
**Issue:** [#152](https://github.com/garnizeh/cinzal/issues/152)

## The question

GDD §15 numbers eight phases. `Resolve` (`internal/rules/resolve.go:16-102`) implements 1 and 4-8. `GenerateOffer`, `AcceptOffer`, `DeclineOffer` (Phase 2, `internal/rules/contracts.go`) and `RefreshMarkets` (Phase 3, `internal/rules/market.go`) are exported and correct in isolation, but `grep -rn` over non-test code finds no caller for any of the four. Three sub-questions, answerable only together:

1. **Who runs Phases 2 and 3, and where do their draws land in the `seq` stream?**
2. **What carries the player's contract choice into the fold?**
3. **What populates a Black Market's — and a seat's — first offer, at round 1?**

## Why it is open

**The phases are absent from the pipeline and from the spec.** `Resolve` goes straight from the entry snapshot to `validate` to movement; RFC §6.7's pipeline diagram starts at `validate(orders)` and never mentions either phase. RFC §6.4's table places `contract.offer.tier`/`.pick` at "Phase 2" and `market.stock` at "Phase 3" and stops there — the only two rows in the table (of the seventeen it currently carries; see [#159](https://github.com/garnizeh/cinzal/issues/159) for the larger sync gap) whose phase falls outside the function that owns the index stream. §6.4's own standard — "every consumer must be enumerated, because an unaccounted draw is a replay divergence that surfaces months later" — is failed just as completely by an enumerated consumer whose *position* in the stream is undefined as by an unenumerated one.

**The golden test already works around it, in writing.** `internal/rules/golden_test.go:163`:

```go
offerRNG := NewRNG(seed, int(round)+100000) // independent of Resolve's own stream
```

a fabricated round index chosen precisely because there is no answer yet for where Phase 2's draws belong on the real stream. That is a reasonable thing for a test to do against an open question; it is not a thing #81 can assert against — "RNG index counts match the §6.4 table, including truncation cases" cannot cover two rows whose stream position is a test-local constant.

**The contract choice has no home in the order log.** RFC §7.1: `state = fold(Resolve, initial(seed, cfg), orderLog)`. `game.Order` (`internal/game/order.go:8-41`) has seven fields, none of them a contract choice; RFC §11.1's form list has none either. `AcceptOffer`/`DeclineOffer` return a mutated `MatchState` from an input that, today, has nowhere to live in that log — so a refold of a real match cannot reproduce which contract a player took, or whether they declined. That is a direct negation of §7.1, and it blocks #80.

**No Black Market ever has stock, and no seat's first offer is ever generated.** Nothing calls `RefreshMarkets`. `market.go`'s own doc comment on `MarketRefreshDue` already reasons that "round 1's own Phase 3 is what populates the first stock… precisely because Setup does not" — but nothing acts on that answer, and [D25](D25-item-market-resolution-gaps.md)'s own aside ("there is no separate Setup-time mechanism, and none is needed") turns out, once the fold's timing is worked through below, not to hold. The same gap exists for the opening contract offer: GDD §4's Setup box lists "opening contract offer" as something Setup produces, but nothing in `initial()` calls `GenerateOffer`.

## Options

### Sub-question 1 — where Phases 2 and 3 execute

**A — Inside `Resolve` itself, timed a round ahead**, the same shape `MatchState.UnstableSector` already uses (`state.go:318-331`): "the round about to begin, not the one just folded in… has to already be sitting here when the round before it returns." Cost: `Resolve` computes state for a phase the round it is resolving never used, and round 1 needs its own bootstrap since there is no `Resolve(round 0)` call.

**B — In `internal/match`'s tick, on a continuation of the same `*RNG`, immediately before orders open.** Matches the golden test's current shape. Cost: the index stream's order now depends on a second package coordinating with `rules` correctly on every call path — the live tick, `cmd/replay --rebuild`, the sampled determinism check (RFC §7.4, §15.4), and every server restart's fold. Each is a separate call site that must remember to invoke the right function, with the right RNG continuation, in the right order relative to `Resolve`. That is exactly the *caller* split D03 declined for `shuffleConstrained` — D03 decomposed the deck build's *purpose string* for trace clarity but kept both stages inside `initial()`, a single caller, specifically so no second call site could get the order wrong.

**C — A separate `Prepare(s, cfg, rng)` in `internal/rules`, called by the tick right before orders open.** Keeps the ordering inside `rules` (single-valued, testable) while keeping it out of `Resolve`. Cost: a second exported entry point into the fold. Every consumer of `state = fold(...)` — the live tick, `cmd/replay --rebuild`, §15.4's sampled check — is built around calling `Resolve` once per round; C requires each of them to also remember `Prepare`, in the correct position relative to `Resolve`, forever. `Resolve` already receives the single `*RNG` every draw in this package uses; C is the same hazard as B, one layer shallower, not a different one.

### Sub-question 2 — what carries the contract choice

**i — A new `game.Order` field.** `orders` is already keyed `(match_id, round, seat)` with the payload as the serialised `Order` (RFC §7.2); a same-round decision has exactly one existing place to live, next to the four other GDD §9 fields that already ride the same form.

**ii — A separate logged decision record.** A new table (or a new row shape) alongside `orders`, with its own primary key. Cost: `state = fold(Resolve, initial(seed, cfg), orderLog)` is a fold over *one* log; a second log means two records that must agree on round and seat before either can be applied, reintroducing exactly the "two independent things must agree on order" hazard sub-question 1 exists to eliminate — now at the schema level instead of the RNG level.

**iii — Folded into the *following* round's order.** The player sees the offer during round N but doesn't have to answer until submitting round N+1. Cost: contradicts GDD §8.2's own timing sentence — "Offers are evaluated once per round, at Phase 2, against the state at the close of the previous round" — which pairs one round's offer with that same round's evaluation, and needs the offer to stay "pending" across a whole extra round-boundary for no stated benefit anywhere in GDD §8.

### Sub-question 3 — round 1's bootstrap

Falls out of sub-question 1's answer directly: whichever mechanism generates a normal round's Phase 2/3 either also runs once, unconditionally, at Setup (extending `initial()`), or it doesn't and round 1 has no stock and no offer. There is no third shape — GDD §4 lists nothing else Setup does that could plausibly own it, and D23/D24 already establish that `initial()` is where per-seat, per-round-1 guarantees are proven, not deferred.

## Decision

**Option A for sub-question 1, Option i for sub-question 2, and `initial()` grows the responsibility for sub-question 3** — all three follow from the same underlying fact: everything that reads match state does so by calling `Resolve` (or `initial`) through the fold, so anything that must never be forgotten belongs *inside* one of those two functions, never beside them.

**Precisely, for sub-question 1:**

At the tail of `Resolve`, immediately after `upkeep()` and before `RoundAnchors` is built — using `next`, the fully-closed state `Resolve` is about to return, and the same `*RNG` parameter `r` every other draw in this package already uses — skipped entirely when `int(next.Round) >= cfg.Rounds` (no further round to prepare for; mirrors `nextUnstableSector`'s own `s.Round >= cfg.Rounds` guard, `incidents.go:216`):

1. **Contract offers**, seat-ascending (RFC §6.5's RNG-batch default): `GenerateOffer(next, seat, cfg, r)` is invoked for every seat, but the function's own existing cooldown check (`offerDue`, unchanged by this decision) is what decides whether it draws — a seat whose Contact Cooldown hasn't elapsed returns `(nil, false)` with **zero** RNG draws and no `PendingOffer` write, exactly as it already does today. A delivered result is stored on the offering seat (a new `Player.PendingOffer []ContractOffer`, nil otherwise). Exactly `PurposeContractOfferTier` × 2 plus `PurposeContractOfferPick` × 0-3 **per offering seat** — RFC §6.4's existing count, unchanged, only relocated.
2. **Market stock**, immediately after, NodeID-ascending Black Markets: `RefreshMarkets(next, r)` when `MarketRefreshDue(next.Round + 1)` — checking the *upcoming* round, not the one just closed.

Round 1 has no `Resolve(round 0)` to run this inside, so `initial()` bootstraps it directly: immediately after `seatPlayers` and before `initialUnstableSector`, on the same Setup `rng`, unconditionally (round 1 is always due — `GenerateOffer`'s cooldown check short-circuits on `LastOfferRound == 0`, and `MarketRefreshDue(1)` is trivially true) — same two steps, same order, same seat-ascending / NodeID-ascending conventions as the tail-of-`Resolve` case above, so a reader never has to learn a second convention for round 1.

`RoundsToNextOffer`/`offerDue` and `MarketRefreshDue` need no signature change — both already take exactly the state and round they need.

**Precisely, for sub-question 2:**

`game.Order` gains `ContractChoice *int` — nil declines a pending offer; a value in `0..len(PendingOffer)-1` accepts the correspondingly-indexed contract. This is the same "pointer means unset" shape `AddOns.OpenDoorsMarket` already uses, deliberately, rather than a new convention. Two distinct cases, not one: when `PendingOffer` is nil, `ContractChoice` is inert regardless of its value — there is no offer to accept or decline, so it is read and ignored, never treated as a decline (below). When `PendingOffer` is non-nil, an out-of-range `ContractChoice` is GDD §15.0's illegal-payload category (malformed relative to the offer the client was shown) and degrades to declining *that* offer — never a partial or best-effort accept.

Applied at the head of `Resolve`, immediately after `resetRoundFlags` and before `validate` — before, not after, because a seat already standing at the newly-accepted contract's origin can legally `Pickup` in the very same round, and `validate`'s legality check needs `p.Contracts` to already reflect the acceptance. The step reads `next.Players[seat].PendingOffer`; **when it is nil, the step does nothing at all for that seat** — `ContractChoice` is not inspected, `AcceptOffer`/`DeclineOffer` are not called, and `LastOfferRound` is left untouched, because there is nothing to accept or decline and calling `DeclineOffer` anyway would incorrectly restart a cooldown that was never due. Only when `PendingOffer` is non-nil does the step read `orders[seat].ContractChoice` and call `AcceptOffer` or `DeclineOffer` (both already correct, unchanged) with `round = next.Round`, then clear `PendingOffer` to nil regardless of which of the two fired — consumed exactly once, the same "decide early, use once" discipline `NextRoundModifiers` already follows for Scaffolding/Retainer/Blackout/Dockers' Strike. No RNG draw either way: applying a choice, or leaving one unmade, is a deterministic state mutation, zero index cost, like `resetRoundFlags` itself.

An absent player's default order (GDD §18) needs no new case: `ContractChoice`'s zero value is already nil, so "no add-ons, no lease renewal" extends to "no contract acceptance" for free — an absent player never commits to a job they didn't choose, consistent with the section's own "deliberately conservative" framing.

**Precisely, for sub-question 3:**

Covered above — `initial()`'s bootstrap is not a fourth mechanism, it is sub-question 1's answer applied once, at Setup, because no `Resolve` call exists yet to carry it. Round 1's opening offer is unconditionally `delivered = true` under D24's own guarantee (constraint 7 provably puts a valid pair in Tier I's pool, and Tier I is the guaranteed slot's target at Infamy 0) — the "held" case never fires at Setup, only in later rounds.

## Reasoning

**Every caller of `state = fold(Resolve, initial(seed, cfg), orderLog)` gets Phases 2 and 3 correct automatically, or none of them do.** That single sentence is the whole argument for A over B and C. RFC §7.4 already establishes the shape: `Resolve` is the one function every fold consumer — the live tick, `cmd/replay --rebuild`, the sampled determinism check (§15.4), a server restart's fold — is built around calling once per **round**, over every seat's order-log row for that round grouped into the one `map[game.SeatID]game.Order` `Resolve` already takes (RFC §7.1's tick pseudocode: `orders := loadOrders(...)` for the whole round, then one `rules.Resolve(state, orders, ...)` call) — never once per individual row. Baking Phase 2/3 generation into that same call means every one of those callers is correct the moment it calls the function it was already calling once per round. B and C both introduce a second call that each of those callers must independently remember, in the right position, with the right RNG continuation — and the cost of getting it wrong is invisible until a replay disagrees, which is the exact failure class RFC §6.4 opens by naming.

**This is the same shape D03 already chose for `shuffleConstrained`, correctly identified in the issue as a precedent rather than an analogy.** D03 had an option to decompose the deck build across two calls and rejected it — not because splitting the *purpose string* was wrong (it did that, for trace clarity), but because both stages stayed inside `initial()`, one caller, so nothing downstream had to coordinate two entry points. Option B recreates exactly the split D03 avoided, one level up the call stack.

**Timed a round ahead is not a stylistic choice — it is the only timing that satisfies GDD §8.2's own sentence.** "Offers are evaluated once per round, at Phase 2, against the state at the close of the previous round" is only satisfiable if the offer already exists before that round's Phase 4 orders are collected, which happens strictly before `Resolve` for that round is ever called. There is no position *inside* `Resolve(round N)` that could produce round N's own offer in time for round N's own orders — the state has to be sitting in `MatchState` already, which is exactly what `UnstableSector` already does for the Headline, `NextRoundModifiers` already does for four event cards, and what this decision extends to Phase 2/3. The market case is identical: `Deal`'s legality and `Project`'s own display of a market's stock both need round N's stock visible before round N's Phase 4, not partway through round N's `Resolve` call.

**D25's aside about round 1 needing "no separate Setup-time mechanism" was scoped to cadence, not to call-site placement, and doesn't survive the fold-timing argument above.** D25 answered *which* rounds refresh (odd, 1-15); it did not work through *when relative to `Resolve`'s boundary* that refresh has to already have happened. Once that question is asked, round 1's stock provably cannot come from "round 1's own Phase 3" in the sense D25's aside imagined (a call that runs as part of processing round 1 — which is exactly what `Resolve(round 1)` is, and which necessarily runs *after* round 1's orders, hence too late) — it has to come from somewhere that runs before round 1's orders exist at all, and the only such place is `initial()`. D25's decided cadence (odd rounds) is untouched; only its passing implementation-shape assumption is corrected here, the same way D24 corrected a loose end D7 had flagged and left open.

**Rejecting the separate-log and next-round options for the contract choice is the same argument as A over B, one layer down.** A second log (ii) reintroduces "two things must agree on order" at the schema layer instead of the RNG layer — the identical hazard class, differently dressed. Deferring the choice to the following round (iii) has no textual support and actively contradicts §8.2's "at Phase 2" framing; it would also mean a `PendingOffer` has to survive an *extra* round-boundary with its own bookkeeping, adding complexity with no rule motivating it.

**The contract choice is a wire/architecture question, not a GDD rule change.** The underlying rule — accept 1 of up to 3, or decline, restarting the cooldown either way — is already fully specified in GDD §8.1-8.2 and is untouched by this decision. GDD §9's "five fields" framing needs no edit: `ContractChoice` isn't one of the five fields that make up "the Order — the heart of the turn," it's a same-round decision that happens to share the one synchronous input surface a round has, for the same reason RFC §11.1a's `round` staleness field lives on `Order` without being a GDD rule either.

## Consequences

**RFC edits, landing with this decision:**

- **§6.4's three affected rows** (`contract.offer.tier`, `contract.offer.pick`, `market.stock`) gain a "drawn a round ahead (D29)" note in the Phase column, and a new prose paragraph states the mechanism once rather than repeating it per row — the same shape §6.4 already uses for the deck-shuffle-at-setup explanation. This does not touch the table's other stale-row gap tracked separately in [#159](https://github.com/garnizeh/cinzal/issues/159) (the fourteen rows §6.4 is still missing entirely) or the `deck.event`/`deck.incident` naming #159 also flags — orthogonal to this decision, left for that task.
- **§6.7's pipeline diagram** gains a `prepareNextRound()` step after `upkeep()`, with the same one-line-per-step density the rest of the diagram uses, and a short prose note cross-referencing this decision — the same treatment r15's changelog gave `upkeep()`'s own four steps.
- **§11.1's order form** gains `contract_choice   optional, index into this round's offered contracts`. This edit is independent of [#160](https://github.com/garnizeh/cinzal/issues/160)'s already-flagged gap in the same section (Open Doors' `addons[]` entry) — different line, same section, no conflict expected.
- **A new changelog entry**, r23 → r24, recording this decision the way every prior RFC-affecting decision already has been.

**`internal/game/order.go` edit, landing with this decision:** `Order` gains `ContractChoice *int` and `Equal` gains the corresponding nil-safe comparison, mirroring `AddOns.OpenDoorsMarket`'s existing shape exactly. This is a struct/wire-contract change, not a `rules`-package implementation — no behavior exists yet to consume the field, matching how D24 separated the decision's own spec edits from the implementation task that follows it.

**Two follow-up tasks, filed against this decision, not folded into it** (same discipline D24 used for #122):

- **[#163](https://github.com/garnizeh/cinzal/issues/163) — `internal/rules`: wire Phases 2 and 3 into the fold.** `Player.PendingOffer []ContractOffer`; the tail-of-`Resolve` and head-of-`Resolve` steps above; `initial()`'s bootstrap; `golden_test.go:163`'s fabricated `offerRNG` replaced with the real path. While touching `MarketRefreshDue`'s call site, this task should also close [#162](https://github.com/garnizeh/cinzal/issues/162) (the function's hardcoded `15` instead of `cfg.Rounds`) rather than propagate the same literal into new code that calls it.
- **[#164](https://github.com/garnizeh/cinzal/issues/164) — `internal/game` / `Project`: surface `PendingOffer` on `SelfState`.** Unlike `StepAllowance`/`RoundsToNextOffer` ([D27](D27-project-config-parameter.md)), this needs no `Config` — it is exposed state, not a formula — so `Project` sets it directly, the same way it already sets every other exact, complete `SelfState` field. No RFC §9.1 table edit: that table is the sixteen-writer *opponent*-position table, and `PendingOffer` is the player's own state, the same category `Contracts`/`Items`/`Ledger` already sit in.

**Reversible at low cost today.** No `testdata/` or golden fixture is committed anywhere in the repository yet, `internal/match` is still `doc.go`-only, and `cmd/replay` likewise — so this decision has no stored replay to invalidate. Both follow-up tasks are the first real callers of the mechanism this decision fixes in place.
