# D56 — uuidv7() as the project-wide surrogate-key convention

**Status:** decided
**Blocks:** nothing yet merged; binds migration 0001 ([#312](https://github.com/garnizeh/cinzal/issues/312)) and every migration after it that adds a surrogate `id`
**Decided:** 2026-08-27
**Issue:** [#379](https://github.com/garnizeh/cinzal/issues/379)

## The question

RFC §7.2 gives every table an `id` column and never states what generates its value. Migration 0001 (#312) needed an answer to write real DDL. Is `uuidv7()` a one-migration default #312 is free to have picked, or the stated, project-wide convention every later migration follows?

## Why it is open

§7.2's schema sketch writes bare `id` with no default expression and no type note — it names columns, not generation strategy, for every table that has one. No RFC section, no GDD section, and no decided `Dnn` document says how a surrogate key is produced. Left unstated, each future migration (#313's `invite_links`, and any table after it) either re-derives the choice from scratch or silently diverges from whatever #312 happened to pick, and an inconsistent id scheme across tables in one schema is the kind of thing nobody notices until it's already inconsistent.

## Options

**A — `gen_random_uuid()` (UUID v4, fully random).** Built into Postgres core since v13, so no version dependency beyond what this project already carries. Simple, no ordering property at all. Cost: v4's randomness scatters every insert across a btree's entire keyspace, which is pure downside for write-heavy tables (`outbox`, `orders`-adjacent `events`) and buys nothing a time-ordered scheme doesn't also give — v4 and v7 are both non-enumerable 128-bit values, so the anti-disclosure property A and B share equally.

**B — `uuidv7()` (UUID v7, time-ordered), native to Postgres 18.** No extension required. The leading bits carry a millisecond timestamp, so newly-inserted rows land index-local in a btree instead of scattering — better locality than v4 for insert-heavy tables — while remaining a full 128-bit UUID with the same non-enumerable property v4 has: no row's `id` discloses how many rows exist or a bare sequential position. Cost: a `uuidv7` value encodes an approximate creation instant, decodable by anyone who can read a UUID's bit layout. For every table this schema has today (`users`, `matches`, `sessions`, `auth_codes`, `outbox`), that timestamp discloses nothing beyond what the row's own `created_at`/`send_after` column already states in plain `TIMESTAMPTZ`, and RFC §9's fog rules already keep every id server-side — no `id` value is projected to a client as an opponent-identifying field, so there is no fog-boundary consequence to weigh here at all.

**C — `bigserial`/sequential integer.** Rejected outright, not weighed further alongside A and B: a sequential id directly discloses row counts and insertion order (exactly how many matches or users exist, and in what sequence) — the one property both UUID options exist to avoid, and the reason RFC-style schemas default to UUIDs for anything that might ever be visible in a URL, log, or error message.

## Decision

**B — every surrogate `id` column, project-wide, defaults to `uuidv7()`.** Never `gen_random_uuid()`, never a serial/bigserial type. Postgres 18 is already this project's pinned version (D46), so `uuidv7()` needs no extension and introduces no new version constraint beyond one already carried. The btree-locality benefit is free — it costs nothing that v4 didn't already cost — and it helps exactly the tables (`outbox`, `sessions`) whose insert volume in a live deployment is highest.

This governs `id` columns specifically. Tables keyed on a natural composite key with no surrogate `id` at all — `match_players (match_id, seat)`, `orders (match_id, round, seat)`, `events (match_id, round, seq)`, `match_summary (match_id, round)`, `rate_limits (scope, key)` — are unaffected; this decision has nothing to say about them, since they were never a candidate for a generated id in the first place.

## Consequences

- **RFC §7.2** gains one line stating the convention: every `id` column's default is `uuidv7()`, Postgres 18 native, chosen over `gen_random_uuid()` for its insert-locality property at equal non-disclosure.
- **Migration 0001 (#312)** already implements this; no change needed there beyond citing this decision by number in its own comment.
- **#313's `invite_links.id`**, and any surrogate `id` any later migration adds, follow the same convention without re-deriving it.
- **No GDD text change** — this is a persistence implementation detail with no game-rule content.
- **Reversible at low cost.** Changing an existing column's default function is a one-line migration; no data-format change, since v4 and v7 are both ordinary 128-bit values in the same `UUID` column type, and existing rows keep whatever value they already have.
