//go:build integration

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/garnizeh/cinzal/internal/bots"
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/match/fold"
	"github.com/garnizeh/cinzal/internal/rules"
	"github.com/garnizeh/cinzal/internal/store"
	"github.com/garnizeh/cinzal/internal/store/orderlog"
	"github.com/garnizeh/cinzal/internal/store/storetest"
	"github.com/jackc/pgx/v5/pgtype"
)

// This file is issue #328's own exit demonstration: RFC §7.1's
// state = fold(Resolve, initial(seed, cfg), orderLog) proved over a real
// match, a real database, and a golden fixture, rather than left as a
// formula. Two code paths reach the same MatchState here — the incremental
// one (NewMatch + Resolve per round, what cmd/simulate and M4's tick do)
// and the folded one (fold.Fold over a log read back with a real SELECT,
// what cmd/replay and any future rebuild do) — and the whole no-snapshot
// architecture (RFC §7.3) assumes they cannot diverge.
//
// updateGolden regenerates this file's own committed testdata fixtures
// (testdata/fold-equivalence-{2,4}p.json) instead of comparing against
// them: `go test -tags=integration ./cmd/replay/... -run
// TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture -update`. This is
// the standard Go golden-file idiom, used here for the first time in this
// repository — the committed file is what lets a re-run after a future rule
// change notice the number moved, which regenerating the order log from its
// own seed every run would not: the log's bot-decision sequence is
// deterministic and would still reproduce itself even if Resolve's rules
// changed underneath it, so only a genuinely separate, previously-recorded
// value can catch that.
var updateGolden = flag.Bool("update", false, "regenerate cmd/replay's fold-equivalence golden fixtures")

// buildIncrementalFixture drives a full players-seat, cfg.Rounds-round match
// forward one round at a time — rules.NewMatch, then rules.ProjectView +
// bots.For(bots.Operator).Decide + rules.Resolve per round — the same shape
// cmd/simulate/driver.go's own RunMatch uses (not imported directly: that
// function lives in a sibling, unimportable package main). Operator, not
// Drifter, is the bot here: this demonstration wants "a real match" per the
// issue's own wording, and Operator's cross-round planning exercises
// contracts, staking and confrontations that an idle or uniform-random log
// would rarely touch, which matters for the SeatArchive comparison below.
//
// Fails closed inline, matching RunMatch's own validateComplete: a match
// that did not actually run cfg.Rounds rounds, or whose log is short a
// round, is a broken fixture, not a demonstration input.
func buildIncrementalFixture(seed [32]byte, players int) (game.Config, rules.OrderLog, rules.MatchState) {
	cfg := game.DefaultConfig()
	bot := bots.For(bots.Operator)

	s, err := rules.NewMatch(seed, cfg, players)
	if err != nil {
		panic(fmt.Sprintf("buildIncrementalFixture: NewMatch: %v", err))
	}

	log := make(rules.OrderLog, cfg.Rounds)
	for round := 1; round <= cfg.Rounds; round++ {
		orders := make(map[game.SeatID]game.Order, players)
		for seat := game.SeatID(0); int(seat) < players; seat++ {
			v := rules.ProjectView(s, seat, cfg)
			o := bot.Decide(v, cfg, rules.NewBotRNG(seed, seat, round))
			// v.Round (and so the Round a Bot naively stamps its own
			// order with, e.g. operator.go's draft) is s.Round — the
			// last round Resolve folded in, one behind the round this
			// order is actually for (game.PlayerView's own Round doc
			// comment; fog.go's Project sets it to s.Round). Persisting
			// under the wrong Round would trip orderlog.Decode's own
			// round-mismatch guard (a CodeRabbit finding on PR #393) the
			// moment this fixture round-trips through the database, so
			// it is corrected here to the round this order is actually
			// stored under — a real submission handler (M5) does the
			// same reconciliation before ever calling AppendOrder.
			o.Round = game.RoundNumber(round)
			orders[seat] = o
		}
		log[game.RoundNumber(round)] = orders

		next, _, err := rules.Resolve(s, orders, cfg, rules.NewRNG(seed, round))
		if err != nil {
			panic(fmt.Sprintf("buildIncrementalFixture: round %d: Resolve: %v", round, err))
		}
		s = next
	}

	if len(log) != cfg.Rounds {
		panic(fmt.Sprintf("buildIncrementalFixture: order log holds %d round(s), want %d", len(log), cfg.Rounds))
	}
	if s.Round != game.RoundNumber(cfg.Rounds) {
		panic(fmt.Sprintf("buildIncrementalFixture: match reached round %d, want %d", s.Round, cfg.Rounds))
	}

	return cfg, log, s
}

