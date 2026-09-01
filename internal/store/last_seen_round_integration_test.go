//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/store"
	"github.com/garnizeh/cinzal/internal/store/storetest"
)

// This file is issue #409's real-Postgres acceptance criteria for
// RebuildLastSeenRounds — the atomic write primitive itself. cmd/replay's
// own wiring (folding a real order log into a per-seat cursor map and
// calling this function) is exercised separately, in cmd/replay's own
// integration suite, the same split rebuild_integration_test.go already
// draws for RebuildProjections vs. cmd/replay/rebuild.go.

// queryAllLastSeenRounds reads back every seat's last_seen_round for
// matchID, ordered by seat — the deterministic shape these tests compare
// against.
func queryAllLastSeenRounds(t *testing.T, s *store.Store, matchID game.MatchID) map[game.SeatID]int32 {
	t.Helper()
	rows, err := s.Pool().Query(context.Background(),
		`SELECT seat, last_seen_round FROM match_players WHERE match_id = $1 ORDER BY seat`, matchID)
	if err != nil {
		t.Fatalf("query last_seen_round rows: %v", err)
	}
	defer rows.Close()

	got := make(map[game.SeatID]int32)
	for rows.Next() {
		var seat game.SeatID
		var lastSeen int32
		if err := rows.Scan(&seat, &lastSeen); err != nil {
			t.Fatalf("scan last_seen_round row: %v", err)
		}
		got[seat] = lastSeen
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate last_seen_round rows: %v", err)
	}
	return got
}

// TestIntegrationRebuildLastSeenRoundsWritesEverySeat is
// RebuildLastSeenRounds' own happy path: every seat named in bySeat lands
// with exactly that value.
func TestIntegrationRebuildLastSeenRoundsWritesEverySeat(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)

	matchID, _, err := s.CreateMatch(ctx, [32]byte{1}, game.DefaultConfig(), mustSeats(3), createdBy, nil, nil)
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}

	want := map[game.SeatID]game.RoundNumber{0: 4, 1: 0, 2: 11}
	if err := s.RebuildLastSeenRounds(ctx, matchID, want); err != nil {
		t.Fatalf("RebuildLastSeenRounds: %v", err)
	}

	got := queryAllLastSeenRounds(t, s, matchID)
	for seat, wantRound := range want {
		if got[seat] != int32(wantRound) {
			t.Errorf("seat %d last_seen_round = %d, want %d", seat, got[seat], wantRound)
		}
	}
}

// TestIntegrationRebuildLastSeenRoundsOverwritesStaleValue is
// RebuildLastSeenRounds' own doc comment made concrete: unlike
// events/match_summary, this table is never cleared first — a rebuild must
// overwrite whatever value is already on the row, not merely agree with a
// fresh row's own default.
func TestIntegrationRebuildLastSeenRoundsOverwritesStaleValue(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)

	matchID, _, err := s.CreateMatch(ctx, [32]byte{2}, game.DefaultConfig(), mustSeats(2), createdBy, nil, nil)
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}

	if _, err := s.Pool().Exec(ctx,
		`UPDATE match_players SET last_seen_round = 999 WHERE match_id = $1 AND seat = 0`, matchID,
	); err != nil {
		t.Fatalf("corrupt last_seen_round: %v", err)
	}

	if err := s.RebuildLastSeenRounds(ctx, matchID, map[game.SeatID]game.RoundNumber{0: 3, 1: 3}); err != nil {
		t.Fatalf("RebuildLastSeenRounds: %v", err)
	}

	got := queryAllLastSeenRounds(t, s, matchID)
	if got[0] != 3 {
		t.Fatalf("seat 0 last_seen_round after rebuild = %d, want 3 (the stale 999 must not survive)", got[0])
	}
}

// TestIntegrationRebuildLastSeenRoundsIsAtomicOnInjectedFailure asserts a
// failure partway through the per-seat update loop leaves every seat's row
// exactly as it was — the same "point-in-time, all-or-nothing" guarantee
// RebuildProjections gives its own delete-then-rewrite, applied here to a
// row-by-row overwrite instead.
func TestIntegrationRebuildLastSeenRoundsIsAtomicOnInjectedFailure(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)

	matchID, _, err := s.CreateMatch(ctx, [32]byte{3}, game.DefaultConfig(), mustSeats(2), createdBy, nil, nil)
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}

	before := queryAllLastSeenRounds(t, s, matchID)

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	err = s.RebuildLastSeenRounds(cancelCtx, matchID, map[game.SeatID]game.RoundNumber{0: 5, 1: 5})
	if err == nil {
		t.Fatal("RebuildLastSeenRounds with an already-cancelled context returned nil error, want the cancellation to surface")
	}

	after := queryAllLastSeenRounds(t, s, matchID)
	for seat, wantRound := range before {
		if after[seat] != wantRound {
			t.Fatalf("seat %d last_seen_round changed despite the injected failure: before = %d, after = %d", seat, wantRound, after[seat])
		}
	}
}
