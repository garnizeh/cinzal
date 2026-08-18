package bots

import (
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// drifterBot is the Drifter tier (RFC-001 §14.3): "uniform random legal
// order," the statistical baseline the property tests and every sweep
// number in M2 are read against. This is a provisional Decide — a uniform
// draw among Known neighbours, not yet the uniform-over-the-legal-order-
// space sampler #191/#192 build on top of rules.Affordances — kept only
// stateless and legal so Bot's own contract is testable before the real
// sampler exists.
type drifterBot struct{}

func (drifterBot) Decide(v game.PlayerView, cfg game.Config, r *rules.BotRNG) game.Order {
	return decideStayOrMove(v, cfg, r, "bot.drifter.route")
}
