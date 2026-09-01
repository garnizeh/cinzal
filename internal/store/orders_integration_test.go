//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/store"
	"github.com/garnizeh/cinzal/internal/store/storetest"
	"github.com/jackc/pgx/v5/pgtype"
)

// This file is issue #317's real-Postgres acceptance criteria for
// AppendOrder itself: the resubmission upsert (GDD §18's "the last
// submission stands") and the human-flips-source Autopilot-undo
// (RFC-001 §8.2). The full AppendOrder -> orderlog.Load round trip and the
// round-gap rejection live in internal/store/orderlog's own integration
// test — orderlog cannot be imported from a package-store test file
// without an import cycle (orderlog itself imports internal/store), so
// anything needing orderlog.Load runs there instead.
//
// Every test below gets its *store.Store from storetest.Container (#325,
// D46) — one documented entry point, a transaction against the shared work
// database rolled back in t.Cleanup, rather than this file starting its own
// container.

// seedOrderMatchWithSeats inserts one user (as the match's creator), one
// matches row, and one match_players row per seat in [0, numSeats) —
// orders' composite FK (match_id, seat) -> match_players(match_id, seat)
// requires every seat an order names to actually exist as a participant.
func seedOrderMatchWithSeats(t *testing.T, s *store.Store, numSeats int) game.MatchID {
	t.Helper()
	ctx := context.Background()

	var userID pgtype.UUID
	if err := s.Pool().QueryRow(ctx,
		`INSERT INTO users (display_name) VALUES ('seed') RETURNING id`,
	).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	q := store.New(s.Pool())
	match, err := q.CreateMatch(ctx, store.CreateMatchParams{
		Config:    []byte(`{}`),
		Seed:      make([]byte, 32),
		CreatedBy: userID,
	})
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}

	for seat := 0; seat < numSeats; seat++ {
		if _, err := q.CreateMatchPlayer(ctx, store.CreateMatchPlayerParams{
			MatchID:              match.ID,
			Seat:                 game.SeatID(seat),
			Faction:              "test",
			UnsubscribeTokenHash: []byte{0},
		}); err != nil {
			t.Fatalf("CreateMatchPlayer seat %d: %v", seat, err)
		}
	}

	return match.ID
}

// TestIntegrationAppendOrderResubmissionReplacesRow is GDD §18's own rule,
// asserted against real Postgres: resubmission within an open round
// replaces the row and leaves exactly one row for (match_id, round, seat).
func TestIntegrationAppendOrderResubmissionReplacesRow(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	matchID := seedOrderMatchWithSeats(t, s, 1)

	first := game.Order{Round: 1, Route: []game.NodeID{1, 2}}
	if err := s.AppendOrder(ctx, matchID, 1, 0, first, store.SourceHuman); err != nil {
		t.Fatalf("AppendOrder (first submission): %v", err)
	}

	second := game.Order{Round: 1, Route: []game.NodeID{9}}
	if err := s.AppendOrder(ctx, matchID, 1, 0, second, store.SourceHuman); err != nil {
		t.Fatalf("AppendOrder (resubmission): %v", err)
	}

	var count int
	if err := s.Pool().QueryRow(ctx,
		`SELECT count(*) FROM orders WHERE match_id = $1 AND round = 1 AND seat = 0`, matchID,
	).Scan(&count); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if count != 1 {
		t.Fatalf("orders row count for (match, round 1, seat 0) = %d, want exactly 1", count)
	}

	var payload []byte
	if err := s.Pool().QueryRow(ctx,
		`SELECT payload FROM orders WHERE match_id = $1 AND round = 1 AND seat = 0`, matchID,
	).Scan(&payload); err != nil {
		t.Fatalf("query payload: %v", err)
	}

	var got game.Order
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode stored payload: %v", err)
	}
	if !got.Equal(second) {
		t.Fatalf("stored order = %+v, want the resubmission %+v (last submission must stand)", got, second)
	}
}

// TestIntegrationAppendOrderHumanResubmissionFlipsSourceFromDefault is
// RFC-001 §8.2's Autopilot-undo mechanism: a human resubmission over a
// default row flips source to 'human', which is what makes autopilot(seat)
// false again on the very next round considered.
func TestIntegrationAppendOrderHumanResubmissionFlipsSourceFromDefault(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	matchID := seedOrderMatchWithSeats(t, s, 1)

	if err := s.AppendOrder(ctx, matchID, 1, 0, game.Order{Round: 1}, store.SourceDefault); err != nil {
		t.Fatalf("AppendOrder (default): %v", err)
	}
	if err := s.AppendOrder(ctx, matchID, 1, 0, game.Order{Round: 1}, store.SourceHuman); err != nil {
		t.Fatalf("AppendOrder (human resubmission): %v", err)
	}

	var source string
	if err := s.Pool().QueryRow(ctx,
		`SELECT source FROM orders WHERE match_id = $1 AND round = 1 AND seat = 0`, matchID,
	).Scan(&source); err != nil {
		t.Fatalf("query source: %v", err)
	}
	if source != string(store.SourceHuman) {
		t.Fatalf("source after human resubmission = %q, want %q", source, store.SourceHuman)
	}
}
