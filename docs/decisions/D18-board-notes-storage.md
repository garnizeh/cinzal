# D18 — Pins and notes have no storage: build the Board's fourth tool in v1, or move it out in writing?

**Status:** decided
**Blocks:** the M3 schema migration; M5's Board deliverable
**Decided:** 2026-08-25
**Issue:** [#304](https://github.com/garnizeh/cinzal/issues/304)

## The question

GDD §7.5 lists manual annotation — pins and notes — as the fourth of the Board's four tools, and GDD §17 puts the Board inside v1 scope. RFC §7.2's schema holds nothing for it. Binary choice: add the storage in M3, or move annotation to v1.1 and correct the GDD so the Board is documented as three tools in v1.

## Why it is open

The roadmap's own risk register (P6) calls the Board "four distinct tools" and "where P3 (the design pillar) actually lives," and warns that "a thin Board means the deduction game does not land." Dropping one of four tools from the feature that carries a design pillar is a design decision that needs to be made and justified, not discovered by whoever builds the Board in M5 against a schema that never held a place for it.

Annotation is also structurally unlike the other three tools. The Log, Attribution and the Heat Map are all functions of `PlayerView` — GDD §7.5's own "data sourcing" section says the Heat Map draws only from "your own observation history" and "public anchors," both of which are already fog-safe because they live inside `MatchState`'s `SeatArchive` (RFC §9.2) and cross the fog boundary through `Project()` like everything else. A pin is different: player-authored free text, stored server-side, with no order-log equivalent to fold it back from. That makes it an exception to RFC §7.1 the way [D17](D17-invite-link-storage.md)'s `invite_links` already is, and it is the first mutable, unbounded, user-supplied string anywhere in the schema — every other string in the system is engine-generated ([D31](D31-node-display-name.md) made even node names RNG-free for exactly this reason).

## Options

**Option A — Add the storage in M3.** A new table, scoped per seat per match, with a length cap and a per-seat count cap enforced declaratively. Cost: one more table, one more thing to get the fog-scoping of right (never render one seat's notes to another).

**Option B — Defer to v1.1.** Correct GDD §7.5 to describe three tools and §17 to drop pins from the v1 scope bullet; decide what, if anything, M5's Board UI shows in the tool's place so the absence reads as deliberate rather than missing.

## Decision

**Option A.** A new `board_notes` table, seat-private, with the per-seat count cap expressed as a bounded slot number rather than a counted `CHECK`:

```sql
-- manual annotation: the Board's fourth tool (GDD §7.5, D18). Authoritative
-- state with no order-log equivalent, like invite_links (D17) — not a
-- derived projection, and never rendered to any seat but its own author.
board_notes(
  match_id, seat,
  slot SMALLINT NOT NULL CHECK (slot BETWEEN 1 AND 20),  -- the per-seat cap, enforced by the bound itself
  node_id INT NULL,                                      -- pinned to a node; NULL = a freeform note
  round INT NOT NULL,                                    -- the round it was written in
  body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 500),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (match_id, seat) REFERENCES match_players(match_id, seat),
  PRIMARY KEY (match_id, seat, slot)
)
```

- **Grain:** per seat per match, optionally attached to a node (`node_id`) via GDD's own object vocabulary; a `NULL` node is a freeform note, not tied to a location. No edge or round attachment beyond the `round` stamp that mirrors the Log's own round/node filter — a route-shaped annotation is already the Heat Map's job.
- **Length cap:** `CHECK (char_length(body) BETWEEN 1 AND 500)`. 500 has no spec anchor; it is a placeholder product parameter, changeable by editing one constant, not a schema shape.
- **Count cap:** a `slot` column bounded `1..20` under the table's own primary key, not a counted `CHECK` (Postgres cannot express "at most N sibling rows" declaratively) and not a trigger — so the cap is **per seat per match** (up to 20 rows for *each* seat in a match), not 20 rows shared across the whole match. Writing note *N* is `INSERT ... ON CONFLICT (match_id, seat, slot) DO UPDATE SET node_id = EXCLUDED.node_id, round = EXCLUDED.round, body = EXCLUDED.body, updated_at = now()`, matching `orders`'s own resubmission shape (RFC §7.2). `updated_at = now()` has to be named explicitly in the `SET` clause — a plain `DEFAULT now()` only fires on `INSERT`, so a conflict that hits `DO UPDATE` without it would silently leave a stale timestamp on every edit after the first. 20 is equally a placeholder.
- **Index:** none beyond the primary key. `PRIMARY KEY (match_id, seat, slot)` already makes `WHERE match_id = $1 AND seat = $2` a prefix-indexed lookup — "my annotations for this match" is the PK's own leading columns, not a separate index.
- **Retention:** notes survive a match finishing — a player may want to revisit their own analysis — but are **never** part of the replay bundle. `{seed, config, orderLog}` (RFC §10.4/§15.4) is shared between every player in the match; `board_notes` is seat-private and has no place in a bundle built to be handed to an opponent.
- **Deletion:** a real `DELETE FROM board_notes WHERE match_id = $1 AND seat = $2 AND slot = $3`. Not a soft-delete flag — unlike `invite_links.revoked_at` or `auth_codes.consumed_at`, nothing else references a `board_notes` row and no other party has an attribution interest in a note surviving its own author's delete.
- **Escaping:** ordinary templ/`html/template` context-aware auto-escaping. This is the first mutable user string in the render path, not the first escaping decision — no new rule is needed.
- **Never projected to another seat, and where that's enforced:** the query surface itself, not a filter applied after the fact and not the template. `internal/store`'s notes query takes `(matchID, seat)`, and `seat` is bound from the session→seat mapping the same way every other per-seat action already resolves it (RFC §19's "Seat impersonation" row) — never a path or form parameter. There is no `ListBoardNotesForMatch` that returns every seat's rows; a cross-seat read is not an expression the query surface can make.

## Reasoning

**Why not Option B.** The technical objections the issue raises — fog exposure, unbounded text, no deletion path — are all real, but each has a small, declarative answer (a `CHECK`, a bounded `slot`, an ordinary `DELETE`, a query scoped by construction). None of them is a reason the *feature* is expensive; they're reasons the *schema entry* needs four extra lines, which M3 can afford. Weighed against that: P6 already names the Board as underestimated and the four-tool count as the thing that makes P3 land, and the GDD, RFC roadmap (line 366, "pins/notes per D18"), and M3's own deliverable bullet (line 317, "the D18–D19 additions (pins...)") all already write as though annotation ships — Option B would mean walking back a design pillar's own tool count on cost grounds that don't hold up once the schema is actually drawn.

**Why a bounded slot, not a trigger or an app-level count.** A per-seat cap needs either procedural enforcement (a trigger, invisible at the schema level and bolted onto one table for one column) or a declarative bound. `slot SMALLINT CHECK (slot BETWEEN 1 AND 20)` under the table's primary key makes the cap physically true rather than application-remembered — the same shape this schema already prefers ([D17](D17-invite-link-storage.md)'s composite FK exists for exactly this reason: "the database enforces X, rather than every future caller having to remember to"). It also gives the client a natural, stable address for editing or replacing one note (`POST /m/{id}/note/{slot}`) instead of needing a generated note ID round-tripped through a form.

**Why `node_id INT` with no foreign key.** `game.NodeID` is a plain `int` (`internal/game/ids.go`), and the map itself is never a stored table — nodes are generated at runtime from `seed`/`config`, the same reason `orders.payload` encodes route node IDs with no FK to reference. `board_notes.node_id` follows the existing convention rather than inventing a `nodes` table this schema has never needed.

**Why retention survives match completion but the replay bundle excludes it.** The bundle is deliberately minimal and deliberately shared — `{seed, config, orderLog}`, "a few kilobytes," handed to every player so each can fold it client-side (RFC §10.4). A note is neither part of that triple nor safe to hand to the other players it was written about. Keeping the row after `matches.status = 'finished'` costs nothing (no retention job is needed anywhere else in this schema either) and lets a player's own analysis outlive the match for their own later reading; excluding it from the bundle is not an extra rule so much as the bundle's existing shape already not including it — this decision just says so explicitly rather than leaving it to be noticed the day someone adds a fifth field to the export and reaches for "everything about this match."

**Why hard delete, unlike the schema's usual flag convention.** `invite_links.revoked_at`, `auth_codes.consumed_at` and `matches.finished_at` are flags because something else references the row (`match_players.invite_link_id`) or another party has a legitimate interest in the history surviving (attribution: "who did Ana's link let in"). Neither holds for a note: nothing FKs to `board_notes`, and no other seat is ever allowed to see it in the first place, so there is no audience for its history once its own author deletes it. Reusing the flag convention here would be applying a pattern past the reason it exists.

**Why the Board-panel fragment is a stated, narrow exception to "every fragment takes `PlayerView`."** RFC §11's fragment discipline says every fragment is a templ component taking `PlayerView`, and the full page composes the same components. `board_notes` has no fog projection to perform — it is already scoped to exactly one seat by the query, with nothing to filter — so routing it through `rules.Project()`/`internal/match` to arrive inside `PlayerView` would mean widening a `game`-package type to carry seat-private store rows that have nothing to do with match state, purely to satisfy a naming convention. The board-panel fragment's templ component takes `PlayerView` (for the Log, Attribution and Heat Map) **plus** `[]BoardNote` fetched directly from `internal/store` — the one named exception to "every fragment takes `PlayerView`," and it is named here rather than left for M5 to either discover or silently violate.

## Consequences

- **RFC §7.2's schema** gains `board_notes`, shown above, alongside a sentence in the "derived projections" paragraph naming it — like `invite_links` — as authoritative state outside `cmd/replay --rebuild`'s scope.
- **RFC §11's route table** — `GET /m/{id}/board-panel` becomes "fragment: log, attribution, heat map, pins (D18)"; two new routes, `POST /m/{id}/note/{slot}` (write/replace) and `POST /m/{id}/note/{slot}/delete`. The fragment-discipline paragraph gains the one-sentence exception described above.
- **RFC §15.4's "Match export" bullet** states explicitly that the replay bundle excludes `board_notes` — seat-private state has no place in an artifact built to be shared with every player in the match.
- **RFC §19's security table** gains a "Board notes" row: fetched only by `(match_id, seat)` with `seat` from the session, never a path or query parameter; no query in the store package can return another seat's notes.
- **M3's schema migration** (roadmap §4) gains `board_notes` named directly, no longer folded into the undifferentiated "D18–D19 additions" placeholder (line 317). D19 — email preferences — was open at the time of this decision; see [D19](D19-email-preference-storage.md) for its resolution.
- **M5's Board deliverable** (roadmap §4, line 366) can now build pins against a real table instead of a citation to an open decision.
- **No GDD text change.** This is a persistence-and-schema decision, not a rule change — GDD §7.5's four-tool description and §17's v1 scope bullet are already correct and stay as written. RFC moves r42 → r43; companion pointer stays at GDD v2.32.
- **Reversible at low cost.** Raising the length cap or the slot count is a `CHECK` edit, not a migration shape change. Adding edge-attached notes later is an additive nullable column. The one thing that would not be a cheap follow-up is discovering post-M5 that annotation needed to be cross-seat (a shared team note, say) — nothing in GDD or the RFC suggests that, and this decision does not build for it speculatively.
