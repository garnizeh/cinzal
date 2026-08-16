package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// selfStateWithInfamy builds a minimal game.PlayerView for the given Infamy
// and step modifiers — everything Steps reads and nothing else.
func selfStateWithInfamy(infamy int, m game.StepModifiers) game.PlayerView {
	return game.PlayerView{
		You: game.SelfState{
			Infamy:        infamy,
			StepModifiers: m,
		},
	}
}

// TestStepsWorkedWorstCase pins GDD §9.1a's own worked example verbatim: "a
// Legend (base 2) who lost an Evasive confrontation, during a Curfew, while
// Flagged, sums to −1 — and floors to 1."
func TestStepsWorkedWorstCase(t *testing.T) {
	cfg := game.DefaultConfig()
	v := selfStateWithInfamy(9, game.StepModifiers{
		Flagged:            true,
		Curfew:             true,
		EvasiveStepPenalty: true,
	})

	got := Steps(v, cfg)
	if got != 1 {
		t.Fatalf("Steps() = %d, want 1 (GDD §9.1a worked worst case)", got)
	}
}

// TestStepsSingleModifiers exercises every one of §9.1a's six modifiers in
// isolation against a Nobody-tier base (4), pinning both direction and
// magnitude.
func TestStepsSingleModifiers(t *testing.T) {
	cfg := game.DefaultConfig()

	tests := []struct {
		name string
		mods game.StepModifiers
		want int
	}{
		{"none", game.StepModifiers{}, 4},
		{"Flagged", game.StepModifiers{Flagged: true}, 3},
		{"Curfew", game.StepModifiers{Curfew: true}, 3},
		{"EvasiveStepPenalty", game.StepModifiers{EvasiveStepPenalty: true}, 3},
		{"DistractedGuard", game.StepModifiers{DistractedGuard: true}, 5},
		{"Scaffolding", game.StepModifiers{Scaffolding: true}, 5},
		{"Retainer", game.StepModifiers{Retainer: true}, 6},
	}

	for _, tc := range tests {
		v := selfStateWithInfamy(0, tc.mods)
		if got := Steps(v, cfg); got != tc.want {
			t.Errorf("%s: Steps() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestStepsEveryTierBase pins the four Infamy-tier bases (GDD §9.1's table)
// with no modifiers applied.
func TestStepsEveryTierBase(t *testing.T) {
	cfg := game.DefaultConfig()

	tests := []struct {
		infamy int
		want   int
	}{
		{0, 4}, {2, 4}, // Nobody
		{3, 4}, {5, 4}, // Known
		{6, 3}, {8, 3}, // Feared
		{9, 2}, {10, 2}, // Legend
	}

	for _, tc := range tests {
		v := selfStateWithInfamy(tc.infamy, game.StepModifiers{})
		if got := Steps(v, cfg); got != tc.want {
			t.Errorf("Infamy %d: Steps() = %d, want %d", tc.infamy, got, tc.want)
		}
	}
}

// TestStepsPinnedToNobodyUnderSuppressInfamyTiers is D11's consequence for
// Suppress.InfamyTiers: the tier lookup always returns the Nobody row,
// regardless of actual Infamy — a Legend-Infamy seat (9) still gets the
// flat Nobody base (4), not Legend's (2) (issue #158). GDD §11's Gains and
// Losses table is unmodified by this: only the ladder-derived step base is
// gated, exactly as ApplyInfamyDelta's own doc already states.
func TestStepsPinnedToNobodyUnderSuppressInfamyTiers(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Suppress.InfamyTiers = true

	v := selfStateWithInfamy(9, game.StepModifiers{})
	if got := Steps(v, cfg); got != 4 {
		t.Fatalf("Steps() at Infamy 9 under Suppress.InfamyTiers = %d, want 4 (flat Nobody base)", got)
	}
}

// TestStepsFloorAppliedOnceAndLast checks the floor lands on 1, never lower,
// even when the modifier sum goes well past -1 — "the floor is applied
// exactly once, at the end" (GDD §9.1a).
func TestStepsFloorAppliedOnceAndLast(t *testing.T) {
	cfg := game.DefaultConfig()
	v := selfStateWithInfamy(9, game.StepModifiers{
		Flagged:            true,
		Curfew:             true,
		EvasiveStepPenalty: true,
	})
	// Legend base 2, minus 3, would be -1 without a floor — and there is no
	// fourth negative modifier to push it further, so this doubles as the
	// worst case. The assertion is exact equality to 1, not merely >= 1.
	if got := Steps(v, cfg); got != 1 {
		t.Fatalf("Steps() = %d, want floored to 1", got)
	}
}

// TestStepsStreetsBlockedHardCap pins GDD §9.1a: "Caps are not modifiers and
// are applied after the floor, so Streets Blocked produces exactly 1 step no
// matter what the sum was." A large positive sum must still cap at 1.
func TestStepsStreetsBlockedHardCap(t *testing.T) {
	cfg := game.DefaultConfig()
	v := selfStateWithInfamy(0, game.StepModifiers{
		DistractedGuard: true,
		Scaffolding:     true,
		Retainer:        true,
		StreetsBlocked:  true,
	})
	// Base 4 + 1 + 1 + 2 = 8 without the cap.
	if got := Steps(v, cfg); got != 1 {
		t.Fatalf("Steps() = %d, want 1 (Streets Blocked hard cap, sum was 8)", got)
	}
}

// TestStepsReadsEntrySnapshotNotLiveState is RFC §16.1's entry-snapshot row:
// "Win a confrontation on step 1 as a Known player; assert the step
// allowance for that round does not drop mid-route." Since Resolve doesn't
// exist yet, this pins the discipline a future Project must follow instead:
// a PlayerView built from rules.EntrySnapshot's frozen values gives the
// pre-round answer, while one built from the live, already-mutated Player
// (as if fed straight from mid-resolution state) gives the wrong one. Steps
// itself is not the bug surface here — it trusts whatever view it's given —
// so this test is the regression guard for whoever writes Project.
func TestStepsReadsEntrySnapshotNotLiveState(t *testing.T) {
	cfg := game.DefaultConfig()

	s := MatchState{
		Round: 5,
		Players: []Player{
			{Seat: 0, Infamy: 4, Flagged: true}, // Known tier entering the round
		},
	}
	entry := s.Snapshot()

	// Mid-round: this seat wins a confrontation on step 1 and jumps to
	// Feared tier (6) — simulated by mutating the live Player directly,
	// since Resolve doesn't exist yet to do it for real.
	s.Players[0].Infamy = 6

	fromSnapshot := selfStateWithInfamy(entry.Seats[0].Infamy, game.StepModifiers{
		Flagged: entry.Seats[0].Flagged,
	})
	fromLiveState := selfStateWithInfamy(s.Players[0].Infamy, game.StepModifiers{
		Flagged: s.Players[0].Flagged,
	})

	gotSnapshot := Steps(fromSnapshot, cfg)
	gotLive := Steps(fromLiveState, cfg)

	if gotSnapshot != 3 {
		t.Fatalf("Steps(view built from entry snapshot) = %d, want 3 (Known tier 4, -1 Flagged)", gotSnapshot)
	}
	if gotLive == gotSnapshot {
		t.Fatalf("expected the live-state view (Feared tier, post-confrontation) to diverge from the "+
			"entry-snapshot view — both gave %d; this test only proves anything if they differ, which is "+
			"exactly why Project must build views from EntrySnapshot, never from live mid-round Player state", gotSnapshot)
	}
}
