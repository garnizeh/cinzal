# CINZAL — Implementation Roadmap

**Status:** draft for review · **Revision:** p1 · **Companion docs:** `cinzal-gdd.md` **v2.8**, `cinzal-architecture-rfc.md` **RFC-001 r12**

*This document sequences the work. It does not re-decide anything the GDD or the RFC already decided — where it appears to, that is a spec gap and it is logged in §3 rather than resolved silently.*

---

## 0. How to read this

The RFC already fixes the build order (§21). This document expands it into milestones with **exit criteria that can be demonstrated**, names the **spec decisions that must be made before each one can start**, and lists the work the RFC implies but never enumerates (CI enforcement, i18n, invite links, the Recap cursor, the map layout algorithm).

Three conventions:

- **Exit criteria are demonstrable, not aspirational.** "Resolve is implemented" is not an exit criterion; "a 15-round match replays byte-identically from `{seed, log}` on two machines" is.
- **Blocking decisions are listed before the milestone they block**, in §3. A milestone does not start with open blockers.
- **No time estimates.** Ordering, dependencies and gates only — per the RFC's own posture that the risky unknowns are measurement problems, not scheduling problems.

There are **no commits in this repository yet**. M0 starts from an empty tree.

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
| 13 | **Telemetry per §22** | M2 (computation) + M4 (persistence) |
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

**D14 · Small resolution gaps** to close while writing the pipeline, each cheap alone and each a silent bug if missed: `Torched` reducing a lease to ≤ 0 (expire, and does the public expiry trace fire?); `Muscle` loss in a 3+ melee (every non-winner loses); buying an item at the hand limit of 3; `Open Doors` letting a player "buy one item at half price" without being at a market — from which market's stock?; **`Bounty`** (highest RP) has no tie-break in RFC §6.5's table, unlike `New Boss`.

**D15 · Two documented cross-reference errors**, worth fixing in the source docs rather than working around: RFC §6.5's tie-break table cites a card called **"Blitz"** which does not exist in GDD §14.2 — the described behaviour (highest Infamy, hits every tied player) is **Raid**. And GDD §9.2's action table still says Stake Post is capped at "**5**", which §10.3 replaced with 4/4/4/3.

### 3.3 Product surface — blocks **M5**/**M6**

**D16 · The Recap has no cursor.** GDD §18 requires "every round since your last visit", and the RFC schema has no per-seat last-seen-round. `sessions.last_seen_at` is per session, not per match. Needs a column (`match_players.last_seen_round`) and a defined update point.

**D17 · Invite links have no storage.** RFC §19 promises "high-entropy, revocable, single-match scope"; §7.2's schema has no table or column for them. Needs a design, including whether revocation is per-link or per-match.

**D18 · Pins and notes have no storage.** GDD §7.5 lists manual annotation as one of the Board's four tools and it is inside the v1 scope list (§17). Nothing in the RFC schema holds it. Either add a table or move it explicitly to v1.1 — the current state is that it is promised and unbuildable.

**D19 · Per-match email preferences have no storage.** RFC §13 specifies four preference levels plus one-click unsubscribe per match. No table.

**D20 · Rate-limit state has no home.** RFC §12.2 specifies per-email and per-IP limits. In-process counters are wrong across two app instances (the deployment target in §18). Options: a Postgres table (simple, consistent, one more write path on the auth hot path — which is low-traffic by nature) or a fixed-window counter in the DB with a cleanup job. Recommendation: Postgres. Redis is explicitly out of the stack (§4) and adding it for rate limiting would be the most expensive line in the deployment topology.

**D21 · i18n is in scope and has no design.** RFC §1's non-goals exclude "i18n beyond the two languages already in play" — i.e. English and Portuguese are **in**. GDD §2.3 names the Portuguese edition. RFC §11.5 makes localisation possible by forbidding prose in `Event`, but nothing specifies the catalogue format, the locale-selection rule, or who owns the ~60 card/item/contract strings. Must be decided before `render` grows a hundred hard-coded English strings. Options: `golang.org/x/text/message` catalogues, or a simple embedded map keyed by `{locale, key}` given the small string count and zero pluralisation complexity in card text. Recommendation: the simple map, upgraded only if the string count grows.

**D22 · Match abandonment is undefined.** `matches.status` includes `abandoned` (RFC §7.2) and nothing says what produces it. Autopilot means a match never stalls, so the plausible trigger is "every seat on autopilot for N rounds" or a host action. Needs a rule, or the status is dead.

---

## 4. Milestones

### M0 · Foundations

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

### M1 · Rules core

**Goal:** the entire game, deterministic and headless. No database, no network, no browser.

Blocked by: **D3–D15**. Blocks: everything.

