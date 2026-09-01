package fold

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/opsmetrics"
	"github.com/garnizeh/cinzal/internal/rules"
)

// idleFullLog builds a complete, valid cfg.Rounds-round order log where
// every seat submits ActionNothing/StanceNeutral every round — the same
// "everybody idles" shape golden_test.go's own idleOrder uses, sufficient to
// drive a real Fold from round 1 through cfg.Rounds without needing any
// particular map topology or contract state.
func idleFullLog(cfg game.Config, players int) rules.OrderLog {
	idleOrder := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}
	log := make(rules.OrderLog, cfg.Rounds)
	for round := 1; round <= cfg.Rounds; round++ {
		orders := make(map[game.SeatID]game.Order, players)
		for seat := game.SeatID(0); int(seat) < players; seat++ {
			orders[seat] = idleOrder
		}
		log[game.RoundNumber(round)] = orders
	}
	return log
}

// withIsolatedDefault swaps opsmetrics.Default for a fresh, empty instance
// for the duration of one test, restoring the previous one on cleanup. Every
// test in this file that calls FoldMeasured uses this rather than reading
// opsmetrics.Default directly — the package-level singleton is shared with
// every other test in this binary (and, under -bench, with
// fold_bench_test.go's own benchmarks), and this test suite needs to assert
// exact facts about "the one sample this call just recorded," which only an
// isolated aggregator can give without depending on execution order.
func withIsolatedDefault(t *testing.T) *opsmetrics.FoldStats {
	t.Helper()
	scoped := opsmetrics.NewFoldStats()
	restore := opsmetrics.SetDefault(scoped)
	t.Cleanup(restore)
	return scoped
}

// TestFoldMeasuredReturnsSameResultAsFold is the wrapper-not-reimplementation
// property FoldMeasured's own doc comment claims: its state and events must
// be identical to a bare Fold call over the same inputs, compared the same
// way this package's other tests do (JSON serialization, per RFC §16.2).
func TestFoldMeasuredReturnsSameResultAsFold(t *testing.T) {
	withIsolatedDefault(t)

	cfg := game.DefaultConfig()
	seed := [32]byte{7}
	players := 2
	log := idleFullLog(cfg, players)

	wantState, wantEvents, err := Fold(seed, cfg, players, log)
	if err != nil {
		t.Fatalf("Fold() = %v", err)
	}

	gotState, gotEvents, err := FoldMeasured(seed, cfg, players, log)
	if err != nil {
		t.Fatalf("FoldMeasured() = %v", err)
	}

	wantStateJSON, _ := json.Marshal(wantState)
	gotStateJSON, _ := json.Marshal(gotState)
	if string(wantStateJSON) != string(gotStateJSON) {
		t.Error("FoldMeasured's returned state differs from a bare Fold call over the same inputs")
	}

	wantEventsJSON, _ := json.Marshal(wantEvents)
	gotEventsJSON, _ := json.Marshal(gotEvents)
	if string(wantEventsJSON) != string(gotEventsJSON) {
		t.Error("FoldMeasured's returned events differ from a bare Fold call over the same inputs")
	}
}

// TestFoldMeasuredObservesOnSuccess is #320's fails-closed acceptance
// criterion for the wrapper itself: a successful FoldMeasured call must
// leave the aggregator with exactly one sample — a fold that emits nothing
// must fail this test, not pass it by producing an empty snapshot.
func TestFoldMeasuredObservesOnSuccess(t *testing.T) {
	scoped := withIsolatedDefault(t)

	cfg := game.DefaultConfig()
	seed := [32]byte{8}
	players := 2
	log := idleFullLog(cfg, players)

	if _, _, err := FoldMeasured(seed, cfg, players, log); err != nil {
		t.Fatalf("FoldMeasured() = %v", err)
	}

	snap := scoped.Snapshot()
	if snap.Count != 1 {
		t.Fatalf("Snapshot().Count = %d after one successful FoldMeasured call, want 1", snap.Count)
	}
	if snap.P99 == 0 {
		t.Error("Snapshot().P99 = 0 after a real fold — a fold that emits nothing must fail this test, not pass it with a zero duration")
	}
}

// TestFoldMeasuredNoObserveOnError is the other half of the fails-closed
// contract: a failed fold records nothing, per FoldMeasured's own doc
// comment — an error means there is no duration or allocation figure worth
// attributing to "one fold."
func TestFoldMeasuredNoObserveOnError(t *testing.T) {
	scoped := withIsolatedDefault(t)

	cfg := game.DefaultConfig()
	seed := [32]byte{9}
	players := 2

	// An empty log for a Rounds>0 config is not itself an error (Fold
	// returns initial() with no events) — force a real error instead: a log
	// with only round 1 present fails Fold's "order log missing round"
	// check on round 2.
	idleOrder := game.Order{
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}
	badLog := rules.OrderLog{
		1: {0: idleOrder, 1: idleOrder},
	}
	if _, _, err := FoldMeasured(seed, cfg, players, badLog); err == nil {
		t.Fatal("FoldMeasured() succeeded on a log missing every round after round 1, want an error")
	}

	snap := scoped.Snapshot()
	if snap.Count != 0 {
		t.Errorf("Snapshot().Count = %d after a FAILED FoldMeasured call, want 0 (nothing recorded)", snap.Count)
	}
}

