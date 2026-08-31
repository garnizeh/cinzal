-- match_players.sql — M3 (issue #315).
--
-- Callers: CreateMatchPlayer is internal/match's seat-creation path (M3/M5:
-- lobby formation and, later, POST /m/{id}/join) — one call per seat, never
-- batched, so a caller cannot accidentally seat a player it didn't mean to.
-- ListMatchPlayers is internal/match's own roster/lobby read (M3, #318/#319)
-- and, later, the lobby list (RFC-001 §8.2).
--
-- ListMatchPlayers returns every seat's roster row for ONE match_id the
-- caller already scoped the call to — seat number, faction, bot_kind. This
-- is lobby/roster data, not the game view: it carries no order payload, no
-- fog-scoped field, nothing GDD §7.1 would gate behind sight or trail. It is
-- still scoped to one match_id per call — nothing here lists across matches.
--
-- last_seen_round, invite_link_id, email_pref (migration 00002, D16-D19) all
-- have schema defaults and are read/written by their own M5 tasks, not this
-- one — omitted from CreateMatchPlayer's column list and from
-- ListMatchPlayers' projection on purpose. unsubscribe_token_hash is the one
-- exception: it is BYTEA NOT NULL with no default (D19 mints it "once at
-- seat creation, unconditionally, for every seat"), so CreateMatchPlayer
-- must take it as a parameter or the INSERT fails the NOT NULL constraint —
-- the hash itself is computed by the M5 caller (sha256 of 32 crypto/rand
-- bytes, per D19), never by this query.
--
-- UpdateLastSeenRound is cmd/replay --rebuild's own write (issue #409, RFC
-- §7.2's third derived projection alongside events/match_summary), not the
-- M5 live-submission path D16/D52 describe: that path advances the cursor
-- by exactly one round, gated on a submission being a seat's first for that
-- round; this one overwrites it outright with a value already fully
-- computed by an ordered per-seat fold over the match's whole order log
-- (internal/store.RebuildLastSeenRounds' own caller, cmd/replay/rebuild.go).

-- name: CreateMatchPlayer :one
INSERT INTO match_players (
  match_id, seat, user_id, bot_kind, faction, unsubscribe_token_hash
) VALUES (
  $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: ListMatchPlayers :many
SELECT match_id, seat, user_id, bot_kind, faction, joined_at, missed_deadlines
FROM match_players
WHERE match_id = $1
ORDER BY seat;

-- name: UpdateLastSeenRound :exec
UPDATE match_players
SET last_seen_round = $3
WHERE match_id = $1 AND seat = $2;
