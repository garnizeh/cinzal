# D05 — What, exactly, happens in Phase 8 · Upkeep?

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-08
**Issue:** [#43](https://github.com/garnizeh/cinzal/issues/43)

## The question

GDD §4's phase diagram lists `Phase 8 · Upkeep (auto)` at the end of every round. RFC §6.7 ends the resolution pipeline with `upkeep()`. **Neither document ever says what it does.** This decision enumerates it: the exact steps, their order, whether each emits an `Event`, whether any are suppressible by [D11](README.md), and whether round 15 runs it at all.

## Why it is open

Everything upkeep plausibly owns is a per-round countdown, and RFC §6.6 names the exact failure mode that makes an unwritten list worse than an ordinary gap: *"Most of these are silent when wrong. A broken `LoiteringStreak` does not crash or corrupt anything — a rule simply stops firing, and nobody notices."* A countdown that never runs looks exactly like a game where nothing expired yet — there is no exception, no wrong number on screen, nothing to file a bug against.

The issue assembled a best-reading list from the rules that need a per-round decrement and have no other stated home, then named four things it does *not* settle: the order of steps within the phase, whether upkeep emits `Event`s, which steps [D11](README.md)'s subsystem-suppression flags silence, and whether round 15 runs upkeep before final scoring counts live leases.

## Options

**A — The issue's own proposed table, taken as written.** Roughly GDD declaration order: decrement leases, decrement contracts, advance the Contact Cooldown, clear `Flagged`/`EvasiveStepPenalty`, decrement `Sinkhole`, tick `LooseCrateHeldRounds`, clear next-round modifiers.

- For: matches the order the rules appear in the GDD; looks like a straight transcription.
- Against: breaks in two independent, GDD-contradicting ways once traced through with concrete numbers (worked below), and two of its eight rows don't belong in Upkeep at all — RFC §6.7 already places one of them (crate heat) in an earlier pipeline step, and the other (Contact Cooldown) turns out to need no per-round mutation whatsoever.

**B — Order by dependency, not by declaration; clear flags before anything that can re-set them.** This decision's answer, detailed below.

- For: both failure cases in A resolve correctly, verified against GDD's own worked text (§13's "a fresh debt while already Flagged simply refreshes it for the following round" only holds under this ordering).
- Against: the order isn't self-evident from reading the specs top to bottom — it has to be derived, the same cost [D9](D09-node-type-rounding.md) paid to get its rounding rule right.

## Decision

**Option B.** Upkeep runs once per round, after Phase 7, for every round including round 15, in this fixed sequence:

```text
0. Clear Flagged and EvasiveStepPenalty          — the value each seat entered this round with
1. Contract deadlines                            — decrement; on expiry: fire penalty, discard
                                                    the contract, drop any cargo held for it,
                                                    cascade through Debt (§13) if unpayable
2. Lease decrement                                — decrement every lease still held (including
                                                    any that survived step 1's Debt cascade);
                                                    expire at zero, emit the public lease-expired
                                                    anchor (RFC §9.1 row 4) regardless of cause
3. Sinkhole decrement                             — decrement; the node becomes passable again
                                                    at zero, no announcement
4. Next-round modifier clear                      — Streets Blocked, Distracted Guard,
                                                    Scaffolding, Retainer, Dockers' Strike,
                                                    Blackout: clear whichever fired for this round
```

Two rows the issue proposed **do not belong in Upkeep**, and are removed rather than reordered:

- **Contact Cooldown needs no Upkeep step.** `LastOfferRound` (RFC §6.6) is written once, when an offer resolves at Phase 2/5, and read at the next Phase 2 as `round − LastOfferRound ≥ cooldown`. There is no stored countdown to decrement — the round counter advancing (which happens in `Tick`, not `upkeep`) does the "advancing" for free. Giving Upkeep a step here would mean writing a mutation whose only effect is to be immediately correct already.
- **`LooseCrateHeldRounds` ticks in `writeTrail`, not Upkeep.** RFC §6.7's pipeline names it explicitly: `writeTrail() → Loitering evaluation …, crate heat, per-node logs, …`. That has to be where it lives — the 2-consecutive-round threshold is evaluated against *this round's* trail entries, and `writeTrail` is where those get generated, three phases before Upkeep runs. Placing the tick in Upkeep would double-count or skip a round depending on exactly how the off-by-one landed.

