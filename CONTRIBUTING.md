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
| **Spec anchor** — a GDD or RFC section | The most important one. The documents are the authority, so a task that cannot cite a section is inventing a requirement. When that happens it is **not a task, it is a decision** — file it as one. |
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
- **Retry after the refill time the message reports**, with a `@coderabbitai review` comment. It is usually 20 to 35 minutes. When the message gives no time at all, the free allowance is exhausted for longer and there is nothing to do but wait.
- **Note that CodeRabbit reviews incrementally** and will not re-review a commit it has already seen. Its own message says the manual command "is applicable only when automatic reviews are paused" — so if an incremental review was missed, the reliable way to get one is a new commit once the allowance is back, not repeating the command.
- **Merge only once a real review has landed.** If waiting is genuinely not an option, say so in the pull request description, so the commit message on `main` records what went in unreviewed rather than implying it did not.

This is the same failure the CI gates in M0 are written to avoid — a check that passes by not running. It is worth recognising in a bot as readily as in our own tooling.

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
make generate  # templ + sqlc
make packages  # assert the package graph matches scripts/packages.txt
```

**`make check` runs exactly what CI runs**, because the workflow calls these targets rather than restating the commands. A CI failure reproduces locally with one command, and there is one definition to keep correct rather than two that drift.

### Requirements

**Go 1.26.5.** RFC §4 names it and §6.3 explains why the project cares: the design is staked on `seed + order log` reproducing a match exactly, and *"which Go built it"* should never be a candidate explanation for a determinism mismatch. Note that `go.mod` can only express a **floor** — no directive pins a version from inside it — so the exact version is enforced in CI.

`golangci-lint` for `make lint`. `templ` and `sqlc` for `make generate`; both are no-ops until M5 and M3 respectively, so you can skip them until then. Postgres 16 arrives with M3.

No Node, no frontend build step, and no Docker for the rules engine — `internal/rules` is pure and its tests do no I/O at all.

**A missing tool fails the target rather than skipping it.** That is deliberate, and it is the same principle as the CI gates below: a check that did not run looks exactly like one that passed.

## The CI gates

Four checks make the architecture real rather than conventional. They are not style checks, and a failure is not a nit:

| Gate | Asserts |
|---|---|
| **Rules purity** | `internal/rules` imports nothing that does I/O, tells time, or generates randomness |
| **Fog boundary** | The rendering and web layers cannot name the full match state |
| **Debug isolation** | The production binary contains no debug routes |
| **Secret scan** | No credentials or connection strings in the diff |

If one of these blocks you, the answer is almost never to weaken the gate.

## Reporting a bug in a match

Once matches exist, the best bug report is a **match export** — `{seed, config, order log}`, a few kilobytes, downloadable by any player of a finished match. Attach it to the issue and `cmd/replay` reproduces your exact match. No description of what went wrong will ever be as useful.

## Licence

Contributions are accepted under the [MIT Licence](LICENSE).
