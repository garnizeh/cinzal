# CINZAL — Architecture RFC
## RFC-001 · Game server, client, and tooling
**Status:** draft for review · **Revision:** r15 · **Companion doc:** `cinzal-gdd.md` **v2.15**

*(The two documents advance independently. Pair them by changelog rather than by version number — each entry records what moved and why.)*

> **Changelog r1 → r2** — four review findings, all accepted
> - **§9 contradicted itself.** The prose named three writers of opponent position; the `PlayerView` struct already carried an `Anchors` field listing three more. Replaced with a full enumeration, now the authoritative list that the fog tests assert against. Without it, the Board's Attribution (GDD §7.5) has nothing to work from.
> - **§10.1 conflated validation with affordance.** `Legal()` returning an error is not the same as a control the player cannot press wrongly. Added §10.2 — the rules that must be encoded as UI state.
> - **§6.4 accounted for Pushing On indices but not pushback hops.** Added a full RNG consumption table; every consumer in the pipeline is now enumerated with its index cost.
> - **§8 left the deadline race undefined.** The deadline is now authoritative against the database clock inside the submit transaction, and Autopilot became a fact derived from the order log rather than a mutable seat flag.
>
> **Changelog r2 → r3** — three findings, all accepted, plus one GDD bug they surfaced
> - **The Headline paradox is real.** Event and incident cards cannot be drawn in Phases 6/7 when Phase 1 already published their metadata. Both decks are now **shuffled at setup**; the round does a deterministic pop and consumes no index.
> - **Batch ordering in Phase 7 was undefined.** Added §6.5 — and the answer is *not* the gameplay tie-break key. Contended actions and RNG batching need different orderings, for different reasons.
> - **`fold()` must never send email.** A rebuild would have re-sent every historical notification. Added §7.5 — effects belong to the caller, and `cmd/replay` passes a null sink.
> - **Found while checking the first item:** the GDD drew 12 event cards for a 15-round match. Fixed in GDD v2.4 — global events now run rounds 4–15, with a quiet opening.
>
> **Changelog r3 → r4** — the map island was overspecified, and v1 shrank
> - **§11.2 rewritten.** r1–r3 claimed the map needed a JS island for sub-100ms feedback. That justification was about hover preview and dragging, not discrete clicks — and a player clicks 3–4 nodes per round inside a 60–90s window. The map is now a **server-rendered SVG generated in Go from `PlayerView`**, clicked through HTMX. Roughly 250 lines of Go instead of ~1,800 lines of JS.
> - **WASM moved off the critical path** (§10). With validation server-side per click, it becomes an RFC-002 optimisation rather than a v1 prerequisite. It stays in this document because the reasoning is still correct — just deferred.
> - **v1 is now "visually lean", not textual.** Resolution renders as a narrated event list rather than an animation, which is the single most expensive thing deferred and doubles as the `round_resolved` email body.
> - **Build order revised** — milestone 5 is promoted from internal scaffolding to the thing that ships, and becomes the research for RFC-002.
>
> **Changelog r4 → r5** — three gaps in derived state
> - **The Heat Map had no data.** `PlayerView` carried one round of trail; GDD §7.5 needs per-node observation history across the whole match. Added a per-seat sight and trail archive to `MatchState` (§9.2) — and note the counters need *sight without traffic* too, which an archive of trail entries alone cannot express.
> - **Conditional RNG draws must be lazy.** Gas Leak truncating a Pushing On walk is one of six early-termination points; the general rule is now stated once, in §6.4, rather than case by case.
> - **Cross-round derived counters were never enumerated.** Loitering was the visible instance; there are eight. Added §6.7 — get one wrong and the bug is a rule quietly not firing, which no test will notice unless it was written for that rule specifically.
>
> **Changelog r5 → r6** — one omission, one class of omission, one simpler fix
> - **`item.tornmap` was missing from the RNG table**, and its cost is method-dependent: rejection sampling is unbounded, partial Fisher-Yates is exactly `min(4, |hidden|)`. The method is now mandated rather than left to the implementer.
> - **The Order had no field for item discards** — a GDD hole, not just an RFC one. Fixed in GDD §9.4; the `Order` struct and form here follow. It also forced a rule fix: Bolt Hole's "retreat 2 nodes of your choosing" is impossible under simultaneous orders.
> - **Tie-breaking on selections was undefined**, and lease surrender is one of five sites. Added a total-ordering table in §6.5.
> - **The Ledger gap is real; the proposed fix is not needed.** "End of the previous round" *is* the state entering `Resolve`, so an entry snapshot suffices and no cross-round dictionary is required (§6.6).
>
> **Changelog r6 → r7** — solo play
> Added §14.5. Solo is a **fourth role for the same bots**, not a mode: one human seat, unlimited timer, no email, no async. The engine work is approximately zero once milestone 4 lands; the cost is scenario data and curriculum, which lives in GDD v2.6 §19.1.
> Two things that do need stating here: scenarios pin a **fixed seed**, and solo matches are **tagged in telemetry** so bot data cannot contaminate the human balance numbers the open questions depend on.
>
> **Changelog r7 → r8** — an audit of the anchor table, and an outbox key that was too simple
> - **The anchor table was missing cargo pickup**, the most load-bearing anchor in the game and the worked example in GDD §7.4. Found while checking whether *item purchase* belonged — it did not, for the reason given, but the audit it prompted did. The table is now eleven rows, with the redundant entries documented as redundant so the fog suite does not flag them.
> - **`UNIQUE (match_id, round, seat, template)` cannot cover `otp`**, which has no match. Replaced with a partial index in §13.1, plus a fix for a race no constraint could have caught: `deadline_soon` firing at a player who submitted between the check and the send.
> - Header now points at GDD v2.6.
>
> **Changelog r8 → r9** — consistency pass, no decisions changed
> - **The decisions table and Q2 still described the JS map island**, which r4 removed. Two readers' first stop at the document contradicted the section that actually decides it.
> - Four cross-references pointed at renumbered sections: `cmd/simulate` and the Config rationale cited §16.3 for the harness (it is §16.4), the tick cited §16.1 for bot filling (it is §14.2), and the test matrix still said "nine-writer anchor table" after r8 made it eleven.
> - §9 had no §9.1 — the projection rules sat in an unnumbered preamble while everything referencing them cited a number.
> - Companion pointer moved to GDD v2.7.
> - **`Config.Rounds` was effectively a free parameter**, and it is not — GDD v2.7 §16.2 shows the deck arithmetic fails above 15. Added a validation note in §6.2 so nobody discovers it by booking a 20-round table.
> - Version-pinned GDD citations replaced with bare section numbers; they were already stale twice over.
>
> **Changelog r9 → r10** — two accepted, one measured and declined
> - **Connection pool starvation is real** and fails at the worst possible moment. Added §8.3: an isolated pool for the sweeper, plus lock and statement timeouts, which the RFC also lacked.
> - **Resubmission after a round closes** needed handling — but the hazard is not the one usually assumed, and the fix is a form field rather than an idempotency table. Added §11.1a.
> - **Snapshotting the fold was declined, with numbers.** A full fold allocates ~1.5 MB, and the arithmetic behind that is now in §7.3 along with an explicit trigger for revisiting and a pre-designed escape hatch.
>
> **Changelog r11 → r12** — a named Go version, and the `game` exception §6.1 had been missing
> - **§4 now names Go 1.26.5** rather than "1.23+". The reason is §6.3: the whole design is staked on `seed + order log` reproducing a match **exactly, forever**, and §15.4 alerts loudly when a refold disagrees with the incrementally computed state. A determinism mismatch is the one alert §17 says is worth waking someone for, and "which Go built it" should never be among the candidate explanations. The four hazards §6.3 names are all in our own code and none are version-dependent, so this removes a variable rather than fixing a known bug.
> - **On what actually enforces it, stated precisely, because the first draft of this entry overclaimed.** The `go` directive in `go.mod` is a **minimum**, not a ceiling — `go 1.26.5` is satisfied by Go 1.27. A `toolchain` directive would not close the gap either: it names a **minimum** toolchain to switch to, so it raises the floor rather than capping it — and one equal to the `go` line is redundant and does not survive, since `go build` errors with *"updates to go.mod needed"* until `go mod tidy` runs, and tidy deletes it. **No directive pins a version from inside `go.mod` at all.** What does is the pairing CI uses: `setup-go` reading `go-version-file: go.mod`, plus `GOTOOLCHAIN=local` to forbid switching. **Exact-version enforcement is a CI concern** — `GOTOOLCHAIN`, or a fixed `go-version` in the workflow — and lands with the CI task, not here. Until then this is a documented intent with a floor behind it, and saying otherwise would be the kind of claim that makes a document untrustworthy in the places it is right.
> - **§6.1 corrected: `rules` imports the standard library *and `internal/game`*.** The old wording, "nothing but the standard library", was written when `PlayerView` lived inside `rules`. D01 moved it to a leaf package, so `Project` cannot return a view and `Resolve` cannot take an order without that import. The purity property is unaffected — `game` imports nothing outside the standard library — but a CI gate written against the old sentence would have rejected the engine's own signature.
>
> **Changelog r10 → r11** — Autopilot switched itself off, and two undefined orderings
> - **Autopilot was self-cancelling.** The streak counted `source = 'default'`, but an autopilot seat's orders are generated by a bot — so the round after Autopilot engaged, the streak broke and it disengaged. It would have flapped on and off forever. Now counts `source <> 'human'`, which needs no new column.
> - **Free-for-all pushback had no ordering.** Two or three losers each drawing 1–2 `pushback.hop` indices, iterated in arrival order, is a replay divergence. Added to §6.5.
> - **Multiple confrontations in one movement step** had no ordering either — a gap nobody raised, found while fixing the one above.
> - Companion pointer moved to GDD v2.8.
>
> **Changelog r12 → r13** — a card that never existed (D15)
> - **§6.5's tie-break table cited "Blitz"**, which is not a card in GDD §14.2. The described behaviour — highest Infamy, hits every tied player — is **Raid**. Table row and prose both corrected; the reasoning attached to the row was right about the rule and only wrong about the name.
> - Companion pointer moved to GDD v2.9.
>
> **Changelog r13 → r14** — Riot's fog dimension (D4), and a numbering bug it surfaced
> - **§9.1's own prose, and the `writeAnchors` pseudocode right below it, had both drifted from the table.** r8 inserted the cargo-taken row at position 1, shifting every later row down by one, but neither the two paragraphs distinguishing global-vs-sight-gated and named-vs-node-only rows nor the code comment stating the same split were updated — all three still cited row 6 as the sight-gated, both-named confrontation entry (it is row 7 now) and rows 4–5 as the node-only ones (they are rows 5–6). Found while confirming which rows Riot is allowed to touch; corrected against the table, which was already right, and against issue #75's independently-restated copy, which had already caught the drift.
> - No table change: Riot's permutation runs entirely inside `Resolve`/`writeTrail`, before `Project` ever sees the round, so the eleven-writer table needed no new row and no footnote. Full reasoning in [D4](../decisions/D04-riot-trail-randomization.md).
> - Companion pointer moved to GDD v2.14.
>
> **Changelog r14 → r15** — `upkeep()` had no body (D5)
> - **§6.7 ended the pipeline with a bare function name.** Expanded into its four ordered steps, and the one real ordering constraint is now stated as a requirement rather than left to whoever writes it: the contract-deadline Debt cascade must run before the lease decrement it depends on reading pre-mutation, or Debt can end up surrendering a healthier lease than "fewest rounds remaining" calls for. Lease removal reuses the existing row 4 anchor (§9.1) regardless of cause — natural expiry and a Debt-driven surrender must be byte-identical on the wire, or which lease died discloses that a player is in debt. Everything else `upkeep()` touches is private to the acting seat, since none of it has a row in §9.1's table.
> - **`Flagged` and `EvasiveStepPenalty` moved out of `upkeep()` entirely, into §6.6's entry-snapshot mechanism**, alongside the Ledger and the step-allowance formula's Infamy read. A first-draft version of this decision cleared both as an Upkeep step and got caught on review: either field can be set fresh from more than one place in the same round (an ordinary Debt trigger during resolution, an Evasive loss, or the contract cascade above), and any Upkeep-phase clear runs after all of resolution, so it cannot distinguish a value the round already consumed from one just written for the next round. Consuming both from the frozen snapshot — the same one the step-allowance formula already reads — and resetting the live copy in that same top-of-`Resolve` step, before anything in the round can write to it, is the only position immune to that.
> - Two more counters the issue's draft list assumed lived in `upkeep()` do not: `LastOfferRound` needs no mutation (§6.6 already describes it as read-only-by-difference), and `LooseCrateHeldRounds` ticks inside `writeTrail`, three steps earlier in the same pipeline. Full reasoning, iteration order, and both worked examples in [D5](../decisions/D05-upkeep-phase.md).
> - Companion pointer moved to GDD v2.15.

