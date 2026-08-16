package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// debtFixture builds a minimal MatchState with one seat at balance, holding
// the given posts (node -> RoundsRemaining), for ResolveDebt's tests.
func debtFixture(balance int, posts map[game.NodeID]int) MatchState {
	nodes := make([]Node, 0, len(posts))
	var owned []game.NodeID
	for node, rounds := range posts {
		for len(nodes) <= int(node) {
			nodes = append(nodes, Node{ID: game.NodeID(len(nodes))})
		}
		nodes[node].Post = &Post{Owner: 0, RoundsRemaining: rounds}
		owned = append(owned, node)
	}
	return MatchState{
		Graph:   Graph{Nodes: nodes},
		Players: []Player{{Seat: 0, Balance: balance, Posts: owned}},
	}
}

// TestResolveDebtPaidInFullNeverTriggers: balance alone covering owed is not
// Debt at all — GDD §13's cascade starts only where a plain payment leaves
// off.
func TestResolveDebtPaidInFullNeverTriggers(t *testing.T) {
	s := debtFixture(10, nil)
	got := ResolveDebt(s, 0, 6)

	if got.Paid != 6 {
		t.Errorf("Paid = %d, want 6", got.Paid)
	}
	if got.Triggered || got.Flagged || got.InfamyDelta != 0 || got.Surrendered || got.Forgiven != 0 {
		t.Errorf("ResolveDebt(balance=10, owed=6) = %+v, want a no-op past step 1", got)
	}
}

// TestResolveDebtNoPostsForgivesTheRest: balance insufficient and no posts
// held — GDD §13 steps 1, 2, then straight to 4 (step 3 has nothing to
// surrender).
func TestResolveDebtNoPostsForgivesTheRest(t *testing.T) {
	s := debtFixture(3, nil)
	got := ResolveDebt(s, 0, 10)

	if got.Paid != 3 {
		t.Errorf("Paid = %d, want 3 (balance floors at Cr$0)", got.Paid)
	}
	if !got.Triggered {
		t.Fatalf("Triggered = false, want true")
	}
	if got.InfamyDelta != DebtInfamyLoss {
		t.Errorf("InfamyDelta = %d, want %d", got.InfamyDelta, DebtInfamyLoss)
	}
	if got.Surrendered {
		t.Errorf("Surrendered = true, want false — no posts held")
	}
	if got.Forgiven != 7 {
		t.Errorf("Forgiven = %d, want 7 (10 owed - 3 paid, no lease credit)", got.Forgiven)
	}
	if !got.Flagged {
		t.Errorf("Flagged = false, want true")
	}
}

// TestResolveDebtSurrendersFewestRoundsRemaining: with posts held, GDD §13
// step 3 surrenders the one with fewest rounds remaining and credits a flat
// Cr$2 against the debt, regardless of which node it is.
func TestResolveDebtSurrendersFewestRoundsRemaining(t *testing.T) {
	s := debtFixture(0, map[game.NodeID]int{2: 9, 5: 1, 7: 4})
	got := ResolveDebt(s, 0, 10)

	if got.Paid != 0 {
		t.Errorf("Paid = %d, want 0", got.Paid)
	}
	if !got.Surrendered || got.SurrenderedPost != 5 {
		t.Fatalf("SurrenderedPost = (%d, %v), want (5, true) — node 5 has fewest rounds remaining (1)", got.SurrenderedPost, got.Surrendered)
	}
	if got.Forgiven != 8 {
		t.Errorf("Forgiven = %d, want 8 (10 owed - 0 paid - Cr$2 lease credit)", got.Forgiven)
	}
	if !got.Flagged {
		t.Errorf("Flagged = false, want true")
	}
}

// TestResolveDebtLeaseCreditNeverGoesNegative: a small remainder more than
// covered by the flat Cr$2 lease credit still floors Forgiven at 0, never
// negative.
func TestResolveDebtLeaseCreditNeverGoesNegative(t *testing.T) {
	s := debtFixture(0, map[game.NodeID]int{3: 2})
	got := ResolveDebt(s, 0, 1)

	if !got.Surrendered {
		t.Fatalf("Surrendered = false, want true")
	}
	if got.Forgiven != 0 {
		t.Errorf("Forgiven = %d, want 0 (Cr$2 credit against a Cr$1 remainder), never negative", got.Forgiven)
	}
}

