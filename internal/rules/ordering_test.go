package rules

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestOnlyOrderingFileUsesSortSlice is the issue's own acceptance
// criterion, verbatim: exactly two orderings exist in this package —
// bySeat and byFairness, both in ordering.go — and any other file in the
// package that reaches for sort.Slice is inventing a third one. Node-ID
// orderings (selection.go's orderConfrontationsByNode) are deliberately
// unaffected: they use the "slices" package, a different symbol, because
// RFC §6.5's "two orderings" are specifically about SeatID batching and
// fairness, not the pre-existing NodeID-ascending convention RFC §6.4
// already establishes throughout.
//
// This is a fail-closed gate (CLAUDE.md): if the directory read finds no
// package files at all, that is a broken check, not a passing one.
func TestOnlyOrderingFileUsesSortSlice(t *testing.T) {
	const allowedFile = "ordering.go"

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(.): %v", err)
	}

	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++

		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		if strings.Contains(string(data), "sort.Slice") && name != allowedFile {
			t.Errorf("%s calls sort.Slice — the only two orderings in this package (byFairness, bySeat) live in %s; a third ordering does not belong anywhere else", name, allowedFile)
		}
	}

	if scanned == 0 {
		t.Fatal("scanned 0 non-test .go files in internal/rules — the check ran over nothing, which is not the same as passing")
	}
}

func matchStateWithPlayers(players ...Player) MatchState {
	return MatchState{Players: players}
}

// TestBySeatIsAscendingSeatIndex pins bySeat's whole contract: the seats it
// returns, in order.
func TestBySeatIsAscendingSeatIndex(t *testing.T) {
	s := matchStateWithPlayers(
		Player{Seat: 0}, Player{Seat: 1}, Player{Seat: 2}, Player{Seat: 3},
	)
	got := bySeat(s)
	want := []game.SeatID{0, 1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bySeat() = %v, want %v", got, want)
	}
}

// TestBySeatNeverMovesForMidPhaseBalanceChange is the acceptance
// criterion's Pressure-batch row: two seats with identical Infamy and
// balance draw in seat order, and swapping their balances mid-phase must
// not reorder them. bySeat reads only Seat, never Infamy, Balance, or RP,
// so this holds by construction — the test exists to pin that as a
// contract, not an accident of the current field list.
func TestBySeatNeverMovesForMidPhaseBalanceChange(t *testing.T) {
	s := matchStateWithPlayers(
		Player{Seat: 0, Infamy: 9, Balance: 20}, // both Legends (GDD §14.4)
		Player{Seat: 1, Infamy: 9, Balance: 20},
	)

	before := bySeat(s)

	// A balance change earlier in the same phase — exactly the hazard RFC
	// §6.5 names as the reason seat index, not the fairness key, batches
	// RNG draws.
	s.Players[0].Balance = 999

	after := bySeat(s)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("bySeat() changed after a mid-phase balance mutation: before %v, after %v", before, after)
	}
	if !reflect.DeepEqual(after, []game.SeatID{0, 1}) {
		t.Fatalf("bySeat() = %v, want seat order [0 1]", after)
	}
}

// TestByFairnessOrdersByInfamyThenBalanceThenRP asserts the three levels
// that need no RNG draw, using seats each tied at exactly one level so a
// wrong comparator produces a visibly wrong order rather than an
// accidentally-correct one.
func TestByFairnessOrdersByInfamyThenBalanceThenRP(t *testing.T) {
	s := matchStateWithPlayers(
		Player{Seat: 0, Infamy: 5, Balance: 10, RP: 1},
		Player{Seat: 1, Infamy: 2, Balance: 999, RP: 999}, // lowest Infamy wins outright
		Player{Seat: 2, Infamy: 5, Balance: 3, RP: 999},   // ties seat 0 on Infamy, wins on Balance
		Player{Seat: 3, Infamy: 5, Balance: 10, RP: 0},    // ties seat 0 on Infamy+Balance, wins on RP
	)

	rng := NewRNG(testSeed(1), 1)
	got := byFairness(s, rng)
	want := []game.SeatID{1, 2, 3, 0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("byFairness() = %v, want %v", got, want)
	}
	if c := rng.Consumed(PurposeConfrontTiebreak); c != 0 {
		t.Errorf("Consumed(confront.tiebreak) = %d, want 0 — no seat was tied through all three levels", c)
	}
}

