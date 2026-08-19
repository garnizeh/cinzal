package rules

import (
	"slices"
	"testing"

	"github.com/garnizeh/cinzal/internal/game"
)

// incidentsTestState reuses eventsTestGraph/eventsTestState (events_test.go)
// — the same 6-node, 4-sector ring rich enough for every card's candidate
// set — since incident cards need exactly the same shape (multiple
// sectors, a Black Market, a Warehouse, an Alley) events.go's own tests
// already established.
func incidentsTestState(start ...game.NodeID) MatchState {
	return eventsTestState(start...)
}

// incidentsTestContext builds a live incidentContext for card, flagged at
// sector — every resolveXxx function in incidents.go takes this directly,
// so tests never need to go through buildIncidentContext's own deck peek.
func incidentsTestContext(card IncidentCardID, sector game.Sector) incidentContext {
	sec := sector
	return incidentContext{sector: &sec, card: card, live: true}
}

// TestIncidentCardThisRoundIndexesFromRoundThree is GDD §14.3: "From round
// 3 onward" — deck[0] is round 3's card, mirroring eventCardThisRound's
// deck[0] == round 4 (trail.go).
func TestIncidentCardThisRoundIndexesFromRoundThree(t *testing.T) {
	deck := []IncidentCardID{IncidentFlood, IncidentSnatchJob, IncidentGuardSweep}

	if card, ok := incidentCardThisRound(3, deck); !ok || card != IncidentFlood {
		t.Errorf("round 3 = (%v, %v), want (Flood, true)", card, ok)
	}
	if card, ok := incidentCardThisRound(5, deck); !ok || card != IncidentGuardSweep {
		t.Errorf("round 5 = (%v, %v), want (GuardSweep, true)", card, ok)
	}
	if _, ok := incidentCardThisRound(2, deck); ok {
		t.Error("round 2 live = true, want false (before round 3)")
	}
	if _, ok := incidentCardThisRound(6, deck); ok {
		t.Error("round 6 live = true, want false (past the deck)")
	}
}

// TestBuildIncidentContextLiveOnlyWithCardAndSector confirms the peek reads
// both s.UnstableSector and the deck's own round index.
func TestBuildIncidentContextLiveOnlyWithCardAndSector(t *testing.T) {
	s := incidentsTestState()
	s.Round = 4
	sector := game.SectorOldDocks
	s.UnstableSector = &sector
	s.Graph.IncidentDeck = []IncidentCardID{IncidentFlood, IncidentSnatchJob}

	ctx := buildIncidentContext(s)

	if !ctx.live || ctx.card != IncidentSnatchJob || ctx.sector == nil || *ctx.sector != sector {
		t.Errorf("buildIncidentContext = %+v, want live SnatchJob at SectorOldDocks (round 4 = deck[1])", ctx)
	}
}

// TestBuildIncidentContextNotLiveBeforeRoundThree confirms a pre-drawn
// round-3 sector (set by initial(), state.go's own doc) does not make round
// 1-2 report live.
func TestBuildIncidentContextNotLiveBeforeRoundThree(t *testing.T) {
	s := incidentsTestState()
	s.Round = 2
	sector := game.SectorOldDocks
	s.UnstableSector = &sector
	s.Graph.IncidentDeck = []IncidentCardID{IncidentFlood}

	if ctx := buildIncidentContext(s); ctx.live {
		t.Errorf("buildIncidentContext = %+v, want live=false (round 2 has no incident yet)", ctx)
	}
}

// TestInitialUnstableSectorNilBelowThreeRounds is GDD §14.3: no incident
// round exists at all for a match shorter than 3 rounds.
func TestInitialUnstableSectorNilBelowThreeRounds(t *testing.T) {
	cfg := legalTestConfig()
	cfg.Rounds = 2
	r := NewRNG(testSeed(1), 0)

	if got := initialUnstableSector(r, cfg); got != nil {
		t.Errorf("initialUnstableSector = %v, want nil", *got)
	}
}

// TestNextUnstableSectorExcludesCurrent is GDD §14.3: "The same sector
// cannot be flagged two rounds running."
func TestNextUnstableSectorExcludesCurrent(t *testing.T) {
	s := incidentsTestState()
	sector := game.SectorOldDocks
	s.UnstableSector = &sector
	s.Round = 5
	cfg := legalTestConfig()
	cfg.Rounds = 15
	r := NewRNG(testSeed(1), 5)

	got := nextUnstableSector(s, cfg, r)

	if got == nil {
		t.Fatal("nextUnstableSector = nil, want a sector (round 5 < cfg.Rounds)")
	}
	if *got == sector {
		t.Errorf("nextUnstableSector = %v, want different from current %v", *got, sector)
	}
}

// TestNextUnstableSectorNilOnFinalRound confirms nothing is drawn once the
// match has no further round to announce it for.
func TestNextUnstableSectorNilOnFinalRound(t *testing.T) {
	s := incidentsTestState()
	sector := game.SectorOldDocks
	s.UnstableSector = &sector
	s.Round = 15
	cfg := legalTestConfig()
	cfg.Rounds = 15
	r := NewRNG(testSeed(1), 15)

	if got := nextUnstableSector(s, cfg, r); got != nil {
		t.Errorf("nextUnstableSector = %v, want nil", *got)
	}
}

// TestIncidentEligibleFiltersByEndingSector is GDD §14.3's own framing:
// "Effect on players ending in the unstable sector."
func TestIncidentEligibleFiltersByEndingSector(t *testing.T) {
	s := incidentsTestState(1, 3) // node 1 Old Docks, node 3 Mist Heights
	validated := map[game.SeatID]game.Order{0: {}, 1: {}}

	got, exposed := incidentEligible(s, validated, []game.SeatID{0, 1}, game.SectorOldDocks)

	if len(got) != 1 || got[0] != 0 {
		t.Errorf("incidentEligible = %v, want [0]", got)
	}
	if len(exposed) != 1 || exposed[0].Kind != game.EventIncidentExposed || exposed[0].Seat != 0 {
		t.Errorf("incidentEligible exposed events = %v, want one EventIncidentExposed for seat 0", exposed)
	}
}

