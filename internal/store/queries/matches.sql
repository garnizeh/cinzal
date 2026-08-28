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
