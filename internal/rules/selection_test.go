package rules

import (
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

func nodeWithLease(id game.NodeID, roundsRemaining int) Node {
	return Node{ID: id, Post: &Post{RoundsRemaining: roundsRemaining}}
}

// TestSelectLeaseByFewestRoundsBreaksTiesByLowestNodeID covers row 1 and
// row 2 of the table with a constructed tie: two leases with identical
// RoundsRemaining, the tie broken by the lower NodeID, not input order.
func TestSelectLeaseByFewestRoundsBreaksTiesByLowestNodeID(t *testing.T) {
	posts := []game.Post{
		{Node: 5, RoundsRemaining: 2},
		{Node: 1, RoundsRemaining: 2}, // tied on RoundsRemaining, lower NodeID
		{Node: 9, RoundsRemaining: 3}, // more rounds remaining, never wins
	}

	got, ok := SelectLeaseByFewestRounds(posts)
	if !ok {
		t.Fatal("SelectLeaseByFewestRounds() returned ok=false for a non-empty slice")
	}
	if got != 1 {
		t.Fatalf("SelectLeaseByFewestRounds() = %v, want NodeID 1", got)
	}
}

// TestSelectLeaseByFewestRoundsIsStableAcross100RunsAndRebuiltState is the
// acceptance criterion verbatim: the same lease surrenders across 100 runs
// and on a rebuilt state. SelectLeaseByFewestRounds is a pure function of
// its input slice, so "100 runs" and "a rebuilt state built independently,
// in a different input order" both have to land on the same answer.
func TestSelectLeaseByFewestRoundsIsStableAcross100RunsAndRebuiltState(t *testing.T) {
	posts := []game.Post{
		{Node: 7, RoundsRemaining: 1},
		{Node: 2, RoundsRemaining: 1}, // the tie: lower NodeID must always win
		{Node: 4, RoundsRemaining: 5},
	}

	for i := range 100 {
		got, ok := SelectLeaseByFewestRounds(posts)
		if !ok || got != 2 {
			t.Fatalf("run %d: SelectLeaseByFewestRounds() = (%v, %v), want (2, true)", i, got, ok)
		}
	}

	// A rebuilt state: the same facts, assembled independently and in a
	// different order, must not change the answer.
	rebuilt := []game.Post{
		{Node: 4, RoundsRemaining: 5},
		{Node: 2, RoundsRemaining: 1},
		{Node: 7, RoundsRemaining: 1},
	}
	got, ok := SelectLeaseByFewestRounds(rebuilt)
	if !ok || got != 2 {
		t.Fatalf("rebuilt state: SelectLeaseByFewestRounds() = (%v, %v), want (2, true)", got, ok)
	}
}

// TestSelectLeaseByFewestRoundsEmpty covers the no-posts case: nothing to
// surrender or renew.
func TestSelectLeaseByFewestRoundsEmpty(t *testing.T) {
	if _, ok := SelectLeaseByFewestRounds(nil); ok {
		t.Fatal("SelectLeaseByFewestRounds(nil) returned ok=true")
	}
}

// TestDebtSurrendersLeaseWithFewestRoundsRemaining exercises row 1 through
// MatchState/leasePosts, the shape Debt actually reads from.
func TestDebtSurrendersLeaseWithFewestRoundsRemaining(t *testing.T) {
	s := MatchState{
		Graph: Graph{Nodes: []Node{
			nodeWithLease(0, 4),
			nodeWithLease(1, 2), // fewest rounds remaining
			nodeWithLease(2, 4),
		}},
		Players: []Player{
			{Seat: 0, Posts: []game.NodeID{0, 1, 2}},
		},
	}

	got, ok := SelectLeaseByFewestRounds(leasePosts(s, 0))
	if !ok || got != 1 {
		t.Fatalf("leasePosts/SelectLeaseByFewestRounds = (%v, %v), want (1, true)", got, ok)
	}
}

// TestAutopilotRenewsLeaseWithFewestRoundsRemaining exercises row 2: the
// identical key, applied to the shape Autopilot actually sees —
// []game.Post, straight off PlayerView.SelfState.Posts, with no MatchState
// in reach at all.
func TestAutopilotRenewsLeaseWithFewestRoundsRemaining(t *testing.T) {
	posts := []game.Post{
		{Node: 3, RoundsRemaining: 6},
		{Node: 8, RoundsRemaining: 1}, // fewest rounds remaining
	}

	got, ok := SelectLeaseByFewestRounds(posts)
	if !ok || got != 8 {
		t.Fatalf("SelectLeaseByFewestRounds() = (%v, %v), want (8, true)", got, ok)
	}
}

// TestSelectNewBossTargetBreaksTieByFairnessKey covers row 3: lowest RP,
// tie broken by the fairness key (here, by Infamy — the first level of the
// chain — since the two candidates differ there).
func TestSelectNewBossTargetBreaksTieByFairnessKey(t *testing.T) {
	s := matchStateWithPlayers(
		Player{Seat: 0, Infamy: 5, RP: 1},
		Player{Seat: 1, Infamy: 2, RP: 1}, // tied lowest RP, wins on lower Infamy
		Player{Seat: 2, Infamy: 5, RP: 9},
	)

	got := selectNewBossTarget(s, NewRNG(testSeed(4), 1))
	if got != 1 {
		t.Fatalf("selectNewBossTarget() = %v, want seat 1", got)
	}
}

// TestSelectBountyTargetBreaksTieByFairnessKey covers row 8: highest RP,
// tied by the identical fairness chain New Boss uses (D14 §5) — mirrored
// so a copy-paste that forgot to flip the comparison direction still fails.
func TestSelectBountyTargetBreaksTieByFairnessKey(t *testing.T) {
	s := matchStateWithPlayers(
		Player{Seat: 0, Infamy: 5, RP: 9},
		Player{Seat: 1, Infamy: 2, RP: 9}, // tied highest RP, wins on lower Infamy
		Player{Seat: 2, Infamy: 5, RP: 1},
	)

	got := selectBountyTarget(s, NewRNG(testSeed(5), 1))
	if got != 1 {
		t.Fatalf("selectBountyTarget() = %v, want seat 1", got)
	}
}

// TestSelectRaidTargetsHitsEveryTiedPlayer covers row 5: GDD §14.2's "Ties:
// all tied players" — no tie-break, every seat at the highest Infamy comes
// back, not just one.
func TestSelectRaidTargetsHitsEveryTiedPlayer(t *testing.T) {
	s := matchStateWithPlayers(
		Player{Seat: 0, Infamy: 7},
		Player{Seat: 1, Infamy: 9}, // tied highest
		Player{Seat: 2, Infamy: 9}, // tied highest
		Player{Seat: 3, Infamy: 3},
	)

	got := selectRaidTargets(s)
	want := []game.SeatID{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selectRaidTargets() = %v, want %v", got, want)
	}
}

// TestSelectDefaultMeleeCargoTargetPicksLowestSeat covers row 4: the
// winner's declared choice always wins in play, but the bot/default
// fallback is the lowest seat among losers actually carrying cargo.
func TestSelectDefaultMeleeCargoTargetPicksLowestSeat(t *testing.T) {
	s := matchStateWithPlayers(
		Player{Seat: 0, Cargo: nil},
		Player{Seat: 1, Cargo: &game.CarriedCargo{}}, // lowest seat with cargo
		Player{Seat: 2, Cargo: &game.CarriedCargo{}},
	)

	got, ok := selectDefaultMeleeCargoTarget(s, []game.SeatID{2, 1, 0})
	if !ok || got != 1 {
		t.Fatalf("selectDefaultMeleeCargoTarget() = (%v, %v), want (1, true)", got, ok)
	}
}

// TestSelectDefaultMeleeCargoTargetNoCargo asserts the false case: every
// loser is empty-handed.
func TestSelectDefaultMeleeCargoTargetNoCargo(t *testing.T) {
	s := matchStateWithPlayers(Player{Seat: 0}, Player{Seat: 1})
	if _, ok := selectDefaultMeleeCargoTarget(s, []game.SeatID{0, 1}); ok {
		t.Fatal("selectDefaultMeleeCargoTarget() returned ok=true when no loser carried cargo")
	}
}

// TestOrderLosersForPushbackIsSeatOrder covers row 6: a 3+-way melee's
// losers, drawing pushback.hop, ordered by seat index — however the
// collision detector happened to append them.
func TestOrderLosersForPushbackIsSeatOrder(t *testing.T) {
	s := matchStateWithPlayers(
		Player{Seat: 0}, Player{Seat: 1}, Player{Seat: 2}, Player{Seat: 3},
	)

	got := orderLosersForPushback(s, []game.SeatID{3, 0, 2})
	want := []game.SeatID{0, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orderLosersForPushback() = %v, want %v", got, want)
	}
}

// TestOrderConfrontationsByNodeIsAscending covers row 7: several
// confrontations in the same movement step, resolved one node at a time,
// lowest NodeID first.
func TestOrderConfrontationsByNodeIsAscending(t *testing.T) {
	got := orderConfrontationsByNode([]game.NodeID{9, 1, 5})
	want := []game.NodeID{1, 5, 9}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orderConfrontationsByNode() = %v, want %v", got, want)
	}
}
