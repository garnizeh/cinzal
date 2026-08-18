package bots

import (
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// operatorBot is the Operator tier (RFC-001 §14.3): plans across rounds,
// reading the heat map and the deck counts. This is a provisional Decide —
// an undirected walk among Known neighbours, not yet the cross-round
// planning #194 builds — kept only stateless and legal so Bot's own
// contract is testable before Operator's real behaviour exists.
type operatorBot struct{}

func (operatorBot) Decide(v game.PlayerView, cfg game.Config, r *rules.BotRNG) game.Order {
	return decideStayOrMove(v, cfg, r, "bot.operator.route")
}
