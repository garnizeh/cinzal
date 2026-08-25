package telemetry

import (
	"reflect"
	"strings"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// TestRoundActionsAgainstHandComputedFixture checks RoundActions against
// fixtureOrderLog's 18 hand-authored orders (fixture_test.go), tallied by
// hand here, independent of the implementation:
//
// Stance, all 18 orders: Aggressive at r1s0, r2s0, r3s2, r4s2, r5s1, r6s0
// (6); Neutral at r1s1, r2s2, r3s1, r4s0, r5s2 (5); Evasive at r1s2, r2s1,
// r3s0, r4s1, r5s0, r6s1, r6s2 (7). 6+5+7 == 18.
//
// Action, all 18 orders: Pickup at r1s0, r4s2, r6s1 (3); Deliver at r2s1,
// r5s0 (2); Stake Post at r2s2, r5s2 (2); Deal at r2s0, r3s2, r4s0, r5s1
// (4); Vanish at r3s0, r6s2 (2); Surveil at r1s1, r4s1, r6s0 (3); Nothing
// at r1s2, r3s1 (2). 3+2+2+4+2+3+2 == 18.
//
// Item, only the 4 Deal orders above: Shiv at r2s0, r3s2 (2); Muscle at
// r4s0 (1); Police Band at r5s1 (1); every other item 0.
//
// Ledger (AddOns.BuyLedger == true): r2s1, r4s0, r5s0, r5s1, r5s2 — 5
// total, by round [0, 1, 0, 1, 3, 0].
//
// playerRounds == 3 players * 6 rounds == 18 throughout.
func TestRoundActionsAgainstHandComputedFixture(t *testing.T) {
	got, err := RoundActions(fixtureState(), fixtureOrderLog(), fixtureConfig())
	if err != nil {
		t.Fatalf("RoundActions() error = %v, want nil", err)
	}

	const n = 18 // playerRounds
	rate := func(count int) Rate { return Rate{Value: float64(count) / float64(n), N: n} }

	want := RoundActionSummary{
		StanceDistribution: map[game.Stance]Rate{
			game.StanceAggressive: rate(6),
			game.StanceNeutral:    rate(5),
			game.StanceEvasive:    rate(7),
		},
		LedgerPurchaseRate: rate(5),
		LedgerPurchasesByRound: []Rate{
			{Value: 0, N: 3},         // round 1: 0 of 3 players
			{Value: 1.0 / 3.0, N: 3}, // round 2: 1 of 3 players
			{Value: 0, N: 3},         // round 3: 0 of 3 players
			{Value: 1.0 / 3.0, N: 3}, // round 4: 1 of 3 players
			{Value: 1, N: 3},         // round 5: 3 of 3 players
			{Value: 0, N: 3},         // round 6: 0 of 3 players
		},
		ActionFrequency: map[game.ActionKind]Rate{
			game.ActionPickup:    rate(3),
			game.ActionDeliver:   rate(2),
			game.ActionStakePost: rate(2),
			game.ActionDeal:      rate(4),
			game.ActionVanish:    rate(2),
			game.ActionSurveil:   rate(3),
			game.ActionNothing:   rate(2),
		},
		ItemPurchaseFrequency: map[game.ItemID]Rate{
			game.ItemShiv:              rate(2),
			game.ItemMuscle:            rate(1),
			game.ItemPoliceBand:        rate(1),
			game.ItemCirculationPermit: rate(0),
			game.ItemTornMap:           rate(0),
			game.ItemDecoy:             rate(0),
			game.ItemBoltHole:          rate(0),
			game.ItemGuardContact:      rate(0),
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("RoundActions() = %+v, want %+v", got, want)
	}
}

// TestRoundActionsIsDeterministic mirrors TestMatchIsDeterministic: nothing
// about ranging OrderLog's two map levels (round_action.go's own fold) may
// leak into the result.
func TestRoundActionsIsDeterministic(t *testing.T) {
	s, log, cfg := fixtureState(), fixtureOrderLog(), fixtureConfig()

	first, err := RoundActions(s, log, cfg)
	if err != nil {
		t.Fatalf("RoundActions() error = %v, want nil", err)
	}
	second, err := RoundActions(s, log, cfg)
	if err != nil {
		t.Fatalf("RoundActions() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("RoundActions() is not deterministic: first = %+v, second = %+v", first, second)
	}
}

// TestRoundActionsFailsClosed is issue #198's own acceptance criterion: "a
// frequency over a zero denominator is an error, not 0%." RoundActions
// enforces this by rejecting every structurally degenerate input up front
// — the same four shapes Match itself rejects — and by rejecting a log
// whose contents fall outside the closed domains this package's fold
// counts against (a round key outside 1..cfg.Rounds, or a Stance/
// ActionKind/ItemID this package does not enumerate), so a rename or a
// misplaced guard in RoundActions' own precondition checks is caught by a
// distinguishing substring of each error rather than a bare "err != nil"
// that would still pass if two checks traded places.
func TestRoundActionsFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		state   rules.MatchState
		log     rules.OrderLog
		cfg     game.Config
		wantErr string
	}{
		{
			name:    "cfg.Rounds is zero",
			state:   fixtureState(),
			log:     fixtureOrderLog(),
			cfg:     game.Config{Rounds: 0},
			wantErr: "cfg.Rounds is zero",
		},
		{
			name: "match did not reach cfg.Rounds",
			state: func() rules.MatchState {
				s := fixtureState()
				s.Round = 3
				return s
			}(),
			log:     fixtureOrderLog(),
			cfg:     fixtureConfig(),
			wantErr: "did not finish",
		},
		{
			name: "no players",
			state: func() rules.MatchState {
				s := fixtureState()
				s.Players = nil
				return s
			}(),
			log:     fixtureOrderLog(),
			cfg:     fixtureConfig(),
			wantErr: "no players",
		},
		{
			name:    "empty order log",
			state:   fixtureState(),
			log:     nil,
			cfg:     fixtureConfig(),
			wantErr: "no order was ever submitted",
		},
		{
			name:  "round key outside 1..cfg.Rounds",
			state: fixtureState(),
			log: func() rules.OrderLog {
				log := fixtureOrderLog()
				log[7] = map[game.SeatID]game.Order{0: {
					Action: game.ActionOrder{Kind: game.ActionNothing},
					Stance: game.StanceOrder{Stance: game.StanceNeutral},
				}}
				return log
			}(),
			cfg:     fixtureConfig(),
			wantErr: "outside 1-6",
		},
		{
			name:  "invalid Stance",
			state: fixtureState(),
			log: func() rules.OrderLog {
				log := fixtureOrderLog()
				log[1][0] = game.Order{Action: game.ActionOrder{Kind: game.ActionNothing}} // Stance zero value
				return log
			}(),
			cfg:     fixtureConfig(),
			wantErr: "invalid Stance",
		},
		{
			name:  "invalid Action",
			state: fixtureState(),
			log: func() rules.OrderLog {
				log := fixtureOrderLog()
				log[1][0] = game.Order{Stance: game.StanceOrder{Stance: game.StanceNeutral}} // Action zero value
				return log
			}(),
			cfg:     fixtureConfig(),
			wantErr: "invalid Action",
		},
		{
			name:  "Deal with an invalid Item",
			state: fixtureState(),
			log: func() rules.OrderLog {
				log := fixtureOrderLog()
				log[1][0] = game.Order{
					Action: game.ActionOrder{Kind: game.ActionDeal}, // Item zero value
					Stance: game.StanceOrder{Stance: game.StanceNeutral},
				}
				return log
			}(),
			cfg:     fixtureConfig(),
			wantErr: "invalid Item",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RoundActions(tc.state, tc.log, tc.cfg)
			if err == nil {
				t.Fatalf("RoundActions() error = nil, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("RoundActions() error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
			if !reflect.DeepEqual(got, RoundActionSummary{}) {
				t.Errorf("RoundActions() = %+v on error, want the zero RoundActionSummary", got)
			}
		})
	}
}

// TestActionFrequencyGap checks the diff arithmetic directly, independent
// of any bot tier: b's frequency minus a's, for every ActionKind, using
// the fixture's own ActionFrequency against a second, hand-built map that
// zeroes out Deal and doubles Surveil — deliberately not itself a
// plausible tier's output, only a small, exact-by-construction check of
// the subtraction. b also omits game.ActionNothing entirely, pinning the
// documented fallback: a missing key reads as the zero Rate, so that
// entry's gap is 0 minus a's own value.
func TestActionFrequencyGap(t *testing.T) {
	got, err := RoundActions(fixtureState(), fixtureOrderLog(), fixtureConfig())
	if err != nil {
		t.Fatalf("RoundActions() error = %v, want nil", err)
	}
	a := got.ActionFrequency // Deal: 4/18, Surveil: 3/18, Nothing: 2/18

	b := map[game.ActionKind]Rate{
		game.ActionPickup:    {Value: 3.0 / 18.0, N: 18},
		game.ActionDeliver:   {Value: 2.0 / 18.0, N: 18},
		game.ActionStakePost: {Value: 2.0 / 18.0, N: 18},
		game.ActionDeal:      {Value: 0, N: 18},
		game.ActionVanish:    {Value: 2.0 / 18.0, N: 18},
		game.ActionSurveil:   {Value: 6.0 / 18.0, N: 18},
		// game.ActionNothing is deliberately absent.
	}

	gap := ActionFrequencyGap(a, b)

	want := map[game.ActionKind]float64{
		game.ActionPickup:    0,
		game.ActionDeliver:   0,
		game.ActionStakePost: 0,
		game.ActionDeal:      0 - 4.0/18.0,
		game.ActionVanish:    0,
		game.ActionSurveil:   3.0 / 18.0,
		game.ActionNothing:   0 - 2.0/18.0, // b's missing entry reads as the zero Rate
	}

	if !reflect.DeepEqual(gap, want) {
		t.Errorf("ActionFrequencyGap() = %+v, want %+v", gap, want)
	}
}