---

## 1. Scope

This RFC covers everything needed to build and operate v1 as scoped in GDD §17: the rules engine, persistence, the round lifecycle, fog projection, the web client, authentication, email, bots, debug tooling, and deployment.

**Non-goals for v1.** Horizontal scale beyond a handful of app instances, mobile apps, matchmaking against strangers, spectator mode, in-app purchase, anti-cheat beyond server authority, i18n beyond the two languages already in play.

**Estimated load this is designed for.** Hundreds of concurrent matches, not hundreds of thousands. A match holds at most 75 orders (15 rounds × 5 seats) and a state object measured in kilobytes. Nearly every performance instinct you might have is wrong at this scale, and §7.3 says so explicitly where it matters.

---

## 2. Decisions at a glance

| Area | Decision | Why not the alternative |
|---|---|---|
| Language | **Go** | Compile-time type separation is load-bearing for fog (§5) |
| Rendering | **HTMX + server-rendered HTML**, zero hand-written JS in v1 | A JSON API is a second surface that must also be fog-filtered |
| Templates | **templ** | Compile-time checked, composes with fragments; `html/template` fails at runtime |
| Client rules | **Server-side in v1**; WASM deferred to RFC-002 | Discrete clicks in a 90s window don't need local evaluation (§11.2) |
| Map | **Server-rendered SVG**, clicked through HTMX | An island buys hover and drag, neither of which v1 ships |
| State | **Event sourcing, no snapshots** | 75 orders re-folds in microseconds; snapshots are a cache nobody needs |
| DB | **Postgres** + `sqlc` + `goose` | Async play is a database problem, not a realtime one |
| Migrations | **goose at process start**, behind an advisory lock | No manual step; lock prevents concurrent-instance races |
| Push | **SSE** | Traffic is one-directional; SSE reconnects itself and crosses proxies |
| Auth | **Email OTP**, no passwords | No password storage, no reset flow, no credential stuffing |
| Email | Transactional provider + outbox table | Login and gameplay both depend on it; inline sending is not acceptable |
| Bots | One interface, **four roles**: filler, autopilot, simulation, practice | Also an executable test of the fog boundary (§14.1) |
| Debug | **Build tag**, not a runtime flag | A runtime flag is one config mistake from a god view in production |

---

## 3. The constraint that decides everything

Fog is private (GDD §7.1). Therefore **the client must never hold the full match state**. This is not an architectural preference; it is a rule of the game. If the whole state reaches the browser, anyone opens DevTools and reads the map.

Everything the player sees passes through exactly one function:

```go
func Project(s State, seat SeatID) PlayerView
```

Three consequences shape the rest of this document.

**One projection, no exceptions.** There is no second path to the client. No "just this once" endpoint, no debug JSON in production, no template that reaches around it.

**Type separation enforced by the compiler.** The `render` and `web` packages do not import `state`. They cannot: `PlayerView` lives in its own package and `MatchState` is not in scope there. A template physically cannot leak what it cannot name. This is the single strongest reason to use a statically typed language here, and it is worth arranging the package graph specifically to get it.

**HTML as the wire format is a security property, not just a style.** A SPA needs an endpoint that returns data, and that endpoint needs the same filtering. It is far easier to over-return in JSON — one extra struct field, one embedded association — than in HTML you had to write by hand. HTMX removes an entire class of leak by removing the surface.

---

## 4. Stack

```text
Go 1.26.5
templ                     — typed templates
HTMX 2.x + SSE extension  — interactivity
sqlc                      — typed queries from SQL
goose                     — migrations, embedded, run at startup
pgx/v5                    — Postgres driver
Postgres 16

Deferred to RFC-002:
Go→WASM                   — client-side rules
client-side map interaction — pan, zoom, hover, animation, touch
```

**v1 ships zero hand-written JavaScript** beyond the HTMX and SSE library tags. No ORM, no Redis, no message broker, no frontend build step beyond `templ generate`. Static assets and templates are embedded with `embed.FS`, so deployment is one binary plus a database URL.

---

## 5. Package layout

```text
cmd/
  server/         — the web process
  simulate/       — headless bot harness (§16.4)
  replay/         — CLI: replay a match log, dump state at round N

internal/
  rules/          — PURE. no I/O, no clock, no rand, no db.
    state.go        MatchState, Player, Node, Graph
    order.go        Order, validation, legality (§15.0 of the GDD)
    resolve.go      Resolve(): the whole round pipeline
    confront.go     confrontation, pushback, displacement
    fog.go          Project() and PlayerView
    config.go       every tunable dial from the GDD
    rng.go          seeded, deterministic selection
    gen/            map generation with the §6 constraints

  store/          — sqlc output + repositories + migrations (embedded)
  match/          — lifecycle: create, join, tick, deadline sweeper
  bots/           — Decide(PlayerView) Order, three difficulty tiers
  mail/           — outbox, templates, provider adapter
  auth/           — OTP issue/verify, sessions, guest accounts
  web/            — handlers, routing, SSE hub
  render/         — templ components. imports rules/fog, NOT rules/state
  debug/          — build-tagged tooling (§17)

wasm/             — GOOS=js entrypoint, wraps internal/rules
```

**The import rule that matters:** `render` may import the fog package for `PlayerView`, and may not import anything that exposes `MatchState`. Enforce it in CI with a one-line `go list` check rather than trusting discipline — this is exactly the kind of rule that erodes at 2am.

---

## 6. The rules core

### 6.1 The contract

```go
package rules

// Resolve is the entire round pipeline. It is a pure function:
// same inputs, same outputs, on any machine, forever.
func Resolve(s State, orders map[SeatID]Order, cfg Config, r *RNG) (State, []Event, error)

// Project is the fog boundary.
func Project(s State, seat SeatID) PlayerView

// Legal answers "would the server accept this order right now?"
// Shared with the client via WASM (§10).
func Legal(v PlayerView, o Order, cfg Config) error
```

`rules` imports the standard library and **`internal/game`**, and nothing else. From the standard library it does not import `time`, `math/rand`, `os`, or anything touching the network. This is enforced by a CI check, not by convention.

*(The `game` exception is not a loosening. `Project` returns a `game.PlayerView` and `Resolve` takes `game.Order` and `game.Config`, so the dependency is unavoidable once the fog boundary lives in its own package — see D01. It costs nothing, because `game` itself imports nothing outside the standard library and declares no `any`, `interface{}` or unconstrained type parameter, so the purity property transits it unchanged. Earlier revisions said "nothing but the standard library" because `PlayerView` was still inside `rules`.)*

### 6.2 Config is data, and it travels with the match

Every dial the GDD calls tunable becomes a field:

```go
type Config struct {
    Rounds            int              // 15 — NOT freely tunable, see below
    StepsByTier       [4]int           // 4,4,3,2        GDD §9.1
    CooldownByTier    [4]int           // 4,3,2,1        GDD §8.2
    LeaseCostPerBlock int              // 3              GDD §10.4  ← most sensitive dial
    LeaseBlockRounds  int              // 3
    PostCapByPlayers  map[int]int      // 2:4 3:4 4:4 5:3
    ShakedownCost     int              // 4
    LedgerCost        int              // 3
    Contracts         [4]ContractTier
    // …
}
```

**`Rounds` is validated, not merely configured.** It reads like a dial and is not one: the event and incident decks are sized to it, and GDD §16.2 works through why 20 rounds is unserviceable — 18 incident rounds against a 16-card pool cannot be dealt without a reshuffle that breaks the displayed hazard/boon counter. Match creation rejects a round count the drawn decks cannot cover, rather than trusting the caller. The same check belongs in the simulation harness, which is otherwise the most likely place to sweep straight past it.

**The config is serialised into the `matches` row at creation and never read from global state.** Two consequences, both important:

- Rebalancing never corrupts an in-flight match. A match created under v1 rules finishes under v1 rules, including its replay six months later.
- The simulation harness (§16.4) can sweep the parameter space by varying one struct, which is how the lease rate question actually gets answered.

### 6.3 Determinism, and the four ways Go will break it

GDD §21 requires that `seed + order log` reproduces a match exactly. Four specific hazards:

**1. Map iteration order.** Go randomises it deliberately. Any `for k := range m` inside resolution is a latent divergence that will surface as a replay mismatch weeks later, on one machine, intermittently. **Rule: resolution never ranges over a map.** Collect into a slice, sort by an explicit documented key, iterate that. The order-resolution sequence in GDD §15 (ascending Infamy, then balance, then RP, then seeded coin) is exactly such a key and must be implemented as a comparator, not as a map traversal.

**2. Floating point.** Avoid entirely. Every percentage in the GDD is expressible as integer arithmetic — Currency Slide's 25% is `bal - bal/4`, Market Surge's +50% is `pay + pay/2`, and rounding is specified as "down" everywhere it appears. No `float64` enters `rules`.

**3. Ambient time.** `time.Now()` inside the rules package would make replay impossible. Round number is the only clock. Deadlines live in the match lifecycle layer (§8), never in resolution.

**4. Concurrency.** Resolution is single-goroutine. There is no scenario at this scale where parallelising a 5-player round is worth a nondeterminism risk.

### 6.4 Seeded randomness

```go
type RNG struct{ seed [32]byte; round int; seq uint32 }

func (r *RNG) Next(purpose string, n int) int   // deterministic draw in [0,n)
```

Each draw derives from `HMAC(seed, round || seq || purpose)` and increments `seq`. The `purpose` string is a tripwire: it does not affect the value's distribution but is recorded in the debug trace, so a divergent replay tells you *which* draw went wrong rather than only that one did.

**Sequence indices are consumed in execution order.** The RNG is threaded through resolution as a single `*RNG`, never branched, never copied, never passed to a goroutine.

Every consumer must be enumerated, because an unaccounted draw is a replay divergence that surfaces months later. r1 accounted for Pushing On and missed pushback entirely.

| Consumer | Purpose string | Indices consumed | Notes |
|---|---|---|---|
| Contract offer | `contract.offer` | 3 per offering seat | Phase 2 |
| Market stock | `market.stock` | 3 per market refreshed | Phase 3, every 2 rounds |
| Confrontation D6 | `confront.d6` | **1 per participant, per confrontation** | Not per confrontation |
| Tie-break coin | `confront.tiebreak` | 1, only at the fourth level | GDD §15 |
| **Pushback, stationary loser** | `pushback.hop` | **1 per hop — a second hop if Evasive** | GDD §15; the case r1 missed |
| Blind edge selection | `pushon.edge` | 1 per blind step | GDD §9.1 |
| Scavenging | `scavenge.d6` | 1 per **newly** explored node | Zero if the node was already Known |
| Pressure D6 | `pressure.d6` | 1 per Legend | Phase 7 |
| **Event deck shuffle** | `deck.event` | **at Setup only** | See below |
| **Incident deck shuffle** | `deck.incident` | **at Setup only** | See below |
| Unstable sector | `incident.sector` | 1 | Phase 1 — drawn where it is announced |
| Snatch Job relocation | `incident.relocate` | 1 per affected player | Phase 7 |
| Crate placement | `crate.node` | 1 | Dead Runner, Spilled Load |
| **Torn Map item** | `item.tornmap` | **exactly `min(4, hidden)`** | Method mandated below |

