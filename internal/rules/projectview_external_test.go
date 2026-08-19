package rules_test

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/bots"
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// TestProjectViewStepAllowanceNeverZero is issue #199's own acceptance
// criterion for the D27 post-fill it promoted to an exported helper: "a
// test asserting StepAllowance is non-zero for every seat in every round —
// the failure this exists to prevent is silent." A PlayerView built by
// rules.Project alone leaves StepAllowance at its zero value by design
// (D27); rules.ProjectView is the one place that fill-in happens, and a bot
// or a human reading v.You.StepAllowance == 0 from it would silently plan
// a zero-step route (issue #191) rather than fail loudly — so this drives
// real matches, at every player count the game supports, and checks every
// seat's view every single round, not just round 1 or a hand-built
// fixture.
func TestProjectViewStepAllowanceNeverZero(t *testing.T) {
	cfg := game.DefaultConfig()
	bot := bots.For(bots.Drifter)

	for players := 2; players <= 5; players++ {
		var seed [32]byte
		seed[0] = 0xc1
		seed[1] = byte(players)

		s, err := rules.NewMatch(seed, cfg, players)
		if err != nil {
			t.Fatalf("players=%d: rules.NewMatch() = %v", players, err)
		}

		for round := 1; round <= cfg.Rounds; round++ {
			orders := make(map[game.SeatID]game.Order, players)
			for seat := game.SeatID(0); int(seat) < players; seat++ {
				v := rules.ProjectView(s, seat, cfg)
				if v.You.StepAllowance == 0 {
					t.Fatalf("players=%d round=%d seat=%d: ProjectView returned StepAllowance == 0", players, round, seat)
				}
				orders[seat] = bot.Decide(v, cfg, rules.NewBotRNG(seed, seat, round))
			}

			next, _, err := rules.Resolve(s, orders, cfg, rules.NewRNG(seed, round))
			if err != nil {
				t.Fatalf("players=%d round=%d: rules.Resolve() = %v", players, round, err)
			}
			s = next
		}

		if s.Round != game.RoundNumber(cfg.Rounds) {
			t.Fatalf("players=%d: s.Round = %d after the loop, want %d", players, s.Round, cfg.Rounds)
		}
	}
}
