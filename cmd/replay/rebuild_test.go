package main

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/store"
)

// This file is issue #409's own pure-logic coverage for
// lastSeenRoundsFromOrders — RFC-001 §7.2's third derived-projection
// rebuild, corrected by D52. The real-Postgres path (rebuildMatch calling
// through to store.RebuildLastSeenRounds) is exercised in
// rebuild_integration_test.go; this file only proves the fold arithmetic
// itself, the same split fixture_test.go/replay_integration_test.go already
// draw for the rest of this package.

// humanOrder is one lastSeenRoundsFromOrders fixture row: a human-sourced
// order for (round, seat), the only source that ever advances a cursor.
func humanOrder(round game.RoundNumber, seat game.SeatID) store.Order {
	return store.Order{Round: round, Seat: seat, Source: string(store.SourceHuman)}
}

// defaultOrder is a round's own fallback order (GDD §18) for (round, seat) —
// never advances that seat's cursor, standing in for a round the seat did
// not actually submit for.
func defaultOrder(round game.RoundNumber, seat game.SeatID) store.Order {
	return store.Order{Round: round, Seat: seat, Source: string(store.SourceDefault)}
}

// TestLastSeenRoundsFromOrdersSteadyState is D52's own accepted no-backlog
// case: one human submission per round, no gaps. Its Reasoning section
// states the invariant this asserts directly — "cursor going into round R
// is always R − 2," i.e. after folding every human round 1..N, the cursor
// lands at N − 1, identical to the pre-D52 GREATEST expression's result for
// this case.
func TestLastSeenRoundsFromOrdersSteadyState(t *testing.T) {
	var rows []store.Order
	for round := game.RoundNumber(1); round <= 5; round++ {
		rows = append(rows, humanOrder(round, 0))
	}

	got := lastSeenRoundsFromOrders(1, rows)
	if want := game.RoundNumber(4); got[0] != want {
		t.Fatalf("last_seen_round after 5 steady human rounds = %d, want %d", got[0], want)
	}
}

// TestLastSeenRoundsFromOrdersClampsAcrossAGap is the acceptance
// criterion's own named case: a multi-round backlog must advance the
// cursor by at most one round per human submission (D52's LEAST clamp),
// never jump straight to the gap's far edge the way the pre-D52 GREATEST
// expression would have. Seat 0 submits humanly for round 1, then skips
// rounds 2-4 (default orders), then resumes at round 5 through 8.
func TestLastSeenRoundsFromOrdersClampsAcrossAGap(t *testing.T) {
	rows := []store.Order{
		humanOrder(1, 0),
		defaultOrder(2, 0),
		defaultOrder(3, 0),
		defaultOrder(4, 0),
		humanOrder(5, 0),
		humanOrder(6, 0),
		humanOrder(7, 0),
		humanOrder(8, 0),
	}

	got := lastSeenRoundsFromOrders(1, rows)

	// Hand-computed via D52's own fold, independent of the production code
	// under test: cursor=0 -> round1: min(0+1,0)=0 -> round5: min(0+1,4)=1
	// -> round6: min(1+1,5)=2 -> round7: min(2+1,6)=3 -> round8: min(3+1,7)=4.
	// A GREATEST(cursor, round-1)-style rebuild would instead land on 7
	// (round8-1), silently overshooting the three skipped rounds — the exact
	// regression D52 exists to prevent.
	if want := game.RoundNumber(4); got[0] != want {
		t.Fatalf("last_seen_round after a 3-round gap then 4 more human rounds = %d, want %d", got[0], want)
	}
}

// TestLastSeenRoundsFromOrdersZeroForSeatWithNoHumanOrders covers the seat
// that never submits a human order at all (an Autopilot seat for the whole
// match) — its cursor must stay 0, D16's own seat-creation default, and
// must still appear in the result explicitly (RebuildLastSeenRounds' own
// doc comment: a stale non-zero value has to be overwritten, not skipped).
func TestLastSeenRoundsFromOrdersZeroForSeatWithNoHumanOrders(t *testing.T) {
	rows := []store.Order{
		defaultOrder(1, 1),
		defaultOrder(2, 1),
		humanOrder(1, 0),
	}

	got := lastSeenRoundsFromOrders(2, rows)
	if _, ok := got[1]; !ok {
		t.Fatal("seat 1 (no human orders) is missing from the result, want an explicit 0 entry")
	}
	if got[1] != 0 {
		t.Fatalf("last_seen_round for a seat with no human orders = %d, want 0", got[1])
	}
}

// TestLastSeenRoundsFromOrdersEmptyLogIsAllZero is the empty-order-log case
// rebuildMatch's own comment describes (a lobby match, or one whose orders
// were all removed): every seat still gets an explicit 0, matching D16's
// seat-creation default.
func TestLastSeenRoundsFromOrdersEmptyLogIsAllZero(t *testing.T) {
	got := lastSeenRoundsFromOrders(3, nil)
	if len(got) != 3 {
		t.Fatalf("len(result) = %d, want 3 (one entry per seat)", len(got))
	}
	for seat := range game.SeatID(3) {
		if got[seat] != 0 {
			t.Fatalf("last_seen_round for seat %d with no orders at all = %d, want 0", seat, got[seat])
		}
	}
}

// TestLastSeenRoundsFromOrdersIsIndependentPerSeat proves two seats folding
// over the same rows arrive at different cursors when their own
// human-submission patterns differ — the map is keyed correctly by seat,
// not accidentally shared across a whole round's row group.
func TestLastSeenRoundsFromOrdersIsIndependentPerSeat(t *testing.T) {
	rows := []store.Order{
		humanOrder(1, 0), humanOrder(1, 1),
		humanOrder(2, 0), defaultOrder(2, 1),
		humanOrder(3, 0), defaultOrder(3, 1),
		humanOrder(4, 0), humanOrder(4, 1),
	}

	got := lastSeenRoundsFromOrders(2, rows)

	// Seat 0: human every round -> steady state, cursor = 4-1 = 3.
	if want := game.RoundNumber(3); got[0] != want {
		t.Fatalf("seat 0 last_seen_round = %d, want %d", got[0], want)
	}
	// Seat 1: human rounds 1 and 4 only -> round1: min(0+1,0)=0;
	// round4: min(0+1,3)=1.
	if want := game.RoundNumber(1); got[1] != want {
		t.Fatalf("seat 1 last_seen_round = %d, want %d", got[1], want)
	}
}
