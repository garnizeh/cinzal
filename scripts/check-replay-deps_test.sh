#!/usr/bin/env bash
#
# Fixture coverage for scripts/check-replay-deps.sh (issue #324). Not wired
# into `make check` directly — `make replay-deps-selftest` is, and that
# target is on the check: line, same as purity-selftest and
# bots-isolation-selftest for the same reason: deterministic, synthetic
# input, none of the real cmd/replay's own churn.
#
# check-replay-deps.sh shells out to `go list -deps`, which needs a real,
# buildable go.mod at the root it runs from — a bare target-directory
# argument (the shape check-bots-isolation.go uses) can't stand in for that.
# So, like check-rules-purity_test.sh, this points the gate at a synthetic
# fixture MODULE via environment overrides
# (REPLAY_DEPS_TEST_ROOT/_PKG/_ALLOWLIST) rather than a fixture directory
# inside the real module.
#
# scripts/check-replay-deps_test.sh is this file's own name, deliberately
# matching scripts/check-bots-isolation_test.sh's and
# scripts/check-rules-purity_test.sh's — the standard this self-test is held
# to: not just the gate's success path, but its failure modes, including the
# positive case (a mail-shaped provider introduced), exercised one at a time.

set -euo pipefail

cd "$(dirname "$0")/.."

GATE="$(pwd)/scripts/check-replay-deps.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
fail() {
	printf 'FAIL: %s\n' "$1" >&2
	failed=1
}

# run <root> [pkg] [allowlist] [path] — invokes the gate against a fixture
# module and captures output and exit code without letting `set -e` abort
# this script on the (expected) non-zero cases. `path` overrides $PATH for
# the invocation (Case: go toolchain absent); left unset, the ambient PATH
# is used.
run() {
	local root="$1" pkg="${2:-./cmd/target}" allow="${3:-$1/allowlist.txt}" pathoverride="${4:-}"
	if [ -n "$pathoverride" ]; then
		out="$(PATH="$pathoverride" REPLAY_DEPS_TEST_ROOT="$root" REPLAY_DEPS_TEST_PKG="$pkg" REPLAY_DEPS_TEST_ALLOWLIST="$allow" bash "$GATE" 2>&1)" && code=0 || code=$?
	else
		out="$(REPLAY_DEPS_TEST_ROOT="$root" REPLAY_DEPS_TEST_PKG="$pkg" REPLAY_DEPS_TEST_ALLOWLIST="$allow" "$GATE" 2>&1)" && code=0 || code=$?
	fi
}

# gomod <root> — a minimal buildable module at root.
gomod() {
	printf 'module fixture\n\ngo 1.26\n' >"$1/go.mod"
}

# ---------------------------------------------------------------------------
# Shared fixture module: cmd/target, internal/rules (imports internal/game),
# internal/game, and internal/mail — a two-hop internal chain (proving
# transitive internal packages are inspected, not just direct ones) plus a
# stand-in effect provider that cases below opt into importing or not.
# ---------------------------------------------------------------------------
base="$tmp/base"
mkdir -p "$base/cmd/target" "$base/internal/game" "$base/internal/rules" "$base/internal/mail"
gomod "$base"

cat >"$base/internal/game/game.go" <<'EOF'
package game

type Config struct{}
EOF

cat >"$base/internal/rules/rules.go" <<'EOF'
package rules

import "fixture/internal/game"

func Resolve(cfg game.Config) error { return nil }
EOF

cat >"$base/internal/mail/mail.go" <<'EOF'
// Package mail stands in for an effect provider a fold path must never
// reach (RFC-001 §7.4) — this fixture's whole reason to exist.
package mail

func Send(to, body string) error { return nil }
EOF

printf 'game\nrules\n' >"$base/allowlist.txt"

# ---------------------------------------------------------------------------
# Case 1 (clean): cmd/target imports only internal/rules (which itself pulls
# in internal/game transitively) — both on the allow-list.
# ---------------------------------------------------------------------------
clean="$tmp/clean"
cp -r "$base" "$clean"
cat >"$clean/cmd/target/main.go" <<'EOF'
package main

import "fixture/internal/rules"

func main() {
	_ = rules.Resolve
}
EOF
run "$clean"
[ "$code" -eq 0 ] || fail "clean fixture: expected exit 0, got $code: $out"
printf '%s\n' "$out" | grep -q '^check-replay-deps: OK' || fail "clean fixture: expected an OK line, got: $out"
printf '%s\n' "$out" | grep -q '2 internal package' || fail "clean fixture: expected 2 internal packages counted (game, rules), got: $out"

# ---------------------------------------------------------------------------
# Case 2 (violation, the positive case): cmd/target additionally imports
# internal/mail, the stand-in effect provider. Allow-list unchanged.
# ---------------------------------------------------------------------------
providerreach="$tmp/provider-reach"
cp -r "$base" "$providerreach"
cat >"$providerreach/cmd/target/main.go" <<'EOF'
package main

import (
	"fixture/internal/mail"
	"fixture/internal/rules"
)