// TestIncidentEligibleExcludesCirculationPermit is GDD §12: "immune to
// this round's Sector Incident."
func TestIncidentEligibleExcludesCirculationPermit(t *testing.T) {
	s := incidentsTestState(1, 1)
	validated := map[game.SeatID]game.Order{
		0: {},
		1: {Items: []game.ItemDiscard{{Item: game.ItemCirculationPermit}}},
	}

	got, exposed := incidentEligible(s, validated, []game.SeatID{0, 1}, game.SectorOldDocks)

	if len(got) != 1 || got[0] != 0 {
		t.Errorf("incidentEligible = %v, want [0] (seat 1 exempt via Circulation Permit)", got)
	}
	if len(exposed) != 1 || exposed[0].Seat != 0 {
		t.Errorf("incidentEligible exposed events = %v, want one EventIncidentExposed for seat 0 (seat 1 exempt)", exposed)
	}
}

// TestRetreatTowardSectorEdgeNoExitWhenFullyInterior confirms a boxed-in
// node simply doesn't move — nothing in GDD text forces an exit that
// doesn't exist.
func TestRetreatTowardSectorEdgeNoExitWhenFullyInterior(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: 0, Sector: game.SectorOldDocks, Edges: []game.NodeID{1}},
		{ID: 1, Sector: game.SectorOldDocks, Edges: []game.NodeID{0}},
	}}
	if _, ok := retreatTowardSectorEdge(g, 0, game.SectorOldDocks); ok {
		t.Error("retreatTowardSectorEdge ok = true, want false (every neighbor still in-sector)")
	}
}

// TestResolveFloodDropsCargoAndRetreats is GDD §14.3's Flood: "Carried
// cargo drops at your node. Retreat 1 step toward the sector edge."
func TestResolveFloodDropsCargoAndRetreats(t *testing.T) {
	s := incidentsTestState(1) // node 1: Border, Old Docks, edges {0, 2}
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true, Contract: 0}
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 5}}
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentFlood, game.SectorOldDocks)

	resolveFlood(&s, ctx, validated, []game.SeatID{0})

	if s.Players[0].Cargo != nil {
		t.Error("still carries cargo, want dropped")
	}
	if len(s.Graph.Cargo) != 1 || s.Graph.Cargo[0].Node != 1 {
		t.Fatalf("Graph.Cargo = %+v, want one dropped crate at node 1", s.Graph.Cargo)
	}
	if got := s.Players[0].Position; got != 2 {
		t.Errorf("Position = %d, want 2 (lowest-NodeID neighbor outside Old Docks)", got)
	}
}

// TestResolveSnatchJobRelocatesAndConsumesRNG is GDD §14.3's Snatch Job:
// "Lose Cr$ 6 and your cargo (gone, not dropped). Dumped at a random node
// in a different sector."
func TestResolveSnatchJobRelocatesAndConsumesRNG(t *testing.T) {
	s := incidentsTestState(1)
	s.Players[0].Balance = 20
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false}
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentSnatchJob, game.SectorOldDocks)
	r := NewRNG(testSeed(1), 5)

	resolveSnatchJob(&s, ctx, validated, []game.SeatID{0}, r)

	if got := s.Players[0].Balance; got != 14 {
		t.Errorf("Balance = %d, want 14 (20 - 6)", got)
	}
	if s.Players[0].Cargo != nil {
		t.Error("still carries cargo, want gone")
	}
	if len(s.Graph.Cargo) != 0 {
		t.Errorf("Graph.Cargo = %+v, want empty — cargo is gone, not dropped", s.Graph.Cargo)
	}
	if got := s.Graph.Nodes[s.Players[0].Position].Sector; got == game.SectorOldDocks {
		t.Errorf("relocated to sector %v, want a different sector", got)
	}
	if got := r.Consumed(PurposeIncidentRelocate); got != 1 {
		t.Errorf("PurposeIncidentRelocate consumed = %d, want 1", got)
	}
}

// TestResolveSnatchJobReturnsDebtEventOnSurrender is the same fix
// TestResolveShakedownReturnsDebtEventOnSurrender documents, for Snatch
// Job's own applyDebt call site.
func TestResolveSnatchJobReturnsDebtEventOnSurrender(t *testing.T) {
	s := incidentsTestState(1)
	s.Players[0].Balance = 0
	s.Players[0].Posts = []game.NodeID{2}
	s.Graph.Nodes[2].Post = &Post{Owner: 0, RoundsRemaining: 3}
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentSnatchJob, game.SectorOldDocks)
	r := NewRNG(testSeed(1), 5)

	events := resolveSnatchJob(&s, ctx, validated, []game.SeatID{0}, r)

	if len(events) != 3 || events[0].Kind != game.EventIncidentExposed || events[1].Kind != game.EventLeaseExpired || events[1].Node != 2 || events[2].Kind != game.EventIncidentHit {
		t.Errorf("events = %+v, want EventIncidentExposed, EventLeaseExpired at node 2 (Debt-forced surrender), EventIncidentHit", events)
	}
}

