package store

import (
	"context"
	"math"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// panicDBTX is a dbtx whose methods fail the test if ever called — it
// proves ConsumeRateLimit's capacity validation short-circuits before
// consumeRateLimitSQL is issued, rather than merely trusting the returned
// error to be right for the wrong reason (e.g. a query the fixture happens
// to fail for lack of a real connection).
type panicDBTX struct{ t *testing.T }

func (p panicDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	p.t.Fatal("QueryRow called despite an invalid capacity — validation did not short-circuit before the query")
	return nil
}

func (p panicDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	p.t.Fatal("Exec called despite an invalid capacity — validation did not short-circuit before the query")
	return pgconn.CommandTag{}, nil
}

// TestConsumeRateLimitRejectsSubOneCapacityWithoutQuerying is the
// regression case from PR #386's review: consumeRateLimitSQL's WHERE
// clause only guards the ON CONFLICT (refill) branch — a brand-new
// (scope, key) has no conflicting row, so the plain INSERT branch has no
// predicate at all. Before this fix, a capacity of 0 or 0.5 would insert
// tokens = capacity-1 unconditionally and RETURNING would hand back a row,
// so a fresh key with fewer than one token of capacity admitted exactly
// one request — D20's fail-closed policy inverted for any caller bug that
// produced such a capacity.
func TestConsumeRateLimitRejectsSubOneCapacityWithoutQuerying(t *testing.T) {
	invalid := []float64{0, 0.5, -1, math.NaN(), math.Inf(1), math.Inf(-1)}

	for _, capacity := range invalid {
		allowed, err := ConsumeRateLimit(context.Background(), panicDBTX{t: t}, "test_scope", "test_key", capacity, 1)
		if err == nil {
			t.Errorf("ConsumeRateLimit(capacity=%v): err = nil, want a validation error", capacity)
		}
		if allowed {
			t.Errorf("ConsumeRateLimit(capacity=%v): allowed = true, want false (fail-closed on invalid input)", capacity)
		}
	}
}
