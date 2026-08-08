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
| D10 | Map generation produces no 2D layout, which the projection and the SVG both need | open |
| D11 | `Config` has no subsystem-suppression flags, which solo scenarios require | open |
| D12 | `Decoy` is unspecified at the fog boundary | open |
| D13 | `Blackout` and `Rain` distort the observation archive's denominator | open |
| D14 | Small resolution gaps: `Torched` at zero lease, `Muscle` in a melee, buying at the hand limit, `Open Doors` stock, `Bounty` tie-break | open |
| ~~D15~~ | Two cross-reference errors in the source documents: the non-existent "Blitz" card, and a stale post cap of 5 | **not a decision** — reclassified as a task, [#40](https://github.com/garnizeh/cinzal/issues/40) |

**On D15.** It arrived in [roadmap §3.2](../project/cinzal-implementation-plan.md) alongside twelve genuine decisions and it is not one of them. RFC §6.5 cites a card called "Blitz" that GDD §14.2 does not contain — the card with that behaviour is **Raid** — and GDD §9.2 still caps Stake Post at 5, which §10.3 replaced with 4/4/4/3. There is no question to answer and no option to weigh: both are wrong sentences, so they produce a pull request against the specs rather than a document here.

The row stays, struck through, because the numbering is cited elsewhere and a silently vanished D15 reads as an oversight. **D3–D14 are the twelve decisions that block M1.**

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
