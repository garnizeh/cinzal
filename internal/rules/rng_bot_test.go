package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// TestNewBotRNGIsPureFunctionOfSeedSeatRound pins D32's core determinism
// property: two BotRNGs constructed with the same (matchSeed, seat, round),
// driven through the same sequence of purpose/n calls, must produce the same
// sequence of draws — the property cmd/simulate's seed-only sweeps depend on.
func TestNewBotRNGIsPureFunctionOfSeedSeatRound(t *testing.T) {
	seed := testSeed(0x9a)

	calls := []struct {
		purpose BotPurpose
		n       int
	}{
		{"bot.drifter.route", 5},
		{"bot.drifter.action", 3},
		{"bot.runner.route", 7},
	}

	a := NewBotRNG(seed, game.SeatID(2), 6)
	b := NewBotRNG(seed, game.SeatID(2), 6)

	for i, c := range calls {
		got := a.NextBot(c.purpose, c.n)
		want := b.NextBot(c.purpose, c.n)
		if got != want {
			t.Fatalf("call %d: NextBot(%q, %d) diverged between identically-derived BotRNGs: %d != %d", i, c.purpose, c.n, got, want)
		}
	}
}

// TestNewBotRNGSeatsDoNotShareAStream asserts RFC-001 §14.5's non-collusion
// rule holds by construction: two seats in the same match, same round, must
// draw from streams that diverge — enforced by the seat forming part of the
// seed derivation (D32), not by review.
func TestNewBotRNGSeatsDoNotShareAStream(t *testing.T) {
	seed := testSeed(0x9b)

	seat0 := NewBotRNG(seed, game.SeatID(0), 4)
	seat1 := NewBotRNG(seed, game.SeatID(1), 4)

	if seat0.NextBot("bot.drifter.route", 1<<30) == seat1.NextBot("bot.drifter.route", 1<<30) {
		t.Error("two seats' first draw is identical in the same round; seat is not part of the derivation")
	}
}

// TestNewBotRNGRoundsDoNotShareAStream asserts a fixed seat's stream still
// varies round to round — the same per-round independence NewRNG already
// gives Resolve's own stream, inherited here by construction since
// NewBotRNG reuses NewRNG internally rather than re-deriving it (D32).
func TestNewBotRNGRoundsDoNotShareAStream(t *testing.T) {
	seed := testSeed(0x9c)

	round1 := NewBotRNG(seed, game.SeatID(3), 1)
	round2 := NewBotRNG(seed, game.SeatID(3), 2)

	if round1.NextBot("bot.runner.route", 1<<30) == round2.NextBot("bot.runner.route", 1<<30) {
		t.Error("one seat's first draw is identical across rounds 1 and 2; round is not part of the derivation")
	}
}

// TestNewBotRNGDivergesFromResolveStream asserts a bot's draws never land on
// the same values Resolve's own *RNG would produce from the plain match
// seed — the executable form of "bot draws never touch Resolve's *RNG"
// (D32): NewBotRNG derives a distinct seed via HMAC before ever constructing
// the inner *RNG, so the two streams are unrelated even when every other
// input (seed, round, purpose, n) is held equal.
func TestNewBotRNGDivergesFromResolveStream(t *testing.T) {
	seed := testSeed(0x9d)

	matchRNG := NewRNG(seed, 7)
	botRNG := NewBotRNG(seed, game.SeatID(0), 7)

	if matchRNG.Next(PurposeCrateNode, 1<<30) == botRNG.NextBot(BotPurpose(PurposeCrateNode), 1<<30) {
		t.Error("Resolve's *RNG and a BotRNG for seat 0 produced the same first draw from the same match seed; the bot stream is not actually distinct")
	}
}

// TestNextBotPanicsOnNonPositiveN mirrors TestNextPanicsOnNonPositiveN for
// the bot-facing draw method: a silent 0 would be an index into an empty
// candidate set, so NextBot must panic rather than return one.
func TestNextBotPanicsOnNonPositiveN(t *testing.T) {
	for _, n := range []int{0, -1, -100} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NextBot(purpose, %d) did not panic", n)
				}
			}()
			NewBotRNG(testSeed(1), game.SeatID(0), 1).NextBot("bot.drifter.route", n)
		}()
	}
}

// TestNextBotConsumptionIsInvisibleToConsumed pins §16.2's invariant
// unconditionally: BotRNG draws must never appear on the match-stream
// consumption accounting the golden replay suite checks per round, no
// matter how many draws a bot makes.
func TestNextBotConsumptionIsInvisibleToConsumed(t *testing.T) {
	matchRNG := NewRNG(testSeed(0x9e), 1)
	botRNG := NewBotRNG(testSeed(0x9e), game.SeatID(0), 1)

	botRNG.NextBot("bot.operator.search", 4)
	botRNG.NextBot("bot.operator.search", 4)

	for _, p := range []Purpose{PurposeConfrontD6, PurposeCrateNode, Purpose("bot.operator.search")} {
		if got := matchRNG.Consumed(p); got != 0 {
			t.Errorf("Resolve's own RNG reports Consumed(%q) = %d after only BotRNG draws; bot draws must be invisible to Resolve's stream", p, got)
		}
	}
}
