//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/store"
	"github.com/garnizeh/cinzal/internal/store/storetest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// This file is issue #321's real-Postgres acceptance criteria for
// WriteEvents, UpsertSummary and ClearProjections: the seq-reproducibility
// property, UpsertSummary's idempotence, and ClearProjections' match-scoped
// isolation, none of which a mocked DBTX could actually prove.
//
// Every test below gets its *store.Store from storetest.Container (#325,
// D46) — one documented entry point, a transaction against the shared work
// database rolled back in t.Cleanup, rather than this file starting its own
// container.

// seedProjectionsMatch inserts one user and one matches row and returns the
// match's id. events/match_summary's own FK is to matches(id) alone
// (migration 00001) — neither table needs a match_players row to write
// against, unlike orders'.
func seedProjectionsMatch(t *testing.T, s *store.Store) game.MatchID {
	t.Helper()
	ctx := context.Background()

	var userID pgtype.UUID
	if err := s.Pool().QueryRow(ctx,
		`INSERT INTO users (display_name) VALUES ('seed') RETURNING id`,
	).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	match, err := store.New(s.Pool()).CreateMatch(ctx, store.CreateMatchParams{
		Config:    []byte(`{}`),
		Seed:      make([]byte, 32),
		CreatedBy: userID,
	})
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}
	return match.ID
}

// fixtureEvents is a small, hand-built []game.Event standing in for one
// round's worth of Resolve() output — three different EventKind values, so
// the seq/kind assertions below cannot pass by coincidentally checking only
// one row.
func fixtureEvents(round game.RoundNumber) []game.Event {
	return []game.Event{
		{Kind: game.EventCargoTaken, Round: round, Seat: 0, Node: 5},
		{Kind: game.EventConfrontation, Round: round, Seat: 1, Target: 2, Decisive: true},
		{Kind: game.EventDelivered, Round: round, Seat: 0, Contract: 3, Tier: 1},
	}
}

type eventRow struct {
	Seq     int32
	Kind    string
	Payload []byte
}

func queryEventRows(t *testing.T, s *store.Store, matchID game.MatchID, round game.RoundNumber) []eventRow {
	t.Helper()
	rows, err := s.Pool().Query(context.Background(),
		`SELECT seq, kind, payload FROM events WHERE match_id = $1 AND round = $2 ORDER BY seq`,
		matchID, round)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	defer rows.Close()

	var got []eventRow
	for rows.Next() {
		var r eventRow
		if err := rows.Scan(&r.Seq, &r.Kind, &r.Payload); err != nil {
			t.Fatalf("scan events row: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate events rows: %v", err)
	}
	return got
}

// TestIntegrationWriteEventsSeqMatchesResolveOrderAndKindIsBareOrdinal is
// #321's own acceptance criterion: "WriteEvents inserts a round's events in
// Resolve's own order with seq derived from that order, not from a sequence
// generator" — asserted by checking seq == the event's own index in the
// slice passed in, in order — plus events.kind's own encoding: the bare
// EventKind ordinal, not its String() name.
func TestIntegrationWriteEventsSeqMatchesResolveOrderAndKindIsBareOrdinal(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	matchID := seedProjectionsMatch(t, s)

	events := fixtureEvents(1)
	if err := s.WriteEvents(ctx, matchID, 1, events); err != nil {
		t.Fatalf("WriteEvents: %v", err)
	}

	got := queryEventRows(t, s, matchID, 1)
	if len(got) != len(events) {
		t.Fatalf("events row count = %d, want %d", len(got), len(events))
	}
	for i, row := range got {
		if row.Seq != int32(i) {
			t.Fatalf("row %d: seq = %d, want %d (Resolve's own order)", i, row.Seq, i)
		}
		wantKind := strconv.Itoa(int(events[i].Kind))
		if row.Kind != wantKind {
			t.Fatalf("row %d: kind = %q, want the bare ordinal %q (never EventKind.String()'s display name)", i, row.Kind, wantKind)
		}
		var decoded game.Event
		if err := json.Unmarshal(row.Payload, &decoded); err != nil {
			t.Fatalf("row %d: decode payload: %v", i, err)
		}
		if decoded != events[i] {
			t.Fatalf("row %d: decoded payload = %+v, want %+v", i, decoded, events[i])
		}
	}
}

// TestIntegrationWriteEventsSeqReproducibleAfterClearProjections is #321's
// byte-identical-rebuild acceptance criterion: "re-running it for the same
// round after ClearProjections reproduces identical seq values."
func TestIntegrationWriteEventsSeqReproducibleAfterClearProjections(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	matchID := seedProjectionsMatch(t, s)
	events := fixtureEvents(1)

	if err := s.WriteEvents(ctx, matchID, 1, events); err != nil {
		t.Fatalf("WriteEvents (first write): %v", err)
	}
	first := queryEventRows(t, s, matchID, 1)

	if err := s.ClearProjections(ctx, matchID); err != nil {
		t.Fatalf("ClearProjections: %v", err)
	}
	if rows := queryEventRows(t, s, matchID, 1); len(rows) != 0 {
		t.Fatalf("events rows after ClearProjections = %d, want 0", len(rows))
	}

	if err := s.WriteEvents(ctx, matchID, 1, events); err != nil {
		t.Fatalf("WriteEvents (rebuild write): %v", err)
	}
	second := queryEventRows(t, s, matchID, 1)

	if len(first) != len(second) {
		t.Fatalf("row count after rebuild = %d, want %d (identical to the first write)", len(second), len(first))
	}
	for i := range first {
		if first[i].Seq != second[i].Seq || first[i].Kind != second[i].Kind || string(first[i].Payload) != string(second[i].Payload) {
			t.Fatalf("row %d changed across rebuild: first = %+v, second = %+v", i, first[i], second[i])
		}
	}
}

func queryMatchSummaryRow(t *testing.T, s *store.Store, matchID game.MatchID, round game.RoundNumber) (submitted []game.SeatID, updatedAt time.Time, found bool) {
	t.Helper()
	err := s.Pool().QueryRow(context.Background(),
		`SELECT submitted_seats, updated_at FROM match_summary WHERE match_id = $1 AND round = $2`,
		matchID, round,
	).Scan(&submitted, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, time.Time{}, false
		}
		t.Fatalf("query match_summary: %v", err)
	}
	return submitted, updatedAt, true
}

