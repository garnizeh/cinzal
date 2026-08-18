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

**The issue's own "against" for Option B claims the fog gate needs new logic; the gate as written does not need any.** `scripts/check-fog-boundary.sh` is a blacklist on `render`'s and `web`'s own **direct** imports of `internal/rules` (`FORBIDDEN="$MODULE/internal/rules"`, `GUARDED="./internal/render/... ./internal/web/..."`) — it has no allowlist of sanctioned rules-importers to keep in sync, and none is needed: any package outside `render`/`web` may import `rules` freely already, including a brand-new one. `internal/telemetry` calling `rules.MatchState` and a new `rules.OrderLog` by name in its own exported signature (below) means `render`/`web` could only call it by first importing `internal/rules` directly themselves — the exact edge the existing check already rejects, unmodified. What *does* need updating, mechanically and unconditionally, is `scripts/packages.txt`: `check-packages.sh` diffs it against `go list ./...` and fails closed on any undeclared package (`check-packages.sh:24-29`), so `internal/telemetry` must be added there the moment the package exists — in #197's PR, the same way every other M2 package addition works, not in this decision.

## Options

**A — `internal/telemetry`, leaf over `internal/game`, input `[]game.Event` + `game.Config`.**

- For: importable everywhere, no fog consequence, no gate change beyond `scripts/packages.txt`.
- Against: only viable if D33 had landed on "events only." It did not — six rows are genuinely state-shaped (lease counts at scoring, sector-majority RP, Infamy at exact values, the two fog-private archive aggregates) and one more needs order-log access. Forcing all of that into new events means inventing end-of-match summary events whose only reader is telemetry, the "state read wearing an event costume" pattern D33 already rejected for exactly this reason.

**B — `internal/telemetry`, imports `internal/rules`, input `rules.MatchState` + a new `rules.OrderLog` + `[]game.Event` + `game.Config`.**

- For: computes everything §22 asks for without inventing events the roadmap already argued against. `cmd/simulate` depends on `rules` + `bots` + `telemetry`, matching RFC §16.4's "needs only `rules` and `bots`" — `telemetry` is a straight package addition to that set, not a foreign dependency, because it imports the same things `cmd/simulate` was always going to import anyway.
- Against: permanently unreachable from `web`/`render` — but see above: this costs nothing beyond adding the package to `scripts/packages.txt`, since the existing fog gate already enforces it without modification.

**C — Inside `internal/rules`.**

- For: no new package; has everything in scope already, including the new `OrderLog` type.
- Against: `rules` is the deepest package in the graph and the one the roadmap protects from churn (RFC §2.3's build-order argument). Telemetry definitions change every time a GDD §22 threshold moves; a pure rules engine gaining an aggregation concern that changes on design review cadence, not engine-correctness cadence, is the wrong owner even before D33's audit is considered.

## Decision

**B.** `internal/telemetry` is a new package, sitting beside `bots` in RFC §5's layout list — imports `internal/rules` and `internal/game`, imported by `cmd/simulate` (M2), `internal/match` (M4's analytics-table write path — `match` already imports `rules` per D01, so this is not a new edge for it) and `internal/debug` (the debug panel, RFC §15.1). Never imported by `internal/render` or `internal/web`; the existing `check-fog-boundary.sh` already makes that a compile-time impossibility for the reason argued above, so this decision changes no gate logic.

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

**`MatchSummary`'s own shape, and whether `Match` returns an error, are #197's to build**, per §22's row-by-row content and acceptance criteria; this decision fixes only the four input parameters and the package's position, which is what the fog boundary and the "one computation" property in RFC §17 actually depend on.

## Reasoning

**Placing `OrderLog` in `internal/rules` rather than `internal/game` is the one departure from what #197's sketch assumed, and it follows D01's own precedent rather than inventing a new one.** D01 kept `MatchState` out of `game` specifically so `render`/`web` could never name it; a full-match order log is the same shape of fact for the same reason and gets the same treatment — if it lived in `game` instead, it would be trivially nameable everywhere, including `render`/`web`, and the only thing stopping a leak would be nobody handing over a value, which is exactly the "discipline instead of a compile-time property" the whole fog architecture exists to avoid (RFC §5). Putting it in `rules` costs nothing: `internal/telemetry` already imports `rules` under Option B, and no other current or planned importer of `OrderLog` sits outside that boundary.

**The audit resolves A versus B, again, the same way D33's audit resolved it for the input rows.** Six of D33's twenty rows need a final-state read and cannot be answered by any event design; that alone rules out A regardless of how the package question is framed. B is not a compromise — it is what D33 already decided, carried through to where the computation that consumes those inputs has to sit.

**Rejecting C:** unchanged from the issue's own argument — `rules` is the package the roadmap protects from churn, and a metric set that moves with design review, not engine correctness, does not belong inside the pure engine whose entire claim is stability.

**The fog-gate "against" in the original issue overstated the cost of B.** Re-reading `check-fog-boundary.sh` against this decision shows the check was already sufficient: it forbids a direct import of `internal/rules` from `render`/`web`, full stop, with no per-package allowlist to maintain. The real, mechanical requirement is `scripts/packages.txt`, and that gate (`check-packages.sh`) already fails closed on an undeclared package by design — the addition is real but is #197's, not this decision's.

## Consequences

- **`internal/telemetry` (new package) needs four inputs, not three:** `rules.MatchState`, the new `rules.OrderLog`, `[]game.Event`, `game.Config` — #197 must build to this signature, not the three-argument sketch in its own issue body.
- **`internal/rules` gains one new production type, `OrderLog`,** alongside `MatchState` — no behavior, pure data, matching the existing `state.go`/`order.go` split. This is separate from and additional to the `EventKind`/field additions [#196](https://github.com/garnizeh/cinzal/issues/196) already owns per D33.
- **`cmd/simulate` (and later `internal/match`'s M4 analytics write path) owns the accumulation** of both `OrderLog` and the cumulative `[]game.Event` across all `cfg.Rounds` rounds — `Resolve` continues to hand back one round at a time, unchanged.
- **`scripts/packages.txt` gains `github.com/garnizeh/cinzal/internal/telemetry`** in the PR that creates the package (#197), not here — this decision is documents-only, matching D33's own precedent.
- **No change to `scripts/check-fog-boundary.sh` or `scripts/check-packages.sh`.** Both already enforce what this decision needs: the former by forbidding the import edge outright, the latter by failing closed on any undeclared package.
- **RFC §17 gains the computation's name and position** ("one computation, three sinks" can now name the computation), and RFC §5's package listing gains an `internal/telemetry/` line beside `bots/` — both land in [#202](https://github.com/garnizeh/cinzal/issues/202), not here.
- **Reversible at low cost before #197 lands** — this is a documents-only change, reversible by superseding this file. After #197 is built against this signature, moving `OrderLog` to a different package or dropping it from the signature costs a rewrite of #197 and anything built against it, but no fixture re-record — unlike D33's `internal/rules` additions, nothing here touches `Resolve`'s event output or the golden replays.
