package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// RateLimitCleanupRetention is D20's cleanup retention window (RFC-001
// §12.3): an hour, auth_ip's own full-refill time and the longer of the two
// v1 scopes' — by then any bucket has certainly refilled to capacity
// regardless of where it stood, so deleting it is exactly equivalent to
// leaving it alone (the consume statement's own LEAST($3, …) clamp).
//
// Named rather than left as a literal inside cleanupRateLimitsSQL: D20 is
// explicit that this constant has to widen if a future scope refills slower
// than auth_ip does, and a named constant makes that widening a reviewed
// diff instead of a buried string edit.
const RateLimitCleanupRetention = time.Hour

// consumeRateLimitSQL is D20's check-and-consume statement, quoted
// verbatim from the decision (RFC-001 §12.3 carries the same text): one
// atomic INSERT … ON CONFLICT DO UPDATE … WHERE … RETURNING, so two
// instances racing the same (scope, key) cannot both observe "under limit"
// and both admit — the two-statement SELECT-then-INSERT shape issue #314
// exists to rule out.
//
// clock_timestamp(), not now(): now() is pinned to the enclosing
// transaction's *start*, and this statement can run inside a transaction
// held open by lock contention from a concurrent request against the same
// key (D50's TX_limits, M5) — a backdated updated_at would let a later
// check compute more elapsed time than actually passed and over-credit the
// refill. clock_timestamp() is evaluated fresh on every call.
//
// $1=scope, $2=key, $3=capacity, $4=refill rate in tokens/second. Zero rows
// returned means limited — see ConsumeRateLimit's own doc comment for how
// that and a query error are collapsed into one signal.
const consumeRateLimitSQL = `
INSERT INTO rate_limits (scope, key, tokens, updated_at)
VALUES ($1, $2, $3 - 1, clock_timestamp())
ON CONFLICT (scope, key) DO UPDATE
  SET tokens = LEAST($3, rate_limits.tokens
                 + EXTRACT(EPOCH FROM (clock_timestamp() - rate_limits.updated_at)) * $4) - 1,
      updated_at = clock_timestamp()
  WHERE LEAST($3, rate_limits.tokens
                 + EXTRACT(EPOCH FROM (clock_timestamp() - rate_limits.updated_at)) * $4) >= 1
RETURNING tokens`

// cleanupRateLimitsSQL is D20's cleanup sweep, restated with the retention
// as a bound parameter (seconds) rather than the literal
// "INTERVAL '1 hour'" D20's own text shows — make_interval(secs => $1) is
// the exact same computation against RateLimitCleanupRetention.Seconds(),
// without a second hardcoded "one hour" for the two to silently drift
// apart from. RateLimitCleanupRetention above is the single source of
// truth for the number itself.
const cleanupRateLimitsSQL = `DELETE FROM rate_limits WHERE updated_at < now() - make_interval(secs => $1)`

// dbtx is the minimal pgx surface ConsumeRateLimit and CleanupRateLimits
// need — satisfied by both *pgxpool.Pool and pgx.Tx (and by sqlc's own
// generated DBTX once #315 lands, which this may fold into then). Neither
// function needs to know whether it is running standalone against the pool
// or coupled to a sibling consume inside one transaction, which is exactly
// what D50 (RFC-001 §12.3) needs from the auth_email/auth_ip pair in M5's
// TX_limits: two consumes that commit or roll back together.
type dbtx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ConsumeRateLimit checks and consumes one token from the bucket named by
// (scope, key), atomically, in the single round trip consumeRateLimitSQL
// performs. capacity and refillPerSecond are entirely caller-supplied —
// this package has no opinion on what a scope's numbers are; D20 places
// auth_email/auth_ip's own constants in internal/auth (M5), not here, so
// the same table and statement serve any future scope with no migration.
//
// capacity is validated before consumeRateLimitSQL ever runs: the statement
// only guards the ON CONFLICT (refill) branch with its WHERE clause (line
// 48 above) — a brand-new (scope, key) has no conflicting row, so the plain
// INSERT ... VALUES ($3 - 1, ...) branch has no predicate at all. A caller
// bug that passed a capacity below one (or a non-finite float) would
// otherwise admit exactly one request the first time a fresh key was seen,
// which is D20's fail-closed posture inverted by construction rather than
// by a code path anyone chose. A caller passing a bad capacity is a bug,
// and D20's fail-closed policy says a bug in the check must deny, not
// admit — so this returns an error and never issues the query.
//
// The store owns the counter, not the policy (issue #314): what ConsumeRateLimit
// hands back is deliberately just "was this admitted", collapsing two
// different D20 outcomes into that one signal on purpose. allowed is false
// both when the bucket legitimately has no tokens left (err is nil) and
// when the check itself errors — a timeout, a connection failure, anything
// (err is non-nil). D20's fail-closed policy — "a rate_limits check that
// errors is treated identically to a returned zero rows: limited, no code
// issued, no mail enqueued" — is thereby the only representable outcome of
// this function, rather than a convention a future caller could invert by
// checking err before allowed. err is still returned, non-nil, on a real
// failure so a caller can log or alert on it; it just never has to be
// consulted to get the fail-closed behavior right.
func ConsumeRateLimit(ctx context.Context, exec dbtx, scope, key string, capacity, refillPerSecond float64) (allowed bool, err error) {
	if math.IsNaN(capacity) || math.IsInf(capacity, 0) || capacity < 1 {
		return false, fmt.Errorf("store: capacity must be a finite number >= 1, got %v", capacity)
	}

	var tokens float64
	err = exec.QueryRow(ctx, consumeRateLimitSQL, scope, key, capacity, refillPerSecond).Scan(&tokens)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CleanupRateLimits deletes every rate_limits row whose bucket was last
// touched more than retention ago and reports how many rows it removed.
// An idle bucket that old has certainly refilled to capacity by
// consumeRateLimitSQL's own LEAST($3, …) clamp (D20), so deleting it loses
// no state a future check would have seen differently.
//
// This is the callable operation only — D20 also names an in-process
// ticker that fires this every 10 minutes in production, the same shape as
// RFC-001 §8's deadline sweeper, but nothing in this repository runs a
// long-lived process yet: cmd/server is doc.go-only until a later
// milestone. Wiring a ticker to call this on a schedule is that milestone's
// job, not this migration-and-queries task's; #314's own acceptance
// criteria ask for the DELETE operation itself, proven against real
// Postgres, which is what this function and its test give.
func CleanupRateLimits(ctx context.Context, exec dbtx, retention time.Duration) (deleted int64, err error) {
	tag, err := exec.Exec(ctx, cleanupRateLimitsSQL, retention.Seconds())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ConsumeRateLimit runs ConsumeRateLimit against this Store's own pool —
// the ordinary, standalone case. A caller coupling this to a sibling
// consume inside one transaction (D50's TX_limits, M5) calls the
// package-level ConsumeRateLimit directly with a pgx.Tx instead.
func (s *Store) ConsumeRateLimit(ctx context.Context, scope, key string, capacity, refillPerSecond float64) (allowed bool, err error) {
	return ConsumeRateLimit(ctx, s.pool, scope, key, capacity, refillPerSecond)
}

// CleanupRateLimits runs CleanupRateLimits against this Store's own pool.
func (s *Store) CleanupRateLimits(ctx context.Context, retention time.Duration) (deleted int64, err error) {
	return CleanupRateLimits(ctx, s.pool, retention)
}