// TestResolveGuardSweepLosesCashInfamyAndCargo is GDD §14.3's Guard Sweep:
// "Lose Cr$ 5 and −1 Infamy. Carrying cargo, you lose it too."
func TestResolveGuardSweepLosesCashInfamyAndCargo(t *testing.T) {
	s := incidentsTestState(1)
	s.Players[0].Balance = 20
	s.Players[0].Infamy = 5
	s.Players[0].Cargo = &game.CarriedCargo{Bound: true, Contract: 0}
	s.Players[0].Contracts = []Contract{{ID: 0, Origin: 0, Destination: 5}}
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentGuardSweep, game.SectorOldDocks)

	resolveGuardSweep(&s, ctx, validated, []game.SeatID{0})

	if got := s.Players[0].Balance; got != 15 {
		t.Errorf("Balance = %d, want 15 (20 - 5)", got)
	}
	if got := s.Players[0].Infamy; got != 4 {
		t.Errorf("Infamy = %d, want 4 (5 - 1)", got)
	}
	if s.Players[0].Cargo != nil {
		t.Error("still carries cargo, want gone")
	}
	if len(s.Players[0].Contracts) != 1 {
		t.Errorf("Contracts = %+v, want the underlying contract left untouched", s.Players[0].Contracts)
	}
}

// TestResolveGuardSweepReturnsDebtEventOnSurrender is the same fix
// TestResolveShakedownReturnsDebtEventOnSurrender documents, for Guard
// Sweep's own applyDebt call site.
func TestResolveGuardSweepReturnsDebtEventOnSurrender(t *testing.T) {
	s := incidentsTestState(1)
	s.Players[0].Balance = 0
	s.Players[0].Posts = []game.NodeID{2}
	s.Graph.Nodes[2].Post = &Post{Owner: 0, RoundsRemaining: 3}
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentGuardSweep, game.SectorOldDocks)

	events := resolveGuardSweep(&s, ctx, validated, []game.SeatID{0})

	if len(events) != 3 || events[0].Kind != game.EventIncidentExposed || events[1].Kind != game.EventLeaseExpired || events[1].Node != 2 || events[2].Kind != game.EventIncidentHit {
		t.Errorf("events = %+v, want EventIncidentExposed, EventLeaseExpired at node 2 (Debt-forced surrender), EventIncidentHit", events)
	}
}

// TestResolveTorchedDecrementsEveryPostInSectorEvenBelowZero is D14 §1
// (decided): "only ever decrements... allowed to go negative."
func TestResolveTorchedDecrementsEveryPostInSectorEvenBelowZero(t *testing.T) {
	s := incidentsTestState()
	s.Graph.Nodes[0].Post = &Post{Owner: 0, RoundsRemaining: 2} // Old Docks
	s.Graph.Nodes[1].Post = &Post{Owner: 1, RoundsRemaining: 5} // Old Docks
	s.Graph.Nodes[4].Post = &Post{Owner: 0, RoundsRemaining: 5} // North Vale, untouched

	resolveTorched(&s, game.SectorOldDocks)

	if got := s.Graph.Nodes[0].Post.RoundsRemaining; got != -1 {
		t.Errorf("node 0 RoundsRemaining = %d, want -1 (2-3)", got)
	}
	if got := s.Graph.Nodes[1].Post.RoundsRemaining; got != 2 {
		t.Errorf("node 1 RoundsRemaining = %d, want 2 (5-3)", got)
	}
	if got := s.Graph.Nodes[4].Post.RoundsRemaining; got != 5 {
		t.Errorf("node 4 (outside sector) RoundsRemaining = %d, want unchanged 5", got)
	}
}

// TestResolveTurfWarWinSurvivesWorstRoll is GDD §14.3's Turf War: "Roll D6
// + your confrontation modifiers against a flat 9." A maximal modifier
// stack wins even on the worst possible roll (1): 1+3(Legend)+1(Aggressive)
// +1(Alley)+3(Shiv)+1(Muscle)+2(stake/3 capped) = 12 > 9.
func TestResolveTurfWarWinSurvivesWorstRoll(t *testing.T) {
	s := incidentsTestState(5) // node 5: Alley, North Vale
	s.Players[0].Infamy = 9
	s.Players[0].Items = []game.ItemID{game.ItemMuscle}
	validated := map[game.SeatID]game.Order{0: {
		Stance: game.StanceOrder{Stance: game.StanceAggressive, Stake: 6},
		Items:  []game.ItemDiscard{{Item: game.ItemShiv}},
	}}
	walks := map[game.SeatID]*seatWalk{0: {}}
	ctx := incidentsTestContext(IncidentTurfWar, game.SectorNorthVale)
	r := NewRNG(testSeed(1), 5)

	resolveTurfWar(&s, ctx, validated, []game.SeatID{0}, walks, game.DefaultConfig(), r)

	if got := s.Players[0].Position; got != 5 {
		t.Errorf("Position = %d, want unchanged 5 (win: no retreat)", got)
	}
	if got := r.Consumed(PurposeConfrontD6); got != 1 {
		t.Errorf("PurposeConfrontD6 consumed = %d, want 1", got)
	}
}

// TestResolveTurfWarLossDropsCargoAndRetreats: the worst-case win stack
// inverted — Nobody tier, Evasive, no bonuses — loses even on the best
// possible roll (6): 6+0-1 = 5 <= 9.
func TestResolveTurfWarLossDropsCargoAndRetreats(t *testing.T) {
	s := incidentsTestState(1) // node 1: Border, Old Docks, edges {0, 2}
	s.Players[0].Infamy = 0
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false}
	validated := map[game.SeatID]game.Order{0: {Stance: game.StanceOrder{Stance: game.StanceEvasive}}}
	walks := map[game.SeatID]*seatWalk{0: {}}
	ctx := incidentsTestContext(IncidentTurfWar, game.SectorOldDocks)
	r := NewRNG(testSeed(1), 5)

	resolveTurfWar(&s, ctx, validated, []game.SeatID{0}, walks, game.DefaultConfig(), r)

	if s.Players[0].Cargo != nil {
		t.Error("still carries cargo, want dropped on loss")
	}
	if got := s.Players[0].Position; got != 2 {
		t.Errorf("Position = %d, want 2 (retreat toward the sector edge on loss)", got)
	}
}

