package bots

import (
	"slices"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// TestRunnerEvasiveWhileCarrying is RFC-001 §14.3's first Runner clause,
// fired both ways: Evasive the instant there is cargo to lose — bound or a
// loose crate — Neutral the instant there is not.
func TestRunnerEvasiveWhileCarrying(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x41)
	r := rules.NewBotRNG(seed, game.SeatID(0), 4)

	notCarrying := fixtureView(4, 2)
	if got := For(Runner).Decide(notCarrying, cfg, r).Stance; got.Stance != game.StanceNeutral {
		t.Fatalf("not carrying: Stance = %v, want Neutral", got.Stance)
	}

	bound := fixtureView(4, 2)
	bound.You.Cargo = &game.CarriedCargo{Bound: true, Contract: 1}
	if got := For(Runner).Decide(bound, cfg, r).Stance; got.Stance != game.StanceEvasive {
		t.Fatalf("carrying bound cargo: Stance = %v, want Evasive", got.Stance)
	}

	loose := fixtureView(4, 2)
	loose.You.Cargo = &game.CarriedCargo{Bound: false}
	if got := For(Runner).Decide(loose, cfg, r).Stance; got.Stance != game.StanceEvasive {
		t.Fatalf("carrying a loose crate: Stance = %v, want Evasive", got.Stance)
	}
}

// TestRunnerReserveBlocksLeaseRenewalAtFloor is RFC-001 §14.3's "keeps Cr$
// 4 in reserve for the shakedown ... including on leases" clause (issue
// #193), fired both ways against the one place Runner spends anything at
// all: renewing a lease about to lapse (GDD §18's own Autopilot
// heuristic). Cr$ 4 is cfg.ShakedownCost's own default value, read live
// rather than duplicated as a second constant.
func TestRunnerReserveBlocksLeaseRenewalAtFloor(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x42)
	r := rules.NewBotRNG(seed, game.SeatID(0), 4)

	base := fixtureView(4, 2)
	base.You.Posts = []game.Post{{Node: 4, RoundsRemaining: 1}} // about to lapse

	atFloor := base
	atFloor.You.Balance = cfg.ShakedownCost + cfg.LeaseCostPerBlock - 1 // one short after renewal
	if got := For(Runner).Decide(atFloor, cfg, r).AddOns; got.RenewBlocks != 0 {
		t.Fatalf("balance %d (renewal would drop below the Cr$ %d reserve): AddOns = %+v, want no lease spend",
			atFloor.You.Balance, cfg.ShakedownCost, got)
	}

	aboveFloor := base
	aboveFloor.You.Balance = cfg.ShakedownCost + cfg.LeaseCostPerBlock // exactly the reserve after renewal
	got := For(Runner).Decide(aboveFloor, cfg, r).AddOns
	if got.RenewBlocks != 1 || got.RenewPost != 4 {
		t.Fatalf("balance %d (renewal leaves exactly the reserve): AddOns = %+v, want a 1-block renewal of node 4",
			aboveFloor.You.Balance, got)
	}
}

// TestRunnerVanishesAboveComfortBand is RFC-001 §14.3's "Vanishes when
// Infamy exceeds its comfort band" clause, fired both ways at
// DefaultRunnerOptions' own default band of 8: strictly above it, with
// nothing more productive to do, Vanish is chosen; at the band exactly, it
// is not.
func TestRunnerVanishesAboveComfortBand(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x43)
	r := rules.NewBotRNG(seed, game.SeatID(0), 4)

	v := fixtureView(4, 2) // no cargo, no contracts — nothing else to do at the ending node

	v.You.Infamy = 9
	if got := For(Runner).Decide(v, cfg, r).Action.Kind; got != game.ActionVanish {
		t.Fatalf("Infamy 9 (above the comfort band of %d): Action = %v, want Vanish", DefaultRunnerOptions().InfamyComfortBand, got)
	}

	v.You.Infamy = 8
	if got := For(Runner).Decide(v, cfg, r).Action.Kind; got == game.ActionVanish {
		t.Fatal("Infamy 8 (at, not above, the comfort band): Action = Vanish, want anything else")
	}
}