**Why the decks are shuffled at setup.** r2 had the event card drawn in Phase 6 and the incident card in Phase 7. That is impossible: GDD §14.1 publishes the event's **category** and the incident's **sector** in the Headline, at Phase 1, before orders are submitted. You cannot reveal metadata about a card you have not selected yet, and selecting it at Phase 1 while claiming to draw it at Phase 6 is the same bug wearing a hat.

Both decks are therefore built and ordered in `initial(seed, cfg)`:

```go
// Setup. Consumes a defined, auditable number of indices.
eventDeck    = shuffleConstrained(allEvents,    3, perCategory)  // 12 of 24
incidentDeck = shuffleConstrained(allIncidents, 9, 4)            // 13 of 16: 9 hazards, 4 boons
```

`shuffleConstrained` enforces the GDD §14.2 and §14.3 draw guarantees — exactly 3 per category, exactly 4 boons — which is also the only place those constraints can be enforced, since they are properties of the whole match deck rather than of any single draw.

At runtime, Phase 1 **peeks** at the head of each deck to print the Headline, and Phases 6 and 7 **pop** it. Neither consumes an index. The deck order is derived from the seed, so it does not need storing: refolding reproduces it.

*(The event deck holds 12 cards for rounds 4–15; rounds 1–3 pop nothing. See GDD v2.4 §14.2.)*

**Torn Map needs its method mandated, not left open.** GDD §12 reveals "4 random Hidden nodes", and the index cost depends entirely on the implementation: naive rejection sampling redraws on collisions and consumes an unbounded number, while a partial Fisher-Yates consumes exactly `min(4, |hidden|)`. Two correct-looking implementations of the same rule would desynchronise against each other.

```go
// Mandated. Candidates SORTED BY NodeID — the hidden set is a map, and
// building the slice by ranging it is the §6.3 hazard wearing a new hat.
candidates := sortedHiddenNodes(v)
n := min(4, len(candidates))
for i := 0; i < n; i++ {
    j := i + rng.Next("item.tornmap", len(candidates)-i)
    candidates[i], candidates[j] = candidates[j], candidates[i]
}
reveal(candidates[:n])
```

Torn Map is the **only** item that draws. The other seven take a player-declared target or apply a fixed effect.

**Conditional draws are lazy. A branch not taken consumes nothing.**

This is the rule that keeps the index stream synchronised, and it must hold at every early-termination point in the pipeline. Indices are never pre-drawn for steps that might not execute:

| Early termination | Draws not consumed |
|---|---|
| Pushing On hits a **Gas Leak** sector boundary — the walk stops at the last node outside it | all remaining `pushon.edge` **and** `scavenge.d6` for steps that never ran |
| Pushing On finds no legal edge | same |
| Blind step lands on an **already-Known** node | that step's `scavenge.d6` |
| Pushback loser is boxed in | remaining `pushback.hop` |
| No confrontation occurs at a node | `confront.d6`, `confront.tiebreak` |
| No Legend on the table | `pressure.d6` |

Determinism is preserved because every abort condition is itself derived from state, so a replay aborts at exactly the same point and the sequence numbers line up. The failure mode to guard against is the opposite instinct: an implementation that draws both blind steps up front and then discards one silently desynchronises everything after it in the round, and the symptom appears rounds later as a replay mismatch with no obvious cause.

The invariant test in §16.2 asserts consumed indices against the predicted count **including truncation cases**, which is the only way this gets caught early.

Two worked cases, because these are the ones that will be got wrong:

- A route with **2 blind steps** onto two previously-Hidden nodes consumes **4** indices: `pushon.edge`, `scavenge.d6`, `pushon.edge`, `scavenge.d6` — interleaved in execution order, not batched by kind.
- A **stationary Evasive loser** consumes **2** `pushback.hop` indices. A stationary Neutral loser consumes 1. A loser who moved consumes **0**, because their fallback walks a known route rather than drawing.

The purpose string is recorded in the debug RNG trace (§15.3), so a divergent replay names the draw that went wrong rather than only the round.

### 6.5 Two orderings, for two different reasons

Any batch operation must iterate in a defined order or determinism dies (§6.3). But r2 left open *which* order, and the answer is not one order — it is two, and conflating them is a mistake in both directions.

**Contended actions use the fairness key.** Where position in the queue confers an advantage — the last cargo at a warehouse, the last Shiv at a market — the order is the GDD §15 key: **ascending Infamy → lower balance → lower RP → seeded coin**. That key exists because it is *fair*, not because it is deterministic; determinism is a side effect.

**Everything else uses seat index.** Pressure rolls, Snatch Job relocations, crate placement, confrontation dice — batches where position confers nothing, because each draw is independent and unpredictable. Here the only requirement is that the order be stable and defined, and **seat index is the right answer**: stable, unique, cheap, and immune to mid-phase state changes.

Using the fairness key for these would be worse, not merely redundant. It couples RNG index assignment to mutable state — a balance that changes earlier in the same phase could reorder the batch — which is exactly the class of subtle nondeterminism this section exists to prevent. Seat index cannot move.

*(Note this is not the "seat order" the GDD deliberately removed from its tie-break chain. That removal was about a static ordering silently handing the same player every contested action for a whole match — a fairness problem. RNG batching confers no advantage by position, so the objection does not apply.)*

```go
// The only two orderings in the codebase. Everything sorts by one of them.
func byFairness(s State) []SeatID   // contended actions
func bySeat(s State) []SeatID       // RNG batches
```

**Every "the one with the least/most X" needs a documented tie-break too.** Lease surrender is the visible case, but it is one of five, and all five share a failure mode: the natural implementation ranges over a map, takes the first match, and answers differently on a different machine.

| Selection | Primary key | Tie-break |
|---|---|---|
| Debt surrenders a lease (GDD §13) | fewest rounds remaining | **lowest NodeID** |
| Autopilot picks a lease to renew | fewest rounds remaining | **lowest NodeID** |
| New Boss targets a player (GDD §14.2) | lowest RP | fairness key (§6.5) |
| Winner takes cargo in a 3+ way melee | the winner's choice | bot or default order: **lowest seat** |
| Raid targets a player (GDD §14.2) | highest Infamy | **none needed** — the GDD hits every tied player |
| **Losers of a 3+ way melee**, drawing `pushback.hop` | — | **seat index**, before any draw is consumed |
| **Several confrontations in the same movement step** | — | **node ID**, resolved one node at a time |

`NodeID` rather than acquisition order for the lease cases, deliberately: acquisition order is one more piece of cross-round state to carry and keep correct (§6.6), and it buys nothing. The choice is arbitrary either way; it only has to be stable.

The last two rows are RNG batches in the §6.5 sense and take seat and node index accordingly. They are called out separately because they are easy to miss: a free-for-all produces **two or three losers, each consuming one or two `pushback.hop` indices**, and the natural implementation walks the loser slice in whatever order the collision detector appended to it. Likewise, two confrontations at different nodes in the same step both draw, and nothing about the step loop imposes an order between them.

The row that will bite is Raid. It looks like it needs a tie-break and it does not — adding one would be a rule change wearing the costume of a determinism fix.

### 6.6 Cross-round derived state

`MatchState` carries per-seat counters that outlive a round. They are derived by the fold, live nowhere else, and are never read from or written to the database. Loitering surfaced this; enumerating the rest revealed eight.

| Field | Consumed by | Lifetime |
|---|---|---|
| `LastEndNode` | Loitering's 1-step radius test (GDD §9.1) | previous round |
| `LoiteringStreak` | Loitering escalation: silent → trace at 2 → global at 3+ | until broken |
| `LooseCrateHeldRounds` | Loose crate heat announcement at 2+ (GDD §8.4) | while carried |
| `Flagged` | −1 step from Debt (GDD §13) — boolean, never stacks | next round only, consumed from the entry snapshot below, not cleared by `upkeep()` |
| `EvasiveStepPenalty` | −1 step after an Evasive loss (GDD §15) | next round only, consumed from the entry snapshot below, not cleared by `upkeep()` |
| `LastOfferRound` | Contact Cooldown countdown (GDD §8.2) | until next offer |
| `DeadlinePauseUsed` | once-per-**contract**, so it lives on the contract instance, not the seat (GDD §8.4) | contract lifetime |
| `ConsecutiveDefaults` | Autopilot (§8.2) | until a real submission |

**One thing that looks like it belongs here and does not: previous-round balances.** The Ledger (GDD §5.1) reports balances *as of the end of the previous round*, and it resolves in the add-ons step — by which point deliveries, stakes and shakedowns have already moved money within the current round. So reading balances at add-on time gives the wrong answer, and the rule is quietly violated with no error anywhere.

The fix is not stored state. **"End of the previous round" is exactly the state entering `Resolve`**, so a snapshot taken at the top of the call is sufficient, and it is a local variable rather than another field to keep synchronised:

```go
func Resolve(s State, orders map[SeatID]Order, cfg Config, r *RNG) (State, []Event, error) {
    entry := s.Snapshot()   // frozen: end-of-previous-round truth
    …
}
```

That snapshot has **three** consumers, which is the argument for naming it explicitly rather than treating the Ledger as a special case:

| Consumer | Why it must read the frozen values |
|---|---|
| **Ledger** | GDD §5.1 specifies one-round staleness by design |
| **Step allowance** | Frozen at round start, or winning a fight on step 1 could shrink your allowance mid-route and invalidate a route the server accepted as legal |
| **Legend order-phase broadcast** | GDD §11.1 evaluates it as the order phase opens — which is this state |

The step-allowance row is the one with teeth. Without freezing, a +2 Infamy from an early confrontation could push a player from Known to Feared mid-resolution, cutting them from 4 steps to 3 halfway through a route they had already committed to.

**`Flagged` and `EvasiveStepPenalty` (table above) are consumers of this same snapshot, not separately-cleared state.** Both are step-allowance inputs (§9.1a), so they're read from `entry` exactly like the Infamy tier base — and the **live** copy is reset to `false` in the same top-of-`Resolve` step that takes the snapshot, before any resolution step (including a same-round Debt trigger or Evasive loss) can write to it. [D5](../decisions/D05-upkeep-phase.md) found this the hard way: an Upkeep-phase clear, at any position in the phase, can't distinguish "the value this round already used" from "a value this round just wrote for next round," because both can be true of the same field by the time Upkeep runs. Clearing at snapshot time — before either write is possible — is the only position immune to that.

Two further implementation notes that are easy to get wrong:

**The Vanish exemption does not need stored state.** Loitering's second condition asks whether a Vanish *actually reduced Infamy by at least 1* — but Vanish resolves in the Actions step of the same round, so the delta is known in-flight. What must be carried across rounds is only `LastEndNode` and `LoiteringStreak`. This does impose an ordering constraint: **Loitering is evaluated in `writeTrail`, after actions have resolved**, or the exemption is computed against a stale Infamy value.

**Most of these are silent when wrong.** A broken `LoiteringStreak` does not crash or corrupt anything — a rule simply stops firing, and nobody notices because the absence of a trail entry looks exactly like nothing having happened. Each row gets a dedicated test asserting the rule fires on the correct round, not merely that state advances.

### 6.7 Resolution pipeline

`Resolve` is a fixed sequence of small pure steps, each independently testable:

