#!/usr/bin/env bash
#
# Fixture coverage for check-bots-isolation.go (issue #195). Not wired into
# `make check` directly — `make bots-isolation-selftest` is, and that target
# is on the check: line, same as bench-regression-selftest for the same
# reason: deterministic, synthetic input, none of a real benchmark's noise
# or (here) a real package's churn.
#
# Every case below is a standalone fixture directory under a temp root, each
# holding exactly the .go files it needs — the checker takes a target
# directory as its first argument specifically so this script can point it
# at fixtures instead of the real internal/bots (see check-bots-isolation.go's
# own doc comment).
#
# scripts/check-bots-isolation_test.sh is this file's own name, deliberately
# matching scripts/check-bench-regression_test.sh's — that is the standard
# this self-test is held to: not just the gate's success path, but its
# failure modes, exercised one at a time.

set -euo pipefail

cd "$(dirname "$0")/.."

GATE="scripts/check-bots-isolation.go"
ALLOWLIST="scripts/bots-isolation-allowlist.txt"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
fail() {
	printf 'FAIL: %s\n' "$1" >&2
	failed=1
}

# run <fixture-dir> [allowlist] — invokes the gate and captures output and
# exit code without letting `set -e` abort this script on the (expected)
# non-zero cases.
run() {
	local dir="$1" allow="${2:-$ALLOWLIST}"
	out="$(go run "$GATE" "$dir" "$allow" 2>&1)" && code=0 || code=$?
}

# ---------------------------------------------------------------------------
# Case 1 (clean): imports rules for allow-listed symbols only. Mirrors the
# real internal/bots shape — Legal, Affordances, Steps, BotRNG — plus a
# harmless [31]byte to prove the seed check is exact about the length, not
# "any fixed-size byte array".
# ---------------------------------------------------------------------------
clean="$tmp/clean"
mkdir -p "$clean"
cat >"$clean/bot.go" <<'EOF'
package bots

import "github.com/garnizeh/cinzal/internal/rules"

type Bot interface {
	Decide(v PlayerView, cfg Config, r *rules.BotRNG) Order
}

type notASeed [31]byte

func decide(v PlayerView, cfg Config, o Order, r *rules.BotRNG) (Order, error) {
	if err := rules.Legal(v, o, cfg); err != nil {
		return Order{}, err
	}
	a := rules.Affordances(v, cfg, o)
	_ = a.MaxLeaseBlocks
	_ = rules.Steps(v, cfg)
	return o, nil
}

type PlayerView struct{}
type Config struct{}
type Order struct{}
EOF
run "$clean"
[ "$code" -eq 0 ] || fail "clean fixture: expected exit 0, got $code: $out"
printf '%s\n' "$out" | grep -q '^check-bots-isolation: OK' || fail "clean fixture: expected an OK line, got: $out"

# ---------------------------------------------------------------------------
# Case 2 (violation, named type): a declaration spells rules.MatchState
# directly.
# ---------------------------------------------------------------------------
named="$tmp/named-matchstate"
mkdir -p "$named"
cat >"$named/bot.go" <<'EOF'
package bots

import "github.com/garnizeh/cinzal/internal/rules"

func peek(s rules.MatchState) {}
EOF
run "$named"
[ "$code" -eq 1 ] || fail "named MatchState: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'rules.MatchState is not on the isolation allow-list' ||
	fail "named MatchState: expected the allow-list violation line, got: $out"

# ---------------------------------------------------------------------------
# Case 3 (violation, inferred local): the shape the gate exists to catch that
# a plain "does the text MatchState appear" scan does not — a `:=` local
# never spells the type name; only the disallowed rules.NewMatch selector
# gives it away, so this fixture is the reason the check walks selectors
# rather than identifiers.
# ---------------------------------------------------------------------------
inferred="$tmp/inferred-matchstate"
mkdir -p "$inferred"
cat >"$inferred/bot.go" <<'EOF'
package bots

import "github.com/garnizeh/cinzal/internal/rules"

func peek(seed [32]byte) {
	s, err := rules.NewMatch(seed, 2)
	_, _ = s, err
}
EOF
run "$inferred"
[ "$code" -eq 1 ] || fail "inferred MatchState local: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'rules.NewMatch is not on the isolation allow-list' ||
	fail "inferred MatchState local: expected a rules.NewMatch violation line, got: $out"
printf '%s\n' "$out" | grep -q 'the match seed' ||
	fail "inferred MatchState local: expected the [32]byte parameter to also be flagged, got: $out"

# ---------------------------------------------------------------------------
# Case 4 (violation, seed shape alone): [32]byte flagged even with no rules
# import in the file at all — the point of keeping this check independent of
# the selector scan.
# ---------------------------------------------------------------------------
seed="$tmp/seed-only"
mkdir -p "$seed"
cat >"$seed/bot.go" <<'EOF'
package bots

func remember(seed [32]byte) [32]byte {
	return seed
}
EOF
run "$seed"
[ "$code" -eq 1 ] || fail "bare [32]byte: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'the match seed' || fail "bare [32]byte: expected the seed-shape line, got: $out"

# ---------------------------------------------------------------------------
# Case 5 (violation, dot import): binds every rules identifier into file
# scope, which would otherwise let MatchState through unqualified.
# ---------------------------------------------------------------------------
dotimport="$tmp/dot-import"
mkdir -p "$dotimport"
cat >"$dotimport/bot.go" <<'EOF'
package bots

import . "github.com/garnizeh/cinzal/internal/rules"

func peek() {
	_ = Legal
}
EOF
run "$dotimport"
[ "$code" -eq 1 ] || fail "dot import: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'dot-imports' || fail "dot import: expected the dot-import line, got: $out"

# ---------------------------------------------------------------------------
# Case 6 (VACUOUS): only doc.go present. Must fail, not pass — the same
# convention generate-check uses for an empty GENERATED.
# ---------------------------------------------------------------------------
vacuous="$tmp/vacuous"
mkdir -p "$vacuous"
cat >"$vacuous/doc.go" <<'EOF'
// Package bots is empty for now.
package bots
EOF
run "$vacuous"
[ "$code" -eq 1 ] || fail "vacuous package: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'VACUOUS' || fail "vacuous package: expected a VACUOUS line, got: $out"

# ---------------------------------------------------------------------------
# Case 7 (parse error): must fail rather than silently skip the broken file.
# ---------------------------------------------------------------------------
broken="$tmp/parse-error"
mkdir -p "$broken"
cat >"$broken/bot.go" <<'EOF'
package bots

func broken( {
EOF
run "$broken"
[ "$code" -eq 1 ] || fail "parse error: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'FAIL' || fail "parse error: expected a FAIL line, got: $out"

# ---------------------------------------------------------------------------
# Case 8 (missing allow-list): an unreadable allow-list file must fail
# closed rather than default to permissive or empty.
# ---------------------------------------------------------------------------
run "$clean" "$tmp/does-not-exist.txt"
[ "$code" -eq 1 ] || fail "missing allow-list: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'could not read the allow-list' ||
	fail "missing allow-list: expected the allow-list-read failure line, got: $out"

if [ "$failed" -eq 0 ]; then
	echo "check-bots-isolation_test: PASS"
fi
exit "$failed"
