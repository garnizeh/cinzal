# D27 — Does `Project` gain a `Config` parameter, or do `StepAllowance`/`RoundsToNextOffer` land some other way?

**Status:** decided
**Blocks:** [#75](https://github.com/garnizeh/cinzal/issues/75) (internal/rules: Project and the fourteen authorised position writers) — narrowly: only `SelfState.StepAllowance` and `SelfState.RoundsToNextOffer`, not `writeAnchors` or anything else in `Project`
**Decided:** 2026-08-14
**Issue:** [#143](https://github.com/garnizeh/cinzal/issues/143)

## The question

`Project`'s signature is stated identically, and decided, in three places — RFC §3 (`docs/project/cinzal-architecture-rfc.md:163`), RFC §9 (`:243`), and [D01](D01-package-layout.md:70):

```go
func Project(s State, seat SeatID) PlayerView
```

No `Config` parameter, and `MatchState` carries none either. But `game/view.go`'s own field comments on two `SelfState` fields say `Project` computes them from one:

- `StepAllowance` (`view.go:97-102`): "computed server-side from the full modifier chain... Project sets this from `rules.Steps(view, cfg)`."
- `RoundsToNextOffer` (`view.go:111-115`): "Computed server-side... the hold rules on a full hand or an empty pool (D7) are not a client computation."

`rules.Steps(v game.PlayerView, cfg game.Config) int` and `rules.RoundsToNextOffer(s MatchState, seat game.SeatID, cfg game.Config) int` (`internal/rules/steps.go:26`, `internal/rules/contracts.go:151`) both require a `Config` `Project`, as decided, has no way to obtain from its own parameters.

## Options

**1 — `Project` gains a `cfg game.Config` parameter**

Revise D01's decided signature and both RFC citations of it to match what `view.go`'s comments already assume, and update every call site already written against the old signature (#75).

- For: `StepAllowance`/`RoundsToNextOffer` land inside `Project` itself, matching `view.go`'s current wording without touching it.
- Against: reopens a signature decided in three places, for a parameter `Project` would use for nothing except forwarding to two field-setters — it never gates visibility on it the way it gates every other field on fog.

**2 — `Project` does not set these two fields**

They stay at their zero value out of `Project`. Whatever caller has `Config` in scope — `internal/match`, once it exists — fills them in as a second pass after `Project` returns, the same way it will already have to supply `Config` to `Legal`/`Steps` for order validation. `view.go`'s two field comments get corrected to say so.

- For: `Project`'s decided signature and both RFC citations stand unamended; no #75 call site changes.
- Against: `StepAllowance`/`RoundsToNextOffer` are wrong (zero) on any `PlayerView` built by calling `Project` alone, until `internal/match` exists to complete them — real until M1's round-lifecycle work lands, not silent (fog fixtures show zero, not a plausible-looking wrong number).

## Decision

**Option 2.** `Project` keeps its decided, `Config`-free signature. `SelfState.StepAllowance` and `SelfState.RoundsToNextOffer` are `internal/match`'s responsibility, filled in immediately after `Project` returns, before the `PlayerView` reaches `internal/web`.

## Reasoning

**The shape already exists, and it isn't `Project`'s.** `rules.Legal(v game.PlayerView, o game.Order, cfg game.Config)` and `rules.Steps(v game.PlayerView, cfg game.Config)` both already take a fog-safe view and a `Config` as two separate arguments from two separate callers' scopes — `Config`-dependent computation was never folded into the value that carries the view. `internal/match` calling `rules.Steps(v, cfg)` after `Project` returns `v` is that same shape, applied one call earlier, not a new pattern invented for this decision.

**`Project` has one job, and `Config` isn't an input to it.** D01 and RFC §3/§9 are specific about what `Project` is: the fog boundary, "the only function in the codebase that turns a MatchState into something internal/render or internal/web may hold" — a function of *visibility*, not of game formulas. `Config` never gates what a seat can see; it gates how a step budget or an offer cadence is computed from what `Project` already exposes as `StepModifiers` and contract state. A parameter `Project` would thread through unused except to forward to two field-setters blurs a function whose entire value is staying small enough to audit in one pass (RFC §16.3's fog suite asserts hidden facts are *absent* from exactly this function's output).

**Cost is asymmetric.** Option 1 reopens a signature decided in three places (D01, RFC §3, RFC §9) and touches every `Project` call site #75 already wrote. Option 2 costs two stale doc comments (`view.go:97-102`, `:111-115`) and a landing note for `internal/match`'s own future work — cheaper, and it undoes nothing already decided.

**`internal/match` pays nothing extra for this.** Per D01, `internal/match` imports both `rules` and `game`, owns the round lifecycle, and already must have `Config` in scope to call `rules.Legal` for order validation. Calling `rules.Steps(v, cfg)` and `rules.RoundsToNextOffer(s, seat, cfg)` right after `Project` returns `v` — it already holds the `s` and `seat` that produced `v` — costs it nothing it wasn't already going to pay.

**This generalises.** `StepModifiers` already demonstrates the pattern this decision formalises: `Project` supplies the frozen, fog-safe *inputs* to a formula (RFC §6.6's entry-snapshot freeze), and a `Config`-aware pass elsewhere computes the derived number from them — `StepAllowance` is `view.go`'s own words, "the cached result", not the thing `Project` derives. Any future `Config`-dependent `PlayerView` field takes the same route, so `Project`'s contract never grows a second axis (visibility *and* formula inputs) as `PlayerView` grows.

## Consequences

- `Project`'s signature is unchanged. D01 and RFC §3/§9 stand as written — no amendment.
- `game/view.go`'s two field comments (`SelfState.StepAllowance`, `:97-102`; `SelfState.RoundsToNextOffer`, `:111-115`) are corrected to say `internal/match` computes them, not `Project`.
- `internal/rules/fog.go`'s `Project` doc comment, which already flagged this as open pending #143, now records the resolution instead.
- Until `internal/match` exists (still `doc.go`-only), `SelfState.StepAllowance` and `SelfState.RoundsToNextOffer` are zero on every `PlayerView` `Project` alone produces — by design, not a gap. #75's own tests should assert that zero, not work around it.
- Concrete task note for `internal/match`'s eventual round-lifecycle code: at the same call site that already supplies `Config` to `Legal` for order validation, also set `v.You.StepAllowance = rules.Steps(v, cfg)` and `v.You.RoundsToNextOffer = rules.RoundsToNextOffer(s, seat, cfg)` before handing `v` to `web`.
- Reversing this later (moving to Option 1) means touching three already-decided documents and every existing `Project` call site — expensive, same shape as D01's own note on the cost of reversing it.
