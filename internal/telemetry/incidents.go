package telemetry

import (
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// liveIncidentRounds returns every round in 1..cfg.Rounds with a live
// sector incident card, re-derived from the deck rather than from a new
// event — GDD §14.3's incident deck runs "From round 3 onward," one card
// per round, the same non-consuming peek incidentCardThisRound
// (internal/rules/incidents.go) uses inside Resolve, reached here through
// its exported form. That is the "seed-derivable, re-derivable exactly"
// move D33 uses for row 14's event card identity.
//
// This function once carried its own copy of the deck[round-3] arithmetic,
// on the argument that it is pure over already-exported fields
// (MatchState.Graph.IncidentDeck, cfg.Rounds) and so safe to duplicate. The
// argument held right up until cmd/simulate needed a third copy for #205's
// per-card R6 breakdown — see rules.IncidentCardForRound's own comment.
func liveIncidentRounds(cfg game.Config, deck []rules.IncidentCardID) []game.RoundNumber {
	var rounds []game.RoundNumber
	for r := 1; r <= cfg.Rounds; r++ {
		if _, live := rules.IncidentCardForRound(game.RoundNumber(r), deck); live {
			rounds = append(rounds, game.RoundNumber(r))
		}
	}
	return rounds
}

// sectorIncidentsHittingAPlayer computes
// MatchSummary.SectorIncidentsHittingAPlayer.
func sectorIncidentsHittingAPlayer(s rules.MatchState, cfg game.Config, events []game.Event) Rate {
	rounds := liveIncidentRounds(cfg, s.Graph.IncidentDeck)
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
