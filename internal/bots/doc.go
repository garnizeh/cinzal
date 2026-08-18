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
//
// # Drifter's sampling scheme (RFC-001 §14.3, issue #192)
//
// Drifter's Decide is Sample (legalspace.go), unmodified. Sample draws every
// field of an order sequentially, uniformly over that field's own legal
// candidate set conditioned on the fields already drawn: a route uniformly
// among Routes(v, cfg); a Pushing On declaration uniformly among PushingOns
// for that route; a stance uniformly among Neutral, Evasive, and Aggressive
// at each of its legal stakes; each held item's discard uniformly between
// keep and discard, and, when discarded, its target uniformly among that
// item's own legal targets; an action uniformly among Actions for the
// route, Pushing On and item discards drawn so far; the two add-ons, the
// abandon-cargo flag, and this round's contract choice, each the same way.
//
// That is uniform *per field*, given the fields drawn before it — it is not
// uniform over the joint legal-order space, and the two are not the same
// claim. A route whose ending node licenses only two legal actions gives
// each of those two actions a larger share of the whole space than a route
// licensing ten actions gives any one of its ten: the routes themselves
// stay equally likely (Route is drawn first, from the whole route set,
// before any of that downstream branching is known), but the orders built
// on top of them do not end up equally likely. "Uniform over the legal
// route set, then uniform over what each stage after it allows" is the
// scheme actually implemented, and it is what every M2 sweep number
// attributed to "the Drifter baseline" is a statement about — not "uniform
// over legal orders," which nothing in this package provides and no
// consumer should assume.
package bots
