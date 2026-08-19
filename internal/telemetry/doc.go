// Package telemetry computes GDD §22's twenty per-match metrics from one
// match's final state, order log, and event stream — RFC-001 §17's "one
// computation, three sinks": cmd/simulate's CSV, M4's analytics table, and
// the debug panel all call Match, never their own copy of this arithmetic,
// so bot data and human data stay directly comparable.
//
// It also computes GDD §22's per-round and per-action metric set — stance
// distribution, Ledger purchase behaviour, action and item selection
// frequency — via RoundActions, a separate function and type from Match/
// MatchSummary (round_action.go). These are distributions read across a
// whole sweep rather than a single per-configuration verdict, and every
// field's own doc comment carries the caveat RFC §16.4 states for this
// data specifically: "bot play is not human play" — a threshold's
// recommended remedy (cut an action, widen a blackout) can be exactly
// wrong when the only evidence is a bot that does not understand the
// action, not a human who has rejected it. round_action.go's own doc
// comment says which GDD §22 bullets have no field here at all, and why.
//
// # Where each row's answer comes from
//
// docs/decisions/D33-telemetry-event-stream-coverage.md audited all twenty
// rows against internal/rules and internal/game as they stand and found
// three shapes of answer, never a fourth: computable from the event stream
// alone, a read of the match's final MatchState (including every seat's
// private SeatArchive), or the order log — row 1's denominator specifically,
// since haltMovement (confront.go) clears a route from memory without ever
// recording that it was submitted. internal/rules/gdd22_metrics_test.go
// transcribes that audit as an executable table, one entry per row; this
// package computes exactly what that table says each row owes.
//
// D34 (docs/decisions/D34-telemetry-package-placement.md) placed the
// computation here rather than in internal/rules or internal/game, because
// six of the twenty rows are genuinely state-shaped (final lease counts,
// sector-majority RP, exact Infamy values, two fog-private archive
// aggregates) and forcing them into new events would mean inventing
// end-of-match summary events whose only reader is this package — "a state
// read wearing an event costume." internal/telemetry imports internal/rules
// deliberately and is never imported by internal/render or internal/web:
// scripts/check-fog-boundary.sh's FORBIDDEN set names this package
// explicitly, the same way it already names internal/rules, rather than
// leaving the guarantee to depend on what Match's signature happens to
// require today.
//
// # Rows this package does not answer
//
// Three rows are not headless facts at all, and MatchSummary carries no
// field for them — a struct field holding 0 for "not measured" is exactly
// the failure mode GDD §22's own framing warns against: row 15 (attribution
// queries) and row 18 (loitering flags from legitimate play) are M5.5 human
// questions; row 16 (Heat Map opens) is M5 UI instrumentation on a feature
// that does not exist yet. Row 13 (RP swing traceable to [O] cards) is
// different again: it is an M2 row, but D33 found no precise event or
// state source for it in this milestone's first pass, and recommended
// shipping without one rather than a proxy number that looks exact but
// isn't — see MatchSummary's own doc comment for where that leaves it.
package telemetry
