# D09 — What rounding rule allocates node types when the shares don't divide evenly?

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-08
**Issue:** [#47](https://github.com/garnizeh/cinzal/issues/47)

## The question

GDD §6.2 gives node-type shares (Warehouse 24%, Border 24%, Black Market 20%, Alley 32%) and one worked example — 6/6/5/8 at 25 nodes. Every other node count the game names produces non-integers with no stated rule for who gets the remainder.

## Why it is open

Naive per-type rounding doesn't even conserve the node count:

| Nodes | Warehouse | Border | Black Market | Alley | Naive sum |
|---|---|---|---|---|---|
| 15 | 3.6→4 | 3.6→4 | 3.0→3 | 4.8→5 | **16**, one over |
| 22 | 5.28→5 | 5.28→5 | 4.4→4 | 7.04→7 | **21**, one short |
| 25 | 6 | 6 | 5 | 8 | **25**, exact — the only stated case |
| 28 | 6.72→7 | 6.72→7 | 5.6→6 | 8.96→9 | **29**, one over |

Two things make this worse than a cosmetic gap. First, a "fix up the total afterward" rule that isn't specified is the RFC §6.3 hazard in new clothes: the count comes out stable, but *which type* absorbs the correction differs by implementation unless the tie order is written down. Second, Warehouse and Border aren't decoration — their counts set how many contract origin/destination pairs exist at all, feeding [D7](https://github.com/garnizeh/cinzal/issues/45) directly, and [D3](D03-rng-consumption-table.md) already leans on both counts (and Black Market's) staying comfortably above zero for Dragnet, Shipping Boom and Fence's Windfall to have a candidate to draw from.

## Options

**A — Largest remainder (Hare quota), with a declared tie order.** Take the floor of each type's exact share, then hand the shortfall to whichever types have the largest fractional part, one node each, until the count matches N. Ties in fractional part broken by a fixed order.

- For: one function, deterministic, extends to any node count without a new case — including the three M7 adds (12, 16, 20).
- Against: none of substance. It's the standard fix for exactly this class of problem (apportionment with a fixed number of categories and a fixed total), and there's no monotonicity concern to weigh against it here — each match generates one map at one fixed N, not a sequence of totals that needs to stay consistent with each other the way, say, parliamentary seat apportionment does.

**B — An explicit per-node-count table**, written by hand for each supported size, the way §6.2 already does for 25.

- For: nothing to compute, reviewable at a glance.
- Against: every new supported size — and M7 adds three — needs a new hand-picked row, with no algorithm backing it and no way to catch a transcription error except re-deriving it by hand again.

## Decision

**Option A.** Floor each type's exact share (`⌊shareᵢ × N⌋`); distribute the remaining `N − Σfloors` nodes one at a time to the types with the largest fractional part; break ties by GDD §6.2's own declaration order — **Warehouse, Border, Black Market, Alley** — the same "fixed, stated order rather than iteration order" fix [D3](D03-rng-consumption-table.md) already applied to card selection.

The resulting counts at every node count named anywhere:

| Nodes | Warehouse | Border | Black Market | Alley | Remainder went to |
|---|---|---|---|---|---|
| 12 | **3** | **3** | **2** | **4** | Warehouse, Border, Alley |
| 15 | **4** | **3** | **3** | **5** | Alley, Warehouse |
| 16 | **4** | **4** | **3** | **5** | Warehouse, Border |
| 20 | **5** | **5** | **4** | **6** | Warehouse, Border |
| 22 | **5** | **5** | **5** | **7** | Black Market |
| 25 | 6 | 6 | 5 | 8 | *(exact — no remainder)* |
| 28 | **7** | **7** | **5** | **9** | Alley, Warehouse, Border |

25 matches GDD §6.2's own worked line unchanged, as it must. 15 matches the worked example already sketched in the issue itself.

**No type needs a floor above what this produces.** The worst case for both economically load-bearing types is 12 nodes, which still gives 3 Warehouses and 3 Borders — comfortably above the "zero Borders is not a game" line the issue names, and above the ≥2-Border, ≥1-Warehouse, ≥1-Black-Market floors [D3](D03-rng-consumption-table.md) already needed for Dragnet, Shipping Boom and Fence's Windfall. No node count in this table produces fewer than 2 Black Markets or 3 of either Warehouse or Border.

**§6.1 constraint 6 (no Warehouse adjacent to a Border) is not settled by this decision, on purpose.** This decision fixes *how many* of each type exist; whether the generator can always *place* that many Warehouses and Borders on the graph without one landing next to the other is a placement question for `rules/gen` (#59), the same shape of open item D8 left for the chokepoint/degree-budget interaction. Nothing in this table makes placement obviously infeasible — the worst case (12 nodes, 3 Warehouses, 3 Borders, 6 other nodes to separate them, max degree 4) leaves real room — but "leaves room" is not a proof, and this decision doesn't claim one.

## Reasoning

**Largest remainder over a divisor method, because there is no sequence to keep consistent.** Divisor methods (D'Hondt, Sainte-Laguë) exist mainly to avoid paradoxes that show up when a total changes over time or across related allocations — neither applies here. Each match fixes its node count once, at setup, and generates one map against it; there's no series of node counts that needs to stay mutually consistent the way, say, successive parliamentary seat counts do. Largest remainder is the simpler of two options that produce identical guarantees for this problem shape, and simpler is the right tie-breaker when both are otherwise equal.

**The tie order is Warehouse, Border, Black Market, Alley because that's GDD §6.2's own table order, not because Warehouse "deserves" priority over Border.** The two share an identical 24%, so they tie on fractional remainder often — four of the seven named node counts hit exactly this tie. Rather than invent a reason to prefer one over the other, this decision reuses the order the GDD already declares them in, which is the same move [D3](D03-rng-consumption-table.md) made for card selection ("declaration order in the card table, not map iteration") and for the same reason: a tie order only has to be fixed and stated, not independently justified, and borrowing an order that already exists in the source document is one fewer arbitrary thing to invent.

**Option B's table would need three new rows at M7 with nothing to check them against.** §6.2 already carries the 25-node line as a worked example; keeping a hand-maintained table as the *only* authority means each new node count is a fresh manual computation with no algorithmic cross-check. The algorithm in this decision produces that same table as output rather than input — GDD §6.2 can (and should) still print the table for readability, with a parity test asserting it matches what the function computes, the same relationship [D01](D01-package-layout.md)'s reasoning describes between a readable artifact and its authority.

**Placement feasibility is flagged, not resolved, for the same reason D8 flagged its own chokepoint question.** A rounding rule is arithmetic; whether 3 Warehouses and 3 Borders can always be placed non-adjacently on a 12-node, max-degree-4 graph is a property of the generator's actual construction process. Claiming that safe here, without generator code to test it against, would repeat the exact mistake [D3](D03-rng-consumption-table.md) caught and had to walk back twice on the sector-size question — asserting a graph-construction property true by arithmetic alone.

## Consequences

- **GDD §6.2 gains the allocation rule and the full table** (12 through 28), replacing the single 25-node example with the general algorithm plus a parity-tested table.
- **`rules/gen`'s node-type assignment (#60) is unblocked** — it was waiting on this the same way it was waiting on D8.
- **A follow-up task, alongside D8's: validate that Warehouse/Border placement can always satisfy constraint 6** at the tightest allocation (12 nodes, 3 and 3). If it can't at some node count, the fix is a placement-algorithm concern (bias assignment away from adjacency, or treat it as another rejection-sampled constraint) — not a reason to revisit the counts in this table.
- **[D7](https://github.com/garnizeh/cinzal/issues/45) can now assume a fixed Warehouse/Border count per node count** rather than a range, when it defines the contract-pool fallback.
- Reversible at low cost while `rules/gen` is unwritten; expensive after — the same golden-replay argument D8 and D3 both carry, since node-type counts are baked into every fixed test map.