```text
validate(orders)              → reject illegal, degrade stale    GDD §15.0
  ↓
for step := 1..maxRouteLen:
    advance(all seats, step)
    detectCrossings()                                            GDD §15
    detectCollisions()          ← evaluates ALL positions,
                                  stationary seats included
    resolveConfrontations()     → pushback, displacement, deadline pause
  ↓
resolveActions()              → ascending Infamy, then balance, then RP, then coin
  ↓
resolveDeliveries()           → payments, Infamy, RP, global announcements
  ↓
resolveAddons()               → Ledger (one round stale), lease renewals
  ↓
writeTrail()                  → Loitering evaluation (AFTER actions, §6.6),
                                crate heat, per-node logs, then distribute by sight
                                and append to each seat's archive (§9.2)
  ↓
globalEvent() · incident() · pressure()
  ↓
upkeep()                      → fixed order, load-bearing (D5, GDD "Upkeep" in §15):
                                1. contract deadlines → penalty, discard, drop bound cargo,
                                   Debt cascade
                                2. lease decrement    → expire at zero, anchor row 4 (§9.1)
                                3. Sinkhole decrement  → clear at zero, no anchor
                                4. next-round modifier clear (Streets Blocked, Distracted
                                   Guard, Scaffolding, Retainer, Dockers' Strike, Blackout)
```

Every step emits `Event` values. Events are the substrate for the trail, the recap, email bodies, the debug trace, and telemetry — one representation, six consumers. `upkeep()` runs after every round, round 15 included, before final scoring (GDD §16) reads the resulting state — D5 settles this explicitly; there is no truncated final round.

**The collision check runs at least once per round** even when every route is empty, which is the boundary case GDD §15 calls out for a table where nobody moves.

**`upkeep()`'s four steps carry no fairness dimension** — a lease decrement confers no advantage by iteration position — so they take the §6.5 default: seat index across seats, a contract's assigned ID across a seat's own two slots, `NodeID` across map-scoped state (leases, Sinkholes). Only steps 1→2 have a real ordering constraint; steps 3 and 4 have none relative to the others and are fixed here purely so the sequence stays single-valued. `Flagged` and `EvasiveStepPenalty` are deliberately absent from this list — see §6.6. Full derivation in [D5](../decisions/D05-upkeep-phase.md).

---

## 7. Persistence

### 7.1 Event sourcing

**The order log is the truth. Match state is derived and never stored.**

```text
state = fold(Resolve, initial(seed, cfg), orderLog)
```

This falls out of the determinism requirement rather than being an independent choice, and it hands us four things for free: asynchronous resume, shareable replays, time-travel debugging, and the ability to answer "what sequence produced this?" when a player disputes an outcome.

### 7.2 Schema

```sql
-- identity
users(id, email UNIQUE NULL, display_name, is_guest, created_at)
auth_codes(id, email, code_hash, expires_at, consumed_at, attempts)
sessions(id PK, user_id, expires_at, created_at, last_seen_at)

-- matches
matches(
  id, status,                 -- lobby | active | finished | abandoned
  config JSONB,               -- the frozen Config (§6.2)
  seed BYTEA,
  round INT,
  timer_seconds INT,          -- sync tables
  deadline_seconds INT,       -- async tables
  deadline_at TIMESTAMPTZ,    -- current round's cutoff
  created_by, created_at, finished_at
)

match_players(
  match_id, seat, user_id NULL, bot_kind NULL,
  faction, joined_at, missed_deadlines INT,
  PRIMARY KEY (match_id, seat)
)

-- THE LOG
orders(
  match_id, round, seat,
  payload JSONB, submitted_at, source,   -- human | bot | default
  PRIMARY KEY (match_id, round, seat)
)

-- derived, rebuildable, kept for cheap reads
events(match_id, round, seq, kind, payload JSONB)
match_summary(match_id, round, submitted_seats INT[], updated_at)

-- email
outbox(id, to_email, template, payload JSONB, attempts,
       match_id NULL, round NULL, seat NULL,     -- null for otp
       send_after TIMESTAMPTZ, sent_at, last_error)
-- dedup is a PARTIAL index, not a plain constraint — see §13.1
```

`orders` has a primary key on `(match_id, round, seat)`, so resubmission during an open round is an `ON CONFLICT DO UPDATE` — which is exactly the GDD §18 rule that the last submission stands.

`events` and `match_summary` are **derived projections**. They exist so the lobby list and the recap don't refold every match on every page load. They carry a comment saying so, and there is a `cmd/replay --rebuild` that regenerates them. Nothing reads them as authority.

### 7.3 On not building a cache — with the arithmetic

A match is at most **75 orders** (15 rounds × 5 seats) and roughly **870 events**. A full fold is 15 calls to `Resolve`. Taking a generous 100 KB of allocation per call:

| Board renders per second | Allocation rate from folding |
|---|---|
| 1 | 1.5 MB/s |
| 10 | 15 MB/s |
| 100 | 150 MB/s |
| 1,000 | 1.5 GB/s |

Go is comfortable to somewhere around the 100 row. And 100 board renders per second is not a busy day — a player opens the board a handful of times across a 35-minute match, so that rate implies on the order of **two hundred thousand concurrent matches**. This is not the constraint, and the numbers are here so nobody has to take that on faith.

**Snapshots were proposed and are declined, for a reason beyond performance.** A snapshot is a second source of truth for state whose only source of truth is supposed to be the log — which is the exact bug class event sourcing exists to remove. It also has a subtler failure: a snapshot is only valid for the code and config that produced it. Change a rule, and every stored snapshot silently encodes the old one. Now you need a `code_version` in the key, an invalidation path, and a policy for what happens to a match mid-flight across a deploy. That is a lot of machinery bolted to the load-bearing wall of the system in exchange for allocation headroom that measurement says is not needed.

*(The proposal assumed a 30-round match with hundreds of events. Matches are 15 rounds — and GDD §16.2 shows why they cannot currently be longer.)*

**The trigger for revisiting**, so this is a measured decision rather than a preference: if p99 fold duration passes **50 ms**, or fold allocation exceeds **20% of total heap churn** (§17 tracks both), it gets built.

**And it gets built like this**, decided now so it is not invented under production pressure:

```text
snapshots(match_id, round, config_hash, code_version, state JSONB)
```

Keyed by config *and* code version; a miss falls back to a full fold rather than to stale state; and the sampled determinism check (§15.4) compares snapshot-accelerated folds against full ones, so a snapshot that has silently drifted is caught by the machinery already running rather than by a player.

**A cheaper step comes first if it is ever needed:** request-scoped memoisation. Several fragments may be rendered for one round — board, order form, Board panel — and folding once per request rather than once per fragment costs nothing in correctness because the memo dies with the request. That is not a cache; it has no staleness window at all.

### 7.4 Effects belong to the caller, never to the fold

`Resolve` is pure and returns `[]Event`. It is tempting to let the layer that computes state also dispatch the consequences — send the `autopilot` email when it derives the second consecutive default, push the SSE notification, write the telemetry row.

**That would be a live bug the first time anyone rebuilds.** `fold()` runs on every state read, on `cmd/replay --rebuild`, on the sampled determinism check (§15.4), and after every server restart. If the fold has an email provider anywhere in its call graph, each of those re-sends every historical notification in the match. Players get told they were put on Autopilot in round 6, again, on a Tuesday, four months later.

So: **events are data; effects are the caller's.**

```go
type Effects interface {
    Enqueue(ctx, OutboxRow) error
    Notify(ctx, matchID, SSEEvent) error
}

// The tick — the only caller that owns effects.
next, events, _ := rules.Resolve(state, orders, cfg, rng)
persist(orders, events)
for _, e := range project(events) {           // fog applies to notifications too (§13)
    fx.Enqueue(ctx, mailFor(e))               // same transaction as the log append
}

// Replay, rebuild, determinism check — same fold, null sink.
rules.Resolve(state, orders, cfg, rng)        // events discarded
```

Three properties make this safe rather than merely tidy:

- **Atomicity.** The outbox insert shares the transaction with the order and event append. Either the round advanced and the mail is queued, or neither happened.
- **Idempotency as a backstop.** A retried tick after a partial failure cannot double-send. The constraint needs care, though — see §13.1.
- **`rules` cannot reach a provider even by accident**, because it imports nothing outside the standard library and `internal/game` (§6.1) and the CI check enforces it. The compiler makes this class of bug unavailable rather than merely discouraged.

The rule generalises past email. SSE pushes, telemetry writes, analytics, webhooks — every effect in the system hangs off the tick, never off the fold.

### 7.5 sqlc and goose

`sqlc` generates typed methods from plain SQL in `internal/store/queries/`. No ORM: the queries are simple, and generating types *from* SQL rather than SQL from types keeps the schema honest.

`goose` migrations live in `internal/store/migrations/` and are embedded via `embed.FS`. They run at process start:

```go
func Migrate(ctx context.Context, db *sql.DB) error {
    // Concurrent instances will race on boot. Serialise them.
    if _, err := db.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
        return err
    }
    defer db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockID)

    goose.SetBaseFS(migrationsFS)
    return goose.Up(db, "migrations")
}
```

**The advisory lock is not optional.** Two instances booting simultaneously after a deploy will otherwise both try to apply the same migration, and goose's own version table will not save you from a partially-applied DDL. The lock costs one line and removes the failure entirely.

The process refuses to serve traffic until migration completes, and exits non-zero on failure so the orchestrator restarts rather than serving against a half-migrated schema.

---

## 8. The round tick

Two triggers advance a round: **everyone has submitted**, or **the deadline passed**. Both must fire exactly once.

```go
func (m *Manager) Tick(ctx, matchID) error {
    return m.tx(ctx, func(q *store.Queries) error {
        match, err := q.GetMatchForUpdate(ctx, matchID)   // SELECT … FOR UPDATE
        if err != nil { return err }
        if match.Status != Active { return nil }          // idempotent no-op

        humans := seatsOf(match, Human)
        submitted := q.SubmittedSeats(ctx, matchID, match.Round)

        ready := len(submitted) == len(humans)
        expired := time.Now().After(match.DeadlineAt)
        if !ready && !expired { return nil }

        orders := loadOrders(...)
        fillBotOrders(&orders, match)        // §14.2 — generated here, not submitted
        fillAbsentDefaults(&orders, match)   // GDD §18

        state := fold(matchID)
        next, events, err := rules.Resolve(state, orders, match.Config, rngFor(match))
        if err != nil { return err }

        persistOrders(orders); persistEvents(events)
        advanceRoundOrFinish(&match, next)
        enqueueNotifications(events)
        return nil
    })
}
```

Two entry points call it: the submit handler (after a successful write) and a sweeper goroutine.

```go
// every 5s: SELECT id FROM matches
//           WHERE status='active' AND deadline_at <= now() LIMIT 100
```

The row lock makes this safe with any number of app instances, and safe against a sweeper and a submit racing on the last order of a round. **No broker, no queue, no scheduler.** A ticker and a `FOR UPDATE`.

**Bot orders are generated inside the tick, not submitted ahead of time.** This is deliberate: it removes a race class entirely, and it guarantees bots see the same information a human would at the deadline rather than at round open. §14.2 has the timing consequence.

### 8.1 The deadline is authoritative, and it is the database's clock

A player submitting milliseconds before the sweeper acquires its lock must not get a variable grace period that depends on sweeper lag. The submit handler therefore checks the deadline **inside its own transaction, against `now()` from Postgres**:

```sql
-- inside the submit transaction
SELECT round, deadline_at, status FROM matches WHERE id = $1 FOR UPDATE;
-- reject if status <> 'active' OR now() >= deadline_at OR round <> $2
INSERT INTO orders (...) VALUES (...)
ON CONFLICT (match_id, round, seat) DO UPDATE SET payload = ..., submitted_at = now();
```

Three properties follow. The grace period is **zero and deterministic**, independent of how often the sweeper runs. Instance clock skew is irrelevant, because there is one clock and it belongs to the database. And a late submission is rejected with an explicit *"this round has closed"* rather than being silently swallowed — the player learns immediately instead of discovering it in the recap.

The submit and the sweep contend on the same row lock, so exactly one of them wins and the other observes the resolved state.

### 8.2 Autopilot is derived, not assigned

Autopilot could be implemented as a flag someone sets. It should not be. It is a **fact about the order log**:

```text
autopilot(seat) ⇔ seat has a user_id
                  AND the last two rounds both recorded source <> 'human'
```

