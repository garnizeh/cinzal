---
name: decision-record
description: Write a Cinzal decision document (docs/decisions/Dnn-slug.md) — for a question the GDD and RFC leave silent, ambiguous or self-contradictory. Use when work is blocked on an unanswered question, when a task turns out to be inventing a requirement, or when the user asks for a Dnn decision, an ADR, or "isso é uma decisão".
---

# Decision record

A decision produces a **written answer, not code**, and it **blocks the tasks
that depend on it** — a milestone does not start with open blockers.

You are here because the specifications are silent, ambiguous, or contradict
each other. If the answer is already in the GDD or the RFC, this is a task, not
a decision. If the documents merely contain a *wrong sentence*, that is a
`docs-change` — there is no question to weigh (see D15's reclassification for
the precedent).

---

## 1. Find the number and the shape

Read `docs/decisions/README.md`: the format, the two standing conventions, and
the catalogue. Take the next free `Dnn`. File name: `Dnn-short-slug.md`, zero
padded below 10 (`D01`, `D02`, … `D52`).

Read three or four recent decisions end to end before writing — D44–D52 are the
current house style, and it is tighter than the template suggests.

## 2. Establish the question is real

Quote the exact sentences that conflict, with their sections. "The RFC is
silent" is not enough on its own; show the place a reader would look and what
they find there. Where possible, produce a **concrete counterexample** — D24 is
the model: a claimed guarantee, a seed where it fails, and the measured rate.

If measurement can settle it, measure. Use the simulation harness, real sample
sizes, real intervals, and paired differences for shared-seed cohorts — from the
first pass, not after review asks for it (D35's rigor bar).

## 3. Write the document

```markdown
# Dnn — <the question, phrased as a question>

**Status:** decided
**Blocks:** M3
**Decided:** YYYY-MM-DD

## The question
## Why it is open
   What the specs say and where they disagree. Cite sections. Quote them.
## Options
   At least two. Each with what it costs — an option with no stated cost has
   not been examined.
## Decision
## Reasoning
## Consequences
   What changes downstream, and what it would cost to reverse later.
```

Two conventions that are not optional here:

- **Record the rejected options and why.** The reasoning outlives the verdict.
  When a decision comes back around, the useful artefact is the argument, not
  the answer. Both specs are written this way.
- **A decision that turns out wrong is superseded, not edited.** Leave the
  original standing with a pointer forward.

Write for the reader who arrives in a year with no context. Prefer the concrete:
the failing seed, the exact SQL, the algebra showing the steady state is
unchanged.

## 4. Land the consequences with it

A decision document alone is an unlanded decision. In the same PR:

- **`docs/decisions/README.md`** — a catalogue row under the right milestone,
  with the status summarised in one line (read the neighbouring rows for the
  register).
- **The roadmap §3 status line** for that decision.
- **The GDD or RFC edits the decision mandates**, with changelog entries and a
  revision bump — see `docs-change`. If the decision corrects an earlier
  decision, that document gets its pointer forward too.
- **`CLAUDE.md`'s "Repository state"** paragraph if the decision range it lists
  moves.

## 5. Verify

There is no gate for prose. Skip `make check` for a docs-only decision and say
so in the test plan. Instead: re-read each edited section whole, resolve every
cross-reference by hand, and trace the decided behaviour against the case it
exists for — the multi-round gap, the empty pool, the two-player map — writing
that trace into the PR's test plan.

## Then

→ `delivery-review`, then `pr-publish`. The PR subject convention is
`Dnn: <the answer in one line>` — see `git log`.