**Iteration order**, per RFC §6.5's default (no positional advantage, no RNG involved — plain seat index):

- Cross-seat steps (contract deadlines, lease decrement, next-round modifier clear where per-seat) iterate seats in **seat-index order**.
- A seat's own contract slots (up to 2) iterate by the contract's assigned ID — both can expire the same round, and each runs the full decrement → penalty → Debt sequence independently.
- Map-scoped state (leases by node, active Sinkholes) iterates by **NodeID**, reusing the same key RFC §6.5 already assigns to lease selection.

**Event emission.** Every step above emits an `Event` — Upkeep is not exempted from the "one representation, six consumers" rule (RFC §6.7). Visibility is decided the same way it always is, per-row:

- **Lease removal is the one row already in the eleven-writer table** (RFC §9.1, row 4: "Lease expired — Global"). It fires identically whether the lease hit zero on its own or was surrendered through step 1's Debt cascade — see Reasoning for why the two causes must **not** be distinguishable.
- **Everything else is private** to the affected seat's own `SelfState`/recap history — the contract-deadline miss and its penalty, the `Flagged`/`EvasiveStepPenalty` clears, the Sinkhole clear, the next-round-modifier clears. None of them has a row in RFC §9.1's table, and `OpponentView` carries no contract information at all, so there is no channel through which any of these could reach another seat without adding a twelfth writer this decision doesn't authorise.

**Suppression ([D11](README.md), open).** No step above needs a branch. Each operates by iterating state that already exists — held leases, active contracts, active Sinkholes, active next-round modifiers. When [D11](README.md) disables a subsystem, no such state is ever created (no Stake Post action, no incident cards drawn), so the corresponding step iterates zero items and is a no-op as a consequence of empty input, not of a suppression check written into Upkeep. This matches [D11](README.md)'s own framing, quoted in the roadmap: flags belong in `Config`, "not as branches in `rules`." Flagged as consistent with the eventual D11 resolution, not resolved by it — the same posture [D9](D09-node-type-rounding.md) took toward placement feasibility.

**Round 15 runs Upkeep.** The GDD §4 phase diagram places Phase 8 inside the `ROUND 1 … ROUND 15` block, with `FINAL SCORING` strictly after it closes — there is no branch drawn for a truncated final round, and inventing one would be an asymmetric special case the diagram doesn't show and neither doc states. So "still live at the end of round 15" (GDD §16) means **survives round 15's own Upkeep decrement**: a lease must hold at least 1 round remaining *after* that decrement to score its 2 RP and count toward sector majority. A contract that expires during round 15's own Upkeep is discarded before scoring counts "active undelivered contracts," so it pays its penalty instead of costing −2 RP — not a double cost, a different one, resolved by the same ordering as every other round.

## Reasoning

**Worked example — why leases must decrement *after* contract-driven Debt, not before.** A player holds two posts entering round 9: Post A at 1 round remaining, Post B at 5. That round, a contract deadline expires and the player can't cover the Cr$8 penalty — Debt triggers (GDD §13).

| Order | What happens | Result |
|---|---|---|
| **Leases first (Option A)** | Lease decrement runs: Post A ticks 1→0 and expires on its own, publicly, no Debt involved. The contract cascade then runs, finds only Post B, and surrenders *it* (5 rounds, gone) for the Cr$2 credit. | **Both posts are lost** from what GDD §13 specifies as a one-lease penalty. |
| **Contracts first (this decision)** | The Debt cascade runs first, finds Post A still present at 1 remaining — genuinely the "fewest rounds remaining" lease — and surrenders it, crediting Cr$2. Lease decrement then runs over the survivor: only Post B, which ticks 5→4. | **Exactly one post is lost**, matching §13. |

