---
name: docs-change
description: Edit Cinzal's specification and project documents — the GDD, the architecture RFC, the implementation roadmap, CLAUDE.md, CONTRIBUTING.md, README, or the decision-log catalogue. Use for any prose task, spec correction, or changelog entry. Not for authoring a new decision document (that is decision-record).
---

# Docs change

The documents are the authority: the **GDD on game rules**, the **RFC on
architecture**, the **roadmap on sequencing and open decisions**. Code follows
them, not the other way round.

---

## Before editing

**Read the changelog at the top of the document first.** Both specs are heavily
changelogged, and the changelog records *why* — usually the part that matters.
Later entries correct earlier sections, so a section can be stale while reading
as authoritative. If a rule looks odd, it is probably closing a loophole found
in design review and the changelog says which one.

Check the RFC's "Companion doc" header: it names the GDD revision it is paired
with. If the GDD has moved past it and your edit depends on the difference, that
mismatch is part of your change.

**Search for the statement you are about to correct.** A wrong sentence in
one document has, twice on this repo, also existed in another — the real bug
was a spec section carrying the same wrong claim somewhere else. `grep` a
paraphrase or reworded restatement as well as the literal text; search the
GDD, the RFC, the roadmap, `docs/decisions/`, `CLAUDE.md` and the code
comments — **and any open GitHub issue you have already read in this session
that quoted the same text** (an issue's spec-anchor line often echoes the
sentence verbatim; `gh api`/`gh search` is the right tool for that part).
Missing this on the first pass turned one correction into three follow-up
rounds once.

## Making the edit

**Every substantive spec edit carries a changelog entry** at the top of that
document, and bumps the revision (GDD `v2.xx`, RFC `rNN`). The entry says what
changed and *why*, and names the issue or decision driving it.

**Correct, do not rewrite.** These documents record rejected options and the
argument for the chosen one — that reasoning outlives the verdict and is the
single most valuable property they have. Preserve it. A superseded decision gets
a pointer forward, not a deletion.

**A decision that turns out wrong is superseded, not edited.** Leave the
original standing.

Fan-out to check on every edit:

| Edited | Also update |
|---|---|
| A GDD rule | RFC sections that restate it; `docs/decisions/` documents that cite it; code comments quoting it |
| RFC §6.4 (RNG table) | The index-count assertions in `internal/rules` tests |
| RFC §9.1 (authorised writers) | The fog tests that assert the row |
| A decision's status | `docs/decisions/README.md` catalogue row **and** the roadmap §3 status line |
| Milestone state | `CLAUDE.md`'s "Repository state" paragraph; the roadmap section |
| A CI gate's behaviour | `CONTRIBUTING.md`'s gate table; the `Makefile` target comment |
| Filed a new issue to track this correction | The milestone's tracking issue, **in the same turn** — don't wait for `pr-publish`'s own step for it. Forgotten before; see [issue-intake](../issue-intake/SKILL.md)'s filing section. |

**Everything committed is in English** — docs, comments, commit messages, PR
text, issue text — regardless of the language the request came in. Replies to
the user may be in the user's language; the repository is English-only.

## Verifying

A prose-only diff gets **no signal** from `make check` — the gates are
content-agnostic, and CI skips its `check` job for a diff that cannot touch Go
source or its own tooling. So:

- **Skip `make check` for a docs-only change** and say so in the PR's test plan.
  The secret scan still runs in CI unconditionally, on purpose — a credential
  pasted into a decision record is exactly what it exists to catch.
- **Verify by reading instead.** Re-read the edited section in full, in place,
  not just the diff. Check every cross-reference you touched actually resolves —
  section numbers, decision links, issue numbers.
- Trace any changed rule or SQL by hand against the case it exists for, and
  record that trace in the PR's test plan. That is the test plan for a spec fix.

`markdownlint` deliberately does not run in CI — style is caught in review,
where the cost of being wrong is a comment rather than a blocked merge.

## Then

→ `delivery-review`, then `pr-publish`.
