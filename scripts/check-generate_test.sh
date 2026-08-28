#!/usr/bin/env bash
#
# Fixture coverage for check-generate.sh (issue #316). Not wired into
# `make check` directly -- `make generate-check-selftest` is, and that
# target is on the check-nosecrets: line, same as purity-selftest and
# bots-isolation-selftest for the same reason: deterministic, synthetic
# input, none of the real internal/store churn or a real `sqlc generate`
# run's cost.
#
# scripts/check-generate_test.sh is this file's own name, deliberately
# matching scripts/check-bots-isolation_test.sh's and
# scripts/check-bench-regression_test.sh's -- that is the standard this
# self-test is held to: not just the gate's success path, but its failure
# modes, exercised one at a time.

set -euo pipefail

cd "$(dirname "$0")/.."

ROOT="$(pwd)"
GATE="$ROOT/scripts/check-generate.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
fail() {
	printf 'FAIL: %s\n' "$1" >&2
	failed=1
}

# ---------------------------------------------------------------------------
# Case 1 (clean): a synthetic git repo whose working tree matches HEAD
# exactly for the one path named. Mirrors the real generate-check's success
# path without touching internal/store or shelling out to sqlc.
# ---------------------------------------------------------------------------
repo="$tmp/repo"
mkdir -p "$repo"
git -C "$repo" init -q
git -C "$repo" config user.email test@example.com
git -C "$repo" config user.name test
printf 'generated content\n' >"$repo/generated.txt"
git -C "$repo" add generated.txt
git -C "$repo" commit -q -m 'seed generated.txt'

out="$(cd "$repo" && "$GATE" generated.txt 2>&1)" && code=0 || code=$?
[ "$code" -eq 0 ] || fail "clean fixture: expected exit 0, got $code: $out"
printf '%s\n' "$out" | grep -q '^check-generate: OK' || fail "clean fixture: expected an OK line, got: $out"

# ---------------------------------------------------------------------------
# Case 2 (dirty / corrupted): the same repo, with the committed file edited
# afterward and not committed -- the fixture-repo equivalent of the
# corrupted-generated-file demonstration this issue also runs once, by hand,
# against the real internal/store/db.go (see the PR description).
# ---------------------------------------------------------------------------
printf 'corrupted content\n' >>"$repo/generated.txt"
out="$(cd "$repo" && "$GATE" generated.txt 2>&1)" && code=0 || code=$?
[ "$code" -eq 1 ] || fail "dirty fixture: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'does not match the tools output' ||
	fail "dirty fixture: expected the drift message, got: $out"
printf '%s\n' "$out" | grep -q 'generated.txt' ||
	fail "dirty fixture: expected the dirty path named in the diff, got: $out"

# ---------------------------------------------------------------------------
# Case 3 (VACUOUS: empty GENERATED): no paths on argv at all. Must fail, not
# pass -- the same convention bots-isolation's own vacuous-package case
# uses. This is the "GENERATED emptied" acceptance criterion: asserted here
# by a fixture, not by reading the Makefile.
# ---------------------------------------------------------------------------
out="$("$GATE" 2>&1)" && code=0 || code=$?
[ "$code" -eq 1 ] || fail "empty argv: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'VACUOUS' || fail "empty argv: expected a VACUOUS line, got: $out"

# ---------------------------------------------------------------------------
# Case 4 (git cannot inspect): GIT_DIR/GIT_WORK_TREE pointed at a path that
# does not exist forces `git status` itself to fail, independent of cwd --
# confirmed against a live `git status` invocation before writing this
# fixture (fatal: not a git repository, exit 128). This is the existing
# error branch the issue asks to be re-verified, not just trusted to still
# work.
# ---------------------------------------------------------------------------
out="$(GIT_DIR=/nonexistent-check-generate-fixture GIT_WORK_TREE=/nonexistent-check-generate-fixture "$GATE" some/path 2>&1)" && code=0 || code=$?
[ "$code" -eq 1 ] || fail "git cannot inspect: expected exit 1, got $code: $out"
printf '%s\n' "$out" | grep -q 'could not inspect' ||
	fail "git cannot inspect: expected the inspection-failure line, got: $out"

# ---------------------------------------------------------------------------
# Case 5 (missing generator: require-sqlc must not be swallowed): builds a
# PATH excluding sqlc's own directory and runs the real `make generate-check`
# against this repository's real Makefile. sqlc, golangci-lint and templ all
# resolve into the same `go install` bin directory here, separate from
# `git`/`make` -- confirmed with `command -v` before writing this fixture --
# so filtering that one directory out removes only sqlc's reachability for
# this invocation, not the tools generate-check's own recipe still needs
# (git). Nothing is written: require-sqlc fails before `generate`'s `sqlc
# generate` step ever runs.
# ---------------------------------------------------------------------------
if sqlc_path="$(command -v sqlc 2>/dev/null)"; then
	sqlc_dir="$(dirname "$sqlc_path")"
	filtered_path="$(printf '%s' "$PATH" | tr ':' '\n' | grep -vFx "$sqlc_dir" | paste -sd: -)"

	out="$(PATH="$filtered_path" make -C "$ROOT" generate-check 2>&1)" && code=0 || code=$?
	[ "$code" -ne 0 ] || fail "missing sqlc: expected generate-check to fail without sqlc on PATH, got exit 0: $out"
	printf '%s\n' "$out" | grep -q 'sqlc is required and is not on PATH' ||
		fail "missing sqlc: expected the require-sqlc failure message, got: $out"
	if printf '%s\n' "$out" | grep -q 'check-generate: OK'; then
		fail "missing sqlc: check-generate must never run at all here, but got an OK line: $out"
	fi
else
	# Fail closed here too: CONTRIBUTING.md says sqlc is no longer skippable
	# as of issue #315 (make test/make check already need it), so an
	# environment running this selftest without sqlc on PATH is missing a
	# real prerequisite, not a reason to skip Case 5.
	fail "sqlc not found on PATH -- cannot exercise the require-sqlc fixture (see CONTRIBUTING.md's Requirements section)"
fi

if [ "$failed" -eq 0 ]; then
	echo "check-generate_test: PASS"
fi
exit "$failed"
