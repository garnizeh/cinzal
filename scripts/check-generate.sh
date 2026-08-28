#!/usr/bin/env bash
#
# Asserts that the committed generated code named on argv matches what the
# generators actually produce. Extracted out of the Makefile's own
# generate-check recipe (issue #316) once that recipe grew enough branches to
# warrant a companion selftest, matching every other non-trivial gate in this
# repository (check-packages.sh, check-rules-purity.sh, check-bots-isolation.go,
# and so on) instead of staying the one inline exception.
#
# THIS CHECK FAILS CLOSED, the same discipline as every other gate here:
# - No paths given (argv empty) -> FAIL, not "nothing to compare, so pass".
#   An empty GENERATED here means exactly what check-packages.sh's own header
#   describes for an empty `go list`: a comparison against nothing always
#   succeeds, and a gate built the obvious way reports green having inspected
#   zero paths. See CLAUDE.md, "Absence of a signal is not evidence of a
#   state."
# - git itself failing to inspect the given paths -> FAIL, not "nothing
#   reported dirty, so pass".
# - The paths reporting dirty (regenerating changed something not committed,
#   or a committed file was hand-edited or corrupted) -> FAIL, with the diff.
#
# Callers: the Makefile's generate-check target, which regenerates first (see
# its own `generate` prerequisite) and then calls this script with
# GENERATED's contents as argv; and scripts/check-generate_test.sh, which
# calls it directly against synthetic fixture git repos and a deliberately
# broken git environment to exercise the branches above without touching the
# real internal/store output.

set -euo pipefail

fail() {
	printf 'check-generate: FAIL: %s\n' "$*" >&2
	exit 1
}

if [ "$#" -eq 0 ]; then
	printf 'check-generate: no generated paths given -- nothing to compare.\n' >&2
	printf '                this check is VACUOUS, not passing. See the GENERATED variable.\n' >&2
	exit 1
fi

dirty="$(git status --porcelain -- "$@")" ||
	fail "git could not inspect the generated paths -- failing rather than reporting OK on an inspection that did not happen."

if [ -n "$dirty" ]; then
	printf 'check-generate: committed generated code does not match the tools output.\n' >&2
	printf '                run `make generate` and commit the result.\n\n' >&2
	printf '%s\n' "$dirty" >&2
	exit 1
fi

printf 'check-generate: OK -- %s unchanged\n' "$*"
