package rules

import "github.com/garnizeh/cinzal/internal/game"

// GDD §16's flat scoring numbers — stated directly by the design, not
// Config-tunable (mirrors events.go's constant block for the same reason:
// none of these appears in game.Config).
const (
	postLiveRP              = 2  // "Each post with a live lease at the end"
	sectorMajorityRP        = 3  // "Sector majority of live posts"
	cashPerRP               = 4  // "Cash | 1 per Cr$ 4, rounded down"
	activeContractPenaltyRP = -2 // "Active undelivered contract"
)

// FinalScoreBreakdown is one seat's GDD §16 final score, broken into its
// five sources plus the total, computed once at the end of round 15.
type FinalScoreBreakdown struct {
	Seat game.SeatID

	// PriorRP is Player.RP as it stands entering final scoring: every RP
	// this seat earned over the match, delivered-contract RP (GDD §16's
	// "Delivered contracts" row) included, already accumulated round by
	// round by resolveOneDelivery and every other RP-granting resolution.
	// Final scoring adds to it; it never recomputes it.
	PriorRP int

	PostsRP    int // postLiveRP * live posts held
	MajorityRP int // sectorMajorityRP * sectors won outright
	CashRP     int // Balance / cashPerRP, truncating
	PenaltyRP  int // activeContractPenaltyRP * undelivered contracts (negative)
	Total      int
}

// FinalScore computes GDD §16's final scoring for every seat in s, ranked
// Total descending, ties broken by byFinalStanding's key (most contracts
// delivered, then highest Infamy, then highest balance) — the returned
// slice is match placement, rank 1 first, not seat order.
//
// Pure and round-agnostic by design, matching upkeep.go's own precedent:
// Resolve has no phase for this (D5 — round 15 runs Upkeep like any other
// round, so "live at the end" already means whatever Upkeep's lease
// decrement left standing by the time this reads s). It is the caller's
// job to invoke this once s.Round >= cfg.Rounds; nothing here branches on
// the round number itself. Takes no game.Config: every §16 source is a
// flat number the design states directly (the constants above), never a
// Config-tunable dial — Player.RP already folded in cfg.Contracts' tier
// values back when each contract was delivered.
//
// An accepted-but-never-picked-up contract still pays the penalty:
// membership in Player.Contracts is the trigger, not carried-cargo state.
func FinalScore(s MatchState) []FinalScoreBreakdown {
	majorities := SectorMajority(s)

	breakdowns := make([]FinalScoreBreakdown, len(s.Players))
	for _, seat := range bySeat(s) {
		p := s.Players[seat]

		posts := 0
		for _, n := range s.Graph.Nodes {
			if n.Post != nil && n.Post.Owner == seat {
				posts++
			}
		}

		sectors := 0
		for sector := game.SectorOldDocks; sector <= game.SectorNorthVale; sector++ {
			if winner, ok := majorities[sector]; ok && winner == seat {
				sectors++
			}
		}

		b := FinalScoreBreakdown{
			Seat:       seat,
			PriorRP:    p.RP,
			PostsRP:    postLiveRP * posts,
			MajorityRP: sectorMajorityRP * sectors,
			CashRP:     p.Balance / cashPerRP,
			PenaltyRP:  activeContractPenaltyRP * len(p.Contracts),
		}
		b.Total = b.PriorRP + b.PostsRP + b.MajorityRP + b.CashRP + b.PenaltyRP
		breakdowns[seat] = b
	}

	rankFinalScores(s, breakdowns)

	return breakdowns
}
