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
.PHONY: help dev prod test lint generate packages check clean

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

## check     everything CI runs — reproduce a CI failure locally with this
check: packages lint test prod dev

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
