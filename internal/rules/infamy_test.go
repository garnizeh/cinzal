package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestApplyInfamyDeltaClampsToZeroToTen pins GDD §5's 0-10 range as a hard
// floor and ceiling, including Amnesty's explicit "−3 Infamy, floor 0" (GDD
// §14.2) and a gain that would otherwise overshoot the ceiling.
func TestApplyInfamyDeltaClampsToZeroToTen(t *testing.T) {
	cases := []struct {
		name    string
		current int
		delta   int
		want    int
	}{
		{"ordinary gain, no boundary", 3, 1, 4},
		{"ordinary loss, no boundary", 5, -1, 4},
		{"gain overshoots the ceiling", 9, 2, 10},
		{"already at the ceiling, another gain", 10, 2, 10},
		{"Amnesty: -3, floors at 0 exactly", 2, -3, 0},
		{"loss overshoots the floor", 1, -2, 0},
		{"already at the floor, another loss", 0, -1, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ApplyInfamyDelta(c.current, c.delta); got != c.want {
				t.Errorf("ApplyInfamyDelta(%d, %d) = %d, want %d", c.current, c.delta, got, c.want)
			}
		})
	}
}

// TestApplyInfamyDeltaNeverLeavesZeroToTenAcrossARound applies every
// combination of a card delta, an incident delta, and a confrontation delta
// (each independently ranging across the full spread this package and
// cargo.go produce) in sequence, as a single round's worth of Infamy events
// would land one after another — one clamp per step, never a single clamp
// at the end that could let an intermediate value wander outside the
// range. The result must stay inside 0-10 after every single step, for
// every start value and every combination.
func TestApplyInfamyDeltaNeverLeavesZeroToTenAcrossARound(t *testing.T) {
	deltas := []int{-3, -2, -1, 0, 1, 2} // Amnesty, Vanish/Debt, loss, no-op, first-post/deliver, win/deliver-III-IV

	for start := 0; start <= 10; start++ {
		for _, card := range deltas {
			for _, incident := range deltas {
				for _, confrontation := range deltas {
					v := start
					for _, d := range []int{card, incident, confrontation} {
						v = ApplyInfamyDelta(v, d)
						if v < 0 || v > 10 {
							t.Fatalf("ApplyInfamyDelta chain from start=%d, deltas=%d,%d,%d produced %d, outside 0-10",
								start, card, incident, confrontation, v)
						}
					}
				}
			}
		}
	}
}

// TestTierOfBoundaries pins GDD §11's ladder table exactly at every
// boundary, both sides.
func TestTierOfBoundaries(t *testing.T) {
	cases := []struct {
		infamy int
		want   game.InfamyTier
	}{
		{0, game.TierNobody},
		{2, game.TierNobody},
		{3, game.TierKnown},
		{5, game.TierKnown},
		{6, game.TierFeared},
		{8, game.TierFeared},
		{9, game.TierLegend},
		{10, game.TierLegend},
	}
	for _, c := range cases {
		if got := TierOf(c.infamy); got != c.want {
			t.Errorf("TierOf(%d) = %v, want %v", c.infamy, got, c.want)
		}
	}
}

// TestGainsAndLossesTableRows is GDD §11's gains/losses table, one row per
// test, each asserting the row's Δ Infamy crosses (or fails to cross) a
// tier boundary the instant it is applied via ApplyInfamyDelta and TierOf —
// "the moment they fire, using your Infamy at that moment" (GDD §11.1), not
// some later or frozen read of it.
func TestGainsAndLossesTableRows(t *testing.T) {
	cfg := game.DefaultConfig()

	t.Run("deliver tier I or II: +1 crosses Nobody to Known", func(t *testing.T) {
		c := Contract{Tier: 0}
		_, _, gain := Deliver(c, cfg)
		if gain != 1 {
			t.Fatalf("Deliver tier I infamyGain = %d, want 1 (GDD §11)", gain)
		}
		got := ApplyInfamyDelta(2, gain)
		if got != 3 || TierOf(got) != game.TierKnown {
			t.Fatalf("2 + deliver-I/II = %d (%v), want 3 (Known)", got, TierOf(got))
		}
	})

	t.Run("deliver tier III or IV: +2 crosses Known to Feared", func(t *testing.T) {
		c := Contract{Tier: 2}
		_, _, gain := Deliver(c, cfg)
		if gain != 2 {
			t.Fatalf("Deliver tier III infamyGain = %d, want 2 (GDD §11)", gain)
		}
		got := ApplyInfamyDelta(4, gain)
		if got != 6 || TierOf(got) != game.TierFeared {
			t.Fatalf("4 + deliver-III/IV = %d (%v), want 6 (Feared)", got, TierOf(got))
		}
	})

	t.Run("win a confrontation: +2 crosses Feared to Legend, clamped", func(t *testing.T) {
		got := ApplyInfamyDelta(8, InfamyGainConfrontationWin)
		if got != 10 || TierOf(got) != game.TierLegend {
			t.Fatalf("8 + win-confrontation = %d (%v), want 10 (Legend)", got, TierOf(got))
		}
	})

	t.Run("stake first post in a sector: +1 crosses Nobody to Known", func(t *testing.T) {
		s := MatchState{
			Graph:   Graph{Nodes: []Node{{ID: 0, Sector: game.SectorOldDocks}}},
			Players: []Player{{Seat: 0}},
		}
		if !FirstPostInSector(s, 0, 0) {
			t.Fatalf("FirstPostInSector = false, want true (no posts held yet)")
		}
		got := ApplyInfamyDelta(2, InfamyGainFirstPost)
		if got != 3 || TierOf(got) != game.TierKnown {
			t.Fatalf("2 + first-post = %d (%v), want 3 (Known)", got, TierOf(got))
		}
	})

	t.Run("lose a confrontation: -1 crosses Known to Nobody", func(t *testing.T) {
		got := ApplyInfamyDelta(3, InfamyLossConfrontationLoss)
		if got != 2 || TierOf(got) != game.TierNobody {
			t.Fatalf("3 - lose-confrontation = %d (%v), want 2 (Nobody)", got, TierOf(got))
		}
	})

	t.Run("Vanish: -2 crosses Legend to Feared", func(t *testing.T) {
		got := ApplyInfamyDelta(9, InfamyLossVanish)
		if got != 7 || TierOf(got) != game.TierFeared {
			t.Fatalf("9 - Vanish = %d (%v), want 7 (Feared)", got, TierOf(got))
		}
	})
}

