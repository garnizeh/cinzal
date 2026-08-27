# Workflow harness

How work moves through this repository, stage by stage: what each stage
receives, what it must produce, and the condition that has to hold before the
next one starts.

The skills in [`.claude/skills/`](skills/) implement the stages. This document is
the contract between them — read it when deciding *which* stage you are in, or
when a stage's output does not look like enough to hand on.

[`skills/README.md`](skills/README.md) is the quick lookup table.

---

## The shape of the pipeline

```
  ┌─ INTAKE ──────────┐   ┌─ EXECUTE ─────────┐   ┌─ VERIFY ──────────┐   ┌─ LAND ────────────┐
  │ issue-intake      │   │ code-change       │   │ test-authoring    │   │ delivery-review   │
  │ task-plan         │──►│ docs-change       │──►│ gates-run         │──►│ pr-publish        │
  │                   │   │ decision-record   │   │ bench-run         │   │ coderabbit-triage │
  │                   │   │ exit-demo         │   │                   │   │ merge-closeout    │
  └───────────────────┘   └───────────────────┘   └───────────────────┘   └───────────────────┘
        brief                  a diff                  green check              a commit
        plan                                                                    on main
```

Execute and Verify iterate. Intake and Land do not — going back to Intake means
the plan was wrong, and that is worth saying out loud rather than absorbing
silently.

---

## Running it end to end

The four stages are not four approval gates. Once a work item is classified
at Intake, the default is to run Intake → Execute → Verify → Land through
`pr-publish` in one continuous pass — read the task, plan it, do it, review
it until it's clean, publish it as a PR — with no pause between stages to
ask "should I keep going?" The gate at the end of each stage in this
document (a closed blocker, a complete diff, a green check, a clean
`delivery-review`) is what decides readiness to move on, not a check-in.

**Stop and ask only when the answer depends on the user's decision** —
Stage 1's own test for it: an unknown that would make the work useless if
guessed wrong, a genuinely ambiguous product or scope call the GDD/RFC don't
settle, or one of the confirm-first actions below. Everything else —
brief to plan, plan to diff, diff to green check, green check to open PR —
is the pipeline doing what it exists for, not a decision point. Landing on
a red `make check` or an unanswered standing obligation is a bug in the
work to go fix, not a reason to pause and check in.

`merge-closeout` is the one stage this doesn't reach on its own — merging is
a separate, explicit ask, gated on a real review having landed, not on
publishing having happened.

---

## Stage 1 — Intake

**Receives:** an issue number, or a description of untracked work.
**Produces:** a brief, then a plan. Both in the scratchpad, neither in the repo.

| Skill | Output | Done when |
|---|---|---|
| `issue-intake` | Brief: what is asked, what governs it, which **kind** of work item, the five standing obligations answered, the unknowns | The work item is classified and the acceptance criterion is demonstrable |
| `task-plan` | Plan: ordered steps, files, the check that proves each, the tests, the gates touched, the spec edits required | It fits **one PR that leaves `main` green by itself** |

**Gate to Execute:** the work item is classified, its blockers are genuinely
closed (a roadmap row saying "decided" with no document in `docs/decisions/` is
not closed), and no open question would make the work useless if guessed wrong.

**The classification decides everything downstream:**

| Kind | Test | Executes as |
|---|---|---|
| Task | Cites a GDD/RFC section that already says what to do | `code-change` or `docs-change` |
| Decision | The specs are silent, ambiguous, or self-contradictory | `decision-record` |
| Exit demonstration | Proves a milestone criterion, often by breaking something | `exit-demo` |

A task that cannot cite a spec anchor **is a decision** — file it as one rather
than inventing the requirement. A document containing a merely *wrong sentence*
is neither: it is a `docs-change`, because there is nothing to weigh.

---

## Stage 2 — Execute

**Receives:** a plan. **Produces:** a diff on a branch.

Whatever the kind, four constraints hold:

1. **Fog is private.** All state goes through `Project`. `render`/`web` never
   import `internal/rules`. Fog tests assert **absence**.
2. **`internal/rules` is pure**, and so are `telemetry` and `bots` — no I/O, no
   `time`, no `math/rand`, no network.