// TestRunnerNeverDealsAtABlackMarketWithCashInHand is RFC-001 §14.3's
// "never buys" clause: standing at an in-sight Black Market with an item
// on offer and ample balance is exactly the situation a bot that did buy
// would act on, and Runner still never emits ActionDeal — chooseAction's
// own switch never names it as a candidate at all (see the golden-match
// test in internal/rules for the same assertion over a real 15-round
// match).
func TestRunnerNeverDealsAtABlackMarketWithCashInHand(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x48)

	v := game.PlayerView{
		Round: 4,
		You:   game.SelfState{Position: 1, Balance: 100},
		Nodes: map[game.NodeID]game.NodeView{
			1: {Fog: game.FogInSight, Type: game.NodeBlackMarket, Market: []game.ItemID{game.ItemDecoy}},
		},
	}

	r := rules.NewBotRNG(seed, game.SeatID(0), 4)
	if got := For(Runner).Decide(v, cfg, r).Action.Kind; got == game.ActionDeal {
		t.Fatal("Action = Deal, want anything else — RFC-001 §14.3's Runner never buys")
	}
}

// TestRunnerRouteTieBreaksByAscendingNodeID is issue #193's own "Path
// selection over the known graph is deterministic under ties, with the
// tie-break stated (node ID order)" criterion: a diamond graph with two
// equally-short paths to the objective (node 1 -> {2,3} -> 4) must always
// resolve to the lower-NodeID branch, on every call.
func TestRunnerRouteTieBreaksByAscendingNodeID(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x46)

	v := game.PlayerView{
		Round: 4,
		You: game.SelfState{
			Position:  1,
			Contracts: []game.ContractInHand{{ID: 1, Origin: 4, Destination: 4, ExpiresRound: 10}},
		},
		Nodes: map[game.NodeID]game.NodeView{
			1: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{2, 3}},
			2: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{1, 4}},
			3: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{1, 4}},
			4: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{2, 3}},
		},
	}

	want := []game.NodeID{2, 4}
	for i := range 5 {
		r := rules.NewBotRNG(seed, game.SeatID(0), 4)
		got := For(Runner).Decide(v, cfg, r).Route
		if !slices.Equal(got, want) {
			t.Fatalf("run %d: Route = %v, want %v (the ascending-NodeID tie-break)", i, got, want)
		}
	}
}

// TestRunnerPicksUpAndDelivers exercises the two halves of "shortest path
// to the current contract objective" actually paying off: Pickup once the
// route has reached the held contract's own Origin, Deliver once bound
// cargo is standing at that same contract's own Destination. Both orders
// are also checked against rules.Legal directly, tying this test to the
// same obligation bot_test.go's TestDecideProducesLegalOrders states for
// every tier.
func TestRunnerPicksUpAndDelivers(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x47)

	nodes := map[game.NodeID]game.NodeView{
		1: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{2}},
		2: {Fog: game.FogKnown, Type: game.NodeBorder, Edges: []game.NodeID{1}},
	}

	t.Run("pickup at the contract's own origin", func(t *testing.T) {
		v := game.PlayerView{
			Round: 4,
			You: game.SelfState{
				Position:  1,
				Contracts: []game.ContractInHand{{ID: 1, Origin: 1, Destination: 2, ExpiresRound: 10}},
			},
			Nodes: nodes,
		}
		r := rules.NewBotRNG(seed, game.SeatID(0), 4)
		got := For(Runner).Decide(v, cfg, r)

		if got.Action.Kind != game.ActionPickup {
			t.Fatalf("Action = %v, want Pickup", got.Action.Kind)
		}
		if len(got.Route) != 0 {
			t.Fatalf("Route = %v, want empty — already standing at the objective", got.Route)
		}
		if err := rules.Legal(v, got, cfg); err != nil {
			t.Fatalf("Legal(%+v) = %v", got, err)
		}
	})

	t.Run("deliver bound cargo at the contract's own destination", func(t *testing.T) {
		v := game.PlayerView{
			Round: 4,
			You: game.SelfState{
				Position:  2,
				Cargo:     &game.CarriedCargo{Bound: true, Contract: 1},
				Contracts: []game.ContractInHand{{ID: 1, Origin: 1, Destination: 2, ExpiresRound: 10}},
			},
			Nodes: nodes,
		}
		r := rules.NewBotRNG(seed, game.SeatID(1), 4)
		got := For(Runner).Decide(v, cfg, r)

		if got.Action.Kind != game.ActionDeliver {
			t.Fatalf("Action = %v, want Deliver", got.Action.Kind)
		}
		if got.Stance.Stance != game.StanceEvasive {
			t.Fatalf("Stance = %v, want Evasive while carrying", got.Stance.Stance)
		}
		if err := rules.Legal(v, got, cfg); err != nil {
			t.Fatalf("Legal(%+v) = %v", got, err)
		}
	})
}

