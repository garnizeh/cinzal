package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestResolveAddonsLedgerReportsPreRoundBalances is the acceptance
// criterion verbatim: "The Ledger reports pre-round balances — RFC §16.1's
// entry-snapshot row: buy a Ledger in a round with a delivery and assert
// the delivery is not in the figures." Runs the whole three-step slice
// (actions, deliveries, addons) so the delivery's own balance change is
// live by the time resolveAddons executes, and asserts the Ledger still
// reports the frozen entry snapshot, not that live figure.
func TestResolveAddonsLedgerReportsPreRoundBalances(t *testing.T) {
	s := actionsTestState(2, 2) // seat 0 delivers, seat 1 just sits at the Border too
	cfg := legalTestConfig()
	s.Players[0].Balance = 20
	s.Players[1].Balance = 7
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2, Tier: 0}}
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true, Contract: 0}
	entry := s.Snapshot() // frozen: Balance 20 and 7, before anything below runs

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionDeliver}, AddOns: game.AddOns{BuyLedger: true}},
		1: {Action: game.ActionOrder{Kind: game.ActionNothing}},
	}
	resolveDeliveries(&s, validated, cfg, globalEventContext{}) // moves seat 0's balance for real
	resolveAddons(&s, validated, entry, cfg, globalEventContext{})

	if s.Players[0].Balance == 20 {
		t.Fatalf("Balance = %d, want it to have moved (payment received) — test setup is broken", s.Players[0].Balance)
	}

	ledger := s.Players[0].Ledger
	if len(ledger) != 2 {
		t.Fatalf("Ledger = %+v, want 2 entries", ledger)
	}
	got := map[game.SeatID]int{}
	for _, e := range ledger {
		got[e.Seat] = e.Balance
	}
	if got[0] != 20 {
		t.Errorf("Ledger[seat 0] = %d, want 20 (the pre-round balance, not the post-delivery one)", got[0])
	}
	if got[1] != 7 {
		t.Errorf("Ledger[seat 1] = %d, want 7", got[1])
	}
}

// TestResolveAddonsLedgerRejectedInFinalRoundAtResolution is half of the
// acceptance criterion "The Ledger is rejected in the final round, at
// Legal and again at resolution" — this half is resolution's own
// defensive floor; TestLegalRejectsLedgerInFinalRound (legal_test.go)
// covers the Legal half.
func TestResolveAddonsLedgerRejectedInFinalRoundAtResolution(t *testing.T) {
	cfg := legalTestConfig()
	s := actionsTestState(2)
	s.Round = game.RoundNumber(cfg.Rounds) // the final round
	entry := s.Snapshot()

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionNothing}, AddOns: game.AddOns{BuyLedger: true}},
	}
	resolveAddons(&s, validated, entry, cfg, globalEventContext{})

	if s.Players[0].Ledger != nil {
		t.Errorf("Ledger = %+v, want nil (rejected in the final round)", s.Players[0].Ledger)
	}
	if s.Players[0].Balance != 20 {
		t.Errorf("Balance = %d, want 20 (no charge for a rejected purchase)", s.Players[0].Balance)
	}
}

// TestResolveAddonsLedgerUnaffordableSkipsSilently confirms the Ledger is a
// discretionary purchase, not Debt-eligible: a balance that can no longer
// cover LedgerCost by resolution time (e.g. spent earlier this same round)
// simply doesn't buy the Ledger — no purchase, no Debt cascade.
func TestResolveAddonsLedgerUnaffordableSkipsSilently(t *testing.T) {
	cfg := legalTestConfig()
	s := actionsTestState(2)
	s.Players[0].Balance = cfg.LedgerCost - 1
	entry := s.Snapshot()

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionNothing}, AddOns: game.AddOns{BuyLedger: true}},
	}
	events := resolveAddons(&s, validated, entry, cfg, globalEventContext{})

	if s.Players[0].Ledger != nil {
		t.Errorf("Ledger = %+v, want nil", s.Players[0].Ledger)
	}
	if s.Players[0].Balance != cfg.LedgerCost-1 {
		t.Errorf("Balance = %d, want unchanged", s.Players[0].Balance)
	}
	if s.Players[0].Flagged {
		t.Errorf("Flagged = true, want false — the Ledger must never trigger Debt")
	}
	if len(events) != 0 {
		t.Errorf("events = %+v, want none", events)
	}
}

// TestResolveAddonsRenewalExtendsAnAlreadyHeldPost is a direct smoke test
// of GDD §9.5/§10.4's remote lease renewal, on a post this seat already
// holds from a previous round (not staked this same round).
func TestResolveAddonsRenewalExtendsAnAlreadyHeldPost(t *testing.T) {
	cfg := legalTestConfig()
	s := actionsTestState(2)
	s.Players[0].Posts = []game.NodeID{1}
	s.Graph.Nodes[1].Post = &Post{Owner: 0, RoundsRemaining: 3}
	entry := s.Snapshot()

	validated := map[game.SeatID]game.Order{
		0: {
			Action: game.ActionOrder{Kind: game.ActionNothing},
			AddOns: game.AddOns{RenewPost: 1, RenewBlocks: 1},
		},
	}
	resolveAddons(&s, validated, entry, cfg, globalEventContext{})

	want := RenewedRoundsRemaining(3, 1, cfg)
	if got := s.Graph.Nodes[1].Post.RoundsRemaining; got != want {
		t.Errorf("RoundsRemaining = %d, want %d", got, want)
	}
	wantBalance := 20 - cfg.LeaseCostPerBlock
	if s.Players[0].Balance != wantBalance {
		t.Errorf("Balance = %d, want %d", s.Players[0].Balance, wantBalance)
	}
}

