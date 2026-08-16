package rules

import (
	"reflect"
	"slices"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestValidateAbsenceDefaultOnMissingOrder is GDD §18's default order on
// absence, applied when a seat submitted nothing this round: empty route,
// Evasive stance, no add-ons, Nothing unless canAutoDeliverHere finds this
// seat already standing on a Border with deliverable cargo.
func TestValidateAbsenceDefaultOnMissingOrder(t *testing.T) {
	s := resolveTestState() // seat 0 at node 0 (Warehouse), seat 1 at node 2 (Border)
	s.Players[1].Cargo = &game.CarriedCargo{Bound: false}
	entry := s.Snapshot()
	seats := bySeat(s)

	validated, events := validate(s, entry, seats, nil, legalTestConfig(), globalEventContext{}, nil)

	want0 := game.Order{Action: game.ActionOrder{Kind: game.ActionNothing}, Stance: game.StanceOrder{Stance: game.StanceEvasive}}
	if got := validated[0]; !reflect.DeepEqual(got, want0) {
		t.Errorf("validated[0] = %+v, want %+v (not standing on a Border)", got, want0)
	}

	want1 := game.Order{Action: game.ActionOrder{Kind: game.ActionDeliver}, Stance: game.StanceOrder{Stance: game.StanceEvasive}}
	if got := validated[1]; !reflect.DeepEqual(got, want1) {
		t.Errorf("validated[1] = %+v, want %+v (standing on a Border with deliverable cargo)", got, want1)
	}

	for _, e := range events {
		if e.Seat == 0 || e.Seat == 1 {
			t.Errorf("absence default produced an event %+v; a missing order is not a rejection or degradation", e)
		}
	}
}

// TestValidateRejectsIllegalPayload is the acceptance criterion verbatim:
// "Every GDD §15.0 illegal-payload row falls back to the absence default,
// never to partial execution." legal_test.go already exhaustively covers
// every Reason row against Legal itself; this proves validate wires that
// check in and reacts correctly, using one representative row — a route
// into node 4, which is only Rumoured (no live-edge truncation applies:
// the edge 0→4 genuinely exists, so this exercises Legal's own
// ReasonNodeNotKnown rather than the truncation path).
func TestValidateRejectsIllegalPayload(t *testing.T) {
	s := resolveTestState()
	entry := s.Snapshot()
	seats := bySeat(s)
	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{4}},
	}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{}, nil)

	want := game.Order{Action: game.ActionOrder{Kind: game.ActionNothing}, Stance: game.StanceOrder{Stance: game.StanceEvasive}}
	if got := validated[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("validated[0] = %+v, want the absence default %+v", got, want)
	}

	if !hasEvent(events, game.EventOrderRejected, 0) {
		t.Errorf("events = %+v, want an EventOrderRejected for seat 0", events)
	}
}

// TestValidateDegradesDestroyedEdge covers GDD §15.0 Step 0's canonical
// degradation example: a route step whose edge no longer exists in the
// live graph. Node 1 has no edge to node 3, so the route truncates after
// node 1, and the (now Nothing) action passes Legal on what survives.
func TestValidateDegradesDestroyedEdge(t *testing.T) {
	s := resolveTestState()
	entry := s.Snapshot()
	seats := bySeat(s)
	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1, 3}, Action: game.ActionOrder{Kind: game.ActionNothing}},
	}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{}, nil)

	got := validated[0]
	if !slices.Equal(got.Route, []game.NodeID{1}) {
		t.Errorf("validated[0].Route = %v, want [1] (truncated before the missing 1→3 edge)", got.Route)
	}
	if got.Action.Kind != game.ActionNothing {
		t.Errorf("validated[0].Action.Kind = %v, want Nothing", got.Action.Kind)
	}

	if !hasEvent(events, game.EventRouteTruncated, 0) {
		t.Errorf("events = %+v, want an EventRouteTruncated for seat 0", events)
	}
}

// TestValidateDegradesStakeTargetTaken covers the first of the two
// degradation checks legal.go itself documents as having no Legal
// equivalent: a Stake Post target already owned by someone else.
func TestValidateDegradesStakeTargetTaken(t *testing.T) {
	s := resolveTestState()
	s.Graph.Nodes[1].Post = &Post{Owner: 1, RoundsRemaining: 3}
	entry := s.Snapshot()
	seats := bySeat(s)
	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1}, Action: game.ActionOrder{Kind: game.ActionStakePost}},
	}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{}, nil)

	got := validated[0]
	if !slices.Equal(got.Route, []game.NodeID{1}) {
		t.Errorf("validated[0].Route = %v, want [1] unchanged (only the action degrades)", got.Route)
	}
	if got.Action.Kind != game.ActionNothing {
		t.Errorf("validated[0].Action.Kind = %v, want Nothing", got.Action.Kind)
	}

	if !hasEvent(events, game.EventStakeTargetTaken, 0) {
		t.Errorf("events = %+v, want an EventStakeTargetTaken for seat 0", events)
	}
}

