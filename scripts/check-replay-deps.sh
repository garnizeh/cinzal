#!/usr/bin/env bash
#
# Asserts cmd/replay's dependency graph cannot reach an effect provider —
# RFC-001 §7.4: "the fold's own execution never dispatches an effect... a
# rebuild that re-sends every historical notification is a live bug." This is
# the compile-time half of that guarantee; the runtime half is M4's exit
# criterion (fold a finished match ten times, assert outbox gains zero rows).
#
# cmd/replay/doc.go already documents the intended mechanism: "consistency
# with [//go:build debug's] discipline is kept by the import-graph mechanism
# instead" — this script is that mechanism. D49
# (docs/decisions/D49-fold-package-boundary.md) names this file outright:
# "check-replay-deps.sh ... stays the one place §7.4 is [enforced]."
#
# SAME SHAPE AS scripts/check-simulate-deps.sh, one committed allow-list
# instead of an inline pattern. scripts/replay-deps-allowlist.txt names every
# internal/ package cmd/replay may depend on — an allow-list, not a
# deny-list, for the same reason scripts/bots-isolation-allowlist.txt is one:
# a deny-list naming internal/mail passes silently the day someone adds
# internal/notify.
#
# THIS CHECK FAILS CLOSED, same discipline as every other gate here. Three
# independent guards, each fatal on its own:
#   - go missing from PATH, or `go list -deps` failing outright.
#   - `go list -deps` succeeding but the internal/ subset being EMPTY. This
#     guard runs before the allow-list comparison and does not consult it, so
#     an allow-list that happens to match everything can never turn "nothing
#     was actually inspected" into a silent pass.
#   - the allow-list file itself missing or unreadable.
#
# ENVIRONMENT OVERRIDES, for scripts/check-replay-deps_test.sh only.
# REPLAY_DEPS_TEST_ROOT points `go list` at a synthetic fixture module
# instead of this repo (go list -deps needs a real, buildable go.mod at the
# root it runs from — a bare directory argument, the shape
# check-bots-isolation.go uses, doesn't help here). REPLAY_DEPS_TEST_PKG
# overrides the import pattern (default ./cmd/replay).
# REPLAY_DEPS_TEST_ALLOWLIST overrides the allow-list path. Unset (the
# default for `make replay-deps`), all three resolve to the real repo.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT="${REPLAY_DEPS_TEST_ROOT:-$REPO_ROOT}"
PKG="${REPLAY_DEPS_TEST_PKG:-./cmd/replay}"
ALLOWLIST="${REPLAY_DEPS_TEST_ALLOWLIST:-$REPO_ROOT/scripts/replay-deps-allowlist.txt}"

fail() { echo "check-replay-deps: FAIL: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || fail "the go toolchain is not on PATH"

# Resolved dynamically rather than hardcoded: a self-test fixture module
# (named e.g. "fixture") still gets an internal/-prefix match this way, with
# no separate override needed for the module path itself.
MODULE="$(cd "$ROOT" && go list -m 2>&1)" || fail "could not determine the module path at $ROOT: $MODULE"

deps="$(cd "$ROOT" && go list -deps "$PKG")" || fail "go list -deps $PKG did not succeed"
[ -n "$deps" ] || fail "go list -deps $PKG reported nothing"

internal_deps="$(printf '%s\n' "$deps" | grep -E "^$MODULE/internal/" || true)"
[ -n "$internal_deps" ] || fail "go list -deps $PKG named no internal/ package at all — nothing was inspected"

[ -r "$ALLOWLIST" ] || fail "could not read the allow-list $ALLOWLIST"

allowed=""
while IFS= read -r line; do
    line="$(printf '%s' "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
    [ -n "$line" ] || continue
    case "$line" in \#*) continue ;; esac
    allowed="$allowed $MODULE/internal/$line"
done <"$ALLOWLIST"

foreign=""
while IFS= read -r dep; do
    [ -n "$dep" ] || continue
    hit=""
    for a in $allowed; do
        [ "$dep" = "$a" ] && hit=1 && break
    done
    [ -n "$hit" ] || foreign="$foreign  $dep"$'\n'
done <<<"$internal_deps"

if [ -n "$foreign" ]; then
    echo "check-replay-deps: $PKG may not depend on a package outside $ALLOWLIST (RFC-001 §7.4, D49)." >&2
    echo "                   If this is a new legitimate dependency, widen the allow-list in a" >&2
    echo "                   reviewed PR that explains why it is still not an effect provider." >&2
    echo "                   If it is not, this is the bug the gate exists to catch:" >&2
    printf '%s' "$foreign" >&2
    exit 1
fi

echo "check-replay-deps: OK - $(printf '%s\n' "$internal_deps" | wc -l | tr -d ' ') internal package(s), all within $ALLOWLIST"
