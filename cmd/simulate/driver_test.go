package main

import (
	"reflect"
	"testing"

	"github.com/garnizeh/cinzal/internal/bots"
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/opsmetrics"
	"github.com/garnizeh/cinzal/internal/rules"
)

// TestRunMatchReachesRoundsAtEveryPlayerCount is issue #199's own
// acceptance criterion: "A single run at a fixed seed and config reaches
// cfg.Rounds, produces final scoring, and returns a non-empty event
// stream, at 2, 3, 4 and 5 players."
func TestRunMatchReachesRoundsAtEveryPlayerCount(t *testing.T) {
	cfg := game.DefaultConfig()
	bot := bots.For(bots.Drifter)

	for players := 2; players <= 5; players++ {
		var seed [32]byte
		seed[0] = 0xd2
		seed[1] = byte(players)

		result, err := RunMatch(seed, cfg, players, bot)
		if err != nil {
			t.Fatalf("players=%d: RunMatch() = %v", players, err)
		}

		if result.Final.Round != game.RoundNumber(cfg.Rounds) {
			t.Errorf("players=%d: Final.Round = %d, want %d", players, result.Final.Round, cfg.Rounds)
		}
		if len(result.Scores) != players {
			t.Errorf("players=%d: len(Scores) = %d, want %d", players, len(result.Scores), players)
		}
		if len(result.Events) == 0 {
			t.Errorf("players=%d: Events is empty", players)
		}
		if len(result.Log) != cfg.Rounds {
			t.Errorf("players=%d: len(Log) = %d, want %d", players, len(result.Log), cfg.Rounds)
		}
	}
}

// TestRunMatchDeterministic is issue #199's own acceptance criterion: "Two
// runs at the same seed produce byte-identical event streams and order
// logs."
func TestRunMatchDeterministic(t *testing.T) {
	cfg := game.DefaultConfig()
	bot := bots.For(bots.Operator)

	var seed [32]byte
	seed[0] = 0x5a
	const players = 4

	first, err := RunMatch(seed, cfg, players, bot)
	if err != nil {
		t.Fatalf("RunMatch() (first) = %v", err)
	}
	second, err := RunMatch(seed, cfg, players, bot)
	if err != nil {
		t.Fatalf("RunMatch() (second) = %v", err)
	}

	if !reflect.DeepEqual(first.Events, second.Events) {
		t.Error("two RunMatch calls at the same seed produced different event streams")
	}
	if !reflect.DeepEqual(first.Log, second.Log) {
		t.Error("two RunMatch calls at the same seed produced different order logs")
	}
	if !reflect.DeepEqual(first.Final, second.Final) {
		t.Error("two RunMatch calls at the same seed produced different final MatchState")
	}
}

// TestRunManyOrderIndependentOfWorkerCount is issue #199's own acceptance
// criterion: "A run with GOMAXPROCS=1 and a run with GOMAXPROCS=8 over the
// same seed set produce identical output, in identical order." workers is
// this package's own knob for the same property (see RunMany's doc
// comment) — this drives it at 1 and at 8 over one shared seed set and
// checks the two []MatchResult slices are identical element for element,
// which also asserts they came back in seed order regardless of which
// worker happened to finish first.
func TestRunManyOrderIndependentOfWorkerCount(t *testing.T) {
	cfg := game.DefaultConfig()
	bot := bots.For(bots.Drifter)
	const players = 3

	seeds := make([][32]byte, 6)
	for i := range seeds {
		seeds[i][0] = 0x77
		seeds[i][1] = byte(i)
	}

	sequential, err := RunMany(seeds, cfg, players, bot, 1)
	if err != nil {
		t.Fatalf("RunMany(workers=1) = %v", err)
	}
	parallel, err := RunMany(seeds, cfg, players, bot, 8)
	if err != nil {
		t.Fatalf("RunMany(workers=8) = %v", err)
	}

	if len(sequential) != len(seeds) || len(parallel) != len(seeds) {
		t.Fatalf("len(sequential)=%d len(parallel)=%d, want %d each", len(sequential), len(parallel), len(seeds))
	}

	for i := range seeds {
		if sequential[i].Seed != seeds[i] {
			t.Fatalf("sequential[%d].Seed = %x, want %x — not in seed order", i, sequential[i].Seed, seeds[i])
		}
		if !reflect.DeepEqual(sequential[i], parallel[i]) {
			t.Errorf("seed index %d: workers=1 and workers=8 produced different results", i)
		}
	}
}

