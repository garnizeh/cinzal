# D01 — Where does `MatchState` live relative to `PlayerView`?

**Status:** decided
**Blocks:** M0 — Foundations
**Decided:** 2026-08-06
**Issue:** [#3](https://github.com/garnizeh/cinzal/issues/3)

## The question

Do `MatchState` and `PlayerView` live in the same Go package or in different ones, and what exactly is the forbidden import edge that CI enforces?

## Why it is open

The RFC contradicts itself, in the same section that mandates the check.

**RFC-001 §5** lists the package layout with `state.go` (`MatchState`, `Player`, `Node`, `Graph`) and `fog.go` (`Project()` and `PlayerView`) as files inside a single `internal/rules` package.

**RFC-001 §3** says the rendering layer *cannot name* `MatchState` — "`PlayerView` lives in its own package and `MatchState` is not in scope there. A template physically cannot leak what it cannot name." That is only true if they are in **different** packages.

§5 then describes the enforcement as "`render` may import the fog package for `PlayerView`, and may not import anything that exposes `MatchState`," naming `rules/fog` and `rules/state` — a third shape again, inconsistent with the flat file listing a few lines above it.

The `go list` check that §5 mandates cannot be written until this is settled, and demonstrating that check is the M0 exit criterion.

## Options

**A — Subpackages of `rules`**

```
internal/rules/state/   MatchState, Player, Node, Graph
internal/rules/view/    PlayerView, NodeView, TrailEntry, Anchor
internal/rules/         Resolve, Project, Legal, RNG, gen
```

- For: closest to the RFC's own wording; the whole engine stays under one directory.
- Against: a three-way split inside one domain, and the CI assertion has to name a specific package rather than express a layering rule.

**B — A leaf package outside `rules`**

```
internal/game/     the shared vocabulary — imports NOTHING
internal/rules/    MatchState + Resolve, Project, Legal, RNG, gen
internal/render/   imports game, never rules
internal/web/      imports game, never rules
```

- For: the forbidden thing becomes a single import edge rather than a set of type names.
- Against: `web` can no longer call `rules.Legal` directly, so `internal/match` must expose the order-draft operations.

## Decision

**Option B**, with the leaf package named **`internal/game`**.

```
internal/game/     PlayerView, NodeView, TrailEntry, Anchor, NodeStats, SeatArchive,
                   Order, OrderDraft, Config, Event, SeatID, NodeID
                   — imports nothing outside the standard library

internal/rules/    MatchState, Player, Node, Graph
                   Resolve, Project, Legal, RNG
internal/rules/gen/  map generation
                   — imports game

internal/match/    lifecycle, tick, and the order-draft operations
                   — imports rules and game

internal/render/   imports game ONLY
internal/web/      imports game and match, never rules
```

`Project` becomes `func Project(s State, seat game.SeatID) game.PlayerView` — the only function in the codebase with a foot on both sides.

### The CI rule

> **No package under `internal/render/...` or `internal/web/...` may directly import `internal/rules` or any package beneath it.**

Checked over both `.Imports` and `.TestImports`.

## Reasoning

**The decisive argument is the shape of the assertion, not the elegance of the layout.**

Option B's check says *do not import this package*. That stays correct when someone adds a field to `MatchState`, adds a new state-bearing type, or renames one. Option A's check has to enumerate what "exposes `MatchState`" means, and every such enumeration is a list that someone must remember to extend. RFC §5 is explicit that this is "exactly the kind of rule that erodes at 2am" — and a check that erodes silently is the same failure as no check.

**Direct imports are the correct thing to check, not an approximation of it.** `web` will depend on `rules` transitively, through `match`. That is fine and unavoidable: in Go, a transitive dependency puts **no names in scope**. You cannot reference a type from a package you do not directly import. So a direct-import check is exactly congruent with the property §3 wants — that the type is unnameable in the template — rather than a weaker proxy for it.

**Test imports are covered too.** A `render` test that can name `MatchState` can build a fixture that non-test code later reads, and the exemption would be the first place the boundary leaks. If a render test needs a realistic view, it should construct a `game.PlayerView` directly — which is what a bot test needs anyway, so the fixtures get shared rather than duplicated.

**On the name.** `internal/view` was the obvious candidate and was rejected. The package holds `Order` and `Config` alongside `PlayerView`, and `view.Order` and `view.Config` misname two of its most-used types — a view is not what either of them is. `game.Order`, `game.Config`, `game.PlayerView` and `game.SeatID` all read correctly.

The split it implies is worth stating so it stays stable: **`game` is the vocabulary, `rules` is the behaviour.** Data that crosses layers goes in `game`; anything that computes goes in `rules`. `RNG` is engine machinery and stays in `rules`, which is why `bots` imports both — and that is fine, because `bots` is not in the forbidden set. RFC §14.1 already relies on the *call site* handing a bot nothing but a `PlayerView`.

**On the cost.** Option B's one real cost — `web` losing direct access to `rules.Legal` — is work that [D02](D02-order-draft-state.md) requires regardless. The two decisions were taken together, and each one's recommendation carries part of the other's cost. If either is revisited, both should be.

## Consequences

- Every M0 file-creating task depends on this. It is settled before the first one starts.
- The M0 exit criterion becomes demonstrable: a pull request adding `import "github.com/garnizeh/cinzal/internal/rules"` to `internal/render` must be rejected by CI.
- `internal/match` gains a documented responsibility beyond the tick: it is the only path from HTTP handlers into the engine.
- Reversing this later means touching every import in the tree. Cheap now, expensive at any subsequent point.
