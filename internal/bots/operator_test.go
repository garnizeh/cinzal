package bots

import (
	"fmt"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
	"github.com/garnizeh/cinzal/internal/rules"
)

// chokepointFixtureView is the two-node graph this file's two leasing tests
// share: the seat stands on node 1, node 2 is one Known step away and is the
// only candidate a NodeStats entry can nominate as a chokepoint, and the
// balance is ample so nothing here is decided by affordability.
//
// Others carries one opponent because legalPostCap (internal/rules/legal.go)
// reads players as len(v.Others)+1, and DefaultConfig's PostCapByPlayers has
// no entry for a 1-player match — a fixture without it would make
// ActionStakePost illegal for a reason neither test means to exercise.
func chokepointFixtureView() game.PlayerView {
	return game.PlayerView{
		Round: 4,
		You:   game.SelfState{Position: 1, Balance: 100},
		Nodes: map[game.NodeID]game.NodeView{
			1: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{2}},
			2: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{1}},
		},
		Others: []game.OpponentView{{Seat: 1}},
	}
}

// TestOperatorLeasesChokepoint is RFC-001 §14.3's "reads the heat map for
// chokepoints and leases them" (GDD §7.5), fired both ways: a node whose
// NodeStats clears DefaultOperatorOptions' own ChokepointTrafficRate and
// ChokepointMinObserved is routed to and staked when nothing more
// productive is happening; a node that falls short of either is not.
func TestOperatorLeasesChokepoint(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x60)
	r := rules.NewBotRNG(seed, game.SeatID(0), 4)
	opts := DefaultOperatorOptions()

	newView := chokepointFixtureView

	// observed and traffic are built from opts itself, not hardcoded
	// numbers, so this test tracks DefaultOperatorOptions' own values
	// rather than silently drifting stale against a future retune.
	observed := opts.ChokepointMinObserved + 1

	chokepoint := newView()
	chokepoint.NodeStats = map[game.NodeID]game.NodeStats{
		2: {ObservedRounds: observed, TrafficRounds: observed}, // 100% traffic, one more than the confidence floor
	}
	got := For(Operator).Decide(chokepoint, cfg, r)
	if len(got.Route) != 1 || got.Route[0] != 2 || got.Action.Kind != game.ActionStakePost {
		t.Fatalf("qualifying chokepoint at node 2: Route=%v Action=%v, want Route=[2] Action=StakePost", got.Route, got.Action)
	}

	belowRateTraffic := int(float64(observed) * opts.ChokepointTrafficRate / 2)
	belowRate := newView()
	belowRate.NodeStats = map[game.NodeID]game.NodeStats{
		2: {ObservedRounds: observed, TrafficRounds: belowRateTraffic}, // half the threshold rate, despite ample observations
	}
	if got := For(Operator).Decide(belowRate, cfg, r).Action.Kind; got == game.ActionStakePost {
		t.Fatalf("traffic rate %d/%d (below ChokepointTrafficRate %.2f): Action = StakePost, want anything else",
			belowRateTraffic, observed, opts.ChokepointTrafficRate)
	}

	lowConfidence := newView()
	lowConfidence.NodeStats = map[game.NodeID]game.NodeStats{
		2: {ObservedRounds: opts.ChokepointMinObserved - 1, TrafficRounds: opts.ChokepointMinObserved - 1}, // 100%, one observation short of the confidence floor
	}
	if got := For(Operator).Decide(lowConfidence, cfg, r).Action.Kind; got == game.ActionStakePost {
		t.Fatalf("only %d observations (below ChokepointMinObserved %d): Action = StakePost, want anything else",
			opts.ChokepointMinObserved-1, opts.ChokepointMinObserved)
	}
}

