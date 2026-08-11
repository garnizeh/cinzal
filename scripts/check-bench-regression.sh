#!/usr/bin/env bash
#
# Flags a benchmark regression between two `go test -bench` result files —
# issue #113, the comparison step deliberately deferred by #112.
#
# THIS IS ADVISORY, NOT A CI GATE. A non-zero exit here means "annotate the
# pull request", never "block the merge" — this script does not decide that;
# the bench-compare job in .github/workflows/ci.yml does, by never being
# added to main's required status check contexts (see CONTRIBUTING.md "What
# is deliberately not a gate": a required check that can go red on noise
# teaches contributors a red check might be nothing, the same failure this
# project already spent real effort undoing for CodeRabbit's own
# green-on-skip behaviour). Two data points (before/after #61) is not enough
# to characterise real CI-runner noise, so this starts advisory; promoting
# it later is a one-line change to the workflow, not to this script.
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

set -euo pipefail

if [ "$#" -ne 2 ]; then
	echo "usage: $0 <baseline.bench> <candidate.bench>" >&2
	exit 1
fi

BASELINE="$1"
CANDIDATE="$2"
THRESHOLD="${CINZAL_BENCH_THRESHOLD:-20}" # percent, slower-than-base, see header

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

if [ "$comparable_rows" -eq 0 ]; then
	verdict="check-bench-regression: no comparable sec/op rows between $BASELINE and $CANDIDATE — inconclusive, not clean (mismatched benchmark names or metadata?)"
	status=1
elif [ -n "$regressions" ]; then
	verdict="check-bench-regression: possible regression(s) detected (sec/op, vs $BASELINE):
$regressions"
	status=1
else
	verdict="check-bench-regression: no sec/op regression past ${THRESHOLD}% vs $BASELINE ($comparable_rows row(s) compared)"
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

