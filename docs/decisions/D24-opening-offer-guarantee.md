# D24 — Does D7's opening-offer guarantee actually hold given constraint 6's real distance floor?

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-11
**Issue:** [#120](https://github.com/garnizeh/cinzal/issues/120)

## The question

Does D7's own claim — *"reachability is the only thing left that can empty a pool"* — actually hold for the opening contract offer, given that a Nobody's eligible tier set is Tier I alone, and GDD §6.1 constraint 6 only guarantees Warehouse/Border non-adjacency (distance ≥ 2), not the ≥ 3 floor D7's reasoning assumed?

## Why it is open

Implementing #63's `GenerateOffer` faithfully to D6+D7+D23 and running its own acceptance criterion — *"Opening offer generates for every seat, on ≥ 1000 seeds, at 2/3/4/5 players — the §8.1 deadlock is a regression test, not a memory"* — fails on the very first seed it hits: `players=2, seed=1, seat=1`.

Concrete counterexample:

```text
seat=1 infamy=0 position=11 contracts=0 lastOfferRound=0
known warehouses: [7]
all borders: [0 4 9]
distances from warehouse 7:
  border 0: dist=5
  border 4: dist=2
  border 9: dist=5
eligible tiers: [0]   (Tier I only — Infamy 0)
```

D23's fog seeding correctly gives this seat a Known Warehouse (node 7). But Tier I's band is `[3,4]` (GDD §8.3), and none of the three reachable Borders fall in it — one is *below* every tier's minimum (distance 2), the other two are above Tier I's ceiling. Since Infamy 0 only makes Tier I eligible, D7's cascade for every offer slot targets Tier I and has nowhere else to go: the pool is genuinely empty, and per D7's own rules the offer correctly **holds**. `GenerateOffer` is behaving exactly as D6+D7 specify — the specification has the gap, not the implementation.

**Root cause, already flagged (and left open) in D7 itself** (`docs/decisions/D07-contract-pool-fallback.md`, Reasoning section):

> "This decision takes GDD §6.1 constraint 6's stated floor — 'every delivery costs at least 3 steps' — as given, the same way the rest of the GDD does, so §8.2's 'can only happen if unreachable' is exact, not approximate... **One loose end is worth flagging without leaning on it: constraint 6's own two sentences aren't obviously the same fact — 'not adjacent' alone only forces distance ≥ 2, and whether the stated 3-step floor holds by construction or needs a sharper constraint that isn't spelled out is a map-generation question, not this decision's.**"

The counterexample above confirms the loose end is real: warehouse 7's distance-2 Border shows constraint 6 only guarantees ≥ 2, not ≥ 3.

There is a second, independent gap even where the ≥ 3 floor does hold: D7's "the four bands cover everything ≥ 3 with no gap" argument is about the **union of all four tiers**, which is only ever fully available to a Legend. A Nobody's eligible set is Tier I alone (width 2); Known adds Tier II (union `[3,6]`); a reachable Warehouse-to-Border pair can easily sit outside even the union of what a low-Infamy seat is eligible for.

This isn't cosmetic. GDD §8.1's own changelog frames v1.9's destination-reveal fix as making the opening offer "mathematically ...possible in every match" — the guarantee this gap breaks is the one the section is proudest of closing.

## Options

**A — Accept the gap.** The opening offer (and any offer to a seat whose eligible tier set doesn't span the reachable pool) may legitimately hold/empty, exactly like any other mid-match empty-pool case (D7's existing mechanism already handles it cleanly — no crash, no invalid contract, cooldown doesn't restart). Cost: contradicts GDD §8.1's own claim that the opening offer is now unconditionally possible, and requires weakening #63's acceptance criterion from "generates for every seat" to "generates or correctly holds."

**B — Widen the guaranteed slot's cascade above the eligible-tier ceiling, generally**, whenever the *full* eligible-tier union still leaves a reachable pair uncovered — not just at Setup. Cost: directly contradicts D7's explicit "never t+1 or above" rule and its own stated reasoning ("cascading up would pay them, and only cascading down is an acceptable fallback"). This would need to formally amend or supersede D7's text, not just add to it, and reopens the balance question D7 already closed (a low-Infamy player occasionally getting a higher-tier job than their Infamy earned).

**C — Strengthen `rules/gen`'s constraint 6** so every Warehouse D23's fog seeding can surface is structurally guaranteed at least one Border within `[3,4]`, not merely non-adjacent. Cost: reopens a closed, tested subsystem (#59/#60) for a map-generation constraint change; not obviously achievable within the existing bounded-retry generator for every player count/node total, and the tightest maps (12–16 nodes, D8) are the likeliest to make it expensive or infeasible.

**D — Scope the exception to the opening offer only.** GDD §8.1's actual claim and proof are about the very first offer specifically — D7's general "held" mechanism was built for the ordinary mid-match case, which nothing suggests was meant to carry the same unconditional guarantee. Round 1's guaranteed slot searches the full reachable pool (any tier, not capped at the seat's current Infamy eligibility) instead of D6's usual "target = highest eligible tier"; every later offer keeps D6/D7's rules exactly as already decided, untouched. Cost: needs to state precisely what tier a pool-widened opening contract is priced/labelled at (the tier its distance actually falls in, most likely — but that's exactly the kind of detail this decision needs to pin down), and is a one-round-only carve-out that needs to be justified as narrowly as the guarantee it's patching.

**E — Strengthen `rules/gen`'s constraint *7*, not constraint 6.** Not in [#120](https://github.com/garnizeh/cinzal/issues/120)'s original list — it was found while measuring D, by asking what constraint 7 is *for* rather than what it says. Constraint 7 currently accepts a start position if some Warehouse sits within 2 steps of it. Its own stated purpose is narrower than that test: *"Without this, the opening contract offer cannot be generated at all — see §8.1."* A Warehouse the seat Knows but which no Border sits at a contractable distance from satisfies the test and fails the purpose. E makes the test match the purpose — a start position qualifies only if some Warehouse within 2 steps *itself* has a Border at a distance inside Tier I's band. Cost: reopens a closed, tested subsystem (#59/#60) and changes every generated map; couples `gen` to one number out of §8.3; makes generation measurably slower.

## Decision

**E.** GDD §6.1 constraint 7 is strengthened. Its new text:

> **Every starting node has at least one Warehouse within 2 steps that itself has a Border at a distance inside Tier I's contract band (3–4 steps, §8.3).** Those nodes begin **Known** to that player. Without this, the opening contract offer cannot be generated at all — see §8.1.

Mechanically, in `gen.selectStartPositions` (`internal/rules/gen/start.go`), the existing `hasNearbyWarehouse` predicate is replaced by one that also requires the Warehouse to be *contractable*:

```text
contractable(w) := w.Type == Warehouse and
                   exists b with b.Type == Border and
                   openingMin <= distance(w, b) <= openingMax

qualifies(candidate) := exists w with distance(candidate, w) <= 2 and contractable(w)
```

`openingMin`/`openingMax` arrive on `gen.Params` (filled by `initial()` from `cfg.Contracts[0].MinDistance`/`.MaxDistance`, alongside the `MapByPlayers` fields already passed the same way), **not** as `gen` constants. `gen` must not import `game.Config` — it takes a `Params` struct precisely so the config table stays on one side of that boundary — and hard-coding `3`/`4` would let a §8.3 band edit silently stop satisfying the constraint that exists to serve it. `Params.validate()` rejects `openingMin < 1 || openingMax < openingMin`, so an unset band fails loudly at the caller-error check rather than exhausting `MaxAttempts` on a constraint nothing can satisfy.

**Nothing else changes.**

- **D6 is untouched.** The guaranteed slot still targets the seat's single highest *eligible* tier.
- **D7 is untouched.** The cascade still walks downward only, never to `t+1`, never by widening a band, never by relaxing the Known-origin rule. No opening-offer carve-out exists, because none is needed.
- **D23 is untouched.** It still seeds every Warehouse within 2 steps as Known; this decision only changes which starting nodes the generator is willing to hand it.
- **No RNG accounting changes.** `PurposeStartSelect` still consumes exactly `n-1` draws (one full shuffle), and the predicate that filters the shuffled order draws nothing. RFC §6.4's table and GDD §21's inventory are both unaffected. What does change is the *attempt count* — a stricter constraint 7 rejects more start-placement attempts (constraint 5 is untouched; the two are simply rejected together, as `constraintStartPlacement`) — but attempts were never fixed-cost, and `rand` is deliberately threaded across attempts without reset (#59), so this is the mechanism working as specified rather than a new cost to account for.

**Why this is a guarantee and not a probability.** At Setup, every premise the opening offer needs is pinned by a rule already decided:

1. No `Bridge Down` or `Sinkhole` has resolved, so the currently-navigable graph (GDD §9.1a item 0) *is* the setup graph the generator measured — the one place in the match where those two are provably the same graph.
2. Constraint 7, as strengthened, gives every seat's start a Warehouse `W` within 2 steps with a Border `B` at distance 3–4 from it.
3. D23 seeds every Warehouse within 2 steps of the start to at least `FogKnown`, so `W` is a valid origin.
4. Tier I's `InfamyRequired` is 0 and thresholds are cumulative, so Tier I is eligible for every seat at Infamy 0 — which is every seat, in round 1.
5. D6 fills the guaranteed slot first, against a without-replacement pool nothing has drawn from yet.

So `(W, B)` is in Tier I's pool when slot 0's cascade looks, and the opening offer delivers ≥ 1 contract. This is a proof, not a sweep result — the sweep below is a check on the proof, not the evidence for it.

**GDD §8.2's empty-pool sentence is corrected in the same edit.** It currently reads *"This can only happen if every Warehouse you Know is presently unreachable from every Border on the navigable graph."* That is false, and was false before this decision — a mid-match seat whose Known Warehouses all sit at distances outside every tier it is *eligible* for has an empty pool with everything perfectly reachable. D7's own Reasoning predicted this sentence would need revisiting if a distance-2 pair ever turned up. It has. New text:

> This happens when no Warehouse you Know reaches any Border at a distance inside a tier you are eligible for — either because the pair is unreachable on the navigable graph (most likely a `Bridge Down` that severed the last path), or because every reachable pair sits outside the bands your current Infamy entitles you to. Constraint 7 rules this out at Setup; nothing rules it out later.

## Reasoning

**Constraint 7 was already the right place for this fix, and was already trying to make it.** Every other option patches a downstream consumer of a guarantee that the generator was supposed to have provided and didn't. Constraint 7 is the only rule in the GDD whose stated justification is *the opening contract offer* — it exists for no other reason, and §8.1 spends a paragraph explaining why it is enough ("genuinely Known, and genuinely reachable"). That paragraph is about the *origin*. It never checks that a destination exists at a distance the origin can be paired with, which is the other half of what a contract is. Strengthening constraint 7 is not adding a new constraint to serve contracts; it is finishing the one that already claimed to.

**This is why E is not the Option C the issue rejected.** C proposed strengthening constraint **6** — then reading "no Warehouse is adjacent to a Border; every delivery costs at least 3 steps" — so that *every* Warehouse on the map has a Border in `[3,4]`. That is a global property of node-type placement, in tension with itself (constraint 6's first sentence pushes Warehouses and Borders apart; a `[3,4]` requirement pulls them back together) and plausibly infeasible on a 12-node map with four sectors. E requires the property of **one Warehouse near each of 2–5 start positions**, and gets to *choose the start positions* — the generator already walks a shuffled order of every node looking for a qualifying set, so tightening what qualifies costs a longer walk, not a structural redesign. The issue's framing collapsed these two into "a map-generation fix," and they are not the same fix or the same cost.

**The gap has two independent causes and only E closes both.** [#120](https://github.com/garnizeh/cinzal/issues/120) identifies them: (1) constraint 6 guarantees distance ≥ 2, not ≥ 3, so a pair can sit *below* every band; (2) a Nobody is eligible for Tier I alone, so a pair can sit *above* the only band available. B and D attack (2) by widening which tiers a slot may reach. Nothing they can do addresses (1) — there is no tier to cascade into that covers distance 2. Measured over 1000 seeds × {2,3,4,5} players, every seat's opening offer:

| Players | Nodes | Baseline (D6+D7+D23 as decided) | Option D | **Option E** |
|---|---|---|---|---|
| 2 | 15 | 142/2000 = 7.10% | 107/2000 = 5.35% | **0/2000 = 0.00%** |
| 3 | 22 | 39/3000 = 1.30% | 4/3000 = 0.13% | **0/3000 = 0.00%** |
| 4 | 25 | 46/4000 = 1.15% | 5/4000 = 0.12% | **0/4000 = 0.00%** |
| 5 | 28 | 30/5000 = 0.60% | 0/5000 = 0.00% | **0/5000 = 0.00%** |

The shape of D's column is what identifies the two causes as separate and tells you which dominates where. D removes ~90% of the failures at 3–5 players and only ~25% of them at 2 — because the 15-node map is small enough that cause (1) is already the dominant one, and no amount of tier-widening reaches it. That is also why "adopt D and accept a small residual" was not a stable answer: the residual is 1 in 19 two-player matches, on the smallest supported map, and 2-player is the configuration the GDD has already had to special-case twice (D8's per-sector minimum, §6.3's rotating borders). E's column is zeroes because it removes the cause rather than the symptom, and the proof above says the zeroes were not luck. `initial()` returned no error on any of the 4000 graphs, so the stricter constraint never exhausted `MaxAttempts` either.

**E is the only option that leaves the balance question closed.** B and D both hand a low-Infamy player a contract from a tier their Infamy did not earn — D7 considered and rejected exactly that ("cascading down costs the player, cascading up would pay them, and only the first is an acceptable fallback"), and reopening it would mean amending a decided decision's stated reasoning, on a path that fires ~5% of the time at 2 players and ~0% at 5. A rule that only bends for players on small maps is a balance change disguised as a robustness fix. E changes no payout, no tier, no eligibility, and no offer: the opening contract a seat receives under E is an ordinary Tier I contract, indistinguishable from one it could already have received.

**The cost is real and it is retries, not per-attempt work.** Measured with `BenchmarkGenerate` (200 iterations × 3 runs, fresh seed per iteration), against a naive implementation that ran a BFS per Warehouse per candidate, generation slowed 22–40%. Hoisting the Warehouse scan out of the candidate loop — one BFS per Warehouse per attempt, ~24% of nodes (§6.2), computed before the shuffled walk begins — brings it to **+6% (2p), +6% (3p), +13% (4p), +10% (5p), +8% (16-node), +37% (12-node)**. What is left is mostly the generator rejecting more attempts, which is the constraint doing its job. In absolute terms the worst case is 51.3 ms → 56.5 ms per map at 5 players, once per match, off the round-tick path entirely. The 12-node D8 scenario case (0.375 ms → 0.51 ms) would cross `check-bench-regression.sh`'s 20% threshold and annotate the implementing PR; that check is advisory by design and this is exactly the case its header describes — a deliberate change, re-baselined when it lands, not a silent regression.

**Reversibility is why this lands now rather than after M1.** E changes the graph every seed produces. Nothing in `internal/` has a golden fixture or a `testdata/` directory today, so today that costs one test-fixture update (`TestSelectStartPositionsFindsValidSet`'s hand-built 8-cycle has no Border at distance 3–4 and stops satisfying the strengthened predicate — the fixture needs a Border added, not the rule relaxed). Once M3's stored matches and #77's golden replays exist, every one of them is pinned to a map the old constraint produced, and this becomes a migration instead of an edit.

## Consequences

- **GDD §6.1 constraints 6 and 7, §8.1 and §8.2 are edited, with a changelog entry — this PR.** Constraint 7 gains the contractable-Warehouse requirement. **Constraint 6 loses its second sentence** — "Every delivery costs at least 3 steps" is a claim this constraint does not enforce and never did (non-adjacency forces distance ≥ 2, and generated maps do produce distance-2 pairs). The 3-step floor is real but §8.3 supplies it: every tier's band starts at 3, so a distance-2 pair is in no tier's pool and can never be contracted. Deleting the sentence changes no behaviour — `gen` only ever implemented non-adjacency, and `contractCandidates` only ever filtered by band — but it removes the false premise D7's fallback reasoning leaned on, which is what let this gap sit undetected through two decisions. §8.1's "Constraint 7 (§6) then guarantees a **Known** Warehouse to originate from" paragraph gains the destination half of the guarantee; §8.2's "can only happen if... unreachable" sentence is corrected as quoted above. The GDD is authoritative on rules, so the rule text moves here, not in the implementing PR.
- **`internal/rules/gen` needs the code change, as its own task** ([#122](https://github.com/garnizeh/cinzal/issues/122)), following D7's precedent that a decision produces a document and the task produces the code. Scope: `Params` gains the two band fields and their `validate()` check; `initial()` fills them from `cfg.Contracts[0]`; `selectStartPositions` gains the hoisted contractable-Warehouse pass; `generate_test.go`'s `verifyStartsHaveNearbyWarehouse` tightens to match the new constraint (it is the property test that would otherwise still pass on a map violating it); `TestSelectStartPositionsFindsValidSet`'s 8-cycle fixture gains a Border at distance 3–4; the bench baseline is re-recorded.
- **[#63](https://github.com/garnizeh/cinzal/issues/63) is unblocked, with its acceptance criterion intact** — *"opening offer generates for every seat, on ≥ 1000 seeds, at 2/3/4/5 players"* stands exactly as written rather than being weakened to "generates or correctly holds," which is what every other option would have required. `contracts.go` needs no change from this decision at all: the WIP `GenerateOffer` on `task/63-contracts` is already correct, and was only ever failing because the maps it was handed could not satisfy it. #63 must merge after #122, or its sweep still fails.
- **[#73](https://github.com/garnizeh/cinzal/issues/73) (Word of Work) is unaffected** beyond reusing a `GenerateOffer` whose semantics this decision does not change.
- **RFC §6.4's RNG consumption table needs no edit.** `gen.startselect` remains one full shuffle, `n-1` draws.
- **D6, D7 and D23 stand unamended.** D7's flagged loose end — "whether the stated 3-step floor holds by construction or needs a sharper constraint that isn't spelled out is a map-generation question, not this decision's" — is resolved here in the direction D7 guessed: it needed a sharper constraint, and it was indeed map generation's to provide.
- **Cheap to reverse today, expensive after M3.** No golden fixture or stored match in the repo depends on a generated graph yet; every one created after this lands will.
