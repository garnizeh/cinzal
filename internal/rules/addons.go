package rules

import (
	"slices"

	"github.com/garnizeh/cinzal/internal/game"
)

// resolveAddons is GDD §15's Step N+3: the Ledger and lease renewal, in
// seat order (bySeat) — neither carries a fairness dimension. A Ledger
// purchase is private and touches nothing another seat could contest; a
// renewal extends a post this seat already holds exclusively.
func resolveAddons(s *MatchState, validated map[game.SeatID]game.Order, entry EntrySnapshot, cfg game.Config, ctx globalEventContext) []game.Event {
	var events []game.Event
	for _, seat := range bySeat(*s) {
		o := validated[seat]
		resolveLedger(s, seat, o, entry, cfg)
		events = append(events, resolveRenewal(s, seat, o, cfg, ctx)...)
	}
	return events
}

// resolveLedger applies GDD §5.1/§9.5's Ledger purchase for seat: every
// seat's exact balance as of the end of the previous round — entry, frozen
// at the top of Resolve (RFC §6.6), never live state, which by Step N+3
// has already moved from this round's own deliveries, stakes, and
// shakedowns. Skipped silently — no purchase, no Debt (confirmed: the
// Ledger, like Deal, is a discretionary purchase absent from GDD §13's
// Debt-trigger list) — when the balance can no longer cover LedgerCost by
// resolution time. The final-round reject belongs to Legal (legal.go) at
// submission; this is that same rule's defensive floor at resolution, per
// this issue's own acceptance criterion ("rejected in the final round, at
// Legal and again at resolution").
func resolveLedger(s *MatchState, seat game.SeatID, o game.Order, entry EntrySnapshot, cfg game.Config) {
	if !o.AddOns.BuyLedger || int(s.Round) >= cfg.Rounds {
		return
	}

	p := &s.Players[seat]
	if p.Balance < cfg.LedgerCost {
		return
	}
	p.Balance -= cfg.LedgerCost

	ledger := make([]game.LedgerEntry, len(entry.Seats))
	for i, snap := range entry.Seats {
		ledger[i] = game.LedgerEntry{Seat: snap.Seat, Balance: snap.Balance}
	}
	p.Ledger = ledger
}

// resolveRenewal applies GDD §10.4/§9.5's remote lease renewal for seat.
// AddOns.RenewBlocks/RenewPost do double duty (affordance.go): on a
// StakePost order they already named that fresh post's own block count,
// fully consumed by resolveActions (actions.go) — a seat whose Action this
// round was StakePost is skipped here, or the same declaration would be
// charged and applied twice. Otherwise RenewPost must currently be a post
// this seat holds; anything else (a stale declaration, or simply
// RenewBlocks left at its zero value) is a no-op.
//
// applyDebt runs before the lease is actually extended: GDD §13's cascade
// does not exempt the very lease a renewal is paying for, so a renewal
// whose own shortfall surrenders that same post (the fewest-rounds-
// remaining case Debt selects, SelectLeaseByFewestRounds — often exactly
// the post being renewed, since that is usually why it's being renewed at
// all) must not then try to extend a lease that no longer exists.
//
// cost is computed via leaseCostPerBlock (events.go), Permit Auction's
// (GDD §14.2, issue #72) discounted rate when live this round, the
// ordinary cfg.LeaseCostPerBlock otherwise.
func resolveRenewal(s *MatchState, seat game.SeatID, o game.Order, cfg game.Config, ctx globalEventContext) []game.Event {
	if o.AddOns.RenewBlocks <= 0 || o.Action.Kind == game.ActionStakePost {
		return nil
	}
	if !slices.Contains(s.Players[seat].Posts, o.AddOns.RenewPost) {
		return nil
	}

	cost := o.AddOns.RenewBlocks * leaseCostPerBlock(cfg, ctx)
	_, debtEvent := applyDebt(s, seat, cost, s.Round)

	var events []game.Event
	if debtEvent != nil {
		events = append(events, *debtEvent)
	}

	post := s.Graph.Nodes[o.AddOns.RenewPost].Post
	if post == nil {
		// Surrendered by this same renewal's own Debt cascade.
		return events
	}
	post.RoundsRemaining = RenewedRoundsRemaining(post.RoundsRemaining, o.AddOns.RenewBlocks, cfg)

	return events
}
