package rules

import "github.com/garnizeh/cinzal/internal/game"

// The named deltas from GDD §11's gains/losses table that don't already have
// a home. Delivery's gain lives on cargo.Deliver and the missed-deadline
// loss lives on cargo.Penalize, because both need a Config.Contracts row to
// compute; everything below is a fixed number, so a constant is all it
// needs. Debt's own −1 (GDD §13) lives with the rest of Debt, in debt.go.
const (
	// InfamyGainConfrontationWin is GDD §11: "Win a confrontation +2."
	InfamyGainConfrontationWin = 2

	// InfamyLossConfrontationLoss is GDD §11: "Lose a confrontation −1."
	InfamyLossConfrontationLoss = -1

	// InfamyGainFirstPost is GDD §11: "Stake your first post in a sector
	// +1" — fires exactly when FirstPostInSector (posts.go) reports true.
	InfamyGainFirstPost = 1

	// InfamyLossVanish is GDD §11: "Vanish action −2."
	InfamyLossVanish = -2
)

// ApplyInfamyDelta returns current+delta clamped to GDD §5's 0-10 range —
// the one floor and ceiling every Infamy-moving rule in the game shares,
// from an ordinary delivery to Amnesty's explicit "−3 Infamy, floor 0" (GDD
// §14.2). Every gain and loss this package computes, and every future one
// (items, events, incidents, confrontations), is meant to pass through this
// rather than clamp its own copy: Infamy must never be observed outside
// 0-10, however many deltas landed against it in the same round.
func ApplyInfamyDelta(current, delta int) int {
	return min(10, max(0, current+delta))
}

// TierOf maps an exact Infamy score to its GDD §11 ladder row. It is the
// single source of the ladder's boundaries — infamyTierIndex (steps.go) is
// built on this rather than repeating the boundary table, so the two can
// never quietly drift apart the way GDD §9.2 and §10.3 once did (#40, D15).
func TierOf(infamy int) game.InfamyTier {
	switch {
	case infamy <= 2:
		return game.TierNobody
	case infamy <= 5:
		return game.TierKnown
	case infamy <= 8:
		return game.TierFeared
	default:
		return game.TierLegend
	}
}

// EndOfRoundPositionRevealFires reports GDD §11's Feared-and-up exposure
// bullet — "Position revealed to all at end of each round" — evaluated
// exactly as GDD §11.1 requires: "the end-of-round position reveal... uses
// your Infamy after resolution has finished," the live value, never a
// snapshot. The Vanish worked case (GDD §11.1) is this function called with
// the post-resolution Infamy: a Legend who Vanishes from 9 to 7 is still
// Feared, so this still reports true — Vanish suppresses the "fresh tracks"
// trail entry on a different channel, never this one.
func EndOfRoundPositionRevealFires(infamy int) bool {
	return TierOf(infamy) >= game.TierFeared
}

// OrderPhasePositionBroadcastFires reports GDD §11's Legend-only exposure
// bullet — "Position public during the entire order phase" — evaluated
// exactly as GDD §11.1 requires: "as the order phase opens," which is the
// entry snapshot's Infamy (SeatSnapshot.Infamy, frozen before the round it
// describes runs), never whatever Resolve computes mid-round.
func OrderPhasePositionBroadcastFires(infamy int) bool {
	return TierOf(infamy) >= game.TierLegend
}
