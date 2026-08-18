// Package bots implements Decide(game.PlayerView, game.Config, *rules.BotRNG)
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
// Decide is pure and takes a seeded, per-(seat, round) *rules.BotRNG — never
// Resolve's own *rules.RNG — so a bot-populated match replays exactly like
// any other (RFC-001 §14, docs/decisions/D32-bot-rng-stream.md).
//
// A Bot is stateless across rounds: everything a bot knows about the past is
// already in the view it is handed. Nothing on a Bot implementation may
// accumulate across Decide calls — that would break replay, since such a
// field lives nowhere in {seed, order log}, and would let two bots in the
// same match collude, which the type signature otherwise makes structurally
// impossible (RFC-001 §14.5). For(t) may therefore return a shared,
// immutable value; a tier that needs scratch space allocates it inside
// Decide and discards it.
package bots
