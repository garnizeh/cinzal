# D17 — Invite links have no storage: table or column, and what does revocation revoke?

**Status:** decided
**Blocks:** the M3 schema migration, and M5's lobby/join flow, which cannot be built against a table that does not exist
**Decided:** 2026-08-25
**Issue:** [#303](https://github.com/garnizeh/cinzal/issues/303)

## The question

RFC §19's security table promises invite links are **"high-entropy, revocable, single-match scope."** §7.2's schema has no table and no column for them, so all three properties are currently promises with nothing to hold them. Four sub-questions:

1. **Table or column?** A `matches.invite_token` column gives one link per match and makes revocation a token rotation. A separate table gives many links per match, per-link revocation, per-link attribution, and expiry.
2. **What is revoked — a link or the match's joinability?** With a column there is only one answer; with a table there are two, and they behave differently for a host who wants to kill one leaked link without kicking the four people already seated.
3. **Is the token stored in the clear?** Sessions and OTP codes are already treated as hashed bearer secrets (§12). An invite token is a bearer credential for a seat in a match — the same argument applies, and it costs a lookup-by-hash instead of a lookup-by-value.
4. **Does a link carry a seat, or just admission?** If the link names a seat, two friends racing on the same link is a conflict the schema must express; if it only grants admission, seat assignment is a separate step and the link is reusable by design.

## Why it is open

**The no-signup promise makes this load-bearing.** GDD §17 promises invite links with no mandatory signup, and RFC §12.1 resolves that into guest sessions for synchronous play. The invite link is therefore the *only* credential standing between a URL and a seat in someone's match for a whole class of players.

**Entropy and lookup interact.** A high-entropy token that is looked up by equality on an indexed column is fine. A high-entropy token that is hashed the way OTP codes are (bcrypt: `POST /auth/request` — "store bcrypt hash") cannot be looked up by equality at all — bcrypt embeds a fresh random salt per hash, so `WHERE hash = bcrypt(input)` is not an expression Postgres can evaluate; OTP gets away with it only because the row is already found by `email` first, and the code is checked against that one row's hash. An invite token has no such second key — the token *is* the only key in the lookup. Deciding "hashed" without deciding the hash function produces either an unindexable table or a design that silently can't work.

**Expiry is unstated.** Nothing in either document says whether an invite outlives its match's `status`, and "the match is finished" is a check the join handler has to make regardless — the question is whether the row expires too, or the check is the only guard.

## Options

**Option A — Column on `matches`.** `matches.invite_token_hash` (plus `invite_token_revoked_at`). One link per match; revocation is either rotating the hash (the old link 404s, replaced by a new one) or a single boolean. Cost: no per-link attribution ("who did Ana's link let in"), no independent expiry from the match's own lifecycle, and — the decisive failure — a host who wants to kill one leaked copy of the link has no way to do it without invalidating every other copy of the same link, including the ones already in friends' group chats who haven't clicked yet.

**Option B — Separate `invite_links` table.** Many rows per match, each with its own hash, `created_at`, `expires_at`, `revoked_at`. Cost: one more table, one more migration, one more FK to reason about on `match_players`.

## Decision

**Option B.** A new `invite_links` table, plus a nullable attribution FK on `match_players`:

```sql
-- invite links: the bearer credential for admission to one match's lobby (D17)
invite_links(
  id, match_id REFERENCES matches(id) NOT NULL,
  token_hash BYTEA UNIQUE NOT NULL,   -- sha256(32 random bytes from crypto/rand); the raw token is never stored
  created_at,
  expires_at TIMESTAMPTZ NULL,        -- NULL = no forced expiry beyond the match's own lobby window
  revoked_at TIMESTAMPTZ NULL         -- NULL = live; set = revoked. A flag, never a DELETE — attribution survives.
)
```

```sql
match_players(
  match_id, seat, user_id NULL, bot_kind NULL,
  faction, joined_at, missed_deadlines INT,
  last_seen_round INT NOT NULL DEFAULT 0,           -- Recap cursor (D16), derived from orders
  invite_link_id NULL REFERENCES invite_links(id),  -- which link admitted this seat (D17); NULL for the host's own seat or a bot fill
  PRIMARY KEY (match_id, seat)
)
```

Answering the four sub-questions directly:

1. **Table**, for the attribution and independent-revocation properties Option A cannot express.
2. **A link is revoked, never the match.** `UPDATE invite_links SET revoked_at = now() WHERE id = $1` — a flag, matching `auth_codes.consumed_at` and `matches.finished_at`'s existing soft-terminal-state convention rather than a `DELETE`. Seats already in `match_players` are untouched: `match_players.invite_link_id` keeps its historical value, and seat validity is never re-checked against the link's current state after the row exists. A host kills one leaked link without touching the four people already seated through it, or through any other link.
3. **Hashed, with SHA-256, not bcrypt.** The token is 32 bytes from `crypto/rand`, hashed with SHA-256 before storage; the raw token is never persisted, only carried in the URL. `token_hash BYTEA UNIQUE` supports a plain indexed equality lookup: `SELECT * FROM invite_links WHERE token_hash = $1`. Bcrypt is wrong here for a structural reason, not a performance one — see Reasoning.
4. **Admission only, not seat-bound.** A link resolves to a match, not a seat. `POST /m/{id}/join` assigns whichever seat is open at INSERT time, through the same `(match_id, seat)` primary-key contention every join already has. Two friends racing on the same link is therefore not a new conflict this schema has to express — it's the ordinary lobby-capacity race, resolved by whichever `INSERT` lands first claiming a seat and the other claiming the next open one (or "this table is full" if none remain). A link is **reusable** — not single-use — until it is revoked, expires, or the match leaves `lobby`.

**Entropy budget: 32 bytes (256 bits) from `crypto/rand`.** `base64.RawURLEncoding.EncodeToString` of those 32 bytes is what goes in the URL (~43 URL-safe characters); the hash is computed over the raw 32 bytes, before encoding.

## Reasoning

**Why not Option A.** The issue's own framing supplies the decisive test: "a host who wants to kill one leaked link without kicking the four people already seated." A column has exactly one revocable thing — the match's link — so revoking it necessarily revokes every copy of it, seated players and all, unless a second mechanism (a per-seat "grandfather" flag) is invented to exempt them. That second mechanism is the table this option was trying to avoid, arrived at the long way. A table also matches the natural usage pattern implied by GDD §17's "send a link to a friend" (plural, over a match with 2–5 seats) better than a single match-wide URL — a host can hand distinct friends distinct links without inventing anything else.

**Why SHA-256, not bcrypt, and why this isn't a downgrade from the OTP precedent.** `auth_codes.code_hash` is bcrypt because a 6-digit OTP code is *low*-entropy (≈20 bits) and must resist offline brute force if the table leaks — bcrypt's deliberate slowness is the point. But bcrypt lookup only works when the row is already found by another column (`email`), because each bcrypt hash carries its own random salt, so no two hashes of the same plaintext are equal and an equality index over them is meaningless. An invite token has no such second key: the token itself is the entire lookup input, resolving directly to `match_id`. A deterministic, unsalted digest is not a weaker choice here, it's the only one that can be looked up at all — and it costs nothing in the direction bcrypt was protecting against, because the input isn't low-entropy. At 256 bits, offline brute force against a leaked `token_hash` column is not a smaller risk than against a bcrypt hash, it's the same "computationally impossible" the entropy budget already buys, achieved by search-space size rather than by hashing cost. This is also what resolves the issue's own "entropy and lookup interact" tension: deciding the hash function *is* deciding the lookup path, and the two cannot be decided independently the way the issue's phrasing implies they might be.

**Why the Timing row in §19 needs no new machinery here.** §19's constant-time requirement exists for secrets compared byte-by-byte against a known plaintext (an OTP digit sequence, a session token compared in application code) where an early-exit comparator can leak how many leading bytes matched. `token_hash = $1` is Postgres evaluating equality between two fixed-length 32-byte digests via an index probe, not an application-level sequential compare against the caller's raw input — there is no partial-match channel to exploit, and even a naive full compare of two hash outputs reveals nothing productive about a preimage in a 256-bit space. Nothing beyond the ordinary indexed `WHERE` is required.

**Why admission-only avoids inventing a race the schema has to arbitrate.** Sub-question 4 poses a real fork: a seat-bound link creates a new, link-specific race — two people submitting for the *one* seat the link names, which needs an explicit winner. Admission-only reduces this to the race every join already has regardless of invite links: contention on `match_players`'s own `(match_id, seat)` primary key when a lobby has more simultaneous claimants than open seats. That race is not new work this decision creates — it is the one the join handler must already resolve for a match with, say, 2 open seats and 3 people trying to click "join" at once, invite link or not. Choosing admission-only means D17 introduces no additional arbitration logic beyond what M4/M5 already owe the lobby.

**Why reusable, not single-use — and the direct parallel to D16.** [D16](D16-recap-cursor.md) rejected advancing the Recap cursor on `GET /m/{id}/recap` because RFC §12.2 already rejected magic links for the identical reason: `GET` requests are prefetchable by browsers, email clients, and HTMX's own speculative triggers, and consuming single-use state on one silently burns it before the intended reader ever sees the page. `GET /m/{id}/join` — "invite link landing" (§11) — sits behind exactly the same transport. If a link were single-use and consumed on that landing `GET`, a chat app's link-preview fetch would burn it before the invited friend ever clicked. The existing route split already supplies the safe boundary without inventing one: the landing `GET` only *validates* (not found / expired / revoked / match not in `lobby`) and renders; only the mutating `POST /m/{id}/join` — "take a seat" — can change any state, and it never touches the link's own validity, only `match_players`. A link is therefore left reusable, gated by `revoked_at`/`expires_at`/the match's own status, never consumed by use, and never by rendering the landing page.

**Why `expires_at` defaults to `NULL` rather than a fixed TTL.** Neither GDD nor RFC states a lobby-formation time budget (GDD's 4h–72h deadlines are round deadlines, §18, which only start once a match is `active` — a different clock). Inventing a default TTL here would be a product decision with no spec anchor, which is exactly the kind of guess D35's own discipline and this project's decision-log convention (cite sections, don't invent policy) argue against. `matches.status = 'lobby'` is already an independent, unconditional guard the join handler must apply regardless of any link (a `POST /m/{id}/join` after the match starts fails on that check alone, per the same invariant D16 relied on for `last_seen_round`'s seat-creation value) — so a `NULL` `expires_at` is not "no expiry at all," it is "no expiry beyond the one the match's own lifecycle already enforces." A host-set TTL becomes available the moment M5's lobby UI wants to offer one, with no schema change.

**Why `revoked_at`/`expires_at` are flags, never a `DELETE`.** Matches the schema's existing convention (`auth_codes.consumed_at`, `matches.finished_at`) and keeps `match_players.invite_link_id`'s FK valid and attribution ("who did Ana's link let in") answerable after the link is dead — a `DELETE` would either cascade and destroy that history or orphan the FK, and gains nothing a flag doesn't already provide.

## Consequences

- **RFC §7.2's schema** gains the `invite_links` table and `match_players.invite_link_id`, both shown above. `invite_links` is **not** a derived projection — unlike `events`/`match_summary`/`last_seen_round`, it is authoritative state with no order-log equivalent to rebuild it from, so `cmd/replay --rebuild`'s scope (M3, roadmap §4) is unchanged by this decision.
- **RFC §19's "Invite links" row** gets a concrete spec instead of unbacked prose: 32-byte `crypto/rand` token, SHA-256 hash stored and indexed, raw token never persisted; revocation flags the link, never the match; admission-only, so concurrent joins on one link resolve as the ordinary lobby-capacity race.
- **RFC §11's route table** — `GET /m/{id}/join` needs a carrier for the token (a query parameter is the natural fit; the exact shape, and whether the handler cross-checks the token's own `match_id` against the `{id}` path segment or resolves the match from the token alone, is link-copy UX and is M5's to design, per the issue's own "not in scope" boundary).
- **M3's schema migration** (roadmap §4, M3 deliverables) gains `invite_links` and the `match_players.invite_link_id` column directly, no longer part of the undifferentiated "D17–D19 additions" placeholder.
- **M5's lobby/join flow** can now be built: `POST /matches` (create) can mint the match's first `invite_links` row in the same transaction; the join handler's full check is `token_hash` found AND `revoked_at IS NULL` AND `(expires_at IS NULL OR expires_at > now())` AND `matches.status = 'lobby'` AND an open seat exists.
- **Reversible at low cost.** A future product decision to bind a link to a specific seat, or to make links single-use, is a superseding decision, not a rewrite: `invite_links` already has the row-per-link granularity either shape would need, and `match_players.invite_link_id` already records attribution either way. The one property that would need to change under a "seat-bound" supersession is the join handler's seat-assignment logic, not the schema.
