package rules

import (
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// eventsTestGraph is the fixed graph every test in this file builds on: a
// 6-node ring with 2 Warehouses, 2 Borders, 1 Black Market and 1 Alley,
// spread across all four sectors — rich enough for every card's candidate
// set (Dragnet needs >=2 Borders, Bridge Down needs >=1 navigable edge,
// Scaffolding's own candidates are the four game.Sector constants directly,
// not searched from the graph).
//
//	0 (Warehouse, Old Docks)   -- 1 (Border, Old Docks)
//	|                                                 |
//	5 (Alley, North Vale)                    2 (Border, Iron Low)
//	|                                                 |
//	4 (Warehouse, North Vale) -- 3 (Black Market, Mist Heights)
func eventsTestGraph() Graph {
	return Graph{Nodes: []Node{
		{ID: 0, Type: game.NodeWarehouse, Sector: game.SectorOldDocks, Edges: []game.NodeID{1, 5}},
		{ID: 1, Type: game.NodeBorder, Sector: game.SectorOldDocks, Edges: []game.NodeID{0, 2}},
		{ID: 2, Type: game.NodeBorder, Sector: game.SectorIronLow, Edges: []game.NodeID{1, 3}},
		{ID: 3, Type: game.NodeBlackMarket, Sector: game.SectorMistHeights, Edges: []game.NodeID{2, 4}},
		{ID: 4, Type: game.NodeWarehouse, Sector: game.SectorNorthVale, Edges: []game.NodeID{3, 5}},
		{ID: 5, Type: game.NodeAlley, Sector: game.SectorNorthVale, Edges: []game.NodeID{4, 0}},
	}}
}

// eventsTestState returns a MatchState over eventsTestGraph with one seat
// per entry in start (seat i at node start[i]), Balance 20 and Round 7 (an
// ordinary live-card round, GDD §14.2) — a baseline every test overrides
// from, matching actionsTestState's own shape in actions_test.go.
func eventsTestState(start ...game.NodeID) MatchState {
	players := make([]Player, len(start))
	for i, pos := range start {
		players[i] = Player{Seat: game.SeatID(i), Balance: 20, Position: pos}
	}
	return MatchState{Round: 7, Graph: eventsTestGraph(), Players: players}
}

// TestResolveRaidHitsAllTiedHighestInfamy is GDD §14.2's Raid: "The
// highest-Infamy player loses carried cargo and −2 Infamy. Ties: all tied
// players" — no fairness tie-break, unlike New Boss/Bounty.
func TestResolveRaidHitsAllTiedHighestInfamy(t *testing.T) {
	s := eventsTestState(1, 1, 0)
	s.Players[0].Infamy, s.Players[1].Infamy, s.Players[2].Infamy = 6, 6, 3
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true, Contract: 0}
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 5}}
	s.Players[1].Cargo = &game.CarriedCargo{Bound: false}

	resolveRaid(&s)

	for _, seat := range []game.SeatID{0, 1} {
		if got := s.Players[seat].Infamy; got != 4 {
			t.Errorf("seat %d Infamy = %d, want 4 (6 - 2)", seat, got)
		}
		if s.Players[seat].Cargo != nil {
			t.Errorf("seat %d still carries cargo, want it dropped", seat)
		}
	}
	if got := s.Players[2].Infamy; got != 3 {
		t.Errorf("seat 2 (not tied for highest) Infamy = %d, want unchanged 3", got)
	}
	if len(s.Graph.Cargo) != 2 {
		t.Fatalf("Graph.Cargo = %+v, want 2 dropped crates", s.Graph.Cargo)
	}
	for _, c := range s.Graph.Cargo {
		if c.Node != 1 {
			t.Errorf("dropped cargo at node %d, want 1 (both seats' own position)", c.Node)
		}
	}
}

// TestResolvePayrollDayChargesPerPostHeld is GDD §14.2's Payroll Day:
// "Every player pays Cr$ 1 per post held."
func TestResolvePayrollDayChargesPerPostHeld(t *testing.T) {
	s := eventsTestState(0, 0)
	s.Players[0].Posts = []game.NodeID{1, 2}
	s.Players[1].Posts = nil

	resolvePayrollDay(&s)

	if got := s.Players[0].Balance; got != 18 {
		t.Errorf("seat 0 Balance = %d, want 18 (20 - 2 posts)", got)
	}
	if got := s.Players[1].Balance; got != 20 {
		t.Errorf("seat 1 (no posts) Balance = %d, want unchanged 20", got)
	}
}