**Deliverables**
- `Config` (§6.2), including `Rounds` **validation** against deck arithmetic (RFC §6.2) and the D11 suppression flags.
- `RNG` (§6.4): HMAC-derived draws, `purpose` strings, single non-branching instance, and the **completed** consumption table from D3, with the lazy-draw rule honoured at all six early-termination points.
- `rules/gen`: graph generation under all seven §6 constraints, plus the D8/D9/D10 resolutions (sector sizing, type-share rounding, deterministic layout).
- `MatchState`, `Player`, `Node`, `Graph`, `Contract` (per-player instances with their own Deadline Pause flag), the four fog states, and the eight cross-round counters from RFC §6.6.
- `Order` + `Legal()` covering every row of GDD §15.0, and the affordance metadata RFC §10.2 requires the server to render.
- `Resolve()` as the fixed pipeline of RFC §6.7 — validate → per-step movement with crossing and collision → actions → deliveries → add-ons → trail → event/incident/pressure/upkeep — with the entry snapshot (§6.6) and both orderings (§6.5) implemented as the only two comparators in the codebase.
- `Project()` and `PlayerView`, including the `SeatArchive` sight/trail history (§9.2), `NodeStats`, and all **eleven** authorised position writers (§9.1) — plus `Decoy` if D12 adds a twelfth.
- Final scoring (GDD §16) and the two-player rule set (§6.3).
- Full test suite from RFC §16.1's matrix that does not need a database: unit, property, golden replays, **fog negative tests**, cross-round counters, lazy RNG, Torn Map, tie-breaks, entry snapshot, anchor parity, headline coherence, adversarial payloads.

**Exit criteria**
- `resolve(s, o) == resolve(s, o)` byte-identical, and a golden 15-round replay reproduces on a second machine and a second OS.
- The RNG index count for each round matches the §6.4 table prediction **including truncation cases**.
- Fog suite: for a state where seat A cannot see node N, `Project(s, A)` serialised to JSON contains **no occurrence of N's ID anywhere in the bytes**.
- Anchor parity test passes against GDD §7.3's trail table, row for row.
- A full match can be driven to final scoring from a Go test with no I/O of any kind.

---

### M2 · Bots and simulation — **the measurement gate**

**Goal:** answer the balance questions the GDD deferred, by measurement, before any of them can be rationalised away.

Blocked by: M1. **Blocks nothing technically — and that is exactly why it is at risk of being skipped.** See §2.

**Deliverables**
- `Bot` interface (`Decide(PlayerView, Config, RNG) Order`) and the three tiers: Drifter, Runner, Operator.
- `cmd/simulate` with parameter sweeps and CSV output.
- **The full GDD §22 metric set**, computed from the `Event` stream — one computation, later shared with the server's analytics path (RFC §17).

**Exit criteria — expressed as answers, not as code**

The milestone is done when the following have numbers attached, from sweeps at 2/3/4/5 players:

| Question | GDD ref | Threshold that forces action |
|---|---|---|
| Confrontations per match | R9 / §22 | > 12 → raise node count before touching anything else |
| Routes cancelled mid-route | R1 | > 15% → soften the confrontation rule |
| Lease rate — the most sensitive dial in the game | §10.4 | live leases at scoring outside 2–4 per player |
| Incidents actually hitting a player | R6 | < 20% or > 70% |
| Matches reaching Infamy 9 | R7 | < 10% → step gradient too steep |
| Endgame camping | R11 | confrontations in the final 3 rounds > 45% |
| Two-player encounter rate under rotating borders | §6.3 | < 4 per match |

Plus: a bot could be written **competently against `PlayerView` alone** (RFC §14.1). If Operator needed information the view does not carry, that is a projection defect and it goes back to M1.

**Caveat to carry forward:** bot play is not human play. A sweep tells you the *shape* of the parameter space — where a dial flips a strategy from dominant to dead — not the exact value. It narrows the range that M5.5 then confirms.

---

### M3 · Persistence

**Goal:** matches survive a restart and reproduce exactly.

**Deliverables**
- Schema per RFC §7.2, plus the D16–D19 additions (Recap cursor, invite links, pins, email preferences) and the D20 rate-limit table.
- `sqlc` queries, `goose` migrations embedded and run at startup behind the **advisory lock** (§7.5).
- `fold()` — `state = fold(Resolve, initial(seed, cfg), orderLog)`.
- `cmd/replay`, including `--rebuild` for the derived `events` / `match_summary` projections.
- Fold duration and fold allocation metrics wired from day one — they are the falsifiability trigger for the no-snapshot decision (§7.3) and are worthless added later.

**Exit criteria**
- A match folded from the log equals the incrementally computed state, asserted over a golden fixture.
- Two app processes booting simultaneously against a fresh database both come up, with migrations applied exactly once.
- `cmd/replay --rebuild` regenerates `events` and `match_summary` to byte-identical content.
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
- **The Board** (GDD §7.5): the Log, anchored Attribution as a candidate table, the Heat Map as a rate with sample counts and the low-confidence flag, and pins/notes per D18.
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
| **Determinism** | M1 | Every new random consumer gets a row in RFC §6.4's table and an index-count assertion, including its truncation cases. |
| **Doc synchronisation** | M1 | GDD §21 and RFC §6.4 must not drift; GDD §7.3 and RFC §9.1 must not drift. Both pairs have parity tests — keep them failing loudly. |
| **Telemetry** | M2 | One computation, three sinks: `cmd/simulate` CSV, the analytics table, the debug panel. Never three implementations. |
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