// TestOperatorAvoidsUnstableSectorWeightedByDeck is RFC-001 §14.3's "routes
// around unstable sectors weighted by the displayed deck counts" (GDD
// §14.3), fired both ways over one diamond graph: node 1 reaches a held
// contract's Origin (node 5) through either node 2 (the Unstable Sector) or
// node 6 (a different sector), both one step away and both distance 1 from
// the objective on the known subgraph — so absent any sector weighting the
// ascending-NodeID tie-break (RFC-001 §6.5) would pick node 2. This
// round's own step budget is capped at 1 (cfg.StepsByTier all set to 1), so
// neither branch can reach node 5 itself this round — the choice is
// genuinely between the two intermediate nodes.
func TestOperatorAvoidsUnstableSectorWeightedByDeck(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.StepsByTier = [4]int{1, 1, 1, 1}
	seed := testSeed(0x61)
	r := rules.NewBotRNG(seed, game.SeatID(0), 4)

	unstable := game.SectorNorthVale
	safe := game.SectorOldDocks

	newView := func() game.PlayerView {
		return game.PlayerView{
			Round: 4,
			You: game.SelfState{
				Position:  1,
				Balance:   100,
				Contracts: []game.ContractInHand{{ID: 1, Origin: 5, Destination: 1, Tier: 0, ExpiresRound: 15}},
			},
			Nodes: map[game.NodeID]game.NodeView{
				1: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{2, 6}},
				2: {Fog: game.FogKnown, Type: game.NodeWarehouse, Sector: unstable, Edges: []game.NodeID{1, 5}},
				6: {Fog: game.FogKnown, Type: game.NodeWarehouse, Sector: safe, Edges: []game.NodeID{1, 5}},
				5: {Fog: game.FogKnown, Type: game.NodeWarehouse, Edges: []game.NodeID{2, 6}},
			},
			Headline: game.Headline{Sector: &unstable},
		}
	}

	hazardHeavy := newView()
	hazardHeavy.Deck = game.DeckCounts{HazardsRemaining: 9, BoonsRemaining: 0}
	got := For(Operator).Decide(hazardHeavy, cfg, r)
	if len(got.Route) != 1 || got.Route[0] != 6 {
		t.Fatalf("deck all-hazard: Route = %v, want [6] (the safe-sector branch, despite node 2's lower NodeID)", got.Route)
	}

	boonHeavy := newView()
	boonHeavy.Deck = game.DeckCounts{HazardsRemaining: 0, BoonsRemaining: 4}
	got = For(Operator).Decide(boonHeavy, cfg, r)
	if len(got.Route) != 1 || got.Route[0] != 2 {
		t.Fatalf("deck all-boon: Route = %v, want [2] (an exhausted hazard deck earns no detour, so the ascending-NodeID tie-break applies)", got.Route)
	}
}

// TestOperatorTimesInfamyAgainstCooldown is RFC-001 §14.3's "times the
// Infamy climb against Contact Cooldown" (GDD §11.1), fired both ways at
// Infamy 9 (above DefaultOperatorOptions' own InfamyComfortBand of 8):
// with the next contract offer imminent (RoundsToNextOffer at or under
// InfamyCooldownMargin), Operator holds off Vanishing to cash in the
// current tier on that offer; with the next offer not imminent, it
// Vanishes exactly as Runner's own comfort-band check would.
func TestOperatorTimesInfamyAgainstCooldown(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x62)
	r := rules.NewBotRNG(seed, game.SeatID(0), 4)

	base := fixtureView(4, 2) // no cargo, no contracts, no chokepoint, no market — nothing else to do
	base.You.Infamy = 9

	offerImminent := base
	offerImminent.You.RoundsToNextOffer = 1
	if got := For(Operator).Decide(offerImminent, cfg, r).Action.Kind; got == game.ActionVanish {
		t.Fatal("RoundsToNextOffer 1 (within InfamyCooldownMargin): Action = Vanish, want anything else")
	}

	offerFar := base
	offerFar.You.RoundsToNextOffer = 5
	if got := For(Operator).Decide(offerFar, cfg, r).Action.Kind; got != game.ActionVanish {
		t.Fatalf("RoundsToNextOffer 5 (past InfamyCooldownMargin), Infamy 9 (above the comfort band): Action = %v, want Vanish", got)
	}
}

