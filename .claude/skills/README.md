# Skill index — which skill, when

Thirteen skills covering the whole path from an issue to a merged commit. Each
is a `SKILL.md` under `.claude/skills/<name>/`, auto-discovered by Claude Code
and invocable as `/<name>`.

The pipeline itself — stage gates, hand-off artefacts, what may not be skipped —
is [`.claude/WORKFLOW.md`](../WORKFLOW.md). This file is the lookup table.

---

## The router

Start at the top and take the first row that matches.

| If the situation is… | Use |
|---|---|
| An issue number was named, or work is not tracked yet | [`issue-intake`](issue-intake/SKILL.md) |
| A brief exists and nothing has been edited yet | [`task-plan`](task-plan/SKILL.md) |
| Writing or editing Go | [`code-change`](code-change/SKILL.md) |
| Editing the GDD, RFC, roadmap, or repo docs | [`docs-change`](docs-change/SKILL.md) |
| The specs are silent, ambiguous, or contradict each other | [`decision-record`](decision-record/SKILL.md) |
| Proving a milestone exit criterion | [`exit-demo`](exit-demo/SKILL.md) |
| Writing or deepening tests | [`test-authoring`](test-authoring/SKILL.md) |
| Running `make check`, a gate, or reproducing red CI | [`gates-run`](gates-run/SKILL.md) |
| Benchmarks, or `bench-compare` went red | [`bench-run`](bench-run/SKILL.md) |
| The work is done and about to be published | [`delivery-review`](delivery-review/SKILL.md) |
| Opening the pull request | [`pr-publish`](pr-publish/SKILL.md) |
| A CodeRabbit review — reading it, answering it, or checking one exists | [`coderabbit-triage`](coderabbit-triage/SKILL.md) |
| Merging, and the bookkeeping after it | [`merge-closeout`](merge-closeout/SKILL.md) |

## By stage

```
INTAKE          issue-intake ──► task-plan
                     │
                     ├─ task (code) ──────► code-change ──┐
EXECUTE              ├─ task (docs) ──────► docs-change ──┤
                     ├─ decision ─────────► decision-record┤
                     └─ exit criterion ───► exit-demo ─────┤
                                                           │
VERIFY          test-authoring ◄──► gates-run ◄──► bench-run
                                     │
                                     ▼
LAND            delivery-review ──► pr-publish ──► coderabbit-triage ──► merge-closeout
```

`issue-intake` decides **which of the three work-item kinds** this is — task,
decision, or exit demonstration — and that choice picks the execute-stage skill.
Getting it wrong is the most expensive mistake in the pipeline: a task with no
spec anchor is a decision wearing a task's clothes, and executing it as a task
means inventing a requirement.

## Shared references

- [`.claude/reference/gh-recipes.md`](../reference/gh-recipes.md) — the `gh api`
  incantations that work here, including the ones that do **not**
  (`gh issue view` is broken; `-f body=@file` does not expand the file).

## Repository authority

The skills defer to these; they do not restate them.

| Document | Authoritative on |
|---|---|
| [`CLAUDE.md`](../../CLAUDE.md) | Repository state, constraints, agent conventions |
| [`CONTRIBUTING.md`](../../CONTRIBUTING.md) | The working agreement, the gates, the PR workflow |
| [`docs/project/cinzal-gdd.md`](../../docs/project/cinzal-gdd.md) | Game rules |
| [`docs/project/cinzal-architecture-rfc.md`](../../docs/project/cinzal-architecture-rfc.md) | Architecture |
| [`docs/project/cinzal-implementation-plan.md`](../../docs/project/cinzal-implementation-plan.md) | Sequencing and open decisions |
| [`docs/decisions/`](../../docs/decisions/) | Decisions D01–D52, and the format for the next one |

**Read the changelog before trusting a spec section.** Later entries correct
earlier ones, and both specs are heavily changelogged.

## Adding a skill

One skill is one stage of the pipeline with one output. If a new one does not
change what the next stage receives, it belongs inside an existing skill.

Frontmatter is `name` plus a `description` written for **auto-activation**: name
the artefacts, the commands, and the phrases that should trigger it. A
description that only describes the skill will not fire.

**Trigger phrases are deliberately bilingual, and that is not a slip.** The
repository is English-only — code, comments, commit messages, issue and PR text,
and the body of every skill here. Trigger phrases are the one exception, because
they are not prose: they are patterns matched against what the maintainer types,
and the maintainer types in Portuguese as readily as English. `"abrir o PR"`,
`"roda o make check"` and `"temos review"` earn their place the same way
`"open a PR"` does. Leave them; do not translate them away.