// persistFixture writes seed/cfg/log through the real production write path
// (Store.CreateMatch, then Store.AppendOrder per (round, seat)) against a
// real Postgres transaction, exactly the row shape the reload half of this
// test reads back — mirrors replay_integration_test.go's own
// seedFullReplayMatch, but with store.SourceBot: this log is bot-authored,
// not a human submission, and orders.source is a game fact (RFC §8.2's
// Autopilot derivation reads it), not bookkeeping this test may misstate.
func persistFixture(t *testing.T, s *store.Store, seed [32]byte, cfg game.Config, players int, log rules.OrderLog) game.MatchID {
	t.Helper()
	ctx := context.Background()

	var userID pgtype.UUID
	if err := s.Pool().QueryRow(ctx,
		`INSERT INTO users (display_name) VALUES ('fold-equivalence-fixture') RETURNING id`,
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
			if err := s.AppendOrder(ctx, matchID, round, seat, o, store.SourceBot); err != nil {
				t.Fatalf("AppendOrder (round %d, seat %d): %v", round, seat, err)
			}
		}
	}

	return matchID
}

// reloadAndFold is the fold half of the demonstration: a genuine SELECT
// (Store.LoadMatch + orderlog.Load), never the in-memory log
// persistFixture was handed — the issue's own acceptance criterion that
// "the fold's input comes from SELECT, not from the in-memory log the
// incremental path used."
func reloadAndFold(t *testing.T, s *store.Store, matchID game.MatchID) rules.MatchState {
	t.Helper()
	ctx := context.Background()

	seed, cfg, meta, err := s.LoadMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("LoadMatch: %v", err)
	}
	log, err := orderlog.Load(ctx, s.Pool(), matchID)
	if err != nil {
		t.Fatalf("orderlog.Load: %v", err)
	}

	folded, _, err := fold.Fold(seed, cfg, len(meta.Seats), log)
	if err != nil {
		t.Fatalf("fold.Fold: %v", err)
	}
	return folded
}

// mustMarshal is json.Marshal, failing the test on error rather than
// returning one — every value passed to it here is a rules.MatchState this
// same test built or read back, which is already known-encodable.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

// TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture is issue #328's
// positive half: a full 15-round match, driven two ways, at two player
// counts, must produce byte-identical MatchState — and the incrementally
// computed state must itself still match a committed golden fixture, so a
// future rule change that moves the number is caught even though the same
// seed always regenerates the identical order log.
func TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture(t *testing.T) {
	cases := []struct {
		name    string
		seed    [32]byte
		players int
		golden  string
	}{
		{name: "2p", seed: [32]byte{41}, players: 2, golden: "testdata/fold-equivalence-2p.json"},
		{name: "4p", seed: [32]byte{42}, players: 4, golden: "testdata/fold-equivalence-4p.json"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, log, incremental := buildIncrementalFixture(tc.seed, tc.players)
			incrementalJSON := mustMarshal(t, incremental)

			if *updateGolden {
				if err := os.WriteFile(tc.golden, append(incrementalJSON, '\n'), 0o644); err != nil {
					t.Fatalf("write golden file %s: %v", tc.golden, err)
				}
				t.Skipf("regenerated %s — re-run without -update to compare", tc.golden)
			}

			wantJSON, err := os.ReadFile(tc.golden)
			if err != nil {
				t.Fatalf("read golden file %s: %v (run with -update to create it)", tc.golden, err)
			}
			if string(incrementalJSON)+"\n" != string(wantJSON) {
				t.Fatalf("incrementally computed state for %s no longer matches the committed golden fixture %s — if this is an intended rule change, re-run with -update and review the diff before committing it", tc.name, tc.golden)
			}

			s := storetest.Container(t)
			matchID := persistFixture(t, s, tc.seed, cfg, tc.players, log)
			folded := reloadAndFold(t, s, matchID)
			foldedJSON := mustMarshal(t, folded)

			if string(foldedJSON) != string(incrementalJSON) {
				t.Fatalf("%s: folded state (from a real database SELECT) does not equal the incrementally computed state — RFC §7.1's fold equivalence does not hold", tc.name)
			}

			// Named sub-check for the issue's own "covers every seat's
			// private SeatArchive" requirement — already implied by the
			// whole-state comparison above, asserted explicitly here so
			// a reader (and this test's own failure message) can point at
			// SeatArchive by name rather than trust it silently.
			for seat := 0; seat < tc.players; seat++ {
				gotArchive := mustMarshal(t, folded.Players[seat].Archive)
				wantArchive := mustMarshal(t, incremental.Players[seat].Archive)
				if string(gotArchive) != string(wantArchive) {
					t.Fatalf("%s: seat %d's folded SeatArchive does not equal the incrementally computed one", tc.name, seat)
				}
			}
		})
	}
}

