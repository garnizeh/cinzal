#!/usr/bin/env bash
#
# Asserts that internal/rules, internal/telemetry and internal/bots perform no
# I/O, read no clock, and draw no ambient randomness.
#
# Each tree has its own spec anchor, but the same property:
#
#   internal/rules      RFC-001 §6.1 — Resolve/Project must be deterministic,
#                        "enforced by a CI check, not by convention".
#   internal/telemetry  D34 fixes Match(s, log, events, cfg) as a pure
#                        function; RFC-001 §7.3 makes match_summary a
#                        rebuildable cache, and M3's own exit criterion
#                        ("cmd/replay --rebuild regenerates match_summary to
#                        byte-identical content") depends on that staying true.
#   internal/bots       D32's whole argument for {seed, order log} reproducing
#                        a bot-populated match forever is that Decide is
#                        deterministic in (view, cfg, BotRNG). The #195
#                        isolation gate is type-level — it stops a bot NAMING
#                        MatchState — and says nothing about a clock read or
#                        ambient randomness, which breaks the same guarantee a
#                        different way.
#
# WHY THIS CHECKS DIRECT IMPORTS AND NOT TRANSITIVE ONES.
#
# The property is "this code cannot call time.Now() or open a file", and in Go
# you cannot reference a name from a package you do not import directly. A
# direct-import check is therefore exactly congruent with the property, not a
# weaker proxy for it.
#
# A transitive check is also unworkable, which settles it independently: `fmt`
# depends transitively on both `os` and `time`, so forbidding those in the
# transitive set would reject the standard library's formatting package and
# every package that uses it.
#
# WHY EACH TREE HAS ITS OWN ALLOWED-IMPORTS LIST, NOT ONE SHARED CONSTANT.
#
# internal/rules may only reach internal/game. internal/telemetry and
# internal/bots each also import internal/rules legitimately — telemetry for
# its final MatchState/OrderLog read (D34), bots for BotRNG (D32) — but that
# import runs one direction only. A single shared allow-list wide enough to
# cover all three trees would, as a side effect, also let internal/rules
# import internal/telemetry or internal/bots, which is backwards and would go
# undetected here. Keeping the lists per-tree keeps the check exactly as
# narrow as each tree's real dependency shape.
#
# WHY TEST IMPORTS ARE EXCLUDED.
#
# `testing` itself imports `os`, `time` and `flag`. A test using it does not
# make the package impure — the property is about the shipped package. This is
# the opposite choice from the fog gate, which DOES cover test imports, because
# there the risk is a fixture built in a test and read elsewhere.
#
# WHAT THIS GATE DOES NOT COVER.
#
# RFC-001 §6.3 names four determinism hazards. This gate closes two of them
# outright — ambient time and ambient randomness. The other two, map iteration
# order and floating point, are not expressible as import rules and are covered
# by tests and review. `math` is therefore allowed, and "no float64 enters
# rules" is not enforced here.
#
# `fmt` is importable because Errorf and Sprintf are pure and all three trees
# need them, but fmt.Print* and Fprint*/Fscan* are rejected at the call site:
# an import rule cannot separate formatting from writing, and only the
# second is I/O. That call-site check is an AST walk, scripts/check-fmt-
# purity.go, run once per tree below — see that file's own header for why
# (issue #297: the textual predecessor of this check matched a selector only
# when directly followed by "(", so `p := fmt.Println; p("x")` sailed
# through unseen, and could not resolve an aliased fmt import at all, so it
# rejected aliasing outright rather than risk being unsound).
#
# `io` is in FORBIDDEN_EXACT below for the same issue's other gap. Importing
# `io` alone does nothing — io.ReadAll/io.Copy are generic over whatever
# io.Reader/io.Writer they are handed, and constructing a concrete one still
# needs os/net/etc., already forbidden. But a future Resolve/Match/Decide-
# adjacent function that accepted an io.Reader/io.Writer PARAMETER would let
# a caller (which may legitimately import os) hand in a real file or socket,
# and the callee would perform I/O without importing anything else this gate
# inspects. Forbidding the import closes that path before any code exploits
# it, even though nothing in these trees does today.
#
# THIS CHECK FAILS CLOSED. Every path that cannot complete the inspection exits
# non-zero rather than falling through to success — per tree, so a typo in one
# path cannot make the gate silently inspect nothing for that tree while still
# reporting overall success.

set -euo pipefail

MODULE="github.com/garnizeh/cinzal"

# REPO_ROOT locates this script's own siblings (scripts/check-fmt-purity.go)
# and never changes. ROOT is where go list is asked to look for the trees
# below and does change, but only for scripts/check-rules-purity_test.sh:
# PURITY_TEST_ROOT/_DIR/_SELF/_ALLOWED point a single synthetic tree at a
# fixture module instead of the real internal/rules et al, so that self-test
# can assert both this gate's failure modes without ever touching real code.
# Unset (the default for `make purity`), the three real trees run exactly as
# declared below.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT="${PURITY_TEST_ROOT:-$REPO_ROOT}"

# The trees this gate protects. Parallel arrays rather than an associative one,
# for portability across bash versions.
TREE_DIRS=(internal/rules internal/telemetry internal/bots)

# Each tree's own package (its subtree, for internal cross-imports) plus
# whatever non-stdlib imports are legitimate for it specifically.
TREE_SELF=(
    "$MODULE/internal/rules"
    "$MODULE/internal/telemetry"
    "$MODULE/internal/bots"
)
TREE_ALLOWED=(
    "$MODULE/internal/game"
    "$MODULE/internal/game $MODULE/internal/rules"
    "$MODULE/internal/game $MODULE/internal/rules"
)

