# CINZAL — Implementation Roadmap

**Status:** draft for review · **Revision:** p3 · **Companion docs:** `cinzal-gdd.md` **v2.24**, `cinzal-architecture-rfc.md` **RFC-001 r30**

*This document sequences the work. It does not re-decide anything the GDD or the RFC already decided — where it appears to, that is a spec gap and it is logged in §3 rather than resolved silently.*

---

## 0. How to read this

The RFC already fixes the build order (§21). This document expands it into milestones with **exit criteria that can be demonstrated**, names the **spec decisions that must be made before each one can start**, and lists the work the RFC implies but never enumerates (CI enforcement, i18n, invite links, the Recap cursor, the map layout algorithm).

Three conventions:

- **Exit criteria are demonstrable, not aspirational.** "Resolve is implemented" is not an exit criterion; "a 15-round match replays byte-identically from `{seed, log}` on two machines" is.
- **Blocking decisions are listed before the milestone they block**, in §3. A milestone does not start with open blockers.
- **No time estimates.** Ordering, dependencies and gates only — per the RFC's own posture that the risky unknowns are measurement problems, not scheduling problems.

*Written when there were no commits in this repository. **M0 and M1 are now closed.** M0 put in the package skeleton, the Makefile and all four CI gates, which are required status checks on `main`. M1 put in the whole game — `internal/game` and `internal/rules` are complete, deterministic and headless, with RFC §16.1's suite behind them. **M2 is open**, with D32–D35 as its blocking decisions, all four decided.*

*Section 3 is kept as the record of what was found and when, not rewritten each time a decision closes. The authoritative, current catalogue of every decision and its status is [`docs/decisions/README.md`](../decisions/README.md).*

---

## 1. What "done" means for v1

v1 ships when GDD §17's v1 list is playable end to end, by real players, in both synchronous and asynchronous modes, with telemetry emitting. Restated as a checklist the roadmap is accountable to:

| # | v1 requirement | Lands in |
|---|---|---|
| 1 | Map generation with every §6 constraint | M1 |
| 2 | Private fog, sight, trail | M1 |
| 3 | Simultaneous orders, synchronized movement, Infamy-scaled steps | M1 |
| 4 | Crossing/collision detection, full confrontation, pushback, displacement | M1 |
| 5 | Contracts I–IV, 3-choose-1, Contact Cooldown | M1 |
| 6 | Post leases, renewal, expiry, player-count-scaled cap | M1 |
| 7 | Infamy, four tiers, both gradients | M1 |
| 8 | 8 items | M1 |
| 9 | 24 global events / 12 drawn, 16 incidents / 13 drawn, Headline | M1 |
| 10 | Two-player rule set (§6.3) | M1 |
| 11 | Credit bands and the Ledger | M1 |
| 12 | Scoring and ranking | M1 |
| 13 | **Telemetry per §22** | M2 (computation, 16 of 20 rows) + M4 (persistence); rows 15 and 18 are M5.5 and row 16 is M5, per [D33](../decisions/D33-telemetry-event-stream-coverage.md); row 1's read joins them at M5.5, per [D43](../decisions/D43-row-1-unmeasurable-post-d39.md) |
| 14 | Synchronous mode | M5 |
| 15 | Private tables, invite links, no mandatory signup to join | M5 |
| 16 | **The Board** (§7.5) — log, attribution, heat map, pins | M5 |
| 17 | Asynchronous mode | M6 |
| 18 | **Solo scenario ladder and free play against bots** (§19.1) | M7 |

Anything not on that list is RFC-002 or later, and §8 records what.

---

## 2. Sequencing principles

The RFC's order is kept. The reasoning, stated once so the order is defensible when it is under pressure to change:

1. **The riskiest unknowns are rules questions, and they resolve without a UI.** Is the game deterministic? Is it balanced? M1 and M2 answer both, and neither needs a browser.
2. **The fog boundary is cheapest to enforce before there is anything to leak.** The package split, the CI checks and the negative fog suite go in at M0/M1, not after `render` exists and has grown a shortcut.
3. **`rules` is the deepest dependency in the graph.** Every refactor of it is paid for by everything downstream, so anything that would later force a change to it — `Config` suppression flags for solo scenarios, parameterised map generation — is pulled forward into M1 even though nothing consumes it until M7.
4. **Persistence before lifecycle, lifecycle before UI.** The tick's correctness properties (idempotence, the row lock, the deadline race) are testable headlessly; testing them through a browser is strictly harder and no more convincing.
5. **M5 is the product, not scaffolding.** Everything before it is a prerequisite that happens to be independently demonstrable.

### The one accepted deviation

**GDD §23 recommends two paper playtest sessions before any code, and this roadmap skips them.** That is a deliberate choice, recorded here rather than left implicit, because it moves risk rather than removing it:

- What paper would have answered — R1 (cancelled routes), R6 (incident load), R7 (step gradient), and the lease rate — is now answered by **M2 alone**.
- **Therefore M2 is not optional and not compressible.** It is the only measurement gate before real players see the game. The temptation to skip from M1 to M3 to reach something visible is exactly what the paper session was insurance against.
- **Mitigation:** a balance-tuning window is scheduled explicitly after M5.5 (closed playtest), and `Config` is serialised per match (RFC §6.2), so retuning never corrupts in-flight matches. The cost of being wrong is a re-tune, not a rewrite — provided M2 actually produced numbers.

---

## 3. Spec decisions that block code

Found while planning against both documents. Each blocks a specific milestone; none should be resolved by whoever hits it first at 2am. Recommendations are given where one option is clearly better; where the tradeoff is real, both sides are stated.

### 3.1 Structural — blocks **M0**

**D1 · Where does `MatchState` live relative to `PlayerView`?**

The RFC contradicts itself. §5 lists `state.go` and `fog.go` as files in a single `internal/rules` package; §3 requires that `render` and `web` "cannot" name `MatchState`, which is only true if they are in different packages. The CI check (§5) cannot be written until this is settled, and retrofitting it later means touching every import in the tree.

| Option | Shape | Tradeoff |
|---|---|---|
| **A — subpackages of `rules`** | `rules/state`, `rules/view`, `rules` (engine) | Closest to the RFC's wording ("imports `rules/fog`, NOT `rules/state`"). Three-way split inside one domain; `render` still transitively sees `rules` if it imports anything else from it. |
| **B — leaf `internal/view` (recommended)** | `internal/view` holds `PlayerView`, `Order`, `Config`, `Event`, IDs, and imports nothing. `internal/rules` imports `view` and owns `MatchState`. `render` and `web` import **`view` only**. | The CI check becomes one line and one rule: *`render` and `web` must never import `internal/rules`.* Forces a clean layering — `web → match → rules` — which the RFC already implies but never states. Cost: `web` cannot call `rules.Legal` directly, so `internal/match` must expose the order-draft operations (which it should anyway, see D2). |

Recommendation: **B**. The package name is negotiable (`view`, `game`, `protocol`); the property that matters is that it is a **leaf with no dependency on the state package**, so the forbidden edge is a single import, not a set of type names.

**D2 · Where does the order draft live between clicks?**

RFC §11 defines `POST /m/{id}/order/node/{node}` to append/remove a node from the route draft, and never says where the draft is stored. This is load-bearing for both fog and correctness.

| Option | Tradeoff |
|---|---|
| **Stateless — draft round-trips in the form (recommended)** | The whole draft is re-posted as hidden inputs on every click and re-validated server-side. No new table, no session storage, no cleanup job, no cross-instance affinity, nothing to expire. Survives a dropped connection by construction. Cost: slightly larger POST bodies, and every draft mutation is a full re-validation — which at 3–4 clicks per round is free. |
| **Server-side draft table / session store** | Smaller payloads; enables "resume your half-built order on another device". Cost: a table, a TTL policy, an instance-affinity question, and a second piece of per-seat state that can disagree with the submitted order. |