// TestOperatorBuysItemsUnderThreat is RFC-001 §14.3's "buys items when a
// confrontation looks likely," fired both ways at one isolated Black
// Market node (no edges, so Routes only ever offers the empty
// stay-put route — routeEndpoint is the position itself regardless of
// tie-break, isolating the purchase decision from route choice): a Feared
// opponent (Infamy 6+, GDD §11's own combat-modifier threshold) clears
// threatEstimate, an ordinary one does not.
func TestOperatorBuysItemsUnderThreat(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x63)
	r := rules.NewBotRNG(seed, game.SeatID(0), 4)

	newView := func() game.PlayerView {
		return game.PlayerView{
			Round: 4,
			You:   game.SelfState{Position: 1, Balance: 100},
			Nodes: map[game.NodeID]game.NodeView{
				1: {Fog: game.FogInSight, Type: game.NodeBlackMarket, Market: []game.ItemID{game.ItemMuscle, game.ItemPoliceBand}},
			},
		}
	}

	threatened := newView()
	threatened.Others = []game.OpponentView{{Seat: 1, Infamy: 7}} // Feared
	got := For(Operator).Decide(threatened, cfg, r)
	if got.Action.Kind != game.ActionDeal || got.Action.Item != game.ItemMuscle {
		t.Fatalf("a Feared opponent present: Action = %+v, want Deal(Muscle) — operatorItemPreference's top choice", got.Action)
	}

	quiet := newView()
	if got := For(Operator).Decide(quiet, cfg, r).Action.Kind; got == game.ActionDeal {
		t.Fatal("no opponents, no recent confrontations: Action = Deal, want anything else")
	}
}

// TestOperatorBuysLedgerOnRivalBandJump is RFC-001 §14.3's "uses the Ledger
// when a rival's band jumps," fired both ways against the only signal
// issue #190's no-memory rule leaves available: a high-tier EventDelivered
// anchor by an opponent, within BandJumpLookback rounds, in the
// match-to-date v.Anchors log. A Tier below BandJumpMinTier is not treated
// as jump evidence.
func TestOperatorBuysLedgerOnRivalBandJump(t *testing.T) {
	cfg := game.DefaultConfig()
	seed := testSeed(0x64)
	r := rules.NewBotRNG(seed, game.SeatID(0), 4)
	rival := game.SeatID(1)

	newView := func(tier int) game.PlayerView {
		return game.PlayerView{
			Round: 4,
			You:   game.SelfState{Position: 1, Balance: 100},
			Nodes: map[game.NodeID]game.NodeView{
				1: {Fog: game.FogKnown, Type: game.NodeWarehouse},
			},
			Others:  []game.OpponentView{{Seat: rival}},
			Anchors: []game.Anchor{{Kind: game.EventDelivered, Round: 3, Node: 9, Actor: &rival, Tier: tier}},
		}
	}

	jump := newView(3) // Tier IV, well above BandJumpMinTier (2)
	if got := For(Operator).Decide(jump, cfg, r).AddOns; !got.BuyLedger {
		t.Fatalf("a Tier IV delivery by an opponent one round ago: AddOns = %+v, want BuyLedger", got)
	}

	noJump := newView(1) // Tier II — below BandJumpMinTier
	if got := For(Operator).Decide(noJump, cfg, r).AddOns; got.BuyLedger {
		t.Fatalf("a Tier II delivery (below BandJumpMinTier): AddOns = %+v, want no Ledger purchase", got)
	}
}

