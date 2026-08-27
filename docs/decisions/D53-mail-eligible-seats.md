# D53 — Which `match_players` seats are eligible for mail at all, and how far does one-click unsubscribe reach?

**Status:** decided
**Blocks:** migration 0002 (#313) — the `unsubscribe_token_hash BYTEA NOT NULL` shape and the enqueue predicate are both schema, not comment text — and M6's outbox worker
**Decided:** 2026-08-27
**Issue:** [#350](https://github.com/garnizeh/cinzal/issues/350)

## The question

[D19](D19-email-preference-storage.md) put two columns on `match_players` and a preference predicate on the enqueue insert, and answered *what level a seat is at*. It never asked *whether a seat can receive mail at all*, and RFC §7.2's own row shape names two kinds of seat that cannot:

- **Bot seats** (§14.2 filler bots): `user_id IS NULL`, `bot_kind` set. §8.2 already relies on `user_id IS NULL` as the exact test that separates a bot seat from a human one ("The `user_id IS NOT NULL` guard keeps filler bots … out of it").
- **Guest seats with no bound email.** §12.1's table requires an email only for asynchronous play; a synchronous guest is "display name only — guest cookie," and `users.email UNIQUE NULL` is nullable "for exactly that reason." §12.1 also states a guest "can bind an email later and keep their history" — so a seat can start with no address and gain one mid-match, in either mode.

Four things have to come out of this: the eligibility predicate itself; whether `unsubscribe_token_hash BYTEA NOT NULL` still holds for a seat with no address; what a guest who later binds an email gets; and whether one-click unsubscribe should reach past the one match a link names.

## Why it is open

**The enqueue predicate as written filters on preference and submission state, nothing else.** §13.1's `deadline_soon` insert, with D19's own addition:

```sql
JOIN match_players mp ON mp.match_id = m.id AND mp.seat = $3
WHERE m.id = $1 AND m.round = $2
  AND mp.email_pref <> 'none'   -- D19
  AND NOT EXISTS (SELECT 1 FROM orders o …)
```

A bot seat passes `mp.email_pref <> 'none'` — it gets the same `DEFAULT 'turn_only'` every seat does — and passes `NOT EXISTS (… orders …)` for the entire open round, because §8.2 is explicit that bot orders are "generated **inside the tick**… never submitted at round open." Nothing here stops the row from being written; whether it "sends" then depends on what `outbox.to_email` receives from the elided `SELECT`, and neither D19 nor §13.1 states that — `outbox.to_email` has no stated nullability either, so the row is either a runtime constraint violation or a delivery attempt with no address.

**D19 answered a different axis.** Its four sub-questions are all "what level is this seat at," none is "can this seat receive mail at all" — that predicate belongs at the same call site (§13.1) and was never written.

**`unsubscribe_token_hash BYTEA NOT NULL` commits to a shape it doesn't argue for.** D19's own words: the token is "generated once, at seat creation." Seat creation for a bot is the lobby fill; for a guest, `POST /m/{id}/join`. `NOT NULL` means both mint 32 bytes of `crypto/rand` and a SHA-256 digest for a row that may never have anywhere to send an unsubscribe link.

**A guest's email can be bound after the row already exists**, and D19 is silent on whether that seat's pre-existing `email_pref`/token still apply or need a second act.

**One-click unsubscribe is per-match by construction** (D19: the token resolves one `(match_id, seat)` row). RFC 8058's `List-Unsubscribe-Post` is read by mailbox providers as "stop sending this," and a player in several async matches who unsubscribes from one keeps getting mail from the others — the more likely next action being a spam complaint than a second unsubscribe. §13's own premise is "email is on the critical path"; deliverability is not a side concern here.

## Options

**1A — Eligibility as a join predicate against `users.email` (derived).** Add `JOIN users u ON u.id = mp.user_id` and `AND u.email IS NOT NULL` to every match-scoped enqueue query. No new column; a bot seat is excluded because the inner join drops its `NULL` `user_id`, a no-email guest because `u.email IS NULL`. Cost: one more join at a call site that already joins `match_players`.

**1B — A stored `match_players.mail_eligible BOOLEAN` (or similar), maintained on write.** Cost: a second source of truth for a fact `users.email` already carries, requiring an update at every point an email can be bound (§12.1's "bind an email later" has no named write path yet) to keep the flag from drifting — exactly the kind of derived-vs-authoritative distinction §7.2 already draws for `last_seen_round`/`events`/`match_summary`, and this fact is cheaper to derive than any of those.

**2A — Keep `unsubscribe_token_hash NOT NULL`; mint unconditionally at seat creation, unchanged from D19.** Cost: a wasted `crypto/rand` draw and 32 stored bytes for a seat that may never use them — cheap, and it is the only option that leaves sub-question 3 (guest binds an email later) with nothing to do, since the token the seat needs already exists.

**2B — Make the column nullable; mint only once an address is known.** Cost: seat creation must branch on whether an address is known yet, and binding an email later becomes a second, new mint path with no named trigger in either spec — §12.1 says a guest "can bind an email," not through what route or handler, so this option manufactures a call site D19 and the RFC don't describe, to save bytes 2A spends for free.

**3A — One-click unsubscribe stays scoped to the one match its link names; no cross-match mechanism.** Cost: the deliverability risk the issue names — mailbox providers can read repeated non-response to `List-Unsubscribe` on other matches as ignored feedback, same failure mode D19 flagged for other reasons.

**3B — Add a per-user global suppression flag, offered only from the confirmation page's human path, alongside the existing per-match unsubscribe.** `users.email_suppressed_at TIMESTAMPTZ NULL`, checked by the same join 1A already adds. Cost: one nullable column, one more `AND` at the same call site, and a second button on a page that already renders one.

## Decision

**1A.** Every match-scoped enqueue query gains a join to `users` and an `email IS NOT NULL` predicate — no new stored column. Concretely, §13.1's `deadline_soon` insert (the pattern every other match-scoped template's enqueue follows, per D19's "analogous predicate" line) becomes:

```sql
INSERT INTO outbox (…)
SELECT … FROM matches m
JOIN match_players mp ON mp.match_id = m.id AND mp.seat = $3
JOIN users u ON u.id = mp.user_id
WHERE m.id = $1 AND m.round = $2
  AND u.email IS NOT NULL             -- D53: eligibility — no join row, no address, no insert
  AND u.email_suppressed_at IS NULL   -- D53: global unsubscribe, see below
  AND mp.email_pref <> 'none'         -- D19: per-match preference
  AND NOT EXISTS (SELECT 1 FROM orders o
                  WHERE o.match_id = m.id AND o.round = m.round AND o.seat = $3)
ON CONFLICT DO NOTHING;
```

`JOIN users u` (inner, not `LEFT JOIN`) does the bot exclusion by itself: a bot seat's `mp.user_id` is `NULL`, no row in `users` has a `NULL` id, so the join contributes no row and the whole `SELECT` returns nothing for that seat, regardless of what any other predicate says. A guest with no bound email survives the join (their `users` row exists, `is_guest = true`) but fails `u.email IS NOT NULL`.

**Autopilot is exempt from `email_pref` and `email_suppressed_at` (D19's exemption, extended below to the suppression flag) but is *not* exempt from `u.email IS NOT NULL`.** Eligibility is not a preference — it is "does an address exist to put in `to_email`," which no exemption can route around. In practice this rarely bites: §8.2's own Autopilot test already requires `user_id IS NOT NULL`, and an async seat always has an email at join (§12.1). It only matters for a synchronous guest seat that never bound an email and later meets Autopilot's two-missed-deadline condition — that seat's Autopilot email is silently not enqueued, correctly, because there is nowhere to send it. `otp` needs no equivalent guard: it has no `match_id` and is keyed directly by the email address the login request supplied, never by a `match_players`/`users` join.

**2A.** `unsubscribe_token_hash` stays `BYTEA NOT NULL`, minted the same way for every seat regardless of whether it currently has a deliverable address, unchanged from D19.

**Sub-question 3 falls out of 2A with no new mechanism.** A guest's row already carries a valid `email_pref` (its seat-creation default, or whatever an authenticated per-match mutation later set it to — D19's "raising" path is available to a guest exactly as to any session holder) and a valid `unsubscribe_token_hash`, minted at seat creation under 2A regardless of address. Binding an email is purely a write to `users.email`; the next enqueue's join simply starts finding a row where `u.email IS NOT NULL` was previously false. Nothing in `match_players` is touched, and no second mint, no reseed, and no new write path is needed.

**3B.** `users` gains `email_suppressed_at TIMESTAMPTZ NULL`, checked in the same join as eligibility, alongside every per-match `email_pref` check, for every match-scoped template. `GET /m/{id}/unsubscribe`'s confirmation page — the human-rendered path, never the automated `List-Unsubscribe-Post` body — gains a second control: "stop all Cinzal mail" alongside the existing per-match unsubscribe. Its `POST` is authorized the same way the existing confirmation-page button already is (the seat-scoped query token proves control of that one seat; D19), distinguished from the RFC 8058 automated path by form content — the automated path's body is always exactly `List-Unsubscribe=One-Click` and can carry nothing else, so the two are never ambiguous at the handler. Its effect is `UPDATE users SET email_suppressed_at = now() WHERE id = (SELECT user_id FROM match_players WHERE match_id = $1 AND seat = $2)`. `autopilot` and `otp` are exempt from `email_suppressed_at` for the identical reason D19 exempted them from `email_pref`: both are about a player's ability to continue playing or to log in at all, not about match content the player has chosen a volume for, and a global "stop match mail" click does not argue for taking either away.

## Reasoning

**Why 1A over 1B.** §7.2 already draws the line this repeats: derive from the columns that already carry the fact when a fold or a join can reach it cheaply, store only what has no fold (`invite_links`, `board_notes`, and D19's own `email_pref`/`unsubscribe_token_hash`, none of which anything else determines). `users.email` already is the fact "can this user receive mail" — a second boolean would drift the moment `users.email` changes and nothing writes the flag, which is exactly the class of bug this decision exists to close, one layer further down.

**Why 2A over 2B, beyond the byte cost.** 2B's saving is real but small (32 bytes, one hash) and its cost is a call site neither spec names: §12.1 says a guest can bind an email, not when, from what handler, or in what transaction — inventing a "mint the token here" step at an unspecified write path is exactly the kind of guess D17/D19's own discipline (cite sections, don't invent policy) argues against. 2A pays a fixed, bounded cost at a call site that already exists (seat creation) and buys sub-question 3 for free, which 2B would have to solve with new machinery.

**Why the eligibility predicate does not compose into `email_pref`'s own `CHECK`.** They are different failure modes at different layers: `email_pref = 'none'` is a preference a seat with a perfectly good address can set; `u.email IS NULL` is a physical absence of an address no preference can override. Folding "no address" into the enum (a fifth value, say `'ineligible'`) would let application code write it onto a seat that later gains an address, which is a lie the moment `users.email` changes — keeping it as a join predicate against the column that is actually authoritative means it can never drift out of sync with reality the way a mirrored enum value could.

**Why `email_suppressed_at` gets the same exemption set as `email_pref`, not a narrower one.** The two flags encode the same underlying axis — does this player want match-content mail — at two different scopes, one match versus all of them. D19's reasoning for exempting Autopilot ("the one notice whose suppression converts a recoverable game state into a silent one") and `otp` ("carries no `match_id`, sent unconditionally") is about what kind of message each is, not about how widely the opt-out reaches. A player who clicks "stop all Cinzal mail" is still a player somewhere in a game that may Autopilot them, and still a player who may need to log in again — nothing about clicking a wider button changes what those two messages are for.

**Why the global affordance lives only on the human path, never the automated one.** RFC 8058 fixes the automated POST's body to exactly `List-Unsubscribe=One-Click` — there is no field in that request to carry a "and also all my other matches" signal, and inventing one would violate the standard the whole mechanism exists to satisfy. Scoping the wider action to a second button only a human reaches by first opening the confirmation page keeps the automated path exactly as narrow as RFC 8058 requires while still giving a human reader the broader option D19 didn't offer.

**Why this isn't reopening D19's rejection of a player-level preferences table.** D19 rejected a *per-user default level* table because nothing asks for cross-match preference memory and a `users.default_email_pref` column was available at no schema cost if ever needed. `email_suppressed_at` is not a preference default — it is a single terminal flag with one write path (this decision's confirmation-page button) and one read path (every match-scoped enqueue's join), holding no level, no default, and no per-match override once set. It answers a deliverability question D19 explicitly left open ("it may still be the right call, but it should be the stated call"), not the question D19 already closed.

## Consequences

- **RFC §7.2's schema** — `match_players.email_pref`/`unsubscribe_token_hash` are unchanged from D19 (2A). `users` gains `email_suppressed_at TIMESTAMPTZ NULL` (3B). The `match_players` comment naming D19's columns gains a pointer to this decision for the eligibility join, so a reader doesn't conclude `NOT NULL` alone makes every seat mail-eligible.
- **RFC §13.1** — every match-scoped template's enqueue query gains `JOIN users u ON u.id = mp.user_id` plus `u.email IS NOT NULL` and `u.email_suppressed_at IS NULL`, shown above for `deadline_soon` and applying identically to `round_resolved`'s and `match_finished`'s own enqueue joins per D19's "analogous predicate" line. `autopilot`'s enqueue gains the `u.email IS NOT NULL` guard only — it stays exempt from the other two, per D19 and this decision's extension of that exemption. `otp` is untouched; it never joins `match_players` or `users` by this path.
- **RFC §11's route table** — `POST /m/{id}/unsubscribe`'s human-rendered confirmation page gains a second, distinguishable form action for the all-matches suppression; the automated `List-Unsubscribe-Post` path (fixed body) is unchanged and cannot reach it.
- **RFC §19's security table** — the "Email unsubscribe" row gains a line noting the two mutations the confirmation page's authenticated path can now make (per-match `email_pref = 'none'`, or all-matches `email_suppressed_at`), both under the same seat-scoped-token authorization D19 already established; the CSRF row's named exception is unchanged, since it only ever covered the automated one-click body.
- **M3's schema migration (#313)** carries `users.email_suppressed_at`, the unchanged `match_players` columns from D19, and can now be built — this was the open blocker.
- **M6's outbox worker** gets a concrete enqueue predicate for every match-scoped template, including the two axes (eligibility, global suppression) D19 didn't state. The send-time re-check D19 already specified extends unchanged: it re-applies the same preference/suppression questions at send, and eligibility cannot regress between enqueue and send within one transaction's scope the way a preference can, so no additional send-time guard is needed for `u.email IS NOT NULL` itself.
- **No GDD text change.** Like D19, this is purely an RFC persistence-and-security decision with no GDD anchor. RFC moves one revision; companion pointer unchanged.
- **Reversible at low cost.** Moving eligibility from a join predicate (1A) to a stored column (1B) is an additive migration plus a backfill, not a shape change to anything already built against this decision. Removing the global-suppression affordance (3B) if it turns out unwanted is dropping one column and one `AND` clause; nothing downstream depends on `email_suppressed_at` existing beyond this decision's own predicate.