3. **`seed + order log` reproduces a match forever.** No map-range order, no
   floats, no `time.Now()`, no concurrency in `Resolve`. Every draw gets an RFC
   §6.4 row and an index-count assertion, truncation cases included.
4. **The documents change first.** A rule or architectural change lands in the
   GDD or the RFC with a changelog entry before it lands in code.

**Gate to Verify:** the diff is complete for the plan's scope, probe edits are
reverted, and nothing beyond the issue's scope crept in.

---

## Stage 3 — Verify

**Receives:** a diff. **Produces:** a green `make check`, or a written reason it
does not apply.

`make check` runs exactly what CI runs. Every gate **fails closed** — a missing
tool, an empty `go list`, an unreadable config all report failure, never a skip.
**A gate that passes when it cannot run is worse than no gate.**

- New guarantee → `test-authoring` places it in the right RFC §16.1 layer.
- Full suite → `gates-run`.
- `internal/rules/gen` or hot paths → `bench-run` (20% per case, 10% geomean).

**A docs-only diff gets no signal from `make check`** — skip it and record what
you verified instead. The secret scan still runs unconditionally in CI, on
purpose.

**Gate to Land:** green, or an explicit, written skip with its substitute
verification.

---

## Stage 4 — Land

**Receives:** a verified diff. **Produces:** one commit on `main`.

| Skill | Gate before it |
|---|---|
| `delivery-review` | Read the whole diff; walk all five standing obligations; ask the fog question in writing |
| `pr-publish` | `delivery-review` came back clean — that is the approval to publish, no separate one needed. The body **is** the commit message. `Fixes #<n>` is present, literally |
| `coderabbit-triage` | Every finding fixed or answered — **including the ones that live only in the review body** |
| `merge-closeout` | A real review has landed, and no finding is still raised against the head |

**After merge, in the same turn:** verify the issue actually closed, update the
milestone tracking issue, sync `main`, file anything deferred.

---

## The principle underneath all of it

**Absence of a signal is not evidence of a state.**

It took four misreadings on this repository to arrive at that sentence, and it
explains three otherwise unrelated rules:

- A **green CodeRabbit check** on a skipped review is indistinguishable from one
  on a clean review.
- A **CI gate** that inspected zero packages reports the same green as one that
  inspected all of them.
- A **test suite** that skipped for a missing Postgres reports the same green as
  one that ran.

So: gates fail closed, reviews are confirmed by the *negative* signal only, and a
skipped check is written down as skipped rather than counted as passed.

---

## Cross-cutting rules that hold in every stage

- **`rtk` prefixes every shell command** — `git`, `gh`, `make`, everything
  chained with `&&`. It passes through transparently where it has no filter.
- **Everything is English** — code, comments, docs, commit messages, PR and
  issue text, and replies to the user — whatever language the request came
  in (CLAUDE.md). The one exception is the bilingual trigger phrases in
  `.claude/skills/*/SKILL.md` frontmatter, matching what the maintainer
  actually types.
- **One task = one PR = one commit on `main`.** `git bisect` is the diagnostic
  tool when the determinism check fires weeks later, and it only works if every
  commit is coherent and buildable alone.
- **Read the changelog before trusting a spec section.** Later entries correct
  earlier ones.
- **Confirm before actions outside the pipeline's own shape** — merging (a
  separate, explicit ask per Stage 4), force-pushing, deleting a branch,
  pushing straight to `main`, or editing/closing an issue the current task
  isn't the one mandated to touch. Pushing your own feature branch and
  opening its PR are the normal output of `pr-publish`, not on this list —
  see "Running it end to end" above.
- **Report faithfully.** Failed tests get quoted. Skipped steps get named. Done
  and verified gets stated plainly without hedging.

---

## Entering mid-pipeline

Most sessions do not start at Intake. Locate yourself by what exists:

| What you have | Enter at |
|---|---|
| An issue number and nothing else | `issue-intake` |
| A brief, no edits yet | `task-plan` |
| A plan, no diff | the matching Execute skill |
| A diff, no green check | `gates-run` |
| A green check, no PR | `delivery-review` |
| An open PR | `coderabbit-triage` |
| An approved PR | `merge-closeout` |

If you cannot tell, you are at Intake. Re-deriving a brief is cheap; executing
against the wrong classification is not.
