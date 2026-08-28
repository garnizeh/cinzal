#!/usr/bin/env bash
#
# check-no-hard-wrap.sh — reject a PR or issue body that is hard-wrapped at a
# fixed column.
#
# Why this exists as a script rather than as prose in a skill: `pr-publish`
# carried this as an inline `python3 -c` heuristic with its input path baked in,
# and `issue-intake` told the reader to run "pr-publish's mechanical check"
# against a different path — which that snippet could not do without being
# edited first. It was the only check in this harness with no runnable form, in
# a repository whose `delivery-review` preamble insists every step be a concrete
# action rather than a read-through. Hand-wrapping a paragraph has recurred more
# than once (#368 among them) precisely because "eyeball the raw text" is not a
# check.
#
# What it looks for: GitHub and `git log` render a soft-wrapped paragraph as the
# intended block, so a body hard-wrapped at ~80 columns renders as ragged,
# broken lines. The tell is a *run* of consecutive prose lines that all land in
# a narrow length band — one such line is a short paragraph, three in a row is a
# wrapped one. Markdown structure that legitimately needs its own line breaks
# (bullets, headings, tables, fenced code) is excluded rather than counted.
#
# This is an authoring aid for scratchpad files, not a repository gate: it takes
# a path outside the tree and so has no place in `make check`. It still fails
# closed like everything else here — no argument, an unreadable file, or a
# missing `awk` is a failure, never a silent pass.
#
# Usage:
#   scripts/check-no-hard-wrap.sh <file>
#   scripts/check-no-hard-wrap.sh --selftest

set -euo pipefail

# A run of this many consecutive wrapped-looking lines means hard-wrapped.
readonly RUN_THRESHOLD=3
# The length band a fixed-column wrap lands in. Below it, a line is a genuinely
# short paragraph; above it, the author let the renderer wrap.
readonly BAND_MIN=60
readonly BAND_MAX=100

die() {
	printf 'check-no-hard-wrap: %s\n' "$1" >&2
	exit 1
}

command -v awk >/dev/null 2>&1 || die "awk not found — cannot run, so reporting failure rather than passing"

