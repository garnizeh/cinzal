package rules_test

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// TestNewMatchExternalConstructionReachesFinalScore is #157's own
// acceptance criterion, verified from outside internal/rules: a caller
// holding no unexported identifier — this file is package rules_test, the
// same isolation M2's internal/bots and cmd/simulate will have — can build
// a match with rules.NewMatch, drive it through every round with
// rules.Resolve, and reach rules.FinalScore. Every seat submits no order
// (GDD §18's absence default) rather than exercising any particular rule;
// this test is only about the construction path being reachable at all,
// not about scoring outcomes — that is #84's exit demo.
func TestNewMatchExternalConstructionReachesFinalScore(t *testing.T) {
	cfg := game.DefaultConfig()
	var seed [32]byte
	seed[0] = 0x9c

	s, err := rules.NewMatch(seed, cfg, 4)
	if err != nil {
		t.Fatalf("rules.NewMatch() = %v", err)
	}
	if s.Round != 0 {
		t.Fatalf("rules.NewMatch() returned Round = %d, want 0", s.Round)
	}

	for round := 1; round <= cfg.Rounds; round++ {
		s, _, err = rules.Resolve(s, map[game.SeatID]game.Order{}, cfg, rules.NewRNG(seed, round))
		if err != nil {
			t.Fatalf("round %d: rules.Resolve() = %v", round, err)
		}
	}

	breakdown := rules.FinalScore(s)
	if len(breakdown) != 4 {
		t.Fatalf("len(rules.FinalScore()) = %d, want 4", len(breakdown))
	}
}
