package bots

import (
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// decideStayOrMove is Operator's placeholder decision, shared in name only
// with Runner's own former use of it: draw uniformly among the current
// position's Known-or-better neighbours and step to one, or stay put if
// none exist or this round's step allowance is zero. Drifter no longer
// uses it — drifter.go wires Sample (legalspace.go) in directly — and
// neither does Runner any more — runner.go (#193) replaced it with the
// tier's real greedy pathing. Operator still does, so Bot, Tier and For
// have a real, legal, stateless, RNG-driven Decide to be tested against
// until #194's cross-round planning replaces this file's use of it too,
// without touching Bot, Tier or For.
//
// The route step never reaches into a node this seat does not have Known
// fog on — mirroring rules.Legal's own legalRoute check (internal/rules/
// legal.go) — and the candidate set is built from the current node's own
// Edges slice, never by ranging v.Nodes, so it carries no RFC-001 §6.3
// map-iteration hazard.
func decideStayOrMove(v game.PlayerView, cfg game.Config, r *rules.BotRNG, purpose rules.BotPurpose) game.Order {
	order := game.Order{
		Round:  v.Round,
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}

	if rules.Steps(v, cfg) < 1 {
		return order
	}

	here, known := v.Nodes[v.You.Position]
	if !known || here.Fog < game.FogKnown {
		return order
	}

	var candidates []game.NodeID
	for _, to := range here.Edges {
		if toView, known := v.Nodes[to]; known && toView.Fog >= game.FogKnown {
			candidates = append(candidates, to)
		}
	}
	if len(candidates) == 0 {
		return order
	}

	order.Route = []game.NodeID{candidates[r.NextBot(purpose, len(candidates))]}
	return order
}
