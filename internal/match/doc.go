// Package match owns the round lifecycle: creating and joining matches, the
// tick, the deadline sweeper, and the order-draft operations.
//
// It is the only path from an HTTP handler into the engine. internal/web calls
// into this package rather than into internal/rules, which is what lets web
// stay clear of the match state entirely (docs/decisions/D01-package-layout.md).
//
// Everything this package returns to web is a game type. It never hands out a
// rules type, and never a match state value inside one.
//
// Two responsibilities that look like plumbing and are not. The tick is the
// only caller that owns side effects — Resolve returns events as data, and
// email, SSE and telemetry are dispatched here, never from the fold, or a
// rebuild re-sends every historical notification (RFC-001 §7.4). And the draft
// operations treat their input as untrusted on every request: an unvalidated
// draft endpoint is a fog oracle, one probe per request to learn whether a node
// exists (docs/decisions/D02-order-draft-state.md).
//
// A third: wherever this package calls rules.Project, it must also fill in
// the two SelfState fields Project's Config-free signature cannot set —
// v.You.StepAllowance from rules.Steps(v, cfg) and v.You.RoundsToNextOffer
// from rules.RoundsToNextOffer(s, seat, cfg) — before handing the
// game.PlayerView to web (docs/decisions/D27-project-config-parameter.md).
package match
