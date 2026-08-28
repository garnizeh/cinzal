---
name: coderabbit-triage
description: Read, verify and answer a CodeRabbit review on a Cinzal PR — including the findings that never become comments. Use when checking whether a review happened, when the user says "temos review" / "olha o coderabbit", after pushing a fix, or before merging. Encodes why a green CodeRabbit check means nothing here.
---

# CodeRabbit triage

CodeRabbit runs on the free OSS tier and frequently skips a PR with **"Review
limit reached"** — **and its status check reports success anyway.** On this
project that has been the common case, not the exception.

> **There are exactly two reliable signals, one negative and one positive: a
> finding still raised against the current head means not addressed; the
> literal text "No actionable comments were generated in the recent review.
> 🎉" in CodeRabbit's main PR comment means genuinely clean.** The status
> check, the presence of a review, the absence of a limit message and the
> `Addressed` marker are all, on their own, inconclusive.

Four separate misreadings on this repository produced that sentence. Each was a
variation on: **absence of a signal is not evidence of a state.**

---

## 1. Collect every surface — there are four

Commands in [gh-recipes.md](../../reference/gh-recipes.md).

**a. The main PR comment.** The first comment after the PR description,
present since the PR opened and edited in place on every review pass. This is
where the clean signal lives — fetch it fresh, do not rely on memory of what
it said on an earlier commit:

```bash
rtk gh api repos/garnizeh/cinzal/issues/<n>/comments --jq '.[0].body'
```

Its current body is expected to be one of three shapes: the exact string **"No
actionable comments were generated in the recent review. 🎉"** (genuinely
clean — [PR #378](https://github.com/garnizeh/cinzal/pull/378) is the
reference), a `Review limit reached` warning (that attempt was skipped), or a
findings summary (still has outstanding comments). Nothing else counts as
clean.

**If it matches none of the three, that is a fourth case with its own rule:
unknown, not clean, and not merely "not clean yet."** The clean gate is a
literal string against a third-party bot's output, so CodeRabbit rewording its
own message would leave this pipeline waiting forever on text that will never
appear again, retrying a review that already ran clean, with nothing
distinguishing that from a review still pending. So when the body fits no
known shape: **stop the loop, do not merge, and say so to the maintainer with
the body quoted.** An unrecognised state is a question for a human, not a
timer to keep re-arming — and if the wording genuinely has changed, the fix is
to update this skill's expected string, which is a harness change with its own
issue and its own PR (`WORKFLOW.md`, Stage 4), never a judgement call made
inline mid-merge.

**The clean text alone is not enough — bind it to the current head before
trusting it.** The comment is a single mutable object with no field of its own
recording which commit it describes, so a clean comment left over from an
earlier commit and a clean comment about the current one are indistinguishable
by text alone. Its body carries the commit range it reviewed inside a nested
`Commits` block (`Reviewing files that changed from the base of the PR and
between <base-sha> and <head-sha>.`) — extract that trailing sha and compare it
against the PR's actual current head:

```bash
rtk gh api repos/garnizeh/cinzal/issues/<n>/comments --jq '.[0].body' \
  | rtk grep -oP 'Reviewing files that changed from the base of the PR and between [0-9a-f]{40} and \K[0-9a-f]{40}'
rtk gh api repos/garnizeh/cinzal/pulls/<n> --jq .head.sha
```

If the two shas differ, or the `Commits` block can't be found at all, the
clean text is stale — **fail closed**: treat it the same as no review having
landed yet, not as confirmation.

**b. The review objects.** List them; note which commits each covered — this
tells you which commit the main comment's current state is actually about.

```bash
rtk gh api repos/garnizeh/cinzal/pulls/<n>/reviews --jq '.[] | "\(.id)\t\(.user.login)\t\(.submitted_at)\t\(.state)"'
```

**c. THE REVIEW BODY — read this for every review whose body is non-empty.**

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

**d. Positioned inline comments, and the later PR-level reply comments** where
`@coderabbitai` invocation replies live.

## 2. Decide whether a review actually happened

- A **green check** tells you nothing.
- **"Limit reached" means *that attempt* was skipped**, not that the PR is
  unreviewed. A PR can carry a full review of its first commit and hit the limit
  on a later incremental one. Look at *which commits* the existing reviews
  covered, not at the most recent message as a verdict.
- **A clean incremental review doesn't create a new `pulls/<n>/reviews`
  object** — so a bare review-list check can look empty on a genuinely clean
  head. It does edit the main PR comment to the exact clean text (§1a). That
  edit is the signal; the absence of a new review object is not.
