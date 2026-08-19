package telemetry

import "github.com/garnizeh/cinzal/internal/game"

// confrontationKey identifies one confrontation — a (Round, Node) pair
// (D33 rows 1/10/11: a tie emits K-1 EventConfrontation for one
// confrontation, a multi-loser decisive result emits one per loser, so raw
// event counting overcounts either shape).
type confrontationKey struct {
	round game.RoundNumber
	node  game.NodeID
}

// groupConfrontations groups every EventConfrontation in events by (Round,
// Node) — the single grouping MatchSummary.ConfrontationsPerMatch, rows 9,
// 14 and 20 all share, computed once so every row reads the identical
// population.
func groupConfrontations(events []game.Event) map[confrontationKey][]game.Event {
	groups := make(map[confrontationKey][]game.Event)
	for _, e := range events {
		if e.Kind != game.EventConfrontation {
			continue
		}
		key := confrontationKey{round: e.Round, node: e.Node}
		groups[key] = append(groups[key], e)
	}
	return groups
}

// confrontationsWonAgainstEvasiveLoser computes
// MatchSummary.ConfrontationsWonAgainstEvasiveLoser: a group counts toward
// the numerator if any of its events is a Decisive win over a Target who
// declared StanceEvasive. Ranges groups only to sum a count — see
// routesCancelledMidRoute (match.go) for why that is fine here, unlike
// inside Resolve's own pipeline.
func confrontationsWonAgainstEvasiveLoser(groups map[confrontationKey][]game.Event) Rate {
	if len(groups) == 0 {
		return Rate{}
	}

	wonAgainstEvasive := 0
	for _, group := range groups {
		for _, e := range group {
			if e.Decisive && e.Stance == game.StanceEvasive {
				wonAgainstEvasive++
				break
			}
		}
	}
	return Rate{Value: float64(wonAgainstEvasive) / float64(len(groups)), N: len(groups)}
}

// confrontationsInFinal3Rounds computes
// MatchSummary.ConfrontationsInFinal3Rounds. Ranges groups only to sum a
// count — see routesCancelledMidRoute (match.go).
func confrontationsInFinal3Rounds(cfg game.Config, groups map[confrontationKey][]game.Event) Rate {
	if len(groups) == 0 {
		return Rate{}
	}

	threshold := game.RoundNumber(cfg.Rounds - 2)
	final := 0
	for key := range groups {
		if key.round >= threshold {
			final++
		}
	}
	return Rate{Value: float64(final) / float64(len(groups)), N: len(groups)}
}
