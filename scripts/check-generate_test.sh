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
# Case 5 (missing generator: require-sqlc must not be swallowed): runs the
# real `make generate-check` against this repository's real Makefile with an
# isolated PATH that cannot resolve sqlc, and confirms require-sqlc fails
# before `generate`'s `sqlc generate` step -- and generate-check's own
# check-generate.sh -- ever run.
#
# This used to build that PATH by filtering sqlc's own directory out of the
# ambient $PATH, on the premise that sqlc lives in a `go install` bin
# directory separate from make/bash. CodeRabbit's review of PR #389 ran
# `command -v` against sqlc/make/bash in its own environment and found them
# sharing one directory there; filtering that directory out would also drop
# make's and/or bash's own reachability, so the fixture would then fail for
# the wrong reason (make or bash not found) rather than the one under test.
# Reproduced by hand: symlinking sqlc/make/bash into one shared directory and
# filtering it out the old way broke with "make: command not found" /
# "bash: No such file or directory", never reaching require-sqlc at all.
#
# Fixed by not depending on directory layout in the first place: resolve
# make's and bash's real absolute paths from the current, unfiltered PATH,
# then run generate-check against a wholly new isolated PATH containing
# nothing but a symlink to bash (SHELL := bash in the Makefile is a bare
# name, so Make still needs to find it via PATH) -- invoking make itself via
# its resolved absolute path so the make lookup never touches that isolated
# PATH at all. sqlc is absent from it unconditionally, regardless of which
# directory it happens to resolve from on the host running this test.
# ---------------------------------------------------------------------------
if command -v sqlc >/dev/null 2>&1; then
	make_abs="$(command -v make)"
	bash_abs="$(command -v bash)"
	isolated_path_dir="$tmp/isolated-path"
	mkdir -p "$isolated_path_dir"
	ln -s "$bash_abs" "$isolated_path_dir/bash"

	out="$(PATH="$isolated_path_dir" "$make_abs" -C "$ROOT" generate-check 2>&1)" && code=0 || code=$?
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