if [ -n "${PURITY_TEST_ROOT:-}" ]; then
    TREE_DIRS=("${PURITY_TEST_DIR:-.}")
    TREE_SELF=("${PURITY_TEST_SELF:-fixture}")
    TREE_ALLOWED=("${PURITY_TEST_ALLOWED:-}")
fi

# Exact stdlib packages that break purity if imported directly.
FORBIDDEN_EXACT="
io
time
hash/maphash
math/rand
math/rand/v2
crypto/rand
os
syscall
context
io/ioutil
database/sql
plugin
runtime/debug
"

# Whole trees, matched by prefix.
FORBIDDEN_PREFIX="
os/
net
net/
log
log/
database/
"

fail() { echo "check-rules-purity: FAIL: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || fail "the go toolchain is not on PATH"

violations=""
total_pkgs=0
summary=""

for i in "${!TREE_DIRS[@]}"; do
    dir="${TREE_DIRS[$i]}"
    self="${TREE_SELF[$i]}"
    allowed="${TREE_ALLOWED[$i]}"
    pattern="./$dir/..."

    pkgs="$(cd "$ROOT" && go list "$pattern")" \
        || fail "go list $pattern did not succeed"
    [ -n "$pkgs" ] || fail "go list $pattern reported no packages — nothing was inspected"

    tree_count=0

    while IFS= read -r pkg; do
        [ -n "$pkg" ] || continue
        tree_count=$((tree_count + 1))

        # `go list` reports only the files selected by the ACTIVE build
        # configuration, so a forbidden import inside rules_windows.go would sail
        # through Linux CI unseen. Rather than parse constrained files, refuse to be
        # blind: there is no reason for a build-constrained file in a pure package,
        # and if one appears this gate stops instead of silently skipping it.
        ignored="$(cd "$ROOT" && go list -f '{{join .IgnoredGoFiles " "}}' "$pkg")" \
            || fail "could not list the build-constrained files of $pkg"
        if [ -n "$ignored" ]; then
            fail "$pkg has build-constrained files this gate cannot inspect: $ignored
                        go list only reports the active build configuration, so their
                        imports are invisible here. Either remove the constraint or
                        teach this script to parse them - do not leave them unchecked."
        fi

        imports="$(cd "$ROOT" && go list -f '{{join .Imports "\n"}}' "$pkg")" \
            || fail "could not list the imports of $pkg"

        while IFS= read -r imp; do
            [ -n "$imp" ] || continue

            case "$imp" in
                "$self"|"$self"/*) continue ;;
            esac

            allowed_hit=""
            for a in $allowed; do
                [ "$imp" = "$a" ] && allowed_hit=1 && break
            done
            [ -n "$allowed_hit" ] && continue

            # Ask the toolchain whether this is standard library rather than
            # guessing from the path. A dotless module path resolved through a local
            # `replace` looks stdlib to a heuristic and is not.
            std="$(cd "$ROOT" && go list -f '{{.Standard}}' "$imp" 2>/dev/null)" \
                || fail "could not classify the import $imp (from $pkg)"
            if [ "$std" != "true" ]; then
                violations="$violations  $pkg -> $imp  (not standard library)"$'\n'
                continue
            fi

            for f in $FORBIDDEN_EXACT; do
                [ "$imp" = "$f" ] && violations="$violations  $pkg -> $imp"$'\n'
            done
            for f in $FORBIDDEN_PREFIX; do
                case "$imp" in "$f"*) violations="$violations  $pkg -> $imp"$'\n' ;; esac
            done
        done <<< "$imports"
    done <<< "$pkgs"

    total_pkgs=$((total_pkgs + tree_count))
    summary="$summary $dir=$tree_count"

    # fmt stays importable - Errorf and Sprintf are pure and these trees need
    # them - but fmt.Print*, Fprint* and Fscan* are I/O in a package that must
    # perform none. An import rule cannot express that distinction, so it is
    # checked at the call site by scripts/check-fmt-purity.go (an AST walk -
    # see that file's header for why, and for its contract with this script:
    # a clean tree prints nothing and exits 0, findings print one line each
    # and exit 1, and an infra failure is distinguished by the literal
    # "check-fmt-purity: FAIL:" prefix rather than by exit code).
    fmtcheck="$(cd "$REPO_ROOT" && go run scripts/check-fmt-purity.go "$ROOT/$dir" 2>&1)" || true
    if printf '%s' "$fmtcheck" | grep -q '^check-fmt-purity: FAIL:'; then
        fail "the fmt call-site check on $dir did not complete:
$fmtcheck"
    fi
    if [ -n "$fmtcheck" ]; then
        violations="$violations$(printf '%s\n' "$fmtcheck" | sed 's/^/  /')"$'\n'
    fi
done

if [ -n "$violations" ]; then
    echo "check-rules-purity: internal/rules, internal/telemetry and internal/bots must stay" >&2
    echo "                    pure — no I/O, no clock, no ambient randomness." >&2
    echo "                    RFC-001 §6.1, D34, D32. The offending direct imports:" >&2
    printf '%s' "$violations" >&2
    echo "" >&2
    echo "                    If you need one of these, the answer is almost certainly to move" >&2
    echo "                    the work to internal/match or a cmd/ harness, not to weaken this gate." >&2
    exit 1
fi

echo "check-rules-purity: OK — $total_pkgs packages inspected ($(printf '%s' "$summary" | sed 's/^ //')), no forbidden direct imports"
