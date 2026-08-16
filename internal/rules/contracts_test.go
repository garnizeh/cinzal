package rules

import (
	"fmt"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// offerTestGraph is a line graph 0..10: node 0 is a Warehouse, nodes 3
// through 10 are Borders — so the origin-to-destination distance equals the
// destination's own NodeID, giving exact, hand-checkable control over which
// tier's distance band a candidate falls in (Tier I 3-4, II 4-6, III 5-8,
// IV 6+, per GDD §8.3).
func offerTestGraph() Graph {
	const n = 11
	nodes := make([]Node, n)
	for i := range nodes {
		var edges []game.NodeID
		if i > 0 {
			edges = append(edges, game.NodeID(i-1))
		}
		if i < n-1 {
			edges = append(edges, game.NodeID(i+1))
		}
		nodes[i] = Node{ID: game.NodeID(i), Type: game.NodeAlley, Edges: edges}
	}
	nodes[0].Type = game.NodeWarehouse
	for _, b := range []int{3, 4, 5, 6, 7, 8, 9, 10} {
		nodes[b].Type = game.NodeBorder
	}
	return Graph{Nodes: nodes}
}

// offerTestState builds a one-seat MatchState on offerTestGraph, with the
// Warehouse at node 0 Known to the seat — everything GenerateOffer needs to
// have a real candidate pool to draw against.
func offerTestState(infamy int, contracts []Contract, lastOfferRound, round game.RoundNumber) MatchState {
	g := offerTestGraph()
	fog := make([]game.FogState, len(g.Nodes))
	fog[0] = game.FogKnown

	return MatchState{
		Round: round,
		Graph: g,
		Players: []Player{{
			Seat:           0,
			Infamy:         infamy,
			Contracts:      contracts,
			LastOfferRound: lastOfferRound,
			Fog:            fog,
		}},
	}
}

// TestGenerateOfferOpeningOfferNeverEmpty is #63's central regression test
// for GDD §8.1's v1.8 deadlock: on a real generated graph (initial(), not a
// hand-built fixture), every seat's opening offer must deliver at least one
// contract, on every seed, at every supported player count. D23's fog
// seeding is what makes this possible (a Known Warehouse at setup); this
// test exercises it through the actual GenerateOffer path rather than
// re-checking fog state alone.
// offerSweepBatches splits [0, total) into this many contiguous,
// equal-sized (last one absorbs the remainder) seed ranges, so the sweep
// below can run each range as its own parallel subtest.
const offerSweepBatches = 8

func TestGenerateOfferOpeningOfferNeverEmpty(t *testing.T) {
	cfg := game.DefaultConfig()

	// Both player count and seed range are split into parallel subtests:
	// initial() + GenerateOffer over 1000 seeds does a full map generation
	// per seed (D24's stricter constraint 7 means more attemptGenerate
	// retries before a start placement is accepted, especially at the
	// largest player count's 28-node map), and running that serially under
	// -race blows past go test's default 10-minute per-package timeout —
	// measured at ~19 minutes for players=5 alone. Splitting each player
	// count's 1000 seeds into offerSweepBatches ranges cut that to ~4.5
	// minutes on a 28-core box. This doesn't weaken the assertion — every
	// seed at every player count is still checked — it only changes how
	// the checking is scheduled.
	const total = 1000
	batchSize := total / offerSweepBatches
	for _, players := range []int{2, 3, 4, 5} {
		for b := range offerSweepBatches {
			start := b * batchSize
			end := start + batchSize
			if b == offerSweepBatches-1 {
				end = total
			}
			t.Run(fmt.Sprintf("players=%d/batch=%d", players, b), func(t *testing.T) {
				t.Parallel()
				for seed := start; seed < end; seed++ {
					matchSeed := seedFromInt(seed)
					s, err := initial(matchSeed, cfg, players)
					if err != nil {
						t.Fatalf("players=%d seed=%d: initial() = %v", players, seed, err)
					}

					rng := NewRNG(matchSeed, 1)
					for seat := range s.Players {
						offer, delivered := GenerateOffer(s, game.SeatID(seat), cfg, rng)
						if !delivered || len(offer) == 0 {
							t.Fatalf("players=%d seed=%d seat=%d: delivered=%v offer=%v, want a delivered offer with >= 1 contract (GDD §8.1 deadlock)",
								players, seed, seat, delivered, offer)
						}
					}
				}
			})
		}
	}
}

// TestGenerateOfferDueWhenNeverOffered asserts LastOfferRound's zero value
// (RoundNumber is 1-indexed, so 0 means "never") is always due, regardless
// of the Contact Cooldown — the rule that makes the opening offer
// unconditional rather than waiting out a cooldown period nobody started.
func TestGenerateOfferDueWhenNeverOffered(t *testing.T) {
	cfg := game.DefaultConfig()
	s := offerTestState(0, nil, 0, 1)
	rng := NewRNG(testSeed(60), 1)

	offer, delivered := GenerateOffer(s, 0, cfg, rng)
	if !delivered {
		t.Fatal("delivered = false, want true (never offered before, so always due)")
	}
	if len(offer) == 0 {
		t.Fatal("offer is empty, want at least the guaranteed Tier I slot")
	}
	for _, c := range offer {
		if c.Tier != 0 {
			t.Errorf("offer contract Tier = %d, want 0 (Infamy 0 is only eligible for Tier I)", c.Tier)
		}
		if c.Origin != 0 {
			t.Errorf("offer contract Origin = %d, want 0 (the only Known Warehouse)", c.Origin)
		}
	}
}

// TestGenerateOfferHeldAtFullSlotsThenDeliveredWhenFreed is GDD §8.2's
// full-slots hold: a seat at 2 active contracts past its cooldown round
// gets nothing this round, and no RNG draw at all — then, once a slot
// frees, the very next attempt delivers immediately, using the same stale
// LastOfferRound (the cooldown never restarted while held).
func TestGenerateOfferHeldAtFullSlotsThenDeliveredWhenFreed(t *testing.T) {
	cfg := game.DefaultConfig()
	full := []Contract{
		{ID: 0, Origin: 0, Destination: 3, Tier: 0, ExpiresRound: 10},
		{ID: 1, Origin: 0, Destination: 4, Tier: 0, ExpiresRound: 10},
	}
	// LastOfferRound=1, Nobody's cooldown is 4 rounds, so due at round 5.
	s := offerTestState(0, full, 1, 5)
	rng := NewRNG(testSeed(61), 5)

	offer, delivered := GenerateOffer(s, 0, cfg, rng)
	if delivered || offer != nil {
		t.Fatalf("delivered=%v offer=%v, want held (false, nil) at 2 active contracts", delivered, offer)
	}
	if rng.Seq() != 0 {
		t.Errorf("Seq() = %d, want 0 — the full-slots hold must not touch the RNG at all", rng.Seq())
	}

	freed := s
	freed.Players = []Player{s.Players[0]}
	freed.Players[0].Contracts = full[:1]

	offer2, delivered2 := GenerateOffer(freed, 0, cfg, rng)
	if !delivered2 || len(offer2) == 0 {
		t.Fatalf("delivered=%v offer=%v, want a delivered offer once a slot frees, same stale LastOfferRound=%d", delivered2, offer2, freed.Players[0].LastOfferRound)
	}
}

// TestGenerateOfferEmptyPoolHoldsWithoutRestartingCooldown is D7's
// empty-pool hold: every eligible tier's pool comes up dry (the only Border
// is at distance 1, below every tier's minimum), so nothing is delivered —
// but unlike the full-slots case, this can only be discovered by actually
// running the tier-mix draw, so the 2 contract.offer.tier draws are still
// consumed even though nothing is delivered.
func TestGenerateOfferEmptyPoolHoldsWithoutRestartingCooldown(t *testing.T) {
	cfg := game.DefaultConfig()
	g := Graph{Nodes: []Node{
		{ID: 0, Type: game.NodeWarehouse, Edges: []game.NodeID{1}},
		{ID: 1, Type: game.NodeBorder, Edges: []game.NodeID{0}},
	}}
	fog := []game.FogState{game.FogKnown, game.FogHidden}
	s := MatchState{
		Round:   1,
		Graph:   g,
		Players: []Player{{Seat: 0, Infamy: 0, Fog: fog, LastOfferRound: 0}},
	}
	rng := NewRNG(testSeed(62), 1)

	offer, delivered := GenerateOffer(s, 0, cfg, rng)
	if delivered || offer != nil {
		t.Fatalf("delivered=%v offer=%v, want held: distance 1 is below Tier I's minimum of 3", delivered, offer)
	}
	if got := rng.Consumed(PurposeContractOfferTier); got != 2 {
		t.Errorf("Consumed(contract.offer.tier) = %d, want 2 (tier draws always happen once generation is attempted)", got)
	}
	if got := rng.Consumed(PurposeContractOfferPick); got != 0 {
		t.Errorf("Consumed(contract.offer.pick) = %d, want 0 (every tier's pool was empty)", got)
	}
}

// TestRoundsToNextOfferUsesNobodyCooldownUnderSuppressInfamyTiers is D11's
// consequence for Suppress.InfamyTiers: Contact Cooldown is pinned to the
// Nobody row regardless of actual Infamy — a Legend-Infamy seat (9) still
// waits out Nobody's cooldown, not Legend's shorter one (issue #158).
func TestRoundsToNextOfferUsesNobodyCooldownUnderSuppressInfamyTiers(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Suppress.InfamyTiers = true
	s := offerTestState(9, nil, 3, 3) // Infamy 9 = Legend, but suppressed

	wantCooldown := cfg.CooldownByTier[0] // Nobody's row
	if got := RoundsToNextOffer(s, 0, cfg); got != wantCooldown {
		t.Errorf("RoundsToNextOffer() at Infamy 9 under Suppress.InfamyTiers = %d, want %d (Nobody's cooldown)", got, wantCooldown)
	}
}

// TestDeclineOfferRestartsCooldownAndHUDMatchesReality is GDD §8.2's
// "always displayed, never a surprise": after a decline, RoundsToNextOffer
// and offerDue must agree, round by round, exactly through the cooldown
// window — never early, never late.
func TestDeclineOfferRestartsCooldownAndHUDMatchesReality(t *testing.T) {
	cfg := game.DefaultConfig()
	s := offerTestState(0, nil, 0, 1) // Infamy 0 = Nobody, cooldown 4

	declined := DeclineOffer(s, 0, 3)
	if got := declined.Players[0].LastOfferRound; got != 3 {
		t.Fatalf("LastOfferRound = %d, want 3", got)
	}

	cases := []struct {
		round         game.RoundNumber
		wantRemaining int
	}{
		{3, 4}, {4, 3}, {5, 2}, {6, 1}, {7, 0}, {8, 0},
	}
	for _, c := range cases {
		declined.Round = c.round
		if got := RoundsToNextOffer(declined, 0, cfg); got != c.wantRemaining {
			t.Errorf("round %d: RoundsToNextOffer = %d, want %d", c.round, got, c.wantRemaining)
		}
		wantDue := c.wantRemaining == 0
		if got := offerDue(declined, 0, cfg); got != wantDue {
			t.Errorf("round %d: offerDue = %v, want %v", c.round, got, wantDue)
		}
	}
}

// TestAcceptOfferBuildsContractAndRestartsCooldown asserts AcceptOffer's
// three effects at once: the accepted offer becomes a real Contract with a
// computed deadline (GDD §8.3: "from acceptance"), the cooldown restarts
// (GDD §8.2), and the input MatchState is never mutated in place.
func TestAcceptOfferBuildsContractAndRestartsCooldown(t *testing.T) {
	cfg := game.DefaultConfig()
	s := offerTestState(0, nil, 0, 1)

	accepted := AcceptOffer(s, 0, ContractOffer{Origin: 0, Destination: 3, Tier: 0}, cfg, 1)

	p := accepted.Players[0]
	if len(p.Contracts) != 1 {
		t.Fatalf("len(Contracts) = %d, want 1", len(p.Contracts))
	}
	c := p.Contracts[0]
	if c.ID != 0 || c.Origin != 0 || c.Destination != 3 || c.Tier != 0 {
		t.Errorf("Contracts[0] = %+v, want {ID:0 Origin:0 Destination:3 Tier:0 ...}", c)
	}
	wantExpires := game.RoundNumber(1) + game.RoundNumber(cfg.Contracts[0].Deadline)
	if c.ExpiresRound != wantExpires {
		t.Errorf("ExpiresRound = %d, want %d (round 1 + Tier I's %d-round deadline)", c.ExpiresRound, wantExpires, cfg.Contracts[0].Deadline)
	}
	if c.DeadlinePauseUsed {
		t.Error("DeadlinePauseUsed = true on a freshly accepted contract, want false")
	}
	if p.LastOfferRound != 1 {
		t.Errorf("LastOfferRound = %d, want 1 (accepting restarts the cooldown)", p.LastOfferRound)
	}

	if len(s.Players[0].Contracts) != 0 {
		t.Error("original MatchState.Players[0].Contracts was mutated by AcceptOffer")
	}
}

// TestAcceptOfferAssignsSecondSlot asserts nextContractID's slot-index
// scheme: a seat already holding ID 0 gets ID 1 for a second contract.
func TestAcceptOfferAssignsSecondSlot(t *testing.T) {
	cfg := game.DefaultConfig()
	existing := []Contract{{ID: 0, Origin: 0, Destination: 3, Tier: 0, ExpiresRound: 5}}
	s := offerTestState(0, existing, 1, 5)

	accepted := AcceptOffer(s, 0, ContractOffer{Origin: 0, Destination: 4, Tier: 0}, cfg, 5)
	if len(accepted.Players[0].Contracts) != 2 {
		t.Fatalf("len(Contracts) = %d, want 2", len(accepted.Players[0].Contracts))
	}
	if got := accepted.Players[0].Contracts[1].ID; got != 1 {
		t.Errorf("second contract's ID = %d, want 1", got)
	}
}

// TestContractCandidatesUsesNavigableGraphDistance is #45's/D7's acceptance
// criterion made concrete: distance-band membership is checked against the
// currently navigable graph (GDD §9.1a item 0), so a Bridge Down that
// lengthens the only path between an origin and a destination reclassifies
// the pair to a lower tier's pool — never a higher one, never by widening a
// band (D7).
func TestContractCandidatesUsesNavigableGraphDistance(t *testing.T) {
	cfg := game.DefaultConfig()

	// Before: 0-1-2-3-4, distance(0,4) = 4 — Tier I [3,4] and Tier II [4,6],
	// not Tier III [5,8].
	before := Graph{Nodes: []Node{
		{ID: 0, Type: game.NodeWarehouse, Edges: []game.NodeID{1}},
		{ID: 1, Type: game.NodeAlley, Edges: []game.NodeID{0, 2}},
		{ID: 2, Type: game.NodeAlley, Edges: []game.NodeID{1, 3}},
		{ID: 3, Type: game.NodeAlley, Edges: []game.NodeID{2, 4}},
		{ID: 4, Type: game.NodeBorder, Edges: []game.NodeID{3}},
	}}
	sBefore := MatchState{Graph: before, Players: []Player{{Fog: []game.FogState{game.FogKnown, game.FogHidden, game.FogHidden, game.FogHidden, game.FogHidden}}}}

	if got := len(contractCandidates(sBefore, 0, 0, cfg, map[pairKey]bool{})); got != 1 {
		t.Fatalf("before Bridge Down: Tier I candidates = %d, want 1", got)
	}
	if got := len(contractCandidates(sBefore, 0, 1, cfg, map[pairKey]bool{})); got != 1 {
		t.Fatalf("before Bridge Down: Tier II candidates = %d, want 1", got)
	}
	if got := len(contractCandidates(sBefore, 0, 2, cfg, map[pairKey]bool{})); got != 0 {
		t.Fatalf("before Bridge Down: Tier III candidates = %d, want 0 (distance 4 is outside [5,8])", got)
	}

	// After: the 2-3 edge is destroyed; the only remaining path detours
	// through 5,6,7, so distance(0,4) = 7 — outside Tier I and II's bands,
	// inside Tier III's.
	after := Graph{Nodes: []Node{
		{ID: 0, Type: game.NodeWarehouse, Edges: []game.NodeID{1}},
		{ID: 1, Type: game.NodeAlley, Edges: []game.NodeID{0, 2}},
		{ID: 2, Type: game.NodeAlley, Edges: []game.NodeID{1, 5}},
		{ID: 3, Type: game.NodeAlley, Edges: []game.NodeID{7, 4}},
		{ID: 4, Type: game.NodeBorder, Edges: []game.NodeID{3}},
		{ID: 5, Type: game.NodeAlley, Edges: []game.NodeID{2, 6}},
		{ID: 6, Type: game.NodeAlley, Edges: []game.NodeID{5, 7}},
		{ID: 7, Type: game.NodeAlley, Edges: []game.NodeID{6, 3}},
	}}
	fog := make([]game.FogState, 8)
	fog[0] = game.FogKnown
	sAfter := MatchState{Graph: after, Players: []Player{{Fog: fog}}}

	if got := len(contractCandidates(sAfter, 0, 0, cfg, map[pairKey]bool{})); got != 0 {
		t.Errorf("after Bridge Down: Tier I candidates = %d, want 0 (distance grew to 7)", got)
	}
	if got := len(contractCandidates(sAfter, 0, 1, cfg, map[pairKey]bool{})); got != 0 {
		t.Errorf("after Bridge Down: Tier II candidates = %d, want 0", got)
	}
	if got := len(contractCandidates(sAfter, 0, 2, cfg, map[pairKey]bool{})); got != 1 {
		t.Errorf("after Bridge Down: Tier III candidates = %d, want 1 (D7: the cascade now fills this slot from Tier III instead)", got)
	}
}

// TestContractCandidatesExcludesSinkholedOrigin asserts a Known Warehouse
// that has since become Sinkholed cannot originate any contract candidate,
// at any tier — GDD §9.1a item 0's navigable graph excludes Sinkholed nodes
// entirely, and contractCandidates calls distances(origin.ID) once per Known
// Warehouse, which is not necessarily the seat's own position, so this must
// hold even though the seat never "walked into" the Sinkhole itself.
func TestContractCandidatesExcludesSinkholedOrigin(t *testing.T) {
	cfg := game.DefaultConfig()
	s := offerTestState(0, nil, 0, 1)
	s.Graph.Nodes[0].SinkholeRounds = 1

	for tier := range cfg.Contracts {
		if got := len(contractCandidates(s, 0, tier, cfg, map[pairKey]bool{})); got != 0 {
			t.Errorf("tier=%d: candidates = %d, want 0 (origin warehouse is Sinkholed)", tier, got)
		}
	}
}

// TestGenerateOfferRNGAccounting is #41's/D6's/D7's accounting made
// concrete: not-due and full-slots consume nothing at all; a delivered
// offer costs exactly 2 contract.offer.tier draws plus one
// contract.offer.pick draw per filled slot; a target tier coming up empty
// never redraws — it costs nothing and the cascade just tries the next
// tier down.
func TestGenerateOfferRNGAccounting(t *testing.T) {
	cfg := game.DefaultConfig()

	t.Run("not due consumes nothing", func(t *testing.T) {
		// LastOfferRound=5, Nobody's cooldown is 4 rounds, so due at round 9.
		s := offerTestState(0, nil, 5, 6)
		rng := NewRNG(testSeed(70), 6)

		if _, delivered := GenerateOffer(s, 0, cfg, rng); delivered {
			t.Fatal("delivered = true, want false (cooldown not elapsed)")
		}
		if rng.Seq() != 0 {
			t.Errorf("Seq() = %d, want 0", rng.Seq())
		}
	})

	t.Run("full slots consumes nothing", func(t *testing.T) {
		full := []Contract{{ID: 0, ExpiresRound: 10}, {ID: 1, ExpiresRound: 10}}
		s := offerTestState(0, full, 0, 1)
		rng := NewRNG(testSeed(71), 1)

		if _, delivered := GenerateOffer(s, 0, cfg, rng); delivered {
			t.Fatal("delivered = true, want false (2 active contracts)")
		}
		if rng.Seq() != 0 {
			t.Errorf("Seq() = %d, want 0", rng.Seq())
		}
	})

	t.Run("delivered offer costs 2 tier draws plus one pick draw per filled slot", func(t *testing.T) {
		s := offerTestState(0, nil, 0, 1)
		rng := NewRNG(testSeed(72), 1)

		offer, delivered := GenerateOffer(s, 0, cfg, rng)
		if !delivered {
			t.Fatal("delivered = false, want true")
		}
		if got := rng.Consumed(PurposeContractOfferTier); got != 2 {
			t.Errorf("Consumed(contract.offer.tier) = %d, want 2", got)
		}
		if got := rng.Consumed(PurposeContractOfferPick); got != len(offer) {
			t.Errorf("Consumed(contract.offer.pick) = %d, want %d (one per filled slot)", got, len(offer))
		}
	})

	t.Run("truncation: only Tier I's pool is non-empty, never a redraw of an already-checked tier", func(t *testing.T) {
		// A single (Warehouse, Border) pair at distance 3: fits only Tier
		// I's [3,4] band, not II's [4,6] or III's [5,8]. Infamy 6 (Feared)
		// is eligible for I, II and III, so every slot's cascade starts at
		// or above Tier I and must walk down through empty tiers to reach
		// the one candidate that exists.
		g := Graph{Nodes: []Node{
			{ID: 0, Type: game.NodeWarehouse, Edges: []game.NodeID{1}},
			{ID: 1, Type: game.NodeAlley, Edges: []game.NodeID{0, 2}},
			{ID: 2, Type: game.NodeAlley, Edges: []game.NodeID{1, 3}},
			{ID: 3, Type: game.NodeBorder, Edges: []game.NodeID{2}},
		}}
		fog := make([]game.FogState, len(g.Nodes))
		fog[0] = game.FogKnown
		s := MatchState{Round: 1, Graph: g, Players: []Player{{Seat: 0, Infamy: 6, Fog: fog}}}
		rng := NewRNG(testSeed(73), 1)

		offer, delivered := GenerateOffer(s, 0, cfg, rng)
		if !delivered {
			t.Fatal("delivered = false, want true (Tier I's pool has the pair)")
		}
		for _, c := range offer {
			if c.Tier != 0 {
				t.Errorf("filled contract Tier = %d, want 0 (only Tier I's band contains the single candidate)", c.Tier)
			}
		}
		if got := rng.Consumed(PurposeContractOfferTier); got != 2 {
			t.Errorf("Consumed(contract.offer.tier) = %d, want 2", got)
		}
		if got := rng.Consumed(PurposeContractOfferPick); got != 1 {
			t.Errorf("Consumed(contract.offer.pick) = %d, want 1 (a single candidate, without replacement, can fill only one slot)", got)
		}
		if len(offer) != 1 {
			t.Errorf("len(offer) = %d, want 1", len(offer))
		}
	})
}
