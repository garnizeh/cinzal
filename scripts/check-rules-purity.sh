#!/usr/bin/env bash
#
# Asserts that internal/rules performs no I/O, reads no clock, and draws no
# ambient randomness — RFC-001 §6.1, which requires this be "enforced by a CI
# check, not by convention".
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
# WHY TEST IMPORTS ARE EXCLUDED.
#
# `testing` itself imports `os`, `time` and `flag`. A test using it does not
# make Resolve impure — the property is about the shipped package. This is the
# opposite choice from the fog gate, which DOES cover test imports, because
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
# `fmt` is importable because Errorf and Sprintf are pure and the engine needs
# them, but fmt.Print* and Fprint* are rejected at the call site: an import rule
# cannot separate formatting from writing, and only the second is I/O.
#
# That call-site check is textual, with two consequences stated rather than
# discovered. Aliased and dot imports of fmt are rejected outright, because they
# would make the qualifier unpredictable and the grep unsound. And a mention of
# fmt.Println inside a comment or a string literal will fail the gate - a false
# positive, deliberately, because it costs a rename while a false negative costs
# the property. If this ever gets noisy, the answer is an AST-based check, not a
# looser pattern.
#
# THIS CHECK FAILS CLOSED. Every path that cannot complete the inspection exits
# non-zero rather than falling through to success.

set -euo pipefail

MODULE="github.com/garnizeh/cinzal"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# The only non-stdlib import internal/rules may have. Project returns a
# game.PlayerView and Resolve takes a game.Order, so this is unavoidable rather
# than a loosening — and it costs nothing, because internal/game imports nothing
# outside the standard library and check-packages.sh verifies that separately.
ALLOWED_MODULE_IMPORT="$MODULE/internal/game"

# Exact stdlib packages that break purity if imported directly.
FORBIDDEN_EXACT="
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

pkgs="$(cd "$ROOT" && go list ./internal/rules/...)" \
    || fail "go list ./internal/rules/... did not succeed"
[ -n "$pkgs" ] || fail "go list ./internal/rules/... reported no packages — nothing was inspected"

violations=""

while IFS= read -r pkg; do
    [ -n "$pkg" ] || continue

    # `go list` reports only the files selected by the ACTIVE build
    # configuration, so a forbidden import inside rules_windows.go would sail
    # through Linux CI unseen. Rather than parse constrained files, refuse to be
    # blind: there is no reason for a build-constrained file in the pure engine,
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
            "$ALLOWED_MODULE_IMPORT") continue ;;
            "$MODULE"/internal/rules|"$MODULE"/internal/rules/*) continue ;;
        esac

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

# fmt stays importable - Errorf and Sprintf are pure and the engine needs them -
# but fmt.Print* and Fprint* write to stdout or a writer and fmt.Fscan* reads
# from a reader, all of which are I/O in a package that must perform none. An
# import rule cannot express that distinction, so it is checked at the call site.
# Sprintf, Sscanf and Errorf operate on strings and stay allowed.
#
# A textual check can only be sound if the qualifier is predictable, so aliased
# and dot imports of fmt are rejected first. `import f "fmt"` or `import . "fmt"`
# would let f.Println or a bare Println through unseen.
aliased="$(cd "$ROOT" && grep -rnE '^[[:space:]]*(import[[:space:]]+)?(\.|[A-Za-z_][A-Za-z0-9_]*)[[:space:]]+"fmt"' \
    --include='*.go' internal/rules 2>/dev/null \
    | grep -v '_test\.go:' \
    | grep -vE ':[[:space:]]*import[[:space:]]+"fmt"([[:space:]]|$)' || true)"
if [ -n "$aliased" ]; then
    violations="$violations$(printf '%s\n' "$aliased" | sed 's/^/  aliased or dot import of fmt: /')"$'\n'
fi

printers="$(cd "$ROOT" && grep -rnE '\bfmt\.(Print|Printf|Println|Fprint|Fprintf|Fprintln|Fscan|Fscanf|Fscanln)\(' \
    --include='*.go' internal/rules 2>/dev/null | grep -v '_test\.go:' || true)"
if [ -n "$printers" ]; then
    violations="$violations$(printf '%s\n' "$printers" | sed 's/^/  writes to output: /')"$'\n'
fi

if [ -n "$violations" ]; then
    echo "check-rules-purity: internal/rules must stay pure — no I/O, no clock, no ambient randomness." >&2
    echo "                    RFC-001 §6.1. The offending direct imports:" >&2
    printf '%s' "$violations" >&2
    echo "" >&2
    echo "                    If you need one of these, the answer is almost certainly to move" >&2
    echo "                    the work to internal/match, not to weaken this gate." >&2
    exit 1
fi

echo "check-rules-purity: OK — $(printf '%s\n' "$pkgs" | wc -l | tr -d ' ') packages, no forbidden direct imports"