// TestRunnerContractChoiceDeclinesUnreachableTakesReachable is RFC-001
// §14.3's "decline nothing it can plausibly finish" clause, against a
// pending offer whose distance genuinely exceeds its own deadline on this
// seat's own known subgraph — cfg.StepsByTier pinned to 1 so the budget
// arithmetic is exact and easy to state. Node 1 is the seat's position; a
// straight line to node 7 makes the Tier 1 offer's own round trip (5 + 1 =
// 6 steps) exceed its Deadline of 5, while the Tier 0 offer's round trip
// (1 + 1 = 2 steps) comfortably clears its Deadline of 4.
func TestRunnerContractChoiceDeclinesUnreachableTakesReachable(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.StepsByTier = [4]int{1, 1, 1, 1}
	seed := testSeed(0x44)

	edges := map[game.NodeID][]game.NodeID{
		1: {2}, 2: {1, 3}, 3: {2, 4}, 4: {3, 5}, 5: {4, 6}, 6: {5, 7}, 7: {6},
	}
	nodes := make(map[game.NodeID]game.NodeView, len(edges))
	for id, e := range edges {
		nodes[id] = game.NodeView{Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: e}
	}

	v := game.PlayerView{
		Round: 4,
		You: game.SelfState{
			Position: 1,
			PendingOffer: []game.ContractOffer{
				{Origin: 2, Destination: 3, Tier: 0}, // reachable
				{Origin: 6, Destination: 7, Tier: 1}, // not, within this seat's known map
			},
		},
		Nodes: nodes,
	}

	r := rules.NewBotRNG(seed, game.SeatID(0), 4)
	got := For(Runner).Decide(v, cfg, r).ContractChoice

	if got == nil {
		t.Fatal("ContractChoice = nil, want index 0 (the only plausibly-finishable offer)")
	}
	if *got != 0 {
		t.Fatalf("ContractChoice = %d, want 0", *got)
	}
}

// TestRunnerContractChoicePicksHighestReachableTier is RFC-001 §14.3's
// "take the highest-value tier it can reach" clause: two offers, both
// plausibly finishable on fixtureView's own default budget, must resolve
// to the higher tier even though it is also the farther one.
func TestRunnerContractChoicePicksHighestReachableTier(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x45)

	v := fixtureView(4, 2)
	v.You.PendingOffer = []game.ContractOffer{
		{Origin: 3, Destination: 4, Tier: 0},
		{Origin: 1, Destination: 4, Tier: 1},
	}

	r := rules.NewBotRNG(seed, game.SeatID(0), 4)
	got := For(Runner).Decide(v, cfg, r).ContractChoice

	if got == nil || *got != 1 {
		t.Fatalf("ContractChoice = %v, want index 1 (the higher tier, still reachable)", got)
	}
}
