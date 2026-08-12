# D25 — Four item/market resolution gaps: refresh cadence, Bolt Hole's origin, Police Band's target, market-stock duplicates

**Status:** decided
**Blocks:** M1 — Rules core
**Decided:** 2026-08-12
**Issue:** [#131](https://github.com/garnizeh/cinzal/issues/131)

## The question

Four small gaps surfaced while scoping [#66](https://github.com/garnizeh/cinzal/issues/66) ("the eight items"). Each is cheap alone; each is a silent bug if missed, in the RFC §6.6 sense — a wrong market-refresh parity or an unbounded Police Band target reproduces identically on every machine and never trips a determinism check.

1. Which rounds does a Black Market's stock actually refresh on?
2. Bolt Hole's pre-declared destination is "2 steps away" — from where?
3. Can Police Band target any node, or only ones the player already has some fog on?
4. Can a single market's 3 rolled items repeat?

## Why it is open

### 1 · Market refresh cadence

GDD §12: *"Each Black Market shows 3 rolled items, refreshed every 2 rounds."* RFC §6.4's consumption table row for `market.stock` repeats the same phrase: *"Phase 3, every 2 rounds."* Neither states which rounds, and #66's own acceptance criteria assert *"on even rounds only, per #41"* — but #41 is D3, the RNG-consumption-table decision, and neither its issue body nor its decided document ([D03](D03-rng-consumption-table.md)) says anything about round parity for market stock anywhere. The "even rounds" claim traces to no citable source; it looks like an assumption that made it into #66's text uncredited.

Tracing the actual phase sequence resolves it instead of leaving it a guess. GDD §4's phase diagram lists `Phase 3 · Market refresh (auto)` inside **every** round's phase list — unlike Phase 6 (*"rounds 4-15 only"*) or Phase 7's incident half (*"rounds 3-15"*), Phase 3 carries no round restriction. Setup (GDD §4: *"map generation, starting positions, opening contract offer"*) does not populate market stock, and RFC §6.4's consumption table has no separate `market.stock`-at-Setup row the way `deck.event` and `deck.incident` each get their own *"at Setup only"* row. So there is no mechanism anywhere that gives a Black Market its first stock before round 1's Phase 4 (orders) runs, other than round 1's own Phase 3 — which means round 1 must be a refresh round; a market with no stock yet cannot show 3 rolled items to a player who has sight of it in round 1, and §7.1's fog table promises exactly that (*"In sight: ... plus market stock if it's a Black Market"*).

Once round 1 is a refresh round, "every 2 rounds" fixes the rest: 1, 3, 5, 7, 9, 11, 13, 15 — the odd rounds, eight refreshes across a 15-round match. "Even rounds only" is not merely unsupported, it is inconsistent with the phase diagram: it would leave every Black Market showing no stock at all through the entirety of round 1, GDD §7.1's promise notwithstanding.

### 2 · Bolt Hole's declared distance

GDD §9.4: *"Bolt Hole — a destination node, 2 steps away | Armed. Fires if you lose a confrontation."* And: *"You declare the node when you declare the item, and if it has become unreachable by the time the item fires, the ordinary pushback rule (§15) applies instead."* The item is declared alongside Field 1 (the route) and Field 4 (items) in the same order, before any resolution happens (§9.1a, §9.4) — so at declaration time the player knows their own planned route but not whether, or where along it, a confrontation will occur, since that depends on how other players' simultaneous routes collide with theirs.

A "2 steps away" measured from a not-yet-known confrontation node would need to be recomputed once resolution discovers where the fight happens — but the text gives no such recomputation step, and it explicitly treats the declared node as a single fixed destination that may simply turn out to be *"unreachable by the time the item fires,"* falling back to ordinary pushback rather than being re-derived. A fixed destination declared "up front" only has one node available to measure "2 steps" from at declaration time: the player's own position at the start of the round, which is the one coordinate both the player and the server agree on before any order resolves.

### 3 · Police Band's target restriction

GDD §9.4's Field 4 table lists, two rows apart:

| Item | Target declared with it |
|---|---|
| Police Band | a node |
| Decoy | a Known node |

Decoy's row states its fog restriction explicitly; Police Band's does not. §7.2's sources-of-sight table lists Police Band's range as *"One node of your choice, this round"* — again with no fog qualifier, in a table where every other row's scope is stated in full (*"The node and everything adjacent," "Everything within 2 steps of your position"*). Nothing rules out that this is simply an oversight mirroring Decoy's restriction, so it is worth pinning down rather than assumed silently either way — especially since Decoy's own restriction was significant enough to need [D12](D12-decoy-fog-writer.md).

### 4 · Market stock duplicates

GDD §12: *"Each Black Market shows 3 rolled items."* Nothing states directly whether the 3 can repeat an item. Every other multi-pick RNG consumer decided so far — Torn Map (RFC §6.4's own worked example), Dragnet, `shuffleConstrained` (both via [D03](D03-rng-consumption-table.md)) — draws **distinct** candidates through partial Fisher-Yates over a sorted pool, specifically because two "correct-looking" implementations of a with/without-replacement draw consume different index counts and desynchronise against each other (RFC §6.4, D3's own framing). Market stock has never had its method stated, so it is exactly the gap D3 exists to close and simply missed the pass that produced D3.

## Options

### 1 · Market refresh cadence

- **A — Even rounds (2, 4, 6, …).** *Against:* leaves every market showing no stock at all in round 1, contradicting §7.1's fog table and requiring an unstated separate "initial stock" mechanism nothing in the RFC's consumption table accounts for.
- **B — Odd rounds, starting at round 1 (1, 3, 5, …, 15).** *For:* round 1's own Phase 3 is the only mechanism that can populate stock before round 1's orders; "every 2 rounds" measured from that first roll lands here with no extra mechanism invented. Matches the phase diagram's lack of a round restriction on Phase 3 and requires no new Setup-time RNG row.

### 2 · Bolt Hole's declared distance

- **A — From wherever the confrontation actually occurs.** *Against:* not knowable at declaration time under simultaneous orders (the exact problem the up-front-declaration fix already exists to solve, GDD §9.4's own v2.4 changelog note); would require recomputing or re-validating the declared node against a fight location discovered only during resolution, which the text never describes and which undercuts the entire point of pre-declaring a single fixed node.
- **B — From the player's position at the start of the round (submission time).** *For:* the one node both player and server know before any order resolves; consistent with "you declare the node when you declare the item" and with the unreachable-by-the-time-it-fires/pushback-fallback language, which only makes sense against a destination fixed once, not recomputed per confrontation.

### 3 · Police Band's target

- **A — Restricted to Known nodes, matching Decoy.** *Against:* invents a restriction the text never states, unlike Decoy's row two lines below it in the same table which does state it — the asymmetry in the source text is the one piece of positive evidence available, and this option discards it.
- **B — Any node the player is currently aware of (Rumoured or Known).** *For:* matches the literal, unqualified "a node" / "one node of your choice" wording, and requires no new rule beyond what fog states already mean: a client cannot reference a node it has no awareness of at all (§7.1: Hidden means *"not even that the node exists"*), so "any node" is naturally bounded to Rumoured-or-better without inventing a restriction — Police Band becomes a genuine, if minor, exploration tool (full sight of a node you've only heard about), distinct from Decoy's narrower, narrative-driven restriction.

### 4 · Market stock duplicates

- **A — Independent draws, duplicates allowed.** *Against:* no textual support, and it would need its own, cheaper RNG mechanism (3 independent `rng.Next(_, 8)` calls) that nothing in D3's established pattern uses for any other "pick k of n" consumer; also weakens the market's stated scarcity design (D14 already treats *"3 rolled items"* as load-bearing when ruling out Open Doors reading from the full catalogue).
- **B — Partial Fisher-Yates over the 8-item catalogue, sorted by `ItemID`, no duplicates.** *For:* matches every other multi-pick RNG consumer decided in D3 and RFC §6.4, needs no new draw mechanism (the generic helper already used by Torn Map and Dragnet applies directly), and preserves the scarcity reading D14 already relies on.

## Decision

### 1 · Market refresh cadence — Option B

**Markets refresh on odd rounds: 1, 3, 5, 7, 9, 11, 13, 15.** Round 1's Phase 3 is the first roll — there is no separate Setup-time mechanism, and none is needed — and "every 2 rounds" is counted from there. #66's acceptance criteria should read "odd rounds (1, 3, …, 15), per D25," not "even rounds only, per #41"; the citation to #41 does not hold up and should be replaced with one to this decision rather than dropped.

### 2 · Bolt Hole's declared distance — Option B

**Measured from the player's position at the start of the round** (their position when the order is declared, before that round's route resolves). This is checkable at submission time (`Legal`, GDD §15.0's "checkable in advance" category) against the player's own known position and their currently-known subgraph, exactly the same category D14's hand-limit ruling already established for submission-time-checkable order content. If the declared node is 2 steps away from the start-of-round position but has since become unreachable — a destroyed edge, a Sinkhole, or simply the wrong side of the map from wherever the player actually loses a confrontation — the ordinary pushback rule (§15) applies instead, exactly as GDD §9.4 already states.

**The declared node must be `FogKnown` at declaration time, not merely `FogRumoured`.** This isn't a fifth option to weigh — it falls directly out of §7.1's own definition: *"Rumoured: ... No edges, so you cannot plot a route through or into it."* The "currently-known subgraph" a 2-step distance is measured against is built entirely from known edges between known nodes; a Rumoured node carries no edges into it by construction, so it cannot appear as the endpoint of any 2-hop path over that subgraph in the first place. `Legal` must reject a Bolt Hole declaration whose target is `FogRumoured` (or `FogHidden`) and must compute the distance check only against the player's own known subgraph, never the full server-side graph — the same boundary every other submission-time check in this package already respects.

### 3 · Police Band's target — Option B

**Any node the player is currently aware of: `FogRumoured` or better in their own view, at declaration time.** No new restriction is added; the boundary falls out of what "a node of your choice" can mean at all once Hidden nodes (unknown to exist) are excluded by definition rather than by a stated rule. `Legal` rejects a Police Band declaration naming a node currently `FogHidden` in the declaring player's view, the same submission-time check shape as Bolt Hole's and Decoy's target validation.

### 4 · Market stock duplicates — Option B

**No duplicates.** `RollMarketStock` draws 3 distinct items via partial Fisher-Yates over the 8-item catalogue sorted by `ItemID`, using the RFC §6.4-mandated shape verbatim — the same generic helper Torn Map and Dragnet already use. This needs no new entry in D3's table: `market.stock`'s existing row (*"3 per market refreshed"*) already states the index count; this decision only fixes the method, which D3 left unstated for this one consumer.

## Reasoning

**Three of the four resolve by tracing the actual pipeline or the actual source text more carefully, rather than by picking between genuinely balanced options** — the same shape D14's own Reasoning section names. Market refresh cadence isn't a coin flip between "even" and "odd": the phase diagram and the absence of a Setup-time consumer settle it, and "even rounds" is simply inconsistent with round 1 needing stock before its own orders. Bolt Hole's distance isn't a design choice either: only one reference point is knowable at the moment the text says the node is declared. Market stock's duplicate question isn't new territory: D3 already mandated a method for every comparable consumer, and this is the one row that fell through that pass.

**Police Band is the one genuine judgment call**, resting on textual silence read as a deliberate contrast against Decoy's explicit restriction two rows away in the same table, rather than on a pipeline fact that forces one answer. It is decided here rather than left open because the alternative — silently assuming it mirrors Decoy — would produce a working implementation that quietly under-delivers the item's own printed effect, exactly the RFC §6.6 failure mode this whole decision exists to close off before #66 is written.

## Consequences

- **#66's acceptance criteria need correcting**, not just implementing: *"on even rounds only, per #41"* is wrong and should read *"on odd rounds (1, 3, 5, …, 15), per D25"*, since #41/D3 never made that claim.
- **`internal/rules/market.go`'s `MarketRefreshDue(round game.RoundNumber) bool`** (per #66's plan) is `round%2 == 1`, not `round%2 == 0` — a one-line consequence, but the one this decision exists to pin down before it's guessed wrong in code.
- **`internal/rules/legal.go` gains two target-validation rows**, not zero: Bolt Hole's target must be exactly graph-distance 2 from the declaring player's start-of-round position, computed only on their currently-known subgraph, and must itself be `FogKnown` (a `FogRumoured` target is rejected — Rumoured nodes carry no edges to route into, §7.1); Police Band's target must not be `FogHidden` in the declaring player's view (`FogRumoured` or `FogKnown` both qualify). Both are submission-time, "checkable in advance" rejections (GDD §15.0), matching the shape D14 already established for the hand-limit row.
- **No new RNG accounting.** `market.stock`'s existing D3 row (*"3 per market refreshed"*) is unchanged; this decision fixes its draw method (partial Fisher-Yates, no duplicates) without changing its index count or adding a row.
- **No RFC §9.1 writer-table change.** None of the four rulings here writes a new disclosure into `PlayerView` — Police Band's widened target is still an existing sight mechanism (§7.2, row already present) applied to a broader but still self-consistent set of legal targets, not a new information channel.
- Reversible at low cost while `Legal`, `RollMarketStock` and `MarketRefreshDue` are unwritten (#66 has not landed yet). After: the refresh-cadence and market-duplicate rulings are pure resolution logic, cheap to revisit; the two `Legal` rows are the ones with real reversal cost once matches have been played and validated against them, the same category D14's own hand-limit row carries.
