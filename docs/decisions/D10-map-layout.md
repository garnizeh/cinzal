# D10 — Where do node 2D coordinates come from, and what guarantees they never move?

**Status:** decided
**Blocks:** M1 — Rules core, M5 — Playable web
**Decided:** 2026-08-08
**Issue:** [#48](https://github.com/garnizeh/cinzal/issues/48)

## The question

GDD §7.1 says a **Rumoured** node carries "name, type, sector, and position on the map" — disclosed without edges, at the second-weakest fog state. RFC §11.2 renders the map as server-side SVG straight from `PlayerView`. So a node's 2D coordinates cross the fog boundary and belong in `game.NodeView` — but **nothing in GDD §6 generates them.** The graph is generated procedurally; the drawing of it is not.

## Why it is open

Two properties are load-bearing and neither is stated anywhere:

**Stability across fog states.** A node's dot must sit in the same place when it is Rumoured as when it becomes Known. Computing layout from the visible subgraph — the obvious approach, and what any force-directed library does — makes every node shift whenever a player discovers something, which breaks the map exactly as the player starts relying on it.

**The `viewBox` must not be a function of what the viewer can see.** A viewBox fitted to visible nodes discloses the map's extent, and it grows as the fog lifts — the same leak RFC §9.1 already forbids for a single field (*"a `NodeView{Type: ""}` is a leak: it tells the client the node exists and, worse, the size of the map"*), arriving here through geometry instead.

Two more constraints follow directly from the rest of the spec once layout is on the table at all: RFC §6.3 forbids `float64` in `rules` — no force-directed or spring-based layout qualifies, since none of them are integer arithmetic — and GDD §9.1's Pushing On sector bias asks a player to steer blind steps "toward" a district, which is only a real choice if a sector renders as one contiguous region on screen rather than nodes scattered across the whole canvas.

## Options

**Where layout is computed**

- **A — Generate coordinates in `rules/gen`, from the seed, on a fixed canvas.** Layout is part of the graph, computed once at Setup, identical for every seat. `Project` copies the coordinates of nodes the seat may see and omits the rest, the same way it already handles every other node field.
- **B — Compute layout in `render` from `PlayerView`.** No `rules` change, but fails both load-bearing properties by construction (recomputing per view, from whatever subset is visible) and puts a layout algorithm on the far side of the fog boundary, where the fog test suite (RFC §16.3) cannot assert anything about it.

**A**, for the reasons already laid out above and because it is the only option that gives every seat byte-identical coordinates without them ever needing to be reconciled.

**How `rules/gen` computes it, given A**

- **C — Fixed lattice, seeded assignment.** Each of the four sectors (fixed count, per [D8](D08-sector-size-constraint.md)) owns a fixed quadrant of the canvas. Inside a quadrant sits a fixed 9-point lattice. Which of a sector's nodes lands on which point is decided by a partial Fisher-Yates shuffle of the lattice — the exact selection shape RFC §6.4 already mandates for Torn Map. Minimum separation is a property of the fixed grid spacing, not a runtime check, so the draw count is exact and bounded before a single index is spent.
- **D — Seeded integer scatter with a minimum-separation rejection loop.** Draw two integers per node, reject and redraw if the point falls too close to one already placed. Integer-only and seeded, so it satisfies determinism — but the draw count is not fixed: a bad seed near a full quadrant can reject repeatedly, and RFC §6.4 already has a name for that failure shape — *"naive rejection sampling redraws on collisions and consumes an unbounded number"* — stated while ruling the identical approach out for Torn Map. Making it safe would mean bolting on the bounded-retry, fail-loudly discipline [D9](D09-node-type-rounding.md) needed for graph placement, for a problem that C doesn't have in the first place.

## Decision

**A, computed by C.**

**Canvas.** Fixed at **1000 × 1000** units, for every match at every player count and every node count (12–28). The SVG `viewBox` is the literal string `"0 0 1000 1000"`, always — never a function of node count, player count, or fog state.

**Quadrants.** The canvas splits into four 500 × 500 quadrants. Sectors are sorted ascending by `SectorID` and assigned to quadrants in fixed reading order — top-left, top-right, bottom-left, bottom-right:

| Sector (by ascending `SectorID`) | Quadrant | X range | Y range |
|---|---|---|---|
| 1st | top-left | [0, 500) | [0, 500) |
| 2nd | top-right | [500, 1000) | [0, 500) |
| 3rd | bottom-left | [0, 500) | [500, 1000) |
| 4th | bottom-right | [500, 1000) | [500, 1000) |

The mapping is fixed, not seeded — the same "declared/ID order, not an extra draw" choice [D3](D03-rng-consumption-table.md) and [D9](D09-node-type-rounding.md) already made for their own tie orders. Randomizing which corner a sector lands in would cost RNG indices for zero informational or gameplay payoff: a player reads sector membership off the node's own `Sector` field regardless of where that sector happens to sit on screen.

**Lattice.** Each quadrant carries a fixed 3×3 grid of 9 candidate points, margin 75 units from the quadrant's own edges, spaced 175 units apart:

```text
local x, y ∈ {75, 250, 425}    (quadrant-local; add the quadrant's origin for canvas coordinates)
```

Row-major candidate order (index = row·3 + col, row 0 = top) is the sorted order the shuffle below runs over.

**Assignment.** For each sector, in ascending `SectorID` order, over that sector's nodes sorted ascending by `NodeID` (the same canonical sort RFC §6.4 already uses for every other candidate set — Dragnet's Borders, Festival's nodes, Torn Map's hidden set):

