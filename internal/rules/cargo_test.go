package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestCanCollectLooseCrateNeedsNoContract is GDD §8.4: "anyone, no contract
// required" — CanCollect must be true even for a seat holding no contracts
// at all, as long as the cargo isn't Bound.
func TestCanCollectLooseCrateNeedsNoContract(t *testing.T) {
	loose := Cargo{Node: 5, Bound: false}
	if !CanCollect(nil, loose) {
		t.Error("CanCollect(nil, loose crate) = false, want true")
	}
}

// TestCanCollectBoundCargoNeedsMatchingContract is GDD §8.4's restriction on
// bound cargo: a contract with a different origin/destination pair does not
// qualify, and no contracts at all never qualifies.
func TestCanCollectBoundCargoNeedsMatchingContract(t *testing.T) {
	bound := Cargo{Node: 5, Bound: true, Origin: 1, Destination: 2}

	if CanCollect(nil, bound) {
		t.Error("CanCollect(nil, bound cargo) = true, want false")
	}

	mismatched := []Contract{{ID: 0, Origin: 1, Destination: 3, ExpiresRound: 10}}
	if CanCollect(mismatched, bound) {
		t.Error("CanCollect with a same-origin, different-destination contract = true, want false")
	}

	matching := []Contract{{ID: 0, Origin: 1, Destination: 2, ExpiresRound: 10}}
	if !CanCollect(matching, bound) {
		t.Error("CanCollect with a matching origin/destination contract = false, want true")
	}
}

// TestDeadlinePauseAndCargoRace is GDD §8.4's own worked case, verbatim: "A
// is ambushed and gets the extension, drops the crate, B collects it — B
// fulfils B's contract on B's untouched deadline with B's own Pause
// available. Nothing is inherited, because there was never a shared
// object." A and B hold separate Contract instances for the identical
// origin/destination pair; A's pause must not touch B's contract at all,
// and both must be able to collect the pair's dropped cargo.
func TestDeadlinePauseAndCargoRace(t *testing.T) {
	a := Contract{ID: 0, Origin: 1, Destination: 2, Tier: 0, ExpiresRound: 10}
	b := Contract{ID: 0, Origin: 1, Destination: 2, Tier: 0, ExpiresRound: 12}

	aPaused := a.WithDeadlinePause()
	if aPaused.ExpiresRound != 11 {
		t.Errorf("A's ExpiresRound after pause = %d, want 11", aPaused.ExpiresRound)
	}
	if !aPaused.DeadlinePauseUsed {
		t.Error("A's DeadlinePauseUsed = false after WithDeadlinePause, want true")
	}

	// B's own contract, from the same starting shape as a (before its
	// pause), is completely untouched by A's pause — there was never a
	// shared object.
	if b.ExpiresRound != 12 || b.DeadlinePauseUsed {
		t.Errorf("B's contract = %+v, want ExpiresRound=12 DeadlinePauseUsed=false (unaffected by A's pause)", b)
	}

	// A's pause is once-per-contract: applying it again is a no-op.
	aPausedAgain := aPaused.WithDeadlinePause()
	if aPausedAgain != aPaused {
		t.Errorf("second WithDeadlinePause() = %+v, want unchanged %+v", aPausedAgain, aPaused)
	}

	// The dropped cargo carries the pair, not either seat's ContractID —
	// both A's and B's contracts (same origin/destination) can collect it.
	dropped := Cargo{Node: 7, Bound: true, Origin: 1, Destination: 2}
	if !CanCollect([]Contract{aPaused}, dropped) {
		t.Error("CanCollect(A's contract, dropped cargo) = false, want true")
	}
	if !CanCollect([]Contract{b}, dropped) {
		t.Error("CanCollect(B's contract, dropped cargo) = false, want true")
	}
}

// TestLooseCrateHeatFiresOnSecondConsecutiveRound is GDD §8.4: "From the
// second consecutive round you end holding one" — never the first, and it
// resets the moment the crate stops being held.
func TestLooseCrateHeatFiresOnSecondConsecutiveRound(t *testing.T) {
	held := 0

	held = NextLooseCrateHeldRounds(true, held) // round 1 of holding
	if held != 1 {
		t.Fatalf("held after round 1 = %d, want 1", held)
	}
	if LooseCrateHeatFires(held) {
		t.Error("heat fired after only 1 round held, want false")
	}

	held = NextLooseCrateHeldRounds(true, held) // round 2 of holding
	if held != 2 {
		t.Fatalf("held after round 2 = %d, want 2", held)
	}
	if !LooseCrateHeatFires(held) {
		t.Error("heat did not fire after 2 consecutive rounds held, want true")
	}

	held = NextLooseCrateHeldRounds(true, held) // round 3 — still hot
	if !LooseCrateHeatFires(held) {
		t.Error("heat did not fire after 3 consecutive rounds held, want true")
	}

	held = NextLooseCrateHeldRounds(false, held) // dropped
	if held != 0 {
		t.Fatalf("held after dropping = %d, want 0", held)
	}
	if LooseCrateHeatFires(held) {
		t.Error("heat fired after the crate was dropped, want false")
	}
}

// TestDeliverMatchesGDDContractTable pins GDD §8.3's payment/RP/Infamy
// numbers per tier, including the III/IV +2-Infamy delivery bonus (vs +1 for
// I/II).
func TestDeliverMatchesGDDContractTable(t *testing.T) {
	cfg := game.DefaultConfig()

	cases := []struct {
		tier                            int
		wantPayment, wantRP, wantInfamy int
	}{
		{0, 8, 2, 1},
		{1, 14, 3, 1},
		{2, 20, 5, 2},
		{3, 30, 8, 2},
	}
	for _, c := range cases {
		contract := Contract{Tier: c.tier}
		payment, rp, infamy := Deliver(contract, cfg)
		if payment != c.wantPayment || rp != c.wantRP || infamy != c.wantInfamy {
			t.Errorf("Deliver(tier=%d) = (%d, %d, %d), want (%d, %d, %d)",
				c.tier, payment, rp, infamy, c.wantPayment, c.wantRP, c.wantInfamy)
		}
	}
}

// TestPenalizeMatchesGDDContractTable pins GDD §8.3's penalty numbers per
// tier, including Tier IV's additional −2 Infamy.
func TestPenalizeMatchesGDDContractTable(t *testing.T) {
	cfg := game.DefaultConfig()

	cases := []struct {
		tier                 int
		wantCost, wantInfamy int
	}{
		{0, 3, 0},
		{1, 5, 0},
		{2, 8, 0},
		{3, 12, 2},
	}
	for _, c := range cases {
		contract := Contract{Tier: c.tier}
		cost, infamyLoss := Penalize(contract, cfg)
		if cost != c.wantCost || infamyLoss != c.wantInfamy {
			t.Errorf("Penalize(tier=%d) = (%d, %d), want (%d, %d)", c.tier, cost, infamyLoss, c.wantCost, c.wantInfamy)
		}
	}
}
