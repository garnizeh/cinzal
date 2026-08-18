package bots

import (
	"fmt"
	"math"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// drifterDistributionFixture builds a small, fully-Known hub-and-spoke
// graph — two neighbours of a hub node, a one-step allowance — so
// Routes(v, cfg) returns a handful of candidates rather than the dozens
// fixtureView's default step budget would allow, keeping this file's
// distribution test's legal-order space small enough to sample
// exhaustively at 10,000 draws. Zero balance and Suppress.Leases (matching
// legalspace_test.go's TestSampleCollapsesUnderMaximalConstraint fixture)
// pin the add-ons, the abandon-cargo flag and the contract choice each to
// one always-legal value, so every draw's real variation is confined to
// Route, Stance and Action — issue #192's own stated scheme (package doc
// comment, doc.go) drawn one stage at a time.
func drifterDistributionFixture() (game.PlayerView, game.Config) {
	cfg := game.DefaultConfig()
	cfg.Suppress.Leases = true
	cfg.StepsByTier = [4]int{1, 1, 1, 1}

	v := game.PlayerView{
		Round: 4,
		You:   game.SelfState{Position: 1},
		Nodes: map[game.NodeID]game.NodeView{
			1: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{2, 3}},
			2: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{1}},
			3: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{1}},
		},
	}
	return v, cfg
}

// orderSignature is the part of a game.Order this file's distribution test
// varies over. Route is folded to its string form because []game.NodeID is
// not comparable and cannot key a map; PushingOn, Stance and Action are
// left as their real types since drifterDistributionFixture never produces
// a Hidden route ending, so PushingOn's Bias pointer is always nil and the
// not-declared zero value is the only value this fixture ever sees.
type orderSignature struct {
	route   string
	pushing game.PushingOn
	stance  game.StanceOrder
	action  game.ActionOrder
}

func signatureOf(o game.Order) orderSignature {
	return orderSignature{
		route:   fmt.Sprint(o.Route),
		pushing: o.PushingOn,
		stance:  o.Stance,
		action:  o.Action,
	}
}

// stanceSupport enumerates every stance sampleStance (legalspace.go) can
// draw for draft, replicating its own n := 2 + a.MaxStake + 1 formula and
// index-to-value mapping — not calling sampleStance itself, since this
// function needs the whole support to build a theoretical distribution
// over, not one draw from it.
func stanceSupport(v game.PlayerView, cfg game.Config, draft game.Order) []game.StanceOrder {
	a := rules.Affordances(v, cfg, draft)
	support := []game.StanceOrder{
		{Stance: game.StanceNeutral},
		{Stance: game.StanceEvasive},
	}
	for stake := 0; stake <= a.MaxStake; stake++ {
		support = append(support, game.StanceOrder{Stance: game.StanceAggressive, Stake: stake})
	}
	return support
}

// drifterStageWiseProbabilities builds the theoretical distribution the
// package doc comment (doc.go) states Drifter provides: uniform over
// Routes(v, cfg), then — independently per route — uniform over that
// route's PushingOns, then over its stanceSupport, then over Actions for
// the draft built so far. It calls the exact functions Sample
// (legalspace.go) itself draws from, rather than re-deriving GDD legality
// by hand, so this is a test of Sample's *sampling* against its own stated
// candidate sets, not a second implementation of Legal's rules to diff
// against.
//
// Fails closed (issue #192's own criterion): fails the test outright if the
// fixture's route space or its full signature space has one element or
// fewer, since a corpus of only such views would pass any uniformity check
// trivially.
func drifterStageWiseProbabilities(t *testing.T, v game.PlayerView, cfg game.Config) map[orderSignature]float64 {
	t.Helper()

	routes := Routes(v, cfg)
	if len(routes) <= 1 {
		t.Fatalf("fixture's route space has %d element(s), want > 1 to exercise route uniformity", len(routes))
	}
	pRoute := 1.0 / float64(len(routes))

	probs := make(map[orderSignature]float64)
	for _, rt := range routes {
		pushings := PushingOns(v, rt.Nodes)
		pPushing := pRoute / float64(len(pushings))

		for _, pushing := range pushings {
			draft := game.Order{
				Round:     v.Round,
				Route:     rt.Nodes,
				PushingOn: pushing,
				Stance:    game.StanceOrder{Stance: game.StanceNeutral},
			}
			stances := stanceSupport(v, cfg, draft)
			pStance := pPushing / float64(len(stances))

			for _, stance := range stances {
				draft.Stance = stance
				actions := Actions(v, cfg, draft)
				if len(actions) == 0 {
					t.Fatalf("Actions(%+v) returned no candidates — legalActionsOrRelax's fallback should never trigger in this fixture", draft)
				}
				pAction := pStance / float64(len(actions))

				for _, action := range actions {
					sig := orderSignature{route: fmt.Sprint(rt.Nodes), pushing: pushing, stance: stance, action: action}
					probs[sig] += pAction
				}
			}
		}
	}

	if len(probs) <= 1 {
		t.Fatalf("fixture's full legal-order signature space has %d element(s), want > 1 — a corpus of only such views would pass a uniformity check trivially", len(probs))
	}
	return probs
}