// TestVanishWorkedCase reproduces GDD §11.1's own worked example verbatim: a
// Legend at 9 broadcasts through the order phase, Vanishes to 7 during
// resolution, and the end-of-round Feared reveal still fires because it
// reads live post-resolution Infamy — not the pre-Vanish snapshot and not
// the "fresh tracks" trail suppression, a different channel entirely. A
// second Vanish the following round, 7 to 5, finally drops them out of
// Feared and the reveal stops.
func TestVanishWorkedCase(t *testing.T) {
	entryInfamy := 9 // "as the order phase opens" (GDD §11.1)
	if !OrderPhasePositionBroadcastFires(entryInfamy) {
		t.Fatalf("OrderPhasePositionBroadcastFires(9) = false, want true — Legend broadcasts through the order phase")
	}

	afterFirstVanish := ApplyInfamyDelta(entryInfamy, InfamyLossVanish) // 9 -> 7
	if afterFirstVanish != 7 {
		t.Fatalf("9 - Vanish = %d, want 7", afterFirstVanish)
	}
	if !EndOfRoundPositionRevealFires(afterFirstVanish) {
		t.Fatalf("EndOfRoundPositionRevealFires(7) = false, want true — 7 is still Feared, the reveal still fires (GDD §11.1)")
	}

	afterSecondVanish := ApplyInfamyDelta(afterFirstVanish, InfamyLossVanish) // 7 -> 5, next round
	if afterSecondVanish != 5 {
		t.Fatalf("7 - Vanish = %d, want 5", afterSecondVanish)
	}
	if EndOfRoundPositionRevealFires(afterSecondVanish) {
		t.Fatalf("EndOfRoundPositionRevealFires(5) = true, want false — 5 is Known, the reveal stops (GDD §11.1)")
	}
	// The order-phase broadcast for the round the second Vanish happens in
	// already used the entry Infamy from before it (7, still Legend-free —
	// see TestOrderPhaseBroadcastUsesEntryNotLiveInfamy for that value
	// specifically). It is only the round *after* the second Vanish that
	// opens with Infamy 5 and stops broadcasting at all.
	if OrderPhasePositionBroadcastFires(afterSecondVanish) {
		t.Fatalf("OrderPhasePositionBroadcastFires(5) = true, want false — 5 is well below Legend")
	}
}

// TestOrderPhaseBroadcastUsesEntryNotLiveInfamy demonstrates GDD §11.1's two
// different channels can disagree within the same round: a Legend who opens
// the order phase at 9 (broadcast fires) and then Vanishes to 7 mid-round is
// no longer Legend by the time resolution ends, yet the order-phase
// broadcast already fired — Vanish cannot retroactively un-fire it, because
// that predicate is never re-evaluated against the post-Vanish value.
func TestOrderPhaseBroadcastUsesEntryNotLiveInfamy(t *testing.T) {
	entry := 9
	live := ApplyInfamyDelta(entry, InfamyLossVanish) // 7, computed during resolution

	if !OrderPhasePositionBroadcastFires(entry) {
		t.Fatalf("OrderPhasePositionBroadcastFires(entry=9) = false, want true")
	}
	if OrderPhasePositionBroadcastFires(live) {
		t.Fatalf("OrderPhasePositionBroadcastFires(live=7) = true, want false — irrelevant to the round already opened at 9")
	}
}