// TestIntegrationUpsertSummaryIdempotentUpdatesTimestamp is #321's own
// acceptance criterion: "UpsertSummary is idempotent for a (match_id,
// round) and updates updated_at" — a second call for the same pair
// replaces the row's submitted_seats rather than adding a row, and
// updated_at strictly advances.
//
// This needs storetest.FreshDatabase, not storetest.Container: updated_at's
// DEFAULT is now(), and Postgres pins now() to the enclosing transaction's
// start time (unlike clock_timestamp(), which ratelimit.go's own SQL uses
// deliberately for exactly this reason — see ratelimit_integration_test.go).
// Two UpsertSummary calls sharing Container's one ambient transaction would
// both see the identical now(), regardless of the real time.Sleep between
// them, and this test would pass or fail for the wrong reason.
func TestIntegrationUpsertSummaryIdempotentUpdatesTimestamp(t *testing.T) {
	dsn := storetest.FreshDatabase(t)
	s, err := store.Open(context.Background(), store.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(s.Close)
	ctx := context.Background()
	matchID := seedProjectionsMatch(t, s)

	if err := s.UpsertSummary(ctx, matchID, 1, []game.SeatID{0, 1}); err != nil {
		t.Fatalf("UpsertSummary (first): %v", err)
	}
	firstSeats, firstUpdatedAt, found := queryMatchSummaryRow(t, s, matchID, 1)
	if !found {
		t.Fatal("match_summary row not found after first UpsertSummary")
	}
	if len(firstSeats) != 2 {
		t.Fatalf("submitted_seats after first UpsertSummary = %v, want 2 seats", firstSeats)
	}

	time.Sleep(10 * time.Millisecond) // guarantee now() advances between writes

	if err := s.UpsertSummary(ctx, matchID, 1, []game.SeatID{0, 1, 2}); err != nil {
		t.Fatalf("UpsertSummary (second): %v", err)
	}

	var count int
	if err := s.Pool().QueryRow(ctx,
		`SELECT count(*) FROM match_summary WHERE match_id = $1 AND round = 1`, matchID,
	).Scan(&count); err != nil {
		t.Fatalf("count match_summary: %v", err)
	}
	if count != 1 {
		t.Fatalf("match_summary row count = %d after second UpsertSummary, want exactly 1 (idempotent)", count)
	}

	secondSeats, secondUpdatedAt, found := queryMatchSummaryRow(t, s, matchID, 1)
	if !found {
		t.Fatal("match_summary row not found after second UpsertSummary")
	}
	if len(secondSeats) != 3 {
		t.Fatalf("submitted_seats after second UpsertSummary = %v, want 3 seats", secondSeats)
	}
	if !secondUpdatedAt.After(firstUpdatedAt) {
		t.Fatalf("updated_at did not advance across the second UpsertSummary: first = %v, second = %v", firstUpdatedAt, secondUpdatedAt)
	}
}

// TestIntegrationClearProjectionsTouchesOnlyItsOwnMatch is #321's own
// acceptance criterion: "ClearProjections removes both tables' rows for one
// match and touches nothing else — asserted against a database holding two
// matches."
func TestIntegrationClearProjectionsTouchesOnlyItsOwnMatch(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	matchA := seedProjectionsMatch(t, s)
	matchB := seedProjectionsMatch(t, s)

	for _, m := range []game.MatchID{matchA, matchB} {
		if err := s.WriteEvents(ctx, m, 1, fixtureEvents(1)); err != nil {
			t.Fatalf("WriteEvents(%s): %v", m, err)
		}
		if err := s.UpsertSummary(ctx, m, 1, []game.SeatID{0}); err != nil {
			t.Fatalf("UpsertSummary(%s): %v", m, err)
		}
	}

	if err := s.ClearProjections(ctx, matchA); err != nil {
		t.Fatalf("ClearProjections(matchA): %v", err)
	}

	if rows := queryEventRows(t, s, matchA, 1); len(rows) != 0 {
		t.Fatalf("matchA events rows after its own ClearProjections = %d, want 0", len(rows))
	}
	if _, _, found := queryMatchSummaryRow(t, s, matchA, 1); found {
		t.Fatal("matchA match_summary row still present after its own ClearProjections")
	}

	if rows := queryEventRows(t, s, matchB, 1); len(rows) != 3 {
		t.Fatalf("matchB events rows after matchA's ClearProjections = %d, want 3 (untouched)", len(rows))
	}
	if _, _, found := queryMatchSummaryRow(t, s, matchB, 1); !found {
		t.Fatal("matchB match_summary row was removed by matchA's ClearProjections")
	}
}