// TestResolveShiftChangeLosesOneInfamyEveryone is GDD §14.2's Shift Change:
// "everyone takes −1 Infamy." The card's other half — no Pressure roll — is
// pressure()'s own concern (issue #73), not testable here since pressure()
// is still a stub.
func TestResolveShiftChangeLosesOneInfamyEveryone(t *testing.T) {
	s := eventsTestState(0, 0)
	s.Players[0].Infamy = 0 // floors at 0, never negative (ApplyInfamyDelta)
	s.Players[1].Infamy = 5

	resolveShiftChange(&s)

	if got := s.Players[0].Infamy; got != 0 {
		t.Errorf("seat 0 Infamy = %d, want floored at 0", got)
	}
	if got := s.Players[1].Infamy; got != 4 {
		t.Errorf("seat 1 Infamy = %d, want 4", got)
	}
}

// TestResolveCurrencySlideRoundsDown is GDD §14.2's Currency Slide: "loses
// 25% of their balance, rounded down" — bal - bal/4, integer arithmetic
// (RFC §6.3), never float64.
func TestResolveCurrencySlideRoundsDown(t *testing.T) {
	s := eventsTestState(0)
	s.Players[0].Balance = 21 // 21/4 == 5 (integer division) -> 21-5 = 16

	resolveCurrencySlide(&s)

	if got := s.Players[0].Balance; got != 16 {
		t.Errorf("Balance = %d, want 16 (21 - 21/4, rounded down)", got)
	}
}

// TestResolveMarketSurgeSetsFlagForEverySeat is GDD §14.2's Market Surge:
// "Each player's next delivery pays +50%" — a standing per-seat flag, not a
// this-round effect (deliveries_test.go covers its consumption).
func TestResolveMarketSurgeSetsFlagForEverySeat(t *testing.T) {
	s := eventsTestState(0, 0, 0)

	resolveMarketSurge(&s)

	for _, seat := range bySeat(s) {
		if !s.Players[seat].MarketSurgeActive {
			t.Errorf("seat %d MarketSurgeActive = false, want true", seat)
		}
	}
}

// TestResolveFencesWindfallOpensStandingOffer is GDD §14.2's Fence's
// Windfall: draws one Black Market (eventsTestGraph has exactly one, node
// 3 — a deterministic candidate set) and opens its standing offer,
// consuming exactly one PurposeEventFencesWindfall draw (D3).
func TestResolveFencesWindfallOpensStandingOffer(t *testing.T) {
	s := eventsTestState()
	r := NewRNG(testSeed(1), 7)

	events := resolveFencesWindfall(&s, r)

	if !s.Graph.Nodes[3].FenceWindfallActive {
		t.Error("node 3 (the only Black Market) FenceWindfallActive = false, want true")
	}
	if got := r.Consumed(PurposeEventFencesWindfall); got != 1 {
		t.Errorf("PurposeEventFencesWindfall consumed = %d, want 1", got)
	}
	if len(events) != 1 || events[0].Kind != game.EventFenceWindfallAnnounced || events[0].Node != 3 {
		t.Errorf("events = %+v, want one EventFenceWindfallAnnounced at node 3", events)
	}
}

// TestResolveFenceWindfallClaimAwardsFirstArrival is GDD §14.2's own "First
// arrival only": a cargo-carrying seat ending the round at the flagged
// node claims Cr$ 12 and loses the cargo; the flag clears so nobody else
// can claim it again.
func TestResolveFenceWindfallClaimAwardsFirstArrival(t *testing.T) {
	s := eventsTestState(3, 0) // seat 0 at the flagged node, seat 1 elsewhere
	s.Graph.Nodes[3].FenceWindfallActive = true
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false}

	resolveFenceWindfallClaim(&s, bySeat(s), NewRNG(testSeed(1), 7))

	if got := s.Players[0].Balance; got != 32 {
		t.Errorf("seat 0 Balance = %d, want 32 (20 + Cr$12)", got)
	}
	if s.Players[0].Cargo != nil {
		t.Error("seat 0 still carries cargo, want it sold")
	}
	if s.Graph.Nodes[3].FenceWindfallActive {
		t.Error("FenceWindfallActive still true after a successful claim, want cleared")
	}
}