// TestResolveStreetsBlockedSetsFlag is GDD §14.3's Streets Blocked: "Your
// route next round is capped at 1 step."
func TestResolveStreetsBlockedSetsFlag(t *testing.T) {
	s := incidentsTestState(1)
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentStreetsBlocked, game.SectorOldDocks)

	resolveStreetsBlocked(&s, ctx, validated, []game.SeatID{0})

	if !s.Players[0].StreetsBlocked {
		t.Error("StreetsBlocked = false, want true")
	}
}

// TestResolveSinkholeSetsThreeRoundsAndConsumesRNG is GDD §14.3's Sinkhole:
// "One random node in the sector is impassable for 3 rounds."
func TestResolveSinkholeSetsThreeRoundsAndConsumesRNG(t *testing.T) {
	s := incidentsTestState()
	r := NewRNG(testSeed(1), 5)

	resolveSinkhole(&s, game.SectorOldDocks, r)

	found := false
	for _, n := range s.Graph.Nodes {
		if n.Sector == game.SectorOldDocks {
			if n.SinkholeRounds != 0 {
				found = true
				if n.SinkholeRounds != 3 {
					t.Errorf("node %d SinkholeRounds = %d, want 3", n.ID, n.SinkholeRounds)
				}
			}
		} else if n.SinkholeRounds != 0 {
			t.Errorf("node %d outside the sector got Sinkholed", n.ID)
		}
	}
	if !found {
		t.Error("no node in the sector was Sinkholed")
	}
	if got := r.Consumed(PurposeIncidentSinkhole); got != 1 {
		t.Errorf("PurposeIncidentSinkhole consumed = %d, want 1", got)
	}
}

// TestResolveSinkholeNoopWhenSectorEmpty confirms an empty candidate pool
// degrades silently rather than panicking on PartialFisherYates(...)[0] —
// D3's own Sinkhole row leaves this pool's safety conditional on
// generation (D8), not fully proven, unlike every other card's whole-map
// pool in this package.
func TestResolveSinkholeNoopWhenSectorEmpty(t *testing.T) {
	s := incidentsTestState()
	s.Graph.Nodes = nil
	r := NewRNG(testSeed(1), 5)

	resolveSinkhole(&s, game.SectorOldDocks, r) // must not panic

	if got := r.Consumed(PurposeIncidentSinkhole); got != 0 {
		t.Errorf("PurposeIncidentSinkhole consumed = %d, want 0 (no candidates)", got)
	}
}

// TestResolveShakedownChargesFlatFee is GDD §14.3's Shakedown: "Every
// player ending here pays Cr$ 4."
func TestResolveShakedownChargesFlatFee(t *testing.T) {
	s := incidentsTestState(1)
	s.Players[0].Balance = 20
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentShakedown, game.SectorOldDocks)

	resolveShakedown(&s, ctx, validated, []game.SeatID{0})

	if got := s.Players[0].Balance; got != 16 {
		t.Errorf("Balance = %d, want 16 (20 - 4)", got)
	}
}

// TestResolveShakedownReturnsDebtEventOnSurrender confirms a Debt-forced
// lease surrender's EventLeaseExpired is captured and returned, matching
// resolveOneDelivery's own precedent (deliveries.go) for every applyDebt
// call site in this package — not silently discarded.
func TestResolveShakedownReturnsDebtEventOnSurrender(t *testing.T) {
	s := incidentsTestState(1)
	s.Players[0].Balance = 0
	s.Players[0].Posts = []game.NodeID{2}
	s.Graph.Nodes[2].Post = &Post{Owner: 0, RoundsRemaining: 3}
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentShakedown, game.SectorOldDocks)

	events := resolveShakedown(&s, ctx, validated, []game.SeatID{0})

	if len(events) != 3 || events[0].Kind != game.EventIncidentExposed || events[1].Kind != game.EventLeaseExpired || events[1].Node != 2 || events[2].Kind != game.EventIncidentHit {
		t.Errorf("events = %+v, want EventIncidentExposed, EventLeaseExpired at node 2 (Debt-forced surrender), EventIncidentHit", events)
	}
}

// TestResolveInformantRingRevealsPosition is GDD §14.3's Informant Ring:
// "Every player ending here has their position revealed publicly."
func TestResolveInformantRingRevealsPosition(t *testing.T) {
	s := incidentsTestState(1)
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentInformantRing, game.SectorOldDocks)

	events := resolveInformantRing(&s, ctx, validated, []game.SeatID{0})

	if len(events) != 2 || events[0].Kind != game.EventIncidentExposed || events[1].Kind != game.EventInformantRing || events[1].Seat != 0 || events[1].Node != 1 {
		t.Errorf("events = %+v, want one EventIncidentExposed then one EventInformantRing{Seat:0, Node:1}", events)
	}
}

// TestResolveSpilledLoadDropsCrateAndConsumesRNG is GDD §14.3's Spilled
// Load: "A crate appears at a random node in the sector... Announced
// publicly."
func TestResolveSpilledLoadDropsCrateAndConsumesRNG(t *testing.T) {
	s := incidentsTestState()
	r := NewRNG(testSeed(1), 5)

	events := resolveSpilledLoad(&s, game.SectorOldDocks, r)

	if len(s.Graph.Cargo) != 1 {
		t.Fatalf("Graph.Cargo = %+v, want one crate", s.Graph.Cargo)
	}
	c := s.Graph.Cargo[0]
	if c.Bound || !c.SpilledLoad {
		t.Errorf("dropped cargo = %+v, want Bound false, SpilledLoad true", c)
	}
	if s.Graph.Nodes[c.Node].Sector != game.SectorOldDocks {
		t.Errorf("crate node sector = %v, want SectorOldDocks", s.Graph.Nodes[c.Node].Sector)
	}
	if len(events) != 1 || events[0].Kind != game.EventSpilledLoadCrate || events[0].Node != c.Node {
		t.Errorf("events = %+v, want one EventSpilledLoadCrate at the crate's node", events)
	}
	if got := r.Consumed(PurposeCrateNode); got != 1 {
		t.Errorf("PurposeCrateNode consumed = %d, want 1", got)
	}
}

