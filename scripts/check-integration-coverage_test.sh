#!/usr/bin/env bash
#
# Fixture coverage for scripts/check-integration-coverage.sh (issue #325).
# Not wired into `make check`/`check-nosecrets` — this gate itself isn't
# either (D54: compiling the fixture's own testcontainers-style tagged file
# is still a `go test -list` compile, deliberately kept out of the default
# loop) — CI's own integration-list job runs `make integration-list` (this
# script), and a contributor runs this selftest by hand or trusts CI.
#
# `go test -tags integration -list` needs a real, buildable go.mod at the
# root it runs from, same reasoning check-replay-deps_test.sh's own header
# gives for REPLAY_DEPS_TEST_ROOT — so this points the gate at a synthetic
# fixture MODULE via INTEGRATION_COVERAGE_TEST_ROOT/_PKGS/_FLOOR rather than
# a fixture directory inside the real module.

set -euo pipefail

cd "$(dirname "$0")/.."

GATE="$(pwd)/scripts/check-integration-coverage.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
fail() {
	printf 'FAIL: %s\n' "$1" >&2
	failed=1
}

# run <root> [pkgs] [floor] [path] — invokes the gate against a fixture
# module, capturing output and exit code without letting `set -e` abort this
# script on the (expected) non-zero cases. `path` overrides $PATH (Case: go
# toolchain absent); left unset, the ambient PATH is used.
run() {
	local root="$1" pkgs="${2:-./...}" floor="${3:-1}" pathoverride="${4:-}"
	if [ -n "$pathoverride" ]; then
		out="$(PATH="$pathoverride" INTEGRATION_COVERAGE_TEST_ROOT="$root" INTEGRATION_COVERAGE_TEST_PKGS="$pkgs" INTEGRATION_COVERAGE_TEST_FLOOR="$floor" bash "$GATE" 2>&1)" && code=0 || code=$?
	else
		out="$(INTEGRATION_COVERAGE_TEST_ROOT="$root" INTEGRATION_COVERAGE_TEST_PKGS="$pkgs" INTEGRATION_COVERAGE_TEST_FLOOR="$floor" "$GATE" 2>&1)" && code=0 || code=$?
	fi
}

gomod() { printf 'module fixture\n\ngo 1.26\n' >"$1/go.mod"; }

# ---------------------------------------------------------------------------
# Case 1 (clean): two tagged test functions, both matching the pattern, one
# of each prefix — proves both TestIntegration and TestConcurrency count,
# and that FLOOR=2 passes at exactly the floor.
# ---------------------------------------------------------------------------
clean="$tmp/clean"
mkdir -p "$clean/internal/store"
gomod "$clean"
cat >"$clean/internal/store/store.go" <<'EOF'
package store
EOF
cat >"$clean/internal/store/store_integration_test.go" <<'EOF'
//go:build integration

package store_test

import "testing"

func TestIntegrationFoo(t *testing.T) {}
func TestConcurrencyBar(t *testing.T) {}

// Deliberately NOT matching the pattern — proves an ordinary test sharing
// the same tagged file is not counted, the same isolation the real
// TestSchemaXxx-style rename (#325) was needed for in the first place.
func TestOrdinaryUnrelated(t *testing.T) {}
EOF
run "$clean" "./internal/store/..." 2
[ "$code" -eq 0 ] || fail "clean fixture: expected exit 0, got $code: $out"
printf '%s\n' "$out" | grep -q '^check-integration-coverage: OK' || fail "clean fixture: expected an OK line, got: $out"
printf '%s\n' "$out" | grep -q '2 Integration/Concurrency test' || fail "clean fixture: expected exactly 2 counted, got: $out"

# ---------------------------------------------------------------------------
# Case 2 (shrunk below floor): same fixture, floor raised past the actual
# count — the regression this gate exists to catch.
# ---------------------------------------------------------------------------
run "$clean" "./internal/store/..." 3
[ "$code" -eq 1 ] || fail "below floor: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'want at least 3' || fail "below floor: expected the floor message, got: $out"

# ---------------------------------------------------------------------------
# Case 3 (zero tests, the acceptance criterion's own named case): a tagged
# file with no test matching the pattern at all — distinct from "below
# floor," asserted independently of FLOOR (set to 0, which must still fail).
# ---------------------------------------------------------------------------
zero="$tmp/zero"
mkdir -p "$zero/internal/store"
gomod "$zero"
cat >"$zero/internal/store/store.go" <<'EOF'
package store
EOF
cat >"$zero/internal/store/store_integration_test.go" <<'EOF'
//go:build integration

package store_test

import "testing"

func TestOrdinaryOnly(t *testing.T) {}
EOF
run "$zero" "./internal/store/..." 0
[ "$code" -eq 1 ] || fail "zero tests: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q '0 Integration/Concurrency tests found' || fail "zero tests: expected the zero-count message, got: $out"

# ---------------------------------------------------------------------------
# Case 4 (go test -list itself fails): a package pattern matching nothing.
# ---------------------------------------------------------------------------
run "$clean" "./internal/does-not-exist/..." 1
[ "$code" -eq 1 ] || fail "bad pattern: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'did not succeed' || fail "bad pattern: expected the go-test-failure line, got: $out"

# ---------------------------------------------------------------------------
# Case 5 (go toolchain absent): an isolated PATH holding nothing but symlinks
# to bash's own resolved absolute path, plus dirname, grep, sed, cat and
# mkdir — the scripts/check-generate_test.sh/check-replay-deps_test.sh
# idiom, so this does not depend on where a real `go` binary happens to
# live on the host running this test. grep is needed here (unlike
# check-replay-deps_test.sh's equivalent case) because this gate's own
# fail() line and pattern filtering call it before the `command -v go`
# check would otherwise be reached in a shell needing to resolve it.
# ---------------------------------------------------------------------------
isolated_path_dir="$tmp/isolated-path"
mkdir -p "$isolated_path_dir"
for tool in bash dirname grep; do
	tool_abs="$(command -v "$tool")"
	ln -s "$tool_abs" "$isolated_path_dir/$tool"
done
run "$clean" "./internal/store/..." 1 "$isolated_path_dir"
[ "$code" -eq 1 ] || fail "go absent: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'the go toolchain is not on PATH' ||
	fail "go absent: expected the missing-toolchain message, got: $out"

if [ "$failed" -eq 0 ]; then
	echo "check-integration-coverage_test: PASS"
fi
exit "$failed"