// TestIntegrationFoldDivergesWhenOrderCorrupted is issue #328's
// broken-on-purpose half, in the M2 tradition of #206: "a comparison that
// cannot fail is not a comparison." One order's payload is mutated in the
// database by a single field — a direct UPDATE, bypassing Store.AppendOrder
// entirely — and the fold/incremental comparison above must catch it. This
// is kept as a permanent regression test, not a revert-after probe: the
// corruption is a data-level mutation exercised inside the test itself, not
// a change to a CI gate's target that would break the tree for anyone else.
func TestIntegrationFoldDivergesWhenOrderCorrupted(t *testing.T) {
	seed := [32]byte{41}
	players := 2
	cfg, log, incremental := buildIncrementalFixture(seed, players)
	incrementalJSON := mustMarshal(t, incremental)

	s := storetest.Container(t)
	matchID := persistFixture(t, s, seed, cfg, players, log)

	// Find one (round, seat) whose order actually carries a non-empty
	// Route — corrupting an idle order's unused fields could easily leave
	// the final state untouched and this test would pass for the wrong
	// reason. Operator-driven logs almost always have plenty of these, but
	// this is asserted rather than assumed.
	var corruptRound game.RoundNumber
	var corruptSeat game.SeatID
	var corruptOrder game.Order
	found := false
	for round := game.RoundNumber(1); int(round) <= cfg.Rounds && !found; round++ {
		for seat, o := range log[round] {
			if len(o.Route) > 0 {
				corruptRound, corruptSeat, corruptOrder = round, seat, o
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("fixture has no order with a non-empty Route to corrupt — this test needs a different seed")
	}

	corrupted := corruptOrder
	corrupted.Route = corrupted.Route[:len(corrupted.Route)-1]
	payload, err := json.Marshal(corrupted)
	if err != nil {
		t.Fatalf("marshal corrupted order: %v", err)
	}

	ctx := context.Background()
	if _, err := s.Pool().Exec(ctx,
		`UPDATE orders SET payload = $1 WHERE match_id = $2 AND round = $3 AND seat = $4`,
		payload, matchID, corruptRound, corruptSeat,
	); err != nil {
		t.Fatalf("corrupt order (round %d, seat %d): %v", corruptRound, corruptSeat, err)
	}

	folded := reloadAndFold(t, s, matchID)
	foldedJSON := mustMarshal(t, folded)

	// Fails closed: both states must be genuine, completed matches before
	// "they differ" means anything — two vacuous states could differ on
	// nothing that matters, or (worse) two empty ones could read as equal
	// and this whole check would be void.
	if incremental.Round != game.RoundNumber(cfg.Rounds) {
		t.Fatalf("incremental state Round = %d, want %d — fixture is not a real completed match", incremental.Round, cfg.Rounds)
	}
	if folded.Round != game.RoundNumber(cfg.Rounds) {
		t.Fatalf("folded state Round = %d, want %d — corrupted fold did not even complete", folded.Round, cfg.Rounds)
	}

	if string(foldedJSON) == string(incrementalJSON) {
		t.Fatal("folded state still matches the incremental state after corrupting one order's Route in the database — the comparison did not catch a genuine data-level divergence")
	}
}
