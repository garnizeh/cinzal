---
name: merge-closeout
description: Merge a Cinzal PR and do the bookkeeping that follows — squash-merge, verify the issue actually closed, update the milestone tracking issue, and confirm main is clean. Use when the user asks to merge, land, "fazer o merge", or close out a delivered PR.
---

# Merge and close-out

Merging is irreversible and outward-facing. **Confirm with the user before
merging** unless they have already said to land it; approval on one PR does not
carry to the next.

---

## 1. The gate: has a real review landed?

Do not merge on a green check. Run `coderabbit-triage` first and satisfy
yourself that:

- Every finding raised is either fixed or answered with reasoning.
- **No finding is still raised against the current head** — the one reliable
  signal, and it is negative.
- Every review conversation is resolved. `main` requires it, and this is what
  caught a factual error in the very first PR of the project.

If waiting for a review genuinely is not an option, **say so in the PR
description before merging**, so the commit on `main` records what went in
unreviewed rather than implying it did not.

## 2. Pre-merge check

```bash
rtk gh pr view <n> --json number,title,state,mergeable,reviewDecision,headRefName
rtk gh pr checks <n>
rtk gh pr view <n> --json body --jq .body | rtk grep -i "fixes #"
```

- All required checks green: the `check` job (or a legitimate path-gated skip),
  `secrets` (always runs), and `bench-compare` if the diff triggered it.
- **The `Fixes #<n>` line is present in the body.** A later edit has dropped it
  before; without it the issue stays open after merge.
- **The title and body are what you want in `git log`.** They become the commit
  verbatim. Trim the body now if review left it messy.

## 3. Merge

```bash
rtk gh pr merge <n> --squash --delete-branch
```

Squash-only, linear history. The branch deletes itself.

## 4. Close-out — same turn, every time

This is the part that gets forgotten. Do all four:

1. **Verify the issue actually closed.**
   ```bash
   rtk gh api repos/garnizeh/cinzal/issues/<n> --jq '{number,state}'
   ```
   If it is still open, close it and link the commit.

2. **Update the milestone tracking issue** — M3's is **#332**. Tick the row, note
   the merged PR. This has been needed unprompted twice (#265→#207 on filing,
   #355/#345→#332 on merge). Edit the body safely: `-f body=@file` does **not**
   expand the file — see [gh-recipes.md](../../reference/gh-recipes.md).

3. **Sync local `main`.**
   ```bash
   rtk git checkout main && rtk git pull --ff-only && rtk git log --oneline -3
   ```
   Confirm the squashed commit reads correctly.

4. **File anything deferred.** Out-of-scope review findings, follow-ups you
   promised in a reply, drift you noticed and did not fix. A promise in a review
   thread with no issue behind it disappears.

## 5. Report

State plainly: merged as `<sha>`, issue `#<n>` closed, tracking issue updated,
and anything filed as follow-up. If a step did not happen, say which and why.
