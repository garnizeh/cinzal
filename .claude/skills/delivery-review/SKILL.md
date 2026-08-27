---
name: delivery-review
description: Self-review a finished Cinzal change before opening the PR — the diff against its issue, the standing obligations, the fog question, the spec fan-out, and whether it still lands as one coherent commit. Use when work is done and about to be published, or when the user asks to review the delivery, "revisar a entrega", before a PR.
---

# Delivery review

The last stage before the change becomes public. Read your own diff as the
reviewer who will land on it with `git bisect` in a year.

```bash
rtk git diff main...HEAD --stat
rtk git diff main...HEAD
rtk git status
```

Read the **whole** diff, not the summary. Most defects caught here are in a file
you did not intend to touch.

**This review is meant to be meticulous, not a confidence check.** "I traced
the logic and it's sound" is a different claim from "I reread the words I
wrote and they say that." Both misses that motivated §4's and §5's checks below
happened because the reasoning was right and nobody reread the literal
sentence or ran the literal grep against it — being satisfied that you
understand the change is exactly the moment this review exists to not trust.
Every step below is a concrete action (grep this, reread that sentence against
that source), not a vibe check — if a step doesn't leave something you could
show someone else, it wasn't done.

---

## 1. Does it answer the issue?

- The **acceptance criterion** from the issue, restated, and the concrete thing
  that demonstrates it is met. If it is not met, the change is not finished —
  say which part and why, and do not narrow the scope silently.
- The **spec anchor** still governs what was built. If the implementation drifted
  from the cited section, either the code is wrong or the section needed a
  `docs-change` first.
- Nothing beyond the issue's scope crept in. Out-of-scope fixes get their own
  issue and their own PR.

## 2. The fog question

**Does this leak state past the fog boundary?** Ask it explicitly, in writing,
even when the diff touches nothing that looks fog-adjacent.

- No path hands out more than `Project` permits.
- `render`/`web` name nothing from `internal/rules`.
- New `PlayerView` fields have a negative fog test asserting **absence**.

## 3. Standing obligations — walk all five

| Obligation | Discharged by |
|---|---|
| Added a `PlayerView` field | A negative fog test in this PR |
| Can disclose a player's position | A row in the RFC §9.1 authorised-writer table |
| Consumes randomness | A row in the RFC §6.4 table **and** an index-count assertion, truncation cases included |
| Changed a game rule or architectural decision | The GDD/RFC changed **first**, with a changelog entry and a revision bump |
| Added a tunable number | It is a `Config` field, never a constant |

Each one prevents a failure that is otherwise silent — nothing crashes, a
guarantee just quietly stops holding.

## 4. Fan-out

Grep for anything you corrected elsewhere in the tree. A wrong statement has
twice, on this repo, also lived in another document or issue. Check:
`docs/decisions/README.md` catalogue rows, roadmap §3 status lines, `CLAUDE.md`'s
"Repository state" paragraph, `CONTRIBUTING.md`'s gate table, `Makefile` target
comments, code comments quoting a spec sentence.

**This covers more than wrong statements — it covers every fact you changed,
including bare numbers.** A revision bump, a decision count, an issue range: if
one file states it, grep the whole repo for the old value before calling the
PR done, not just the files you already know cite it. `rtk grep -rn "<old
value>"` is one command; missing a second citation site is a second review
round. Found on PR #362 (D53): the RFC's own header was bumped r47 → r48, but
`CLAUDE.md`'s document index cites the same revision number on its own line,
one line away from the decision-range line that *was* checked — and it was
missed because the check was scoped to "what quotes D19's shape," not to "what
else states this revision number."

New `EventKind`? Confirm it **appends at the end of the `iota` block** — a
mid-block insert shifts later ordinals into `Anchor.Kind`.

Touched `docs/project/cinzal-gdd.md` or `docs/project/cinzal-architecture-rfc.md`?
Confirm the changelog entry and the revision line at the top (`v2.xx` / `rNN`)
actually moved, not just the prose below it — `docs-change` mandates the bump
on the way in, this is the check that catches it if it didn't land. A
substantive body edit with a stale header is the same silent-drift failure
this section exists to catch, just inside one document instead of across
several, and it applies whether the PR's own kind is `docs-change` or a task
that edited the spec first per Stage 2's "documents change first" rule.

## 5. Self-consistency — does the new prose say what the new decision says?

When a PR carries both a `docs/decisions/Dnn-*.md` and the RFC/GDD prose it
mandates, these are two independently-written artifacts, and *tracing the
decision's own logic against a scenario is not the same check as confirming
the RFC sentence you wrote states that logic correctly.* Reread every new RFC/GDD
sentence against the decision's own Decision/Consequences section, literally,
looking for a clause that quietly widens or drops an exemption the decision
stated.

The failure shape to watch for: a sentence that bundles two predicates or two
axes together ("gains predicates X and Y, exempting Z from the reasoning given
for X") and only states the exemption reasoning for one of them, silently
implying the other applies unconditionally. Found on PR #362 (D53): the
decision doc correctly stated `autopilot` is exempt from *both*
`email_pref` and the new `email_suppressed_at`; the RFC §13.1 paragraph
describing the same enqueue predicate bundled both under one exemption clause
that only justified the eligibility half, so as literally worded it applied
`email_suppressed_at` to `autopilot` too. The scenario trace that would have
caught this ("does Autopilot still fire after a global unsubscribe?") had
already been run correctly against the *decision* — it was never rerun against
the *sentence*.

## 6. Hygiene

- **Probe edits reverted.** Anything you patched to test a gate or hypothesis is
  gone. `rtk git diff main...HEAD -- internal/` should contain nothing you cannot
  justify.
- **No build artifacts.** Binaries go in `bin/`; check none landed at the root.
- **No `*.bench`, no scratch files, no temp payloads** in the diff.
- **Everything committed is English** — code, comments, docs, and the PR text
  you are about to write.
- **Comments say why**, and cite the decision or spec section, matching the
  density of the surrounding code.

## 7. Is it one commit?

**One task = one pull request = one commit on `main`.** Sizing test: *does this
land as one PR that leaves `main` green by itself?* If not, split it now — after
the PR is open it is more expensive.

## 8. Verification is real

`make check` green (`gates-run`), or — for a docs-only diff — explicitly skipped
with what you verified instead written down. Report the outcome faithfully: if
something failed, say so with the output; if a step was skipped, say that.

## Then

→ `pr-publish`.
