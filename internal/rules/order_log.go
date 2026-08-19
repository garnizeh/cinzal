package rules

import "github.com/garnizeh/cinzal/internal/game"

// OrderLog is every order submitted across a whole match, one round's
// submissions per round number (docs/decisions/D34-telemetry-package-
// placement.md). It is never fog-filtered and is at least as sensitive as
// MatchState itself — it names every seat's full route and action history,
// not one seat's — so it lives beside MatchState, nameable only where
// MatchState already is, rather than in internal/game where render/web
// could name it too (D01's own precedent for keeping MatchState out of
// game).
//
// Resolve takes one round's orders as its own argument, distinct from the
// MatchState it folds into (resolve.go), and never accumulates them itself
// — building and keeping this map, round by round, is the caller's job
// (cmd/simulate today, internal/match's fold in M3). internal/telemetry
// needs it specifically for GDD §22's "routes cancelled mid-route"
// denominator: haltMovement (confront.go) clears a route from the
// in-memory validated copy without ever having recorded that the route was
// submitted in the first place (D33 row 1) — this is the only remaining
// record of that fact once a round resolves.
//
// This is a new, separate production declaration — not an export of
// determinism_test.go's own unexported orderLog/roundOrders shape, which
// stays exactly as it is, local to that test file.
type OrderLog map[game.RoundNumber]map[game.SeatID]game.Order
