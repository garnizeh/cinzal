//go:build integration

package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

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
		// board_notes (D18) and invite_links (D17) are #313's additions,
		// rate_limits (D20) is #314's — this list extends #312's own, per
		// plan, rather than replacing it.
		"auth_codes", "board_notes", "events", "goose_db_version",
		"invite_links", "match_players", "match_summary", "matches",
		"orders", "outbox", "rate_limits", "sessions", "users",
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
		// round = i+1, not i: round is 1-indexed (orders_round_positive,
		// migration 00004) and this loop only needs three distinct rounds
		// so the three inserts don't collide on the primary key — it is not
		// itself asserting anything about round.
		if _, err := db.ExecContext(ctx,
			`INSERT INTO orders (match_id, round, seat, payload, source) VALUES ($1, $2, 0, '{}', $3)`,
			matchID, i+1, source,
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

// TestSchemaOrdersRoundPositiveCheck asserts orders_round_positive
// (migration 00004): game.RoundNumber is 1-indexed (GDD §4), so round 0 and
// below must never reach the table, even by a path that bypasses
// AppendOrder's own Go-level check (internal/store/orders.go) — a
// CodeRabbit finding on PR #393 (issue #317) that checkNoRoundGap's
// [1, maxRound] gap scan alone would let a round-0 row ride along
// undetected whenever the rest of the log is otherwise gapless.
func TestSchemaOrdersRoundPositiveCheck(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, _ := seedMatch(t, db)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO orders (match_id, round, seat, payload, source) VALUES ($1, 1, 0, '{}', 'human')`, matchID,
	); err != nil {
		t.Fatalf("insert with round = 1: %v", err)
	}

	for _, round := range []int{0, -1} {
		_, err := db.ExecContext(ctx,
			`INSERT INTO orders (match_id, round, seat, payload, source) VALUES ($1, $2, 0, '{}', 'human')`,
			matchID, round,
		)
		if !isCheckViolation(err) {
			t.Fatalf("insert with round = %d = %v, want a CHECK violation", round, err)
		}
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
		// unsubscribe_token_hash (D19) is NOT NULL with no DDL default — it is
		// minted in application code at seat creation, so every seed here must
		// supply one; a fixed placeholder is fine, no test relies on its value.
		`INSERT INTO match_players (match_id, seat, user_id, faction, unsubscribe_token_hash)
			VALUES ($1, 0, $2, 'seed', '\x00')`,
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

// TestSchemaRecapCursorDefaultsToZero asserts D16's seat-creation value: 0,
// for both a lobby-formation seat and one that joins mid-lobby — D16's own
// reasoning is that POST /m/{id}/join only ever runs before round 1
// resolves, so no reachable seat needs a different default. seedMatch's
// seat 0 stands in for the lobby-formation case; seat 1, inserted here with
// last_seen_round omitted, stands in for a mid-lobby join.
func TestSchemaRecapCursorDefaultsToZero(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, userID := seedMatch(t, db)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO match_players (match_id, seat, user_id, faction, unsubscribe_token_hash)
			VALUES ($1, 1, $2, 'seed', '\x01')`,
		matchID, userID,
	); err != nil {
		t.Fatalf("seed mid-lobby-join seat 1: %v", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT seat, last_seen_round FROM match_players WHERE match_id = $1 ORDER BY seat`, matchID)
	if err != nil {
		t.Fatalf("query match_players: %v", err)
	}
	defer rows.Close()

	got := map[int]int{}
	for rows.Next() {
		var seat, cursor int
		if err := rows.Scan(&seat, &cursor); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[seat] = cursor
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate: %v", err)
	}
	for _, seat := range []int{0, 1} {
		if got[seat] != 0 {
			t.Fatalf("match_players.last_seen_round for seat %d = %d, want 0 (D16's seat-creation default)", seat, got[seat])
		}
	}
}

// TestSchemaEmailPrefDefault asserts D19's stated default — RFC-001 §13's
// "only when it's my turn and I haven't moved" — takes effect when the
// column is omitted from the insert, not just documented in DDL prose.
func TestSchemaEmailPrefDefault(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, _ := seedMatch(t, db)

	var pref string
	if err := db.QueryRowContext(ctx,
		`SELECT email_pref FROM match_players WHERE match_id = $1 AND seat = 0`, matchID,
	).Scan(&pref); err != nil {
		t.Fatalf("query email_pref: %v", err)
	}
	if pref != "turn_only" {
		t.Fatalf("match_players.email_pref default = %q, want %q (D19, RFC-001 §13)", pref, "turn_only")
	}
}

// TestSchemaEmailPrefCheck asserts D19's CHECK: the three levels it actually
// built are accepted, and daily_digest — deferred by D19, never one of
// these three — is rejected the same way a typo would be.
func TestSchemaEmailPrefCheck(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, userID := seedMatch(t, db)

	for i, pref := range []string{"every_round", "turn_only", "none"} {
		seat := i + 1
		if _, err := db.ExecContext(ctx,
			`INSERT INTO match_players (match_id, seat, user_id, faction, email_pref, unsubscribe_token_hash)
				VALUES ($1, $2, $3, 'seed', $4, '\x00')`,
			matchID, seat, userID, pref,
		); err != nil {
			t.Fatalf("insert with email_pref = %q: %v", pref, err)
		}
	}

	_, err := db.ExecContext(ctx,
		`INSERT INTO match_players (match_id, seat, user_id, faction, email_pref, unsubscribe_token_hash)
			VALUES ($1, 99, $2, 'seed', 'daily_digest', '\x00')`,
		matchID, userID)
	if !isCheckViolation(err) {
		t.Fatalf("insert with email_pref = 'daily_digest' = %v, want a CHECK violation (D19: deferred, not one of the three)", err)
	}
}

// TestSchemaUnsubscribeTokenHashRequired asserts D19's NOT NULL with no DDL
// default: unlike last_seen_round/email_pref, the token has to be supplied
// by the inserting caller (application code mints it at seat creation) —
// an insert that omits it must fail, not silently store an empty token.
func TestSchemaUnsubscribeTokenHashRequired(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, userID := seedMatch(t, db)

	_, err := db.ExecContext(ctx,
		`INSERT INTO match_players (match_id, seat, user_id, faction) VALUES ($1, 1, $2, 'seed')`,
		matchID, userID)
	if err == nil {
		t.Fatal("insert with unsubscribe_token_hash omitted succeeded, want a NOT NULL violation")
	}
}

// TestSchemaInviteLinksTokenHashLookup asserts D17's stated lookup path: an
// invite link is found by an indexed equality scan on token_hash, the same
// shape a real join handler's `WHERE token_hash = $1` would use.
func TestSchemaInviteLinksTokenHashLookup(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, _ := seedMatch(t, db)

	const wantHash = `\xdeadbeef`
	var linkID string
	if err := db.QueryRowContext(ctx,
		`INSERT INTO invite_links (match_id, token_hash) VALUES ($1, $2) RETURNING id`,
		matchID, wantHash,
	).Scan(&linkID); err != nil {
		t.Fatalf("insert invite_links: %v", err)
	}

	var gotID string
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM invite_links WHERE token_hash = $1`, wantHash,
	).Scan(&gotID); err != nil {
		t.Fatalf("lookup by token_hash: %v", err)
	}
	if gotID != linkID {
		t.Fatalf("lookup by token_hash returned id %s, want %s", gotID, linkID)
	}

	// A second link with the same hash must collide — token_hash is UNIQUE
	// (D17), the property that makes the equality lookup above well-defined.
	_, err := db.ExecContext(ctx,
		`INSERT INTO invite_links (match_id, token_hash) VALUES ($1, $2)`, matchID, wantHash)
	if !isUniqueViolation(err) {
		t.Fatalf("second insert with the same token_hash = %v, want a unique violation", err)
	}
}

// TestSchemaInviteLinksNoPlaintextTokenColumn is the schema-level half of
// D17's "the raw token is never stored" — asserted as a shape check, since
// a store test has no application code that could mint one to check for
// leakage in a row. If a future column ever reintroduces the plaintext
// token, this test's exact column set breaks and calls it out by name.
func TestSchemaInviteLinksNoPlaintextTokenColumn(t *testing.T) {
	db := applyBaseSchema(t)

	rows, err := db.QueryContext(context.Background(),
		`SELECT column_name FROM information_schema.columns
		  WHERE table_schema = 'public' AND table_name = 'invite_links' ORDER BY column_name`)
	if err != nil {
		t.Fatalf("query information_schema.columns: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column_name: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate column_name rows: %v", err)
	}

	want := []string{"created_at", "expires_at", "id", "match_id", "revoked_at", "token_hash"}
	if len(got) != len(want) {
		t.Fatalf("invite_links columns = %v, want exactly %v (no plaintext token column, D17)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("invite_links columns = %v, want %v", got, want)
		}
	}
}

// TestSchemaInviteLinkCrossMatchRejected asserts the composite FK
// (invite_link_id, match_id) -> invite_links(id, match_id): a link minted
// for one match cannot be attributed to a seat in a different match. D17's
// own reasoning is that a plain FK to invite_links(id) alone only proves the
// link exists somewhere, not that it belongs to this match.
func TestSchemaInviteLinkCrossMatchRejected(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchA, userID := seedMatch(t, db)
	matchB, _ := seedMatch(t, db)

	var linkID string
	if err := db.QueryRowContext(ctx,
		`INSERT INTO invite_links (match_id, token_hash) VALUES ($1, '\xaaaa') RETURNING id`,
		matchA,
	).Scan(&linkID); err != nil {
		t.Fatalf("insert invite_links for match A: %v", err)
	}

	// Same-match attribution must succeed.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO match_players (match_id, seat, user_id, faction, invite_link_id, unsubscribe_token_hash)
			VALUES ($1, 1, $2, 'seed', $3, '\x00')`,
		matchA, userID, linkID,
	); err != nil {
		t.Fatalf("attribute match A's own link to a match A seat: %v", err)
	}

	// Cross-match attribution — match B's seat citing match A's link — must
	// be rejected by the composite FK.
	_, err := db.ExecContext(ctx,
		`INSERT INTO match_players (match_id, seat, user_id, faction, invite_link_id, unsubscribe_token_hash)
			VALUES ($1, 1, $2, 'seed', $3, '\x00')`,
		matchB, userID, linkID)
	if !isForeignKeyViolation(err) {
		t.Fatalf("attribute match A's link to a match B seat = %v, want a foreign key violation (D17: single-match scope)", err)
	}
}

// TestSchemaBoardNotesSlotCheck asserts D18's per-seat cap: a bounded slot
// number under the table's own primary key, 1..20, not a counted CHECK or a
// trigger.
func TestSchemaBoardNotesSlotCheck(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, _ := seedMatch(t, db)

	for _, slot := range []int{1, 20} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO board_notes (match_id, seat, slot, round, body) VALUES ($1, 0, $2, 1, 'note')`,
			matchID, slot,
		); err != nil {
			t.Fatalf("insert with slot = %d: %v", slot, err)
		}
	}

	for _, slot := range []int{0, 21} {
		_, err := db.ExecContext(ctx,
			`INSERT INTO board_notes (match_id, seat, slot, round, body) VALUES ($1, 0, $2, 1, 'note')`,
			matchID, slot)
		if !isCheckViolation(err) {
			t.Fatalf("insert with slot = %d = %v, want a CHECK violation (D18: slot bound 1..20)", slot, err)
		}
	}
}

