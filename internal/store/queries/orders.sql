-- orders.sql — M3 (issue #315). THE LOG (RFC-001 §7.1: state = fold(Resolve,
-- initial(seed, cfg), orderLog)) — the only source of truth in this schema.
--
-- Callers: UpsertOrder is the order-submission path (M3 internally,
-- M5's POST /m/{id}/order at the HTTP layer) — GDD §18's "resubmission is
-- allowed while the round is open; the last submission stands", enforced by
-- ON CONFLICT (match_id, round, seat) DO UPDATE against orders' own primary
-- key (migration 00001's comment on CREATE TABLE orders). One seat, one
-- round, one call.
--
-- ListOrdersForMatch is internal/match's fold-loading path (M3, #319) and
-- cmd/replay (M4): it returns EVERY seat's order, for EVERY round, in one
-- match — by design, because fold() needs the whole log to reconstruct
-- state. THIS IS PRE-FOG DATA. It feeds Resolve(), which computes the
-- authoritative MatchState that Project(s, seat) later filters per seat
-- (RFC-001 §3, §9; GDD §7.1). Nothing in internal/store or any caller above
-- it may hand this result to a renderer, an HTTP response, or an SSE push
-- directly — only to the fold pipeline. A future caller reaching for "all
-- seats' orders for the current round" to render something is exactly the
-- one-call-away leak RFC-001 §7.5/D315's own text warns about; this comment
-- is the review trip-wire for that.
--
-- payload surfaces as []byte here — D44's codec boundary, decoded only
-- above internal/store. is_first_submission (the xmax=0 idiom, D52) belongs
-- to the M5 submit handler's Recap-cursor logic, not this query — out of
-- scope here.

-- name: UpsertOrder :one
INSERT INTO orders (
  match_id, round, seat, payload, source
) VALUES (
  $1, $2, $3, $4, $5
)
ON CONFLICT (match_id, round, seat) DO UPDATE
  SET payload = excluded.payload,
      source = excluded.source,
      submitted_at = excluded.submitted_at
RETURNING *;

-- name: ListOrdersForMatch :many
SELECT * FROM orders
WHERE match_id = $1
ORDER BY round, seat;