// TestResolveAddonsRenewalSkipsAFreshStakePostSameRound is D3's own guard:
// a StakePost order's AddOns fields describe that fresh stake, already
// fully consumed by resolveActions — resolveAddons must not also treat
// them as a renewal and charge the seat twice.
func TestResolveAddonsRenewalSkipsAFreshStakePostSameRound(t *testing.T) {
	cfg := legalTestConfig()
	s := actionsTestState(1, 0)
	entry := s.Snapshot()

	validated := map[game.SeatID]game.Order{
		0: {
			Action: game.ActionOrder{Kind: game.ActionStakePost},
			AddOns: game.AddOns{RenewPost: 1, RenewBlocks: 2},
		},
	}
	resolveActions(&s, validated, bySeat(s), cfg, globalEventContext{}, NewRNG(testSeed(1), 6))
	balanceAfterActions := s.Players[0].Balance
	roundsAfterActions := s.Graph.Nodes[1].Post.RoundsRemaining

	resolveAddons(&s, validated, entry, cfg, globalEventContext{})

	if s.Players[0].Balance != balanceAfterActions {
		t.Errorf("Balance = %d, want %d (unchanged by resolveAddons)", s.Players[0].Balance, balanceAfterActions)
	}
	if s.Graph.Nodes[1].Post.RoundsRemaining != roundsAfterActions {
		t.Errorf("RoundsRemaining = %d, want %d (unchanged by resolveAddons)",
			s.Graph.Nodes[1].Post.RoundsRemaining, roundsAfterActions)
	}
}

// TestResolveAddonsRenewalUsesPermitAuctionDiscount is GDD §14.2's Permit
// Auction: "Lease blocks cost Cr$ 2 instead of 3, next round only" —
// applied to a renewal's own block cost via leaseCostPerBlock (events.go).
func TestResolveAddonsRenewalUsesPermitAuctionDiscount(t *testing.T) {
	cfg := legalTestConfig()
	s := actionsTestState(2)
	s.Players[0].Posts = []game.NodeID{1}
	s.Graph.Nodes[1].Post = &Post{Owner: 0, RoundsRemaining: 3}
	entry := s.Snapshot()

	validated := map[game.SeatID]game.Order{
		0: {
			Action: game.ActionOrder{Kind: game.ActionNothing},
			AddOns: game.AddOns{RenewPost: 1, RenewBlocks: 1},
		},
	}
	resolveAddons(&s, validated, entry, cfg, globalEventContext{live: true, permitAuctionActive: true})

	wantBalance := 20 - permitAuctionCostPerBlock
	if s.Players[0].Balance != wantBalance {
		t.Errorf("Balance = %d, want %d (discounted rate, not cfg.LeaseCostPerBlock)", s.Players[0].Balance, wantBalance)
	}
}

// TestResolveActionsStakePostUsesPermitAuctionDiscount confirms the same
// discount applies to a fresh Stake Post's own block cost — GDD §14.2 says
// "lease blocks," not "renewals only" (events.go's leaseCostPerBlock doc).
func TestResolveActionsStakePostUsesPermitAuctionDiscount(t *testing.T) {
	cfg := legalTestConfig()
	s := actionsTestState(1, 0) // a second seat so PostCapByPlayers[2] applies
	validated := map[game.SeatID]game.Order{
		0: {
			Action: game.ActionOrder{Kind: game.ActionStakePost},
			AddOns: game.AddOns{RenewPost: 1, RenewBlocks: 1},
		},
	}
	resolveActions(&s, validated, bySeat(s), cfg, globalEventContext{live: true, permitAuctionActive: true}, NewRNG(testSeed(1), 6))

	wantBalance := 20 - permitAuctionCostPerBlock
	if s.Players[0].Balance != wantBalance {
		t.Errorf("Balance = %d, want %d (discounted rate)", s.Players[0].Balance, wantBalance)
	}
}

// TestResolveAddonsRenewalUnaffordableTriggersDebt confirms a renewal is
// Debt-eligible (GDD §13's literal list: "a lease renewal") — unlike the
// Ledger, an unaffordable renewal still extends the post and cascades
// through Debt rather than silently failing. A second post at a higher
// NodeID gives Debt's fewest-rounds-remaining surrender (#58's tie-break)
// something else to take, so the renewed post itself survives to prove
// the renewal really did still apply.
func TestResolveAddonsRenewalUnaffordableTriggersDebt(t *testing.T) {
	cfg := legalTestConfig()
	s := actionsTestState(2)
	s.Players[0].Balance = 0
	s.Players[0].Posts = []game.NodeID{1, 2}
	s.Graph.Nodes[1].Post = &Post{Owner: 0, RoundsRemaining: 6} // the post being renewed — must survive
	s.Graph.Nodes[2].Post = &Post{Owner: 0, RoundsRemaining: 3} // fewest remaining — Debt surrenders this one
	entry := s.Snapshot()

	validated := map[game.SeatID]game.Order{
		0: {
			Action: game.ActionOrder{Kind: game.ActionNothing},
			AddOns: game.AddOns{RenewPost: 1, RenewBlocks: 1},
		},
	}
	resolveAddons(&s, validated, entry, cfg, globalEventContext{})

	if !s.Players[0].Flagged {
		t.Errorf("Flagged = false, want true")
	}
	if s.Graph.Nodes[2].Post != nil {
		t.Errorf("Nodes[2].Post = %+v, want nil (surrendered — fewest rounds remaining)", s.Graph.Nodes[2].Post)
	}
	want := RenewedRoundsRemaining(6, 1, cfg)
	if got := s.Graph.Nodes[1].Post.RoundsRemaining; got != want {
		t.Errorf("RoundsRemaining = %d, want %d (renewal still applies despite Debt)", got, want)
	}
}
