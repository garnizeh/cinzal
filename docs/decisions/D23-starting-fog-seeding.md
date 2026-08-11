# D23 — What does GDD §6.1 constraint 7's starting-fog seeding actually seed?

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-11
**Issue:** [#115](https://github.com/garnizeh/cinzal/issues/115)

## The question

GDD §6.1 constraint 7 states: *"Every starting node has at least one Warehouse within 2 steps. Those nodes begin Known to that player."* Which nodes, exactly, become Known to a seat at setup, and is that the right fog level for all of them?

## Why it is open

The generator (`rules/gen`, #60) already guarantees the structural property — every starting node has at least one Warehouse within 2 graph-steps — but guaranteeing the property is not the same as seeding the fog state that follows from it. `initial(seed, cfg)`'s `seatPlayers` (`internal/rules/initial.go`) builds every seat's `Fog []game.FogState` and currently leaves it at all-`FogHidden`, with a comment naming this decision as the reason. The single sentence in constraint 7 is genuinely ambiguous about scope:

- **"Those nodes"** could mean just the starting node and one nearest qualifying Warehouse, or every Warehouse within 2 steps — constraint 7 only guarantees "at least one," but a walk of length 2 can pass more than one.
- It is silent on **intermediate nodes** — a Warehouse 2 steps away sits on a path through some other node; does that node become Known too, or does fog jump straight to the endpoints?
- It is silent on whether the starting node itself is Known independent of any Warehouse being nearby.

Read in isolation, §6.1 constraint 7 is the only evidence. It is not read in isolation, though: GDD §8.1 already relies on a fog-at-setup claim while explaining why v1.8's both-endpoints-Known rule was a deadlock, and that explanation says more than constraint 7 does on its own:

> "At setup you Know your starting node and its neighbours, so any two nodes you Know are at most 2 steps apart." (§8.1, the v1.9 deadlock proof)
>
> "A Warehouse at distance 2 sits behind exactly one intermediate node, and that node is adjacent to your start, so it is already Known from opening sight." (§8.1, immediately after citing constraint 7)

Both sentences are stated as established fact, not as something constraint 7 grants — "already Known from **opening sight**," not "already Known **because of constraint 7**." That phrasing points at §7.2's general sight rule ("Your ending position: the node and everything adjacent") applying at round 0, with the player's starting position standing in for "ending position" before any order has been resolved. If that is right, the starting node and its neighbours were never constraint 7's to seed — they follow from a rule that already exists for every other round, and constraint 7's own textual promise is only doing new work for the Warehouse itself, which sits outside ordinary sight range (distance 2, never adjacent).

No existing decision covers this. D10 (map layout) and D12 (Decoy) both deal in fog-adjacent territory but neither answers this question.

## Options

**A — Starting node + the single nearest qualifying Warehouse only.** Minimal reading: exactly what constraint 7 promises exists, seeded and nothing more. Cost: needs a tie-break rule when more than one Warehouse sits at the same minimum distance, and "nearest" must be computed against the same navigable-graph distance the rest of the engine uses (GDD §9.1a item 0).

**B — Starting node + every Warehouse within 2 steps.** Seeds the full set the constraint's search would find, not just one witness. Cost: slightly more generous disclosure than the constraint text strictly requires; still needs a definition of "step" (edge-count shortest path).

**C — Starting node + every Warehouse within 2 steps + every node on some shortest path to each.** Full "walked" area becomes Known, not just endpoints. Cost: most generous reading, hardest to justify from a single sentence in isolation, and the most fog information handed out for free at round 0.

**D — Two separate mechanisms, not one blanket rule: apply the general opening-sight rule (§7.2) to the starting position for round 0, and let constraint 7 seed *only* the Warehouse(s) that ordinary sight does not already reach.** This is what §8.1's own proof text describes. Cost: requires stating explicitly, once, that "opening sight" is the existing §7.2 rule run at round 0 rather than a new rule — which is a documentation cost, not a design one, since the behaviour it describes is not new.

## Decision

**D.** Two mechanisms, composed, with no tie-break needed for either:

1. **Opening sight (§7.2, not this decision's invention).** At round 0, the player's starting position is treated as their "ending position" for that round, exactly as an ordinary round's movement produces one. This grants `FogInSight` to the starting node and to every node adjacent to it (`Edges[start]`) — the same "node and everything adjacent" rule §7.2 states for every other round, just run once before any order exists. This is at most 5 nodes (the start plus up to 4 neighbours, degree-4 being the map's cap), independent of graph size.
2. **Constraint 7's seeding (this decision).** For every node of type Warehouse whose shortest-path distance from the starting node, on the setup graph, is **≤ 2**: if that node's fog state is still `FogHidden` after step 1, set it to `FogKnown`. Nothing is downgraded — a Warehouse that happens to be the starting node or one of its neighbours already has `FogInSight` from step 1, which is a strictly higher state, and stays there.

As an algorithm `initial(seed, cfg)` can implement directly:

```
fog[start] = FogInSight
for n in graph.Nodes[start].Edges:
    fog[n] = FogInSight
for node in graph.Nodes:
    if node.Type == NodeWarehouse and distance(start, node.ID) <= 2 and fog[node.ID] == FogHidden:
        fog[node.ID] = FogKnown
```

`distance` is plain BFS shortest-path on the setup graph (equivalently, on the currently-navigable graph per GDD §9.1a item 0 — the two coincide at setup, since no `Bridge Down` or `Sinkhole` has happened yet). No RNG index is consumed: this is a deterministic function of the already-generated graph, matching the expectation in #41/D3's table that `initial()`'s fog seeding is not a further random draw.

**No tie-break is needed anywhere.** Option A's cost — breaking a tie among multiple nearest Warehouses — disappears because the decision seeds *every* qualifying Warehouse, not the nearest one. A second tie-break Option C's framing implies — which intermediate node "the" shortest path runs through, when a Warehouse at distance 2 is reachable via more than one neighbour — also disappears, because step 1 already puts *every* neighbour of the starting node at `FogInSight`, regardless of which one happens to sit on a particular Warehouse's shortest path.

## Reasoning

**§8.1's own deadlock proof is textual evidence, not just precedent, and it says more than constraint 7 does alone.** "At setup you Know your starting node and its neighbours" and "already Known from opening sight" are both stated as facts the proof leans on, not as things constraint 7 is credited with granting. Reading constraint 7 in isolation — the only text the open question in #115 could point to — makes "those nodes begin Known" carry the neighbour-level disclosure too, which forces Option A, B or C's framing (pick a scope for one blanket rule). Reading it alongside §8.1 shows the neighbour-level disclosure was never constraint 7's to grant: it is what §7.2's *existing* sight rule already produces the moment "ending position" is understood to include round 0's starting position. Constraint 7's sentence is then doing exactly one new thing — reaching a Warehouse that ordinary sight, capped at distance 1, cannot.

**This dissolves the three-way choice the issue poses, rather than picking among A/B/C as written.** None of A, B or C is exactly right in isolation:

- A is wrong to restrict to the *nearest* Warehouse — constraint 7's own text says "at least one," which describes a variable-size qualifying set, not a single witness, and nothing in §8.1's supporting text singles one out either.
- B is right about the Warehouse scope but silent on why no tie-break or path-selection is needed — that gap is exactly what recognising the neighbour-level disclosure as *not* B's problem (it's step 1's, unconditionally) closes.
- C's extra generality — every node on some shortest path — happens to produce the same node set as B once §7.2 is applied at round 0, because at depth ≤ 2 "some node on a shortest path to the Warehouse" is always exactly "a neighbour of start," which step 1 already covers regardless of which Warehouse or which path put it there. But C frames this as a general "the walked area becomes Known" principle, which is the wrong lesson to bake in: §7.2 is deliberately restrictive everywhere else in the game — "passing through is not looking around" exists precisely to stop a player's route from lighting up everything it touches (§7.2's own reasoning: "at an average degree of 3 that is 15 to 20 nodes of sight per round... for free"). A blanket "path nodes become Known" rule would be the one exception to that principle in the entire GDD, justified by nothing but a coincidence at distance 2. Framing it instead as "ordinary sight, applied once at round 0" keeps the exception count at zero.

**The fog level split (`FogInSight` for step 1, `FogKnown` for step 2) is not arbitrary — it is what each mechanism already produces elsewhere.** §7.2's sight sources grant the full `FogInSight` state (GDD §7.1's table: "Everything above, plus this round's trail log..."), which is what "ending position" grants every other round too; there is no reason round 0 should grant a lesser state through the same mechanism, even though round 0's trail log is trivially empty. A Warehouse the player has not visited or ended a route on, by contrast, fits `FogKnown` exactly: "Name, type, sector, edges, post owner and lease expiry. But not what happens there" (§7.1) — which is precisely constraint 7's promise ("genuinely Known, and genuinely reachable," §8.1) and no more.

**Zero new RNG consumption keeps this consistent with D3's finalized table and with `initial()`'s own doc comment**, which already states every Setup draw is accounted for in a fixed order (`gen.layout` → `deck.event` → `deck.incident`) with no gap for a fog-seeding draw. A BFS to depth 2 from a fixed start is a pure function of the graph `gen.Generate` already produced; nothing here reads a clock, does I/O, or calls into `*RNG`.

## Consequences

- **Unblocks `internal/rules/initial.go`'s `seatPlayers`.** Its doc comment currently reads "seeding that promise is left open pending D23 (issue #115)"; the implementer can now write the two-step algorithm above and drop the caveat.
- **Unblocks #63's opening-offer acceptance criteria**, which require a Known Warehouse to exist for every seat at setup on every seed (GDD §8.1, D7's fallback ladder) — this decision is the algorithm that produces that guarantee, not just a restatement that it holds.
- **No RFC §6.4 RNG-consumption-table edit needed.** Fog seeding draws no RNG index, so D3's table is unaffected.
- **Low cost to reverse now**, while `initial()`'s fog seeding is unwritten; expensive once a stored match's replay or a golden-fixture test depends on which fog-seeding rule produced its round-1 state, for the same reason every decision in this log carries that caveat.
