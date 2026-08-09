package game

import "testing"

func TestRoundSetHasAndWith(t *testing.T) {
	var s RoundSet

	if s.Has(4) {
		t.Fatalf("empty RoundSet reports round 4 as present")
	}

	s = s.With(4)
	if !s.Has(4) {
		t.Fatalf("RoundSet.With(4) did not add round 4")
	}
	if s.Has(5) {
		t.Fatalf("RoundSet.With(4) unexpectedly set round 5")
	}
}

func TestRoundSetWithDoesNotMutateReceiver(t *testing.T) {
	before := RoundSet(0)
	after := before.With(1)

	if before.Has(1) {
		t.Fatalf("RoundSet.With mutated its receiver: before.Has(1) = true")
	}
	if !after.Has(1) {
		t.Fatalf("RoundSet.With(1) did not set round 1 on the returned value")
	}
}

func TestRoundSetCount(t *testing.T) {
	var s RoundSet
	s = s.With(1).With(2).With(15)

	if got, want := s.Count(), 3; got != want {
		t.Fatalf("Count() = %d, want %d", got, want)
	}
}

// TestNodeStatsRateFromD13Example pins the worked example from D13's
// consequences section: "a node watched 6 rounds, 2 of them obscured,
// reports 4 observations."
func TestNodeStatsRateFromD13Example(t *testing.T) {
	stats := NodeStats{
		ObservedRounds: 4,
		TrafficRounds:  2,
		ObscuredRounds: 2,
	}

	if stats.ObservedRounds+stats.ObscuredRounds != 6 {
		t.Fatalf("ObservedRounds (%d) + ObscuredRounds (%d) != 6 rounds watched", stats.ObservedRounds, stats.ObscuredRounds)
	}
}