// TestResolveSpilledLoadNoopWhenSectorEmpty is the same fix
// TestResolveSinkholeNoopWhenSectorEmpty documents, for Spilled Load's own
// PurposeCrateNode draw.
func TestResolveSpilledLoadNoopWhenSectorEmpty(t *testing.T) {
	s := incidentsTestState()
	s.Graph.Nodes = nil
	r := NewRNG(testSeed(1), 5)

	events := resolveSpilledLoad(&s, game.SectorOldDocks, r) // must not panic

	if events != nil {
		t.Errorf("events = %+v, want nil (no candidates)", events)
	}
	if got := r.Consumed(PurposeCrateNode); got != 0 {
		t.Errorf("PurposeCrateNode consumed = %d, want 0 (no candidates)", got)
	}
}

// TestResolveOneDeliverySpilledLoadPaysIncidentRate confirms
// resolveOneDelivery (deliveries.go) distinguishes Spilled Load's Cr$10/2RP
// from Dead Runner's Cr$12/3RP via Cargo.SpilledLoad, carried through
// pickup onto game.CarriedCargo.SpilledLoad.
func TestResolveOneDeliverySpilledLoadPaysIncidentRate(t *testing.T) {
	s := incidentsTestState(1) // node 1 is a Border
	s.Players[0].Balance = 20
	s.Players[0].Cargo = &game.CarriedCargo{Bound: false, SpilledLoad: true}
	cfg := legalTestConfig()

	resolveOneDelivery(&s, 0, cfg, globalEventContext{})

	want := 20 - cfg.GateFee + spilledLoadPayout
	if got := s.Players[0].Balance; got != want {
		t.Errorf("Balance = %d, want %d (20 - gate fee %d + %d)", got, want, cfg.GateFee, spilledLoadPayout)
	}
	if got := s.Players[0].RP; got != spilledLoadRP {
		t.Errorf("RP = %d, want %d", got, spilledLoadRP)
	}
}

// TestResolveLocalInformantSetsFlag is GDD §14.3's Local Informant: "gains
// sight of every node within 2 steps of wherever they end their route next
// round."
func TestResolveLocalInformantSetsFlag(t *testing.T) {
	s := incidentsTestState(1)
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentLocalInformant, game.SectorOldDocks)

	resolveLocalInformant(&s, ctx, validated, []game.SeatID{0})

	if !s.Players[0].LocalInformant {
		t.Error("LocalInformant = false, want true")
	}
}

// TestSeatSightGrantsLocalInformantTwoStepRadius confirms seatSight
// (trail.go) reads the flag next round exactly like a declared Surveil.
func TestSeatSightGrantsLocalInformantTwoStepRadius(t *testing.T) {
	s := incidentsTestState(0)
	s.Players[0].LocalInformant = true
	validated := map[game.SeatID]game.Order{0: {}}

	sight := seatSight(s, validated, 0, false)

	want := map[game.NodeID]bool{0: true, 1: true, 5: true, 2: true, 4: true}
	got := map[game.NodeID]bool{}
	for _, n := range sight {
		got[n] = true
	}
	if len(got) != len(want) {
		t.Fatalf("seatSight = %v, want nodes within 2 steps of node 0: %v", sight, want)
	}
	for n := range want {
		if !got[n] {
			t.Errorf("seatSight missing node %d", n)
		}
	}
}

// TestResolveDistractedGuardSetsStepFlag is the step half of GDD §14.3's
// Distracted Guard: "gains +1 step next round."
func TestResolveDistractedGuardSetsStepFlag(t *testing.T) {
	s := incidentsTestState(1)
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentDistractedGuard, game.SectorOldDocks)

	resolveDistractedGuard(&s, ctx, validated, []game.SeatID{0})

	if !s.Players[0].DistractedGuard {
		t.Error("DistractedGuard = false, want true")
	}
}

// TestOwnTraceSuppressedByDistractedGuard is the trace half of GDD §14.3's
// Distracted Guard: "leaves no trace this round" — resolved inside
// writeTrail (trail.go), not incident(), since incident() runs too late
// this same round (see incidentContext's own doc).
func TestOwnTraceSuppressedByDistractedGuard(t *testing.T) {
	s := incidentsTestState(1)
	validated := map[game.SeatID]game.Order{0: {}}
	incCtx := incidentsTestContext(IncidentDistractedGuard, game.SectorOldDocks)

	if !ownTraceSuppressed(s, globalEventContext{}, incCtx, validated, 0, 1) {
		t.Error("ownTraceSuppressed = false, want true (ending in the flagged sector)")
	}
}

// TestOwnTraceSuppressedByDistractedGuardExemptWithCirculationPermit is GDD
// §12: a seat immune to the incident is immune to both of its halves.
func TestOwnTraceSuppressedByDistractedGuardExemptWithCirculationPermit(t *testing.T) {
	s := incidentsTestState(1)
	validated := map[game.SeatID]game.Order{0: {Items: []game.ItemDiscard{{Item: game.ItemCirculationPermit}}}}
	incCtx := incidentsTestContext(IncidentDistractedGuard, game.SectorOldDocks)

	if ownTraceSuppressed(s, globalEventContext{}, incCtx, validated, 0, 1) {
		t.Error("ownTraceSuppressed = true, want false — Circulation Permit exempts this seat")
	}
}

