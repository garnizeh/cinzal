//go:build !race

// This file's 2,000-match cohort comparison is excluded from race builds
// (go test -race sets this implicit build tag): its own runtime, race
// detector overhead, and internal/rules' own already-substantial existing
// suite under -race together exceed Go's 10-minute per-package test
// timeout (confirmed directly — make check's go test -race ./... panics
// with "test timed out after 10m0s" mid-run of
// TestOperatorBeatsRunnerOverAThousandMatches, having already spent most
// of that budget on the rest of the package's own tests). This is the same
// shape as bench-baseline/bench-compare (Makefile): expensive,
// deterministic, and meant to be run and read directly
// (`go test ./internal/rules/... -run TestOperatorBeatsRunnerOverAThousandMatches -v`)
// rather than bundled into the standard race-detected gate. It still runs
// under a plain `go test ./...` (no -race), and testing.Short() skips it
// there too for a fast local loop.

package rules_test

import (
	"math"
	"testing"

	"github.com/garnizeh/cinzal/internal/bots"
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// runCohort plays n real 4-player, 15-round matches — every seat played by
// tier, one distinct seed per match drawn from base by flipping its low
// byte — and returns each match's mean RP across its 4 seats, plus how many
// matches actually reached round 15. Real rules.NewMatch/Resolve/Project
// calls throughout, exactly like TestRunnerGoldenMatchFourPlayersFifteenRounds
// above; projectForBot (that file) performs the same StepAllowance/
// RoundsToNextOffer fill-in this harness also needs.
func runCohort(t *testing.T, tier bots.Tier, cfg game.Config, base [32]byte, n int) (meanRP []float64, completed int) {
	t.Helper()

	const players = 4
	bot := bots.For(tier)
	meanRP = make([]float64, 0, n)

	for i := range n {
		seed := base
		seed[31] = byte(i)
		seed[30] = byte(i >> 8)

		s, err := rules.NewMatch(seed, cfg, players)
		if err != nil {
			t.Fatalf("%s match %d: NewMatch: %v", tier, i, err)
		}

		for round := 1; round <= cfg.Rounds; round++ {
			orders := make(map[game.SeatID]game.Order, players)
			for seat := game.SeatID(0); int(seat) < players; seat++ {
				v := projectForBot(s, seat, cfg)
				orders[seat] = bot.Decide(v, cfg, rules.NewBotRNG(seed, seat, round))
			}

			next, _, err := rules.Resolve(s, orders, cfg, rules.NewRNG(seed, round))
			if err != nil {
				t.Fatalf("%s match %d, round %d: Resolve: %v", tier, i, round, err)
			}
			s = next
		}

		if s.Round != game.RoundNumber(cfg.Rounds) {
			continue
		}
		completed++

		total := 0
		for _, p := range s.Players {
			total += p.RP
		}
		meanRP = append(meanRP, float64(total)/float64(players))
	}

	return meanRP, completed
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	total := 0.0
	for _, x := range xs {
		total += x
	}
	return total / float64(len(xs))
}

// TestOperatorBeatsRunnerOverAThousandMatches is issue #194's own
// acceptance criterion: "Operator beats Runner over 1,000 matches at 4
// players on the golden seed set, by mean RP, by a margin reported in the
// test output." One shared 1,000-seed golden set drives both cohorts —
// every seat Operator against it, then every seat Runner against the exact
// same 1,000 seeds — reduced to mean RP per match, then compared. Sharing
// the seed set (rather than drawing an independent one per cohort) is a
// paired comparison, not just a convenient re-use: map generation, contract
// pools and the event deck all vary enormously by seed, and that variance
// dwarfs the actual behavioural difference between two competent tiers —
// pairing on identical seeds cancels it out of the margin instead of
// letting it swamp the signal. Not beating Runner is a result worth having;
// failing to report the margin is not (hence t.Logf below regardless of
// outcome).
//
// Fails closed exactly as the acceptance criteria state: both cohorts must
// have completed all 1,000 matches to round 15 (a cohort that silently
// dropped matches to a degenerate early end would report a mean over a
// biased subset), and the RP spread between cohorts must be non-zero —
// two cohorts that both scored zero, or identically, compare equal, which
// is not a result this test may report as a win.
func TestOperatorBeatsRunnerOverAThousandMatches(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the 2,000-match cohort comparison in -short mode")
	}

	cfg := game.DefaultConfig()
	const n = 1000

	var goldenSeeds [32]byte
	goldenSeeds[0] = 0xa0 // the golden seed set: one fixed root, varied per match below, shared by both cohorts

	operatorRP, operatorCompleted := runCohort(t, bots.Operator, cfg, goldenSeeds, n)
	runnerRP, runnerCompleted := runCohort(t, bots.Runner, cfg, goldenSeeds, n)

	if operatorCompleted != n {
		t.Fatalf("Operator cohort: %d of %d matches reached round 15, want %d", operatorCompleted, n, n)
	}
	if runnerCompleted != n {
		t.Fatalf("Runner cohort: %d of %d matches reached round 15, want %d", runnerCompleted, n, n)
	}

	operatorMean, runnerMean := mean(operatorRP), mean(runnerRP)
	margin := operatorMean - runnerMean

	t.Logf("Operator mean RP = %.3f, Runner mean RP = %.3f, margin = %.3f (%d matches per cohort)",
		operatorMean, runnerMean, margin, n)

	if math.Abs(margin) < 1e-9 {
		t.Fatalf("Operator and Runner cohorts scored identically (mean RP %.3f both) — a zero spread is not a comparison", operatorMean)
	}
	if margin <= 0 {
		t.Errorf("Operator mean RP %.3f did not beat Runner mean RP %.3f (margin %.3f) — a result worth having, not a test bug",
			operatorMean, runnerMean, margin)
	}
}