```go
// n = number of nodes in this sector, 3..8 per D8 — always ≤ 9, the lattice size.
cells := quadrantLattice(sectorID) // fixed 9 points, row-major order
for i := 0; i < n; i++ {
    j := i + rng.Next("gen.layout", 9-i)
    cells[i], cells[j] = cells[j], cells[i]
}
for i, node := range sectorNodesSortedByNodeID {
    node.X, node.Y = cells[i].X, cells[i].Y
}
```

Exactly `n` draws per sector, **always** — D8 caps every sector at 3–8 nodes, and 8 ≤ 9, so the lattice never runs short and no rejection branch exists to bound. Total layout cost for a match is exactly its node count: 12 at the smallest named map, 28 at the largest.

**RNG table entry, for [D3](D03-rng-consumption-table.md)'s table:**

| Consumer | Purpose string | Indices consumed | Notes |
|---|---|---|---|
| Node layout — coordinate assignment | `gen.layout` | **exactly `n` per sector — total node count over the whole map** | `rules/gen`, Setup only. Runs after node-type assignment (D9) and before the event/incident deck shuffles. Partial Fisher-Yates over a fixed 9-cell lattice per sector; always exactly `n` since D8 caps sector size at 8 ≤ 9 — no truncation case exists. |

**Minimum separation, by construction.** Adjacent lattice points inside one sector are 175 units apart. Across a quadrant boundary, the two nearest points in neighbouring sectors are 75 + 75 = 150 units apart. Both numbers fall straight out of the fixed grid — no runtime check, no retry loop, no failure mode to design for.

**Edges carry no geometry beyond their two endpoints.** A rendered edge is a straight line from one endpoint's `(X, Y)` to the other's. No waypoints, no bend points, no curve control points — in v1 or ever, unless a future decision explicitly reopens this one.

**What `game.NodeView` gains.** Two integer fields, canvas-space, `0–1000`: `X, Y int`. They follow the same absence rule RFC §9.1 already states for the rest of `NodeView` — present whenever the node itself is present in `PlayerView.Nodes` (Rumoured and above), simply because a Hidden node is never in that map at all.

## Reasoning

**C's exact, bounded RNG cost is not a nice-to-have — it's the same property the RFC already picked Torn Map's method to get, for the same reason.** RFC §6.4 rejects rejection sampling for Torn Map specifically because its draw count depends on the data instead of being fixed in advance, and mandates partial Fisher-Yates instead. Option D reintroduces exactly the pattern §6.4 already ruled out, just for a different field. C never needed to be *made* safe with a retry cap — it has no failure mode, because the lattice is sized (9) with headroom above the largest possible sector (8, per D8) before a single index is drawn.

**Quadrant-per-sector gets §9.1's "district is a real choice" property for free, where scatter would need a bolted-on constraint to get the same thing.** A random scatter across the whole canvas, even with minimum separation enforced, says nothing about which region a sector occupies — that would need its own constraint layered on top, undoing exactly the simplicity that made scatter look attractive. The quadrant partition makes "this sector is one contiguous area" true by construction, at zero extra cost.

**The sector→quadrant mapping is fixed rather than seeded because there is no question here worth spending an RNG index on.** Every other fixed-order tie-break in this codebase (D3's card declaration order, D9's Warehouse/Border/Black Market/Alley order) exists because *something* has to break the tie and inventing a reason not to would be worse than reusing an order that already exists. The same logic applies here, harder: there is no existing order to reuse, but there's also no cost to *not* randomizing — no player-facing information depends on which corner of the screen a given sector happens to occupy, unlike, say, which lattice point a specific node lands on (which the fog system does treat as meaningful, since it's disclosed at Rumoured).

**This does not claim spatial quadrant-adjacency matches the sector-adjacency graph the chokepoints define, and it does not need to.** [D8](D08-sector-size-constraint.md)'s own reasoning already notes that nothing in §6.1 bounds the sector-adjacency topology — a fully-meshed four-sector layout is legal. §9.1's bias only needs a sector to read as one region; it never promised that adjacent quadrants on screen are graph-adjacent sectors, and this decision doesn't manufacture that promise either.

## Consequences

- **`game.NodeView` gains `X, Y int`.** Present under the same absence rule as every other field on that type — a Hidden node simply never appears in `PlayerView.Nodes`, so there is no leaked-but-zeroed coordinate to worry about, the identical guarantee RFC §9.1 already states for `Type`.
- **`rules/gen` gains a layout pass**, positioned after node-type assignment (D9) and sector partitioning, before `initial(seed, cfg)`'s deck shuffles consume any indices. The generator's own boundary — the point past which nothing about the graph changes for the rest of the match — now includes coordinates, not just topology.
- **RFC §11.2's SVG map gets a literal constant `viewBox`.** `internal/render`'s `Map()` function needs no layout logic at all — it is a straight projection from stored coordinates to SVG markup, exactly as the roadmap's own recommendation anticipated.
- **A follow-up note, not a follow-up decision: node radius, label placement, and any other rendering-scale detail are `internal/render`'s concern, not this decision's.** This decision fixes the coordinate space nodes live in; it says nothing about how large a circle looks at a given zoom level, which is free to change without touching `game.NodeView` or replay data at all.
- **GDD §6 gains a short subsection stating the rule** (layout is deterministic from the seed, generated once, stable for the whole match, and sectors render as four contiguous regions) with the mechanism itself — exact canvas, lattice, and RNG cost — left to this document and the RFC, the same split D4 used between GDD's rule-level passage and its own RNG-method appendix.
- **RFC §6.4's RNG table gains the `gen.layout` row above; §9.1's `PlayerView` sketch gains the two coordinate fields; §11.2 notes the `viewBox` is now a named constant from this decision, not a rendering-time computation.**
- Reversible at low cost while `rules/gen` is unwritten; expensive after, for the same reason every structural decision in this log carries that caveat — a chosen coordinate scheme is baked into any fixed golden replay the moment one exists.
