package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestBuildRoundAnchorsMapsEachGlobalEventKind covers every game.EventKind
// buildRoundAnchors recognises — RFC §9.1 rows 2, 3, 4, 11, 13, 14, plus
// EventInformantRing (row 15) and EventSpilledLoadCrate (row 16), decided
// by D26 (#142) — asserting the resulting Anchor's Kind, Node, Actor (nil
// for the three node-only kinds) and Tier (EventDelivered only).
func TestBuildRoundAnchorsMapsEachGlobalEventKind(t *testing.T) {
	tests := []struct {
		name      string
		event     game.Event
		wantActor *game.SeatID
		wantTier  int
	}{
		{"EventDelivered", game.Event{Kind: game.EventDelivered, Node: 3, Seat: 1, Tier: 2}, new(game.SeatID(1)), 2},
		{"EventPostStaked", game.Event{Kind: game.EventPostStaked, Node: 2, Seat: 0}, new(game.SeatID(0)), 0},
		{"EventLeaseExpired", game.Event{Kind: game.EventLeaseExpired, Node: 2, Seat: 0}, new(game.SeatID(0)), 0},
		{"EventInformants", game.Event{Kind: game.EventInformants, Node: 1, Seat: 1}, new(game.SeatID(1)), 0},
		{"EventInformantRing", game.Event{Kind: game.EventInformantRing, Node: 4, Seat: 0}, new(game.SeatID(0)), 0},
		{"EventDeadRunnerCrate", game.Event{Kind: game.EventDeadRunnerCrate, Node: 3}, nil, 0},
		{"EventFenceWindfallAnnounced", game.Event{Kind: game.EventFenceWindfallAnnounced, Node: 2}, nil, 0},
		{"EventSpilledLoadCrate", game.Event{Kind: game.EventSpilledLoadCrate, Node: 1}, nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			anchors := buildRoundAnchors([]game.Event{tt.event}, 7)
			if len(anchors) != 1 {
				t.Fatalf("buildRoundAnchors = %+v, want exactly 1 anchor", anchors)
			}
			a := anchors[0]
			if a.Kind != tt.event.Kind || a.Round != 7 || a.Node != tt.event.Node {
				t.Errorf("anchor = %+v, want Kind %v, Round 7, Node %v", a, tt.event.Kind, tt.event.Node)
			}
			if (a.Actor == nil) != (tt.wantActor == nil) {
				t.Fatalf("anchor.Actor = %v, want nil-ness %v", a.Actor, tt.wantActor == nil)
			}
			if a.Actor != nil && *a.Actor != *tt.wantActor {
				t.Errorf("anchor.Actor = %v, want %v", *a.Actor, *tt.wantActor)
			}
			if a.Tier != tt.wantTier {
				t.Errorf("anchor.Tier = %d, want %d", a.Tier, tt.wantTier)
			}
		})
	}
}

// TestBuildRoundAnchorsIgnoresUnlistedKinds asserts every EventKind not in
// RFC §9.1's global rows — including the four sight-gated kinds (rows 1, 7,
// 8, 12), which reach Project through each seat's own Archive.Trail
// instead — produces no anchor.
func TestBuildRoundAnchorsIgnoresUnlistedKinds(t *testing.T) {
	events := []game.Event{
		{Kind: game.EventCargoTaken, Node: 0, Seat: 0},
		{Kind: game.EventFreshTracks, Node: 0},
		{Kind: game.EventConfrontation, Node: 0, Seat: 0, Target: 1},
		{Kind: game.EventItemPurchased, Node: 0, Seat: 0},
		{Kind: game.EventLoitering, Node: 0},
		{Kind: game.EventLooseCrateHeld, Node: 0},
		{Kind: game.EventTierFeared, Node: 0, Seat: 0},
		{Kind: game.EventTierLegend, Node: 0, Seat: 0},
		{Kind: game.EventOrderRejected, Seat: 0},
	}

	anchors := buildRoundAnchors(events, 7)
	if len(anchors) != 0 {
		t.Errorf("buildRoundAnchors = %+v, want none of these kinds to produce an anchor", anchors)
	}
}