**`<> 'human'`, not `= 'default'`, and the difference is the whole mechanism.** Once Autopilot engages, that seat's orders are produced by a bot and written with `source = 'bot'`. A streak counting `default` would therefore break on the very first Autopilot round — the system would disengage itself the round after it engaged, re-engage two rounds later when the defaults accumulated again, and flap for the rest of the match. Counting *absence of a human submission* is both the correct semantics and immune to how generated orders are tagged.

The `user_id IS NOT NULL` guard keeps filler bots (§14.2) out of it. Their orders are always `bot`; they are not absent humans and have nothing to return from.

No new column is needed — `orders.source` already carries `human | bot | default` (§7.2).

Deriving it rather than storing it buys three things that matter here. It replays correctly — refolding a match reproduces exactly when Autopilot engaged, which a mutable flag would not. It cannot drift, because there is no second source of truth to disagree with the log. And no authorisation rule is needed about who may set it: nobody sets it, so the question does not arise.

`match_players.missed_deadlines` remains as a denormalised read cache for the lobby list, carrying the same "derived, rebuildable" comment as `events` and `match_summary` (§7.2).

The `autopilot` email fires from the tick that produced the second consecutive default, off the same event stream as every other notification — from the **tick's effect sink**, never from the fold that derives the state (§7.4). A rebuild recomputes that the seat went to Autopilot and sends nothing. A returning player reclaims the seat on their next submission, which by definition breaks the two-consecutive-defaults condition and ends Autopilot without any state to unwind.

### 8.3 The sweeper gets its own connections

The sweeper and the HTTP handlers must not share a connection pool. The failure this prevents is a priority inversion that lands at exactly the wrong moment: **deadline approaching means peak submission traffic**, so the surge that exhausts the pool is the same surge that makes the tick urgent. The sweeper then blocks waiting for a connection, the round overruns, and every player sees a deadline that did not fire.

```go
appPool,   _ := pgxpool.NewWithConfig(ctx, appCfg)     // MaxConns 20 — handlers
sweepPool, _ := pgxpool.NewWithConfig(ctx, sweepCfg)   // MaxConns 3  — sweeper only
```

Three conns is enough because the sweeper is one goroutine doing one bounded query and one transaction per due match, sequentially.

**Timeouts belong on the sweeper connection, and the RFC did not have them:**

```sql
SET lock_timeout       = '2s';    -- never queue behind a long submit txn
SET statement_timeout  = '10s';   -- a wedged tick must not hold the row lock
SET idle_in_transaction_session_timeout = '15s';
```

A sweeper transaction that cannot take the row lock inside two seconds **gives up rather than waiting**. That is correct rather than merely defensive: failing to take the lock means a submit handler holds it, which means that handler is about to tick the same match. The sweep would have been redundant, and the next pass five seconds later catches anything genuinely stalled.

**Most ticks never touch the sweeper at all.** The last player to submit triggers the tick on their own request, so the sweeper only handles rounds that actually time out — which in synchronous play is rare and in asynchronous play is spread across hours. Sizing it small is not a compromise.

Sweep queries carry a jittered interval per instance so multiple app processes do not scan in lockstep.

---

## 9. Fog projection

### 9.1 What crosses the boundary

```go
type PlayerView struct {
    Round     int
    You       SelfState              // exact: balance, infamy, cargo, contracts, items
    Nodes     map[NodeID]NodeView    // Hidden nodes are ABSENT, not zeroed
    Others    []OpponentView         // band, infamy, RP, posts — never position
    Trail     []TrailEntry           // only nodes you had sight of
    Anchors   []Anchor               // public: deliveries, confrontations, stakings
    Headline  Headline
    Deck      DeckCounts             // hazards/boons remaining (GDD §14.3)
    Archive   SeatArchive            // YOUR history only — see §9.2
    NodeStats map[NodeID]NodeStats   // derived from Archive, for the Heat Map
}
```

Three implementation rules, each protecting a specific way this leaks:

**Hidden nodes are absent from the map, not present with empty fields.** A `NodeView{Type: ""}` is a leak: it tells the client the node exists and, worse, the size of the map. Absence is the only safe representation.

**Rumoured nodes carry no edges.** GDD §7.1 makes this a rule of the game; here it is also the mechanism. A Rumoured node ships with type, sector, and position, and its `Edges` field is nil — so the client physically cannot plot into it, and cannot infer the local topology from a contract destination.

**Opponent position information has exactly eleven authorised writers**, and the projection must implement all of them. The Board's anchored Attribution (GDD §7.5) is built entirely on this table; omit a row and the deduction layer has nothing to intersect. This list is checked against the GDD §7.3 trail table, row for row, whenever either document moves.

| # | Source | Distribution | Names the player? | Fixes a node? |
|---|---|---|---|---|
| 1 | **Cargo taken**, Infamy ≥ 3 | Sight-gated trail | Yes | Yes — the Warehouse |
| 2 | **Delivery** | **Global** | Yes | Yes — the Border |
| 3 | **Post staked** | **Global** | Yes | Yes |
| 4 | **Lease expired** | **Global** | Yes (owner) | Yes |
| 5 | **Loitering, 3+ rounds** | **Global** | **No** | Yes |
| 6 | **Loose crate held 2+ rounds** | **Global** | **No** | Yes |
| 7 | **Confrontation** | Sight-gated trail | Yes, both parties | Yes |
| 8 | **Item purchased**, Infamy ≥ 6 | Sight-gated trail | Yes | Yes — the Black Market |
| 9 | **Feared tier** (Infamy 6–8) | Global, end of round | Yes | Yes |
| 10 | **Legend tier** (Infamy 9–10) | Global, whole order phase | Yes | Yes |
| 11 | **Informants** event | Global, once | Yes | Yes |

**Row 1 is the most important line in this document and it was missing until r8.** A player at Infamy 3–5 is *Known*, and the Known tier does **not** reveal position — only Feared and above do. So a Known player taking cargo leaves a **named trace at a specific Warehouse while their position is otherwise entirely hidden**. That is not a redundant disclosure; it is the primary one, and it is the worked example the GDD uses to explain the whole deduction layer (§7.4: *"I see cargo taken at Docks Warehouse… I know you're crossing"*). Omitting it would have left Attribution with almost nothing to work from below Infamy 6, which is where most players spend the match.

**Row 8 is redundant, and is listed precisely because it is.** Item purchase names the buyer only at Infamy ≥ 6 — which means Feared or Legend, whose position is already published globally that same round by rows 9 and 10. The trace can therefore never disclose a node the reader did not already have, and it is sight-gated on top of that, so it reaches strictly fewer players than the reveal it duplicates.

It still belongs in the table for two reasons. A fog test suite asserting "no un-enumerated writer may name a player at a node" would flag it as a leak, and an implementer would then either weaken the assertion or delete a legitimate trace. And the trace is not *only* position: it discloses **that the player bought something**, which is inventory information the tier reveal does not carry. Redundant on one axis is not redundant on all of them.

Two distinctions the implementation must not blur:

**Global versus sight-gated.** Rows 2–6 and 9–11 reach every player unconditionally. Rows 1, 7 and 8 are trail entries: row 7 (confrontation) names both parties, rows 1 and 8 name one, but all three reach only players who had sight of that node. Treating a confrontation as global is a leak; treating a delivery as sight-gated breaks the anchor system.

**Named versus node-only.** Rows 5 and 6 announce a *node*, not a player — "someone has been standing here", "someone is holding the runner's crate". They are still position information, and the Board must surface them, but attribution has to treat them as an unattributed fix rather than a named one. Getting this wrong in either direction is a bug: naming the player is a leak, and dropping the entry removes a real deduction input.

```go
// The single place any of this may be written.
func writeAnchors(v *PlayerView, s State, seat SeatID) {
    // rows 2-6, 9-11: unconditional
    // rows 1, 7, 8: gated by hadSight(seat, node)
}
```

One function, eleven cases, one test per case. The fog suite (§16.3) asserts both directions for each: present when it should be, absent when it should not.

### 9.2 The observation archive

`PlayerView` as sketched above carries one round of trail. The Board needs the whole match: GDD §7.5 specifies a persistent log going back to round 1, and a Heat Map showing a **rate** — *"tracks on 4 of 6 rounds observed · 67%"* — with sample counts and a low-confidence flag under three observations.

That cannot be computed from a single round's view, and §7.3 rules out a cache to compute it from. So `MatchState` carries a per-seat archive, accumulated in `writeTrail` and derived by the fold like everything else:

```go
type SeatArchive struct {
    Sight map[NodeID]RoundSet    // which rounds you had sight of this node
    Trail []StampedTrailEntry    // every entry you have ever received
}

type NodeStats struct {
    ObservedRounds int   // |Sight[node]|
    TrafficRounds  int   // rounds within that set carrying a tracks entry
}                        // rate = TrafficRounds / ObservedRounds
```

**The `Sight` set is the part that is easy to miss.** A trail archive alone cannot produce the denominator, because *sight with no traffic* is itself an observation — "I watched this chokepoint for six rounds and nothing crossed" is exactly the evidence the rate is built from, and it leaves no trail entry behind. Storing only the entries would silently turn every rate into 100%, which is precisely the misleading raw-count reading that GDD §7.5 introduced the rate to avoid.

**This is fog-sensitive state living inside `MatchState`.** Seat A's archive must never reach seat B, so `Project` ships only the requesting seat's, and the fog suite tests it as it tests any other hidden fact.

Size is a non-issue: 25 nodes × 15 rounds is a 15-bit set per node per seat, and the trail archive runs to a few hundred entries per match.

### 9.3 Testing the projection

The projection has a dedicated test suite (§16.3) that asserts leakage negatively — for every hidden fact, a test that it is *not* reachable from the view.

---

## 10. WASM

> **Status: deferred to RFC-002.** With the map server-rendered (§11.2), legality is evaluated server-side on every click by the same `rules.Legal` that guards submission — so v1 needs no client-side rules engine at all. This section is retained because the reasoning holds and the work is planned; it is simply no longer on the critical path to launch.
>
> Deferring it removes an entire workstream from v1: a second build target, a size budget, a version-pinning mechanism (§10.6), and the client/server drift risk that comes with any duplicated engine.

The `rules` package compiles to WASM and ships to the browser. This gives the client the *same* legality and preview logic as the server, from one source.

### 10.1 What runs client-side

| Capability | Uses | Notes |
|---|---|---|
| Route legality | `rules.Legal` | adjacency, step budget, Hidden-node termination, Pushing On rules (GDD §15.0) |
| Step allowance | `rules.Steps` | the §9.1a formula, so the HUD number always matches the server's |
| Action legality | `rules.Legal` | is Deliver valid at this node, with this cargo, on this contract |
| Cost preview | `rules.Cost` | lease blocks, Ledger, shakedown reserve — with affordability against your own exact balance |
| Confrontation modifier | `rules.SelfModifier` | your own sum only; opponent terms are not in the view |
| Replay folding | `rules.Resolve` | post-match only (§10.3) |

### 10.2 Validation is not affordance

`Legal()` returning an error tells you a submission *was* wrong. That is the server's job. The client's job is to make the wrong submission unreachable, and several v2.0/v2.1 rules can only be honoured by **UI state**, not by post-hoc validation. A form that lets a player build an illegal order and then rejects it has already wasted their round-planning time.

| Rule | Required UI state |
|---|---|
| Pushing On forbids an action (GDD §9.1, §15.0) | Declaring blind steps **disables the action selector and forces it to Nothing**, with the reason shown. Not an error on submit. |
| A route holds at most one Hidden node, and it must be last (GDD §15.0) | Once the route enters a Hidden node, **all further node targets are unclickable** except the Pushing On control |
| Step allowance (GDD §9.1a) | The budget counter is live and derives from `rules.Steps`, including Flagged, Curfew, and the Evasive step loss — so the HUD number can never disagree with the server's |
| Evasive shakedown (GDD §15) | Selecting Evasive while carrying cargo with a balance under Cr$ 4 shows a **warning, not a block**: the policy will not pay out and the cargo is forfeit. Capped at balance, never triggers Debt or a lease surrender — the preview must not imply otherwise |
| Post cap (GDD §10.3) | Stake Post is disabled at the cap, showing which lease to let expire |
| Affordability | Lease blocks, Ledger, and stake are clamped to the exact balance the player can see for themselves |

