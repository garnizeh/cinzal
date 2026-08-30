---
name: merge-closeout
description: Merge a Cinzal PR and do the bookkeeping that follows — squash-merge, verify the issue actually closed, update the milestone tracking issue, and confirm main is clean. Use when the user asks to merge, land, "fazer o merge", or close out a delivered PR.
---

# Merge and close-out

Merging is irreversible and outward-facing, but this pipeline carries a
standing authorization (2026-08-27, WORKFLOW.md "Running it end to end"): once
the gate below is met, merge runs automatically, same turn, with no separate
"go ahead" — the gate itself is the approval, checked, not asked for. What
still needs a human decision is *whether the gate is actually met*, not
whether to act once it is.

---

## 1. The gate: has a real review landed, and is it clean?

**The gate is a literal string, not a feeling.** CodeRabbit keeps one PR-level
comment — the first comment after the PR description, present since the PR
opened and edited in place on every review pass. Merge is authorized the
moment that comment's current body is, verbatim:

> No actionable comments were generated in the recent review. 🎉

[PR #378](https://github.com/garnizeh/cinzal/pull/378) is the reference case —
that exact text, and nothing else, is what a genuinely clean review looks
like. Do not merge on a green check, and do not merge on silence — a `Review
limit reached` skip and a genuinely clean review both look like "no new
comments" from the outside, but the main comment itself carries the tell: it
either still shows the rate-limit warning or a findings summary, or it shows
the clean line. That exact ambiguity is why `coderabbit-triage` exists. Run it
first and confirm, concretely:

- **The main comment, read fresh, against the PR's current head** — not a
  memory of what it said earlier, and not the presence/absence of new
  comments underneath it. The comment has no field recording which commit it
  describes, so the clean text alone doesn't prove it — extract the head sha
  from its nested `Commits` block and diff it against `pulls/<n>.head.sha`
  (`coderabbit-triage` §1a has the exact commands). A mismatch, or no
  `Commits` block at all, means the text is stale: **fail closed**, treat it
  as no review having landed.
- **If findings were raised and got fixed, that is necessary but not
  sufficient.** Resolving every thread and going green does not rewrite the
  main comment — it still records whatever the review that raised those
  findings said. A fresh review has to run after the fix, and the gate is
  only met once *that* review edits the main comment to the exact clean text.
- Every review conversation is resolved. `main` requires it, and this is what
  caught a factual error in the very first PR of the project.

**If the current main comment shows `Review limit reached` — not the clean
text — this is a race, not a stall, and not a reason to merge on the
silence:**

1. Wait the stated refill window and request a fresh review, polling in the
   background (`coderabbit-triage`'s own guidance on this).
2. **Whichever happens first wins:** the main comment updates to the exact
   clean text → the gate is met, merge proceeds automatically; or the
   maintainer explicitly asks for the merge → that request is acted on
   immediately, gate or no gate, without waiting for one more retry round.

Merging on an explicit ask before the clean text ever landed is still a
decision the maintainer made, not the pipeline inventing an exception —
record in the PR description that the exact text had not appeared, so the
commit on `main` says plainly what did or did not go in reviewed.

## 2. Pre-merge check

```bash
rtk gh pr view <n> --json number,title,state,mergeable,reviewDecision,headRefName
rtk gh pr checks <n>
rtk gh pr view <n> --json body --jq .body | rtk grep -i "fixes #"
```

- **All required checks green.** GitHub enforces six — `check`, `secrets`,
  `bench-compare`, `vuln`, `replay (ubuntu-latest)`, `replay (macos-latest)`.
  A path-gated job still reports `success` rather than skipping, so a
  docs-only PR satisfies them all normally. There are no rulesets adding to
  the list; read it rather than trusting this line:

  ```bash
  rtk gh pr checks <n>
  rtk gh api repos/garnizeh/cinzal/branches/main/protection --jq .required_status_checks.contexts
  ```

  `bench` appears in `gh pr checks` as `skipping` on every PR and is not
  required — that is by design (`push`-only), not a failure to investigate.
- **The `Fixes #<n>` line is present in the body.** A later edit has dropped it
  before; without it the issue stays open after merge.
- **The title and body are what you want in `git log`.** They become the commit
  verbatim. Trim the body now if review left it messy.

## 3. Merge

```bash
rtk gh pr merge <n> --squash --delete-branch
```

Squash-only, linear history. The branch deletes itself.

**A non-zero exit or an error message here is not proof the merge failed.**
`gh pr merge` also tries to sync the local checkout (fast-forward `main`,
delete the local branch), and that local step can fail — e.g. uncommitted
changes elsewhere in the working tree, a branch checked out in another
worktree — *after* the remote squash-merge has already succeeded through
GitHub's API. The error text reads exactly like a full failure either way.
Check the real state before assuming either outcome:

```bash
rtk gh pr view <n> --json state,mergedAt,mergeCommit
```

`state: "MERGED"` means it merged regardless of what the local error said —
do not retry the merge command. `state: "OPEN"` means the merge itself
actually failed — that is the only case a retry belongs to. A third value,
`state: "CLOSED"` with `mergedAt` null, means the PR was closed without
merging (by a human, or some other action) — stop the close-out entirely and
report that plainly; retrying the merge on a closed PR is not the fix. This
happened for real, twice in one session (#397, #398 — see #399/#401): the
local error was mistaken for a stalled merge needing a retry, until the
second occurrence made the pattern visible.

## 4. Close-out — same turn, every time

This is the part that gets forgotten. Do all five:

1. **Verify the issue actually closed.**
   ```bash
   rtk gh api repos/garnizeh/cinzal/issues/<n> --jq '{number,state}'
   ```
   If it is still open, close it and link the commit.

2. **Update the milestone tracking issue** — whichever `CLAUDE.md`'s
   "Repository state" names, read there rather than from a number pasted into
   this file. Tick the row, note the merged PR. This has been needed unprompted
   twice (#265→#207 on filing, #355/#345→#332 on merge). Edit the body safely:
   `-f body=@file` does **not** expand the file — see
   [gh-recipes.md](../../reference/gh-recipes.md).

   **Skip this step for an out-of-band PR** — a harness or process fix filed
   with `**Milestone:** Out of band` belongs to no milestone and has no row to
   tick. Say so in the report rather than leaving the step unaccounted for.

3. **Sync local `main`.**
   ```bash
   rtk git checkout main && rtk git pull --ff-only && rtk git log --oneline -3
   ```
   Confirm the squashed commit reads correctly.

4. **File anything deferred.** Out-of-scope review findings, follow-ups you
   promised in a reply, drift you noticed and did not fix. A promise in a review
   thread with no issue behind it disappears.

5. **Write a journal entry** — `.claude/journal.md`, local and gitignored,
   never committed (no branch-protection concern applies: this file never
   enters git). This is additive to the chat-turn report below, not a
   replacement of it — the durable record for when nobody was watching the
   session live.

   **If `.claude/journal.md` does not exist yet** (a fresh checkout, or it
   was deleted — do not assume it survives), create it first with this exact
   header, verbatim, so the file is self-describing even without this skill
   open:

   ```markdown
   # Task execution journal

   Local, async-monitoring log — never committed (`.claude/journal.md` in
   `.gitignore`). Written by `merge-closeout`
   (`.claude/skills/merge-closeout/SKILL.md`), one entry per merged task,
   **most recent entry first**. This is additive to the chat-turn summary
   `merge-closeout` already gives, not a replacement of it — the durable
   record for when nobody was watching the session live.

   ---
   ```

   **Then, every time, prepend one entry** directly below that header (above
   any existing entries — most recent first):

   ```markdown
   ## #<n> — <issue title>

   - **Merged:** <UTC ISO 8601> as `<sha>` (PR #<pr-number>)
   - **Issue closed:** <UTC ISO 8601 — record separately even if it matches Merged>

   <the same execution-detail content the chat-turn report gives: what was
   built, what review found and how it was resolved, what landed, anything
   still open>

   ---
   ```

   Use real UTC timestamps — `rtk date -u +%Y-%m-%dT%H:%M:%SZ` — not an
   estimate.

## 6. Report — and close the whole pass, not just this stage

State plainly: merged as `<sha>`, issue `#<n>` closed, tracking issue updated,
journal entry written, and anything filed as follow-up. If a step did not
happen, say which and why.

**When this merge is the end of a pipeline run that started at `issue-intake`
or `task-plan`**, this is also the pipeline's own final step (WORKFLOW.md,
"Running it end to end"): give one summary of the whole pass — what was
built, what review found and how it was resolved (or why it merged without
one, if that's how it went), what landed, and anything still open. Not a
restatement of every tool call — the shape of what happened, for someone who
was not watching it happen.