// TestOperatorNeverIllegalOverGoldenCorpus is issue #194's own acceptance
// criterion: "Operator never emits an order rules.Legal rejects, over the
// same corpus as #191" — legalspace_test.go's TestSampleCorpusFromGoldenMatches,
// here driven by Operator's own Decide in place of Sample, across real
// rules.NewMatch/Resolve matches at every supported player count.
func TestOperatorNeverIllegalOverGoldenCorpus(t *testing.T) {
	cfg := game.DefaultConfig()
	const rounds = 15
	operator := For(Operator)

	orders := 0
	for _, players := range []int{2, 3, 4, 5} {
		seed := testSeed(byte(0x70 + players))

		s, err := rules.NewMatch(seed, cfg, players)
		if err != nil {
			t.Fatalf("%d players: NewMatch: %v", players, err)
		}

		for round := 1; round <= rounds; round++ {
			seatOrders := make(map[game.SeatID]game.Order, players)

			for seat := range players {
				sid := game.SeatID(seat)

				// rules.Project's Config-free signature cannot set
				// StepAllowance or RoundsToNextOffer (D27) — internal/match
				// will fill both in once it exists; this loop performs that
				// fill-in itself here, the same as internal/rules' own
				// bots_runner_golden_external_test.go does for its package.
				// Operator reads v.You.RoundsToNextOffer directly
				// (timeToVanish), so leaving it at its zero value would plan
				// against a bound that was never actually computed.
				v := rules.Project(s, sid)
				v.You.StepAllowance = rules.Steps(v, cfg)
				v.You.RoundsToNextOffer = rules.RoundsToNextOffer(s, sid, cfg)

				o := operator.Decide(v, cfg, rules.NewBotRNG(seed, sid, round))

				if err := rules.Legal(v, o, cfg); err != nil {
					t.Fatalf("%d players, round %d, seat %d: illegal order %+v: %v", players, round, seat, o, err)
				}
				orders++
				seatOrders[sid] = o
			}

			s, _, err = rules.Resolve(s, seatOrders, cfg, rules.NewRNG(seed, round))
			if err != nil {
				t.Fatalf("%d players, round %d: Resolve: %v", players, round, err)
			}
		}
	}

	if orders == 0 {
		t.Fatal("corpus produced zero orders — the driven loop ran over nothing")
	}
}

