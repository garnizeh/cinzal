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
# That congruence has a precondition this header did not use to state: it holds
# only if every exported function reachable through an ALLOWED import itself
# returns nothing from a FORBIDDEN package. An import check alone does not give
# you that for free — internal/match is an allowed import for web, so a
# rules.MatchState-returning export placed directly in internal/match would let
# a handler write `state, _, _ := match.Fold(...)` and hold a full match state
# without ever naming internal/rules. internal/match/fold exists, and is on the
# FORBIDDEN list below, precisely to keep this precondition true (D49).
#
# WHAT "TEACH THIS SCRIPT TO PARSE THEM" TURNED OUT TO MEAN (D54).
#
# D46 puts //go:build integration on every Integration- and Concurrency-layer
# test file, and states plainly that the tagged set grows to include
# internal/web once M5 builds the HTTP layer — squarely inside GUARDED. Under
# the default build configuration those files are exactly the IgnoredGoFiles
# this script already refuses to be blind about, and D46 is right that the
# tests belong there.
#
# The fix is not a bespoke import parser: go list itself can enumerate a
# package's imports under a build configuration that includes the tag, so the
# mechanism this whole script already trusts is asked twice instead of once —
# under the default configuration, and under every other build tag set this
# repository actually uses — rather than reimplementing what go list already
# does. BUILD_TAG_SETS below is that explicit allow-list; a file gated by a
# tag not named there is never compiled under ANY entry, so it still fails
# IgnoredGoFiles exactly as before. That is deliberate — this is a list of
# known configurations to inspect, not a blanket "-tags anything" escape
# hatch that would defeat the refusal above.
#
# A file being ignored under ONE tag set is not itself a failure — that is
# exactly what a //go:build integration file is supposed to report under the
# default configuration, since a later entry in BUILD_TAG_SETS is what
# compiles it instead. The failure condition is the INTERSECTION: a file
# still ignored under every configured tag set has genuinely never been
# compiled by anything this script runs, and only that is unchecked.
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
#
# internal/match/fold does not exist yet (it lands with #319) but is listed
# here in advance: internal/match itself is NOT forbidden — web must import
# it, per D01 — so any rules.MatchState-returning export placed directly in
# internal/match would be reachable from web with no forbidden edge to
# catch it. internal/match/fold is where Fold/FoldMeasured live instead
# (D49), specifically so this array is what stops web from reaching them,
# rather than a doc comment in internal/match/doc.go.
#
# internal/store/orderlog (issue #317) is the same shape of fix, one layer
# down: internal/store itself is NOT forbidden — RFC-001 §11.2 already
# commits internal/web to importing internal/store directly for
# []BoardNote (D18), and that edge is legitimate and must stay open — but
# orderlog.Load returns a rules.OrderLog, at least as fog-sensitive as
# MatchState (internal/rules/order_log.go's own doc comment: "it names
# every seat's full route and action history, not one seat's"). Putting
# Load directly on *store.Store would make it reachable from web through
# the exact same allowed edge BoardNotes needs, with no forbidden import to
# catch it — so, mirroring D49, it lives in its own sub-package instead,
# and only that sub-package's import path is forbidden here. See
# internal/store/orderlog's own package doc comment for the full reasoning.
FORBIDDEN=("$MODULE/internal/rules" "$MODULE/internal/telemetry" "$MODULE/internal/match/fold" "$MODULE/internal/store/orderlog")
GUARDED="./internal/render/... ./internal/web/..."

# Every non-default build configuration this repository actually uses (D54).
# "" is the default configuration; each other entry is a comma-separated tag
# set passed to `go list -tags`. A file gated by a tag not listed here still
# hard-fails via IgnoredGoFiles below, in whichever configuration is active
# when it does — extending this list is the fix, never a silent skip.
BUILD_TAG_SETS=("" "integration")

fail() { echo "check-fog-boundary: FAIL: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || fail "the go toolchain is not on PATH"

violations=""
inspected=0

# shellcheck disable=SC2086
pkgs="$(cd "$ROOT" && go list $GUARDED)" \
    || fail "go list $GUARDED did not succeed"
[ -n "$pkgs" ] || fail "go list $GUARDED reported no packages — nothing was inspected"

while IFS= read -r pkg; do
    [ -n "$pkg" ] || continue

    # go list reports only the files selected by the ACTIVE build
    # configuration, so a forbidden import inside a constrained file would
    # sail through unseen under any configuration this loop does not also
    # run. Rather than parse constrained files by hand, ask go list again
    # under every configuration BUILD_TAG_SETS names, and only fail on a file
    # that is ignored under ALL of them — the intersection, not any single
    # pass's result, since a //go:build integration file is SUPPOSED to be
    # ignored under the default configuration and inspected under a later one.
    common_ignored=""
    first_tags=1
    for tags in "${BUILD_TAG_SETS[@]}"; do
        ignored="$(cd "$ROOT" && go list -tags "$tags" -f '{{join .IgnoredGoFiles "\n"}}' "$pkg" | sort -u)" \
            || fail "could not list the build-constrained files of $pkg under tags '$tags'"
        if [ "$first_tags" -eq 1 ]; then
            common_ignored="$ignored"
            first_tags=0
        elif [ -n "$common_ignored" ] && [ -n "$ignored" ]; then
            common_ignored="$(comm -12 <(printf '%s\n' "$common_ignored") <(printf '%s\n' "$ignored"))"
        else
            common_ignored=""
        fi
    done
    if [ -n "$common_ignored" ]; then
        fail "$pkg has build-constrained files this gate cannot inspect under any configured tag set: $common_ignored
                    Their imports are invisible to go list under every entry in
                    BUILD_TAG_SETS. Add the tag that selects them to
                    BUILD_TAG_SETS above, or remove the constraint - do not
                    leave them unchecked."
    fi

    for tags in "${BUILD_TAG_SETS[@]}"; do
        inspected=$((inspected + 1))

        for field in Imports TestImports XTestImports; do
            imports="$(cd "$ROOT" && go list -tags "$tags" -f "{{join .$field \"\\n\"}}" "$pkg")" \
                || fail "could not list the $field of $pkg under tags '$tags'"

            while IFS= read -r imp; do
                [ -n "$imp" ] || continue
                for forbidden in "${FORBIDDEN[@]}"; do
                    case "$imp" in
                        "$forbidden"|"$forbidden"/*)
                            violations="$violations  $pkg [tags=$tags] ($field) -> $imp"$'\n' ;;
                    esac
                done
            done <<< "$imports"
        done
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

echo "check-fog-boundary: OK — $inspected package/build-tag pairs across ${#BUILD_TAG_SETS[@]} configuration(s), no path to internal/rules"
