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
at Intake, the default is to run the **whole pipeline in one continuous
pass, unattended**: read the task, understand it, plan it, execute it,
review-and-fix it until `make check` is clean, publish it as a PR, watch for
and answer CodeRabbit — fix, verify, push, reply, repeat — until no finding
is still raised against the head, merge, delete the branch, close the issue,
update the milestone tracking issue, and end with one summary of everything
that happened. No pause between any of these to ask "should I keep going?" —
the gate at the end of each stage in this document (a closed blocker, a
complete diff, a green check, a clean review) is what decides readiness to
move on, not a check-in.

**Stop and ask only when something out of the ordinary surfaces** — Stage
1's own test for it: an unknown that would make the work useless if guessed
wrong, a genuinely ambiguous product or scope call the GDD/RFC don't settle,
a CodeRabbit finding that would expand scope beyond the one-PR plan, or one
of the confirm-first actions below (force-push, pushing straight to `main`,
deleting a branch other than the feature branch this task itself opened and
just merged, or touching an issue this task wasn't mandated to touch).
Everything else — brief to plan, plan to diff, diff to a green check, that
to an open PR, review to a clean review, clean review to merged-and-closed —
is the pipeline doing what it exists for, not a decision point. Landing on a
red `make check` with no substitute written down, or an unanswered standing
obligation, is a bug in the work to go fix, not a reason to pause and check
in.

**This includes `merge-closeout`, gated precisely.** Squash-merge, delete the
just-merged feature branch, confirm the issue closed, update the milestone
tracking issue, write the local journal entry — all of it runs the moment,
and *only* the moment, CodeRabbit's main PR comment — the first comment after
the PR description, edited in place on every review pass — reads exactly
**"No actionable comments were generated in the recent review. 🎉"** against
the current head. PR [#378](https://github.com/garnizeh/cinzal/pull/378) is
the reference case: that literal text, and nothing else, is what a genuinely
clean review looks like. "No comments visible" is not that condition on its
own — a `Review limit reached` skip also shows no *new* comments while the
main comment itself still carries the rate-limit warning, not the clean line,
and this repo's whole `coderabbit-triage` skill exists because that shape has
been mistaken for a clean review before. Auto-merge fires on the positive
signal — that exact string — never on the mere absence of a negative one.

**Fixing every raised finding is necessary but not sufficient.** Resolving
threads and going green does not rewrite the main comment; it still records
whatever the review that raised those findings said. A fresh review has to
run after the fix, and the gate is only met once *that* review edits the main
comment to the exact clean text.

**On `Review limit reached`, this is a race, not a stall:** wait the stated
refill window (usually 20–45 min, polled in the background — see below), then
request a fresh review (`@coderabbitai review`); if the main comment then
updates to the exact clean text, merge proceeds automatically as above.
**Whichever comes first wins** — if the maintainer explicitly asks for the
merge before that text ever lands, that request is acted on immediately
regardless of CodeRabbit's state, and does not wait for the retry cycle to
resolve.

This is a deliberate, standing authorization the maintainer gave in full
knowledge of what it covers (2026-08-27) — not the pipeline quietly picking
up a hard-reverse action it used to stop for. State plainly what happened at
each of these steps as they happen; reporting faithfully is not the same as
asking permission.

**Waiting on CodeRabbit is not a reason to stall the turn.** A review can
take minutes to post, or never post at all (the free-tier skip). Poll for it
in the background — `Monitor` with an `until`-loop checking
`gh api .../pulls/<n>/reviews`, not a foreground sleep — so the turn is
freed to report progress and pick back up when the poll resolves, and cap
the wait rather than polling forever if nothing ever posts (see
`coderabbit-triage`).

**The one thing this pipeline does not do on its own is stop iterating on a
review early.** "Fix → verify → push → reply → repeat" continues until
either CodeRabbit has no more findings raised against the head, or the user
explicitly says to stop — a fixed number of rounds is not the exit
condition, silence from CodeRabbit on an unresolved finding is not either.

**Running this across a whole milestone, not just one issue, is a separate,
explicit mode** — [`loop-dispatch`](skills/loop-dispatch/SKILL.md), started
only when the maintainer invokes it (`/loop /loop-dispatch`), never a side
effect of running one pass. Three rules hold there that do not apply to a
single unattended pass: issues run strictly serially, never two in flight at
once; any doubt at any stage is decided by the outer dispatcher, never
guessed inside the subagent running the pass, and the dispatcher holds
indefinitely rather than rescheduling a re-poll; and the loop stops
entirely, with no rescheduled wakeup, once the active milestone has no
unblocked issue left.

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

**Receives:** a verified diff. **Produces:** one commit on `main`, merged and
closed out.

| Skill | Gate before it |
|---|---|
| `delivery-review` | Read the whole diff; walk all five standing obligations; ask the fog question in writing |
| `pr-publish` | `delivery-review` came back clean — that is the approval to publish, no separate one needed. The body **is** the commit message. `Fixes #<n>` is present, literally |
| `coderabbit-triage` | Every finding fixed or answered — **including the ones that live only in the review body** — looping fix → verify → push → reply until none remain raised against the head |
| `merge-closeout` | A real review has landed, and no finding is still raised against the head. This gate is **checked, not asked for** — meeting it is what runs the merge, same turn, no separate go-ahead |

**After merge, in the same turn:** verify the issue actually closed, update the
milestone tracking issue, sync `main`, file anything deferred, and close the
pass with one summary — what was built, what it took to get through review,
what landed, what (if anything) is still open.

**The summary also names any harness lesson this pass surfaced** — a gap
between what a skill claimed and what actually happened, a rule that only
lived in an agent's memory instead of the skill file, a check that turned
out to need a real command instead of "read it back" or "eyeball it." Naming
it in the summary without fixing it is how the same gap gets rediscovered on
the next task instead of the one after that. **Fix it the same pass, tracked
like everything else here:** file a plain issue for it — no GDD/RFC spec
anchor, since a harness/process fix isn't about game content and doesn't
have one; every other field in CONTRIBUTING's "what every issue must carry"
table still applies — then land the fix as its **own follow-up PR closing
that issue**, never bundled into the task's own PR, since a content change
and a process fix are different things reviewed for different reasons (the
precedent already in use: #368's Postgres fix and #370's workflow fix landed
as two PRs, not one — #370 itself should have had an issue behind it and did
not, which is the gap this paragraph closes). This repeats for as long as a
pass keeps surfacing gaps — there is no cap on how many harness-fix issues
and PRs one task's pass produces, only on bundling them with the content PR.

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
- **Confirm before actions outside the pipeline's own shape** —
  force-pushing, pushing straight to `main`, deleting any branch other than
  the one this task itself opened and just squash-merged, or
  editing/closing an issue the current task isn't the one mandated to touch.
  Pushing your own feature branch, opening its PR, merging it once
  `coderabbit-triage`'s gate is met, and deleting that same feature branch
  as part of `merge-closeout` are all the normal output of the pipeline, not
  on this list — see "Running it end to end" above.
- **Report faithfully.** Failed tests get quoted. Skipped steps get named. Done
  and verified gets stated plainly without hedging.

---

## Entering mid-pipeline

Most sessions do not start at Intake. Locate yourself by what exists:

| What you have | Enter at |
|---|---|
| No issue number — "start next issue" was said | `issue-intake` §0, which picks one (out-of-band unmilestoned+unblocked work first, then the current milestone's next unblocked row) |
| An issue number and nothing else | `issue-intake` |
| A brief, no edits yet | `task-plan` |
| A plan, no diff | the matching Execute skill |
| A diff, no green check | `gates-run` |
| A green check, no PR | `delivery-review` |
| An open PR | `coderabbit-triage` |
| An approved PR | `merge-closeout` |

If you cannot tell, you are at Intake. Re-deriving a brief is cheap; executing
against the wrong classification is not.
