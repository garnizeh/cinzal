//go:build integration

// Package storetest is the one documented entry point RFC-001 §16.1's
// Integration and Concurrency test layers use to get a real Postgres
// database, per D46 (docs/decisions/D46-postgres-backed-test-layer.md) and
// its correction D54 (docs/decisions/D54-integration-tag-fog-gate-and-check-scope.md).
//
// This is a normal Go package, not itself a _test.go file — it is compiled
// and imported like any other, but only ever from other packages' _test.go
// files, so it never reaches internal/store's own production build or
// cmd/server/cmd/replay. It imports internal/store for Migrate(), which
// means any file inside package store itself (not store_test) that imports
// storetest forms an import cycle Go's tooling rejects (store -> storetest
// -> store) — every internal/store test file that uses this package must
// therefore be package store_test, never package store. See
// internal/store/migrate_integration_test.go's own header for the one file
// that cannot follow this rule at all, and why.
//
// Three entry points, matching the three isolation tiers D46 decided:
//
//   - Container(t) — an ordinary Integration-layer test: a *store.Store
//     backed by a transaction opened against a shared, already-migrated
//     database and rolled back in t.Cleanup. Cheapest isolation, no commit
//     ever visible outside the test.
//   - FreshDatabase(t) — a test that genuinely needs two independent,
//     real connections and real commits (the SELECT ... FOR UPDATE
//     contention test). A freshly cloned, real database, dropped in
//     t.Cleanup.
//   - FreshUnmigratedDatabase(t) — a database that has never seen a
//     migration at all, for the migration-race exit demonstration to race
//     two real processes' Migrate() calls against.
//
// The underlying container is started lazily, the first time any of the
// three is called from within one test binary (= one package, since that is
// the unit go test ./... already runs as a separate process), and reused
// for every other call in that same binary — never inside TestMain, so that
// `go test -list` (which runs TestMain but no individual test body) never
// touches Docker. Every //go:build integration test function in this
// repository is named TestIntegrationXxx or TestConcurrencyXxx for exactly
// this reason: scripts/check-integration-coverage.sh counts names matching
// that pattern under go test -list to catch the suite silently shrinking,
// without needing Docker itself to check.
package storetest
