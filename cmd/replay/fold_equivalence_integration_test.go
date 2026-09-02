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

// foldFixture is the committed golden fixture itself, literally
// {seed, config, orderLog} plus the expected final state — the issue's own
// acceptance criterion. It is decoded straight off disk by every ordinary
// test run; nothing at test time regenerates OrderLog from a bot's current
// decision logic, so a future change to bots.Operator's own heuristics can
// never silently redefine which historical match this file represents.
// generateFoldFixture (bots-driven) produces one, but only under -update.
type foldFixture struct {
	Seed     [32]byte
	Config   game.Config
	Players  int
	OrderLog rules.OrderLog
	Final    rules.MatchState
}

// updateGolden regenerates this file's own committed testdata fixtures
// (testdata/fold-equivalence-{2,4}p.json) instead of comparing against
// them: `go test -tags=integration ./cmd/replay/... -run
// TestIntegrationFoldEqualsIncrementalMatchesGoldenFixture -update`. This is
// the standard Go golden-file idiom, used here for the first time in this
// repository.
var updateGolden = flag.Bool("update", false, "regenerate cmd/replay's fold-equivalence golden fixtures")

// wantFixtureRounds pins this demonstration to a full 15-round match,
// independent of whatever fx.Config.Rounds a committed fixture happens to
// carry. Checking only len(fx.OrderLog) != fx.Config.Rounds proves a
// fixture is internally self-consistent, never that it is actually the
// full match the issue's own acceptance criterion demands — a fixture
// whose Config, OrderLog and Final were all edited down together (by hand,
// or by a future generator with a different Rounds default) would pass
// that check while no longer demonstrating anything close to "a full
// 15-round match." A CodeRabbit review finding on this PR caught this.
const wantFixtureRounds = 15

// runIncremental replays log through rules.NewMatch + rules.Resolve, round
// by round — the incremental path (RFC §7.1), applied to a log that already
// exists rather than one decided live. This is the only place in this file
// that calls rules.Resolve, and it takes no bot: whether log came from a
// live bots.For(bots.Operator) run (generateFoldFixture, -update only) or
// was decoded straight from a committed fixture file (the ordinary test
// path), this function treats it identically — a fixed sequence of orders
// to replay, not something to decide afresh. Generation and every later
// test run therefore compute Final through the exact same code path; the
// only thing that differs between them is where OrderLog came from.
//
// Fails closed: a log that is not exactly cfg.Rounds rounds long, in
// strict 1..cfg.Rounds order with every seat present, is a broken fixture,
// not a demonstration input — mirrors cmd/simulate/driver.go's own
// validateComplete.
func runIncremental(seed [32]byte, cfg game.Config, players int, log rules.OrderLog) rules.MatchState {
	if len(log) != cfg.Rounds {
		panic(fmt.Sprintf("runIncremental: order log holds %d round(s), want %d", len(log), cfg.Rounds))
	}

	s, err := rules.NewMatch(seed, cfg, players)
	if err != nil {
		panic(fmt.Sprintf("runIncremental: NewMatch: %v", err))
	}

	for round := 1; round <= cfg.Rounds; round++ {
		orders, ok := log[game.RoundNumber(round)]
		if !ok {
			panic(fmt.Sprintf("runIncremental: order log missing round %d", round))
		}
		if len(orders) != players {
			panic(fmt.Sprintf("runIncremental: round %d holds %d order(s), want %d", round, len(orders), players))
		}

		next, _, err := rules.Resolve(s, orders, cfg, rules.NewRNG(seed, round))
		if err != nil {
			panic(fmt.Sprintf("runIncremental: round %d: Resolve: %v", round, err))
		}
		s = next
	}

	if s.Round != game.RoundNumber(cfg.Rounds) {
		panic(fmt.Sprintf("runIncremental: match reached round %d, want %d", s.Round, cfg.Rounds))
	}
	return s
}

// generateFoldFixture drives a full players-seat, cfg.Rounds-round match
// forward one round at a time via bots.For(bots.Operator) — the same shape
// cmd/simulate/driver.go's own RunMatch uses (not imported directly: that
// function lives in a sibling, unimportable package main) — to produce the
// OrderLog half of a foldFixture. Called only under -update: Operator never
// runs during an ordinary test, only when a human deliberately regenerates
// the committed fixture and reviews the diff before committing it.
//
// Operator, not Drifter, is the bot here: this demonstration wants "a real
// match" per the issue's own wording, and Operator's cross-round planning
// exercises contracts, staking and confrontations that an idle or
// uniform-random log would rarely touch, which matters for the SeatArchive
// comparison below.
func generateFoldFixture(seed [32]byte, players int) foldFixture {
	cfg := game.DefaultConfig()
	bot := bots.For(bots.Operator)

	s, err := rules.NewMatch(seed, cfg, players)
	if err != nil {
		panic(fmt.Sprintf("generateFoldFixture: NewMatch: %v", err))
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
			panic(fmt.Sprintf("generateFoldFixture: round %d: Resolve: %v", round, err))
		}
		s = next
	}

	// Recomputed via runIncremental rather than trusting s directly, so
	// Final is always the output of the same function every later test run
	// calls too — a second, independent replay of the just-built log,
	// which also means generation itself cross-checks that the log it
	// produced is genuinely self-consistent before it is ever committed.
	final := runIncremental(seed, cfg, players, log)
	return foldFixture{Seed: seed, Config: cfg, Players: players, OrderLog: log, Final: final}
}