The shakedown row is the subtle one. It is a **warning rather than a block** because going Evasive while broke is a legal and sometimes correct choice — you may simply be betting you will not be intercepted. The UI's obligation is that the player knows the policy has lapsed, not that the game refuses the choice for them.

**In v1 these are rendered as markup, not computed in the browser.** The server knows the draft order, evaluates `rules.Legal`, and returns the action selector already disabled with its reason attached. The table above is therefore a specification of *what the markup must express*, not of what WASM must compute — and it is satisfied by the server-rendered path with no client engine involved. When WASM lands in RFC-002 it takes over the same table to remove the round trip, and the rules do not change.

### 10.3 Shipping the rules is not shipping the state

The obvious worry is that sending the engine to the browser leaks something. It does not, and the distinction is worth being precise about: **code is not data.** The client already knows the rules — they are in the reference panel (GDD §19). What it must not have is other players' positions, hidden nodes, and the seed. None of those are in the binary.

Two obligations follow:

- **The seed never reaches the client during a live match.** It ships only with a finished match's replay bundle.
- Making the algorithm public is fine and always was. Anyone determined enough would reconstruct it from play. Security here rests entirely on the server owning the state and the seed.

### 10.4 Replay runs entirely client-side

A finished match's replay bundle is `{seed, config, orderLog}` — a few kilobytes. The browser folds it with the same WASM binary and scrubs through rounds locally. No replay endpoints, no server-side rendering of past states, no load from someone sharing a match on social media.

This only works *after* the match ends, because folding requires the full state. During a live match the client folds nothing.

### 10.5 Size, and the TinyGo question

Standard Go WASM will land around 2 MB uncompressed. Because `rules` is pure and uses no reflection, TinyGo is a genuine option and would bring that to a few hundred kilobytes.

**Recommendation: standard Go first, measure, revisit.** The binary is cached after first load and served with Brotli, and a 2 MB one-time cost against a 30-minute game is acceptable. TinyGo's constraints are real and debugging a TinyGo-specific miscompilation in the rules engine would be a genuinely bad week. Revisit if mobile-network first-load telemetry says it hurts.

**A hard requirement either way:** the server never trusts WASM output. Client-side legality is an affordance that prevents obviously-broken submissions and gives instant feedback. The server re-validates everything (GDD §15.0), and a malformed payload falls back to the absence default. This must be tested by submitting hand-crafted illegal payloads with the client bypassed.

### 10.6 Version pinning

A stale cached WASM binary disagreeing with the server produces the worst possible bug: orders that look valid and get rejected. The binary is served at a content-hashed path, the served build ID is embedded in the page, and the client refuses to submit if its build ID differs from the page's — prompting a reload instead.

---

## 11. HTTP surface

Server-rendered HTML throughout. HTMX swaps fragments; there is no JSON API.

```text
GET  /                          landing
POST /auth/request              email → issue OTP, enqueue mail
POST /auth/verify               code → session cookie
POST /auth/guest                display name → guest session (sync tables only)
POST /logout

GET  /matches                   your matches: whose turn, time left, unread recap
POST /matches                   create (config, player count, timer/deadline, bot seats)
GET  /m/{id}/join               invite link landing
POST /m/{id}/join               take a seat

GET  /m/{id}                    the match page (full render)
GET  /m/{id}/board              fragment: map + HUD          ← HTMX target
GET  /m/{id}/order              fragment: the order form     ← HTMX target
POST /m/{id}/order/node/{node}  append/remove a node from the route draft
POST /m/{id}/order              submit or resubmit; triggers Tick
GET  /m/{id}/recap              fragment: rounds since your last visit
GET  /m/{id}/board-panel        fragment: log, attribution, heat map
GET  /m/{id}/events             SSE stream
GET  /m/{id}/replay             finished only: {seed, config, log} bundle
```

**Fragment discipline.** Every fragment is a templ component taking `PlayerView` and rendering a self-contained `<div id="…">`. The full page render composes the same components. There is exactly one code path per piece of UI, whether it arrives as a page load or a swap.

### 11.1 The order form

The core interaction, and where HTMX earns its place. It is one `<form>` carrying:

```text
round           the round this form was rendered for  — see §11.1a
route[]         node IDs in order
blind_steps     0–2          (Pushing On)
bias_sector     enum
action          enum
action_target   optional
stance          enum
stake           0–6
items[]         {item, target?}    — GDD §9.4, up to the hand limit of 3
addons[]        ledger, renew:{postID}:{blocks}
abandon_cargo   bool
```

`items[]` carries an optional target: a node for Police Band and Decoy, a **pre-declared destination** for Bolt Hole. Torn Map, Guard Contact, Shiv and Circulation Permit take none. GDD §9.4 has the resolution timing — immediate discards land before movement, armed discards fire on their trigger and are spent either way.

Bolt Hole is worth flagging for the implementer: its destination *must* be declared in the order, because simultaneous resolution never pauses for input. If the declared node is unreachable when the item fires, the ordinary pushback rule applies and the item is still consumed.

Every field is a plain input. Resubmission is the same POST. Absence of JavaScript degrades to a working — if tedious — form, which matters more than it sounds: it means the async flow is testable with `curl`, and it means a broken WASM load does not brick the game.

### 11.1a Resubmission, retries, and recovery

The order form is a mutating POST that a player can fire more than once. Three cases, and only one of them is dangerous.

**Same round, repeated submission — already safe.** `orders` is keyed `(match_id, round, seat)` and the write is `ON CONFLICT DO UPDATE` (§7.2), so a second POST overwrites the first. That is not an accident of the schema; it *is* the GDD §18 rule that the last submission stands. A player editing their route four times produces one row.

**The dangerous case is a stale round.** A player submits for round 5, the tick fires, their connection drops, and they resubmit — from a form built against round 5, into a match now on round 6. The route was plotted from a position they no longer occupy. Nothing about the schema catches this, because it is a perfectly well-formed insert for the current round.

The fix is a field, not a table:

```text
round    the round this form was rendered for   ← required
```

The submit transaction already rejects on `round <> $2` (§8.1); it simply had nowhere to get the value from. A mismatch returns *"this round has closed"* and re-renders the board at the current round, so the player sees what changed instead of unknowingly acting on stale information.

**An idempotency key was considered and is not needed.** Same-round duplicates are handled by the upsert and cross-round staleness by the round field, which between them cover the whole space. A key would add a table, a cleanup job, and a second thing to keep correct, in exchange for nothing the two existing mechanisms miss.

**Reads always reconstruct.** There is no cached state to go stale (§7.3), so any `GET` on the board renders the current fold. A client that missed an SSE event and refetches gets the truth; recovery from a dropped connection is a page load.

**Browsers do not retry POSTs, and HTMX does not either.** The resubmission risk here is user-initiated — a refresh, a back-button, an impatient second click. It is handled by the round field plus swapping the form out of the DOM on success, rather than by anything defensive in the transport.

### 11.2 The map: server-rendered SVG

**The map is HTMX like everything else.** Earlier revisions of this RFC claimed otherwise — that plotting a route needed sub-100ms feedback and therefore a JavaScript island. That was wrong, and the error is worth recording because it nearly cost a milestone.

The sub-100ms requirement is real for **hover preview and dragging**. It is not real for **discrete clicks**. A player selects three or four nodes per round, inside a 60–90 second order window. Each click is a ~100ms HTMX swap returning the SVG with the path drawn and the step budget updated. For a turn-based game that is comfortable, and the original justification silently generalised from the expensive interactions to the cheap ones.

```go
// internal/render — takes the projection, returns markup. ~250 lines.
func Map(v rules.PlayerView, draft OrderDraft) templ.Component
```

Each node renders as a clickable region posting to the order draft:

```html
<g hx-post="/m/{id}/order/node/{nodeID}"
   hx-target="#board" hx-swap="outerHTML">
  <circle class="node known warehouse"/>
  <text>Docks Warehouse</text>
</g>
```

Four properties follow, and three of them are things the island would have cost us:

- **No JSON leaves the server.** r3 embedded a `PlayerView` blob in the page for the island to read. That blob is gone — the projection never crosses the wire as data, only as markup. One fewer surface to keep fog-safe.
- **No client/server rules duplication.** Legality is evaluated once, server-side, by the same `rules.Legal` that guards submission. The affordance rules in §10.2 are rendered as disabled markup rather than enforced twice.
- **It works without JavaScript.** The whole game, map included, degrades to plain forms. That makes the async flow testable with `curl` and means a failed asset load does not brick a match.
- **Rendering is Go**, so it is covered by the same type separation as everything else (§3) and the same test suite.

**What this defers rather than solves.** Pan, zoom, hover preview, layer switching, touch gestures, and the resolution animation all still want client-side code. They are the expensive parts, they were always the expensive parts, and they are RFC-002's subject. The v1 map is static per render, carries one overlay, and targets desktop.

### 11.3 Resolution as a narrated list

v1 does not animate resolution. It renders the round's outcome as a **narrated list of events, projected through the fog** — what happened to you, what your posts saw, what was announced globally.

This is the largest single deferral in the plan, and the cheapest to live with. It is also not throwaway work: the `round_resolved` email (§13) needs exactly this rendering, from exactly the same projected event stream. One implementation, two consumers, and the animation in RFC-002 becomes a presentation layer over a list that already exists and is already correct.

### 11.4 SSE

One stream per match. Events are small and mostly cause a fragment refetch rather than carrying data:

```text
event: submitted     data: {"seats": 3, "of": 4}
event: resolved      data: {"round": 7}
event: deadline      data: {"at": "…"}
event: finished
```

The client responds with `hx-get` on the affected fragment. This keeps all rendering server-side and means the SSE payload can never leak — it carries no game state at all.

An in-process hub holds subscribers per match. With multiple app instances a player might connect to an instance that did not resolve the round, so the hub also listens on Postgres `LISTEN/NOTIFY`, and the tick issues a `NOTIFY match_{id}`. Same mechanism, no broker.

### 11.5 The rendering contract

Three guarantees the backend owes RFC-002. They are cheap now and expensive to retrofit, so they are fixed in v1 even though nothing consumes them yet.

**1. `Event` never carries prose.** An event is `{kind, params}`, structured. It must not hold a rendered string like `"A cargo left here"`, because that string has already made two decisions it has no right to make: which language, and what to disclose. Localisation and fog filtering both belong at the render edge, with the `PlayerView` in hand.

**2. Events are emitted per movement sub-step, not as a round delta.** v1 renders a narrated list and does not need this granularity. The animation in RFC-002 does — it narrates *your* round and reveals traces as discoveries, which is impossible to reconstruct from an end-of-round state diff. Emitting the fine-grained stream now costs nothing; adding it later means reopening the resolution pipeline in `rules`.

**3. Attribution is computed client-side.** The projection therefore ships **raw anchors, each rival's Infamy history, and the requesting seat's observation archive** (§9.2), rather than pre-resolved candidate lists or a pre-computed Heat Map. This keeps the server stateless about deduction and lets the Board stay responsive when RFC-002 makes it interactive. It changes the shape of `PlayerView`, which is why it is decided here rather than later.

Nothing else about rendering is settled in this RFC. The remaining questions — screen budget, layer exclusion, colour carrying too many channels, whether noir atmosphere survives contact with legibility — are RFC-002's, and they are better answered after milestone 5 has been played.

---

## 12. Authentication

No passwords. Email OTP, which pairs with the notification infrastructure the game needs anyway — one sender, one template system, one deliverability problem.

