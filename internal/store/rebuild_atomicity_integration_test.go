//go:build integration

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/jackc/pgx/v5/pgtype"
)

// This file is the one test in issue #323's real-Postgres acceptance
// criteria that cannot live in rebuild_integration_test.go (package
// store_test) alongside its siblings: TestIntegrationRebuildProjectionsIs
// AtomicOnInjectedFailure overrides afterRebuildProjectionsDelete
// (projections.go), an unexported package-level seam deliberately kept
// unexported so production code can never reach it — exporting it just to
// satisfy storetest's package-boundary convention would widen a test-only
// injection point into the public API for no other purpose. Package store
// cannot import storetest (D46's own cycle warning: store -> storetest ->
// store), so this file keeps its own minimal ad hoc bootstrap, reusing
// migrate_integration_test.go's already-shared startPostgres/sub/
// openDedicated/migrationsFS/postgresImage rather than duplicating them —
// the same structural exception migrate_integration_test.go's own header
// documents for its own four tests. fixtureEvents/snapshotProjections'
// equivalents are reimplemented locally, minimally, rather than shared
// across the package/store_test boundary, which Go's own visibility rules
// do not let two packages' test files reach across either way.

func openRebuildAtomicityStore(t *testing.T) *Store {
	t.Helper()
	dsn := startPostgres(t)
	fsys := sub(t, migrationsFS, "migrations")

	migrateDB := openDedicated(t, dsn)
	if err := migrate(context.Background(), migrateDB, fsys); err != nil {
		t.Fatalf("migrate() against the production migration set: %v", err)
	}

	s, err := Open(context.Background(), Config{DSN: dsn})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func seedRebuildAtomicityMatch(t *testing.T, s *Store) game.MatchID {
	t.Helper()
	ctx := context.Background()

	var userID pgtype.UUID
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO users (display_name) VALUES ('seed') RETURNING id`,
	).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	match, err := New(s.pool).CreateMatch(ctx, CreateMatchParams{
		Config:    []byte(`{}`),
		Seed:      make([]byte, 32),
		CreatedBy: userID,
	})
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}
	return match.ID
}

func rebuildAtomicityFixtureEvents(round game.RoundNumber) []game.Event {
	return []game.Event{
		{Kind: game.EventCargoTaken, Round: round, Seat: 0, Node: 5},
		{Kind: game.EventConfrontation, Round: round, Seat: 1, Target: 2, Decisive: true},
		{Kind: game.EventDelivered, Round: round, Seat: 0, Contract: 3, Tier: 1},
	}
}

func rebuildAtomicityFixtureProjection() (events map[game.RoundNumber][]game.Event, submitted map[game.RoundNumber][]game.SeatID) {
	events = map[game.RoundNumber][]game.Event{
		1: rebuildAtomicityFixtureEvents(1),
		2: rebuildAtomicityFixtureEvents(2),
	}
	submitted = map[game.RoundNumber][]game.SeatID{
		1: {0, 1},
		2: {0},
	}
	return events, submitted
}

func writeRebuildAtomicityFixture(t *testing.T, s *Store, matchID game.MatchID) {
	t.Helper()
	ctx := context.Background()
	events, submitted := rebuildAtomicityFixtureProjection()
	for round := game.RoundNumber(1); round <= 2; round++ {
		if err := s.WriteEvents(ctx, matchID, round, events[round]); err != nil {
			t.Fatalf("WriteEvents(round %d): %v", round, err)
		}
		if err := s.UpsertSummary(ctx, matchID, round, submitted[round]); err != nil {
			t.Fatalf("UpsertSummary(round %d): %v", round, err)
		}
	}
}

type rebuildAtomicityEventSnapshot struct {
	Round   game.RoundNumber
	Seq     int32
	Kind    string
	Payload json.RawMessage
}

type rebuildAtomicitySummarySnapshot struct {
	Round     game.RoundNumber
	Submitted []game.SeatID
}

func snapshotRebuildAtomicityProjections(t *testing.T, s *Store, matchID game.MatchID) []byte {
	t.Helper()
	ctx := context.Background()

	eventRows, err := s.pool.Query(ctx,
		`SELECT round, seq, kind, payload FROM events WHERE match_id = $1 ORDER BY round, seq`,
		matchID)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	var events []rebuildAtomicityEventSnapshot
	for eventRows.Next() {
		var e rebuildAtomicityEventSnapshot
		if err := eventRows.Scan(&e.Round, &e.Seq, &e.Kind, &e.Payload); err != nil {
			eventRows.Close()
			t.Fatalf("scan events row: %v", err)
		}
		events = append(events, e)
	}
	eventRows.Close()
	if err := eventRows.Err(); err != nil {
		t.Fatalf("iterate events rows: %v", err)
	}

	summaryRows, err := s.pool.Query(ctx,
		`SELECT round, submitted_seats FROM match_summary WHERE match_id = $1 ORDER BY round`,
		matchID)
	if err != nil {
		t.Fatalf("query match_summary: %v", err)
	}
	var summaries []rebuildAtomicitySummarySnapshot
	for summaryRows.Next() {
		var sm rebuildAtomicitySummarySnapshot
		if err := summaryRows.Scan(&sm.Round, &sm.Submitted); err != nil {
			summaryRows.Close()
			t.Fatalf("scan match_summary row: %v", err)
		}
		summaries = append(summaries, sm)
	}
	summaryRows.Close()
	if err := summaryRows.Err(); err != nil {
		t.Fatalf("iterate match_summary rows: %v", err)
	}

	out, err := json.Marshal(struct {
		Events    []rebuildAtomicityEventSnapshot
		Summaries []rebuildAtomicitySummarySnapshot
	}{events, summaries})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return out
}

// TestIntegrationRebuildProjectionsIsAtomicOnInjectedFailure is #323's own
// atomicity acceptance criterion: "an injected failure after the delete
// leaves the original rows in place." Neither table's schema gives a
// caller-supplied value anything to violate at insert time (see
// afterRebuildProjectionsDelete's own doc comment in projections.go), so
// the injection here is a context cancelled at exactly that seam — the
// only failure this transaction can actually suffer between its delete and
// insert phases in practice (a lost connection, a statement timeout)
// behaves identically from pgx's side.
func TestIntegrationRebuildProjectionsIsAtomicOnInjectedFailure(t *testing.T) {
	s := openRebuildAtomicityStore(t)
	matchID := seedRebuildAtomicityMatch(t, s)

	writeRebuildAtomicityFixture(t, s, matchID)
	before := snapshotRebuildAtomicityProjections(t, s, matchID)

	cancelCtx, cancel := context.WithCancel(context.Background())
	afterRebuildProjectionsDelete = cancel
	t.Cleanup(func() { afterRebuildProjectionsDelete = func() {} })

	events, submitted := rebuildAtomicityFixtureProjection()
	err := s.RebuildProjections(cancelCtx, matchID, events, submitted)
	if err == nil {
		t.Fatal("RebuildProjections with a context cancelled between delete and insert returned nil error, want the cancellation to surface")
	}
	afterRebuildProjectionsDelete = func() {}

	after := snapshotRebuildAtomicityProjections(t, s, matchID)
	if !bytes.Equal(before, after) {
		t.Fatalf("projections changed despite the injected failure:\nbefore = %s\nafter  = %s", before, after)
	}
}
