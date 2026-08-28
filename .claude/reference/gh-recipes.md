# `gh` recipes for this repository

Shared by `issue-intake`, `pr-publish`, `coderabbit-triage` and `merge-closeout`.
Every command is prefixed with `rtk` per [CLAUDE.md](../../CLAUDE.md).

`REPO` below is `garnizeh/cinzal`.

---

## Issues

**The installed `gh` is old, and that — not the repository, and not GraphQL —
is what breaks.** `gh 2.45.0` hardcodes `repository.issue.projectCards` in its
`issue view` query, a field GitHub has since deprecated, so raw `gh issue view`
fails with `GraphQL: Projects (classic) is being deprecated`. Two things follow
that are easy to over-generalise from and have been:

- **GraphQL itself works.** A hand-written `gh api graphql` query against this
  repository returns normally — verified 2026-08-28. The thread-state query at
  the bottom of this file is fine. It is `gh`'s own baked-in queries that carry
  the dead field, not anything about this account.
- **`rtk gh issue view <n>` works too**, because `rtk`'s filter answers it
  without the deprecated query. Only the raw form (`rtk proxy gh issue view`)
  fails.

Prefer `gh api` below anyway: it is explicit about the fields it asks for, so
it cannot be broken by a client-side query going stale the way `issue view`
was. If `gh` is ever upgraded past this, re-verify and simplify this section
rather than leaving a fixed problem documented as live.

```bash
# Read one issue, body included
rtk gh api repos/garnizeh/cinzal/issues/<n> --jq '{number,title,state,labels:[.labels[].name],body}'

# Just the body, unescaped
rtk gh api repos/garnizeh/cinzal/issues/<n> --jq .body

# Comments on an issue
rtk gh api repos/garnizeh/cinzal/issues/<n>/comments --jq '.[] | "--- \(.user.login)\n\(.body)"'

# Open issues, pull requests filtered out
rtk gh api "repos/garnizeh/cinzal/issues?state=open&per_page=100" \
  --jq '.[] | select(.pull_request|not) | "\(.number)\t\(.title)"'

# Cross-references: what links to this issue
rtk gh api repos/garnizeh/cinzal/issues/<n>/timeline --jq '.[] | select(.event=="cross-referenced") | .source.issue.number'
```

## Writing bodies — never `-f body=@file`

`rtk gh api -f key=@file` **does not expand the file**; it sends the literal
string `@path`. Two safe shapes, both verified after the call:

```bash
# 1. Build the JSON with python3, pipe it in
rtk python3 - <<'PY' > /tmp/payload.json
import json, pathlib
print(json.dumps({"body": pathlib.Path("/tmp/body.md").read_text()}))
PY
rtk gh api repos/garnizeh/cinzal/issues/<n> -X PATCH --input /tmp/payload.json

# 2. gh pr create / gh issue create read a file directly and are fine
rtk gh pr create --title "..." --body-file /tmp/body.md
```

**Never `rtk <cmd> > file` for large output.** rtk's truncation-notice footer can
land inside the redirected file and corrupt it. Write payloads with the Write
tool or `python3`, then `rtk jq . <file>` to prove it parses before sending.

**The `/tmp/...` paths below are placeholders.** Write bodies and payloads into
the session's own scratchpad directory instead — it is isolated per session, so
two concurrent passes cannot overwrite each other's `/tmp/pr-body.md`.

**Always read back what you wrote** (`--jq .body`) and confirm it is what you meant.

## Pull requests

```bash
# Create — body file, so the description survives verbatim into the commit
rtk gh pr create --base main --title "<subject>" --body-file /tmp/pr-body.md

# Status, checks, mergeability
rtk gh pr view <n> --json number,title,state,mergeable,reviewDecision,headRefName
rtk gh pr checks <n>

# Edit the body later (the `Fixes #N` line must survive the edit)
rtk gh pr edit <n> --body-file /tmp/pr-body.md

# Squash-merge and delete the branch
rtk gh pr merge <n> --squash --delete-branch
```

## CodeRabbit review objects

Positioned comments and the review body are **two different surfaces**. A finding
that cannot be anchored to a diff position lands only in the review body.

```bash
# Every review on the PR
rtk gh api repos/garnizeh/cinzal/pulls/<n>/reviews --jq '.[] | "\(.id)\t\(.user.login)\t\(.submitted_at)\t\(.state)"'

# THE REVIEW BODY — read this for every review whose body is non-empty
rtk gh api repos/garnizeh/cinzal/pulls/<n>/reviews/<review_id> --jq .body

# Positioned inline comments (NOT the complete set of findings)
rtk gh api "repos/garnizeh/cinzal/pulls/<n>/comments?per_page=100" \
  --jq '.[] | "\(.id)\t\(.path):\(.line)\t\(.user.login)\n\(.body)\n"'

# PR-level (issue) comments — where "Review limit reached" and @coderabbitai replies live
rtk gh api "repos/garnizeh/cinzal/issues/<n>/comments?per_page=100" \
  --jq '.[] | "--- \(.user.login) \(.created_at)\n\(.body)"'

# Reply into an existing review thread
rtk gh api repos/garnizeh/cinzal/pulls/<n>/comments/<comment_id>/replies -X POST --input /tmp/reply.json

# Thread ids, with resolution state (read-only diagnostic — see note below)
rtk gh api graphql -f query='
query { repository(owner:"garnizeh",name:"cinzal") { pullRequest(number:<n>) {
  reviewThreads(first:100) { nodes { id isResolved isOutdated comments(first:1){nodes{body path}} } } } } }'
```

If a reply 404s, the thread went outdated after your push. Post a PR-level
comment that quotes the finding instead — a finding never gets a silent fix.

**Never resolve a thread yourself.** Reply with the fix or the justification
and stop there — CodeRabbit resolves its own threads automatically, usually
within a few minutes, once it agrees. A thread still open after that means
it disagreed and will explain why in a reply of its own (`coderabbit-triage`
§4). The resolution-state query above is diagnostic only, for checking what
CodeRabbit has already done — not a step to act on with a resolve mutation.

## Milestone tracking issues

Every filing and every merge of **milestone work** updates that milestone's
tracking issue in the same turn. The current one is whichever `CLAUDE.md`'s
"Repository state" paragraph names — read it there, not from a number pasted
here. Out-of-band work (`**Milestone:** Out of band`) has no tracking issue and
skips this step.

```bash
# Find the current one
rtk gh api "repos/garnizeh/cinzal/issues?state=open&per_page=100" \
  --jq '.[] | select(.pull_request|not) | select(.title|test("^M[0-9.]+ ")) | "\(.number)\t\(.title)"'
```
