---
name: task-plan
description: Turn a Cinzal brief into an executable plan — research the specs, the decision log and the existing code, then lay out the change as ordered steps with their tests and gates. Use after issue-intake, or when the user asks to plan, scope, or "como vamos fazer" a tracked piece of work, before any file is edited.
---

# Task plan

Input: a brief (from `issue-intake`) or an issue the user has already described.
Output: a plan the executing skill can follow without re-deriving anything.

**Nothing is edited in this stage.** If you find yourself wanting to write code
to understand the problem, write it in the scratchpad, not the repo.

---

## 1. Re-read the authority, in this order

1. **The changelog** of the GDD and the RFC. Both are heavily changelogged and
   later entries correct earlier sections. Check the RFC's "Companion doc"
   header for which GDD revision it is paired with.
2. **The cited sections themselves.**
3. **Every decision document in the dependency chain** — not just the one the
   issue names. `docs/decisions/README.md` is the catalogue; its status column
   tells you what is decided and what the decision actually was, so you can
   spot the neighbouring ones that bind you too.
4. **The roadmap section** for the milestone — deliverables and exit criteria.
   Your change has to leave the exit criteria reachable.

For a third-party library, framework or CLI (pgx, sqlc, goose, templ, HTMX,
testcontainers, benchstat), fetch current docs with Context7 rather than
recalling API shapes.

## 2. Read the code that already exists

- `scripts/packages.txt` is the declared package graph. Any new package changes it.
- `internal/game` is a **leaf**: shared vocabulary only, imports nothing, and no
  `any`/`interface{}`/unconstrained type params. There is no `game.State` and
  there must never be one.
- `internal/rules` owns match state; imports only stdlib + `internal/game`, and
  is **pure** — no I/O, `time`, `math/rand`, network.
- `internal/render` and `internal/web` must **never** import `internal/rules`
  directly. Everything arrives via `internal/match` as `game` types.
- Most of `internal/` is still `doc.go` only. Read the `doc.go` — it usually
  states the package's contract before any code exists.

Find the nearest existing analogue and read it end to end. This codebase has
strong local idiom (comment density, table-driven tests, error phrasing) and a
plan that ignores it produces a diff that reads as foreign.

## 3. Decide the shape, then write the plan

The plan is ordered steps. Each step names the files it touches and the check
that proves it. Shape:

```markdown
# Plan — #<n> <title>

## Approach
The shape of the change in three or four sentences, and the alternative you
rejected with the reason. If there is a real trade-off, state your
recommendation rather than surveying options.

## Steps
1. **<what>** — `path/to/file.go`
   - <the concrete change>
   - proved by: <test name / gate / command>
2. ...

## Tests
Which existing tests must still pass, which new ones this needs, and which RFC
§16.1 layer each belongs to (golden replay, RNG index accounting, fog negative,
anchor parity, cross-round, property/adversarial, bot determinism).

## Gates this touches
`make check` runs: packages purity purity-selftest fog debug-isolation secrets
bots-isolation bots-isolation-selftest simulate-deps lint test
bench-regression-selftest prod dev. Name the ones this change can move, and why.
Say whether `bench-compare` triggers (it does only for `internal/rules/gen`,
`.github/workflows/ci.yml`, `scripts/check-bench-regression.sh` or `Makefile`).

## Spec edits required
The GDD or RFC changes **first** if this changes a rule or an architectural
decision — with a changelog entry. List the sections and the changelog line.

## Standing obligations carried from the brief
The ones that answered yes, and where in the steps each is discharged.

## Open questions
Only the ones that change what gets built. Each with your assumption if
unanswered.
```

## 4. Size it against the one-PR rule

**One task = one pull request = one commit on `main`.** `main` is squash-only
with linear history because `git bisect` is the diagnostic tool when the
determinism check fires weeks later, and bisect only works if every commit is
coherent and buildable alone.

If the plan does not fit that, split it into issues and say so **before**
executing anything. If it is trivially small, fold it into its neighbour.

## 5. Present it and get agreement

Show the plan. Flag the open questions. Ask only where different answers produce
materially different work — otherwise state the assumption and proceed.

Then route:

- code → `code-change`
- documentation or spec edits → `docs-change`
- a decision document → `decision-record`
- a milestone criterion → `exit-demo`
