package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestResolveDeliveriesPaysAndAnnounces is the acceptance criterion
// verbatim: "A delivery emits the global announcement with name, tier and
// location, to every seat, regardless of sight" — this issue's own scope is
// emitting the fact unconditionally (RFC §9.1's Anchor row, not a
// sight-gated Trail row); #71/Project own its distribution. It also checks
// the ordinary payout: payment minus nothing extra (the gate fee is
// deducted separately, see the Debt test below, and is affordable here),
// +RP, +Infamy, the contract and cargo cleared.
func TestResolveDeliveriesPaysAndAnnounces(t *testing.T) {
	s := actionsTestState(2) // seat 0 at node 2 (Border)
	cfg := legalTestConfig()
	tier := cfg.Contracts[0]
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2, Tier: 0}}
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true, Contract: 0}

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionDeliver}},
	}
	events := resolveDeliveries(&s, validated, cfg, globalEventContext{})

	wantBalance := 20 - cfg.GateFee + tier.Payment
	if s.Players[0].Balance != wantBalance {
		t.Errorf("Balance = %d, want %d", s.Players[0].Balance, wantBalance)
	}
	if s.Players[0].RP != tier.RP {
		t.Errorf("RP = %d, want %d", s.Players[0].RP, tier.RP)
	}
	if s.Players[0].Infamy != 1 { // Tier I/II delivery: +1 Infamy (GDD §11)
		t.Errorf("Infamy = %d, want 1", s.Players[0].Infamy)
	}
	if s.Players[0].Cargo != nil {
		t.Errorf("Cargo = %+v, want nil", s.Players[0].Cargo)
	}
	if len(s.Players[0].Contracts) != 0 {
		t.Errorf("Contracts = %+v, want none (fulfilled contract removed)", s.Players[0].Contracts)
	}
	if s.Players[0].ContractsDelivered != 1 {
		t.Errorf("ContractsDelivered = %d, want 1 (GDD §16's tiebreak counter)", s.Players[0].ContractsDelivered)
	}

	var found *game.Event
	for i := range events {
		if events[i].Kind == game.EventDelivered {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("events = %+v, want an EventDelivered", events)
	}
	if found.Seat != 0 || found.Node != 2 || found.Tier != 0 || found.Contract != 0 {
		t.Errorf("EventDelivered = %+v, want Seat 0, Node 2, Tier 0, Contract 0", found)
	}
}

// TestResolveDeliveriesGateFeeUnaffordableTriggersDebt is the acceptance
// criterion verbatim: "Gate fee unaffordable → Debt, in the §13 order, with
// the lease surrender tie-broken per #58." The gate fee is deducted from
// the seat's pre-delivery balance (before payment is added), which is the
// only ordering that makes this reachable at all — payment (8cr+) dwarfs a
// Cr$1 fee.
func TestResolveDeliveriesGateFeeUnaffordableTriggersDebt(t *testing.T) {
	s := actionsTestState(2)
	cfg := legalTestConfig()
	s.Players[0].Balance = 0
	s.Players[0].Infamy = 5
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2, Tier: 0}}
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true, Contract: 0}
	// Two posts so SelectLeaseByFewestRounds (#58, lowest NodeID tie-break)
	// has something to pick between.
	s.Players[0].Posts = []game.NodeID{0, 1}
	s.Graph.Nodes[0].Post = &Post{Owner: 0, RoundsRemaining: 3}
	s.Graph.Nodes[1].Post = &Post{Owner: 0, RoundsRemaining: 3}

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionDeliver}},
	}
	events := resolveDeliveries(&s, validated, cfg, globalEventContext{})

	if !s.Players[0].Flagged {
		t.Errorf("Flagged = false, want true (GDD §13 step 4)")
	}
	if s.Players[0].Infamy != 5-1+1 { // -1 Debt (§13 step 2), +1 Tier I delivery
		t.Errorf("Infamy = %d, want %d", s.Players[0].Infamy, 5-1+1)
	}
	if len(s.Players[0].Posts) != 1 || s.Players[0].Posts[0] != 1 {
		t.Errorf("Posts = %v, want [1] (node 0, the tied lowest NodeID, surrendered)", s.Players[0].Posts)
	}
	if s.Graph.Nodes[0].Post != nil {
		t.Errorf("Nodes[0].Post = %+v, want nil (surrendered)", s.Graph.Nodes[0].Post)
	}
	if !hasEvent(events, game.EventLeaseExpired, 0) {
		t.Errorf("events = %+v, want an EventLeaseExpired for the surrendered lease", events)
	}
	if !hasEvent(events, game.EventDelivered, 0) {
		t.Errorf("events = %+v, want the delivery to still complete", events)
	}
	// Balance: Debt pays what it has (0) toward the Cr$1 fee — the lease
	// credit only reduces what's forgiven, it is never cash in hand — then
	// the delivery's payment still lands in full.
	wantBalance := cfg.Contracts[0].Payment
	if s.Players[0].Balance != wantBalance {
		t.Errorf("Balance = %d, want %d", s.Players[0].Balance, wantBalance)
	}
}