// TestFoldMeasuredTimerCoversInitialAndResolve is #320's own acceptance
// criterion: "the fold timer covers initial() through the final Resolve,
// asserted by a test that folds a 15-round match and checks the recorded
// duration is at least the sum of a separately timed inner section — not
// merely that it is non-zero."
//
// The "separately timed inner section" here is the sum of 15 individually
// timed rules.Resolve calls replaying the same log, with rules.NewMatch's
// own cost deliberately excluded from that sum. If FoldMeasured's recorded
// duration only covered Resolve (the bug D45 explicitly warns against —
// "gives a number a third the size, still under the threshold, and wrong"),
// it would read as approximately equal to this inner sum; since the timer
// must also cover initial()'s own non-zero cost, the recorded duration has
// to be at least that much.
//
// A first version of this test compared a single sample of each side
// directly, with only NewMatch's own cost — for a 2-player match, the
// cheapest map-generation case in the game (GDD §6.1) — as margin. That
// margin was microseconds wide, comparable to ordinary run-to-run scheduling
// noise between two independently-timed sequences on a shared CI runner, and
// the test failed on an unrelated PR as a result (#423). Two changes fix
// that without weakening the property being tested:
//
//  1. Every quantity below is the minimum across `trials` independent
//     replays, not a single sample. Scheduling interference (GC pauses,
//     runner contention, cache effects) can only inflate an observed
//     duration, never deflate it below the code's true cost, so the minimum
//     across many trials converges toward that true cost — the same
//     noise-resistant technique Go's own benchmarking tools rely on.
//  2. The assertion requires the recorded duration to exceed the
//     Resolve-only sum by at least half of NewMatch's own (minimized) cost,
//     not merely by more than zero. A correct FoldMeasured clears this by a
//     wide margin (it pays ~all of initOnly on top of resolveOnly); the D45
//     regression clears it by ~none (its recorded duration is just
//     resolveOnly again) — so this stays a decisive, reliable check rather
//     than one two noisy minimums could tip either way by chance.
func TestFoldMeasuredTimerCoversInitialAndResolve(t *testing.T) {
	withIsolatedDefault(t)

	cfg := game.DefaultConfig()
	seed := [32]byte{10}
	players := 2
	log := idleFullLog(cfg, players)

	const trials = 40

	// Each trial takes both measurements back to back — the manual
	// init+resolve timing, then FoldMeasured's own — rather than running all
	// of one kind first and all of the other second. Interleaving keeps
	// environmental drift over the test's run (CPU frequency scaling, cache
	// warm-up, background load) from biasing one side's minimum more than
	// the other's; measuring in two separate blocks let exactly that kind of
	// drift produce an occasional false negative during verification of this
	// fix (the corrected test failing to flag a deliberately reintroduced
	// D45 regression).
	var initOnlyMin, resolveOnlyMin, recordedMin time.Duration
	for trial := range trials {
		initStart := time.Now()
		s, err := rules.NewMatch(seed, cfg, players)
		initDur := time.Since(initStart)
		if err != nil {
			t.Fatalf("NewMatch() = %v", err)
		}

		var resolveOnly time.Duration
		for round := 1; round <= cfg.Rounds; round++ {
			start := time.Now()
			next, _, err := rules.Resolve(s, log[game.RoundNumber(round)], cfg, rules.NewRNG(seed, round))
			resolveOnly += time.Since(start)
			if err != nil {
				t.Fatalf("Resolve() round %d: %v", round, err)
			}
			s = next
		}

		// recorded is FoldMeasured's own internally-recorded duration for
		// the identical {seed, cfg, players, log}, read from a fresh,
		// isolated aggregator so its Snapshot holds exactly this trial's one
		// sample — the same reasoning withIsolatedDefault's own doc comment
		// gives for why this suite never reads Default directly.
		trialStats := opsmetrics.NewFoldStats()
		restore := opsmetrics.SetDefault(trialStats)
		_, _, err = FoldMeasured(seed, cfg, players, log)
		restore()
		if err != nil {
			t.Fatalf("FoldMeasured() = %v", err)
		}
		snap := trialStats.Snapshot()
		if snap.Count != 1 {
			t.Fatalf("Snapshot().Count = %d, want 1", snap.Count)
		}
		recorded := snap.P99

		if trial == 0 || initDur < initOnlyMin {
			initOnlyMin = initDur
		}
		if trial == 0 || resolveOnly < resolveOnlyMin {
			resolveOnlyMin = resolveOnly
		}
		if trial == 0 || recorded < recordedMin {
			recordedMin = recorded
		}
	}

	margin := recordedMin - resolveOnlyMin
	wantMargin := initOnlyMin / 2
	if margin < wantMargin {
		t.Errorf("FoldMeasured recorded duration exceeds the Resolve-only sum by %v (recorded %v, resolve-only %v), want at least %v (half of NewMatch's own minimized %v cost) — the timer must cover initial() (NewMatch) as well as every Resolve call, not Resolve alone (D45)", margin, recordedMin, resolveOnlyMin, wantMargin, initOnlyMin)
	}
}
