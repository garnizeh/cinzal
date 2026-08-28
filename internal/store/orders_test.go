package store

import (
	"context"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// This file unit-tests AppendOrder's own input validation — the part of its
// contract that never touches the database — against a zero-value *Store.
// The resubmission-upsert and human-flips-source behavior that does need a
// real Postgres lives in orders_integration_test.go.

// TestAppendOrderRejectsRoundBelowOne is a CodeRabbit review finding on
// PR #393 (issue #317): game.RoundNumber is 1-indexed (GDD §4), and nothing
// stopped a round < 1 from reaching the orders table before this check
// existed. AppendOrder must reject it before ever encoding the payload or
// touching the pool — asserted here with a nil-pool *Store, since a
// round-1-indexed rejection must not depend on a live connection existing.
func TestAppendOrderRejectsRoundBelowOne(t *testing.T) {
	var s Store // zero value: nil pool, never dereferenced if the check fires first
	ctx := context.Background()
	matchID := game.MatchID("11111111-1111-7111-8111-111111111111")

	for _, round := range []game.RoundNumber{0, -1} {
		err := s.AppendOrder(ctx, matchID, round, 0, game.Order{Round: round}, SourceHuman)
		if err == nil {
			t.Fatalf("AppendOrder(round %d) returned nil error, want a rejection (rounds are 1-indexed)", round)
		}
	}
}
