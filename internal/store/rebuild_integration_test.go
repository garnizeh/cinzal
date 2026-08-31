//go:build integration

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// This file is issue #323's real-Postgres acceptance criteria for
// RebuildProjections: byte-identical regeneration (including from three
// kinds of deliberate corruption), atomicity on an injected mid-transaction
// failure, and the "touches nothing else" guarantee — outbox, orders and
// matches. cmd/replay's own wiring (folding a real order log and calling
// this function) is exercised separately, in cmd/replay's own integration
// suite; this file tests the atomic primitive itself, the same split
// projections_integration_test.go already draws for WriteEvents/
// UpsertSummary/ClearProjections.

// fixtureProjection is a small, two-round fixture standing in for what a
// real fold would produce: fixtureEvents(round) for events, and a
// different submitted-seat set per round for match_summary — different
// per round so a bug that swapped the two rounds' content would be caught.
func fixtureProjection() (events map[game.RoundNumber][]game.Event, submitted map[game.RoundNumber][]game.SeatID) {
	events = map[game.RoundNumber][]game.Event{
		1: fixtureEvents(1),
		2: fixtureEvents(2),
	}
	submitted = map[game.RoundNumber][]game.SeatID{
		1: {0, 1},
		2: {0},
	}
	return events, submitted
}

// eventSnapshot and summarySnapshot are the stable, deterministic
// serialisation issue #323's own acceptance criteria ask for: "compared
// over a stable serialisation of the rows in a deterministic order."
// summarySnapshot omits updated_at deliberately — UpsertMatchSummary's own
// doc comment guarantees it advances on every write, so a byte-identical
// comparison across two separate writes (the original, then a rebuild) can
// only ever hold over the columns that carry the actual projection content.
type eventSnapshot struct {
	Round   game.RoundNumber
	Seq     int32
	Kind    string
	Payload json.RawMessage
}

type summarySnapshot struct {
	Round     game.RoundNumber
	Submitted []game.SeatID
}

// snapshotProjections reads matchID's own events and match_summary rows
// back in a fixed order (round, then seq for events) and marshals them to
// one deterministic byte slice — the "stable serialisation" the byte-
// identical acceptance criteria compare.
func snapshotProjections(t *testing.T, s *Store, matchID game.MatchID) []byte {
	t.Helper()
	ctx := context.Background()

	eventRows, err := s.pool.Query(ctx,
		`SELECT round, seq, kind, payload FROM events WHERE match_id = $1 ORDER BY round, seq`,
		matchID)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	var events []eventSnapshot
	for eventRows.Next() {
		var e eventSnapshot
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
	var summaries []summarySnapshot
	for summaryRows.Next() {
		var sm summarySnapshot
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
		Events    []eventSnapshot
		Summaries []summarySnapshot
	}{events, summaries})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return out
}

// writeFixtureProjection writes fixtureProjection's own content through the
// ordinary per-round write path (WriteEvents/UpsertSummary) — standing in
// for what the round tick (M4) would have written originally, so
// RebuildProjections' own output can be compared against a write that did
// not go through it.
func writeFixtureProjection(t *testing.T, s *Store, matchID game.MatchID) {
	t.Helper()
	ctx := context.Background()
	events, submitted := fixtureProjection()
	for round := game.RoundNumber(1); round <= 2; round++ {
		if err := s.WriteEvents(ctx, matchID, round, events[round]); err != nil {
			t.Fatalf("WriteEvents(round %d): %v", round, err)
		}
		if err := s.UpsertSummary(ctx, matchID, round, submitted[round]); err != nil {
			t.Fatalf("UpsertSummary(round %d): %v", round, err)
		}
	}
}

// TestRebuildProjectionsIsByteIdenticalToOriginalWrite is #323's central
// acceptance criterion: "rebuilding a match whose projections are intact
// produces byte-identical events and match_summary content." It also fails
// closed (#323's own explicit requirement): the snapshot is asserted
// non-empty and to contain a known event kind from the fixture, so two
// empty tables agreeing vacuously cannot pass this test.
func TestRebuildProjectionsIsByteIdenticalToOriginalWrite(t *testing.T) {
	s := openProjectionsStore(t)
	ctx := context.Background()
	matchID := seedProjectionsMatch(t, s)

	writeFixtureProjection(t, s, matchID)
	original := snapshotProjections(t, s, matchID)

	if len(original) == 0 {
		t.Fatal("original snapshot is empty")
	}
	if !bytes.Contains(original, []byte(`"Kind":"1"`)) {
		t.Fatalf("original snapshot = %s, want it to contain a known event kind (EventCargoTaken, ordinal 1)", original)
	}

	events, submitted := fixtureProjection()
	if err := s.RebuildProjections(ctx, matchID, events, submitted); err != nil {
		t.Fatalf("RebuildProjections: %v", err)
	}
	rebuilt := snapshotProjections(t, s, matchID)

	if !bytes.Equal(original, rebuilt) {
		t.Fatalf("rebuilt snapshot differs from the original write:\noriginal = %s\nrebuilt  = %s", original, rebuilt)
	}
}

