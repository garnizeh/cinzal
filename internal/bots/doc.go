// Package bots implements Decide(game.PlayerView, game.Config, *rules.RNG)
// game.Order across three difficulty tiers, serving four roles: filler,
// autopilot, simulation, and practice.
//
// A bot receives a PlayerView and nothing else — not the match state, not the
// graph, not the seed. That is fairness, and it is also an executable test of
// the projection: if a bot cannot be written competently against the view, the
// view is missing something a human player also needs. A human would assume
// they were bad at the game; the bot surfaces it as a coding problem
// (RFC-001 §14.1).
//
// Decide is pure and takes the seeded RNG, so a bot-populated match replays
// exactly like any other.
package bots
