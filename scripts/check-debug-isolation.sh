#!/usr/bin/env bash
#
# Asserts that debug tooling cannot reach a production binary.
#
# RFC-001 §15.1 makes this a build tag rather than a runtime flag, and says why
# in one sentence: "a runtime flag is one environment-variable mistake away from
# serving a god view of live matches, and that mistake is unrecoverable — you
# cannot un-leak a map." §15.3 lists what the panel exposes: all positions, all
# cargo, all contracts, all hands. Every hidden fact in the game, for every
# player, at once.
#
# WHAT IS ASSERTED TODAY, AND WHAT IS NOT.
#
# Two checks are live now and one is not, and the difference is announced rather
# than hidden - a gate that reports success while inspecting nothing is the
# failure this milestone exists to prevent.
#
#   LIVE  Only doc.go compiles into internal/debug under the production build
#         configuration. This catches the mistake that will actually happen:
#         someone adds godview.go and forgets the //go:build debug line. The
#         toolchain answers it exactly - an untagged file shows up in GoFiles.
#
#   LIVE  doc.go declares nothing. It is exempt from the build tag only because
#         it holds a package clause and comments; the moment it holds code, the
#         exemption becomes the hole.
#
#   VACUOUS FOR NOW  internal/debug must not appear in the production dependency
#         graph of ./cmd/... Nothing imports it in either configuration yet, so
#         the assertion is true without being informative. It is reported as
#         vacuous until the debug build actually reaches it.
#
#   DEFERRED  RFC-001 §15.1 specifies "building both and diffing the route
#         table". There are no routes yet. When internal/web gains a router the
#         mechanism is to have each binary print its route table and compare -
#         that needs a decision about how routes are enumerated, and it belongs
#         with the web layer rather than being guessed here.
#
# THIS CHECK FAILS CLOSED. Every path that cannot complete the inspection exits
# non-zero rather than falling through to success.

set -euo pipefail

MODULE="github.com/garnizeh/cinzal"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

DEBUG_PKG="$MODULE/internal/debug"
EXEMPT_FILE="doc.go"

fail() { echo "check-debug-isolation: FAIL: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || fail "the go toolchain is not on PATH"

# ---------------------------------------------------------------------------
# 1. Only doc.go compiles under the production configuration.
# ---------------------------------------------------------------------------

# The whole tree, not just the top package: a subpackage such as
# internal/debug/godview is a separate package that ./internal/debug does not
# match, and an untagged file inside it would never be looked at.
#
# A fully tagged package does not appear at all under the production
# configuration - go list simply omits it. So the rule is: internal/debug may
# appear, carrying exactly doc.go, and nothing else may appear at all.
prod_pkgs="$(cd "$ROOT" && go list -f '{{.ImportPath}}|{{join .GoFiles " "}}' ./internal/debug/...)" \
    || fail "could not list internal/debug/... in the production configuration"
[ -n "$prod_pkgs" ] || fail "go list ./internal/debug/... reported nothing — nothing was inspected"

untagged=""
saw_root=0
while IFS= read -r line; do
    [ -n "$line" ] || continue
    pkg="${line%%|*}"
    files="${line#*|}"
    if [ "$pkg" = "$DEBUG_PKG" ]; then
        saw_root=1
        [ "$files" = "$EXEMPT_FILE" ] || untagged="$untagged  $pkg: $files"$'\n'
    else
        untagged="$untagged  $pkg: $files"$'\n'
    fi
done <<< "$prod_pkgs"

[ "$saw_root" = "1" ] || fail "$DEBUG_PKG did not appear in the production configuration — nothing was inspected"

if [ -n "$untagged" ]; then
    echo "check-debug-isolation: files without //go:build debug compile into production." >&2
    printf '%s' "$untagged" >&2
    echo "                       Only $DEBUG_PKG may appear, carrying exactly $EXEMPT_FILE." >&2
    echo "" >&2
    echo "                       RFC-001 §15.1: debug code is not compiled into the" >&2
    echo "                       production binary. Add the build constraint - the panel" >&2
    echo "                       exposes every hidden fact in the game at once, and you" >&2
    echo "                       cannot un-leak a map." >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# 2. doc.go declares nothing, so its exemption is not a hole.
# ---------------------------------------------------------------------------

doc_path="$ROOT/internal/debug/$EXEMPT_FILE"
[ -r "$doc_path" ] || fail "cannot read internal/debug/$EXEMPT_FILE"

# Strip comments and blank lines; only the package clause may remain.
doc_code="$(grep -vE '^[[:space:]]*(//|$)' "$doc_path" | grep -vE '^package[[:space:]]+debug[[:space:]]*$' || true)"
if [ -n "$doc_code" ]; then
    echo "check-debug-isolation: internal/debug/$EXEMPT_FILE must declare nothing." >&2
    echo "                       It is exempt from the build constraint only because it" >&2
    echo "                       holds a package clause and comments. Code here ships to" >&2
    echo "                       production. Offending lines:" >&2
    printf '%s\n' "$doc_code" | sed 's/^/  /' >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# 3. internal/debug is absent from the production dependency graph.
# ---------------------------------------------------------------------------

prod_deps="$(cd "$ROOT" && go list -deps ./cmd/...)" \
    || fail "could not list the production dependencies of ./cmd/..."
[ -n "$prod_deps" ] || fail "go list -deps ./cmd/... reported nothing — nothing was inspected"

debug_deps="$(cd "$ROOT" && go list -deps -tags debug ./cmd/...)" \
    || fail "could not list the debug dependencies of ./cmd/..."
[ -n "$debug_deps" ] || fail "go list -deps -tags debug ./cmd/... reported nothing — nothing was inspected"

# Matched with `case` rather than piped into grep, for two reasons that both
# bite silently. `grep -q` exits at the first match, so printf takes SIGPIPE and
# exits 141; under `pipefail` the pipeline is then non-zero, the `if` body is
# skipped, and a DETECTED leak reports OK - fail-open, and inverted. And the
# import path is full of dots, which a basic regex treats as "any character".
# Prefix matching also covers subpackages, which an exact match would miss.
reaches() { # $1 = newline-separated package list
    local dep
    while IFS= read -r dep; do
        case "$dep" in
            "$DEBUG_PKG"|"$DEBUG_PKG"/*) return 0 ;;
        esac
    done <<< "$1"
    return 1
}

if reaches "$prod_deps"; then
    echo "check-debug-isolation: internal/debug is reachable from a production binary." >&2
    echo "                       Something in ./cmd/... imports it without a build" >&2
    echo "                       constraint. RFC-001 §15.1 - this is the unrecoverable one." >&2
    exit 1
fi

if reaches "$debug_deps"; then
    echo "check-debug-isolation: OK — internal/debug is reachable under -tags debug and absent from production"
else
    echo "check-debug-isolation: OK — no untagged files in internal/debug"
    echo "                       NOTE: the dependency-graph assertion is VACUOUS, not passing."
    echo "                       Nothing imports internal/debug in either configuration yet, so"
    echo "                       'absent from production' is true without being informative."
    echo "                       It becomes live when the debug panel is wired up."
fi
