//go:build integration

package storetest

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"testing"

	"github.com/garnizeh/cinzal/internal/store"
	"github.com/garnizeh/cinzal/internal/store/pgimage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// adminDatabase is the container's own bootstrap database — always present
// on any Postgres server, and deliberately never migrated or cloned from,
// so a connection to it is always safe to use for CREATE DATABASE/DROP
// DATABASE regardless of what state idleDatabase or workDatabase are in.
const adminDatabase = "postgres"

// idleDatabase carries the one, real, migrated schema every other database
// in this package clones from. Nothing but setup's own Migrate call ever
// connects to it directly — Postgres refuses CREATE DATABASE ... TEMPLATE
// while any other session holds a connection to the source, so keeping it
// permanently idle is what lets FreshDatabase/FreshUnmigratedDatabase clone
// from it at any point without racing an open connection.
const idleDatabase = "cinzal_idle"

// workDatabase is cloned once from idleDatabase and is what Container's own
// per-test transactions actually run against — a separate database from
// idleDatabase for the same reason: ordinary tests hold open connections to
// it, which would otherwise block any later clone of idleDatabase itself.
const workDatabase = "cinzal_work"

var (
	setupOnce sync.Once
	setupErr  error
	baseDSN   string
	workPool  *pgxpool.Pool
)

// setup starts the pinned-digest container, migrates idleDatabase via the
// real production store.Migrate (RFC-001 §7.5's goose.Up against the
// embedded migrations, advisory lock included — never a dumped schema),
// and clones workDatabase from it. It runs at most once per test binary
// (sync.Once, keyed per binary — which is per package, the unit go test
// ./... already runs as a separate process) and is only ever invoked from
// inside a test body (ensureSetup, below), never from TestMain, so that
// `go test -list` never touches Docker.
//
// Deliberately no explicit container cleanup is registered here: there is
// no single *testing.T whose Cleanup would fire at the right time — the
// first test to call ensureSetup finishes long before the package's other
// tests do, and registering cleanup on it would tear the shared container
// down while its siblings still need it. testcontainers-go's own reaper
// (started automatically alongside the container) removes it when this
// test binary's process exits instead, which is exactly the container's
// intended lifetime.
func setup() {
	ctx := context.Background()

	ctr, err := postgres.Run(ctx, pgimage.Ref,
		postgres.WithDatabase(adminDatabase),
		postgres.WithUsername("cinzal"),
		postgres.WithPassword("cinzal"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		setupErr = fmt.Errorf("storetest: start postgres container: %w", err)
		return
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		setupErr = fmt.Errorf("storetest: postgres connection string: %w", err)
		return
	}
	baseDSN = dsn

	if err := adminExec(ctx, fmt.Sprintf("CREATE DATABASE %s", quoteIdent(idleDatabase))); err != nil {
		setupErr = fmt.Errorf("storetest: create idle template database: %w", err)
		return
	}
	if err := store.Migrate(ctx, dsnFor(idleDatabase)); err != nil {
		setupErr = fmt.Errorf("storetest: migrate idle template database: %w", err)
		return
	}
	if err := adminExec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", quoteIdent(workDatabase), quoteIdent(idleDatabase))); err != nil {
		setupErr = fmt.Errorf("storetest: create work database: %w", err)
		return
	}

	pool, err := pgxpool.New(ctx, dsnFor(workDatabase))
	if err != nil {
		setupErr = fmt.Errorf("storetest: open work database pool: %w", err)
		return
	}
	workPool = pool
}

// ensureSetup runs setup at most once and fails t, on every call, if it
// ever failed. The error is captured inside setup rather than raised with
// t.Fatal there deliberately: sync.Once.Do marks itself done via a deferred
// store, so it completes even when its function calls runtime.Goexit —
// which is exactly what t.Fatal does — and a t.Fatal called from inside the
// guarded closure would let Once report "done" after a failed start,
// letting every later test in the package sail past a container that never
// came up. Calling t.Fatal here instead, after Do returns, on every call,
// fails every test in the package the same way a failed first attempt
// would, not just the first one loudly and everything after it silently.
func ensureSetup(t *testing.T) {
	t.Helper()
	setupOnce.Do(setup)
	if setupErr != nil {
		t.Fatalf("storetest: %v", setupErr)
	}
}

// Container returns a *store.Store for an ordinary Integration-layer test
// (RFC-001 §16.1) — backed by a transaction opened against the shared work
// database and rolled back in t.Cleanup, so no commit this test or the
// *Store under test makes is ever visible outside it. store.NewFromTx
// documents why a *Store built this way behaves identically to one built
// by store.Open: the transaction satisfies the exact same internal surface
// a pool does, including Begin, so the *Store's own internal transactions
// become savepoints nested inside this one.
func Container(t *testing.T) *store.Store {
	t.Helper()
	ensureSetup(t)

	ctx := context.Background()
	tx, err := workPool.Begin(ctx)
	if err != nil {
		t.Fatalf("storetest: begin work transaction: %v", err)
	}
	t.Cleanup(func() {
		_ = tx.Rollback(context.Background())
	})

	return store.NewFromTx(tx)
}

// dsnFor returns baseDSN with its database name replaced by name. baseDSN
// is only ever set once, by setup, before any caller can reach this
// function.
func dsnFor(name string) string {
	u, err := url.Parse(baseDSN)
	if err != nil {
		// baseDSN comes straight from testcontainers' own ConnectionString;
		// a URL it cannot parse itself is this package's own bug, not a
		// caller error worth a returned error for.
		panic(fmt.Sprintf("storetest: parse base dsn: %v", err))
	}
	u.Path = "/" + name
	return u.String()
}

// quoteIdent safely quotes a Postgres identifier this package generated
// itself (never user input) for interpolation into a CREATE/DROP DATABASE
// statement — neither supports parameter placeholders.
func quoteIdent(name string) string {
	return `"` + name + `"`
}

// adminExec opens one short-lived connection to adminDatabase, runs sql,
// and closes it — CREATE DATABASE and DROP DATABASE must run outside any
// transaction and never against the database being created/dropped/cloned,
// so every DDL call in this package goes through a fresh connection here
// rather than a shared, long-lived one.
func adminExec(ctx context.Context, sql string) error {
	conn, err := pgx.Connect(ctx, dsnFor(adminDatabase))
	if err != nil {
		return fmt.Errorf("connect to admin database: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, sql); err != nil {
		return fmt.Errorf("exec %q: %w", sql, err)
	}
	return nil
}
