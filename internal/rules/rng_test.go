package rules

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

func testSeed(b byte) [32]byte {
	var seed [32]byte
	for i := range seed {
		seed[i] = b
	}
	return seed
}

// seedFromInt derives a [32]byte seed from an ordinary int via SHA-256, for
// property-test sweeps that need many more distinct seeds than testSeed's
// single repeated byte can produce (256 at most). Used wherever a test needs
// to range over >= 1000 seeds — the same shape internal/rules/gen's own
// generate_test.go sweeps use, adapted to *RNG's [32]byte seed rather than
// gen's plain-int Rand.
func seedFromInt(i int) [32]byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(i))
	return sha256.Sum256(buf[:])
}

// TestNextIsPureFunctionOfSeedRoundSeqPurposeN pins the core determinism
// property: two RNGs constructed with the same seed and round, driven
// through the same sequence of purpose/n calls, must produce the same
// sequence of draws.
func TestNextIsPureFunctionOfSeedRoundSeqPurposeN(t *testing.T) {
	seed := testSeed(0x42)

	calls := []struct {
		purpose Purpose
		n       int
	}{
		{PurposeContractOfferTier, 6},
		{PurposeContractOfferPick, 6},
		{PurposeMarketStock, 12},
		{PurposeConfrontD6, 6},
		{PurposeItemTornMap, 9},
	}

	a := NewRNG(seed, 3)
	b := NewRNG(seed, 3)

	for i, c := range calls {
		got := a.Next(c.purpose, c.n)
		want := b.Next(c.purpose, c.n)
		if got != want {
			t.Fatalf("call %d: Next(%q, %d) diverged between identically-seeded RNGs: %d != %d", i, c.purpose, c.n, got, want)
		}
	}
}

// TestNextPanicsOnNonPositiveN guards the "n <= 0 is a programming error"
// acceptance criterion: a silent 0 would be an index into an empty
// candidate set, so Next must panic rather than return one.
func TestNextPanicsOnNonPositiveN(t *testing.T) {
	for _, n := range []int{0, -1, -100} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Next(purpose, %d) did not panic", n)
				}
			}()
			NewRNG(testSeed(1), 1).Next(PurposeCrateNode, n)
		}()
	}
}

// TestNextDerivationIsHMAC asserts the draw is computed from
// HMAC(seed, round || seq || purpose) as RFC-001 §6.4 specifies, rather
// than from any ambient source — recomputing the same digest independently
// and checking it against RNG.Next's output.
func TestNextDerivationIsHMAC(t *testing.T) {
	seed := testSeed(0x7a)
	const round = 5
	const n = 7

	rng := NewRNG(seed, round)
	got := rng.Next(PurposeItemTornMap, n)

	msg := make([]byte, 0, 8+4+len(PurposeItemTornMap))
	msg = binary.BigEndian.AppendUint64(msg, uint64(round))
	msg = binary.BigEndian.AppendUint32(msg, 0) // first draw, seq starts at 0
	msg = append(msg, PurposeItemTornMap...)

	mac := hmac.New(sha256.New, seed[:])
	mac.Write(msg)
	digest := mac.Sum(nil)
	want := int(binary.BigEndian.Uint64(digest[:8]) % n)

	if got != want {
		t.Fatalf("Next(%q, %d) = %d, want %d (HMAC(seed, round||seq||purpose) recomputed independently)", PurposeItemTornMap, n, got, want)
	}
}

// TestSeqIncrementsPerDraw asserts sequence indices are consumed one per
// call, in execution order — the property the whole index-accounting
// design rests on.
func TestSeqIncrementsPerDraw(t *testing.T) {
	rng := NewRNG(testSeed(2), 1)

	for i := uint32(0); i < 5; i++ {
		if rng.Seq() != i {
			t.Fatalf("Seq() = %d before draw %d, want %d", rng.Seq(), i, i)
		}
		rng.Next(PurposeCrateNode, 4)
	}
	if rng.Seq() != 5 {
		t.Fatalf("Seq() = %d after 5 draws, want 5", rng.Seq())
	}
}

// TestConsumedIsQueryablePerPurpose asserts consumption accounting is
// queryable per purpose (RFC-001 §16.2's invariant needs exactly this to
// assert against).
func TestConsumedIsQueryablePerPurpose(t *testing.T) {
	rng := NewRNG(testSeed(3), 1)

	rng.Next(PurposeConfrontD6, 6)
	rng.Next(PurposeConfrontD6, 6)
	rng.Next(PurposeConfrontD6, 6)
	rng.Next(PurposeConfrontTiebreak, 6)

	if got := rng.Consumed(PurposeConfrontD6); got != 3 {
		t.Errorf("Consumed(confront.d6) = %d, want 3", got)
	}
	if got := rng.Consumed(PurposeConfrontTiebreak); got != 1 {
		t.Errorf("Consumed(confront.tiebreak) = %d, want 1", got)
	}
	if got := rng.Consumed(PurposePressureD6); got != 0 {
		t.Errorf("Consumed(pressure.d6) = %d, want 0 (never drawn)", got)
	}
}

// TestConsumedIsQueryablePerRound asserts the "per round" half of the same
// acceptance criterion: an RNG is bound to one round, so accounting for a
// different round lives on a different RNG instance and does not bleed in.
func TestConsumedIsQueryablePerRound(t *testing.T) {
	seed := testSeed(4)

	round1 := NewRNG(seed, 1)
	round1.Next(PurposePressureD6, 6)
	round1.Next(PurposePressureD6, 6)

	round2 := NewRNG(seed, 2)
	round2.Next(PurposePressureD6, 6)

	if got := round1.Consumed(PurposePressureD6); got != 2 {
		t.Errorf("round 1 Consumed(pressure.d6) = %d, want 2", got)
	}
	if got := round2.Consumed(PurposePressureD6); got != 1 {
		t.Errorf("round 2 Consumed(pressure.d6) = %d, want 1", got)
	}
	if round1.Round() != 1 || round2.Round() != 2 {
		t.Fatalf("Round() = (%d, %d), want (1, 2)", round1.Round(), round2.Round())
	}

	// The round is part of the derivation, so identically-seeded RNGs bound
	// to different rounds must not draw the same first value — a regression
	// that dropped r.round from the HMAC message would pass every assertion
	// above and still desynchronise every round after the first.
	if NewRNG(seed, 1).Next(PurposePressureD6, 1<<30) == NewRNG(seed, 2).Next(PurposePressureD6, 1<<30) {
		t.Errorf("first draw is identical across rounds 1 and 2; round is not part of the derivation")
	}
}