- **`✅ Addressed` is not guaranteed either.** When it appears it is good
  confirmation; its absence proves nothing. If you match on it in tooling, match
  `Addressed in commits? …` — the wording varies with commit count
  (`in commit <sha>` vs `in commits <sha> to <sha>`), and a pattern written for
  one form silently misreads the other as unaddressed. That mistake was made
  twice here, once in each direction.

On "Review limit reached": **the comment states the exact wait, not a
range** — "Next included review available in *N* minutes." Parse that
number; do not substitute a guessed "20–45 min" window when an exact one is
printed right there. Wait exactly `N + 1` minutes (one minute of buffer, via
a timer/`Monitor` sleep for that duration — not a foreground sleep, not a
shorter poll-and-hope), then comment `@coderabbitai review` to request the
retry. **If that attempt reports rate-limited again, it carries its own new
stated `N`** — parse it and repeat the same wait-then-trigger cycle; this is
not an escalation or a backoff, just the same step run again with whatever
number the message gives this time. When the message gives no refill time at
all, the allowance is exhausted for longer — fall back to a longer guessed
wait (about 45 minutes) instead of a parsed one, but the trigger step still
happens at the end of it: comment `@coderabbitai review` once that wait
elapses, exactly as the parsed-time case does. Only the wait length is a
guess here, never whether the retry gets sent — a loop that only waits and
re-checks, with no trigger at the end, never gets a second attempt.
**"Already reviewed" is an answer, not a refusal** — the review ran; look
for `Addressed` markers instead of retrying.

**A new commit already triggers CodeRabbit's own review attempt on its
own** — if the wait window is spent pushing a fix rather than idling, do not
also comment `@coderabbitai review` for that same push; the explicit comment
is only for waking a PR that has no commit of its own to trigger one. When a
review genuinely was missed with no push to explain it, a new commit after
the allowance returns is the more natural way to get one, ahead of repeating
the command on an unchanged head.

**Check proactively.** Poll after pushing; do not wait to be told. Do it in the
background (`Monitor` with an `until`-loop against **the main issue comment**,
`gh api .../issues/<n>/comments[0]` — not `pulls/<n>/reviews`, since a clean
incremental review often edits that comment in place without creating a new
review object, so a reviews-list poll can sit past a review that already
landed — not a foreground sleep) so the wait — which can span the exact
refill window above, possibly more than once — doesn't stall the turn. Bind
whatever the comment says to the PR's current head SHA (§1a) before trusting
it.

## The merge gate this skill hands to `merge-closeout`

**"No comments visible" is never the merge signal on its own.** A rate-limited
skip and a genuinely clean review both show zero *new* comments — that is
exactly the ambiguity this whole skill exists to resolve. The gate
`merge-closeout` checks is a **positive, literal** one: the main PR comment
(§1a), read fresh against the current head, is exactly **"No actionable
comments were generated in the recent review. 🎉"** —
[PR #378](https://github.com/garnizeh/cinzal/pull/378) is the reference case.
Reaching that state is what authorizes the automatic merge (WORKFLOW.md,
"Running it end to end") — not silence, not a green check, not time elapsed,
and not "every thread got resolved" on its own: fixing findings from an
earlier review does not rewrite that comment, so a fresh review has to run
after the fix before the gate can be met.

**While rate-limited, this is a race, not a stall:** keep polling/retrying per
the paragraph above, in the background. If a subsequent review edits the main
comment to the exact clean text, the merge gate is met and `merge-closeout`
proceeds automatically. If the maintainer explicitly asks for the merge
first, that wins immediately and is acted on regardless of where the retry
cycle is — do not make an explicit merge request wait for one more polling
round.

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
4. If not, and you believe the fix is right: **reply saying why, and stop —
   do not resolve the thread yourself.** CodeRabbit resolves its own threads
   automatically, usually within a few minutes of the reply, once it agrees
   with the fix or the justification given. **If a thread is still open
   after a few minutes, that is the signal, not a stuck UI action** — it
   means CodeRabbit disagreed, and it posts its own reply explaining why.
   Read that reply and treat it as a fresh finding to address (§3's verify
   step again), not as a resolve step you forgot to do.

If a reply 404s, the thread went outdated after your push — post a **PR-level
comment** that quotes the finding instead.

A finding with no comment thread (a review-body nitpick) still gets a
**standalone PR comment** linking it to the fix. Never a silent fix.

## 5. Report

Tell the user, per finding: what it said, whether it was right, what you did,
and the thread's state. Then hand off.

## Then

→ `merge-closeout`, whose gate is: **the main PR comment reads exactly "No
actionable comments were generated in the recent review. 🎉" against the
current head.** If waiting is genuinely not an option, say so **in the PR
description**, so the commit on `main` records what went in unreviewed.
