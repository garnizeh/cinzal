//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
	"github.com/garnizeh/cinzal/internal/store"
	"github.com/garnizeh/cinzal/internal/store/storetest"
	"github.com/jackc/pgx/v5/pgtype"
)

// This file is issue #322's real-Postgres acceptance criteria for the
// --db/--match path and the bundle export/assemble equality property. The
// pure decode/dump logic (--bundle, flag validation, the fog-negative
// assertion) is exercised with no database at all in run_test.go and
// bundle_test.go — D46's split, mirrored here the same way
// internal/store/orderlog keeps its own pure decode tests separate from its
// //go:build integration file.

// openReplayStore returns a DSN and a *store.Store both pointed at a
// freshly cloned, already-migrated database (storetest.FreshDatabase, D46
// tier 2) rather than storetest.Container's shared per-test transaction —
// every test below passes the DSN to run() as --db, which opens its own
// separate connection to the same database, so the seeding *Store and the
// CLI under test must share a real, independently connectable database
// rather than one ambient transaction only this file's own *Store could see.
func openReplayStore(t *testing.T) (dsn string, s *store.Store) {
	t.Helper()
	dsn = storetest.FreshDatabase(t)
	s, err := store.Open(context.Background(), store.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(s.Close)
	return dsn, s
}

// seedFullReplayMatch creates a match via the real Store.CreateMatch and
// appends one idle order per (round, seat) through the whole
// testFixture-shaped log via the real Store.AppendOrder — the production
// write path, not a hand-rolled INSERT, so this exercises the exact rows
// --match reads back.
func seedFullReplayMatch(t *testing.T, s *store.Store) game.MatchID {
	t.Helper()
	ctx := context.Background()
	cfg, seed, players, log := testFixture()

	var userID pgtype.UUID
	if err := s.Pool().QueryRow(ctx,
		`INSERT INTO users (display_name) VALUES ('seed') RETURNING id`,
	).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	seats := make([]store.SeatSpec, players)
	for i := range seats {
		seats[i] = store.SeatSpec{Faction: "test"}
	}

	matchID, _, err := s.CreateMatch(ctx, seed, cfg, seats, userID, nil, nil)
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}

	for round, orders := range log {
		for seat, o := range orders {
			if err := s.AppendOrder(ctx, matchID, round, seat, o, store.SourceHuman); err != nil {
				t.Fatalf("AppendOrder (round %d, seat %d): %v", round, seat, err)
			}
		}
	}

	return matchID
}

// TestIntegrationReplayMatchAndBundleProduceByteIdenticalOutput is #322's own
// acceptance criterion: "replay --bundle f.json produces byte-identical
// output to --match for the same match, which is the property that makes a
// bug report reproducible."
func TestIntegrationReplayMatchAndBundleProduceByteIdenticalOutput(t *testing.T) {
	dsn, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)

	var fromDB, stderr bytes.Buffer
	if code := run([]string{"--db", dsn, "--match", string(matchID)}, &fromDB, &stderr); code != 0 {
		t.Fatalf("run(--match) = %d, stderr = %s", code, stderr.String())
	}

	bundlePath := filepath.Join(t.TempDir(), "exported.json")
	stderr.Reset()
	var exportOut bytes.Buffer
	if code := run([]string{"--db", dsn, "--match", string(matchID), "--export-bundle", bundlePath}, &exportOut, &stderr); code != 0 {
		t.Fatalf("run(--export-bundle) = %d, stderr = %s", code, stderr.String())
	}

	stderr.Reset()
	var fromBundle bytes.Buffer
	if code := run([]string{"--bundle", bundlePath}, &fromBundle, &stderr); code != 0 {
		t.Fatalf("run(--bundle) = %d, stderr = %s", code, stderr.String())
	}

	if !bytes.Equal(fromDB.Bytes(), fromBundle.Bytes()) {
		t.Fatal("run(--match) and run(--bundle) for the same match produced different bytes")
	}
	if fromDB.Len() == 0 {
		t.Fatal("dump is empty")
	}
}

