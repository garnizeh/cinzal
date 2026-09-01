# Contributing to Cinzal

This is the working agreement. It is short on ceremony and specific about the few things that are genuinely load-bearing in this project.

**There is no code yet.** The repository currently holds the specification and the roadmap. If you are here to build, start at [the implementation roadmap](docs/project/cinzal-implementation-plan.md) — it says what is being built, in what order, and which decisions are still open.

---

## Before anything else: two things you need to know

### 1. The documents are the authority

| Document | Authoritative on |
|---|---|
| [`cinzal-gdd.md`](docs/project/cinzal-gdd.md) | Game rules |
| [`cinzal-architecture-rfc.md`](docs/project/cinzal-architecture-rfc.md) | Architecture |
| [`cinzal-implementation-plan.md`](docs/project/cinzal-implementation-plan.md) | Sequencing and open decisions |

Both specs are heavily changelogged, and the changelog entries record *why* — which is usually the part that matters. **Read the changelog before assuming a section is current**: later entries correct earlier ones, and several early designs were deliberately cut.

If a rule looks odd, it is probably closing a loophole found during design review, and the changelog says which one.

### 2. Fog is private, and it decides almost everything

The client must never hold the full match state — only what a player's fog-of-war entitles them to see. This is a rule of the game, not a UX preference, and it is why the architecture looks the way it does: one projection function, a package graph that makes the state unnameable in the rendering layer, server-rendered HTML instead of a JSON API, and a test suite that asserts hidden facts are **absent** rather than merely unused.

Before writing anything, ask: *does this leak state past the fog boundary?*

---

## How work is organised

Three kinds of work item. They are different things and are tracked differently.

### Decision

Produces a **written answer**, not code. The open ones are catalogued as `D1`–`D22` in [roadmap §3](docs/project/cinzal-implementation-plan.md); more will appear.

A decision is closed by a document in [`docs/decisions/`](docs/decisions/) recording the question, the options with their tradeoffs, the choice, and the reasoning. **Decisions block the tasks that depend on them** — a milestone does not start with open blockers.

### Task

Produces code or documentation. **One task is one pull request is one commit on `main`.**

That is not a style preference. `main` is squash-only with linear history, so every commit is a reviewed unit that passed CI. When the determinism check fires — and RFC §6.3 says it will, weeks later, intermittently, on one machine — the diagnostic tool is `git bisect`, and bisect is only useful if each commit is coherent and buildable on its own.

It gives a concrete sizing test: **can this land as one pull request that leaves `main` green by itself?** If not, split it. If it is three lines, fold it into its neighbour.

### Exit demonstration

Proves a milestone met its exit criteria. This is deliberately **not** a task, because closing every task in a milestone is not the same as meeting its exit criteria.

M0's criteria, for example, are demonstrations that things *fail*: a pull request adding `import "time"` to `internal/rules` must be **rejected** by CI. That can only be shown by breaking it on purpose. If the demonstration is not its own tracked item, nobody performs it, and the milestone closes with gates that were never tested against what they exist to stop.

---

## What every issue must carry

