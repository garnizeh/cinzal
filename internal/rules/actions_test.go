package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// actionsTestGraph is the fixed small graph every test in this file builds
// on: one node of each economically-relevant type.
//
//	0 (Warehouse) - 1 (Alley) - 2 (Border) - 3 (Black Market)
func actionsTestGraph() Graph {
	return Graph{Nodes: []Node{
		{ID: 0, Type: game.NodeWarehouse, Edges: []game.NodeID{1}},
		{ID: 1, Type: game.NodeAlley, Edges: []game.NodeID{0, 2}},
		{ID: 2, Type: game.NodeBorder, Edges: []game.NodeID{1, 3}},
		{ID: 3, Type: game.NodeBlackMarket, Edges: []game.NodeID{2}},
	}}
}

// actionsTestState returns a MatchState over actionsTestGraph with n seats,
// positioned at start (seat i at node start[i]), Balance 20, otherwise
// zero — a baseline every test overrides from, matching confrontTestState's
// own shape in confront_test.go.
func actionsTestState(start ...game.NodeID) MatchState {
	players := make([]Player, len(start))
	for i, pos := range start {
		players[i] = Player{Seat: game.SeatID(i), Balance: 20, Position: pos}
	}
	return MatchState{Round: 5, Graph: actionsTestGraph(), Players: players}
}

// TestResolveActionsOrdersByFairness is the acceptance criterion verbatim:
// "Action ordering is the fairness key, tested with a constructed
// four-level tie that reaches the coin." Two seats, tied through Infamy,
// Balance, and RP, contest the same dropped crate at the same node — only
// the fairness key's own coin decides which one gets it, so the winner
// resolveActions actually produces must match byFairness's own computed
// order for the identical seed, not merely be *some* deterministic pick.
func TestResolveActionsOrdersByFairness(t *testing.T) {
	build := func() MatchState {
		s := actionsTestState(2, 2) // both seats already at node 2 (Border)
		s.Players[0].Infamy, s.Players[1].Infamy = 3, 3
		s.Players[0].RP, s.Players[1].RP = 1, 1
		s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2}}
		s.Players[1].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2}}
		s.Graph.Cargo = []Cargo{{Node: 2, Bound: true, Origin: 0, Destination: 2}}
		return s
	}
	cfg := legalTestConfig()
	seed := testSeed(70)

	predict := build()
	order := byFairness(predict, NewRNG(seed, 6))
	winner := order[0]

	got := build()
	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionPickup}},
		1: {Action: game.ActionOrder{Kind: game.ActionPickup}},
	}
	resolveActions(&got, validated, bySeat(got), cfg, NewRNG(seed, 6))

	if got.Players[winner].Cargo == nil {
		t.Errorf("seat %d (byFairness's own first pick) has no cargo, want it to have won the crate", winner)
	}
	loser := game.SeatID(1)
	if winner == 1 {
		loser = 0
	}
	if got.Players[loser].Cargo != nil {
		t.Errorf("seat %d (not byFairness's first pick) has cargo, want nil", loser)
	}
}

// TestResolveActionsPickupBothSucceedAtSameWarehouse is the acceptance
// criterion verbatim: "Two players Pickup at the same Warehouse in the
// same round: both succeed" — GDD §6.2's warehouse supply limit was cut in
// v1.1, and a reimplementation of it (e.g. via the ground-cargo path) is a
// regression this test guards against.
func TestResolveActionsPickupBothSucceedAtSameWarehouse(t *testing.T) {
	s := actionsTestState(0, 0) // both seats already at node 0 (Warehouse)
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2}}
	s.Players[1].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2}}

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionPickup}},
		1: {Action: game.ActionOrder{Kind: game.ActionPickup}},
	}
	resolveActions(&s, validated, bySeat(s), legalTestConfig(), NewRNG(testSeed(1), 6))

	for _, seat := range []game.SeatID{0, 1} {
		p := s.Players[seat]
		if p.Cargo == nil || !p.Cargo.Bound || p.Cargo.Contract != 0 {
			t.Errorf("seat %d cargo = %+v, want bound cargo against contract 0", seat, p.Cargo)
		}
	}
	if len(s.Graph.Cargo) != 0 {
		t.Errorf("Graph.Cargo = %+v, want empty — a Warehouse Pickup consumes no ground cargo", s.Graph.Cargo)
	}
}

