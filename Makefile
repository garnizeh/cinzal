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

.DEFAULT_GOAL := help
.PHONY: help dev prod test lint generate generate-check packages purity fog check clean

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

## purity    assert internal/rules does no I/O, reads no clock, draws no randomness
purity:
	./scripts/check-rules-purity.sh

## fog       assert render and web never DIRECTLY import internal/rules
fog:
	./scripts/check-fog-boundary.sh

# Paths holding generated output. EMPTY UNTIL M3 AND M5 — templ output arrives
# with the templates, sqlc output with the schema. Each milestone appends here.
GENERATED :=

## generate-check  assert the committed generated code matches what the tools produce
generate-check: generate
	@if [ -z "$(GENERATED)" ]; then \
		printf 'generate-check: no generated paths declared yet — nothing to compare (M3, M5)\n'; \
		printf '                this check is VACUOUS, not passing. See the GENERATED variable.\n'; \
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
# issue carries that as an acceptance criterion — #11 debug isolation and
# #12 gitleaks remain.
#
# If this line and the CI workflow ever disagree, the workflow is wrong: it
# calls these targets rather than restating them, so there is one definition.
check: packages purity fog lint test prod dev generate-check

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
