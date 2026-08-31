---
name: gates-run
description: Run and interpret Cinzal's verification suite — make check, make test, lint, and the individual CI gates (packages, purity, fog, debug-isolation, secrets, vuln, bots-isolation, simulate-deps). Use to verify a change before review, to reproduce a red CI job locally, or when the user asks to test, run the checks, or "roda o make check".
---

# Gates run

`make check` runs **exactly what CI runs** — the workflow calls these targets
rather than restating the commands, so a CI failure reproduces locally with one
command and there is one definition to keep correct rather than two that drift.

```bash
rtk make check
```

which is `check-nosecrets` + `secrets` + `vuln`, where `check-nosecrets` is:

```
packages purity purity-selftest fog debug-isolation bots-isolation
bots-isolation-selftest simulate-deps generate-check generate-check-selftest
lint test bench-regression-selftest prod dev
```

---

## Normalize before starting: `go fmt` / `go fix`

Run both before `make check` and before any review pass, not after — a diff
still carrying gofmt drift or a pre-modernization idiom is noise none of the
gates below are meant to catch, and it's cheaper to normalize once, up front,
than to have a reviewer flag it as a finding.

```bash
rtk go fmt ./...
rtk go fix ./...
```

**`go fix` is a real rewrite, not a formatter — its output is a diff to read,
never a step to trust blind.** It has already produced a file that failed to
compile on this repo: a `T{...}; x.Field = v` merge that folded the
assignment into the struct literal but left the original field in place too,
so the literal ended up with the same field named twice — caught only by the
`lint`/`test` gates this step is supposed to precede, not by `go fix` itself.
After running it, re-run `go build ./...` and `go vet ./...` (or just go
straight to `make check`) before treating the tree as clean, and read
`git diff` for anything it touched that you did not expect.

## The governing principle

**Every check here fails closed.** Missing tool, empty `go list` output,
unreadable config — it reports **failure**, not a skip. A gate built the obvious
way reports green having inspected zero packages, which is the same failure as a
review bot reporting success on a skipped review.

**A gate that passes when it cannot run is worse than no gate.** Hold new checks
to this, and never "fix" a noisy gate by letting it skip.