// TestIntegrationExportBundleFromDBEqualsAssembledFromRows is #322's own acceptance
// criterion: "a test asserts a bundle exported from a match and a bundle
// assembled from that match's rows are equal." exportBundleFromDB is
// exercised via the package's own function; the "assembled from rows" half
// is a second, independent query built directly in this test rather than
// reusing that function, so the two constructions are genuinely separate.
func TestIntegrationExportBundleFromDBEqualsAssembledFromRows(t *testing.T) {
	_, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)
	ctx := context.Background()

	got, err := exportBundleFromDB(ctx, s.Pool(), matchID)
	if err != nil {
		t.Fatalf("exportBundleFromDB: %v", err)
	}

	q := store.New(s.Pool())
	m, err := q.GetMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("GetMatch: %v", err)
	}
	seats, err := q.ListMatchPlayers(ctx, matchID)
	if err != nil {
		t.Fatalf("ListMatchPlayers: %v", err)
	}
	rows, err := q.ListOrdersForMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("ListOrdersForMatch: %v", err)
	}
	want := Bundle{Seed: m.Seed, Config: json.RawMessage(m.Config), Players: len(seats), OrderLog: make([]BundleOrder, len(rows))}
	for i, r := range rows {
		want.OrderLog[i] = BundleOrder{Round: r.Round, Seat: r.Seat, Payload: json.RawMessage(r.Payload)}
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exportBundleFromDB disagrees with a bundle independently assembled from the same match's rows")
	}
}

// TestIntegrationReplayByteIdenticalAcrossRunsRealMatch is #322's own acceptance
// criterion applied to the --match path itself: "folds from the database
// and prints a deterministic dump; two runs produce byte-identical output."
func TestIntegrationReplayByteIdenticalAcrossRunsRealMatch(t *testing.T) {
	dsn, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)

	var out1, out2, stderr bytes.Buffer
	if code := run([]string{"--db", dsn, "--match", string(matchID)}, &out1, &stderr); code != 0 {
		t.Fatalf("run() #1 = %d, stderr = %s", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"--db", dsn, "--match", string(matchID)}, &out2, &stderr); code != 0 {
		t.Fatalf("run() #2 = %d, stderr = %s", code, stderr.String())
	}

	if !bytes.Equal(out1.Bytes(), out2.Bytes()) {
		t.Fatal("two runs against the same database match produced different bytes")
	}

	// Fails closed the same way TestRunBundleByteIdenticalAcrossRuns does:
	// non-empty bytes alone would also be true of a vacuous MatchState{}.
	var state rules.MatchState
	if err := json.Unmarshal(out1.Bytes(), &state); err != nil {
		t.Fatalf("decode dump: %v", err)
	}
	cfg, _, _, _ := testFixture()
	if state.Round != game.RoundNumber(cfg.Rounds) {
		t.Fatalf("dumped Round = %d, want %d (cfg.Rounds) — a vacuous fold would report 0", state.Round, cfg.Rounds)
	}
	if len(state.Players) != 2 || state.Players[0].LoiteringStreak != 11 {
		t.Fatalf("dumped seat 0 LoiteringStreak = %+v, want 11 — the same known, hand-verified fact TestRunBundleByteIdenticalAcrossRuns checks against testFixture's log", state.Players)
	}
}

// TestIntegrationReplayRoundBeyondLastRoundOnRealMatch is #322's own acceptance
// criterion against the --match path: "--round N beyond the match's last
// round is an error naming the last round, not a silently clamped dump."
func TestIntegrationReplayRoundBeyondLastRoundOnRealMatch(t *testing.T) {
	dsn, s := openReplayStore(t)
	matchID := seedFullReplayMatch(t, s)
	cfg, _, _, _ := testFixture()

	var stdout, stderr bytes.Buffer
	code := run([]string{"--db", dsn, "--match", string(matchID), "--round", strconv.Itoa(cfg.Rounds + 1)}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("run() with --round beyond the match's last round succeeded, want an error")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty on error: %s", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte(strconv.Itoa(cfg.Rounds))) {
		t.Errorf("stderr = %q, want it to name the match's actual last round (%d)", stderr.String(), cfg.Rounds)
	}
}
