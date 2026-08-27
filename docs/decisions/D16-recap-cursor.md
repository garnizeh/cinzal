# D16 — The Recap has no cursor. Where does last-seen-round live, and what advances it?

**Status:** decided — its `UPDATE` statement is corrected by [D52](D52-recap-cursor-multi-round-advance.md)
**Blocks:** M3's schema migration (RFC §7.2) and the scope of its `cmd/replay --rebuild`; every Recap query M6 will write against this column
**Decided:** 2026-08-25
**Issue:** [#302](https://github.com/garnizeh/cinzal/issues/302)

## The question

GDD §18 requires the async entry screen to answer *"what happened while I was gone?"* with the Recap: *"a text and short-animation summary of every round since your last visit."* Nothing in RFC §7.2's schema records what a seat has visited. Three sub-questions, and answering the first without the other two produces a column nobody can update correctly:

1. **Where does the cursor live?** Per seat, per match — the grain GDD §18 states.
2. **What advances it, and at what moment?** Rendering the board, opening the Recap fragment, and an explicit acknowledgement each answer "since your last visit" differently, and the wrong one silently eats a round the player never saw.
3. **What is it at seat creation, and on a seat that joins mid-lobby?**

Plus two that have to be answered explicitly rather than left to drift: whether the column is authoritative or derived from the order log (RFC §7.1's `state = fold(...)` covers everything else in this schema), and whether an Autopilot seat's cursor moves.

## Why it is open

`sessions.last_seen_at` (RFC §7.2) is the wrong grain twice over — **per session**, so a player on a phone and a laptop has two, and **per player**, not per match, so a player in three async matches has one timestamp covering all three; opening match A then match B tells match B nothing changed. It is also the wrong *type*: the Recap's unit is a round, not an instant, and mapping a timestamp back onto rounds needs a `deadline_at` history the schema doesn't keep.

The update point is the genuinely undecided part. The routes already named in RFC §11 make the three failure modes concrete rather than abstract:

- `GET /m/{id}` or `GET /m/{id}/board` (the match page, HTMX board target) — a player opens it, sees the map, closes the tab without reading the resolution narrative. If the cursor moves here, that round's Recap is gone forever.
- `GET /m/{id}/recap` (the Recap fragment itself, RFC §11) — advancing on render looks like the natural fix, but RFC §12.2 already rejects this exact shape for OTP magic links: *"Email clients prefetch links, which silently consumes single-use tokens."* A `GET` fragment is exactly as prefetchable (HTMX's hover/reveal triggers, browser speculative fetch, a double-submit on a flaky connection), and the failure is the same class of bug.
- An explicit acknowledgement control — never loses a round while read, but a player who never presses it accumulates Recap content indefinitely, and the feature becomes a wall of text.

## Options

**Option A — Advance on board render.** `last_seen_round` updates inside whichever handler serves `GET /m/{id}` or `GET /m/{id}/board`. Simplest possible implementation. Cost: exactly the tab-close failure above — content is marked seen before the player has actually read it.

**Option B — Advance only on an explicit acknowledgement.** A dedicated control (button/link) the player presses after reading. Cost: never loses a round while the player engages, but nothing bounds how long a disengaged-but-still-opening-the-tab player can go without pressing it — the wall-of-text failure, plus a new interactive control whose only job is this one bookkeeping action.

**Option C — Advance on rendering `GET /m/{id}/recap`.** Cost: the magic-link precedent above — a `GET` is not a safe place to hang a single-fire state mutation when the transport in front of it (HTMX, browsers) treats `GET` as prefetchable by design.

**Option D — Advance inside the order-submission transaction (`POST /m/{id}/order`), to the last round that has actually resolved, and only there.** Concretely: `last_seen_round = GREATEST(last_seen_round, round − 1)`, where `round` is the round being submitted for, run in the same transaction as the `orders` upsert (RFC §8.1). Cost: a player who views the Recap without submitting (common early in an async round, well before the deadline) doesn't have the cursor move on that visit — but nothing is *lost* either, since the same content simply reappears on the next visit until they act. **[Corrected by D52]** This `GREATEST` expression itself turned out to carry a second, worse cost this document never analysed: it jumps the cursor to `round − 1` from wherever it was in one statement, marking every round in between as seen at once rather than advancing by one. [D52](D52-recap-cursor-multi-round-advance.md) replaces the expression below with one bounded to one round per submission; nothing else about Option D (the update point, the column, its type and default) changes.

## Decision

**Option D.** Add `last_seen_round INT NOT NULL DEFAULT 0` to `match_players`, advanced by one additional statement inside the existing submit transaction of RFC §8.1:

```sql
-- inside the submit transaction, immediately after the orders upsert
UPDATE match_players
   SET last_seen_round = GREATEST(last_seen_round, $2 - 1)
 WHERE match_id = $1 AND seat = $3;
-- $1 = match id, $2 = round being submitted for, $3 = seat
```

**[Corrected by D52]** This statement jumps the cursor to `round − 1` from wherever it was, marking every round in between as seen in one statement rather than one round at a time — [D52](D52-recap-cursor-multi-round-advance.md) replaces it with a statement bounded to one round per submission, gated on the submission being the seat's first for that round. Everything else below this point — the column, its type, its default, the update point being `POST /m/{id}/order` — is unaffected and still authoritative.

`0`, not `NULL`, is the seat-creation value — for both a lobby-formation seat and one that joins mid-lobby — and no special case is needed for either. The column is a **derived, rebuildable read cache**, the same category as `events`, `match_summary` and `match_players.missed_deadlines` (RFC §7.2, §8.2), not a new exception to §7.1's `state = fold(...)`.

## Reasoning

**Why not A, B or C.** A loses a round the player never read — the one thing the issue and GDD §18 both call the worst outcome ("without it a player comes back after twenty hours, remembers nothing, and stops opening the tab"). B trades that failure for the opposite one and adds a control with no other purpose. C repeats a mistake this RFC has already made once and already corrected: §12.2's magic-link rejection is not really about email specifically, it's about not hanging a single-fire consumption on a request method the surrounding transport is free to fire speculatively. `GET /m/{id}/recap` sits behind exactly that transport (HTMX).

**Why D works, and why it's the only one that answers every sub-question at once.**

*Update point.* `POST /m/{id}/order` — "submit or resubmit; triggers Tick" (RFC §11) — is the one write in the entire request surface that both proves engagement (a player cannot submit an order blind; the order form and the Recap it sits beside on the same composed page, RFC §11.1's "fragment discipline," are what they'd have had to look at to fill it in) and is immune to prefetch by construction: browsers and HTMX both restrict speculative/preload behaviour to safe methods, and `POST` isn't one. GDD §18's own resubmission rule — *"resubmission is allowed while the round is open; the last submission stands"* — makes the `GREATEST(...)` update idempotent for free: a resubmit in the same round recomputes the same value.

*The one accepted asymmetry.* When a player's submission is also the one that completes the round (RFC §8: "when everyone has submitted, the round resolves immediately") and triggers `Tick()` synchronously, this UPDATE runs *before* that resolution, using the pre-tick `round`. So `last_seen_round` lands one short of the round the player is about to watch resolve in the same response (via the ordinary board re-render / narrated resolution list, RFC §11.3 — not the Recap). Their next visit's Recap then shows that one round again, redundantly. This is deliberately accepted: it repeats a round the player already saw once, which is the safe direction of the two failure modes this decision exists to choose between (§18's own framing is about a round *disappearing*, never about one being narrated twice).

*Type, nullability, default.* `INT NOT NULL DEFAULT 0`, not a nullable column with a `NULL`-means-never / `0`-means-seen-nothing split. The codebase already has this exact pattern and already resolved it the same way: `Player.LastOfferRound` (`internal/rules/contracts.go:139`) reads *"its Go zero value — `RoundNumber` is 1-indexed, so 0 unambiguously means 'never'"*. `last_seen_round` is the identical shape — a 1-indexed round counter where 0 is not a real round and therefore safe as the "nothing yet" sentinel. Introducing `NULL` alongside it would be new house style invented for one column rather than reused from the one that already exists.

*Seat creation and mid-lobby join.* `POST /m/{id}/join` — "take a seat" (RFC §11, line 1119 in the endpoint table) — only ever runs while `matches.status = 'lobby'` (RFC §7.2's status enum), strictly before round 1 has resolved. Every seat, whenever within the lobby phase it joins, has by construction seen zero resolved rounds of this match at creation time. `DEFAULT 0` is simultaneously correct for the match's first seat and its last — there is no reachable "joined after round 1" case in the current spec for this to special-case.

*Authoritative or derived.* Derived, not an exception. `last_seen_round` is reconstructible from `orders` alone: `COALESCE((SELECT MAX(round) FROM orders WHERE match_id = X AND seat = Y AND source = 'human') − 1, 0)` — the `COALESCE` is load-bearing, not decorative: `MAX()` over zero rows is `NULL`, and `NULL − 1` is still `NULL`, so a rebuild for a seat with no human orders yet must not be written as the bare subtraction. This is because the value this decision writes *is* exactly that quantity, computed incrementally instead of by a full scan. Stored for the same reason `events`/`match_summary`/`missed_deadlines` are stored despite being derivable: so the entry screen ("your matches: whose turn, time left, **unread recap**" — RFC §11, `GET /matches`) doesn't refold or rescan on every list render.

*Autopilot.* No `source` check is needed anywhere, because none is possible to skip: `POST /m/{id}/order` is exhaustively the human path. Bot and Autopilot orders are generated inside `Tick()` (RFC §8, §8.2 — *"generated inside the tick, not submitted ahead of time"*), a different code path that never executes this UPDATE. An Autopilot seat's cursor is therefore structurally incapable of moving until a human actually submits again — which is exactly what RFC §8.2 already says ends Autopilot ("a returning player reclaims the seat on their next submission... and ends Autopilot without any state to unwind"), so the returning player's first Recap is guaranteed intact without a separate rule to guarantee it.

## Consequences

- **RFC §7.2's schema block** gains `last_seen_round INT NOT NULL DEFAULT 0` on `match_players`, commented as a derived, rebuildable read cache alongside `missed_deadlines`.
- **RFC §8.1's submit-transaction code sample** gains the `UPDATE match_players` statement above, immediately after the `orders` upsert, in the same transaction.
- **RFC §7.2/§8.2's "derived, rebuildable" sentence** is extended to name `match_players.last_seen_round` alongside `events`, `match_summary` and `missed_deadlines`.
- **M3's `cmd/replay --rebuild` deliverable** (roadmap §4, M3) widens by one column: rebuilding `match_players.last_seen_round` from `orders` is now in scope alongside `events`/`match_summary`, on the same "derived, rebuildable" basis — it costs the rebuild command one more `UPDATE ... SELECT MAX(...)` per seat, nothing else.
- **No GDD text changes.** This is a persistence-and-timing decision, not a rule change — GDD §18's Recap requirement is unamended, matching the posture RFC r22→r23 (D29) and r22→r23 (D26) already took for fold-attachment/schema-only decisions. RFC moves r40 → r41; companion pointer stays at GDD v2.32.
- **M6's Recap query** (out of scope here) reads as `round > last_seen_round AND round < matches.round` — rounds that have resolved and haven't been marked seen — and `GET /matches`'s "unread recap" indicator is the same comparison's boolean form, so no separate design is needed for it.
- **Reversible at low cost, with one caveat.** Like D27 and D31, a later product decision that "seen" should mean "board opened" rather than "order submitted" is a superseding decision, not a rewrite — the column, its type, and its default all survive unchanged regardless of which request triggers the update. Its **derived-from-`orders` status does not automatically survive**, though: that property holds only as long as the trigger is itself a write already present in the order log. A future trigger tied to a bare `GET` (board render) has no corresponding `orders` row to reconstruct from — `cmd/replay --rebuild` would need a new durable "visit" record to replay against, and the column would become authoritative state rather than a read cache, the one exception to §7.1's fold this decision's own "why it is open" section asked to be stated explicitly if it ever arose. It does not arise under Option D, precisely because `POST /m/{id}/order` is already a logged write.
