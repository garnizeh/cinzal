#!/usr/bin/env bash
#
# The fail-closed guard on D46's own guard (issue #325): asserts the
# Postgres-backed Integration/Concurrency test suite has not silently
# shrunk to fewer tests, or to zero. CLAUDE.md/CONTRIBUTING.md's own rule —
# "a gate that passes when it can't run is worse than no gate" — applied to
# a suite instead of a tool: a //go:build integration file deleted, or a
# test quietly renamed off the TestIntegrationXxx/TestConcurrencyXxx
# pattern D46 fixed for exactly this reason, must fail a build, not print
# "ok" having compiled nothing worth counting.
#
# `go test -tags integration -list PATTERN PKGS` compiles every tagged file
# without executing a single test body (no TestMain in this design,
# storetest's own doc comment explains why), so this needs no Docker and no
# database — it is deliberately as cheap as check-packages.sh, checking a
# count instead of a package graph.
#
# THIS CHECK FAILS CLOSED, same discipline as every other gate here. Three
# independent guards, each fatal on its own:
#   - go missing from PATH, or `go test -list` failing outright.
#   - the count is exactly zero — D46/#325's own explicit acceptance
#     criterion ("a zero-test run fails... asserted by a fixture, not by
#     inspection"), checked before the floor comparison and independent of
#     it, so a floor of 0 (never true here, but never trusted blindly
#     either) could not turn this into a silent pass.
#   - the count is below FLOOR — bumped upward only, by hand, in a reviewed
#     PR, the same discipline check-bench-regression.sh's threshold and the
#     bots-isolation allow-list already hold contributors to.
#
# Scoped to exactly the packages D46 names as allowed to hold
# //go:build integration files (internal/store, internal/match, cmd/replay
# — storetest itself carries no test functions of its own, per its own doc
# comment) — never a bare -list '.*' ./..., which would dilute a lost test
# inside a total dominated by every ordinary test in the repository.
#
# ENVIRONMENT OVERRIDES, for scripts/check-integration-coverage_test.sh
# only. INTEGRATION_COVERAGE_TEST_ROOT points this at a synthetic fixture
# module instead of this repo (go test needs a real, buildable go.mod at
# the root it runs from). INTEGRATION_COVERAGE_TEST_PKGS overrides the
# package pattern. INTEGRATION_COVERAGE_TEST_FLOOR overrides the floor —
# the real repo's floor (below) would never let a small fixture module
# pass. Unset (the default for `make integration-list` and CI), all three
# resolve to the real repo and its real floor.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT="${INTEGRATION_COVERAGE_TEST_ROOT:-$REPO_ROOT}"
PKGS="${INTEGRATION_COVERAGE_TEST_PKGS:-./internal/store/... ./internal/match/... ./cmd/replay/...}"

# Bumped upward only, by hand, as the suite grows — never automatically, and
# never lowered to make a shrinking suite pass. 66 is the exact count this
# repository's Integration/Concurrency suite held the day #325 landed the
# naming convention across every file that predates it; 68 is the count as
# of #328, which added TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture
# and TestIntegrationFoldDivergesWhenOrderCorrupted to cmd/replay; 70 is the
# count as of #329, which added
# TestIntegrationMigrateBootRaceAppliesEachMigrationExactlyOnce and
# TestIntegrationMigrateBootRaceSecondProcessRecoversAfterFirstIsKilled to
# internal/store.
DEFAULT_FLOOR=70
FLOOR="${INTEGRATION_COVERAGE_TEST_FLOOR:-$DEFAULT_FLOOR}"

fail() { echo "check-integration-coverage: FAIL: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || fail "the go toolchain is not on PATH"

# shellcheck disable=SC2086 # PKGS is a deliberately unquoted, space-separated package-pattern list, the same shape check-replay-deps.sh's PKG takes.
out="$(cd "$ROOT" && go test -tags integration -list '^Test(Integration|Concurrency)' $PKGS 2>&1)" \
    || fail "go test -tags integration -list did not succeed: $out"

names="$(printf '%s\n' "$out" | grep -E '^Test(Integration|Concurrency)' || true)"
count="$(printf '%s\n' "$names" | grep -c . || true)"

if [ "$count" -eq 0 ]; then
    fail "0 Integration/Concurrency tests found across $PKGS — a zero-test run is a failure, not a vacuous pass (D46, #325)"
fi

if [ "$count" -lt "$FLOOR" ]; then
    fail "$count Integration/Concurrency tests found, want at least $FLOOR — the suite shrank. FLOOR is bumped upward only (see this script's own header): a deliberately removed test gets replaced by an equivalent one so the count is preserved, not compensated for by lowering FLOOR. If this drop is genuinely intended, that is itself a decision for a reviewed PR to make explicitly — not a case this script accommodates on its own."
fi

echo "check-integration-coverage: OK - $count Integration/Concurrency test(s) found across $PKGS (floor $FLOOR)"
