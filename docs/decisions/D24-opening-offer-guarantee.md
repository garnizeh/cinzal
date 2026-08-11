# D24 — Does D7's opening-offer guarantee actually hold given constraint 6's real distance floor?

**Status:** open
**Blocks:** M1 — Rules core
**Issue:** [#120](https://github.com/garnizeh/cinzal/issues/120)

## The question

Does D7's own claim — *"reachability is the only thing left that can empty a pool"* — actually hold for the opening contract offer, given that a Nobody's eligible tier set is Tier I alone, and GDD §6.1 constraint 6 only guarantees Warehouse/Border non-adjacency (distance ≥ 2), not the ≥ 3 floor D7's reasoning assumed?

## Why it is open

Implementing #63's `GenerateOffer` faithfully to D6+D7+D23 and running its own acceptance criterion — *"Opening offer generates for every seat, on ≥ 1000 seeds, at 2/3/4/5 players — the §8.1 deadlock is a regression test, not a memory"* — fails on the very first seed it hits: `players=2, seed=1, seat=1`.

Concrete counterexample:

```
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

## Decision

*(To be filled in once resolved.)*

## Reasoning

*(To be filled in once resolved.)*

## Consequences

Blocks `internal/rules`' `GenerateOffer` (issue #63, branch `task/63-contracts`, WIP already committed and paused) — specifically its guaranteed-slot logic in `contracts.go` and its opening-offer regression test in `contracts_test.go`. Also touches #73 (Word of Work reuses the same `GenerateOffer`). Low cost to reverse now, while the contract subsystem is still unmerged; expensive once a golden-replay fixture or a stored match depends on which pool-widening rule produced its round-1 offer.
