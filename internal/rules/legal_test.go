package rules

import (
	"errors"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// legalTestGraph builds a small, fixed fog view used by every adversarial
// test below:
//
//	0 (Warehouse, Known) --- 1 (Alley, Known) --- 2 (Border, Known) --- 3 (Hidden)
//	 \
//	  4 (Alley, Rumoured — no edges disclosed)
//
// The player starts at node 0, sees one opponent (so PostCapByPlayers[2]
// applies), and carries nothing.
func legalTestView() game.PlayerView {
	return game.PlayerView{
		You: game.SelfState{
			Position: 0,
			Balance:  20,
		},
		Others: []game.OpponentView{{Seat: 1}},
		Nodes: map[game.NodeID]game.NodeView{
			0: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{1, 4}},
			1: {Fog: game.FogKnown, Type: game.NodeAlley, Edges: []game.NodeID{0, 2}},
			2: {Fog: game.FogKnown, Type: game.NodeBorder, Edges: []game.NodeID{1, 3}},
			4: {Fog: game.FogRumoured, Type: game.NodeAlley},
			// 3 is deliberately absent: a genuinely Hidden node adjacent to 2.
		},
	}
}

func legalTestConfig() game.Config {
	return game.DefaultConfig()
}

func wantReason(t *testing.T, err error, want Reason) {
	t.Helper()
	if err == nil {
		t.Fatalf("Legal() = nil, want a %s error", want)
	}
	var ioe *IllegalOrderError
	if !errors.As(err, &ioe) {
		t.Fatalf("Legal() = %v (%T), want *IllegalOrderError", err, err)
	}
	if ioe.Reason != want {
		t.Fatalf("Legal() reason = %s, want %s", ioe.Reason, want)
	}
}

// TestLegalAcceptsWellFormedOrder is the positive baseline every adversarial
// test below is a mutation of.
func TestLegalAcceptsWellFormedOrder(t *testing.T) {
	v := legalTestView()
	o := game.Order{Route: []game.NodeID{1}, Action: game.ActionOrder{Kind: game.ActionNothing}}
	if err := Legal(v, o, legalTestConfig()); err != nil {
		t.Fatalf("Legal() = %v, want nil", err)
	}
}

// TestLegalRejectsNonAdjacentStep: GDD §15.0 row "A step between
// non-adjacent nodes".
func TestLegalRejectsNonAdjacentStep(t *testing.T) {
	v := legalTestView()
	// Node 0's edges are [1, 4] — 2 is not adjacent to 0.
	o := game.Order{Route: []game.NodeID{2}, Action: game.ActionOrder{Kind: game.ActionNothing}}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonNonAdjacentStep)
}

// TestLegalRejectsHiddenNodeNotLast: GDD §15.0 row "More than one Hidden
// node, or a Hidden node that isn't last" — and doubles as the row "A step
// out of a Hidden node without Pushing On declared", since both collapse to
// the same check (see legal.go's legalRoute doc).
func TestLegalRejectsHiddenNodeNotLast(t *testing.T) {
	v := legalTestView()
	// 0 -> 1 -> 2 -> 3 (Hidden, adjacent to 2) -> 1: a further step attempts
	// to leave the Hidden node.
	o := game.Order{Route: []game.NodeID{1, 2, 3, 1}, Action: game.ActionOrder{Kind: game.ActionNothing}}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonHiddenNodeNotLast)
}

// TestLegalAcceptsHiddenNodeAsLastEntry confirms the mirror case is legal:
// exploring into a Hidden node as the route's final step (GDD §9.1).
func TestLegalAcceptsHiddenNodeAsLastEntry(t *testing.T) {
	v := legalTestView()
	o := game.Order{Route: []game.NodeID{1, 2, 3}, Action: game.ActionOrder{Kind: game.ActionNothing}}
	if err := Legal(v, o, legalTestConfig()); err != nil {
		t.Fatalf("Legal() = %v, want nil (a Hidden node as the last route entry is exploration, GDD §9.1)", err)
	}
}

// TestLegalRejectsRouteReferencingAnyAbsentNodeNotLast is issue #62's
// acceptance criterion: "Legal cannot be satisfied by any order referencing
// a node absent from the view" — for any position but the last.
func TestLegalRejectsRouteReferencingAnyAbsentNodeNotLast(t *testing.T) {
	v := legalTestView()
	// 999 is a second, arbitrary Hidden neighbour of node 2 — not the one
	// wired into legalTestView by default — added here to show the check
	// holds for any absent NodeID, not just the one the fixture happens to
	// use elsewhere.
	node2 := v.Nodes[2]
	node2.Edges = append(node2.Edges, 999)
	v.Nodes[2] = node2

	o := game.Order{Route: []game.NodeID{1, 2, 999, 1}, Action: game.ActionOrder{Kind: game.ActionNothing}}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonHiddenNodeNotLast)
}

// TestLegalRejectsRouteThroughRumouredNode: GDD §15.0 row "A route plotted
// through nodes not Known to that player" — the row GDD §15.0 calls out for
// logging as well as rejection. GDD §7.1: a Rumoured node has "no edges, so
// you cannot plot a route through or into it."
func TestLegalRejectsRouteThroughRumouredNode(t *testing.T) {
	v := legalTestView()
	o := game.Order{Route: []game.NodeID{4}, Action: game.ActionOrder{Kind: game.ActionNothing}}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonNodeNotKnown)
}

// TestLegalRejectsRouteLongerThanStepAllowance: GDD §15.0 row "Route longer
// than your step allowance".
func TestLegalRejectsRouteLongerThanStepAllowance(t *testing.T) {
	v := legalTestView()
	v.You.Infamy = 9 // Legend tier: 2 steps per round.
	// 0 <-> 1 is a mutual edge, so this route is fully adjacency-legal —
	// only its length (5) should trip the check, against an allowance of 2.
	o := game.Order{Route: []game.NodeID{1, 0, 1, 0, 1}, Action: game.ActionOrder{Kind: game.ActionNothing}}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonStepAllowanceExceeded)
}

