package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// migrationLockID is the Postgres advisory lock key that serializes
// concurrently-booting instances during migration (RFC-001 §7.5).
//
// This value must never change. Two instances of the same deploy agree to
// serialize only because they hash the identical constant into
// pg_advisory_lock; an old instance and a newly-deployed one holding
// different lock IDs race exactly as if there were no lock at all — the
// failure this whole mechanism exists to remove.
const migrationLockID int64 = 7_312_005_501_884

// migrationsFS embeds internal/store/migrations, which carries a README
// alongside the real migrations so this embed directive always has a
// non-empty, non-dotfile directory to embed; go:embed over an empty
// directory is a compile error, and a directory embed excludes dotfiles
// (".gitkeep") by default. goose's own scan ignores any file that isn't
// NNN_name.sql/.go, so the README is inert to it.
//
//go:embed migrations
var migrationsFS embed.FS

// Migrate applies every pending goose migration embedded in migrations/,
// serialized against every other instance racing the same DSN by a
// session-scoped Postgres advisory lock (RFC-001 §7.5). It refuses to
// return until migration has either completed or definitively failed —
// RFC-001 §18: "The process refuses to serve traffic until migration
// completes, and exits non-zero on failure so the orchestrator restarts
// rather than serving against a half-migrated schema."
//
// Migrate owns its own connection rather than accepting a caller-supplied
// *sql.DB: the advisory lock is session-scoped, and a *sql.DB backed by an
// ordinary pool can silently route the unlock to a different physical
// connection than the lock — goose's own README calls this "the single most
// common way this pattern is written wrong," producing a lock that is never
// released and a second instance that hangs forever rather than failing.
// Migrate pins its connection pool to exactly one connection so that cannot
// happen, rather than leaving it as a caller obligation.
func Migrate(ctx context.Context, dsn string) error {
	db, err := openSingleConnection(dsn)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("store: close migration connection", "error", closeErr)
		}
	}()

	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("store: migrations sub-filesystem: %w", err)
	}
	return migrate(ctx, db, fsys)
}

// openSingleConnection builds a *sql.DB backed by exactly one physical
// connection, dedicated to migration. requireExplicitHost is the same guard
// Open (pool.go) applies — RFC-001 §18's no-fallback-DSN rule binds every
// entry point into this package, not only the pool's.
func openSingleConnection(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, ErrEmptyDSN
	}
	if err := requireExplicitHost(dsn); err != nil {
		return nil, err
	}

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}
	// RFC-001 §7.5 / D55: same UTC session pin as the pool (pool.go) —
	// RuntimeParams is carried in the connection's own startup packet.
	cfg.RuntimeParams["timezone"] = "UTC"

	db := stdlib.OpenDB(*cfg)
	// Exactly one physical connection for this *sql.DB's entire lifetime:
	// the lock and its release must land on the same Postgres session, and
	// database/sql gives no other way to guarantee that against a driver
	// that otherwise multiplexes across a pool.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	return db, nil
}

// migrate is Migrate's testable core: given an already-open, single-
// connection db and a migration source, it acquires the advisory lock,
// applies every pending migration through goose, and releases the lock
// unconditionally — success or failure.
//
// goose.NewProvider is used instead of the package-level
// SetBaseFS/SetDialect/Up trio deliberately: that trio is global mutable
// state, and two Migrate calls racing each other (the acceptance
// criterion's own concurrency test) would then race those globals under
// -race for a reason that has nothing to do with the advisory lock under
// test. A Provider carries its dialect and filesystem as values, so two
// concurrent calls each build their own and never share mutable package
// state.
func migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("store: acquire migration lock: %w", err)
	}
	defer releaseMigrationLock(db)

	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys)
	if err != nil {
		return fmt.Errorf("store: build migration provider: %w", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("store: apply migrations: %w", err)
	}
	return nil
}

// releaseMigrationLock unlocks migrationLockID on db's one dedicated
// connection, deliberately on a fresh background context rather than the
// caller's ctx: a ctx that is already canceled or past its deadline (the
// common case when migrate is returning *because* it failed) must not also
// block releasing the lock, or a subsequent Migrate call could never
// acquire it. A failed unlock is logged, not discarded — an operator needs
// to know a session-scoped lock may still be held until this connection's
// own session ends, even though that is harmless for a process about to
// exit and the reason this function does not treat it as fatal.
//
// pg_advisory_unlock returns a boolean rather than erroring when the
// current session does not hold the lock (Postgres docs, §9.27.5) — it
// only ever raises a SQL warning, not an error. QueryRowContext, not
// ExecContext, is required here: Exec would silently discard that boolean,
// and a false result is exactly the case this function exists to surface.
func releaseMigrationLock(db *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var released bool
	if err := db.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockID).Scan(&released); err != nil {
		slog.Error("store: release migration advisory lock", "error", err)
		return
	}
	if !released {
		slog.Error("store: release migration advisory lock: session did not hold it", "lock_id", migrationLockID)
	}
}
