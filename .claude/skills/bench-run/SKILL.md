---
name: bench-run
description: Run and interpret Cinzal benchmarks — make bench, bench-baseline, bench-compare, benchstat, and the 20%/10% regression thresholds. Use when a change touches internal/rules/gen or hot resolution paths, when bench-compare goes red on a PR, or when the user asks for benchmarks or performance numbers.
---

# Benchmark run

Performance here is a **gate**, not a curiosity. `bench-compare` is required —
but shaped differently from the other checks: it runs only on a PR touching
`internal/rules/gen`, `.github/workflows/ci.yml`,
`.github/actions/changed-paths/action.yml`,
`scripts/check-bench-regression.sh` or the `Makefile`. Most PRs skip it rather
than pass it.

---

## The commands

```bash
rtk make bench                 # one sample of the suite, stdout only — not a comparison
rtk make bench-baseline        # BENCH_COUNT samples into baseline.bench
# ...make the change...
rtk make bench-compare         # fresh samples vs BASELINE, through check-bench-regression.sh
```

`BENCH_COUNT` defaults to **10** — benchstat's commonly recommended minimum, and
verified against this suite specifically: repeated comparisons of identical code
at this count produced no false positive, where a shorter `-benchtime` alone did.
Do not lower it to save time; that is the setting the gate's credibility rests on.

`*.bench` is gitignored. It is CI history or a personal local aid, never
something to commit.

## The thresholds

**20% per case, 10% geomean.** Past either, the gate fails for real.

It started advisory (#113) because two data points cannot characterise CI-runner
noise against those thresholds. It was promoted to required in #127 once seven
real same-runner comparisons had landed with zero false positives — worst single
case +6.27%, worst geomean 0.93%, both comfortably inside.

In CI, `BASELINE` is `base.bench`: the PR's own base commit, checked out and
benchmarked **on the same runner** immediately before the head is (#125 is why
the same runner is non-negotiable). Everything after that checkout is the one
`make bench-compare` target, so a flagged regression reproduces locally with the
same command.

## Reading a red bench-compare

Work through these in order before concluding a real regression:

1. **Did the benchmark's own inputs change?** A known false-positive shape on
   this repo: editing `MapByPlayers` moves the topology the benchmark runs
   against, so the numbers move for reasons unrelated to the code path. **Pin
   the benchmark topology** rather than accepting the drift or loosening the
   threshold.
2. **Is it one case or the geomean?** A single case past 20% with a flat geomean
   points at that case's input; a moved geomean points at a shared path.
3. **Reproduce locally** with `bench-baseline` on the base commit and
   `bench-compare` on the head — same machine, same `BENCH_COUNT`.
4. **Only then** is it a real regression. Fix the code, or state the regression
   and its justification explicitly in the PR body so the commit on `main`
   records it.

Never widen a threshold to make a PR green.

## The selftest

`bench-regression-selftest` **is** part of `make check`: fixture coverage for
`check-bench-regression.sh` against fixed synthetic `.bench` data, no real
`go test -bench` involved. It is deterministic and fast, so unlike
`bench-compare` it carries none of the real-benchmark noise. A change to the
gate script needs its fixture case.

## M3's own performance obligation

Fold duration and fold allocation share are wired **from day one** — they are the
falsifiability trigger for the no-snapshot decision (RFC §7.3) and are worthless
added later. Thresholds: **p99 fold duration 50 ms**, **fold allocation share 20%
of heap churn**. Per D51 the allocation-share denominator is process-wide, so M3
reports **two labeled bounds**, not one compared number — do not collapse them.

## Then

→ back to `code-change` with a fix, or `gates-run` / `delivery-review` if clean.