// TestResolveFenceWindfallClaimContestedByFairnessTie: two seats both end
// the round at the flagged node carrying cargo — the claim is contested by
// the identical fairness-tie-break chain New Boss's and Bounty's own
// target selection already reuses (D14 §5), consuming exactly one
// PurposeConfrontTiebreak draw for the genuine tie.
func TestResolveFenceWindfallClaimContestedByFairnessTie(t *testing.T) {
	s := eventsTestState(3, 3)
	s.Graph.Nodes[3].FenceWindfallActive = true
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false}
	s.Players[1].Cargo = &game.CarriedCargo{Bound: false}
	r := NewRNG(testSeed(1), 7)

	resolveFenceWindfallClaim(&s, bySeat(s), r)

	if got := r.Consumed(PurposeConfrontTiebreak); got != 1 {
		t.Errorf("PurposeConfrontTiebreak consumed = %d, want 1 (a genuine two-way tie)", got)
	}
	winners := 0
	for _, seat := range bySeat(s) {
		if s.Players[seat].Cargo == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Errorf("%d seats won the claim, want exactly 1", winners)
	}
}

// TestResolveFenceWindfallClaimNoOpWithoutEligibleSeat confirms no node
// flagged, or nobody eligible, is a silent no-op — the offer stays open for
// a future round.
func TestResolveFenceWindfallClaimNoOpWithoutEligibleSeat(t *testing.T) {
	s := eventsTestState(3)
	s.Graph.Nodes[3].FenceWindfallActive = true
	// seat 0 is at the flagged node but carries no cargo.

	resolveFenceWindfallClaim(&s, bySeat(s), NewRNG(testSeed(1), 7))

	if !s.Graph.Nodes[3].FenceWindfallActive {
		t.Error("FenceWindfallActive cleared with no eligible claimant, want it to stay open")
	}
	if got := s.Players[0].Balance; got != 20 {
		t.Errorf("Balance = %d, want unchanged 20", got)
	}
}

// TestResolveInformantsRevealsEveryPosition is GDD §14.2's Informants:
// "Every player's current position is revealed to everyone" — one
// EventInformants per seat (RFC §9.1 row 11), the one card whose EventKind
// already existed before this issue.
func TestResolveInformantsRevealsEveryPosition(t *testing.T) {
	s := eventsTestState(1, 3)

	events := resolveInformants(&s)

	if len(events) != 2 {
		t.Fatalf("events = %+v, want 2 (one per seat)", events)
	}
	want := map[game.SeatID]game.NodeID{0: 1, 1: 3}
	for _, e := range events {
		if e.Kind != game.EventInformants {
			t.Errorf("event kind = %s, want Informants", e.Kind)
		}
		if e.Node != want[e.Seat] {
			t.Errorf("seat %d revealed at node %d, want %d", e.Seat, e.Node, want[e.Seat])
		}
	}
}

// TestResolveAmnestyFloorsAtZero is GDD §14.2's Amnesty: "Everyone −3
// Infamy, floor 0."
func TestResolveAmnestyFloorsAtZero(t *testing.T) {
	s := eventsTestState(0, 0)
	s.Players[0].Infamy = 1
	s.Players[1].Infamy = 8

	resolveAmnesty(&s)

	if got := s.Players[0].Infamy; got != 0 {
		t.Errorf("seat 0 Infamy = %d, want floored at 0 (1 - 3)", got)
	}
	if got := s.Players[1].Infamy; got != 5 {
		t.Errorf("seat 1 Infamy = %d, want 5 (8 - 3)", got)
	}
}

