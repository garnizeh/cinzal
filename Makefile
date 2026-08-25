# Cinzal build entry points.
#
# The CI workflow calls these targets rather than restating the commands, so
# `make check` reproduces a CI failure locally. One definition, two callers.
#
# Every target that needs a tool FAILS when it is missing rather than skipping.
# A check that did not run is indistinguishable from one that passed, and that
# is the failure this milestone exists to prevent — see the M0 section of
# docs/project/cinzal-implementation-plan.md.

GO      ?= go
ALL     := ./...
CMD     := ./cmd/...
BIN     := bin

# bash, with pipefail, rather than make's default /bin/sh: bench-baseline and
# bench-compare pipe `go test` into `tee` below, and without pipefail that
# pipeline's exit status is tee's, so a failing benchmark run would still
# report success — the same failure CI's own bench step already guards
# against explicitly (see .github/workflows/ci.yml), just reached here via
# make's default shell instead of the workflow's.
SHELL := bash
.SHELLFLAGS := -o pipefail -c

.DEFAULT_GOAL := help
.PHONY: help dev prod test bench bench-baseline bench-compare bench-regression-selftest lint generate generate-check packages purity fog debug-isolation secrets bots-isolation bots-isolation-selftest simulate-deps check replay clean

## help      list these targets
help:
	@printf '\nCinzal — make targets\n\n'
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/^## /  /'
	@printf '\n'

## dev       build with the debug tag — the debug panel exists in this binary
dev:
	@mkdir -p $(BIN)/dev
	$(GO) build -tags debug -o $(BIN)/dev/ $(CMD)

## prod      build without it — debug routes do not exist in this binary
prod:
	@mkdir -p $(BIN)/prod
	$(GO) build -o $(BIN)/prod/ $(CMD)

## test      race-enabled tests
test:
	$(GO) test -race $(ALL)

## bench     run the benchmark suite (issue #112) and print results
#
# Prints to stdout only — nothing here writes a file, and a single sample
# is not enough for a real before/after comparison. Use bench-baseline and
# bench-compare below for that.
#
# -run '^$' skips every Test function so only Benchmark* runs.
bench:
	$(GO) test -run '^$$' -bench . -benchmem $(ALL)

# Repeated samples are what makes bench-compare's comparison statistically
# meaningful rather than a bare percentage diff against one prior number —
# exactly what issue #113's acceptance criteria rules out, because a shared
# CI runner is noisy enough to make that read as a regression when nothing
# regressed. 10 matches benchstat's own commonly recommended minimum, and
# was verified against this suite specifically: repeated comparisons of
# identical code at this count produced no false positive, where a shorter
# -benchtime alone did (see check-bench-regression.sh's header).
BENCH_COUNT ?= 10
BASELINE    ?= baseline.bench

## bench-baseline  record BENCH_COUNT samples of the suite to baseline.bench
#
# Three callers: CI's bench job, recording main's history as an artifact on
# every push (issue #112); CI's bench-compare job, measuring a pull request's
# base commit before measuring its head on the same runner (issue #125); and
# a developer locally, before making a change they want to measure.
# `*.bench` is gitignored (see .gitignore) — this is recorded CI history or
# a personal local aid, never something to commit.
bench-baseline:
	$(GO) test -run '^$$' -bench . -benchmem -count=$(BENCH_COUNT) $(ALL) | tee "$(BASELINE)"

## bench-compare  compare BENCH_COUNT fresh samples against BASELINE (issue #113)
#
# Advisory only, deliberately not part of `check` — see
# scripts/check-bench-regression.sh's own header for the reasoning, and
# CONTRIBUTING.md "What is deliberately not a gate". Locally:
#   make bench-baseline
#   ...make a change...
#   make bench-compare
# In CI, BASELINE is base.bench: the pull request's own base commit, checked
# out and benchmarked by the bench-compare workflow job on the same runner
# immediately before this target runs (see .github/workflows/ci.yml, and
# issue #125 for why it must be the same runner) — everything after that
# checkout is this one target, so a flagged regression reproduces locally
# with the same command.
bench-compare: require-benchstat
	$(GO) test -run '^$$' -bench . -benchmem -count=$(BENCH_COUNT) $(ALL) | tee candidate.bench
	./scripts/check-bench-regression.sh $(BASELINE) candidate.bench

## bench-regression-selftest  fixture coverage for check-bench-regression.sh (issue #150)
#
# Deterministic and fast — fixed synthetic .bench data, no `go test -bench`
# involved — so unlike bench-compare it carries none of the real-benchmark
# noise that keeps bench-compare advisory. See
# scripts/check-bench-regression_test.sh's own header.
bench-regression-selftest: require-benchstat
	./scripts/check-bench-regression_test.sh

## lint      go vet and golangci-lint
lint: require-golangci-lint
	$(GO) vet $(ALL)
	golangci-lint run

## generate  templ and sqlc — no-ops until M5 and M3 respectively
generate: require-templ require-sqlc
	templ generate
	@if [ -f sqlc.yaml ]; then \
		sqlc generate; \
	else \
		printf 'make: no sqlc.yaml yet — nothing to generate (arrives with M3)\n'; \
	fi

## packages  assert the package graph matches scripts/packages.txt
packages:
	./scripts/check-packages.sh

## purity    assert internal/rules, internal/telemetry, internal/bots do no I/O, read no clock, draw no randomness
purity:
	./scripts/check-rules-purity.sh

## fog       assert render and web never DIRECTLY import internal/rules
fog:
	./scripts/check-fog-boundary.sh

