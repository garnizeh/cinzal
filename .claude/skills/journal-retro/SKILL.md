---
name: journal-retro
description: Mine .claude/journal.md for process and harness learnings that have not actually been folded into WORKFLOW.md, a skill, CLAUDE.md, or auto-memory yet — recurring root causes across entries, harness notes marked unresolved, and fixes an entry claims but that current files don't actually show. Use when the user asks to review the journal, look for lessons learned, "revisar o journal", "tem algum aprendizado", "o que podemos melhorar no fluxo", or periodically after a batch of merges.
---

# Journal retrospective

`merge-closeout` writes one journal entry per merged task — a local,
gitignored, never-committed log — but nothing reads it back. Real learnings
sit inside individual entries' prose (a "Harness note:", a root cause named
once and then hit again three merges later) with no mechanism turning them
into a durable fix. This skill is that mechanism: it reads the whole journal,
finds what is still actionable, and turns confirmed findings into an edit to
the repo or a memory entry — not a summary of what already happened.

This is a meta skill like `loop-dispatch`, not a pipeline stage: run it
on demand, or periodically after a run of merges, never as a side effect of
closing one task.

**The value is synthesis, not transcription.** Re-stating each entry's
"what was built" is `merge-closeout`'s job already, done at write time. This
skill exists for the things visible only across entries or only against
today's files — do not produce a table of contents of the journal.

---

## Step 0 — read the whole journal

`.claude/journal.md` in full, oldest and newest entries alike. It is short
enough to read whole; do not sample recent entries and call it coverage —
the pattern this skill exists to catch is exactly the one that repeats far
enough apart that a partial read misses it (see the `blocked`-label example
below, three entries apart).

## Step 1 — pull candidate signals

Three shapes are worth pulling out. Everything else in an entry is
already-landed work history — leave it.

1. **Explicit harness/process notes** — text under "Harness note:", "Harness
   bug found and fixed", "Real harness/process fixes landed", or similar.
   These are self-flagged by whoever wrote the entry as process learnings,
   not task content.
2. **Flagged-unresolved items** — phrasing like "unresolved", "worked around
   ... get the maintainer's read on this before assuming", "whether X is a
   real boundary is unresolved". These are open questions someone deferred
   rather than decided.
3. **Cross-entry recurring root causes** — the same underlying cause named
   independently in two or more entries, even when each entry treats it as
   a one-off. This is the highest-value category and the one a single-entry
   read cannot surface: e.g. three consecutive M3 entries (#313, #314, #315)
   each separately discover that an issue's `blocked` label was stale and
   the real blockers were already closed — no single entry calls this a
   pattern, but three independent occurrences is a real gap in how blocked
   status gets checked, not noise.

## Step 2 — verify against present-day state, not the entry's claim

A journal entry records what someone believed was true, or did, at write
time — treat it as a lead, not a fact. For every candidate from Step 1:

- If the entry says a file was updated ("Updated `WORKFLOW.md`,
  `coderabbit-triage/SKILL.md`..."), grep that file now for the claimed
  content. Files drift after the entry was written; a fix an entry describes
  as landed can have been reverted, superseded, or never actually match what
  the entry says.
- Check `.claude/projects/*/memory/MEMORY.md` and its linked files — a
  cross-entry pattern may already be captured there (the `blocked`-label
  example above already is:
  `feedback-always-run-issue-intake-selector.md`). Do not re-propose a fix
  that already exists as a memory or a skill line; confirm it's still
  present and move on.
- If neither the repo nor memory reflects it, it's a live candidate.

## Step 3 — report before editing

Present findings in three buckets, most actionable first:

1. **Genuine gaps** — verified absent from both the repo and memory, with a
   concrete proposed edit: which file, what text, why (cite the journal
   entry/entries backing it).
2. **Open questions the maintainer deferred** — surface as a question, don't
   decide silently. This is the same boundary `decision-record` draws for
   specs: a real ambiguity gets asked, not guessed.
3. **Already handled** — brief, for confidence that the read was thorough,
   not as filler.

Skip a candidate entirely rather than reporting it if it's a one-off already
fully resolved within the same entry (e.g. a bug found and fixed same-PR,
with nothing left open) — that's normal task execution, not a process gap.

## Step 4 — apply what's confirmed

- A repo-documentable fix (a stale count, a missing cross-reference, a
  process rule that should exist in `WORKFLOW.md` or a skill but doesn't)
  lands through the normal path: edit directly for something as
  mechanical as a stale number or a missing link; route anything that
  changes actual pipeline behavior or a gate through `docs-change`'s
  conventions and land it via `pr-publish`. **File an issue for it first**,
  per `WORKFLOW.md`'s Stage 4 — `**Milestone:** Out of band`, no GDD/RFC spec
  anchor (a process fix isn't about game content and doesn't have one), every
  other field in CONTRIBUTING's issue table still filled — and have the PR
  close it with the literal `Fixes #<n>`. Earlier process PRs landed with no
  issue behind them (#382, #385, #370); Stage 4 was written specifically to
  end that, so they are precedent for the shape of the PR, not for skipping
  the issue. Never batch an unrelated code task into this PR.
- A learning about collaboration style, standing preference, or project
  state rather than repo content belongs in auto-memory instead, per the
  memory system's own type rules — not every finding here is a file edit.
- An open question from bucket 2 that the maintainer answers on the spot:
  apply the same turn, same as any other decision made in conversation.
- **Never edit `.claude/journal.md` itself.** It's an append-only log owned
  by `merge-closeout`; this skill only reads it.

## Boundaries

- Don't re-litigate a decision the maintainer already made deliberately —
  only flag it if the journal itself shows the decision was never actually
  made (bucket 2), not because you'd have chosen differently.
- Don't manufacture a pattern out of a single entry. Cross-entry recurrence
  needs at least two independent occurrences; one instance is task history.
- Don't let this run block or gate anything else — it produces findings and,
  where confirmed, edits; it never blocks a merge or a pipeline stage.