// TestRebuildProjectionsRecoversFromCorruption is #323's own three
// corruption cases: "a payload edited, a row deleted, a seq shuffled" —
// each restored to the pristine content by a rebuild driven from the same
// (uncorrupted) source data a real fold would have produced, since a
// rebuild never reads the tables it is about to overwrite.
func TestRebuildProjectionsRecoversFromCorruption(t *testing.T) {
	tests := []struct {
		name    string
		corrupt func(t *testing.T, s *Store, matchID game.MatchID)
	}{
		{
			name: "row deleted",
			corrupt: func(t *testing.T, s *Store, matchID game.MatchID) {
				if _, err := s.pool.Exec(context.Background(),
					`DELETE FROM events WHERE match_id = $1 AND round = 1 AND seq = 0`, matchID,
				); err != nil {
					t.Fatalf("corrupt (delete row): %v", err)
				}
			},
		},
		{
			name: "payload edited",
			corrupt: func(t *testing.T, s *Store, matchID game.MatchID) {
				if _, err := s.pool.Exec(context.Background(),
					`UPDATE events SET payload = '{"garbage":true}' WHERE match_id = $1 AND round = 1 AND seq = 0`, matchID,
				); err != nil {
					t.Fatalf("corrupt (edit payload): %v", err)
				}
			},
		},
		{
			name: "seq shuffled",
			corrupt: func(t *testing.T, s *Store, matchID game.MatchID) {
				// Swap seq 0 and seq 1 within round 1 via a temporary
				// out-of-range value, since (match_id, round, seq) is the
				// primary key and a direct swap would collide mid-statement.
				ctx := context.Background()
				if _, err := s.pool.Exec(ctx, `UPDATE events SET seq = -1 WHERE match_id = $1 AND round = 1 AND seq = 0`, matchID); err != nil {
					t.Fatalf("corrupt (shuffle seq, step 1): %v", err)
				}
				if _, err := s.pool.Exec(ctx, `UPDATE events SET seq = 0 WHERE match_id = $1 AND round = 1 AND seq = 1`, matchID); err != nil {
					t.Fatalf("corrupt (shuffle seq, step 2): %v", err)
				}
				if _, err := s.pool.Exec(ctx, `UPDATE events SET seq = 1 WHERE match_id = $1 AND round = 1 AND seq = -1`, matchID); err != nil {
					t.Fatalf("corrupt (shuffle seq, step 3): %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openProjectionsStore(t)
			ctx := context.Background()
			matchID := seedProjectionsMatch(t, s)

			writeFixtureProjection(t, s, matchID)
			pristine := snapshotProjections(t, s, matchID)

			tt.corrupt(t, s, matchID)
			if corrupted := snapshotProjections(t, s, matchID); bytes.Equal(corrupted, pristine) {
				t.Fatal("corruption step did not actually change the snapshot — test is not exercising anything")
			}

			events, submitted := fixtureProjection()
			if err := s.RebuildProjections(ctx, matchID, events, submitted); err != nil {
				t.Fatalf("RebuildProjections after corruption (%s): %v", tt.name, err)
			}

			restored := snapshotProjections(t, s, matchID)
			if !bytes.Equal(pristine, restored) {
				t.Fatalf("rebuild after corruption (%s) did not restore the pristine content:\npristine = %s\nrestored = %s", tt.name, pristine, restored)
			}
		})
	}
}

// TestRebuildProjectionsIsAtomicOnInjectedFailure is #323's own atomicity
// acceptance criterion: "an injected failure after the delete leaves the
// original rows in place." Neither table's schema gives a caller-supplied
// value anything to violate at insert time (see afterRebuildProjectionsDelete's
// own doc comment in projections.go), so the injection here is a context
// cancelled at exactly that seam — the only failure this transaction can
// actually suffer between its delete and insert phases in practice (a lost
// connection, a statement timeout) behaves identically from pgx's side.
func TestRebuildProjectionsIsAtomicOnInjectedFailure(t *testing.T) {
	s := openProjectionsStore(t)
	matchID := seedProjectionsMatch(t, s)

	writeFixtureProjection(t, s, matchID)
	before := snapshotProjections(t, s, matchID)

	cancelCtx, cancel := context.WithCancel(context.Background())
	afterRebuildProjectionsDelete = cancel
	t.Cleanup(func() { afterRebuildProjectionsDelete = func() {} })

	events, submitted := fixtureProjection()
	err := s.RebuildProjections(cancelCtx, matchID, events, submitted)
	if err == nil {
		t.Fatal("RebuildProjections with a context cancelled between delete and insert returned nil error, want the cancellation to surface")
	}
	afterRebuildProjectionsDelete = func() {}

	after := snapshotProjections(t, s, matchID)
	if !bytes.Equal(before, after) {
		t.Fatalf("projections changed despite the injected failure:\nbefore = %s\nafter  = %s", before, after)
	}
}

// TestRebuildProjectionsTouchesOnlyEventsAndMatchSummary is #323's own
// "nothing else is written" acceptance criteria: orders, matches and
// outbox are all asserted unchanged across a rebuild — orders and matches
// because the log is not rebuildable and must never be touched by the tool
// that rebuilds things, and outbox because RFC §7.4's null-sink guarantee
// applies to --rebuild specifically, not only to the round tick's own
// fold.
func TestRebuildProjectionsTouchesOnlyEventsAndMatchSummary(t *testing.T) {
	s := openProjectionsStore(t)
	ctx := context.Background()
	matchID := seedProjectionsMatch(t, s)
	writeFixtureProjection(t, s, matchID)

	// orders' own FK is (match_id, seat) -> match_players (migration 00001),
	// which seedProjectionsMatch does not create — events/match_summary
	// need no seat roster, but AppendOrder below does.
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO match_players (match_id, seat, faction, unsubscribe_token_hash) VALUES ($1, 0, 'test', decode('00', 'hex'))`,
		matchID,
	); err != nil {
		t.Fatalf("seed match_players row: %v", err)
	}

	if err := s.AppendOrder(ctx, matchID, 1, 0,
		game.Order{Round: 1, Action: game.ActionOrder{Kind: game.ActionNothing}, Stance: game.StanceOrder{Stance: game.StanceNeutral}},
		SourceHuman); err != nil {
		t.Fatalf("AppendOrder: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO outbox (to_email, template, payload, match_id, round, seat) VALUES ($1, $2, $3, $4, $5, $6)`,
		"player@example.com", "round_resolved", []byte(`{}`), matchID, 1, 0,
	); err != nil {
		t.Fatalf("seed outbox row: %v", err)
	}

	beforeOrders := snapshotOrders(t, s, matchID)
	beforeMatch := snapshotMatch(t, s, matchID)
	var beforeOutboxCount int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&beforeOutboxCount); err != nil {
		t.Fatalf("count outbox (before): %v", err)
	}

	events, submitted := fixtureProjection()
	if err := s.RebuildProjections(ctx, matchID, events, submitted); err != nil {
		t.Fatalf("RebuildProjections: %v", err)
	}

	afterOrders := snapshotOrders(t, s, matchID)
	afterMatch := snapshotMatch(t, s, matchID)
	var afterOutboxCount int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&afterOutboxCount); err != nil {
		t.Fatalf("count outbox (after): %v", err)
	}

	if !bytes.Equal(beforeOrders, afterOrders) {
		t.Fatalf("orders changed across a rebuild:\nbefore = %s\nafter  = %s", beforeOrders, afterOrders)
	}
	if !bytes.Equal(beforeMatch, afterMatch) {
		t.Fatalf("matches row changed across a rebuild:\nbefore = %s\nafter  = %s", beforeMatch, afterMatch)
	}
	if afterOutboxCount != beforeOutboxCount {
		t.Fatalf("outbox row count changed across a rebuild: before = %d, after = %d, want unchanged (RFC §7.4/§16.1's Effects row)", beforeOutboxCount, afterOutboxCount)
	}
}

