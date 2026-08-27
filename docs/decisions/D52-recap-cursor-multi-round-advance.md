# D52 — D16's cursor advance marks every unseen round as seen in one statement. Is that acceptable, and if not, what replaces it?

**Status:** decided
**Blocks:** migration 0002 (#313) only in its comment text; M6's Recap query outright, since the semantics of the column are what that query is written against
**Decided:** 2026-08-27
**Issue:** [#349](https://github.com/garnizeh/cinzal/issues/349)

## The question

[D16](D16-recap-cursor.md) advances the Recap cursor inside the submit transaction:

```sql
UPDATE match_players
   SET last_seen_round = GREATEST(last_seen_round, $2 - 1)
 WHERE match_id = $1 AND seat = $3;
```

`GREATEST(last_seen_round, round − 1)` does not advance the cursor by one. It advances it to `round − 1` from wherever it was, marking every round in between as seen in a single statement. D16 analyses one asymmetry in detail (the cursor landing one round short when the submission itself triggers the tick, which repeats a round — the safe direction) and never analyses this one, which runs the other way and can discard an arbitrary number of rounds at once.

Three things have to come out of this: whether a multi-round jump is acceptable at all; if not, the corrected expression; and an explicit statement of what a returning Autopilot'd seat's Recap shows, since that is the case the column was added for.

## Why it is open

**The multi-round skip lands hardest on exactly the player the Recap exists for.** A seat that missed rounds 3, 4 and 5 (two missed deadlines is §8.2's Autopilot threshold, so this is the ordinary absent-player case) has a cursor sitting at 2. They return in round 6, fill in the order form, and submit. The cursor goes 2 → 5 in that one statement, and the Recap for rounds 3, 4 and 5 — the rounds the entire feature exists to narrate — is gone, whether or not they actually read it first.

**D16's defence of the update point is a UI assertion, not a guarantee, and it is weakest here.** D16 argues a player "cannot submit an order blind; the order form and the Recap it sits beside on the same composed page … are what they'd have had to look at to fill it in." A player can fill in an order without reading a three-round narrative sitting beside it, and the wider the backlog, the more likely they are to act first and read later. This is the same failure D16 rejected its own Option A for ("a player opens it, sees the map, closes the tab without reading the resolution narrative"), reintroduced at up to fourteen rounds' magnitude instead of one.

**D16's Autopilot argument is safe only up to the moment of return.** The Recap is intact when an Autopilot'd seat's owner arrives — nothing has touched `last_seen_round` while a bot is submitting for them. It is their own first submission, the same request that ends Autopilot, that destroys it under the current expression.

**The cheap fixes each cost something, which is why this is a decision and not a one-line patch.** `last_seen_round + 1`, bounded by `round − 1`, advances one round per submission — but naïvely applied, it also advances once per *call*, not once per *round*, so a resubmission of the same round (GDD §18: "resubmission is allowed while the round is open") would advance it again on every resubmit, reopening the same multi-round-skip failure through a different door. A separate acknowledge control is D16's already-rejected Option B. Capping the Recap at the last *N* rounds regardless of the cursor changes what GDD §18 promises ("every round since your last visit"). A hybrid — advance by one when the gap is exactly one, require a dismissal control otherwise — reintroduces the rejected new UI control for precisely the case where it would matter most.

## Options

**Option 1 — Bound the advance to one round per submission, gated on it being the seat's first submission for that round.** `last_seen_round = LEAST(last_seen_round + 1, round − 1)`, executed only when the `orders` upsert for `(match_id, round, seat)` was a genuine first submission rather than a resubmission. No new column, no new UI. Cost: a returning seat with a real backlog now sees a Recap window that stays exactly as wide as the original gap for as long as it keeps submitting every round without missing another — it never shrinks back to width one on its own, only widens further on a fresh gap.

**Option 2 — `last_seen_round + 1` bounded by `round − 1`, applied on every submission unconditionally (the issue's original "cheap fix").** Same intent as Option 1, but without the first-submission gate. Cost: not idempotent under resubmission — GDD §18 explicitly allows resubmitting while a round is open, and each resubmission would advance the cursor again, letting a player who resubmits `k` times skip `k` rounds of backlog in one round's worth of activity. This reopens the exact class of bug D52 exists to close, just gated by resubmit count instead of gap size.

**Option 3 — A dedicated acknowledge control for any submission where the gap exceeds one round.** D16's Option B, scoped to the multi-round case. Cost: identical to Option B's original rejection — a new interactive control whose only job is bookkeeping — and it lands on exactly the players (multi-round absentees) least likely to also press an extra button.

**Option 4 — Cap the Recap query at the last *N* rounds regardless of `last_seen_round`.** Cost: silently and permanently drops any round beyond *N*, which is worse than Option 1's widened-window cost — Option 1 never discards a round, it only delays when it's shown.

## Decision

**Option 1.** The submit transaction's cursor-advance statement is replaced with one that advances by at most one round, and only on the seat's first submission for that round:

```sql
-- inside the submit transaction, replacing D16's UPDATE
WITH upsert AS (
    INSERT INTO orders (match_id, round, seat, payload, submitted_at)
    VALUES ($1, $2, $3, $4, now())
    ON CONFLICT (match_id, round, seat) DO UPDATE
        SET payload = EXCLUDED.payload, submitted_at = now()
    RETURNING (xmax = 0) AS is_first_submission
)
UPDATE match_players
   SET last_seen_round = LEAST(last_seen_round + 1, $2 - 1)
 WHERE match_id = $1 AND seat = $3
   AND last_seen_round < $2 - 1
   AND (SELECT is_first_submission FROM upsert);
-- $1 = match id, $2 = round being submitted for, $3 = seat
-- xmax = 0 is the standard INSERT ... ON CONFLICT idiom for "this row was
-- inserted, not updated" — Postgres sets xmax on the UPDATE branch, never
-- the INSERT branch, of the same statement.
```

The `last_seen_round < $2 - 1` guard is a no-op safety net, not load-bearing: it just skips the write entirely once the seat has fully caught up, rather than writing the same value back on every steady-state submission.

## Reasoning

**Why not Option 2.** Its formula is right; its trigger is wrong. `LEAST(cursor + 1, round − 1)` computed once per *round* is exactly what this decision needs. Computed once per *POST*, it silently assumes one submission per round, which GDD §18 already contradicts. The fix isn't a different arithmetic expression, it's gating the same expression on "is this the first time this round has been submitted," which the `orders` upsert already has to determine in order to choose its `INSERT` vs. `DO UPDATE` branch — `xmax = 0` reads that decision back out for free, without a second query.

**Why not Option 3 or 4.** Both were already argued against in D16 (Option 3 is D16's Option B verbatim) or worse than the option chosen (Option 4 discards permanently what Option 1 only defers). Neither is repeated here beyond that pointer.

**Why Option 1's steady-state behaviour is unchanged from D16.** In the no-backlog case — one submission per round, no missed deadlines — `last_seen_round` going into round `R` is always `R − 2` (D16's own accepted one-round-behind asymmetry). `LEAST((R−2)+1, R−1) = LEAST(R−1, R−1) = R−1`, identical to what the old `GREATEST(last_seen_round, R−1)` produced. Nothing changes for the common case; the correction only bites when there's an actual gap to bound.

**Why the "window stays wide" cost is the right one to accept, not merely the cheapest.** Trace the return-from-absence example through the new formula. Cursor is 2, current round is 6 (gap of 3: rounds 3, 4, 5 unseen). The board page's `GET`, rendered before any submission, is unaffected by this decision — it still shows the full backlog (rounds 3–5) exactly as D16's original design intended, since nothing has touched the cursor yet. The player submits for round 6: cursor advances to `LEAST(2+1, 5) = 3`. Their *next* visit shows rounds 4–6 — round 3 is now assumed seen (the one-round asymmetry D16 already accepts, applied once), but rounds 4 and 5, which the old expression would have discarded outright, are still there. Each subsequent distinct-round submission advances the cursor by exactly one further round, so the window slides forward one round at a time rather than collapsing to nothing. Under continuous submission with no further gaps, the algebra shows this width never shrinks back to one on its own (`round − 1 − last_seen_round` is invariant once no round is skipped): the returning player sees a permanently 3-round-wide Recap for the rest of the match unless they open a *new* gap. That is a real, named cost — but it is a cost of showing slightly more context than strictly necessary, not one of ever losing a round. It generalizes D16's own accepted trade-off (repeat rather than skip) uniformly to arbitrary gap sizes instead of treating a multi-round gap as a special case that gets the opposite treatment.

**What M6's Recap query shows a returning Autopilot'd player, stated explicitly (sub-question 3).** Nothing about Autopilot changes under this decision — §8.2's rule that bot/Autopilot orders are generated inside `Tick()`, never through the submit handler, still means `last_seen_round` is structurally frozen for the entire duration a seat is on Autopilot. The sequence for a seat that returns after N rounds of Autopilot:
1. **Their first `GET` of the match page, before any submission**, runs the unmodified query `round > last_seen_round AND round < matches.round` against the still-frozen cursor: the full N-round backlog, unabridged, exactly as D16 already guaranteed.
2. **Their first `POST /m/{id}/order`**, which both re-seats them (ending Autopilot, §8.2) and is a first submission for that round, advances the cursor by exactly one round — not to `round − 1`. If that same submission also triggers `Tick()` (round resolves, current round advances), the next Recap view is `round > last_seen_round AND round < matches.round`, still an N-round-wide window, just shifted forward by one — never `N − 1` and shrinking, since the current round bound moved forward by the same one round the cursor did.
3. **Every subsequent Tick-triggering submission** advances both the cursor and the current-round bound by one, so the window's *width* stays at `N` for as long as the seat keeps submitting every round without a further gap — it slides forward, it does not narrow. No round generated during the Autopilot window is ever silently discarded by the act of returning; each one is shown, just inside a window that stays `N` rounds wide rather than shrinking to one (see the "window stays wide" cost above, which this is the Autopilot instance of).

The guarantee D16 stated — "the returning player's first Recap is guaranteed intact" — now holds for the *visit*, not only the *arrival*, which is exactly the gap the issue identified.

## Consequences

- **D16's decision document** — the `UPDATE` statement in its "Decision" section and the corresponding line in the RFC — is superseded by the statement above; D16 itself is not reopened on any of its other sub-questions (cursor grain, seat-creation default, derived/rebuildable status, update point being `POST`), none of which this decision touches.
- **RFC §8.1's submit-transaction code sample** is replaced with the CTE-based statement above, and its inline comment ("GREATEST guards a resubmission … from moving it backward") is corrected to describe the new idiom: the resubmission guard is now the `is_first_submission` gate, not `GREATEST`.
- **RFC §7.2/§8.2's rebuild expression changes, and this is the one real added cost of this decision.** D16's `COALESCE((SELECT MAX(round) FROM orders WHERE match_id = X AND seat = Y AND source = 'human') − 1, 0)` was correct only because the old live update was itself a pure function of the single latest submitted round — `GREATEST` doesn't care about the path taken to get there. `LEAST(cursor + 1, round − 1)` is not: it is a genuine ordered fold over the seat's distinct human-submitted rounds, and its result depends on the sequence, not just the maximum. A rebuild has to replay that sequence rather than aggregate it: for each seat, take the distinct rounds with a human order, ascending, and run `cursor = min(cursor + 1, round − 1)` starting from `cursor = 0` — the exact step the submit transaction applies live, replayed once per row instead of once per request. `cmd/replay --rebuild` already iterates the order log to fold `internal/rules` state; this is one more per-seat accumulator carried through that same pass, not a new scan. A single `MAX(...)`-based rebuild would silently overshoot for any seat that ever had a gap, marking rounds seen that the live column never actually advanced past.
- **Migration 0002 (#313)** carries the corrected `UPDATE` statement (and its comment) in the submit-handler code it documents, not in DDL — the column's type, nullability and default from D16 are all unchanged, so this is a comment-and-logic correction to that task, not a schema change.
- **M6's Recap query** (`round > last_seen_round AND round < matches.round`) is unchanged by this decision — only how `last_seen_round` itself advances changes, not how it's read.
- **Reversible at low cost.** Like D16 itself, this changes only the expression and the trigger condition inside one transaction. A future decision to accept a faster catch-up rate, or to bound the window's maximum width, amends this document without touching D16's column, type, or default.