// writeFixture and readFixture are the golden file's own encode/decode —
// plain encoding/json on the whole foldFixture, not D44's versioned
// matches.config envelope: this file is test bookkeeping this package
// reads back into memory itself, never attacker-controlled data reaching
// the fold from an untrusted source, so it carries none of D44's strict
// decode discipline. game.RoundNumber and game.SeatID (rules.OrderLog's map
// keys) are plain int-kinded types, which encoding/json already renders as
// decimal-string object keys and restores exactly on decode.
func writeFixture(t *testing.T, path string, fx foldFixture) {
	t.Helper()
	data, err := json.MarshalIndent(fx, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write golden file %s: %v", path, err)
	}
}

func readFixture(t *testing.T, path string) foldFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %s: %v (run with -update to create it)", path, err)
	}
	var fx foldFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("decode golden file %s: %v", path, err)
	}
	return fx
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
// counts, must produce byte-identical MatchState. The committed golden
// fixture is decoded, never regenerated, so a future rule change is what
// moves the number here — not a change to the bot that only ever ran once,
// at -update time, to produce the fixture in the first place.
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
			if *updateGolden {
				writeFixture(t, tc.golden, generateFoldFixture(tc.seed, tc.players))
				t.Skipf("regenerated %s — re-run without -update to compare", tc.golden)
			}

			fx := readFixture(t, tc.golden)
			// Fails closed: a truncated or empty committed fixture proves
			// nothing (the issue's own acceptance criterion — "two empty
			// states are equal"), checked before anything below trusts it.
			// fx.Config.Rounds is checked against the fixed wantFixtureRounds
			// first — otherwise a fixture edited down to fewer rounds (Config,
			// OrderLog and Final all shrunk together) would pass the length
			// check below while no longer being the full match this
			// demonstration claims to be.
			if fx.Config.Rounds != wantFixtureRounds {
				t.Fatalf("%s: committed fixture's Config.Rounds = %d, want %d — this demonstration is defined over a full match, not whatever a fixture happens to carry", tc.name, fx.Config.Rounds, wantFixtureRounds)
			}
			if len(fx.OrderLog) != fx.Config.Rounds {
				t.Fatalf("%s: committed fixture's order log holds %d round(s), want %d", tc.name, len(fx.OrderLog), fx.Config.Rounds)
			}
			if fx.Players != tc.players {
				t.Fatalf("%s: committed fixture is for %d player(s), want %d", tc.name, fx.Players, tc.players)
			}

			incremental := runIncremental(fx.Seed, fx.Config, fx.Players, fx.OrderLog)
			incrementalJSON := mustMarshal(t, incremental)
			finalJSON := mustMarshal(t, fx.Final)
			if string(incrementalJSON) != string(finalJSON) {
				t.Fatalf("%s: replaying the committed order log no longer reproduces the committed final state — if this is an intended rule change, re-run with -update and review the diff before committing it", tc.name)
			}

			s := storetest.Container(t)
			matchID := persistFixture(t, s, fx.Seed, fx.Config, fx.Players, fx.OrderLog)
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
			for seat := 0; seat < fx.Players; seat++ {
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
//
// Reads the same committed 2-player fixture the positive test does, rather
// than generating its own — the corruption is meant to be the only thing
// distinguishing this run from that one.
func TestIntegrationFoldDivergesWhenOrderCorrupted(t *testing.T) {
	fx := readFixture(t, "testdata/fold-equivalence-2p.json")
	// Same fixed-round validation as the positive test above, so this test
	// fails closed on its own committed fixture even when run independently
	// (e.g. via -run TestIntegrationFoldDivergesWhenOrderCorrupted alone).
	if fx.Config.Rounds != wantFixtureRounds {
		t.Fatalf("committed fixture's Config.Rounds = %d, want %d — this demonstration is defined over a full match, not whatever a fixture happens to carry", fx.Config.Rounds, wantFixtureRounds)
	}
	if len(fx.OrderLog) != fx.Config.Rounds {
		t.Fatalf("committed fixture's order log holds %d round(s), want %d", len(fx.OrderLog), fx.Config.Rounds)
	}

	incremental := runIncremental(fx.Seed, fx.Config, fx.Players, fx.OrderLog)
	incrementalJSON := mustMarshal(t, incremental)

	s := storetest.Container(t)
	matchID := persistFixture(t, s, fx.Seed, fx.Config, fx.Players, fx.OrderLog)

	// Find one (round, seat) whose order actually carries a non-empty
	// Route — corrupting an idle order's unused fields could easily leave
	// the final state untouched and this test would pass for the wrong
	// reason. Operator-driven logs almost always have plenty of these, but
	// this is asserted rather than assumed.
	var corruptRound game.RoundNumber
	var corruptSeat game.SeatID
	var corruptOrder game.Order
	found := false
	for round := game.RoundNumber(1); int(round) <= fx.Config.Rounds && !found; round++ {
		for seat, o := range fx.OrderLog[round] {
			if len(o.Route) > 0 {
				corruptRound, corruptSeat, corruptOrder = round, seat, o
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("fixture has no order with a non-empty Route to corrupt — this test needs a different fixture")
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
	if incremental.Round != game.RoundNumber(fx.Config.Rounds) {
		t.Fatalf("incremental state Round = %d, want %d — fixture is not a real completed match", incremental.Round, fx.Config.Rounds)
	}
	if folded.Round != game.RoundNumber(fx.Config.Rounds) {
		t.Fatalf("folded state Round = %d, want %d — corrupted fold did not even complete", folded.Round, fx.Config.Rounds)
	}

	if string(foldedJSON) == string(incrementalJSON) {
		t.Fatal("folded state still matches the incremental state after corrupting one order's Route in the database — the comparison did not catch a genuine data-level divergence")
	}
}
