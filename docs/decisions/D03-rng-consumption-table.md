# D03 — What completes the RNG consumption table, and what method does each new consumer use?

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-08
**Issue:** [#41](https://github.com/garnizeh/cinzal/issues/41)

## The question

RFC §6.4 states that every RNG consumer must be enumerated, with a mandated method for anything that draws more than once, because an unaccounted draw is a replay divergence that surfaces months later — and cites its own history as proof: r1 accounted for Pushing On and missed pushback entirely. The table as it stands is not that enumeration. Ten entries implied by card text elsewhere in both documents are either absent or only partially specified. Which method does each one use, and what does that make the exact index cost?

## Why it is open

Ten gaps, all traceable to sections that were never cross-checked against §6.4 when they were written:

| Card / effect | Where specified | What it implies | In §6.4? |
|---|---|---|---|
| **Dragnet** — two random Borders sealed | GDD §14.2 POLICE | a selection of 2 nodes | no |
| **Bridge Down** — one random edge destroyed | GDD §14.2 CITY | a selection of 1 edge | no |
| **Festival** — one random node | GDD §14.2 CITY | a selection of 1 node | no |
| **Scaffolding** — one random sector | GDD §14.2 CITY | a selection of 1 sector | no |
| **Shipping Boom** — one random Warehouse | GDD §14.2 ECONOMY | a selection of 1 node | no |
| **Fence's Windfall** — one random Black Market | GDD §14.2 ECONOMY | a selection of 1 node | no |
| **Sinkhole** — one random node in the sector | GDD §14.3 | a selection of 1 node | no |
| **Riot** — trail entries "randomized" | GDD §14.3 | unbounded, method undefined | no |
| **Rotating borders** — "the active set rotates" | GDD §6.3 | unspecified: schedule or draw? | no |
| **`shuffleConstrained`**, both decks | RFC §6.4 itself | "a defined, auditable number" | **stated, never defined** |

None of these is a hard problem individually — each is a bounded selection from a candidate set, the same shape RFC §6.4 already solved once for Torn Map. The risk is exactly the one the RFC names: "two correct-looking implementations of the same rule would desynchronise against each other." Leaving the method to whoever implements each card first means nine or ten independent chances to guess differently from a second implementation, in a package where the desync is invisible until a replay disagrees.

Riot is the one genuine exception: its randomization *mechanism* — permute existing entries or regenerate them, and whether it may fabricate a name for a player who was never in the sector — is [D4](https://github.com/garnizeh/cinzal/issues/42)'s question, not this one. This decision cannot fix Riot's index cost without first knowing what it draws for. It fixes everything it can now and reserves the rest.

## Options

Eight of the ten entries have one reasonable method — a bounded selection from an already-well-defined candidate set, using the same partial Fisher-Yates Torn Map already mandates. There is no real fork to record for those; the "decision" is mechanical extension, and recording eight near-identical A-vs-nothing choices would bury the two questions that actually have competing answers.

**Rotating borders — deterministic schedule, or a seeded draw?**

- **A — Seeded draw each round.** `rng.Next("event.rotatingborders", ...)` picks the active half. For: matches the "random" flavor of everything else in the table. Against: the active set is **announced in the Headline before Phase 4**, so the draw would have to happen at Phase 1, adding a fourteenth-round-boundary special case to a table that otherwise draws only where effects resolve; and a random half-map denies a player the ability to plan more than one round of border strategy ahead, which cuts against §6.3's own stated goal of *concentrating* pressure into a shrinking, legible target area rather than adding noise.
- **B — Deterministic, a pure function of round number.** No draw at all. For: zero index cost, zero new purpose string, trivially reproducible for the Headline announcement (which can compute the schedule from the sorted Border list alone), and consistent with §6.3's framing — the mechanic wants a *knowable, shrinking* target, which is a scheduling property, not a randomness property. Against: none identified; this is strictly simpler with no offsetting cost.

**`shuffleConstrained` — one opaque purpose string, or a decomposed cost model?**

- **A — Keep `deck.event` / `deck.incident` as single purpose strings.** For: no RFC table edit beyond a count. Against: it directly contradicts the reason `purpose` exists at all — §6.4 says a divergent replay should name *which draw* went wrong, and one string standing in for what turns out to be a two-stage, 23-or-25-draw operation tells a debugger nothing about which stage diverged.
- **B — Split each into a `.select` and `.order` purpose.** Category-constrained selection is mechanically different work from the final shuffle into round order, and giving them different strings makes a category-quota bug and an ordering bug distinguishable from the trace alone, at zero runtime cost. Against: touches two purpose strings already written into §6.4's table — a documentation edit, not a behavioural one.

Option B wins both, for the same underlying reason: this decision exists to make the debug trace trustworthy, and picking the option that argues against that purpose because it's less typing would be a strange way to close it.

## Decision

**Rotating borders: Option B (deterministic, zero draws). `shuffleConstrained`: Option B (decomposed purpose strings).** The completed table:

| Consumer | Purpose string | Indices consumed | Notes |
|---|---|---|---|
| Dragnet — two Borders sealed | `event.dragnet` | **2** | Phase 6. Candidates: all Border nodes, sorted by `NodeID`. Partial Fisher-Yates, `n = min(2, len(candidates))`. |
| Bridge Down — one edge destroyed | `event.bridgedown` | **1** | Phase 6. Candidates: all edges in the **currently navigable graph** — already-destroyed edges excluded — sorted by `(min(NodeID), max(NodeID))`. |
| Festival — one node | `event.festival` | **1** | Phase 6. Candidates: all nodes regardless of type, sorted by `NodeID`. |
| Scaffolding — one sector | `event.scaffolding` | **1** | Phase 6. Candidates: the four sectors, sorted by `SectorID`. |
| Shipping Boom — one Warehouse | `event.shippingboom` | **1** | Phase 6. Candidates: all Warehouse nodes, sorted by `NodeID`. |
| Fence's Windfall — one Black Market | `event.fenceswindfall` | **1** | Phase 6. Candidates: all Black Market nodes, sorted by `NodeID`. |
| Sinkhole — one node in the unstable sector | `incident.sinkhole` | **1** | Phase 7. Candidates: nodes in the sector already flagged by `incident.sector`, sorted by `NodeID`. |
| Riot — trail entries randomized | `incident.riot` | **reserved — count fixed by D4** | Phase 7. Whatever mechanism D4 chooses, it must consume **exactly one index per randomized trail entry** under this purpose string — not per node, not per player — to stay auditable in the same unit as every other per-entry consumer. |
| Rotating borders (2p only) | — | **0 — no draw** | Deterministic. Sort Border nodes by `NodeID`; assign alternating labels A, B, A, B… down the sorted list. Active set = A on even rounds, B on odd (or the reverse — either is a fixed, statable rule, the choice between them is arbitrary and does not need re-deciding). Computable from the graph alone, so the Headline announcement needs no round-1 lookahead. |
| Event deck — category-constrained selection | `deck.event.select` | **12** (3 per category × 4 categories) | Setup only. Per category: partial Fisher-Yates picking 3 of that category's 6 cards, candidates sorted by a fixed canonical card order (declaration order in the card table, not map iteration). |
| Event deck — final round order | `deck.event.order` | **11** | Setup only. Full Fisher-Yates shuffle of the 12 selected cards (`n − 1` draws for `n = 12`) — required because §14.2's own "4.0 candidates in the final round" property depends on the 12 not being grouped by category. |
| Incident deck — hazard/boon-constrained selection | `deck.incident.select` | **13** (9 of 11 hazards + 4 of 5 boons) | Setup only. Same partial Fisher-Yates, two pools. |
| Incident deck — final round order | `deck.incident.order` | **12** | Setup only. Full Fisher-Yates shuffle of the 13 selected cards (`n − 1` for `n = 13`). |

Event deck total at Setup: **23** draws. Incident deck total at Setup: **25** draws. Both replace the single `deck.event` / `deck.incident` rows RFC §6.4 currently carries.

Riot is the one row this decision cannot close. It reserves the purpose string and states the unit the count must be expressed in; [D4](https://github.com/garnizeh/cinzal/issues/42) supplies the count.

## Reasoning

**Every selection method is Torn Map's, because Torn Map already settled the hard part.** RFC §6.4 mandates a partial Fisher-Yates over a `NodeID`-sorted candidate slice specifically to avoid "ranging over a map" — §6.3's own named hazard, restated as "wearing a new hat" for exactly this reason. Eight of these ten rows are the identical shape (pick *k* of *n* from a well-defined set) and get the identical fix: sort deterministically, partial Fisher-Yates, `k` draws. Inventing a second correct-looking method anywhere in this list would recreate the desync risk the RFC wrote Torn Map's mandate to prevent.

**Candidate sets are drawn from the full graph, not the acting player's fog.** Every one of these effects is public and global — announced, or visible as a public trace — so there is no fog question hiding in the selection itself. (Whether the resulting effect's *disclosure* respects fog — e.g. whether Festival's "leaves no trace" interacts with who can see the crowd — is a GDD rules question, not an RNG-accounting one, and is out of scope here.)

**On Bridge Down and connectivity, noted but not resolved here.** Nothing in GDD §14.2 constrains which edge Bridge Down may destroy, and generation constraint 1 (§6.1) only guarantees the graph is connected *at setup* — GDD §15.0 already acknowledges the navigable graph can differ from the setup graph. A permanently destroyed edge chosen without regard to bridges (in the graph-theoretic sense) could isolate a node for the rest of the match. That is a real gap, but it is a **gameplay-legality** question — should the candidate pool exclude graph bridges? — not an RNG-accounting one: excluding them or not, the draw still costs exactly 1 index over however many candidates remain, so this decision's table is correct either way. Flagging it for a follow-up task rather than deciding it here, since deciding it would smuggle a rules change into a document about index costs.

**Splitting the deck purpose strings costs a documentation edit and buys back exactly the property `purpose` exists for.** RFC §6.4 is explicit that the string's entire job is telling a divergent replay which draw went wrong. A single `deck.event` string standing in for a 23-draw, two-stage operation fails that job precisely in the case it matters most — setup, before a single round has run, where a wrong category count and a wrong final order look identical from outside.

**Deterministic rotation over a seeded draw, because the mechanic's own stated goal is legibility, not noise.** §6.3 introduces rotating borders to concentrate delivery pressure into a "shrinking target area" that the Headline announces in advance. A schedule a player can compute one round ahead is exactly what that sentence describes; a random draw revealed one round at a time is not a stronger version of the same idea, it is a different mechanic that happens to reuse the phrase "rotates." Zero index cost is a consequence of the right answer, not the reason for it.

## Consequences

- **RFC §6.4's table gains eleven new rows** (ten resolved, one reserved) and its two `deck.*` rows split into four. The prose immediately below the table — the Torn Map mandate and the lazy-draw section — needs no change; every new row follows the existing pattern rather than adding a new one.
- **This decision's Bridge Down and Sinkhole candidate sets assume [D8](https://github.com/garnizeh/cinzal/issues/46) and [D9](https://github.com/garnizeh/cinzal/issues/47) resolve to a map that always has at least one edge and, for Dragnet, at least two Border nodes**, at every supported player count and scenario size. §6.2's 24% Border share is ≥ 2 nodes at every node count the GDD or the roadmap currently names (15 and up), including the 12-node scenario floor D8 is deciding between — but D8/D9's eventual rounding rule should be checked against this floor when they land, rather than assumed safe by omission.
- **Riot's row stays open until [D4](https://github.com/garnizeh/cinzal/issues/42) resolves.** `internal/rules`'s RNG table and its index-count test cannot be complete until then; the property test in RFC §16.2 should carry a `t.Skip` (or equivalent) on Riot specifically, not on the table as a whole, so the other eleven rows are enforced from the first commit rather than waiting on D4.
- **The event/incident deck cost (23 + 25 = 48 draws) happens once, at Setup, before round 1.** It has no interaction with the lazy-draw rule — nothing here is conditional — so it adds no new row to §6.4's early-termination table.
- Reversible at low cost while `rules/gen` and `Resolve` are unwritten; expensive afterward, for the reason every entry in this table is expensive to change late — a fixed golden replay would encode the old index count.
