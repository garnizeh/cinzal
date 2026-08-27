---
name: exit-demo
description: Perform a Cinzal exit demonstration — proof that a milestone met one of its exit criteria, often by breaking something on purpose to show a gate catches it. Use for issues labelled exit-demonstration, for "prove the gate works", "demonstração de saída", or when closing a milestone.
---

# Exit demonstration

**Closing every task in a milestone is not the same as meeting its exit
criteria.** This is deliberately not a task: several criteria are demonstrations
that something *fails* — a PR adding `import "time"` to `internal/rules` must be
**rejected** by CI, and that can only be shown by breaking it on purpose.

If the demonstration is not performed, the milestone closes with gates that were
never tested against what they exist to stop.

---

## 1. Quote the criterion, exactly

From the roadmap's milestone section, verbatim. Then classify it:

| Kind | Means | Passing looks like |
|---|---|---|
| **Positive** | Something must succeed | The run, the artifact, the byte-identical output |
| **Negative** | Something must be rejected | The **failing** check, with its output quoted |
| **Measurement** | A number lands in a stated band | The number, the interval, the sample size, the threshold line |

Say up front what would count as failing. A demonstration with no failure
condition demonstrates nothing.

## 2. Design the method

**Negative demonstrations:** state exactly what you will break and which gate
must catch it — the gate name, the target, and the expected message. Break it in
a scratch copy or a throwaway branch, capture the failure, then **revert
immediately in the same turn**: a left-behind probe breaks the build and the
user hits it in the same tree.

**Measurements:** apply the D35 rigor bar from the first pass — real sample
sizes (10k matches where that is the standard), real intervals, paired
differences for shared-seed cohorts. Not "roughly", not one run.

**Positive demonstrations:** the assertion is byte-identity or exact equality
wherever the criterion allows it, not "looks right".

## 3. Run it and record evidence

Evidence lives in **`docs/exit-demos/<issue-number>/`** — the directory is named
for the issue, so a re-run for a different criterion never lands on top of an
existing one. CSVs, logs, the captured gate output.

Capture, do not paraphrase. A quoted failing check is evidence; "CI rejected it"
is a claim.

**Do not redirect large `rtk` output into a file** — its truncation footer can
land inside the file. Write evidence with `python3`/the Write tool, or run the
command bare through `rtk proxy` when you need the raw stream.

## 4. Scope a re-run to the row that changed

When re-running a demonstration after a fix, **scope it to the fixed row.** If
another row has drifted because of an unrelated change that already merged, do
**not** silently absorb that drift into this demonstration — file it separately
and say so. Use a fresh issue-numbered evidence directory for the re-run.

## 5. Report

Fill the issue's **Evidence** field: the run link, the failing check, the CSV,
the numbers. Then in the PR:

```markdown
## The criterion
<quoted from the roadmap>

## Method
<what was done; for a negative, what was broken and which gate caught it>

## Result
<the evidence, quoted — plus the paths under docs/exit-demos/<n>/>

## Verdict
Met / not met, and what that means for the milestone.
```

A demonstration that **fails** is a successful demonstration of a real problem.
Report it as met-or-not plainly; do not soften it, and do not adjust the
criterion to fit the result.

## Then

→ `delivery-review`, then `pr-publish`. Update the milestone tracking issue in
the same turn as the merge.
