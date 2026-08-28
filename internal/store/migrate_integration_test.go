//go:build integration

package store

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// postgresImage pins the exact digest D46 (#309) already decided for the
// whole persistence layer's test suite — the postgres:18.6 manifest-list
// digest, verified directly against the registry, never a floating tag,
// since §8.1's deadline-boundary test needs an identical database byte for
// byte on every run.
//
// This file is a deliberately ad hoc, package-local container helper, not
// the general storetest package RFC-001 §16.1/D46 describes. #325 owns
// that package and is itself blocked by this issue — it runs Migrate,
// which does not exist until this PR — so this constant and the helpers
// below exist only to make #311's own acceptance criteria checkable, and
// are expected to be lifted into storetest largely unchanged once #325
// lands.
const postgresImage = "postgres@sha256:4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280"

// okFixtureFS, failingFixtureFS and slowFixtureFS are test-only migration
// sets, embedded from this file rather than from internal/store/migrations
// — the production migrations directory is deliberately empty until #312,
// and these fixtures must never be mistaken for real schema.
//
//go:embed testdata/migrations
var okFixtureFS embed.FS

//go:embed testdata/migrations_failing
var failingFixtureFS embed.FS

//go:embed testdata/migrations_slow
var slowFixtureFS embed.FS

func sub(t *testing.T, fsys embed.FS, dir string) fs.FS {
	t.Helper()
	s, err := fs.Sub(fsys, dir)
	if err != nil {
		t.Fatalf("fs.Sub(%q): %v", dir, err)
	}
	return s
}

// startPostgres starts one pinned-digest Postgres container for a single
// test and returns a DSN naming an explicit host, so it satisfies
// requireExplicitHost the same as any production DSN must. Each test gets
// its own container — simplicity over speed, appropriate for the three
// tests this file owns; #325's storetest amortizes this cost per package.
func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase("cinzal_test"),
		postgres.WithUsername("cinzal"),
		postgres.WithPassword("cinzal"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	testcontainers.CleanupContainer(t, ctr)

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}
	return dsn
}

// openDedicated opens its own single-connection *sql.DB against dsn — the
// exact same construction openSingleConnection (migrate.go) uses, called
// directly since these tests exercise migrate()'s core, not the exported
// Migrate wrapper.
func openDedicated(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := openSingleConnection(dsn)
	if err != nil {
		t.Fatalf("openSingleConnection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() }) // best-effort; the container itself is torn down right after
	return db
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)", name,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("check table %q exists: %v", name, err)
	}
	return exists
}

// TestConcurrencyMigrateAppliesEachMigrationExactlyOnce is #311's own
// acceptance criterion in unit form: two Migrate calls against one fresh
// database, started simultaneously, both return nil, and goose's version
// table shows each migration applied exactly once. The assertion is on a
// positive, exact count against the fixture's known migration count —
// "both calls returned nil" alone would pass against an empty migrations
// directory, which is exactly the vacuous pass the issue calls out.
func TestConcurrencyMigrateAppliesEachMigrationExactlyOnce(t *testing.T) {
	dsn := startPostgres(t)
	fsys := sub(t, okFixtureFS, "testdata/migrations")

	db1 := openDedicated(t, dsn)
	db2 := openDedicated(t, dsn)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errs[0] = migrate(context.Background(), db1, fsys) }()
	go func() { defer wg.Done(); errs[1] = migrate(context.Background(), db2, fsys) }()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent migrate() call %d: %v", i, err)
		}
	}

	// A third, uninvolved connection reads the outcome — both racing
	// connections may already be mid-Close by the time this runs.
	verify := openDedicated(t, dsn)
	var applied int
	if err := verify.QueryRowContext(context.Background(),
		"SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0",
	).Scan(&applied); err != nil {
		t.Fatalf("query goose_db_version: %v", err)
	}
	const wantMigrations = 2 // testdata/migrations: 00001, 00002
	if applied != wantMigrations {
		t.Fatalf("goose_db_version shows %d applied migrations, want exactly %d — "+
			"either the concurrent calls double-applied one, or the fixture didn't run at all",
			applied, wantMigrations)
	}
	if !tableExists(t, verify, "widgets") || !tableExists(t, verify, "gadgets") {
		t.Fatal("both fixture migrations reported applied, but their tables are missing")
	}
}