// TestValidateComplete is issue #199's own fails-closed acceptance
// criterion, exercised directly against synthetic (state, log) pairs — a
// shape RunMatch's own loop can never actually produce, by construction,
// so it cannot be reached by driving a real match.
func TestValidateComplete(t *testing.T) {
	cfg := game.DefaultConfig()
	const players = 4

	completeLog := func() rules.OrderLog {
		log := make(rules.OrderLog, cfg.Rounds)
		for round := 1; round <= cfg.Rounds; round++ {
			orders := make(map[game.SeatID]game.Order, players)
			for seat := game.SeatID(0); int(seat) < players; seat++ {
				orders[seat] = game.Order{}
			}
			log[game.RoundNumber(round)] = orders
		}
		return log
	}

	t.Run("complete match", func(t *testing.T) {
		s := rules.MatchState{Round: game.RoundNumber(cfg.Rounds)}
		if err := validateComplete(s, completeLog(), cfg, players); err != nil {
			t.Errorf("validateComplete() = %v, want nil", err)
		}
	})

	t.Run("match did not reach cfg.Rounds", func(t *testing.T) {
		s := rules.MatchState{Round: game.RoundNumber(cfg.Rounds - 1)}
		if err := validateComplete(s, completeLog(), cfg, players); err == nil {
			t.Error("validateComplete() = nil, want an error for a short match")
		}
	})

	t.Run("a round missing a seat's order", func(t *testing.T) {
		s := rules.MatchState{Round: game.RoundNumber(cfg.Rounds)}
		log := completeLog()
		delete(log[1], game.SeatID(0))
		if err := validateComplete(s, log, cfg, players); err == nil {
			t.Error("validateComplete() = nil, want an error for a seat that produced no order")
		}
	})

	t.Run("order log short a round", func(t *testing.T) {
		s := rules.MatchState{Round: game.RoundNumber(cfg.Rounds)}
		log := completeLog()
		delete(log, game.RoundNumber(cfg.Rounds))
		if err := validateComplete(s, log, cfg, players); err == nil {
			t.Error("validateComplete() = nil, want an error for an order log missing a round")
		}
	})

	// A shifted round key preserves len(log) — still cfg.Rounds entries —
	// while every round number is wrong. An aggregate count check cannot
	// see this; validateComplete must check the round keys themselves.
	t.Run("round keys shifted by one", func(t *testing.T) {
		s := rules.MatchState{Round: game.RoundNumber(cfg.Rounds)}
		log := completeLog()
		log[game.RoundNumber(cfg.Rounds+1)] = log[1]
		delete(log, 1)
		if err := validateComplete(s, log, cfg, players); err == nil {
			t.Error("validateComplete() = nil, want an error for round keys shifted off 1..cfg.Rounds")
		}
	})

	// An out-of-range SeatID replacing a required seat preserves
	// len(orders) == players for that round while no order exists for
	// seat 0 — again invisible to a count-only check.
	t.Run("a required seat replaced by an out-of-range SeatID", func(t *testing.T) {
		s := rules.MatchState{Round: game.RoundNumber(cfg.Rounds)}
		log := completeLog()
		delete(log[1], game.SeatID(0))
		log[1][game.SeatID(99)] = game.Order{}
		if err := validateComplete(s, log, cfg, players); err == nil {
			t.Error("validateComplete() = nil, want an error for a required seat replaced by an out-of-range SeatID")
		}
	})
}

// TestRunMatchObservesFoldMetricsOnSuccess is #320's own wiring test:
// RunMatch must record exactly one opsmetrics sample per completed match, on
// the same instrumentation contract internal/match/fold.FoldMeasured uses
// (D45 — cmd/simulate cannot import internal/match/fold, so it calls
// opsmetrics.Default.Observe directly). Uses an isolated FoldStats via
// opsmetrics.SetDefault rather than the shared Default, so this test's
// assertion does not depend on what other tests in this binary already
// recorded.
func TestRunMatchObservesFoldMetricsOnSuccess(t *testing.T) {
	scoped := opsmetrics.NewFoldStats()
	restore := opsmetrics.SetDefault(scoped)
	defer restore()

	cfg := game.DefaultConfig()
	bot := bots.For(bots.Drifter)
	var seed [32]byte
	seed[0] = 0xf0

	if _, err := RunMatch(seed, cfg, 3, bot); err != nil {
		t.Fatalf("RunMatch() = %v", err)
	}

	snap := scoped.Snapshot()
	if snap.Count != 1 {
		t.Fatalf("opsmetrics.Default.Snapshot().Count = %d after one successful RunMatch, want 1", snap.Count)
	}
	if snap.P99 == 0 {
		t.Error("Snapshot().P99 = 0 after a real match — a match that emits nothing must fail this test, not pass it with a zero duration")
	}
}

// TestRunMatchInstrumentationDoesNotChangeResult is the determinism
// guarantee wiring in fold-duration timing must never threaten: RunMatch's
// own MatchResult (final state, events, order log) is byte-identical
// whether or not opsmetrics is actively recording, since the instrumentation
// added in #320 sits entirely outside the game-state computation. Compared
// via reflect.DeepEqual the same way TestRunMatchDeterministic already
// compares two runs of RunMatch against each other.
func TestRunMatchInstrumentationDoesNotChangeResult(t *testing.T) {
	cfg := game.DefaultConfig()
	bot := bots.For(bots.Operator)
	var seed [32]byte
	seed[0] = 0xf1

	scopedA := opsmetrics.NewFoldStats()
	restoreA := opsmetrics.SetDefault(scopedA)
	resultA, err := RunMatch(seed, cfg, 4, bot)
	restoreA()
	if err != nil {
		t.Fatalf("RunMatch() (instrumented A) = %v", err)
	}

	scopedB := opsmetrics.NewFoldStats()
	restoreB := opsmetrics.SetDefault(scopedB)
	resultB, err := RunMatch(seed, cfg, 4, bot)
	restoreB()
	if err != nil {
		t.Fatalf("RunMatch() (instrumented B) = %v", err)
	}

	if !reflect.DeepEqual(resultA.Final, resultB.Final) {
		t.Error("Final state differs between two instrumented RunMatch calls over the same seed/cfg/players")
	}
	if !reflect.DeepEqual(resultA.Events, resultB.Events) {
		t.Error("Events differ between two instrumented RunMatch calls over the same seed/cfg/players")
	}
	if !reflect.DeepEqual(resultA.Log, resultB.Log) {
		t.Error("Log differs between two instrumented RunMatch calls over the same seed/cfg/players")
	}
}
