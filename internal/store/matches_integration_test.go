//go:build integration

package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	mathrand "math/rand/v2"
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/store"
	"github.com/garnizeh/cinzal/internal/store/storetest"
	"github.com/jackc/pgx/v5/pgtype"
)

// This file is issue #318's real-Postgres acceptance criteria for
// CreateMatch/LoadMatch: the one-transaction write, the rollback on
// failure, the seed/config round trip, D44's missing-field policy exercised
// against a real row, and the seat-gap-on-reload rejection. matches_test.go
// covers everything that doesn't need a live connection.
//
// Every test below gets its *store.Store from storetest.Container (#325,
// D46) — one documented entry point, a transaction against the shared work
// database rolled back in t.Cleanup, rather than this file starting its own
// container.

// seedUser inserts one users row and returns its id — CreateMatch's
// createdBy and, optionally, a seat's UserID both need a real users.id to
// satisfy matches.created_by/match_players.user_id's own FK.
func seedUser(t *testing.T, s *store.Store) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := s.Pool().QueryRow(context.Background(),
		`INSERT INTO users (display_name) VALUES ('seed') RETURNING id`,
	).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func mustSeats(n int) []store.SeatSpec {
	seats := make([]store.SeatSpec, n)
	for i := range seats {
		seats[i] = store.SeatSpec{Faction: "test"}
	}
	return seats
}

// TestIntegrationCreateMatchWritesMatchAndEverySeatRow is #318's own
// acceptance criterion: "CreateMatch writes the match row and every
// match_players row in one transaction."
func TestIntegrationCreateMatchWritesMatchAndEverySeatRow(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)

	matchID, _, err := s.CreateMatch(ctx, [32]byte{1, 2, 3}, game.DefaultConfig(), mustSeats(4), createdBy, nil, nil)
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}

	var matchCount int
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM matches WHERE id = $1`, matchID).Scan(&matchCount); err != nil {
		t.Fatalf("count matches: %v", err)
	}
	if matchCount != 1 {
		t.Fatalf("matches row count = %d, want 1", matchCount)
	}

	var seatCount int
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM match_players WHERE match_id = $1`, matchID).Scan(&seatCount); err != nil {
		t.Fatalf("count match_players: %v", err)
	}
	if seatCount != 4 {
		t.Fatalf("match_players row count = %d, want 4", seatCount)
	}
}

