package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestAffordanceActionForcedByPushingOn: RFC §10.2 row 1 — declaring
// Pushing On must force the action selector to unavailable.
func TestAffordanceActionForcedByPushingOn(t *testing.T) {
	v := legalTestView()
	draft := game.Order{Route: []game.NodeID{1, 2, 3}, PushingOn: game.PushingOn{Steps: 1}}
	got := Affordances(v, legalTestConfig(), draft)
	if !got.ActionForced {
		t.Fatalf("Affordances().ActionForced = false, want true when Pushing On is declared")
	}
}

// TestAffordanceRouteBlockedByHiddenNode: RFC §10.2 row 2 — once the route
// has entered a Hidden node, no further node targets should be offered.
func TestAffordanceRouteBlockedByHiddenNode(t *testing.T) {
	v := legalTestView()
	draft := game.Order{Route: []game.NodeID{1, 2, 3}}
	got := Affordances(v, legalTestConfig(), draft)
	if !got.RouteBlocked {
		t.Fatalf("Affordances().RouteBlocked = false, want true once the route ends on a Hidden node")
	}

	notBlocked := Affordances(v, legalTestConfig(), game.Order{Route: []game.NodeID{1}})
	if notBlocked.RouteBlocked {
		t.Fatalf("Affordances().RouteBlocked = true for a route ending on a Known node, want false")
	}
}

// TestAffordanceStepsRemaining: RFC §10.2 row 3 — the live budget counter,
// so the HUD number can never disagree with the server's.
func TestAffordanceStepsRemaining(t *testing.T) {
	v := legalTestView() // Infamy 0 -> Nobody tier, 4 steps.
	cfg := legalTestConfig()

	got := Affordances(v, cfg, game.Order{Route: []game.NodeID{1, 2}})
	if got.StepsRemaining != 2 {
		t.Fatalf("StepsRemaining = %d, want 2 (allowance 4, 2 steps drafted)", got.StepsRemaining)
	}

	// A draft longer than the allowance floors at 0, never goes negative.
	overBudget := Affordances(v, cfg, game.Order{Route: []game.NodeID{1, 0, 1, 0, 1, 0}})
	if overBudget.StepsRemaining != 0 {
		t.Fatalf("StepsRemaining = %d, want 0 (floored, never negative)", overBudget.StepsRemaining)
	}
}

// TestAffordanceEvasiveShakedownIsWarningNotBlock: RFC §10.2 row 4 — going
// Evasive while broke and carrying cargo is a legal, sometimes-correct
// choice, so it must surface as a warning and Legal must still accept it.
func TestAffordanceEvasiveShakedownIsWarningNotBlock(t *testing.T) {
	v := legalTestView()
	v.You.Cargo = &game.CarriedCargo{Bound: false}
	v.You.Balance = 0 // below cfg.ShakedownCost
	cfg := legalTestConfig()

	draft := game.Order{Stance: game.StanceOrder{Stance: game.StanceEvasive}}
	got := Affordances(v, cfg, draft)
	if !got.EvasiveShakedownWarning {
		t.Fatalf("EvasiveShakedownWarning = false, want true when broke and carrying cargo under Evasive")
	}

	o := game.Order{Action: game.ActionOrder{Kind: game.ActionNothing}, Stance: game.StanceOrder{Stance: game.StanceEvasive}}
	if err := Legal(v, o, cfg); err != nil {
		t.Fatalf("Legal() = %v, want nil — the shakedown warning must never become a rejection (RFC §10.2)", err)
	}
}

// TestAffordanceEvasiveShakedownNotWarnedWhenAffordable confirms the warning
// is conditional on actually being unable to pay.
func TestAffordanceEvasiveShakedownNotWarnedWhenAffordable(t *testing.T) {
	v := legalTestView()
	v.You.Cargo = &game.CarriedCargo{Bound: false}
	v.You.Balance = 20 // above cfg.ShakedownCost

	draft := game.Order{Stance: game.StanceOrder{Stance: game.StanceEvasive}}
	got := Affordances(v, legalTestConfig(), draft)
	if got.EvasiveShakedownWarning {
		t.Fatalf("EvasiveShakedownWarning = true, want false when the shakedown is affordable")
	}
}

// TestAffordanceStakePostAtCap: RFC §10.2 row 5 — Stake Post disabled at the
// cap, naming which lease to let expire.
func TestAffordanceStakePostAtCap(t *testing.T) {
	v := legalTestView()
	cfg := legalTestConfig()
	postCap := cfg.PostCapByPlayers[len(v.Others)+1]

	for i := range postCap {
		// Ascending RoundsRemaining so node 100 (i=0) is the soonest to expire.
		v.You.Posts = append(v.You.Posts, game.Post{Node: game.NodeID(100 + i), RoundsRemaining: i + 1})
	}

	got := Affordances(v, cfg, game.Order{})
	if !got.StakePostAtCap {
		t.Fatalf("StakePostAtCap = false, want true at the post cap")
	}
	if got.ExpiringPost != 100 {
		t.Fatalf("ExpiringPost = %d, want 100 (fewest RoundsRemaining)", got.ExpiringPost)
	}
}

// TestAffordanceMaxLeaseBlocksAndStakeClampToBalance: RFC §10.2 row 6 —
// lease blocks and stake clamped to the player's own exact balance.
func TestAffordanceMaxLeaseBlocksAndStakeClampToBalance(t *testing.T) {
	v := legalTestView()
	cfg := legalTestConfig() // LeaseCostPerBlock: 3

	v.You.Balance = 5
	got := Affordances(v, cfg, game.Order{})
	if got.MaxLeaseBlocks != 1 {
		t.Fatalf("MaxLeaseBlocks = %d, want 1 (floor(5/3))", got.MaxLeaseBlocks)
	}
	if got.MaxStake != 5 {
		t.Fatalf("MaxStake = %d, want 5 (balance, under the Cr$6 cap)", got.MaxStake)
	}

	v.You.Balance = 100
	got = Affordances(v, cfg, game.Order{})
	if got.MaxLeaseBlocks != 4 {
		t.Fatalf("MaxLeaseBlocks = %d, want 4 (GDD §10.4's per-post maximum)", got.MaxLeaseBlocks)
	}
	if got.MaxStake != 6 {
		t.Fatalf("MaxStake = %d, want 6 (GDD §9.3's per-round maximum)", got.MaxStake)
	}
}
