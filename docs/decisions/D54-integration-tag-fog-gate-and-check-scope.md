# D54 — How do D46's `//go:build integration` files coexist with the fog gate, and does `integration-list` belong inside `make check`?

**Status:** decided
**Blocks:** the test-harness task ([#325](https://github.com/garnizeh/cinzal/issues/325)) and the CI task ([#316](https://github.com/garnizeh/cinzal/issues/316)) — both build against D46's shape, and both touch the gates below
**Decided:** 2026-08-27
**Issue:** [#351](https://github.com/garnizeh/cinzal/issues/351)

## The question

[D46](D46-postgres-backed-test-layer.md) put `//go:build integration` on every Integration- and Concurrency-layer test file, and put the Docker-free `integration-list` coverage check inside `check-nosecrets`'s aggregate. Both choices collide with something this repository had already decided, in a way D46 did not consider because neither collision existed on the ground it was written against — `internal/store` and `internal/web` were, and still are, `doc.go` only.

**1. The fog gate hard-fails on a build-constrained file it guards, and D46 says `internal/web` grows exactly one.** `scripts/check-fog-boundary.sh` guards `./internal/render/... ./internal/web/...` and, until this decision, refused to inspect any package reporting a non-empty `IgnoredGoFiles`:

> `$pkg has build-constrained files this gate cannot inspect: $ignored`
> `Their imports are invisible to go list under the active build`
> `configuration. Remove the constraint or teach this script to parse`
> `them - do not leave them unchecked.`

D46 states plainly that the tagged set grows into that guarded tree:

> The integration suite's real dependency graph already spans `internal/rules`, `internal/game`, `internal/match` and `internal/store`, and will grow to include `internal/web`/`cmd/server` once M5 builds the HTTP layer §16.1's Integration row describes

§16.1's Integration row is *"full match through the HTTP layer"* — that test file lives in or beside `internal/web`, carries `//go:build integration`, and the first `make check` after it lands would have failed the fog gate with the message above. The gate was right to refuse: a tagged file could import `internal/rules` and no configuration then in use would see it. D46 is right that the tests belong there. Something had to give.

**2. `integration-list` inside `make check` makes the testcontainers-go dependency tree a build input of the default developer loop.** There is no `go.sum` in this repository today. D46 puts `go test -tags integration -list …` into `check-nosecrets`, and `-list` **compiles** the tagged files, which import `storetest`, which imports testcontainers-go and, through it, the Docker client libraries. D46's reasoning for wanting this check to run everywhere is sound and survives unchanged:

> A coverage check that itself needs Docker to run would be exactly as skippable as the thing it's supposed to catch skipping

The Docker *daemon* is genuinely not needed for `-list` — that argument holds. What D46 did not weigh is that the Docker *client library tree* becomes something `go build`/`go list` must resolve and compile for `-list` to run at all, on every contributor's machine, every time `make check` (not just `make integration`) is invoked — a cost CONTRIBUTING.md's Requirements section reads as ruled out:

> No Node, no frontend build step, and no Docker for the rules engine — `internal/rules` is pure and its tests do no I/O at all.

That sentence is scoped to "the rules engine" and stays literally true either way. What breaks is the thing a reader takes from `make check`'s existing aggregate: every other line in `check-nosecrets` (`purity-selftest`, `bots-isolation-selftest`, `bench-regression-selftest`) is there because it is "fast enough to belong on that line, not held out the way `bench-compare` is" (Makefile, `check` target comment) — a criterion `integration-list` does not obviously meet once it needs a first-time module download and a heavier compile, even though it needs no running Docker.

## Why it is open

Both are collisions between two things this repository had already decided on purpose, not oversights in either one. The fog gate's refusal-to-be-blind is the property that makes it trustworthy; D46's build tag is the property that makes the integration suite non-skippable. Neither should be weakened casually, and each has more than one shape that resolves it.

## Options

### Fog gate vs. build-tagged files

- **Remove the `//go:build integration` constraint from files under `internal/web`.** Off the table — it is exactly the property D46 decided the suite needs, for the same reason `internal/store`'s and `internal/match`'s tagged files need it: a build tag, not a `t.Skip`, is what keeps these tests out of `go test ./...` entirely rather than merely reporting "ran and passed" on nothing.
- **Keep `internal/web`'s integration test in a sibling package outside `GUARDED`** (e.g. `internal/webtest`, alongside `internal/store/storetest`). Rejected: this is the same loophole the gate's own header already warns about, reached by a different door. The reason `.TestImports`/`.XTestImports` are checked at all is that *"a render test that can name the match state can build a fixture that non-test code later reads"* — an Integration-layer test is exactly the kind of file with the strongest reason to reach for `internal/rules` directly (constructing match state faster than driving it through the HTTP surface), and moving it outside the glob would make that reach invisible to the one check built to catch it. Sidestepping the gate is not answering it.
- **Move the HTTP-layer integration test to `cmd/server`'s tree.** Rejected for the same reason: `cmd/server` is not `GUARDED` either, so this is the previous option wearing a different path, and it fights the RFC's own placement — §16.1's Integration row is a property of the HTTP layer (`internal/web`), not of the binary that happens to serve it.
- **Teach the gate to inspect every build configuration this repository actually uses, not just the active one.** `go list` can already enumerate a package's imports under any tag set given to `-tags`; running the existing check twice — once under the default configuration, once under `-tags integration` — closes the blind spot without writing a bespoke import parser, and without exempting a single file from inspection. A file gated by a tag not on the list still hard-fails exactly as before, so this is an explicit allow-list of known configurations, not a blanket escape hatch. Chosen.

### `integration-list`'s placement relative to `make check`

- **Leave it inside `check-nosecrets`, as D46 specified.** The property D46 wanted — non-skippable, run on every `make check` — survives, but at the cost identified above: `-list` compiling `storetest` pulls the testcontainers-go/Docker-client tree into every contributor's build, including one editing only `internal/rules`, the package edited most and the one D01 makes leaf-and-dependency-free on purpose.
- **A separate, always-run required CI job, structured the way `secrets` was split out of the aggregate line in [#336](https://github.com/garnizeh/cinzal/issues/336), but still invoked by local `make check`** (the way `check: check-nosecrets secrets` still runs `secrets` locally today). Rejected: `secrets`' split was about a CI path-gating boundary, not about removing a local cost — `secrets` never imposed one to begin with. Copying that shape here would give `integration-list` its own CI job while leaving the actual problem, the local build-time cost, exactly where it was.
- **Out of `check`'s aggregate entirely — the `generate-check`/`bench-compare` shape — with its own required CI job that local `make check` does not invoke.** Costs the same thing those two already cost: a contributor has to know a second command (`make integration-list`) exists and run it deliberately, or trust CI to catch a regression there. Chosen — see Decision.

## Decision

**Fog gate: `check-fog-boundary.sh` now inspects `GUARDED` under an explicit list of build configurations, `BUILD_TAG_SETS=("" "integration")`, not only the active one.** For each entry, it re-runs the existing `go list`-based `IgnoredGoFiles` refusal and the existing `Imports`/`TestImports`/`XTestImports` scan under that tag set. A file gated by any tag not named in `BUILD_TAG_SETS` still hard-fails exactly as before — this list is grown deliberately, the same discipline `FORBIDDEN` already holds contributors to, never widened to "whatever tags happen to be in play." Applied now, ahead of `internal/web` growing a tagged file, the same way D49 pre-declared `internal/match/fold` in `FORBIDDEN` before the package existed: confirmed inert today (`make fog` reports OK, `internal/web` has no tagged files yet), and it means #325's package layout for the eventual HTTP-layer integration test is chosen with the gate's behaviour already known, not guessed at under a red build in M5.

**`make check` scope: `integration-list` is not folded into `check-nosecrets`.** When #316/#325 land the Makefile targets D46 specified, `integration` (Docker-required, hard failure on an unreachable daemon) and `integration-list` (Docker-free, `-list`-only) both stay out of `check`'s aggregate line, in their own required CI job, path-gated the same broad list `check`/`replay`/the future `integration` job already share. This keeps every property D46 wanted from `integration-list` — required in CI, no `t.Skip` escape hatch, catches the suite silently shrinking via `scripts/check-integration-coverage.sh`'s floor — while keeping `make check` itself exactly what CONTRIBUTING.md already promises: no Docker, and now, no testcontainers-go compile either, for the default developer loop, even after M3.

**CONTRIBUTING.md's Requirements section gains a clause, landed in this PR,** stating that `make integration`/`make integration-list` exist from M3 onward, that the former needs Docker and the latter needs network access to fetch the testcontainers-go dependency tree the first time it runs, and that neither is part of `make check` for exactly that reason — the existing "no Docker for the rules engine" sentence is reaffirmed for `make check` as a whole, not merely left technically true.

## Reasoning

**The fog gate's own header already named the right fix.** "Remove the constraint or teach this script to parse them" poses two options; D46 already foreclosed the first for these specific files, so the question was only ever how to parse them without hand-rolling an import parser (`check-game-types.go` exists precisely because *that* kind of parsing — a type expression versus an identically spelled word in a comment — cannot be done any other way; import statements have no such ambiguity, and `go list` already resolves them correctly per build configuration). Re-running the identical, already-trusted mechanism under a second configuration costs one loop and zero new trust surface — the check that a violation gets caught is the same `go list -f '{{join .Imports ...}}'` line the gate has always run, asked twice instead of once.

**Sidestepping the glob is not a smaller version of the same fix — it is the absence of one.** The two rejected fog-gate options both keep the file compiling under a tag while moving *where* it lives, and the gate's own reason for reading `.TestImports`/`.XTestImports` at all — a test file building a fixture that leaks past the boundary — applies with undiminished force to a file that happens to sit one directory over. A gate that can be satisfied by choosing where to put a file was never really checking imports.

**The `integration-list` placement question turns on what "fast enough" already means on that Makefile line, not on whether Docker is reachable.** D46's stated criterion — no Docker — is necessary but was not, on inspection, sufficient: `purity-selftest`, `bots-isolation-selftest`, and `bench-regression-selftest` all joined `check-nosecrets` under an explicit "deterministic and fast enough" test, stated inline in the Makefile, not under "needs no Docker" alone. Compiling a package that imports the Docker client libraries — even without invoking them — is a first-time network fetch and a heavier build, which is a different cost shape than any existing line in that aggregate pays. Moving it out costs exactly what `generate-check` and `bench-compare` already cost the project: a second command a contributor must know about, or trust CI to run — a cost this repository has already decided twice is worth paying to keep the default loop cheap.

## Consequences

- `scripts/check-fog-boundary.sh`: `BUILD_TAG_SETS` added, both the `IgnoredGoFiles` refusal and the `Imports`/`TestImports`/`XTestImports` scan now run once per entry. `GUARDED`, `FORBIDDEN`, and the violation-reporting shape are otherwise unchanged. Landed in this PR; confirmed inert (`make fog` OK, 4 package/tag pairs today — 2 guarded packages × 2 configurations — versus 2 before).
- [D46](D46-postgres-backed-test-layer.md)'s **Status** line and its "Fail-closed guard on the guard" and **Consequences** paragraphs are corrected in place: `integration-list` stays out of `check`'s aggregate, never joins `check-nosecrets`, and gets its own required CI job instead — see the `[Corrected by D54]` markers there. Nothing else about D46 — the testcontainers mechanism, the build tag, the three isolation tiers, the Docker-required `make integration` target itself, or `check-integration-coverage.sh`'s floor logic — changes.
- CONTRIBUTING.md's Requirements section gains the clause described above, landed in this PR ahead of the Makefile targets it describes, the same way the rest of that section already documents `templ`/`sqlc` as "no-ops until M5 and M3 respectively."
- No RFC/GDD change: this decision is CI/tooling mechanics under an already RFC-authorized test layer (§16.1), not a game rule or an architectural boundary.
- When #316/#325 implement D46's Makefile targets, `integration-list` must **not** be added to `check-nosecrets`'s target line — it joins a new, separate, required CI job instead, reusing `check`/`replay`'s existing broad path-gate list per D46's own placement reasoning for `integration` itself.
- Reversible at negligible cost: both changes are CI/tooling shape, not fixture or schema. Folding `integration-list` back into `check-nosecrets` later, if the build-time cost turns out not to matter in practice, is a one-line Makefile edit with no data migration.
