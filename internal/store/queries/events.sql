-- events.sql — M3 (issue #315). Derived, rebuildable projection of the
-- order log — never authority (migration 00001's COMMENT ON TABLE events;
-- RFC-001 §7.1-7.3).
--
-- Callers: InsertEventsBatch is the round tick (M4), immediately after
-- Resolve() returns its []Event for one round — one batch, one round, one
-- match; the tick is the only caller with events to insert (RFC-001 §7.4:
-- Resolve returns pure events, only the tick's caller dispatches anything
-- with them). DeleteEventsByMatch is cmd/replay --rebuild (M4): it clears
-- one match's derived rows before InsertEventsBatch regenerates them from a
-- fresh fold, so a rebuild can never leave stale rows behind a truncated
-- replay.
--
-- payload surfaces as []byte here — D44's codec boundary. Like
-- orders.ListOrdersForMatch, nothing queried through this file's read side
-- (there is none in M3 — no ListEventsForMatch yet) may be added later
-- without the same pre-fog warning: an event row is the same authoritative,
-- unfiltered shape as an order, one call away from a fog leak if ever handed
-- to a renderer directly instead of through Project().

-- name: InsertEventsBatch :batchexec
INSERT INTO events (
  match_id, round, seq, kind, payload
) VALUES (
  $1, $2, $3, $4, $5
);

-- name: DeleteEventsByMatch :exec
DELETE FROM events
WHERE match_id = $1;