// TestLegalRejectsActionIllegalAtNode: GDD §15.0 row "An action illegal at
// the ending node's type" — Deliver off a non-Border node.
func TestLegalRejectsActionIllegalAtNode(t *testing.T) {
	v := legalTestView()
	o := game.Order{Route: []game.NodeID{1}, Action: game.ActionOrder{Kind: game.ActionDeliver}}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonIllegalActionAtNode)
}

// TestLegalRejectsDeliverWithoutDeliverableCargo checks the other half of
// the same row: a Border ending node with no cargo that could legally be
// delivered there.
func TestLegalRejectsDeliverWithoutDeliverableCargo(t *testing.T) {
	v := legalTestView()
	o := game.Order{Route: []game.NodeID{1, 2}, Action: game.ActionOrder{Kind: game.ActionDeliver}}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonIllegalActionAtNode)
}

// TestLegalAcceptsDeliverOfUnboundCargoAnywhereOnBorder confirms a loose
// crate (GDD §8.4, the Spilled Load boon) may be delivered to any Border,
// not only one tied to a contract.
func TestLegalAcceptsDeliverOfUnboundCargoAnywhereOnBorder(t *testing.T) {
	v := legalTestView()
	v.You.Cargo = &game.CarriedCargo{Bound: false}
	o := game.Order{Route: []game.NodeID{1, 2}, Action: game.ActionOrder{Kind: game.ActionDeliver}}
	if err := Legal(v, o, legalTestConfig()); err != nil {
		t.Fatalf("Legal() = %v, want nil (unbound cargo delivers to any Border)", err)
	}
}

// TestLegalRejectsPushingOnWithAction: GDD §15.0 row "Pushing On combined
// with an action".
func TestLegalRejectsPushingOnWithAction(t *testing.T) {
	v := legalTestView()
	o := game.Order{
		Route:     []game.NodeID{1, 2, 3},
		PushingOn: game.PushingOn{Steps: 1},
		Action:    game.ActionOrder{Kind: game.ActionDeliver},
	}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonPushingOnWithAction)
}

// TestLegalRejectsPushingOnWithoutHiddenEnd checks the structural
// precondition Pushing On depends on: it may only be appended to a route
// that ends on a Hidden node (GDD §9.1).
func TestLegalRejectsPushingOnWithoutHiddenEnd(t *testing.T) {
	v := legalTestView()
	o := game.Order{
		Route:     []game.NodeID{1},
		PushingOn: game.PushingOn{Steps: 1},
		Action:    game.ActionOrder{Kind: game.ActionNothing},
	}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonHiddenNodeNotLast)
}

// TestLegalRejectsAddOnsExceedingBalance: GDD §15.0 row "Add-ons or a lease
// exceeding your balance at submission".
func TestLegalRejectsAddOnsExceedingBalance(t *testing.T) {
	v := legalTestView()
	v.You.Balance = 0
	o := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		AddOns: game.AddOns{BuyLedger: true},
	}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonInsufficientBalance)
}

// TestLegalRejectsStakeBeyondPostCap: GDD §15.0 row "Staking beyond your
// post cap".
func TestLegalRejectsStakeBeyondPostCap(t *testing.T) {
	v := legalTestView()
	cfg := legalTestConfig()
	postCap := cfg.PostCapByPlayers[len(v.Others)+1]
	for i := range postCap {
		v.You.Posts = append(v.You.Posts, game.Post{Node: game.NodeID(100 + i), RoundsRemaining: 3})
	}
	o := game.Order{Action: game.ActionOrder{Kind: game.ActionStakePost}}
	wantReason(t, Legal(v, o, cfg), ReasonPostCapExceeded)
}

// TestLegalRejectsDealAtHandLimit: D14/#52's row — a Deal that would exceed
// the hand limit of 3 after this order's own Field 4 discards.
func TestLegalRejectsDealAtHandLimit(t *testing.T) {
	v := legalTestView()
	v.Nodes[0] = game.NodeView{Fog: game.FogKnown, Type: game.NodeBlackMarket, Edges: []game.NodeID{1, 4}}
	v.You.Items = []game.ItemID{game.ItemShiv, game.ItemMuscle, game.ItemPoliceBand}
	o := game.Order{Action: game.ActionOrder{Kind: game.ActionDeal}}
	wantReason(t, Legal(v, o, legalTestConfig()), ReasonHandLimitExceeded)
}

// TestLegalAcceptsDealAtHandLimitWithComposedDiscard confirms D14's
// resolution: a same-round Field 4 discard frees the slot a same-round Deal
// needs, because Field 4 discards resolve before movement while Deal
// resolves after (GDD §9.4; D14 case 3).
func TestLegalAcceptsDealAtHandLimitWithComposedDiscard(t *testing.T) {
	v := legalTestView()
	v.Nodes[0] = game.NodeView{Fog: game.FogKnown, Type: game.NodeBlackMarket, Edges: []game.NodeID{1, 4}}
	v.You.Items = []game.ItemID{game.ItemShiv, game.ItemMuscle, game.ItemPoliceBand}
	o := game.Order{
		Action: game.ActionOrder{Kind: game.ActionDeal},
		Items:  []game.ItemDiscard{{Item: game.ItemShiv}},
	}
	if err := Legal(v, o, legalTestConfig()); err != nil {
		t.Fatalf("Legal() = %v, want nil (a same-round discard frees the hand slot, D14 case 3)", err)
	}
}
