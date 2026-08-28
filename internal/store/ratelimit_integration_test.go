//go:build integration

package store

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// openRateLimitStore migrates the real production migration set (including
// 00003_rate_limits.sql) against a fresh database, then opens a *Store
// against the same DSN — the actual production path (Open, pool.go) rather
// than the database/sql handle applyBaseSchema (schema_integration_test.go)
// returns, since ConsumeRateLimit/CleanupRateLimits need a pgx dbtx.
func openRateLimitStore(t *testing.T) *Store {
	t.Helper()
	dsn := startPostgres(t)
	fsys := sub(t, migrationsFS, "migrations")

	migrateDB := openDedicated(t, dsn)
	if err := migrate(context.Background(), migrateDB, fsys); err != nil {
		t.Fatalf("migrate() against the production migration set: %v", err)
	}

	s, err := Open(context.Background(), Config{DSN: dsn})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

// TestRateLimitConcurrencyAdmitsExactlyCapacity is #314's own load-bearing
// acceptance criterion: "N goroutines racing against a limit of K admit
// exactly K, asserted against real Postgres with N substantially greater
// than K." refillPerSecond is 0 so no refill can happen mid-race — the
// bound this test proves is about the check-and-consume statement's
// atomicity, not about timing.
func TestRateLimitConcurrencyAdmitsExactlyCapacity(t *testing.T) {
	s := openRateLimitStore(t)
	ctx := context.Background()

	const capacity = 10
	const n = 200 // substantially greater than capacity, per the acceptance criterion

	var admitted int64
	var refused int64
	var unexpectedErrs int64

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ok, err := s.ConsumeRateLimit(ctx, "test_scope", "test_key_concurrency", capacity, 0)
			if err != nil {
				atomic.AddInt64(&unexpectedErrs, 1)
				return
			}
			if ok {
				atomic.AddInt64(&admitted, 1)
			} else {
				atomic.AddInt64(&refused, 1)
			}
		}()
	}
	wg.Wait()

	if unexpectedErrs != 0 {
		t.Fatalf("%d of %d ConsumeRateLimit calls returned an unexpected error", unexpectedErrs, n)
	}
	// Both bounds, explicitly — not just "at most capacity": a limiter that
	// refuses everything also satisfies "no more than capacity admitted."
	if admitted != capacity {
		t.Fatalf("admitted = %d, want exactly %d (capacity)", admitted, capacity)
	}
	if admitted+refused != n {
		t.Fatalf("admitted(%d) + refused(%d) = %d, want %d (every call accounted for)", admitted, refused, admitted+refused, n)
	}
}

// TestRateLimitFailurePolicyIsFailClosed forces ConsumeRateLimit's own query
// to fail — a closed pool, a real connection failure rather than a
// simulated one — and asserts the D20 fail-closed outcome: allowed is
// false, and the caller can see it failed via err. This is the "the
// fail-open/fail-closed decision actually taking effect" test #314 asks
// for, not just documented in a comment.
func TestRateLimitFailurePolicyIsFailClosed(t *testing.T) {
	s := openRateLimitStore(t)
	s.Close() // closes the pool: every subsequent query fails, deterministically

	allowed, err := s.ConsumeRateLimit(context.Background(), "test_scope", "test_key_failclosed", 10, 1)
	if err == nil {
		t.Fatal("ConsumeRateLimit against a closed pool returned a nil error, want a real connection failure")
	}
	if allowed {
		t.Fatal("ConsumeRateLimit against a closed pool returned allowed=true, want false (D20: fail-closed)")
	}
}

// TestRateLimitCleanupRemovesStaleRowsOnly is the cleanup acceptance
// criterion: "rows past their window are removed by the named mechanism,
// asserted by a test that advances time rather than by inspection." Time is
// advanced on the data itself — one row's updated_at is backdated past
// RateLimitCleanupRetention — rather than by sleeping real wall-clock time.
func TestRateLimitCleanupRemovesStaleRowsOnly(t *testing.T) {
	s := openRateLimitStore(t)
	ctx := context.Background()

	// Fresh: well within retention.
	if _, err := s.ConsumeRateLimit(ctx, "test_scope", "fresh_key", 10, 1); err != nil {
		t.Fatalf("seed fresh row: %v", err)
	}
	// Stale: consume once to create the row, then backdate updated_at
	// directly, simulating a bucket untouched for two full retention windows.
	if _, err := s.ConsumeRateLimit(ctx, "test_scope", "stale_key", 10, 1); err != nil {
		t.Fatalf("seed stale row: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rate_limits SET updated_at = now() - make_interval(secs => $1) WHERE scope = $2 AND key = $3`,
		(2 * RateLimitCleanupRetention).Seconds(), "test_scope", "stale_key",
	); err != nil {
		t.Fatalf("backdate stale row: %v", err)
	}

	deleted, err := s.CleanupRateLimits(ctx, RateLimitCleanupRetention)
	if err != nil {
		t.Fatalf("CleanupRateLimits: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("CleanupRateLimits deleted %d rows, want exactly 1 (the backdated one)", deleted)
	}

	var staleCount, freshCount int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM rate_limits WHERE scope = $1 AND key = $2`,
		"test_scope", "stale_key").Scan(&staleCount); err != nil {
		t.Fatalf("query stale row: %v", err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM rate_limits WHERE scope = $1 AND key = $2`,
		"test_scope", "fresh_key").Scan(&freshCount); err != nil {
		t.Fatalf("query fresh row: %v", err)
	}
	if staleCount != 0 {
		t.Fatal("the backdated (stale) row survived CleanupRateLimits")
	}
	if freshCount != 1 {
		t.Fatal("the fresh row was removed by CleanupRateLimits, want it to survive")
	}
}

// TestRateLimitWorstCaseBoundedToDoubleCapacity documents D20's stated seam
// behavior rather than leaving it unasserted: a bucket drained, refilled to
// capacity, then drained again admits at most capacity+capacity within one
// rolling window — the same bound a fixed window has, accepted per D20's
// Reasoning — not more.
func TestRateLimitWorstCaseBoundedToDoubleCapacity(t *testing.T) {
	s := openRateLimitStore(t)
	ctx := context.Background()

	const capacity = 2
	const refillPerSecond = 2 // full refill in exactly 1s

	drain := func() int {
		admitted := 0
		for i := 0; i < capacity+1; i++ { // one extra attempt to prove the bound holds
			ok, err := s.ConsumeRateLimit(ctx, "test_scope", "test_key_worstcase", capacity, refillPerSecond)
			if err != nil {
				t.Fatalf("ConsumeRateLimit: %v", err)
			}
			if ok {
				admitted++
			}
		}
		return admitted
	}

	first := drain()
	if first != capacity {
		t.Fatalf("first drain admitted %d, want exactly %d", first, capacity)
	}

	time.Sleep(1100 * time.Millisecond) // past the 1s full-refill point, with margin

	second := drain()
	if second != capacity {
		t.Fatalf("second drain (after a full refill) admitted %d, want exactly %d", second, capacity)
	}

	total := first + second
	if total != 2*capacity {
		t.Fatalf("total admitted across both drains = %d, want exactly %d (D20's stated worst case, capacity+capacity)", total, 2*capacity)
	}
}
