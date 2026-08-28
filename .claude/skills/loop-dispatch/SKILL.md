---
name: loop-dispatch
description: Drive Cinzal's task pipeline (issue-intake through merge-closeout) unattended across a whole milestone's remaining issues, one at a time, via an outer dispatcher session and fresh isolated subagents per pipeline pass. Use when the maintainer asks to loop the pipeline across a milestone, run it unattended across issues, "loop pela milestone", "dispatcha as issues", or wants to start a self-paced /loop instead of invoking "do next task" by hand for every row.
---

# Loop dispatch

`WORKFLOW.md`'s "Running it end to end" already runs one issue, start to
merge, in a single unattended pass. This skill is the outer layer on top of
that: it repeats the pass across a milestone's remaining issues without the
maintainer re-triggering it each time, while holding to three rules agreed
directly with the maintainer (2026-08-27, captured in #381): **strict serial
execution, a hard stop on any doubt routed through the dispatcher, and a full
stop — no rescheduled wakeup — once the milestone's queue is empty.**

**Starting this loop is a separate, explicit act** — building this skill does
not start it. The maintainer invokes it as the repeated prompt of the
generic, self-paced `/loop`: `/loop /loop-dispatch`.

---

## 1. The dispatcher is thin — it never does task work itself

The outer `/loop` session (dynamic, self-paced — no fixed interval) is a
**dispatcher, not a worker**. Its own per-cycle job is only:

1. Decide whether a task is already in flight.
2. Either wait on it, hand off its next stage, or — if nothing is in
   flight — select the next issue and hand off a fresh full pass.
3. Schedule the next wakeup, or stop.

Every actual pipeline stage (`issue-intake`, `task-plan`, `code-change` /
`docs-change` / `decision-record` / `exit-demo`, `delivery-review`,
`pr-publish`, `coderabbit-triage`, `merge-closeout`) runs inside an `Agent`
subagent, never in the dispatcher's own context. This is what keeps the
dispatcher's context small regardless of how many issues the loop processes,
and it is the direct substitute for the maintainer's manual `/clear` between
tasks — a genuinely fresh subagent starts with no context to clear, rather
than the dispatcher trying to summarize away what came before.

## 2. Per-cycle decision

**An open PR is not the only in-flight signal — it is not even the first
one to check.** A delegated pass runs `issue-intake` through `pr-publish`
before any PR exists; a subagent can be minutes into `task-plan` or
`code-change` with no PR yet. If the dispatcher's *own* per-cycle check
looked only at open PRs, a fallback `ScheduleWakeup` firing during that
window would see "nothing in flight" and select a second issue — two
subagents genuinely mid-pipeline at once, which is exactly what Rule 2
forbids. So the dispatcher keeps its own record, checked first:

**0. Read the active-issue marker** — a small scratchpad file (e.g.
`loop-dispatch-active.md`) the dispatcher itself writes and owns, holding:
the issue number, the UTC time the pass was spawned, and the subagent's
`agentId`. Three cases:

- **No marker** — nothing dispatched by this loop is running. Fall through
  to the "no task in flight" branch below.
- **Marker present, and that issue's PR is now merged and closed** — the
  pass finished since the last cycle. Delete the marker, then fall through
  to the "no task in flight" branch in this same cycle (no need to wait for
  another wakeup just to notice).
- **Marker present, and that issue's pass has not finished** (no PR yet, or
  an open PR, or a report of failure/crash) — this **is** the in-flight
  case; do not select a new issue this cycle no matter what the open-PR
  check alone would suggest:
  - **No PR yet** — the subagent is still pre-`pr-publish`. Do not spawn
    anything for this issue or any other; just re-arm a long fallback
    `ScheduleWakeup` and wait. The subagent's own completion notification is
    the primary wake signal (per the general `Agent`-tool guidance the
    dispatcher itself runs on) — this fallback only covers it being missed
    or the pass running long.
  - **A PR now exists and is open** — proceed as the old single-signal
    check did: if it's waiting on CI or a CodeRabbit review, arm a `Monitor`
    (per `WORKFLOW.md`'s "Waiting on CodeRabbit is not a reason to stall the
    turn") and hold with a long fallback `ScheduleWakeup`; if it has moved
    (checks landed, a review posted), hand off a **fresh** subagent to
    continue from wherever `WORKFLOW.md`'s "Entering mid-pipeline" table
    says that state enters — never a subagent that already ran a previous
    stage in this same loop.
  - **The subagent's completion notification reports failure or a crash**
    (not the Rule 1 "blocked" report — an outright error) — treat this the
    same as Rule 1: surface it to the maintainer with what's known and
    `ScheduleWakeup stop: true`. Guessing how to resume a pass that failed
    outright, with no written state, is itself the kind of doubt Rule 1
    exists for.

**Nothing in flight** (no marker, or the marker was just cleared above)?
Run `issue-intake` §0's selector exactly as it is defined there (out-of-band
unmilestoned+unblocked work first, then the active milestone's next
unblocked checklist row). Two outcomes:

- **Found one** — **write the active-issue marker first**, then spawn a
  fresh subagent with the full brief the selector would give a human:
  "drive issue #`<n>` through `issue-intake` → `task-plan` → its execute
  skill → `delivery-review` → `pr-publish`, in one continuous pass, per
  `WORKFLOW.md`." Stop there for this cycle — `coderabbit-triage` and
  `merge-closeout` are picked up by a later cycle, once the PR exists and
  this cycle's subagent has exited. Writing the marker *before* spawning
  (not after) is what closes the race: the marker exists the instant the
  issue is chosen, before the subagent has had any chance to run.
- **Nothing found** — this is Rule 3 (§5 below): report and stop the loop
  entirely, do not schedule another wakeup.

## 3. Rule 1 — any doubt stops the loop, decided by the dispatcher, never inside a subagent

The bar is `issue-intake`/`task-plan`'s existing stop condition, at least
that strict: a genuine ambiguity where guessing wrong makes the work useless
— a spec gap, a gate that looks wrong, any real design choice the GDD/RFC/
decision log do not settle. **A subagent that hits this must not decide it,
guess past it, or try to hold a paused conversation with the maintainer
itself** — the dispatcher is the only session with a channel to them. The
subagent's contract on hitting this:

1. Stop making progress immediately — no further edits, no guess "for now."
2. Write its state to the scratchpad (see §6) — not just the open question,
   the whole recoverable picture: what stage it was in, what is done, what
   is pending, and a pointer to any in-progress diff.
3. Report back to the dispatcher: the concrete problem, a few options with
   their tradeoffs, and the scratchpad file's path. Nothing else.

The dispatcher, on receiving that report:

1. States the problem and the options to the maintainer plainly, in the
   dispatcher's own turn — this is the one piece of output in the whole loop
   a human is expected to read and act on.
2. Calls `ScheduleWakeup` with `stop: true`. **Not** a rescheduled wakeup
   that re-polls for an answer — the issue's own rule is "no timeout, no
   retry, no proceeding on an assumption," and the maintainer's next message
   is itself what resumes the session; a poll loop watching for that message
   would be redundant at best and a silent retry-on-a-timer at worst.

## 4. Rule 2 — strict serial execution

At most one issue is ever mid-pipeline. The gate is the active-issue marker
from §2 step 0, not the open-PR check alone — the marker is what covers the
whole pass from the moment an issue is selected, including the
pre-`pr-publish` window when no PR exists yet for the open-PR check to see.
The dispatcher runs `issue-intake` §0's selector to pick a *new* issue only
when no marker is present (or the marked issue's own pass has just finished
and the marker was cleared this cycle). Two subagents for two different
issues are never spawned in the same cycle, and a cycle that finds an active
marker does not also look for a next issue — it either waits or continues
that same issue's pass.

## 5. Rule 3 — stop-at-milestone-end, no reschedule

This is `issue-intake` §0 step 4, used the same way: "if everything reachable
is blocked, say so — don't guess past it." When the selector reports nothing
selectable — no out-of-band work, and the active milestone's tracking issue
has no unblocked unchecked row — the dispatcher reports which rows (if any)
remain and what blocks each, then calls `ScheduleWakeup` with `stop: true`.
This repository runs one milestone at a time; the maintainer creates the next
milestone and its issues by hand, then starts a fresh loop explicitly. The
loop does not idle-poll waiting for that to happen.

## 6. Resumption is a fresh subagent reading written state, never a resumed idle one

`SendMessage` can resume a previously spawned agent "with full context," but
whether an idle subagent stays resumable across a multi-hour gap — the
maintainer deciding from their phone, possibly hours later — has no
documented lifetime. Per this repository's own "absence of a signal is not
evidence of a state," an undocumented limit is treated as a real one, not as
freedom. So resumption never relies on it:

- The blocked subagent's scratchpad file (§3 step 2) is the **only** thing
  a resuming subagent may depend on, plus the maintainer's answer. It must
  be sufficient on its own — progress so far, the open question, the options
  — for a subagent with genuinely empty context to continue correctly.
- Once the maintainer answers (in a normal reply — this is what ends the
  dispatcher's `stop: true` hold), the dispatcher spawns a **new** `Agent`
  call pointed at the scratchpad file's path plus the maintainer's decision.
  It does not `SendMessage` the original blocked subagent, resumed or
  otherwise.
- **The active-issue marker (§2 step 0) is updated, not cleared, across
  this handoff** — the pass for that issue is still not done. Overwrite the
  marker's `agentId` with the new resuming subagent's, keep the same issue
  number, and the marker keeps gating serial execution exactly as it did
  before the stop.

## 7. What this skill does not cover

- **Actually starting the loop.** That is the maintainer's explicit act
  (`/loop /loop-dispatch`), not a side effect of this skill existing.
- **Creating a milestone's issue set.** Stays a manual maintainer step
  (Rule 3) — the loop only ever works through issues that already exist.
- **Deciding which pipeline skill an issue routes to.** That is
  `issue-intake`'s classification (task/decision/exit-demonstration),
  unchanged by running inside a subagent.

## Then

The loop's own "then" is itself — the next cycle, or a stop. When a cycle's
subagent completes an issue through merge, `merge-closeout`'s own final
summary is what the maintainer sees for that issue; the dispatcher's job is
only to notice the PR is gone (merged and closed) on its next in-flight
check and move to selecting the next one.