The GDD's own wording drives the fix: "fewest rounds remaining" is a claim about a value, and Option A lets that value already be mutated (via a different rule, in the same phase) before Debt gets to read it. Reading it before any of Upkeep's own decrements touch it is the only way "fewest remaining" means what a player watching the HUD would expect it to mean.

**Worked example — why `Flagged`/`EvasiveStepPenalty` must clear *before* the contract cascade, not after.** A player is already `Flagged` entering round 7 (set by a debt event last round; round 7's step formula already applied the −1). During round 7's own Upkeep, that same player's contract deadline expires and they again can't cover the penalty — Debt fires again, and per GDD §13, *"a fresh debt while already Flagged simply refreshes it for the following round."*

- **Clearing last (Option A):** the contract cascade runs, sets `Flagged = true` fresh for round 8. The clear step then runs, sees `Flagged = true`, and clears it — **wiping out the refresh the GDD's own sentence describes**. Round 8 silently gets full steps.
- **Clearing first (this decision):** the clear removes round 7's already-spent value before the cascade runs. If the cascade sets `Flagged` fresh, nothing downstream in this pass touches it again, and round 8 correctly applies −1.

This is the RFC §6.6 failure mode by name: nothing crashes either way, the counter still "advances," and only a test asserting the rule fires *on the correct round* — the discipline RFC §16.1's cross-round-state row already demands — would catch the wrong order.

**Why cause-blind lease removal isn't a simplification, it's a fog requirement.** A separate "surrendered for debt" trace, distinguishable from ordinary expiry, would newly disclose that a player is in debt — private information under GDD §5's Cr$-band-only visibility. RFC §9.1's existing row 4 is already singular ("Lease expired — Global"); keeping it that way isn't laziness, it's the same negative-assertion discipline RFC §16.3 holds the whole projection to: the two causes must produce byte-identical `Event` payloads, or the fog suite has a new leak to test for that this decision would have quietly introduced.

**Why Contact Cooldown and crate heat are corrections, not just omissions.** Both looked, on a first read of the issue's list, like they belonged — they're per-round countdowns with no other stated home, which is exactly the pattern the rest of the list follows. Tracing each one against where its consuming logic actually runs (Phase 2 read for the cooldown, `writeTrail`'s explicit pipeline position for crate heat) shows neither needs an Upkeep mutation at all. This is the same "verify before applying" habit this repository holds findings to generally — a plausible-looking row in a table is a claim, not a fact, until it's checked against where the value is actually read.

## Consequences

- **GDD §4 and RFC §6.7 gain the enumerated step list** in this decision, replacing the empty `upkeep()` stub name with the five-step, dependency-ordered sequence above.
- **The roadmap's `Resolve()` task is unblocked** (implementation plan, M1 task list: *"`Resolve()` as the fixed pipeline of RFC §6.7 … event/incident/pressure/upkeep"*) — it was waiting on this the same way it waited on [D3](D03-rng-consumption-table.md)'s consumption table and [D9](D09-node-type-rounding.md)'s rounding rule.
- **RFC §16.1's cross-round-state row gets five concrete test rows**: contract-deadline expiry-and-cascade, lease expiry-and-cascade-interaction, `Flagged` clear-before-cascade, `EvasiveStepPenalty` clear, Sinkhole decrement — each asserting the rule fires on the correct round, per that row's own standard. Two more tests are negative by design: assert `LastOfferRound` is untouched by `upkeep()`, and assert `LooseCrateHeldRounds` changes only inside `writeTrail`, guarding against either row silently migrating back into Upkeep in a future refactor.
- **A new fog test**: the two-cause worked example above (natural expiry vs. Debt-driven surrender) must serialise to identical `Event` bytes, matching RFC §16.3's negative-assertion style.
- **[D11](README.md) inherits no new obligation.** When it resolves, it only has to guarantee that suppressed subsystems never populate the state Upkeep iterates over — Upkeep itself needs no further change.
- Reversible at low cost now, while `rules.Resolve` is unwritten; expensive after golden replays exist, since every one of these steps changes cross-round state that a fixed-seed fixture would bake in — the same argument [D8](D08-sector-size-constraint.md) and [D9](D09-node-type-rounding.md) both carry.
