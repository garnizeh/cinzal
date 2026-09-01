//go:build integration

package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
)

// randomDatabaseName returns prefix followed by 16 hex characters of
// crypto/rand — collision-proof enough for one test binary's lifetime, and
// this is a database name for test isolation, not a draw the RFC-001 §6.4
// consumption table or any replay determinism guarantee has any stake in.
func randomDatabaseName(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is a broken host, not a recoverable test
		// condition — every caller of this function is already inside a
		// t.Fatal-only setup path.
		panic(fmt.Sprintf("storetest: read random bytes: %v", err))
	}
	return prefix + hex.EncodeToString(b[:])
}

// FreshDatabase returns a DSN naming a brand-new database cloned from the
// already-migrated idle template — real, independent connections and real
// commits, for the one test that genuinely needs both: the SELECT ... FOR
// UPDATE contention test (D46 tier 2). Postgres's template-copy is a
// file-level clone, not a re-run of migrations, so this is cheap even paid
// once per test.
//
// The clone comes from idleDatabase, never workDatabase — idleDatabase is
// never connected to by anything but setup's own Migrate call, so cloning
// from it never races an open connection the way cloning from workDatabase
// (which ordinary Container-backed tests hold transactions against) would.
func FreshDatabase(t *testing.T) string {
	t.Helper()
	ensureSetup(t)

	name := randomDatabaseName("cinzal_test_")
	ctx := context.Background()
	if err := adminExec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", quoteIdent(name), quoteIdent(idleDatabase))); err != nil {
		t.Fatalf("storetest: clone fresh database: %v", err)
	}
	t.Cleanup(func() {
		// WITH (FORCE) (Postgres 13+) rather than tracking every connection
		// this test opened against name by hand: a test that fails partway
		// through and leaks a connection must not also fail its own
		// cleanup and obscure the real failure behind a DROP DATABASE
		// error. Run from a connection to adminDatabase, never to name
		// itself — Postgres refuses DROP DATABASE on the database a
		// session is connected to.
		if err := adminExec(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quoteIdent(name))); err != nil {
			t.Errorf("storetest: drop fresh database %s: %v", name, err)
		}
	})

	return dsnFor(name)
}

// FreshUnmigratedDatabase returns a DSN naming a brand-new database cloned
// from Postgres's own template0, deliberately not the idle template — for
// the migration-race exit demonstration (D46 tier 3, out of scope for this
// package) to race two real processes' Migrate() calls against a database
// that has genuinely never seen a migration. This function's job stops at
// handing back the connection string; orchestrating the two OS processes
// the exit demo needs is that demonstration's own harness to build.
func FreshUnmigratedDatabase(t *testing.T) string {
	t.Helper()
	ensureSetup(t)

	name := randomDatabaseName("cinzal_unmigrated_")
	ctx := context.Background()
	if err := adminExec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE template0", quoteIdent(name))); err != nil {
		t.Fatalf("storetest: clone fresh unmigrated database: %v", err)
	}
	t.Cleanup(func() {
		if err := adminExec(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quoteIdent(name))); err != nil {
			t.Errorf("storetest: drop fresh unmigrated database %s: %v", name, err)
		}
	})

	return dsnFor(name)
}
