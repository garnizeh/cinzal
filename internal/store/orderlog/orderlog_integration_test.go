//go:build integration

package orderlog

import (
	"context"
	mathrand "math/rand/v2"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// This file is issue #317's real-Postgres acceptance criteria for the
// AppendOrder -> Load round trip and the round-gap rejection.
// internal/store's own orders_integration_test.go covers AppendOrder's
// resubmission-upsert and human-flips-source behavior directly against
// the orders table; internal/store/orderlog cannot be imported from a
// package-store test file (that would be an import cycle, since this
// package itself imports internal/store), so anything needing Load lives
// here instead. orderlog_test.go (no build tag) covers the pure
// decode/grouping/gap logic with no database at all.
//
// postgresImage is the same pinned digest D46 (#309) already decided for
// the persistence layer's test suite, duplicated rather than shared:
// migrate_integration_test.go's own comment states this is deliberate,
// ad hoc, package-local scaffolding until #325's storetest package lands
// and "these helpers... are expected to be lifted into storetest largely
// unchanged once #325 lands" — this package's copy is written to be
// trivially replaceable then, not to invent its own convention now.
const postgresImage = "postgres@sha256:4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280"

// startPostgres starts one pinned-digest Postgres container for a single
// test and returns a DSN naming an explicit host.
func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase("cinzal_test"),
		postgres.WithUsername("cinzal"),
		postgres.WithPassword("cinzal"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	testcontainers.CleanupContainer(t, ctr)

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}
	return dsn
}

