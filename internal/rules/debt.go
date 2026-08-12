package rules

import "github.com/garnizeh/cinzal/internal/game"

// DebtLeaseCredit is GDD §13 step 3's flat Cr$ credited against an unpaid
// debt when a lease is surrendered — not the lease's value or its rounds
// remaining, just this fixed figure, applied once per debt regardless of
// how large the remainder is.
const DebtLeaseCredit = 2

// DebtInfamyLoss is GDD §13 step 2's fixed cost of any debt balance alone
// couldn't cover — GDD §11's gains/losses table has no row for it because
// it belongs to Debt, not to an action.
const DebtInfamyLoss = -1

// DebtResult is what one Debt resolution (GDD §13) says should happen.
// Applying it — deducting Paid from the seat's balance, adding InfamyDelta
// (through ApplyInfamyDelta), removing SurrenderedPost's lease when
// Surrendered, and setting Player.Flagged when Flagged — is the caller's
// job (Resolve, #67); ResolveDebt only computes the cascade, in the fixed
// order GDD §13 specifies, without mutating MatchState itself.
type DebtResult struct {
	// Paid is Cr$ actually deducted from the seat's balance — never more
	// than they had (GDD §13 step 1: "You pay what you have. Balance
	// floors at Cr$ 0.").
	Paid int

	// Triggered is true once balance alone could not cover the debt — the
	// gate for every step from here on. False means owed <= balance: Paid
	// == owed, and every other field is the zero value.
	Triggered bool

	// InfamyDelta is DebtInfamyLoss when Triggered, else 0 (GDD §13 step
	// 2). Apply through ApplyInfamyDelta, never added directly — Infamy's
	// 0-10 floor and ceiling still apply here like everywhere else.
	InfamyDelta int

	// SurrenderedPost is the node whose lease is surrendered (GDD §13 step
	// 3: the one with fewest rounds remaining), meaningful only when
	// Surrendered is true.
	SurrenderedPost game.NodeID
	Surrendered     bool

	// Forgiven is whatever remains owed after balance and (if any) the
	// lease credit — GDD §13 step 4, written off, never collected from the
	// player by any other means.
	Forgiven int

	// Flagged is always true when Triggered (GDD §13 step 4: "you are
	// Flagged: −1 step next round"), regardless of whether a lease was
	// surrendered or how large Forgiven ended up. "Flagged is a boolean,
	// not a counter" (GDD §13): ResolveDebt reports the same true on the
	// first debt a resolution produces as on the fifth, and it is the
	// caller's job to OR it into Player.Flagged rather than count it.
	Flagged bool
}

// ResolveDebt runs GDD §13's cascade for one payment obligation of owed Cr$
// against seat's current balance and posts in s, in the fixed order the GDD
// specifies: pay what you have, −1 Infamy, surrender the lease with fewest
// rounds remaining for a flat Cr$2 credit, forgive whatever is left and
// flag the player. It is safe to call for any payment a player can't fully
// cover — a stake, a gate fee, a penalty, a lease renewal — including one
// balance already covers in full, in which case Triggered is false and
// nothing past step 1 applies.
//
// The Evasive shakedown (GDD §15) is deliberately not routed through this
// function: "the shakedown... never triggers Debt" is a carve-out the GDD
// states explicitly, and Shakedown (this file) is its own, separate path
// that never reads Infamy, posts, or Flagged at all.
func ResolveDebt(s MatchState, seat game.SeatID, owed int) DebtResult {
	balance := s.Players[seat].Balance
	paid := min(balance, owed)
	remainder := owed - paid

	result := DebtResult{Paid: paid}
	if remainder <= 0 {
		return result
	}

	result.Triggered = true
	result.InfamyDelta = DebtInfamyLoss
	result.Flagged = true

	if node, ok := SelectLeaseByFewestRounds(leasePosts(s, seat)); ok {
		result.SurrenderedPost = node
		result.Surrendered = true
		remainder = max(0, remainder-DebtLeaseCredit)
	}

	result.Forgiven = remainder
	return result
}

// Shakedown reports how much of GDD §15's Cr$ 4 fee an Evasive confrontation
// loser actually pays: capped at their current balance, and never more.
// This is the whole of the carve-out GDD §15 states — "the shakedown is
// capped at their current balance and never triggers Debt (§13)": a loser
// who cannot cover the full fee simply has the difference go uncollected,
// with no Infamy loss, no lease surrendered, and no Flagged, because this
// function never touches any of those — it takes and returns nothing but
// Cr$. Whatever it leaves unpaid is the caller's own signal (RFC-level,
// #69) that the cargo is forfeit on the same terms as any other loss; it is
// never a signal to fall back to ResolveDebt for the shortfall.
func Shakedown(balance, cost int) (paid int) {
	return min(balance, cost)
}

// CreditBandFor maps an exact Cr$ balance to GDD §5.1's disclosed band —
// the only view of a balance any seat gets of an opponent (OpponentView.Band),
// exact balances staying inside this package and its callers (Debt included)
// and never crossing into game.OpponentView itself.
func CreditBandFor(balance int) game.CreditBand {
	switch {
	case balance <= 5:
		return game.BandBroke
	case balance <= 15:
		return game.BandGettingBy
	case balance <= 30:
		return game.BandFlush
	default:
		return game.BandLoaded
	}
}
