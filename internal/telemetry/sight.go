package telemetry

import (
	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// shareOfMapUnderSightFinalThird computes
// MatchSummary.ShareOfMapUnderSightFinalThird: for each seat, the share of
// s.Graph.Nodes with a Sight round-bit set for any round in the match's
// final third, meaned across every seat.
func shareOfMapUnderSightFinalThird(s rules.MatchState, cfg game.Config) float64 {
	start := finalThirdStart(cfg.Rounds)
	total := len(s.Graph.Nodes)

	sum := 0.0
	for _, p := range s.Players {
		seen := 0
		for _, n := range s.Graph.Nodes {
			if sightInFinalThird(p.Archive.Sight[n.ID], start, cfg.Rounds) {
				seen++
			}
		}
		sum += float64(seen) / float64(total)
	}
	return sum / float64(len(s.Players))
}

// sightInFinalThird reports whether set has any round in [start, rounds]
// marked.
func sightInFinalThird(set game.RoundSet, start game.RoundNumber, rounds int) bool {
	for r := start; r <= game.RoundNumber(rounds); r++ {
		if set.Has(r) {
			return true
		}
	}
	return false
}

// heatMapLowConfidenceEntries computes
// MatchSummary.HeatMapLowConfidenceEntries, pooled across every (seat,
// node) pair this match ever produced a genuine Heat Map entry for.
func heatMapLowConfidenceEntries(s rules.MatchState) Rate {
	entries, lowConfidence := 0, 0
	for _, p := range s.Players {
		for _, n := range s.Graph.Nodes {
			observed := p.Archive.Sight[n.ID].Count()
			if observed == 0 {
				continue
			}
			entries++
			if observed < 3 {
				lowConfidence++
			}
		}
	}
	if entries == 0 {
		return Rate{}
	}
	return Rate{Value: float64(lowConfidence) / float64(entries), N: entries}
}