// openOrderLogStore starts a fresh container, applies the real production
// migration set via the exported store.Migrate (migrate.go), and opens a
// *store.Store against the same DSN — the production path, not a
// low-level *sql.DB the way internal/store's own migrate_integration_test.go
// exercises Migrate's internals.
func openOrderLogStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	dsn := startPostgres(t)

	if err := store.Migrate(ctx, dsn); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	s, err := store.Open(ctx, store.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

// seedMatchWithSeats inserts one user (as the match's creator), one
// matches row, and one match_players row per seat in [0, numSeats) —
// orders' composite FK (match_id, seat) -> match_players(match_id, seat)
// requires every seat an order names to actually exist as a participant.
// It talks to the database directly (there is no exported store.Store
// method for user creation yet — auth is M5) via the sqlc-generated
// queries this package already depends on.
func seedMatchWithSeats(t *testing.T, s *store.Store, q *store.Queries, numSeats int) game.MatchID {
	t.Helper()
	ctx := context.Background()

	var userID pgtype.UUID
	if err := s.Pool().QueryRow(ctx,
		`INSERT INTO users (display_name) VALUES ('seed') RETURNING id`,
	).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

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

// randomIntegrationOrder is a small, self-contained generator exercising
// the same corners internal/game's own arbitrary-Order property test does
// (nil vs. non-nil-empty Route, every enum's reserved zero vs. a real
// constant, a non-nil PushingOn.Bias) — this package cannot reach
// internal/game's unexported test-only generator across a package
// boundary.
func randomIntegrationOrder(gen *mathrand.Rand, round game.RoundNumber) game.Order {
	o := game.Order{Round: round}

	n := gen.IntN(4)
	if n > 0 {
		o.Route = make([]game.NodeID, n)
		for i := range o.Route {
			o.Route[i] = game.NodeID(gen.IntN(100))
		}
	} else if gen.IntN(2) == 0 {
		o.Route = []game.NodeID{}
	}

	if gen.IntN(2) == 0 {
		bias := []game.Sector{game.SectorOldDocks, game.SectorIronLow, game.SectorMistHeights, game.SectorNorthVale}[gen.IntN(4)]
		o.PushingOn = game.PushingOn{Steps: gen.IntN(3), Bias: &bias}
	}

	kinds := []game.ActionKind{0, game.ActionPickup, game.ActionDeliver, game.ActionDeal, game.ActionNothing}
	o.Action.Kind = kinds[gen.IntN(len(kinds))]
	if o.Action.Kind == game.ActionDeal {
		o.Action.Item = game.ItemShiv
	}

	stances := []game.Stance{0, game.StanceAggressive, game.StanceNeutral, game.StanceEvasive}
	o.Stance.Stance = stances[gen.IntN(len(stances))]

	o.AbandonCargo = gen.IntN(2) == 0
	return o
}

// TestAppendOrderThenLoadRoundTripArbitrary is D44 §3's required property,
// run through the real store: append arbitrary orders across several
// rounds and seats, read the whole log back through Load, and assert deep
// equality — issue #317's acceptance criteria call this "asserted against
// real Postgres."
func TestAppendOrderThenLoadRoundTripArbitrary(t *testing.T) {
	s := openOrderLogStore(t)
	q := store.New(s.Pool())
	ctx := context.Background()

	const numSeats = 3
	const numRounds = 5
	matchID := seedMatchWithSeats(t, s, q, numSeats)

	gen := mathrand.New(mathrand.NewPCG(7, 11))
	want := make(map[game.RoundNumber]map[game.SeatID]game.Order)

	for round := game.RoundNumber(1); round <= numRounds; round++ {
		want[round] = make(map[game.SeatID]game.Order)
		for seat := 0; seat < numSeats; seat++ {
			o := randomIntegrationOrder(gen, round)
			want[round][game.SeatID(seat)] = o
			if err := s.AppendOrder(ctx, matchID, round, game.SeatID(seat), o, store.SourceHuman); err != nil {
				t.Fatalf("AppendOrder(round %d, seat %d): %v", round, seat, err)
			}
		}
	}

	got, err := Load(ctx, s.Pool(), matchID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got) != numRounds {
		t.Fatalf("len(got) = %d, want %d rounds", len(got), numRounds)
	}
	for round, seats := range want {
		for seat, wantOrder := range seats {
			gotOrder, ok := got[round][seat]
			if !ok {
				t.Fatalf("round %d seat %d missing from loaded log", round, seat)
			}
			if !gotOrder.Equal(wantOrder) {
				t.Fatalf("round %d seat %d: got %+v, want %+v", round, seat, gotOrder, wantOrder)
			}
		}
	}
}

// TestLoadRejectsRoundGapAgainstRealPostgres is issue #317's own
// acceptance criterion, exercised end to end: a match whose orders skip a
// round (round 2 has no rows at all, round 3 does) must fail Load rather
// than fold short silently.
func TestLoadRejectsRoundGapAgainstRealPostgres(t *testing.T) {
	s := openOrderLogStore(t)
	q := store.New(s.Pool())
	ctx := context.Background()
	matchID := seedMatchWithSeats(t, s, q, 1)

	if err := s.AppendOrder(ctx, matchID, 1, 0, game.Order{Round: 1}, store.SourceHuman); err != nil {
		t.Fatalf("AppendOrder round 1: %v", err)
	}
	// round 2 deliberately skipped
	if err := s.AppendOrder(ctx, matchID, 3, 0, game.Order{Round: 3}, store.SourceHuman); err != nil {
		t.Fatalf("AppendOrder round 3: %v", err)
	}

	if _, err := Load(ctx, s.Pool(), matchID); err == nil {
		t.Fatal("Load with round 2 missing returned nil error, want a gap error")
	}
}

// TestLoadFreshMatchIsEmptyNoError asserts a match with no orders yet (a
// fresh lobby) is not itself a gap.
func TestLoadFreshMatchIsEmptyNoError(t *testing.T) {
	s := openOrderLogStore(t)
	q := store.New(s.Pool())
	ctx := context.Background()
	matchID := seedMatchWithSeats(t, s, q, 1)

	log, err := Load(ctx, s.Pool(), matchID)
	if err != nil {
		t.Fatalf("Load on a fresh match: %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("len(log) = %d, want 0 for a match with no orders yet", len(log))
	}
}