`generate-check` joined `check` in issue #316: `GENERATED` now names sqlc's
real output (issue #315), and emptying it back out still fails rather than
passing — the VACUOUS branch stays reachable, proven by
`scripts/check-generate_test.sh` (`generate-check-selftest`). Its still-empty
templ half (`GENERATED_TEMPL`) stays that way until M5, documented inline at
the Makefile's own definition — not a reason for the whole check to report
VACUOUS again.

## What each gate asserts, and what a failure means

| Gate | Asserts | A failure means |
|---|---|---|
| `packages` | The package graph matches `scripts/packages.txt` | A new package, or an import that changes the graph — declare it deliberately |
| `purity` | `rules`, `telemetry`, `bots` do no I/O, tell no time, draw no randomness — indirect references and `fmt` call sites included | The change reached for the outside world from a pure package |
| `fog` | `render`/`web` never directly import `internal/rules` | The rendering layer can name the full match state |
| `debug-isolation` | The production binary contains no debug routes | Debug tooling escaped its `//go:build debug` tag |
| `secrets` | No credentials in the tree **or in the commits this branch adds** | Scoped to `origin/main..HEAD`; over-scans rather than under-scans |
| `vuln` | No `go.mod`/`go.sum` dependency carries a known OSV/GHSA vulnerability (osv-scanner) or a reachable one govulncheck's symbol data can confirm | A newly (or newly-disclosed) vulnerable dependency — bump or replace it, never suppress the finding |
| `bots-isolation` | `internal/bots` production code names no `MatchState`, graph, or `[32]byte` seed, and no `rules.X` outside the allow-list | Widening `scripts/bots-isolation-allowlist.txt` is its own reviewed change with a fog-safety argument |
| `simulate-deps` | `cmd/simulate` depends on only `rules`/`bots`/`game`/`telemetry` | `internal/match` (or anything else) leaked into the driver |
| `generate-check` | Committed generated code (`GENERATED`, currently sqlc's output) matches a fresh regenerate | Regenerating changed something not committed — run `make generate` and commit the result |
| `lint` | `go vet` + `golangci-lint` | — |
| `test` | `go test -race ./...` | — |
| `*-selftest` | The gates' own fixture coverage | The gate stopped catching what it exists to catch |
| `prod` / `dev` | Both build tags compile | — |
| `replay` | `internal/rules` behaves identically on Linux and macOS — its own CI job with a cross-OS matrix, not part of `make check` | A determinism break that only shows on one OS, which is the shape `seed + order log` guarantees against |

**What blocks a merge is a checkable list, not the set of jobs that ran.**
Branch protection requires six status checks — `check`, `secrets`,
`bench-compare`, `vuln`, `replay (ubuntu-latest)`, `replay (macos-latest)` —
and there are no rulesets. `vuln` and both `replay` legs were promoted in
#391, having run on every PR while blocking nothing. `bench` stays out: it is
the one job with a job-level `if` (`push` only), so it reports `skipped` on
every PR and requiring it would deadlock all of them.

```bash
rtk gh api repos/garnizeh/cinzal/branches/main/protection --jq .required_status_checks.contexts
```

**If a gate blocks you, the answer is almost never to weaken the gate.** These
are not style checks and a failure is not a nit. Each exists because the failure
it prevents is invisible and expensive.

Style linters deliberately stay out of the required set — `markdownlint` does
not run in CI (decided in #23). If a required check can fail on a cosmetic nit,
contributors learn a red check might be nothing.

## Narrower runs while iterating

```bash
rtk make test                     # go test -race ./...
rtk make lint                     # go vet + golangci-lint
rtk make fog                      # one gate
rtk make purity
rtk make vuln
rtk make bots-isolation
rtk go test -race ./internal/rules/...
rtk make dev                      # debug build — the debug panel exists here
rtk make prod                     # debug routes do not exist in this binary
rtk make replay                   # what CI's cross-OS matrix runs, on this OS
```

**Two more targets arrive with M3 and are deliberately not in `make check`**
([D46](../../../docs/decisions/D46-postgres-backed-test-layer.md),
[D54](../../../docs/decisions/D54-integration-tag-fog-gate-and-check-scope.md)):
`make integration`, the Postgres-backed suite behind `//go:build integration`,
which needs a reachable Docker daemon and hard-fails without one; and `make
integration-list`, its Docker-free companion asserting that suite hasn't
silently shrunk, which runs as its own CI job. Neither exists yet — #325 builds
them. When they land, `make check` still gains no Docker dependency, and that
is the point of the split, not an oversight.

Run the full `make check` once before opening the PR, not only the narrow ones.

## When to skip

**A docs-only change gets no signal from `make check`** — the gates are
content-agnostic, and CI's own `check` job skips itself for a diff that cannot
touch `internal/`, `cmd/`, `scripts/`, `go.mod`/`go.sum`, `Makefile`,
`.golangci.yml` or the CI definitions. Skip it and **say so in the PR test
plan**, with what you verified instead.

Neither the secret scan nor `vuln` participates in that skip, on purpose —
`secrets` scans docs too, and `vuln` is checking a fact about time (has a CVE
been disclosed against a dependency already in `go.sum`?), not about what this
diff touches; both run in their own always-on CI jobs regardless of what a
diff touches. The `check`/`replay` path check **fails open**: an unresolvable
diff runs everything.

## Reproducing a red CI job

CI calls these same targets, so start by running the failing target's name
locally. If it passes locally and fails in CI, the difference is almost always
scope (`secrets` uses `CINZAL_SECRETS_RANGE`) or the Go version — the floor is
**Go 1.27.0**, enforced in CI because `go.mod` can only express a floor.

## Then

- A regression in `internal/rules/gen` → `bench-run`
- Green → `delivery-review`
