---
name: test-authoring
description: Write or deepen Cinzal Go tests — fog negatives, RNG index accounting, golden replays, anchor parity, cross-round state, property and adversarial tests, Postgres-backed store tests. Use when asked for tests, coverage, "criar testes", or when a change touches a layer whose guarantee is only checkable by a test.
---

# Test authoring

Coverage here is not a percentage. The suite exists because several of this
project's guarantees fail **silently**: a fog leak ships without crashing, a
determinism break surfaces weeks later on one machine. A test earns its place by
being the only thing that would notice.

---

## The layers (RFC §16.1)

Decide which layer a new test belongs to before writing it:

| Layer | Asserts |
|---|---|
| **Golden replay** | A recorded `seed + order log` reproduces byte-identically |
| **RNG index accounting** | Draw count per round matches the RFC §6.4 table exactly — including truncation and zero-draw cases |
| **Fog negatives** | A hidden fact is **absent** from `PlayerView`, not merely unused |
| **Anchor parity** | Projected anchors match what the fog rules permit |
| **Cross-round state** | Carry-over between rounds — cooldowns, leases, flags, cursors |
| **Property / adversarial** | Invariants over generated inputs; deliberately hostile orders |
| **Bot determinism** | A bot-populated match replays byte-identically and its per-round index count does not move — the Autopilot handover case included |
| **Postgres-backed** (D46) | Real database, behind its build tag, failing rather than skipping when there is none |

## What a good test looks like here

**Assert absence, not silence.** A fog test that checks a field is unused proves
nothing. Check the hidden value cannot be reached from the projection at all.

**Assert exact equality where the guarantee is exact.** Byte-identical replays,
exact draw counts, exact event ordering. "Contains" and "roughly" hide the
regressions this suite exists to catch.

**Table-driven, with the case name saying what it protects.** Match the
surrounding style — the existing tests name the loophole, not the input.

**A new randomness consumer needs its index-count assertion in the same PR,
including its truncation case.** An unaccounted draw is a replay divergence with
no obvious cause months later.

**A new `PlayerView` field needs its negative fog test in the same PR.** If it
can disclose a player's position it also needs a row in the RFC §9.1 table.

**A new gate needs fixture coverage for each failure mode.** That is the bar
`bots-isolation-selftest`, `purity-selftest` and `bench-regression-selftest`
already meet: a named type, an inferred local, a bare `[32]byte`, a dot-import,
a `doc.go`-only package (**VACUOUS, not a pass**), a parse error, a missing
allow-list.

## The rule every test inherits

**A test that passes when it did not run is worse than no test.** A skipped
Postgres suite reporting green is the same failure as a review bot reporting
success on a skipped review. Where a dependency is missing, **fail**, do not
skip — and never "fix" a noisy test by letting it skip.

## Constraints

`internal/rules` tests do **no I/O at all** — the package is pure and its tests
stay that way. `internal/bots`'s own tests are the exception to the bots
isolation gate: they legitimately build real matches (`rules.NewMatch`,
`rules.Project`, `rules.Resolve`) to drive bots against realistic corpora, and
test files are out of the gate's scope. Production files are not.

Fixture and golden data changes are load-bearing — if a golden file moves,
explain in the PR *why the old bytes were wrong*, never just that they changed.

## Running

```bash
rtk go test -race ./internal/rules/...          # narrow, while iterating
rtk go test -race -run TestFogNegative ./...    # one case
rtk make test                                    # everything, race-enabled
```

## Then

→ `gates-run` for the full suite, or back to `code-change`.
