# D19 — Email preferences have no storage, and 'daily digest' is not a filter: what does M3 build, and what does it defer?

**Status:** decided
**Blocks:** the M3 schema migration, and M6's outbox worker, which must consult the preference both before it enqueues and again before it sends (see sub-question 4).
**Decided:** 2026-08-26
**Issue:** [#305](https://github.com/garnizeh/cinzal/issues/305)

## The question

RFC §13's volume-control paragraph specifies four per-match preference levels — *every round* / *only when it's my turn and I haven't moved* / *daily digest* / *none* — plus one-click unsubscribe, default the second. §7.2's schema has no table, no column, and no unsubscribe-token storage for any of it. Four sub-questions, per the issue:

1. Where does the preference live?
2. Is there a per-player default a new seat inherits, or is "default is the second" just a column default?
3. What backs one-click unsubscribe, given §12.2 already refuses magic links for the same prefetch reason?
4. Is the preference consulted at enqueue, at send, or both — against §13.1's existing send-time re-check rule?

## Why it is open

**`daily digest` is not a filter, it is a second delivery mechanism.** The other three levels are predicates over a row that `Tick()` writes anyway — *do I write this `outbox` row, yes or no.* A digest is a scheduled aggregation over rows that were never individually enqueued: it needs a per-player send window, a record of what has already been digested, and a job that is not the outbox worker's drain loop (§13, §8). Treating it as a fourth `CHECK` value alongside three predicates makes it look like the same cost, and it is not.

**The unsubscribe token cannot be the invite token's shape.** §12.2's magic-link refusal is explicit: email clients prefetch links, which silently consumes single-use tokens. D16's Recap cursor and D17's join flow both already turn on this exact hazard — a `GET` must never mutate. An unsubscribe link is prefetched by the same clients, and RFC 8058's `List-Unsubscribe-Post: List-Unsubscribe=One-Click` is the standard answer: the mutation is a `POST`, the `GET` only renders. §13 as written says "one-click unsubscribe" — a behaviour, not a method — and doesn't say which.

**`none` interacts with Autopilot.** §8.2's `autopilot` mail is the one notification whose entire purpose is telling an absent player how to return. A player on `none` who goes to Autopilot after two missed deadlines is, by their own preference, never told — that may be correct, or it may quietly turn a recoverable game state into an unrecoverable one, and the issue is right that this needs a stated answer, not a fallout of an enum.

## Options

**Option A — Build all four levels, including digest, in M3.** One `email_pref` enum with four values, and a digest scheduling mechanism (a `digest_queue` or `digest_window` construct) designed and shipped alongside the other three. Cost: M3's stated deliverable is "matches survive a restart and reproduce exactly" — schema and fold, not a second delivery pipeline. Designing a digest job now means designing it against a mail worker (§13, M6's deliverable) that does not exist yet, and RFC gives no per-player send-window rule, no "already digested" bookkeeping shape, and no cadence to build against — everything about the digest's actual mechanics would be invented for this decision, not read out of the spec.

**Option B — Build the three filter levels now; defer `daily digest`.** `email_pref` holds `every_round` / `turn_only` / `none` — the three predicates M3's own schema can express and M6's own outbox worker can consult unmodified. `daily digest` is named here as deferred, with RFC §13's line corrected to say so, rather than left silently unbuilt behind an enum value nobody backs.

## Decision

**Option B**, plus concrete answers to the other three sub-questions. `match_players` gains two columns:

```sql
match_players(
  match_id, seat, user_id NULL, bot_kind NULL,
  faction, joined_at, missed_deadlines INT,
  last_seen_round INT NOT NULL DEFAULT 0,
  invite_link_id NULL,
  email_pref TEXT NOT NULL DEFAULT 'turn_only'
    CHECK (email_pref IN ('every_round', 'turn_only', 'none')),  -- RFC §13's four levels minus daily_digest (D19, deferred — see Reasoning)
  unsubscribe_token_hash BYTEA NOT NULL,  -- sha256(32 random bytes from crypto/rand), generated at seat creation like invite_links' token (D17)
  FOREIGN KEY (invite_link_id, match_id) REFERENCES invite_links(id, match_id),
  PRIMARY KEY (match_id, seat)
)
```

Answering the four sub-questions directly:

1. **`match_players.email_pref`.** Per-seat-per-match is exactly `match_players`'s own grain, and it is the only per-seat setting the mail path reads (§13) — no new table earns its keep for one column with the same lifecycle as the row it would sit next to.
2. **A DDL default, not a per-user setting.** "Default is the second" reads as a property of the column, not a promise of cross-match memory — nothing in GDD or RFC describes a player-level preferences table, and inventing one here would be building for a feature neither spec asks for. `DEFAULT 'turn_only'` is what a new seat gets, in every match, until that seat's own unsubscribe lowers it, or an in-match settings write raises it back. Reversible at low cost: a `users.default_email_pref` column, consulted at seat-creation time to seed `email_pref` instead of the literal default, is an additive column and a one-line change to the `INSERT`, not a schema shape change.
3. **A stored, hashed per-seat token; `POST`-only mutation; `GET` renders only.** `unsubscribe_token_hash` is generated once, at seat creation, the same 32-byte-`crypto/rand`-then-SHA-256 shape D17 uses for invite tokens — high-entropy input, so a deterministic digest is fine, and an indexed equality lookup isn't even needed here (see Reasoning). `GET /m/{id}/unsubscribe?seat={seat}&token={token}` validates the token against that one seat's row and renders a confirmation page; it changes nothing. `POST` to the same URL — either the confirmation page's own button, or a mail client's `List-Unsubscribe-Post` header hitting the URL directly — is the only request that writes `email_pref = 'none'`.
4. **Both: filtered at enqueue, and re-checked at send — but as a second, orthogonal check, not an extension of §13.1's existing one.** The enqueue `INSERT ... WHERE ...` (§13.1) gains an `email_pref` predicate so a row is never written for a level that would just discard it. At send time, the worker gains a *second* guard — "does this seat's current `email_pref` still permit this template" — checked for every match-scoped template, including `round_resolved` and `match_finished`. This is **not** the same re-check §13.1 already scopes to time-sensitive templates: that check is about content staleness (has the asserted fact stopped being true), which `round_resolved`/`match_finished` are correctly exempt from since they describe the past. A preference change is a different axis — not "is this still true" but "does the recipient still want it" — and it can flip between enqueue and send for any template, past-tense or not. The two checks compose at the same call site; neither substitutes for the other.