// TestResolveOpenDoorsBuysDeclaredItemAtHalfPrice applies D14 §4's
// mechanism for GDD §14.3's Open Doors.
func TestResolveOpenDoorsBuysDeclaredItemAtHalfPrice(t *testing.T) {
	s := incidentsTestState(3) // node 3: Black Market, Mist Heights
	s.Players[0].Balance = 20
	s.Players[0].Fog = []game.FogState{
		game.FogInSight, game.FogInSight, game.FogInSight,
		game.FogInSight, game.FogInSight, game.FogInSight,
	}
	s.Graph.Nodes[3].Market = []game.ItemID{game.ItemMuscle}
	market := game.NodeID(3)
	validated := map[game.SeatID]game.Order{0: {
		AddOns: game.AddOns{OpenDoorsMarket: &market, OpenDoorsItem: game.ItemMuscle},
	}}
	ctx := incidentsTestContext(IncidentOpenDoors, game.SectorMistHeights)

	events := resolveOpenDoors(&s, ctx, validated, []game.SeatID{0})

	wantPrice := itemPrice(game.ItemMuscle) / 2
	if got := s.Players[0].Balance; got != 20-wantPrice {
		t.Errorf("Balance = %d, want %d", got, 20-wantPrice)
	}
	if !slices.Contains(s.Players[0].Items, game.ItemMuscle) {
		t.Errorf("Items = %v, want to contain Muscle", s.Players[0].Items)
	}
	if len(s.Graph.Nodes[3].Market) != 0 {
		t.Errorf("Market = %v, want emptied", s.Graph.Nodes[3].Market)
	}
	if len(events) != 2 || events[0].Kind != game.EventIncidentExposed || events[1].Kind != game.EventItemPurchased {
		t.Errorf("events = %+v, want one EventIncidentExposed then one EventItemPurchased", events)
	}
}

// TestResolveOpenDoorsDegradesWhenNotDeclared is D14 §4: "degrades
// silently, never rejects."
func TestResolveOpenDoorsDegradesWhenNotDeclared(t *testing.T) {
	s := incidentsTestState(3)
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentOpenDoors, game.SectorMistHeights)

	events := resolveOpenDoors(&s, ctx, validated, []game.SeatID{0})

	if len(events) != 1 || events[0].Kind != game.EventIncidentExposed {
		t.Errorf("events = %+v, want one EventIncidentExposed and nothing else (nothing declared)", events)
	}
}

// TestResolveOpenDoorsDegradesOnOutOfRangeMarket confirms a client-declared
// OpenDoorsMarket outside the graph's node range degrades silently rather
// than panicking Resolve for the whole table — Legal accepts the
// declaration unconditionally at submission (D14 §4), so this function is
// the only remaining bounds check before Graph.Nodes is indexed.
func TestResolveOpenDoorsDegradesOnOutOfRangeMarket(t *testing.T) {
	s := incidentsTestState(3)
	oob := game.NodeID(999)
	validated := map[game.SeatID]game.Order{0: {
		AddOns: game.AddOns{OpenDoorsMarket: &oob, OpenDoorsItem: game.ItemMuscle},
	}}
	ctx := incidentsTestContext(IncidentOpenDoors, game.SectorMistHeights)

	events := resolveOpenDoors(&s, ctx, validated, []game.SeatID{0}) // must not panic

	if len(events) != 1 || events[0].Kind != game.EventIncidentExposed {
		t.Errorf("events = %+v, want one EventIncidentExposed and nothing else (out-of-range OpenDoorsMarket)", events)
	}
}

// TestResolveWordOfWorkResetsCooldown is GDD §14.3's Word of Work:
// "immediately receives a contract offer, ignoring the Contact Cooldown."
func TestResolveWordOfWorkResetsCooldown(t *testing.T) {
	s := incidentsTestState(1)
	s.Players[0].LastOfferRound = 10
	validated := map[game.SeatID]game.Order{0: {}}
	ctx := incidentsTestContext(IncidentWordOfWork, game.SectorOldDocks)

	resolveWordOfWork(&s, ctx, validated, []game.SeatID{0})

	if s.Players[0].LastOfferRound != 0 {
		t.Errorf("LastOfferRound = %d, want 0", s.Players[0].LastOfferRound)
	}
}

// TestAdvanceTruncatesGasLeakEntryAndStaysLazy is GDD §14.3's Gas Leak:
// "routes truncate at the last node outside it, resolved during
// movement" — and RFC §6.4's lazy-draw rule: a step never taken consumes
// no index.
func TestAdvanceTruncatesGasLeakEntryAndStaysLazy(t *testing.T) {
	s := incidentsTestState(0)
	s.Players[0].Fog = make([]game.FogState, len(s.Graph.Nodes)) // all Hidden is fine: to==from at the truncation point, so scavenge never runs there
	sector := game.SectorIronLow                                 // node 2
	incCtx := incidentContext{sector: &sector, card: IncidentGasLeak, live: true}
	validated := map[game.SeatID]game.Order{0: {Route: []game.NodeID{1, 2}, PushingOn: game.PushingOn{Steps: 2}}}
	walks := newSeatWalks(s, []game.SeatID{0})
	r := NewRNG(testSeed(1), 5)

	advance(&s, walks, validated, []game.SeatID{0}, 1, incCtx, r) // 0->1, legal
	advance(&s, walks, validated, []game.SeatID{0}, 2, incCtx, r) // 1->2 would enter the flagged sector

	if got := s.Players[0].Position; got != 1 {
		t.Errorf("Position = %d, want 1 (truncated before entering the flagged sector)", got)
	}
	if o := validated[0]; o.Route != nil || o.PushingOn.Steps != 0 {
		t.Errorf("validated[0] = %+v, want Route/PushingOn cleared", o)
	}
	if got := r.Consumed(PurposePushonEdge); got != 0 {
		t.Errorf("PurposePushonEdge consumed = %d, want 0 (truncation halts before any blind step runs)", got)
	}
}

