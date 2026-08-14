package rules

import (
	"slices"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// bordersFixture returns a graph with three Border nodes (IDs 1, 3, 5),
// interspersed with non-Border nodes at 0, 2, 4 so a test can catch a
// borderNodeIDs bug that assumes Borders are contiguous. players controls
// MatchState.Players' length, the only thing activeBordersForRound and
// friends key rotation on.
func bordersFixture(players int) MatchState {
	s := MatchState{
		Graph: Graph{
			Nodes: []Node{
				{ID: 0, Type: game.NodeWarehouse},
				{ID: 1, Type: game.NodeBorder},
				{ID: 2, Type: game.NodeAlley},
				{ID: 3, Type: game.NodeBorder},
				{ID: 4, Type: game.NodeBlackMarket},
				{ID: 5, Type: game.NodeBorder},
			},
		},
		Players: make([]Player, players),
	}
	for i := range s.Players {
		s.Players[i].Seat = game.SeatID(i)
	}
	return s
}

// TestActiveBordersForRoundTwoPlayersAlternates is D03's mechanism,
// verbatim: sort Borders by NodeID (1, 3, 5), alternate A, B, A (so
// A = {1, 5}, B = {3}), active = A on odd rounds, B on even — round 1
// opens on A.
func TestActiveBordersForRoundTwoPlayersAlternates(t *testing.T) {
	s := bordersFixture(2)

	tests := []struct {
		round        game.RoundNumber
		wantActive   []game.NodeID
		wantInactive []game.NodeID
	}{
		{round: 1, wantActive: []game.NodeID{1, 5}, wantInactive: []game.NodeID{3}},
		{round: 2, wantActive: []game.NodeID{3}, wantInactive: []game.NodeID{1, 5}},
		{round: 3, wantActive: []game.NodeID{1, 5}, wantInactive: []game.NodeID{3}},
		{round: 4, wantActive: []game.NodeID{3}, wantInactive: []game.NodeID{1, 5}},
	}
	for _, tt := range tests {
		if got := activeBordersForRound(s.Graph, tt.round, 2); !slices.Equal(got, tt.wantActive) {
			t.Errorf("round %d: activeBordersForRound() = %v, want %v", tt.round, got, tt.wantActive)
		}
		if got := inactiveBordersForRound(s.Graph, tt.round, 2); !slices.Equal(got, tt.wantInactive) {
			t.Errorf("round %d: inactiveBordersForRound() = %v, want %v", tt.round, got, tt.wantInactive)
		}
	}
}

// TestActiveBordersForRoundInertAboveTwoPlayers is issue #76's own
// acceptance criterion: at 3+ players every Border accepts delivery,
// regardless of round — rotation must never leak into a 3+ player table.
func TestActiveBordersForRoundInertAboveTwoPlayers(t *testing.T) {
	s := bordersFixture(3)

	for players := 3; players <= 5; players++ {
		for round := game.RoundNumber(1); round <= 4; round++ {
			if got := activeBordersForRound(s.Graph, round, players); got != nil {
				t.Errorf("players=%d round=%d: activeBordersForRound() = %v, want nil", players, round, got)
			}
			if got := inactiveBordersForRound(s.Graph, round, players); got != nil {
				t.Errorf("players=%d round=%d: inactiveBordersForRound() = %v, want nil", players, round, got)
			}
		}
	}
}

// TestBlockedBordersThisRoundCombinesDragnetAndRotation checks the
// ordinary case (no safety valve involved): rotation's inactive set and
// Dragnet's seal both contribute, and the result is their union.
func TestBlockedBordersThisRoundCombinesDragnetAndRotation(t *testing.T) {
	s := bordersFixture(2)

	// Round 1: active = {1, 5}, inactive = {3}. Dragnet additionally seals
	// node 5 (an active Border) — the union should be {3, 5}, leaving only
	// node 1 deliverable.
	got := blockedBordersThisRound(s.Graph, 1, 2, []game.NodeID{5})
	want := []game.NodeID{3, 5}
	if !slices.Equal(got, want) {
		t.Fatalf("blockedBordersThisRound() = %v, want %v", got, want)
	}
}

// TestBlockedBordersThisRoundSafetyValve is D26, verbatim: at 2 players,
// Dragnet's seal can coincide with rotation's inactive set to close every
// Border. Round 2's active set is {3} alone (see the alternation test
// above); Dragnet sealing exactly {3} would leave zero Borders deliverable
// without the fallback. The lowest-NodeID Border (1) must reopen.
func TestBlockedBordersThisRoundSafetyValve(t *testing.T) {
	s := bordersFixture(2)

	got := blockedBordersThisRound(s.Graph, 2, 2, []game.NodeID{3})
	want := []game.NodeID{3, 5} // node 1 (lowest NodeID) reopened
	if !slices.Equal(got, want) {
		t.Fatalf("blockedBordersThisRound() = %v, want %v (node 1 should reopen)", got, want)
	}
	if slices.Contains(got, game.NodeID(1)) {
		t.Fatalf("blockedBordersThisRound() = %v, want node 1 (lowest NodeID) reopened, not blocked", got)
	}
}

// TestBlockedBordersThisRoundSafetyValveNeverFiresAboveTwoPlayers is D26's
// own claim that the fallback is structurally inert at 3+ players: with
// rotation contributing nothing there, Dragnet's own 2-Border seal out of
// this fixture's 3 total Borders never covers every Border, so the result
// is exactly Dragnet's seal, unmodified.
func TestBlockedBordersThisRoundSafetyValveNeverFiresAboveTwoPlayers(t *testing.T) {
	s := bordersFixture(3)

	got := blockedBordersThisRound(s.Graph, 1, 3, []game.NodeID{1, 5})
	want := []game.NodeID{1, 5}
	if !slices.Equal(got, want) {
		t.Fatalf("blockedBordersThisRound() = %v, want %v (Dragnet's seal alone, no reopening)", got, want)
	}
}

// TestBlockedBordersThisRoundNoDragnet checks a Dragnet-quiet round: only
// rotation contributes, and the safety valve cannot fire on its own since
// rotation always leaves at least one Border active by construction.
func TestBlockedBordersThisRoundNoDragnet(t *testing.T) {
	s := bordersFixture(2)

	got := blockedBordersThisRound(s.Graph, 1, 2, nil)
	want := []game.NodeID{3}
	if !slices.Equal(got, want) {
		t.Fatalf("blockedBordersThisRound() = %v, want %v", got, want)
	}
}