func main() {
	_ = rules.Resolve
	_ = mail.Send
}
EOF
run "$providerreach"
[ "$code" -eq 1 ] || fail "provider reached: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'fixture/internal/mail' || fail "provider reached: expected fixture/internal/mail named as offending, got: $out"

# ---------------------------------------------------------------------------
# Case 3 (zero internal packages reached): cmd/target imports only stdlib.
# The allow-list is deliberately permissive (every name any fixture here
# uses) — proves the internal_deps-empty guard is independent of the
# allow-list, so a permissive list can never turn "nothing was inspected"
# into a silent pass.
# ---------------------------------------------------------------------------
novacuous="$tmp/no-internal-deps"
mkdir -p "$novacuous/cmd/target"
gomod "$novacuous"
cat >"$novacuous/cmd/target/main.go" <<'EOF'
package main

import "fmt"

func main() {
	fmt.Println("no internal deps here")
}
EOF
printf 'game\nrules\nmail\n' >"$novacuous/allowlist.txt"
run "$novacuous"
[ "$code" -eq 1 ] || fail "zero internal deps: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'named no internal/ package at all' ||
	fail "zero internal deps: expected the nothing-inspected message, got: $out"

# ---------------------------------------------------------------------------
# Case 3b (go list -deps itself reports nothing): the guard one line above the
# internal_deps-empty guard Case 3 exercises — `[ -n "$deps" ]`, on the raw
# `go list -deps` output before the internal/-prefix filter ever runs. Real
# `go`, run against a real package, always names the package itself even with
# zero imports (verified by hand: `go list -deps` on a no-import package
# still prints its own path), so this can only be reached by faking `go`
# itself — a minimal stand-in on PATH that answers `list -m` and reports
# nothing for `list -deps`, leaving every other tool this script needs
# (bash, grep, sed, dirname, wc) resolving from the ambient PATH unchanged.
# ---------------------------------------------------------------------------
emptydeps="$tmp/empty-go-list-deps"
mkdir -p "$emptydeps/cmd/target"
gomod "$emptydeps"
cat >"$emptydeps/cmd/target/main.go" <<'EOF'
package main

func main() {}
EOF
printf 'game\n' >"$emptydeps/allowlist.txt"

fake_go_bin="$tmp/fake-go-bin"
mkdir -p "$fake_go_bin"
cat >"$fake_go_bin/go" <<'EOF'
#!/usr/bin/env bash
case "$1 $2" in
	"list -m") echo "fixture" ;;
	"list -deps") exit 0 ;;
	*) exit 1 ;;
esac
EOF
chmod +x "$fake_go_bin/go"

out="$(PATH="$fake_go_bin:$PATH" REPLAY_DEPS_TEST_ROOT="$emptydeps" REPLAY_DEPS_TEST_PKG="./cmd/target" REPLAY_DEPS_TEST_ALLOWLIST="$emptydeps/allowlist.txt" bash "$GATE" 2>&1)" && code=0 || code=$?
[ "$code" -eq 1 ] || fail "empty go list -deps: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'reported nothing' || fail "empty go list -deps: expected the reported-nothing message, got: $out"

# ---------------------------------------------------------------------------
# Case 4 (missing allow-list): unreadable path must fail closed rather than
# default to permissive or empty.
# ---------------------------------------------------------------------------
run "$clean" "./cmd/target" "$tmp/does-not-exist.txt"
[ "$code" -eq 1 ] || fail "missing allow-list: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'could not read the allow-list' ||
	fail "missing allow-list: expected the allow-list-read failure line, got: $out"

# ---------------------------------------------------------------------------
# Case 5 (go toolchain absent): an isolated PATH holding nothing but symlinks
# to bash's own resolved absolute path, plus dirname (the gate's very first
# line, computing REPO_ROOT, needs it before the `command -v go` check ever
# runs) — the scripts/check-generate_test.sh idiom, so this does not depend
# on where a real `go` binary happens to live on the host running this test.
# ---------------------------------------------------------------------------
bash_abs="$(command -v bash)"
dirname_abs="$(command -v dirname)"
isolated_path_dir="$tmp/isolated-path"
mkdir -p "$isolated_path_dir"
ln -s "$bash_abs" "$isolated_path_dir/bash"
ln -s "$dirname_abs" "$isolated_path_dir/dirname"
run "$clean" "./cmd/target" "$clean/allowlist.txt" "$isolated_path_dir"
[ "$code" -eq 1 ] || fail "go absent: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'the go toolchain is not on PATH' ||
	fail "go absent: expected the missing-toolchain message, got: $out"

# ---------------------------------------------------------------------------
# Case 6 (go list itself fails): a package pattern matching nothing.
# ---------------------------------------------------------------------------
run "$clean" "./cmd/does-not-exist"
[ "$code" -eq 1 ] || fail "bad pattern: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'did not succeed' || fail "bad pattern: expected the go-list-failure line, got: $out"

if [ "$failed" -eq 0 ]; then
	echo "check-replay-deps_test: PASS"
fi
exit "$failed"
