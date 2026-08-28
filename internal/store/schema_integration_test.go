//go:build integration

package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// applyBaseSchema runs the real production migration set (not a testdata
// fixture) against a fresh database, via migrate()'s tested core rather
// than the exported Migrate wrapper — this file exercises 00001's DDL
// itself, not the lock/connection machinery migrate_integration_test.go
// already covers.
func applyBaseSchema(t *testing.T) *sql.DB {
	t.Helper()
	dsn := startPostgres(t)
	fsys := sub(t, migrationsFS, "migrations")
	db := openDedicated(t, dsn)
	if err := migrate(context.Background(), db, fsys); err != nil {
		t.Fatalf("migrate() against the production migration set: %v", err)
	}
	return db
}

// TestSchemaBaseTablesExactlyPresent is #312's own fail-closed acceptance
// criterion: the expected table set is exactly present after migration, so
// both an extra table (something added but not intended) and a missing one
// (something intended but not migrated) fail it. #313/#314 extend this
// list, per plan, rather than replacing it.
func TestSchemaBaseTablesExactlyPresent(t *testing.T) {
	db := applyBaseSchema(t)

	rows, err := db.QueryContext(context.Background(),
		"SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name")
	if err != nil {
		t.Fatalf("query information_schema.tables: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table_name: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_name rows: %v", err)
	}

	want := []string{
		// goose_db_version is goose's own migration-tracking table, created
		// alongside every migration it applies — not part of #312's schema,
		// but a real table in `public` all the same, so it belongs in the
		// exact set this test asserts rather than being an unaccounted extra.
		"auth_codes", "events", "goose_db_version", "match_players",
		"match_summary", "matches", "orders", "outbox", "sessions", "users",
	}
	if len(got) != len(want) {
		t.Fatalf("public schema has %d tables %v, want exactly %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("public schema tables = %v, want %v", got, want)
		}
	}
}

// TestSchemaOutboxDedup is RFC-001 §16.1's "Outbox scoping" row, asserted
// against real Postgres per §13.1: two otp rows (match_id NULL) for the
// same email both insert; two round_resolved rows for the same
// (match_id, round, seat) collide on outbox_match_dedup.
func TestSchemaOutboxDedup(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, _ := seedMatch(t, db)

	const insertOTP = `INSERT INTO outbox (to_email, template, payload, send_after)
		VALUES ('player@example.com', 'otp', '{}', now())`
	if _, err := db.ExecContext(ctx, insertOTP); err != nil {
		t.Fatalf("first otp insert: %v", err)
	}
	if _, err := db.ExecContext(ctx, insertOTP); err != nil {
		t.Fatalf("second otp insert (should not collide, no match_id): %v", err)
	}

	const insertRoundResolved = `INSERT INTO outbox (to_email, template, payload, send_after, match_id, round, seat)
		VALUES ('player@example.com', 'round_resolved', '{}', now(), $1, 1, 0)`
	if _, err := db.ExecContext(ctx, insertRoundResolved, matchID); err != nil {
		t.Fatalf("first round_resolved insert: %v", err)
	}
	_, err := db.ExecContext(ctx, insertRoundResolved, matchID)
	if !isUniqueViolation(err) {
		t.Fatalf("second round_resolved insert for the same (match_id, round, seat, template) = %v, want a unique violation on outbox_match_dedup", err)
	}
}

// TestSchemaOrdersResubmissionUpsert exercises GDD §18's "the last
// submission stands" against real Postgres: a second submission for the
// same (match_id, round, seat) via ON CONFLICT DO UPDATE replaces the row
// rather than adding one.
func TestSchemaOrdersResubmissionUpsert(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, _ := seedMatch(t, db)

	const upsert = `INSERT INTO orders (match_id, round, seat, payload, source)
		VALUES ($1, 1, 0, $2, 'human')
		ON CONFLICT (match_id, round, seat) DO UPDATE SET payload = EXCLUDED.payload`
	if _, err := db.ExecContext(ctx, upsert, matchID, `{"first":true}`); err != nil {
		t.Fatalf("first submission: %v", err)
	}
	if _, err := db.ExecContext(ctx, upsert, matchID, `{"first":false}`); err != nil {
		t.Fatalf("resubmission: %v", err)
	}

	var count int
	var payload string
	if err := db.QueryRowContext(ctx,
		"SELECT count(*), max(payload::text) FROM orders WHERE match_id = $1 AND round = 1 AND seat = 0",
		matchID,
	).Scan(&count, &payload); err != nil {
		t.Fatalf("query orders: %v", err)
	}
	if count != 1 {
		t.Fatalf("orders row count = %d after resubmission, want exactly 1 (last submission stands)", count)
	}
	if payload != `{"first": false}` && payload != `{"first":false}` {
		t.Fatalf("orders.payload = %s after resubmission, want the second submission's payload", payload)
	}
}

