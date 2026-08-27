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

New `EventKind`? Confirm it **appends at the end of the `iota` block** — a
mid-block insert shifts later ordinals into `Anchor.Kind`.

## 5. Hygiene

- **Probe edits reverted.** Anything you patched to test a gate or hypothesis is
  gone. `rtk git diff main...HEAD -- internal/` should contain nothing you cannot
  justify.
- **No build artifacts.** Binaries go in `bin/`; check none landed at the root.
- **No `*.bench`, no scratch files, no temp payloads** in the diff.
- **Everything committed is English** — code, comments, docs, and the PR text
  you are about to write.
- **Comments say why**, and cite the decision or spec section, matching the
  density of the surrounding code.

## 6. Is it one commit?

**One task = one pull request = one commit on `main`.** Sizing test: *does this
land as one PR that leaves `main` green by itself?* If not, split it now — after
the PR is open it is more expensive.

## 7. Verification is real

`make check` green (`gates-run`), or — for a docs-only diff — explicitly skipped
with what you verified instead written down. Report the outcome faithfully: if
something failed, say so with the output; if a step was skipped, say that.

## Then

→ `pr-publish`.