# Prints the longest run of wrapped-looking lines, and the line number the worst
# run starts at. Exits non-zero only on an awk failure; the verdict is the
# caller's to read.
longest_run() {
	awk -v lo="$BAND_MIN" -v hi="$BAND_MAX" '
		BEGIN { run = 0; max = 0; at = 0; start = 0; fence = 0; fchar = ""; flen = 0 }
		{
			line = $0
			sub(/^[[:space:]]+/, "", line)

			# Fenced code blocks keep their own line breaks, so they are skipped
			# rather than counted. A plain open/close toggle is not enough:
			# CommonMark lets a fence nest inside a longer one, and a nested
			# ``` inside a ```` block would close it early, after which real
			# code gets counted as prose. So remember the opening delimiter and
			# its length, and close only on the same character repeated at
			# least as many times.
			#
			# The delimiter run is counted character by character rather than
			# read off RLENGTH: mawk 1.3.4 (the awk on Debian/Ubuntu) does not
			# return the longest match for an interval like /`{3,}/ — it reports
			# RLENGTH 3 for a four-backtick fence — so an RLENGTH-based length
			# comparison silently sees every fence as three and closes on the
			# first nested one anyway. Counting is exact on every awk.
			if (line ~ /^```/ || line ~ /^~~~/) {
				dchar = substr(line, 1, 1)
				dlen = 0
				while (substr(line, dlen + 1, 1) == dchar) dlen++
				if (!fence) {
					fence = 1; fchar = dchar; flen = dlen; run = 0; next
				} else if (dchar == fchar && dlen >= flen) {
					fence = 0; fchar = ""; flen = 0; run = 0; next
				}
				# Otherwise it is a nested fence: content, handled below.
			}
			if (fence) { next }

			# Blank lines, list items, headings, blockquotes and table rows all
			# carry meaningful line breaks of their own.
			if (line == "" || line ~ /^([-*+>|]|#|[0-9]+\.)/) { run = 0; next }
			n = length($0)
			if (n >= lo && n <= hi) {
				if (run == 0) start = NR
				run++
				if (run > max) { max = run; at = start }
			} else {
				run = 0
			}
		}
		END { print max, at }
	' "$1"
}

report() {
	local file="$1" result max at
	result="$(longest_run "$file")"
	max="${result%% *}"
	at="${result##* }"

	if [ "$max" -ge "$RUN_THRESHOLD" ]; then
		printf 'HARD-WRAPPED: %s\n' "$file" >&2
		printf '  %d consecutive prose lines of %d-%d chars, starting at line %d.\n' \
			"$max" "$BAND_MIN" "$BAND_MAX" "$at" >&2
		printf '  Join each paragraph onto one physical line and let the renderer wrap it.\n' >&2
		return 1
	fi

	printf 'ok: %s (longest wrapped-looking run: %d, threshold %d)\n' "$file" "$max" "$RUN_THRESHOLD"
	return 0
}

# --- selftest ---------------------------------------------------------------
#
# Fixture coverage for each verdict the script can reach, held to the same bar
# as the repository's other `*-selftest` targets: a check nobody has watched
# fail is a check nobody knows still works.

selftest() {
	local tmp status=0
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' RETURN

	# 1. A hard-wrapped paragraph — must be caught.
	cat >"$tmp/wrapped.md" <<'EOF'
## Summary

The order log is the only source of truth here, and every projection built on
top of it is a rebuildable cache rather than an authority of its own, which is
what lets a corrupted projection be repaired by one rebuild rather than by a
restore from somewhere else entirely.
EOF
	if report "$tmp/wrapped.md" >/dev/null 2>&1; then
		printf 'selftest FAILED: a hard-wrapped paragraph was accepted\n' >&2
		status=1
	fi

	# 2. The same prose, unwrapped — must pass.
	cat >"$tmp/clean.md" <<'EOF'
## Summary

The order log is the only source of truth here, and every projection built on top of it is a rebuildable cache rather than an authority of its own, which is what lets a corrupted projection be repaired by one rebuild rather than by a restore from somewhere else entirely.
EOF
	if ! report "$tmp/clean.md" >/dev/null 2>&1; then
		printf 'selftest FAILED: an unwrapped paragraph was rejected\n' >&2
		status=1
	fi

	# 3. A table — literal line breaks in the band, and legitimate. Must pass.
	cat >"$tmp/table.md" <<'EOF'
| Gate | Asserts | A failure means |
|---|---|---|
| `purity` | `rules` does no I/O and tells no time | The change reached outside |
| `fog` | `render` never imports `internal/rules` | The view can name the state |
| `packages` | The graph matches `scripts/packages.txt` | A new package appeared |
EOF
	if ! report "$tmp/table.md" >/dev/null 2>&1; then
		printf 'selftest FAILED: a table was rejected as hard-wrapped\n' >&2
		status=1
	fi

	# 4. A fenced code block — same shape, also legitimate. Must pass.
	cat >"$tmp/fenced.md" <<'EOF'
Run it like this:

```bash
rtk gh api repos/garnizeh/cinzal/issues/391 --jq '{number,title,state}'
rtk gh api repos/garnizeh/cinzal/pulls/391/reviews --jq '.[] | .state'
rtk gh api repos/garnizeh/cinzal/branches/main/protection --jq .required_status
```
EOF
	if ! report "$tmp/fenced.md" >/dev/null 2>&1; then
		printf 'selftest FAILED: a fenced code block was rejected as hard-wrapped\n' >&2
		status=1
	fi

	# 5. Consecutive bullets in the band — the shape most likely to false-positive.
	cat >"$tmp/bullets.md" <<'EOF'
- The order log is the truth; match state is derived and never stored anywhere.
- `events` and `match_summary` are rebuildable caches, never authority of their own.
- A corrupted projection is one `--rebuild` away from correct, sending nothing.
EOF
	if ! report "$tmp/bullets.md" >/dev/null 2>&1; then
		printf 'selftest FAILED: consecutive bullets were rejected as hard-wrapped\n' >&2
		status=1
	fi

	# 6. A fence nested inside a longer one. Caught by CodeRabbit on PR #392:
	#    a naive open/close toggle closes on the inner ```, after which the
	#    code below it is counted as prose and reported as hard-wrapped.
	cat >"$tmp/nested-fence.md" <<'EOF'
Intro paragraph that is genuinely short.

````markdown
Inside a four-backtick fence, showing a nested block:

```bash
rtk gh api repos/garnizeh/cinzal/issues/391 --jq .body | head -c 400
rtk gh api repos/garnizeh/cinzal/pulls/392 --jq .head.sha > /tmp/x.txt
rtk gh api repos/garnizeh/cinzal/branches/main/protection --jq .required
```
````
EOF
	if ! report "$tmp/nested-fence.md" >/dev/null 2>&1; then
		printf 'selftest FAILED: a fence nested in a longer fence was rejected as hard-wrapped\n' >&2
		status=1
	fi

	# 7. Fails closed on an unreadable path.
	if "$0" "$tmp/does-not-exist.md" >/dev/null 2>&1; then
		printf 'selftest FAILED: a missing file was accepted\n' >&2
		status=1
	fi

	# 8. Fails closed on no argument.
	if "$0" >/dev/null 2>&1; then
		printf 'selftest FAILED: a missing argument was accepted\n' >&2
		status=1
	fi

	if [ "$status" -eq 0 ]; then
		printf 'check-no-hard-wrap selftest: ok (8 cases)\n'
	fi
	return "$status"
}

# --- entry point ------------------------------------------------------------

if [ "$#" -eq 0 ]; then
	die "no file given. Usage: scripts/check-no-hard-wrap.sh <file> | --selftest"
fi

if [ "$1" = "--selftest" ]; then
	selftest
	exit $?
fi

[ -f "$1" ] || die "not a readable file: $1"
[ -r "$1" ] || die "not a readable file: $1"

report "$1"
