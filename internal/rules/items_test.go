package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestAllItemsCatalogMatchesGDD pins GDD §12's price table and §9.4's timing
// table against allItems, item by item — the same data-integrity discipline
// TestPurposeTableMatchesDeclaredConstants (rng_purpose_test.go) applies to
// the RNG consumption table.
func TestAllItemsCatalogMatchesGDD(t *testing.T) {
	want := map[game.ItemID]itemMeta{
		game.ItemShiv:              {game.ItemShiv, 4, TimingArmed},
		game.ItemMuscle:            {game.ItemMuscle, 7, TimingPermanent},
		game.ItemPoliceBand:        {game.ItemPoliceBand, 3, TimingImmediate},
		game.ItemCirculationPermit: {game.ItemCirculationPermit, 5, TimingArmed},
		game.ItemTornMap:           {game.ItemTornMap, 3, TimingImmediate},
		game.ItemDecoy:             {game.ItemDecoy, 5, TimingEndOfRound},
		game.ItemBoltHole:          {game.ItemBoltHole, 5, TimingArmed},
		game.ItemGuardContact:      {game.ItemGuardContact, 6, TimingImmediate},
	}

	if len(allItems) != len(want) {
		t.Fatalf("len(allItems) = %d, want %d (all eight GDD §12 items, no more no fewer)", len(allItems), len(want))
	}

	seen := map[game.ItemID]bool{}
	for _, got := range allItems {
		if seen[got.Item] {
			t.Fatalf("allItems declares %s twice", got.Item)
		}
		seen[got.Item] = true

		w, ok := want[got.Item]
		if !ok {
			t.Fatalf("allItems declares %s, not one of GDD §12's eight items", got.Item)
		}
		if got != w {
			t.Errorf("allItems[%s] = %+v, want %+v", got.Item, got, w)
		}
	}
}

// TestRevealTornMapConsumesMinFourHidden is RFC §16.1's Torn Map row and
// #66's own acceptance criteria: exactly min(4, hidden) draws, no
// duplicates, and zero draws when hidden is empty.
func TestRevealTornMapConsumesMinFourHidden(t *testing.T) {
	cases := []struct {
		name   string
		hidden int
		want   int
	}{
		{"zero hidden nodes", 0, 0},
		{"fewer than four", 1, 1},
		{"exactly four", 4, 4},
		{"more than four", 10, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rng := NewRNG(testSeed(1), 1)
			hidden := make([]game.NodeID, tc.hidden)
			for i := range hidden {
				hidden[i] = game.NodeID(i)
			}

			got := RevealTornMap(rng, hidden)

			if len(got) != tc.want {
				t.Fatalf("len(RevealTornMap) = %d, want %d", len(got), tc.want)
			}
			if consumed := rng.Consumed(PurposeItemTornMap); consumed != tc.want {
				t.Fatalf("Consumed(item.tornmap) = %d, want %d", consumed, tc.want)
			}

			seen := map[game.NodeID]bool{}
			for _, n := range got {
				if seen[n] {
					t.Fatalf("RevealTornMap returned duplicate node %d", n)
				}
				seen[n] = true
			}
		})
	}
}

// TestResolveGuardContactAppliesFixedDelta pins GDD §12's fixed −3, applied
// through the same 0-10 clamp every other Infamy delta in this package
// uses.
func TestResolveGuardContactAppliesFixedDelta(t *testing.T) {
	cases := []struct{ infamy, want int }{
		{10, 7},
		{6, 3},
		{2, 0}, // clamps at the floor rather than going negative
		{0, 0},
	}
	for _, c := range cases {
		if got := ResolveGuardContact(c.infamy); got != c.want {
			t.Errorf("ResolveGuardContact(%d) = %d, want %d", c.infamy, got, c.want)
		}
	}
}

// TestGuardContactChangesConfrontationTierBeforeFight is #66's own
// acceptance criterion: "Guard Contact's −3 lands before confrontation
// modifiers are computed — a test where it changes the outcome of a fight
// in the same round." The confrontation resolver itself is #69's scope and
// does not exist yet, so this is scoped honestly against what does exist:
// GDD §11's ladder table, whose Combat column (+0/+1/+2/+3 by tier) is what
// a confrontation's outcome will read once #69 lands. Guard Contact moving a
// player from Infamy 6 (Feared, Combat +2) to Infamy 3 (Known, Combat +1)
// is exactly the case GDD §9.4 describes: "dropping three Infamy after the
// order phase but before the fighting" changes which Combat bonus a
// same-round confrontation is evaluated against.
func TestGuardContactChangesConfrontationTierBeforeFight(t *testing.T) {
	before := 6
	if got := TierOf(before); got != game.TierFeared {
		t.Fatalf("TierOf(%d) = %v, want TierFeared (test fixture assumption)", before, got)
	}

	after := ResolveGuardContact(before)
	if got := TierOf(after); got != game.TierKnown {
		t.Fatalf("TierOf(ResolveGuardContact(%d)) = %v, want TierKnown — Guard Contact must cross a tier boundary for this test to demonstrate anything", before, got)
	}
}

// TestMuscleLossTable pins D14/#52's ruling verbatim: every non-winner of a
// 3+-way melee loses Muscle if held; a tie loses nobody's.
func TestMuscleLossTable(t *testing.T) {
	cases := []struct {
		name            string
		isWinner, isTie bool
		want            bool
	}{
		{"winner keeps it", true, false, false},
		{"non-winner loses it", false, false, true},
		{"tie: nobody loses it, even the eventual non-winner label", false, true, false},
		{"tie: the nominal winner slot also keeps it", true, true, false},
	}
	for _, c := range cases {
		if got := MuscleLoss(c.isWinner, c.isTie); got != c.want {
			t.Errorf("%s: MuscleLoss(isWinner=%v, isTie=%v) = %v, want %v", c.name, c.isWinner, c.isTie, got, c.want)
		}
	}
}
