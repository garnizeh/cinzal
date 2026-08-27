---
name: coderabbit-triage
description: Read, verify and answer a CodeRabbit review on a Cinzal PR — including the findings that never become comments. Use when checking whether a review happened, when the user says "temos review" / "olha o coderabbit", after pushing a fix, or before merging. Encodes why a green CodeRabbit check means nothing here.
---

# CodeRabbit triage

CodeRabbit runs on the free OSS tier and frequently skips a PR with **"Review
limit reached"** — **and its status check reports success anyway.** On this
project that has been the common case, not the exception.

> **There is exactly one reliable signal, and it is negative: a finding still
> raised against the current head. That means not addressed.** The status check,
> the presence of a review, the absence of a limit message and the `Addressed`
> marker are all, on their own, inconclusive.

Four separate misreadings on this repository produced that sentence. Each was a
variation on: **absence of a signal is not evidence of a state.**

---

## 1. Collect every surface — there are three

Commands in [gh-recipes.md](../../reference/gh-recipes.md).

**a. The review objects.** List them; note which commits each covered.

```bash
rtk gh api repos/garnizeh/cinzal/pulls/<n>/reviews --jq '.[] | "\(.id)\t\(.user.login)\t\(.submitted_at)\t\(.state)"'
```

**b. THE REVIEW BODY — read this for every review whose body is non-empty.**

```bash
rtk gh api repos/garnizeh/cinzal/pulls/<n>/reviews/<review_id> --jq .body
```

A finding CodeRabbit cannot anchor to a diff position — typically a line
untouched by the commit range an incremental review diffed against — **never
becomes a comment at all.** It lands as an *"Outside diff range comments"* block
inside the review object's own `body`, with no comment id and nothing in
`pulls/<n>/comments`.

Found 2026-08-20 on PR #228: a full `pulls/<n>/comments` audit reported zero
unaddressed findings while a real one sat unposted in the review body, and the
user had to paste it by hand. **A comments-only audit reads as complete and
silently is not.**

**c. Positioned inline comments, and the PR-level comments** where "Review limit
reached" and `@coderabbitai` replies live.

## 2. Decide whether a review actually happened

- A **green check** tells you nothing.
- **"Limit reached" means *that attempt* was skipped**, not that the PR is
  unreviewed. A PR can carry a full review of its first commit and hit the limit
  on a later incremental one. Look at *which commits* the existing reviews
  covered, not at the most recent message as a verdict.
- **A clean incremental review posts nothing at all.** No review object on your
  latest commit proves nothing.
- **`✅ Addressed` is not guaranteed either.** When it appears it is good
  confirmation; its absence proves nothing. If you match on it in tooling, match
  `Addressed in commits? …` — the wording varies with commit count
  (`in commit <sha>` vs `in commits <sha> to <sha>`), and a pattern written for
  one form silently misreads the other as unaddressed. That mistake was made
  twice here, once in each direction.

On "Review limit reached": wait the stated refill (usually 20–45 min), then
comment `@coderabbitai review`. **"Already reviewed" is an answer, not a
refusal** — the review ran; look for `Addressed` markers instead of retrying.
When the message gives no refill time, the allowance is exhausted for longer and
there is nothing to do but wait. When a review genuinely was missed, the way to
get one is a new commit after the allowance returns, not repeating the command.

**Check proactively.** Poll after pushing; do not wait to be told.

If the user says "temos review", that is a checked fact — they looked and saw
one. Go fetch it and act; do not ask them to confirm.

## 3. Verify each finding before applying it

CodeRabbit is usually right here — it has caught real defects — but check every
finding against the specs.

- **Right about the problem, wrong about the fix** happens. Say so, and do the
  correct fix instead of adopting the suggestion.
- **Wrong findings get a reasoned reply.** It answers and concedes when it is
  wrong — it did on a suggestion that would have introduced `game.State`,
  inverting D01.
- **Findings point past their own file.** Twice here the real bug was a spec
  section or an unrelated issue carrying the same wrong statement. When a finding
  exposes a wrong statement, **grep for it elsewhere.**
- **Out-of-scope findings become issues.** A reply saying "pre-existing" is not
  enough on its own — file it and link the issue in the reply.

## 4. Verify, gate, push, then reply — in that order, same turn

**Before pushing: verify the fix against real behavior, then rerun `make
check` locally.** §3's verification is about the finding's *premise* — is
CodeRabbit right that there's a problem; this is about the *fix* — reproduce
the specific behavior the finding claims (a throwaway probe test, a doc
lookup, tracing the actual code path) rather than trusting the suggested diff
or the review's own reasoning at face value, and confirm the full gate suite
is still green before the commit goes out. This is deliberately **not** a
re-run of `delivery-review`'s full checklist — that is calibrated for opening
a PR, and repeating its fan-out/self-consistency/hygiene passes on every small
fixup would be ceremony, not signal. It is narrower: did you confirm this fix
actually does what you are about to claim it does, in the reply, to the
maintainer. Found on PR #366: the missing-host DSN gap (`postgres:///cinzal`
silently resolving through `PGHOST`) was confirmed with a throwaway probe test
against pgx's real `ParseConfig` behavior before `pool.go` was ever touched,
not by pattern-matching CodeRabbit's suggested diff.

**Reply right after pushing the fix. Do not wait for a second review pass
first** — that gate is for merging, not for replying.

For each finding:

1. Verify the fix for real, and rerun `make check`.
2. Push the fix.
3. Check whether the finding is still raised against the head.
4. If not, and you believe the fix is right: **resolve the thread and say *why*
   in the reply.** That is your judgement closing it rather than a confirmation,
   and the difference is worth putting on the record.

If a reply 404s, the thread went outdated after your push — post a **PR-level
comment** that quotes the finding instead.

A finding with no comment thread (a review-body nitpick) still gets a
**standalone PR comment** linking it to the fix. Never a silent fix.

## 5. Report

Tell the user, per finding: what it said, whether it was right, what you did,
and the thread's state. Then hand off.

## Then

→ `merge-closeout`, whose gate is: **a real review has landed.** If waiting is
genuinely not an option, say so **in the PR description**, so the commit on
`main` records what went in unreviewed.
