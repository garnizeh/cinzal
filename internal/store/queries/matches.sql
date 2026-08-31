-- matches.sql — M3 (issue #315).
--
-- Callers: CreateMatch is internal/match's match-creation path (M3, #318) —
-- called once, at the moment a lobby becomes a match, freezing config and
-- seed for the match's whole lifetime (RFC-001 §6.2: "a match created under
-- v1 rules finishes under v1 rules"). GetMatch is internal/match's own
-- load/fold-start path (M3, #319) and cmd/replay (M4) — both read exactly
-- one match's own row by its own id, which the caller already possesses
-- (from a URL path segment, a job argument, or its own prior CreateMatch
-- call); neither query can be handed an id it wasn't already scoped to.
--
-- config and seed surface as []byte here — D44's codec boundary, not
-- decoded until a layer above internal/store.
--
-- Locking variants (SELECT ... FOR UPDATE, round-advance UPDATE) belong to
-- the round tick, M4's own task — out of scope here.

-- name: CreateMatch :one
INSERT INTO matches (
  config, seed, created_by, timer_seconds, deadline_seconds
) VALUES (
  $1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetMatch :one
SELECT * FROM matches
WHERE id = $1;

-- ListMatchIDsByStatus is cmd/replay --all's own scope query (issue #323):
-- "'--all' needs a scope rule... restrict to status='finished' by default
-- and require an explicit flag to touch an active one." statuses is the
-- caller's own scope decision — {"finished"} by default, {"finished",
-- "active"} only with the explicit --include-active flag — passed as one
-- slice rather than called once per status, so the result is one query and
-- one globally consistent id order rather than several separately-ordered
-- result sets concatenated by the caller. Ordered by id — uuidv7 (D56), so
-- this is also creation order — purely so a --all run's own output is
-- deterministic across runs, not because any caller depends on it for
-- correctness.
-- name: ListMatchIDsByStatus :many
SELECT id FROM matches
WHERE status = ANY(@statuses::text[])
ORDER BY id;