// TestAdvanceGasLeakExemptsCirculationPermit is GDD §12: immunity applies
// to Gas Leak's truncation too.
func TestAdvanceGasLeakExemptsCirculationPermit(t *testing.T) {
	s := incidentsTestState(0)
	s.Players[0].Fog = make([]game.FogState, len(s.Graph.Nodes))
	sector := game.SectorIronLow
	incCtx := incidentContext{sector: &sector, card: IncidentGasLeak, live: true}
	validated := map[game.SeatID]game.Order{0: {
		Route: []game.NodeID{1, 2},
		Items: []game.ItemDiscard{{Item: game.ItemCirculationPermit}},
	}}
	walks := newSeatWalks(s, []game.SeatID{0})
	r := NewRNG(testSeed(1), 5)

	advance(&s, walks, validated, []game.SeatID{0}, 1, incCtx, r)
	advance(&s, walks, validated, []game.SeatID{0}, 2, incCtx, r)

	if got := s.Players[0].Position; got != 2 {
		t.Errorf("Position = %d, want 2 (Circulation Permit exempts this seat)", got)
	}
}

// TestApplyRiotPermutationStaysWithinRealOrigins is D4's own core
// guarantee: a closed permutation of real origins, never inventing a
// target with no real activity.
func TestApplyRiotPermutationStaysWithinRealOrigins(t *testing.T) {
	s := incidentsTestState()
	sector := game.SectorOldDocks
	incCtx := incidentContext{sector: &sector, card: IncidentRiot, live: true}
	actor1, actor2 := game.SeatID(0), game.SeatID(1)
	entries := map[game.NodeID][]game.TrailEntry{
		0: {{Kind: game.EventFreshTracks, Node: 0}},
		1: {{Kind: game.EventCargoTaken, Node: 1, Actor: &actor1}},
		3: {{Kind: game.EventCargoTaken, Node: 3, Actor: &actor2}}, // outside the sector
	}
	r := NewRNG(testSeed(1), 5)

	applyRiotPermutation(&s, entries, incCtx, r)

	total := 0
	for _, list := range entries {
		total += len(list)
	}
	if total != 3 {
		t.Fatalf("total entries = %d, want 3 (permutation never invents or drops one)", total)
	}
	if len(entries[3]) != 1 || entries[3][0].Actor == nil || *entries[3][0].Actor != actor2 {
		t.Errorf("node 3 (outside the sector) entries = %+v, want the original untouched entry", entries[3])
	}
	for node, list := range entries {
		if node == 3 {
			continue
		}
		if len(list) > 0 && node != 0 && node != 1 {
			t.Errorf("entry landed at node %d, want 0 or 1 (the sector's real origins)", node)
		}
	}
	if got := r.Consumed(PurposeIncidentRiot); got != 2 {
		t.Errorf("PurposeIncidentRiot consumed = %d, want 2 (one per eligible entry)", got)
	}
}

// TestApplyRiotPermutationQuietRoundConsumesNothing is RFC §6.4's
// lazy-draw rule applied to D4's own n=0 case.
func TestApplyRiotPermutationQuietRoundConsumesNothing(t *testing.T) {
	s := incidentsTestState()
	sector := game.SectorOldDocks
	incCtx := incidentContext{sector: &sector, card: IncidentRiot, live: true}
	entries := map[game.NodeID][]game.TrailEntry{}
	r := NewRNG(testSeed(1), 5)

	applyRiotPermutation(&s, entries, incCtx, r)

	if got := r.Consumed(PurposeIncidentRiot); got != 0 {
		t.Errorf("PurposeIncidentRiot consumed = %d, want 0", got)
	}
}

// TestApplyRiotPermutationLeavesGlobalEntriesUntouched is D4: the five
// global-announcement kinds are never eligible.
func TestApplyRiotPermutationLeavesGlobalEntriesUntouched(t *testing.T) {
	s := incidentsTestState()
	sector := game.SectorOldDocks
	incCtx := incidentContext{sector: &sector, card: IncidentRiot, live: true}
	entries := map[game.NodeID][]game.TrailEntry{0: {{Kind: game.EventLoitering, Node: 0}}}
	r := NewRNG(testSeed(1), 5)

	applyRiotPermutation(&s, entries, incCtx, r)

	if len(entries[0]) != 1 || entries[0][0].Kind != game.EventLoitering {
		t.Errorf("entries[0] = %+v, want the Loitering entry untouched", entries[0])
	}
	if got := r.Consumed(PurposeIncidentRiot); got != 0 {
		t.Errorf("PurposeIncidentRiot consumed = %d, want 0", got)
	}
}

// TestPressureConsumesZeroIndicesWithNoLegend is RFC §6.4's lazy-draw rule:
// "No Legend on the table" costs nothing.
func TestPressureConsumesZeroIndicesWithNoLegend(t *testing.T) {
	s := incidentsTestState(0, 0)
	s.Players[0].Infamy, s.Players[1].Infamy = 5, 8 // Known, Feared — neither Legend
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), 5)

	pressure(&s, cfg, r)

	if got := r.Consumed(PurposePressureD6); got != 0 {
		t.Errorf("PurposePressureD6 consumed = %d, want 0", got)
	}
}

