package rules_test

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/bots"
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// TestRunnerGoldenMatchFourPlayersFifteenRounds is issue #193's own golden-
// match acceptance criterion: a real 15-round, 4-player match — real
// rules.NewMatch/Resolve/Project calls, no test-scripted orders — with
// every seat played by Runner. It lives here, in internal/rules's own
// external test package, rather than in internal/bots, because it needs
// both rules.MatchState (which internal/bots may never name — bot_test.go's
// TestPackageNamesNoMatchStateNowhere) and internal/bots itself (which
// internal/rules may never import in production code — D01). package
// rules_test compiles as neither, exactly like newmatch_external_test.go
// already does for #157.
//
// Fails closed the same way issue #193 states: a harness that silently
// resolved zero rounds, or produced no real gameplay at all, would satisfy
// "no seat produced more than 3 empty routes" trivially — so this also
// asserts the match actually reached round 15 and that the event stream
// carries at least one EventDelivered and one EventConfrontation.
func TestRunnerGoldenMatchFourPlayersFifteenRounds(t *testing.T) {
	cfg := game.DefaultConfig()
	var seed [32]byte
	seed[0] = 0xe1
	const players = 4

	s, err := rules.NewMatch(seed, cfg, players)
	if err != nil {
		t.Fatalf("rules.NewMatch() = %v", err)
	}

	runner := bots.For(bots.Runner)

	emptyRoutes := make(map[game.SeatID]int, players)
	var delivered, confronted, dealt int

	for round := 1; round <= cfg.Rounds; round++ {
		orders := make(map[game.SeatID]game.Order, players)
		for seat := game.SeatID(0); int(seat) < players; seat++ {
			v := rules.ProjectView(s, seat, cfg)
			o := runner.Decide(v, cfg, rules.NewBotRNG(seed, seat, round))

			if len(o.Route) == 0 {
				emptyRoutes[seat]++
			}
			if o.Action.Kind == game.ActionDeal {
				dealt++
			}
			orders[seat] = o
		}

		next, events, err := rules.Resolve(s, orders, cfg, rules.NewRNG(seed, round))
		if err != nil {
			t.Fatalf("round %d: rules.Resolve() = %v", round, err)
		}
		s = next

		for _, e := range events {
			switch e.Kind {
			case game.EventDelivered:
				delivered++
			case game.EventConfrontation:
				confronted++
			}
		}
	}

	if s.Round != game.RoundNumber(cfg.Rounds) {
		t.Fatalf("s.Round = %d after the loop, want %d — the match did not run to completion", s.Round, cfg.Rounds)
	}

	if dealt != 0 {
		t.Errorf("Runner emitted ActionDeal %d times across the match — RFC-001 §14.3's Runner \"never buys\"", dealt)
	}

	for seat, n := range emptyRoutes {
		if n > 3 {
			t.Errorf("seat %d produced an empty route in %d of %d rounds, want <= 3 (issue #193's own bound)", seat, n, cfg.Rounds)
		}
	}

	if delivered == 0 {
		t.Error("zero EventDelivered across the whole match — an all-Runner 4-player match should complete at least one delivery (issue #193)")
	}
	if confronted == 0 {
		t.Error("zero EventConfrontation across the whole match — issue #193's own fails-closed criterion")
	}
}