// TestResolveDeliveriesGatesClosedHalvesPayment is GDD §14.2's Gates
// Closed: "Deliveries this round pay half. RP unaffected."
func TestResolveDeliveriesGatesClosedHalvesPayment(t *testing.T) {
	s := actionsTestState(2) // seat 0 at node 2 (Border)
	cfg := legalTestConfig()
	tier := cfg.Contracts[0]
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2, Tier: 0}}
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true, Contract: 0}

	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionDeliver}}}
	resolveDeliveries(&s, validated, cfg, globalEventContext{live: true, gatesClosedActive: true})

	wantBalance := 20 - cfg.GateFee + tier.Payment/2
	if s.Players[0].Balance != wantBalance {
		t.Errorf("Balance = %d, want %d (half payment, rounded down)", s.Players[0].Balance, wantBalance)
	}
	if s.Players[0].RP != tier.RP {
		t.Errorf("RP = %d, want %d (unaffected)", s.Players[0].RP, tier.RP)
	}
}

// TestResolveDeliveriesMarketSurgeAppliesFiftyPercentAndClears is GDD
// §14.2's Market Surge: "Each player's next delivery pays +50%" — a
// standing flag (Player.MarketSurgeActive) consumed on whichever delivery
// actually applies it, then cleared so it never fires twice.
func TestResolveDeliveriesMarketSurgeAppliesFiftyPercentAndClears(t *testing.T) {
	s := actionsTestState(2)
	cfg := legalTestConfig()
	tier := cfg.Contracts[0]
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2, Tier: 0}}
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true, Contract: 0}
	s.Players[0].MarketSurgeActive = true

	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionDeliver}}}
	resolveDeliveries(&s, validated, cfg, globalEventContext{})

	wantBalance := 20 - cfg.GateFee + tier.Payment + tier.Payment/2
	if s.Players[0].Balance != wantBalance {
		t.Errorf("Balance = %d, want %d (+50%%, rounded down)", s.Players[0].Balance, wantBalance)
	}
	if s.Players[0].MarketSurgeActive {
		t.Error("MarketSurgeActive still true after applying, want cleared")
	}
}

// TestResolveDeliveriesMarketSurgeThenGatesClosedStack confirms the
// documented stacking order (deliveries.go's resolveOneDelivery doc): +50%
// is applied to the base payment first, then Gates Closed's halving is
// applied to the result — "a final, round-wide reduction layered on top of
// a standing per-seat bonus."
func TestResolveDeliveriesMarketSurgeThenGatesClosedStack(t *testing.T) {
	s := actionsTestState(2)
	cfg := legalTestConfig()
	tier := cfg.Contracts[0]
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2, Tier: 0}}
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true, Contract: 0}
	s.Players[0].MarketSurgeActive = true

	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionDeliver}}}
	resolveDeliveries(&s, validated, cfg, globalEventContext{live: true, gatesClosedActive: true})

	surged := tier.Payment + tier.Payment/2
	wantBalance := 20 - cfg.GateFee + surged/2
	if s.Players[0].Balance != wantBalance {
		t.Errorf("Balance = %d, want %d ((payment*1.5)/2, both rounded down)", s.Players[0].Balance, wantBalance)
	}
}

