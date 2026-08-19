#!/usr/bin/env bash
#
# The game's core security property, made mechanical.
#
# Fog is private (GDD §7.1): the client must never hold the full match state.
# RFC-001 §3 turns that into a compile-time property — the rendering and web
# layers cannot NAME the match state, so "a template physically cannot leak what
# it cannot name" — and §5 requires it be enforced in CI "rather than trusting
# discipline, this is exactly the kind of rule that erodes at 2am".
#
# THE RULE (docs/decisions/D01-package-layout.md, extended by D34 for
# internal/telemetry):
#
#   No package under internal/render/... or internal/web/... may DIRECTLY import
#   internal/rules, internal/telemetry, or any package beneath either.
#
# WHY DIRECT IMPORTS, AND WHY THAT IS EXACT RATHER THAN A COMPROMISE.
#
# web depends on rules transitively through match, unavoidably. That is fine: in
# Go a transitive dependency puts NO names in scope. You cannot reference a type
# from a package you do not import directly, so a direct-import check is exactly
# congruent with the property §3 wants.
#
# WHY TEST IMPORTS ARE INCLUDED — the opposite choice from the purity gate.
#
# All three of .Imports, .TestImports and .XTestImports are checked. A render
# test that can name the match state can build a fixture that non-test code
# later reads, and that exemption would be the first place the boundary leaks.
# The purity gate excludes test imports because `testing` itself pulls in os and
# time and that says nothing about the shipped package; no equivalent applies
# here.
#
# .XTestImports is the one that is easy to miss: `go list` reports the imports
# of an EXTERNAL test package - a file declaring `package render_test` rather
# than `package render` - in its own field. A check reading only the first two
# leaves that door open, and it is exactly where a convenient fixture is written.
#
# THIS CHECK FAILS CLOSED. Every path that cannot complete the inspection exits
# non-zero rather than falling through to success.

set -euo pipefail

MODULE="github.com/garnizeh/cinzal"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# internal/telemetry is a sanctioned fourth reader of MatchState, alongside
# cmd/simulate and internal/debug (D34) — it imports internal/rules
# deliberately, for its final-state and order-log reads, and must be
# forbidden from render/web exactly like internal/rules itself, rather than
# leaving the guarantee to depend on what telemetry.Match's signature
# happens to require today (D01's own rejected "what does this expose"
# reasoning).
FORBIDDEN=("$MODULE/internal/rules" "$MODULE/internal/telemetry")
GUARDED="./internal/render/... ./internal/web/..."

fail() { echo "check-fog-boundary: FAIL: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || fail "the go toolchain is not on PATH"

# shellcheck disable=SC2086
pkgs="$(cd "$ROOT" && go list $GUARDED)" \
    || fail "go list $GUARDED did not succeed"
[ -n "$pkgs" ] || fail "go list $GUARDED reported no packages — nothing was inspected"

violations=""

while IFS= read -r pkg; do
    [ -n "$pkg" ] || continue

    # go list reports only the files selected by the ACTIVE build configuration,
    # so a forbidden import inside render_windows.go would sail through Linux CI
    # unseen. Rather than parse constrained files, refuse to be blind.
    ignored="$(cd "$ROOT" && go list -f '{{join .IgnoredGoFiles " "}}' "$pkg")" \
        || fail "could not list the build-constrained files of $pkg"
    if [ -n "$ignored" ]; then
        fail "$pkg has build-constrained files this gate cannot inspect: $ignored
                    Their imports are invisible to go list under the active build
                    configuration. Remove the constraint or teach this script to
                    parse them - do not leave them unchecked."
    fi

    for field in Imports TestImports XTestImports; do
        imports="$(cd "$ROOT" && go list -f "{{join .$field \"\\n\"}}" "$pkg")" \
            || fail "could not list the $field of $pkg"

        while IFS= read -r imp; do
            [ -n "$imp" ] || continue
            for forbidden in "${FORBIDDEN[@]}"; do
                case "$imp" in
                    "$forbidden"|"$forbidden"/*)
                        violations="$violations  $pkg ($field) -> $imp"$'\n' ;;
                esac
            done
        done <<< "$imports"
    done
done <<< "$pkgs"

# The import rule proves render and web cannot NAME the match state. It does not
# stop a state VALUE travelling inside a type they can name, so D01 also forbids
# dynamic containers in internal/game. That check needs to distinguish a type
# expression from an identically spelled word in a comment or a string literal -
# `any` is ordinary English - so it is a parser rather than a pattern. See
# scripts/check-game-types.go for why, and for the rule it enforces.
if ! (cd "$ROOT" && go run scripts/check-game-types.go); then
    fail "internal/game declares a dynamic container (see above)"
fi

if [ -n "$violations" ]; then
    echo "check-fog-boundary: the fog boundary was crossed." >&2
    echo "                    GDD §7.1, RFC-001 §3 and §5, D01. Offending edges:" >&2
    printf '%s' "$violations" >&2
    echo "" >&2
    echo "                    render and web reach the engine through internal/match, which" >&2
    echo "                    returns game types. If you need something from rules, the answer" >&2
    echo "                    is to expose it on match as a game type, never to widen this." >&2
    exit 1
fi

echo "check-fog-boundary: OK — $(printf '%s\n' "$pkgs" | wc -l | tr -d ' ') guarded packages, no path to internal/rules"