```text
POST /auth/request   { email }
  → 6-digit code, store bcrypt hash + 10min expiry, enqueue mail
  → ALWAYS the same response, whether or not the account exists

POST /auth/verify    { email, code }
  → max 5 attempts per code, then burn it
  → rate limit per email and per IP
  → session cookie: HttpOnly, Secure, SameSite=Lax, 90 days
```

**Sessions are long — 90 days — on purpose.** Email is now on the login critical path, so login must be rare. A player who has to wait for an email every time they check an asynchronous match will stop checking.

### 12.1 Guests, and the GDD's no-signup promise

GDD §17 promises invite links with no mandatory signup. OTP requires an email. The resolution is a rule, not a compromise:

| Mode | Requirement | Reason |
|---|---|---|
| **Synchronous** | Display name only — guest cookie | Everyone is present; nobody needs notifying |
| **Asynchronous** | Email required | The deadline notification *is* the product |

A guest can bind an email later and keep their history. A guest session that expires loses its matches, and the join page says so plainly rather than discovering it later.

### 12.2 OTP threat notes

- Codes are single-use and burned on success, expiry, or fifth failure.
- The `/auth/request` response is identical for known and unknown emails; enumeration is not free.
- Rate limits: 3 requests per email per 15 min, 20 per IP per hour.
- Magic links are **not** offered alongside codes. Email clients prefetch links, which silently consumes single-use tokens and generates support tickets that are miserable to diagnose. A code the user types has no prefetch problem.

---

## 13. Email

Email is on the critical path for both login and gameplay. It gets an outbox, not an inline `send()`.

```text
handler → INSERT INTO outbox → worker goroutine → provider
                                  ↓ failure
                              exponential backoff, 5 attempts, then dead-letter
```

Sending inline would couple request latency to a third party and lose mail on a crash mid-request. The outbox is one table and one goroutine, and it makes delivery observable.

**Templates:**

| Template | Trigger | Contents |
|---|---|---|
| `otp` | login request | the code, expiry |
| `round_open` | async round opens | whose deadline, when, one-click resume link |
| `deadline_soon` | 25% of window remaining | only if not yet submitted |
| `round_resolved` | tick completes | what happened *to you*, from your projected events |
| `match_finished` | final scoring | standings, replay link |
| `autopilot` | 2 missed deadlines | you've been taken over, how to return |

**`round_resolved` is projected, not global.** The email is generated from the same `Event` stream filtered through the same fog — otherwise the notification becomes the leak that the entire architecture exists to prevent. This is the single easiest place in the system to accidentally send someone the whole board, and it deserves its own test.

### 13.1 Deduplication, and the two things it cannot do

The obvious constraint is `UNIQUE (match_id, round, seat, template)`. It does not work, for a boring reason and an interesting one.

**The boring reason: `otp` has no match.** Login mail is not match-scoped, so `match_id`, `round` and `seat` are all null, and Postgres treats nulls as distinct — every login row is unique and the constraint silently does nothing for the one template most exposed to retry storms. The fix is a partial index plus a separate guard:

```sql
CREATE UNIQUE INDEX outbox_match_dedup
  ON outbox (match_id, round, seat, template)
  WHERE match_id IS NOT NULL;

-- otp is rate-limited per email in §12.2, not deduplicated here:
-- a second login request is a legitimate second code.
```

**The interesting one: deduplication cannot fix a message that was correct when queued and wrong when sent.** `deadline_soon` fires at 25% of the window remaining, from the sweeper rather than the tick, and it says *you have not submitted yet*. A player who submits between the check and the send receives a false statement — and no uniqueness constraint helps, because the row was never a duplicate.

Two rules, therefore:

```sql
-- Insert only if still true, in the transaction that read the deadline.
INSERT INTO outbox (…)
SELECT … FROM matches m
WHERE m.id = $1 AND m.round = $2
  AND NOT EXISTS (SELECT 1 FROM orders o
                  WHERE o.match_id = m.id AND o.round = m.round AND o.seat = $3)
ON CONFLICT DO NOTHING;
```

And **the worker re-checks time-sensitive templates at send time**, discarding a queued `deadline_soon` whose round has since closed or been submitted. Anything that asserts a fact about *current* state gets this treatment; `round_resolved` and `match_finished` describe the past and never need it.

**On the Autopilot case specifically:** a player who returns shortly after the `autopilot` mail was queued still receives it, and that is correct — it did happen, and the message tells them how to come back. It describes a past event, so it is not re-checked. What must not happen is the mail firing *without* the round having advanced, and that is guaranteed by the shared transaction in §7.4 rather than by any constraint.

**Volume control.** An async match with a 24h deadline generates up to 15 `round_resolved` emails per player. That is a lot of mail. Per-match preferences: *every round* / *only when it's my turn and I haven't moved* / *daily digest* / *none*. Default is the second. Every email carries one-click unsubscribe per match.

Provider: Resend or Postmark for v1 behind a `Sender` interface. SES is cheaper at volume and worse at deliverability out of the box; it is the migration target, not the starting point.

---

## 14. Bots

Bots serve four roles from one implementation.

```go
type Bot interface {
    Decide(v rules.PlayerView, cfg rules.Config, r *rules.RNG) rules.Order
}
```

### 14.1 The fog boundary, enforced by construction

**A bot receives `PlayerView` and nothing else.** Not `MatchState`, not the graph, not opponent positions. This is not only fairness — it is an executable test of the projection.

If a bot cannot be written competently against the view, the view is missing something a human also needs. That failure mode is otherwise very hard to notice: a human player will assume they are bad at the game rather than that the UI is starving them. The bot surfaces it as a coding problem.

`Decide` is pure and takes the seeded RNG, so a bot-populated match replays exactly like any other.

### 14.2 Four roles

**Filler bots** take seats to complete a match. A table of two humans can play a five-seat game.

- **Disclosed in the lobby and in the match HUD.** Hiding which factions are bots is a trust problem, and it is the kind of thing that, once discovered, poisons a player's view of every match they have played.
- Bot orders are generated **inside the tick** (§8), never submitted at round open. Consequence: in an asynchronous match, bots do not collapse the round the moment it opens. A round still runs its full window unless every human has submitted.
- Filler bots count as seats for `PostCapByPlayers` and map sizing, and are excluded from the "everyone submitted" readiness check.

**Autopilot** takes over an absent human after two missed deadlines (GDD §18). Same code, `Runner` tier, and the seat is marked recoverable — a returning player takes it back on their next visit.

**Simulation bots** run headless in `cmd/simulate` (§16.4).

**Practice bots** are the opponents in solo play (§14.4).

### 14.3 Difficulty tiers

| Tier | Behaviour |
|---|---|
| **Drifter** | Uniform random legal order. Baseline for statistics and for the property tests. |
| **Runner** | Greedy. Shortest path to the current contract objective, Evasive while carrying, keeps Cr$ 4 in reserve for the shakedown, never buys, Vanishes when Infamy exceeds its comfort band. The autopilot default. |
| **Operator** | Plans across rounds. Reads the heat map for chokepoints and leases them, routes around unstable sectors weighted by the displayed deck counts, times the Infamy climb against Contact Cooldown, buys items when a confrontation looks likely, uses the Ledger when a rival's band jumps. |

Filler bots default to **Runner**, with the tier selectable at match creation.

**Operator is deliberately not superhuman.** It plays the game the way the GDD says the game is meant to be played, and if it beats every human easily that is a balance finding, not a bot achievement. Its win rate against humans is a telemetry metric, not a target.

### 14.4 Solo play

Solo is not a mode. It is an ordinary match with **one human seat, the rest filled by bots, and the order timer set to unlimited** — no deadline sweeper, no email, no SSE beyond the local one. Once milestone 4 exists, solo is configuration.

Four things do need building, and all four are cheap:

**Scenarios are data.** Each of the five scenarios in GDD §19.1 is a row: node count, round count, `Config` overrides, bot seats and tiers, and which subsystems are suppressed. Scenario 1 disables leases, incidents, items and Infamy tiers; scenario 2 re-enables posts and the trail. Suppression is expressed as `Config` flags, not as branches in `rules` — the same discipline the city catalogue would need, and for the same reason.

**Scenarios pin a fixed seed.** The map, the contract offers and the event order are identical for every player who runs a given scenario. This buys three things at once: contextual tips can be attached to specific rounds and specific nodes, the tutorial becomes testable as a golden replay, and two players can compare notes on the same board.

**Difficulty is tier selection.** Drifter, Runner, Operator — the same implementations as §14.3, with no bonuses, no fog exemptions, no starting advantage. A bot with hidden advantages would break the property that makes solo useful as a testbed and would teach strategies that do not transfer.

**Solo telemetry is tagged separately.** Solo matches emit the same GDD §22 metrics, and every row carries an `opponents=bots` flag. Mixing them into the human-versus-human set would corrupt exactly the numbers R1, R9, R11 and the lease rate depend on — and would do it invisibly, because the metrics would still look plausible.

Scenario reset costs nothing: truncate the order log and refold (§7.1). Retrying a tutorial stage is a delete, not a state rollback.

**Solo does not need auth.** A guest session (§12.1) is sufficient, and this is the natural first experience — play the tutorial, then supply an email only when you want an asynchronous match.

### 14.5 What bots must never do

- Read `MatchState`, the graph, or the seed.
- Coordinate with each other. Each `Decide` sees only its own view. Two bots in the same match colluding would be undetectable and unfair, and the type signature prevents it.
- Submit outside the tick.

---

## 15. Debug mode

### 15.1 It is a build tag, not a flag

```go
//go:build debug
```

Debug code is **not compiled into the production binary**. A runtime flag is one environment-variable mistake away from serving a god view of live matches, and that mistake is unrecoverable — you cannot un-leak a map.

```text
make dev    → go build -tags debug
make prod   → go build            (debug routes do not exist)
```

CI asserts that the production binary contains no debug symbols, by building both and diffing the route table.

### 15.2 The fog inspector

The most valuable tool in the set, because **fog bugs are invisible from the inside.** A projection that leaks looks completely normal to the developer playing the game.

The inspector renders every seat's `PlayerView` side by side against the true `MatchState`, with a diff view flagging any fact present in a view that the fog rules say should not be there. It is the first thing to open when a projection changes.

### 15.3 The rest of the panel

| Tool | What it does |
|---|---|
| **God view** | Full `MatchState`: all positions, all cargo, all contracts, all hands |
| **RNG trace** | Every draw with its sequence index, purpose string, inputs, and result |
| **Step trace** | Resolution replayed sub-step by sub-step, showing state deltas |
| **Time travel** | Jump to round N by refolding a prefix of the log |
| **Order injection** | Submit an arbitrary order as any seat, including illegal ones — this is the test harness for §15.0 |
| **Force draws** | Pin the next event card, incident, unstable sector, market stock, or contract offer |
| **Config live-edit** | Change a dial mid-match; the match forks to a new ID so the original replay stays valid |
| **Telemetry live** | Every GDD §22 metric computed against the running match |
| **Speed run** | Auto-resolve N rounds with bots, for reaching a late-game state in seconds |

### 15.4 Debug artefacts in production

Two things survive into the production build because they are diagnostics, not god views:

- **Match export.** `{seed, config, orderLog}` for a *finished* match, downloadable by its players. It is the replay bundle (§10.4), and it is also the perfect bug report: attach it to an issue and `cmd/replay` reproduces the exact match.
- **Determinism check.** After each tick, in a sampled fraction of matches, refold from scratch and compare to the incrementally computed state. A mismatch is logged loudly with the match ID and round. This is how a map-iteration bug (§6.3) gets caught in days instead of months.

---

## 16. Testing

### 16.1 Layers

