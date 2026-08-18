package bots

import (
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// runnerBot is the Runner tier (RFC-001 §14.3): greedy pathing to the
// current contract objective, the Autopilot default (§8.2). This is a
// provisional Decide — an undirected walk among Known neighbours, not yet
// the objective-directed greedy pathing #193 builds — kept only stateless
// and legal so Bot's own contract is testable before Runner's real
// behaviour exists.
type runnerBot struct{}

func (runnerBot) Decide(v game.PlayerView, cfg game.Config, r *rules.BotRNG) game.Order {
	return decideStayOrMove(v, cfg, r, "bot.runner.route")
}