// TestResolveDeliveriesUnboundCargoPaysFlatDeadRunnerRate is GDD §14.2's
// Dead Runner (and §14.3's Spilled Load): an unbound loose crate pays a
// flat Cr$ 12 and 3 RP, grants no Infamy, and removes no contract — the
// seat holds none for it.
func TestResolveDeliveriesUnboundCargoPaysFlatDeadRunnerRate(t *testing.T) {
	s := actionsTestState(2)
	cfg := legalTestConfig()
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false}

	validated := map[game.SeatID]game.Order{0: {Action: game.ActionOrder{Kind: game.ActionDeliver}}}
	events := resolveDeliveries(&s, validated, cfg, globalEventContext{})

	wantBalance := 20 - cfg.GateFee + deadRunnerPayout
	if s.Players[0].Balance != wantBalance {
		t.Errorf("Balance = %d, want %d", s.Players[0].Balance, wantBalance)
	}
	if s.Players[0].RP != deadRunnerRP {
		t.Errorf("RP = %d, want %d", s.Players[0].RP, deadRunnerRP)
	}
	if s.Players[0].Infamy != 0 {
		t.Errorf("Infamy = %d, want 0 (a loose crate is not 'delivering a contract', GDD §11)", s.Players[0].Infamy)
	}
	if s.Players[0].Cargo != nil {
		t.Errorf("Cargo = %+v, want nil", s.Players[0].Cargo)
	}
	if s.Players[0].ContractsDelivered != 0 {
		t.Errorf("ContractsDelivered = %d, want 0 (a loose crate is not a delivered contract, GDD §16)", s.Players[0].ContractsDelivered)
	}

	var found *game.Event
	for i := range events {
		if events[i].Kind == game.EventDelivered {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("events = %+v, want an EventDelivered", events)
	}
	if found.Contract != 0 || found.Tier != 0 {
		t.Errorf("EventDelivered = %+v, want zero-valued Contract/Tier (never bound to one)", found)
	}
}

// TestResolveDeliveriesUncontestedSeatOrder is a smoke test that
// resolveDeliveries iterates bySeat rather than byFairness (RFC §6.5:
// delivery carries no fairness dimension) — two independent deliveries in
// the same call both complete regardless of Infamy/Balance/RP ordering.
func TestResolveDeliveriesUncontestedSeatOrder(t *testing.T) {
	s := actionsTestState(2, 2)
	cfg := legalTestConfig()
	for seat := range 2 {
		s.Players[seat].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2, Tier: 0}}
		s.Players[seat].Cargo = &game.CarriedCargo{Bound: true, Contract: 0}
	}
	// Seat 0 has higher Infamy/Balance/RP than seat 1 — if this were
	// fairness-ordered it wouldn't matter for delivery either way, but
	// pins that both still resolve independently.
	s.Players[0].Infamy, s.Players[0].RP = 9, 9

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionDeliver}},
		1: {Action: game.ActionOrder{Kind: game.ActionDeliver}},
	}
	events := resolveDeliveries(&s, validated, cfg, globalEventContext{})

	if !hasEvent(events, game.EventDelivered, 0) || !hasEvent(events, game.EventDelivered, 1) {
		t.Fatalf("events = %+v, want an EventDelivered for both seats", events)
	}
	for seat := range 2 {
		if s.Players[seat].Cargo != nil {
			t.Errorf("seat %d Cargo = %+v, want nil", seat, s.Players[seat].Cargo)
		}
	}
}
