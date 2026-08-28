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
bots-isolation-selftest simulate-deps lint test bench-regression-selftest
prod dev
```

---

## The governing principle

**Every check here fails closed.** Missing tool, empty `go list` output,
unreadable config — it reports **failure**, not a skip. A gate built the obvious
way reports green having inspected zero packages, which is the same failure as a
review bot reporting success on a skipped review.

**A gate that passes when it cannot run is worse than no gate.** Hold new checks
to this, and never "fix" a noisy gate by letting it skip.

`generate-check` is deliberately **not** in `check` yet: `GENERATED` is empty
until M3/M5 land real generated paths, and it reports **VACUOUS, not passing**.
Run it standalone to see that message.

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
| `lint` | `go vet` + `golangci-lint` | — |
| `test` | `go test -race ./...` | — |
| `*-selftest` | The gates' own fixture coverage | The gate stopped catching what it exists to catch |
| `prod` / `dev` | Both build tags compile | — |

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
```

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
