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
func resolveDeliveries(s *MatchState, validated map[game.SeatID]game.Order, cfg game.Config) []game.Event {
	var events []game.Event
	for _, seat := range bySeat(*s) {
		if validated[seat].Action.Kind != game.ActionDeliver {
			continue
		}
		events = append(events, resolveOneDelivery(s, seat, cfg)...)
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
// Only the bound-cargo case (a held contract) is implemented: GDD's two
// loose-crate deliveries (Dead Runner, Spilled Load) are both boon-card
// payouts, and no card-issuing code exists yet (#72/#73) — so an unbound
// delivery cannot occur in practice ahead of that work. It is a documented
// no-op here rather than a guess at a payout, so a future boon lands
// cleanly instead of colliding with an invented figure.
func resolveOneDelivery(s *MatchState, seat game.SeatID, cfg game.Config) []game.Event {
	p := &s.Players[seat]
	node := p.Position

	if p.Cargo == nil || !p.Cargo.Bound {
		return nil
	}

	idx := slices.IndexFunc(p.Contracts, func(c Contract) bool { return c.ID == p.Cargo.Contract })
	if idx < 0 {
		return nil
	}
	contract := p.Contracts[idx]

	var events []game.Event
	if _, e := applyDebt(s, seat, cfg.GateFee, s.Round); e != nil {
		events = append(events, *e)
	}

	payment, rp, infamyGain := Deliver(contract, cfg)
	p.Balance += payment
	p.RP += rp
	p.Infamy = ApplyInfamyDelta(p.Infamy, infamyGain)

	p.Contracts = slices.Delete(p.Contracts, idx, idx+1)
	p.Cargo = nil

	return append(events, game.Event{
		Kind:     game.EventDelivered,
		Round:    s.Round,
		Node:     node,
		Seat:     seat,
		Contract: contract.ID,
		Tier:     contract.Tier,
	})
}
