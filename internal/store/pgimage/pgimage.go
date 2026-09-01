// Package pgimage holds the single pinned Postgres 18.6 manifest-list digest
// D46 chose for the whole persistence layer's test suite — the one place the
// value is written down, per D46's own reasoning ("there is only one place
// the image reference is written down"). internal/store/storetest and
// internal/store's own white-box integration tests both import Ref instead
// of each hardcoding the literal, closing the duplication that D46's
// store -> storetest -> store cycle otherwise forced (see
// internal/store/migrate_integration_test.go's history).
//
// compose.yaml cannot import Go source, so it keeps its own copy of the
// literal; scripts/check-postgres-digest.sh asserts that copy still matches
// Ref, so a drift between local dev tooling and the test harness fails a
// gate instead of surfacing as a mismatched clock in a Concurrency test.
package pgimage

// Ref is the exact Postgres 18.6 manifest-list digest, verified directly
// against the registry (docker buildx imagetools inspect postgres:18.6) —
// the official multi-architecture manifest-list digest, never a
// platform-specific child digest, and never a floating tag (D46).
//
// This value must not change without updating docs/decisions/D46 and
// compose.yaml together — scripts/check-postgres-digest.sh fails the build
// the moment the two disagree.
const Ref = "postgres@sha256:4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280"
