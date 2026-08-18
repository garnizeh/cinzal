# D32 — Do bot draws consume the match RNG stream, and what is their row in the §6.4 table?

**Status:** decided
**Blocks:** [#190](https://github.com/garnizeh/cinzal/issues/190) (the `Bot` interface — the third parameter's contract is unwritable without this), [#192](https://github.com/garnizeh/cinzal/issues/192)/[#193](https://github.com/garnizeh/cinzal/issues/193)/[#194](https://github.com/garnizeh/cinzal/issues/194) (the three tiers), [#201](https://github.com/garnizeh/cinzal/issues/201) (golden replays and index accounting for bot-populated matches), [#199](https://github.com/garnizeh/cinzal/issues/199)/[#200](https://github.com/garnizeh/cinzal/issues/200) (`cmd/simulate`, whose sweep numbers are only worth anything if reproducible), and the RFC/GDD documentation catch-up, [#202](https://github.com/garnizeh/cinzal/issues/202) (bundled with D33/D34, landing after all three are decided)
**Decided:** 2026-08-17
**Issue:** [#186](https://github.com/garnizeh/cinzal/issues/186)

## The question

RFC §14's `Bot` interface takes the engine's seeded RNG:

```go
type Bot interface {
    Decide(v rules.PlayerView, cfg rules.Config, r *rules.RNG) rules.Order
}
```

(This decision fixes the exact type of that third parameter — see Decision below; the signature above is the RFC's literal text today, and D32 corrects it to `*rules.BotRNG`.)

RFC §6.4's consumption table — 34 rows deep after [D03](D03-rng-consumption-table.md), enumerating every consumer from resolution, both decks, and all nine `rules/gen` steps — has no row for it. `grep -in "bot" docs/project/cinzal-architecture-rfc.md` returns nothing anywhere near §6.4. Three sub-questions, answerable only together:

1. Do bot draws land in the match's `seq` stream, or in a stream of their own?
2. If they land in the match stream, what is the row — given that Drifter's draw count is a function of the legal order space's size and Operator's is a function of how many branches its search evaluated, neither one a fact the *rules* determine rather than the bot's own implementation?
3. RFC §8.2 derives Autopilot from `source <> 'human'`, meaning bot orders are written into the order log. A refold reads them from the log and never calls `Decide`. What does that do to the first two answers?

## Why it is open

**The one consumer the RFC hands an `*RNG` to in its own type signature is the one consumer §6.4 never mentions.** That is exactly the shape of the r1 omission §6.4 itself warns against ("r1 accounted for Pushing On and missed pushback entirely"), and it is worse here: pushback's index cost is at least derivable from the rules as written. A bot's is derivable only from the bot's own implementation — and Runner and Operator do not exist yet to derive it from.

**If bot draws shared `Resolve`'s stream, #81 stops being assertable the way M1 left it.** M1 closed on "RNG index counts match the §6.4 table, including truncation cases" (#81), which is one specific, checkable property: for a fixed round, the table predicts an exact draw count. If `Decide` drew from the same `*RNG` `Resolve` threads through resolution, the per-round count would depend on how many seats were bot-filled that round and which tier each one is — Operator's especially, since its cost is "however deep the search went," not a formula. Worse, per RFC §8.2, Autopilot engagement is a fact about the order log's last two rounds, not a stored flag: a seat can be bot-driven for rounds 1–7 and hand-driven for 8–15 within one match. The number of `Decide` calls in round 7 would then be a property of *when a human stopped submitting*, which is not in `{seed, order log}` — a refold of that exact match would consume a different number of match-stream indices than the live run did, and every draw after the divergence point would differ. RFC §7.1 (`state = fold(Resolve, initial(seed, cfg), orderLog)`) would be false for any match with a bot in it.

**A separate stream needs a seed, and that seed has to be reproducible without help from a human.** `cmd/simulate` (§16.4) has no order log and no humans; every order in a sweep comes from `Decide`, and the sweep's numbers are only worth anything if two runs from the same seed produce the same CSV. `internal/rules/golden_test.go`'s own precedent for this exact shape — a fabricated round index (`NewRNG(seed, int(round)+100000)`) used to keep Phase 2's contract-offer draws off `Resolve`'s stream before [D29](D29-phase-2-3-fold-attachment.md) gave them a real position — was flagged in D29 itself as a workaround for an open question, not an answer to one. This decision is that open question, for a different consumer.

**The replay path may make most of this moot for the live game, but not for the simulator.** §8.2's derivation means a bot order, once written, is `source = 'bot'` in the order log exactly like a human order is `source = 'human'`. A refold calls `Resolve(state, orders, cfg, rng)` once per round over the logged orders — it never calls `Decide` at all. Under that reading, `Decide`'s draws are not part of what a replay reproduces; they are part of how an order was *authored*, the same way a human's mouse clicks are not part of what a replay reproduces, only the order they produced is. That reading is attractive for the live game and does not, by itself, answer anything for `cmd/simulate`, which has no log to read orders back from and needs the whole sweep reproducible from a seed alone.

## Options

**A — Bot draws share the match `seq` stream, with a table row per tier.** One stream, one seed, nothing new to derive; bot play would be visible in the RNG trace (§15.3) alongside every other consumer. Against: §6.4's table already tolerates variable, input-dependent counts — #77's predictor/exemption split (`determinism_test.go`'s `consumptionPredictors`/`consumptionExemptions`, checked complete by `TestConsumptionPredictorsAccountForEveryNonSetupRow`) means a row need not state a literal constant. But every existing predictor and every existing exemption is still a function of *rules-known* facts — hidden-node count, a candidate pool's size, a cascade's own prior draw — derivable in principle even where a test declines to re-derive it for cost reasons. A bot's count is a function of the bot's own algorithm, which the rules say nothing about and which can change with no GDD/RFC edit at all: there is no ground truth for a predictor to check itself against, and an "exemption" that can never be discharged by definition isn't the same thing #77's exemption list already contains. Decisively, independent of that argument: Autopilot handover makes per-round consumption a function of *when a human stopped submitting*, which directly negates §7.1. #81's assertion would narrow to bot-free matches, which is most of M1's suite but none of M2's.

**B — A separate bot RNG, derived deterministically from `(match seed, seat)`, constructed fresh per round the same way `Resolve`'s own `*RNG` already is.** `Resolve`'s stream stays exactly as M1 left it; #81 keeps its full strength, unconditionally; `cmd/simulate` stays reproducible from a seed with no log; two bots in the same round cannot influence each other's draws, because they never share a stream — RFC §14.5's non-collusion rule enforced by construction, not by review. Against: a second RNG-construction path in the system, and a second, deliberately unenumerated purpose namespace that needs its own (small) documentation, not a §6.4 row.

**C — `Decide` is not part of the replayed computation at all; bot orders are logged like human ones.** Matches §8.2's Autopilot derivation exactly, and makes the live/replay asymmetry explicit instead of accidental. Against: says nothing about `cmd/simulate`, which has no log — C needs B underneath it to make sweeps reproducible, at which point C is a clarification riding on B's answer, not a competing option.

## Decision

**B, with C's clarification stated alongside it as the reason B's seed does not need round baked into its derivation.**

**The stream.** Bot draws never touch `Resolve`'s `*RNG`. A bot-seat's draws for a given round come from a dedicated `*rules.BotRNG` — a distinct type, not `*RNG` itself — constructed fresh for that `(seat, round)` pair and discarded after the one `Decide` call. Same per-round-instance lifecycle `rng.go`'s own doc comment already states for `Resolve`'s RNG ("bound to one round... constructs a fresh one, seeded identically, at the start of every round"), applied to a second, independent seed, behind a type that cannot be confused with the first.

**Why a distinct type, not a second mode on `RNG`.** `Decide`'s third parameter has to make it structurally impossible to hand a bot `Resolve`'s own live `*RNG` — not merely undocumented to do so. If `Decide` took `*RNG` and drew via a `NextBot` method added to that same type, nothing would stop a miswired call site from passing the round's real `*RNG` straight through: `NextBot` would advance that instance's `seq` without touching `consumed map[Purpose]int`, silently shifting the position of every subsequent `Next` call in `Resolve`'s own stream and breaking §16.2's `rng.seq consumed == predicted` invariant in a way no test scoped to `Resolve` alone would catch, since the corruption originates outside it. That is exactly the class of bug §6.4 opens by naming — invisible until a replay disagrees, months later. A distinct type closes it at compile time instead: `BotRNG` exposes only the bot-facing draw method, `RNG` exposes only the match-facing one, and passing one where the other is expected is a type error, not a review item.

**The type and its constructor**, in `rules`, beside `RNG` in `rng.go`:

```go
// BotRNG is the stream Bot.Decide draws from — never Resolve's own *RNG.
// A distinct type, not a mode flag on RNG: BotRNG has no Next method and
// RNG has no NextBot method, so a call site cannot pass the wrong one to
// Decide and have it compile (D32).
type BotRNG struct{ r *RNG }

// NewBotRNG constructs the stream a bot's Decide call draws from.
// Deterministic in (matchSeed, seat), independent of whether seat was
// bot- or human-controlled in any other round: a returning player
// reclaiming a seat in round 8 changes nothing about what round 9's bot
// RNG would produce if the seat reverts to Autopilot later (D32).
func NewBotRNG(matchSeed [32]byte, seat game.SeatID, round int) *BotRNG {
    mac := hmac.New(sha256.New, matchSeed[:])
    mac.Write([]byte("cinzal.bot.rng"))
    binary.Write(mac, binary.BigEndian, int32(seat))
    var botSeed [32]byte
    copy(botSeed[:], mac.Sum(nil))
    return &BotRNG{r: NewRNG(botSeed, round)}
}
```

`round` is not folded into the seed derivation itself — it doesn't need to be, because `NewRNG`'s own HMAC message already includes `round` on every draw (`rng.go:66-69`). Per-round independence for a fixed seat falls out of reusing `NewRNG` internally, unchanged; the only new derivation is per-seat, and it needs to be, since two seats' bots must never share a stream (RFC §14.5).

**The purpose namespace.** `Purpose` (`rng_purpose.go`) stays exactly what it is today: a closed enum, one constant per §6.4 row, checked complete against `ConsumptionTable` by `TestPurposeTableMatchesDeclaredConstants`. A bot purpose cannot be a `Purpose` value without either breaking that completeness check or adding a §6.4 row Option A's rejection above already establishes has no honest ground truth to state. `BotRNG` therefore carries its own open string type and its own single draw method — never `RNG.Next`, never reachable from a `*RNG`:

```go
type BotPurpose string

// NextBot draws exactly like RNG.Next — same HMAC(seed, round‖seq‖purpose)
// shape, same lazy single-draw contract, same panic on n <= 0 — against
// this BotRNG's own inner stream. It takes the open BotPurpose type, not
// Purpose, because unlike every Purpose, a bot's draw count is not a
// property of the rules that a table could audit it against.
func (b *BotRNG) NextBot(purpose BotPurpose, n int) int
```

`NextBot` draws are invisible to `Consumed`/`consumed map[Purpose]int` — that map, and the `rng.seq consumed == predicted` invariant it backs (§16.2), stay scoped exactly to `Resolve`'s own `*RNG`, unconditionally, regardless of how many seats a round is bot-filled, and `BotRNG` has no path to that map at all. Purpose strings for `NextBot` calls follow a `bot.<tier>.<mechanic>` convention (e.g. `bot.drifter.select`, `bot.operator.search`) purely for RNG-trace legibility (§15.3) — the HMAC key already differs per seat via `NewBotRNG`, so no two streams can collide regardless of what purpose strings they reuse; the convention buys a human reading a trace the ability to tell which tier produced a draw, nothing more.

**The replay path.** For the live game: a bot order is written to the order log with `source = 'bot'` the moment the tick produces it (§8.2), exactly like a human order. A refold calls `Resolve` once per round over the logged orders and never calls `Decide` — `NewBotRNG`'s output is consumed once, live, at authoring time, the same relationship a human's own decision-making has to the order they submit. For `cmd/simulate`, which authors every order live and keeps no log to refold from: `NewBotRNG(seed, seat, round)`'s determinism *is* the reproducibility guarantee a log would otherwise provide — re-running the same sweep seed reconstructs the identical sequence of bot RNGs, hence the identical orders, with no log required. Both properties come from the same construction; neither needs the other to hold.

## Reasoning

**The property M1 closed on and the property M2 needs are the same property, and only B buys both.** #81 needs `Resolve`'s per-round index count to be a pure function of the round's orders and the §6.4 table — true today, for hand-written orders. `cmd/simulate`'s sweeps need re-running the same seed to reproduce the same CSV. Option A breaks the first the moment a bot is in the match. Option C alone answers neither, because it only describes the live/log relationship and is silent on a harness with no log. B is the one option that leaves `Resolve`'s stream untouched (keeping #81 exactly as strong as M1 left it, for every match regardless of bot population) while giving `cmd/simulate` a seed-reproducible source for the orders it has no log to fall back on.

**A bot's action still costs whatever the rules already charge for that action — nothing here is a second cost.** Once `Decide` returns a `game.Order`, that order is validated and resolved exactly like a human's: a bot's `Deliver` through a contested node still draws `confront.d6` on `Resolve`'s own stream, a bot's Pushing On route still draws `pushon.edge`. `NewBotRNG` only replaces the *authoring* randomness — which route Drifter happened to pick uniformly, which branch Operator's search preferred — never the resolution randomness the chosen order then triggers. §6.4's table is complete for what it has always covered; this decision adds a second, differently-shaped stream next to it, not a hole inside it.

**Autopilot's instability, worked through under B, actually vanishes rather than merely shrinking.** The scenario the issue opens with — a seat is bot-filled for rounds 1–7 and human-driven for 8–15 — is a fact about `orders.source`, and under B it has zero effect on any RNG stream. `Resolve`'s stream never knew the seat was ever a bot in the first place, because `Decide` never touched it. `NewBotRNG(seed, seat, round)` for a hypothetical round 9, if the seat reverted to Autopilot later, is identical whether or not the seat was human for rounds 8–15 in between — the derivation has no memory of anything except `(seed, seat, round)`. This is the literal reading of "a returning player taking a seat back breaks the two-consecutive-defaults condition and ends Autopilot without any state to unwind" (§8.2) — extended from "no state to unwind" to "no RNG state that could even be perturbed."

**Reusing `NewRNG` internally, rather than adding a seat field or a second draw mode to `RNG` itself, keeps the one existing per-round-instance lifecycle the only one that exists.** `rng.go`'s doc comment already states the discipline: fresh instance, same round, never branched or copied. Giving `RNG` an optional seat dimension, or a `NextBot` method living on the same type as `Next`, would mean a single type carries two call disciplines distinguished only by which method someone happens to call — precisely the ambiguity that lets `Resolve`'s own `*RNG` get passed somewhere a bot stream belongs. Wrapping a differently-seeded `*RNG` inside a distinct `BotRNG` keeps `RNG` itself exactly as simple as M1 left it, turns the *has this call site got the right stream* question into something the compiler answers, and confines this decision's entire footprint to one new type, one new constructor, one new method, and one new open string type.

**Rejecting A directly:** not because §6.4 requires every row to state a literal constant — #77's own predictor/exemption split already proves it doesn't — but because both a predictor and an exemption presuppose a rules-derivable ground truth they are approximating or declining to re-derive, and a bot's draw count has none: it is a fact about the implementation, free to change the moment someone retunes Operator's search, with no GDD or RFC edit to mark that it happened. Independent of that argument, Autopilot handover settles it on its own: #81 was written and closed against a suite with no bots in it, and A is the only option under which that suite's own passing state was already an illusion of a stronger guarantee than it had.

**Rejecting C alone:** it is not wrong, it is incomplete — everything C claims about the live game is restated above as part of B's own consequences, but C by itself has nothing to say about `cmd/simulate`, and the issue that opened this decision named the sweep numbers as exactly what's riding on the answer.

## Consequences

- **No RFC or GDD text changes land in this PR.** [#202](https://github.com/garnizeh/cinzal/issues/202) batches the RFC §6.4/§14/§17 and GDD §21/§22 catch-up for D32, D33 and D34 together, once all three are decided, the same way [#159](https://github.com/garnizeh/cinzal/issues/159) batch-synced §6.4 to D03 well after D03 itself was decided. **This includes correcting RFC §14's `Bot.Decide` signature** from `r *rules.RNG` to `r *rules.BotRNG`. This decision document is the authoritative source for `BotRNG`, `NewBotRNG`, `BotPurpose`/`NextBot`, and the `bot.<tier>.<mechanic>` naming convention until #202 lands; implementers should cite this file, not a yet-unwritten RFC passage.
- **`internal/rules` gains `BotRNG`, `NewBotRNG` (`rng.go`) and `BotPurpose`/`BotRNG.NextBot` (`rng_purpose.go` or a sibling file)** — a small, additive change, landed as part of [#190](https://github.com/garnizeh/cinzal/issues/190) (the `Bot` interface task) rather than in this decision's own PR, matching how D29 split its `internal/rules` wiring into its own follow-up tasks. `BotRNG` is a distinct type from `RNG`, deliberately: it is what makes passing `Resolve`'s own `*RNG` to a bot call site a compile error rather than a wiring mistake #190's own tests would have to catch by inspection.
- **`ConsumptionTable` and `TestPurposeTableMatchesDeclaredConstants` need no change.** Bot draws are structurally outside the table they check the completeness of — the same "deliberately absent, priced at zero, not overlooked" treatment D03 already gives Rotating Borders, for a different reason: Rotating Borders costs zero draws; bot draws cost a real, variable number, but on a stream, and behind a type, the table was never scoped to cover.
- **§16.2's `rng.seq consumed == predicted` invariant is unconditionally true regardless of bot population**, which is the exact property [#201](https://github.com/garnizeh/cinzal/issues/201)'s mid-match-handover fixture (bot rounds 1–7, hand-driven 8–15) needs to hold for its refold assertion to pass.
- **The tier tasks (#192/#193/#194) can now state their own RNG cost honestly**: Drifter's is "however large the legal order space was, via `NextBot`," Operator's is "however deep the search went, via `NextBot`" — neither owes §6.4 a number, because neither is a §6.4 consumer.
- **Reversible at moderate cost, not low.** Once #190 lands `NewBotRNG`, and #201's golden fixtures are recorded against it, moving a bot-RNG consumer onto `Resolve`'s own stream later (Option A) would invalidate every recorded bot-populated fixture and every sweep result computed under B — the same cost §6.4 itself names for getting an RNG-shape decision wrong after fixtures exist. Reversing before #190 lands costs only this document.