// TestByFairnessCoinFiresOnlyAtFourthLevel is the acceptance criterion
// verbatim: the seeded coin consumes exactly one index, at the fourth
// level only, and zero at every level above it — a truncation case for
// #77's replay invariant.
func TestByFairnessCoinFiresOnlyAtFourthLevel(t *testing.T) {
	cases := []struct {
		name        string
		players     []Player
		wantDraws   int
		description string
	}{
		{
			name: "tied through nothing",
			players: []Player{
				{Seat: 0, Infamy: 1, Balance: 1, RP: 1},
				{Seat: 1, Infamy: 2, Balance: 1, RP: 1},
			},
			wantDraws: 0,
		},
		{
			name: "tied through Infamy only",
			players: []Player{
				{Seat: 0, Infamy: 1, Balance: 1, RP: 1},
				{Seat: 1, Infamy: 1, Balance: 2, RP: 1},
			},
			wantDraws: 0,
		},
		{
			name: "tied through Infamy and Balance",
			players: []Player{
				{Seat: 0, Infamy: 1, Balance: 1, RP: 1},
				{Seat: 1, Infamy: 1, Balance: 1, RP: 2},
			},
			wantDraws: 0,
		},
		{
			name: "tied through Infamy, Balance, and RP",
			players: []Player{
				{Seat: 0, Infamy: 1, Balance: 1, RP: 1},
				{Seat: 1, Infamy: 1, Balance: 1, RP: 1},
			},
			wantDraws: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := matchStateWithPlayers(c.players...)
			rng := NewRNG(testSeed(2), 1)
			byFairness(s, rng)
			if got := rng.Consumed(PurposeConfrontTiebreak); got != c.wantDraws {
				t.Errorf("Consumed(confront.tiebreak) = %d, want %d", got, c.wantDraws)
			}
		})
	}
}

// TestByFairnessCoinOrdersATiedGroupOfAnySize asserts the single draw at
// the fourth level produces a full, deterministic order for a tied group
// larger than two — the common case at match start, when every seat begins
// at identical Infamy, Balance, and RP.
func TestByFairnessCoinOrdersATiedGroupOfAnySize(t *testing.T) {
	s := matchStateWithPlayers(
		Player{Seat: 0, Infamy: 0, Balance: 10, RP: 0},
		Player{Seat: 1, Infamy: 0, Balance: 10, RP: 0},
		Player{Seat: 2, Infamy: 0, Balance: 10, RP: 0},
		Player{Seat: 3, Infamy: 0, Balance: 10, RP: 0},
	)

	rng := NewRNG(testSeed(3), 1)
	got := byFairness(s, rng)

	if c := rng.Consumed(PurposeConfrontTiebreak); c != 1 {
		t.Fatalf("Consumed(confront.tiebreak) = %d, want 1 — one draw for the whole tied group, not one per pair", c)
	}
	if len(got) != 4 {
		t.Fatalf("byFairness() returned %d seats, want 4", len(got))
	}

	// Every seat must still appear exactly once — a rotation, not a lossy
	// selection.
	seen := map[game.SeatID]bool{}
	for _, seat := range got {
		if seen[seat] {
			t.Fatalf("byFairness() = %v contains %v twice", got, seat)
		}
		seen[seat] = true
	}

	// Replaying the identical seed and round must reproduce the identical
	// order (RFC §16.1).
	replay := byFairness(s, NewRNG(testSeed(3), 1))
	if !reflect.DeepEqual(got, replay) {
		t.Fatalf("byFairness() is not stable on replay: %v != %v", got, replay)
	}
}
