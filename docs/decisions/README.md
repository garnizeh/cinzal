# Decision log

A decision is a question that must be answered **in writing** before dependent work can start. Decisions produce a document, not code, and they **block the tasks that depend on them** — a milestone does not open with blockers outstanding.

The catalogue below comes from [roadmap §3](../project/cinzal-implementation-plan.md), which found these while planning the GDD and the RFC against each other. Most are not implementation details: they are places where the specifications are silent, ambiguous, or contradict themselves.

## Format

One file per decision, named `Dnn-short-slug.md`:

```markdown
# Dnn — <the question, as a question>

**Status:** open | decided | superseded by Dnn
**Blocks:** M0 | M1 | …
**Decided:** YYYY-MM-DD

## The question
## Why it is open
   What the specs say, and where they disagree. Cite sections.
## Options
   At least two, each with what it costs.
## Decision
## Reasoning
## Consequences
   What changes downstream, and what it would cost to reverse later.
```

Two conventions worth keeping:

**Record the rejected options and why.** The reasoning outlives the verdict — when a decision comes back around, the useful artefact is the argument, not the answer. Both specs are written this way and it is the single most valuable property they have.

**A decision that turns out wrong is superseded, not edited.** Leave the original standing with a pointer forward.

## Catalogue

Status is `open` until a document exists here.

### Blocks M0 — Foundations

| # | Question | Status |
|---|---|---|
| [D1](D01-package-layout.md) | Where does `MatchState` live relative to `PlayerView`? RFC §5 and §3 contradict each other, and the CI check cannot be written until this is settled | **decided** — leaf `internal/game`; `render` and `web` may not directly import `internal/rules` |
| [D2](D02-order-draft-state.md) | Where does the order draft live between clicks? | **decided** — stateless, carried in the form |

### Blocks M1 — Rules core

