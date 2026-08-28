---
name: pr-publish
description: Open the pull request for a finished Cinzal change — branch, commit, the description that becomes the commit message, the Fixes keyword, and the milestone tracking issue. Use when the user asks to publish, open a PR, "abrir o PR", "publicar a entrega", or push the work for review.
---

# PR publish

Everything lands through a PR. `main` is protected for the maintainer too:
squash-only, linear history, every review conversation resolved before merge.

**The PR title becomes the commit subject and the PR body becomes the commit
body.** Write both for whoever reads `git log` in a year and for whoever `git
bisect` lands there.

---

## 1. Branch and commit

Branch prefixes in use: `task/`, `decision/`, `exit/`, `fix/`, `docs/`,
`chore/`. Naming is a convenience, not a gate.

```bash
rtk git checkout -b task/<short-slug>
rtk git add <paths>          # never `git add -A` — read what you are staging
rtk git commit -m "<subject>"
rtk git push -u origin HEAD
```

End the commit message with:

```
Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
```

## 2. Write the body

Subject conventions from `git log`:

- `<area>: <what changed>` — e.g. `rules: …`, `ci: …`, `docs: …`, `telemetry: …`
- `Dnn: <the answer in one line>` for a decision
- `Exit demo: <the criterion, met>` for a demonstration

Body — this is the commit message, so no scaffolding, no "as requested":

```markdown
## Summary

- What changed, and why it was needed. Bullets, each a complete thought.
- The reasoning that will not be re-derivable from the diff: the case it
  closes, the alternative rejected, the algebra showing the steady state is
  unchanged.
- Which specs/decisions moved, with revisions (GDD v2.xx, RFC rNN).

## Test plan

- [x] `make check` green — or, for a docs-only diff, explicitly skipped per repo
      convention, with what was verified instead.
- [x] The specific things you traced or ran, named.

Fixes #<n>

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

**No manual line-wrap inside a paragraph or bullet.** One physical line per
paragraph, per bullet — let the renderer wrap it, never a hand-inserted
newline at ~80 columns. GitHub (and `git log`) render a soft-wrapped
paragraph as the intended block; a paragraph hard-wrapped in the source
renders as broken, ragged lines instead. This has recurred more than
once — "eyeball the raw text" is not enough, it has already failed twice.
Run a mechanical check before every publish or body edit, not just a read-through:

```bash
rtk python3 -c "
lines = open('/tmp/pr-body.md').read().split(chr(10))
run = maxrun = 0
for l in lines:
    if l.strip() and 60 <= len(l) <= 100 and not l.strip().startswith(('-', '*', '#', '\`\`\`')):
        run += 1; maxrun = max(maxrun, run)
    else:
        run = 0
print('longest wrapped-looking run:', maxrun, '(3+ means still wrapped)')
"
```

A result of 3 or more means the file is still hard-wrapped — fix it before
`gh pr create`/`gh pr edit` ever runs, not after. Tables and code blocks are
exempt — they need their own literal line breaks, and will trip this
heuristic harmlessly if they happen to fall in that length range; use
judgement on those, not the raw number.

**The literal `Fixes #<n>` line is what auto-closes the issue.** It has been
missing at creation and dropped by a later edit — both left the issue open after
merge. Put it in, and verify it is still there after any body edit.

```bash
rtk gh pr create --base main --title "<subject>" --body-file /tmp/pr-body.md
rtk gh pr view <n> --json body --jq .body | rtk grep -i "fixes #"
```

Write the body with the Write tool or `python3`, not `rtk … > file` — rtk's
truncation footer can corrupt a redirected file.

## 3. Checklist in the template

`.github/pull_request_template.md` carries two boxes. Tick them honestly:

- The acceptance criterion in the linked issue is met.
- Standing obligations reviewed.

## 4. Update the milestone tracking issue — same turn

Filing and merging both require it, and it has been forgotten twice. M3's
tracking issue is **#332**. See [gh-recipes.md](../../reference/gh-recipes.md)
for finding the current one and for editing an issue body safely (`-f body=@file`
does **not** expand the file).

## 5. Watch CI, then wait for review

```bash
rtk gh pr checks <n>
rtk gh pr view <n> --json state,mergeable,reviewDecision
```

Fix red checks before asking for a review. Then hand off to `coderabbit-triage`
— **and go look proactively**; do not wait to be told a review exists. Poll for
the review in the background (`Monitor`, not a foreground sleep) rather than
stalling the turn — see WORKFLOW.md's "Running it end to end."

**Do not merge here.** Merging is `merge-closeout`, and it runs automatically
the moment `coderabbit-triage`'s gate is met — CodeRabbit's main PR comment
reads exactly "No actionable comments were generated in the recent review.
🎉" against the current head. That gate is a fact to check, not a fresh "go
ahead" to wait for.

---

## Confirm before pushing — unless the repo's own gate already ran

Opening a PR is outward-facing. **The exception: once this change has gone
through the repo's own review/verification steps** — `delivery-review`'s
checklist run and clean, or `make check` green where it applies — that *is*
the approval, and it is enough to push and open the PR without a separate
yes/no. The maintainer said so explicitly (issue #350/D53, 2026-08-27): asking
again on top of a passed `delivery-review` is friction, not safety.

Ask first only when publishing **without** having gone through that gate —
skipping straight from an edit to a PR with no review step in between.
Approval to open one PR under this rule is not blanket approval to skip
`delivery-review` itself; the gate still has to run, it just doesn't also
need a chat confirmation once it has.