// TestResolveActionsPickupContestedGroundCargo is the acceptance criterion
// verbatim: "Two players contest a single dropped crate: the fairness key
// decides, deterministically, across 100 runs." Both seats hold a matching
// contract for the one dropped crate on the board; across 100 seeds, the
// crate always goes to exactly the seat byFairness's own order would name
// first, and never to both or neither.
func TestResolveActionsPickupContestedGroundCargo(t *testing.T) {
	build := func() MatchState {
		s := actionsTestState(2, 2)
		s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2}}
		s.Players[1].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2}}
		s.Graph.Cargo = []Cargo{{Node: 2, Bound: true, Origin: 0, Destination: 2}}
		return s
	}
	cfg := legalTestConfig()
	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionPickup}},
		1: {Action: game.ActionOrder{Kind: game.ActionPickup}},
	}

	for i := range 100 {
		seed := testSeed(byte(i))

		predict := build()
		winner := byFairness(predict, NewRNG(seed, 6))[0]

		got := build()
		resolveActions(&got, validated, bySeat(got), cfg, NewRNG(seed, 6))

		gotWinner, gotCount := game.SeatID(0), 0
		for _, seat := range []game.SeatID{0, 1} {
			if got.Players[seat].Cargo != nil {
				gotWinner, gotCount = seat, gotCount+1
			}
		}
		if gotCount != 1 {
			t.Fatalf("seed %d: %d seats hold the crate, want exactly 1", i, gotCount)
		}
		if gotWinner != winner {
			t.Fatalf("seed %d: crate went to seat %d, want byFairness's first pick, seat %d", i, gotWinner, winner)
		}
		if len(got.Graph.Cargo) != 0 {
			t.Fatalf("seed %d: Graph.Cargo = %+v, want the crate consumed", i, got.Graph.Cargo)
		}
	}
}

// TestResolveActionsConfrontationLoserRunsNoAction is the acceptance
// criterion verbatim: "A confrontation loser's action does not execute" —
// #69's haltMovement (confront.go) already nulls a loser's Action to
// Nothing before resolveActions ever runs; this proves resolveActions
// itself performs no mutation and emits no event for a Nothing action,
// even one standing on a node with a collectible crate.
func TestResolveActionsConfrontationLoserRunsNoAction(t *testing.T) {
	s := actionsTestState(2)
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2}}
	s.Graph.Cargo = []Cargo{{Node: 2, Bound: true, Origin: 0, Destination: 2}}

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionNothing}}, // haltMovement's own result
	}
	events := resolveActions(&s, validated, bySeat(s), legalTestConfig(), NewRNG(testSeed(1), 6))

	if s.Players[0].Cargo != nil {
		t.Errorf("Cargo = %+v, want nil — a Nothing action must not pick up", s.Players[0].Cargo)
	}
	if len(s.Graph.Cargo) != 1 {
		t.Errorf("Graph.Cargo = %+v, want the crate untouched", s.Graph.Cargo)
	}
	if len(events) != 0 {
		t.Errorf("events = %+v, want none", events)
	}
}

// TestResolveActionsStakePost is a direct smoke test of GDD §9.2/§10.4's
// Stake Post: a fresh post is created at the block count and cost
// AddOns.RenewBlocks declares (affordance.go's documented reuse of the
// AddOns fields for a fresh stake), Infamy gains GDD §11's first-post-in-
// sector bonus, and EventPostStaked fires.
func TestResolveActionsStakePost(t *testing.T) {
	s := actionsTestState(1, 0) // seat 1 unused, present only so PostCapByPlayers[2] applies
	cfg := legalTestConfig()

	validated := map[game.SeatID]game.Order{
		0: {
			Action: game.ActionOrder{Kind: game.ActionStakePost},
			AddOns: game.AddOns{RenewPost: 1, RenewBlocks: 2},
		},
	}
	events := resolveActions(&s, validated, bySeat(s), cfg, NewRNG(testSeed(1), 6))

	post := s.Graph.Nodes[1].Post
	if post == nil || post.Owner != 0 {
		t.Fatalf("Nodes[1].Post = %+v, want owned by seat 0", post)
	}
	wantRounds := RenewedRoundsRemaining(0, 2, cfg)
	if post.RoundsRemaining != wantRounds {
		t.Errorf("RoundsRemaining = %d, want %d", post.RoundsRemaining, wantRounds)
	}
	wantBalance := 20 - 2*cfg.LeaseCostPerBlock
	if s.Players[0].Balance != wantBalance {
		t.Errorf("Balance = %d, want %d", s.Players[0].Balance, wantBalance)
	}
	if s.Players[0].Infamy != InfamyGainFirstPost {
		t.Errorf("Infamy = %d, want %d (first post in sector)", s.Players[0].Infamy, InfamyGainFirstPost)
	}
	if !hasEvent(events, game.EventPostStaked, 0) {
		t.Errorf("events = %+v, want an EventPostStaked for seat 0", events)
	}
}

