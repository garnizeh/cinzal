#!/usr/bin/env bash
#
# Fixture coverage for check-bench-regression.sh's geomean check (issue #148).
# Not wired into `make check` or CI — none of this repo's other check-*.sh
# scripts are self-tested there either. This exists so the fix is
# reproducible without benchmarking the real suite: run it with
#   bash scripts/check-bench-regression_test.sh
#
# Three synthetic benchmarks, 10 samples each (BENCH_COUNT, see Makefile) so
# benchstat treats them the same way it treats real CI output.

set -euo pipefail

cd "$(dirname "$0")/.."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failed=0
fail() {
	printf 'FAIL: %s\n' "$1" >&2
	failed=1
}

# gen_bench <file> <multiplier> — ten identical samples per benchmark, each
# scaled by <multiplier> from a fixed base so the comparison is deterministic.
gen_bench() {
	local file="$1" mult="$2"
	{
		echo "goos: linux"
		echo "goarch: amd64"
		echo "pkg: example.com/fixture"
		echo "cpu: Fixture CPU"
		for bench in "BenchmarkA-8:1000" "BenchmarkB-8:2000" "BenchmarkC-8:5000"; do
			name="${bench%%:*}"
			base_ns="${bench#*:}"
			ns=$(awk -v b="$base_ns" -v m="$mult" 'BEGIN { printf "%.0f", b * m }')
			for _ in $(seq 1 10); do
				printf '%s\t1000\t%s ns/op\n' "$name" "$ns"
			done
		done
	} >"$file"
}

gen_bench "$tmp/base.bench" 1.0

# Case 1 (the bug this issue closes): every row +19%, individually below the
# 20% per-case threshold, geomean also +19% — above the 10% default
# CINZAL_BENCH_GEOMEAN_THRESHOLD. Must be flagged.
gen_bench "$tmp/uniform19.bench" 1.19
out="$(./scripts/check-bench-regression.sh "$tmp/base.bench" "$tmp/uniform19.bench" 2>&1)" && code=0 || code=$?
[ "$code" -eq 1 ] || fail "uniform +19% slowdown: expected exit 1, got $code"
printf '%s\n' "$out" | grep -q "^geomean: +19.00% slower than baseline" ||
	fail "uniform +19% slowdown: expected a geomean regression line in the output"

# Case 2 (control): identical code, must stay clean — proves the geomean
# check does not fire on its own noise floor.
gen_bench "$tmp/clean.bench" 1.0
out="$(./scripts/check-bench-regression.sh "$tmp/base.bench" "$tmp/clean.bench" 2>&1)" && code=0 || code=$?
[ "$code" -eq 0 ] || fail "identical code: expected exit 0, got $code"

# Case 3 (control): the gate's original purpose, unaffected by this change —
# one benchmark regresses past the per-case threshold, the rest untouched.
{
	echo "goos: linux"
	echo "goarch: amd64"
	echo "pkg: example.com/fixture"
	echo "cpu: Fixture CPU"
	for _ in $(seq 1 10); do printf 'BenchmarkA-8\t1000\t3000 ns/op\n'; done # +200%
	for _ in $(seq 1 10); do printf 'BenchmarkB-8\t1000\t2000 ns/op\n'; done # unchanged
	for _ in $(seq 1 10); do printf 'BenchmarkC-8\t1000\t5000 ns/op\n'; done # unchanged
} >"$tmp/percase.bench"
out="$(./scripts/check-bench-regression.sh "$tmp/base.bench" "$tmp/percase.bench" 2>&1)" && code=0 || code=$?
[ "$code" -eq 1 ] || fail "single-case +200% regression: expected exit 1, got $code"
printf '%s\n' "$out" | grep -q "^A-8: .*slower than baseline (threshold 20%)" ||
	fail "single-case +200% regression: expected a per-case regression line in the output"

if [ "$failed" -eq 0 ]; then
	echo "check-bench-regression_test: PASS"
fi
exit "$failed"
