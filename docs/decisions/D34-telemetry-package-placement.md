# D34 — Where does the telemetry computation live, and what is its input type?

**Status:** decided
**Blocks:** [#197](https://github.com/garnizeh/cinzal/issues/197) (`internal/telemetry`: the per-match metric set), [#199](https://github.com/garnizeh/cinzal/issues/199)/[#200](https://github.com/garnizeh/cinzal/issues/200) (`cmd/simulate`), [#202](https://github.com/garnizeh/cinzal/issues/202) (RFC/GDD catch-up), and through them the exit demonstrations that read those numbers
**Decided:** 2026-08-18
**Issue:** [#188](https://github.com/garnizeh/cinzal/issues/188)

## The question

RFC §17 requires **one** telemetry computation with three sinks — `cmd/simulate`'s CSV, M4's analytics table, and the debug panel. [D01](D01-package-layout.md) fixed the package layout and named no package for it. So: where does it live, what is its input type, and which packages are allowed to import it?

## Why it is open

**[D33](D33-telemetry-event-stream-coverage.md) settled *what* telemetry needs, not *where* it lives or its exact signature.** D33 landed on its own Option B: five §22 rows need nothing new, six need a final `MatchState` read (including every seat's private `SeatArchive`), four need small additions to `internal/rules`' event stream, and — separately from all of that — row 1's denominator needs **order-log access**, because `haltMovement` (`confront.go:472`) clears a route from the in-memory `validated` copy without ever having recorded that the route was submitted. D33's own words: this is "a real constraint on D34, not merely a preference." That constraint is checked against the code below, because #197 already sketches a signature — `func Match(s rules.MatchState, events []game.Event, cfg game.Config) MatchSummary` — that carries the state and the events but has no fourth argument for the order log, exactly the gap D33 warned about.

**No production type for either aggregate exists today.** `Resolve`'s signature is `func Resolve(s MatchState, orders map[game.SeatID]game.Order, cfg game.Config, r *RNG) (MatchState, []game.Event, error)` (`internal/rules/resolve.go:16`) — one round's orders in, one round's events out. Neither a whole match's order log nor its cumulative event stream is a type anything hands over; both are built by the caller, round by round. The only precedent for the order-log shape is test-only: `internal/rules/determinism_test.go:25-35` defines

```go
type roundOrders map[game.SeatID]game.Order
type orderLog map[int]roundOrders
```

unexported, reachable from nothing outside that test file. `determinism_test.go:279-295`'s `foldLog` shows the accumulation pattern by hand — `append(allEvents, events...)` inside the round loop — for events too. `cmd/simulate/doc.go` says nothing about accumulation strategy; it is unbuilt. So this decision is not just picking a package for existing plumbing — it is naming a production type that does not exist yet, on both sides.

**The order log is at least as fog-sensitive as `MatchState`, which bears on where its type lives, not only telemetry's.** A whole-match order log names every seat's full route and action history — not one player's, all of them. That is the same shape of fact `MatchState` is (D01: *"there is no `game.State` and there must never be one"*), and arguably a sharper one, since `MatchState.Players[i].Archive` (`state.go:192`, `game.SeatArchive` — `archive.go:42-60`) is already scoped per-seat while a full order log is explicitly cross-seat by definition. Naming its type matters independently of where `internal/telemetry` itself sits.

**The issue's own "against" for Option B is right, and the fix is smaller than it sounds.** `scripts/check-fog-boundary.sh` is a blacklist on `render`'s and `web`'s own **direct** imports of `internal/rules` (`FORBIDDEN="$MODULE/internal/rules"`, `GUARDED="./internal/render/... ./internal/web/..."`). Because `telemetry.Match` requires a `rules.MatchState`/`rules.OrderLog` argument, no caller can satisfy that signature today without importing `rules` directly first — so the edge happens to be closed already, as a consequence of today's signature. That is exactly the style of argument D01 rejected for `MatchState` itself (its own Option A: enumerate what a package "exposes," rejected as something that "erodes at 2am" as the package changes) in favor of a blanket per-package import prohibition that holds regardless of any given function's signature. `internal/telemetry` gets the same treatment: `FORBIDDEN` gains the package path explicitly, alongside `internal/rules`, so the guarantee does not quietly depend on `Match` always requiring a `rules`-typed argument. `scripts/packages.txt` needs the matching one-line addition regardless, mechanically and unconditionally — `check-packages.sh` diffs it against `go list ./...` and fails closed on any undeclared package (`check-packages.sh:24-29`). Both land in #197's PR, the same way every other M2 package addition works, not in this decision.

## Options

**A — `internal/telemetry`, leaf over `internal/game`, input `[]game.Event` + `game.Config`.**

- For: importable everywhere, no fog consequence, no gate change beyond `scripts/packages.txt`.
- Against: only viable if D33 had landed on "events only." It did not — six rows are genuinely state-shaped (lease counts at scoring, sector-majority RP, Infamy at exact values, the two fog-private archive aggregates) and one more needs order-log access. Forcing all of that into new events means inventing end-of-match summary events whose only reader is telemetry, the "state read wearing an event costume" pattern D33 already rejected for exactly this reason.

**B — `internal/telemetry`, imports `internal/rules`, input `rules.MatchState` + a new `rules.OrderLog` + `[]game.Event` + `game.Config`.**

- For: computes what D33 found computable from this input set — fourteen of twenty rows without qualification, rows 13/14 scoped down as D33 recommended — without inventing events the roadmap already argued against. Rows 15/16 (M5 UI instrumentation) and row 18 (no operational definition) stay out of scope regardless of input type; no signature answers them. `cmd/simulate` depends on `rules` + `bots` + `telemetry`, matching RFC §16.4's "needs only `rules` and `bots`" — `telemetry` is a straight package addition to that set, not a foreign dependency, because it imports the same things `cmd/simulate` was always going to import anyway.
- Against: permanently unreachable from `web`/`render` by intent, and the fog gate should say so explicitly rather than leave it to inference — see Consequences.

**C — Inside `internal/rules`.**

- For: no new package; has everything in scope already, including the new `OrderLog` type.
- Against: `rules` is the deepest package in the graph and the one the roadmap protects from churn (RFC §2.3's build-order argument). Telemetry definitions change every time a GDD §22 threshold moves; a pure rules engine gaining an aggregation concern that changes on design review cadence, not engine-correctness cadence, is the wrong owner even before D33's audit is considered.

## Decision

**B.** `internal/telemetry` is a new package, sitting beside `bots` in RFC §5's layout list — imports `internal/rules` and `internal/game`, imported by `cmd/simulate` (M2), `internal/match` (M4's analytics-table write path — `match` already imports `rules` per D01, so this is not a new edge for it) and `internal/debug` (the debug panel, RFC §15.1). Never imported by `internal/render` or `internal/web` — enforced the same way `internal/rules` already is: `scripts/check-fog-boundary.sh`'s `FORBIDDEN` set gains `internal/telemetry` alongside `internal/rules`, in #197's PR. Because `Match` requires a `rules.MatchState`/`rules.OrderLog` argument, no caller could satisfy that signature today without already importing `rules` directly — but D01 rejected exactly that style of reasoning ("what does this expose") for `MatchState` itself, in favor of a blanket "do not import this package" rule that stays correct independent of what any future exported function happens to require. `telemetry` gets the same treatment for the same reason, not because the narrower argument is wrong today.

**The signature D33's audit actually requires:**

```go
package telemetry

// Match computes the GDD §22 per-match metric set. s is the match's final
// MatchState (including every seat's SeatArchive); log is every order
// submitted across the whole match, independent of what haltMovement
// later cleared in memory; events is the caller's own accumulation of
// every round's Resolve output.
func Match(s rules.MatchState, log rules.OrderLog, events []game.Event, cfg game.Config) (MatchSummary, error)
```

**`OrderLog` is a new type, defined in `internal/rules`, not `internal/game`:**

```go
// OrderLog is every order submitted across a whole match, one round's
// submissions per round number. It is never fog-filtered and is at least
// as sensitive as MatchState itself — it names every seat's full route
// and action history, not one seat's — so it lives beside MatchState,
// nameable only where MatchState already is.
type OrderLog map[game.RoundNumber]map[game.SeatID]game.Order
```

promoted from `determinism_test.go`'s unexported `orderLog`/`roundOrders` shape, which stays as the test's own local type — this is a new, separate production declaration, not an export of the test one. `cmd/simulate` builds both `OrderLog` and `[]game.Event` the same way `foldLog` already does in the determinism tests: append each round's `Resolve` inputs and outputs as the harness drives the match loop. That accumulation is #199/#200's job, not this decision's, and not `internal/rules`' — `Resolve` itself gains nothing.

**`MatchSummary`'s own field shape is #197's to build**, per §22's row-by-row content. The `(MatchSummary, error)` return contract itself is fixed here, matching #197's own fail-closed acceptance criterion (*"`Match` returns an error, not a zero summary, when the event stream is empty, when the match did not reach `cfg.Rounds`, or when a denominator is zero"*) — this decision fixes the four input parameters, the error return, and the package's position, which is what the fog boundary and the "one computation" property in RFC §17 actually depend on.

## Reasoning

**Placing `OrderLog` in `internal/rules` rather than `internal/game` is the one departure from what #197's sketch assumed, and it follows D01's own precedent rather than inventing a new one.** D01 kept `MatchState` out of `game` specifically so `render`/`web` could never name it; a full-match order log is the same shape of fact for the same reason and gets the same treatment — if it lived in `game` instead, it would be trivially nameable everywhere, including `render`/`web`, and the only thing stopping a leak would be nobody handing over a value, which is exactly the "discipline instead of a compile-time property" the whole fog architecture exists to avoid (RFC §5). Putting it in `rules` costs nothing: `internal/telemetry` already imports `rules` under Option B, and no other current or planned importer of `OrderLog` sits outside that boundary.

**The audit resolves A versus B, again, the same way D33's audit resolved it for the input rows.** Six of D33's twenty rows need a final-state read and cannot be answered by any event design; that alone rules out A regardless of how the package question is framed. B is not a compromise — it is what D33 already decided, carried through to where the computation that consumes those inputs has to sit.

**Rejecting C:** unchanged from the issue's own argument — `rules` is the package the roadmap protects from churn, and a metric set that moves with design review, not engine correctness, does not belong inside the pure engine whose entire claim is stability.

**The fog-gate "against" in the original issue was right, and cheap.** `check-fog-boundary.sh` forbids a direct import of `internal/rules` from `render`/`web`, full stop, with no per-package allowlist — which means `telemetry.Match` is already uncallable from either package today, since no caller can supply a `rules.MatchState`/`rules.OrderLog` argument without importing `rules` directly first. But D01 explicitly rejected reasoning about what a package "exposes" as the basis for a fog check — that is Option A from D01 itself, rejected because such an enumeration "erodes at 2am" as the package changes. Extending `FORBIDDEN` to name `internal/telemetry` directly costs one line and turns an argument that happens to hold today into a property that holds regardless of what `telemetry` exports next. `scripts/packages.txt` needs the same one-line addition, mechanically, via `check-packages.sh`'s existing fail-closed diff — both land in #197, neither in this decision.

## Consequences

- **`internal/telemetry` (new package) needs four inputs, not three:** `rules.MatchState`, the new `rules.OrderLog`, `[]game.Event`, `game.Config` — #197 must build to this signature, not the three-argument sketch in its own issue body.
- **`internal/rules` gains one new production type, `OrderLog`,** alongside `MatchState` — no behavior, pure data, matching the existing `state.go`/`order.go` split. This is separate from and additional to the `EventKind`/field additions [#196](https://github.com/garnizeh/cinzal/issues/196) already owns per D33.
- **`cmd/simulate` (and later `internal/match`'s M4 analytics write path) owns the accumulation** of both `OrderLog` and the cumulative `[]game.Event` across all `cfg.Rounds` rounds — `Resolve` continues to hand back one round at a time, unchanged.
- **`scripts/packages.txt` gains `github.com/garnizeh/cinzal/internal/telemetry`**, and **`scripts/check-fog-boundary.sh`'s `FORBIDDEN` set gains the same package path alongside `internal/rules`** — both in the PR that creates the package (#197), not here. This decision is documents-only, matching D33's own precedent; the gate changes are real but deferred to where the package first exists, the same way `scripts/packages.txt` additions always land with the package they declare.
- **RFC §17 gains the computation's name and position** ("one computation, three sinks" can now name the computation), and RFC §5's package listing gains an `internal/telemetry/` line beside `bots/` — both land in [#202](https://github.com/garnizeh/cinzal/issues/202), not here.
- **Reversible at low cost before #197 lands** — this is a documents-only change, reversible by superseding this file. After #197 is built against this signature, moving `OrderLog` to a different package or dropping it from the signature costs a rewrite of #197 and anything built against it, but no fixture re-record — unlike D33's `internal/rules` additions, nothing here touches `Resolve`'s event output or the golden replays.
