package telemetry

import (
	"slices"
	"time"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

var _ = time.Now

// liveConvergenceRounds returns every round in 1..cfg.Rounds whose global
// event card (deck[round-4], GDD §14.2: rounds 4 onward) carries
// rules.TagConvergence — the same seed-derivable-identity move
// liveIncidentRounds (incidents.go) uses for the sector incident deck,
// mirroring internal/rules/trail.go's own eventCardThisRound peek.
func liveConvergenceRounds(cfg game.Config, deck []rules.EventCardID) []game.RoundNumber {
	var rounds []game.RoundNumber
	for r := 1; r <= cfg.Rounds; r++ {
		idx := r - 4
		if idx < 0 || idx >= len(deck) {
			continue
		}
		if slices.Contains(rules.EventCardTags(deck[idx]), rules.TagConvergence) {
			rounds = append(rounds, game.RoundNumber(r))
		}
	}
	return rounds
}

// convergenceCardConfrontations computes
// MatchSummary.ConvergenceCardConfrontations — D33's loose reading: a
// round a [C] card was live that also had at least one confrontation,
// never causation.
func convergenceCardConfrontations(s rules.MatchState, cfg game.Config, groups map[confrontationKey][]game.Event) Rate {
	rounds := liveConvergenceRounds(cfg, s.Graph.EventDeck)
	if len(rounds) == 0 {
		return Rate{}
	}

	confrontationRounds := make(map[game.RoundNumber]bool, len(groups))
	for key := range groups {
		confrontationRounds[key.round] = true
	}

	produced := 0
	for _, r := range rounds {
		if confrontationRounds[r] {
			produced++
		}
	}
	return Rate{Value: float64(produced) / float64(len(rounds)), N: len(rounds)}
}
