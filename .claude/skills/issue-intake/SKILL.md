---
name: issue-intake
description: Read a Cinzal issue and turn it into a work brief, or file a new one. Use when the user names an issue number ("vamos pegar a #319", "analisa a issue 325", "what does #328 ask for"), asks for a brief, asks to open/file an issue, describes work that is not tracked yet, or says "start next issue" / "próxima issue" / "pega a próxima" with no number given. Also decides which of the three work-item kinds it is — task, decision, or exit demonstration — which determines every downstream step.
---

# Issue intake

The first stage. Produces a **brief**: a self-contained statement of what is
being asked, what governs it, and what "done" looks like. Nothing is planned or
written here.

`gh issue view` is broken on this repo — read issues with `gh api`, see
[gh-recipes.md](../../reference/gh-recipes.md).

---

## 0. No issue named — "start next issue"

Trigger phrases: "start next issue", "próxima issue", "pega a próxima" — no
number given, asking the pipeline to pick its own starting point. Selection
order:

1. **Out-of-band work first.** Open issues with no milestone assigned (body
   says `**Milestone:** Out of band`) whose `Blocked by` is empty or fully
   closed, **and** that carry no `blocked` label — a `blocked` label on an
   unmilestoned issue marks deliberate deferral to a future milestone (e.g.
   #241/#237's M5.5 hand-offs), not readiness now, and those are skipped
   here, not picked. This is broader than `harness: …`-titled issues
   (process/skill gaps, WORKFLOW.md's "the summary also names any harness
   lesson") — it also catches things like a CI/tooling gap an out-of-scope
   review finding got filed as (#373), which shares the same "Out of band"
   milestone convention without the `harness:` prefix. Oldest first (lowest
   number). These go before milestone work: a gap left open here costs every
   task that runs before it's fixed, not just the one that found it.
2. **Otherwise, the current milestone's next task.** "Current" is whichever
   milestone `CLAUDE.md`'s "Repository state" paragraph names as underway,
   and its tracking issue is the one `CLAUDE.md` points at — not GitHub's own
   milestone `state`, which stays `open` on every milestone here regardless of
   whether `CLAUDE.md` calls it closed. Read that issue's checklist top to
   bottom; the first unchecked `- [ ]` row whose "Blocked by" is fully closed
   (or has none) is next.
3. **Skip anything already mid-pipeline.** An unchecked row with an open PR
   already referencing its issue number is in progress, not next — check
   before picking one, and continue that PR instead
   ([WORKFLOW.md](../../WORKFLOW.md)'s "Entering mid-pipeline" table) rather
   than starting over.
4. **If everything reachable is blocked, say so — don't guess past it.**
   Report which rows are next in line and what blocks each, rather than
   silently picking something further down the list.

Once an issue is selected, proceed exactly as if the user had named it —
§1 onward, same brief, same continuous pass.

## 1. Read the issue and everything it points at

```bash
rtk gh api repos/garnizeh/cinzal/issues/<n> --jq '{number,title,state,labels:[.labels[].name],body}'
rtk gh api repos/garnizeh/cinzal/issues/<n>/comments --jq '.[] | "--- \(.user.login)\n\(.body)"'
```

Comments are not decoration here — issues on this repo get amended in comments,
and a later comment often narrows or corrects the body. Read them all.

Then follow every link the issue makes. **Use the Memex MCP for this** — the
GDD, the RFC, the roadmap and `docs/decisions/` are all indexed there, and
semantic search surfaces a section's changelog history and cross-references
that a keyword `grep` misses. Fall back to `grep`/`Read` only if Memex is
unavailable.

- **Spec anchor** — open the cited GDD/RFC section *and read that document's
  changelog first*. Later changelog entries correct earlier sections. A brief
  written against a superseded section is worse than no brief.
- **Blocked by** — check each blocker is actually closed. `Dnn` blockers close
  with a document in `docs/decisions/`; a roadmap row saying "decided" with no
  file is not closed.
- **Decision documents** the issue or the spec section names. Read the
  *Consequences* section of each — that is where the constraint on your work is.

## 2. Classify the work item

This is the load-bearing judgement. The three kinds are tracked differently and
execute through different skills.

| Kind | Test | Produces | Next skill |
|---|---|---|---|
| **Task** | Can cite a GDD/RFC section that already says what to do | Code or documentation | `code-change` / `docs-change` |
| **Decision** | The specs are silent, ambiguous, or contradict each other | A `docs/decisions/Dnn-*.md` document | `decision-record` |
| **Exit demonstration** | Proves a milestone criterion, often by breaking something on purpose | Evidence under `docs/exit-demos/<issue>/` | `exit-demo` |

**A task that cannot cite a spec anchor is a decision.** If you find yourself
about to invent a requirement, stop and say so in the brief — the output is then
"file this as a decision", not a plan.

Sizing test for a task: *can this land as one pull request that leaves `main`
green by itself?* If not, say so and propose the split. If it is three lines,
say which neighbour it folds into.

## 3. Check the standing obligations

Walk all five explicitly and record a yes/no for each, with the reason:

- Adds a field to `PlayerView` → needs a **negative fog test**.
- Can disclose a player's position → needs a row in the **RFC §9.1 authorised-writer table**.
- Consumes randomness → needs a row in the **RFC §6.4 consumption table** and an
  index-count assertion, **including truncation cases**.
- Changes a game rule or an architectural decision → the **GDD or RFC changes
  first, with a changelog entry**, and code second.
- Introduces a number the design calls tunable → it is a **`Config` field**, never a constant.

Also ask the question that governs everything here: **does this leak state past
the fog boundary?**

## 4. Write the brief

Keep it in the scratchpad, not the repo. Shape:

```markdown
# Brief — #<n> <title>

**Kind:** task | decision | exit demonstration
**Milestone / Area:** M3 / store
**Blocked by:** #<n> (closed ✓ / OPEN ✗)

## What is being asked
One paragraph, in your own words, not the issue's.

## Governing authority
- GDD §x.y — <what it mandates, quoted where exact wording matters>
- RFC §x.y — <same>
- D<nn> — <the consequence that binds this work>
- Changelog: <any entry that corrects the above>

## Acceptance criterion
Demonstrable, restated concretely. If the issue's is aspirational, say so and
propose a demonstrable one.

## Standing obligations
Five rows, each yes/no with the reason.

## Unknowns and risks
Anything the specs do not settle. Each one is either a question for the user or
a candidate decision — say which.

## Out of scope
What a reader might reasonably expect to be included and is not.
```

## 5. Hand off — do not stop here

State the brief plainly (what's asked, its kind, the acceptance criterion),
then **continue straight into `task-plan` in the same turn** — per
[WORKFLOW.md](../../WORKFLOW.md)'s "Running it end to end," intake and plan
are one continuous pass, not a checkpoint to wait at.

**The one exception:** if an unknown would make the work useless if guessed
wrong — a genuinely ambiguous product/scope call the GDD/RFC don't settle —
stop and ask before planning. Everything else gets a stated assumption in the
brief and the pipeline keeps moving.

---

## Filing a new issue

When the work is not tracked yet, the same classification applies — pick the
matching template under `.github/ISSUE_TEMPLATE/` (`task.yml`, `decision.yml`,
`exit-demonstration.yml`) and fill **every** required field. An issue with no
spec anchor and no demonstrable acceptance criterion will not survive review.

Out-of-scope findings from a code review get filed too — a reply saying
"pre-existing" is not enough on its own; track it as a real issue and link it.

**No manual line-wrap inside a paragraph or bullet** — one physical line per
paragraph, same rule `pr-publish` states for PR bodies, and it has already
been violated here too (#368, 2026-08-27). Run the mechanical check against
the body file before `gh issue create`, not an eyeball read — it takes the
path as an argument, so it works on an issue body exactly as it does on a PR
body:

```bash
rtk ./scripts/check-no-hard-wrap.sh <body-file>
rtk gh issue create --title "<area>: <what>" --body-file <body-file> --label task
```

**After filing milestone work, update that milestone's tracking issue in the
same turn** — that has been forgotten twice. The current one is whichever
`CLAUDE.md`'s "Repository state" names; see
[gh-recipes.md](../../reference/gh-recipes.md) for the discovery query.

**An out-of-band issue is the exception, not an oversight.** Work filed with
`**Milestone:** Out of band` — harness and process fixes, a CI gap an
out-of-scope review finding produced — belongs to no milestone and so has no
tracking issue to tick (#373 and #391 are both filed this way, and neither
appears in #332). Filing one ends here.
