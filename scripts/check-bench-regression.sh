#!/usr/bin/env bash
#
# Flags a benchmark regression between two `go test -bench` result files —
# issue #113, the comparison step deliberately deferred by #112.
#
# THIS IS A CI GATE. A non-zero exit here blocks the merge — the bench-compare
# job in .github/workflows/ci.yml is a required status check on `main`
# (issue #127). It started advisory (two data points, before/after #61, was
# not enough to characterise real CI-runner noise) and was promoted once
# seven real same-runner comparisons had landed with zero false positives:
# worst single-case drift +6.27% (issue #124) and worst geomean drift 0.93%,
# both well inside THRESHOLD and GEOMEAN_THRESHOLD below. See issue #127 for
# the full data and CONTRIBUTING.md's gate table for where this is recorded.
#
# WHY BOTH A P-VALUE AND A MAGNITUDE THRESHOLD.
#
# benchstat already tests significance at its own default alpha (0.05) and
# prints "~" when a difference is not distinguishable from noise. That alone
# is not enough on a shared runner: measured directly while building this
# gate, two back-to-back runs of *identical* code, with a short -benchtime,
# produced a "significant" -31.79% swing (p=0.004) on one case purely from
# scheduling noise. Requiring a real magnitude on top of significance —
# CINZAL_BENCH_THRESHOLD, generous by default — is what keeps that kind of
# noise from reading as a regression. The default -benchtime and -count=10
# the CI job uses (see ci.yml) were chosen because they did not reproduce
# that swing across repeated identical-code comparisons.
#
# WHY ONLY sec/op.
#
# BenchmarkGenerate's own doc comment calls its number "the one a regression
# would actually be felt as" — that stays the single gated signal. benchstat
# still prints B/op and allocs/op in its normal output for a human to read;
# they are informational here, not compared against a threshold.
#
# WHY THE geomean ROW IS ALSO CHECKED, AND WITH A SEPARATE, LOWER THRESHOLD
# (issue #148).
#
# The per-case scan below skips the geomean row on purpose — it is an
# aggregate, not a benchmark — but that made the gate per-case ONLY: a change
# that slowed every benchmark by the same ~19% passed clean, because no
# single row crossed the 20% threshold even though benchstat's own geomean
# row read +19.00%. Verified directly: a synthetic candidate built by scaling
# every ns/op in a real 10-count sample of internal/rules/gen's suite by
# 1.19 produced exactly that — 18/18 rows individually "+19.00%, below
# threshold" and a geomean of "+19.00%" — and the unpatched script exited 0.
#
# Gating the geomean row at the SAME threshold as individual cases cannot fix
# this: when every row moves by the same percentage, the geomean moves by
# that identical percentage (it is a mean), so a uniform slowdown that stays
# under THRESHOLD per row is, by construction, also under THRESHOLD in
# aggregate. Catching it needs a threshold the uniform case can actually
# cross — CINZAL_BENCH_GEOMEAN_THRESHOLD, lower than THRESHOLD by default.
#
# The gap is safe to spend because aggregating cancels a good deal of
# per-row noise: measured directly, two independent 10-count same-runner
# samples of identical code — the same setup #124/#125 characterised at
# ~1-6% drift for a single untouched benchmark — moved the geomean by only
# +0.71%. The default (10%) leaves >10x that margin, comfortably below
# THRESHOLD's own 20% and well clear of the 19% synthetic case above.

set -euo pipefail

if [ "$#" -ne 2 ]; then
	echo "usage: $0 <baseline.bench> <candidate.bench>" >&2
	exit 1
fi

BASELINE="$1"
CANDIDATE="$2"
THRESHOLD="${CINZAL_BENCH_THRESHOLD:-20}"                 # percent, slower-than-base, see header
GEOMEAN_THRESHOLD="${CINZAL_BENCH_GEOMEAN_THRESHOLD:-10}" # percent, aggregate, see "WHY THE geomean ROW..." above

