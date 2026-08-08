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

None of these is a hard problem individually — each is a bounded selection from a candidate set, the same shape RFC §6.4 already solved once for Torn Map. The risk is exactly the one the RFC names: "two correct-looking implementations of the same rule would desynchronise against each other." Leaving the method to whoever implements each card first means ten independent chances to guess differently from a second implementation, in a package where the desync is invisible until a replay disagrees.

Riot is the one genuine exception: its randomization *mechanism* — permute existing entries or regenerate them, and whether it may fabricate a name for a player who was never in the sector — is [D4](https://github.com/garnizeh/cinzal/issues/42)'s question, not this one. This decision cannot fix Riot's index cost without first knowing what it draws for. It fixes everything it can now and reserves the rest.

## Options

Of the ten, one (Riot) is out of scope — reserved for D4, above — and seven have one reasonable method: a bounded selection from an already-well-defined candidate set, using the same partial Fisher-Yates Torn Map already mandates (Dragnet, Bridge Down, Festival, Scaffolding, Shipping Boom, Fence's Windfall, Sinkhole). There is no real fork to record for those; the "decision" is mechanical extension, and recording seven near-identical A-vs-nothing choices would bury the two questions that actually have competing answers — which leaves two.

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
| Dragnet — two Borders sealed | `event.dragnet` | **`min(2, len(candidates))`** | Phase 6. Candidates: all Border nodes, sorted by `NodeID`. Partial Fisher-Yates, same `min(k, n)` shape as Torn Map. Always **2** in practice — see Reasoning on why the pool never drops below it. |
| Bridge Down — one edge destroyed | `event.bridgedown` | **`min(1, len(candidates))`** | Phase 6. Candidates: all edges in the **currently navigable graph** — already-destroyed edges excluded — sorted by `(min(NodeID), max(NodeID))`. Always **1** in practice — see Reasoning. |
| Festival — one node | `event.festival` | **1** | Phase 6. Candidates: all nodes regardless of type, sorted by `NodeID`. |
| Scaffolding — one sector | `event.scaffolding` | **1** | Phase 6. Candidates: the four sectors, sorted by `SectorID`. Safe at every named node count — [D8](https://github.com/garnizeh/cinzal/issues/46) fixed the sector-size floor at 3–8, satisfiable everywhere. |
| Shipping Boom — one Warehouse | `event.shippingboom` | **1** | Phase 6. Candidates: all Warehouse nodes, sorted by `NodeID`. |
| Fence's Windfall — one Black Market | `event.fenceswindfall` | **1** | Phase 6. Candidates: all Black Market nodes, sorted by `NodeID`. |
| Sinkhole — one node in the unstable sector | `incident.sinkhole` | **1** | Phase 7. Candidates: nodes in the sector already flagged by `incident.sector`, sorted by `NodeID`. Safe at every named node count — see Reasoning. |
| Riot — trail entries randomized | `incident.riot` | **exactly `n`, one per eligible entry (`n = 0` in a quiet round)** | Phase 7. [D4](D04-riot-trail-randomization.md): candidates are the round's **sight-gated** trail entries generated in the flagged sector — cargo taken, fresh tracks, confrontation, item purchased; the five **global** types (delivery, post staked, lease expired, Loitering, loose crate) are untouched. Entries sorted by the total key `(origin NodeID, entry-type declaration order, participant SeatID — lower of the two for confrontation, `writeTrail`'s own emission order for fresh tracks, which carries no seat identity)`; partial Fisher-Yates over the resulting `n`-element node list, `k = n` — Torn Map's selection shape, not the deck's `n − 1` shuffle shape, which is exactly why the count lands on `n` and needs no special-casing. |
| Rotating borders (2p only) | — | **0 — no draw** | Deterministic. Sort Border nodes by `NodeID`; assign alternating labels A, B, A, B… down the sorted list. **Active set = A when the round number (GDD §4's 1-indexed `ROUND 1`, `ROUND 2`, …) is odd, B when it is even** — round 1 opens on set A. This specific mapping is arbitrary, but arbitrary is not the same as undecided: two implementations that each pick a self-consistent orientation still disagree with each other, which is the exact failure this whole document exists to close. Computable from the graph alone, so the Headline announcement needs no round-1 lookahead. |
| Event deck — category-constrained selection | `deck.event.select` | **12** (3 per category × 4 categories) | Setup only. Per category: partial Fisher-Yates picking 3 of that category's 6 cards, candidates sorted by a fixed canonical card order (declaration order in the card table, not map iteration). |
| Event deck — final round order | `deck.event.order` | **11** | Setup only. Full Fisher-Yates shuffle of the 12 selected cards (`n − 1` draws for `n = 12`) — required because §14.2's own "4.0 candidates in the final round" property depends on the 12 not being grouped by category. |
| Incident deck — hazard/boon-constrained selection | `deck.incident.select` | **13** (9 of 11 hazards + 4 of 5 boons) | Setup only. Same partial Fisher-Yates, two pools. |
| Incident deck — final round order | `deck.incident.order` | **12** | Setup only. Full Fisher-Yates shuffle of the 13 selected cards (`n − 1` for `n = 13`). |

Event deck total at Setup: **23** draws. Incident deck total at Setup: **25** draws. Both replace the single `deck.event` / `deck.incident` rows RFC §6.4 currently carries.

Riot was the one row this decision could not close on its own — it reserved the purpose string and fixed the unit the count had to be expressed in. [D4](D04-riot-trail-randomization.md) has since supplied the method and the count above.

## Reasoning

**Every selection method is Torn Map's, because Torn Map already settled the hard part.** RFC §6.4 mandates a partial Fisher-Yates over a `NodeID`-sorted candidate slice specifically to avoid "ranging over a map" — §6.3's own named hazard, restated as "wearing a new hat" for exactly this reason. Seven of these ten rows are the identical shape (pick *k* of *n* from a well-defined set) and get the identical fix: sort deterministically, partial Fisher-Yates, `k` draws. Inventing a second correct-looking method anywhere in this list would recreate the desync risk the RFC wrote Torn Map's mandate to prevent.

**Candidate sets are drawn from the full graph, not the acting player's fog.** Every one of these effects is public and global — announced, or visible as a public trace — so there is no fog question hiding in the selection itself. (Whether the resulting effect's *disclosure* respects fog — e.g. whether Festival's "leaves no trace" interacts with who can see the crowd — is a GDD rules question, not an RNG-accounting one, and is out of scope here.)

**On candidate-pool cardinality for Dragnet, Bridge Down, Sinkhole, Shipping Boom and Fence's Windfall.** All five need their pool non-empty (Dragnet needs it ≥2) at the moment they resolve, not just at setup, and the argument starts from one structural fact that's worth stating once rather than five times: **nodes are never removed from the graph.** Nothing in the pipeline (§6.7) or the card list (§14.2, §14.3) deletes a node — Sinkhole makes one temporarily impassable, which is a movement restriction, not a removal, and it still counts as a Warehouse, Border, Black Market or plain node for every other card's purposes. So every node-based pool here is exactly its generation-time count, fixed for the whole match — but "fixed" only proves "non-empty" for the pools whose generation-time count this decision can actually pin down, and that splits the five in two:

**Dragnet (Borders), Shipping Boom (Warehouses) and Fence's Windfall (Black Markets) depend only on total node count, not on sector structure, and are safe today, confirmed by [D9](D09-node-type-rounding.md)'s allocation table.** §6.2's shares applied to **12 nodes — the smallest node count named anywhere in either document**, GDD §19.1's *First Run* scenario — give 3 Warehouses, 3 Borders and 2 Black Markets under D9's largest-remainder rule: comfortably enough for Dragnet's 2 and Shipping Boom's/Fence's Windfall's 1. This holds at every larger node count the GDD or the roadmap names too (D9's table covers all seven), and it holds **regardless of how D8 resolves**, because it never touches the sector question at all.

**Sinkhole (one node in the flagged sector) and Scaffolding (one sector, of "the four") inherit [D8](D08-sector-size-constraint.md)'s own qualification, precisely.** §6.1 constraint 3 — four sectors, internally connected, enforced by rejection sampling ("the generator rejects and retries until all hold") — means any graph the generator ever accepts already has a non-empty, in-range sector count by construction; no separate argument is needed for Sinkhole's or Scaffolding's pool once a match exists at all. The only real question was ever *whether generation can succeed in the first place*, and D8 lowered the per-sector floor from 4 to 3 nodes so the **partition arithmetic** now fits at every node count named anywhere — 12, 15 and 16 included, the three that failed under the old floor. Whether the generator's **edge construction** actually succeeds at the tightest of those (12 nodes, four 3-node sectors, against constraint 4's chokepoint range) is D8's own flagged follow-up, still open — not re-litigated here, but not silently assumed either. Both rows' *method* (partial Fisher-Yates over whatever sector partition generation produced) was never in question regardless of how that follow-up resolves; it's the same method at every node count, before and after D8.

**Edges are the one thing that *can* shrink, and Bridge Down is the one consumer that draws from them** — which is why it needed its own argument rather than inheriting the node one. Two facts bound it: each event card is a single, unique copy in the 24-card pool (§14.2), so **Bridge Down resolves at most once in any match**; and §6.1's edge-count table puts every supported map at 21 or more edges to start. One removal against a floor of 21 leaves the pool nonempty by a wide margin — the relevant guarantee is "fires once, starts high," not "was connected at setup," which (as stated in an earlier draft of this section) doesn't by itself say anything about the pool's size after a removal. Whether the specific edge Bridge Down is allowed to pick should exclude graph-theoretic bridges — so the *remaining* graph stays connected, not just nonempty — is a real question this decision does not answer: it's a **gameplay-legality** question, not an RNG-accounting one, since the draw costs exactly 1 index over however many candidates remain either way. Left for a follow-up task.

**Splitting the deck purpose strings costs a documentation edit and buys back exactly the property `purpose` exists for.** RFC §6.4 is explicit that the string's entire job is telling a divergent replay which draw went wrong. A single `deck.event` string standing in for a 23-draw, two-stage operation fails that job precisely in the case it matters most — setup, before a single round has run, where a wrong category count and a wrong final order look identical from outside.

**Deterministic rotation over a seeded draw, because the mechanic's own stated goal is legibility, not noise.** §6.3 introduces rotating borders to concentrate delivery pressure into a "shrinking target area" that the Headline announces in advance. A schedule a player can compute one round ahead is exactly what that sentence describes; a random draw revealed one round at a time is not a stronger version of the same idea, it is a different mechanic that happens to reuse the phrase "rotates." Zero index cost is a consequence of the right answer, not the reason for it.

## Consequences

- **RFC §6.4's table changes by a net eleven rows.** Nine consumers that were entirely absent get added — Dragnet, Bridge Down, Festival, Scaffolding, Shipping Boom, Fence's Windfall, Sinkhole, Rotating borders and Riot, **all nine now resolved**, the last of them (Riot) by [D4](D04-riot-trail-randomization.md) — and the table's two existing `deck.*` rows are replaced by the four decomposed ones above (a net +2). 9 + 2 = 11. The prose immediately below the table — the Torn Map mandate and the lazy-draw section — needs no change; every row here follows the existing pattern rather than adding a new one.
- **Candidate-pool safety is fully settled for three of the five, and settled-conditional-on-generation for the other two.** Dragnet, Shipping Boom and Fence's Windfall are safe at every node count named anywhere (12 and up) — confirmed, not just argued: [D9](D09-node-type-rounding.md)'s allocation table gives at least 3 Warehouses, 3 Borders and 2 Black Markets at the smallest named map (12 nodes), comfortably above the 2/1/1 this decision needs. Bridge Down is safe on its own, unrelated argument (fires at most once, against a 21+-edge floor). **Sinkhole and Scaffolding, the two rows that depended on [D8](D08-sector-size-constraint.md), inherit exactly D8's own qualification, not full clearance**: D8 lowered the per-sector floor to 3 nodes so that *any graph the generator accepts* has non-empty, in-range sectors at every named node count — but whether the generator can accept a graph at all at 12 nodes is D8's own open follow-up task (validating the chokepoint/degree-budget interaction at a 3-node sector), not resolved by this decision either. No re-check needed once that follow-up lands one way or the other; #56 (RNG) and #59 (`rules/gen`) should track it via D8, not re-derive it.
- **Riot's row is complete.** [D4](D04-riot-trail-randomization.md) resolved it: `internal/rules`'s RNG table and its index-count test can now cover all thirteen rows from the first commit, including Riot's — no `t.Skip` needed in the RFC §16.2 property test.
- **The event/incident deck cost (23 + 25 = 48 draws) happens once, at Setup, before round 1.** It has no interaction with the lazy-draw rule — nothing here is conditional — so it adds no new row to §6.4's early-termination table.
- Reversible at low cost while `rules/gen` and `Resolve` are unwritten; expensive afterward, for the reason every entry in this table is expensive to change late — a fixed golden replay would encode the old index count.