// TestValidateStakeOwnPostDoesNotDegrade checks the seat-comparison this
// degradation relies on: a Stake Post target the seat already owns is not
// "someone else's" post, so it must not degrade or fire
// EventStakeTargetTaken.
func TestValidateStakeOwnPostDoesNotDegrade(t *testing.T) {
	s := resolveTestState()
	s.Graph.Nodes[1].Post = &Post{Owner: 0, RoundsRemaining: 3}
	entry := s.Snapshot()
	seats := bySeat(s)
	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1}, Action: game.ActionOrder{Kind: game.ActionStakePost}},
	}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{}, nil)

	got := validated[0]
	if got.Action.Kind != game.ActionStakePost {
		t.Errorf("validated[0].Action.Kind = %v, want ActionStakePost unchanged (own post, not taken)", got.Action.Kind)
	}

	if hasEvent(events, game.EventStakeTargetTaken, 0) {
		t.Errorf("events = %+v, want no EventStakeTargetTaken for seat 0 staking its own post", events)
	}
}

// TestValidateDegradesPickupTargetGone covers the second of legal.go's
// named Resolve-only degradation checks: a Pickup whose declared cargo
// isn't on the ground. Legal deliberately never checks Pickup at all, so
// this exercises a path Legal itself would have passed.
func TestValidateDegradesPickupTargetGone(t *testing.T) {
	s := resolveTestState() // no Graph.Cargo entries anywhere
	entry := s.Snapshot()
	seats := bySeat(s)
	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1}, Action: game.ActionOrder{Kind: game.ActionPickup}},
	}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{}, nil)

	got := validated[0]
	if got.Action.Kind != game.ActionNothing {
		t.Errorf("validated[0].Action.Kind = %v, want Nothing", got.Action.Kind)
	}

	if !hasEvent(events, game.EventPickupTargetGone, 0) {
		t.Errorf("events = %+v, want an EventPickupTargetGone for seat 0", events)
	}
}

// TestValidateDoesNotDegradeWarehousePickup guards the bug this issue (#70)
// fixed: pickupAvailable (cargo.go) must recognize GDD §9.2's first Pickup
// source — "a Warehouse matching a held contract" — not only dropped ground
// cargo. Before the fix, checkActionDegradation's narrower check would have
// wrongly degraded every legitimate Warehouse Pickup to Nothing here, since
// a Warehouse's own supply is never represented as a Graph.Cargo entry.
func TestValidateDoesNotDegradeWarehousePickup(t *testing.T) {
	s := resolveTestState() // seat 0 at node 0 (Warehouse), no Graph.Cargo entries
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 2}}
	entry := s.Snapshot()
	seats := bySeat(s)
	orders := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionPickup}},
	}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{}, nil)

	if got := validated[0].Action.Kind; got != game.ActionPickup {
		t.Errorf("validated[0].Action.Kind = %v, want Pickup (not degraded)", got)
	}
	if hasEvent(events, game.EventPickupTargetGone, 0) {
		t.Errorf("events = %+v, want no EventPickupTargetGone for a genuine Warehouse Pickup", events)
	}
}

// TestValidatePassesThroughLegalOrder checks the un-degraded, un-rejected
// case: a fully legal order with no live-state conflict comes back
// unchanged and produces no Step 0 event.
func TestValidatePassesThroughLegalOrder(t *testing.T) {
	s := resolveTestState()
	entry := s.Snapshot()
	seats := bySeat(s)
	o := game.Order{Route: []game.NodeID{1}, Action: game.ActionOrder{Kind: game.ActionNothing}}
	orders := map[game.SeatID]game.Order{0: o}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{}, nil)

	if got := validated[0]; !reflect.DeepEqual(got, o) {
		t.Errorf("validated[0] = %+v, want the submitted order unchanged %+v", got, o)
	}
	for _, e := range events {
		if e.Seat == 0 {
			t.Errorf("events = %+v, want none for seat 0's fully legal order", events)
		}
	}
}

