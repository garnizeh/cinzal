package rules

import "github.com/garnizeh/cinzal/internal/game"

// Affordance is the per-draft-order UI state RFC §10.2 requires the server
// to compute, so an illegal submission becomes unreachable in the rendered
// form rather than rejected after the fact ("Validation is not affordance").
// It is a single struct rather than a per-ActionKind map because RFC §10.2's
// six rows are not uniformly action-shaped: two describe route/action-
// selector state, one a stance warning, one a numeric budget, two clamps.
type Affordance struct {
	// ActionForced is true once Pushing On is declared: the action selector
	// must be disabled and forced to Nothing (RFC §10.2 row 1; GDD §9.1,
	// §15.0).
	ActionForced bool

	// RouteBlocked is true once the drafted route has already entered a
	// Hidden node: no further node targets are clickable except the Pushing
	// On control (RFC §10.2 row 2; GDD §15.0).
	RouteBlocked bool

	// StepsRemaining is the live budget after the drafted route so far (RFC
	// §10.2 row 3), derived from Steps and floored at 0.
	StepsRemaining int

	// EvasiveShakedownWarning is a warning, never a block: true when Evasive
	// is selected while carrying cargo and the balance is under the
	// shakedown cost, so the policy would lapse (RFC §10.2 row 4; GDD §15).
	// Going Evasive while broke stays a legal, sometimes-correct choice.
	EvasiveShakedownWarning bool

	// StakePostAtCap is true when the player is at their post cap; Stake
	// Post must be disabled (RFC §10.2 row 5; GDD §10.3).
	StakePostAtCap bool

	// ExpiringPost names which lease to let expire, when StakePostAtCap is
	// true: the post with the fewest RoundsRemaining, ties broken by the
	// lowest NodeID (RFC §6.4-§6.5's ascending-NodeID tie-break convention).
	// Zero value otherwise.
	ExpiringPost game.NodeID

	// MaxLeaseBlocks is the most lease blocks affordable at the player's own
	// exact balance, capped at 4 (GDD §10.4's per-post maximum) (RFC §10.2
	// row 6).
	MaxLeaseBlocks int

	// MaxStake is the most Cr$ affordable as an Aggressive stance stake,
	// capped at 6 (GDD §9.3's per-round maximum) (RFC §10.2 row 6).
	MaxStake int
}

// Affordances computes the UI-state RFC §10.2 requires for the order draft
// in progress. Unlike Legal, it never rejects — it describes what the form
// should allow right now.
func Affordances(v game.PlayerView, cfg game.Config, draft game.Order) Affordance {
	a := Affordance{
		ActionForced: pushingOnDeclared(draft),
		RouteBlocked: routeEndsHidden(v, draft),
	}

	a.StepsRemaining = max(0, Steps(v, cfg)-len(draft.Route))

	a.EvasiveShakedownWarning = draft.Stance.Stance == game.StanceEvasive &&
		v.You.Cargo != nil &&
		v.You.Balance < cfg.ShakedownCost

	players := len(v.Others) + 1
	if postCap, ok := cfg.PostCapByPlayers[players]; ok && len(v.You.Posts) >= postCap {
		a.StakePostAtCap = true
		a.ExpiringPost = expiringPost(v.You.Posts)
	}

	// A selected Ledger purchase reserves its cost first, so the two clamps
	// stay jointly affordable — otherwise the affordance layer could offer a
	// lease-block count that, combined with BuyLedger, Legal then rejects.
	availableBalance := v.You.Balance
	if draft.AddOns.BuyLedger {
		availableBalance -= cfg.LedgerCost
	}
	if cfg.LeaseCostPerBlock > 0 {
		a.MaxLeaseBlocks = max(0, min(4, availableBalance/cfg.LeaseCostPerBlock))
	}
	a.MaxStake = min(6, v.You.Balance)

	return a
}

// expiringPost picks the post with the fewest RoundsRemaining, breaking ties
// by the lowest NodeID.
func expiringPost(posts []game.Post) game.NodeID {
	if len(posts) == 0 {
		return 0
	}

	best := posts[0]
	for _, p := range posts[1:] {
		if p.RoundsRemaining < best.RoundsRemaining ||
			(p.RoundsRemaining == best.RoundsRemaining && p.Node < best.Node) {
			best = p
		}
	}
	return best.Node
}
