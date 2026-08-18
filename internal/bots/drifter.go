package bots

import (
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// drifterBot is the Drifter tier (RFC-001 §14.3): "uniform random legal
// order," the statistical baseline the property tests and every sweep
// number in M2 are read against. Decide is Sample (legalspace.go) wired in
// whole, unmodified — Drifter adds no behaviour of its own beyond that
// wiring, because Sample already draws each of an order's fields uniformly
// from its own legal candidate set, which is the scheme the package doc
// comment states and disclaims.
type drifterBot struct{}

func (drifterBot) Decide(v game.PlayerView, cfg game.Config, r *rules.BotRNG) game.Order {
	return Sample(v, cfg, r)
}
