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

// TestAffordanceMaxLeaseBlocksReservesSelectedLedgerCost: a selected Ledger
// purchase must reserve its cost before MaxLeaseBlocks is computed, so the
// two stay jointly affordable — otherwise the affordance layer could offer a
// lease-block count that, combined with BuyLedger, Legal then rejects.
func TestAffordanceMaxLeaseBlocksReservesSelectedLedgerCost(t *testing.T) {
	v := legalTestView()
	cfg := legalTestConfig() // LedgerCost: 3, LeaseCostPerBlock: 3
	v.You.Balance = 5

	withoutLedger := Affordances(v, cfg, game.Order{})
	if withoutLedger.MaxLeaseBlocks != 1 {
		t.Fatalf("MaxLeaseBlocks (no ledger) = %d, want 1 (floor(5/3))", withoutLedger.MaxLeaseBlocks)
	}

	withLedger := Affordances(v, cfg, game.Order{AddOns: game.AddOns{BuyLedger: true}})
	if withLedger.MaxLeaseBlocks != 0 {
		t.Fatalf("MaxLeaseBlocks (ledger selected) = %d, want 0 (balance 5 - ledger 3 = 2, floor(2/3))", withLedger.MaxLeaseBlocks)
	}

	// The clamp must never go negative even when the ledger cost exceeds
	// the balance outright.
	v.You.Balance = 1
	broke := Affordances(v, cfg, game.Order{AddOns: game.AddOns{BuyLedger: true}})
	if broke.MaxLeaseBlocks != 0 {
		t.Fatalf("MaxLeaseBlocks (balance 1, ledger selected) = %d, want 0, not negative", broke.MaxLeaseBlocks)
	}
}

// TestAffordanceMaxLeaseBlocksClampedByRenewalCeiling: GDD §10.4 — a
// renewal against a post already close to the 12-round ceiling must not be
// offered more blocks than fit under it, even when the balance alone would
// afford more. This is the acceptance bar's "the clamp is visible in the
// affordance metadata": the number itself is the clamp, not a separate flag.
func TestAffordanceMaxLeaseBlocksClampedByRenewalCeiling(t *testing.T) {
	v := legalTestView()
	cfg := legalTestConfig() // LeaseCostPerBlock: 3, LeaseBlockRounds: 3
	v.You.Balance = 100      // affordable well past the block cap on its own
	v.You.Posts = []game.Post{{Node: 9, RoundsRemaining: 9}}

	draft := game.Order{AddOns: game.AddOns{RenewPost: 9, RenewBlocks: 1}}
	got := Affordances(v, cfg, draft)
	if got.MaxLeaseBlocks != 1 {
		t.Fatalf("MaxLeaseBlocks (renewing a post at 9/12 remaining) = %d, want 1 (only 1 more block fits under the 12-round ceiling)", got.MaxLeaseBlocks)
	}

	// A post already at the ceiling: renewal offers no further blocks.
	v.You.Posts = []game.Post{{Node: 9, RoundsRemaining: 12}}
	atCeiling := Affordances(v, cfg, draft)
	if atCeiling.MaxLeaseBlocks != 0 {
		t.Fatalf("MaxLeaseBlocks (renewing a post already at 12) = %d, want 0", atCeiling.MaxLeaseBlocks)
	}

	// A target the seat does not hold — most commonly the node this same
	// order is staking new — is not an existing lease to clamp against, so
	// the balance-based cap alone applies.
	fresh := game.Order{AddOns: game.AddOns{RenewPost: 42, RenewBlocks: 1}}
	notHeld := Affordances(v, cfg, fresh)
	if notHeld.MaxLeaseBlocks != maxLeaseBlocks {
		t.Fatalf("MaxLeaseBlocks (RenewPost not currently held) = %d, want %d (unclamped)", notHeld.MaxLeaseBlocks, maxLeaseBlocks)
	}
}