// TestValidateCurfewDegradesRouteToReducedAllowance is issue #72's own
// acceptance criterion: "Retroactive Curfew: a route legal at submission
// that exceeds the reduced allowance degrades, per GDD §15.0, rather than
// being rejected." Seat 0 is Infamy 0 (Nobody tier, 4 steps); the 4-step
// route below is legal against that base allowance, but Curfew's live -1
// (GDD §14.2) reduces it to 3 — "legal at submission, the world moved
// under it" (truncateForCurfew, validate.go), not an illegal payload.
func TestValidateCurfewDegradesRouteToReducedAllowance(t *testing.T) {
	s := resolveTestState()
	entry := s.Snapshot()
	seats := bySeat(s)
	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1, 0, 1, 2}, Action: game.ActionOrder{Kind: game.ActionNothing}},
	}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{curfewActive: true}, nil)

	got := validated[0]
	if !slices.Equal(got.Route, []game.NodeID{1, 0, 1}) {
		t.Errorf("validated[0].Route = %v, want [1 0 1] (truncated to the Curfew-reduced allowance of 3)", got.Route)
	}
	if got.Action.Kind != game.ActionNothing {
		t.Errorf("validated[0].Action.Kind = %v, want Nothing", got.Action.Kind)
	}
	if !hasEvent(events, game.EventCurfewTruncated, 0) {
		t.Errorf("events = %+v, want an EventCurfewTruncated for seat 0", events)
	}
	if hasEvent(events, game.EventOrderRejected, 0) {
		t.Errorf("events = %+v, want no EventOrderRejected — this degrades, it does not reject", events)
	}
}

// TestValidateCurfewStillRejectsGenuinelyOverAllowanceRoute confirms Curfew's
// degrade path does not swallow an ordinary illegal payload: a route that
// exceeds even the no-Curfew allowance was never legal to begin with (GDD
// §15.0's ordinary "route longer than your step allowance" reject row), so
// it still rejects rather than truncating to the Curfew figure.
func TestValidateCurfewStillRejectsGenuinelyOverAllowanceRoute(t *testing.T) {
	s := resolveTestState()
	entry := s.Snapshot()
	seats := bySeat(s)
	orders := map[game.SeatID]game.Order{
		0: {Route: []game.NodeID{1, 0, 1, 0, 1}, Action: game.ActionOrder{Kind: game.ActionNothing}},
	}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{curfewActive: true}, nil)

	want := game.Order{Action: game.ActionOrder{Kind: game.ActionNothing}, Stance: game.StanceOrder{Stance: game.StanceEvasive}}
	if got := validated[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("validated[0] = %+v, want the absence default %+v", got, want)
	}
	if !hasEvent(events, game.EventOrderRejected, 0) {
		t.Errorf("events = %+v, want an EventOrderRejected for seat 0", events)
	}
	if hasEvent(events, game.EventCurfewTruncated, 0) {
		t.Errorf("events = %+v, want no EventCurfewTruncated — this route was never legal, Curfew or not", events)
	}
}

// TestValidateDragnetDegradesDeliverAtSealedBorder covers Dragnet's own
// degradation case (checkActionDegradation's third branch, GDD §14.2):
// "every delivery must route to the ones that remain" — a Deliver
// otherwise legal at a Border degrades to Nothing when that Border is
// this round's sealed target.
func TestValidateDragnetDegradesDeliverAtSealedBorder(t *testing.T) {
	s := resolveTestState()
	s.Players[0].Position = 2 // the Border
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false}
	entry := s.Snapshot()
	seats := bySeat(s)
	orders := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionDeliver}},
	}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{}, []game.NodeID{2})

	if got := validated[0].Action.Kind; got != game.ActionNothing {
		t.Errorf("validated[0].Action.Kind = %v, want Nothing", got)
	}
	if !hasEvent(events, game.EventDeliveryBlocked, 0) {
		t.Errorf("events = %+v, want an EventDeliveryBlocked for seat 0", events)
	}
}

// TestValidateDeliverAtUnsealedBorderDoesNotDegrade checks the negative
// case: a Border not in sealedBorders is unaffected.
func TestValidateDeliverAtUnsealedBorderDoesNotDegrade(t *testing.T) {
	s := resolveTestState()
	s.Players[0].Position = 2
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false}
	entry := s.Snapshot()
	seats := bySeat(s)
	orders := map[game.SeatID]game.Order{
		0: {Action: game.ActionOrder{Kind: game.ActionDeliver}},
	}

	validated, events := validate(s, entry, seats, orders, legalTestConfig(), globalEventContext{}, []game.NodeID{3})

	if got := validated[0].Action.Kind; got != game.ActionDeliver {
		t.Errorf("validated[0].Action.Kind = %v, want Deliver (node 2 is not sealed)", got)
	}
	if hasEvent(events, game.EventDeliveryBlocked, 0) {
		t.Errorf("events = %+v, want no EventDeliveryBlocked", events)
	}
}

