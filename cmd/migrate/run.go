package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/garnizeh/cinzal/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// run is main's entire implementation, factored out so it can be driven
// directly from a test with a controlled argv — the same shape
// cmd/simulate's and cmd/replay's own run/runWithDeps split already uses.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbFlag := fs.String("db", "", "database connection string (e.g. $DATABASE_URL); required")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *dbFlag == "" {
		logLine(stderr, "cmd/migrate: --db is required")
		return 1
	}

	ctx := context.Background()

	before, err := countAppliedMigrations(ctx, *dbFlag)
	if err != nil {
		logLine(stderr, "cmd/migrate: count applied migrations before: %v", err)
		return 1
	}

	// store.Migrate itself validates *dbFlag (RFC-001 §18: no fallback DSN)
	// before dialing anything, the same guard every other entry point into
	// internal/store gets — countAppliedMigrations above already dialed
	// successfully, so a DSN bad enough to be rejected here would have
	// already failed there.
	if err := store.Migrate(ctx, *dbFlag); err != nil {
		logLine(stderr, "cmd/migrate: %v", err)
		return 1
	}

	after, err := countAppliedMigrations(ctx, *dbFlag)
	if err != nil {
		logLine(stderr, "cmd/migrate: count applied migrations after: %v", err)
		return 1
	}

	applied, ok := evaluateMigration(before, after)
	if !ok {
		logLine(stderr, "cmd/migrate: database had zero migrations applied before this run, and this run "+
			"applied zero too — internal/store/migrations is empty or misconfigured, this is not a success")
		return 1
	}

	logLine(stdout, "cmd/migrate: applied %d migration(s) (%d total)", applied, after)
	return 0
}

// evaluateMigration turns a before/after applied-migration count into the
// delta to report and whether the run counts as a success. Pure and
// side-effect-free so the fail-closed case (issue #326's own acceptance
// criterion: "a run that applies zero migrations against an empty database
// is a failure, not a success") is testable without a database.
//
// before == 0 means goose_db_version was absent or empty going in — a
// genuinely fresh database. If after is also 0, internal/store/migrations
// had nothing to apply against it, which can only be a bug (an empty or
// misconfigured migrations set), not a legitimate no-op — ok is false.
// Any other case (something was already applied, or something got applied
// this run) is a real success, including the ordinary idempotent re-run
// (before > 0, after == before).
func evaluateMigration(before, after int) (delta int, ok bool) {
	delta = after - before
	if before == 0 && after == 0 {
		return delta, false
	}
	return delta, true
}

// countAppliedMigrations opens one short-lived connection to dsn and counts
// goose_db_version's applied rows — the same query
// TestConcurrencyMigrateAppliesEachMigrationExactlyOnce
// (migrate_integration_test.go) already asserts against. A database that
// has never been migrated has no goose_db_version table at all; that
// specific error (Postgres SQLSTATE 42P01, undefined_table) is reported as
// zero applied rather than propagated, since "the table doesn't exist yet"
// and "the table exists and is empty" mean the same thing here. Any other
// error (a bad DSN, a network failure, a permissions error) is returned as
// a real failure.
func countAppliedMigrations(ctx context.Context, dsn string) (int, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return 0, fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var n int
	err = conn.QueryRow(ctx, "SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0").Scan(&n)
	if err == nil {
		return n, nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" { // undefined_table
		return 0, nil
	}
	return 0, fmt.Errorf("query goose_db_version: %w", err)
}

// logLine writes one diagnostic line to w, discarding the write error
// deliberately — a failure to write a stderr line isn't actionable here,
// and every call site already reports its own error (or none) through
// run's return code, not through this line landing. Mirrors cmd/replay's
// and cmd/simulate's own logLine.
func logLine(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}