## debug-isolation  assert debug tooling cannot reach a production binary
debug-isolation:
	./scripts/check-debug-isolation.sh

## secrets   scan the tree, and the commits this change adds, for credentials
#
# The history scan is scoped to a commit range rather than every ref the clone
# can see - #37, where a credential on one branch failed the gate on three
# unrelated pull requests. CI sets CINZAL_SECRETS_RANGE to the range the pull
# request adds; unset, the script derives it from origin/main. The reasoning,
# and the fail-open it closes, are in the script.
secrets: require-gitleaks
	./scripts/check-secrets.sh

## bots-isolation  assert internal/bots names no MatchState, graph, or seed
bots-isolation:
	$(GO) run scripts/check-bots-isolation.go

## bots-isolation-selftest  fixture coverage for check-bots-isolation.go (issue #195)
#
# Deterministic and fast — synthetic fixture packages, nothing about the
# real internal/bots involved — so like bench-regression-selftest this
# carries none of the noise that would keep it out of `check`. See
# scripts/check-bots-isolation_test.sh's own comment.
bots-isolation-selftest:
	./scripts/check-bots-isolation_test.sh

## simulate-deps  assert cmd/simulate depends on only rules/bots/game/telemetry
simulate-deps:
	./scripts/check-simulate-deps.sh

# Paths holding generated output. EMPTY UNTIL M3 AND M5 — templ output arrives
# with the templates, sqlc output with the schema. Each milestone appends here.
GENERATED :=

## generate-check  assert the committed generated code matches what the tools produce
generate-check: generate
	@if [ -z "$(GENERATED)" ]; then \
		printf 'generate-check: no generated paths declared yet — nothing to compare (M3, M5)\n' >&2; \
		printf '                this check is VACUOUS, not passing. See the GENERATED variable.\n' >&2; \
		exit 1; \
	else \
		dirty=$$(git status --porcelain -- $(GENERATED)) || { \
			printf 'generate-check: git could not inspect the generated paths.\n' >&2; \
			printf '                failing rather than reporting OK on an inspection that did not happen.\n' >&2; \
			exit 1; \
		}; \
		if [ -n "$$dirty" ]; then \
			printf 'generate-check: committed generated code does not match the tools output.\n' >&2; \
			printf '                run `make generate` and commit the result.\n\n' >&2; \
			printf '%s\n' "$$dirty" >&2; \
			exit 1; \
		fi; \
		printf 'generate-check: OK — %s unchanged\n' '$(GENERATED)'; \
	fi

## check     everything CI runs — reproduce a CI failure locally with this
#
# EVERY GATE ADDS ITSELF HERE. The four gates of M0 do not exist yet and are
# deliberately absent rather than stubbed: a target that cannot run must not be
# listed as if it had. As each lands it appends itself to this line, and its
# All four M0 gates are now listed. Anything added later appends itself here.
#
# bots-isolation joined in M2 (issue #195), the same shape as the four M0
# gates before it: it can always run once internal/bots exists, so unlike
# generate-check it belongs on this line rather than staying out until
# something makes it non-vacuous. simulate-deps joined the same way, in the
# same milestone, once cmd/simulate held a real driver to check (issue
# #199) — a plain go list/grep, so unlike bots-isolation it needs no
# selftest of its own (check-packages.sh, the same shape, has none either).
#
# generate-check is deliberately absent from this line for the same reason,
# not an oversight: with GENERATED still empty (M3/M5 not landed), it can only
# report VACUOUS, and listing it here would make `check`'s own success mean
# "every gate that could run, passed" instead of "every gate passed" — exactly
# the failure this file's own header warns against. It rejoins this line once
# GENERATED holds real paths.
#
# bench-compare is deliberately absent from this line too, but for the
# opposite reason: it can run, and deciding it should still not block is the
# point of issue #113 — see bench-compare's own comment and
# CONTRIBUTING.md "What is deliberately not a gate". bench-regression-selftest
# is not the same script and carries none of that noise — see its own
# comment above — so it is listed here rather than kept out alongside it.
#
# If this line and the CI workflow ever disagree, the workflow is wrong: it
# calls these targets rather than restating them, so there is one definition.
check: packages purity fog debug-isolation secrets bots-isolation bots-isolation-selftest simulate-deps lint test bench-regression-selftest prod dev

## replay    golden-replay determinism suite, for the cross-OS/arch matrix (issue #80)
#
# The same tests `make test` already runs on ubuntu-latest as part of
# `check` — this target exists to be invoked a SECOND time, from a second OS
# and architecture, by CI's replay job (.github/workflows/ci.yml). Not
# folded into `check`'s own aggregate line: doing so would just repeat the
# identical run on the identical runner `test` already covers there,
# mirroring generate-check/bench-compare's own reasons for staying out of
# that line (see their comments above).
replay:
	$(GO) test -race ./internal/rules/...

## clean     remove build output
clean:
	rm -rf $(BIN)

# ---------------------------------------------------------------------------
# Tool presence. A missing tool is a failure, never a skip.
#
# `dev`, `prod`, `test` and `packages` need only the Go toolchain, and the `go`
# directive in go.mod already refuses a toolchain older than the project needs.
# ---------------------------------------------------------------------------

require-%:
	@command -v '$*' >/dev/null 2>&1 || { \
		printf '\nmake: %s is required and is not on PATH.\n\n' '$*' >&2; \
		printf '  This fails rather than skipping on purpose: a check that did not\n' >&2; \
		printf '  run looks exactly like one that passed.\n\n' >&2; \
		printf '  See the local development section of CONTRIBUTING.md.\n\n' >&2; \
		exit 1; }