type orderSnapshot struct {
	Round   game.RoundNumber
	Seat    game.SeatID
	Payload json.RawMessage
	Source  string
}

func snapshotOrders(t *testing.T, s *Store, matchID game.MatchID) []byte {
	t.Helper()
	rows, err := s.pool.Query(context.Background(),
		`SELECT round, seat, payload, source FROM orders WHERE match_id = $1 ORDER BY round, seat`, matchID)
	if err != nil {
		t.Fatalf("query orders: %v", err)
	}
	defer rows.Close()
	var out []orderSnapshot
	for rows.Next() {
		var o orderSnapshot
		if err := rows.Scan(&o.Round, &o.Seat, &o.Payload, &o.Source); err != nil {
			t.Fatalf("scan orders row: %v", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate orders rows: %v", err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal orders snapshot: %v", err)
	}
	return b
}

func snapshotMatch(t *testing.T, s *Store, matchID game.MatchID) []byte {
	t.Helper()
	var status string
	var config, seed []byte
	var round int32
	if err := s.pool.QueryRow(context.Background(),
		`SELECT status, config, seed, round FROM matches WHERE id = $1`, matchID,
	).Scan(&status, &config, &seed, &round); err != nil {
		t.Fatalf("query matches row: %v", err)
	}
	b, err := json.Marshal(struct {
		Status string
		Config json.RawMessage
		Seed   []byte
		Round  int32
	}{status, config, seed, round})
	if err != nil {
		t.Fatalf("marshal match snapshot: %v", err)
	}
	return b
}
