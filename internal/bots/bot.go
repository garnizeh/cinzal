package bots

import (
	"fmt"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// Bot decides one seat's order for one round from its own view alone
// (RFC-001 §14). Decide is a pure function of its three arguments — same
// view, same config, same BotRNG draw sequence produce the same order, byte
// for byte — and an implementation must hold no field that accumulates
// across Decide calls (package doc comment).
//
// Decide must return an order rules.Legal(v, o, cfg) accepts. An illegal
// order is a bug in the bot that produced it, not an input the tick has to
// defend against — the tick re-validates everything regardless (RFC-001
// §19) — so this is a test obligation on each tier, not a runtime check
// here.
type Bot interface {
	Decide(v game.PlayerView, cfg game.Config, r *rules.BotRNG) game.Order
}

// Tier is one of RFC-001 §14.3's three difficulty tiers. The same
// implementation serves all four roles a Bot is asked to play — filler,
// autopilot, simulation, practice (§14.2) — nothing in this package, and
// nothing on Tier, knows which role a given Decide call is serving; the role
// lives entirely in the caller.
type Tier uint8

// The three doc comments below state each tier's RFC-001 §14.3 target
// behaviour. Drifter's Decide is now that behaviour: Sample (legalspace.go),
// wired in by drifter.go. Runner and Operator's Decide is still, for now,
// the same provisional decideStayOrMove (decide.go) — their real greedy
// pathing and cross-round planning land in #193 and #194 respectively, each
// replacing only its own tier's file.
const (
	_ Tier = iota // zero is reserved invalid, matching internal/game's enum convention

	// Drifter draws uniformly from the legal order space, one field at a
	// time (legalspace.go's Sample, package doc comment): the statistical
	// baseline for the property tests and for the sweep numbers everything
	// else is read against (RFC-001 §14.3).
	Drifter

	// Runner will be greedy — shortest path to the current contract
	// objective, Evasive while carrying, keeps a shakedown reserve, never
	// buys. It is the Autopilot default (RFC-001 §8.2, §14.2, §14.3).
	Runner

	// Operator will plan across rounds — reading the heat map for
	// chokepoints, routing around unstable sectors, timing its Infamy
	// climb (RFC-001 §14.3). It is deliberately not meant to be
	// superhuman: a lopsided win rate against humans will be a balance
	// finding, not a bot achievement.
	Operator
)

// String returns the tier's RFC-001 §14.3 name, or "Tier(n)" for an invalid
// value.
func (t Tier) String() string {
	switch t {
	case Drifter:
		return "Drifter"
	case Runner:
		return "Runner"
	case Operator:
		return "Operator"
	default:
		return fmt.Sprintf("Tier(%d)", uint8(t))
	}
}

// For returns tier's Bot. The returned value is stateless and may be shared
// across every Decide call, for every seat and every round: two calls
// sharing nothing but a view cannot collude (RFC-001 §14.5), and a freshly
// constructed Bot driven through the same views as an already-used one
// produces the identical order sequence (package doc comment).
//
// For panics on a Tier outside the three declared constants — a caller
// passing an undeclared Tier is a programming error, not a runtime
// condition to degrade from, the same discipline RNG.Next holds n <= 0 to.
func For(t Tier) Bot {
	switch t {
	case Drifter:
		return drifterBot{}
	case Runner:
		return runnerBot{}
	case Operator:
		return operatorBot{}
	default:
		panic(fmt.Sprintf("bots: For(%d): not a declared Tier", uint8(t)))
	}
}