// TestResolveNewBossPaysLowestRP is GDD §14.2's New Boss: "The lowest-RP
// player takes Cr$ 10 and +1 Infamy."
func TestResolveNewBossPaysLowestRP(t *testing.T) {
	s := eventsTestState(0, 0, 0)
	s.Players[0].RP, s.Players[1].RP, s.Players[2].RP = 5, 1, 9
	s.Players[1].Infamy = 3

	resolveNewBoss(&s, NewRNG(testSeed(1), 7))

	if got := s.Players[1].Balance; got != 30 {
		t.Errorf("lowest-RP seat 1 Balance = %d, want 30 (20 + Cr$10)", got)
	}
	if got := s.Players[1].Infamy; got != 4 {
		t.Errorf("lowest-RP seat 1 Infamy = %d, want 4 (3 + 1)", got)
	}
	for _, seat := range []game.SeatID{0, 2} {
		if got := s.Players[seat].Balance; got != 20 {
			t.Errorf("seat %d Balance = %d, want unchanged 20", seat, got)
		}
	}
}

// TestResolveNewBossTieBreaksByFairnessKey: two seats tied at the lowest
// RP resolve through D14 §5's reused chain (ascending Infamy first), no
// coin needed when Infamy alone already separates them.
func TestResolveNewBossTieBreaksByFairnessKey(t *testing.T) {
	s := eventsTestState(0, 0, 0)
	s.Players[0].RP, s.Players[1].RP, s.Players[2].RP = 1, 1, 9
	s.Players[0].Infamy, s.Players[1].Infamy = 5, 2 // seat 1 lower Infamy wins the tie

	resolveNewBoss(&s, NewRNG(testSeed(1), 7))

	if got := s.Players[1].Balance; got != 30 {
		t.Errorf("seat 1 (lower Infamy among the RP tie) Balance = %d, want 30", got)
	}
	if got := s.Players[0].Balance; got != 20 {
		t.Errorf("seat 0 Balance = %d, want unchanged 20", got)
	}
}

// TestResolveOldFavourResetsCooldownForEverySeat is GDD §14.2's Old
// Favour: "Every player immediately receives a contract offer, ignoring
// the Contact Cooldown" — realized as LastOfferRound reset to due-
// immediately (0), since offer generation and acceptance both happen
// outside Resolve entirely (events.go's own doc comment on
// resolveOldFavour explains why).
func TestResolveOldFavourResetsCooldownForEverySeat(t *testing.T) {
	s := eventsTestState(0, 0)
	s.Players[0].LastOfferRound = 6
	s.Players[1].LastOfferRound = 7

	resolveOldFavour(&s)

	for _, seat := range bySeat(s) {
		if got := s.Players[seat].LastOfferRound; got != 0 {
			t.Errorf("seat %d LastOfferRound = %d, want 0 (due immediately)", seat, got)
		}
	}
}

// TestResolveDeadRunnerPlacesLooseCrate is GDD §14.2's Dead Runner: "A
// crate appears at a random node, announced publicly" — an unbound loose
// crate (cargo.go's CanCollect: collectible by anyone), one
// PurposeCrateNode draw (D3, shared with the Spilled Load incident).
func TestResolveDeadRunnerPlacesLooseCrate(t *testing.T) {
	s := eventsTestState()
	r := NewRNG(testSeed(1), 7)

	events := resolveDeadRunner(&s, r)

	if len(s.Graph.Cargo) != 1 {
		t.Fatalf("Graph.Cargo = %+v, want exactly 1 crate", s.Graph.Cargo)
	}
	if s.Graph.Cargo[0].Bound {
		t.Error("dropped crate is Bound, want an unbound loose crate")
	}
	if got := r.Consumed(PurposeCrateNode); got != 1 {
		t.Errorf("PurposeCrateNode consumed = %d, want 1", got)
	}
	if len(events) != 1 || events[0].Kind != game.EventDeadRunnerCrate || events[0].Node != s.Graph.Cargo[0].Node {
		t.Errorf("events = %+v, want one EventDeadRunnerCrate naming the crate's node", events)
	}
}