// TestOperatorLeaseSurvivesAtEveryBlockRounds is issue #236's regression, run
// as the full lease lifecycle rather than an arithmetic assertion about
// AddOns: stake at a qualifying chokepoint, then step the post through every
// remaining round of the match applying the same three rules functions
// resolution itself applies — rules.RenewedRoundsRemaining for the stake's
// own up-front blocks (resolveStakePost, internal/rules/actions.go) and for
// each renewal (resolveAddons, internal/rules/addons.go), rules.DecrementLease
// for the Upkeep that runs at the end of every one of those same rounds
// (upkeepLeases, internal/rules/upkeep.go). The post must be live at the end.
//
// Before the fix, LeaseBlockRounds=1 failed at the very first decrement: a
// one-block stake was placed with RoundsRemaining=1 and expired in the round
// it was bought, so maybeRenewLease never saw it in v.You.Posts again and
// Operator ended every such match holding exactly zero leases — the precise,
// reproducible 0.0000 at every player count and every cost in
// docs/exit-demos/204-lease-rate.md's rounds curve. Swept across the same
// LeaseBlockRounds values that demo swept, so the shipped default is covered
// by the identical assertion rather than a separate one.
//
// The view's balance is held constant round over round on purpose: this
// asserts the renewal *timing* invariant, not the lease economy — whether
// Operator can afford an indefinite renewal streak is exactly the question
// runnerReserve and the cost sweep already answer, and letting the balance
// drain here would fold that separate question into this test's outcome.
func TestOperatorLeaseSurvivesAtEveryBlockRounds(t *testing.T) {
	opts := DefaultOperatorOptions()
	observed := opts.ChokepointMinObserved + 1
	const chokepoint = game.NodeID(2)

	for _, blockRounds := range []int{1, 2, 3, 4, 6} {
		t.Run(fmt.Sprintf("LeaseBlockRounds=%d", blockRounds), func(t *testing.T) {
			cfg := game.DefaultConfig()
			cfg.LeaseBlockRounds = blockRounds
			seed := testSeed(0x62)

			v := chokepointFixtureView()
			v.NodeStats = map[game.NodeID]game.NodeStats{
				chokepoint: {ObservedRounds: observed, TrafficRounds: observed},
			}

			stake := For(Operator).Decide(v, cfg, rules.NewBotRNG(seed, game.SeatID(0), int(v.Round)))
			if stake.Action.Kind != game.ActionStakePost || stake.AddOns.RenewPost != chokepoint {
				t.Fatalf("round %d: Action=%v RenewPost=%d, want StakePost at node %d",
					v.Round, stake.Action.Kind, stake.AddOns.RenewPost, chokepoint)
			}

			rounds := rules.RenewedRoundsRemaining(0, stake.AddOns.RenewBlocks, cfg)
			rounds, expired := rules.DecrementLease(rounds)
			if expired {
				t.Fatalf("a %d-block stake at LeaseBlockRounds=%d expired in the round it was staked — maybeRenewLease can never see it again (issue #236)",
					stake.AddOns.RenewBlocks, blockRounds)
			}

			for round := v.Round + 1; round <= game.RoundNumber(cfg.Rounds); round++ {
				held := chokepointFixtureView()
				held.Round = round
				held.You.Posts = []game.Post{{Node: chokepoint, RoundsRemaining: rounds}}

				o := For(Operator).Decide(held, cfg, rules.NewBotRNG(seed, game.SeatID(0), int(round)))

				// resolveAddons' own two conditions (internal/rules/addons.go):
				// a renewal naming a post this seat holds, on an order whose
				// action this round was not itself a StakePost.
				if o.AddOns.RenewBlocks > 0 && o.AddOns.RenewPost == chokepoint && o.Action.Kind != game.ActionStakePost {
					rounds = rules.RenewedRoundsRemaining(rounds, o.AddOns.RenewBlocks, cfg)
				}

				rounds, expired = rules.DecrementLease(rounds)
				if expired {
					t.Fatalf("post at node %d lapsed at the end of round %d of %d, having been renewed by %d block(s) that round",
						chokepoint, round, cfg.Rounds, o.AddOns.RenewBlocks)
				}
			}
		})
	}
}

// TestOperatorStakeBlocksLeavesTheShippedDefaultAlone pins the other half of
// issue #236's fix: stakeBlocksToSurviveUpkeep buys a second block only where
// one genuinely cannot outlive its own round, so at the shipped
// LeaseBlockRounds=3 — and at every longer duration — a fresh stake still
// costs exactly one block, as it did before the fix. Without this, a fix that
// simply always bought two blocks would satisfy the lifecycle test above
// while silently doubling the up-front cost of every lease in the shipped
// configuration.
func TestOperatorStakeBlocksLeavesTheShippedDefaultAlone(t *testing.T) {
	cfg := game.DefaultConfig()
	if got := stakeBlocksToSurviveUpkeep(cfg); got != 1 {
		t.Errorf("DefaultConfig (LeaseBlockRounds=%d): stakeBlocksToSurviveUpkeep = %d, want 1 — the shipped stake cost must not change",
			cfg.LeaseBlockRounds, got)
	}

	for _, tc := range []struct{ blockRounds, want int }{
		{0, 0}, // leasing switched off entirely — fundNewStake reads this as "do not stake"
		{1, 2},
		{2, 1},
		{3, 1},
		{6, 1},
		{15, 1},
	} {
		cfg := game.DefaultConfig()
		cfg.LeaseBlockRounds = tc.blockRounds
		if got := stakeBlocksToSurviveUpkeep(cfg); got != tc.want {
			t.Errorf("LeaseBlockRounds=%d: stakeBlocksToSurviveUpkeep = %d, want %d", tc.blockRounds, got, tc.want)
		}
	}
}
