---
name: issue-intake
description: Read a Cinzal issue and turn it into a work brief, or file a new one. Use when the user names an issue number ("vamos pegar a #319", "analisa a issue 325", "what does #328 ask for"), asks for a brief, asks to open/file an issue, or describes work that is not tracked yet. Also decides which of the three work-item kinds it is — task, decision, or exit demonstration — which determines every downstream step.
---

# Issue intake

The first stage. Produces a **brief**: a self-contained statement of what is
being asked, what governs it, and what "done" looks like. Nothing is planned or
written here.

`gh issue view` is broken on this repo — read issues with `gh api`, see
[gh-recipes.md](../../reference/gh-recipes.md).

---

## 1. Read the issue and everything it points at

```bash
rtk gh api repos/garnizeh/cinzal/issues/<n> --jq '{number,title,state,labels:[.labels[].name],body}'
rtk gh api repos/garnizeh/cinzal/issues/<n>/comments --jq '.[] | "--- \(.user.login)\n\(.body)"'
```

Comments are not decoration here — issues on this repo get amended in comments,
and a later comment often narrows or corrects the body. Read them all.

Then follow every link the issue makes:

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

## 5. Report and stop

Hand the brief to the user with a one-line recommendation of the next step. Do
not start planning; `task-plan` is the next skill, and it may want the user's
input on the unknowns first.

**If an unknown would make the work useless if guessed wrong, ask before
planning.** Everything else gets a stated assumption in the brief.

---

## Filing a new issue

When the work is not tracked yet, the same classification applies — pick the
matching template under `.github/ISSUE_TEMPLATE/` (`task.yml`, `decision.yml`,
`exit-demonstration.yml`) and fill **every** required field. An issue with no
spec anchor and no demonstrable acceptance criterion will not survive review.

Out-of-scope findings from a code review get filed too — a reply saying
"pre-existing" is not enough on its own; track it as a real issue and link it.

```bash
rtk gh issue create --title "<area>: <what>" --body-file /tmp/issue.md --label task
```

**After filing, update the milestone's tracking issue in the same turn** — that
has been forgotten twice. M3's is #332; see [gh-recipes.md](../../reference/gh-recipes.md).
