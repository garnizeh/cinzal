package fold

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// idleOrderThrough is the same shape fold_test.go's own idleOrder fixtures
// use across this package's other tests: no route, no action, Neutral
// stance.
func idleOrderThrough(round game.RoundNumber) game.Order {
	return game.Order{
		Round:  round,
		Action: game.ActionOrder{Kind: game.ActionNothing},
		Stance: game.StanceOrder{Stance: game.StanceNeutral},
	}
}

func fullIdleLogThrough(cfg game.Config, players int) rules.OrderLog {
	log := rules.OrderLog{}
	for round := 1; round <= cfg.Rounds; round++ {
		orders := make(map[game.SeatID]game.Order, players)
		for seat := 0; seat < players; seat++ {
			orders[game.SeatID(seat)] = idleOrderThrough(game.RoundNumber(round))
		}
		log[game.RoundNumber(round)] = orders
	}
	return log
}

// TestFoldThroughMatchesFoldAtCfgRounds is issue #322's own consistency
// requirement for the function Fold now delegates to: folding through
// cfg.Rounds must produce byte-identical output to Fold itself, since Fold
// is defined as exactly that call.
func TestFoldThroughMatchesFoldAtCfgRounds(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{21}
	players := 3
	log := fullIdleLogThrough(cfg, players)

	wantState, wantEvents, wantErr := Fold(seed, cfg, players, log)
	gotState, gotEvents, gotErr := FoldThrough(seed, cfg, players, log, game.RoundNumber(cfg.Rounds))

	if (wantErr == nil) != (gotErr == nil) {
		t.Fatalf("Fold err = %v, FoldThrough(cfg.Rounds) err = %v", wantErr, gotErr)
	}

	wantJSON, err := json.Marshal(wantState)
	if err != nil {
		t.Fatalf("marshal Fold state: %v", err)
	}
	gotJSON, err := json.Marshal(gotState)
	if err != nil {
		t.Fatalf("marshal FoldThrough state: %v", err)
	}
	if string(wantJSON) != string(gotJSON) {
		t.Fatal("Fold(seed, cfg, players, log) and FoldThrough(seed, cfg, players, log, cfg.Rounds) produced different state")
	}

	if len(wantEvents) != len(gotEvents) {
		t.Fatalf("Fold produced %d events, FoldThrough(cfg.Rounds) produced %d", len(wantEvents), len(gotEvents))
	}
}

// TestFoldThroughIntermediateRoundDiffersFromTruncatedCfg is the
// load-bearing regression this function exists to prevent: folding through
// an intermediate round N < cfg.Rounds must NOT behave as though cfg.Rounds
// were N. incidents.go's nextUnstableSector — run at Phase 7 of every round
// with a live sector incident (round 3 onward) — returns nil once
// "int(s.Round) >= cfg.Rounds", i.e. "there is no further round to
// announce the Unstable Sector for." A caller that ran the fold with a
// shortened cfg (Rounds: N) instead of this function's separate throughRound
// parameter would trip that early, at round N, even though the real match
// keeps going past it.
//
// This proves the two approaches actually diverge at round 3 (the first
// round a sector incident is live, GDD §14.3): FoldThrough(realCfg, ...,
// throughRound=3), the production path, versus Fold(wrongCfg, ...) where
// wrongCfg.Rounds is shortened to 3, the naive approach #322's own doc
// comment on FoldThrough warns against. With the real, 15-round cfg,
// round 3 is not the match's last round, so UnstableSector must still be
// set (announcing round 4's flag); with cfg.Rounds wrongly truncated to 3,
// round 3 IS (falsely) the last round, so UnstableSector must go nil.
func TestFoldThroughIntermediateRoundDiffersFromTruncatedCfg(t *testing.T) {
	cfg := game.DefaultConfig()
	if cfg.Rounds < 4 {
		t.Fatal("fixture assumption broken: DefaultConfig().Rounds must be >= 4 for this test to mean anything")
	}
	seed := [32]byte{22}
	players := 2
	log := fullIdleLogThrough(cfg, players)
	through3 := rules.OrderLog{1: log[1], 2: log[2], 3: log[3]}

	rightState, _, err := FoldThrough(seed, cfg, players, log, 3)
	if err != nil {
		t.Fatalf("FoldThrough(realCfg, log, throughRound=3): %v", err)
	}

	wrongCfg := cfg
	wrongCfg.Rounds = 3
	wrongState, _, err := Fold(seed, wrongCfg, players, through3)
	if err != nil {
		t.Fatalf("Fold(wrongCfg (Rounds:3), through3): %v", err)
	}

	rightJSON, _ := json.Marshal(rightState)
	wrongJSON, _ := json.Marshal(wrongState)
	if string(rightJSON) == string(wrongJSON) {
		t.Fatal("FoldThrough(realCfg, throughRound=3) produced the same state as Fold(cfg with Rounds truncated to 3) — the regression this function exists to prevent is not actually caught")
	}

	if rightState.UnstableSector == nil {
		t.Fatal("FoldThrough(realCfg, throughRound=3): UnstableSector is nil — round 3 was (wrongly) treated as the match's last round")
	}
	if wrongState.UnstableSector != nil {
		t.Fatal("Fold(wrongCfg with Rounds:3): UnstableSector is non-nil — the fixture no longer demonstrates the truncated-cfg failure mode this test relies on")
	}
}

