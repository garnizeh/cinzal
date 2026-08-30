-- match_summary.sql — M3 (issue #315). Derived, rebuildable projection of
-- the order log — never authority (migration 00001's COMMENT ON TABLE
-- match_summary; RFC-001 §7.1-7.3).
--
-- Caller: UpsertMatchSummary is the round tick (M4), writing the cheap
-- "which seats have submitted this round" cache after each round resolves;
-- cmd/replay --rebuild (M4) regenerates it identically from a fresh fold.
-- submitted_seats is boolean-shaped roster data — who has submitted, not
-- what they submitted — the same category RFC-001 §8.2's lobby list already
-- reads safely, unlike orders.payload or events.payload. Each call is still
-- scoped to one (match_id, round): no query here returns another match's or
-- another round's row.
--
-- DeleteMatchSummaryByMatch is issue #321's ClearProjections, alongside
-- events.sql's DeleteEventsByMatch: cmd/replay --rebuild's first step (M4),
-- clearing one match's derived rows — both tables, one transaction — before
-- a fresh fold regenerates them.

-- name: UpsertMatchSummary :one
INSERT INTO match_summary (
  match_id, round, submitted_seats
) VALUES (
  $1, $2, $3
)
ON CONFLICT (match_id, round) DO UPDATE
  SET submitted_seats = excluded.submitted_seats,
      updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteMatchSummaryByMatch :exec
DELETE FROM match_summary
WHERE match_id = $1;