// TestLegalViewWiresEntrySnapshotIntoStepModifiers guards the wiring gap
// this issue closes: legalView must copy Infamy, Flagged and
// EvasiveStepPenalty from the entry snapshot into the PlayerView Steps
// actually reads — before this fix, every legality check silently computed
// against Infamy 0 and no modifiers regardless of the seat's true state.
func TestLegalViewWiresEntrySnapshotIntoStepModifiers(t *testing.T) {
	s := resolveTestState()
	s.Players[0].Infamy = 9 // Legend tier: 2 steps per round.
	entry := s.Snapshot()

	v := legalView(s, entry, 0, false)

	if v.You.Infamy != 9 {
		t.Errorf("You.Infamy = %d, want 9 (from the entry snapshot)", v.You.Infamy)
	}
	if got := Steps(v, legalTestConfig()); got != 2 {
		t.Errorf("Steps() = %d, want 2 (Legend tier)", got)
	}
}

// TestLegalViewScaffoldingBonusIsSectorScoped is GDD §14.2's Scaffolding,
// consumed the round after the card fires: "+1 step" only for a player
// whose entry position is inside the flagged sector.
func TestLegalViewScaffoldingBonusIsSectorScoped(t *testing.T) {
	s := resolveTestState() // seat 0 at node 0
	s.Graph.Nodes[0].Sector = game.SectorOldDocks
	sector := game.SectorOldDocks
	s.NextRound.Scaffolding = &sector
	entry := s.Snapshot()

	v := legalView(s, entry, 0, false)
	if !v.You.StepModifiers.Scaffolding {
		t.Error("StepModifiers.Scaffolding = false, want true for a seat inside the flagged sector")
	}

	other := game.SectorNorthVale
	s.NextRound.Scaffolding = &other
	v = legalView(s, entry, 0, false)
	if v.You.StepModifiers.Scaffolding {
		t.Error("StepModifiers.Scaffolding = true, want false for a seat outside the flagged sector")
	}
}

// TestLegalViewRetainerBonusRequiresNoCargo is GDD §14.2's Retainer,
// consumed the round after the card fires: "+2 steps" only for a player
// carrying no cargo.
func TestLegalViewRetainerBonusRequiresNoCargo(t *testing.T) {
	s := resolveTestState()
	s.NextRound.Retainer = true
	entry := s.Snapshot()

	v := legalView(s, entry, 0, false)
	if !v.You.StepModifiers.Retainer {
		t.Error("StepModifiers.Retainer = false, want true for a seat carrying no cargo")
	}

	s.Players[0].Cargo = &game.CarriedCargo{Bound: false}
	v = legalView(s, entry, 0, false)
	if v.You.StepModifiers.Retainer {
		t.Error("StepModifiers.Retainer = true, want false for a seat carrying cargo")
	}
}

// TestLegalViewNodesSharesProjectNodes guards issue #161: legalView's
// v.Nodes must come from the same fog-filtering projectNodes (fog.go) uses,
// not a second, independently-maintained walk of p.Fog — so a field like
// Post can never populate under one and stay nil under the other just
// because a future legality rule started reading it.
func TestLegalViewNodesSharesProjectNodes(t *testing.T) {
	s := resolveTestState()
	s.Graph.Nodes[1].Post = &Post{Owner: 0, RoundsRemaining: 3}
	entry := s.Snapshot()

	v := legalView(s, entry, 0, false)
	want := projectNodes(s, s.Players[0])

	if !reflect.DeepEqual(v.Nodes, want) {
		t.Errorf("legalView v.Nodes = %+v, want exactly projectNodes(s, p)'s output %+v", v.Nodes, want)
	}
	if v.Nodes[1].Post == nil {
		t.Error("Nodes[1].Post = nil, want populated — legalView must not zero out fields Legal doesn't itself read")
	}
}

func hasEvent(events []game.Event, kind game.EventKind, seat game.SeatID) bool {
	for _, e := range events {
		if e.Kind == kind && e.Seat == seat {
			return true
		}
	}
	return false
}