// TestSchemaBoardNotesUpsert exercises D18's stated resubmission shape —
// the same ON CONFLICT (match_id, seat, slot) DO UPDATE pattern as orders'
// own resubmission — and asserts updated_at is restated explicitly in the
// SET clause: D18 warns a plain DEFAULT now() only fires on INSERT, so a
// conflict reaching DO UPDATE without it would leave a stale timestamp.
func TestSchemaBoardNotesUpsert(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, _ := seedMatch(t, db)

	const upsert = `INSERT INTO board_notes (match_id, seat, slot, round, body)
		VALUES ($1, 0, 1, $2, $3)
		ON CONFLICT (match_id, seat, slot) DO UPDATE
		  SET node_id = EXCLUDED.node_id, round = EXCLUDED.round,
		      body = EXCLUDED.body, updated_at = now()`
	if _, err := db.ExecContext(ctx, upsert, matchID, 1, "first"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	var firstUpdatedAt time.Time
	if err := db.QueryRowContext(ctx,
		`SELECT updated_at FROM board_notes WHERE match_id = $1 AND seat = 0 AND slot = 1`, matchID,
	).Scan(&firstUpdatedAt); err != nil {
		t.Fatalf("query after first write: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // guarantee now() advances between writes
	if _, err := db.ExecContext(ctx, upsert, matchID, 2, "second"); err != nil {
		t.Fatalf("second write (upsert): %v", err)
	}

	var count int
	var round int
	var body string
	var updatedAt time.Time
	if err := db.QueryRowContext(ctx,
		`SELECT count(*), max(round), max(body), max(updated_at) FROM board_notes WHERE match_id = $1 AND seat = 0 AND slot = 1`,
		matchID,
	).Scan(&count, &round, &body, &updatedAt); err != nil {
		t.Fatalf("query after upsert: %v", err)
	}
	if count != 1 {
		t.Fatalf("board_notes row count = %d after upsert, want exactly 1", count)
	}
	if round != 2 || body != "second" {
		t.Fatalf("board_notes (round, body) = (%d, %q) after upsert, want (2, \"second\")", round, body)
	}
	if !updatedAt.After(firstUpdatedAt) {
		t.Fatalf("board_notes.updated_at did not advance across the upsert (D18: SET updated_at = now() must be explicit)")
	}
}

// TestSchemaBoardNotesSeatScoping asserts D18's isolation guarantee: a query
// scoped to (match_id, seat) never surfaces another seat's rows. This is the
// query shape every real store method must use — the store package has no
// query that omits the seat predicate, per D18's "never rendered to any
// seat but its own author."
func TestSchemaBoardNotesSeatScoping(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()
	matchID, userID := seedMatch(t, db)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO match_players (match_id, seat, user_id, faction, unsubscribe_token_hash)
			VALUES ($1, 1, $2, 'seed', '\x01')`,
		matchID, userID,
	); err != nil {
		t.Fatalf("seed seat 1: %v", err)
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO board_notes (match_id, seat, slot, round, body) VALUES ($1, 0, 1, 1, 'seat 0 note')`,
		matchID,
	); err != nil {
		t.Fatalf("insert seat 0 note: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO board_notes (match_id, seat, slot, round, body) VALUES ($1, 1, 1, 1, 'seat 1 note')`,
		matchID,
	); err != nil {
		t.Fatalf("insert seat 1 note: %v", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT body FROM board_notes WHERE match_id = $1 AND seat = 0`, matchID)
	if err != nil {
		t.Fatalf("query scoped to seat 0: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, body)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate: %v", err)
	}
	if len(got) != 1 || got[0] != "seat 0 note" {
		t.Fatalf("query scoped to (match_id, seat=0) returned %v, want exactly [\"seat 0 note\"] — seat 1's row must never appear", got)
	}
}

// TestSchemaUsersEmailSuppressedAtDefaultsNull asserts D53's all-matches
// unsubscribe flag starts unset for every new user.
func TestSchemaUsersEmailSuppressedAtDefaultsNull(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()

	var suppressed sql.NullTime
	if err := db.QueryRowContext(ctx,
		`INSERT INTO users (display_name) VALUES ('seed') RETURNING email_suppressed_at`,
	).Scan(&suppressed); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if suppressed.Valid {
		t.Fatalf("users.email_suppressed_at = %v on insert, want NULL (D53)", suppressed.Time)
	}
}

// TestSchemaRateLimitsColumnsAndPrimaryKey is #314's DDL-shape assertion:
// rate_limits has exactly the columns D20 specifies, and (scope, key) is the
// real primary key constraint — not merely two columns that happen to exist
// — which is what makes ON CONFLICT (scope, key) in the check-and-consume
// statement (ratelimit.go) a valid conflict target at all.
func TestSchemaRateLimitsColumnsAndPrimaryKey(t *testing.T) {
	db := applyBaseSchema(t)
	ctx := context.Background()

	rows, err := db.QueryContext(ctx,
		`SELECT column_name FROM information_schema.columns
		  WHERE table_schema = 'public' AND table_name = 'rate_limits' ORDER BY column_name`)
	if err != nil {
		t.Fatalf("query information_schema.columns: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column_name: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate column_name rows: %v", err)
	}

	want := []string{"key", "scope", "tokens", "updated_at"}
	if len(got) != len(want) {
		t.Fatalf("rate_limits columns = %v, want exactly %v (D20)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rate_limits columns = %v, want %v", got, want)
		}
	}

	// (scope, key) must be the real primary key constraint, asserted via
	// information_schema rather than assumed from the column list above.
	pkRows, err := db.QueryContext(ctx,
		`SELECT kcu.column_name
		   FROM information_schema.table_constraints tc
		   JOIN information_schema.key_column_usage kcu
		     ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		  WHERE tc.table_schema = 'public' AND tc.table_name = 'rate_limits'
		    AND tc.constraint_type = 'PRIMARY KEY'
		  ORDER BY kcu.ordinal_position`)
	if err != nil {
		t.Fatalf("query primary key constraint: %v", err)
	}
	defer pkRows.Close()

	var pkCols []string
	for pkRows.Next() {
		var name string
		if err := pkRows.Scan(&name); err != nil {
			t.Fatalf("scan primary key column: %v", err)
		}
		pkCols = append(pkCols, name)
	}
	if err := pkRows.Err(); err != nil {
		t.Fatalf("iterate primary key rows: %v", err)
	}

	wantPK := []string{"scope", "key"}
	if len(pkCols) != len(wantPK) {
		t.Fatalf("rate_limits primary key columns = %v, want exactly %v (D20)", pkCols, wantPK)
	}
	for i := range wantPK {
		if pkCols[i] != wantPK[i] {
			t.Fatalf("rate_limits primary key columns = %v, want %v", pkCols, wantPK)
		}
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