| Field | Why |
|---|---|
| **Spec anchor** — the section that governs the work | The most important one. The documents are the authority, so a task that cannot cite a section is inventing a requirement. When that happens it is **not a task, it is a decision** — file it as one. Normally this is a GDD or RFC section. **A harness or process fix is not exempt from the field** — its anchor is a harness document instead, because that is what governs it: [#373](https://github.com/garnizeh/cinzal/issues/373) cited this file's "Gates fail closed", [#391](https://github.com/garnizeh/cinzal/issues/391) cited [`.claude/WORKFLOW.md`](.claude/WORKFLOW.md)'s Stage 4. The field stays required either way; what changes is which document it points at. |
| **Acceptance criterion** — demonstrable | "Implement `Resolve`" is not one. "A 15-round replay reproduces byte-identically on two machines" is. |
| **Blocked by** | Decisions and predecessor tasks, explicitly. |
| **Area** | Mirrors [`CODEOWNERS`](.github/CODEOWNERS): `rules`, `fog`, `ci`, `store`, `web`, `render`, `bots`, `mail`, `auth`, `docs`. |

Issue templates carry these fields. Use them.

---

## Pull requests

### The workflow

1. Branch from `main`. Naming is a convenience, not a gate — `task/`, `decision/`, `exit/`, `fix/`, `docs/`, `chore/` prefixes are what is in use.
2. Open the pull request and link the issue it closes.
3. CI runs. CodeRabbit reviews — **but check that it actually did**, see below.
4. **Resolve every review conversation.** `main` requires it, and this is what caught a factual error in the very first pull request of the project.
5. Squash-merge. The branch deletes itself.

Direct pushes to `main` are blocked, force-pushes and deletions are blocked, and the rules apply to the maintainer too.

### A green CodeRabbit check does not mean it reviewed

CodeRabbit runs on the free OSS tier, which has short per-developer limits. It will often reply **"Review limit reached"** and skip a review attempt — **and its status check reports success anyway.** Green on a skipped review is indistinguishable from green on a clean one.

On this project that has been the common case, not the exception. So:

- **Read the comments, not the check.** The check tells you nothing about whether a review happened.
- **"Limit reached" means *that attempt* was skipped, not that the pull request is unreviewed.** Both happen. A pull request can carry a full review of its first commit and then hit the limit on the incremental review of a later one — so look at whether a review exists and *which commits it covered*, rather than reading the most recent message as a verdict on the whole pull request.
- **A clean incremental review leaves no review record at all.** When you push a fix and CodeRabbit finds nothing new, it posts no new review. Checking for a review object on your latest commit and finding none proves nothing.
- **The `✅ Addressed` marker is not guaranteed either.** It usually appears on the finding comment when a fix lands, and when it does it is good confirmation — but a finding can be reviewed on a later commit, not re-raised, and still never marked. Its absence proves nothing, exactly like the absence of a review record.

  If you match on it, note that **the wording varies with how many commits the fix took**: `Addressed in commit <sha>` for one, `Addressed in commits <sha> to <sha>` for several. A pattern written for either form alone silently misses the other and reads it as "unaddressed" — the mistake this paragraph exists to prevent, made twice here while checking for it, once in each direction. Match `Addressed in commits? …`.
- **Retry after the refill time the message reports**, with a `@coderabbitai review` comment. It is usually 20 to 35 minutes. When the message gives no time at all, the free allowance is exhausted for longer and there is nothing to do but wait.
- **CodeRabbit reviews incrementally and will not re-review a commit it has seen.** So `@coderabbitai review` replying **"Already reviewed"** is an answer, not a refusal — the review ran. Its own message notes the manual command "is applicable only when automatic reviews are paused", so when a review genuinely was missed, the way to get one is a new commit after the allowance returns, not repeating the command.
- **Merge only once a real review has landed.** If waiting is genuinely not an option, say so in the pull request description, so the commit message on `main` records what went in unreviewed rather than implying it did not.

**In short — there is exactly one reliable signal, and it is negative: a finding still raised against the current head.** That means *not addressed*. The status check, the presence of a review, the absence of a limit message and the `Addressed` marker are all, on their own, inconclusive.

So after pushing a fix: **check whether the finding is still raised.** If it is not, and you believe the fix is right, resolve the thread and say *why* in the reply — that is your judgement closing it rather than a confirmation, and the difference is worth putting on the record.

This is the same failure the CI gates in M0 are written to avoid — a check that passes by not running. It is worth recognising in a bot as readily as in our own tooling, and it took **four** separate misreadings on this repository to arrive at the sentence above. Each one was a variation on the same theme: **absence of a signal is not evidence of a state.**

### Your pull request description becomes the commit message

`main` squashes with the **PR title as the commit subject and the PR body as the commit body**. Whatever you write in the description is what someone reads in `git log` a year from now, and what `git bisect` lands them on.

So: write the description for that reader. Say what changed and why. Trim it before merging if review turned it into a mess.

### The standing obligations

These come from the roadmap's cross-cutting workstreams. Each one exists because the failure it prevents is silent — nothing crashes, a guarantee just quietly stops holding.

- **Added a field to `PlayerView`?** It needs a negative fog test. If it can disclose a player's position, it also needs a row in the RFC §9.1 authorised-writer table.
- **Added anything that consumes randomness?** It needs a row in the RFC §6.4 consumption table and an index-count assertion — *including its truncation cases*. An unaccounted draw is a replay divergence that surfaces months later with no obvious cause.
- **Changed a game rule or an architectural decision?** It belongs in the GDD or the RFC first, with a changelog entry, and in code second.
- **Added a number the design calls tunable?** It is a `Config` field, never a constant.

---

## Local development

`make help` lists everything. The ones you will use:

```text
make check     # everything CI runs — start here
make test      # go test -race ./...
make lint      # go vet + golangci-lint
make dev       # build with the debug tag; the debug panel exists in this binary
make prod      # build without it; debug routes do not exist in this binary
make generate  # sqlc (templ joins at M5)
make packages  # assert the package graph matches scripts/packages.txt
```

**`make check` runs exactly what CI runs**, because the workflow calls these targets rather than restating the commands. A CI failure reproduces locally with one command, and there is one definition to keep correct rather than two that drift.

### Requirements

**Go 1.27.0.** RFC §4 names it and §6.3 explains why the project cares: the design is staked on `seed + order log` reproducing a match exactly, and *"which Go built it"* should never be a candidate explanation for a determinism mismatch. Note that `go.mod` can only express a **floor** — no directive pins a version from inside it — so the exact version is enforced in CI.

`golangci-lint` for `make lint`. `gitleaks` for `make secrets`. `templ` for `make generate` — a no-op until M5, so you can skip installing it until then. `sqlc` (issue #315, M3) is no longer skippable: `make generate` uses it for real, and `make test`/`make check` need it too, since `internal/store/sqlc_generate_test.go` shells out to it to prove `sqlc generate` fails closed on a query naming a nonexistent column. Postgres 18.6 arrives with M3.

No Node, no frontend build step, and no Docker for the rules engine — `internal/rules` is pure and its tests do no I/O at all. **This holds for `make check` as a whole, not only for `internal/rules`, even after M3** ([D46](docs/decisions/D46-postgres-backed-test-layer.md), [D54](docs/decisions/D54-integration-tag-fog-gate-and-check-scope.md)): `internal/store`'s Postgres-backed Integration and Concurrency tests live behind `//go:build integration` and a separate `make integration` target, which needs a reachable Docker daemon and hard-fails without one — but is never compiled by `make check`, only by that target itself. `make integration-list` is a Docker-free companion that confirms the tagged suite hasn't silently shrunk; it still needs network access the first time it runs, to fetch the testcontainers-go dependency tree its compile pulls in, so it also stays out of `make check` and runs instead as its own required CI job. Neither target exists before M3.

**A missing tool fails the target rather than skipping it.** That is deliberate, and it is the same principle as the CI gates below: a check that did not run looks exactly like one that passed.

### RTK (agent tooling)

[`.rtk/filters.toml`](.rtk/filters.toml) is committed for [RTK](https://github.com/rtk-ai/rtk), a token-optimised CLI proxy that Claude Code sessions in this repo use to compact command output (`git`, `go test`, `grep`, and friends) without changing what the commands do. It is agent tooling, not a build dependency — human contributors and CI never need it, and its absence fails nothing.

## The CI gates

Ten checks make the architecture real rather than conventional. They are not style checks, and a failure is not a nit:

| Gate | Asserts |
|---|---|
| **Rules purity** | `internal/rules`, `internal/telemetry` and `internal/bots` import nothing that does I/O, tells time, or generates randomness |
| **Fog boundary** | The rendering and web layers cannot name the full match state |
| **Debug isolation** | The production binary contains no debug routes |
| **Secret scan** | No credentials or connection strings in the tree, or in the commits this change adds |
| **Dependency scan** | No `go.mod`/`go.sum` dependency carries a known OSV/GHSA vulnerability |
| **Bots isolation** | `internal/bots`'s production code cannot name `MatchState`, the graph, or the match seed |
| **Simulate dependencies** | `cmd/simulate` depends on nothing but `internal/rules`, `internal/bots`, `internal/game` and `internal/telemetry` |
| **Replay dependencies** | `cmd/replay` depends on nothing outside a committed allow-list, so its fold path can never reach an effect provider |
| **Postgres integration suite** | `internal/store`'s Integration/Concurrency tests (RFC-001 §16.1) pass against a real, pinned-digest Postgres, and fail — never skip — when Docker is unreachable |
| **Integration coverage floor** | The tagged suite has not silently shrunk to fewer tests, or to zero, since the last reviewed floor bump |

If one of these blocks you, the answer is almost never to weaken the gate.

The secret scan reads history as well as the tree, because a credential added in one commit and deleted in the next is absent from the tree and present in the history forever. It is scoped to the commits your branch adds: a credential somewhere else in the repository is not your pull request's problem, and making it one was a real defect ([#37](https://github.com/garnizeh/cinzal/issues/37)). Locally, `make secrets` scopes itself the same way, to `origin/main..HEAD` — falling back to the whole history of `HEAD` when `origin/main` is unknown or your branch adds nothing to it, as on `main` itself. It over-scans rather than under-scans: no path through the gate inspects zero commits, because a scan of nothing reports "no leaks found" exactly like a clean one.

### Which of these actually block a merge

**"Runs in CI" and "blocks a merge" are two different facts.** `main`'s branch protection requires six status checks — `check`, `secrets`, `bench-compare`, `vuln`, `replay (ubuntu-latest)` and `replay (macos-latest)` — and this repository has no rulesets, so that list is the entire enforcement surface.

Three of those are recent. `vuln` and both `replay` legs ran on every pull request while blocking nothing, which meant a red `vuln` — a newly-disclosed CVE against a dependency already sitting in `go.sum` — could be merged, and a cross-OS determinism break with it. [The dependency-vulnerability gate](#the-dependency-vulnerability-gate) below had argued `vuln` should be unskippable, and it was unskippable *by path* while not being a required context at all; [#391](https://github.com/garnizeh/cinzal/issues/391) closed that gap.

**Requiring a path-gated job does not deadlock a prose-only pull request**, which is the objection that would otherwise block this. `check`, `vuln` and `replay` carry no job-level `if`: the job always runs and reports `success` even when its path gate skips every step inside. Verified on [#369](https://github.com/garnizeh/cinzal/pull/369), a documentation-only change, where `check` and both `replay` legs each reported `success` rather than `skipped`.

**`bench` is deliberately excluded.** It is the only CI job with a job-level condition — `if: github.event_name == 'push'` — so it reports `skipped` on every pull request. It records `main`'s benchmark history; `bench-compare` is the gate.

**`integration` and `integration-list` (issue #325) run on every pull request but are not yet in branch protection's required list** — the same gap `vuln` and both `replay` legs sat in before [#391](https://github.com/garnizeh/cinzal/issues/391) closed it. Promoting them is a repository-settings change, not a file in this repository, and is tracked as a follow-up to #325 rather than bundled into it.

Read the list rather than trusting this paragraph when it matters:

```bash
rtk gh api repos/garnizeh/cinzal/branches/main/protection --jq .required_status_checks.contexts
```

### A docs-only pull request does not pay for a Go toolchain

The other six gates above, plus lint/test/build, run through CI's `check` job as `make check-nosecrets` — everything `make check` runs except the secret scan and the dependency scan (see the Makefile's own comment on that split). That job, and `replay`'s cross-OS matrix, skip themselves on a pull request whose diff cannot touch `internal/`, `cmd/`, `scripts/`, `go.mod`/`go.sum`, `Makefile`, `.golangci.yml`, or this repository's CI definitions — a decision record, a GDD/RFC edit, a roadmap update ([#336](https://github.com/garnizeh/cinzal/issues/336)). The check that decides this (`.github/actions/changed-paths`) fails **open**: if it cannot resolve the base commit to diff against, it reports "touched" and everything runs, the same posture `check-secrets.sh` takes when its own commit range does not resolve.

**Neither the secret scan nor the dependency scan participates in this gate, on purpose.** Both run in their own CI jobs, unconditionally, on every pull request. A credential pasted into a decision record is exactly the case the secret scan exists to catch, so it must never be skippable by what a diff touches; a newly-disclosed CVE against a dependency already sitting in `go.sum` is a fact about time, not about this diff, so the dependency scan can't be skippable by path either — see [The dependency-vulnerability gate](#the-dependency-vulnerability-gate) below. `bench-compare`'s own path check (below) uses the same `changed-paths` action but a narrower, unrelated path list — internal/rules/gen and its own gate definition — because what it decides is whether there is anything to benchmark, not whether there is anything to build.

### The bots isolation gate

`internal/bots` legitimately imports `internal/rules` — `Legal`, `Affordances`, `Steps`, `BotRNG` — so this cannot be an import-graph check the way the fog boundary is: the forbidden thing is not the import, it is which identifiers it is used to reach ([#195](https://github.com/garnizeh/cinzal/issues/195)). `scripts/check-bots-isolation.go` walks `internal/bots`'s production `.go` files and fails on two independent things: any `rules.X` reference whose `X` is not named in `scripts/bots-isolation-allowlist.txt`, and any `[32]byte` — the match seed's own shape — appearing anywhere a type can, whether or not the file names `rules` at all. Widening what a bot may reach means adding a name to that allow-list, in a pull request that explains why the new symbol is still fog-safe — see the entries already there for the shape that explanation takes.

Test files are out of scope: `internal/bots`'s own tests legitimately build real matches (`rules.NewMatch`, `rules.Project`, `rules.Resolve`) to drive bots against realistic corpora rather than only hand-built fixtures. `bot_test.go`'s `TestPackageNamesNoMatchStateNowhere` is issue #190's narrower, source-level predecessor to this gate, and still runs — across every file in the package, test files included — as a second net against the literal identifier `MatchState`.

### The simulate dependency gate

RFC-001 §16.4 states `cmd/simulate` "needs only `rules` and `bots`" — a dependency claim, not just a description, and issue #199 is the one that makes it an enforced one. `scripts/check-simulate-deps.sh` runs `go list -deps ./cmd/simulate` and fails unless the internal packages it names are exactly `internal/rules` (its own `internal/rules/gen` dependency included), `internal/bots`, `internal/game` and `internal/telemetry` — the last two are what the RFC's own sentence leaves implicit: `rules` and `bots` both hand the driver `game` values it has to name to call them, and `internal/telemetry` is D34's GDD §22 metric set, the thing this whole command exists to produce. Anything else — `internal/match` chief among them, once it exists — fails the gate. Unlike bots isolation, this stays a plain `go list`/`grep` pipeline rather than an AST walk, so it needs no fixture selftest of its own.

`make bots-isolation-selftest` drives the gate against fixtures for each failure mode — a named `rules.MatchState`, a `:=`-inferred local that never spells the type name, a bare `[32]byte` with no `rules` import at all, a dot-import, a `doc.go`-only package (VACUOUS, not a pass), a parse error, and a missing allow-list — the same standard `check-bench-regression_test.sh` is held to below.

### The replay dependency gate

RFC-001 §7.4 makes a dependency claim about `cmd/replay`, not just about `internal/rules`: the fold's own execution must never dispatch an effect, or a rebuild re-sends every historical notification the match ever generated. `internal/match/fold.FoldThrough` already runs `cmd/replay`'s fold with a null effect sink, but that is a runtime property — [D49](docs/decisions/D49-fold-package-boundary.md), decided while scoping this gate, names `scripts/check-replay-deps.sh` as the compile-time proof standing next to it: `go list -deps ./cmd/replay`'s `internal/` subset must be a subset of a committed allow-list, `scripts/replay-deps-allowlist.txt` ([#324](https://github.com/garnizeh/cinzal/issues/324)). The allow-list is `internal/game`, `internal/rules` (its own `internal/rules/gen` dependency included), `internal/match/fold`, `internal/opsmetrics` (D45's fold-duration/allocation metrics, pulled in transitively by `FoldMeasured` — the same allowance `cmd/simulate` already has), `internal/store` and `internal/store/orderlog`. `internal/mail`, `internal/web` and anything else fail the gate; widening the list is a deliberate, reviewed change, same discipline as `scripts/bots-isolation-allowlist.txt`.

Same shape as the simulate dependency gate — a plain `go list`/`grep` pipeline — but unlike it, this one carries a fixture selftest (`make replay-deps-selftest`, `scripts/check-replay-deps_test.sh`): #324's own acceptance criteria required proof of every failure mode, including the positive case a plain dependency check exists to catch — a mail-shaped provider package introduced into the graph — a bar the simulate gate was never held to. Because `go list -deps` needs a real, buildable `go.mod` to run against, the selftest points the gate at a synthetic fixture module via environment-variable overrides (`REPLAY_DEPS_TEST_ROOT`/`_PKG`/`_ALLOWLIST`), the same idiom `check-rules-purity_test.sh` uses for the same reason, rather than the target-directory argument `check-bots-isolation.go`'s AST walk takes.

### The Postgres integration gate

RFC-001 §16.1 adds two test layers no `internal/store` file had before #325: Integration (a real Postgres, §16.1's own "full match through the HTTP layer" row) and Concurrency (two goroutines racing a shared resource, e.g. §8.1's `deadline_at ± 1ms` boundary). [D46](docs/decisions/D46-postgres-backed-test-layer.md) decided the shape, corrected in one place by [D54](docs/decisions/D54-integration-tag-fog-gate-and-check-scope.md): every file in either layer carries `//go:build integration`, so `go test ./...` (and therefore `make test`/`make check`) never compiles them at all — not a skip, a file `go test ./...`'s own invocation never had in scope. `internal/store/storetest` is the one entry point every such test acquires a database through (`Container` — a transaction against a shared, already-migrated database, rolled back after the test; `FreshDatabase` — a real, independent clone for a test that needs two live connections or a real commit; `FreshUnmigratedDatabase` — for the migration-race exit demonstration alone), started lazily from inside a test body rather than `TestMain`, so that `go test -list` never touches Docker.

`make integration` is the separate command that owns these files: `go test -tags integration -race ./internal/store/... ./internal/match/... ./cmd/replay/...`, gated by `require-docker`, and `storetest.Container`'s own setup hard-fails (`t.Fatal`, never `t.Skip`) every test in a package the instant Docker is unreachable — CLAUDE.md's "a gate that passes when it can't run is worse than no gate," aimed at a suite instead of a tool.

**The coverage floor is the gate that actually catches the suite shrinking.** `go test -tags integration -list '^Test(Integration|Concurrency)' <packages>` compiles every tagged file without running a single test body — Docker-free, since nothing beyond `-list` ever executes — and `scripts/check-integration-coverage.sh` counts the names it returns against a floor recorded in the script, bumped upward only, the same discipline `check-bench-regression.sh`'s threshold and `scripts/bots-isolation-allowlist.txt` already hold contributors to. Every Integration/Concurrency test function is named `TestIntegrationXxx`/`TestConcurrencyXxx` for exactly this reason: the count is scoped by package *and* filtered by name, so a floor over `./...` could not lose one test inside a total dominated by every ordinary test in the repository. `make integration-list` runs the raw `-list` command; `scripts/check-integration-coverage_test.sh` is its own fixture selftest, the same shape `check-replay-deps_test.sh` uses for the same reason (`go test -list` needs a real, buildable `go.mod`, so the selftest points the gate at a synthetic fixture module via environment-variable overrides rather than a fixture directory inside this module).

**Neither joins `check`/`check-nosecrets`'s aggregate, for two different reasons.** `make integration` needs a reachable Docker daemon, which this document's Requirements section already promises `make check` will never require, for `internal/rules`'s sake — the package edited most, and the one D01 keeps dependency-free including this one. `make integration-list` needs no Docker, but D54 found that compiling `storetest`'s own `testcontainers-go` import is still a first-time module fetch and a heavier build than anything else on that aggregate's line pays — a cost this repository has already decided twice (`generate-check`, `bench-compare`) is worth paying to keep the default developer loop cheap, applied a third time here. Both get their own required CI job instead (`.github/workflows/ci.yml`), path-gated with the identical broad list `check`/`replay` already share.

### The dependency-vulnerability gate

Surfaced during [#372](https://github.com/garnizeh/cinzal/pull/372) ([#311](https://github.com/garnizeh/cinzal/issues/311)'s PR): CodeRabbit's SAST step flagged `github.com/moby/go-archive` v0.2.0 — a `testcontainers-go` transitive dependency, since bumped — as carrying GHSA-hfg8-hc9c-6c3h / GO-2026-6253, a tar-extraction path-traversal CVE. Nothing in `make check` would have caught it ([#373](https://github.com/garnizeh/cinzal/issues/373)).

Two tools, not one, because neither alone is sufficient. `govulncheck ./...`'s call-graph reachability analysis only fires against an OSV entry carrying symbol-level data to match against — GO-2026-6253's is `"review_status":"UNREVIEWED"` with an empty `ecosystem_specific` block, so `govulncheck` reports no vulnerabilities against it even with the vulnerable version deliberately reintroduced. `osv-scanner scan source -L go.mod` catches it immediately with plain version-range matching, no reachability analysis required. `govulncheck` still runs, as defense-in-depth for vulnerabilities that *do* carry symbol data — this CVE's gap is specific to it, not a general reason to drop the tool.

**The dependency scan does not participate in the path gate either, for the same shape of reason the secret scan doesn't.** A CVE against a dependency already vendored in `go.sum` can be disclosed at any time, independent of what a given pull request touches — gating the scan on `go.mod`/`go.sum` changes would let a freshly-disclosed CVE ride through any PR that doesn't happen to touch those files. `vuln` runs in its own always-on CI job, unconditionally, the same shape `secrets` already has.

### The benchmark regression gate

**`bench-compare` is also required**, but shaped differently from the eight above: it only runs on a pull request that touches `internal/rules/gen`, this workflow, its own path-gate action (`.github/actions/changed-paths`), `check-bench-regression.sh`, or the `Makefile` — most pull requests skip it rather than pass it. When it runs, it benchmarks the base commit and this pull request's merge commit back to back on the same runner and fails for real past a 20%-per-case or 10%-geomean threshold (`scripts/check-bench-regression.sh`).

It started advisory (issue #113) because two data points could not characterise real CI-runner noise against those thresholds, and was promoted to required in [#127](https://github.com/garnizeh/cinzal/issues/127) once seven real same-runner comparisons had landed with zero false positives — worst single-case drift +6.27%, worst geomean 0.93%, both comfortably inside the thresholds.

### What is deliberately not a gate

**Style linters do not join the required set.** Specifically, `markdownlint` does not run in CI — decided in [#23](https://github.com/garnizeh/cinzal/issues/23), recorded here because that is where the next person to feel strongly will look.

Each gate above exists because the failure it prevents is invisible and expensive: a fog leak ships silently, a determinism break surfaces weeks later on one machine. A missing code-fence language is neither. Mixing the two weakens the signal — if a required check can fail on a cosmetic nit, contributors learn that a red check might be nothing, which is precisely the mirror of the problem this project has already spent real time on: a green check that might be nothing. Keeping the blocking set to *"this would break the game"* is what makes it worth reading.

Style still gets caught; it is caught in review, where the cost of being wrong is a comment rather than a blocked merge. CodeRabbit reports `MD040` and its siblings already.

This bars a **required** check, not the tool. An advisory job — one that annotates and cannot block — was left unbuilt rather than forbidden; if someone wants it, the thing to preserve is that it never becomes required.

## Reporting a bug in a match

Once matches exist, the best bug report is a **match export** — `{seed, config, players, order log}`, a few kilobytes, downloadable by any player of a finished match. Attach it to the issue and `cmd/replay` reproduces your exact match. No description of what went wrong will ever be as useful.

## Licence

Contributions are accepted under the [MIT Licence](LICENSE).
