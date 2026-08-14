# D26 — What happens when Dragnet and rotating borders would close every Border at once?

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-14
**Issue:** [#76](https://github.com/garnizeh/cinzal/issues/76)

## The question

GDD §6.3 (two-player rules) rotates which Borders accept deliveries — only half are active each round. GDD §14.2's Dragnet independently seals **two random Borders**, drawn from the full set, "every delivery must route to the ones that remain." Neither rule is aware of the other. At 2 players the map has only 3 Borders total (§6.2's node-type allocation table), so the two can coincide: Dragnet's seal can land on every Border this round's rotation left active, leaving **zero** deliverable Borders — a round where "the ones that remain" is the empty set, and Dragnet's own card text stops meaning anything. Issue #76 names this gap explicitly and neither the GDD nor the decision log resolves it. What closes it?

## Why it is open

The two mechanics were specified independently and never checked against each other:

- Rotating borders (§6.3, mechanism fixed by [D03](D03-rng-consumption-table.md)): sort Border nodes by `NodeID`, alternate labels A, B, A, B…; active set = A on odd rounds, B on even. At 3 total Borders this splits 2/1 — the active set is **1 or 2** Borders, never more.
- Dragnet (§14.2): draws exactly `min(2, total Borders)` seals from **every** Border on the map, unfiltered by rotation.
- Border counts by player count (§6.2's allocation table, keyed to §6.1's node counts): 2p → 3, 3p → 5, 4p → 6, 5p → 7. **Only the 2-player count is at risk** — Dragnet sealing 2 of 3 can exhaust every Border; sealing 2 of 5–7 never can, with or without rotation on top, since rotation itself only applies at 2 players (§6.3: "Both are 2p-only").
- Concretely: on an odd round (2 active), Dragnet's 2 seals land on exactly the active pair with probability `C(2,2)/C(3,2) = 1/3`. On an even round (1 active), any 2-of-3 draw includes that 1 active Border with probability `2/3`. This is common enough on a Dragnet round to need a real answer, not an edge case to hand-wave.

## Options

**A — Safety-valve reopen.**

- For: leaves both mechanics' own rules completely untouched — Dragnet still draws 2 random Borders from the full set (same RNG cost, same `PurposeEventDragnet`, no replay impact), rotation still alternates exactly as D03 specifies. The fallback is a single downstream check: after combining Dragnet's seal with rotation's inactive set, if the result would cover every Border, the lowest-`NodeID` Border reopens for that round. It is a pure function of already-computed data — no new draw, no new state.
- Against: introduces a special case that only ever fires in a genuinely rare corner (2 players, Dragnet drawn, and its 2 seals happen to hit the full active set) — a reader has to notice it exists at all.

**B — Restrict Dragnet's candidate pool to the active rotation set.**

- For: keeps rotation's "half the Borders accept deliveries" promise as the *only* source of unavailability during a round Dragnet doesn't touch, which reads cleanly in isolation.
- Against: does not actually solve the problem — it relocates it. Drawing 2 seals from a 2-Border active set can still seal both of them, and drawing from a 1-Border active set (the even-round case) seals the only one with a single draw. The pool restriction needs the exact same safety-valve fallback underneath it that Option A already provides on its own, so it pays Option A's cost and adds a second one: Dragnet's candidate pool would depend on player count and round parity, a card that otherwise resolves identically for every seat count and every round it appears in.

**C — Halve Dragnet at 2 players (seal 1 Border instead of 2).**

- For: simple to state, and guarantees at least one Border survives whenever the active set has 2 members (though not when it has only 1 — the even-round case can still be fully sealed by a single seal).
- Against: adds a **third** 2p-only special case to a section whose own text is explicit that there are exactly two — "Two changes apply at this player count only" (§6.3), naming rotation and the tighter map. It also changes a global event card's stated effect ("Two random Borders are sealed") conditionally on seat count, which nothing else in the deck does — every other card in §14.2 resolves identically regardless of player count. And it still doesn't fully close the gap on even rounds, so it would need its own fallback anyway.

## Decision

**Option A.** Neither Dragnet's draw nor rotation's rule changes. A new downstream check applies after both are computed: if the union of Dragnet's sealed Borders and this round's rotation-inactive Borders covers every Border on the map, the lowest-`NodeID` Border among them reopens for that round only.

## Reasoning

**The fallback has to live below both mechanics, not inside either of them, because the problem is an interaction, not a defect in either rule on its own.** Rotating borders is correct in isolation (it always leaves at least one Border active, by construction of the 2/1 split). Dragnet is correct in isolation (it always leaves at least one Border unsealed once there are 3+ Borders, which is true everywhere it can be drawn). Only the combination can reach zero, and only at 2 players. A fix that reaches into either mechanic to guard against the other — Option B — has to duplicate the same fallback anyway once the active-set-of-1 case is worked through, so it buys nothing extra for a real cost: Dragnet's candidate pool becoming player-count- and round-conditional.

**The fallback is structurally inert everywhere it isn't needed, which is what makes it safe to add without re-touching the balance work already done.** At 3+ players there are always at least 5 Borders (§6.2's table), rotation never applies (§6.3 is 2p-only), and Dragnet seals at most 2 — the union can never reach "every Border," so the reopen check can never trigger there. This mirrors D03's own framing for rotation itself: correctness that falls out of the data shape, not a branch on player count that has to be kept in sync by hand.

**Lowest-`NodeID` is an arbitrary but deterministic tiebreak, exactly like every other "pick one, deterministically" choice already in this codebase** (Fisher-Yates draws, `bySeat` ordering, D03's own A/B assignment) — it needs no RNG draw, costs nothing in the consumption table, and reproduces identically on replay. Nothing about *which* Border reopens is meaningful to the player; only that one does.

**Considered and rejected: doing nothing.** GDD §14.2's Dragnet text ("every delivery must route to the ones that remain") stops describing a coherent rule if "the ones that remain" can be empty — a player with a legal Deliver order would have no Border it could possibly resolve against, which is not degradation to `Nothing` (the existing behaviour for a Deliver at a sealed Border, `checkActionDegradation`/`EventDeliveryBlocked`) but an unreachable state the engine has no defined response to. That is worse than a rare special case; it is a gap the acceptance criteria for #76 call out by name.

## Consequences

- **GDD §6.3 gains one sentence**: a Border can never be closed by every source at once — at least one always remains open — cross-referencing this decision. Player-facing; the `NodeID` tiebreak mechanism itself is an implementation detail that belongs here, not in the GDD prose.
- **`internal/rules` gains one new pure function** (the union-and-reopen check) sitting between Dragnet's existing seal computation and rotation's new inactive-set computation, both already computed before Step 0 validation runs. No RNG consumption changes; RFC §6.4's table needs no new row.
- **No change to Dragnet's card text, candidate pool, or draw count**, and no change to rotating borders' A/B assignment. Both mechanics stay independently testable exactly as already planned; the fallback is tested as its own case (constructed 2-player state where the seal exactly equals the active set).
- Reversible at low cost: the fallback is a single, isolated check with no state of its own. If a future balance pass wants different behaviour (e.g., Dragnet simply not drawing on a round where it would fully close the map), that's a superseding decision, not a rewrite of surrounding code.