// TestResolveBountyPaysEachWinnerAgainstHighestRP is GDD §14.2's Bounty:
// "The highest-RP player has a price on their head. Whoever defeats them
// in a confrontation this round takes Cr$ 10 from the bank" — per D14 §5,
// scales with however many times the target is actually defeated, scanned
// from this round's own already-resolved EventConfrontation rows (Seat the
// winner, Target the loser — confront.go's decisiveEvents).
func TestResolveBountyPaysEachWinnerAgainstHighestRP(t *testing.T) {
	s := eventsTestState(0, 0, 0)
	s.Players[0].RP, s.Players[1].RP, s.Players[2].RP = 1, 9, 2
	roundEvents := []game.Event{
		{Kind: game.EventConfrontation, Seat: 0, Target: 1}, // seat 0 defeated the target
		{Kind: game.EventConfrontation, Seat: 2, Target: 0}, // unrelated: target 0, not the bounty
	}

	resolveBounty(&s, roundEvents, NewRNG(testSeed(1), 7))

	if got := s.Players[0].Balance; got != 30 {
		t.Errorf("seat 0 (defeated the highest-RP target) Balance = %d, want 30", got)
	}
	if got := s.Players[2].Balance; got != 20 {
		t.Errorf("seat 2 (defeated a non-target) Balance = %d, want unchanged 20", got)
	}
}

// TestResolveBlackoutSetsNextRoundModifier is GDD §14.2's Blackout: "Next
// round nobody generates trail entries and nobody has sight beyond their
// own node" — sets the flag trail.go's writeTrail/seatSight consume the
// round after this one (trail_test.go covers consumption).
func TestResolveBlackoutSetsNextRoundModifier(t *testing.T) {
	s := eventsTestState()
	resolveBlackout(&s)
	if !s.NextRound.Blackout {
		t.Error("NextRound.Blackout = false, want true")
	}
}

// TestResolveDockersStrikeSetsNextRoundModifier is GDD §14.2's Dockers'
// Strike: "No Pickup action may be performed next round" — sets the flag
// legal.go's legalAction consumes the round after this one (legal_test.go
// covers consumption).
func TestResolveDockersStrikeSetsNextRoundModifier(t *testing.T) {
	s := eventsTestState()
	resolveDockersStrike(&s)
	if !s.NextRound.DockersStrike {
		t.Error("NextRound.DockersStrike = false, want true")
	}
}

// TestResolveRetainerSetsNextRoundModifier is GDD §14.2's Retainer: "Every
// player carrying no cargo gains +2 steps next round" — sets the flag
// validate.go's legalView consumes the round after this one
// (validate_test.go covers consumption).
func TestResolveRetainerSetsNextRoundModifier(t *testing.T) {
	s := eventsTestState()
	resolveRetainer(&s)
	if !s.NextRound.Retainer {
		t.Error("NextRound.Retainer = false, want true")
	}
}

// TestResolveScaffoldingDrawsSectorAndConsumesRNG is GDD §14.2's
// Scaffolding: draws one of the four sectors (PurposeEventScaffolding, D3)
// and sets it as the next-round modifier (consumption in
// validate_test.go).
func TestResolveScaffoldingDrawsSectorAndConsumesRNG(t *testing.T) {
	s := eventsTestState()
	r := NewRNG(testSeed(1), 7)

	resolveScaffolding(&s, r)

	if s.NextRound.Scaffolding == nil {
		t.Fatal("NextRound.Scaffolding = nil, want a drawn sector")
	}
	if got := r.Consumed(PurposeEventScaffolding); got != 1 {
		t.Errorf("PurposeEventScaffolding consumed = %d, want 1", got)
	}
}

// TestResolveBridgeDownRemovesEdgePermanently is GDD §14.2's Bridge Down:
// "One random edge is destroyed permanently" — one PurposeEventBridgeDown
// draw (D3), removed from both endpoints' Node.Edges so it survives
// refold/clone for free (an ordinary Graph mutation, no separate flag).
func TestResolveBridgeDownRemovesEdgePermanently(t *testing.T) {
	s := eventsTestState()
	before := navigableEdges(s.Graph)
	r := NewRNG(testSeed(1), 7)

	resolveBridgeDown(&s, r)

	after := navigableEdges(s.Graph)
	if len(after) != len(before)-1 {
		t.Fatalf("navigable edges = %d, want %d (one removed)", len(after), len(before)-1)
	}
	if got := r.Consumed(PurposeEventBridgeDown); got != 1 {
		t.Errorf("PurposeEventBridgeDown consumed = %d, want 1", got)
	}

	// Refold check: cloning the resulting state must preserve the removal.
	clone := s.clone()
	if len(navigableEdges(clone.Graph)) != len(after) {
		t.Error("Bridge Down's edge removal did not survive clone()")
	}
}

