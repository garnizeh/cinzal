package telemetry

import "github.com/garnizeh/cinzal/internal/rules"

// winnerRPLeadOverLastPlace computes MatchSummary.WinnerRPLeadOverLastPlace
// from rules.FinalScore(s) — GDD §16's own scoring, not a re-derivation of
// it. FinalScore already ranks its result Total descending, so the winner
// and last place are its first and last entries.
func winnerRPLeadOverLastPlace(s rules.MatchState) Rate {
	breakdowns := rules.FinalScore(s)
	winner := breakdowns[0].Total
	last := breakdowns[len(breakdowns)-1].Total
	if winner <= 0 {
		return Rate{}
	}
	return Rate{Value: float64(winner-last) / float64(winner), N: 1}
}

// liveLeasesAtFinalScoring computes MatchSummary.LiveLeasesAtFinalScoring:
// every Node with a non-nil Post, divided by the player count.
func liveLeasesAtFinalScoring(s rules.MatchState) float64 {
	posts := 0
	for _, n := range s.Graph.Nodes {
		if n.Post != nil {
			posts++
		}
	}
	return float64(posts) / float64(len(s.Players))
}
