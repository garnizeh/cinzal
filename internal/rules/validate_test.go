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

	validated, events := validate(s, entry, seats, nil, legalTestConfig())

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

	validated, events := validate(s, entry, seats, orders, legalTestConfig())

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

	validated, events := validate(s, entry, seats, orders, legalTestConfig())

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

	validated, events := validate(s, entry, seats, orders, legalTestConfig())

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

	validated, events := validate(s, entry, seats, orders, legalTestConfig())

	got := validated[0]
	if got.Action.Kind != game.ActionNothing {
		t.Errorf("validated[0].Action.Kind = %v, want Nothing", got.Action.Kind)
	}

	if !hasEvent(events, game.EventPickupTargetGone, 0) {
		t.Errorf("events = %+v, want an EventPickupTargetGone for seat 0", events)
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

	validated, events := validate(s, entry, seats, orders, legalTestConfig())

	if got := validated[0]; !reflect.DeepEqual(got, o) {
		t.Errorf("validated[0] = %+v, want the submitted order unchanged %+v", got, o)
	}
	for _, e := range events {
		if e.Seat == 0 {
			t.Errorf("events = %+v, want none for seat 0's fully legal order", events)
		}
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