// TestResolveFestivalAwardInfamyToEveryoneAtTheDrawnNode is half of GDD
// §14.2's Festival: "+1 Infamy" for whoever ends this round at the
// already-drawn node (ctx.festivalNode, buildGlobalEventContext — see
// TestBuildGlobalEventContextFestivalDrawsEarly for the draw itself).
// "Leaves no trace" is trail.go's own concern (TestWriteTrailFestivalNoTrace,
// trail_test.go).
func TestResolveFestivalAwardInfamyToEveryoneAtTheDrawnNode(t *testing.T) {
	s := eventsTestState(2, 4)
	s.Players[0].Infamy, s.Players[1].Infamy = 3, 3

	resolveFestivalAward(&s, 2)

	if got := s.Players[0].Infamy; got != 4 {
		t.Errorf("seat 0 (at the drawn node) Infamy = %d, want 4", got)
	}
	if got := s.Players[1].Infamy; got != 3 {
		t.Errorf("seat 1 (elsewhere) Infamy = %d, want unchanged 3", got)
	}
}

// TestBuildGlobalEventContextDragnetDrawsBordersEarly is issue #72's own
// architectural centerpiece: Dragnet's target set is drawn at Resolve's
// round-start peek, not at globalEvent's Phase 6 call site, because
// validate (Curfew-shaped Deliver degradation) and resolveDeliveries both
// need it before Phase 6 ever runs. eventsTestGraph has exactly 2 Borders
// (1, 2), so PartialFisherYates's own min(k, n) selects both — a
// deterministic set, exercising D3's "min(2, len(candidates))" formula at
// its own boundary.
func TestBuildGlobalEventContextDragnetDrawsBordersEarly(t *testing.T) {
	s := eventsTestState()
	s.Graph.EventDeck = []EventCardID{EventDragnet} // round 7 -> deck[7-4] = deck[3]... see below
	s.Round = 4                                     // deck[0]: the first live round

	r := NewRNG(testSeed(1), 4)
	ctx := buildGlobalEventContext(s, r)

	if !ctx.live || ctx.card != EventDragnet {
		t.Fatalf("ctx = %+v, want live Dragnet", ctx)
	}
	if len(ctx.sealedBorders) != 2 {
		t.Fatalf("sealedBorders = %v, want 2 (both Borders in a 2-Border graph)", ctx.sealedBorders)
	}
	for _, want := range []game.NodeID{1, 2} {
		found := false
		for _, got := range ctx.sealedBorders {
			found = found || got == want
		}
		if !found {
			t.Errorf("sealedBorders = %v, want to include node %d", ctx.sealedBorders, want)
		}
	}
	if got := r.Consumed(PurposeEventDragnet); got != 2 {
		t.Errorf("PurposeEventDragnet consumed = %d, want 2", got)
	}
}

// TestBuildGlobalEventContextShippingBoomDrawsWarehouseEarly mirrors
// Dragnet's own early-draw reasoning: a Pickup's +Cr$5 bonus resolves at
// Step N+1 (actions.go), before Phase 6 pops the card.
func TestBuildGlobalEventContextShippingBoomDrawsWarehouseEarly(t *testing.T) {
	s := eventsTestState()
	s.Graph.EventDeck = []EventCardID{EventShippingBoom}
	s.Round = 4

	r := NewRNG(testSeed(1), 4)
	ctx := buildGlobalEventContext(s, r)

	if !ctx.live || ctx.card != EventShippingBoom {
		t.Fatalf("ctx = %+v, want live Shipping Boom", ctx)
	}
	if got := s.Graph.Nodes[ctx.shippingBoomNode].Type; got != game.NodeWarehouse {
		t.Errorf("shippingBoomNode %d has type %s, want a Warehouse", ctx.shippingBoomNode, got)
	}
	if got := r.Consumed(PurposeEventShippingBoom); got != 1 {
		t.Errorf("PurposeEventShippingBoom consumed = %d, want 1", got)
	}
}