Recommendation: **stateless**. It also keeps the `curl`-testability property the RFC values (§11.1).

### 3.2 Rules-engine — blocks **M1**

**D3 · The RNG consumption table is incomplete.** RFC §6.4 states that every consumer must be enumerated because an unaccounted draw is a replay divergence — and then omits at least eight consumers implied by the GDD §14.2/§14.3 card text:

| Card / effect | GDD | Draws implied | In RFC §6.4? |
|---|---|---|---|
| **Dragnet** — two random Borders sealed | §14.2 POLICE | 2 (or 1 combination draw) | **no** |
| **Bridge Down** — one random edge destroyed | §14.2 CITY | 1 | **no** |
| **Festival** — one random node | §14.2 CITY | 1 | **no** |
| **Scaffolding** — one random sector | §14.2 CITY | 1 | **no** |
| **Shipping Boom** — one random Warehouse | §14.2 ECONOMY | 1 | **no** |
| **Fence's Windfall** — one random Black Market | §14.2 ECONOMY | 1 | **no** |
| **Sinkhole** — one random node in the sector | §14.3 | 1 | **no** |
| **Riot** — trail entries "randomized" | §14.3 | unbounded, method undefined | **no** |
| **Rotating borders** (2p) — "the active set rotates" | §6.3 | unspecified: deterministic rotation or seeded draw? | **no** |
| **`shuffleConstrained`** for both decks | RFC §6.4 | "a defined, auditable number" — never defined | partially |

Action: complete the table **before** `Resolve` is written, and mandate the method for every multi-draw case exactly as RFC §6.4 mandated partial Fisher-Yates for Torn Map. `shuffleConstrained` has the same hazard as Torn Map and needs the same treatment: two correct-looking implementations desynchronise against each other.

**D4 · `Riot` has no specification.** "Every trail entry generated in this sector this round is randomized. Names, events, all of it." Undefined: whether entries are permuted among nodes or regenerated; whether a randomized name may name a player who was not in the sector; whether the affected player knows their own entry was corrupted. It also has a **fog dimension** — a randomized trail must not become a channel that discloses a player who was never there, and it must not disclose to the reader that randomization occurred at a node they could not see. Needs a written rule; this is the single most under-specified card in the deck.

**D5 · Phase 8 (Upkeep) is never enumerated.** GDD §4 lists it in the phase diagram and RFC §6.7 ends the pipeline with `upkeep()`, but no section says what it does. Best reading, to be confirmed and written down: decrement lease blocks and expire at zero (emitting the public "corner went quiet" trace), decrement contract deadlines and fire penalties on expiry, advance the Contact Cooldown, clear `Flagged` and `EvasiveStepPenalty` consumed this round, decrement `Sinkhole` duration, and tick `LooseCrateHeldRounds`. Getting this list wrong is the RFC §6.6 failure mode: a rule quietly stops firing and nothing crashes.