**Setting or restoring `every_round`/`turn_only`, and resubscribing from `none`, needs no new mechanism — just a route M5/M6 has yet to name.** The one-click unsubscribe path above exists because *lowering* the preference has to survive an unauthenticated `GET`/prefetch from an email client, which is why it needs its own token. *Raising* it — a logged-in player picking `every_round` or `turn_only`, or resubscribing after `none` — carries no such hazard: it is an ordinary authenticated, session-scoped, CSRF-protected mutation, the same shape `POST /m/{id}/note/{slot}` already has (seat taken from the session, never the payload, per §19's "Seat impersonation" row). It needs a route — a match-settings fragment is the natural place — but that route is UI, not schema, and is M5/M6's to design, the same boundary D17 drew around its own landing-page markup. This decision's scope is the column and the one path (`none`) that needed new machinery; the other two values are reachable the moment M5/M6 wires an authenticated form to `UPDATE match_players SET email_pref = $1 WHERE match_id = $2 AND seat = $3`.

**Autopilot is exempt from `email_pref`, `none` included.** `autopilot` fires unconditionally, the same as `otp` — neither is gated by the column at all. It is not match content in the sense the other four templates are; it is the one notice whose suppression converts a recoverable game state (a returning player retakes their seat, §8.2) into a silent one, and GDD/RFC give no basis for treating that as an acceptable cost of `none`. The unsubscribe link still travels on the `autopilot` mail — a player can still opt future `round_open`/`deadline_soon`/`round_resolved`/`match_finished` mail out through it, it just doesn't stop `autopilot` itself.

**Template-to-level mapping**, needed to make the enqueue predicate concrete (§13's own prose only states the boundary for `turn_only`):

| Template | `every_round` | `turn_only` (default) | `none` |
|---|---|---|---|
| `round_open`, `deadline_soon` | sent | sent | not sent |
| `round_resolved` | sent | **not sent** | not sent |
| `match_finished` | sent | sent | not sent |
| `autopilot` | sent | sent | sent |

`otp` isn't in this table — it carries no `match_id`, so no seat's `email_pref` ever applies to it; it is sent unconditionally, exactly as §12.2 already requires. `autopilot`'s row is uniform because the template is exempt, not because none of the three levels differ for it — see above.

`turn_only`'s row is the literal reading of §13's own "only when it's my turn and I haven't moved" for `round_open`/`deadline_soon`, and its exclusion of `round_resolved` is the one distinction §13's prose actually draws between the two named levels. `match_finished` sitting outside that split — sent under both non-`none` levels — has no direct spec anchor; it is this decision's own reading, grouping a one-time terminal notice with the turn reminders rather than with the up-to-fifteen-times-per-match recap that `turn_only` exists to suppress. It is a one-line move in this table if a later product call disagrees.

## Reasoning

**Why deferring digest is not walking back a promised level.** Unlike D18 (where Option B would have meant removing a design-pillar tool GDD and the roadmap both already write as shipping), nothing downstream is currently written as though `daily digest` exists — M6's deliverable list (roadmap §4) names "all six templates" and dedup/send-time re-check, and `daily digest` was never one of the six templates to begin with. Deferring it costs no already-committed downstream text, only a corrected sentence in §13 saying so.

**Why the unsubscribe token doesn't need `UNIQUE`, unlike `invite_links.token_hash`.** D17's reasoning for a deterministic SHA-256 digest turns on the token being "the only key in the lookup" — `invite_links` is found *by* its hash, so the hash has to support an indexed equality scan across the whole table. An unsubscribe link is different: the URL already carries `{id}` (match) and `seat`, so the row is found by `match_players`'s own primary key first, and the token is only ever compared against that one row's stored hash — the same shape as `auth_codes.code_hash`'s comparison, minus bcrypt's justification (see next). `UNIQUE` on `unsubscribe_token_hash` would enforce a global-uniqueness property nothing in this flow ever tests for.

**Why SHA-256 here too, not bcrypt.** `auth_codes.code_hash` is bcrypt because a 6-digit OTP is low-entropy and must resist offline brute force if the table leaks. `unsubscribe_token_hash`, like `invite_links.token_hash`, is a digest of 256 bits of `crypto/rand` output — deterministic hashing costs nothing in the direction bcrypt protects against, because the input isn't guessable regardless of hash speed. D17's argument applies unchanged; it just no longer also has to justify the *lookup path*, since here the row is already found by other means.

**Why `List-Unsubscribe-Post`'s POST can't split GET-carries-query/POST-carries-form the way D17's join flow does.** D17's `POST /m/{id}/join` receives the token as a hidden form field the landing page renders, deliberately separate from the `GET`'s query parameter, so the two are independently validated requests. RFC 8058 doesn't allow that: a mail client's one-click POST hits the *exact* `List-Unsubscribe` header URL with a fixed body (`List-Unsubscribe=One-Click`) and cannot be made to also submit a form field. `seat`/`token` therefore have to travel in the query string on both verbs of the same route — the human-confirm path (`GET` renders a page whose own button `POST`s the same URL) and the one-click path (a mail client `POST`s it directly) converge on one URL, differentiated only by method. This is a narrower shape than D17's own GET/POST split, not a smaller version of it applied carelessly.

**Why `POST /m/{id}/unsubscribe` is a named exception to §19's CSRF rule, not a hole in it.** §19 requires a token on every mutating form, checked against the session. A mail client's `List-Unsubscribe-Post` request has no session cookie and no form to have embedded a token in — it is the outbox worker's own template, not a page the player's browser rendered. Requiring the ordinary CSRF token here would just reject every RFC 8058 request, which is the opposite of what "one-click unsubscribe" (§13) asks for. The route is authorized instead by the seat-scoped query token itself, and only when the body is exactly `List-Unsubscribe=One-Click` — the same "possession of a high-entropy secret is the proof" shape invite links and the token's own `GET`/`POST` split already use, applied to the one endpoint that structurally cannot carry a session. The confirmation page's own "unsubscribe" button, reached by a human clicking through the `GET`, still submits an ordinary session-and-CSRF-token form like every other mutation in the system — the exception is scoped to the automated one-click path alone.

**Why enqueue-time filtering isn't enough on its own, and why the send-time preference check is a second guard, not an extension of §13.1's re-check.** The issue's own framing supplies both directions: checking only at enqueue means a preference change *after* the row is written but before it sends does nothing — the mail goes out anyway, the false-positive failure mode the issue names. Checking only at send means every level below `every_round` still writes rows to `outbox` for templates it will always discard — for a `none` seat on a fifteen-round async match, up to fifteen `round_resolved` inserts that exist only to be thrown away at send time. Both checks are cheap — `email_pref` is one column on the same `match_players` row the enqueue query already touches for `last_seen_round` — so there's no cost argument for picking one over the other. Crucially, the send-time check is *additional* to, not a widening of, §13.1's existing content re-check: `round_resolved`/`match_finished` remain correctly exempt from re-checking whether their content is still true (it always is — they describe the past), while gaining the separate, unconditional check for whether the recipient still wants any mail for this match at all.

**Why Autopilot's exemption doesn't extend to `match_finished`.** The line between "exempt" and "filtered" here is whether the message is about the *player's ability to continue playing* or about *match content the player has already chosen a volume level for*. `autopilot` is the former — GDD §18 is explicit that an Autopilot'd seat "is never removed" and can be retaken, which only works if a player who set `none` still hears about it. `match_finished` is the latter: a player who chose `none` chose it knowing the match would eventually finish, and nothing about that outcome requires the player's action the way Autopilot does. Grouping `match_finished` with the turn-reminder rows (sent under both non-`none` levels) rather than exempting it entirely reflects that it's a one-time notice worth keeping, not a structural necessity like Autopilot's.

## Consequences

- **RFC §7.2's schema** gains `match_players.email_pref` and `match_players.unsubscribe_token_hash`, shown above. Both are directly-written state, not derived projections — unlike `missed_deadlines`/`last_seen_round`, nothing in `orders` determines a preference or a token, so `cmd/replay --rebuild`'s scope is unchanged by this decision, matching `invite_link_id`'s own classification (D17).
- **RFC §13's volume-control paragraph** states `daily digest` as deferred, not one of v1's three levels, with a pointer to this decision for why. The template table (six rows) is unaffected — `daily digest` was never one of the six.
- **RFC §13.1** gains the `email_pref` predicate on the enqueue `INSERT ... WHERE`, plus a second, independent send-time guard applied to every match-scoped template — distinct from the existing content re-check, which stays scoped to time-sensitive templates exactly as written.
- **RFC §11's route table** gains `GET|POST /m/{id}/unsubscribe` — `GET` validates and renders only, `POST` (from the rendered page's own confirm button, or a mail client's `List-Unsubscribe-Post`) writes `email_pref = 'none'`. Every match-scoped template's headers carry `List-Unsubscribe`/`List-Unsubscribe-Post` pointing at it; `autopilot`'s does too, even though the template itself is exempt from the preference the link sets.
- **RFC §19's security table** gains an "Email unsubscribe" row (token found by `(match_id, seat)` first, not a standalone lookup key, unlike invite links; SHA-256 at rest; `GET` never mutates; `POST` is the only write path) and the CSRF row gains one named exception: `POST /m/{id}/unsubscribe`'s one-click path is authorized by the seat-scoped query token, not a session-bound form token, since a mail client's automated request has neither a session nor a form to have carried one.
- **M3's schema migration** (roadmap §4) gains `email_pref`/`unsubscribe_token_hash` named directly, no longer folded into the placeholder "D19 addition."
- **M6's outbox worker** (roadmap §4, currently "Blocked by: D19") can now be built: the enqueue predicate, the send-time re-check, and the unsubscribe route are all specified above. `daily digest` remains out of M6's six-template scope; if a later product decision wants it, it is an additive `CHECK` value plus a genuinely new scheduling job, not a rewrite of anything decided here.
- **No GDD text change.** Email volume control has no GDD anchor at all — this is purely an RFC persistence-and-security decision. RFC moves r43 → r44; companion pointer stays at GDD v2.32.
- **Reversible at low cost.** Widening `email_pref`'s `CHECK` to add `daily_digest` back is one migration touching one constraint, not a shape change to `match_players`. Moving `match_finished` between the "exempt" and "filtered" rows of the mapping table is a one-line change to the enqueue predicate. The one thing that would not be a cheap follow-up is discovering post-M6 that the unsubscribe token needed independent revocation (D17's reason for `invite_links` being a table, not a column) — nothing in GDD or the RFC suggests a seat would ever need more than one live unsubscribe token, and this decision does not build for that speculatively.
