package telemetry

import (
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// liveIncidentRounds returns every round in 1..cfg.Rounds with a live
// sector incident card, re-derived from deck's own length rather than a new
// event — GDD §14.3's incident deck runs "From round 3 onward," one card
// per round, deck[round-3], the same non-consuming peek
// incidentCardThisRound (internal/rules/incidents.go) already uses inside
// Resolve. Duplicated here rather than exported from internal/rules because
// it is pure arithmetic over already-exported fields (MatchState.Graph.
// IncidentDeck, cfg.Rounds) — no RNG, no rules-internal state — the same
// "seed-derivable, re-derivable exactly" move D33 uses for row 14's event
// card identity.
func liveIncidentRounds(cfg game.Config, deckLen int) []game.RoundNumber {
	var rounds []game.RoundNumber
	for r := 1; r <= cfg.Rounds; r++ {
		idx := r - 3
		if idx >= 0 && idx < deckLen {
			rounds = append(rounds, game.RoundNumber(r))
		}
	}
	return rounds
}

// sectorIncidentsHittingAPlayer computes
// MatchSummary.SectorIncidentsHittingAPlayer.
func sectorIncidentsHittingAPlayer(s rules.MatchState, cfg game.Config, events []game.Event) Rate {
	rounds := liveIncidentRounds(cfg, len(s.Graph.IncidentDeck))
	if len(rounds) == 0 {
		return Rate{}
	}

	hitRounds := make(map[game.RoundNumber]bool)
	for _, e := range events {
		if e.Kind == game.EventIncidentHit {
			hitRounds[e.Round] = true
		}
	}

	hits := 0
	for _, r := range rounds {
		if hitRounds[r] {
			hits++
		}
	}
	return Rate{Value: float64(hits) / float64(len(rounds)), N: len(rounds)}
}

// playersEndingInFlaggedSector computes
// MatchSummary.PlayersEndingInFlaggedSector. playerCount and cfg.Rounds are
// both already known positive by the time Match calls this — never a
// row-level empty population the way liveIncidentRounds/
// liveConvergenceRounds can legitimately be.
func playersEndingInFlaggedSector(playerCount int, cfg game.Config, events []game.Event) Rate {
	n := playerCount * cfg.Rounds
	return Rate{Value: float64(countEvents(events, game.EventIncidentExposed)) / float64(n), N: n}
}

// roundsFlaggedLoitering computes MatchSummary.RoundsFlaggedLoitering. See
// playersEndingInFlaggedSector for why n needs no zero check here.
func roundsFlaggedLoitering(playerCount int, cfg game.Config, events []game.Event) Rate {
	n := playerCount * cfg.Rounds
	return Rate{Value: float64(countEvents(events, game.EventLoitering)) / float64(n), N: n}
}