**D6 · The contract offer has no tier distribution.** GDD §8.1 says three are drawn and §8.3 gates tiers by Infamy, but nothing says the tier mix of an offer. A Legend eligible for I–IV could plausibly be offered three Tier I contracts. Needs a rule (weighted by tier? guaranteed at least one at the player's highest eligible tier?), and it interacts with the ladder arithmetic in §24.2 that the balance case rests on.

**D7 · Contract generation can produce an empty or short pool mid-match.** The offer requires a **Known** origin Warehouse at a §8.3 tier distance from a valid Border. §6 constraint 7 guarantees this at setup only. A player who explores little can reach a state where fewer than three valid contracts exist. Needs a defined fallback: offer fewer than three, relax the distance band, or relax the Known-origin rule. Silence here is a crash or an infinite retry loop in the generator.

**D8 · Sector size constraint is arithmetically impossible on small maps.** §6.1 constraint 3 requires each of four sectors to hold **4–8 nodes** — a minimum of 16. But the same section's table specifies **15 nodes for two players**, and GDD §19.1 specifies **12- and 16-node** scenario maps. Options: relax the minimum for small maps, reduce the sector count below four on small maps (which changes the Unstable Sector rotation and sector-majority scoring), or raise the two-player node count. Must be decided before the generator is written, because scenario maps (M7) reuse it.

**D9 · Node type shares do not divide.** §6.2 gives 24/24/20/32% and an explicit 6/6/5/8 breakdown at 25 nodes. At 15, 22 and 28 nodes the shares produce non-integers with no stated rounding rule. Needs a deterministic allocation rule (largest-remainder, with a documented tie order) or an explicit per-player-count table like the 25-node one.

**D10 · Map generation produces no layout.** GDD §7.1 says a **Rumoured** node carries "position on the map", and RFC §11.2 renders an SVG from `PlayerView` — so 2D coordinates are part of the projection. Nothing in §6 generates them. The layout must be **derived deterministically from the seed** (or stored with the graph) and **stable across fog states**, so a node's dot does not move when it goes Rumoured → Known. Recommendation: generate coordinates inside `rules/gen` as part of the graph, on a fixed canvas, so the SVG `viewBox` is a constant and never a function of which nodes the viewer can see — a viewBox fitted to visible nodes is a slow leak of map extent.

**D11 · `Config` has no subsystem-suppression flags.** RFC §14.4 requires solo scenarios to disable leases, incidents, items and Infamy tiers "as `Config` flags, not as branches in `rules`". The §6.2 `Config` sketch has none. Add them in M1 even though M7 is the first consumer — retrofitting them means reopening the pure package everything depends on.

**D12 · `Decoy` is unspecified at the fog boundary.** GDD §12: "plant a false 'Cargo left here' trace on any Known node." Undefined: whether the false trace carries a name (the real one is named only at Infamy ≥ 3 — whose Infamy applies?), whether it feeds the victim's Heat Map and Attribution as a genuine observation (it must, or the item does nothing), and whether the planter's own view distinguishes it. Also needs an entry in RFC §9.1's authorised-writer table, since it writes a node/name pair.

**D13 · `Blackout` and `Rain` distort the observation archive.** RFC §9.2 builds the Heat Map rate on a `Sight` denominator where "sight with no traffic" is real evidence. Under **Rain** no tracks are recorded anywhere, and under **Blackout** nobody has sight beyond their own node. If those rounds count toward the denominator, every watched node's rate is silently deflated by an event the player had no part in. Needs a rule: exclude suppressed rounds from the denominator, or surface them in the confidence flag.

**Resolved by [D13](../decisions/D13-observation-denominator.md): excluded only where a real entry was actually erased by Rain or Blackout, not the whole round.** `SeatArchive` gains a parallel `Obscured` set; Vanish, Distracted Guard and Festival suppress the *acting* player's own entry by design and never populate it.

**D14 · Small resolution gaps** to close while writing the pipeline, each cheap alone and each a silent bug if missed: `Torched` reducing a lease to ≤ 0 (expire, and does the public expiry trace fire?); `Muscle` loss in a 3+ melee (every non-winner loses); buying an item at the hand limit of 3; `Open Doors` letting a player "buy one item at half price" without being at a market — from which market's stock?; **`Bounty`** (highest RP) has no tie-break in RFC §6.5's table, unlike `New Boss`.

**~~D15~~ · Two documented cross-reference errors** — *not a decision; reclassified as a task.* RFC §6.5's tie-break table cites a card called **"Blitz"** which does not exist in GDD §14.2 — the described behaviour (highest Infamy, hits every tied player) is **Raid**. And GDD §9.2's action table still says Stake Post is capped at "**5**", which §10.3 replaced with 4/4/4/3.

Both are wrong sentences rather than open questions: there is nothing to weigh and no option to reject, so they produce a pull request against the source documents rather than a document in `docs/decisions/`. Listing them here alongside twelve genuine decisions was a filing mistake, kept visible because the numbering is cited elsewhere. **The twelve decisions that block M1 are D3–D14.**

**D23 · GDD §6.1 constraint 7's starting-fog seeding is unspecified.** Surfaced while scoping #61: the constraint guarantees every starting node has a Warehouse within 2 steps and says "those nodes begin Known," but not which nodes exactly — just the starting node and the nearest Warehouse, every Warehouse within 2 steps, or every node on a path to one. Blocks #63's opening-offer acceptance criteria, which need a Known Warehouse to exist for every seat at setup on every seed. See [issue #115](https://github.com/garnizeh/cinzal/issues/115).

**Resolved by [D23](../decisions/D23-starting-fog-seeding.md): two composed mechanisms, not one blanket rule.** §7.2's ordinary sight rule, applied once at round 0 with the starting position standing in for "ending position," puts the starting node and every neighbour of it at `FogInSight` — this was never constraint 7's to grant, and §8.1's own deadlock proof already says so ("already Known from opening sight"). Constraint 7 then seeds `FogKnown` on every Warehouse within 2 steps still at `FogHidden` after that (all of them, not just the nearest — no tie-break needed, and nothing already at `FogInSight` is downgraded), the one thing ordinary sight's distance-1 range cannot reach on its own.

**D24 · Does D7's opening-offer guarantee actually hold?** Surfaced while implementing #63: `GenerateOffer`, built faithfully to D6+D7+D23, fails its own opening-offer acceptance criterion on the very first seed a 1000-seed sweep tries. D7's reasoning that "reachability is the only thing that can empty a pool" rests on a 3-step Warehouse-to-Border floor GDD §6.1 constraint 6 does not actually guarantee (only non-adjacency, distance ≥2 — D7's own Reasoning section already flagged this as an open loose end) — and separately, D7's "the four bands cover everything ≥3" argument is about the union of all four tiers, which only a Legend is ever eligible to draw from; a Nobody's eligible set is Tier I alone. Confirmed with a concrete counterexample (`players=2 seed=1 seat=1`: the seat's only Known Warehouse has Borders at distances 2, 5, 5 — none in Tier I's `[3,4]` band). See [D24](../decisions/D24-opening-offer-guarantee.md) and [issue #120](https://github.com/garnizeh/cinzal/issues/120).

**Resolved by [D24](../decisions/D24-opening-offer-guarantee.md): the fix belongs in constraint 7, not in the contract rules.** Constraint 7 exists for exactly one reason — *"without this, the opening contract offer cannot be generated at all"* — and guaranteed only the **origin**. A contract is a pair, so it now also requires the nearby Warehouse to be *contractable*: to have a Border 3–4 steps away, Tier I's band, the one tier every seat is eligible for at Infamy 0. Measured over 1000 seeds × 2/3/4/5 players, opening-offer failures go from 7.10%/1.30%/1.15%/0.60% of seats to **0 of 14 000**, with no generator exhaustion — and D24's Decision section carries the proof that those zeroes are structural, not lucky. D6, D7 and D23 stand unamended; no tier, payout, eligibility or fallback rule moves, and no RNG accounting changes. The rejected alternatives all patched a downstream consumer of a guarantee the generator was supposed to provide: widening the offer cascade above the eligible-tier ceiling still leaves 5.35% of two-player seats failing, because it cannot reach a distance-2 pair that sits below *every* band. GDD v2.17 carries the rule edits: §6.1 constraint 7's new requirement, the deletion of constraint 6's "every delivery costs at least 3 steps" (a claim non-adjacency does not enforce — §8.3's bands are the actual floor), §8.1's guarantee paragraph, and a correction to §8.2's wrong "can only happen if... unreachable" sentence. Code lands as [#122](https://github.com/garnizeh/cinzal/issues/122), which must merge before #63.

**D25 · Four item/market resolution gaps.** Surfaced while scoping #66 ("the eight items"): which rounds a Black Market's stock refreshes on (#66's own text claimed "even rounds, per #41" — a citation that does not hold up), what Bolt Hole's pre-declared "2 steps away" is measured from, whether Police Band can target any node or only ones the player already has some fog on, and whether a market's 3 rolled items can repeat. See [D25](../decisions/D25-item-market-resolution-gaps.md) and [issue #131](https://github.com/garnizeh/cinzal/issues/131).

**Resolved by [D25](../decisions/D25-item-market-resolution-gaps.md): three settle by tracing the pipeline and the source text more closely; one is a genuine judgment call.** Markets refresh on **odd** rounds (1, 3, …, 15) — round 1's own Phase 3 is the only mechanism that can populate stock before round 1's orders, and the phase diagram gives Phase 3 no round restriction, so "even rounds" would leave every market empty through all of round 1 against §7.1's own promise. Bolt Hole's 2 steps is measured from the **player's start-of-round position** — the one coordinate both player and server know at declaration time, before any route resolves. Police Band can target **any node the player isn't `FogHidden` to** (Rumoured or better) — GDD §9.4's table states Decoy's Known-node restriction explicitly two rows away but states none for Police Band, and a Hidden node is unselectable by definition regardless. Market stock draws **3 distinct items**, no duplicates, via the same partial-Fisher-Yates method every other multi-pick RNG consumer in D3 already uses. No RNG accounting changes and no RFC §9.1 writer-table row is added; `Legal` gains two submission-time target-validation rows (Bolt Hole's distance, Police Band's fog floor).

**D27 · `Project`'s decided signature has no `Config`, but two of its own output fields' docs say it computes them from one.** Surfaced while scoping #75: RFC §3, RFC §9 and D01 all cite `func Project(s State, seat SeatID) PlayerView` — no `Config` parameter — but `game/view.go`'s field comments on `SelfState.StepAllowance` and `SelfState.RoundsToNextOffer` say `Project` sets them from `rules.Steps(view, cfg)` and `rules.RoundsToNextOffer(s, seat, cfg)`, both of which require a `Config` `Project` has no way to obtain. See [D27](../decisions/D27-project-config-parameter.md) and [issue #143](https://github.com/garnizeh/cinzal/issues/143).

**Resolved by [D27](../decisions/D27-project-config-parameter.md): `Project`'s signature stands unamended.** `Config` feeds formulas, not visibility, so it was never `Project`'s to take — `rules.Legal` and `rules.Steps` already establish the shape, taking a fog-safe view and a `Config` as two separate arguments from two separate callers' scopes. `SelfState.StepAllowance` and `SelfState.RoundsToNextOffer` stay zero out of `Project` by design; `internal/match`, which already must have `Config` in scope to call `Legal` for order validation, fills both in immediately after `Project` returns and before handing the view to `web`. `view.go`'s two field comments are corrected accordingly; D01 and RFC §3/§9 are untouched.

**Five more surfaced the same way and are recorded in full in the decision log rather than restated here**, since by the time they were found this section had stopped being where decisions are read:

| # | Surfaced while | Decided |
|---|---|---|
| [D26](../decisions/D26-sector-incident-fog-writer-rows.md) | `internal/rules/anchors.go` provisionally folded `EventInformantRing` and `EventSpilledLoadCrate` into existing §9.1 rows | Two new rows, 15 and 16, citing GDD §14.3 directly. **RFC §9.1's table is sixteen writers, not fourteen** — the count this roadmap's M1 section had used since p1. |
| [D28](../decisions/D28-dragnet-rotating-borders-fallback.md) | scoping [#76](https://github.com/garnizeh/cinzal/issues/76) — at 2 players, Dragnet plus rotating borders can seal every Border at once | A downstream safety valve reopens the lowest-`NodeID` Border; neither mechanic changes, and the check is structurally inert at 3+ players |
| [D29](../decisions/D29-phase-2-3-fold-attachment.md) | scoping [#152](https://github.com/garnizeh/cinzal/issues/152) — GDD §15's Phases 2 and 3 had no caller and the contract choice had no field to travel through the fold | Both phases draw a round ahead at the tail of `Resolve`; the choice becomes `Order.ContractChoice` |
| [D30](../decisions/D30-contended-action-loss-notification.md) | scoping [#153](https://github.com/garnizeh/cinzal/issues/153) — four action categories failed with no event at all | GDD §15.0's "never silently fail" is not Step-0-scoped; six new private, actor-scoped `EventKind` values |
| [D31](../decisions/D31-node-display-name.md) | scoping [#154](https://github.com/garnizeh/cinzal/issues/154) — `Node.Name` was disclosed from Rumoured onward and never assigned | Deterministic and RNG-free: sector + type + rank among same-type nodes in that sector |

### 3.3 Bots, telemetry and the numbers — blocks **M2**

Four, all decided before M2 opened. Unlike §3.2's, none of these were found by reading the two specs against each other — they were found by asking what `cmd/simulate` would actually print.

**D32 · Do bot draws consume the match RNG stream?** `Decide`'s third parameter was `*rules.RNG` and RFC §6.4's consumption table had no row for it. If bots draw from the match stream, a round's index count becomes a function of how many seats were bot-filled — which changes mid-match the moment Autopilot hands a seat back to a returning player, and is therefore not a function of `{seed, order log}` at all. See [D32](../decisions/D32-bot-rng-stream.md) and [issue #186](https://github.com/garnizeh/cinzal/issues/186).

**Resolved by [D32](../decisions/D32-bot-rng-stream.md): a distinct `BotRNG` type, not a row in the table.** `NewBotRNG(matchSeed, seat, round)` gives each bot seat its own stream per round, deterministic in `(seed, seat)` and independent of every other seat's human/bot history. `BotRNG` has no `Next` and `RNG` has no `NextBot`, so a call site cannot mix the two streams and still compile — §14.5's non-collusion rule and §16.2's per-round index determinism both become compile-time properties. `ConsumptionTable` is unchanged, and bot draws are invisible to `consumed map[Purpose]int` unconditionally, whatever the bot population.

**D33 · Which GDD §22 metrics are actually computable from the `Event` stream?** RFC §17 said the metric set is *"computed from the event stream"* and never checked the claim row by row. See [D33](../decisions/D33-telemetry-event-stream-coverage.md) and [issue #187](https://github.com/garnizeh/cinzal/issues/187).

**Resolved by [D33](../decisions/D33-telemetry-event-stream-coverage.md): the sentence was wrong for eleven of twenty rows.** Five need nothing new; six need a read of the final `MatchState`, two of those against fog-private per-seat archives no `Project()` call ever aggregates; four need small, targeted additions to `internal/rules` (three new events and one new `Stance` field on `Event`), and row 1's denominator needs order-log access regardless, since `haltMovement` clears a route from memory without recording that it was submitted. Rows 13/14 are scoped down rather than answered exactly; **rows 15 and 18 defer to M5.5 and row 16 to M5** — they are not headless facts at all, which is why §1's v1 table now says so.

**D34 · Where does the telemetry computation live, and what is its input type?** D01 named no package for it, and the input signature fixes its position relative to the fog boundary permanently. See [D34](../decisions/D34-telemetry-package-placement.md) and [issue #188](https://github.com/garnizeh/cinzal/issues/188).

**Resolved by [D34](../decisions/D34-telemetry-package-placement.md): `internal/telemetry`, beside `bots` in RFC §5.** `Match(s rules.MatchState, log rules.OrderLog, events []game.Event, cfg game.Config) (MatchSummary, error)`. It imports `internal/rules` deliberately and is never imported by `render`/`web` — `scripts/check-fog-boundary.sh`'s `FORBIDDEN` set gains `internal/telemetry` alongside `internal/rules`, landing in [#197](https://github.com/garnizeh/cinzal/issues/197)'s PR. The new `OrderLog` type goes in `internal/rules`, not `internal/game`, on D01's own reasoning: a full-match order log names every seat's history, not one seat's, and is at least as fog-sensitive as `MatchState`.

**D35 · How many matches per configuration, and what makes a threshold verdict actionable?** The seven exit criteria below are point estimates with actions attached and no stated precision. A sweep returning 12.3 against R9's *"> 12"* is either a rule change or nothing. See [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md) and [issue #189](https://github.com/garnizeh/cinzal/issues/189).

**Resolved by [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md): 10,000 matches, one interval formula, and action only when the interval clears the line.** Not GDD §20's 3,000-match R2 precedent — RFC §16.4's own worked invocation already ran `--matches 10000` and two exit demonstrations were already written against it. The scope is two-part and the halves do not have the same width: **every row `cmd/simulate` actually computes** carries an interval in the CSV — each reduced to one number per match first, making the match the sampling unit unconditionally, then `mean ± 1.96 · s / √n` over that vector — while the **verdict rule below governs only the seven exit-criteria rows**. Rows 15, 16 and 18 have no per-match statistic to reduce and stay absent from the CSV entirely. A straddling baseline check is *watch, not act*: one re-run under a second root seed, pooled into a 20,000-match interval, then parked for M5.5 if it still straddles. No cross-metric correction. Both tiers always reported; each threshold's verdict read against one, per the table in M2's exit criteria below.

### 3.4 Product surface — blocks **M5**/**M6**

**D16 · The Recap has no cursor.** GDD §18 requires "every round since your last visit", and the RFC schema has no per-seat last-seen-round. `sessions.last_seen_at` is per session, not per match. Needs a column (`match_players.last_seen_round`) and a defined update point. See [D16](../decisions/D16-recap-cursor.md) and [issue #302](https://github.com/garnizeh/cinzal/issues/302).

**Resolved by [D16](../decisions/D16-recap-cursor.md): `match_players.last_seen_round INT NOT NULL DEFAULT 0`, advanced inside the order-submission transaction, human submissions only.** `last_seen_round = GREATEST(last_seen_round, round − 1)` runs alongside the `orders` upsert (RFC §8.1) — never on a `GET` of the board or the Recap fragment, which RFC §12.2's magic-link reasoning already rules out as prefetchable. `0` is the seat-creation value for every seat regardless of when in the lobby phase it joins. The column is derived, rebuildable from `orders` exactly like `events`/`match_summary`/`missed_deadlines`, not a new exception to §7.1's fold — and since bot/Autopilot orders never pass through the submit handler (§8.2), an Autopilot seat's cursor cannot move without a genuine human resubmission.

**D17 · Invite links have no storage.** RFC §19 promises "high-entropy, revocable, single-match scope"; §7.2's schema has no table or column for them. Needs a design, including whether revocation is per-link or per-match.

**Resolved by [D17](../decisions/D17-invite-link-storage.md): a separate `invite_links` table, not a `matches` column.** `match_id`, `token_hash BYTEA UNIQUE` (SHA-256 of 32 `crypto/rand` bytes, raw token never stored), `expires_at`, `revoked_at` — the last two flags, never a `DELETE`, matching `auth_codes.consumed_at`/`matches.finished_at`. Revocation kills one link without touching seats already joined through it (`match_players.invite_link_id`, a new nullable attribution FK, keeps its value regardless). A link grants admission, not a seat — two people racing on the same link resolve through the ordinary lobby-capacity contention on `match_players`'s own primary key, not a link-specific arbitration — and it stays reusable until revoked, expired, or the match leaves `lobby`, since `GET /m/{id}/join`'s landing page is exactly as prefetchable as the magic links §12.2 already rejects, and D16 already ruled out consuming single-use state on a `GET` for that reason.

**D18 · Pins and notes have no storage.** GDD §7.5 lists manual annotation as one of the Board's four tools and it is inside the v1 scope list (§17). Nothing in the RFC schema holds it. Either add a table or move it explicitly to v1.1 — the current state is that it is promised and unbuildable.

**Resolved by [D18](../decisions/D18-board-notes-storage.md): a seat-private `board_notes` table, kept in v1.** `board_notes(match_id, seat, slot SMALLINT CHECK (slot BETWEEN 1 AND 20), node_id INT NULL, round, body TEXT CHECK (char_length(body) BETWEEN 1 AND 500), updated_at)`, primary key `(match_id, seat, slot)`. The per-seat count cap (up to 20 notes per seat per match) is the bounded `slot` column itself rather than a trigger or a counted `CHECK`, which Postgres cannot express declaratively across sibling rows; the primary key's own leading columns already make "my notes for this match" a prefix-indexed lookup. The upsert is `ON CONFLICT (match_id, seat, slot) DO UPDATE`, naming `updated_at = now()` explicitly since the column default only fires on `INSERT`. Authoritative state like `invite_links`, not derived from `orders`; excluded from the replay bundle, which is shared with every player in the match; deleted with a real `DELETE`, not a flag, since nothing references a note and no other seat has an attribution interest in one surviving its author's delete. The board-panel fragment becomes the one named exception to "every fragment takes `PlayerView`" — it also takes `[]BoardNote`, fetched directly from `internal/store` since notes have no fog projection to perform.

**D19 · Per-match email preferences have no storage.** RFC §13 specifies four preference levels plus one-click unsubscribe per match. No table.

**D20 · Rate-limit state has no home.** RFC §12.2 specifies per-email and per-IP limits. In-process counters are wrong across two app instances (the deployment target in §18). Options: a Postgres table (simple, consistent, one more write path on the auth hot path — which is low-traffic by nature) or a fixed-window counter in the DB with a cleanup job. Recommendation: Postgres. Redis is explicitly out of the stack (§4) and adding it for rate limiting would be the most expensive line in the deployment topology.

**D21 · i18n is in scope and has no design.** RFC §1's non-goals exclude "i18n beyond the two languages already in play" — i.e. English and Portuguese are **in**. GDD §2.3 names the Portuguese edition. RFC §11.5 makes localisation possible by forbidding prose in `Event`, but nothing specifies the catalogue format, the locale-selection rule, or who owns the ~60 card/item/contract strings. Must be decided before `render` grows a hundred hard-coded English strings. Options: `golang.org/x/text/message` catalogues, or a simple embedded map keyed by `{locale, key}` given the small string count and zero pluralisation complexity in card text. Recommendation: the simple map, upgraded only if the string count grows.

**D22 · Match abandonment is undefined.** `matches.status` includes `abandoned` (RFC §7.2) and nothing says what produces it. Autopilot means a match never stalls, so the plausible trigger is "every seat on autopilot for N rounds" or a host action. Needs a rule, or the status is dead.

---

## 4. Milestones

### M0 · Foundations — **closed**

**Goal:** a repository where the fog boundary and the purity of `rules` are enforced by machinery, before there is any code to violate them.

Blocked by: **D1, D2**.

**Deliverables**
- `go.mod` (Go 1.26.5), module path, toolchain pin; initial commit; `.gitignore`.
- Package skeleton per RFC §5 with the D1 split applied — packages compile empty.
- `Makefile`: `dev` (`go build -tags debug`), `prod`, `test`, `lint`, `generate` (templ + sqlc), `migrate`.
- **CI (the load-bearing part of this milestone):**
  - **Purity check** — `go list -deps ./internal/rules/...` contains none of `time`, `math/rand`, `os`, `net/...`, `database/sql` (RFC §6.1).
  - **Fog boundary check** — `go list -deps` of `internal/render` and `internal/web` does not contain the state-bearing package (RFC §5).
  - **Debug isolation check** — build with and without `-tags debug`, diff the route tables, assert the production binary has no debug routes (RFC §15.1).
  - `go vet`, `golangci-lint`, `go test -race`, and a check that `templ generate` / `sqlc generate` output is committed and current.
- A decision log (`docs/decisions/`) for D1–D22 and RFC §20's Q1–Q6, so answers land somewhere durable instead of in commit messages.

**Exit criteria**
- A pull request that adds `import "time"` to `internal/rules` **fails CI**.
- A pull request that adds an import of the state package to `internal/render` **fails CI**.
- A pull request that adds a debug-only route reachable in the production build **fails CI**.

*These three are the whole point of M0. If they cannot be demonstrated by deliberately breaking them, the milestone is not done.*

---

### M1 · Rules core — **closed**

**Goal:** the entire game, deterministic and headless. No database, no network, no browser.

Blocked by: **D3–D14**. Blocks: everything.

**Deliverables**
- `Config` (§6.2), including `Rounds` **validation** against deck arithmetic (RFC §6.2) and the D11 suppression flags.
- `RNG` (§6.4): HMAC-derived draws, `purpose` strings, single non-branching instance, and the **completed** consumption table from D3, with the lazy-draw rule honoured at all six early-termination points.
- `rules/gen`: graph generation under all seven §6 constraints, plus the D8/D9/D10 resolutions (sector sizing, type-share rounding, deterministic layout).
- `MatchState`, `Player`, `Node`, `Graph`, `Contract` (per-player instances with their own Deadline Pause flag), the four fog states, and the eight cross-round counters from RFC §6.6.
- `Order` + `Legal()` covering every row of GDD §15.0, and the affordance metadata RFC §10.2 requires the server to render.
- `Resolve()` as the fixed pipeline of RFC §6.7 — validate → per-step movement with crossing and collision → actions → deliveries → add-ons → trail → event/incident/pressure/upkeep — with the entry snapshot (§6.6) and both orderings (§6.5) implemented as the only two comparators in the codebase.
- `Project()` and `PlayerView`, including the `SeatArchive` sight/trail/obscured history (§9.2, `Obscured` excluding only rounds a real entry was actually erased by Rain or Blackout per [D13](../decisions/D13-observation-denominator.md)), `NodeStats`, and all **sixteen** authorised position writers (§9.1), including Decoy's self-named row per [D12](../decisions/D12-decoy-fog-writer.md), Dead Runner's/Fence's Windfall's own rows from issue #72, and Informant Ring's/Spilled Load's per [D26](../decisions/D26-sector-incident-fog-writer-rows.md).
- Final scoring (GDD §16) and the two-player rule set (§6.3).
- Full test suite from RFC §16.1's matrix that does not need a database: unit, property, golden replays, **fog negative tests**, cross-round counters, lazy RNG, Torn Map, tie-breaks, entry snapshot, anchor parity, headline coherence, adversarial payloads.

**Exit criteria**
- `resolve(s, o) == resolve(s, o)` byte-identical, and a golden 15-round replay reproduces on a second machine and a second OS.
- The RNG index count for each round matches the §6.4 table prediction **including truncation cases**.
- Fog suite: for a state where seat A cannot see node N, `Project(s, A)` serialised to JSON contains **no occurrence of N's ID anywhere in the bytes**.
- Anchor parity test passes: every GDD §7.3 row with a name attached maps to its correct row of RFC §9.1's sixteen-writer table (or correctly to no row, for Fresh tracks); every writer row without a §7.3 counterpart cites its source section instead (RFC §16.1).
- A full match can be driven to final scoring from a Go test with no I/O of any kind.

---

### M2 · Bots and simulation — **the measurement gate** · **closed**

**Goal:** answer the balance questions the GDD deferred, by measurement, before any of them can be rationalised away.

Blocked by: M1 (closed) and **D32–D35** (§3.3, all decided). **Blocks nothing technically — and that is exactly why it is at risk of being skipped.** See §2.

**Deliverables**
- `internal/bots`: the `Bot` interface — `Decide(v game.PlayerView, cfg game.Config, r *rules.BotRNG) game.Order` per [D32](../decisions/D32-bot-rng-stream.md) and [D01](../decisions/D01-package-layout.md) — the tier registry, the no-memory rule, and the legal-order enumeration built **from the view and the `Config` — and nothing else**. The boundary is `MatchState`, the graph and the seed, not `Config`: `rules.Legal` and `rules.Steps` both take a `Config` alongside the view, so an enumerator denied one cannot evaluate its own candidates, and [D11](../decisions/D11-config-suppression-flags.md)'s suppression flags would make an M7 scenario bot enumerate orders the scenario has switched off. `Config` is per-match data every human player is shown (GDD §19.2's reference panel); it discloses nothing about a rival.
- The three tiers: **Drifter** (uniform random legal order, the statistical baseline), **Runner** (greedy, and the tier a returning player inherits), **Operator** (plans across rounds, from the view and no memory).
- `rules.BotRNG` and `NewBotRNG(matchSeed, seat, round)` — a distinct type from `Resolve`'s `*RNG`, with disjoint methods, so a miswired call site is a compile error ([D32](../decisions/D32-bot-rng-stream.md)).
- **A type-level CI gate**: `internal/bots` may not name `MatchState`, the graph, or the seed. Not an import check — `bots` legitimately imports `rules` for `BotRNG`, so §5's `go list` shape cannot express this one. It **fails closed** like every other gate in this repository: a missing tool, an empty symbol list, an unreadable config or a package set that came back empty is a **failure**, never a pass. A gate that reports green having inspected nothing is exactly how `MatchState` reaches `bots` unnoticed, and it is the failure this milestone's own exit demonstration (#206) breaks on purpose.
- The `internal/rules` additions GDD §22 needs and the `Event` stream does not carry: three new events and one new `Stance` field ([D33](../decisions/D33-telemetry-event-stream-coverage.md)).
- `internal/telemetry.Match(s rules.MatchState, log rules.OrderLog, events []game.Event, cfg game.Config) (MatchSummary, error)` ([D34](../decisions/D34-telemetry-package-placement.md)) — **one computation, three sinks**, later shared with the server's analytics path and the debug panel (RFC §17). `scripts/check-fog-boundary.sh` gains `internal/telemetry` in its `FORBIDDEN` set in the same PR.
- The **16 of 20** per-match §22 rows that are headless facts, plus the per-round and per-action sets. Rows 15 and 18 belong to M5.5 and row 16 to M5; row 13 ships without a precise answer and says so. **Row 1 is computed and reports no value**: [D39](../decisions/D39-r1-confrontation-softening.md)'s own rule change converted away the quantity it counts, so `telemetry` returns an empty `Rate` for it — no mean, no interval, no verdict — and R1's read moves to M5.5 ([D43](../decisions/D43-row-1-unmeasurable-post-d39.md)). Row 1 is still one of §22's twenty rows and is still computed; what M2 emits is sixteen headless statistics and, for row 1, none.
- `cmd/simulate`: the headless match driver, parameter sweeps, and the CSV — with D35's interval per row, computed in the harness across many `MatchSummary` values, never on `MatchSummary` itself. Every row carries its **effective `n` and its excluded-match count** beside the interval, not the interval alone: D35 excludes a match with an empty denominator from that metric's vector, so a configuration that silently lost 9,000 matches must not read as one that measured 10,000. Fewer than two values left is **not a measurement** — the row reports the failure and no verdict, on the same reasoning as the zero-width interval below.
- Golden replays and per-round index accounting for **bot-populated** matches, including the Autopilot handover case, where a seat switches from human to bot mid-match and the match stream's index count must not move.

**Exit criteria — expressed as answers, not as code**

The milestone is done when the following have numbers attached, from sweeps at 2/3/4/5 players — **10,000 matches per configuration, each number reported with its interval, each verdict read against the tier named** ([D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md), §3.3):

| Question | GDD ref | Threshold that forces action | Read against | Result |
|---|---|---|---|---|
| Confrontations per match | R9 / §22 | > 12 → raise node count before touching anything else, and read §22 rows 8 and 19 as the guardrail on how far ([D38](../decisions/D38-board-going-unused-indicator.md)) | **Drifter** — a map-shape question | 4p 11.75/11.81 [11.68, 11.89] — **met**, node count raised 25→28 ([#229](../exit-demos/229-node-count-raise.md)). 5p 19.10/19.12 [19.01, 19.20] — **deferred to M5.5** via [#241](https://github.com/garnizeh/cinzal/issues/241): clears only past 46 nodes ([D37](../decisions/D37-five-player-confrontation-load.md)), declined — by then rows 8 and 19 have already left their own bands |
| Two-player encounter rate under rotating borders | §6.3 | < 4 per match | **Drifter** — whether the mechanic geometrically forces encounters | 4.47–4.51 [4.42, 4.55] — **met** ([#203](../exit-demos/203-confrontation-load.md)) |
| Routes cancelled mid-route | R1 | > 15% → soften the confrontation rule | **Operator** — about a player who is trying | Rule already softened, confirmed working ([D39](../decisions/D39-r1-confrontation-softening.md)); the fix converted away the row's own numerator, so it reports no value — **deferred to M5.5** via [#241](https://github.com/garnizeh/cinzal/issues/241) ([D43](../decisions/D43-row-1-unmeasurable-post-d39.md)) |
| Incidents actually hitting a player | R6 | < 20% or > 70% | **Operator** — the question is whether they land against active avoidance | 25.68–44.55% [25.53, 44.70] at 2p–5p — **no action**, re-measured after a two-card under-count ([D40](../decisions/D40-gas-leak-riot-row-6-and-silent-truncation.md)) |
| Matches reaching Infamy 9 | R7 | < 10% → step gradient too steep | **Operator** — a managed climb only Runner/Operator model | 2p 8.93% [8.53%, 9.33%] — trips; all three GDD-named remedies rejected on measurement, **deferred to M5.5** via [#241](https://github.com/garnizeh/cinzal/issues/241) ([D42](../decisions/D42-r7-two-player-remedies-rejected.md)). 3p–5p 21.83%–44.52% — **met** |
| Endgame camping | R11 | confrontations in the final 3 rounds > 45% | **Operator** — a rational-incentive question | 20.5%–21.3% [20.3%, 21.7%] at 2p–5p — **met**, with a standing weak-evidence caveat carried to M5.5 (Operator cannot model being out of contention) ([#203](../exit-demos/203-confrontation-load.md)) |
| Lease rate — the most sensitive dial in the game | §10.4 | live leases at scoring outside 2–4 per player | **Operator** — Runner never buys, Drifter has no plan | 0.00175 [0.00134, 0.00216] at the shipped default; 0.28387 [0.27853, 0.28922] with the chokepoint gate swept to its most permissive setting — still 7–14× short of the 2–4 band (7.05× the floor, 14.09× the ceiling) — **deferred to M5.5** via [#241](https://github.com/garnizeh/cinzal/issues/241) ([D36](../decisions/D36-lease-rate-chokepoint-gate.md)) |

**A threshold is tripped only when the whole 95% interval sits on the failing side of it.** A point estimate of 12.3 with the interval spanning 12 is *watch, not act*: re-run that configuration under a second root seed, pool both vectors into one 20,000-match interval, and if it still straddles, record "watch, unresolved at n = 20,000" and hand it to M5.5. Both tiers are reported for every row regardless — a metric where Drifter and Operator disagree sharply is a metric that rewards skill, which is its own finding. A zero-width interval is a degenerate sample, not a result.

The lease rate's verdict is a **range, not an accept/reject**: sweep `LeaseCostPerBlock` in dial order and report the breakpoint as the region between the last point whose interval sits fully inside the band and the first whose interval sits fully outside it. More than one crossing is reported as every transition found; a sweep that never leaves the band has not found the shape and must be extended.

Plus: a bot could be written **competently against `PlayerView` alone** (RFC §14.1). If Operator needed information the view does not carry, that is a projection defect and it goes back to M1 as an issue, not around it as a workaround. The type-level gate on `internal/bots` is what keeps that criterion honest — one `MatchState` parameter added on a hard afternoon and Operator gets better, the criterion reads as met, and the projection defect ships to M5.

**Caveat to carry forward:** bot play is not human play. A sweep tells you the *shape* of the parameter space — where a dial flips a strategy from dominant to dead — not the exact value. It narrows the range that M5.5 then confirms.

---

### M3 · Persistence

**Goal:** matches survive a restart and reproduce exactly.

**Deliverables**
- Schema per RFC §7.2 — which now includes D16's `last_seen_round`, D17's `invite_links` table (plus `match_players.invite_link_id`), and D18's `board_notes` table directly — plus the D19 addition (email preferences) and the D20 rate-limit table.
- `sqlc` queries, `goose` migrations embedded and run at startup behind the **advisory lock** (§7.5).
- `fold()` — `state = fold(Resolve, initial(seed, cfg), orderLog)`.
- `cmd/replay`, including `--rebuild` for the derived `events` / `match_summary` / `match_players.last_seen_round` projections.
- Fold duration and fold allocation metrics wired from day one — they are the falsifiability trigger for the no-snapshot decision (§7.3) and are worthless added later.

**Exit criteria**
- A match folded from the log equals the incrementally computed state, asserted over a golden fixture.
- Two app processes booting simultaneously against a fresh database both come up, with migrations applied exactly once.
- `cmd/replay --rebuild` regenerates `events`, `match_summary` and `match_players.last_seen_round` to byte-identical content.
- p99 fold duration and fold allocation share are visible on a dashboard, with the §7.3 thresholds (50 ms, 20% of heap churn) marked.

---

### M4 · Round lifecycle

**Goal:** a full match runs end to end with no browser.

**Deliverables**
- `Tick()` under `SELECT … FOR UPDATE`, idempotent, with both entry points (submit handler, sweeper).
- Deadline authority against the **database clock** inside the submit transaction (§8.1).
- Sweeper on its **own pool** with `lock_timeout`, `statement_timeout` and `idle_in_transaction_session_timeout` set (§8.3), and jittered intervals per instance.
- Bot filling **inside the tick** (§14.2); absence defaults (GDD §18); Autopilot **derived** from `source <> 'human'` (§8.2).
- `Effects` interface — the tick owns side effects; `fold()` never does (§7.4).
- Telemetry rows written on match completion, from the same computation as M2.
- Sampled determinism check after each tick (§15.4), with the mismatch alert wired.

**Exit criteria**
- Two goroutines submitting the last order of a round produce **exactly one** resolution.
- Submissions at `deadline_at ± 1ms` land on the correct side of the boundary against a real Postgres clock.
- **Fold a finished match ten times; `outbox` gains zero rows.** (RFC §16.1 calls this the single most valuable regression test in the suite.)
- Autopilot engages on the correct round and **stays engaged** across five further rounds without flapping.
- A 15-round match completes driven only by the sweeper, with every seat a bot.

---

### M5 · Playable web — **this is what ships**

**Goal:** people can play the game.

Blocked by: **D2, D16–D18, D21**.

**Deliverables**
- `templ` components, one per fragment, each taking `PlayerView` — full page render composes the same components (§11).
- Auth: email OTP, sessions, guest accounts, CSRF, the §12.2 threat controls.
- Lobby, match creation, invite links, seat joining.
- The order form (§11.1) with all five fields, the `round` staleness field (§11.1a), and the §10.2 affordance rules rendered as disabled markup with reasons.
- **Server-rendered SVG map** (§11.2) clicked through HTMX, on the deterministic layout from D10 and a fixed `viewBox`.
- Narrated resolution list (§11.3) — the same projected event stream the `round_resolved` email will use.
- **The Board** (GDD §7.5): the Log, anchored Attribution as a candidate table, the Heat Map as a rate with sample counts and the low-confidence flag, and pins/notes against the `board_notes` table (D18).
- HUD invariants (GDD §19.2): rounds to next offer, current step allowance, rounds remaining per lease.
- Reference panel (contract table, Infamy ladder, confrontation formula, lease rates).
- SSE hub with `LISTEN/NOTIFY` fan-out across instances (§11.4).
- i18n scaffolding per D21, with English and Portuguese catalogues.

**Exit criteria**
- A four-player synchronous match plays start to finish in browsers.
- **The entire game is driveable with `curl`** — no JavaScript beyond the HTMX and SSE library tags, and a failed asset load does not brick a match (§11.2).
- The fog inspector (§15.2) shows no diff between any seat's `PlayerView` and what the fog rules permit, across a full match.
- No `PlayerView` blob appears anywhere in served HTML as data.

---

### M5.5 · Closed playtest and balance window

**Goal:** confirm M2's numbers against humans, and gather the RFC-002 research the RFC asks for (§21: "twenty matches on the lean build").

Not a code milestone. It is listed because skipping it is how the paper-playtest deviation in §2 turns into a real cost.

**Deliverables**
- ~20 tracked matches across 2, 3, 4 and 5 players.
- The §22 metric set compared human-vs-human against M2's bot baselines, with solo/bot rows excluded (RFC §14.4).
- A `Config` retune, if the numbers say so — cheap, because config is per-match.
- Notes on which information players kept hunting for and which they ignored — the input to RFC-002.

**Exit criteria**
- Every §22 metric with a stated target band has a measured human value.
- The GDD §19.2 acceptance test passes: a new player watching over a shoulder can explain what is happening after two minutes.

---

### M6 · Asynchronous mode

**Goal:** the mode that makes the product distinct.

Blocked by: **D19**.

**Deliverables**
- Outbox table, worker goroutine, exponential backoff, dead-letter, `Sender` interface with one provider adapter.
- All six templates (§13), with `round_resolved` generated from the **fog-projected** event stream.
- Dedup as the **partial index** (§13.1), plus the send-time re-check for time-sensitive templates.
- The Recap (GDD §18) on the D16 cursor.
- Per-match email preferences and one-click unsubscribe (D19).
- Deadline notifications and the `deadline_soon` race fix (§13.1).

**Exit criteria**
- `round_resolved` for seat A contains nothing seat A could not see — with a dedicated test, since this is the easiest place in the system to mail someone the whole board.
- A player who submits between the `deadline_soon` check and the send receives **no mail**.
- Two `otp` rows for one email do not collide; two `round_resolved` rows for one seat and round do.
- A 24-hour-deadline match runs to completion with real mail delivery.

---

### M7 · Onboarding

**Goal:** people can learn the game without spending someone else's 35 minutes.

Blocked by: **D8** (scenario maps at 12/16/20 nodes) and **D11** (suppression flags), both resolved back in M1.

**Deliverables**
- The five-scenario ladder (GDD §19.1) as **data rows** — node count, round count, `Config` overrides, bot seats and tiers, suppressed subsystems.
- **Fixed seeds per scenario**, so tips can attach to specific rounds and nodes and the tutorial is testable as a golden replay.
- Free play against bots at any tier, any seat count.
- Contextual tips at the six GDD §19.2 moments, then silence.
- Solo telemetry tagged `opponents=bots` (RFC §14.4) so it cannot contaminate the human balance set.
- Scenario reset as a log truncation and refold — a delete, not a state rollback.

**Exit criteria**
- Each of the five scenarios completes as a golden replay test.
- Solo requires only a guest session — no email.
- No solo row appears in the human-vs-human analytics set.

---

### M8 · Launch hardening

**Goal:** operable by someone who did not write it.

**Deliverables**
- Deployment target chosen and provisioned (Fly.io / Railway / VM + systemd — RFC §18 leaves this open); single binary, `embed.FS`, one database URL.
- Secrets management for `SESSION_KEY`, `MAIL_PROVIDER_KEY`, `DATABASE_URL`.
- **Backups with a tested restore.** The order log is irreplaceable — there is no state table to fall back on (§18). A restore that has not been performed is not a backup.
- Graceful shutdown: drain connections, let in-flight ticks finish, close SSE with a retry hint.
- Observability: `slog` JSON with `match_id`/`round`, the §17 operational metric set, and the **one alert worth waking someone for** — a determinism-check mismatch.
- Load validation sufficient to establish the §7.3 fold baseline under realistic concurrency.
- Legal surface for collecting email: privacy policy, retention, deletion path.
- A runbook: how to replay a disputed match, how to read the outbox, what a determinism mismatch means and what to do about it.

**Exit criteria**
- A restore from backup into a clean database reproduces a finished match's final state exactly.
- Killing an app instance mid-tick leaves no corrupted match — the sweeper picks it up and the round resolves once.
- A deliberately injected determinism mismatch fires the alert.

---

## 5. Cross-cutting workstreams

These are not milestones; they run throughout and each has an owner-of-record from the milestone that introduces it.

| Workstream | Starts | Standing obligation |
|---|---|---|
| **Fog enforcement** | M0 | Every new field on `PlayerView` gets a negative test. Every new position writer gets a row in RFC §9.1's table and a parity test. |
| **Determinism** | M1 | Every new random consumer gets a row in RFC §6.4's table and an index-count assertion, including its truncation cases. The one standing exception is `BotRNG` ([D32](../decisions/D32-bot-rng-stream.md)) — a separate stream the table was never scoped to cover, whose draws must stay invisible to `consumed map[Purpose]int`. |
| **Doc synchronisation** | M1 | GDD §21 and RFC §6.4 must not drift; GDD §7.3 and RFC §9.1 must not drift. Both pairs have parity tests — keep them failing loudly. |
| **Telemetry** | M2 | One computation — `internal/telemetry.Match` ([D34](../decisions/D34-telemetry-package-placement.md)) — three sinks: `cmd/simulate` CSV, the analytics table, the debug panel. Never three implementations. `internal/telemetry` sits behind the fog gate beside `internal/rules`, and never becomes importable by `render`/`web`. |
| **Debug tooling** | M1 | Grows with each milestone behind `//go:build debug`; the fog inspector (§15.2) is the highest-value item and should exist as soon as `Project` does. |
| **Config as data** | M1 | Every number the GDD calls tunable is a `Config` field, never a constant. The lease rate especially (§10.4). |

---

## 6. Risk register

Risks specific to *delivery*. The game-design risks (R1–R12) live in GDD §20 and are answered by M2 and M5.5.

| # | Risk | Consequence | Mitigation |
|---|---|---|---|
| **P1** | **M2 gets skipped or compressed** to reach something visible sooner | The balance questions the paper playtest was going to answer get answered by real players, late, when config is already in production | M2's exit criteria are numbers, not code. It cannot be marked done without them. This roadmap treats it as the only gate before M5.5. |
| **P2** | The D3 RNG gap is found during M4 instead of M1 | Replay divergences surfacing weeks later, intermittently, on one machine — the exact failure mode RFC §6.3 warns about | D3 is a blocker on M1 start, and the index-count assertion runs per round from the first test. |
| **P3** | The D1 package split is deferred | `render` grows an import of the state package and the fog boundary becomes convention instead of compilation | M0's exit criterion is a deliberately-broken PR that fails CI. |
| **P4** | Solo scenarios (M7) force a refactor of `rules` | The deepest package in the graph changes after everything depends on it | D8 and D11 are resolved in M1, not M7 — parameterised generation and suppression flags ship unused. |
| **P5** | i18n retrofit (D21) | Several hundred hard-coded strings across `render` | Decided at M5 start; RFC §11.5 already forbids prose in `Event`, which is the expensive half. |
| **P6** | The Board (GDD §7.5) is underestimated | It is inside v1 scope, is four distinct tools, and is where P3 (the design pillar) actually lives — a thin Board means the deduction game does not land | Scoped explicitly in M5's deliverables; §22's Heat Map and Attribution metrics are the check that it is being used. |
| **P7** | Fold performance forces a snapshot layer under production pressure | A second source of truth bolted to the load-bearing wall of the system | Metrics from M3, thresholds pre-declared (§7.3), and the escape hatch already designed. Request-scoped memoisation is the cheaper first step. |
| **P8** | Order-log loss | Matches cannot be reconstructed — there is no state table | M8: PITR with a **tested** restore, not an assumed one. |

---

## 7. Milestone dependency graph

```text
M0 Foundations
 └─> M1 Rules core ──────────────┬─> M2 Bots + simulation ──> (balance answers)
                                 │                                   │
                                 └─> M3 Persistence                  │
                                        └─> M4 Round lifecycle       │
                                               └─> M5 Playable web <─┘
                                                      └─> M5.5 Closed playtest
                                                             ├─> M6 Async
                                                             └─> M7 Onboarding
                                                                    └─> M8 Launch hardening
```

M2 is drawn off to the side deliberately: nothing downstream *compiles* against it, and it is the only milestone whose output is a set of numbers rather than a binary. That is the shape of a step that gets skipped, and §2 explains why it must not be.

M6 and M7 are independent of each other and can proceed in either order or together.

---

## 8. Explicitly out of scope for this roadmap

Deferred to RFC-002 (RFC §21), listed so nothing here is mistaken for an omission: resolution animation, pan/zoom/hover, layer switching, attribution cones, client-side rules via WASM, touch and small-viewport layouts, and the replay viewer. v1's substitutes for each are in RFC §21's exclusion table.

Deferred to v1.1 (GDD §17): standing orders, better autopilot, shareable replay UI, player statistics. Deferred to v2: asymmetric factions, curated fixed maps, duel mode, faction contracts.

Still open in the RFC (§20) and needing an answer by the milestone shown: **Q1** TinyGo (RFC-002), **Q2** map interaction (after M5.5, deliberately), **Q3** one-click resubmit from email (M6 or v1.1), **Q4** bots in ranked play (post-v1), **Q5** multi-region (never, unless proven otherwise), **Q6** guest session loss disclosure (M5 — the join page must say so before someone invests 35 minutes).
