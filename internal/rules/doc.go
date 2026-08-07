// Package rules is the game engine. It owns the match state and computes the
// two functions the whole design rests on: Resolve, the round pipeline, and
// Project, the fog boundary.
//
// This package is pure. It imports nothing but the standard library, and from
// the standard library it does not import time, math/rand, os, net/..., or
// anything that performs I/O. This is enforced by a CI gate, not by convention
// (RFC-001 §6.1).
//
// The purity is not tidiness. RFC-001 §6.3 requires that seed plus the order
// log reproduce a match exactly, forever, and names four ways Go breaks that:
// ranging over maps during resolution, floating point, ambient time.Now(), and
// concurrency inside Resolve. The import rule prevents the last two categories
// outright; the first two need tests.
//
// The match state has no name outside this package. Everything a player is
// entitled to see leaves through Project, which returns a game.PlayerView, and
// there is no second path — no debug JSON in production, no template that
// reaches around it (RFC-001 §3).
package rules