| Layer | Approach |
|---|---|
| `rules` unit | Table-driven. Every GDD worked example becomes a test case verbatim — the Legend/Tier IV margin (§15), the pushback table, the step formula (§9.1a), the Vanish/Legend precedence (§11.1). |
| `rules` property | Randomised orders; assert invariants hold for every reachable state. |
| Golden replays | Recorded `{seed, config, log}` fixtures with expected final state. Any unintended rule change fails these. |
| Fog | Negative assertions (§16.3 below), plus one positive and one negative test per row of the eleven-writer anchor table (§9.1). |
| Integration | Real Postgres in a container. Full match through the HTTP layer. |
| Concurrency | Two goroutines submitting the last order of a round; assert exactly one resolution. Submit at `deadline_at ± 1ms` against a real Postgres clock; assert the reject/accept boundary is exact and that Autopilot engages on the correct round (§8.1, §8.2). |
| Effects | **Fold a finished match ten times; assert `outbox` gains zero rows.** The single most valuable regression test in the suite (§7.4). |
| Cross-round state | One test per row of §6.6, asserting the rule **fires on the correct round** — not merely that the counter advances. These fail silently otherwise. |
| Archive | A node watched 6 rounds with traffic in 4 reports 4/6; a node watched with *no* traffic reports 0/N, not absence (§9.2). |
| Lazy RNG | Truncate a Pushing On walk at a Gas Leak boundary; assert the unrun steps consumed zero indices (§6.4). |
| Torn Map | Fewer than 4 hidden nodes remaining; assert exactly `min(4, hidden)` indices consumed and no duplicate reveal (§6.4). |
| Tie-breaks | Two leases with identical rounds remaining; assert the same one is surrendered across 100 runs and on a rebuilt state (§6.5). |
| Autopilot persistence | Engage Autopilot, then run five further rounds with no human submission; assert it stays engaged and does not flap (§8.2). This is the test that would have caught it. |
| Melee pushback order | Three seats collide, two lose, both stationary and Evasive; assert `pushback.hop` indices are consumed in seat order and the replay is byte-identical (§6.5). |
| Entry snapshot | Win a confrontation on step 1 as a Known player; assert the step allowance for that round does **not** drop to Feared mid-route, and that a Ledger bought the same round reports pre-round balances (§6.6). |
| Anchor parity | Assert the §9 table and the GDD §7.3 trail table agree row for row. A table-driven test over both lists, so a change to either fails the build. |
| Known-tier pickup | A player at Infamy 4 takes cargo; assert the trace names them and that their position is otherwise absent from every rival's view (§9, row 1). |
| `deadline_soon` race | Submit between the sweeper's check and the send; assert no mail leaves (§13.1). |
| Outbox scoping | Two `otp` rows for the same email do not collide; two `round_resolved` rows for the same seat and round do (§13.1). |
| Headline coherence | For every round, assert the category printed at Phase 1 matches the card popped at Phase 6, and the sector matches the incident at Phase 7 (§6.4). |
| Batch ordering | Two Legends with identical Infamy and balance; assert Pressure draws consume indices in seat order and that swapping their balances mid-phase does not reorder them (§6.5). |
| Adversarial | Hand-crafted illegal payloads with the client bypassed, one per row of GDD §15.0. |

### 16.2 Invariants for property tests

Drawn directly from the GDD, and each corresponds to a rule that would otherwise fail silently:

```text
balance >= 0                            always (GDD §13)
steps   >= 1                            always (GDD §9.1a)
cargo   <= 1 per player                 always
posts   <= cap(playerCount)             always
every position is a valid node
no player is on a Hidden node they could not reach
sum(credits in play) changes only via defined sources
resolve(s, o) == resolve(s, o)          same seed, twice, byte-identical
rng.seq consumed == predicted            per the §6.4 table, asserted per round
fold(log) == fold(log)                  refold stability
```

### 16.3 Fog tests are negative

The valuable assertion is not "the view contains X" but **"the view does not contain Y."** For a state constructed so that seat A cannot see node N:

```go
v := rules.Project(s, seatA)
require.NotContains(t, v.Nodes, nodeN)        // absent, not zeroed
require.Nil(t, v.Nodes[rumouredNode].Edges)   // Rumoured carries no topology
for _, o := range v.Others {
    require.Zero(t, o.Position)                // unless Feared/Legend/Informants
}
```

Serialise the view to JSON and assert that hidden node IDs do not appear anywhere in the bytes. That catches leaks through fields nobody thought to check, which is how leaks actually happen.

### 16.4 The simulation harness

`cmd/simulate` runs matches headlessly and emits the GDD §22 metrics as CSV.

```bash
simulate --matches 10000 --players 4 --bots operator \
         --sweep LeaseCostPerBlock=1,2,3,4,5 --out sweep.csv
```

**This answers the open questions the GDD could not.** R1 (cancelled routes), R9 (encounters per match), R11 (endgame camping), and above all the lease rate — the dial §10.4 calls the most sensitive number in the game.

**Build this before the UI.** It needs only `rules` and `bots`, both of which are prerequisites for everything else, and it can be running parameter sweeps while the web layer is still being written. A weekend of sweeps is worth more than a month of arguing about the lease rate.

Caveat worth writing down: bot play is not human play, and a sweep tells you about the *shape* of the parameter space — where a dial flips a strategy from dominant to dead — not the exact right value. It narrows the range that paper testing then confirms.

---

## 17. Observability

`slog` to stdout as JSON, with `match_id` and `round` on every game-related line.

**Metrics that matter operationally:** tick duration, **fold duration (p50/p99)**, **fold allocation as a share of heap churn**, outbox depth and age, deadline sweeper lag, **sweeper pool saturation and lock-timeout count**, SSE subscriber count, determinism-check mismatches.

The two fold metrics exist to make §7.3's no-cache decision falsifiable rather than permanent — they are the trigger, and without them the question would be reopened on vibes every few months.

**Metrics that matter for design** are the GDD §22 set, computed from the event stream and written to an analytics table on match completion. The same computation runs in `cmd/simulate`, so bot data and human data are directly comparable — which is the entire point of having both.

The one alert worth waking someone for is a **determinism-check mismatch**. Everything else can wait for morning; that one means replays are lying.

---

## 18. Deployment

Single static binary, `embed.FS` for templates, static assets, WASM, and migrations. Postgres. That is the entire production topology.

```text
docker build → one binary + one image
env: DATABASE_URL, MAIL_PROVIDER_KEY, BASE_URL, SESSION_KEY
start: migrate (advisory lock) → serve
```

Fly.io, Railway, or a VM with systemd all work. Two app instances behind a load balancer is the ceiling this design needs; the row lock (§8) and `LISTEN/NOTIFY` (§11.3) already handle that case.

**Backups matter more than usual.** The order log is irreplaceable — lose it and matches cannot be reconstructed, because there is no state table to fall back on. Point-in-time recovery, tested by actually restoring, not by assuming.

**Graceful shutdown:** stop accepting new connections, let in-flight ticks finish, close SSE streams with a retry hint. A tick killed mid-transaction rolls back and is retried by the sweeper, so the worst case is a delayed round rather than a corrupted one.

---

## 19. Security notes

| Surface | Handling |
|---|---|
| State leak | §3, §9, §16.3 — the primary threat, and the reason for the whole architecture |
| Forged orders | Server re-validates everything; WASM is never trusted (§10.5) |
| Seat impersonation | Session → seat mapping checked per request, never taken from the payload |
| Replay-before-end | The replay bundle is served only for `status='finished'` |
| OTP brute force | 5 attempts per code, rate limits per email and per IP (§12.2) |
| Enumeration | Identical response for known and unknown emails |
| CSRF | Same-site cookies plus a token on every mutating form |
| Invite links | High-entropy, revocable, single-match scope |
| Timing | Constant-time comparison on OTP hashes and session tokens |

---

## 20. Open questions

**Q1 — TinyGo for the WASM binary.** 2 MB versus a few hundred KB. Recommendation: standard Go, measure first-load on mobile, revisit only if the data says so (§10.5).

**Q2 — What does the map need once RFC-002 adds interaction?** v1 has no client-side map code at all (§11.2), so this is genuinely open rather than half-decided. Pan, zoom, hover preview and layer switching could be a few hundred lines of vanilla JS over the same server-rendered SVG, or could justify a small framework. Deciding it before milestone 5 has been played would be guessing.

**Q3 — Order-form UX in email.** A one-click "submit last round's plan again" link would help asynchronous retention, but it means a mutating action from an email link. Probably a signed single-use token, probably v1.1.

**Q4 — Anonymous filler bots in ranked play.** Disclosure is right for casual tables (§14.2). If competitive play appears later, the question of whether bot-filled matches count at all needs an answer, and the answer is probably no.

**Q5 — Multi-region.** Not needed. Noted only so nobody adds it speculatively; the row-lock model would need real thought first.

**Q6 — Guest session loss.** A guest who clears cookies loses their matches. Acceptable for synchronous play, but the join page must say so before they invest 30 minutes.

---

## 21. Build order

Each milestone is independently demonstrable, and the sequence is chosen so the riskiest unknowns resolve first.

| # | Milestone | Contents | Proves |
|---|---|---|---|
| **1** | `rules` core | State, Order, Resolve, Project, Config, RNG, map gen. Unit and property tests. | The game is implementable and deterministic |
| **2** | Bots + simulation | Three tiers, `cmd/simulate`, GDD §22 metrics as CSV | **Answers R1, R9, R11 and the lease rate before any UI exists** |
| **3** | Persistence | Schema, sqlc, goose, fold, `cmd/replay` | Matches survive restarts and reproduce exactly |
| **4** | Round lifecycle | Tick, deadline sweeper, bot filling (§14.2), absence defaults, autopilot | A full match runs end to end with no browser |
| **5** | **Playable web — this is what ships** | templ, HTMX, auth, lobby, order form, server-rendered SVG map, narrated resolution, Board as tables, one overlay | People can play the game |
| **6** | Async | Email, outbox, notifications, recap | The mode that makes the product distinct |
| **7** | Onboarding | Solo scenario ladder (§14.4), reference panel, contextual tips | People can *learn* the game without spending someone else's 35 minutes |
| **RFC-002** | Interface | Pan/zoom, hover, layer switching, animation, touch, WASM, replay viewer | The game becomes pleasant |

### v1 scope, stated as exclusions

Everything below is deliberately absent from launch and belongs to RFC-002:

| Deferred | Substitute in v1 |
|---|---|
| Resolution animation | Narrated event list (§11.3) |
| Pan, zoom, hover preview | Static SVG per render, fit to viewport |
| Layer switching | One overlay — Heat Map, per GDD §7.5 |
| Attribution cones | Anchor table with candidate seats listed |
| Client-side rules (WASM) | Server-side evaluation per click (§10) |
| Touch, 380px layouts | Desktop-first; mobile usable but unpolished |
| Replay viewer | Downloadable `{seed, config, log}` bundle |

**The former milestone 5 was scaffolding; it is now the product.**

**On shipping something lean rather than something textual.** An early plan for v1 was a text-only build, with graphics as a second phase. Rewriting §11.2 removed the reason for it: once the map is a server-rendered SVG, the graphical version *is* the textual version plus 250 lines of Go, sharing the same handler, form, and projection. There is no rewrite between them, so there is nothing to save by skipping it.

There was also a reason not to. **The graph is the board.** P3 rests on reading it and inferring routes, and while the log, the heat map, and attribution all work fine as tables — a heat map sorted by rate is arguably clearer than a picture — "this chokepoint sits between Iron Low and North Vale" is spatial intuition. Testing the game's central pillar in the one medium where that pillar cannot function would have produced a confident and worthless result.

What ships is therefore lean rather than textual: the map is there, and everything expensive around it is not.

**Milestone 5 also became the research for RFC-002.** Writing an interface document without having played the game in any interface is speculation. Twenty matches on the lean build tell you which information you kept hunting for, which you ignored, and where the screen budget actually binds — worth more than any wireframe drawn beforehand.

**Milestone 2 is the one worth insisting on.** It is tempting to skip to something visible, but the GDD leaves several numbers explicitly unresolved and pins them on measurement. Building the measuring device second — before it can be rationalised away — is what makes those questions get answered instead of guessed.