// TestResolveActionsStakePostAlreadyOwnedWithinSameStep proves the
// Step-N+1-time re-verification D4 describes: node 1 is already owned by
// seat 1 by the time seat 0's own Stake Post would execute (simulating an
// earlier, lower-Infamy seat in the same fairness order having staked it
// this round) — seat 0's stake must not execute, and the degradation event
// fires again at this later point.
func TestResolveActionsStakePostAlreadyOwnedWithinSameStep(t *testing.T) {
	s := actionsTestState(1)
	s.Graph.Nodes[1].Post = &Post{Owner: 1, RoundsRemaining: 3}

	validated := map[game.SeatID]game.Order{
		0: {
			Action: game.ActionOrder{Kind: game.ActionStakePost},
			AddOns: game.AddOns{RenewPost: 1, RenewBlocks: 1},
		},
	}
	events := resolveActions(&s, validated, bySeat(s), legalTestConfig(), NewRNG(testSeed(1), 6))

	if s.Players[0].Balance != 20 {
		t.Errorf("Balance = %d, want 20 (unaffordable stake never charged)", s.Players[0].Balance)
	}
	if s.Graph.Nodes[1].Post.Owner != 1 {
		t.Errorf("Nodes[1].Post.Owner = %d, want 1 (unchanged)", s.Graph.Nodes[1].Post.Owner)
	}
	if !hasEvent(events, game.EventStakeTargetTaken, 0) {
		t.Errorf("events = %+v, want an EventStakeTargetTaken for seat 0", events)
	}
}

// TestResolveActionsDeal is a direct smoke test of GDD §9.2/§12's Deal: the
// declared item moves from the market's stock into the buyer's hand, the
// price is deducted, and EventItemPurchased fires.
func TestResolveActionsDeal(t *testing.T) {
	s := actionsTestState(3)
	s.Graph.Nodes[3].Market = []game.ItemID{game.ItemShiv, game.ItemMuscle, game.ItemTornMap}

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionDeal, Item: game.ItemMuscle}},
	}
	events := resolveActions(&s, validated, bySeat(s), legalTestConfig(), NewRNG(testSeed(1), 6))

	if want := 20 - itemPrice(game.ItemMuscle); s.Players[0].Balance != want {
		t.Errorf("Balance = %d, want %d", s.Players[0].Balance, want)
	}
	found := false
	for _, it := range s.Players[0].Items {
		found = found || it == game.ItemMuscle
	}
	if !found {
		t.Errorf("Items = %v, want Muscle", s.Players[0].Items)
	}
	if len(s.Graph.Nodes[3].Market) != 2 {
		t.Errorf("Market = %v, want 2 items remaining", s.Graph.Nodes[3].Market)
	}
	if !hasEvent(events, game.EventItemPurchased, 0) {
		t.Errorf("events = %+v, want an EventItemPurchased for seat 0", events)
	}
}

// TestResolveActionsDealItemAlreadyGone covers the contested case: the
// declared item is no longer in the market's stock (an earlier seat this
// same fairness order already bought it) — the Deal silently fails, no
// purchase, no event, matching D14's Open Doors precedent for a legal
// declaration whose target moved under it.
func TestResolveActionsDealItemAlreadyGone(t *testing.T) {
	s := actionsTestState(3)
	s.Graph.Nodes[3].Market = []game.ItemID{game.ItemShiv}

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionDeal, Item: game.ItemMuscle}},
	}
	events := resolveActions(&s, validated, bySeat(s), legalTestConfig(), NewRNG(testSeed(1), 6))

	if s.Players[0].Balance != 20 {
		t.Errorf("Balance = %d, want 20 (no purchase)", s.Players[0].Balance)
	}
	if len(s.Players[0].Items) != 0 {
		t.Errorf("Items = %v, want none", s.Players[0].Items)
	}
	if len(events) != 0 {
		t.Errorf("events = %+v, want none", events)
	}
}

// TestResolveActionsVanish is a direct smoke test of GDD §9.2/§11's
// Vanish: −2 Infamy, floored at 0 (ApplyInfamyDelta), and no event — Vanish
// is not one of GDD §7.3's eight trail archetypes.
func TestResolveActionsVanish(t *testing.T) {
	s := actionsTestState(1)
	s.Players[0].Infamy = 1

	validated := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionVanish}},
	}
	events := resolveActions(&s, validated, bySeat(s), legalTestConfig(), NewRNG(testSeed(1), 6))

	if s.Players[0].Infamy != 0 {
		t.Errorf("Infamy = %d, want 0 (floored, not -1)", s.Players[0].Infamy)
	}
	if len(events) != 0 {
		t.Errorf("events = %+v, want none", events)
	}
}

// TestApplyDiscardsRemovesDeclaredItems is the minimal Field-4 hand
// bookkeeping this issue's scope covers: a declared discard leaves the
// hand, once per declared copy, with no effect of its own firing.
func TestApplyDiscardsRemovesDeclaredItems(t *testing.T) {
	s := actionsTestState(1)
	s.Players[0].Items = []game.ItemID{game.ItemShiv, game.ItemShiv, game.ItemMuscle}

	o := game.Order{Items: []game.ItemDiscard{{Item: game.ItemShiv}}}
	applyDiscards(&s, 0, o)

	want := []game.ItemID{game.ItemShiv, game.ItemMuscle}
	if len(s.Players[0].Items) != len(want) {
		t.Fatalf("Items = %v, want %v", s.Players[0].Items, want)
	}
	for i, it := range want {
		if s.Players[0].Items[i] != it {
			t.Errorf("Items[%d] = %v, want %v", i, s.Players[0].Items[i], it)
		}
	}
}