// TestBuildGlobalEventContextFestivalDrawsNodeEarly mirrors Dragnet's own
// early-draw reasoning: writeTrail (Step N+4) needs to know Festival's node
// before Phase 6 pops the card, to suppress that seat's own entries there
// this same round.
func TestBuildGlobalEventContextFestivalDrawsNodeEarly(t *testing.T) {
	s := eventsTestState()
	s.Graph.EventDeck = []EventCardID{EventFestival}
	s.Round = 4

	r := NewRNG(testSeed(1), 4)
	ctx := buildGlobalEventContext(s, r)

	if !ctx.live || ctx.card != EventFestival {
		t.Fatalf("ctx = %+v, want live Festival", ctx)
	}
	if got := r.Consumed(PurposeEventFestival); got != 1 {
		t.Errorf("PurposeEventFestival consumed = %d, want 1", got)
	}
}

// TestBuildGlobalEventContextPeeksCurfewGatesClosedPermitAuctionFree
// confirms Curfew, Gates Closed and Permit Auction set their ctx flags
// without consuming any RNG index at all — deterministic once the card's
// identity is known, D3's table has no row for any of the three.
func TestBuildGlobalEventContextPeeksCurfewGatesClosedPermitAuctionFree(t *testing.T) {
	cases := []struct {
		name string
		card EventCardID
		want func(globalEventContext) bool
	}{
		{"Curfew", EventCurfew, func(c globalEventContext) bool { return c.curfewActive }},
		{"GatesClosed", EventGatesClosed, func(c globalEventContext) bool { return c.gatesClosedActive }},
		{"PermitAuction", EventPermitAuction, func(c globalEventContext) bool { return c.permitAuctionActive }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := eventsTestState()
			s.Graph.EventDeck = []EventCardID{tc.card}
			s.Round = 4
			r := NewRNG(testSeed(1), 4)

			ctx := buildGlobalEventContext(s, r)

			if !tc.want(ctx) {
				t.Errorf("ctx = %+v, want %s's own flag set", ctx, tc.name)
			}
			if got := r.Seq(); got != 0 {
				t.Errorf("RNG.Seq() = %d, want 0 (no draw for a deterministic peek)", got)
			}
		})
	}
}

// TestGlobalEventNoOpWhenNoCardLive confirms rounds 1-3 (GDD §14.2: "Rounds
// 1-3 have no global event") produce nothing.
func TestGlobalEventNoOpWhenNoCardLive(t *testing.T) {
	s := eventsTestState()
	s.Round = 2
	ctx := buildGlobalEventContext(s, NewRNG(testSeed(1), 2))

	if events := globalEvent(&s, ctx, nil, NewRNG(testSeed(1), 2)); events != nil {
		t.Errorf("globalEvent() = %+v, want nil", events)
	}
}

// TestGlobalEventAlreadyAppliedCardsAreNoOpAtPop confirms the six cards
// whose effect was already fully applied earlier this same round (Curfew,
// Gates Closed, Permit Auction, Dragnet, Shipping Boom, Rain) do nothing a
// second time when globalEvent finally pops the card — guarding against a
// double-application bug, since each already has its own earlier-consumer
// test above or in validate_test.go/deliveries_test.go/addons_test.go/
// trail.go's existing rainActive coverage.
func TestGlobalEventAlreadyAppliedCardsAreNoOpAtPop(t *testing.T) {
	for _, card := range []EventCardID{
		EventCurfew, EventGatesClosed, EventPermitAuction, EventDragnet, EventShippingBoom, EventRain,
	} {
		t.Run(allEvents[card-1].Name, func(t *testing.T) {
			s := eventsTestState(0, 0)
			s.Players[0].Balance, s.Players[0].Infamy = 20, 5
			ctx := globalEventContext{card: card, live: true}

			events := globalEvent(&s, ctx, nil, NewRNG(testSeed(1), 7))

			if events != nil {
				t.Errorf("globalEvent() = %+v, want nil (already applied earlier this round)", events)
			}
			if s.Players[0].Balance != 20 || s.Players[0].Infamy != 5 {
				t.Errorf("state mutated at pop time: Balance=%d Infamy=%d, want unchanged", s.Players[0].Balance, s.Players[0].Infamy)
			}
		})
	}
}
