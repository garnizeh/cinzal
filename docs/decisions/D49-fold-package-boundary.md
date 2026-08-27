# D49 — Where does `Fold`'s `rules.MatchState` live so the fog gate can still see it?

**Status:** decided
**Blocks:** [#319](https://github.com/garnizeh/cinzal/issues/319) (`Fold`), [#320](https://github.com/garnizeh/cinzal/issues/320) (fold-metrics wiring), [#322](https://github.com/garnizeh/cinzal/issues/322) (`cmd/replay`), [#324](https://github.com/garnizeh/cinzal/issues/324) (the CI gate task, currently scoped to effect providers and not to this)
**Decided:** 2026-08-27
**Issue:** [#346](https://github.com/garnizeh/cinzal/issues/346)

## The question

[#319](https://github.com/garnizeh/cinzal/issues/319) gives `internal/match` its defining function:

```go
// in internal/match
func Fold(seed [32]byte, cfg game.Config, players int, log rules.OrderLog) (rules.MatchState, []game.Event, error)
```

[D45](D45-fold-metrics-emitter-and-dashboard.md) sketches a second, `FoldMeasured`, with the same return type, in the same package. `internal/match/doc.go` says, today, truthfully:

> Everything this package returns to web is a game type. It never hands out a rules type, and never a match state value inside one.

Both cannot be exported from `internal/match` at once. The question is which one moves — `Fold`'s home, or the doc comment's claim — and what enforces the answer.

## Why it is open

**`scripts/check-fog-boundary.sh` checks direct imports, and its own header explains why that is supposed to be exact rather than approximate:**

> in Go a transitive dependency puts NO names in scope. You cannot reference a type from a package you do not import directly, so a direct-import check is exactly congruent with the property §3 wants.

That is true of **naming** a type and false of **holding** one. `internal/web` must import `internal/match` directly — D01 makes it the only path from a handler into the engine, and that edge is deliberately allowed. If `Fold` is exported from `internal/match` itself, a handler that never writes `"internal/rules"` in its own import block can still write

```go
state, _, err := match.Fold(seed, cfg, players, log)   // state is a rules.MatchState
```

and read every field on it — fog archives, every seat's position, the full graph. `go list` reports no forbidden edge on `internal/web`, `check-fog-boundary.sh` prints OK, and RFC §300's claim — "a template physically cannot leak what it cannot name" — is false for that one path, silently.

**M3 is where the exposure is created; M5 is where it gets used.** Nothing in `internal/web` exists yet, so today nothing is broken. But `Fold` is the function §7.1 defines the whole persistence model in terms of, and the first M5 handler that wants "the state, for this seat" will find an exported function in exactly the right package returning exactly that. Catching this once a real handler exists means the gate has been silently wrong for two milestones.

**`cmd/replay` genuinely needs the raw type**, which rules out the simplest fix. [#322](https://github.com/garnizeh/cinzal/issues/322)'s default dump is explicitly the full `MatchState`, not a `PlayerView` — "a CLI is neither [render nor web], so it may print it." Unexporting `Fold` and giving `internal/match` only game-type wrappers would strand that requirement outside the package.

## Options

**A — Unexport `fold`, wrap everything else in game types.**
`cmd/replay` (#322) and `--rebuild` (#323) need the raw `MatchState` and the raw `[]game.Event` slice from outside the package. An unexported `fold` cannot serve them at all. Rejected on that requirement alone, not on style.

**B — Move `Fold`/`FoldMeasured` to a subpackage, `internal/match/fold`, added to the fog gate's `FORBIDDEN` set.**
Restores the exact-congruence argument the header already relies on: `internal/web` importing `internal/match` stays allowed and unchanged, but reaching `Fold` now requires a *second*, distinct import — `internal/match/fold` — which is exactly the kind of edge `check-fog-boundary.sh` already knows how to forbid, the same way it already forbids `internal/rules` and `internal/telemetry` (D34). No new script, no new class of check — one string added to an existing array. Cost: `internal/match`'s own tick (M4) needs `Fold`/`FoldMeasured` to reconstruct state from the order log every round (RFC §7.3's no-snapshot decision), so the parent package ends up importing its own child.

**C — Extend a gate to `internal/match`'s exported signatures: no exported identifier may have a `rules.*` type in its results.**
An AST walk in the shape `scripts/check-game-types.go` already establishes. It enforces the doc comment as literally written — but only if `Fold` is *not* exported from `internal/match` at all, which means this option doesn't stand on its own: something else still has to say where `Fold` actually lives once it is barred from the package whose signatures are being walked. Taken to its conclusion it collapses into Option B (or A) plus an extra enforcement layer on top. And the walk itself is a materially bigger surface than `check-game-types.go`'s: that script only has to recognize `any`, an interface literal, and a type parameter — three shapes. A return-type walk has to resolve type aliases, pointers, slices, maps, channels, embedded struct fields, and generic instantiations, each capable of smuggling a `rules.*` type past a check that only looks at the immediate result type. More gate to get wrong for a property Option B gets for free from the mechanism already in production.

**D — Amend the doc comment; accept `internal/match` as two-audience, convention-enforced.**
The status quo answer, and per this repo's own rule that a gate which cannot fail is worse than no gate, it is being rejected in writing rather than by default. `web` and `cmd/replay` would rely on a comment, not a check, to stay on their respective sides — exactly the erosion-at-2am failure mode RFC §5 names for this whole area.

## Decision

**Option B.** `Fold` and `FoldMeasured` (D45's wrapper) live in a new sub-package, `internal/match/fold`, not in top-level `internal/match`. `internal/match/doc.go`'s claim is **not amended** — it stays true, because the one function capable of violating it is not in the package the claim is about.

**`internal/match/fold`'s callers:**

- `cmd/replay` imports it directly, for the raw `MatchState` its default dump prints and the `[]game.Event` `--rebuild` writes.
- `internal/match` (top-level) imports its own child, from the tick (M4), to reconstruct current state from the order log each round with no snapshot layer — mirroring the existing `internal/rules` → `internal/rules/gen` precedent for exactly this shape of parent-imports-child, already in production (`internal/rules/initial.go`, `internal/rules/gen_bridge.go`).
- `internal/web` does **not** import it, and is stopped from doing so by the mechanism below rather than by convention.

**The mechanism:** `scripts/check-fog-boundary.sh`'s `FORBIDDEN` array gains `"$MODULE/internal/match/fold"`, joining `internal/rules` and `internal/telemetry`, checked over the same `.Imports`/`.TestImports`/`.XTestImports` fields against the same guarded set (`internal/render/...`, `internal/web/...`). Added in this decision's own PR, ahead of the package existing: the entry is inert until #319 creates `internal/match/fold` (no import can match a path with no package behind it, and there is no self-test fixture for this gate that inspects the array's exact contents), and pre-declaring it removes any dependence on #319's PR remembering to add it.

**This is the existing fog gate, extended — not #324's gate, and not a third one.** #324 protects RFC §7.4: `cmd/replay`'s call graph must not reach an effect provider (mail, SSE, telemetry-dispatch). This decision protects RFC §3: `internal/web`'s call graph must not reach a `rules` type. They run in opposite directions over unrelated package sets and would only ever collide by coincidence; `check-fog-boundary.sh` stays the one place §3 is enforced, `check-replay-deps.sh` (or whatever #324 names it) stays the one place §7.4 is.

**The `check-fog-boundary.sh` header correction**, made in this PR since the false claim already exists in shipped code and needs no `#319` to be wrong:

> web depends on rules transitively through match, unavoidably. That is fine: in Go a transitive dependency puts NO names in scope. You cannot reference a type from a package you do not import directly, so a direct-import check is exactly congruent with the property §3 wants.

is true only when every exported function reachable through an *allowed* import itself returns nothing from a *forbidden* package. That is a property of `internal/match`'s own signatures, not a consequence of the import graph alone — D49 is what makes it hold, by keeping every `rules`-returning function out of `internal/match` and in a separately forbidden sub-package. The header now says this in one added sentence (see the accompanying diff).

## Reasoning

**The decisive argument is the same one D01 already made for the top-level split, applied one level down.** D01 chose "forbid the import" over "enumerate what exposes `MatchState`" because an import-based check stays correct as `MatchState` grows fields or gains siblings, while an enumeration is a list someone has to remember to extend. Option C is exactly the enumeration shape D01 already rejected, rebuilt as an AST walk over return types instead of over `internal/game`'s own field declarations. Option B is exactly D01's shape, reapplied: forbid an edge, not a signature.

**The parent-imports-child cost is not novel — it is the existing shape of this codebase.** `internal/rules/gen`'s own doc comment states the direction plainly: "This package does not import internal/rules, even though internal/rules will import this one." `internal/match` importing `internal/match/fold` for the same reason — the child holds a capability the parent's own runtime logic needs, but the parent's *exported surface* must not leak the child's return type — is the identical relationship, one layer up the tree. Nothing about Option B asks the codebase to accept a shape it doesn't already have.

**Why not extend `internal/telemetry`'s carve-out to cover this instead of adding a new forbidden entry.** `internal/telemetry` is already a sanctioned, top-level reader of `MatchState` (D34) — but it is a *sibling* of `match`, not something `internal/web` has any standing reason to import at all. `internal/match/fold` is different in kind: it exists specifically because `internal/match` — the one package `web` *must* import — cannot itself carry the exposure. A single new forbidden entry that is a sub-path of an otherwise-unforbidden package is a shape the script's matching logic (`"$forbidden"|"$forbidden"/*`) already handles without modification; it does not need a new category of rule, only a new string.

**Why the doc comment does not move.** Amending it to admit a `rules`-returning export would be choosing Option D by the back door — the comment would then describe a convention, not a checked property, for the one package whose entire job is to be the checked boundary. Leaving it stated exactly as-is, and making it true by construction, is cheaper than rewording it and produces a stronger guarantee.

## Consequences

- `internal/match/fold` is a new package, created in #319's PR, holding `Fold` and (once #320/D45 land) `FoldMeasured`. Neither is ever declared in top-level `internal/match`.
- `scripts/check-fog-boundary.sh`'s `FORBIDDEN` array already lists `internal/match/fold`, added by this decision ahead of the package existing, so #319's PR has nothing left to remember on this point.
- `internal/match/doc.go` needs no edit. Its existing claim remains true after #319 lands, provided #319 follows this decision.
- **D45's sketch is refined, not superseded.** D45 shows `FoldMeasured` "in internal/match"; that line was written before this decision settled the package boundary and is now read as `internal/match/fold`. D45's substance — the emitter, the duration/allocation computation, the dashboard artefact — is untouched. Per this repository's convention (see D41 following D39 without editing it), D45's file is left as written rather than edited; this document is the pointer forward.
- **#324's allow-list gains an explicit entry for `internal/match/fold`, not just `internal/match`.** `cmd/replay` reaches `Fold`/`FoldMeasured` through the sub-package directly, so an allow-list naming only `internal/match` would either miss the real edge or need a prefix match written carefully enough to include it. #324's own PR should state this plainly rather than rediscover it.
- Reversible at low cost while `internal/web` still does not exist: moving `Fold` back to `internal/match` before M5 is a rename with no caller-visible break, since `cmd/replay` is `internal/match/fold`'s only caller outside `internal/match` itself today. It becomes expensive exactly at the point it needs to be expensive — once `internal/web` exists and a real fog-relevant caller depends on the boundary holding.
