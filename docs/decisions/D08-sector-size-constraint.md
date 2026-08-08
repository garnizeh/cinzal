# D08 — What fixes the sector-size constraint at 12, 15 and 16 nodes?

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-08
**Issue:** [#46](https://github.com/garnizeh/cinzal/issues/46)

## The question

GDD §6.1 constraint 3 requires four sectors, each holding 4 to 8 nodes — a floor of 16 nodes total. Three node counts the GDD itself names fail to clear that floor with any room to spare: the 15-node two-player map (§6.3) and the 12-node solo scenario map (§19.1's *First Run*) are genuinely **below** it, and the 16-node scenario (*The Trail*) sits exactly **on** it — the unique 4-4-4-4 split, with no alternative. What changes so the generator can be written at all?

## Why it is open

Three node counts, one arithmetic wall:

| Node count | Where named | 4 sectors × 4 minimum | Fits? |
|---|---|---|---|
| 12 | GDD §19.1, *First Run* scenario | 16 | **No — 4 short** |
| 15 | GDD §6.3, two-player map | 16 | **No — 1 short** |
| 16 | GDD §19.1, *The Trail* scenario | 16 | Exactly on the floor, no room to spare |
| 20 | GDD §19.1, *The Ladder* scenario | 16 | Yes, with slack |
| 22 / 25 / 28 | GDD §6.1, 3/4/5-player maps | 16 | Yes, with slack |

Nothing downstream can be written against a constraint that three of seven named node counts fail. `rules/gen` (#59) generates the graph; #60 assigns node types and starting positions on top of it; both are blocked outright. The roadmap names three candidate fixes and defers the choice: relax the per-sector minimum, reduce the sector count on small maps, or raise the node count. It flags the middle option's cost explicitly — it "changes the Unstable Sector rotation and sector-majority scoring" — and leaves the other two unweighed.

## Options

**A — Raise the node count at the three small sizes.**

- For: leaves constraint 3 untouched; nothing else in the GDD changes.
- Against: both small counts were **sized on purpose, with reasons already on record**. §6.3's 15-node two-player map is the result of a specific simulation fix — the pre-fix 19-node map produced 27% of matches with fewer than three encounters, and "simulated together \[tighter map + rotating borders\] put the two-player rate into the 9–12 band, comfortably inside target." Raising the node count works directly against that fix: more nodes on an unchanged player count is a sparser map, which is the exact failure §6.3 was written to close. §19.1's 12-node *First Run* scenario is sized for a different reason — it is deliberately the smallest, simplest map in a five-step teaching ladder ("Route, action, stance, one delivery," with leases, incidents, Infamy tiers and items all withheld). Raising it works against the pedagogical gradient the ladder is built around. Both costs are backed by reasoning already in the GDD; this option pays both simultaneously with no new evidence to justify it.

**B — Reduce the sector count below four on small maps.**

- For: keeps the well-established 4–8-per-sector range and the well-established 3–5-edge chokepoint range (constraint 4) completely unchanged — every sector, at every node count, stays a size the design has already reasoned about.
- Against: it is a strictly bigger change than it first looks. At **3 sectors**, 12 nodes divides as 4-4-4 — landing on the *same* zero-slack minimum boundary this decision exists to get away from, just relocated from 4 sectors to 3. Getting real headroom at 12 nodes requires dropping to **2 sectors** (12/2 = 6, comfortably inside [4, 8]) — and two sectors is worse than the boundary case it replaces: with "the same sector cannot be flagged two rounds running" (§14.3) and only one other sector to choose from, the Unstable Sector becomes **fully predictable from round 2 onward**, which directly undoes the unpredictability §14.2/§14.3 measured and designed around (the whole reason events and incidents are drawn with category/hazard-boon quotas rather than freely). It also changes the sector-majority ceiling at exactly the small-map sizes (9 RP instead of 12 at 3 sectors, worse at 2), a scored-and-balanced number (GDD §16) that would need re-validating, not just re-stated. Three sectors avoids the predictability problem but not the zero-slack one; two sectors avoids the zero-slack problem by creating a worse one.

**C — Relax the per-sector minimum for small maps, keep four sectors always.**

- For: a single-number change to constraint 3 (4 → 3) is arithmetically sufficient at every named node count (see Decision), and it is the **only** option that changes nothing else in either document — not the Unstable Sector's candidate count, not the sector-majority formula, not the two-player encounter-rate fix, not the scenario ladder's teaching progression. Everything downstream that currently assumes "four sectors" keeps assuming it, correctly, everywhere.
- Against: a 3-node sector has less room to satisfy constraint 4's chokepoint range (3–5 edges to *each* adjacent sector) without pushing some node's degree toward constraint 2's cap of 4 — a real interaction this decision can bound but not fully close without generation code to test against (see Reasoning and Consequences).

## Decision

**Option C.** GDD §6.1 constraint 3 becomes:

> 3. Each sector holds **3–8** nodes and is internally connected.

Sector count stays **four, unconditionally, at every node count the game supports.** Every named node count now has a valid partition:

| Node count | 4 sectors of [3, 8], one example split |
|---|---|
| 12 | 3-3-3-3 (the only split at this count — see Reasoning) |
| 15 | 3-4-4-4 |
| 16 | 3-4-4-5 (or 4-4-4-4 — no longer the only option) |
| 20, 22, 25, 28 | unchanged from today; already comfortably inside [4, 8] |

Constraint 4 (chokepoints, 3–5 edges between adjacent sectors) and every other §6.1 constraint are unchanged.

## Reasoning

**The zero-slack case at 12 nodes is not the same problem as "arithmetically impossible," and this decision treats it accordingly.** 3-3-3-3 is the *only* way to split 12 nodes into four parts each ≥3 — any sector above 3 forces another below the floor. That is a fully determined partition, not an absence of one. The distinction matters because §6.1's generator does not need to *guess* a partition and hope it clears the minimum by luck — sizing sectors is a constraint the generator can satisfy directly, before a single edge is drawn; it is *edge* generation (satisfying constraints 1, 2, 4, 5, 6, 7 against a fixed partition) where rejection-and-retry actually does the work, and that already happens at every other node count today without documented difficulty. A unique partition changes nothing about that process except removing one further degree of freedom the generator wasn't relying on regardless.

**What the unique partition does put pressure on is constraint 4, and this decision does not claim that pressure away.** A 3-node sector needs only 2 edges to be internally connected — trivial on its own. The risk is chokepoints: if that sector is adjacent to multiple others (nothing in §6.1 bounds how many — a fully-meshed 4-sector layout is not excluded), each adjacency costs 3 to 5 edges landing on just 3 nodes, and constraint 2 caps every node at degree 4. Two adjacencies at the high end of the chokepoint range (5 edges each) is 10 edge-endpoints across 3 nodes before a single internal edge is drawn — arithmetically tight enough that it deserves a real check, not an assumption. This is not a new risk this decision introduces: the same interaction exists at every node count today, just with more nodes to absorb it (a 6-to-8-node sector has far more degree budget to spread chokepoints across). Lowering the floor to 3 makes the existing interaction sharper at exactly the sizes where it was already going to be sharpest, rather than inventing a new failure mode. Flagged as a concrete follow-up in Consequences rather than asserted safe, for the same reason D3 declined to assert Bridge Down's edge choice safe against connectivity — a document about constraint arithmetic shouldn't quietly absorb a graph-construction question it can't fully answer without the generator's actual output to test.

**Option A costs real, already-paid-for design work for no offsetting benefit.** Both the 15-node two-player map and the 12-node scenario are sized against evidence already in the GDD, not arbitrarily — raising either reopens a question the design already closed, and does so without any new measurement to justify reopening it. That is the same shape of mistake RFC §6.4's own history warns about: changing something that was already correct because it happened to be convenient at the point someone touched it next.

**Option B's apparent simplicity is a trap tied to the same zero-slack boundary this decision is trying to escape.** Choosing 3 sectors doesn't dodge the unique-partition situation (12/3 = 4 is the same shape of boundary as 12/4 = 3 would be, just at a different sector count); the only way to actually escape it is 2 sectors, which is a strictly larger design change — the Unstable Sector loses its unpredictability, and the sector-majority scoring ceiling moves, at precisely the smallest, most tutorial-adjacent maps. Option C changes one number and leaves every other downstream mechanic exactly as designed.

## Consequences

- **GDD §6.1 constraint 3 changes from "4–8" to "3–8."** One number, one section, no other constraint touched.
- **`rules/gen` (#59) and node-type/layout assignment (#60) are unblocked.** Both were waiting on this.
- **[D9](https://github.com/garnizeh/cinzal/issues/47)'s node-type rounding rule can now assume four sectors at every node count** — it no longer needs a separate case for "how many sectors" alongside "how to round type shares," since sector count is fixed.
- **[D3](docs/decisions/D03-rng-consumption-table.md)'s Sinkhole and Scaffolding rows are unblocked, in the same qualified sense as the rest of this decision.** Both were marked "pending D8, safe only at 20/22/25/28 nodes." With sector count fixed at four and the minimum at 3, **any graph the generator actually accepts** has non-empty sectors (≥3 nodes each) at every node count named anywhere — that part follows from the partition arithmetic alone. Whether the generator *can* accept a graph at 12 nodes at all is the same open item as the follow-up task below, not a second one; D3's passages are updated in this same pull request to say exactly that, rather than overstating the resolution or going stale.
- **A follow-up task, not a follow-up decision: validate that `rules/gen`'s generator actually terminates in reasonable time at 12 nodes**, specifically checking whether the chokepoint range (constraint 4) is satisfiable against a 3-node sector without exceeding the degree cap (constraint 2) at whatever sector-adjacency topology the generator produces. If it isn't, the fix is narrower than reopening this decision — cap sector adjacency (e.g., a ring rather than a full mesh) or bias the generator toward assigning small sectors fewer neighbours — not a reason to revisit the 3–8 range itself.
- Reversible at low cost while `rules/gen` is unwritten; expensive after, since a chosen partition algorithm and any fixed test maps would encode the old range.