// TestIntegrationCreateMatchReturnsTokensMatchingStoredHashes is the
// CodeRabbit finding on PR #394: newUnsubscribeTokenHash minted a hash from
// bytes nobody kept, so no unsubscribe link CreateMatch's caller could
// construct would ever match what got stored. This asserts the actual fix
// — CreateMatch now returns the raw per-seat tokens, one per seat, in seat
// order — round-trips against a real row: sha256(returned raw token) must
// equal the seat's stored unsubscribe_token_hash, for every seat, per D17's
// "raw-token handoff at creation" (the only moment the raw value exists is
// right here).
func TestIntegrationCreateMatchReturnsTokensMatchingStoredHashes(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)

	matchID, tokens, err := s.CreateMatch(ctx, [32]byte{7}, game.DefaultConfig(), mustSeats(3), createdBy, nil, nil)
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("len(tokens) = %d, want 3 (one per seat)", len(tokens))
	}

	// ListMatchPlayers' own query doesn't select unsubscribe_token_hash (not
	// among match_players.sql's needs elsewhere), so this reads the raw
	// column directly — the same shape seedUser already uses for a query
	// this package has no generated method for.
	rows, err := s.Pool().Query(ctx,
		`SELECT seat, unsubscribe_token_hash FROM match_players WHERE match_id = $1 ORDER BY seat`, matchID)
	if err != nil {
		t.Fatalf("query unsubscribe_token_hash: %v", err)
	}
	defer rows.Close()

	seen := make(map[[32]byte]bool)
	count := 0
	for rows.Next() {
		var seat game.SeatID
		var hash []byte
		if err := rows.Scan(&seat, &hash); err != nil {
			t.Fatalf("scan: %v", err)
		}
		i := int(seat)
		if seen[tokens[i]] {
			t.Fatalf("seat %d: token %x duplicates an earlier seat's token", i, tokens[i])
		}
		seen[tokens[i]] = true

		sum := sha256.Sum256(tokens[i][:])
		if string(sum[:]) != string(hash) {
			t.Fatalf("seat %d: sha256(returned token) = %x, want stored unsubscribe_token_hash %x", i, sum, hash)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if count != 3 {
		t.Fatalf("scanned %d match_players rows, want 3", count)
	}
}

// TestIntegrationCreateMatchFailurePartWayLeavesNoMatch is the other half of
// the same acceptance criterion: a failure partway through the seat loop
// must leave no match row at all, not a match with fewer seats than it
// claims. The failure is forced with a UserID naming a users row that does
// not exist — match_players.user_id's own FK (migration 00001) rejects it,
// and that rejection has to happen after the matches row insert already
// succeeded inside the same transaction for this test to actually exercise
// the rollback rather than a pre-flight check.
func TestIntegrationCreateMatchFailurePartWayLeavesNoMatch(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)

	ghostUser := pgtype.UUID{Bytes: [16]byte{0xff}, Valid: true}
	seats := []store.SeatSpec{
		{Faction: "ok"},
		{Faction: "bad", UserID: &ghostUser},
	}

	_, _, err := s.CreateMatch(ctx, [32]byte{9}, game.DefaultConfig(), seats, createdBy, nil, nil)
	if err == nil {
		t.Fatal("CreateMatch with a seat naming a nonexistent user returned nil error, want the FK violation")
	}

	var matchCount int
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM matches`).Scan(&matchCount); err != nil {
		t.Fatalf("count matches: %v", err)
	}
	if matchCount != 0 {
		t.Fatalf("matches row count after a failed CreateMatch = %d, want 0 (the whole transaction must roll back)", matchCount)
	}
}

// TestIntegrationCreateMatchThenLoadMatchSeedRoundTripsArbitrary is #318's
// own property test: "a property test over arbitrary 32-byte seeds asserts
// byte equality on reload." Iteration count is smaller than the pure
// game-package property tests (5000, e.g. order_codec_test.go) because
// every iteration here is a real transaction against a live Postgres
// container, not an in-memory json.Marshal/Unmarshal round trip.
func TestIntegrationCreateMatchThenLoadMatchSeedRoundTripsArbitrary(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)
	gen := mathrand.New(mathrand.NewPCG(7, 11))

	const n = 25
	for i := 0; i < n; i++ {
		var want [32]byte
		for j := range want {
			want[j] = byte(gen.IntN(256))
		}

		matchID, _, err := s.CreateMatch(ctx, want, game.DefaultConfig(), mustSeats(2), createdBy, nil, nil)
		if err != nil {
			t.Fatalf("iteration %d: CreateMatch: %v", i, err)
		}

		got, _, _, err := s.LoadMatch(ctx, matchID)
		if err != nil {
			t.Fatalf("iteration %d: LoadMatch: %v", i, err)
		}
		if got != want {
			t.Fatalf("iteration %d: seed round trip not equal.\n want: %x\n got:  %x", i, want, got)
		}
	}
}

// TestIntegrationCreateMatchThenLoadMatchConfigRoundTripsExactly is #318's
// config half of the same property: "The config round-trips to a value
// that is reflect.DeepEqual to the input" — checked against a config that
// is neither the zero value nor DefaultConfig(), per the fails-closed
// acceptance criterion.
func TestIntegrationCreateMatchThenLoadMatchConfigRoundTripsExactly(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)

	want := game.DefaultConfig()
	want.Rounds = 12
	want.LeaseCostPerBlock = 77
	want.Contracts[2].Deadline = 9
	want.Suppress.Items = true

	if reflect.DeepEqual(want, game.Config{}) {
		t.Fatal("fails closed: want must not be the zero value")
	}
	if reflect.DeepEqual(want, game.DefaultConfig()) {
		t.Fatal("fails closed: want must differ from DefaultConfig() in at least one field")
	}

	matchID, _, err := s.CreateMatch(ctx, [32]byte{4}, want, mustSeats(3), createdBy, nil, nil)
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}

	_, got, _, err := s.LoadMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("LoadMatch: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("config round trip not equal.\n want: %+v\n got:  %+v", want, got)
	}
}

// TestIntegrationLoadMatchReturnsMetaAndValidatesAgainstSeatCount confirms
// LoadMatch's own acceptance criterion: "LoadMatch calls cfg.Validate(players)
// and returns its error rather than handing back a partially-valid config" —
// exercised on the success path here (5 players is out of DefaultConfig()'s
// supported range {2,3,4,5}... actually within range) alongside the meta
// fields a caller needs.
func TestIntegrationLoadMatchReturnsMetaAndValidatesAgainstSeatCount(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)

	timer := int32(30)
	matchID, _, err := s.CreateMatch(ctx, [32]byte{5}, game.DefaultConfig(), mustSeats(5), createdBy, &timer, nil)
	if err != nil {
		t.Fatalf("CreateMatch: %v", err)
	}

	_, _, meta, err := s.LoadMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("LoadMatch: %v", err)
	}
	if len(meta.Seats) != 5 {
		t.Fatalf("len(meta.Seats) = %d, want 5", len(meta.Seats))
	}
	if meta.Status != "lobby" {
		t.Fatalf("meta.Status = %q, want %q (matches' own DDL default)", meta.Status, "lobby")
	}
	if meta.TimerSeconds == nil || *meta.TimerSeconds != 30 {
		t.Fatalf("meta.TimerSeconds = %v, want *30", meta.TimerSeconds)
	}
	if meta.CreatedBy != createdBy {
		t.Fatalf("meta.CreatedBy = %v, want %v", meta.CreatedBy, createdBy)
	}
}

// TestIntegrationLoadMatchRejectsConfigMissingField is D44's own required
// exercise: "D44's missing-field policy is exercised with a real row: a
// config JSONB written without a field the current game.Config declares
// behaves exactly as D44 specified — including the failure case if D44
// chose to reject." The row is written directly, bypassing EncodeConfig
// entirely, since the point is to prove LoadMatch itself refuses a
// malformed row regardless of how it got there — a corrupted write, a
// hand-run migration, or a future bug in EncodeConfig.
func TestIntegrationLoadMatchRejectsConfigMissingField(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)

	full, err := store.EncodeConfig(game.DefaultConfig())
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(full, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(env["config"], &top); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	delete(top, "lease_cost_per_block") // D44's own named example field
	configRaw, err := json.Marshal(top)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	env["config"] = configRaw
	broken, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	var matchID game.MatchID
	if err := s.Pool().QueryRow(ctx,
		`INSERT INTO matches (config, seed, created_by) VALUES ($1, $2, $3) RETURNING id`,
		broken, make([]byte, 32), createdBy,
	).Scan(&matchID); err != nil {
		t.Fatalf("insert broken match row: %v", err)
	}
	for seat := 0; seat < 2; seat++ {
		if _, err := store.New(s.Pool()).CreateMatchPlayer(ctx, store.CreateMatchPlayerParams{
			MatchID:              matchID,
			Seat:                 game.SeatID(seat),
			Faction:              "test",
			UnsubscribeTokenHash: []byte{0},
		}); err != nil {
			t.Fatalf("insert seat %d: %v", seat, err)
		}
	}

	if _, _, _, err := s.LoadMatch(ctx, matchID); err == nil {
		t.Fatal("LoadMatch against a config row missing lease_cost_per_block returned nil error, want a rejection")
	}
}

// TestIntegrationLoadMatchRejectsSeatGap writes match_players rows for
// seats 0 and 2, skipping 1, directly — again bypassing CreateMatch, which
// would never itself produce a gap, to prove LoadMatch refuses a corrupted
// roster regardless of how it arose. This is #318's "the players argument
// to rules.NewMatch and the match_players row count are checked to agree
// on reload; a disagreement is an error, not a fold," exercised against a
// real row rather than only the pure seatsFromRows unit test.
func TestIntegrationLoadMatchRejectsSeatGap(t *testing.T) {
	s := storetest.Container(t)
	ctx := context.Background()
	createdBy := seedUser(t, s)

	encoded, err := store.EncodeConfig(game.DefaultConfig())
	if err != nil {
		t.Fatalf("EncodeConfig: %v", err)
	}
	var matchID game.MatchID
	if err := s.Pool().QueryRow(ctx,
		`INSERT INTO matches (config, seed, created_by) VALUES ($1, $2, $3) RETURNING id`,
		encoded, make([]byte, 32), createdBy,
	).Scan(&matchID); err != nil {
		t.Fatalf("insert match row: %v", err)
	}
	for _, seat := range []game.SeatID{0, 2} {
		if _, err := store.New(s.Pool()).CreateMatchPlayer(ctx, store.CreateMatchPlayerParams{
			MatchID:              matchID,
			Seat:                 seat,
			Faction:              "test",
			UnsubscribeTokenHash: []byte{0},
		}); err != nil {
			t.Fatalf("insert seat %d: %v", seat, err)
		}
	}

	if _, _, _, err := s.LoadMatch(ctx, matchID); err == nil {
		t.Fatal("LoadMatch against a seat roster with a gap (0, 2, missing 1) returned nil error, want a rejection")
	}
}