// TestSchemaOrdersSourceCheck asserts orders.source's CHECK (RFC-001 §8.2):
// only human/bot/default are accepted, since Autopilot is derived from
// this column and a typo here would silently change that derivation.
func TestSchemaOrdersSourceCheck(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, _ := seedMatch(t, db)

	for i, source := range []string{"human", "bot", "default"} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO orders (match_id, round, seat, payload, source) VALUES ($1, $2, 0, '{}', $3)`,
			matchID, i, source,
		); err != nil {
			t.Fatalf("insert with source = %q: %v", source, err)
		}
	}

	_, err := db.ExecContext(ctx,
		`INSERT INTO orders (match_id, round, seat, payload, source) VALUES ($1, 99, 0, '{}', 'typo')`, matchID)
	if !isCheckViolation(err) {
		t.Fatalf("insert with source = 'typo' = %v, want a CHECK violation", err)
	}
}

// TestSchemaMatchesStatusCheck asserts matches.status's CHECK, including
// 'abandoned' — unreachable today (D22 is open) but present in the
// vocabulary so a later migration doesn't need to reopen this constraint.
func TestSchemaMatchesStatusCheck(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	_, userID := seedMatch(t, db)

	for _, status := range []string{"lobby", "active", "finished", "abandoned"} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO matches (status, config, seed, created_by) VALUES ($1, '{}', '\x00', $2)`,
			status, userID,
		); err != nil {
			t.Fatalf("insert with status = %q: %v", status, err)
		}
	}

	_, err := db.ExecContext(ctx,
		`INSERT INTO matches (status, config, seed, created_by) VALUES ('paused', '{}', '\x00', $1)`, userID)
	if !isCheckViolation(err) {
		t.Fatalf("insert with status = 'paused' = %v, want a CHECK violation", err)
	}
}

// TestSchemaOrdersCannotCascadeDelete is the direct assertion behind
// RFC-001 §18's "lose it and matches cannot be reconstructed": deleting a
// matches row with a dependent orders row must fail, not cascade.
func TestSchemaOrdersCannotCascadeDelete(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, _ := seedMatch(t, db)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO orders (match_id, round, seat, payload, source) VALUES ($1, 1, 0, '{}', 'human')`, matchID,
	); err != nil {
		t.Fatalf("seed order: %v", err)
	}

	_, err := db.ExecContext(ctx, `DELETE FROM matches WHERE id = $1`, matchID)
	if err == nil {
		t.Fatal("DELETE FROM matches with a dependent orders row succeeded, want a foreign key violation")
	}
	var count int
	if scanErr := db.QueryRowContext(ctx, `SELECT count(*) FROM orders WHERE match_id = $1`, matchID).Scan(&count); scanErr != nil {
		t.Fatalf("query orders after failed delete: %v", scanErr)
	}
	if count != 1 {
		t.Fatalf("orders row count after failed delete = %d, want 1 (nothing cascaded)", count)
	}
}

// seedMatch inserts one user, one matches row, and a match_players row for
// seat 0 (orders' composite FK requires seat to be an actual participant),
// returning the user and match ids for tests that only need a valid
// foreign-key target.
func seedMatch(t *testing.T, db *sql.DB) (matchID, userID string) {
	t.Helper()
	ctx := context.Background()

	if err := db.QueryRowContext(ctx,
		`INSERT INTO users (display_name) VALUES ('seed') RETURNING id`,
	).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`INSERT INTO matches (config, seed, created_by) VALUES ('{}', '\x00', $1) RETURNING id`, userID,
	).Scan(&matchID); err != nil {
		t.Fatalf("seed match: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO match_players (match_id, seat, user_id, faction) VALUES ($1, 0, $2, 'seed')`,
		matchID, userID,
	); err != nil {
		t.Fatalf("seed match_players seat 0: %v", err)
	}
	return matchID, userID
}

// TestSchemaOrdersRequireValidMatchPlayer asserts the composite FK
// (match_id, seat) -> match_players(match_id, seat): an order for a seat
// with no corresponding participant must be rejected, not stored as an
// order fold/replay can never map back to anyone.
func TestSchemaOrdersRequireValidMatchPlayer(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, _ := seedMatch(t, db)

	// seat 0 was seeded by seedMatch and must succeed.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO orders (match_id, round, seat, payload, source) VALUES ($1, 1, 0, '{}', 'human')`, matchID,
	); err != nil {
		t.Fatalf("insert for seeded seat 0: %v", err)
	}

	// seat 99 has no match_players row for this match and must be rejected.
	_, err := db.ExecContext(ctx,
		`INSERT INTO orders (match_id, round, seat, payload, source) VALUES ($1, 1, 99, '{}', 'human')`, matchID)
	if !isForeignKeyViolation(err) {
		t.Fatalf("insert for seat 99 with no match_players row = %v, want a foreign key violation", err)
	}
}

// isUniqueViolation and isCheckViolation decode pgx's wrapped Postgres
// error code rather than matching on message text, which goose/pgx do not
// guarantee stays stable across versions.
func isUniqueViolation(err error) bool     { return pgErrorCode(err) == "23505" }
func isCheckViolation(err error) bool      { return pgErrorCode(err) == "23514" }
func isForeignKeyViolation(err error) bool { return pgErrorCode(err) == "23503" }

func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