// TestDrifterDistributionMatchesStageWiseUniformScheme is issue #192's
// headline acceptance criterion: over 10,000 draws from a fixed view, every
// order in the legal set appears at least once, and the empirical
// distribution matches the stated scheme (doc.go's package doc comment)
// within a documented tolerance.
//
// Each draw uses a fresh BotRNG for a distinct seat against the same
// (matchSeed, round) — never the same BotRNG drawn from repeatedly, which
// would be one Decide call's worth of draws rather than 10,000 independent
// samples of what one Decide call could produce, and never a seed built by
// XOR-ing a single byte (this package's own drawSeed helper, sized for a
// handful of draws in legalspace_test.go's corpus test), which cycles every
// 256 draws and would silently turn most of 10,000 into repeats. Varying
// the seat instead sidesteps both: NewBotRNG derives an independent HMAC
// key per seat, so 10,000 distinct seats are 10,000 genuinely independent
// streams.
//
// The tolerance is six binomial standard deviations plus a flat floor: at
// n=10,000 draws, six sigma keeps this test's false-failure rate
// astronomically low while still catching a sampler that draws some stage
// non-uniformly.
func TestDrifterDistributionMatchesStageWiseUniformScheme(t *testing.T) {
	v, cfg := drifterDistributionFixture()
	probs := drifterStageWiseProbabilities(t, v, cfg)

	const draws = 10000
	seed := testSeed(0x92)
	counts := make(map[orderSignature]int, len(probs))

	for i := range draws {
		r := rules.NewBotRNG(seed, game.SeatID(i), int(v.Round))
		o := For(Drifter).Decide(v, cfg, r)
		if err := rules.Legal(v, o, cfg); err != nil {
			t.Fatalf("draw %d: Drifter produced an illegal order %+v: %v", i, o, err)
		}
		counts[signatureOf(o)]++
	}

	if len(counts) != len(probs) {
		t.Errorf("10,000 draws produced %d distinct signatures, want exactly the %d the fixture's legal set contains", len(counts), len(probs))
	}

	for sig, p := range probs {
		want := p * draws
		sigma := math.Sqrt(draws * p * (1 - p))
		tolerance := 6*sigma + 5

		got, ok := counts[sig]
		if !ok || got == 0 {
			t.Errorf("order %+v never produced in %d draws (stated probability %.4f, expected ~%.1f)", sig, draws, p, want)
			continue
		}
		if diff := math.Abs(float64(got) - want); diff > tolerance {
			t.Errorf("order %+v: got %d draws, want %.1f +/- %.1f (stated probability %.4f)", sig, got, want, tolerance, p)
		}
	}
}

// TestDrifterDecideDeterministicAcrossManyReplays is issue #192's own "same
// view, same RNG state -> identical order, across 1,000 replays" acceptance
// criterion. It doubles as this package's own instance of the
// map-iteration-hazard check internal/rules/resolve_test.go's
// TestResolveIsDeterministic already performs for Resolve: Go's map
// iteration order is randomised fresh on every range statement, not once
// per process, so a candidate set anywhere in Sample's call graph that was
// built by ranging a map without first sorting it — the discipline
// sortedNodeIDs/knownNodeIDs/heardOfNodeIDs/distanceTwoNodeIDs in
// legalspace.go already follow — would make at least one of these 1,000
// replays diverge from the first.
func TestDrifterDecideDeterministicAcrossManyReplays(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x93)
	v := fixtureView(4, 2)

	first := For(Drifter).Decide(v, cfg, rules.NewBotRNG(seed, game.SeatID(0), 4))

	for i := 1; i < 1000; i++ {
		got := For(Drifter).Decide(v, cfg, rules.NewBotRNG(seed, game.SeatID(0), 4))
		if !got.Equal(first) {
			t.Fatalf("replay %d diverged from the first: %+v != %+v", i, got, first)
		}
	}
}

// TestDrifterItemStageConsumesZeroDrawsWhenNoItemsHeld is issue #192's own
// "a view with no items in hand consumes zero item draws" acceptance
// criterion, asserted against the RNG's consumption accounting. BotRNG
// deliberately exposes no public way to read that accounting from outside
// internal/rules (rng_bot_purpose.go: draws made through NextBot are
// invisible to RNG.Consumed, so "§16.2 stays unconditionally true
// regardless of how many seats a round is bot-filled"), so this is the
// check BotRNG's own determinism leaves available from here: two BotRNG
// values built from the identical (seed, seat, round) start in identical
// internal state, and stay identical for as long as neither has been drawn
// from. Calling sampleItems on one and then asking both for the same next
// draw produces the same value if and only if sampleItems consumed
// nothing — any draw at all advances rA's internal seq and desynchronises
// the two HMAC streams, changing the next value either would produce.
func TestDrifterItemStageConsumesZeroDrawsWhenNoItemsHeld(t *testing.T) {
	seed := testSeed(0x94)
	v := game.PlayerView{Round: 4, You: game.SelfState{Position: 1}} // no items held

	rA := rules.NewBotRNG(seed, game.SeatID(0), 4)
	if discards := sampleItems(v, rA); discards != nil {
		t.Fatalf("sampleItems on an empty item hand returned %v, want nil", discards)
	}

	rB := rules.NewBotRNG(seed, game.SeatID(0), 4)
	const probe rules.BotPurpose = "test.probe"
	if got, want := rA.NextBot(probe, 1_000_000), rB.NextBot(probe, 1_000_000); got != want {
		t.Fatalf("sampleItems consumed at least one draw on an empty item hand: probe draws diverged (%d != %d)", got, want)
	}
}