// TestFoldThroughRejectsRoundBeyondCfgRounds is #322's own acceptance
// criterion: "--round N beyond the match's last round is an error naming
// the last round, not a silently clamped dump."
func TestFoldThroughRejectsRoundBeyondCfgRounds(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{23}
	players := 2
	log := fullIdleLogThrough(cfg, players)

	_, _, err := FoldThrough(seed, cfg, players, log, game.RoundNumber(cfg.Rounds+1))
	if err == nil {
		t.Fatal("FoldThrough(cfg.Rounds+1) returned nil error, want a rejection naming cfg.Rounds")
	}
	wantMsg := fmt.Sprintf("fold: round %d is beyond the match's last round (%d)", cfg.Rounds+1, cfg.Rounds)
	if err.Error() != wantMsg {
		t.Errorf("err = %q, want %q", err.Error(), wantMsg)
	}
}

// TestFoldThroughRejectsRoundBelowOne asserts throughRound < 1 is rejected
// rather than silently treated as "fold nothing."
func TestFoldThroughRejectsRoundBelowOne(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{24}
	players := 2
	log := fullIdleLogThrough(cfg, players)

	_, _, err := FoldThrough(seed, cfg, players, log, 0)
	if err == nil {
		t.Fatal("FoldThrough(throughRound=0) returned nil error, want a rejection")
	}
}

// TestFoldThroughByteIdenticalAcrossRuns is RFC §16.2's own determinism
// requirement, applied to FoldThrough directly rather than only through
// Fold: folding the same {seed, cfg, players, log, throughRound} twice
// produces byte-identical state and event sequence.
func TestFoldThroughByteIdenticalAcrossRuns(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := [32]byte{25}
	players := 2
	log := fullIdleLogThrough(cfg, players)
	through := game.RoundNumber(cfg.Rounds / 2)

	s1, e1, err1 := FoldThrough(seed, cfg, players, log, through)
	if err1 != nil {
		t.Fatalf("FoldThrough #1: %v", err1)
	}
	s2, e2, err2 := FoldThrough(seed, cfg, players, log, through)
	if err2 != nil {
		t.Fatalf("FoldThrough #2: %v", err2)
	}

	j1, _ := json.Marshal(s1)
	j2, _ := json.Marshal(s2)
	if string(j1) != string(j2) {
		t.Fatal("two FoldThrough calls with identical inputs produced different state")
	}
	if len(e1) != len(e2) {
		t.Fatalf("two FoldThrough calls produced %d and %d events", len(e1), len(e2))
	}
}