// TestConcurrencyMigrateHoldsLockOnDedicatedConnection asserts the
// session-scoping guarantee directly: while migrate() is mid-migration, the
// advisory lock is held by exactly the one backend PID belonging to
// migrate()'s own dedicated connection — not merely "some connection in a
// pool" — and it is released by the time migrate() returns.
func TestConcurrencyMigrateHoldsLockOnDedicatedConnection(t *testing.T) {
	dsn := startPostgres(t)
	fsys := sub(t, slowFixtureFS, "testdata/migrations_slow")

	db := openDedicated(t, dsn)
	var backendPID int
	if err := db.QueryRowContext(context.Background(), "SELECT pg_backend_pid()").Scan(&backendPID); err != nil {
		t.Fatalf("query pg_backend_pid: %v", err)
	}

	observer := openDedicated(t, dsn)

	done := make(chan error, 1)
	go func() { done <- migrate(context.Background(), db, fsys) }()

	deadline := time.Now().Add(10 * time.Second)
	held := false
	for time.Now().Before(deadline) {
		var n int
		if err := observer.QueryRowContext(context.Background(),
			"SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND granted AND pid = $1", backendPID,
		).Scan(&n); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if n == 1 {
			held = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !held {
		t.Fatal("advisory lock never observed held by migrate()'s own dedicated connection")
	}

	if err := <-done; err != nil {
		t.Fatalf("migrate() returned an error: %v", err)
	}

	var n int
	if err := observer.QueryRowContext(context.Background(),
		"SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND pid = $1", backendPID,
	).Scan(&n); err != nil {
		t.Fatalf("query pg_locks after completion: %v", err)
	}
	if n != 0 {
		t.Fatalf("advisory lock still held by pid %d after migrate() returned", backendPID)
	}
}

// TestIntegrationReleaseMigrationLockLogsWhenNotHeld exercises the branch
// ExecContext used to hide entirely: pg_advisory_unlock returns false, not
// an error, when the current session does not hold the lock (Postgres
// docs, §9.27.5), so a bare Exec sees a normal successful query and never
// notices. This calls releaseMigrationLock directly against a connection
// that never acquired the lock and asserts the false-result branch logs,
// by swapping in a buffered slog handler for the duration of the call.
func TestIntegrationReleaseMigrationLockLogsWhenNotHeld(t *testing.T) {
	dsn := startPostgres(t)
	db := openDedicated(t, dsn)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	releaseMigrationLock(db) // never acquired — must observe and log released == false

	if got := buf.String(); !strings.Contains(got, "did not hold it") {
		t.Fatalf("releaseMigrationLock on an unheld lock logged %q, want a message noting the lock was not held", got)
	}
}

// TestIntegrationMigrateReleasesLockOnMidMigrationFailure is the failure
// half of the acceptance criteria: a migration that fails mid-file leaves
// the lock released, and a subsequent Migrate on the same database is able
// to acquire it rather than hanging. The proof is in *how* the second call
// fails: its error must come from applying migrations, not from acquiring
// the lock — if the lock were still held, the second call would block
// inside pg_advisory_lock until its own context deadline instead.
func TestIntegrationMigrateReleasesLockOnMidMigrationFailure(t *testing.T) {
	dsn := startPostgres(t)
	fsys := sub(t, failingFixtureFS, "testdata/migrations_failing")

	db1 := openDedicated(t, dsn)
	if err := migrate(context.Background(), db1, fsys); err == nil {
		t.Fatal("migrate() with a deliberately broken second migration returned nil error")
	} else if !strings.Contains(err.Error(), "apply migrations") {
		t.Fatalf("first migrate() failed at an unexpected stage: %v", err)
	}
	if !tableExists(t, db1, "ok_table") {
		t.Fatal("the migration before the broken one was not applied — goose should commit it independently")
	}
	if tableExists(t, db1, "broken_table") {
		t.Fatal("the broken migration's table exists — its failing statement should have rolled it back")
	}

	db2 := openDedicated(t, dsn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := migrate(ctx, db2, fsys)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("second migrate() against the same still-broken fixture returned nil error")
	}
	if !strings.Contains(err.Error(), "apply migrations") {
		t.Fatalf("second migrate() failed at %q, want it to fail applying migrations — "+
			"a failure acquiring the lock means the first call never released it: %v", err, err)
	}
	if elapsed >= 5*time.Second {
		t.Fatalf("second migrate() took %s, at or past its own timeout — looks like it blocked "+
			"acquiring the lock rather than failing on the broken migration", elapsed)
	}
}