// TestResolveDebtNeverPaysMoreThanBalance is RFC §16.2's invariant: Paid can
// never exceed what the seat actually had, across a spread of balances and
// debts including balance == 0 and owed == 0.
func TestResolveDebtNeverPaysMoreThanBalance(t *testing.T) {
	cases := []struct{ balance, owed int }{
		{0, 0}, {0, 5}, {5, 0}, {5, 5}, {3, 100}, {100, 3},
	}
	for _, c := range cases {
		s := debtFixture(c.balance, nil)
		got := ResolveDebt(s, 0, c.owed)
		if got.Paid > c.balance {
			t.Errorf("ResolveDebt(balance=%d, owed=%d).Paid = %d, exceeds balance", c.balance, c.owed, got.Paid)
		}
		if got.Paid < 0 {
			t.Errorf("ResolveDebt(balance=%d, owed=%d).Paid = %d, negative", c.balance, c.owed, got.Paid)
		}
	}
}

// TestResolveDebtTwoDebtsOneFlagOneStep is GDD §13 verbatim: "Two forgiven
// debts in one resolution... still produce one flag and one lost step." Two
// separate ResolveDebt calls against the same broke seat both trigger, but
// folding both Flagged results into Player.Flagged is a boolean OR, and the
// step-allowance formula (Steps, steps.go) only ever reads that one bool —
// it cannot see or subtract for "two".
func TestResolveDebtTwoDebtsOneFlagOneStep(t *testing.T) {
	s := debtFixture(0, nil)

	first := ResolveDebt(s, 0, 4)
	second := ResolveDebt(s, 0, 6) // a second, independent shortfall the same resolution

	if !first.Flagged || !second.Flagged {
		t.Fatalf("both debts must trigger Flagged individually: first=%v second=%v", first.Flagged, second.Flagged)
	}

	var flagged bool
	flagged = flagged || first.Flagged
	flagged = flagged || second.Flagged
	if !flagged {
		t.Fatalf("folded Flagged = false, want true")
	}

	cfg := game.DefaultConfig()
	got := Steps(selfStateWithInfamy(0, game.StepModifiers{Flagged: flagged}), cfg)
	if base := cfg.StepsByTier[infamyTierIndex(0, cfg)]; got != base-1 {
		t.Fatalf("Steps() with Flagged folded from two debts = %d, want %d (base %d, exactly one step lost, not two)", got, base-1, base)
	}
}

// TestShakedownCapsAtBalanceNeverTriggersDebt is GDD §15's carve-out,
// worked exactly as #65's acceptance criteria names it: a broke Evasive
// loser's shakedown payment never surrenders a lease and never sets
// Flagged, because Shakedown has no access to either — it is architecturally
// impossible for it to reach into MatchState at all, unlike ResolveDebt.
func TestShakedownCapsAtBalanceNeverTriggersDebt(t *testing.T) {
	cfg := game.DefaultConfig() // ShakedownCost: 4
	s := debtFixture(0, map[game.NodeID]int{1: 3})

	paid := Shakedown(s.Players[0].Balance, cfg.ShakedownCost)
	if paid != 0 {
		t.Fatalf("Shakedown(balance=0, cost=%d) = %d, want 0", cfg.ShakedownCost, paid)
	}

	// The broke loser's state is untouched: no lease surrendered, no flag
	// set. Shakedown never called ResolveDebt for the Cr$4 shortfall.
	if s.Players[0].Flagged {
		t.Fatalf("Player.Flagged = true after Shakedown alone, want false")
	}
	if len(s.Players[0].Posts) != 1 || s.Graph.Nodes[1].Post == nil {
		t.Fatalf("post at node 1 was touched by Shakedown, want untouched")
	}
}

// TestShakedownCapsAtBalanceGenerally pins the cap itself across a spread of
// balances, including one that fully covers the cost.
func TestShakedownCapsAtBalanceGenerally(t *testing.T) {
	cost := 4
	cases := []struct{ balance, want int }{
		{0, 0},
		{2, 2},
		{4, 4},
		{10, 4},
	}
	for _, c := range cases {
		if got := Shakedown(c.balance, cost); got != c.want {
			t.Errorf("Shakedown(balance=%d, cost=%d) = %d, want %d", c.balance, cost, got, c.want)
		}
	}
}

// TestCreditBandForBoundaries pins GDD §5.1's four bands at every boundary,
// both sides.
func TestCreditBandForBoundaries(t *testing.T) {
	cases := []struct {
		balance int
		want    game.CreditBand
	}{
		{0, game.BandBroke},
		{5, game.BandBroke},
		{6, game.BandGettingBy},
		{15, game.BandGettingBy},
		{16, game.BandFlush},
		{30, game.BandFlush},
		{31, game.BandLoaded},
		{1000, game.BandLoaded},
	}
	for _, c := range cases {
		if got := CreditBandFor(c.balance); got != c.want {
			t.Errorf("CreditBandFor(%d) = %v, want %v", c.balance, got, c.want)
		}
	}
}