# A non-numeric threshold silently disables the check it gates rather than
# erroring: awk's `pct + 0 > threshold` falls back to a string comparison
# against anything that isn't a POSIX numeric string — and that includes
# something that merely looks numeric, like "1.2.3" — and every percentage
# this script prints ("85.69" and friends) sorts below a non-numeric string
# lexicographically — verified directly:
# awk -v threshold=invalid 'BEGIN { print (85.69 > threshold) }' prints 0.
# That means every regression, at any magnitude, would silently stop being
# flagged — the opposite of "fail closed". Matched with bash's =~ rather
# than a case glob for exactly that reason: a glob like *[!0-9.]* accepts
# "1.2.3", which still hits the same bug once it reaches awk.
for pair in "THRESHOLD:CINZAL_BENCH_THRESHOLD" "GEOMEAN_THRESHOLD:CINZAL_BENCH_GEOMEAN_THRESHOLD"; do
	var="${pair%%:*}"
	env_name="${pair#*:}"
	val="${!var}"
	if ! [[ "$val" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
		echo "check-bench-regression: $env_name must be a non-negative number, got '$val'" >&2
		exit 1
	fi
done

for f in "$BASELINE" "$CANDIDATE"; do
	if [ ! -s "$f" ]; then
		echo "check-bench-regression: $f is missing or empty — nothing to compare" >&2
		exit 1
	fi
done

if ! command -v benchstat >/dev/null 2>&1; then
	echo "check-bench-regression: benchstat is required and is not on PATH" >&2
	echo "                        go install golang.org/x/perf/cmd/benchstat@<pinned version>" >&2
	exit 1
fi

# Captured rather than streamed straight to stdout: the same text is also
# what gets written to $GITHUB_STEP_SUMMARY below, in CI, so the PR
# annotation and this script's own console output are the same bytes, not
# two things that can drift apart.
text="$(benchstat "$BASELINE" "$CANDIDATE")"
printf '%s\n' "$text"
echo

# -format=csv is parsed below; its warnings ("F21: all samples are equal"
# and siblings) go to stderr and are discarded here — they describe rows
# benchstat could not compute a CI for, not a failure of this script.
csv="$(benchstat -format=csv "$BASELINE" "$CANDIDATE" 2>/dev/null)"

# Counted separately from the regression scan below: a benchmark that
# exists in only one of the two files (disjoint names, or metadata
# benchstat can't line up) has no candidate-side value, so its CSV row is
# short a field and $(NF-1) resolves to something else entirely rather than
# a "vs base" percentage — that row must not count as compared. Requiring
# NF >= 7 (name,base,baseCI,cand,candCI,vs,P) and a genuine vs value ("~" or
# a signed percentage) is what makes "0 comparable rows" mean what it says.
# Zero comparable rows is inconclusive, not clean — the same "a check that
# did not run must not report success" rule this repo's other gates hold to.
comparable_rows="$(printf '%s\n' "$csv" | awk -F',' '
	/^,sec\/op,/ { intable = 1; next }
	intable && /^$/ { intable = 0 }
	intable && $1 != "" && $1 != "geomean" && NF >= 7 {
		vs = $(NF - 1)
		if (vs == "~" || vs ~ /^[+-][0-9.]+%$/) count++
	}
	END { print count + 0 }
')"

regressions="$(printf '%s\n' "$csv" | awk -F',' -v threshold="$THRESHOLD" '
	/^,sec\/op,/ { intable = 1; next }
	intable && /^$/ { intable = 0 }
	intable && $1 != "" && $1 != "geomean" && NF >= 7 {
		vs = $(NF - 1)
		if (vs != "~" && vs ~ /^\+/) {
			pct = vs
			gsub(/[+%]/, "", pct)
			if (pct + 0 > threshold) {
				printf "%s: %s slower than baseline (threshold %s%%)\n", $1, vs, threshold
			}
		}
	}
')"

# Catches a uniform slowdown no single row is loud enough to trip — see
# "WHY THE geomean ROW IS ALSO CHECKED" above. Same table, same threshold,
# same "only a genuine +pct past threshold counts" shape as the scan above;
# the only difference is $1 == "geomean" instead of excluding it.
geomean_regression="$(printf '%s\n' "$csv" | awk -F',' -v threshold="$GEOMEAN_THRESHOLD" '
	/^,sec\/op,/ { intable = 1; next }
	intable && /^$/ { intable = 0 }
	intable && $1 == "geomean" && NF >= 7 {
		vs = $(NF - 1)
		if (vs != "~" && vs ~ /^\+/) {
			pct = vs
			gsub(/[+%]/, "", pct)
			if (pct + 0 > threshold) {
				printf "geomean: %s slower than baseline (geomean threshold %s%%) — every benchmark moved together; no single case crossed the per-case threshold\n", vs, threshold
			}
		}
	}
')"

ns=$'\n'
all_regressions="$regressions"
if [ -n "$geomean_regression" ]; then
	all_regressions="${all_regressions:+$all_regressions$ns}$geomean_regression"
fi

if [ "$comparable_rows" -eq 0 ]; then
	# benchstat pairs rows on the benchmark name AND on the configuration
	# lines above it (goos, goarch, pkg, cpu), so a differing cpu: line alone
	# empties this count while both files look perfectly fine to a reader.
	# That is what #125 was: the two samples came from different hosted
	# runners. CI now measures both sides on one runner, so a mismatch here
	# means something else — a renamed or deleted benchmark, or a baseline
	# taken from another machine — and either way this is inconclusive, which
	# is not clean.
	verdict="check-bench-regression: no comparable sec/op rows between $BASELINE and $CANDIDATE — inconclusive, not clean (renamed benchmark, or differing goos/goarch/pkg/cpu metadata — compare the two files' headers)"
	status=1
elif [ -n "$all_regressions" ]; then
	verdict="check-bench-regression: possible regression(s) detected (sec/op, vs $BASELINE):
$all_regressions"
	status=1
else
	verdict="check-bench-regression: no sec/op regression past ${THRESHOLD}% per-case or ${GEOMEAN_THRESHOLD}% geomean vs $BASELINE ($comparable_rows row(s) compared)"
	status=0
fi

if [ "$status" -ne 0 ]; then
	echo "$verdict" >&2
else
	echo "$verdict"
fi

# Advisory annotation, not a gate — see this file's own header. Written
# whenever this runs inside GitHub Actions (the variable is only ever set
# there), regardless of $status: a clean comparison is exactly as worth
# seeing on the pull request as a flagged one.
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
	{
		echo '### Benchmark comparison (issue #113, advisory — never blocks merge)'
		echo '```'
		printf '%s\n' "$text"
		echo
		echo "$verdict"
		echo '```'
	} >>"$GITHUB_STEP_SUMMARY"
fi

exit "$status"

