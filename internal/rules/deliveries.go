package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// resolveDeliveries is GDD §15's Step N+2: Deliver, for every seat validated
// declared it, in seat order (bySeat) rather than the fairness key —
// delivery is uncontested (RFC §6.5: "position confers no advantage" —
// nobody else can take your cargo out from under a Deliver), so it carries
// none of Step N+1's contention and gets the plain seat-index default.
//
// Gate fee is deducted before payment is added, from the seat's
// pre-delivery balance (applyDebt, debt.go) — the only order that makes
// "gate fee unaffordable → Debt" reachable at all, since an ordinary
// delivery's payment (8-30cr) dwarfs the Cr$1 fee and would make the debt
// case unreachable if applied after.
func resolveDeliveries(s *MatchState, validated map[game.SeatID]game.Order, cfg game.Config, ctx globalEventContext) []game.Event {
	var events []game.Event
	for _, seat := range bySeat(*s) {
		if validated[seat].Action.Kind != game.ActionDeliver {
			continue
		}
		events = append(events, resolveOneDelivery(s, seat, cfg, ctx)...)
	}
	return events
}

// resolveOneDelivery applies GDD §8.3/§15's delivery payout for seat at its
// current (post-movement) position: the gate fee, payment, RP, Infamy, the
// fulfilled contract's removal, and the unconditional global announcement
// (GDD §7.3: "There is no quiet victory" — EventDelivered is an RFC §9.1
// Anchor row, not a sight-gated Trail row, so it is emitted here
// regardless of who has sight of what; #71/Project distribute it to every
// seat unconditionally).
//
// An unbound loose crate pays a flat rate rather than the tier table, and
// grants no Infamy (unlike a contract delivery's own +1/+2 — GDD §11 states
// that gain against "delivering any contract," which a loose crate, by
// definition, is not one of) and removes no contract, since it was never
// bound to one. Two sources produce a loose crate, at different flat
// rates: Dead Runner (GDD §14.2, events.go's resolveDeadRunner) pays Cr$
// 12/3 RP; Spilled Load (GDD §14.3, incidents.go's resolveSpilledLoad)
// pays Cr$ 10/2 RP — Cargo.SpilledLoad (state.go), carried through pickup
// onto game.CarriedCargo.SpilledLoad (actions.go's resolvePickup), is the
// only thing that tells the two apart once both are just "an unbound crate
// on the ground."
//
// ctx.gatesClosedActive (Gates Closed, GDD §14.2) halves whatever payment
// this delivery would otherwise pay, rounded down, RP unaffected — applied
// after Market Surge, so "this round pays half" reads as a final,
// round-wide reduction layered on top of a standing per-seat bonus rather
// than the other way around. Market Surge's own +50% (Player.
// MarketSurgeActive, resolveMarketSurge — events.go) is consumed and
// cleared here, on whichever delivery — bound or loose — actually applies
// it, since the card text names "next delivery," not "next contract
// delivery."
func resolveOneDelivery(s *MatchState, seat game.SeatID, cfg game.Config, ctx globalEventContext) []game.Event {
	p := &s.Players[seat]
	node := p.Position

	if p.Cargo == nil {
		return nil
	}

	var payment, rp, infamyGain int
	var contract Contract
	bound := p.Cargo.Bound

	if bound {
		idx := slices.IndexFunc(p.Contracts, func(c Contract) bool { return c.ID == p.Cargo.Contract })
		if idx < 0 {
			return nil
		}
		contract = p.Contracts[idx]
		payment, rp, infamyGain = Deliver(contract, cfg)
	} else if p.Cargo.SpilledLoad {
		payment, rp, infamyGain = spilledLoadPayout, spilledLoadRP, 0
	} else {
		payment, rp, infamyGain = deadRunnerPayout, deadRunnerRP, 0
	}

	if p.MarketSurgeActive {
		payment += payment / 2
		p.MarketSurgeActive = false
	}
	if ctx.live && ctx.gatesClosedActive {
		payment /= 2
	}

	var events []game.Event
	if _, e := applyDebt(s, seat, cfg.GateFee, s.Round); e != nil {
		events = append(events, *e)
	}

	p.Balance += payment
	p.RP += rp
	p.Infamy = ApplyInfamyDelta(p.Infamy, infamyGain)

	ev := game.Event{Kind: game.EventDelivered, Round: s.Round, Node: node, Seat: seat}
	if bound {
		idx := slices.IndexFunc(p.Contracts, func(c Contract) bool { return c.ID == contract.ID })
		p.Contracts = slices.Delete(p.Contracts, idx, idx+1)
		ev.Contract = contract.ID
		ev.Tier = contract.Tier
	}
	p.Cargo = nil

	return append(events, ev)
}
