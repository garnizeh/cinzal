---
name: code-change
description: Implement a Cinzal code task in Go — internal/rules, game, match, store, bots, web, render, telemetry, or cmd/. Use when actually writing or editing Go source against a plan or issue. Carries the fog boundary, purity, determinism and package-graph constraints that make a change survive the gates.
---

# Code change

Execution stage for a task that produces Go. Assumes a plan exists
(`task-plan`); if it does not, stop and make one — this codebase punishes
improvisation with gate failures that read as unrelated.

---

## The four constraints that fail a change

### 1. Fog is private

The client must never receive the full match state — only what a seat's
fog-of-war entitles it to see. This is a **game rule** (GDD §7.1), not a UX
choice.

- All state passes through `Project(s State, seat SeatID) PlayerView`. No debug
  JSON, no template reaching around it.
- `internal/render` and `internal/web` may not import `internal/rules`.
  `internal/match` translates, in `game` types.
- Fog tests assert hidden facts are **absent**, not merely unused (RFC §16.3).
  Hold new code to that standard.

Before every commit: *does this leak state past the fog boundary?*

### 2. `internal/rules` is pure

No I/O, no `time`, no `math/rand`, no network — and the gate now covers
`internal/telemetry` and `internal/bots` too. It checks indirect references and
`fmt` call sites, not just imports.

### 3. Determinism: `seed + order log` reproduces a match forever

- No map-range order, no floats, no `time.Now()`, no concurrency inside `Resolve`.
- Every RNG draw goes through the one seeded `*RNG` and needs a row in the **RFC
  §6.4 consumption table** plus an index-count assertion — **including its
  truncation cases**. Conditional or early-terminated draws must stay lazy, or
  replays desync months later on one machine.

### 4. The package graph is declared

`internal/game` is a leaf: shared vocabulary, imports nothing, no
`any`/`interface{}`/unconstrained type params. **There is no `game.State`.**
A new package means editing `scripts/packages.txt` deliberately.

`internal/bots` may not name `MatchState`, the graph, or the match seed. It
imports `rules` legitimately, so the gate walks every `rules.X` selector against
`scripts/bots-isolation-allowlist.txt`. Widening that list is its own reviewed
change with a written fog-safety argument — never a drive-by edit.

`cmd/simulate` may depend on only `rules`, `bots`, `game`, `telemetry`.

---

## Working rules

**Effects versus state.** `Resolve` returns pure `[]Event`. Only the tick's
caller dispatches side effects. This is what keeps refold, replay and rebuild
from re-sending historical notifications (RFC §7.4). Never reach for an effect
from inside the fold.

**A new `EventKind` appends at the end of the `iota` block.** A mid-block
insertion shifts later ordinals, which leaks into `Anchor.Kind` and widens the
golden-fixture blast radius far beyond the change.

**Tunable numbers are `Config` fields, never constants.**

**Rules and architecture change in the documents first.** If the code needs a
rule the GDD does not state, that is a `docs-change` or a `decision-record`
landing before this one — not a comment explaining the deviation.

**Match the surrounding code.** Comment density here is high and load-bearing:
comments say *why*, and cite the decision or spec section. Follow that.
Locate the nearest existing analogue with the **CodeGraph MCP**
(`codegraph_explore`), not `grep`/`find` — one call returns its verbatim
source plus who calls it, which is what "match the surrounding code" actually
needs. Fall back to `grep`/`find` only if CodeGraph is unavailable.

**Build artifacts go in `bin/`.** Always `-o bin/...`; a bare `go build` drops a
binary at the repo root.

**Probe edits get reverted immediately.** If you patch a source file to test a
gate or a hypothesis, revert it in the same turn — a left-behind probe breaks
the rules test build and the user hits it in the same tree.

---

## Loop

1. Write the smallest coherent slice from the plan.
2. `rtk make test` — or narrower: `rtk go test -race ./internal/rules/...`
3. `rtk make lint` before you get attached to a shape.
4. Add or extend tests as you go — `test-authoring` covers what a good one is
   here; do not defer the whole test set to the end.
5. When the slice is complete, run the full suite via `gates-run`.

Nothing is finished until `make check` is green locally. A gate failure is not a
nit — and **the answer is almost never to weaken the gate**. If a gate blocks
you, the change is usually wrong; if the gate is genuinely wrong, that is its own
issue and its own PR.

---

## Then

- Tests to write or deepen → `test-authoring`
- Full verification → `gates-run`
- Performance-sensitive change in `internal/rules/gen` → `bench-run`
- Ready to land → `delivery-review`, then `pr-publish`