// TestPressureConsumesZeroIndicesUnderSuppressInfamyTiers is D11's
// consequence for Suppress.InfamyTiers: Legend is unreachable through the
// gated tier lookup, so Pressure never fires even for a seat sitting at
// numeric Infamy 10 — the SubsystemSuppression doc's own claim that
// "Pressure is already fully suppressed as a consequence" (game/config.go,
// issue #158).
func TestPressureConsumesZeroIndicesUnderSuppressInfamyTiers(t *testing.T) {
	s := incidentsTestState(0, 0)
	s.Players[0].Infamy, s.Players[1].Infamy = 9, 10
	cfg := legalTestConfig()
	cfg.Suppress.InfamyTiers = true
	r := NewRNG(testSeed(1), 5)

	pressure(&s, cfg, r)

	if got := r.Consumed(PurposePressureD6); got != 0 {
		t.Errorf("PurposePressureD6 consumed = %d, want 0 (Legend unreachable under Suppress.InfamyTiers)", got)
	}
}

// TestPressureBatchesInSeatOrder is RFC §6.5: "Pressure rolls... take
// seat and node index" — one draw per Legend.
func TestPressureBatchesInSeatOrder(t *testing.T) {
	s := incidentsTestState(0, 0)
	s.Players[0].Infamy, s.Players[1].Infamy = 9, 10
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), 5)

	pressure(&s, cfg, r)

	if got := r.Consumed(PurposePressureD6); got != 2 {
		t.Errorf("PurposePressureD6 consumed = %d, want 2 (one per Legend)", got)
	}
}

// TestPressureShiftChangeSuppressesTheRoll is GDD §14.2's Shift Change:
// "No Pressure roll this round."
func TestPressureShiftChangeSuppressesTheRoll(t *testing.T) {
	s := incidentsTestState(0)
	s.Players[0].Infamy = 10
	s.Round = 4
	s.Graph.EventDeck = []EventCardID{EventShiftChange}
	cfg := legalTestConfig()
	r := NewRNG(testSeed(1), 5)

	pressure(&s, cfg, r)

	if got := r.Consumed(PurposePressureD6); got != 0 {
		t.Errorf("PurposePressureD6 consumed = %d, want 0 (Shift Change suppresses it)", got)
	}
}

// TestPressureAppliesCashAndInfamyPenaltyOnATrigger is GDD §14.4: "On 1 or
// 2, the Guard finds you: −Cr$ 5 and −1 Infamy" — Threshold 6 here makes
// every roll a trigger, so the test needs no particular seed.
func TestPressureAppliesCashAndInfamyPenaltyOnATrigger(t *testing.T) {
	s := incidentsTestState(0)
	s.Players[0].Infamy = 10
	s.Players[0].Balance = 20
	cfg := legalTestConfig()
	cfg.Pressure = game.PressureConfig{Threshold: 6, CashPenalty: 5, InfamyPenalty: 1}
	r := NewRNG(testSeed(1), 5)

	pressure(&s, cfg, r)

	if got := s.Players[0].Balance; got != 15 {
		t.Errorf("Balance = %d, want 15 (20 - 5)", got)
	}
	if got := s.Players[0].Infamy; got != 9 {
		t.Errorf("Infamy = %d, want 9 (10 - 1)", got)
	}
}

// TestPressureReturnsDebtEventOnSurrender is the same fix
// TestResolveShakedownReturnsDebtEventOnSurrender documents, for
// Pressure's own applyDebt call site.
func TestPressureReturnsDebtEventOnSurrender(t *testing.T) {
	s := incidentsTestState(0)
	s.Players[0].Infamy = 10
	s.Players[0].Balance = 0
	s.Players[0].Posts = []game.NodeID{2}
	s.Graph.Nodes[2].Post = &Post{Owner: 0, RoundsRemaining: 3}
	cfg := legalTestConfig()
	cfg.Pressure = game.PressureConfig{Threshold: 6, CashPenalty: 5, InfamyPenalty: 1}
	r := NewRNG(testSeed(1), 5)

	events := pressure(&s, cfg, r)

	if len(events) != 1 || events[0].Kind != game.EventLeaseExpired || events[0].Node != 2 {
		t.Errorf("events = %+v, want one EventLeaseExpired at node 2 (Debt-forced surrender)", events)
	}
}

// TestIncidentSectorNeverFlaggedTwiceRunning is the issue's own acceptance
// criterion, verbatim: across a full 15-round match, on >= 1000 seeds, the
// same sector is never flagged twice running.
func TestIncidentSectorNeverFlaggedTwiceRunning(t *testing.T) {
	cfg := game.DefaultConfig()
	const seeds = 1000

	for i := range seeds {
		seed := testSeed(byte(i))
		seed[31] = byte(i >> 8)

		s, err := initial(seed, cfg, 2)
		if err != nil {
			t.Fatalf("seed %d: initial() error = %v", i, err)
		}

		// s.UnstableSector holds round 3's value from Setup onward, even
		// during rounds 1-2 before it is ever live (initialUnstableSector,
		// initial.go) — comparing it round to round only means something
		// once buildIncidentContext reports the round as actually live
		// (GDD §14.3: "From round 3 onward"); before that it is the same
		// unconsumed pre-draw, not a repeat.
		var last *game.Sector
		for round := 0; round < cfg.Rounds; round++ {
			if ctx := buildIncidentContext(s); ctx.live {
				if last != nil && *ctx.sector == *last {
					t.Fatalf("seed %d, round %d: sector flagged twice running: %v", i, round+1, *ctx.sector)
				}
				sector := *ctx.sector
				last = &sector
			}

			r := NewRNG(seed, int(s.Round)+1)
			next, _, err := Resolve(s, map[game.SeatID]game.Order{}, cfg, r)
			if err != nil {
				t.Fatalf("seed %d, round %d: Resolve() error = %v", i, round+1, err)
			}
			s = next
		}
	}
}
