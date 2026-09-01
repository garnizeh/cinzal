#!/usr/bin/env bash
#
# Asserts that the package graph is exactly what scripts/packages.txt declares,
# and that internal/game depends on nothing but the standard library.
#
# THIS CHECK FAILS CLOSED. Every path that cannot complete the inspection exits
# non-zero rather than falling through to success. That is the whole point: a
# `go list` over a package set that does not exist yet returns nothing, a
# comparison against nothing succeeds, and a gate built the obvious way reports
# green while having looked at exactly zero packages. It is the same failure as
# a review bot that reports success on a review it skipped.

set -euo pipefail

MODULE="github.com/garnizeh/cinzal"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXPECTED_FILE="$ROOT/scripts/packages.txt"

fail() { echo "check-packages: FAIL: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || fail "the go toolchain is not on PATH"
[ -r "$EXPECTED_FILE" ] || fail "cannot read $EXPECTED_FILE"

expected="$(grep -vE '^\s*(#|$)' "$EXPECTED_FILE" | sort)"
[ -n "$expected" ] || fail "$EXPECTED_FILE declares no packages"

# Two configurations, unioned, the same reasoning check-fog-boundary.sh's own
# BUILD_TAG_SETS already applies (D54): internal/store/storetest (#325) is
# every file //go:build integration, on purpose, so testcontainers-go never
# reaches the default `go build ./...`/`make check`. A plain `go list ./...`
# never sees a directory whose files are entirely tagged out — not even as
# ignored — so checking only the default configuration would make this gate
# blind to that package's existence or disappearance either way. Each
# configuration still fails closed independently: neither may be silently
# empty.
actual_default="$(cd "$ROOT" && go list ./...)" || fail "go list ./... did not succeed"
[ -n "$actual_default" ] || fail "go list ./... reported no packages"

actual_integration="$(cd "$ROOT" && go list -tags integration ./...)" \
    || fail "go list -tags integration ./... did not succeed"
[ -n "$actual_integration" ] || fail "go list -tags integration ./... reported no packages"

actual="$(printf '%s\n%s\n' "$actual_default" "$actual_integration" | sort -u)"

if ! diff_out="$(diff <(printf '%s\n' "$expected") <(printf '%s\n' "$actual"))"; then
    echo "check-packages: the package graph does not match $EXPECTED_FILE" >&2
    echo "  '<' declared but missing   '>' present but undeclared" >&2
    printf '%s\n' "$diff_out" >&2
    exit 1
fi

# internal/game is the leaf that crosses the fog boundary. If it grows a
# dependency, render and web inherit whatever that dependency can name.
# Standard-library import paths have no dot in their first path segment.
game_deps="$(cd "$ROOT" && go list -deps ./internal/game/...)" \
    || fail "go list -deps ./internal/game/... did not succeed"
[ -n "$game_deps" ] || fail "go list -deps ./internal/game/... reported nothing"

foreign="$(printf '%s\n' "$game_deps" \
    | grep -v "^$MODULE/internal/game\$" \
    | awk -F/ '$1 ~ /\./' || true)"

if [ -n "$foreign" ]; then
    echo "check-packages: internal/game must import only the standard library" >&2
    printf '  %s\n' $foreign >&2
    exit 1
fi

echo "check-packages: OK — $(printf '%s\n' "$actual" | wc -l | tr -d ' ') packages, internal/game is stdlib-only"
