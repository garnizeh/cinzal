# CINZAL
## Game Design Document — v2.25 (scope-locked for prototype)

> **Changelog from v0.9**
> - Tolls **removed** (R4). Posts no longer generate income; money comes from contracts only.
> - Post ownership is now a **lease**, not a purchase — you pay for a duration.
> - Match length raised to **15 rounds** (R5). The hard "30-minute" promise is retired.
> - **Contact Cooldown** added: waiting time between contracts, shortened by Infamy (Q1).
> - **Steps per round now scale inversely with Infamy** (Q2).
> - Balances shown in **bands**; exact figures purchasable via the **Ledger** (Q3).
> - **Post cap of 5** (Q4).
> - New system: **Sector Incidents** — localized hazards (floods, snatch jobs, sweeps).
> - New section: **§21 Randomness Inventory** — every die and draw in the game, and when it fires.
> - New section: **§22 Telemetry** — the instrumentation that answers R1 and R2.
>
> **Changelog v1.0 → v1.1** (all five items from the flaw review; see §24 for the reasoning, including the one that was rejected)
> - **Post sight reduced to the node itself.** Adjacency vision is gone. This was the load-bearing fix for map saturation.
> - **Post cap now scales inversely with player count** (5 / 4 / 4 / 3).
> - **Evasive rebalanced**: keeps the cargo, but pays a **Cr$ 4 shakedown**, is pushed back **2 nodes**, and loses **1 step next round**.
> - **The Ledger now reports last round's closing balances** and **cannot be bought in the final round**.
> - **§7.5 The Board** — the deduction UI is now a v1 requirement, not a nice-to-have.
> - The claim that a low-Infamy player could win on volume was tested and is false (§24.2). The Nobody tier is a **starting state, not a strategy**, and §11 has been corrected to say so.
> - Three flaws found while checking that review: edge density figures were wrong, the warehouse cargo limit never fires, and the 2-contract cap only binds at high Infamy. All fixed below.
>
> **Changelog v1.1 → v1.2** — variance, variety, and the encounter-rate correction
> - **R2 was wrong.** Simulation (§20) shows encounters are *plentiful* at 3–5 players and sparse only at 2. The real risk is the opposite one, now logged as **R9**.
> - **Global deck doubled to 24 cards**; **incident deck doubled to 16**. Same rate — one of each per round. Variety, not volume.
> - Every card is now tagged **[D] damage**, **[C] convergence**, or **[O] opportunity**. Convergence cards push players into the same nodes; that is how an event legitimately manufactures interaction.
> - **Five of the sixteen incidents are now boons.** The Unstable Sector flag becomes a gamble instead of a one-way "avoid" signal.
> - New **§14.0 The Boon Rule** — the constraint that keeps beneficial randomness from violating P2.
> - **Scavenging**: exploring a Hidden node now rolls on a small find table.
> - **Two-player rotating borders** (§6.3) — the fix for the one player count where encounters really are too rare.
> - Fixed: the *Dockers' Strike* card still referenced the warehouse supply limit that v1.1 deleted.
>
> **Changelog v1.2 → v1.3** — the deduction horizon
> - **Ghost paths are cut.** Measurement (§7.5) shows a reachability cone covers 94% of the map after one round. The feature answers "they could be anywhere", which is not information. Replaced by **Attribution** (one-round horizon, anchored) and the **Heat Map** (pattern over position).
> - The doc now states plainly what the hidden-information layer actually is: **this round and patterns**, not tracking people across a match.
> - **Seat-order tie-break removed** — replaced by RP, then a per-round seeded roll. The flaw was persistence, not arbitrariness.
> - **Loitering** added: standing still on a non-Warehouse, non-Border node leaks a public trace. Camping is now legal, readable, and counterable.
> - **Cash Drop** and **Insurance Payout** replaced — both handed out free cash and both contradicted the Boon Rule written a page earlier.
>
> **Changelog v1.3 → v1.4** — closing spec gaps
> - **Sight is granted at your ending node only.** Nodes you merely pass through become Known but yield no trail log. This was undefined and load-bearing; the permissive reading would have handed a moving player most of the map every round.
> - **Heat Map data sourcing defined**: private observations, **normalized as a rate**, with sample count shown, plus public anchors overlaid. Raw counts were going to mislead even without a fog leak.
> - **Loitering redefined** to trigger on ending within 1 step of last round's end node, with an exemption for productive actions. The old wording was beatable by pacing.
> - **§11.1 Precedence**: Infamy tier effects evaluate at the moment they fire, not at round start. Resolves the Vanish/Legend conflict and every future case of that shape without a special-case rule.
>
> **Changelog v1.4 → v1.5** — order legality and displacement
> - **Vanish exempts from Loitering, but only when it actually lowers Infamy.** A blanket exemption would have created a permanent invisible camper at Infamy 0.
> - **§15.0 Order legality** — the full list of illegal payloads, what the client must prevent, and what the server must truncate. Previously the degradation rule existed without a definition of what it was degrading.
> - **Pushing On** — blind steps past the frontier are now declarable, turning an engine limitation into a gamble.
> - **Displacement rule**: wherever you actually end the round is your ending node for sight, however you got there. Covers confrontation fallbacks, Snatch Job, and Gas Leak truncation in one line.
>
> **Changelog v1.5 → v1.6** — audit honesty and displacement bounds
> - **§21 was wrong, not Pushing On.** The audit claimed "exactly two dice rolls" while the game had been making seeded random *selections* since v0.9 — contract offers, market stock, event draws. The claim has been corrected and the inventory extended; the design was never the problem.
> - **Pushing On now takes a sector bias.** Blind steps are steered rather than purely random, which is the P2-shaped version of the same gamble.
> - **Pushback fully bounded** (§15). Falls back along the traversed route, stops at the starting node, and has a defined answer for a player who never moved — the case that actually crashes the engine.
> - **Scavenging during Pushing On** resolves strictly sequentially, and an already-Known node rolls nothing.
>
> **Changelog v1.6 → v1.7** — a P4 violation and two undefined resolutions
> - **Deadline Pause.** Losing a confrontation while carrying cargo extends that contract's deadline by 1 round, once per contract. Without it, a single interception mathematically guaranteed mission failure for a Legend on a Tier IV run — a cascade, which is exactly what P4 forbids.
> - **Stationary Evasive pushback defined**: two hops, never re-entering the confrontation node, stopping early if boxed in.
> - **"Toward" defined algorithmically** as a shortest-path priority ladder. It was not a native property of an undirected procedural graph and no backend could have resolved it consistently.
> - **R7 restated with the actual arithmetic** — the Legend tier's margin on Tier IV is one round, and that number should be watched.
>
> **Changelog v1.7 → v1.8** — ownership, navigability, and deck integrity
> - **Contract instances are per-player.** The Deadline Pause flag lives on the contract, never on the cargo. Two players can hold matching contracts and race for the same dropped crate; neither inherits the other's extension.
> - **Two cargo classes defined**: bound contract cargo and loose crates. The distinction was implied by the Dead Runner and Spilled Load cards and never written down.
> - **Pathfinding uses the currently navigable graph**, so a sector bias routes around Sinkholes and destroyed edges instead of steering into them.
> - **Draw constraint added: exactly 3 cards per category** in the match deck. This is what actually protects against deck counting — the resolved-card log stays fully public, because hiding information players witnessed is an anti-feature.
>
> **Changelog v1.8 → v1.9** — a setup deadlock and two unbounded loops
> - **The opening contract offer was impossible to generate.** Not rarely — *always*. Fixed by revealing contract destinations and adding generation constraint 7. This one would have crashed the prototype before round 1.
> - **Loose crates now run hot.** Holding one past a second round fires a public announcement of the node. Camping with a deadline-free crate was a genuine hole in the anti-camping argument.
> - **Pushback no longer cascades**, and a start-of-round collision check closes the "share a node forever" hole that not cascading would otherwise open.
>
> **Changelog v1.9 → v2.0** — double jeopardy, a fourth node state, and insurance you can't afford
> - **The start-of-round collision check is deleted.** It cancelled a fleeing player's whole round for a displacement that happened the round before. Replaced by a clarification that was doing the work all along: the collision check evaluates **all** positions after each step, stationary players included.
> - **Rumoured** added as a fourth node state, for locations you've been told about but can't yet route through. Contract destinations now land here rather than in Known, which is what the v1.9 text actually meant.
> - **The Evasive shakedown is capped at your balance and never triggers Debt** — but if you can't pay it in full, the cargo protection fails. Insurance you can't afford doesn't cover you.
>
> **Changelog v2.0 → v2.1** — three resolution formalities
> - **Taking cargo is optional and capacity-bound.** Refused or impossible transfers drop the crate at the node, still bound to its contract pair. **Abandoning cargo** added as a free order option, which the game needed anyway.
> - **Flagged is a boolean.** Multiple forgiven debts in one round still cost one step.
> - **§9.1a Step allowance formula** — explicit order of operations, floor applied once and last, caps applied after the floor.
>
> **Changelog v2.1 → v2.2** — consistency pass, no rule changes
> A full read-through against every prior revision. Thirteen internal contradictions found and fixed; none of them changed a rule, all of them would have confused an implementer.
> - Duplicated §14.1 heading; "three states" where the table listed four.
> - **Two-player node count contradicted itself** — §6 said 19, §6.3 said 15, §10.3 said 19. Now 15 everywhere, with the post cap corrected to 4.
> - The warehouse-supply-limit note had drifted into the middle of the two-player section.
> - **§16's expected spread contradicted §24.2's ladder table** (4–6 deliveries vs. 2–5). §24.2 is the one with arithmetic behind it, so §16 was rewritten to match. Totals moved from 18–36 to 14–34.
> - "Four or five leases" for the Landlord archetype exceeded the post cap at every player count above two.
> - §17 still listed 12 events, 8 incidents, and a flat cap of 5; §21 still quoted a 3-card category distribution; §22 still opened on R2, which was resolved two revisions ago.
> - Lease arithmetic said Cr$ 13 where the table gives 14.
> - Risk register reordered numerically and de-tagged — nine entries had accumulated in the order they were discovered.
> - **Added an explicit statement that no passive income exists.** It had been true since tolls were removed in v1.1 and was never stated in one place, which is exactly the kind of silence that produces a phantom income phase in the first build.
>
> **Changelog v2.2 → v2.3** — title
> The game is now **Cinzal**, named for its city. *Shadowed Routes* was serviceable and anonymous; the shortlisted alternative, *Shady Routes*, collided head-on with an established English phrase for a walking route with tree cover — AllTrails lists, hiking guides, and a Barcelona wayfinding app all own that search term — and its colloquial register ("a shady character") pulled against the setting's tone.
>
> **Cinzal** is one word, spelled and pronounced the same in English and Portuguese, carries no phrase collision, and was already load-bearing in the design: the top of the Infamy ladder is *Legend of Cinzal*. The Portuguese edition can run as **Cinzal: Rotas Sombrias** without splitting the identity.
>
> **Changelog v2.3 → v2.4** — the deck didn't cover the match
> Surfaced while writing the architecture RFC: the match is **15 rounds** and Phase 6 fires a global event **every round**, but only **12 cards** are drawn. Three rounds had no card and nothing said which three. The count was correct back when the match was 10 rounds and never moved with R5.
> Fixed by giving the opening a **quiet spell**: global events run **rounds 4–15** (12 rounds, 12 cards). Incidents were already correct — 13 cards for rounds 3–15. The staggered ramp is a small gain in its own right, and it restores something the earliest draft had and lost.
>
> **Changelog v2.4 → v2.5** — the order had nowhere to declare an item
> Seven of the eight items in §12 read "Discard: …", and the order in §9 had four fields, none of which was a place to discard one. The items had no declaration point and no resolution timing. Added **§9.4 Field 4 · ITEMS**.
> This also forced a rule fix: **Bolt Hole** said "retreat 2 nodes of your choosing", which is impossible under simultaneous orders — there is no moment to choose. The destination is now pre-declared with the item.
>
> **Changelog v2.5 → v2.6** — solo play made explicit
> §19 had a single four-minute tutorial and nothing after it, which left a real gap: a player's second experience of the game was a 35-minute match against three humans. Expanded into **§19.1 Solo play** — a five-scenario ladder plus free play against bots, with difficulty expressed as **bot tier rather than bot bonuses**.
> No new systems. Solo is an ordinary match with one human, and the bots are the ones the simulation harness already needs.
>
> **Changelog v2.6 → v2.7** — consistency pass, no rule changes
> - **Section and field numbers in §9 were inverted** by the v2.5 edit: §9.4 was labelled "Field 5" and §9.5 "Field 4". Two live cross-references pointed at the wrong section as a result — the Ledger and remote lease renewal both sent the reader to ITEMS instead of ADD-ONS.
> - §9 still announced **four fields**. There are five. The intro now lists them.
> - **§6 subsections renumbered** 6.1–6.3; the file jumped from unnumbered headings straight to "6.5".
> - **§17 never listed solo play** in the v1 scope, despite v2.6 adding it as a required onboarding path.
> - §6.3's opening still asserted 19 nodes for two players without flagging that the figure is pre-fix.
> - **The phase diagram showed global events and incidents firing from round 1**, contradicting the quiet spell added one revision earlier in §14.2.
> - **The 20-round long variant has no deck.** Events would need 17 cards against a 12-card draw, and incidents 18 against a 16-card deck — the second is not merely short, it is impossible. Demoted to §16.2 with the problem stated rather than left as a bookable option.
> - The time budget said "animated resolution", which mandates a presentation the design has no business mandating. Now "resolution review".
>
> **Changelog v2.7 → v2.8** — two pieces of loose prose that would have become bugs
> - **"Delivered the moment a slot frees"** (§8.2) read as though an offer could arrive mid-round. It cannot, and an implementer taking it literally would have had to draw contract cards against a slot count that does not exist yet. It means the next offer phase, and now says so.
> - **Abandoning cargo was missing from the field table** in §9. It was described in prose and carried in the order, but a reader building from the table would have shipped without it.
>
> **Changelog v2.8 → v2.9** — a stale cross-reference (D15)
> - **§9.2's Stake Post row restated a flat cap of 5.** The player-count-scaled cap in §10.3 has existed since v1.1 (5/4/4/3), and v2.2 corrected the two-player value to 4 — §9.2 never picked up either change. The row now points at §10.3 instead of quoting a number, so the two sections cannot drift apart again.
>
> **Changelog v2.9 → v2.10** — the sector-size floor didn't fit three named maps (D8)
> - **§6.1 constraint 3's 4–8-nodes-per-sector floor needed 16 nodes minimum**, which the 15-node two-player map and the 12- and 16-node §19.1 scenario maps all fall short of or barely meet. Lowered to **3–8**, which every named node count now satisfies with four sectors, unconditionally. Sector count, the Unstable Sector mechanic, and sector-majority scoring are all unchanged — full reasoning in [D8](../decisions/D08-sector-size-constraint.md).
>
> **Changelog v2.10 → v2.11** — the node-type shares had no rounding rule (D9)
> - **§6.2 gave one worked example (25 nodes) and no rule for anywhere else.** Naive per-type rounding doesn't even conserve the node count at 15, 22 or 28. Added the largest-remainder allocation rule (floor each share, hand remainders to the largest fractional part, ties broken by the table's own declaration order) plus the full table for every named node count, 12 through 28 — full reasoning in [D9](../decisions/D09-node-type-rounding.md).
>
> **Changelog v2.11 → v2.12** — the contract pool can run dry mid-match (D7)
> - **§8.1's opening-offer guarantee (constraint 7) only covers setup.** A player who explores little, or a `Bridge Down` that severs the last path to a Known Warehouse, can leave a later offer with fewer than three valid contracts, or none. Added a fallback ladder to §8.1 — drop to the next-lower tier's band, never a higher one, never by widening a band or relaxing the Known-origin rule; a slot still empty at Tier I is dropped, not filled invalidly. **§8.2's held-offer rule now also covers a completely empty pool**, not just a full 2-contract slot, on the same terms: the cooldown does not restart, and the offer arrives the moment any pair becomes valid. Full reasoning in [D7](../decisions/D07-contract-pool-fallback.md).
>
> **Changelog v2.12 → v2.13** — the offer had no tier mix (D6)
> - **Nothing said what tiers a three-contract offer draws from.** Read literally, a Legend eligible for I–IV could be shown three Tier I contracts — and §24.2's own ladder arithmetic silently assumed that never happens, since every completed job in that table lands at exactly the top eligible tier's RP. §8.1 now states the rule that arithmetic already assumed: one of the three targets your highest currently eligible tier (still subject to the existing pool-short fallback below if that tier is empty), the other two are drawn from your eligible tiers on an even split by default. Full reasoning in [D6](../decisions/D06-contract-tier-mix.md).
>
> **Changelog v2.13 → v2.14** — Riot had no specification, and it had a fog dimension (D4)
> - **§14.3's Riot entry was one sentence — "randomized... names, events, all of it" — with no method and no rule against manufacturing a false named fix.** Added a passage under the HAZARDS table: Riot **permutes**, never invents, and only touches the round's sight-gated trail entries in the flagged sector (cargo taken, fresh tracks, confrontations, item purchases) — the five global-announcement types (delivery, post staked, lease expired, Loitering, loose crate) are untouched, because scrambling those would contradict already-broadcast fact rather than obscure fog. Entry contents, including whether and whom an entry names, travel unchanged; only the node it's attached to moves. The affected player is told their own trace moved, not where; nobody else is told anything the public Headline didn't already disclose. Full reasoning, the RNG method, and the index cost in [D4](../decisions/D04-riot-trail-randomization.md).
>
> **Changelog v2.14 → v2.15** — Phase 8 (Upkeep) was never enumerated (D5)
> - **§4's phase diagram named "Upkeep" and nothing else ever said what it does.** Added a new §15 subsection spelling out the four ordered steps — contract deadlines, lease decrement, Sinkhole decrement, next-round modifier clear — and the one ordering among them that's load-bearing, not stylistic: the contract-deadline Debt cascade has to run before the lease decrement it depends on reading "fewest rounds remaining" from, or a healthier lease than necessary gets surrendered. `Flagged` and the Evasive step penalty turn out not to be Upkeep steps at all — both are consumed at the moment the following round reads them, not cleared at the end of the round that set them, because either can be set fresh from more than one place in a single round and an end-of-phase clear can't tell "already used" apart from "just set for next round." Also settles that round 15 runs Upkeep like any other round, so final scoring reads state after it. Full reasoning, all four corrections, and both worked examples in [D5](../decisions/D05-upkeep-phase.md).
>
> **Changelog v2.15 → v2.16** — map generation produced no 2D layout (D10)
> - **§7.1 discloses a Rumoured node's "position on the map", and nothing in §6 ever generated one.** Added §6.4: node coordinates are generated once, deterministically from the seed, as part of map generation — never recomputed per viewer, so a node's dot never moves as it goes Rumoured → Known. The four sectors each render as one contiguous region of the canvas, which is what §9.1's Pushing On sector bias needs to be a real choice rather than a bias over confetti. Exact canvas size, lattice, and RNG cost live in RFC §6.4 — full reasoning in [D10](../decisions/D10-map-layout.md).
>
> **Changelog v2.16 → v2.17** — constraint 7 guaranteed an origin but never a destination (D24)
> - **The opening contract offer was not, in fact, "possible in every match."** Measured: it fails for **7.1% of two-player seats** (and 0.6–1.3% at 3–5 players) over 1000 seeds. §6.1 constraint 7 guaranteed a Known **Warehouse** near every start and stopped there — but a contract is a *pair*, and a Warehouse with no Border at a distance inside Tier I's 3–4 band cannot originate one. Two independent causes, both invisible from §8.1's side: constraint 6's "every delivery costs at least 3 steps" is not what its own first sentence enforces (non-adjacency only forces distance ≥ 2, which is below *every* tier's band), and §8.1's fallback ladder cascades within the tiers your **Infamy** allows — which, at setup, is Tier I alone, a band two steps wide, not the full 3+ union the ladder's reasoning assumes.
> - **Constraint 7 now requires the nearby Warehouse to be *contractable*** — to have a Border at 3–4 steps, so the pair the opening offer needs provably exists. This makes §8.1's guarantee structural rather than statistical: 0 failures in 14 000 seats across the same sweep. Nothing about contracts changed — no tier, payout, eligibility or fallback rule moves, and the opening contract a player receives is an ordinary Tier I job.
> - **Constraint 6's second sentence was deleted.** "Every delivery costs at least 3 steps" is true of every delivery that happens, but this constraint is not what makes it true — non-adjacency forces distance ≥ 2, and generated maps do produce distance-2 pairs. §8.3's tier bands, all starting at 3, are the actual floor: a distance-2 pair is in no tier's pool and can never be contracted. A placement rule was being credited with a distance guarantee, and D7's fallback reasoning had leaned on the credit.
> - **§8.2's empty-pool sentence was wrong and is corrected.** "This can only happen if every Warehouse you Know is presently unreachable" missed the commoner mid-match cause: every reachable pair sitting outside the bands your current Infamy entitles you to. Full reasoning in [D24](../decisions/D24-opening-offer-guarantee.md).
>
> **Changelog v2.17 → v2.18** — Dragnet and rotating borders could combine to close every Border (D28)
> - **§6.3's rotating borders and §14.2's Dragnet were specified independently and never checked against each other.** At 2 players the map has only 3 Borders (§6.2's allocation table); rotation leaves 1–2 of them active, and Dragnet seals 2 random Borders from the full set, unfiltered by rotation. The two can coincide and leave zero Borders deliverable — a round where Dragnet's own text, "every delivery must route to the ones that remain," has nothing left to route to. Added a sentence to §6.3: a Border can never be closed by every source at once, at least one always stays open. Full reasoning, the rejected alternatives, and why this can never trigger at 3+ players in [D28](../decisions/D28-dragnet-rotating-borders-fallback.md).
>
> **Changelog v2.18 → v2.19** — the Black Market's refresh cadence was unstated, and three more item/market gaps with it (D25)
> - **§12 said stock "refreshed every 2 rounds" without saying which rounds, and #66's own acceptance criteria had assumed even rounds — a claim that traced to no citable source.** Setup only generates the map, starting positions, and the opening contract offer (§4); nothing populates a market's first stock before round 1's own Phase 3 runs, so round 1 must be a refresh round, and "every 2 rounds" measured from there lands on the **odd** rounds — 1, 3, 5, …, 15 — not the even ones. "Even rounds only" would leave every Black Market showing no stock at all through round 1, contradicting §7.1's promise that sight of the node includes its stock.
> - **§12 now also states the market draws 3 *distinct* items.** Nothing previously ruled out a market repeating an item in its own stock; it draws the same without-replacement way every other multi-pick RNG consumer in this game does (Torn Map, Dragnet), never independent rolls that could duplicate and undercut the market's own scarcity design.
> - **§9.4's Bolt Hole now states what its "2 steps away" is measured from and over: the player's own position at the start of the round, walked through the player's own currently-known subgraph** — the one coordinate both player and server agree on before any order resolves, and the only distance an implementer can check against without reaching past the fog boundary into the full server-side graph. The declared node must itself be Known, not merely Rumoured — a Rumoured node carries no edges to route into (§7.1), so it cannot be the endpoint of a 2-step path over a subgraph that doesn't contain it.
> - **§9.4's Police Band row and §7.2's matching entry now state its target restriction: any node the player is not Hidden to (Rumoured or Known), never a node they have no awareness of at all.** This isn't a new restriction so much as the floor "a node of your choice" already implied — a client can't reference a node it doesn't know exists — but it was previously unstated, unlike Decoy's explicit Known-only restriction two rows below it in the same table. Full reasoning for all four rulings in [D25](../decisions/D25-item-market-resolution-gaps.md).
>
> **Changelog v2.19 → v2.20** — §7.1 promised every node a disclosed name, and nothing said how a node gets one (D31)
> - **§6.2 gains one paragraph stating the naming scheme.** A node's name is its sector, its type, and its 1-based rank among same-type nodes in that sector — e.g. "Old Docks Warehouse 2" — assigned once at map generation, deterministically, from facts already fixed by then. No new randomness, no new deck, no new item. Full reasoning, the rejected alternatives (a drawn name from per-district pools; treating the name as a render-only label), and why the deterministic reading is the more literal one given how §2 and §7.1 already refer to nodes in prose, in [D31](../decisions/D31-node-display-name.md).
>
> **Changelog v2.20 → v2.21** — "Orders never silently fail" read as scoped to Step 0 only, leaving four resolution-time action categories (six failure causes) silently doing nothing (D30)
> - **§15.0's Step 0 paragraph gains a clause: the "never silently fail" promise applies at every resolution step, not only Step 0.** A Deal that loses the market race or whose balance no longer covers it, a Pickup that loses a dropped crate or finds its own cargo slot full, a Stake Post that's already at cap, and a Ledger purchase the balance no longer covers all now name the specific reason to the acting player — the same "legal order can no longer complete" category Step 0 already covers, just discovered one or more steps later. Open Doors is named as the one exception, out of scope rather than silently fixed, and stays exactly as [D14](../decisions/D14-five-resolution-gaps.md) §4 already decided it: a pre-declared, conditionally-triggered boon that may never fire at all isn't a failed action. Full reasoning in [D30](../decisions/D30-contended-action-loss-notification.md).
>
> **Changelog v2.21 → v2.22** — §21's randomness inventory never gained D03's card-target draws (issue #159)
> - **§21 gains entries 16–23**: Dragnet's two sealed Borders, Festival's node, Scaffolding's sector, Shipping Boom's Warehouse, Fence's Windfall's Black Market, Sinkhole's node, Riot's permutation, and Torn Map's four revealed nodes — each a genuine, separate randomness source [D03](../decisions/D03-rng-consumption-table.md) (and, for Riot, [D04](../decisions/D04-riot-trail-randomization.md)) priced into RFC §6.4's table but never counted here. Entry 4's cadence is corrected from the stale "every 2 rounds" to the odd-round schedule §12 already states (D25).
> - **The closing P2-audit count is re-run against all twenty-three entries, not the old fifteen.** Six resolve before commitment (unchanged). Of the seventeen that don't, six are opt-in, four are avoidable by routing around the flagged sector, and four of the eight new entries turn out to be boons the existing Boon Rule (§14.0) already keeps fair — earned by a decision already made, or won as an open contest — leaving only three genuinely blind draws: the event card itself, Dragnet's Border seal, and the fourth-level tie-break. The count of blind draws does not grow from adding eight entries, because the audit's real question is fairness, not row count, and most of the newly-listed sources were already covered by an existing rule that just hadn't been credited against them in this table.
> - No rule change: every source counted here was already live in the code and already governed by D03, D04, D25 or the Boon Rule. This closes a documentation gap, not a design gap. Companion RFC moves to r27, which runs the matching correction against §6.4's consumption table.
>
> **Changelog v2.22 → v2.23** — §21 and §22 catch up to D32, D33 and D34 (issue #202)
> - **§21 gains entry 24: bot decision-making.** A bot's own `Decide` draws (Drifter's route pick, Operator's search) are a real source of randomness in the game, but they resolve **neither** before nor after the player's decision — they're a separate actor's own decision process, not a random event happening to a human's choice, and they draw on a dedicated stream (`BotRNG`) RFC §6.4 deliberately keeps outside the match's own consumption table ([D32](../decisions/D32-bot-rng-stream.md)). The closing P2-audit count is unaffected: entry 24 sits outside the six-before/seventeen-after split it was already scoped to.
> - **§22's framing is corrected.** "Every match — paper or digital — records the following" is true for seventeen of the twenty rows. Three are not headless facts at all: row 16 (Heat Map opened) needs UI instrumentation that doesn't exist until M5; rows 15 (Attribution queries) and 18 (Loitering flags from legitimate play) are human questions, deferred to M5.5 — the second because no operational definition exists yet to compute it against ([D33](../decisions/D33-telemetry-event-stream-coverage.md)). Every row now carries the milestone that produces it.
> - No rule change: every metric counted here was already the correct target, band and failure condition. This closes a documentation gap between what §22 implied was available today and what D33's row-by-row audit against the actual event stream, final state and order log found. Companion RFC moves to r29, which names `internal/telemetry.Match` (D34) and corrects "computed from the event stream" to name the final state and order log D33 found some rows need.
> 
> **Changelog v2.23 → v2.24** — §22 never gained D35's sample size, interval or verdict rule (issue #213)
> - **§22 gains a "Reading these bands" subsection.** Every band in §22 is a point estimate with an action attached and no stated precision — a sweep returning 12.3 against R9's *"> 12"* was either a rule change or nothing, and nothing in either document said which. [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md)'s four parts now sit where the bands are read: 10,000 matches per configuration, the match as the sampling unit with one interval formula for every row shape, action only when the interval clears the band, and a per-band tier baseline (Drifter for the two map-geometry questions, Operator for the five that depend on a player who is trying).
> - **§20's R2 entry gains a pointer to that rule**, so its 3,000-match precedent is not read as the standing sample size — D35 explicitly declines to carry it forward.
> - No rule change and no band moves. D35's own Consequences section assigned this edit to [#202](https://github.com/garnizeh/cinzal/issues/202), which closed covering D32–D34 only; this closes the gap it left. Companion RFC moves to r30.
>
> **Changelog v2.24 → v2.25** — R9 measured clear of §22's threshold at 4-5 players (issue #229)
> - **§6.1's 4-player row raises to 28 nodes, 41-45 edges** (from 25 nodes, 36-40 edges). [#203](https://github.com/garnizeh/cinzal/blob/main/docs/exit-demos/203-confrontation-load.md)'s exit demonstration measured 13.35-13.38 confrontations per match at the old 25-node map (Drifter, both root seeds) — clearing R9's `> 12` action threshold. The roadmap's own action table names raising node count as the first lever to pull; re-measured at 28 nodes, the rate drops to 11.7-11.9, back inside the 4-12 target band.
> - **The 5-player row is unchanged.** The same measurement found 19.10-19.12 confrontations per match at the shipped 28-node map — also over threshold, and by a wider margin. Raising node count cannot close this one: 32 nodes is the most this package's map generator can produce under [D8](../decisions/D08-sector-size-constraint.md)'s four-sector, 3-8-nodes-per-sector split, and even there the measured rate (16.8) stays well above 12. Closing the 5-player gap needs a decision this changelog entry does not make — see `docs/exit-demos/229-node-count-raise.md` for the full measurement and the follow-up decision it opened.
> - No other rule changes. Companion RFC is unaffected — §6.1's table is GDD-owned data, not an RFC-level architecture concern.

---

## 1. Pitch

**Cinzal** is a digital strategy game for 2 to 5 players, played in rounds of **simultaneous orders** on a partially hidden graph map. Each player runs a criminal faction in the city the game is named for — a metropolis in decay — and competes for smuggling contracts: pick up cargo at a warehouse, cross the city without being intercepted, deliver at a border checkpoint.

You never see where your rivals are. But you see what they *do* — cargo vanishing from a warehouse, a new post staked at a corner, a delivery called out on the police band. From those traces you deduce routes, set ambushes, and lie about your own.

A match runs **15 rounds**. With the default 60-second order timer, that lands around **30 to 35 minutes**, and — critically — **the number of players does not change the length**, because everyone plays at once.

**Tagline:** *Everyone knows what you did. Nobody knows where you are.*

---

## 2. Design pillars

Every decision in this document answers to one of these five. If a future rule serves none of them, cut it.

### P1 — Simultaneity above all
Nobody waits for anyone. This solves three problems at once: length is independent of player count, there is zero downtime in synchronous play, and asynchronous play becomes viable — a round is a time window, not a queue.

### P2 — Luck that creates decisions, not luck that delivers verdicts
"You lose 3 credits" is noise. "Choose 1 of these 3 contracts" is a decision. Randomness enters mostly **before** the choice (contract offers, headlines, market stock, the unstable sector). The exceptions — confrontation rolls, Pressure, incident resolution — are short, telegraphed, and voluntarily entered. See §21 for the full audit.

### P3 — Hidden information with a readable trail
Hiding everything produces noise, not bluffing. The game hides **positions** and publishes **consequences**. You infer where someone came from and where they're headed. Anyone who wants to truly disappear pays for it — in Infamy, in route length, in items.

### P4 — No death spiral
Losing a confrontation never costs you territory or income. The trailing player is never hit on three axes at once. The lead is visible, and it is expensive — the most famous player is the one the Guard comes for.

### P5 — Rules that fit on a card
A turn is: pick a route, pick an action, pick a stance. Three decisions. All depth comes from how they interact and from reading the trail — not from rule volume.

---

## 3. Setting

### The city
**Cinzal** — the city, and the title. It was built over a port that silted up. The state walked away from the perimeter twelve years ago, when the cost of policing it passed the cost of pretending it wasn't there. What's left of government is the **Metropolitan Guard** — a contractor that doesn't prevent crime, it invoices it. They man the border gates, sell circulation permits, and when the press gets loud they stage one spectacular raid against whatever name has been in the papers that month.

What moves through Cinzal isn't drugs or guns. It's **cargo** — the local euphemism for anything that has to leave a warehouse and reach a border without touching a register. Diverted medicine, machine parts, people. Nobody asks.

### The four districts

| Sector | Colour | Character | Predominant nodes |
|---|---|---|---|
| **Old Docks** | Blue-green | Dead wharves, seized cranes | Warehouses |
| **Iron Low** | Ochre | Industrial flats, sheds, overpasses | Alleys and Warehouses |
| **Mist Heights** | Violet | Residential hillside, stairs, standing fog | Black Markets |
| **North Vale** | Dry red | The way out, guard huts, containers | Borders |

Geography carries weight: the borders, where money is made, sit far from the warehouses, where cargo is taken. Every delivery is a crossing.

### The factions
Five playable factions. **In v1 they are cosmetic only** — identical rules. Asymmetry is a post-balance module (§17).

- **The Ravens** — scrappers out of Old Docks. Beak and wire.
- **The Port Brotherhood** — dockhands who organized, then armed.
- **The Pale Syndicate** — bookkeepers and loan sharks from Mist Heights. Clean suits.
- **Children of the Vale** — born in the containers of North Vale. Nobody knows how many.
- **Scrap Iron & Co.** — officially a recycling firm. Has a tax number and everything.

### Tone
Brazilian urban noir. Concrete, rain, sodium light. No heroes. Game text — events, contracts, items — is dry and faintly ironic; closer to *City of God* than *Sin City*. Violence isn't glamorized: this is a game about logistics and reputation, and violence is a tedious operating cost.

---

## 4. Shape of a match

```text
SETUP
  └─ map generation, starting positions, opening contract offer

ROUND 1 ─────────────────────────────────────┐
  Phase 1 · Headline + Unstable Sector (auto) │
  Phase 2 · Contract offer            (auto)  │
  Phase 3 · Market refresh            (auto)  │
  Phase 4 · ORDERS                  (PLAYERS) ←── the only input phase
  Phase 5 · Resolution                (auto)  │
  Phase 6 · Global Event              (auto)  │  rounds 4-15 only
  Phase 7 · Incident + Pressure       (auto)  │  incident: rounds 3-15
  │                                           │  pressure: every round
  Phase 8 · Upkeep                    (auto)  │
ROUND 2 … ROUND 15 ──────────────────────────┘

FINAL SCORING
```

**Only Phase 4 asks anything of the player.** Everything else is the engine running and showing its work. That single sync point per round is exactly what makes asynchronous play trivial rather than a separate mode.

### Time budget (synchronous)

| Item | Time |
|---|---|
| Order phase (default timer) | 60 s |
| Resolution review | 45 s |
| All automatic phases | 20 s |
| **Per round** | **~2 min 5 s** |
| **15 rounds** | **~31 min** |
| Setup + final scoring | ~4 min |
| **Full match** | **~35 min** |

The order timer is configurable: 45 s / 60 s / 90 s / unlimited. A 45-second table finishes in about 28 minutes; a 90-second table in about 43. **Match length does not scale with player count.**

**On the retired 30-minute promise:** the original target was 30 minutes flat. Raising the round count to 15 buys the economy the room it needs — with tolls gone and leases running, 10 rounds gave players roughly two and a half contracts each, which isn't enough to feel like a career. The honest position is that this is a **30–40 minute game** whose length is set by the timer, not the player count. That second property is the one worth protecting.

**Long game variant:** see §16.2 — it needs deck rules it does not currently have.

---

## 5. Player state

Five attributes. You see all of yours; of others, you see what the fog allows (§7).

| Attribute | Start | Range | Visibility |
|---|---|---|---|
| **Cr$ (Credits)** | 12 | 0 – ∞ | **Band only** (§5.1) |
| **Infamy** | 0 | 0 – 10 | Public, exact |
| **Reputation (RP)** | 0 | 0 – ∞ | Public, exact |
| **Position** | starting node | 1 node | **Hidden** |
| **Cargo** | empty | 0 or 1 | Hidden (the pickup leaves a trace) |
| **Posts** | 0 | 0 – cap (§10.3) | Public, with lease expiry visible |

**There is no passive income.** Every credit in the game arrives from a delivery, a boon, a stake won, or a shakedown collected. Posts pay nothing (§10.2), there is no per-round stipend, and the phase list in §4 has no income phase because there is nothing to pay out. Starting capital is Cr$ 12 and the first delivery typically lands around round 4 or 5, so the opening is deliberately lean — the first lease and the first item purchase are real trade-offs against an empty pipeline.

**Why Infamy and RP are exact and public.** They are the game's threat axes. If nobody can tell who is winning, nobody knows who to hunt, and the leader escapes unopposed. Deliveries are announced globally anyway, so RP would be derivable regardless — publishing it is honesty, not a leak.

**Contracts in hand are hidden.** This is the information opponents most want and the foundation of every bluff. Seeing you take cargo at Docks Warehouse tells me you're crossing. It does not tell me whether you're headed for North Gate or East Gate.

### 5.1 Credit bands

Exact balances are hidden. Players see a band:

| Band | Range | Label shown |
|---|---|---|
| 0 | Cr$ 0–5 | **Broke** |
| 1 | Cr$ 6–15 | **Getting by** |
| 2 | Cr$ 16–30 | **Flush** |
| 3 | Cr$ 31+ | **Loaded** |

Bands update live. A player dropping from Flush to Broke in one round is itself a loud signal — someone paid a big stake, or bought a long lease, or got robbed.

**The Ledger.** As an order add-on (§9.5, Field 5), any player may pay **Cr$ 3** to buy the Pale Syndicate's books. At resolution they receive the **exact balance of every player as of the end of the previous round**. The purchase is private — nobody knows you bought it.

Two constraints, both deliberate:

- **The books are always one round stale.** You learn where everyone stood, not where they stand. Stakes paid, leases bought, and items purchased this round are not in the figures you get.
- **The Ledger cannot be purchased in the final round.** The Syndicate closes its books before the month ends.

Without those two rules, every player buys the Ledger on the last round and computes the exact delivery needed to win by one point — which deletes the entire purpose of credit bands at the precise moment the bands matter most. With them, the Ledger is what it should be: a tool for reading **trajectory** over a match, not a calculator for the final move.

---

## 6. The map

### 6.1 Generation
Undirected graph, generated procedurally per match from a recorded seed (needed for replay and for fixed-map tournament tables).

| Players | Nodes | Edges | Average degree |
|---|---|---|---|
| 2 | **15** (see §6.3) | 21–23 | 2.8–3.0 |
| 3 | 22 | 31–35 | 2.8–3.2 |
| 4 | **28** (see §20, R9) | 41–45 | 2.9–3.2 |
| 5 | 28 | 40–45 | 2.9–3.2 |

*(v1.0 quoted "~1.9 edges per node", which implies an average degree of 3.8 — incompatible with the max-degree-4 constraint below, and a far denser map than intended. Corrected.)*

**Generation constraints.** The generator rejects and retries until all hold:
1. The graph is connected.
2. Minimum degree 2, maximum degree 4. No dead ends (frustrating), no super-hubs (dominant).
3. Each sector holds 3–8 nodes and is internally connected.
4. Between any two adjacent sectors there are **at least 3 and at most 5 edges**. These are the **chokepoints** — where ambushes happen and where a post is worth its lease.
5. Minimum graph distance of 4 between any two starting positions.
6. No Warehouse is adjacent to a Border.

    *(v2.17 deleted a second sentence here, "Every delivery costs at least 3 steps," which this constraint does not enforce and never did: non-adjacency forces distance ≥ **2**, not ≥ 3, and generated maps do produce distance-2 Warehouse/Border pairs. The 3-step floor is real, but §8.3 is what provides it — every tier's distance band starts at 3, so a distance-2 pair is never in any tier's pool and can never be contracted. A delivery that happens always costs at least 3 steps; a **placement** rule was being credited with it. [D7](../decisions/D07-contract-pool-fallback.md) flagged the two sentences as "not obviously the same fact" and declined to lean on it; [D24](../decisions/D24-opening-offer-guarantee.md) found the counterexample.)*
7. **Every starting node has at least one Warehouse within 2 steps that itself has a Border 3–4 steps away** — a *contractable* Warehouse, at Tier I's distance band (§8.3). Those nodes begin **Known** to that player. Without this, the opening contract offer cannot be generated at all — see §8.1.

    *(v2.17: the second clause is new. "A Warehouse within 2 steps" guarantees an **origin**; a contract is a pair, and a Warehouse with no Border inside the only band a Nobody is eligible for cannot originate one. Measured before the fix, 7.1% of two-player seats opened with no offer available. See [D24](../decisions/D24-opening-offer-guarantee.md).)*

### 6.2 Node types

| Type | Share | Function |
|---|---|---|
| **Warehouse** | 24% | Where cargo is taken (Pickup). No supply limit — see note below. |
| **Border** | 24% | Where cargo is delivered. Delivery costs **Cr$ 1** in gate fees — the Guard always takes its cut. |
| **Black Market** | 20% | Where items are bought (§12). Each market shows 3 rolled items, visible to anyone with sight of it. |
| **Alley** | 32% | No economic function. But an **Aggressive** player in an Alley gets **+1** in confrontation. Predator ground. |

**Allocation rule (D9), for every supported node count.** Floor each type's exact share, then hand the remaining nodes one at a time to whichever types have the largest fractional part, breaking ties by the table order above — Warehouse, Border, Black Market, Alley. At 25 nodes every share is already an integer, so the rule assigns no remainder and reduces to the plain 6/6/5/8 split; it is not a special case, just the one row with nothing left over:

| Nodes | Warehouse | Border | Black Market | Alley |
|---|---|---|---|---|
| 12 | 3 | 3 | 2 | 4 |
| 15 | 4 | 3 | 3 | 5 |
| 16 | 4 | 4 | 3 | 5 |
| 20 | 5 | 5 | 4 | 6 |
| 22 | 5 | 5 | 5 | 7 |
| 25 | 6 | 6 | 5 | 8 |
| 28 | 7 | 7 | 5 | 9 |

The seven rows above are every node count currently supported (§6.1's per-player-count table, plus §19.1's scenario sizes). The table is the readable form; the rule is the authority a new supported node count is computed against.

**Cut in v1.1: the warehouse supply limit.** v1.0 said each warehouse released only 2 cargo per round. With 6 warehouses, hidden positions, and contracts pointing players at different origins, three players converging on the same warehouse in the same round is close to a never-event. It was a rule that added a line to the reference card and fired approximately zero times per match. Cut. If playtesting shows warehouses genuinely contested, it comes back — but as a **Dockers' Strike** effect, which is where scarcity belongs.

**Node names (D31).** Every node has a display name, disclosed alongside its type and sector from Rumoured onward (§7.1): its sector, its type, and its 1-based rank among nodes of that same type within that sector, ranks assigned by scanning the map's nodes in ascending `NodeID` order — the first node of a given (sector, type) pair encountered that way is rank 1, the next is rank 2, and so on — e.g. **"Old Docks Warehouse 2"**. Assigned once at map generation, deterministically, from facts already fixed by that point — no randomness, no authored name pool. Full reasoning in [D31](../decisions/D31-node-display-name.md).


### 6.3 Two-player rules

Two players average 4.5 encounters a match, and **27% of matches produce fewer than three** (§20) — figures measured on the pre-fix 19-node map, which is what motivated the changes below. A duel that runs fifteen rounds with no interaction isn't a duel, it's two spreadsheets sharing a background image. Two changes apply at this player count only:

1. **Rotating borders.** Each round, only **half the Borders accept deliveries**, announced in the Headline alongside the unstable sector. The active set rotates. This concentrates every delivery run in the match into a shrinking target area, which is the most direct convergence pressure available without touching the map. A Border can never be closed by every source at once: if Dragnet's seal (§14.2) would combine with this round's rotation to leave none open, at least one always reopens — see [D28](../decisions/D28-dragnet-rotating-borders-fallback.md).
2. **Tighter map.** 15 nodes — already reflected in the generation table above. The 19-node figure quoted in the simulation below was the pre-fix value.

Simulated together these put the two-player rate into the 9–12 band, comfortably inside target. Both are 2p-only and should never leak into 3+ player tables, where the problem is the reverse.

### 6.4 Layout (D10)

Map generation produces 2D coordinates for every node, not just its topology. Layout is:

- **Generated once, deterministically from the seed**, as the last step of map generation — never recomputed for a particular viewer, and never a function of which nodes that viewer can currently see.
- **Stable for the whole match.** A node's position is fixed the moment the graph is generated; it does not move when the node's fog state changes for a player, including the Rumoured → Known transition §7.1 disclosure depends on.
- **Sector-coherent.** Each of the four sectors renders as one contiguous region of the map, not nodes scattered across the whole canvas — the property §9.1's Pushing On sector bias needs to make "aim at a district" a real choice.

The exact canvas size, the per-sector lattice, and the RNG cost are an architecture concern, not a rules one — see RFC §6.4 for the full mechanism. Full reasoning in [D10](../decisions/D10-map-layout.md).

---

## 7. Fog, sight, and the trail

This is the core of the game. Read it carefully.

### 7.1 Fog is private
Each player carries their own discovery map. My revealing the Docks Warehouse reveals nothing to you. This departs from the original concept of public revelation, for one reason: if everyone sees the same map, exploring is charity.

Four states, per player:

| State | What you see |
|---|---|
| **Hidden** | Nothing. Not even that the node exists. |
| **Rumoured** | Name, type, sector, and position on the map. **No edges**, so you cannot plot a route through or into it. An address without directions. |
| **Known** | Name, type, sector, edges, post owner and lease expiry. But **not** what happens there. |
| **In sight** | Everything above, plus this round's **trail log**, plus market stock if it's a Black Market. |

### 7.2 Sources of sight

| Source | Range |
|---|---|
| Your **ending** position | The node and everything adjacent |
| Each post you hold | **The node itself only** |
| **Surveil** action | Everything within 2 steps of your position, this round |
| *Police Band* item | One node of your choice this round, Rumoured or Known — never Hidden |

**Rumoured** is what a contract destination arrives as (§8.1). You have been told the gate exists and where it sits; you have not been told the way in. A Rumoured node becomes **Known** the moment it falls within your sight — which is to say, the moment you reach something adjacent to it.

A visited node becomes **Known** permanently. Sight is temporary; knowledge is not.

**Passing through is not looking around.** Sight comes from the node you *end* your route on — not from every node you crossed to get there. Nodes you pass through become **Known** (you now have them on your map) but yield **no trail log**. You were moving, head down.

This needed stating because the permissive reading breaks the game. A player with 4 steps crosses 5 nodes; at an average degree of 3 that is 15 to 20 nodes of sight per round out of 25 — most of the board, every round, for free. Posts would be pointless and §7.5's saturation problem would arrive through a second door. Ending-node-only keeps information scarce and keeps the **Surveil** action worth taking.

**The strategic consequence, now more than ever.** With tolls removed (§10), a post earns you no money. What it earns is **vision** — a permanent tripwire on one specific node. A post is a camera you pay rent on. That is the entire reason to hold one, plus 2 RP at scoring.

**Why posts no longer see their neighbours (v1.1).** In v1.0 a post granted sight of its node *and everything adjacent*. On a map with average degree 3, that is four nodes per post. Run the saturation numbers and the game stops working:

| Players | Posts each | Share of map under permanent sight |
|---|---|---|
| 2 | 3 | **~76%** |
| 4 | 3 | **~88%** |
| 5 | 5 | **~98%** |

At those coverage levels every trail entry on the board is legible to everyone every round, and reconstructing a rival's full route stops being inference and becomes arithmetic. P3 dies. Note that this is already broken at **three posts in a two-player game** — capping posts lower does not fix it, because the problem is coverage per post, not post count.

Restricting a post to its own node gives 28–60% coverage across the same range, which leaves real darkness on the board at every player count. It also makes placement a **precision** decision rather than a blanket one: you have to guess the exact node your rival will cross, not smother a neighbourhood. That is a strictly better skill to reward.

### 7.3 The trail

At the end of resolution each node accumulates a **trail log** for the round. You read the logs of nodes you had sight of.

| Event | Trail entry | Name attached? |
|---|---|---|
| Cargo taken | "Cargo left here." | **Only if Infamy ≥ 3** |
| Someone passed through | "Fresh tracks." | Never |
| Confrontation | "Blood and broken glass." | Always, both names |
| Post staked | "Territory marked." | Always |
| Loitering, 2+ rounds | "Someone's been standing here." | Never (global from round 3) |
| Lease expired | "The corner went quiet." | Always |
| Delivery | **Global announcement** — everyone sees it | Always, with tier and location |
| Item purchased | "Someone worked the counter." | Only if Infamy ≥ 6 |

Deliveries are always global, with name, tier, and location. This is how the table knows who's winning, and it's how the leader becomes a target. **There is no quiet victory.**

### 7.4 The trail in practice

> Round 4. My post on Fifteen Overpass reports *fresh tracks*. Round 6, same post, *fresh tracks* again. The Overpass links Iron Low to North Vale, and North Vale is nothing but borders. Someone is running the same line twice. Round 7 I don't go to the Overpass — I go to Water Tank Alley, the only other way through, Aggressive. If I've read it right, they take the detour and I take their cargo.

That is the game. Everything else in this document is scaffolding for that paragraph to happen.

### 7.5 The Board

*Promoted to a v1 requirement in v1.1; substantially rescoped in v1.3.* The trail only works if players can hold it, and across a 48-hour asynchronous round nobody can. Left to memory, the interesting part of this game migrates into a spreadsheet on a second monitor — which means the design failed and a third-party tool is doing the work.

**The Board** is the client-side intelligence wall, and P3 lives inside it.

#### What the deduction layer actually is

v1.1 specified **ghost paths**: select a trace, and the client shades every node the actor could have reached. Measured against the real map parameters, that feature does not work — and not for the reason you would guess. The computation is trivial (25 nodes, five actors, fifteen rounds is microseconds of work on any device). The problem is that the answer is always *everywhere*:

| Nodes | Steps/round | Cone after 1 round | 2 rounds | 3 rounds |
|---|---|---|---|---|
| 19 | 4 | **96%** | 100% | 100% |
| 25 | 4 | **94%** | 100% | 100% |
| 25 | 3 | 71% | 100% | 100% |
| 25 | 2 | 40% | 93% | 100% |
| 70 | 3 | 38% | 98% | 100% |

A four-step cone covers 94% of the board after a single round. Capping the window to three rounds — the obvious mitigation — buys nothing, because saturation happens inside round one. Note the last row: this does not survive at **any** map size a 30-minute game could justify.

**The conclusion is structural, and the rest of the document should stop pretending otherwise.** Positional inference in this game has a horizon of roughly **one round**. What has no horizon is **pattern**: a chokepoint that reports tracks in rounds 4, 6, and 8 tells you a route is in use, and that remains true no matter how mobile the actors are, because it is a claim about an edge rather than about a person. The §7.4 worked example is exactly this, and it is the durable form of the mechanic. Track *routes*, not *people*.

The Board is therefore built around three tools, none of which is a ghost path.

#### 1. The Log
Every trail entry you have ever received, stamped with round and node. Filterable by round, node, sector, and entry type. Never expires, never scrolls away. Unglamorous and the most-used feature in the app.

#### 2. Attribution — hard one-round horizon
Select a trace and the client answers *who could have made this*, working **backwards** from public named anchors rather than forwards into fog.

Anchors are events that fix a named player at a known node in a known round: **deliveries** (global, named, located), **confrontations** (both names), **post stakings**, **Loitering** entries, and the position reveals that Feared and Legend players emit automatically (§11).

From an anchor one round old, the client can genuinely rule players in and out. From an anchor two rounds old it cannot, and **the UI says so** rather than rendering a shrug as a diagram. Attribution greys out past the horizon; it does not degrade silently.

The corollary is a real strategic consequence: **the Feared and Legend tiers are the only players in the game who are trackable across rounds**, because they anchor themselves every round for free. That is not a bug to fix. It is the exposure cost of the Infamy ladder finally paying off in the fiction, and it means hunting the leader is genuinely easier than hunting anyone else.

#### 3. The Heat Map
Traffic frequency per node and per edge across the whole match, no time limit and no attribution attempted. Nodes that report tracks repeatedly are corridors. Corridors are where posts and ambushes go.

This is the tool that survives saturation, because it is a claim about traffic rather than about a person, and it is the one that should be on screen by default.

**Data sourcing — and why raw counts would lie.** The Heat Map draws from exactly two sources, and never from server truth the player hasn't earned:

1. **Your own observation history** — every trail log you have personally received, all the way back to round 1. Private fog stays private (P3); the Board aggregates what you saw, it does not query what you didn't.
2. **Public anchors** — deliveries, confrontations, post stakings, Loitering entries. These are announced globally by rule, so overlaying them leaks nothing.

The subtler problem is that **a raw count measures where you were looking, not where the traffic was.** A chokepoint you watched for ten rounds will out-score a busier one you watched twice, and the player would read that as signal. So the Heat Map displays a **rate, not a total**:

> **Fifteen Overpass** — tracks on 4 of 6 rounds observed · **67%**
> **Water Tank Alley** — tracks on 2 of 2 rounds observed · **100%** *(low confidence)*

Sample count is always shown, and anything under three observations is flagged low-confidence. This is a straight improvement over a total even setting the fog question aside — it normalises for observation bias, and it makes the player's own coverage gaps legible instead of invisible.

**On the "one post means an empty map" concern:** sight also comes from your ending node every round (§7.2), so a mobile player accumulates observations across most of the board over fifteen rounds — unevenly, which is exactly what the confidence flag is for. A player who never moves and holds one post *should* have a thin Heat Map. That is not a UI failure, it is the game correctly reporting that they haven't been looking.

#### 4. Pins and notes
Manual annotation, because players will out-think the tooling and should be able to record it.

**Design position:** the client does the bookkeeping, the player does the inference — and the client never fabricates confidence it does not have. A tool that shades 94% of the map and calls it intelligence is worse than no tool, because it teaches players the deduction layer is noise.

---

## 8. Contracts

### 8.1 How they work
You may hold up to **2 active contracts**. You do not receive an offer every round — offers are gated by the **Contact Cooldown** (§8.2).

When an offer comes, you see **3 contracts** and take **1**, or decline all. Declining does not reset the cooldown — it restarts it. This is the game's main injection of randomness, and it always passes through a choice (P2).

**Tier mix of an offer (D6).** One of the three always **targets** your **highest currently eligible tier** — eligibility uses your Infamy at Phase 2, the same moment that gates the Contact Cooldown (§11.1). The other two are drawn independently from your eligible tiers, on an even split by default. This is what makes climbing the ladder pay off in fact and not just in expectation: every offer to a Known, Feared or Legend player includes a shot at the tier their Infamy just bought them, not three more Tier I jobs at long odds against ever seeing the one they climbed for. (If that targeted tier's pool happens to be empty, the pool-short fallback below still applies to this slot exactly as it does to the other two — the guarantee is on the target, not an exemption from the fallback.)

A contract specifies:
- **Origin** — a specific Warehouse
- **Destination** — a specific Border
- **Deadline** — rounds until it expires
- **Payment** — Cr$ on delivery
- **Reputation** — RP at final scoring
- **Penalty** — cost of failure

**Rule of generation:** the game only offers contracts whose **origin** you already Know. The **destination is revealed by the offer itself** and becomes **Rumoured** when you accept (§7.1) — your fixer tells you which gate; finding your way there is your problem.

**Why this changed in v1.9.** v1.8 required both endpoints to be Known, and that rule made the **opening offer mathematically impossible**, in every match, for every player. The proof is three lines:

- At setup you Know your starting node and its neighbours, so any two nodes you Know are **at most 2 steps apart**.
- The cheapest contract, Tier I, requires **distance 3–4**.
- Therefore no valid contract exists at setup. The generator draws from an empty pool.

Constraint 6 makes it worse rather than merely unlikely: since no Warehouse is adjacent to a Border, a player starting on either type is guaranteed to Know none of the other. This was not an edge case waiting for an unlucky seed — it was a deadlock on the first frame of every match.

Revealing the destination fixes it without giving anything away that matters. **Knowing where a gate is does not tell you how to reach it**: a Rumoured node carries no edges, so you cannot plot into it until you have brought something adjacent to it into sight. A destination on the far side of the fog is a goal you have to explore toward. The contract supplies the objective; exploration still supplies the path.

*(v1.9 said destinations become "Known", which was wrong in a way that mattered: Known nodes are plottable, and that would have handed players a free route endpoint across the fog. The Rumoured state, added in v2.0, is what that sentence was reaching for.)*

Constraint 7 (§6) then guarantees a **Known** Warehouse to originate from — genuinely Known, and genuinely reachable. A Warehouse at distance 2 sits behind exactly one intermediate node, and that node is adjacent to your start, so it is already Known from opening sight. The path start → intermediate → Warehouse is Known end to end, with no gap to plot around.

Constraint 7 guarantees the other half of the pair too: that Warehouse has a Border **3 to 4 steps away**, which is Tier I's band (§8.3), and Tier I is the tier every player is eligible for at Infamy 0. So a valid `(origin, destination)` at your own tier provably exists on round 1, on every seed — not merely a Known Warehouse to hope one can be built from.

*(v2.17: the second half is new. Until then constraint 7 stopped at the origin, and the offer failed for 7.1% of two-player seats — the same class of bug as the v1.8 deadlock above, one step further along. See [D24](../decisions/D24-opening-offer-guarantee.md).)*

Exploration keeps its return, too — a wider Known map means more origins available, which means more and better offers.

**Fallback when the pool is short (D7).** Constraint 7 guarantees a valid opening offer; nothing guarantees one later — a player who explores little Knows few Warehouses, and `Bridge Down` can push a pair permanently out of range. If a slot's target tier has fewer than one valid (origin, destination) pair, the generator drops to the next-lower tier's distance band and tries again — **never** to a higher tier, and never by widening a band or relaxing the Known-origin rule — repeating down to Tier I. A slot still empty after Tier I is dropped rather than filled with an invalid contract: an offer of one or two contracts is honest, and declining an offer entirely is already a documented outcome. The offer names the reason ("fewer contracts than usual") rather than staying silent — you already know your own fog, so nothing is disclosed that you couldn't already work out. See §8.2 for what happens when the pool empties out completely.

*(The fallback is a fixed, bounded cascade — target tier, then each lower tier in turn, at most three slots × four tiers — never a loop that redraws until three are found.)*

### 8.2 Contact Cooldown

Your fixer doesn't take everyone's calls at the same rate. The more your name is worth, the faster the work comes back around.

| Infamy | Title | Cooldown between offers |
|---|---|---|
| 0–2 | Nobody | **4 rounds** |
| 3–5 | Known | **3 rounds** |
| 6–8 | Feared | **2 rounds** |
| 9–10 | Legend | **1 round** |

The cooldown starts the round you accept or decline an offer. It is **always displayed** — the player sees "Next offer: round 9" on the HUD at all times, never a surprise.

If you're at 2 active contracts when the cooldown elapses, the offer is **held and delivered at the next offer phase in which you have a free slot**. The cooldown does not restart while an offer is held — the moment a slot opens, the offer is waiting.

**The same hold applies when the pool itself is empty (D7)** — every eligible tier's fallback cascade (§8.1) coming up with nothing. This happens when no Warehouse you Know reaches any Border at a distance inside a tier you are **eligible** for: either the pair is unreachable on the navigable graph (most likely a `Bridge Down` that severed the last path), or every reachable pair sits outside the bands your current Infamy entitles you to. Constraint 7 rules this out at setup; nothing rules it out later. The cooldown does not restart; the offer is delivered, at whatever size the pool then supports, the moment any pair becomes valid again.

*(v2.17 corrected this. It previously read "can only happen if... unreachable", which missed the commoner cause: a low-Infamy player's eligible bands are narrow — Tier I alone is two steps wide — so a perfectly reachable pair can sit outside all of them. Reachability was never the only way to empty the pool. See [D24](../decisions/D24-opening-offer-guarantee.md).)* A held-for-full-slots offer and a held-for-empty-pool offer compose — the round waits on both conditions clearing.

*(Read literally, "the moment a slot frees" would mean an offer arriving mid-resolution — after a delivery in Phase 5, say. That cannot work: orders for the round are already submitted, so an offer you cannot act on is not an offer, and generating one would mean drawing cards against a slot count the engine has no way to know at Phase 2. Offers are evaluated once per round, at Phase 2, against the state at the close of the previous round.)*

**Why this matters.** This is the second carrot on the Infamy ladder, and it's a sharp one. A Nobody at Infamy 0 gets roughly 4 offers across a 15-round match. A Legend gets an offer nearly every round and is limited only by the 2-slot cap and by travel time. Combined with tier gating (§8.3), staying anonymous now has a real and compounding cost, which is precisely what the original design was missing.

### 8.3 Contract table

| Tier | Infamy required | Distance | Payment | RP | Deadline | Penalty |
|---|---|---|---|---|---|---|
| **I — Small time** | — | 3–4 | Cr$ 8 | 2 | 4 rounds | Cr$ 3 |
| **II — Standing** | ≥ 3 | 4–6 | Cr$ 14 | 3 | 5 rounds | Cr$ 5 |
| **III — Heavy** | ≥ 6 | 5–8 | Cr$ 20 | 5 | 5 rounds | Cr$ 8 |
| **IV — The Score** | ≥ 9 | 6+ | Cr$ 30 | 8 | 6 rounds | Cr$ 12, −2 Infamy |

Distance is the shortest path between origin and destination.

Delivering any contract grants **+1 Infamy**; tiers III and IV grant **+2**.

Deadlines are deliberately generous at the high tiers, because high-Infamy players move slowest (§9.1). A Legend on 2 steps per round needs three rounds just to cover a distance-6 run, before counting the trip to the warehouse.

### 8.4 The cargo
- You **pick up** at the origin warehouse (Pickup action).
- You carry **one cargo at a time**. Two active contracts means two trips, or dropping one.
- Lose a confrontation while carrying, and **the cargo falls at that node**. It stays there for the rest of the match, and **any player holding a matching contract can collect it**. Lost cargo doesn't leave the world — it becomes a prize on the ground that pulls people toward it.

**Two classes of cargo.** The distinction matters for deadlines and for who may pick a crate up:

| Class | Where it comes from | Who can collect it | Deadline |
|---|---|---|---|
| **Bound cargo** | A Warehouse, against a contract | Any player holding a contract with the **same origin and destination** | The collecting player's own contract deadline |
| **Loose crate** | Dead Runner, Spilled Load | **Anyone**, no contract required | None — but it runs **hot** (below) |

**Loose crates run hot.** A crate nobody has papers for is the most talked-about object in the district. From the **second consecutive round** you end holding one, a **global announcement** fires naming the node you're standing on — not you, but the node is enough.

This exists because loose crates otherwise punched a hole straight through the anti-camping argument in §9.1. That argument rests on opportunity cost: a stationary player earns nothing while their deadlines run down. A loose crate has no deadline, so a camper could take one, park on a chokepoint in Aggressive stance, and farm Infamy and stakes indefinitely with no clock running against them.

Heat restores the pressure without inventing a new subsystem or a decay tracker. It reuses the Loitering escalation shape, and it does something a value penalty wouldn't: it converts the exploit into a **convergence event**. Everyone learns where the crate is, and they come for it. The camper stops being the predator and becomes the destination.

**Contracts are per-player instances, not global objects.** Each player draws their own offer of three and keeps their own contract, with its own deadline and its own Deadline Pause flag. Two players may hold contracts with the same origin/destination pair — and when they do, a dropped crate becomes a race between them.

Which settles the ownership question the Pause raises: **the flag lives on the contract instance.** If A is ambushed and gets the extension, then drops the crate and B collects it, B is fulfilling *B's* contract, on *B's* untouched deadline, with *B's* own Pause still available. Nothing is inherited, because there was never a shared object to inherit from. A's contract also stays active — A can go back for the same crate, which is exactly the race described above.
- Miss the deadline: pay the penalty, discard the contract, and any cargo you were carrying for it is gone.

---

## 9. The Order — the heart of the turn

In the Order phase every player fills in five fields, simultaneously and in secret:

| Field | | Required? |
|---|---|---|
| 1 | **Route** (§9.1) | yes — an empty route is a legal route |
| 2 | **Action** at the final node (§9.2) | yes — *Nothing* is an option |
| 3 | **Stance** (§9.3) | yes |
| 4 | **Items** to discard (§9.4) | optional |
| 5 | **Add-ons** (§9.5) | optional |
| — | **Abandon carried cargo** — a flag, not a field of its own (§9.3) | optional |

### 9.1 Field 1 · ROUTE

A sequence of steps along graph edges, starting from your position. An empty route (staying put) is legal. You may only plot through **Known** nodes.

**Your step allowance is set by your Infamy:**

| Infamy | Title | Steps per round |
|---|---|---|
| 0–2 | Nobody | **4** |
| 3–5 | Known | **4** |
| 6–8 | Feared | **3** |
| 9–10 | Legend | **2** |

#### 9.1a Step allowance — order of operations

Every modifier is summed against the Infamy base, **then** the floor is applied, **then** any hard cap. The floor is applied exactly once, at the end. Anything else lets a negative reach the path validator.

```text
steps  =  infamy_base                    # 4 / 4 / 3 / 2  by tier (§11)
        − 1  if Flagged                  # §13, boolean, never stacks
        − 1  if Curfew is in effect      # global event, §14.2
        − 1  if you lost an Evasive confrontation last round   # §15
        + 1  if Distracted Guard          # incident boon, §14.3
        + 1  if Scaffolding               # global event, §14.2
        + 2  if Retainer                  # global event, §14.2

steps  =  max(1, steps)                  # floor, applied once, applied last
steps  =  min(steps, hard_cap)           # Streets Blocked sets hard_cap = 1
```

The worked worst case: a Legend (base 2) who lost an Evasive confrontation, during a Curfew, while Flagged, sums to **−1** — and floors to **1**. Additions and subtractions commute, so their order among themselves is irrelevant; only the position of the floor matters, and it is last.

Caps are not modifiers and are applied after the floor, so **Streets Blocked** produces exactly 1 step no matter what the sum was.

**Why mobility falls as Infamy rises.** Nobody looks twice at a nobody. A Legend can't cross three blocks without being recognized, reported, and followed — every route is a careful route. Mechanically this is the counterweight that makes the Infamy ladder a genuine curve rather than a straight climb: rising buys you better contracts, faster offers, and combat weight, and it costs you speed, anonymity, and police attention. The optimum sits somewhere in the middle for most players, and finding where is the strategic spine of the match.

**Exploration.** You may spend a step entering a **Hidden** node adjacent to your current one — the interface shows the stubs of unexplored edges. You don't know what's there until you arrive. It's the only blind movement in the game, and it's always optional.

**Loitering.** You are loitering in a round if **both** of the following hold:

1. Your route ends **within 1 step** of where it ended last round — the same node, or any node adjacent to it.
2. You performed no **Pickup, Deliver, Stake Post, Deal**, or qualifying **Vanish** action.

A **Vanish** qualifies only if it actually reduced your Infamy by at least 1. At Infamy 0 it is a no-op and does not exempt you.

Warehouses and Borders are exempt as ending nodes; waiting for a warehouse window is legitimate work.

| Consecutive loitering rounds | Consequence |
|---|---|
| 1 | Nothing. |
| 2 | A **"Someone's been standing here"** trace fires at that node, readable by anyone with sight of it. |
| 3+ | The same entry becomes a **global announcement** naming the node — though not the player. |

*(On the Vanish exemption: laying low is a deliberate act, not idling, and §11.1 explicitly contemplates a Legend spending two consecutive rounds on it to get out from under position reveals — which under v1.4 would have fired a global Loitering announcement naming the exact node they were hiding at. Self-defeating. But a blanket exemption creates a worse problem: a player sitting at Infamy 0 could Vanish on a chokepoint every round forever, exempt from Loitering and with their movement traces suppressed, which is the perfect invisible camper. Requiring Vanish to actually move the Infamy number closes it — the action exempts you while it is doing something, and stops exempting you when it becomes a fig leaf. Note also that Vanish suppresses **"fresh tracks"** entries, which are about movement; it never suppresses a **Loitering** entry, which is about the absence of it.)*

*(v1.3 defined this as "ending on the same node twice", which was beatable by pacing. With 4 steps a camper can run C→D→C→D within a single round — putting themselves on the chokepoint at half the resolution steps, where collisions are checked — and simply alternate their ending node between C and D across rounds. They never end twice on the same node, never trigger a trace, and still hold the corner. The 1-step radius forces a camper to genuinely leave, and the action exemption keeps short-haul players who legitimately end near where they started out of the net.)*

**This is not an anti-camping penalty, and camping is not an exploit.** Parking on a chokepoint in Aggressive stance is a legitimate play, and it is already brutally self-limiting: contracts are 10–22 RP of a 18–36 RP total, and a stationary player earns none of it while their deadlines run down. The opportunity cost is the balance, and it's severe enough that sustained camping is a losing line on its own.

What Loitering fixes is that camping was **invisible**. An ambush you cannot see coming and cannot route around is bad hidden information — it's the "no rastro to read" failure that P3 exists to prevent. Now a corner held for two rounds shows up on the board, opponents can route around it or bring a Shiv, and the camper has to weigh going quiet against staying effective. The information leak is the cost, and it's a more interesting cost than an Infamy penalty would be.

The narrow case where camping *is* strong — the last two or three rounds, when a trailing player can no longer finish a contract and switches to farming confrontations for Infamy and stakes — remains open, and is logged as part of R11.

**A Hidden node terminates a plotted route.** You cannot plot a step *out of* a Hidden node, because you don't know its edges — the topology past the frontier doesn't exist on your client. So a route may contain **at most one Hidden node, and it must be the last entry**. This is a hard client-side restriction and a server-side validation (§15.0), not a convention.

**Pushing On.** Rather than leave that as a dead end, you may append a **blind step count** of 0–2 to a route that ends on a Hidden node, together with a **sector bias** — one of the four districts, or "none".

Each blind step picks an edge from wherever you've arrived by this priority ladder, evaluated against the **server's true graph**:

0. All distances are computed against the **currently navigable graph** — the base graph minus edges destroyed by **Bridge Down** and minus nodes made impassable by **Sinkhole**. Never against the static setup graph, or the ladder will cheerfully steer a player into a hole.
1. Let *d* be the shortest graph distance from the current node to the **nearest node of the declared sector**.
2. Prefer edges whose destination has distance **less than** *d*. Choose at random among them.
3. If none, prefer edges whose destination has distance **equal to** *d*. Choose at random among them.
4. If none, choose at random among all remaining edges.
5. At every level, exclude the node you just came from unless it is the only option.

If the declared sector is the one you are already standing in, *d* is 0, the ladder degenerates, and the step is a plain random walk with no backtracking. Declaring "none" does the same. If the sector is **unreachable** on the navigable graph, every neighbour has infinite distance, the ladder falls through to level 4, and the same thing happens.

Two terminating cases: if no edge leads anywhere legal, the walk **stops early**, per the pattern used for pushback (§15). And if a blind step would end inside a sector under **Gas Leak**, the walk stops at the last node outside it — Gas Leak restricts *ending*, not traversal, so it constrains where the walk can stop rather than where it can path.

*("Toward" is not a property an undirected procedural graph has natively — v1.6 used the word as though it were self-evident, and no backend could have resolved it consistently without inventing a definition. Strictly preferring distance reduction over maintenance matters too: a "reduce or maintain" rule permits a walk that circles the target sector at constant distance indefinitely, which is not what a player who declared a bias asked for.)* You cannot take an action at the end of a Pushing On route — you have no idea where you'll be.

The bias exists because a purely random walk is luck applied to you, while a steered one is a gamble you shaped: sector boundaries are geography, visible on the map even where individual nodes aren't, so aiming for the Vale while feeling your way through the dark is a real and knowable choice. It is also just what the fiction says you would do.

**Resolution is strictly sequential**, one blind step at a time: move, roll Scavenging, apply the result, then take the next step. A node that is **already Known when you arrive rolls no Scavenging** — including one that became Known thirty seconds earlier because a 6 on the previous step revealed it. This ordering is fixed so replay reproduces exactly (§21), and the whole sequence plays out in the resolution animation rather than arriving as a single teleport.

This turns an engine limitation into a decision, which is the P2-shaped way to handle it. Without it, exploration is permanently capped at one node past your frontier per round and every sensible player explores only on their final step, which makes the mechanic predictable and small. With it, a player can gamble three steps into the dark chasing map knowledge and Scavenging rolls, and occasionally surface somewhere useless. That is a good risk to be able to take.

**Scavenging.** Entering a Hidden node rolls 1D6:

| Roll | Find |
|---|---|
| 1–3 | Nothing but rust. |
| 4–5 | **Cr$ 3** in a stripped car or a dead drop nobody came back for. |
| 6 | Every node adjacent to this one becomes **Known**. |

This exists to satisfy condition 2 of the Boon Rule (§14.0): the payout is random, but it only reaches players who spent a step walking into the dark. It also props up exploration, which otherwise competes badly against a known-good route — and exploration is what feeds the contract generator (§8.1), so the game wants it happening.

### 9.2 Field 2 · ACTION at the final node

Pick one:

| Action | Where | Effect |
|---|---|---|
| **Pickup** | Warehouse matching a held contract, or any node with dropped cargo you have the contract for | Take the cargo. |
| **Deliver** | The destination Border, carrying the right cargo | Payment −Cr$ 1 gate fee, +Infamy, +RP. Global announcement. |
| **Stake Post** | Any unowned node, if under your cap (§10.3) | Buy a lease (§10.4). Public announcement. |
| **Deal** | Black Market | Buy 1 of the 3 items on offer. |
| **Vanish** | Anywhere | **−2 Infamy.** You leave no "fresh tracks" trace this round. |
| **Surveil** | Anywhere | Sight of everything within 2 steps this round. |
| **Nothing** | — | — |

### 9.3 Field 3 · STANCE

One stance, applying to any confrontation anywhere along your route.

| Stance | Effect |
|---|---|
| **Aggressive** | +1 to confrontation. **+1 more if the confrontation lands in an Alley** (ambush). May declare a **stake** of Cr$ 0–6. |
| **Neutral** | No modifier. Stake fixed at 0. |
| **Evasive** | −1 to confrontation. On a loss you **keep your cargo** *if you can pay* — a **Cr$ 4 shakedown** to the winner — and you are pushed back **2 nodes** and lose **1 step next round**. Stake fixed at 0. |

Evasive is the courier's stance; Aggressive is the interceptor's. You choose without knowing whether a meeting will happen, and that is the game's central bluff. Running Evasive every round is expensive in tempo. Being caught Aggressive while carrying is how you hand someone a contract.

**Why Evasive got more expensive (v1.1).** In v1.0 the Evasive penalty was "lose the rest of your route" — which, if you were ambushed on your **final** step, was nothing at all. A −1 modifier bought guaranteed cargo retention with no downside in the most common interception scenario, making it a nearly free insurance policy. Worse, since Evasive stakes are fixed at zero, beating an Evasive courier paid the winner **nothing** but Infamy, which quietly removed the incentive to intercept at all — feeding straight into R2.

The v1.1 penalty fixes both ends. The **Cr$ 4 shakedown** is the premium, and it's paid only when the insurance actually pays out. It also gives the interceptor a payday for a successful ambush against a courier who was never going to be robbed of cargo. The **2-node pushback** and **−1 step next round** are position-independent, so being caught on your last step now costs exactly as much as being caught on your first.

**Abandoning cargo.** Any order may include *abandon carried cargo*, free and costing no action. The crate falls at your ending node, still bound to its pair.

This exists because a winner can end up holding a crate they have no contract for, and without a way to put it down their cargo slot is dead for the rest of the match. It also gives a courier under pressure a legitimate move: drop the crate somewhere awkward and come back for it when the heat is off, at the risk that a rival with a matching contract gets there first.

### 9.4 Field 4 · ITEMS (optional, no action cost)

Seven of the eight items in §12 are discards, and this is where you declare them. You may discard any number of items you hold, up to your hand limit of 3.

| Item | Target declared with it | Resolves |
|---|---|---|
| **Torn Map** | — | **Immediately**, before movement |
| **Guard Contact** | — | **Immediately**, before movement — so the −3 Infamy applies to this round's confrontations |
| **Police Band** | a node you're not Hidden to (Rumoured or Known) | Immediately; sight lasts the round |
| **Decoy** | a Known node | With the trail, at the end of the round |
| **Shiv** | — | Armed. Fires on your **first** confrontation this round |
| **Circulation Permit** | — | Armed. Fires on this round's incident and on the gate fee |
| **Bolt Hole** | **a Known destination node, 2 steps away from your position at the start of the round** | Armed. Fires if you lose a confrontation |
| *Muscle* | *not a discard* | *Permanent while held* |

**Immediate discards resolve before movement**, which is what makes Guard Contact worth its Cr$ 6 — dropping three Infamy after the order phase but before the fighting is the whole point of the item.

**Armed discards are spent whether or not they fire.** A Shiv declared in a round with no confrontation is gone. That is the gamble, and it is what stops item declaration from being a free default.

**Bolt Hole now names its destination up front.** v2.4 said "retreat 2 nodes of your choosing", which cannot work under simultaneous orders — there is no moment at which to choose, because resolution does not pause for input. You declare the node when you declare the item, measured 2 steps from **your position at the start of the round** — the one coordinate fixed before that round's routes and confrontations play out — over **your own currently-known subgraph only**, never the full map: the declared node must itself be Known (a Rumoured node carries no edges to route into, §7.1, so it can't be a 2-step path's endpoint), and the distance is walked through edges you know, not ones that happen to exist. If the declared node has since become unreachable by the time the item fires, the ordinary pushback rule (§15) applies instead.

### 9.5 Field 5 · ADD-ONS (optional, no action cost)

Checkboxes on the order form. They resolve in Phase 5 and don't consume your Action.

| Add-on | Cost | Effect |
|---|---|---|
| **Buy the Ledger** | Cr$ 3 | Learn every player's exact balance **as of the end of last round**. Private purchase. **Unavailable in the final round.** |
| **Renew lease** | per §10.4 | Extend the lease on any post you hold, remotely. |

---

## 10. Territory: posts and leases

### 10.1 What a post does
- **Sight** of its node — and only its node — for as long as the lease runs. (Changed in v1.1; see §7.2 for the saturation maths that forced it.)
- **2 RP** at final scoring, if the lease is still live at the end of round 15.
- Counts toward **sector majority** (+3 RP), again only if live at the end.
- **No income.** Tolls are gone (see below).

### 10.2 Tolls are removed
In v0.9, entering an enemy post while carrying cargo cost Cr$ 2. That's out. The economy is now clean and one-directional:

> **Money comes in from contracts. Money goes out through leases, items, stakes, gate fees, penalties, and the police.**

The reason is that a Cr$ 2 toll was never going to justify a post's cost, and pretending otherwise muddied what posts are actually for. A post is a **camera**, not a tollbooth. That reads more clearly, removes a whole class of fiddly resolution edge cases, and removes the one remaining mechanism by which the leader passively taxed everyone else.

### 10.3 The cap

The cap scales inversely with player count, so total territory pressure on the map stays roughly constant:

| Players | Nodes | Post cap per player | Max posts on the map |
|---|---|---|---|
| 2 | 15 | **4** | 8 (53% of nodes) |
| 3 | 22 | **4** | 12 (55%) |
| 4 | 25 | **4** | 16 (64%) |
| 5 | 28 | **3** | 15 (54%) |

A player at the cap must let a lease expire or abandon a post before staking another.

**Honest note on how much this achieves.** The dynamic cap is cheap insurance, not the fix for map saturation — the lease economy already makes capping out unaffordable for most players, and even three posts each in a two-player game covered 76% of the map under the old adjacency rule. The load-bearing change is node-only sight (§7.2). The cap is here to bound the worst case, particularly at 4 players where nodes-per-player is lowest.

### 10.4 The lease

Posts are not bought. They are **rented**, and you declare the duration up front.

| Blocks purchased | Rounds held | Cost |
|---|---|---|
| 1 | 3 rounds | Cr$ 3 |
| 2 | 6 rounds | Cr$ 6 |
| 3 | 9 rounds | Cr$ 9 |
| 4 | 12 rounds | Cr$ 12 |

- Maximum 4 blocks held at once on a single post. Renewals may top a post back up to 12 remaining rounds.
- **Renewal is remote** (order add-on, §9.5) — you never have to travel back to pay rent. Travelling to collect rent is not interesting gameplay.
- Expiry is telegraphed: the HUD warns 2 rounds out, and a **"The corner went quiet"** trace fires publicly on expiry, telling everyone the tripwire just came down.
- **Abandoning** a post early refunds nothing.

**A note on the rate.** The proposal on the table was Cr$ 1 per 3 rounds. Running the numbers against a 15-round match: a post bought in round 2 and held to the end would cost Cr$ 5 and return 2 RP, against a cash-to-RP rate of 4:1. That makes a post strictly better than holding cash, so every player buys the cap immediately and territory stops being a decision. At **Cr$ 1 per round** (the table above), holding a post from round 2 through round 15 is 14 rounds — Cr$ 14, with one renewal along the way since 4 blocks caps at 12 — for those same 2 RP — clearly a loss on points, which is correct, because **you're buying the vision and the sector majority, not the two points**.

**This rate is the single most sensitive dial in the game.** It's the one number I'd expect paper testing to move first, and the code should treat it as configuration, not a constant.

---

## 11. Infamy

Infamy is simultaneously an **access key** (contract tiers), a **throughput multiplier** (offer frequency), **combat weight**, and **exposure** (visibility, mobility, police attention). It is the only progression axis in the game, and every one of its four tiers trades something real for something real.

### The ladder

| Range | Title | Combat | Contracts | Cooldown | Steps | Exposure |
|---|---|---|---|---|---|---|
| **0–2** | Nobody | +0 | I | 4 rounds | 4 | Pickups anonymous. **Immune** to events targeting "most infamous". |
| **3–5** | Known | +1 | I–II | 3 rounds | 4 | Pickups carry your name in the trail. |
| **6–8** | Feared | +2 | I–III | 2 rounds | 3 | Purchases also show. **Position revealed to all at end of each round.** |
| **9–10** | Legend of Cinzal | +3 | I–IV | 1 round | 2 | **Position public during the entire order phase.** Always the police's first target. Takes **Pressure** every round (§14.3). |

### Gains and losses

| Action | Δ Infamy |
|---|---|
| Deliver tier I or II | +1 |
| Deliver tier III or IV | +2 |
| Win a confrontation | +2 |
| Stake your first post in a sector | +1 |
| Lose a confrontation | −1 |
| **Vanish** action | −2 |
| Amnesty, incidents, events | varies |

### 11.1 Precedence: when tier effects are evaluated

**Infamy tier effects are evaluated at the moment they fire, using your Infamy at that moment.** Not at round start, not at round end.

Concretely:
- The **order-phase position broadcast** (Legend) uses your Infamy as the order phase opens.
- The **end-of-round position reveal** (Feared and Legend) uses your Infamy after resolution has finished.
- **Contract tier gating** and the **Contact Cooldown** use your Infamy at Phase 2, when the offer is generated.
- **Confrontation bonuses** use your Infamy at the instant the confrontation resolves — so winning a fight early in your route makes you stronger in a second fight later in the same route.

**The Vanish case, worked.** A Legend at Infamy 9 broadcasts their position through the order phase — that has already happened before they act. During resolution they Vanish, dropping to 7. Two separate things follow, and they don't conflict because they run on **different channels**: Vanish suppresses the *"fresh tracks" trail entry* they would have left, while the tier broadcast is a *direct position readout* driven by their Infamy. Vanish never touched the second one.

At 7 they are Feared, so the end-of-round reveal still fires. To get out from under position reveals entirely, a Legend must Vanish **twice** — 9 → 7 → 5 — spending two full rounds of actions. That is expensive, legible, and reachable, which is the right shape. A Legend who does it stops broadcasting during the *next* order phase, because by then their Infamy is 7.

**No special-case rule was added for this.** A ruling that "Vanish suppresses the Legend broadcast" would fix one collision and leave every future one open. The general precedence rule fixes the whole class.

### The shape of the curve

Read the ladder as two opposing gradients. Going up, you gain: contract tiers worth 4× the payout, offers four times as often, and +3 in a fight. Going up, you lose: half your mobility, your anonymity in the trail, your position on the board, and immunity from the Guard.

The result is that the two ends of the ladder play differently rather than one being better:

- A **Nobody** is fast and invisible but starved of work — four offers a match, all tier I, for a ceiling of **8 RP from contracts**. This is not a winning line and is not meant to be. Infamy 0–2 is a **starting state and a recovery state**, not a strategy: you pass through it in the opening rounds and you return to it deliberately after a Raid or a bad run, to get your speed and anonymity back while you rebuild. *(v1.0 claimed a Nobody "wins by never being seen". That was wrong — see §24.2.)*
- A **Legend** has a firehose of tier IV work and wins fights on sight, but crawls at 2 steps a round with their position painted on the board, taking Pressure rolls every round.
- The **Feared** middle is where most competitive play should land, and if playtesting shows one end dominating, the dials are the cooldown table and the step table — not the payout table.

---

## 12. The Black Market

Each Black Market shows **3 distinct rolled items** — never a repeat within the same market's stock — refreshing on **odd rounds: 1, 3, 5, …, 15** ([D25](../decisions/D25-item-market-resolution-gaps.md)). You see the stock if you have sight of the node. Buying requires being there (Deal action). Items in hand are **hidden**.

| Item | Price | Effect |
|---|---|---|
| **Shiv** | Cr$ 4 | Discard: +3 in one confrontation. |
| **Muscle** | Cr$ 7 | Permanent: +1 in all confrontations. Lost when you lose a confrontation. |
| **Police Band** | Cr$ 3 | Discard: full sight of one node of your choice this round — Rumoured or Known, never Hidden. |
| **Circulation Permit** | Cr$ 5 | Discard: immune to this round's **Sector Incident** and to the gate fee. |
| **Torn Map** | Cr$ 3 | Discard: reveal 4 random Hidden nodes as Known. |
| **Decoy** | Cr$ 5 | Discard: plant a false "Cargo left here" trace on any Known node. |
| **Bolt Hole** | Cr$ 5 | Discard: on losing a confrontation, keep your cargo and retreat to a node **declared in advance** (§9.4), 2 steps away. |
| **Guard Contact** | Cr$ 6 | Discard: −3 Infamy immediately, without spending your action. |

Hand limit **3 items**. Items are worth no RP — credits parked in inventory are credits that never became score. That keeps the market a tactical instrument rather than a savings account.

---

## 13. Debt

If you owe money you don't have — a stake, a gate fee, a penalty, a lease renewal:

1. You pay what you have. Balance floors at **Cr$ 0**.
2. **−1 Infamy.**
3. If you hold posts, the game surrenders **one lease** (the one with fewest rounds remaining) and credits **Cr$ 2** against the debt.
4. Any remainder is **forgiven**, and you are **Flagged**: **−1 step** next round.

**Flagged is a boolean, not a counter.** Two forgiven debts in one resolution — losing two shakedowns in a free-for-all, say, or a shakedown plus a failed lease renewal — still produce one flag and one lost step. A player who has already hit zero has nothing left to take, and stacking immobilisation on top of bankruptcy is the death spiral P4 exists to prevent. A fresh debt while already Flagged simply refreshes it for the following round; it does not extend or deepen it.

**No negative balances. No elimination, ever.** A broke player still plays; they just play worse for a round. This is P4 in its most literal form.

---

## 14. Events, incidents, and pressure

The game applies pressure at three scales: **global** (everyone, everywhere), **local** (one sector), and **personal** (Legends only). All three are telegraphed before orders and resolved after.

Not all of it is pressure. Roughly a third of the deck helps rather than hurts — a spilled load, a fence paying over the odds, a guard who wants his cut and takes less today. A game made only of punishment reads as grim and, more practically, gives players nothing to hope for when they open the app on day three of an asynchronous match.

### 14.0 The Boon Rule

Beneficial randomness is where P2 is easiest to violate. "You find Cr$ 5 in an alley" is precisely the kind of luck the pillar exists to forbid: it lands after the decision, it rewards nobody's choice, and — because cash converts to RP at 4:1 — it is **free score handed to a random player**.

So every beneficial event in this game must satisfy at least one of three conditions:

1. **Public and contestable.** The boon appears on the board and anyone may go take it. A crate at a named node isn't a gift, it's a race — and a race is an encounter waiting to happen.
2. **Earned by a decision already made.** Scavenging pays out because you spent a step walking into the dark. Sector boons pay out because you chose to enter a sector flagged as unstable. The randomness resolves after the choice, but the choice is what put you in range of it.
3. **Targeted at whoever is behind.** A boon that finds the trailing player is a comeback valve, not a lottery.

And a preference, not a rule: **boons should pay in tempo, position, information, or opportunity before they pay in cash.**

*(v1.2 broke its own preference twice, within a page of stating it: **Cash Drop** handed Cr$ 5 to anyone standing in a sector, and **Insurance Payout** handed Cr$ 4 to anyone between jobs. Both were untargeted, uncontested cash — which is to say free RP at 4:1 requiring no subsequent decision. Both replaced in v1.3 by boons paying in sight and in tempo. A stated preference that the deck ignores is decoration, and worse, it teaches the next person editing the deck that the rule is optional.)*

An extra step next round, a free contract offer, a revealed node, a half-price item — all of these have to be converted into score by playing well. Credits convert themselves.

### 14.1 The Headline

At the start of each round, before orders, the game publishes two facts:

```text
ROUND 7 · HEADLINE

  "THE GUARD PROMISES A RESPONSE"
  ── global event category: POLICE ──

  UNSTABLE SECTOR: NORTH VALE
  ── something is going to happen there ──
```

You learn the **category** of the global event and the **sector** that will take an incident — but never which card. This is the P2 pattern applied twice: enough information to plan against a distribution, not enough to plan against a certainty.

### 14.2 Global event deck

**24 cards, 6 per category. 12 are drawn for the match at setup — exactly 3 from each category.**

**Global events run in rounds 4 through 15** — twelve rounds, twelve cards. Rounds 1–3 have no global event.

This is not padding to make the arithmetic work; the opening needs it. Nobody has a contract in hand on round 1, half the map is dark, and the first delivery lands around round 4 or 5 (§5). Dropping a Currency Slide on a table whose players are still finding their first warehouse punishes position rather than decision. The quiet spell gives everyone three rounds to establish, and it stacks with the incident schedule to produce a pressure ramp:

| Rounds | Global event | Sector incident |
|---|---|---|
| 1–2 | — | — |
| 3 | — | **yes** |
| 4–15 | **yes** | **yes** |

The Headline (§14.1) prints only what is live: nothing in rounds 1–2, the unstable sector alone in round 3, both from round 4.

**The resolved-card log is fully public and permanent.** Every card that fires stays visible in the Board for the rest of the match, with its round.

That is a deliberate choice against the obvious alternative, which is hiding resolved cards to stop players counting the deck. Hiding information the players already witnessed is an anti-feature: it punishes attention, rewards nobody, and pushes note-taking outside the app — the same failure the Board exists to prevent (§7.5). If a design's uncertainty depends on players forgetting things, it doesn't have uncertainty.

**What actually protects the deck is the draw, and it was measured.** With the Headline announcing the next card's category, the question is how many cards the next event could be. Across 20,000 simulated matches:

| Draw rule | Round 10 | Final round | Worst case observed |
|---|---|---|---|
| Free 12-of-24 | 4.1 candidates | 3.6 | **1** — fully solvable |
| **3 per category** | 4.4 candidates | **4.0** | **4** |

A free draw is *usually* fine and *occasionally* collapses: a category can be over-represented in the match deck, get exhausted, and hand a late-game player a certainty. The 3-per-category constraint makes that impossible — at most 3 of any category's 6 cards can ever fire, so the final round always has at least 3 unseen candidates no matter how carefully anyone counted.

It also guarantees a balanced spread of [D], [C] and [O] across every match, which stops a run of pure punishment or pure gifts from happening by accident.

Each card carries a tag: **[D]** damage, **[C]** convergence, **[O]** opportunity. Convergence cards are the ones that matter most structurally — they pull players toward the same nodes, which is the only honest way an event can manufacture interaction. A card that merely subtracts Cr$ 5 produces bookkeeping; a card that puts something valuable on a named node produces a confrontation.

Target mix per category: roughly 2–3 [D], 1–2 [C], 2 [O].

**POLICE**
| Card | Tag | Effect |
|---|---|---|
| **Raid** | [D] | The highest-Infamy player loses carried cargo and **−2 Infamy**. Ties: all tied players. |
| **Curfew** | [D] | **−1 step** for everyone this round, applied retroactively. |
| **Gates Closed** | [D] | Deliveries this round pay half. RP unaffected. |
| **Payroll Day** | [D] | Every player pays **Cr$ 1 per post held**. The Guard wants its cut of your territory. |
| **Dragnet** | [C] | Two random Borders are sealed this round. Every delivery must route to the ones that remain. |
| **Shift Change** | [O] | No Pressure roll this round, and everyone takes **−1 Infamy**. |

**ECONOMY**
| Card | Tag | Effect |
|---|---|---|
| **Currency Slide** | [D] | Everyone loses **25%** of their balance, rounded down. |
| **Market Surge** | [O] | Each player's next delivery pays **+50%**. |
| **Permit Auction** | [O] | Lease blocks cost **Cr$ 2** instead of 3, next round only. |
| **Retainer** | [O] | Every player carrying no cargo gains **+2 steps** next round. Relief for anyone between jobs, paid in tempo. |
| **Fence's Windfall** | [C][O] | One random Black Market, announced publicly, buys **any** cargo outright for **Cr$ 12** — no contract needed. First arrival only. |
| **Shipping Boom** | [C] | One random Warehouse is overloaded. Anyone picking up there this round takes **+Cr$ 5**. |

**UNDERWORLD**
| Card | Tag | Effect |
|---|---|---|
| **Informants** | [D] | Every player's current position is revealed to everyone. |
| **Amnesty** | [O] | Everyone **−3 Infamy**, floor 0. |
| **New Boss** | [O] | The lowest-RP player takes **Cr$ 10** and **+1 Infamy**. |
| **Old Favour** | [O] | Every player immediately receives a contract offer, ignoring the Contact Cooldown. |
| **Dead Runner** | [C][O] | A crate appears at a random node, announced publicly. Anyone may pick it up and deliver it to **any** Border for **Cr$ 12 and 3 RP**. |
| **Bounty** | [C] | The highest-RP player has a price on their head. Whoever defeats them in a confrontation this round takes **Cr$ 10** from the bank. |

**CITY**
| Card | Tag | Effect |
|---|---|---|
| **Blackout** | [D] | Next round nobody generates trail entries and nobody has sight beyond their own node. |
| **Bridge Down** | [D] | One random edge is destroyed **permanently**. The map is now different. |
| **Rain** | [D] | No "fresh tracks" entries are recorded anywhere this round. The weather washes the board clean. |
| **Dockers' Strike** | [D] | No Pickup action may be performed next round. |
| **Festival** | [C] | One random node draws a crowd. Anyone ending there this round gains **+1 Infamy** and leaves **no trace**. |
| **Scaffolding** | [O] | One random sector: every player inside it gains **+1 step** next round. |

### 14.3 Sector Incidents

From **round 3 onward**, one sector is flagged **Unstable** in the Headline. At Phase 7, one Incident card resolves and hits **every player whose route ended inside that sector**. The same sector cannot be flagged two rounds running.

**16 cards: 11 hazards and 5 boons. 13 are drawn for the match — exactly 4 boons and 9 hazards.**

The remaining composition is **always displayed** — "6 hazards, 3 boons left" — because that is what turns the flag into a priced gamble rather than a coin flip. The fixed 4/9 split is what makes that counter trustworthy: without it a match could draw one boon or all five, and the ~31% figure the design leans on would be a description of the deck rather than of any actual match.

**This is the point of the boons.** In v1.1 an Unstable Sector flag was a one-way signal: stay out. That is not a decision, it is an instruction, and it made a third of the map dead every round. At roughly 31% boons, entering a flagged sector becomes a real risk/reward call — and, because everyone is weighing the same call about the same sector, flagged sectors become places players converge on rather than uniformly abandon. The boons do double duty as an encounter mechanism.

**HAZARDS (11)**
| Incident | Effect on players ending in the unstable sector |
|---|---|
| **Flood** | Carried cargo drops at your node. Retreat 1 step toward the sector edge. |
| **Snatch Job** | Lose **Cr$ 6** and your cargo (gone, not dropped). Dumped at a random node in a different sector. |
| **Guard Sweep** | Lose **Cr$ 5** and **−1 Infamy**. Carrying cargo, you lose it too. |
| **Torched** | Every post in the sector loses **3 rounds** of lease, present or not. |
| **Turf War** | Roll D6 + your confrontation modifiers against a flat **9**. Lose: drop cargo, retreat 1 step. |
| **Streets Blocked** | Your route next round is capped at **1 step**. |
| **Gas Leak** | Nobody may end their route in this sector — routes truncate at the last node outside it, resolved during movement. |
| **Riot** | Every trail entry generated in this sector this round is randomized. Names, events, all of it. |
| **Sinkhole** | One random node in the sector is impassable for **3 rounds**. |
| **Shakedown** | Every player ending here pays **Cr$ 4**. |
| **Informant Ring** | Every player ending here has their position revealed publicly. |

**Riot, precisely (D4).** "Randomized" means *permuted*, not invented. Only the round's **sight-gated** trail entries generated in the flagged sector move — cargo taken, fresh tracks, confrontations, item purchases. Deliveries, post stakings, lease expiries, and Loitering/loose-crate entries are **global announcements of real game state**, not fog information, and Riot leaves them exactly where they happened. Among the entries that do move, only their **node assignment** is shuffled — the entry's contents (whether it's named, and whose name) travel with it unchanged, so Riot never speaks a name that wasn't already in the sector this round, and the usual name gates (Infamy ≥ 3 for cargo, ≥ 6 for a purchase) are checked against the real actor exactly as they always are. Who can read a moved entry follows the node it moved *to*, exactly like any other trail entry — you still need sight of that node, but you no longer need sight of where it truly happened, and someone who only had sight of the true node loses it. That is the mechanic working, not a leak: sight of a node already entitles you to whatever is truly logged there, Riot only changes which real entry that is. A player whose own trace was moved is told so — not where it went, not what now sits at the node they actually stood on — because they already know what they did; withholding that would be lying to them about their own round, and telling them more would leak someone else's. Anyone reading the sector's log this round already knows Riot fired — it's the Unstable flag's own incident, announced in the Headline — so that much is never a new disclosure; which specific entry they find at a node they can already see may be.

**BOONS (5)**
| Incident | Effect on players ending in the unstable sector |
|---|---|
| **Spilled Load** | A crate appears at a random node in the sector. Deliverable to **any** Border for **Cr$ 10 and 2 RP**. Announced publicly — anyone may go for it. |
| **Local Informant** | Every player ending here gains **sight of every node within 2 steps** of wherever they end their route next round. |
| **Distracted Guard** | Every player ending here gains **+1 step** next round and leaves **no trace** this round. |
| **Open Doors** | Every player ending here may immediately buy one item at **half price**, without being at a Black Market. |
| **Word of Work** | Every player ending here immediately receives a contract offer, ignoring the Contact Cooldown. |

**Frequency check.** Rounds 3 through 15 is 13 incidents across 4 sectors — about three visits each. Enough that a player crossing North Vale repeatedly will eat one; not so much that routes become unplannable. If paper testing says the map feels chaotic, the dials in order are: start at round 5; odd rounds only; raise the boon share above 31%.

### 14.4 Pressure

If you're a **Legend of Cinzal** (Infamy 9–10), at the end of each round you roll 1D6. On **1 or 2**, the Guard finds you: **−Cr$ 5** and **−1 Infamy**.

This isn't arbitrary. It's the published price of the top of the ladder, and the player who climbed there did it with the number in front of them.

---

## 15. Resolution

Resolution is **deterministic given the set of orders plus the seed**. This is non-negotiable — replay, debugging, dispute handling, and async trust all depend on it.

### The sequence

### 15.0 Order legality

The degradation rule only means something once "invalid" is defined. Two categories, handled differently.

**Illegal payloads — the client must prevent these, and the server must reject them.** These are never the result of the world changing; they are malformed orders, and in a client-authoritative-looking UI they are also the obvious attack surface.

| Illegal payload | Server response |
|---|---|
| A step between non-adjacent nodes | Reject order, apply the absence default (§18) |
| A step *out of* a Hidden node without Pushing On declared | Reject |
| Route longer than your step allowance (§9.1) | Reject |
| More than one Hidden node, or a Hidden node that isn't last | Reject |
| An action illegal at the ending node's type | Reject |
| Pushing On combined with an action | Reject |
| Add-ons or a lease exceeding your balance at submission | Reject |
| Staking beyond your post cap (§10.3) | Reject |
| A route plotted through nodes not Known **to that player** | Reject — and log it, because a well-formed client cannot produce this |

Rejection falls back to the absence default rather than to a partial execution, because a malformed payload means the client's state and the server's state disagree, and guessing the player's intent from a broken order is worse than doing nothing.

**Step 0 — Degradation.** This is the other category: the order was legal when submitted and the world moved underneath it. The target node got staked by someone else, a Bridge Down destroyed an edge, funds went to a stake you lost. Here the game **degrades** rather than rejects: the route truncates at the last valid step and the action becomes **Nothing**. The player is notified with the specific reason. Orders never silently fail — this holds at every resolution step, not only here at Step 0. Whether it's the target that moved (a Deal that loses the market race, a Pickup that loses a dropped crate) or the player's own state that stopped covering it (a Stake Post already at cap, a Ledger purchase the balance no longer covers), any action a later step discovers can no longer complete still names the specific reason to the player who declared it (D30). The one standing exception is a pre-declared, conditionally-triggered boon like Open Doors, whose trigger genuinely may never fire at all — that isn't a failed action, it's an unmet condition (D14 §4).

**Steps 1 to N — Synchronized movement.** Everyone takes their first step; then everyone takes their second; and so on to the longest route on the table. After **each** step:

  a. **Crossing.** If two players traverse the **same edge in opposite directions** in the same step, they meet mid-edge. Confrontation resolves at the origin node of whichever player is Aggressive; if neither or both, at the lower-indexed node.

  b. **Collision.** Two or more players in the **same node** at the end of a step: confrontation.

**Step N+1 — Actions.** Resolved in **ascending order of Infamy** — the anonymous get there first, the famous arrive with an entourage. Ties break by **lower balance**, then **lower RP**, then a **per-round seeded roll**.

*(v1.2 ended this chain at "seat order". The problem with seat order isn't that it's arbitrary — any tie-break chain needs a final fallback, and at the fourth level of tie the situation is genuinely symmetric. The problem is that it's **static**: the same player wins every contested tie for the whole match, which is arbitrariness that compounds. A per-round seeded roll is equally deterministic for replay purposes and doesn't accumulate. The RP step was added ahead of it so the comeback philosophy holds at one more level before the coin comes out.)*

**Step N+2 — Deliveries and announcements.** Payments, Infamy, RP, global announcements.

**Step N+3 — Add-ons.** The Ledger resolves; lease renewals are applied.

**The pushback rule.** A loser falls back **along the route they actually traversed this round**, one node (two if Evasive), and this is bounded at every edge:

| Situation | Where they end up |
|---|---|
| Traversed enough nodes | Back along the route, 1 or 2 nodes |
| Traversed only 1 node, pushed 2 | **Stops at their starting node.** The route is exhausted, not extrapolated. |
| **Traversed nothing** — was stationary and got attacked | Pushed to a **random adjacent node**, seeded per §21. **If Evasive: a second random hop**, which must not re-enter the confrontation node and must not immediately backtrack where an alternative exists. |
| Any hop has no legal destination | The loop **stops early**. A boxed-in loser is displaced as far as the graph allows and no further. Evasive cargo rules still apply. |
| The destination holds an **uninvolved third player** | The push happens anyway. **No secondary confrontation.** See below. |

The two-hop rule for a stationary Evasive loser needs the "must not re-enter the confrontation node" clause specifically, or the walk can go C → D → C and deposit the camper back on the contested chokepoint — turning a penalty into a free round of holding position. Without it the rule is worse than no rule.

The stationary case is the one that matters: it is the only one with no route array to walk backwards through, it is **not rare** — it's exactly what happens to a camper who loses a fight on their own chokepoint (§9.1) — and left undefined it is an index-out-of-bounds waiting for the first playtest. Note also that it is the *only* branch introducing randomness, which is why it needs a seeded index rather than whatever the language's default RNG hands back.

**Displacement never cascades.** A player pushed into a node occupied by someone else does **not** trigger a second confrontation. Two or more players may share a node at the end of a round when at least one of them arrived there by pushback, and they see each other — players in the same node are always mutually visible.

The reason is that the alternative is unbounded. B loses to A and is pushed into C; B loses to C and is pushed into D; D was mid-route, and the whole resolution step becomes a recursion with no natural termination and no fair ordering. Confrontations are evaluated **once per movement step**, on the positions produced by that step, and displacement is a consequence of resolution rather than a movement of its own.

**Cohabitation still doesn't survive the night — but nothing special is needed to end it.** v1.9 added a pre-movement collision check at the start of the following round, and that was a mistake: a player pushed into an enemy's node on round 4, who then submits a four-step route to run on round 5, would lose that fight *before taking a step* and forfeit their entire round. Punished twice for one displacement, with no way to act on the information.

The check was also unnecessary. The **collision rule already evaluates every player's position after each step, whether or not they moved** — a player standing still is at a node just as much as a player who walked there. So the cases resolve themselves:

| Round 5 orders | Outcome |
|---|---|
| Both plot routes leaving in different directions | They separate after step 1. No fight. Two factions can decide not to have it out. |
| Both stay put | Both are still co-located after step 1. **Fight.** |
| One leaves, one stays | The leaver is gone after step 1. No fight, and nobody can pin anybody. |

Cohabitation therefore lasts exactly as long as both parties want it to, which is the right answer, and the fleeing player gets to use the round they paid for.

*(One boundary case: if every player on the table submits an empty route, no movement step occurs at all. The collision evaluation runs **at least once per round** regardless, so a shared node still resolves.)*

**Displacement rule.** Wherever you **actually end the round** is your ending node for the purposes of sight (§7.2), Loitering (§9.1), and the trail — however you got there. This covers confrontation fallbacks, Snatch Job relocation, and Gas Leak truncation with one line rather than three special cases.

So a player who plotted four steps, was ambushed on step one, and fell back to where they started gets sight of **that** node and nothing else. They do not gain intelligence on a route they were physically prevented from walking. They keep the map **knowledge** of nodes they did traverse before being stopped, per §7.2 — Known is not sight.

They also learn about the fight regardless, because a confrontation writes a public trace naming both parties at that node. You never need sight of a place you bled in.

**Step N+4 — Trail written.** Each node's log closes and is distributed according to each player's sight.

### The confrontation

```text
TOTAL = D6
      + Infamy tier bonus            (+0 / +1 / +2 / +3)
      + stance modifier              (+1 / 0 / −1)
      + ambush                       (+1 if Aggressive in an Alley)
      + items
      + stake ÷ 3                    (rounded down, capped at +2)
      + underestimated               (+1 if your Infamy is the LOWEST present)
```

**Highest total wins.** On a **tie**, nobody wins: everyone falls back to the node they came from, nobody loses cargo, stakes are returned. Ties are anticlimactic by design — it makes speculative aggression a worse bet.

**Winner**
- **+2 Infamy**
- Takes **half** of each loser's stake, rounded down
- **May** take the **cargo** of one loser of their choice, unless that loser was Evasive and paid the shakedown. Taking is optional and requires an **empty cargo slot**; a winner already carrying something cannot take more. Refused or impossible, the crate **falls at the node**, still bound to its original origin/destination pair (§8.4) — it does not become a loose crate
- A winner may take cargo they hold **no matching contract for**. They cannot deliver it, but the owner cannot either, which is frequently the point. The price is their own cargo slot
- Holds the node; their route continues

**Loser**
- Loses their stake
- Loses their cargo — **unless Evasive and able to pay** the **Cr$ 4 shakedown** to the winner. The shakedown is **capped at their current balance** and **never triggers Debt** (§13); if they cannot pay the full Cr$ 4, the winner takes whatever was in their pockets **and the cargo is forfeit on the same terms as any other loss** — claimed by the winner if they want it and can carry it, otherwise dropped at the node
- **−1 Infamy**
- Falls back **one node** (**two** if Evasive) per the pushback rule below, and **loses the remainder of their route and their action**
- If Evasive: **−1 step next round**
- **Deadline Pause**: if they were carrying cargo, the deadline on that contract extends by **1 round**. Once per contract, whether or not the cargo was kept.
- **Keeps every post. Loses no credits beyond the stake and the shakedown.**

**Why the Deadline Pause exists.** Work the Legend case. A Legend moves 2 steps a round. A Tier IV contract runs distance 6+ on a 6-round deadline. Reaching the warehouse costs roughly 2 rounds, the haul costs 3, and the delivery lands on round 5 — a margin of **one round**.

Now take one Evasive loss: pushed back 2 nodes, then 1 step instead of 2 the following round. That is roughly three nodes of ground, or a round and a half. The margin was one round. The contract fails, and failure costs **Cr$ 12 and −2 Infamy** on top of the RP that never arrives.

That is a cascade — one lost fight converting into a large, unrecoverable loss — and it is precisely what P4 exists to forbid. The pillar was written about territory and income, and it quietly failed to cover deadlines, which turn out to be the tightest resource on the board. The Pause makes an interception a setback of the size it looks like.

**Why the shakedown is capped, and why failing to pay it costs the cargo.** Under v1.9 a broke courier who went Evasive and lost owed Cr$ 4 they didn't have, which fired the Debt cascade in §13: balance to zero, **−1 Infamy**, and **a lease surrendered**. For a trailing player, losing a chokepoint post and a point of Infamy is far worse than dropping a Tier I cargo would have been — so the courier's insurance policy was strictly worse than not buying it, and worst for exactly the players P4 exists to protect. Evasive was a trap for the poor.

Capping at balance fixes that. But a flat exemption would go too far the other way and hand broke players free insurance, which is a strange thing for a game about paying people off. So: **you can't afford the premium, the policy doesn't pay out.** The winner takes the loose change and the crate, which is the same outcome a Neutral stance would have produced — Evasive costs the poor player nothing extra and gains them nothing, and the real lesson is to keep Cr$ 4 in reserve whenever you're carrying. That is a decision, which is better than an exemption.

**Once per contract**, so nobody can farm losses to extend a deadline indefinitely. It applies whether or not the cargo was kept, because a non-Evasive loser has it worse — their cargo is on the ground and has to be walked back to.

With three or more players in a node it's a free-for-all: highest total wins, everyone else loses.

**Why this shape.** The cost of losing is a wasted round and a cargo — painful, recoverable, and over. It is not the v0.9 original, where a loser paid stake plus fee plus Infamy plus a base. The **underestimated** modifier is the comeback valve: the player everyone has stopped watching gets a real shot at the one who thinks they own the city.

### Upkeep

Phase 8, once per round, after Pressure (§14.4) — including round 15, before final scoring (§16) reads the table. Four steps, in this fixed order:

1. **Contract deadlines.** Decrement every held contract. At zero: pay the penalty (§8.3), discard the contract, drop any cargo held for it, and run Debt (§13) if the penalty can't be paid in full — which may surrender a lease and re-flag the player for next round.
2. **Leases.** Decrement every lease still held, including any that survived step 1's Debt cascade. At zero, the lease expires and **"The corner went quiet"** fires publicly (§10.4) — the same trace whether the lease expired on its own or was surrendered for debt. The district never learns which.
3. **Sinkhole.** Decrement every active Sinkhole (§14.3). At zero, the node is passable again — no announcement; it's read off the map like any other passable node.
4. **Next-round modifiers.** Clear whichever of Streets Blocked, Distracted Guard, Scaffolding, Retainer, Dockers' Strike, or Blackout applied this round.

**Steps 1 and 2 are ordered on purpose, not by convention.** A contract's Debt cascade surrenders "the lease with fewest rounds remaining" (§13) — that has to be read *before* this same phase's own lease decrement touches it, or a lease that would have expired on its own this round is spared while a healthier one gets taken in its place, turning a one-lease penalty into two. Get it backwards and nothing crashes; a rule just quietly stops firing, which is exactly the failure shape the rest of this document keeps naming and testing against directly.

**`Flagged` and the Evasive step penalty are not cleared here at all.** Both are consumed the moment they're read — at the start of the round they apply to, when the step-allowance formula (§9.1a) checks them — not by an Upkeep step at the end of it. That has to be true because both can be set fresh from more than one place in the same round a debt or an Evasive loss occurs: an ordinary Debt trigger during resolution (a stake or gate fee that can't be covered), or this very phase's own step 1. An Upkeep-phase clear, wherever it sat in the order, could not tell "the value this round already used" apart from "a value this round just set for next round" — it would wipe out exactly the refresh §13 describes ("a fresh debt while already Flagged simply refreshes it for the following round") whenever the debt happened to land before the clear ran. Both counters are cleared once, at the moment they're consumed for the round they apply to, which is the only point that can make that distinction.

Two more counters that look like they belong here don't. The Contact Cooldown (§8.2) needs no per-round action — `LastOfferRound` is written once and read as a difference against the current round, so there is nothing to decrement. And the loose-crate heat tick (§8.4) fires earlier, in the same pass that writes the round's trail, because its 2-consecutive-round threshold is evaluated against that round's own entries.

Full reasoning, all four corrections, and both worked examples in [D5](../decisions/D05-upkeep-phase.md).

---

## 16. Scoring and end of match

The match ends at the end of **round 15**. No sudden death, no extension.

| Source | RP |
|---|---|
| Delivered contracts | Tier value — 2 / 3 / 5 / 8 |
| Each post with a live lease at the end | **2 RP** |
| Sector majority of live posts | **+3 RP** per sector; ties score nobody |
| Cash | **1 RP per Cr$ 4**, rounded down |
| Active undelivered contract | **−2 RP** each |

**Tiebreakers:** most contracts delivered → highest Infamy → highest balance.

### Expected spread
Reference simulation, 4 players, 15 rounds:

- Contracts: **2–5 deliveries** per player, tier-dependent ≈ **8–16 RP** *(per the ladder table in §24.2 — deliveries are gated by the Contact Cooldown at the bottom of the ladder and by travel time at the top, so nobody gets six)*
- Posts: 2–4 live at the end ≈ **4–8 RP**
- Sector majority: 0–1 ≈ **0–3 RP**
- Final cash: Cr$ 10–30 ≈ **2–7 RP**
- **Expected total 14–34 RP**, winner typically **26–36**.

Three archetypes should sit inside that band, and balancing chases exactly this:

- **The Courier** — Infamy 3–5, four steps a round, high volume of tier I–II, minimal territory.
- **The Landlord** — few contracts, leases up to the cap on chokepoints (3 to 5 depending on player count, §10.3), wins on posts plus sector majorities plus reading everyone else's trail.
- **The Legend** — sprints to Infamy 9, absorbs Pressure, converts two or three tier IV scores worth 8 RP apiece.

If any of the three is systematically 20% ahead after paper testing, the corrective dials in order of preference are: lease rate → cooldown table → step table → contract payouts. Payouts last, because they're the numbers players feel most.

### 16.1 Variant: Ascension
*Out of v1 scope, kept for discussion.* From round 10, a player with 30+ RP and 10 RP over second place ends the match immediately at that round's end. Adds a climax; costs predictability. Only worth testing once the core is stable.

### 16.2 Variant: the 20-round long game

*Not bookable. Listed because it was, and because the reason it cannot be is worth recording.*

A 20-round table was offered in earlier drafts as an asynchronous option. It does not survive the deck arithmetic:

| | Rounds needing a card | Cards available |
|---|---|---|
| Global events (rounds 4–20) | **17** | 12 drawn, from a pool of 24 |
| Sector incidents (rounds 3–20) | **18** | 13 drawn, from a pool of **16** |

The events row is merely awkward — drawing 17 of 24 means four or five per category, which weakens the counting guarantee §14.2 measured and paid for. The incidents row is impossible: eighteen rounds cannot be served by a sixteen-card pool without reshuffling used cards back in, which breaks the displayed hazard/boon counter that makes the Unstable Sector a priced gamble rather than a coin flip.

Making it work needs a decision — a larger pool, a reshuffle rule with a corrected counter, or event-free rounds late — and none of those should be made before the 15-round game has been balanced. Until then the table length is 15.

---

## 17. Scope

### v1 — required for a playable, testable game
- Map generation with every constraint in §6
- Private fog, sight, trail
- **The Board** (§7.5) — persistent log, anchored attribution, heat map, pins
- Simultaneous orders with a timer
- Synchronized movement with Infamy-scaled step allowance
- Crossing and collision detection, full confrontation
- Contracts I–IV, 3-choose-1, with Contact Cooldown
- Post leases, renewal, expiry, player-count-scaled cap (§10.3)
- Infamy with all four tiers and both gradients
- 8 items
- 24 global events, 12 drawn per match, + Headline
- 16 Sector Incidents, 13 drawn per match, + Unstable Sector telegraph
- Two-player rule set (§6.3)
- **Solo scenario ladder and free play against bots (§19.1)**
- Credit bands and the Ledger
- Scoring and ranking
- Synchronous and asynchronous modes
- Private tables with invite links; no mandatory signup to join
- **Telemetry per §22**

### v1.1
- Standing orders (§18)
- Better autopilot
- Shareable match replay
- Player statistics

### v2 — only after the core is balanced
- **Asymmetric factions.** Early sketches: the Ravens keep 4 steps at every Infamy tier; the Pale Syndicate see the Ledger free once every 3 rounds; Children of the Vale get +2 on "underestimated" instead of +1. Each asymmetry is a new balancing dimension — don't mix it into core tuning.
- Curated fixed maps ("Cinzal Classic")
- 1v1 duel mode with a tightened ruleset
- Faction contracts — secret personal objectives

### Out of scope, probably permanently
- Real-time negotiation chat (kills async)
- Between-match progression (turns a board game into a service)
- 6+ players (the map can't take it without ballooning)

---

## 18. Asynchronous mode

Async isn't a separate mode. It's **the same engine on a different timer**. That's the whole payoff of simultaneous orders.

### Setup
The host picks a **round deadline**: 4h / 12h / 24h / 48h / 72h.

### The cycle
1. The round opens. Everyone is notified — push, email, webhook.
2. Each player submits when they can. **Resubmission is allowed** while the round is open; the last submission stands.
3. When **everyone** has submitted, the round resolves **immediately** — it does not wait out the clock. Active groups play fast even on a 48-hour deadline.
4. If the deadline expires, it resolves with what's there.

### Default order on absence
- **Route:** empty. Stay put.
- **Action:** Deliver, if you're standing on the right border with the right cargo. Otherwise Nothing.
- **Stance:** Evasive.
- **Add-ons:** none. Leases are **not** auto-renewed — but expiry was telegraphed two rounds out, so this is never a surprise.

Deliberately conservative: an absent player never spends money and never loses cargo to a fight. Missing a round should sting, not gut you. Over a multi-day match, everyone misses one.

### Standing orders
A player may register a **two-round plan**. If they don't show, the plan runs instead of the default. Useful for anyone who knows they'll be away. **v1.1.**

### Abandonment
After **two consecutive missed deadlines** the player goes to **Autopilot**: a heuristic AI — pursue the nearest contract objective, Evasive, never buy, renew leases about to lapse — plays for them until they return. They are never removed. Removing a player from a territory game breaks the map and the economy for everyone left.

### The async interface
The entry screen must answer three questions inside three seconds:
1. **Have I submitted?**
2. **What happened while I was gone?** → the **Recap**: a text and short-animation summary of every round since your last visit.
3. **How long do I have?**

The Recap is the single most important feature in async and the easiest to underrate. Without it a player comes back after twenty hours, remembers nothing, and stops opening the tab.

---

## 19. Onboarding

The promise is "easy to learn". That has to be built, not hoped for.

### 19.1 Solo play

Solo against bots is the on-ramp, and it carries more weight here than in most games. Three reasons it is not optional:

- **A real match costs 35 minutes and three other people.** Learning the Infamy ladder by wasting one is expensive for everyone at the table.
- **Asynchronous play has a days-long feedback loop.** A new player needs to see cause and effect in minutes before they can tolerate seeing it in days.
- **It costs almost nothing to build.** The bots exist for the simulation harness regardless; solo is an ordinary match with one human seat.

#### The scenario ladder

Five scenarios, each isolating one system. The player can skip ahead; nothing is locked behind a previous stage.

| # | Scenario | Length | Opponents | Teaches | Deliberately absent |
|---|---|---|---|---|---|
| 1 | **First Run** | 4 rounds, 12 nodes | 1 Drifter | Route, action, stance, one delivery | Leases, incidents, Infamy tiers, items |
| 2 | **The Trail** | 6 rounds, 16 nodes | 1 Runner on a visible loop | Posts as cameras, reading a trace, the Heat Map | Infamy tiers, events |
| 3 | **The Ladder** | 8 rounds, 20 nodes | 2 Runners | Contact Cooldown, tier gating, mobility traded for access | Incidents |
| 4 | **Pressure** | 10 rounds, 25 nodes | 2 Runners | Headline, unstable sectors, deck counters, reading a telegraphed risk | — |
| 5 | **Full Match** | 15 rounds, 25 nodes | 3 Runners | Everything, scored normally | — |

Then **free play**: any city, any bot tier, any seat count, no scenario rails.

#### Difficulty is bot tier, never bot bonuses

The three tiers are Drifter, Runner and Operator. A harder opponent plays *better* — it does not roll extra dice, see through fog, or start with more credits.

This is not purism. A bot with hidden advantages teaches the player wrong lessons about what works, and those lessons transfer straight into multiplayer where nobody has advantages. It also destroys the one property that makes solo useful as a testbed: that a strategy which beats a bot is a strategy that might beat a person.

**If Operator beats competent humans consistently, that is a balance finding, not a difficulty achievement**, and it goes into telemetry rather than into a patch note.

#### What solo cannot teach

Worth stating plainly rather than overselling the mode: solo teaches the **machine**, not the **game**. Bots do not bluff, do not hold grudges, do not notice you took the same route twice and start waiting for you. The deduction layer — the thing P3 exists for — only comes alive against people who are also deducing.

So solo's job is to get a player to the point where a real match is legible, and then get out of the way. The scenario ladder should end by saying so.

#### Scoring

Solo results are recorded and shown to the player, and are **excluded from any competitive standing**. They are also tagged separately in telemetry (§22), because bot-opponent data would otherwise contaminate the human-versus-human balance numbers that the open questions depend on.

### 19.2 In-match onboarding

**First real match.** Contextual tips at six moments: first contract offer, first trail entry read, first confrontation, crossing Infamy 3, first Unstable Sector headline, first lease expiry warning. Then silence.

**Reference panel.** Permanently accessible: contract table, Infamy ladder (both gradients), confrontation formula, lease rates. Nobody should be memorizing numbers.

**HUD invariants.** Three things must be on screen at all times, because all three are new sources of confusion in v1.0: **rounds until next contract offer**, **your current step allowance**, and **rounds remaining on each lease**.

**Acceptance test.** A new player, no tutorial, watching over a friend's shoulder, should be able to explain what's happening after two minutes of observation.

---

## 20. Open items

Resolved from v0.9: R3, R4, R5, Q1, Q2, Q3, Q4 — all folded into the rules above.

Still open, and deliberately deferred to instrumentation rather than argument:

**R1 — Cancelled routes may frustrate.** You plan four steps, take a confrontation on step one, and lose everything after it. If that's common, the game reads as a lottery. v1 ships the simple rule; §22 measures it. **Threshold: if more than 15% of submitted routes are cancelled mid-route, the confrontation rule gets softened** — most likely by letting the loser continue their route from the fallback node with remaining steps intact.

**R2 — RESOLVED, and it was wrong.** v1.0 and v1.1 both worried that confrontations would be too rare. Simulated under random movement across 3,000 matches per configuration, against the target band of 4–8 (M2 re-runs this and every other threshold at 10,000 per configuration, with an interval and a verdict rule — §22 and [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md)):

| Setup | Encounters per match | Matches under 3 |
|---|---|---|
| 2p / 19 nodes / 4 steps | **4.5** | **27%** |
| 3p / 22 nodes / 4 steps | 11.3 | 1% |
| 4p / 25 nodes / 4 steps | **19.5** | 0% |
| 5p / 28 nodes / 3 steps | **21.2** | 0% |

The worry was intuition, not arithmetic. At 3–5 players encounters are abundant — at four players, roughly 2.5× the top of the intended band. The caveat is that this models random walks: real players actively avoid each other, which pulls the number down, while contracts pull everyone toward the same warehouses and borders, which pushes it up. The order of magnitude should hold.

**Two consequences.** The real risk is the opposite one (R9), and the only player count with a genuine sparsity problem is **two** (§6.3).

**R6 — Incidents may overwhelm.** Thirteen incidents in a 15-round match is a lot of chaos to layer on top of a global event every round. Watch for players reporting that routes feel unplannable. Dials in order: start at round 5; odd rounds only; cut the deck to the six least punishing.

**R7 — The step gradient may be too steep, and the margin is now measured.** A Legend on a Tier IV run has roughly **one round of slack** against a 6-round deadline: about 2 rounds to reach the warehouse, about 3 for a distance-6 haul at 2 steps a round. The Deadline Pause (§15) buys a second round when an interception happens, which is what stops a single lost fight from guaranteeing failure — but the underlying margin is still thin enough that two bad rounds end the contract.

Watch for Tier IV contracts being accepted and then abandoned. If nobody voluntarily crosses Infamy 8, the dials are: extend the Tier IV deadline to 7 rounds; raise Tier IV to Cr$ 35 / 10 RP; or raise Legend to 3 steps. Try them in that order — the step-table change is the coupled one, because it moves every deadline in §8.3 with it.

**R8 — The 2-contract slot cap barely binds.** With one contract per offer and cooldowns of 3–4 rounds, a Nobody or a Known can almost never hold two at once — the cap only bites at Feared and Legend, where offers arrive every 1–2 rounds. It functions as a safety rail at the top of the ladder rather than a constraint players feel, and the document should stop presenting it as a general limit. Harmless as written; worth knowing it does less than it looks like it does.

**R9 — Encounters may be far too frequent at 4–5 players.** If you collide with someone twenty times in fifteen rounds, you never need to read the trail — you just walk. That kills the deduction layer as thoroughly as sparsity would, but from the other direction, and it is much harder to notice because the match *feels* eventful. **Threshold: above 12 confrontations per match, raise node count before touching anything else.** Watch for the Board going unused (§22) as the leading indicator.

**R10 — Deck expansion may have diluted the memorable cards.** Going from 12 global cards to 24 halves the chance of seeing any given one. Cards that swing a match are what players talk about afterwards; cards nobody remembers are reading time. If post-match recall of "what happened" drops, cut the deck back toward 18 and keep the loudest cards.

**R11 — Endgame camping.** In the last two or three rounds a trailing player can no longer complete a contract, and switching to farming confrontations on a chokepoint becomes rational — Infamy and stakes with no opportunity cost left to pay. Loitering makes it visible, which is most of the fix, but visibility doesn't help the player being kingmade against. Candidate remedies if it shows up: confrontation Infamy gains halved in the final two rounds; or stakes returned rather than transferred once contracts can no longer be delivered. Do not pre-emptively patch this — verify it happens first.

**R12 — The step allowance may be too high for any positional inference.** The saturation table in §7.5 shows a one-round cone at 94% with 4 steps and 40% with 2. If playtesters report that reading the board never pays, the dial is the step table (§9.1), not the trace rules. Dropping to 3/3/2/2 would roughly halve the cone. The cost is contract deadlines, which would all need extending — so this is a coupled change, not a one-line tweak.

---

## 21. Randomness inventory

*Added because the previous draft never stated this in one place: here is every source of randomness in the game, when it fires, and whether it lands before or after the player decides.*

| # | Source | Fires in | Before or after the decision | What the player can do about it |
|---|---|---|---|---|
| 1 | Map generation, including node layout (§6.4, D10) | Setup | Before | Nothing — but it's symmetric and constrained (§6) |
| 2 | Starting positions | Setup | Before | Nothing — minimum distance 4 guaranteed |
| 3 | Contract offer (3 drawn) | Phase 2 | **Before** | Choose 1 of 3, or decline |
| 4 | Market stock (3 items) | Phase 3, odd rounds only (1, 3, …, 15) | **Before** | Choose whether to travel there |
| 5 | Global event **category** | Phase 1 | **Before** | Plan against that category's remaining candidates — never fewer than 3 (§14.2) |
| 6 | Unstable **sector** | Phase 1 | **Before** | Route around it, or accept the risk |
| 7 | Global event **card** | Phase 6 | After | Category was known; ~4 candidates within it (§14.2) |
| 8 | Incident **card** | Phase 7 | After | Sector was known; only hits players who ended there |
| 9 | **Confrontation D6** | Phase 5, per meeting | After | Stance, stake, items, ambush ground |
| 10 | **Pressure D6** (Legends only) | Phase 7 | After | Opt out by not being a Legend |
| 11 | Scavenging roll on a newly explored node | Phase 5 | After | Exploration is always optional |
| 12 | Relocation node (Snatch Job) | Phase 7 | After | Don't end your route in an unstable sector |
| 13 | Blind edge selection during **Pushing On** | Phase 5, per blind step | After | Opt-in entirely; steered by the declared sector bias |
| 14 | Pushback destination for a **stationary** loser | Phase 5 | After | Only reachable by choosing to stand still and fight |
| 15 | Tie-break coin at the fourth level (§15) | Phase 5 | After | Genuinely symmetric situation by that point |
| 16 | **Dragnet's** two sealed Borders | Phase 6 | After | Category (POLICE) was known, not which Borders; a Deliver order already aimed at one that gets sealed degrades rather than fails silently (§15.0) |
| 17 | **Festival's** node | Phase 6 | After | Nothing to avoid — it's a boon (§14.0): whoever's route already ended there gains, no downside for anyone else |
| 18 | **Scaffolding's** sector | Phase 6 | After | Nothing to avoid — a boon; the step bonus only helps players already in the sector |
| 19 | **Shipping Boom's** Warehouse | Phase 6 | After | Nothing to avoid — a boon; only pays whoever already picked up there |
| 20 | **Fence's Windfall's** Black Market | Phase 6 | After | Race to be first arrival once it's announced — a contest, not a certainty (§14.0) |
| 21 | **Sinkhole's** node | Phase 7 | After | Sector was known; route around it, or accept the risk |
| 22 | **Riot's** permutation of this round's trail entries | Phase 7 | After | Sector was known; route around it to keep your own traces out of reach |
| 23 | **Torn Map's** four revealed nodes | Phase 5, immediately before movement | After | Opt-in entirely — buying and using the item is the player's own choice |
| 24 | **Bot decision-making** (`Decide`'s own draws, via `BotRNG`) | Whichever phase produces the bot's order | **Neither** — a separate actor's own decision process, not a random event happening to a player's choice | Nothing directly; bot seats are disclosed in the lobby and HUD (RFC §14.2), and every draw is legible in the RNG trace under a `bot.<tier>.<mechanic>` purpose (D32) |

**Six of twenty-four sources resolve before the player commits, and one — entry 24, bot decision-making — resolves neither before nor after, because it isn't a random event that happens to a player at all: it's a separate actor's own authoring process, on a stream ([D32](../decisions/D32-bot-rng-stream.md)'s `BotRNG`) deliberately outside RFC §6.4's match-consumption table.** Of the seventeen that remain: six are opt-in (9, 10, 11, 13, 14, 23); four are avoidable by routing around the flagged sector (8, 12, 21, 22); four are boons the Boon Rule (§14.0) already keeps fair — earned by a decision already made, or won as an open contest, never a surprise punishment (17, 18, 19, 20) — leaving the global event card draw, Dragnet's Border seal, and the fourth-level tie-break as the only randomness that can reach a player who did nothing to invite it and stands to gain nothing from it (7, 15, 16). That's the P2 audit, and it should be re-run against this table any time a new random element is proposed.

### Dice, specifically

*Corrected in v1.6.* Earlier drafts of this section claimed the game contained "exactly two dice rolls" and that "neither is a movement roll." Both halves were wrong, and the error was in the audit rather than in the design — the table above has listed seeded random *selections* since v0.9. Drawing three contracts from a pool is randomness; so is rolling market stock, drawing an event card, and picking the unstable sector. None of them are dice, and the section had quietly conflated the two words.

The accurate statement is that there are exactly **two player-facing dice rolls**, both D6, both visible in the resolution animation:

1. **The confrontation D6**, rolled during resolution whenever two or more players collide or cross. Both parties roll; modifiers per §15.
2. **The Pressure D6**, rolled in Phase 7, only by players at Infamy 9–10.

*(A third, the Turf War incident, reuses the confrontation formula against a flat 9. Scavenging also rolls a D6, but against a fixed table rather than against an opponent.)*

Everything else in the table is a seeded **selection**, not a roll: the engine picks from a set. That distinction is worth keeping in the vocabulary, because rolls are dramatised in the UI and selections are not, and blurring them is how a design ends up feeling more random than it is.

**Why there is still no movement die** — and why Pushing On isn't one. The v0.9 original rolled a D6 *before* moving to set your movement allowance. That cannot survive simultaneous orders: you would have to submit a route before knowing how many steps you had, or the game would have to sequence players, which destroys P1. That objection was about **allowance**, and it still stands — allowance now comes from Infamy (§9.1), which is a standing consequence of your own choices.

Pushing On randomises **destination**, not allowance, only for players who opted in, only past the edge of the known map, and steered by a sector bias the player declares. It does not touch P1, and it resolves after commitment rather than before it. Different mechanism, different objection, and the objection doesn't apply. But it *is* randomness tied to movement, it belongs in the inventory as entry 13, and the previous wording of this section would have let it slip through unlisted — which is precisely the failure mode an audit exists to prevent.

**Route disruption**, the other half of what the old movement die did, now comes from incidents and confrontations — telegraphed, avoidable, and caused by other players rather than by a die.

### Determinism

All rolls are server-side and derived from `hash(match_seed, round, sequence_index)`. Two consequences the implementation must preserve:

- A match can be replayed exactly from its seed plus the order log.
- No roll may ever be re-derived after a player sees its result. The sequence index must be fixed by resolution order, not by iteration order over a hash map.
- Sequential sub-rolls — the blind steps of a Pushing On route, each with its own Scavenging roll — consume sequence indices **in execution order**. A route that pushes on twice consumes four indices, not one, and reordering them changes the match.

---

## 22. Telemetry

R1 and R9 are answered by measurement, not opinion — R2 already was, and turned out to be backwards (§20). Every match — paper or digital — records the following, with one caveat [D33](../decisions/D33-telemetry-event-stream-coverage.md)'s row-by-row audit made explicit: seventeen of the twenty rows are headless facts, computable from a running match's event stream, final state, or order log alone; three are not, and are marked below with the milestone that produces them instead. The digital build computes each row from its own declared source, not one uniform mechanism: event-backed rows are emitted as structured events from day one, because retrofitting instrumentation always costs more than building it in; final-state and order-log rows are computed once, at scoring, through `telemetry.Match` (RFC §17). Row 13 ships without a precise answer in M2's first pass regardless of source — marked open below, not silently approximate.

### Reading these bands: sample size, interval, verdict

A band is not a verdict on its own. M2's simulation harness (RFC §16.4) reads the seven bands the roadmap's exit criteria name, and [D35](../decisions/D35-simulation-sample-size-and-verdict-rule.md) fixes how:

- **10,000 matches per configuration** — not §20's R2 precedent of 3,000, which measured a different and less sensitive question; RFC §16.4's own worked invocation already runs `--matches 10000`.
- **The match is the sampling unit, always.** Reduce each match to one number first — a per-match count or 0/1 indicator as it stands, a per-player metric to that match's mean across its own players, a ratio over in-match events to that match's own pooled ratio — then report `mean ± 1.96 · s / √n` over the resulting vector. One formula for counts, rates and indicators alike. A match with an empty denominator is excluded from that row's vector, and the exclusion count is reported beside it rather than papered over.
- **Action only when the whole interval sits on the failing side of the band.** A point estimate over the line with the interval still straddling it is *watch, not act*: re-run that configuration under a second, independently drawn root seed, pool both vectors into one 20,000-match interval, and if it still straddles, record "watch, unresolved at n = 20,000" and hand it to M5.5 rather than inflating n to manufacture precision.
- **A zero-width interval is a degenerate sample, not a finding.** Flag it and re-examine, never read it as a confident verdict. Fewer than two values left in a vector is the same failure at its limit — `s` is undefined at n = 1 and there is nothing to average at n = 0 — so the row reports **no measurement and no verdict**, never a bare mean dressed as one.
- **Both bot tiers are always reported, but each band's verdict is read against one** (RFC §14.3): **Drifter** for the two map-geometry questions — confrontations per match at 4–5 players, and the two-player floor under rotating borders — and **Operator** for every band that depends on a player who is trying: routes cancelled, incident exposure, the Infamy climb, the endgame share, and the lease rate.
- **No cross-metric multiplicity correction.** These are separate design questions governing independent levers, not one hypothesis family.

Rows marked *(M5)* or *(M5.5)* below have no headless statistic to reduce and sit outside this rule entirely.

### Per match

| Metric | Target band | Fails if |
|---|---|---|
| Routes cancelled mid-route *(M2)* | < 15% of submitted routes | **> 15%** → confrontation too punishing (R1) |
| Deliveries per player *(M2)* | 4–6 | < 3 → cooldown too long or map too large |
| Winner's RP lead over last place *(M2)* | < 40% | > 40% → comeback mechanisms insufficient |
| Matches where a player reached Infamy 9 *(M2)* | > 20% | **< 10%** → step gradient too steep (R7) |
| Matches where a player stayed at Infamy ≤ 2 to the end *(M2)* | < 25% | > 40% → anonymity still dominant (R3) |
| Sector incidents actually hitting a player *(M2)* | 30–60% of incidents | < 20% → players route around trivially; > 70% → unavoidable, feels unfair (R6) |
| Live leases at final scoring, per player *(M2)* | 2–4 | < 1 → lease rate too high (§10.4) |
| Share of map under sight in the final third *(M2)* | 30–55% | **> 65%** → post sight still too generous (§7.2) |
| Confrontations won against an Evasive loser *(M2)* | 20–40% of all confrontations | > 55% → Evasive is still the default insurance (§9.3) |
| Confrontations per match, 4–5 players *(M2)* | 4–12 | **> 12** → map too small (R9); Board goes unused as leading indicator |
| Confrontations per match, 2 players *(M2)* | 4–12 | < 4 → rotating borders (§6.3) not converging hard enough |
| Players ending a route in a flagged unstable sector *(M2)* | 25–50% of player-rounds | **< 15%** → boon share too low, the flag is still just "stay out" (§14.3) |
| Share of RP swing traceable to [O] cards *(M2, open — no precise answer in the first pass, see D33)* | < 15% | > 25% → boons are paying in cash where they should pay in tempo (§14.0) |
| Convergence [C] cards that produced a confrontation *(M2, loose reading — "a confrontation occurred in a [C]-card round," see D33)* | > 40% | < 20% → the convergence tag is decorative |
| Attribution queries that ruled out at least one player *(M5.5 — human question over a UI that doesn't exist yet)* | > 50% | **< 30%** → the one-round horizon is still too generous; the tool is theatre (§7.5) |
| Heat Map opened per player per match *(M5 — UI instrumentation, not a headless fact)* | > 5 | < 2 → pattern-reading isn't landing; check whether corridors actually exist on generated maps |
| Rounds flagged as Loitering *(M2)* | < 8% of player-rounds | > 15% → camping is outcompeting contracts, revisit R11 |
| Loitering flags triggered by legitimate short-haul play *(M5.5 — no operational definition exists to compute against)* | < 10% of flags | > 20% → the 1-step radius is too wide, or the action exemption too narrow |
| Heat Map entries at low confidence (< 3 observations) *(M2)* | < 40% | > 60% → observation coverage too thin for the tool to be usable |
| Confrontations occurring in the final 3 rounds *(M2)* | < 30% of all confrontations | > 45% → endgame farming is real (R11) |

### Per round
- Median and 90th-percentile time to submit an order, by round number. **Target: median under the timer by round 3.** If players are still timing out at round 6, the order form is too complex.
- Number of players who let the timer expire.
- Distribution of stance choices. **If Evasive exceeds 60%, the stance choice isn't a real choice.**
- Ledger purchase rate. **If under 10%, bands aren't creating real uncertainty; if over 70%, Cr$ 3 is too cheap.**
- Ledger purchases by round number. **A spike in the last three rounds means the staleness rule isn't doing its job** and the blackout window needs widening.

### Per action
- Action selection frequency. Any action chosen in **under 3%** of turns is dead weight and should be cut or buffed — the current suspects are Surveil and Vanish.
- Item purchase frequency by item. Same threshold.
- Global event and incident cards, by how much they swing final standings. A card that never changes an outcome is a card that only costs reading time.
- **Post-match recall.** In playtests, ask each player to name what happened without looking. If they can't name two events, the deck is too wide (R10).

---

## 23. Recommended next step

Before a line of Go, HTMX, or SPA: **play it on paper.**

Twenty-five nodes on a sheet, coloured chips, three people writing orders on slips simultaneously. A fourth person plays server — collects the slips, resolves steps, writes the trail, and hands each player only what they can see. It's tedious for the server and revealing for everyone else.

Two sessions like that answer R1, R2, R6, R7, and the lease rate — every question that could force a core rewrite. Two hours of paper beats three weeks of Go followed by the same discovery.

**Bring a scoresheet with the §22 metrics on it.** The point of a paper session isn't to find out whether the game is fun — you'll convince yourself it is either way. The point is to come back with eight numbers.

---

## 24. Appendix — the v1.1 flaw review, item by item

Recorded because the reasoning matters more than the verdicts, and because one of these will come back around in playtesting.

### 24.1 Vision saturation — **accepted, fix replaced**

The diagnosis was right and understated. The proposed fix — scaling the post cap by player count — does not work, because the problem is **coverage per post**, not post count. At average degree 3, one post under the old rule watched four nodes. Two players holding three posts each already put 76% of a 19-node map under permanent sight; no cap that still permits three posts fixes that.

The fix is **node-only sight** (§7.2), which brings coverage to 28–60% across all player counts. The dynamic cap was adopted anyway as secondary insurance (§10.3), with an honest note that it is not the load-bearing change.

### 24.2 The "Nobody speedrun" — **rejected**

The claim was that a player could sit at Infamy 0–2 and win on volume by endlessly cycling tier I contracts. This requires cycling to be possible. It isn't: at Infamy 0–2 the Contact Cooldown is **4 rounds**, and an offer yields **one** contract. That is 4 offers across a 15-round match, hard ceiling **8 RP from contracts** against an expected winning score of 30–40.

Working the whole ladder:

| Tier | Offers received | Rounds per job | Jobs completed | Contract RP |
|---|---|---|---|---|
| Nobody (0–2) | 4 | 3 | 4 | **8** |
| Known (3–5) | 5 | 3 | 5 | **15** |
| Feared (6–8) | 8 | 4 | 3 | **15** |
| Legend (9–10) | 15 | 6 | 2 | **16** |

Two things worth noticing beyond the rejection. The top three tiers land within 1 RP of each other, which is a better balance result than I expected and should be treated as suspiciously lucky until paper testing confirms it. And the **binding constraint changes** as you climb — offers limit the bottom of the ladder, travel time limits the top. That is a genuinely good structural property and worth protecting through future tuning.

There is also a second reason the strategy can't run: **delivering a tier I contract grants +1 Infamy**. Three deliveries push you into Known automatically. Staying a Nobody requires spending your action on **Vanish** repeatedly, which costs you the very tempo the strategy depends on.

What the review did surface correctly is that the v1.0 text **claimed** a Nobody "wins by never being seen", which is false by the document's own numbers. §11 has been corrected: Infamy 0–2 is a starting and recovery state, not a strategy. The residual risk that the bottom tier is a **trap** rather than a stepping stone is real, and telemetry already watches it.

### 24.3 Cognitive burden of the trail — **accepted in full**

The strongest point in the review. Across a 48-hour asynchronous round, no player holds a multi-round movement timeline in their head, and the failure mode is specific and ugly: the interesting part of the game migrates into a spreadsheet on a second monitor.

The suggested ghost-path overlay was adopted in v1.1 and **cut again in v1.3** once it was measured: reachability cones saturate at 94% of the map inside one round, so the overlay answers "anywhere" and calls it intelligence (§7.5). The Board survived and grew; the ghost path did not. What replaced it is anchored **Attribution** on a one-round horizon plus the **Heat Map**, which tracks routes instead of people and is the only part of the deduction layer that is immune to saturation.

### 24.4 Evasive too forgiving — **accepted, fix modified**

The observation that "lose the rest of your route" is worth nothing when you're ambushed on your final step is precise and correct.

Neither proposed remedy was adopted. The "attacker didn't double your roll" condition introduces a new comparison type for one edge case and makes outcomes swingier at exactly the moment players want them legible. A flat Cr$ 1 cost is too small to change any decision.

The adopted fix (§9.3, §15) is a **Cr$ 4 shakedown, a 2-node pushback, and −1 step next round**. The shakedown is a premium paid only when the insurance actually pays out, and it fixes a second problem the review didn't name: because Evasive stakes are fixed at zero, defeating an Evasive courier previously paid the winner **nothing but Infamy** — which removed the incentive to intercept, feeding directly into R2. The pushback and step loss are position-independent, closing the last-step hole.

### 24.5 Ledger abuse in the final rounds — **accepted, fix strengthened**

Correct. Universal final-round purchase converts bands into exact numbers precisely when the opacity is load-bearing.

The suggested blackout on rounds 14–15 works but only displaces the purchase to round 13. Pairing it with **one-round staleness** closes the loop: the Ledger becomes a tool for reading trajectory across a match rather than a calculator for the winning move. Adopted as a **final-round blackout plus permanent staleness** (§5.1), with telemetry watching for a late-match purchase spike.

### 24.6 Three flaws found while checking the review

- **Edge density figures were wrong.** "~1.9 edges per node" implies average degree 3.8, incompatible with the stated max degree of 4. §6 now specifies edge counts and average degree directly.
- **The warehouse supply limit never fires.** Two cargo per warehouse per round, across six warehouses with hidden positions and divergent contract origins, is a rule that costs a line on the reference card and triggers approximately never. Cut.
- **The 2-contract slot cap barely binds.** It only bites at Feared and Legend, where offers arrive every 1–2 rounds. Harmless, but the document was presenting a top-of-ladder safety rail as a general constraint. Logged as R8.