| # | Question | Status |
|---|---|---|
| [D3](D03-rng-consumption-table.md) | The RNG consumption table is incomplete — at least eight consumers implied by the card text are missing from RFC §6.4 | **decided** — all ten gaps closed with a mandated method each, including Riot's, completed by [D4](D04-riot-trail-randomization.md) |
| [D4](D04-riot-trail-randomization.md) | `Riot` has no specification, and it has a fog dimension | **decided** — permutes the flagged sector's current-round sight-gated trail entries among themselves; global entries untouched, no name is ever invented, no new row lands in RFC §9.1 |
| [D5](D05-upkeep-phase.md) | Phase 8 (Upkeep) is never enumerated in either document | **decided** — five-step dependency-ordered sequence (flags clear, contracts, leases, Sinkhole, next-round modifiers); Contact Cooldown and crate heat turn out to belong elsewhere, not in Upkeep |
| [D6](D06-contract-tier-mix.md) | The contract offer has no tier distribution | **decided** — one slot targets the highest eligible tier (subject to D7's fallback if that pool is empty); the other two are a weighted draw over eligible tiers, default uniform |
| [D7](D07-contract-pool-fallback.md) | Contract generation can produce an empty or short pool mid-match | **decided** — drop to a lower tier, then offer fewer than three; hold the offer (cooldown not restarted) when the whole pool is empty |
| [D8](D08-sector-size-constraint.md) | The sector size constraint is arithmetically impossible at 15, 16 and 12 nodes | **decided** — per-sector minimum lowered from 4 to 3 nodes; sector count stays four everywhere |
| [D9](D09-node-type-rounding.md) | Node type shares do not divide at 15, 22 and 28 nodes, and no rounding rule is stated | **decided** — largest remainder, ties broken by §6.2's declaration order |
| [D10](D10-map-layout.md) | Map generation produces no 2D layout, which the projection and the SVG both need | **decided** — coordinates generated in `rules/gen` on a fixed 1000×1000 canvas; four sectors sit in fixed quadrants, nodes placed by partial Fisher-Yates over a per-quadrant 9-point lattice |
| [D11](D11-config-suppression-flags.md) | `Config` has no subsystem-suppression flags, which solo scenarios require | **decided** — one boolean per subsystem in a nested `SubsystemSuppression` struct (Leases, Incidents, Events, InfamyTiers, Items); no separate Pressure flag, since it's unreachable once InfamyTiers is suppressed |
| [D12](D12-decoy-fog-writer.md) | `Decoy` is unspecified at the fog boundary | **decided** — plants a real "Cargo Taken" entry naming only the planter (self-misdirection, never another player), gated by the planter's own Infamy ≥ 3 at end-of-round resolution — row 1's existing gate, not a new one; row 12 in RFC §9.1's table |
| [D13](D13-observation-denominator.md) | `Blackout` and `Rain` distort the observation archive's denominator | **decided** — excluded only where a real entry was actually erased, not per whole round; `SeatArchive` gains a parallel `Obscured` set; Vanish, Distracted Guard and Festival need no special case, since they suppress the actor's own entry by design, not the world's |
| [D14](D14-five-resolution-gaps.md) | Small resolution gaps: `Torched` at zero lease, `Muscle` in a melee, buying at the hand limit, `Open Doors` stock, `Bounty` tie-break | **decided** — one authoritative Torched expiry via Upkeep's own check (widened to ≤0); melee losers lose Muscle, ties lose nobody's; a new `Legal` row rejects Deal at hand limit unless a same-order discard frees it; `Open Doors` draws from a Black Market already in the player's sight, declared up front; `Bounty` is a single target tied by the fairness key, matching `New Boss` |
| ~~D15~~ | Two cross-reference errors in the source documents: the non-existent "Blitz" card, and a stale post cap of 5 | **not a decision** — reclassified as a task, [#40](https://github.com/garnizeh/cinzal/issues/40) |

**On D15.** It arrived in [roadmap §3.2](../project/cinzal-implementation-plan.md) alongside twelve genuine decisions and it is not one of them. RFC §6.5 cites a card called "Blitz" that GDD §14.2 does not contain — the card with that behaviour is **Raid** — and GDD §9.2 still caps Stake Post at 5, which §10.3 replaced with 4/4/4/3. There is no question to answer and no option to weigh: both are wrong sentences, so they produce a pull request against the specs rather than a document here.

The row stays, struck through, because the numbering is cited elsewhere and a silently vanished D15 reads as an oversight. **D3–D14 are the twelve decisions that block M1.**

| # | Question | Status |
|---|---|---|
| [D23](D23-starting-fog-seeding.md) | GDD §6.1 constraint 7 says starting nodes "begin Known" along with a nearby Warehouse, but no document states which nodes exactly — surfaced while scoping [#61](https://github.com/garnizeh/cinzal/issues/61) | **decided** — two composed mechanisms: §7.2's ordinary sight rule applied at round 0 (starting node + neighbours), plus constraint 7 seeding every still-Hidden Warehouse within 2 steps as Known (never downgrading one already in sight); no tie-break needed either way |
| [D24](D24-opening-offer-guarantee.md) | D7 claims reachability alone guarantees a non-empty opening contract offer, but that assumes a 3-step Warehouse/Border floor GDD §6.1 constraint 6 does not actually provide (only 2, non-adjacency), and a Nobody's eligible tier set (Tier I alone) doesn't span D7's own "bands cover everything ≥3" argument — surfaced while implementing [#63](https://github.com/garnizeh/cinzal/issues/63), confirmed with a concrete counterexample | **decided** — the fix belongs in constraint 7, not the contract rules: a start position now needs a Warehouse within 2 steps that itself has a Border 3–4 steps away (Tier I's band), so the pair the opening offer needs provably exists; 7.10%→0.00% of two-player seats over 1000 seeds, D6/D7/D23 unamended, no RNG change |
| [D25](D25-item-market-resolution-gaps.md) | Four item/market gaps surfaced while scoping [#66](https://github.com/garnizeh/cinzal/issues/66): which rounds a Black Market's stock refreshes on, what Bolt Hole's "2 steps away" is measured from, whether Police Band can target any node, and whether market stock can repeat an item | **decided** — refreshes odd rounds (1,3,…,15), not the "even rounds" #66 assumed; Bolt Hole measures from the player's start-of-round position; Police Band can target any node the player isn't Hidden to (Rumoured or better); market stock draws 3 distinct items via partial Fisher-Yates, no duplicates |

### Blocks M5 — Playable web, and M6 — Async

| # | Question | Status |
|---|---|---|
| D16 | The Recap has no per-seat cursor | open |
| D17 | Invite links have no storage | open |
| D18 | Pins and notes are promised in v1 scope and have no storage | open |
| D19 | Per-match email preferences have no storage | open |
| D20 | Rate-limit state has no home, and in-process counters are wrong across two instances | open |
| D21 | i18n is in scope and has no design | open |
| D22 | Match abandonment is undefined, so `matches.status = 'abandoned'` is unreachable | open |

## Also open, from RFC §20

Carried from the RFC's own list, with the point at which each needs an answer:

| # | Question | Needed by |
|---|---|---|
| Q1 | TinyGo for the WASM binary | RFC-002 |
| Q2 | What the map needs once RFC-002 adds interaction | after M5.5, deliberately |
| Q3 | One-click resubmit from an email link | M6 or v1.1 |
| Q4 | Filler bots in ranked play | post-v1 |
| Q5 | Multi-region | not needed; recorded so nobody adds it speculatively |
| Q6 | Guest session loss disclosure on the join page | M5 |
